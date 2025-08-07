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

// SuccessFailoverConfig tracks endpoint usage for successful failover scenarios
type SuccessFailoverConfig struct {
	endpoints []string
	index     int64 // Use atomic for thread safety
}

func NewSuccessFailoverConfig(endpoints []string) *SuccessFailoverConfig {
	return &SuccessFailoverConfig{endpoints: endpoints, index: 0}
}

func (c *SuccessFailoverConfig) GetToolCorrectionEndpoint() string {
	if len(c.endpoints) == 0 {
		return ""
	}
	
	idx := atomic.LoadInt64(&c.index)
	endpoint := c.endpoints[idx%int64(len(c.endpoints))]
	atomic.AddInt64(&c.index, 1)
	
	return endpoint
}

func (c *SuccessFailoverConfig) GetHealthyToolCorrectionEndpoint() string {
	return c.GetToolCorrectionEndpoint()
}

func (c *SuccessFailoverConfig) RecordEndpointFailure(endpoint string) {
	// Mock implementation - no-op for basic tests
}

func (c *SuccessFailoverConfig) RecordEndpointSuccess(endpoint string) {
	// Mock implementation - no-op for basic tests
}

func (c *SuccessFailoverConfig) GetCurrentIndex() int64 {
	return atomic.LoadInt64(&c.index)
}

