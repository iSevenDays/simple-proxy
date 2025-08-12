package test

import (
	"bytes"
	"claude-proxy/config"
	"claude-proxy/proxy"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAnthropicStreamingFormat tests the Anthropic SSE streaming format
func TestAnthropicStreamingFormat(t *testing.T) {
	tests := []struct {
		name            string
		requestStream   bool
		expectStreaming bool
		expectedEvents  []string // Expected SSE event types
	}{
		{
			name:            "request stream=false",
			requestStream:   false,
			expectStreaming: false,
			expectedEvents:  nil, // No streaming events expected
		},
		{
			name:            "request stream=true", 
			requestStream:   true,
			expectStreaming: true,
			expectedEvents:  []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server that returns OpenAI streaming response
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check if backend request has streaming enabled
				var req map[string]interface{}
				json.NewDecoder(r.Body).Decode(&req)
				
				if stream, ok := req["stream"].(bool); ok && stream {
					// Return OpenAI streaming format
					w.Header().Set("Content-Type", "text/plain; charset=utf-8")
					fmt.Fprint(w, "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n")
					fmt.Fprint(w, "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"},\"finish_reason\":null}]}\n\n")
					fmt.Fprint(w, "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
					fmt.Fprint(w, "data: [DONE]\n\n")
				} else {
					// Return regular JSON response
					w.Header().Set("Content-Type", "application/json")
					response := map[string]interface{}{
						"id":      "chatcmpl-test",
						"object":  "chat.completion",
						"created": 1234567890,
						"model":   "test-model",
						"choices": []map[string]interface{}{
							{
								"index": 0,
								"message": map[string]interface{}{
									"role":    "assistant",
									"content": "Hello world",
								},
								"finish_reason": "stop",
							},
						},
					}
					json.NewEncoder(w).Encode(response)
				}
			}))
			defer mockServer.Close()

			// Create test configuration
			cfg := config.GetDefaultConfig()
			cfg.BigModel = "test-model"
			cfg.SmallModel = "test-model"
			cfg.CorrectionModel = "test-model"
			cfg.BigModelEndpoints = []string{mockServer.URL}
			cfg.SmallModelEndpoints = []string{mockServer.URL}
			cfg.ToolCorrectionEndpoints = []string{mockServer.URL}
			cfg.BigModelAPIKey = "test-key"
			cfg.SmallModelAPIKey = "test-key"
			cfg.ToolCorrectionAPIKey = "test-key"

			// Create handler
			handler := proxy.NewHandler(cfg, nil)

			// Create test request
			reqBody := map[string]interface{}{
				"model":      "claude-sonnet-4-20250514",
				"max_tokens": 100,
				"stream":     tt.requestStream,
				"messages": []map[string]interface{}{
					{
						"role":    "user",
						"content": "Hello",
					},
				},
			}
			
			reqJSON, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(reqJSON))
			req.Header.Set("Content-Type", "application/json")

			// Create response recorder
			rr := httptest.NewRecorder()

			// Handle request
			handler.HandleAnthropicRequest(rr, req)

			// Check response
			if tt.expectStreaming {
				// Should return SSE streaming format
				if !strings.Contains(rr.Header().Get("Content-Type"), "text/plain") {
					t.Errorf("Expected streaming content type, got: %s", rr.Header().Get("Content-Type"))
				}

				// Check for expected SSE events
				body := rr.Body.String()
				for _, eventType := range tt.expectedEvents {
					if !strings.Contains(body, fmt.Sprintf("event: %s", eventType)) {
						t.Errorf("Expected event type %s not found in response", eventType)
					}
				}

				// Verify SSE format structure
				if !strings.Contains(body, "event: message_start") {
					t.Error("Expected message_start event in streaming response")
				}
				if !strings.Contains(body, "data: ") {
					t.Error("Expected SSE data format in streaming response")
				}
			} else {
				// Should return regular JSON
				if !strings.Contains(rr.Header().Get("Content-Type"), "application/json") {
					t.Errorf("Expected JSON content type, got: %s", rr.Header().Get("Content-Type"))
				}

				// Should not contain SSE events
				body := rr.Body.String()
				if strings.Contains(body, "event:") {
					t.Error("Unexpected SSE events in non-streaming response")
				}
			}
		})
	}
}

