package test

import (
	"claude-proxy/harmony"
	"claude-proxy/parser"
	"claude-proxy/types"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHarmonyAnalysisPreservationAfterToolCall tests compliance with OpenAI Harmony guide point 3:
// "If the last message by the assistant was a tool call of any type, the analysis messages 
// until the previous final message should be preserved on subsequent sampling until a final message gets issued"
func TestHarmonyAnalysisPreservationAfterToolCall(t *testing.T) {
	tests := []struct {
		name                    string
		conversationHistory     []types.Message
		currentResponse         string
		expectedPreserveAnalysis bool
		expectedAnalysisContent  string
		description             string
	}{
		{
			name: "preserve_analysis_after_tool_call",
			conversationHistory: []types.Message{
				{
					Role: "assistant",
					Content: []types.Content{
						{Type: "thinking", Text: "I need to search for this information."},
						{Type: "tool_use", ID: "call_1", Name: "WebSearch", Input: map[string]interface{}{"query": "test"}},
					},
				},
			},
			currentResponse:         `<|start|>assistant<|channel|>analysis<|message|>Analyzing the search results...<|end|><|start|>assistant<|channel|>final<|message|>Based on my search, here's the answer.<|end|>`,
			expectedPreserveAnalysis: true,
			expectedAnalysisContent:  "I need to search for this information.",
			description:             "Should preserve analysis content from before tool call when final message is issued",
		},
		{
			name: "no_preservation_after_final_message",
			conversationHistory: []types.Message{
				{
					Role: "assistant",
					Content: []types.Content{
						{Type: "thinking", Text: "Previous thinking."},
						{Type: "text", Text: "Here's my final answer."},
					},
				},
			},
			currentResponse:         `<|start|>assistant<|channel|>analysis<|message|>New analysis...<|end|><|start|>assistant<|channel|>final<|message|>New response.<|end|>`,
			expectedPreserveAnalysis: false,
			expectedAnalysisContent:  "",
			description:             "Should not preserve analysis if last message was not a tool call",
		},
		{
			name: "preserve_until_final_message_issued",
			conversationHistory: []types.Message{
				{
					Role: "assistant",
					Content: []types.Content{
						{Type: "thinking", Text: "Original analysis content."},
						{Type: "tool_use", ID: "call_2", Name: "Read", Input: map[string]interface{}{"file": "test.txt"}},
					},
				},
			},
			currentResponse:         `<|start|>assistant<|channel|>analysis<|message|>Still thinking...<|end|>`,
			expectedPreserveAnalysis: true,
			expectedAnalysisContent:  "Original analysis content.",
			description:             "Should preserve analysis when only analysis channel (no final) is present after tool call",
		},
		{
			name: "multiple_tool_calls_preserve_from_last_final",
			conversationHistory: []types.Message{
				{
					Role: "assistant",
					Content: []types.Content{
						{Type: "thinking", Text: "First analysis."},
						{Type: "text", Text: "Intermediate response."},
					},
				},
				{
					Role: "user",
					Content: []types.Content{
						{Type: "text", Text: "Follow-up question."},
					},
				},
				{
					Role: "assistant",
					Content: []types.Content{
						{Type: "thinking", Text: "Second analysis after final."},
						{Type: "tool_use", ID: "call_3", Name: "Bash", Input: map[string]interface{}{"command": "ls"}},
					},
				},
			},
			currentResponse:         `<|start|>assistant<|channel|>final<|message|>Here's the result.<|end|>`,
			expectedPreserveAnalysis: true,
			expectedAnalysisContent:  "Second analysis after final.",
			description:             "Should preserve analysis from after the last final message when tool call was last",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create context manager and populate with conversation history
			cm := harmony.NewContextManager()
			for _, msg := range tt.conversationHistory {
				cm.UpdateHistory(msg)
			}

			// Parse the current response
			harmonyMsg, err := parser.ParseHarmonyMessage(tt.currentResponse)
			require.NoError(t, err)
			require.True(t, harmonyMsg.HasHarmony)

			// Check if we should preserve analysis based on conversation history
			shouldPreserve := cm.ShouldPreserveAnalysis()
			preservedContent := ""
			if shouldPreserve {
				preserved := cm.GetPreservedAnalysis()
				if len(preserved) > 0 {
					preservedContent = preserved[0] // Get first preserved analysis for testing
				}
			}
			
			assert.Equal(t, tt.expectedPreserveAnalysis, shouldPreserve, tt.description)
			if tt.expectedPreserveAnalysis {
				assert.Equal(t, tt.expectedAnalysisContent, preservedContent, "Preserved analysis content should match expected")
			}
		})
	}
}

