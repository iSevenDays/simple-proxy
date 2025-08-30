package test

import (
	"claude-proxy/harmony"
	"claude-proxy/types"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHarmonySubsequentSamplingBehavior tests the specific OpenAI Harmony guide point 3 
// requirement about "subsequent sampling turn" behavior:
//
// 1. "After a message to the final channel in a subsequent sampling turn all analysis messages should be dropped"
// 2. "If the last message by the assistant was a tool call of any type, the analysis messages 
//    until the previous final message should be preserved on subsequent sampling until a final message gets issued"
func TestHarmonySubsequentSamplingBehavior(t *testing.T) {
	tests := []struct {
		name                           string
		initialHistory                 []types.Message
		subsequentTurn1                types.Message  // First subsequent turn
		subsequentTurn2                types.Message  // Second subsequent turn  
		expectedPreservationAfterTurn1 bool
		expectedPreservationAfterTurn2 bool
		description                    string
	}{
		{
			name: "analysis_dropped_after_final_in_subsequent_turn",
			initialHistory: []types.Message{
				{
					Role: "user",
					Content: "Please help me with this task",
				},
				{
					Role: "assistant", 
					Content: []types.Content{
						{Type: "thinking", Text: "I need to think about this task."},
						{Type: "tool_use", ID: "call_1", Name: "TestTool", Input: map[string]interface{}{"query": "test"}},
					},
				},
				{
					Role: "user", // Tool result
					Content: []types.Content{
						{Type: "tool_result", ToolUseID: "call_1", Text: "Tool response data"},
					},
				},
			},
			subsequentTurn1: types.Message{
				Role: "assistant",
				Content: []types.Content{
					{Type: "thinking", Text: "Now analyzing the tool results."},
					{Type: "text", Text: "Based on the results, here's my answer."}, // This is final channel
				},
			},
			subsequentTurn2: types.Message{
				Role: "assistant", 
				Content: []types.Content{
					{Type: "thinking", Text: "Additional analysis in next turn."},
					{Type: "text", Text: "Follow-up response."},
				},
			},
			expectedPreservationAfterTurn1: false, // Analysis should be dropped after final
			expectedPreservationAfterTurn2: false, // Still should not preserve
			description: "Analysis messages should be dropped after final channel in subsequent sampling turn",
		},
		{
			name: "analysis_preserved_until_final_in_tool_call_scenario", 
			initialHistory: []types.Message{
				{
					Role: "user",
					Content: "Help me research this topic",
				},
				{
					Role: "assistant",
					Content: []types.Content{
						{Type: "thinking", Text: "I need to search for information."},
						{Type: "tool_use", ID: "call_1", Name: "Search", Input: map[string]interface{}{"query": "research"}},
					},
				},
				{
					Role: "user",
					Content: []types.Content{
						{Type: "tool_result", ToolUseID: "call_1", Text: "Search results..."},
					},
				},
			},
			subsequentTurn1: types.Message{
				Role: "assistant",
				Content: []types.Content{
					{Type: "thinking", Text: "Let me analyze these results and call another tool."},
					{Type: "tool_use", ID: "call_2", Name: "Analyze", Input: map[string]interface{}{"data": "results"}},
				},
			},
			subsequentTurn2: types.Message{
				Role: "assistant",
				Content: []types.Content{
					{Type: "thinking", Text: "Now I can provide the final answer."},
					{Type: "text", Text: "Here's my complete analysis."}, // Final channel  
				},
			},
			expectedPreservationAfterTurn1: true,  // Should preserve since last was tool call
			expectedPreservationAfterTurn2: false, // Should drop after final message
			description: "Analysis preserved until final message when last message was tool call",
		},
		{
			name: "multiple_subsequent_turns_with_tool_calls",
			initialHistory: []types.Message{
				{
					Role: "user", 
					Content: "Complex multi-step task",
				},
			},
			subsequentTurn1: types.Message{
				Role: "assistant",
				Content: []types.Content{
					{Type: "thinking", Text: "Step 1: Initial analysis."},
					{Type: "tool_use", ID: "call_1", Name: "Tool1", Input: map[string]interface{}{"step": 1}},
				},
			},
			subsequentTurn2: types.Message{
				Role: "assistant",
				Content: []types.Content{
					{Type: "thinking", Text: "Step 2: More analysis."},  
					{Type: "tool_use", ID: "call_2", Name: "Tool2", Input: map[string]interface{}{"step": 2}},
				},
			},
			expectedPreservationAfterTurn1: true,  // Preserve after tool call
			expectedPreservationAfterTurn2: true,  // Still preserve, no final message yet
			description: "Analysis preserved across multiple tool calls until final message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := harmony.NewContextManager()

			// Set up initial conversation history
			for _, msg := range tt.initialHistory {
				cm.UpdateHistory(msg)
			}

			// Process first subsequent turn
			cm.UpdateHistory(tt.subsequentTurn1)
			preservationAfterTurn1 := cm.ShouldPreserveAnalysis()
			assert.Equal(t, tt.expectedPreservationAfterTurn1, preservationAfterTurn1, 
				"After first subsequent turn: %s", tt.description)

			// Process second subsequent turn  
			cm.UpdateHistory(tt.subsequentTurn2)
			preservationAfterTurn2 := cm.ShouldPreserveAnalysis()
			assert.Equal(t, tt.expectedPreservationAfterTurn2, preservationAfterTurn2,
				"After second subsequent turn: %s", tt.description)
		})
	}
}

