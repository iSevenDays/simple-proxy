package test

import (
	"bytes"
	"claude-proxy/config"
	"claude-proxy/proxy"
	"claude-proxy/types"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoopDetectionIntegration_TodoWriteLoop(t *testing.T) {
	// Create a minimal config for testing
	cfg := &config.Config{
		BigModel:              "test-model",
		BigModelEndpoint:      "http://localhost/v1/completions",
		BigModelAPIKey:        "test-key",
		SmallModel:            "test-small-model",
		SmallModelEndpoint:    "http://localhost/v1/completions",
		SmallModelAPIKey:      "test-key",
		ToolCorrectionEnabled: false, // Disable to focus on loop detection
	}

	// Create handler
	handler := proxy.NewHandler(cfg, nil)

	// Create a request that simulates the loop pattern from the logs
	// This matches the actual Anthropic API format that Claude Code sends
	anthropicReq := types.AnthropicRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []types.Message{
			{Role: "user", Content: "help me find a way to fix some loops that Claude Code goes into"},
			{Role: "assistant", Content: []types.Content{{Type: "tool_use", ID: "1", Name: "TodoWrite", Input: map[string]interface{}{"todos": []interface{}{}}}}},
			{Role: "user", Content: []types.Content{{Type: "tool_result", ToolUseID: "1", Text: "Todos have been modified successfully. Ensure that you continue to use the todo list to track your progress."}}},
			{Role: "assistant", Content: []types.Content{{Type: "tool_use", ID: "2", Name: "TodoWrite", Input: map[string]interface{}{"todos": []interface{}{}}}}},
			{Role: "user", Content: []types.Content{{Type: "tool_result", ToolUseID: "2", Text: "Todos have been modified successfully. Ensure that you continue to use the todo list to track your progress."}}},
			{Role: "assistant", Content: []types.Content{{Type: "tool_use", ID: "3", Name: "TodoWrite", Input: map[string]interface{}{"todos": []interface{}{}}}}},
		},
		Tools: []types.Tool{
			{
				Name:        "TodoWrite",
				Description: "Use this tool to create and manage a structured task list",
				InputSchema: types.ToolSchema{
					Type: "object",
					Properties: map[string]types.ToolProperty{
						"todos": {Type: "array", Description: "List of todos"},
					},
				},
			},
		},
	}

	// Marshal request
	reqBody, err := json.Marshal(anthropicReq)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	// Create HTTP request
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	// Create response recorder
	w := httptest.NewRecorder()

	// Call handler
	handler.HandleAnthropicRequest(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		return
	}

	// Parse response
	var response types.AnthropicResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Check that response contains loop detection message
	if len(response.Content) == 0 {
		t.Fatal("Expected content in response")
	}

	if response.Content[0].Type != "text" {
		t.Errorf("Expected text content, got %s", response.Content[0].Type)
	}

	// Check that the response mentions loop detection
	responseText := response.Content[0].Text
	if !strings.Contains(responseText, "Loop Detection") && !strings.Contains(responseText, "loop") {
		t.Errorf("Expected loop detection message in response, got: %s", responseText)
	}

	// Check that it mentions TodoWrite specifically
	if !strings.Contains(responseText, "TodoWrite") {
		t.Errorf("Expected TodoWrite-specific message, got: %s", responseText)
	}

	// Check that the response role is assistant
	if response.Role != "assistant" {
		t.Errorf("Expected role 'assistant', got '%s'", response.Role)
	}

	// Check that it has a proper model ID indicating loop detection
	if !strings.Contains(response.Model, "loop") {
		t.Errorf("Expected loop-related model ID, got: %s", response.Model)
	}
}

