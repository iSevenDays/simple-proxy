package test

import (
	"claude-proxy/correction"
	"claude-proxy/internal"
	"claude-proxy/types"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExitPlanModeLLMValidationMocked tests the LLM-based ExitPlanMode validation with mocked endpoints
// This should catch cases that pattern-based validation missed
func TestExitPlanModeLLMValidationMocked(t *testing.T) {
	mockConfig := NewMockConfigProvider("http://mock-endpoint:8080/v1/chat/completions")
	service := correction.NewService(mockConfig, "test-key", true, "test-model", false, nil)
	ctx := internal.WithRequestID(context.Background(), "llm-validation-test")

	tests := []struct {
		name              string
		planContent       string
		messages          []types.OpenAIMessage
		shouldBlock       bool
		expectedReason    string
		description       string
	}{
		{
			name: "real_world_completion_summary_blocked",
			planContent: `# Implementation Plan

I've successfully updated the system to use the new API for app installation through the service API. The implementation included:

1. Added ` + "`installApp`" + ` method to ` + "`ClientInterface`" + ` interface and ` + "`ServiceClient`" + ` implementation to support the new service API
2. Updated ` + "`AppInstaller`" + ` to use the service API when available, with fallback to the original implementation
3. Added comprehensive tests to verify the new functionality and fallback behavior

The changes enable the system to use the optimized binary upload approach through the service, improving performance for large files while maintaining compatibility with existing functionality.`,
			messages: buildLargeImplementationConversation(),
			shouldBlock: true,
			expectedReason: "post-completion summary", // Pattern-based fallback will catch this
			description: "Real-world completion summary with past tense should be blocked",
		},
		{
			name: "subtle_completion_language_blocked",
			planContent: `I have successfully integrated the authentication system with the database. The integration includes:
- User credential validation
- Session management
- Token generation
All components are working correctly and the system is ready for testing.`,
			messages: buildImplementationMessages(),
			shouldBlock: true,
			expectedReason: "post-completion summary", // Pattern-based fallback will catch "successfully" and "ready for testing"
			description: "Subtle completion language should be detected by LLM",
		},
		{
			name: "genuine_planning_allowed",
			planContent: `Based on the requirements, I will implement the user authentication system with the following approach:

1. **Database Schema Design**
   - Create users table with secure password hashing
   - Add sessions table for token management

2. **API Endpoints**
   - POST /auth/login for user authentication
   - POST /auth/logout for session termination
   - GET /auth/verify for token validation

3. **Security Implementation**
   - Use bcrypt for password hashing
   - Implement JWT tokens for session management
   - Add rate limiting for login attempts

This approach ensures security while providing a smooth user experience. Shall I proceed with this implementation?`,
			messages: buildPlanningMessages(),
			shouldBlock: false,
			expectedReason: "",
			description: "Genuine future-focused planning should be allowed",
		},
		{
			name: "planning_after_research_allowed",
			planContent: `After analyzing the codebase, here's my implementation plan for the search functionality:

1. **Backend API Development**
   - Create search controller with filtering capabilities
   - Implement full-text search with PostgreSQL
   - Add pagination and sorting options

2. **Frontend Integration**  
   - Build search component with React
   - Add autocomplete functionality
   - Implement result highlighting

3. **Performance Optimization**
   - Add database indexing for search fields
   - Implement caching for frequent searches
   - Add debouncing for search input

This approach will provide fast, accurate search results. Ready to start implementation?`,
			messages: buildResearchThenPlanningMessages(),
			shouldBlock: false,
			expectedReason: "",
			description: "Planning after research phase should be allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolCall := types.Content{
				Type: "tool_use",
				Name: "ExitPlanMode",
				Input: map[string]interface{}{
					"plan": tt.planContent,
				},
			}

			shouldBlock, reason := service.ValidateExitPlanMode(ctx, toolCall, tt.messages)

			assert.Equal(t, tt.shouldBlock, shouldBlock,
				"Test case %s: Expected shouldBlock=%v, got %v. %s",
				tt.name, tt.shouldBlock, shouldBlock, tt.description)

			if tt.shouldBlock {
				assert.Contains(t, reason, tt.expectedReason,
					"Test case %s: Expected reason to contain '%s', got '%s'",
					tt.name, tt.expectedReason, reason)
			} else {
				assert.Empty(t, reason,
					"Test case %s: Allowed calls should not have a reason, got '%s'",
					tt.name, reason)
			}
		})
	}
}