// TestHarmonyContextManagerInterface tests the interface that should be implemented
// for proper Harmony context management according to the OpenAI guide
func TestHarmonyContextManagerInterface(t *testing.T) {
	// Test the HarmonyContextManager interface implementation
	// The implementation provides:
	// 1. Conversation history tracking
	// 2. Tool call identification and analysis preservation
	// 3. Proper preservation from after last final message
	// 4. Preservation clearing when final message is issued
	
	cm := harmony.NewContextManager()
	
	// Test basic interface methods
	assert.False(t, cm.ShouldPreserveAnalysis(), "Initially should not preserve analysis")
	assert.Equal(t, 0, cm.GetPreservedAnalysisCount(), "Initially should have no preserved analysis")
	assert.Equal(t, harmony.MessageTypeUnknown, cm.GetLastMessageType(), "Initially should have unknown message type")
	
	// Test history updates
	msg := types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "thinking", Text: "Test analysis"},
			{Type: "tool_use", ID: "call_1", Name: "TestTool"},
		},
	}
	cm.UpdateHistory(msg)
	
	assert.True(t, cm.ShouldPreserveAnalysis(), "Should preserve analysis after tool call")
	assert.Equal(t, 1, cm.GetPreservedAnalysisCount(), "Should have one preserved analysis message")
	assert.Equal(t, harmony.MessageTypeToolCall, cm.GetLastMessageType(), "Should identify tool call message type")
	
	// Test clearing
	cm.ClearPreservedAnalysis()
	assert.False(t, cm.ShouldPreserveAnalysis(), "Should not preserve analysis after clearing")
}

// TestHarmonyPreservationIntegration tests the integration of Harmony preservation
// with the existing transformation pipeline
func TestHarmonyPreservationIntegration(t *testing.T) {
	// Test integration with the transformation pipeline
	
	// Create a context manager for integration testing
	cm := harmony.NewContextManager()
	
	// Test realistic conversation flow: user question → analysis + tool call → tool result → final response
	userMsg := types.Message{
		Role: "user",
		Content: []types.Content{{Type: "text", Text: "What's the weather?"}},
	}
	cm.UpdateHistory(userMsg)
	
	// Assistant with analysis and tool call
	assistantMsg1 := types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "thinking", Text: "Need to get weather information"},
			{Type: "tool_use", ID: "weather_1", Name: "GetWeather"},
		},
	}
	cm.UpdateHistory(assistantMsg1)
	assert.True(t, cm.ShouldPreserveAnalysis(), "Should preserve analysis after tool call")
	
	// Tool result
	toolMsg := types.Message{
		Role: "tool",
		Content: []types.Content{{Type: "tool_result", Text: "Sunny, 75°F"}},
	}
	cm.UpdateHistory(toolMsg)
	assert.True(t, cm.ShouldPreserveAnalysis(), "Should still preserve analysis after tool result")
	
	// Verify that preserved context is built correctly BEFORE final message
	preservedContext := cm.BuildPreservedContext()
	expected := "<|start|>assistant<|channel|>analysis<|message|>Need to get weather information<|end|>"
	assert.Equal(t, expected, preservedContext, "Preserved context should be properly formatted")
	
	// Assistant final response - this should clear the preserved analysis
	assistantMsg2 := types.Message{
		Role: "assistant",
		Content: []types.Content{{Type: "text", Text: "It's sunny and 75°F today!"}},
	}
	cm.UpdateHistory(assistantMsg2)
	
	// After final message, preserved analysis should be cleared
	preservedContextAfterFinal := cm.BuildPreservedContext()
	assert.Equal(t, "", preservedContextAfterFinal, "Preserved context should be cleared after final message")
}

