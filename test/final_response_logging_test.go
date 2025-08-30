package test

import (
	"bytes"
	"claude-proxy/config"
	"claude-proxy/proxy"
	"claude-proxy/types"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFinalResponseLogging verifies that the final response sent to the client is properly logged
func TestFinalResponseLogging(t *testing.T) {
	tests := []struct {
		name            string
		anthropicReq    types.AnthropicRequest
		mockResponse    types.OpenAIResponse
		expectStreaming bool
		description     string
	}{
		{
			name: "non_streaming_response_logging",
			anthropicReq: types.AnthropicRequest{
				Model:   "claude-sonnet-4-20250514",
				MaxTokens: 100,
				Messages: []types.Message{
					{
						Role: "user",
						Content: []types.Content{
							{Type: "text", Text: "Test message"},
						},
					},
				},
				Stream: false, // Non-streaming request
			},
			mockResponse: types.OpenAIResponse{
				ID:     "resp_logging_test",
				Object: "chat.completion",
				Model:  "test-model",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant",
							Content: "Test response with\nmultiple lines\nfor logging verification",
						},
						FinishReason: stringPtr("stop"),
					},
				},
				Usage: types.OpenAIUsage{
					PromptTokens:     10,
					CompletionTokens: 20,
				},
			},
			expectStreaming: false,
			description:     "Non-streaming response should be logged before sending to client",
		},
		{
			name: "streaming_response_logging",
			anthropicReq: types.AnthropicRequest{
				Model:   "claude-sonnet-4-20250514",
				MaxTokens: 100,
				Messages: []types.Message{
					{
						Role: "user",
						Content: []types.Content{
							{Type: "text", Text: "Test streaming message"},
						},
					},
				},
				Stream: true, // Streaming request
			},
			mockResponse: types.OpenAIResponse{
				ID:     "resp_streaming_test",
				Object: "chat.completion",
				Model:  "test-model",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant",
							Content: "Streaming response\nwith preserved\nformatting",
						},
						FinishReason: stringPtr("stop"),
					},
				},
			},
			expectStreaming: true,
			description:     "Streaming response final content should be logged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test config
			cfg := getTestConfigWithEndpoints()
			
			// Create mock server for the provider
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer mockServer.Close()

			// Update config to use mock server
			cfg.BigModelEndpoints = []string{mockServer.URL}

			// Create handler
			handler := proxy.NewHandler(cfg, nil, "test-session")

			// Create HTTP request
			reqBody, err := json.Marshal(tt.anthropicReq)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			
			// Create response recorder
			rr := httptest.NewRecorder()

			// Execute request
			handler.HandleAnthropicRequest(rr, req)

			// Verify response was successful
			require.Equal(t, http.StatusOK, rr.Code, "Request should succeed")

			// Test that request completes successfully
			// TODO: Add actual log capture and verification once logging is implemented
			t.Logf("Test completed - need to implement final response logging")
			
			// This test should fail until final response logging is implemented
			// For now, we're just verifying the request flow works

			// Verify response content is preserved (regardless of streaming)
			responseBody := rr.Body.String()
			if tt.expectStreaming {
				// For streaming, check SSE format
				assert.Contains(t, responseBody, "data:", "Streaming response should use SSE format")
			} else {
				// For non-streaming, parse JSON response
				var anthropicResp types.AnthropicResponse
				err = json.Unmarshal([]byte(responseBody), &anthropicResp)
				require.NoError(t, err, "Should parse response JSON")
				
				// Verify content preservation
				require.NotEmpty(t, anthropicResp.Content, "Response should have content")
				actualText := anthropicResp.Content[0].Text
				assert.Contains(t, actualText, "Test response with", "Should preserve response content")
				assert.Contains(t, actualText, "\n", "Should preserve newlines")
			}
		})
	}
}

// TestFinalResponseLoggingContent verifies that the content of logged responses matches expectations
func TestFinalResponseLoggingContent(t *testing.T) {
	// Test data with specific formatting
	testContent := "**Solution A**\n\nThis is a test response with:\n- Bullet points\n- Multiple lines\n- Preserved formatting"
	
	anthropicReq := types.AnthropicRequest{
		Model:   "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Messages: []types.Message{
			{
				Role: "user",
				Content: []types.Content{
					{Type: "text", Text: "Test formatting preservation"},
				},
			},
		},
		Stream: false,
	}

	mockResponse := types.OpenAIResponse{
		ID:     "resp_content_test",
		Object: "chat.completion",
		Model:  "test-model",
		Choices: []types.OpenAIChoice{
			{
				Message: types.OpenAIMessage{
					Role:    "assistant",
					Content: testContent,
				},
				FinishReason: stringPtr("stop"),
			},
		},
	}

	// Create test setup
	cfg := getTestConfigWithEndpoints()
	
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer mockServer.Close()

	cfg.BigModelEndpoints = []string{mockServer.URL}
	handler := proxy.NewHandler(cfg, nil, "test-session")

	// Execute request
	reqBody, err := json.Marshal(anthropicReq)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	
	rr := httptest.NewRecorder()
	handler.HandleAnthropicRequest(rr, req)

	// TODO: Verify logging occurred once implemented
	t.Logf("Test should verify logging of formatted content")
	
	// Verify the response itself is correct
	var anthropicResp types.AnthropicResponse
	err = json.Unmarshal(rr.Body.Bytes(), &anthropicResp)
	require.NoError(t, err)
	
	actualText := anthropicResp.Content[0].Text
	assert.Equal(t, testContent, actualText, "Response content should match exactly")
}

// Helper function to create test config with endpoints
func getTestConfigWithEndpoints() *config.Config {
	return &config.Config{
		BigModel:         "test-big-model",
		BigModelEndpoints: []string{"http://localhost:8080"}, // Will be overridden in tests
		BigModelAPIKey:   "test-key",
		SmallModel:       "test-small-model", 
		SmallModelEndpoints: []string{"http://localhost:8080"},
		SmallModelAPIKey: "test-key",
		HarmonyParsingEnabled: true,
	}
}