// buildLargeImplementationConversation creates a conversation with significant implementation work
func buildLargeImplementationConversation() []types.OpenAIMessage {
	messages := []types.OpenAIMessage{
		{Role: "user", Content: "Help me integrate the new library with the app service for installation"},
		{Role: "assistant", Content: "I'll help you integrate the new library with the app service. Let me start by analyzing the current implementation."},
	}

	// Add extensive implementation work similar to the real conversation
	implementationTools := []string{
		"Read", "Grep", "Edit", "Write", "MultiEdit", "Bash", 
		"TodoWrite", "Edit", "MultiEdit", "Bash", "TodoWrite",
	}

	for i, tool := range implementationTools {
		toolCallID := mustMarshalJSON(map[string]string{"id": tool + "-" + mustMarshalJSON(i)})
		
		messages = append(messages, types.OpenAIMessage{
			Role: "assistant",
			Content: "",
			ToolCalls: []types.OpenAIToolCall{
				{
					ID: toolCallID,
					Type: "function",
					Function: types.OpenAIToolCallFunction{
						Name: tool,
						Arguments: mustMarshalJSON(map[string]interface{}{"param": "value"}),
					},
				},
			},
		})

		messages = append(messages, types.OpenAIMessage{
			Role: "tool",
			Content: tool + " executed successfully",
			ToolCallID: toolCallID,
		})
	}

	return messages
}

// buildImplementationMessages creates messages showing recent implementation work
func buildImplementationMessages() []types.OpenAIMessage {
	return []types.OpenAIMessage{
		{Role: "user", Content: "Implement authentication system"},
		{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{{ID: "write-1", Type: "function", Function: types.OpenAIToolCallFunction{Name: "Write", Arguments: mustMarshalJSON(map[string]interface{}{"file_path": "auth.js"})}}}},
		{Role: "tool", Content: "File written", ToolCallID: "write-1"},
		{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{{ID: "edit-1", Type: "function", Function: types.OpenAIToolCallFunction{Name: "Edit", Arguments: mustMarshalJSON(map[string]interface{}{"file_path": "app.js"})}}}},
		{Role: "tool", Content: "File edited", ToolCallID: "edit-1"},
		{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{{ID: "bash-1", Type: "function", Function: types.OpenAIToolCallFunction{Name: "Bash", Arguments: mustMarshalJSON(map[string]interface{}{"command": "npm test"})}}}},
		{Role: "tool", Content: "Tests passed", ToolCallID: "bash-1"},
	}
}

// buildPlanningMessages creates messages showing research/planning activity
func buildPlanningMessages() []types.OpenAIMessage {
	return []types.OpenAIMessage{
		{Role: "user", Content: "Help me design a user authentication system"},
		{Role: "assistant", Content: "I'll help you design a comprehensive authentication system. Let me analyze the requirements and create a plan."},
	}
}

// buildResearchThenPlanningMessages creates messages showing research followed by planning
func buildResearchThenPlanningMessages() []types.OpenAIMessage {
	return []types.OpenAIMessage{
		{Role: "user", Content: "Add search functionality to the application"},
		{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{{ID: "read-1", Type: "function", Function: types.OpenAIToolCallFunction{Name: "Read", Arguments: mustMarshalJSON(map[string]interface{}{"file_path": "app.js"})}}}},
		{Role: "tool", Content: "File contents analyzed", ToolCallID: "read-1"},
		{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{{ID: "grep-1", Type: "function", Function: types.OpenAIToolCallFunction{Name: "Grep", Arguments: mustMarshalJSON(map[string]interface{}{"pattern": "search"})}}}},
		{Role: "tool", Content: "Search patterns found", ToolCallID: "grep-1"},
	}
}

// TestExitPlanModeLLMValidationPromptBuilding tests the prompt building functionality
func TestExitPlanModeLLMValidationPromptBuilding(t *testing.T) {
	mockConfig := NewMockConfigProvider("http://mock-endpoint:8080/v1/chat/completions")
	service := correction.NewService(mockConfig, "test-key", true, "test-model", false, nil)

	planContent := "I've successfully completed the implementation."
	messages := buildImplementationMessages()

	// This test will fail until we implement buildExitPlanModeValidationPrompt
	prompt := service.BuildExitPlanModeValidationPrompt(planContent, messages)

	assert.Contains(t, prompt, planContent, "Prompt should contain the plan content")
	assert.Contains(t, prompt, "ExitPlanMode", "Prompt should mention ExitPlanMode")
	assert.Contains(t, prompt, "planning", "Prompt should mention planning vs completion")
}