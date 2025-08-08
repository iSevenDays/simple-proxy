package test

import (
	"claude-proxy/correction"
	"claude-proxy/internal"
	"claude-proxy/types"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Helper function to marshal JSON for test data
func mustMarshalJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// TestExitPlanModeValidation tests ExitPlanMode usage validation
// Following TDD: Write tests first to define expected behavior
func TestExitPlanModeValidation(t *testing.T) {
	mockConfig := NewMockConfigProvider("http://mock-endpoint:8080/v1/chat/completions")
	service := correction.NewService(mockConfig, "test-key", true, "test-model", false)
	ctx := internal.WithRequestID(context.Background(), "test-request-001")

	tests := []struct {
		name              string
		toolCall          types.Content
		messages          []types.OpenAIMessage
		shouldBlock       bool
		expectedReason    string
		description       string
	}{
		{
			name: "valid_planning_usage_allowed",
			toolCall: types.Content{
				Type: "tool_use",
				Name: "ExitPlanMode",
				Input: map[string]interface{}{
					"plan": "I'll implement the following steps:\n1. Create user registration form\n2. Add validation logic\n3. Connect to database",
				},
			},
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "Help me implement user registration for my app"},
				{Role: "assistant", Content: "I'll help you implement user registration. Let me plan this out..."},
			},
			shouldBlock:    false,
			expectedReason: "",
			description:    "Valid planning usage should be allowed",
		},
		{
			name: "completion_summary_blocked",
			toolCall: types.Content{
				Type: "tool_use", 
				Name: "ExitPlanMode",
				Input: map[string]interface{}{
					"plan": "✅ **All tasks completed successfully**\n\nSummary of changes:\n1. User registration implemented\n2. Tests passing\n3. Ready for production",
				},
			},
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "Help me implement user registration"},
				{Role: "assistant", Content: "I'll implement user registration for you."},
				{
					Role: "assistant", 
					Content: "", 
					ToolCalls: []types.OpenAIToolCall{
						{
							ID: "write-001",
							Type: "function",
							Function: types.OpenAIToolCallFunction{
								Name: "Write", 
								Arguments: mustMarshalJSON(map[string]interface{}{"file_path": "register.js", "content": "// Registration logic"}),
							},
						},
					},
				},
				{Role: "tool", Content: "File written successfully", ToolCallID: "write-001"},
				{
					Role: "assistant",
					Content: "",
					ToolCalls: []types.OpenAIToolCall{
						{
							ID: "bash-001",
							Type: "function",
							Function: types.OpenAIToolCallFunction{
								Name: "Bash", 
								Arguments: mustMarshalJSON(map[string]interface{}{"command": "npm test"}),
							},
						},
					},
				},
				{Role: "tool", Content: "Tests passed", ToolCallID: "bash-001"},
			},
			shouldBlock:    true,
			expectedReason: "post-completion summary",
			description:    "ExitPlanMode used as completion summary should be blocked",
		},
		{
			name: "post_implementation_blocked",
			toolCall: types.Content{
				Type: "tool_use",
				Name: "ExitPlanMode", 
				Input: map[string]interface{}{
					"plan": "Implementation completed. The user registration feature is now ready.",
				},
			},
			messages: buildMessagesWithImplementationWork(),
			shouldBlock:    true,
			expectedReason: "post-implementation usage",
			description:    "ExitPlanMode after implementation work should be blocked",
		},
		{
			name: "research_task_allowed",
			toolCall: types.Content{
				Type: "tool_use",
				Name: "ExitPlanMode", 
				Input: map[string]interface{}{
					"plan": "Based on my analysis, here's the implementation plan:\n1. Refactor authentication module\n2. Add rate limiting\n3. Update tests",
				},
			},
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "Analyze the current authentication system and suggest improvements"},
				{
					Role: "assistant",
					Content: "",
					ToolCalls: []types.OpenAIToolCall{
						{
							ID: "read-001",
							Type: "function",
							Function: types.OpenAIToolCallFunction{
								Name: "Read",
								Arguments: mustMarshalJSON(map[string]interface{}{"file_path": "auth.js"}),
							},
						},
					},
				},
				{Role: "tool", Content: "File contents: [auth code]", ToolCallID: "read-001"},
				{
					Role: "assistant",
					Content: "",
					ToolCalls: []types.OpenAIToolCall{
						{
							ID: "grep-001",
							Type: "function",
							Function: types.OpenAIToolCallFunction{
								Name: "Grep",
								Arguments: mustMarshalJSON(map[string]interface{}{"pattern": "security", "glob": "*.js"}),
							},
						},
					},
				},
				{Role: "tool", Content: "Found security references", ToolCallID: "grep-001"},
			},
			shouldBlock:    false,
			expectedReason: "",
			description:    "ExitPlanMode after research/analysis tasks should be allowed",
		},
		{
			name: "minimal_conversation_allowed",
			toolCall: types.Content{
				Type: "tool_use",
				Name: "ExitPlanMode",
				Input: map[string]interface{}{
					"plan": "I'll help you implement the login system with the following approach:\n1. Create login form\n2. Add authentication logic",
				},
			},
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "Help me build a login system"},
				{Role: "assistant", Content: "I'll help you build a login system. Let me create a plan..."},
			},
			shouldBlock:    false,
			expectedReason: "",
			description:    "ExitPlanMode in short conversations should be allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldBlock, reason := service.ValidateExitPlanMode(ctx, tt.toolCall, tt.messages)
			
			assert.Equal(t, tt.shouldBlock, shouldBlock, 
				"Test case %s: Expected shouldBlock=%v, got %v. %s", 
				tt.name, tt.shouldBlock, shouldBlock, tt.description)
			
			if tt.shouldBlock {
				assert.Contains(t, reason, tt.expectedReason,
					"Test case %s: Expected reason to contain '%s', got '%s'", 
					tt.name, tt.expectedReason, reason)
				assert.NotEmpty(t, reason,
					"Test case %s: Blocked calls should have a reason", tt.name)
			} else {
				assert.Empty(t, reason,
					"Test case %s: Allowed calls should not have a reason", tt.name)
			}
		})
	}
}

