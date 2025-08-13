package test

import (
	"bytes"
	"claude-proxy/circuitbreaker"
	"claude-proxy/config"
	"claude-proxy/proxy"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProxyIntegration tests end-to-end proxy functionality
// Following SPARC: Integration test covering the complete workflow
func TestProxyIntegration(t *testing.T) {
	// Create test configuration
	cfg := &config.Config{
		BigModelEndpoints:       []string{"http://mock-kimi"},
		BigModelAPIKey:         "test-key",
		SmallModelEndpoints:     []string{"http://mock-qwen"},
		SmallModelAPIKey:       "qwen-key",
		ToolCorrectionEndpoints: []string{"http://mock-correction"},
		ToolCorrectionAPIKey:   "correction-key",
		BigModel:               "kimi-k2",
		SmallModel:             "qwen2.5-coder:latest",
		CorrectionModel:        "qwen2.5-coder:latest",
		Port:                   "3456",
		ToolCorrectionEnabled:  false, // Disable for basic integration test
		SkipTools:              []string{},
		HealthManager:          circuitbreaker.NewHealthManager(circuitbreaker.DefaultConfig()),
	}

	// Create mock backend server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request format
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		// Parse request to verify transformation
		var req map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify OpenAI format
		assert.Equal(t, "kimi-k2", req["model"])
		assert.Contains(t, req, "messages")

		// Send mock OpenAI response
		response := map[string]interface{}{
			"id":      "test_response_123",
			"object":  "chat.completion",
			"created": 1640995200,
			"model":   "kimi-k2",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello! This is a test response.",
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
	defer mockServer.Close()

	// Update config to use mock server
	cfg.BigModelEndpoints = []string{mockServer.URL}

	// Create handler
	handler := proxy.NewHandler(cfg, nil, nil)

	// Create test request (Anthropic format)
	requestBody := map[string]interface{}{
		"model":      "kimi-k2",
		"max_tokens": 100,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "Hello, how are you?", // String format
			},
		},
	}

	reqJSON, err := json.Marshal(requestBody)
	require.NoError(t, err)

	// Create HTTP request
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewBuffer(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	// Create response recorder
	rr := httptest.NewRecorder()

	// Call handler
	handler.HandleAnthropicRequest(rr, req)

	// Verify response
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	// Parse response
	var response map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify Anthropic format response
	assert.Equal(t, "test_response_123", response["id"])
	assert.Equal(t, "message", response["type"])
	assert.Equal(t, "assistant", response["role"])
	assert.Equal(t, "kimi-k2", response["model"])
	assert.Equal(t, "end_turn", response["stop_reason"])

	// Verify content
	content, ok := response["content"].([]interface{})
	require.True(t, ok)
	require.Len(t, content, 1)

	contentItem, ok := content[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "text", contentItem["type"])
	assert.Equal(t, "Hello! This is a test response.", contentItem["text"])

	// Verify usage
	usage, ok := response["usage"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(10), usage["input_tokens"])
	assert.Equal(t, float64(8), usage["output_tokens"])
}

// TestProxyToolCallIntegration tests tool calling workflow
func TestProxyToolCallIntegration(t *testing.T) {
	cfg := &config.Config{
		BigModelEndpoints:       []string{"http://mock-kimi"},
		BigModelAPIKey:         "test-key",
		SmallModelEndpoints:     []string{"http://mock-qwen"},
		SmallModelAPIKey:       "qwen-key",
		ToolCorrectionEndpoints: []string{"http://mock-correction"},
		ToolCorrectionAPIKey:   "correction-key",
		BigModel:               "kimi-k2",
		SmallModel:             "qwen2.5-coder:latest",
		CorrectionModel:        "qwen2.5-coder:latest",
		Port:                   "3456",
		ToolCorrectionEnabled:  false,
		SkipTools:              []string{},
		HealthManager:          circuitbreaker.NewHealthManager(circuitbreaker.DefaultConfig()),
	}

	// Mock server that returns tool calls
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id":      "tool_response_456",
			"object":  "chat.completion",
			"created": 1640995200,
			"model":   "kimi-k2",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role": "assistant",
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_write_123",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "Write",
									"arguments": `{"file_path":"output.txt","content":"Generated content"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     20,
				"completion_tokens": 15,
				"total_tokens":      35,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	cfg.BigModelEndpoints = []string{mockServer.URL}
	handler := proxy.NewHandler(cfg, nil, nil)

	// Request with tools
	requestBody := map[string]interface{}{
		"model":      "kimi-k2",
		"max_tokens": 200,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "Please write 'Hello World' to output.txt", // String format
			},
		},
		"tools": []map[string]interface{}{
			{
				"name":        "Write",
				"description": "Writes content to a file",
				"input_schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"file_path": map[string]interface{}{"type": "string"},
						"content":   map[string]interface{}{"type": "string"},
					},
					"required": []string{"file_path", "content"},
				},
			},
		},
	}

	reqJSON, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewBuffer(reqJSON))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.HandleAnthropicRequest(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify tool_use response
	assert.Equal(t, "tool_use", response["stop_reason"])

	content, ok := response["content"].([]interface{})
	require.True(t, ok)
	require.Len(t, content, 1)

	toolUse, ok := content[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "tool_use", toolUse["type"])
	assert.Equal(t, "call_write_123", toolUse["id"])
	assert.Equal(t, "Write", toolUse["name"])

	input, ok := toolUse["input"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "output.txt", input["file_path"])
	assert.Equal(t, "Generated content", input["content"])
}

// TestProxyErrorHandling tests error scenarios
func TestProxyErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "invalid_json_returns_400",
			requestBody:    `{invalid json}`,
			expectedStatus: 400,
			expectedError:  "Invalid request format",
		},
		{
			name: "missing_model_handled_gracefully",
			requestBody: `{
				"messages": [{"role": "user", "content": "hello"}]
			}`,
			expectedStatus: 200, // Will be handled by our proxy validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server for missing model test to avoid hanging on real server
			var cfg *config.Config
			if tt.name == "missing_model_handled_gracefully" {
				mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Mock response for empty model test
					response := map[string]interface{}{
						"id":      "test_empty_model",
						"object":  "chat.completion",
						"created": 1640995200,
						"model":   "default-model",
						"choices": []map[string]interface{}{
							{
								"index": 0,
								"message": map[string]interface{}{
									"role":    "assistant",
									"content": "Handled empty model gracefully.",
								},
								"finish_reason": "stop",
							},
						},
						"usage": map[string]interface{}{
							"prompt_tokens":     5,
							"completion_tokens": 6,
							"total_tokens":      11,
						},
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(response)
				}))
				defer mockServer.Close()
				
				cfg = &config.Config{
					BigModelEndpoints:       []string{mockServer.URL},
					BigModelAPIKey:         "test-key",
					SmallModelEndpoints:     []string{mockServer.URL},
					SmallModelAPIKey:       "test-key",
					ToolCorrectionEndpoints: []string{mockServer.URL},
					ToolCorrectionAPIKey:   "test-key",
					BigModel:               "kimi-k2",
					SmallModel:             "qwen2.5-coder:latest",
					CorrectionModel:        "qwen2.5-coder:latest",
					Port:                   "3456",
					ToolCorrectionEnabled:  false,
					SkipTools:              []string{},
					HealthManager:          circuitbreaker.NewHealthManager(circuitbreaker.DefaultConfig()),
				}
			} else {
				cfg = config.GetDefaultConfig()
			}
			
			handler := proxy.NewHandler(cfg, nil, nil)
			req := httptest.NewRequest("POST", "/v1/messages", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.HandleAnthropicRequest(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedError != "" {
				assert.Contains(t, rr.Body.String(), tt.expectedError)
			}
		})
	}
}

// TestMethodNotAllowed tests HTTP method validation
func TestMethodNotAllowed(t *testing.T) {
	cfg := config.GetDefaultConfig()
	handler := proxy.NewHandler(cfg, nil, nil)

	req := httptest.NewRequest("GET", "/v1/messages", nil)
	rr := httptest.NewRecorder()

	handler.HandleAnthropicRequest(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}
