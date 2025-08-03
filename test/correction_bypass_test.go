package test

import (
	"claude-proxy/correction"
	"claude-proxy/internal"
	"claude-proxy/types"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTodoWriteCorrectionBypass tests the specific issue where TodoWrite correction
// is bypassed due to early validation checks
func TestTodoWriteCorrectionBypass(t *testing.T) {
	service := correction.NewService("http://test.com", "test-key", true, "test-model", false)
	
	// Define TodoWrite tool schema (simplified - matches the actual tool)
	availableTools := []types.Tool{
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

	// Create TodoWrite call with malformed structure (has 'task' instead of 'content')
	malformedCall := types.Content{
		Type: "tool_use",
		Name: "TodoWrite",
		ID:   "call_todo_test",
		Input: map[string]interface{}{
			"todos": []interface{}{
				map[string]interface{}{
					"task":     "Fix the TodoWrite issue",  // Should be 'content'
					"status":   "in_progress",
					"priority": "high",
					"id":       "1",
				},
			},
		},
	}

	// Test 1: Validate that the malformed call passes OpenAI validation
	ctx := internal.WithRequestID(context.Background(), "test_bypass")
	validation := service.ValidateToolCall(ctx, malformedCall, availableTools)
	
	// This should pass OpenAI validation (the root cause of the bypass)
	assert.True(t, validation.IsValid, "Malformed TodoWrite should pass OpenAI validation")
	assert.False(t, validation.HasCaseIssue, "Should not have case issues")
	assert.False(t, validation.HasToolNameIssue, "Should not have tool name issues")

	// Test 2: Verify correction is needed by checking the structure
	// The corrected call should transform 'task' to 'content'
	correctedContent, err := service.CorrectToolCalls(ctx, []types.Content{malformedCall}, availableTools)
	assert.NoError(t, err, "Correction should not fail")
	assert.Len(t, correctedContent, 1, "Should return one corrected item")
	
	// Extract the todos array from corrected call
	if assert.Equal(t, "tool_use", correctedContent[0].Type) && 
	   assert.Equal(t, "TodoWrite", correctedContent[0].Name) {
		
		todosInterface, exists := correctedContent[0].Input["todos"]
		assert.True(t, exists, "Todos should exist in corrected call")
		
		if todosArray, ok := todosInterface.([]interface{}); ok && len(todosArray) > 0 {
			if todoItem, ok := todosArray[0].(map[string]interface{}); ok {
				// Should have 'content' field after correction
				_, hasContent := todoItem["content"]
				_, hasTask := todoItem["task"]
				
				// This test will FAIL with current bypass logic - it should have content, not task
				assert.True(t, hasContent, "Corrected TodoWrite should have 'content' field")
				assert.False(t, hasTask, "Corrected TodoWrite should not have 'task' field")
			}
		}
	}
}