// Helper function to build messages with implementation work
func buildMessagesWithImplementationWork() []types.OpenAIMessage {
	return []types.OpenAIMessage{
		{Role: "user", Content: "Add user registration"},
		{
			Role: "assistant",
			Content: "",
			ToolCalls: []types.OpenAIToolCall{
				{
					ID: "write-001",
					Type: "function",
					Function: types.OpenAIToolCallFunction{
						Name: "Write",
						Arguments: mustMarshalJSON(map[string]interface{}{"file_path": "auth.js", "content": "auth code"}),
					},
				},
			},
		},
		{Role: "tool", Content: "File created", ToolCallID: "write-001"},
		{
			Role: "assistant", 
			Content: "",
			ToolCalls: []types.OpenAIToolCall{
				{
					ID: "edit-001",
					Type: "function",
					Function: types.OpenAIToolCallFunction{
						Name: "Edit",
						Arguments: mustMarshalJSON(map[string]interface{}{"file_path": "app.js", "old_string": "old", "new_string": "new"}),
					},
				},
			},
		},
		{Role: "tool", Content: "File updated", ToolCallID: "edit-001"},
		{
			Role: "assistant",
			Content: "",
			ToolCalls: []types.OpenAIToolCall{
				{
					ID: "bash-001",
					Type: "function",
					Function: types.OpenAIToolCallFunction{
						Name: "Bash",
						Arguments: mustMarshalJSON(map[string]interface{}{"command": "npm start"}),
					},
				},
			},
		},
		{Role: "tool", Content: "Server started", ToolCallID: "bash-001"},
	}
}