// TestAnthropicStreamingEventStructure tests the exact structure of streaming events
func TestAnthropicStreamingEventStructure(t *testing.T) {
	// Create mock server that returns OpenAI streaming response
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		// Simulate OpenAI streaming chunks
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world!\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer mockServer.Close()

	// Create test configuration
	cfg := config.GetDefaultConfig()
	cfg.BigModel = "test-model"
	cfg.BigModelEndpoints = []string{mockServer.URL}
	cfg.BigModelAPIKey = "test-key"

	// Create handler
	handler := proxy.NewHandler(cfg, nil)

	// Create test request
	reqBody := map[string]interface{}{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 100,
		"stream":     true, // Client must explicitly request streaming to get SSE format
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "Say hello",
			},
		},
	}
	
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")

	// Create response recorder
	rr := httptest.NewRecorder()

	// Handle request
	handler.HandleAnthropicRequest(rr, req)

	// Parse streaming response
	body := rr.Body.String()
	lines := strings.Split(body, "\n")

	// Verify expected Anthropic SSE event structure
	expectedEvents := []struct {
		eventType string
		dataKey   string
	}{
		{"message_start", "message"},
		{"content_block_start", "content_block"},
		{"content_block_delta", "delta"},
		{"content_block_stop", "index"},
		{"message_delta", "delta"},
		{"message_stop", "type"},
	}

	for _, expected := range expectedEvents {
		found := false
		for _, line := range lines {
			if strings.HasPrefix(line, fmt.Sprintf("event: %s", expected.eventType)) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected event type %s not found", expected.eventType)
		}
	}

	// Verify message_start event structure
	if !strings.Contains(body, `"type":"message_start"`) {
		t.Error("Expected message_start event data structure")
	}

	// Verify content_block_delta event structure
	if !strings.Contains(body, `"type":"content_block_delta"`) {
		t.Error("Expected content_block_delta event data structure")
	}

	// Verify text_delta structure
	if !strings.Contains(body, `"type":"text_delta"`) {
		t.Error("Expected text_delta structure in content_block_delta")
	}
}

// TestAnthropicStreamingWithTools tests streaming responses with tool calls
func TestAnthropicStreamingWithTools(t *testing.T) {
	// Create mock server that returns OpenAI streaming response with tool calls
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		// Simulate OpenAI streaming chunks with tool calls
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_test\",\"type\":\"function\",\"function\":{\"name\":\"test_tool\",\"arguments\":\"\"}}]},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"param\\\":\\\"value\\\"}\"}}]},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1234567890,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer mockServer.Close()

	// Create test configuration
	cfg := config.GetDefaultConfig()
	cfg.BigModel = "test-model"
	cfg.BigModelEndpoints = []string{mockServer.URL}
	cfg.BigModelAPIKey = "test-key"

	// Create handler
	handler := proxy.NewHandler(cfg, nil)

	// Create test request with tools
	reqBody := map[string]interface{}{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 100,
		"stream":     true, // Client must explicitly request streaming to get SSE format
		"tools": []map[string]interface{}{
			{
				"name":        "test_tool",
				"description": "A test tool",
				"input_schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"param": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
		},
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "Use the test tool",
			},
		},
	}
	
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")

	// Create response recorder
	rr := httptest.NewRecorder()

	// Handle request
	handler.HandleAnthropicRequest(rr, req)

	// Parse streaming response
	body := rr.Body.String()

	// Verify tool_use content block in streaming response
	if !strings.Contains(body, `"type":"tool_use"`) {
		t.Error("Expected tool_use content block in streaming response")
	}

	// Verify input_json_delta events for tool parameters
	if !strings.Contains(body, `"type":"input_json_delta"`) {
		t.Error("Expected input_json_delta events for tool parameters")
	}
}

// Helper function to parse SSE events from response body
func parseSSEEvents(body string) []map[string]interface{} {
	var events []map[string]interface{}
	lines := strings.Split(body, "\n")
	
	var currentEvent map[string]interface{}
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if currentEvent != nil {
				events = append(events, currentEvent)
				currentEvent = nil
			}
			continue
		}
		
		if strings.HasPrefix(line, "event: ") {
			if currentEvent == nil {
				currentEvent = make(map[string]interface{})
			}
			currentEvent["event_type"] = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			if currentEvent == nil {
				currentEvent = make(map[string]interface{})
			}
			dataStr := strings.TrimPrefix(line, "data: ")
			var data interface{}
			if err := json.Unmarshal([]byte(dataStr), &data); err == nil {
				currentEvent["data"] = data
			} else {
				currentEvent["data_raw"] = dataStr
			}
		}
	}
	
	if currentEvent != nil {
		events = append(events, currentEvent)
	}
	
	return events
}