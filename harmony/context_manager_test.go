package harmony

import (
	"claude-proxy/parser"
	"claude-proxy/types"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestContextManagerBasicFunctionality tests the core functionality of ContextManager
func TestContextManagerBasicFunctionality(t *testing.T) {
	cm := NewContextManager()

	// Test initial state
	assert.False(t, cm.ShouldPreserveAnalysis())
	assert.Equal(t, 0, cm.GetPreservedAnalysisCount())
	assert.Equal(t, MessageTypeUnknown, cm.GetLastMessageType())

	// Test updating with a tool call message
	toolCallMsg := types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "thinking", Text: "I need to use a tool"},
			{Type: "tool_use", ID: "call_1", Name: "TestTool"},
		},
	}
	cm.UpdateHistory(toolCallMsg)

	assert.True(t, cm.ShouldPreserveAnalysis())
	assert.Equal(t, 1, cm.GetPreservedAnalysisCount())
	assert.Equal(t, MessageTypeToolCall, cm.GetLastMessageType())

	// Test preserved content
	preserved := cm.GetPreservedAnalysis()
	assert.Len(t, preserved, 1)
	assert.Equal(t, "I need to use a tool", preserved[0])
}

// TestContextManagerFinalMessageClearing tests that final messages clear preservation
func TestContextManagerFinalMessageClearing(t *testing.T) {
	cm := NewContextManager()

	// Add a tool call message
	toolCallMsg := types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "thinking", Text: "Analyzing the problem"},
			{Type: "tool_use", ID: "call_1", Name: "TestTool"},
		},
	}
	cm.UpdateHistory(toolCallMsg)
	assert.True(t, cm.ShouldPreserveAnalysis())

	// Add a final message
	finalMsg := types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "text", Text: "Here's the final answer"},
		},
	}
	cm.UpdateHistory(finalMsg)

	assert.False(t, cm.ShouldPreserveAnalysis())
	assert.Equal(t, 0, cm.GetPreservedAnalysisCount())
	assert.Equal(t, MessageTypeFinal, cm.GetLastMessageType())
}

// TestContextManagerStringContent tests handling of string content messages
func TestContextManagerStringContent(t *testing.T) {
	cm := NewContextManager()

	// Test string content (typical for simple messages)
	stringMsg := types.Message{
		Role:    "assistant",
		Content: "This is a simple string response",
	}
	cm.UpdateHistory(stringMsg)

	assert.False(t, cm.ShouldPreserveAnalysis())
	assert.Equal(t, MessageTypeFinal, cm.GetLastMessageType())
}

// TestContextManagerMultipleToolCalls tests preservation across multiple tool calls
func TestContextManagerMultipleToolCalls(t *testing.T) {
	cm := NewContextManager()

	// First tool call
	toolCall1 := types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "thinking", Text: "First analysis"},
			{Type: "tool_use", ID: "call_1", Name: "Tool1"},
		},
	}
	cm.UpdateHistory(toolCall1)

	// Second tool call
	toolCall2 := types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "thinking", Text: "Second analysis"},
			{Type: "tool_use", ID: "call_2", Name: "Tool2"},
		},
	}
	cm.UpdateHistory(toolCall2)

	assert.True(t, cm.ShouldPreserveAnalysis())
	assert.Equal(t, 2, cm.GetPreservedAnalysisCount())

	preserved := cm.GetPreservedAnalysis()
	assert.Contains(t, preserved, "First analysis")
	assert.Contains(t, preserved, "Second analysis")
}

// TestContextManagerBuildPreservedContext tests Harmony format generation
func TestContextManagerBuildPreservedContext(t *testing.T) {
	cm := NewContextManager()

	// Add tool call with analysis
	toolCallMsg := types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "thinking", Text: "Need to analyze this carefully"},
			{Type: "tool_use", ID: "call_1", Name: "TestTool"},
		},
	}
	cm.UpdateHistory(toolCallMsg)

	// Build preserved context
	preserved := cm.BuildPreservedContext()
	expected := "<|start|>assistant<|channel|>analysis<|message|>Need to analyze this carefully<|end|>"
	assert.Equal(t, expected, preserved)
}

// TestContextManagerValidation tests the validation functionality
func TestContextManagerValidation(t *testing.T) {
	cm := NewContextManager()

	// Test with valid state
	toolCallMsg := types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "thinking", Text: "Valid analysis"},
			{Type: "tool_use", ID: "call_1", Name: "TestTool"},
		},
	}
	cm.UpdateHistory(toolCallMsg)

	errors := cm.ValidateHarmonyCompliance()
	assert.Empty(t, errors, "Should have no validation errors for valid state")
}

// TestContextManagerReset tests the reset functionality
func TestContextManagerReset(t *testing.T) {
	cm := NewContextManager()

	// Add some history
	toolCallMsg := types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "thinking", Text: "Some analysis"},
			{Type: "tool_use", ID: "call_1", Name: "TestTool"},
		},
	}
	cm.UpdateHistory(toolCallMsg)

	assert.True(t, cm.ShouldPreserveAnalysis())
	assert.Equal(t, 1, cm.GetHistoryLength())

	// Reset and verify
	cm.Reset()
	assert.False(t, cm.ShouldPreserveAnalysis())
	assert.Equal(t, 0, cm.GetHistoryLength())
	assert.Equal(t, 0, cm.GetPreservedAnalysisCount())
	assert.Equal(t, MessageTypeUnknown, cm.GetLastMessageType())
}

