package test

import (
	"claude-proxy/config"
	"claude-proxy/correction"
	"claude-proxy/internal"
	"claude-proxy/logger"
	"claude-proxy/proxy"
	"claude-proxy/types"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNeedsCorrection tests the needsCorrection optimization function
func TestNeedsCorrection(t *testing.T) {
	// Create correction service for testing
	service := correction.NewService(NewMockConfigProvider("http://test.com"), "test-key", false, "test-model", false)
	
	// Define available tools
	availableTools := []types.Tool{
		{
			Name:        "Task",
			Description: "Execute a task",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"description": {Type: "string", Description: "Task description"},
					"prompt":      {Type: "string", Description: "Task prompt"},
				},
				Required: []string{"description", "prompt"},
			},
		},
		{
			Name:        "Write", 
			Description: "Write to file",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"file_path": {Type: "string", Description: "File path"},
					"content":   {Type: "string", Description: "Content to write"},
				},
				Required: []string{"file_path", "content"},
			},
		},
		{
			Name:        "TodoWrite",
			Description: "Create and manage a structured task list",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"todos": {
						Type:        "array",
						Description: "The updated todo list",
						Items: &types.ToolPropertyItems{
							Type: "object",
						},
					},
				},
				Required: []string{"todos"},
			},
		},
	}

	tests := []struct {
		name           string
		content        []types.Content
		expectedNeeds  bool
		description    string
	}{
		{
			name: "valid_tool_calls_need_no_correction",
			content: []types.Content{
				{Type: "text", Text: "I'll help you with that"},
				{
					Type: "tool_use",
					Name: "Task",
					ID:   "call_123",
					Input: map[string]interface{}{
						"description": "Test task",
						"prompt":      "Execute test",
					},
				},
			},
			expectedNeeds: false,
			description:   "Valid tool calls should not need correction",
		},
		{
			name: "invalid_tool_name_needs_correction",
			content: []types.Content{
				{
					Type: "tool_use", 
					Name: "/code-reviewer", // Invalid name - should be Task
					ID:   "call_456",
					Input: map[string]interface{}{
						"subagent_type": "code-reviewer",
					},
				},
			},
			expectedNeeds: true,
			description:   "Invalid tool names should need correction",
		},
		{
			name: "missing_required_params_needs_correction",
			content: []types.Content{
				{
					Type: "tool_use",
					Name: "Write",
					ID:   "call_789", 
					Input: map[string]interface{}{
						"file_path": "test.txt",
						// Missing required "content" parameter
					},
				},
			},
			expectedNeeds: true,
			description:   "Missing required parameters should need correction",
		},
		{
			name: "case_issue_needs_correction",
			content: []types.Content{
				{
					Type: "tool_use",
					Name: "task", // Wrong case - should be "Task"
					ID:   "call_000",
					Input: map[string]interface{}{
						"description": "Test task",
						"prompt":      "Execute test",
					},
				},
			},
			expectedNeeds: true,
			description:   "Case issues should need correction",
		},
		{
			name: "text_only_content_needs_no_correction",
			content: []types.Content{
				{Type: "text", Text: "This is just a text response"},
			},
			expectedNeeds: false,
			description:   "Text-only content should not need correction",
		},
		{
			name: "todowrite_malformed_task_field_needs_correction",
			content: []types.Content{
				{
					Type: "tool_use",
					Name: "TodoWrite",
					ID:   "call_todo_malformed",
					Input: map[string]interface{}{
						"todos": []interface{}{
							map[string]interface{}{
								"task":     "Fix the issue", // Should be 'content'
								"status":   "pending",
								"priority": "high",
								"id":       "1",
							},
						},
					},
				},
			},
			expectedNeeds: true,
			description:   "TodoWrite with 'task' field should need correction",
		},
		{
			name: "todowrite_malformed_description_field_needs_correction",
			content: []types.Content{
				{
					Type: "tool_use",
					Name: "TodoWrite",
					ID:   "call_todo_malformed2",
					Input: map[string]interface{}{
						"todos": []interface{}{
							map[string]interface{}{
								"description": "Fix the issue", // Should be 'content'
								"status":      "pending",
								"priority":    "high",
								"id":          "1",
							},
						},
					},
				},
			},
			expectedNeeds: true,
			description:   "TodoWrite with 'description' field should need correction",
		},
		{
			name: "todowrite_missing_content_needs_correction",
			content: []types.Content{
				{
					Type: "tool_use",
					Name: "TodoWrite",
					ID:   "call_todo_missing_content",
					Input: map[string]interface{}{
						"todos": []interface{}{
							map[string]interface{}{
								// Missing 'content' field
								"status":   "pending",
								"priority": "high",
								"id":       "1",
							},
						},
					},
				},
			},
			expectedNeeds: true,
			description:   "TodoWrite missing 'content' field should need correction",
		},
		{
			name: "todowrite_valid_structure_needs_no_correction",
			content: []types.Content{
				{
					Type: "tool_use",
					Name: "TodoWrite",
					ID:   "call_todo_valid",
					Input: map[string]interface{}{
						"todos": []interface{}{
							map[string]interface{}{
								"content":  "Fix the issue",
								"status":   "pending",
								"priority": "high",
								"id":       "1",
							},
						},
					},
				},
			},
			expectedNeeds: false,
			description:   "TodoWrite with correct structure should not need correction",
		},
		{
			name: "mixed_valid_invalid_needs_correction",
			content: []types.Content{
				{
					Type: "tool_use",
					Name: "Task",
					ID:   "call_valid",
					Input: map[string]interface{}{
						"description": "Valid task",
						"prompt":      "Execute task",
					},
				},
				{
					Type: "tool_use",
					Name: "InvalidTool", // Unknown tool
					ID:   "call_invalid",
					Input: map[string]interface{}{
						"param": "value",
					},
				},
			},
			expectedNeeds: true,
			description:   "Mixed valid/invalid should need correction",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := internal.WithRequestID(context.Background(), "test_needs_correction")
			// Create a minimal logger config for testing
			cfg := &config.Config{
				DisableSmallModelLogging:      false,
				DisableToolCorrectionLogging: false,
			}
			loggerConfig := logger.NewConfigAdapter(cfg)
			result := proxy.NeedsCorrection(ctx, tt.content, availableTools, service, loggerConfig)
			assert.Equal(t, tt.expectedNeeds, result, tt.description)
		})
	}
}