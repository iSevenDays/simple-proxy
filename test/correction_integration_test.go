package test

import (
	"claude-proxy/correction"
	"claude-proxy/internal"
	"claude-proxy/types"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSlashCommandIntegration tests the complete slash command correction flow
// Following SPARC: End-to-end integration testing for slash command correction
func TestSlashCommandIntegration(t *testing.T) {
	mockConfig := NewMockConfigProvider("http://test.com")
	service := correction.NewService(mockConfig, "test-key", true, "test-model", false)

	// Define Task tool schema
	taskToolSchema := types.Tool{
		Name:        "Task",
		Description: "Launch a new agent to handle complex tasks",
		InputSchema: types.ToolSchema{
			Type: "object",
			Properties: map[string]types.ToolProperty{
				"description": {Type: "string", Description: "Description of the task"},
				"prompt":      {Type: "string", Description: "The task prompt"},
			},
			Required: []string{"description", "prompt"},
		},
	}

	// Mock available tools
	availableTools := []types.Tool{taskToolSchema}

	tests := []struct {
		name          string
		inputCalls    []types.Content
		expectedCalls []types.Content
	}{
		{
			name: "slash_command_corrected_in_flow",
			inputCalls: []types.Content{
				{
					Type: "tool_use",
					ID:   "test_id_1",
					Name: "/code-reviewer",
					Input: map[string]interface{}{
						"subagent_type": "code-reviewer",
					},
				},
			},
			expectedCalls: []types.Content{
				{
					Type: "tool_use",
					ID:   "test_id_1",
					Name: "Task",
					Input: map[string]interface{}{
						"description":   "Code Reviewer",
						"prompt":        "/code-reviewer",
						"subagent_type": "code-reviewer",
					},
				},
			},
		},
		{
			name: "normal_tool_unchanged",
			inputCalls: []types.Content{
				{
					Type: "tool_use",
					ID:   "test_id_2",
					Name: "Task",
					Input: map[string]interface{}{
						"description": "Test task",
						"prompt":      "test prompt",
					},
				},
			},
			expectedCalls: []types.Content{
				{
					Type: "tool_use",
					ID:   "test_id_2",
					Name: "Task",
					Input: map[string]interface{}{
						"description": "Test task",
						"prompt":      "test prompt",
					},
				},
			},
		},
		{
			name: "multiple_slash_commands_corrected",
			inputCalls: []types.Content{
				{
					Type: "tool_use",
					ID:   "test_id_3",
					Name: "/init",
					Input: map[string]interface{}{},
				},
				{
					Type: "tool_use",
					ID:   "test_id_4",
					Name: "/review",
					Input: map[string]interface{}{
						"target": "main",
					},
				},
			},
			expectedCalls: []types.Content{
				{
					Type: "tool_use",
					ID:   "test_id_3",
					Name: "Task",
					Input: map[string]interface{}{
						"description": "Init",
						"prompt":      "/init",
					},
				},
				{
					Type: "tool_use",
					ID:   "test_id_4",
					Name: "Task",
					Input: map[string]interface{}{
						"description": "Review",
						"prompt":      "/review",
						"target":      "main",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := internal.WithRequestID(context.Background(), "integration_test")

			// Run the complete correction flow
			correctedCalls, err := service.CorrectToolCalls(ctx, tt.inputCalls, availableTools)
			require.NoError(t, err, "Correction should not fail")

			// Verify the results
			require.Equal(t, len(tt.expectedCalls), len(correctedCalls), "Number of corrected calls should match")

			for i, expectedCall := range tt.expectedCalls {
				actualCall := correctedCalls[i]

				assert.Equal(t, expectedCall.Type, actualCall.Type, "Call type should match")
				assert.Equal(t, expectedCall.ID, actualCall.ID, "Call ID should be preserved")
				assert.Equal(t, expectedCall.Name, actualCall.Name, "Tool name should be corrected")

				// Check input parameters
				for key, expectedValue := range expectedCall.Input {
					actualValue, exists := actualCall.Input[key]
					assert.True(t, exists, "Parameter %s should exist", key)
					assert.Equal(t, expectedValue, actualValue, "Parameter %s should have correct value", key)
				}
			}
		})
	}
}