// TestHarmonyIntegration tests integration between ContextManager and parser
func TestHarmonyIntegration(t *testing.T) {
	// Test real Harmony format parsing works
	content := `<|start|>assistant<|channel|>analysis<|message|>Need to search for information<|end|><|start|>assistant<|channel|>final<|message|>Here is the answer<|end|>`
	
	msg, err := parser.ParseHarmonyMessage(content)
	assert.NoError(t, err)
	assert.True(t, msg.HasHarmony)
	assert.Equal(t, 2, len(msg.Channels))
	assert.Equal(t, "Need to search for information", msg.ThinkingText)
	assert.Equal(t, "Here is the answer", msg.ResponseText)
	
	// Test ContextManager with tool call scenario
	cm := NewContextManager()
	toolMsg := types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "thinking", Text: "Need to search for information"},
			{Type: "tool_use", ID: "search_1", Name: "WebSearch"},
		},
	}
	cm.UpdateHistory(toolMsg)
	
	// Should preserve analysis after tool call
	assert.True(t, cm.ShouldPreserveAnalysis())
	preserved := cm.GetPreservedAnalysis()
	assert.Len(t, preserved, 1)
	assert.Equal(t, "Need to search for information", preserved[0])
	
	// Generated preserved context should be valid Harmony format
	preservedContext := cm.BuildPreservedContext()
	assert.True(t, parser.IsHarmonyFormat(preservedContext))
	
	// Parse the generated context to verify structure
	parsedPreserved, err := parser.ParseHarmonyMessage(preservedContext)
	assert.NoError(t, err)
	assert.True(t, parsedPreserved.HasHarmony)
	assert.Equal(t, "Need to search for information", parsedPreserved.ThinkingText)
}

// TestSessionPersistence tests that ContextManager maintains state across requests
func TestSessionPersistence(t *testing.T) {
	// This test will verify the SessionCache functionality once implemented
	// For now, it documents the expected behavior
	
	// Test 1: Same session should return same ContextManager
	_ = "test-session-123"
	
	// Mock session cache behavior
	// cm1 := getContextManagerForSession(sessionID)
	// cm2 := getContextManagerForSession(sessionID) 
	// assert.Same(t, cm1, cm2, "Same session should return same ContextManager instance")
	
	// Test 2: Different sessions should have different ContextManagers
	// cm3 := getContextManagerForSession("different-session")
	// assert.NotSame(t, cm1, cm3, "Different sessions should have different ContextManager instances")
	
	// Test 3: Session cleanup should remove old entries
	// cleanupSessions(time.Hour)  // cleanup sessions older than 1 hour
	// verifySessionCount(2) // should still have 2 sessions
	
	t.Skip("Session persistence test - implementation pending")
}

// TestContextManagerMemoryBounds tests that ContextManager limits history size
func TestContextManagerMemoryBounds(t *testing.T) {
	cm := NewContextManager()
	
	// Add more messages than the limit (assume limit is 50)
	for i := 0; i < 60; i++ {
		msg := types.Message{
			Role: "assistant",
			Content: []types.Content{
				{Type: "thinking", Text: fmt.Sprintf("Analysis %d", i)},
				{Type: "text", Text: fmt.Sprintf("Response %d", i)},
			},
		}
		cm.UpdateHistory(msg)
	}
	
	// History should be bounded
	historyLength := cm.GetHistoryLength()
	assert.LessOrEqual(t, historyLength, 50, "History should not exceed maximum limit")
	
	// Should still function correctly
	assert.Equal(t, MessageTypeFinal, cm.GetLastMessageType())
}

// TestContextManagerCommentaryChannelPreservation tests commentary channel preservation per Harmony spec
func TestContextManagerCommentaryChannelPreservation(t *testing.T) {
	cm := NewContextManager()

	// Add a tool call with both analysis and commentary content
	toolCallMsg := types.Message{
		Role: "assistant",
		Content: []types.Content{
			{Type: "thinking", Text: "Need to call a function"},
			{Type: "commentary", Text: "Let me search for information"},
			{Type: "tool_use", ID: "call_1", Name: "SearchTool"},
		},
	}
	cm.UpdateHistory(toolCallMsg)

	assert.True(t, cm.ShouldPreserveAnalysis())
	assert.Equal(t, 2, cm.GetPreservedAnalysisCount()) // Both thinking and commentary

	preserved := cm.GetPreservedAnalysis()
	assert.Contains(t, preserved, "Need to call a function")
	assert.Contains(t, preserved, "Let me search for information")

	// Verify preserved context contains both
	preservedContext := cm.BuildPreservedContext()
	assert.Contains(t, preservedContext, "Need to call a function")
	assert.Contains(t, preservedContext, "Let me search for information")
}

// TestContextManagerConcurrency tests thread safety of ContextManager operations
func TestContextManagerConcurrency(t *testing.T) {
	cm := NewContextManager()
	
	// Test concurrent UpdateHistory calls
	done := make(chan bool, 20)
	
	for i := 0; i < 20; i++ {
		go func(id int) {
			defer func() { done <- true }()
			
			msg := types.Message{
				Role: "assistant",
				Content: []types.Content{
					{Type: "thinking", Text: fmt.Sprintf("Concurrent analysis %d", id)},
					{Type: "tool_use", ID: fmt.Sprintf("call_%d", id), Name: "Tool"},
				},
			}
			cm.UpdateHistory(msg)
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify state consistency
	historyLength := cm.GetHistoryLength()
	assert.Equal(t, 20, historyLength, "All messages should be recorded")
	assert.True(t, cm.ShouldPreserveAnalysis(), "Should preserve analysis after tool calls")
	
	// Verify no data races or corruption
	preserved := cm.GetPreservedAnalysis()
	assert.LessOrEqual(t, len(preserved), 20, "Preserved analysis should not exceed message count")
}