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

// TestImmediateFailoverWithinRequest tests that when the first endpoint fails,
// the same request immediately tries the second endpoint without returning error
func TestImmediateFailoverWithinRequest(t *testing.T) {
	t.Run("SmallModelFailoverWithinSameRequest", func(t *testing.T) {
		// Create a failing first endpoint
		failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate connection timeout/failure
			time.Sleep(100 * time.Millisecond)
			http.Error(w, "First endpoint unavailable", http.StatusInternalServerError)
		}))
		defer failingServer.Close()

		// Create a working second endpoint
		workingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
							"content": "Success from second endpoint",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     10,
					"completion_tokens": 5,
					"total_tokens":      15,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer workingServer.Close()

		// Configure with failing endpoint first, working endpoint second
		cfg := &config.Config{
			BigModelEndpoints:        []string{"http://localhost:8081"}, // Not used
			BigModelAPIKey:          "test-key",
			BigModel:                "kimi-k2",
			SmallModelEndpoints:     []string{failingServer.URL, workingServer.URL}, // First fails, second works
			SmallModelAPIKey:       "test-key",
			SmallModel:             "qwen2.5-coder:latest",
			ToolCorrectionEnabled:  false,
			SkipTools:              []string{},
			DefaultConnectionTimeout: 15, // 15 seconds connection timeout
			HealthManager:          circuitbreaker.NewHealthManager(circuitbreaker.Config{
				FailureThreshold:   1, // Open circuit after 1 failure
				BackoffDuration:    100 * time.Millisecond,
				MaxBackoffDuration: 1 * time.Second,
				ResetTimeout:       1 * time.Minute,
			}),
		}

		handler := proxy.NewHandler(cfg, nil)

		// Make a small model request (claude-3-5-haiku-20241022 maps to qwen2.5-coder:latest)
		reqBody := `{
			"model": "claude-3-5-haiku-20241022",
			"max_tokens": 100,
			"messages": [{"role": "user", "content": "Test request"}]
		}`

		req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Execute request
		handler.HandleAnthropicRequest(rr, req)

		// CRITICAL: The request should succeed (200 OK) despite first endpoint failing
		// This proves immediate failover worked within the same request
		assert.Equal(t, http.StatusOK, rr.Code, "Request should succeed via immediate failover to second endpoint")

		// Verify response contains success content
		var response map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		assert.NoError(t, err, "Response should be valid JSON")

		// Check that we got content indicating success from second endpoint
		content := response["content"].([]interface{})
		textContent := content[0].(map[string]interface{})
		assert.Contains(t, textContent["text"], "Success from second endpoint", 
			"Response should show success from working endpoint")

		t.Logf("✅ Immediate failover test passed - request succeeded despite first endpoint failure")
	})

	t.Run("BigModelEndpointsShouldNotFailover", func(t *testing.T) {
		// Create a failing big model endpoint
		failingBigServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Big model endpoint failure", http.StatusInternalServerError)
		}))
		defer failingBigServer.Close()

		cfg := &config.Config{
			BigModelEndpoints:        []string{failingBigServer.URL}, // Only one big model endpoint
			BigModelAPIKey:          "test-key",
			BigModel:                "kimi-k2",
			SmallModelEndpoints:     []string{"http://localhost:8080"}, // Not used
			SmallModelAPIKey:       "test-key",
			SmallModel:             "qwen2.5-coder:latest",
			ToolCorrectionEnabled:  false,
			SkipTools:              []string{},
			DefaultConnectionTimeout: 15, // 15 seconds connection timeout
			HealthManager:          circuitbreaker.NewHealthManager(circuitbreaker.Config{
				FailureThreshold:   1,
				BackoffDuration:    100 * time.Millisecond,
				MaxBackoffDuration: 1 * time.Second,
				ResetTimeout:       1 * time.Minute,
			}),
		}

		handler := proxy.NewHandler(cfg, nil)

		// Make a big model request (claude-3-5-sonnet-20241022 maps to kimi-k2)
		reqBody := `{
			"model": "claude-3-5-sonnet-20241022",
			"max_tokens": 100,
			"messages": [{"role": "user", "content": "Test request"}]
		}`

		req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Execute request
		handler.HandleAnthropicRequest(rr, req)

		// Big model endpoints should fail immediately (no failover - 30min timeout acceptable)
		assert.Equal(t, http.StatusBadGateway, rr.Code, "Big model request should fail without failover")

		t.Logf("✅ Big model no-failover test passed - request failed as expected (no circuit breaker)")
	})
}