func TestLoopDetectionIntegration_NoLoop(t *testing.T) {
	// Create a minimal config for testing  
	cfg := &config.Config{
		BigModel:              "test-model",
		BigModelEndpoint:      "http://localhost/v1/completions",
		BigModelAPIKey:        "test-key",
		SmallModel:            "test-small-model",
		SmallModelEndpoint:    "http://localhost/v1/completions",
		SmallModelAPIKey:      "test-key",
		ToolCorrectionEnabled: false,
	}

	// Create handler
	handler := proxy.NewHandler(cfg, nil)

	// Create a normal request (no loop)
	anthropicReq := types.AnthropicRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []types.Message{
			{Role: "user", Content: "Help me read a file"},
			{Role: "assistant", Content: []types.Content{{Type: "tool_use", ID: "1", Name: "Read", Input: map[string]interface{}{"file_path": "/test.go"}}}},
		},
		Tools: []types.Tool{
			{
				Name:        "Read",
				Description: "Read file",
				InputSchema: types.ToolSchema{Type: "object"},
			},
		},
	}

	// Marshal request
	reqBody, err := json.Marshal(anthropicReq)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	// Create HTTP request
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	// Create response recorder
	w := httptest.NewRecorder()

	// Call handler (this should try to proxy to the provider and fail, but not trigger loop detection)
	handler.HandleAnthropicRequest(w, req)

	// For this test, we expect it to fail at the proxy stage (not loop detection)
	// The important thing is that it doesn't return a loop detection message
	responseBody := w.Body.String()
	if strings.Contains(responseBody, "Loop Detection") || strings.Contains(responseBody, "loop detected") {
		t.Errorf("Unexpected loop detection in normal request. Response: %s", responseBody)
	}
}

func TestLoopDetectionIntegration_ConsecutiveIdentical(t *testing.T) {
	cfg := &config.Config{
		BigModel:              "test-model",
		BigModelEndpoint:      "http://localhost/v1/completions",
		BigModelAPIKey:        "test-key",
		SmallModel:            "test-small-model",
		SmallModelEndpoint:    "http://localhost/v1/completions",
		SmallModelAPIKey:      "test-key",
		ToolCorrectionEnabled: false,
	}

	handler := proxy.NewHandler(cfg, nil)

	// Create consecutive identical Edit pattern (this should be detected as a loop)
	anthropicReq := types.AnthropicRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []types.Message{
			{Role: "user", Content: "Edit the file multiple times"},
			{Role: "assistant", Content: []types.Content{{Type: "tool_use", ID: "1", Name: "Edit", Input: map[string]interface{}{"file_path": "/test.go", "old_string": "old", "new_string": "new"}}}},
			{Role: "user", Content: []types.Content{{Type: "tool_result", ToolUseID: "1", Text: "File edited successfully"}}},
			{Role: "assistant", Content: []types.Content{{Type: "tool_use", ID: "2", Name: "Edit", Input: map[string]interface{}{"file_path": "/test.go", "old_string": "old", "new_string": "new"}}}},
			{Role: "user", Content: []types.Content{{Type: "tool_result", ToolUseID: "2", Text: "File edited successfully"}}},
			{Role: "assistant", Content: []types.Content{{Type: "tool_use", ID: "3", Name: "Edit", Input: map[string]interface{}{"file_path": "/test.go", "old_string": "old", "new_string": "new"}}}},
			{Role: "user", Content: []types.Content{{Type: "tool_result", ToolUseID: "3", Text: "File edited successfully"}}},
			{Role: "assistant", Content: []types.Content{{Type: "tool_use", ID: "4", Name: "Edit", Input: map[string]interface{}{"file_path": "/test.go", "old_string": "old", "new_string": "new"}}}},
		},
		Tools: []types.Tool{
			{
				Name:        "Edit",
				Description: "Edit file",
				InputSchema: types.ToolSchema{Type: "object"},
			},
		},
	}

	reqBody, err := json.Marshal(anthropicReq)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.HandleAnthropicRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		return
	}

	var response types.AnthropicResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should detect consecutive identical pattern
	responseText := response.Content[0].Text
	if !strings.Contains(responseText, "Loop Detection") && !strings.Contains(responseText, "loop") {
		t.Errorf("Expected loop detection for consecutive identical pattern, but got: %s", responseText)
	}
}