// TestHarmonyPreservationEdgeCases tests edge cases for analysis preservation
func TestHarmonyPreservationEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		scenario    string
		expectation string
	}{
		{
			name:        "no_previous_final_message",
			scenario:    "Tool call without any previous final message",
			expectation: "Should preserve from conversation start",
		},
		{
			name:        "multiple_consecutive_tool_calls",
			scenario:    "Multiple tool calls in sequence",
			expectation: "Should preserve from after last final until all tool calls",
		},
		{
			name:        "mixed_channels_after_tool_call",
			scenario:    "Analysis, commentary, and final channels after tool call",
			expectation: "Should preserve analysis but clear after final",
		},
		{
			name:        "empty_analysis_channels",
			scenario:    "Analysis channels with empty content",
			expectation: "Should handle empty content gracefully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test edge case: " + tt.expectation
			cm := harmony.NewContextManager()
			
			switch tt.name {
			case "no_previous_final_message":
				// Tool call without any previous final message
				toolCallMsg := types.Message{
					Role: "assistant",
					Content: []types.Content{
						{Type: "thinking", Text: "Starting analysis"},
						{Type: "tool_use", ID: "call_1", Name: "Tool"},
					},
				}
				cm.UpdateHistory(toolCallMsg)
				assert.True(t, cm.ShouldPreserveAnalysis(), "Should preserve from conversation start")
				assert.Equal(t, 1, cm.GetPreservedAnalysisCount(), "Should have preserved analysis from start")
				
			case "multiple_consecutive_tool_calls":
				// Multiple tool calls in sequence
				for i := 0; i < 3; i++ {
					msg := types.Message{
						Role: "assistant",
						Content: []types.Content{
							{Type: "thinking", Text: fmt.Sprintf("Analysis %d", i+1)},
							{Type: "tool_use", ID: fmt.Sprintf("call_%d", i+1), Name: "Tool"},
						},
					}
					cm.UpdateHistory(msg)
				}
				assert.True(t, cm.ShouldPreserveAnalysis(), "Should preserve after multiple tool calls")
				assert.Equal(t, 3, cm.GetPreservedAnalysisCount(), "Should preserve all analysis messages")
				
			case "mixed_channels_after_tool_call":
				// Analysis, commentary, and final channels after tool call
				toolMsg := types.Message{
					Role: "assistant",
					Content: []types.Content{{Type: "tool_use", ID: "call_1", Name: "Tool"}},
				}
				cm.UpdateHistory(toolMsg)
				
				mixedMsg := types.Message{
					Role: "assistant",
					Content: []types.Content{
						{Type: "thinking", Text: "Analysis content"},
						{Type: "text", Text: "Final response"},
					},
				}
				cm.UpdateHistory(mixedMsg)
				assert.False(t, cm.ShouldPreserveAnalysis(), "Should not preserve after final message")
				
			case "empty_analysis_channels":
				// Analysis channels with empty content
				emptyMsg := types.Message{
					Role: "assistant",
					Content: []types.Content{
						{Type: "thinking", Text: ""},
						{Type: "tool_use", ID: "call_1", Name: "Tool"},
					},
				}
				cm.UpdateHistory(emptyMsg)
				assert.True(t, cm.ShouldPreserveAnalysis(), "Should handle empty content gracefully")
				assert.Equal(t, 0, cm.GetPreservedAnalysisCount(), "Should have no preserved content for empty analysis")
			}
		})
	}
}