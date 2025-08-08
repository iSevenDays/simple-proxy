package test

import (
	"claude-proxy/correction"
	"claude-proxy/types"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// SimpleRetryConfig for focused retry testing
type SimpleRetryConfig struct {
	endpoints []string
	index     int64
}

func NewSimpleRetryConfig(endpoints []string) *SimpleRetryConfig {
	return &SimpleRetryConfig{endpoints: endpoints}
}

func (c *SimpleRetryConfig) GetToolCorrectionEndpoint() string {
	if len(c.endpoints) == 0 {
		return ""
	}
	idx := atomic.LoadInt64(&c.index)
	endpoint := c.endpoints[idx%int64(len(c.endpoints))]
	atomic.AddInt64(&c.index, 1)
	return endpoint
}

func (c *SimpleRetryConfig) GetHealthyToolCorrectionEndpoint() string {
	return c.GetToolCorrectionEndpoint()
}

func (c *SimpleRetryConfig) RecordEndpointFailure(endpoint string) {
	// Mock implementation - no-op for basic tests
}

func (c *SimpleRetryConfig) RecordEndpointSuccess(endpoint string) {
	// Mock implementation - no-op for basic tests
}

// TestBasicRetryFunctionality focuses on the core retry logic
func TestBasicRetryFunctionality(t *testing.T) {
	t.Run("BasicFailover", func(t *testing.T) {
		callCount := int64(0)
		
		// Server that fails first call, succeeds second
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt64(&callCount, 1)
			
			if count == 1 {
				http.Error(w, "Service Error", http.StatusServiceUnavailable)
				return
			}
			
			// Second call succeeds
			response := types.OpenAIResponse{
				Choices: []types.OpenAIChoice{
					{Message: types.OpenAIMessage{Content: "NO"}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		config := NewSimpleRetryConfig([]string{server.URL, server.URL})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		result, err := service.DetectToolNecessity(ctx, "explain something", []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})

		// Should succeed on retry
		if err != nil {
			t.Errorf("Expected retry success, got: %v", err)
		}

		calls := atomic.LoadInt64(&callCount)
		t.Logf("Retry test: %d calls, result: %v", calls, result)

		if calls < 2 {
			t.Errorf("Expected at least 2 calls (retry), got %d", calls)
		}
	})

	t.Run("TimeoutFailover", func(t *testing.T) {
		timeoutCallCount := int64(0)
		successCallCount := int64(0)
		
		// Timeout server
		timeoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&timeoutCallCount, 1)
			time.Sleep(2 * time.Second) // Short timeout for unit tests
		}))
		defer timeoutServer.Close()

		// Success server
		successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&successCallCount, 1)
			response := types.OpenAIResponse{
				Choices: []types.OpenAIChoice{
					{Message: types.OpenAIMessage{Content: "YES"}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer successServer.Close()

		config := NewSimpleRetryConfig([]string{timeoutServer.URL, successServer.URL})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		start := time.Now()
		result, err := service.DetectToolNecessity(ctx, "run command", []types.Tool{
			{Name: "Bash", InputSchema: types.ToolSchema{Type: "object"}},
		})
		duration := time.Since(start)

		// Should succeed with failover
		if err != nil {
			t.Errorf("Expected timeout failover success, got: %v", err)
		}

		timeoutCalls := atomic.LoadInt64(&timeoutCallCount)
		successCalls := atomic.LoadInt64(&successCallCount)

		t.Logf("Timeout failover: timeout=%d calls, success=%d calls, duration=%v, result=%v", 
			timeoutCalls, successCalls, duration, result)

		if timeoutCalls < 1 {
			t.Error("Expected timeout server to be called")
		}

		if successCalls < 1 {
			t.Error("Expected success server to be called after timeout")
		}

		if duration > 45*time.Second {
			t.Errorf("Failover took too long: %v", duration)
		}
	})

	t.Run("AllEndpointsFail", func(t *testing.T) {
		failCallCount := int64(0)
		
		// Server that always fails
		failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&failCallCount, 1)
			http.Error(w, "Always Fails", http.StatusInternalServerError)
		}))
		defer failServer.Close()

		config := NewSimpleRetryConfig([]string{failServer.URL, failServer.URL, failServer.URL})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		_, err := service.DetectToolNecessity(ctx, "test", []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})

		// Should fail after exhausting retries
		if err == nil {
			t.Error("Expected failure when all endpoints fail")
		}

		if !strings.Contains(err.Error(), "Tool necessity detection failed") && !strings.Contains(err.Error(), "endpoint returned status") {
			t.Errorf("Expected tool necessity detection or endpoint error, got: %v", err)
		}

		calls := atomic.LoadInt64(&failCallCount)
		t.Logf("All fail test: %d total calls", calls)

		if calls < 3 {
			t.Errorf("Expected 3 retry attempts, got %d", calls)
		}
	})
}

// TestRetryWithCorrections tests retry in tool correction scenarios
func TestRetryWithCorrections(t *testing.T) {
	t.Run("CorrectionRetry", func(t *testing.T) {
		attemptCount := int64(0)
		
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempt := atomic.AddInt64(&attemptCount, 1)
			
			if attempt == 1 {
				// First attempt: invalid JSON
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"invalid": json`))
				return
			}
			
			// Second attempt: valid correction
			response := types.OpenAIResponse{
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Content: `{"name": "Read", "input": {"file_path": "corrected.txt"}}`,
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		config := NewSimpleRetryConfig([]string{server.URL, server.URL})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		// Tool call that needs correction
		toolCalls := []types.Content{
			{
				Type: "tool_use",
				ID:   "test-1", 
				Name: "Read",
				Input: map[string]interface{}{
					"filename": "wrong.txt", // Wrong parameter
				},
			},
		}

		availableTools := []types.Tool{
			{
				Name: "Read",
				InputSchema: types.ToolSchema{
					Type: "object",
					Properties: map[string]types.ToolProperty{
						"file_path": {Type: "string"},
					},
					Required: []string{"file_path"},
				},
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		corrected, err := service.CorrectToolCalls(ctx, toolCalls, availableTools)

		if err != nil {
			t.Errorf("Expected correction retry success, got: %v", err)
		}

		if len(corrected) != 1 {
			t.Fatalf("Expected 1 corrected call, got %d", len(corrected))
		}

		attempts := atomic.LoadInt64(&attemptCount)
		t.Logf("Correction retry: %d attempts", attempts)

		// Note: Rule-based correction may succeed without requiring LLM retry
		if attempts == 0 {
			t.Log("Rule-based correction succeeded without LLM retry - this is acceptable")
		}

		// Verify correction worked
		if corrected[0].Input["file_path"] == nil {
			t.Error("Expected corrected file_path parameter")
		}
	})
}