// TestExitPlanModeContentAnalysis tests plan content analysis for completion indicators
func TestExitPlanModeContentAnalysis(t *testing.T) {
	mockConfig := NewMockConfigProvider("http://mock-endpoint:8080/v1/chat/completions")
	service := correction.NewService(mockConfig, "test-key", true, "test-model", false)
	ctx := internal.WithRequestID(context.Background(), "test-request-002")

	completionIndicatorTests := []struct {
		name        string
		planContent string
		shouldBlock bool
		description string
	}{
		{
			name:        "checkmark_completion_blocked",
			planContent: "✅ All features implemented successfully",
			shouldBlock: true,
			description: "Plans with checkmarks indicate completion",
		},
		{
			name:        "completed_successfully_blocked",
			planContent: "The implementation has been completed successfully and is ready for review.",
			shouldBlock: true,
			description: "Plans with 'completed successfully' indicate completion",
		},
		{
			name:        "all_tasks_completed_blocked", 
			planContent: "All tasks completed. The system is now functional.",
			shouldBlock: true,
			description: "Plans with 'all tasks completed' indicate completion",
		},
		{
			name:        "ready_for_production_blocked",
			planContent: "Implementation finished. Code is ready for production deployment.",
			shouldBlock: true,
			description: "Plans with 'ready for' indicate completion",
		},
		{
			name:        "work_is_done_blocked",
			planContent: "The work is done. All requirements have been met.",
			shouldBlock: true,
			description: "Plans with 'work is done' indicate completion",
		},
		{
			name:        "future_planning_allowed",
			planContent: "I will implement the following features:\n1. User authentication\n2. Data validation\n3. Error handling",
			shouldBlock: false,
			description: "Plans with future tense should be allowed",
		},
		{
			name:        "planning_steps_allowed",
			planContent: "Here's my approach:\n1. Analyze requirements\n2. Design the architecture\n3. Implement core features",
			shouldBlock: false,
			description: "Plans with clear steps should be allowed",
		},
		{
			name:        "mixed_completion_blocked",
			planContent: "✅ **Summary of Implementation**\n\n1. User registration - completed\n2. Login system - completed\n3. Dashboard - completed\n\nAll functionality is working correctly.",
			shouldBlock: true,
			description: "Plans with mixed completion indicators should be blocked",
		},
	}

	for _, tt := range completionIndicatorTests {
		t.Run(tt.name, func(t *testing.T) {
			toolCall := types.Content{
				Type: "tool_use",
				Name: "ExitPlanMode",
				Input: map[string]interface{}{
					"plan": tt.planContent,
				},
			}

			// Minimal message context for content-only testing
			messages := []types.OpenAIMessage{
				{Role: "user", Content: "Implement feature X"},
			}

			shouldBlock, _ := service.ValidateExitPlanMode(ctx, toolCall, messages)
			
			assert.Equal(t, tt.shouldBlock, shouldBlock,
				"Test case %s: Expected shouldBlock=%v, got %v. %s",
				tt.name, tt.shouldBlock, shouldBlock, tt.description)
		})
	}
}

// TestExitPlanModeImplementationPatterns tests detection of implementation work patterns
func TestExitPlanModeImplementationPatterns(t *testing.T) {
	mockConfig := NewMockConfigProvider("http://mock-endpoint:8080/v1/chat/completions")
	service := correction.NewService(mockConfig, "test-key", true, "test-model", false)
	ctx := internal.WithRequestID(context.Background(), "test-request-003")

	implementationPatternTests := []struct {
		name         string
		messages     []types.OpenAIMessage
		shouldBlock  bool
		description  string
	}{
		{
			name: "high_implementation_activity_blocked",
			messages: buildMessagesWithToolCalls([]string{
				"Write", "Edit", "Bash", "MultiEdit", "TodoWrite", "Bash", "Write",
			}),
			shouldBlock: true,
			description: "High implementation activity should trigger blocking",
		},
		{
			name: "moderate_implementation_activity_blocked",
			messages: buildMessagesWithToolCalls([]string{
				"Bash", "TodoWrite", "Edit", "Bash",
			}),
			shouldBlock: true,
			description: "Moderate implementation activity should trigger blocking",
		},
		{
			name: "minimal_implementation_activity_allowed",
			messages: buildMessagesWithToolCalls([]string{
				"Read", "Grep",
			}),
			shouldBlock: false,
			description: "Minimal implementation activity should be allowed",
		},
		{
			name: "research_only_activity_allowed",
			messages: buildMessagesWithToolCalls([]string{
				"Read", "Grep", "Glob", "WebSearch", "Read",
			}),
			shouldBlock: false,
			description: "Research-only activity should be allowed",
		},
		{
			name: "mixed_research_implementation_blocked",
			messages: buildMessagesWithToolCalls([]string{
				"Read", "Grep", "Write", "Edit", "Bash",
			}),
			shouldBlock: true,
			description: "Mixed research and implementation should be blocked",
		},
	}

	for _, tt := range implementationPatternTests {
		t.Run(tt.name, func(t *testing.T) {
			toolCall := types.Content{
				Type: "tool_use",
				Name: "ExitPlanMode",
				Input: map[string]interface{}{
					"plan": "Standard planning content for testing implementation patterns",
				},
			}

			shouldBlock, _ := service.ValidateExitPlanMode(ctx, toolCall, tt.messages)
			
			assert.Equal(t, tt.shouldBlock, shouldBlock,
				"Test case %s: Expected shouldBlock=%v, got %v. %s",
				tt.name, tt.shouldBlock, shouldBlock, tt.description)
		})
	}
}

