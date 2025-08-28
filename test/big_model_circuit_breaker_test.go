package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"claude-proxy/circuitbreaker"
	"claude-proxy/config"
	"claude-proxy/proxy"

	"github.com/stretchr/testify/assert"
)

// TestBigModelCircuitBreakerBypass verifies that big model endpoints bypass circuit breaker logic
// This allows for 30+ minute processing times without being blocked
func TestBigModelCircuitBreakerBypass(t *testing.T) {
	t.Run("BigModelEndpointsBypassCircuitBreaker", func(t *testing.T) {
		// Create a server that always returns 500 (to test circuit breaker bypass)
		failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Simulated big model failure", http.StatusInternalServerError)
		}))
		defer failingServer.Close()

		// Create config with failing big model endpoint  
		cfg := &config.Config{
			BigModelEndpoints:      []string{failingServer.URL}, // This will fail
			BigModelAPIKey:        "test-key",
			BigModel:              "kimi-k2",
			SmallModelEndpoints:   []string{"http://localhost:8080"}, // Not used in this test
			SmallModelAPIKey:     "test-key",
			SmallModel:           "qwen2.5-coder:latest",
			ToolCorrectionEnabled: false,
			SkipTools:            []string{},
			HealthManager:        circuitbreaker.NewHealthManager(circuitbreaker.Config{
				FailureThreshold:   1, // Very aggressive - would open circuit after 1 failure
				BackoffDuration:    100 * time.Millisecond,
				MaxBackoffDuration: 1 * time.Second,
				ResetTimeout:       1 * time.Minute,
			}),
		}

		handler := proxy.NewHandler(cfg, nil, "")

		// Test big model request (should bypass circuit breaker)
		reqBody := `{"model":"claude-3-5-sonnet-20241022","max_tokens":100,"messages":[{"role":"user","content":"Test"}]}`

		// Make multiple requests to the failing big model endpoint
		// Circuit breaker should NOT open because big models bypass circuit breaker logic
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handler.HandleAnthropicRequest(rr, req)

			// Should return 502 (bad gateway) but NOT be blocked by circuit breaker
			assert.Equal(t, http.StatusBadGateway, rr.Code, "Request %d should return 502, not be circuit breaker blocked", i+1)
		}

		t.Logf("✅ Big model endpoints correctly bypass circuit breaker - all 3 requests processed despite failures")
	})

	t.Run("SmallModelEndpointsStillUseCircuitBreaker", func(t *testing.T) {
		// Verify small model endpoints still use circuit breaker for comparison
		failingSmallServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Simulated small model failure", http.StatusInternalServerError)
		}))
		defer failingSmallServer.Close()

		workingSmallServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
							"content": "Response from small model",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     5,
					"completion_tokens": 8,
					"total_tokens":      13,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer workingSmallServer.Close()

		cfg := &config.Config{
			BigModelEndpoints:      []string{"http://localhost:8081"}, // Not used in this test
			BigModelAPIKey:        "test-key", 
			BigModel:              "kimi-k2",
			SmallModelEndpoints:   []string{failingSmallServer.URL, workingSmallServer.URL},
			SmallModelAPIKey:     "test-key",
			SmallModel:           "qwen2.5-coder:latest", 
			ToolCorrectionEnabled: false,
			SkipTools:            []string{},
			HealthManager:        circuitbreaker.NewHealthManager(circuitbreaker.Config{
				FailureThreshold:   1, // Open circuit after 1 failure
				BackoffDuration:    100 * time.Millisecond,
				MaxBackoffDuration: 1 * time.Second,
				ResetTimeout:       1 * time.Minute,
			}),
		}

		handler := proxy.NewHandler(cfg, nil, "")

		// Test small model request (should use circuit breaker)
		reqBody := `{"model":"claude-3-5-haiku-20241022","max_tokens":100,"messages":[{"role":"user","content":"Test"}]}`

		// First request should now succeed via immediate failover (improved behavior)
		req1 := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(reqBody))
		req1.Header.Set("Content-Type", "application/json") 
		rr1 := httptest.NewRecorder()
		handler.HandleAnthropicRequest(rr1, req1)
		assert.Equal(t, http.StatusOK, rr1.Code, "First small model request should succeed via immediate failover")

		// Second request should also succeed using circuit breaker routing
		req2 := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(reqBody))
		req2.Header.Set("Content-Type", "application/json")
		rr2 := httptest.NewRecorder()
		handler.HandleAnthropicRequest(rr2, req2)
		assert.Equal(t, http.StatusOK, rr2.Code, "Second small model request should succeed via circuit breaker")

		t.Logf("✅ Small model endpoints correctly use circuit breaker - failover working")
	})
}