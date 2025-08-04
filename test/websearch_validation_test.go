package test

import (
	"claude-proxy/correction"
	"claude-proxy/internal"
	"claude-proxy/proxy"
	"claude-proxy/types"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestWebSearchValidationWithFallbackSchema tests that WebSearch tool validation
// works correctly even when WebSearch is not in the availableTools list
// This addresses the infinite retry loop issue where WebSearch was marked as "Unknown tool"
func TestWebSearchValidationWithFallbackSchema(t *testing.T) {
	service := correction.NewService("http://test.com", "test-key", true, "test-model", false)
	ctx := internal.WithRequestID(context.Background(), "websearch_test")

	// Simulate the real scenario: availableTools only contains tools from the original request
	// but WebSearch tool call comes from LLM response and is not in availableTools
	availableTools := []types.Tool{
		{
			Name:        "Read",
			Description: "Read a file",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"file_path": {Type: "string", Description: "Path to file"},
				},
				Required: []string{"file_path"},
			},
		},
		// Note: WebSearch is intentionally NOT in this list
		// This simulates the real issue where WebSearch tool call comes from LLM
		// but wasn't in the original request's tools
	}

	tests := []struct {
		name        string
		toolCall    types.Content
		expectValid bool
		description string
	}{
		{
			name: "websearch_with_query_parameter_should_be_valid",
			toolCall: types.Content{
				Type: "tool_use",
				ID:   "test_id",
				Name: "WebSearch",
				Input: map[string]interface{}{
					"query": "Java code review security best practices 2025",
				},
			},
			expectValid: true,
			description: "WebSearch with correct 'query' parameter should be valid even if not in availableTools",
		},
		{
			name: "websearch_with_pattern_parameter_should_be_invalid",
			toolCall: types.Content{
				Type: "tool_use",
				ID:   "test_id",
				Name: "WebSearch",
				Input: map[string]interface{}{
					"pattern": "Java code review security best practices 2025",
				},
			},
			expectValid: false,
			description: "WebSearch with 'pattern' parameter should be invalid (should be 'query')",
		},
		{
			name: "websearch_with_missing_query_should_be_invalid",
			toolCall: types.Content{
				Type: "tool_use",
				ID:   "test_id",
				Name: "WebSearch",
				Input: map[string]interface{}{
					"allowed_domains": []string{"example.com"},
				},
			},
			expectValid: false,
			description: "WebSearch missing required 'query' parameter should be invalid",
		},
		{
			name: "unknown_tool_should_be_invalid",
			toolCall: types.Content{
				Type: "tool_use",
				ID:   "test_id",
				Name: "UnknownTool",
				Input: map[string]interface{}{
					"param": "value",
				},
			},
			expectValid: false,
			description: "Truly unknown tools should still be invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validation := service.ValidateToolCall(ctx, tt.toolCall, availableTools)
			
			if tt.expectValid {
				assert.True(t, validation.IsValid, tt.description)
				assert.False(t, validation.HasCaseIssue, "Valid tool should not have case issues")
				assert.False(t, validation.HasToolNameIssue, "Valid tool should not have name issues")
				assert.Empty(t, validation.MissingParams, "Valid tool should not have missing params")
				assert.Empty(t, validation.InvalidParams, "Valid tool should not have invalid params")
			} else {
				assert.False(t, validation.IsValid, tt.description)
			}
		})
	}
}

// TestWebSearchCorrectionWithFallbackSchema tests the complete correction flow
// TestWebSearchSchemaRestoration tests that corrupted web_search tools are restored correctly
func TestWebSearchSchemaRestoration(t *testing.T) {
	// Test the exact scenario from the logs: web_search with empty schema
	corruptedTool := types.Tool{
		Name:        "web_search",
		Description: "",
		InputSchema: types.ToolSchema{
			Type:       "",
			Properties: map[string]types.ToolProperty{},
			Required:   []string{},
		},
	}

	// Test fallback schema function directly
	fallbackTool := types.GetFallbackToolSchema("web_search")
	assert.NotNil(t, fallbackTool, "Should return fallback schema for web_search")
	assert.Equal(t, "WebSearch", fallbackTool.Name, "Should map web_search to WebSearch")
	assert.Equal(t, "object", fallbackTool.InputSchema.Type, "Should have valid schema type")
	assert.Contains(t, fallbackTool.InputSchema.Properties, "query", "Should have query property")
	assert.Equal(t, []string{"query"}, fallbackTool.InputSchema.Required, "Should require query parameter")

	// Test FindValidToolSchema function 
	availableTools := []types.Tool{} // Empty - simulating the real scenario
	restoredTool := proxy.FindValidToolSchema(corruptedTool, availableTools)
	assert.NotNil(t, restoredTool, "Should restore corrupted web_search tool")
	assert.Equal(t, "WebSearch", restoredTool.Name, "Should restore to WebSearch")
	assert.Equal(t, "object", restoredTool.InputSchema.Type, "Should have valid restored schema")
}

func TestWebSearchCorrectionWithFallbackSchema(t *testing.T) {
	service := correction.NewService("http://test.com", "test-key", true, "test-model", false)
	ctx := internal.WithRequestID(context.Background(), "websearch_correction_test")

	// Same scenario: availableTools doesn't include WebSearch
	availableTools := []types.Tool{
		{
			Name:        "Read",
			Description: "Read a file",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"file_path": {Type: "string", Description: "Path to file"},
				},
				Required: []string{"file_path"},
			},
		},
	}

	// Test tool call with correct WebSearch parameters
	validWebSearchCall := types.Content{
		Type: "tool_use",
		ID:   "test_id",
		Name: "WebSearch",
		Input: map[string]interface{}{
			"query": "Java code review security best practices 2025",
		},
	}

	// This should NOT require correction because WebSearch with 'query' is valid
	validation := service.ValidateToolCall(ctx, validWebSearchCall, availableTools)
	assert.True(t, validation.IsValid, "WebSearch with correct parameters should be valid")

	// Test that NeedsCorrection returns false for valid WebSearch
	// (This simulates the handler.go flow)
	needsCorrection := !validation.IsValid || validation.HasCaseIssue || validation.HasToolNameIssue
	assert.False(t, needsCorrection, "Valid WebSearch should not need correction")
}