// Helper function to build messages with specific tool calls for testing
func buildMessagesWithToolCalls(toolNames []string) []types.OpenAIMessage {
	messages := []types.OpenAIMessage{
		{Role: "user", Content: "Implement the requested feature"},
	}

	for i, toolName := range toolNames {
		toolCallID := fmt.Sprintf("tool-%03d", i+1)
		
		// Add assistant message with tool call
		messages = append(messages, types.OpenAIMessage{
			Role:    "assistant", 
			Content: "",
			ToolCalls: []types.OpenAIToolCall{
				{
					ID: toolCallID,
					Type: "function",
					Function: types.OpenAIToolCallFunction{
						Name: toolName,
						Arguments: mustMarshalJSON(map[string]interface{}{"test_param": "test_value"}),
					},
				},
			},
		})

		// Add tool response message
		messages = append(messages, types.OpenAIMessage{
			Role:       "tool",
			Content:    fmt.Sprintf("Tool %s executed successfully", toolName),
			ToolCallID: toolCallID,
		})
	}

	return messages
}

// TestExitPlanModeEdgeCases tests edge cases and boundary conditions
func TestExitPlanModeEdgeCases(t *testing.T) {
	mockConfig := NewMockConfigProvider("http://mock-endpoint:8080/v1/chat/completions")
	service := correction.NewService(mockConfig, "test-key", true, "test-model", false)
	ctx := internal.WithRequestID(context.Background(), "test-request-004")

	edgeCaseTests := []struct {
		name        string
		toolCall    types.Content
		messages    []types.OpenAIMessage
		shouldBlock bool
		description string
	}{
		{
			name: "empty_plan_content_allowed",
			toolCall: types.Content{
				Type: "tool_use",
				Name: "ExitPlanMode",
				Input: map[string]interface{}{
					"plan": "",
				},
			},
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "Help me with X"},
			},
			shouldBlock: false,
			description: "Empty plan content should be allowed (edge case)",
		},
		{
			name: "missing_plan_parameter_allowed",
			toolCall: types.Content{
				Type: "tool_use",
				Name: "ExitPlanMode",
				Input: map[string]interface{}{},
			},
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "Help me with X"},
			},
			shouldBlock: false,
			description: "Missing plan parameter should be allowed (let schema validation handle)",
		},
		{
			name: "non_string_plan_allowed",
			toolCall: types.Content{
				Type: "tool_use", 
				Name: "ExitPlanMode",
				Input: map[string]interface{}{
					"plan": 12345,
				},
			},
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "Help me with X"},
			},
			shouldBlock: false,
			description: "Non-string plan should be allowed (let schema validation handle)",
		},
		{
			name: "empty_messages_allowed",
			toolCall: types.Content{
				Type: "tool_use",
				Name: "ExitPlanMode", 
				Input: map[string]interface{}{
					"plan": "Here's my implementation plan...",
				},
			},
			messages: []types.OpenAIMessage{},
			shouldBlock: false,
			description: "Empty message list should be allowed",
		},
	}

	for _, tt := range edgeCaseTests {
		t.Run(tt.name, func(t *testing.T) {
			shouldBlock, _ := service.ValidateExitPlanMode(ctx, tt.toolCall, tt.messages)
			
			assert.Equal(t, tt.shouldBlock, shouldBlock,
				"Test case %s: Expected shouldBlock=%v, got %v. %s",
				tt.name, tt.shouldBlock, shouldBlock, tt.description)
		})
	}
}