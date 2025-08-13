package test

import (
	"claude-proxy/correction"
	"claude-proxy/internal"
	"claude-proxy/types"
	"context"
	"testing"
)

// TestTodoWriteStatusPreservation tests that existing status values are not overridden during correction
func TestTodoWriteStatusPreservation(t *testing.T) {
	service := correction.NewService(NewMockConfigProvider("http://test"), "test-key", true, "test-model", false, nil)
	ctx := context.WithValue(context.Background(), internal.RequestIDKey, "test-status-preservation")
	
	tests := []struct {
		name           string
		input          types.Content
		expectFixed    bool
		expectedStatus []string // Expected status for each todo in order
	}{
		{
			name: "Already valid todos with mixed statuses - should not be modified",
			input: types.Content{
				Type: "tool_use",
				ID:   "test-valid-mixed",
				Name: "TodoWrite",
				Input: map[string]interface{}{
					"todos": []interface{}{
						map[string]interface{}{
							"content":  "First task",
							"status":   "pending",
							"priority": "high",
							"id":       "1",
						},
						map[string]interface{}{
							"content":  "Second task", 
							"status":   "in_progress", // This should NOT be changed to "pending"
							"priority": "medium",
							"id":       "2",
						},
						map[string]interface{}{
							"content":  "Third task",
							"status":   "completed",    // This should NOT be changed to "pending"
							"priority": "low",
							"id":       "3",
						},
					},
				},
			},
			expectFixed:    false, // Should not need correction
			expectedStatus: []string{"pending", "in_progress", "completed"},
		},
		{
			name: "Valid todos missing only priority - should add priority but preserve status",
			input: types.Content{
				Type: "tool_use", 
				ID:   "test-missing-priority",
				Name: "TodoWrite",
				Input: map[string]interface{}{
					"todos": []interface{}{
						map[string]interface{}{
							"content": "Task with status but no priority",
							"status":  "in_progress", // This should be preserved
							"id":      "1",
						},
					},
				},
			},
			expectFixed:    true, // Should fix by adding missing priority
			expectedStatus: []string{"in_progress"}, // Status should be preserved
		},
		{
			name: "Malformed todo with task field but valid status - should preserve status",
			input: types.Content{
				Type: "tool_use",
				ID:   "test-malformed-preserve-status",
				Name: "TodoWrite", 
				Input: map[string]interface{}{
					"todos": []interface{}{
						map[string]interface{}{
							"task":     "Fix the issue", // Wrong field name - should become "content"
							"status":   "in_progress",   // This status should be preserved
							"priority": "high",
							"id":       "1",
						},
					},
				},
			},
			expectFixed:    true, // Should fix by transforming task → content
			expectedStatus: []string{"in_progress"}, // Status should be preserved
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Attempt the rule-based correction
			result, fixed := service.AttemptRuleBasedTodoWriteCorrection(ctx, tt.input)
			
			if fixed != tt.expectFixed {
				t.Errorf("Expected fixed=%v, got %v", tt.expectFixed, fixed)
			}
			
			// Check the result (either original or corrected)
			var finalInput map[string]interface{}
			if fixed {
				finalInput = result.Input
			} else {
				finalInput = tt.input.Input
			}
			
			// Verify todos structure and status preservation
			if todos, exists := finalInput["todos"]; exists {
				if todosArray, ok := todos.([]interface{}); ok {
					if len(todosArray) != len(tt.expectedStatus) {
						t.Errorf("Expected %d todos, got %d", len(tt.expectedStatus), len(todosArray))
						return
					}
					
					for i, expectedStatus := range tt.expectedStatus {
						if todoMap, ok := todosArray[i].(map[string]interface{}); ok {
							if actualStatus, hasStatus := todoMap["status"]; hasStatus {
								if actualStatus != expectedStatus {
									t.Errorf("Todo %d: expected status=%s, got %s (CRITICAL: Status was not preserved during correction!)", i, expectedStatus, actualStatus)
								}
							} else {
								t.Errorf("Todo %d: missing status field", i)
							}
							
							// Also verify content field exists
							if _, hasContent := todoMap["content"]; !hasContent {
								t.Errorf("Todo %d: missing content field", i)
							}
							
							// For the malformed test, verify that "task" was transformed to "content"
							if tt.name == "Malformed todo with task field but valid status - should preserve status" {
								if _, hasTask := todoMap["task"]; hasTask {
									t.Errorf("Todo %d: still has 'task' field - transformation failed", i)
								}
							}
						} else {
							t.Errorf("Todo %d: not a map", i)
						}
					}
				} else {
					t.Error("todos field is not an array")
				}
			} else {
				t.Error("todos field missing from result")
			}
		})
	}
}