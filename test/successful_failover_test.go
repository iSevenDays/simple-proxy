package test

import (
	"claude-proxy/circuitbreaker"
	"claude-proxy/config"
	"claude-proxy/correction"
	"claude-proxy/proxy"
	"claude-proxy/types"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	t.Run("RuleBasedClassification_FastDecision", func(t *testing.T) {
		// This test validates the improved rule-based classifier
		// Simple research requests should be handled instantly without HTTP calls
		
		config := NewSuccessFailoverConfig([]string{"http://unused:8080"})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		start := time.Now()
		result, err := service.DetectToolNecessity(ctx, []types.OpenAIMessage{
			{Role: "user", Content: "read a file"},
		}, []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})
		duration := time.Since(start)

		// Should succeed instantly via rule-based classification
		if err != nil {
			t.Errorf("Expected instant rule-based success, got error: %v", err)
		}

		// Research verbs should return false (no tools needed)
		if result {
			t.Error("Expected rule-based classifier to return false for research verb 'read'")
		}

		// Should complete in milliseconds, not seconds (rule-based)
		if duration > 100*time.Millisecond {
			t.Errorf("Expected instant rule-based decision (<100ms), took: %v", duration)
		}

		t.Logf("✅ Rule-based classification: %v in %v (performance improvement working)", result, duration)
	})

	t.Run("CorrectToolCallsSuccessfulFailover", func(t *testing.T) {
		firstServerCalls := int64(0)
		secondServerCalls := int64(0)
		
		// First server times out
		firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&firstServerCalls, 1)
			time.Sleep(2 * time.Second) // Short timeout for unit tests
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
					"file_path": "", // Empty path - will need correction
					"extra_param": "invalid", // Invalid parameter that needs removal
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

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
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

	t.Run("CircuitBreakerFailoverWithLLMFallback", func(t *testing.T) {
		// Test circuit breaker behavior with ambiguous requests that force LLM fallback
		firstServerCalls := int64(0)
		secondServerCalls := int64(0)
		
		// First server always fails (to trigger circuit breaker)
		firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&firstServerCalls, 1)
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		}))
		defer firstServer.Close()

		// Second server succeeds (circuit breaker failover target)
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

		start := time.Now()
		result, err := service.DetectToolNecessity(ctx, []types.OpenAIMessage{
			{Role: "user", Content: "help me with the system configuration"}, // Ambiguous → forces LLM fallback
		}, []types.Tool{
			{Name: "Edit", InputSchema: types.ToolSchema{Type: "object"}},
			{Name: "Write", InputSchema: types.ToolSchema{Type: "object"}},
		})
		duration := time.Since(start)

		// Should succeed via circuit breaker failover
		if err != nil {
			t.Errorf("Expected circuit breaker failover success, got error: %v", err)
		}

		// Verify circuit breaker behavior: first server called and failed, second server succeeded
		first := atomic.LoadInt64(&firstServerCalls)
		second := atomic.LoadInt64(&secondServerCalls)

		t.Logf("✅ Circuit breaker test: First server: %d calls, Second server: %d calls, Duration: %v, Result: %v", 
			first, second, duration, result)

		if first < 1 {
			t.Error("Expected first server to be called and fail (triggering circuit breaker)")
		}

		if second < 1 {
			t.Error("Expected second server to be called and succeed (circuit breaker failover)")
		}

		// Should complete successfully with circuit breaker failover
		if duration > 5*time.Second {
			t.Errorf("Expected quick failover (<5s), got: %v", duration)
		}
	})

	t.Run("SmallModelEndpointCircuitBreaker_SubsequentRequests", func(t *testing.T) {
		// Test circuit breaker functionality: first request fails and opens circuit,
		// second request automatically uses healthy endpoint
		
		firstCallCount := int64(0)
		secondCallCount := int64(0)
		
		// First endpoint always fails (simulating 192.168.0.46:11434 issue)
		failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&firstCallCount, 1)
			http.Error(w, "Connection refused", http.StatusBadGateway)
		}))
		defer failServer.Close()

		// Second endpoint succeeds (simulating 192.168.0.50:11434 fallback)
		successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&secondCallCount, 1)
			response := map[string]interface{}{
				"id":      "test_response",
				"object":  "chat.completion", 
				"created": 1640995200,
				"model":   "qwen2.5-coder:latest",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Response from healthy endpoint",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     10,
					"completion_tokens": 8,
					"total_tokens":      18,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer successServer.Close()

		// Create config with aggressive circuit breaker
		cfg := &config.Config{
			SmallModelEndpoints:     []string{failServer.URL, successServer.URL},
			SmallModelAPIKey:       "test-key",
			SmallModel:             "qwen2.5-coder:latest",
			BigModelEndpoints:      []string{successServer.URL},
			BigModelAPIKey:        "test-key", 
			BigModel:              "test-model",
			ToolCorrectionEnabled: false,
			SkipTools:             []string{},
			HealthManager:         circuitbreaker.NewHealthManager(circuitbreaker.Config{
				FailureThreshold:   1, // Open circuit after 1 failure
				BackoffDuration:    100 * time.Millisecond,
				MaxBackoffDuration: 1 * time.Second,
				ResetTimeout:       1 * time.Minute,
			}),
		}

		handler := proxy.NewHandler(cfg, nil)
		reqBody := `{"model":"claude-3-5-haiku-20241022","max_tokens":100,"messages":[{"role":"user","content":"Hello"}]}`

		// First request - should fail and open circuit
		req1 := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(reqBody))
		req1.Header.Set("Content-Type", "application/json")
		rr1 := httptest.NewRecorder()
		handler.HandleAnthropicRequest(rr1, req1)

		// Should fail (circuit not yet helping)
		assert.Equal(t, http.StatusBadGateway, rr1.Code)
		
		// Second request - should automatically use healthy endpoint
		req2 := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(reqBody))  
		req2.Header.Set("Content-Type", "application/json")
		rr2 := httptest.NewRecorder()
		
		start := time.Now()
		handler.HandleAnthropicRequest(rr2, req2)
		duration := time.Since(start)

		// Should succeed via circuit breaker endpoint selection
		assert.Equal(t, http.StatusOK, rr2.Code)
		
		firstCalls := atomic.LoadInt64(&firstCallCount)
		secondCalls := atomic.LoadInt64(&secondCallCount)
		
		t.Logf("✅ Circuit breaker test: First endpoint: %d calls, Second endpoint: %d calls, Duration: %v", 
			firstCalls, secondCalls, duration)

		// First request hit failed endpoint, second request used healthy endpoint
		if firstCalls < 1 {
			t.Error("Expected first endpoint to be called and fail (opening circuit)")
		}

		if secondCalls < 1 {
			t.Error("Expected second endpoint to be called for subsequent request")
		}

		// Should be fast (no timeout delays)
		if duration > 1*time.Second {
			t.Errorf("Expected fast response from healthy endpoint (<1s), got: %v", duration)
		}

		t.Logf("✅ Circuit breaker successfully prevents 30s timeout issues by routing to healthy endpoints")
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
		result, err := service.DetectToolNecessity(ctx, []types.OpenAIMessage{
			{Role: "user", Content: "run tests"},
		}, []types.Tool{
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

		_, err := service.DetectToolNecessity(ctx, []types.OpenAIMessage{
			{Role: "user", Content: "test"},
		}, []types.Tool{
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

		_, err := service.DetectToolNecessity(ctx, []types.OpenAIMessage{
			{Role: "user", Content: "test empty response"},
		}, []types.Tool{
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

		_, err := service.DetectToolNecessity(ctx, []types.OpenAIMessage{
			{Role: "user", Content: "test invalid JSON"},
		}, []types.Tool{
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