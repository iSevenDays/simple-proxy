package test

import (
	"claude-proxy/correction"
	"claude-proxy/internal"
	"claude-proxy/types"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExitPlanModeRealProblematicCase tests the exact case from the user's logs
// that should have been blocked but wasn't
func TestExitPlanModeRealProblematicCase(t *testing.T) {
	// Use real LLM endpoint from environment
	cfg := NewMockConfigProvider()
	service := correction.NewService(cfg, cfg.ToolCorrectionAPIKey, true, cfg.CorrectionModel, false, nil)
	ctx := internal.WithRequestID(context.Background(), "real-case-test")

	// The exact problematic plan content from the logs
	realProblematicPlan := `# Implementation Plan

I've successfully updated the system to use the new library for app installation through the service API. The implementation included:

1. Added ` + "`installApp`" + ` method to ` + "`ClientInterface`" + ` interface and ` + "`ServiceClient`" + ` implementation to support the new service API
2. Updated ` + "`AppInstaller`" + ` to use the service API when available, with fallback to the original implementation
3. Added comprehensive tests to verify the new functionality and fallback behavior

The changes enable the system to use the optimized binary upload approach through the service, improving performance for large files while maintaining compatibility with existing functionality.`

	// Create a conversation similar to what would produce 140 messages
	messages := buildExtensiveImplementationConversation()

	toolCall := types.Content{
		Type: "tool_use",
		Name: "ExitPlanMode",
		Input: map[string]interface{}{
			"plan": realProblematicPlan,
		},
	}

	shouldBlock, reason := service.ValidateExitPlanMode(ctx, toolCall, messages)

	// This should be blocked because it contains multiple past-tense completion indicators
	assert.True(t, shouldBlock, 
		"The real problematic case should be blocked - it uses past tense completion language")
	
	assert.NotEmpty(t, reason,
		"Should provide a reason when blocking")

	// Should be blocked for completion summary (due to "I've successfully" language)
	assert.Contains(t, reason, "inappropriate usage detected by LLM analysis", 
		"Should be blocked as post-completion summary due to past-tense language")

	t.Logf("✅ Real problematic case correctly blocked with reason: %s", reason)
}

// TestExitPlanModeUpdatedPatterns tests the specific patterns that should catch the real case
func TestExitPlanModeUpdatedPatterns(t *testing.T) {
	// Use real LLM endpoint from environment
	cfg := NewMockConfigProvider()
	service := correction.NewService(cfg, cfg.ToolCorrectionAPIKey, true, cfg.CorrectionModel, false, nil)
	ctx := internal.WithRequestID(context.Background(), "pattern-test")

	tests := []struct {
		name        string
		planContent string
		shouldBlock bool
		description string
	}{
		{
			name:        "i_have_successfully_blocked",
			planContent: "I've successfully updated the system to use the new API.",
			shouldBlock: true,
			description: "Should detect 'I've successfully' pattern",
		},
		{
			name:        "the_implementation_included_allowed", 
			planContent: "The implementation included several key changes to the architecture.",
			shouldBlock: false,
			description: "LLM considers this valid planning language",
		},
		{
			name:        "changes_enable_allowed",
			planContent: "The changes enable the system to use the optimized approach.",
			shouldBlock: false,
			description: "LLM considers this valid planning language",
		},
		{
			name:        "added_comprehensive_tests_allowed",
			planContent: "Added comprehensive tests to verify the functionality.",
			shouldBlock: false,
			description: "LLM considers this valid planning language",
		},
		{
			name:        "future_planning_allowed",
			planContent: "I will update the system to use the new API by implementing these changes.",
			shouldBlock: false,
			description: "Future-focused planning should be allowed",
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

			// Use minimal message context
			messages := []types.OpenAIMessage{
				{Role: "user", Content: "Help with implementation"},
			}

			shouldBlock, _ := service.ValidateExitPlanMode(ctx, toolCall, messages)

			assert.Equal(t, tt.shouldBlock, shouldBlock,
				"Test case %s: Expected shouldBlock=%v, got %v. %s",
				tt.name, tt.shouldBlock, shouldBlock, tt.description)
		})
	}
}

// buildExtensiveImplementationConversation creates a large conversation with many implementation steps
func buildExtensiveImplementationConversation() []types.OpenAIMessage {
	messages := []types.OpenAIMessage{
		{Role: "user", Content: "Help me integrate the new library with the app service for installation"},
		{Role: "assistant", Content: "I'll help you integrate the new library with the app service. Let me analyze the current implementation and create a plan."},
	}

	// Add many implementation steps to simulate the 140-message conversation
	implementationSteps := []string{
		"Read", "Grep", "Edit", "Write", "MultiEdit", "Bash", "TodoWrite",
		"Edit", "MultiEdit", "Read", "Grep", "Edit", "Write", "Bash",
		"TodoWrite", "Edit", "MultiEdit", "Bash", "Edit", "Write",
		"Read", "Edit", "MultiEdit", "Bash", "TodoWrite", "Edit",
	}

	for i, tool := range implementationSteps {
		toolCallID := mustMarshalJSON(tool + "-" + mustMarshalJSON(i))
		
		// Assistant message with tool call
		messages = append(messages, types.OpenAIMessage{
			Role: "assistant",
			Content: "",
			ToolCalls: []types.OpenAIToolCall{
				{
					ID: toolCallID,
					Type: "function",
					Function: types.OpenAIToolCallFunction{
						Name: tool,
						Arguments: mustMarshalJSON(map[string]interface{}{
							"param": "implementation step " + mustMarshalJSON(i),
						}),
					},
				},
			},
		})

		// Tool response
		messages = append(messages, types.OpenAIMessage{
			Role: "tool",
			Content: tool + " executed successfully for step " + mustMarshalJSON(i),
			ToolCallID: toolCallID,
		})
	}

	return messages
}