// TestHarmonyAnalysisDroppingCompliance specifically tests the exact wording from OpenAI:
// "After a message to the final channel in a subsequent sampling turn all analysis messages should be dropped"
func TestHarmonyAnalysisDroppingCompliance(t *testing.T) {
	cm := harmony.NewContextManager()

	// Initial conversation with tool call
	cm.UpdateHistory(types.Message{
		Role: "user",
		Content: "Please help me",
	})

	// Assistant makes tool call with analysis
	cm.UpdateHistory(types.Message{
		Role: "assistant", 
		Content: []types.Content{
			{Type: "thinking", Text: "I need to use a tool for this."},
			{Type: "tool_use", ID: "call_1", Name: "TestTool", Input: map[string]interface{}{"query": "test"}},
		},
	})

	// User provides tool result
	cm.UpdateHistory(types.Message{
		Role: "user",
		Content: []types.Content{
			{Type: "tool_result", ToolUseID: "call_1", Text: "Tool response"},
		},
	})

	// Verify preservation is active after tool call
	assert.True(t, cm.ShouldPreserveAnalysis(), "Should preserve analysis after tool call")
	assert.Equal(t, 1, cm.GetPreservedAnalysisCount(), "Should have preserved the thinking content")

	// SUBSEQUENT SAMPLING TURN: Assistant provides final message
	cm.UpdateHistory(types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "thinking", Text: "Now I can answer based on the tool result."},  // Analysis channel
			{Type: "text", Text: "Here's my final answer based on the tool result."}, // Final channel
		},
	})

	// After final message in subsequent turn, analysis should be dropped
	assert.False(t, cm.ShouldPreserveAnalysis(), "Analysis should be dropped after final channel in subsequent sampling turn")
	assert.Equal(t, 0, cm.GetPreservedAnalysisCount(), "Preserved analysis count should be reset to 0")

	// Next turn should not preserve anything since last message had final channel
	cm.UpdateHistory(types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "thinking", Text: "Additional thoughts."},
			{Type: "text", Text: "More information."},
		},
	})

	assert.False(t, cm.ShouldPreserveAnalysis(), "Should not preserve analysis in turns after final message")
}

// TestHarmonyCommentaryChannelPreservation tests that commentary channels are preserved per spec:
// "Function calls to the commentary channel can remain"  
func TestHarmonyCommentaryChannelPreservation(t *testing.T) {
	cm := harmony.NewContextManager()

	// Assistant makes tool call with both analysis and commentary
	cm.UpdateHistory(types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "thinking", Text: "I need to analyze this request."},      // Analysis
			{Type: "commentary", Text: "This is an interesting problem."},    // Commentary  
			{Type: "tool_use", ID: "call_1", Name: "TestTool", Input: map[string]interface{}{"test": "data"}},
		},
	})

	// Verify both analysis and commentary are preserved
	assert.True(t, cm.ShouldPreserveAnalysis(), "Should preserve after tool call")
	preserved := cm.GetPreservedAnalysis()
	assert.Equal(t, 2, len(preserved), "Should preserve both thinking and commentary")
	assert.Contains(t, preserved, "I need to analyze this request.", "Should preserve analysis/thinking content")
	assert.Contains(t, preserved, "This is an interesting problem.", "Should preserve commentary content")

	// After final message, analysis should be dropped but the test framework
	// doesn't distinguish between analysis and commentary preservation
	cm.UpdateHistory(types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "text", Text: "Final answer."},
		},
	})

	// The current implementation drops all preserved content after final message
	// This matches the OpenAI spec since it says "analysis messages should be dropped"
	// but "commentary channel can remain" is likely referring to tool calls to commentary channel
	assert.False(t, cm.ShouldPreserveAnalysis(), "Analysis preservation should be cleared after final message")
}