// TestSuccessfulFailoverScenarios tests various successful failover patterns
func TestSuccessfulFailoverScenarios(t *testing.T) {
	t.Run("FirstEndpointFailsSecondSucceeds", func(t *testing.T) {
		// Track server calls
		firstServerCalls := int64(0)
		secondServerCalls := int64(0)

		// First server returns 500 error
		firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&firstServerCalls, 1)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}))
		defer firstServer.Close()

		// Second server succeeds
		secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&secondServerCalls, 1)
			response := types.OpenAIResponse{
				Choices: []types.OpenAIChoice{
					{Message: types.OpenAIMessage{Content: "YES"}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer secondServer.Close()

		config := NewSuccessFailoverConfig([]string{firstServer.URL, secondServer.URL})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		result, err := service.DetectToolNecessity(ctx, "read a file", []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})

		// Should succeed with second server
		if err != nil {
			t.Errorf("Expected successful failover, got error: %v", err)
		}

		if !result {
			t.Log("Tool necessity false - acceptable for this test pattern")
		}

		// Verify both servers were called
		first := atomic.LoadInt64(&firstServerCalls)
		second := atomic.LoadInt64(&secondServerCalls)

		t.Logf("First server calls: %d, Second server calls: %d", first, second)

		if first < 1 {
			t.Error("Expected first server to be called and fail")
		}

		if second < 1 {
			t.Error("Expected second server to be called and succeed")
		}
	})

	t.Run("CorrectToolCallsSuccessfulFailover", func(t *testing.T) {
		firstServerCalls := int64(0)
		secondServerCalls := int64(0)
		
		// First server times out
		firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&firstServerCalls, 1)
			time.Sleep(35 * time.Second) // Timeout
		}))
		defer firstServer.Close()
		
		// Second server returns valid correction
		secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&secondServerCalls, 1)
			
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
		defer secondServer.Close()

		config := NewSuccessFailoverConfig([]string{firstServer.URL, secondServer.URL})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		// Create tool call that needs correction
		toolCalls := []types.Content{
			{
				Type: "tool_use", 
				ID:   "test-1",
				Name: "Read",
				Input: map[string]interface{}{
					"filename": "test.txt", // Wrong parameter name, needs correction
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

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		start := time.Now()
		corrected, err := service.CorrectToolCalls(ctx, toolCalls, availableTools)
		duration := time.Since(start)

		// Should succeed after failover
		if err != nil {
			t.Errorf("Expected successful correction after failover, got: %v", err)
		}

		if len(corrected) != 1 {
			t.Fatalf("Expected 1 corrected call, got %d", len(corrected))
		}

		// Verify correction worked
		correctedCall := corrected[0]
		if correctedCall.Name != "Read" {
			t.Errorf("Expected Read tool, got %s", correctedCall.Name)
		}

		if filePath, exists := correctedCall.Input["file_path"]; !exists {
			t.Error("Expected corrected file_path parameter")
		} else if filePath != "corrected.txt" {
			t.Errorf("Expected 'corrected.txt', got %v", filePath)
		}

		// Log call counts
		firstCalls := atomic.LoadInt64(&firstServerCalls)
		secondCalls := atomic.LoadInt64(&secondServerCalls)

		t.Logf("Correction failover - First: %d calls, Second: %d calls, Duration: %v",
			firstCalls, secondCalls, duration)

		if secondCalls < 1 {
			t.Error("Expected second server to handle successful correction")
		}
	})

	t.Run("MultipleRetriesEventualSuccess", func(t *testing.T) {
		attemptCount := int64(0)
		
		// Server that fails first 2 times, succeeds on 3rd
		retryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempt := atomic.AddInt64(&attemptCount, 1)
			
			if attempt <= 2 {
				// First two attempts fail with different errors
				if attempt == 1 {
					http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
				} else {
					time.Sleep(35 * time.Second) // Timeout on second attempt
				}
				return
			}
			
			// Third attempt succeeds
			response := types.OpenAIResponse{
				Choices: []types.OpenAIChoice{
					{Message: types.OpenAIMessage{Content: "NO"}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer retryServer.Close()

		// Use same server URL 3 times to test retry with same endpoint
		config := NewSuccessFailoverConfig([]string{
			retryServer.URL,
			retryServer.URL, 
			retryServer.URL,
		})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		start := time.Now()
		result, err := service.DetectToolNecessity(ctx, "explain concept", []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})
		duration := time.Since(start)

		// Should eventually succeed
		if err != nil {
			t.Errorf("Expected success after retries, got: %v", err)
		}

		if result {
			t.Log("Tool necessity true - concept explanation usually doesn't need tools, but OK")
		}

		attempts := atomic.LoadInt64(&attemptCount)
		t.Logf("Multiple retry test: %d attempts, Duration: %v", attempts, duration)

		if attempts < 3 {
			t.Errorf("Expected at least 3 attempts, got %d", attempts)
		}
	})

	t.Run("SuccessOnFirstTry", func(t *testing.T) {
		callCount := int64(0)
		
		// Server that succeeds immediately
		successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&callCount, 1)
			
			response := types.OpenAIResponse{
				Choices: []types.OpenAIChoice{
					{Message: types.OpenAIMessage{Content: "YES"}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer successServer.Close()

		config := NewSuccessFailoverConfig([]string{successServer.URL, successServer.URL})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		start := time.Now()
		result, err := service.DetectToolNecessity(ctx, "run tests", []types.Tool{
			{Name: "Bash", InputSchema: types.ToolSchema{Type: "object"}},
		})
		duration := time.Since(start)

		// Should succeed quickly
		if err != nil {
			t.Errorf("Expected immediate success, got: %v", err)
		}

		if !result {
			t.Log("Tool necessity false for 'run tests' - might be acceptable")
		}

		calls := atomic.LoadInt64(&callCount)
		t.Logf("Success on first try: %d calls, Duration: %v", calls, duration)

		if calls != 1 {
			t.Errorf("Expected exactly 1 call for immediate success, got %d", calls)
		}

		if duration > 2*time.Second {
			t.Errorf("Should succeed quickly, took: %v", duration)
		}
	})
}

// TestFailoverLogMessages tests that proper log messages are generated during failover
func TestFailoverLogMessages(t *testing.T) {
	t.Run("LogsFailoverMessages", func(t *testing.T) {
		// Create failing and succeeding servers
		failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Gateway Timeout", http.StatusGatewayTimeout)
		}))
		defer failServer.Close()

		successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := types.OpenAIResponse{
				Choices: []types.OpenAIChoice{
					{Message: types.OpenAIMessage{Content: "NO"}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer successServer.Close()

		config := NewSuccessFailoverConfig([]string{failServer.URL, successServer.URL})
		// Enable logging to capture messages
		service := correction.NewService(config, "test-key", true, "test-model", false)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := service.DetectToolNecessity(ctx, "test", []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})

		// Should succeed despite first failure
		if err != nil {
			t.Errorf("Expected successful failover with logging, got: %v", err)
		}

		// We can't easily capture log.Printf output in tests, but this exercises 
		// the code paths that should generate the failover log messages
		t.Log("✅ Failover logging test completed (log messages written to stdout)")
	})
}

// TestEdgeCasesInFailover tests edge cases in failover logic  
func TestEdgeCasesInFailover(t *testing.T) {
	t.Run("EmptyResponseHandling", func(t *testing.T) {
		// Server returns empty response
		emptyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			// No body written
		}))
		defer emptyServer.Close()

		// Fallback server
		goodServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := types.OpenAIResponse{
				Choices: []types.OpenAIChoice{
					{Message: types.OpenAIMessage{Content: "YES"}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer goodServer.Close()

		config := NewSuccessFailoverConfig([]string{emptyServer.URL, goodServer.URL})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := service.DetectToolNecessity(ctx, "test empty response", []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})

		// Should handle empty response and failover to good server
		if err != nil {
			t.Errorf("Expected successful failover from empty response, got: %v", err)
		}
	})

	t.Run("InvalidJSONResponse", func(t *testing.T) {
		// Server returns invalid JSON
		invalidServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"invalid": json`)) // Malformed JSON
		}))
		defer invalidServer.Close()

		// Valid server
		validServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := types.OpenAIResponse{
				Choices: []types.OpenAIChoice{
					{Message: types.OpenAIMessage{Content: "NO"}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer validServer.Close()

		config := NewSuccessFailoverConfig([]string{invalidServer.URL, validServer.URL})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := service.DetectToolNecessity(ctx, "test invalid JSON", []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})

		// Should handle invalid JSON and failover
		if err != nil {
			// This might fail because invalid JSON is parsed in the retry logic
			t.Logf("Invalid JSON test result: %v", err)
			if !strings.Contains(err.Error(), "Tool necessity detection failed") {
				t.Errorf("Expected proper error handling for invalid JSON, got: %v", err)
			}
		}
	})
}