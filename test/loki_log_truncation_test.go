package test

import (
	"claude-proxy/internal"
	"claude-proxy/proxy"
	"claude-proxy/types"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLokiLogTruncationIssue tests the specific truncation issue from the Loki logs
func TestLokiLogTruncationIssue(t *testing.T) {
	// This reproduces the exact truncation issue from the Loki logs
	// Full logged content vs what Claude Code UI shows
	
	fullLoggedContent := `✅ PRD created: .claude/prds/installation-outcome-idea-b.md

The PRD defines a refactoring to use an InstallationOutcome record that captures the final result of an iOS app installation. This ensures metrics like app-installation.ios are reported exactly once per request, giving developers accurate visibility into installation success/failure rates, independent of internal fallback attempts.

Ready to create implementation epic? Run: /pm:prd-parse installation-outcome-idea-b`

	// This is what Claude Code UI actually shows (truncated version)
	expectedTruncatedContent := `✅ PRD created: .claude/prds/installation-outcome-idea-b.md

The PRD defines a refactoring to use an InstallationOutcome record that captures the curate visibility into installation success/failure rates, independent of internal fallback attempts.

Ready to create implementation epic? Run:`

	openaiResponse := types.OpenAIResponse{
		ID:     "chatcmpl-8rpOza3M2jwODmmRJz49egkyFsBs4Q2T",
		Object: "chat.completion", 
		Model:  "claude-sonnet-4-20250514",
		Choices: []types.OpenAIChoice{
			{
				Message: types.OpenAIMessage{
					Role:    "assistant",
					Content: fullLoggedContent,
				},
				FinishReason: stringPtr("stop"),
			},
		},
		Usage: types.OpenAIUsage{
			PromptTokens:     0,
			CompletionTokens: 0,
		},
	}

	ctx := internal.WithRequestID(context.Background(), "loki_truncation_test")
	result, err := proxy.TransformOpenAIToAnthropic(ctx, &openaiResponse, "claude-sonnet-4-20250514", getTestConfig())
	require.NoError(t, err)

	actualContent := result.Content[0].Text

	t.Logf("Full logged content length: %d", len(fullLoggedContent))
	t.Logf("Actual transformed content length: %d", len(actualContent))
	t.Logf("Expected truncated content length: %d", len(expectedTruncatedContent))

	// Primary assertion - transformation should preserve complete content
	assert.Equal(t, fullLoggedContent, actualContent, "Content transformation should preserve full content")

	// Verify the missing command is present in transformed content
	assert.Contains(t, actualContent, "/pm:prd-parse installation-outcome-idea-b", 
		"The complete command should be present")

	// Verify the missing text is present
	assert.Contains(t, actualContent, "final result of an iOS app installation", 
		"The full descriptive text should be present")

	// Verify the response ends with the complete command
	assert.True(t, strings.HasSuffix(actualContent, "/pm:prd-parse installation-outcome-idea-b"),
		"Response should end with the complete command")

	// Test if this is a streaming-specific issue
	// If actualContent matches fullLoggedContent but Claude Code UI shows truncated,
	// then the issue is in streaming delivery, not content transformation
	if actualContent == fullLoggedContent {
		t.Logf("✅ DIAGNOSIS: Content transformation is working correctly")
		t.Logf("❌ ISSUE: Truncation occurs during streaming delivery to Claude Code UI")
		t.Logf("🔍 ROOT CAUSE: Likely client-side streaming timeout or incomplete chunk delivery")
	} else {
		t.Logf("❌ ISSUE: Content transformation is truncating content")
		t.Logf("🔍 ROOT CAUSE: Bug in TransformOpenAIToAnthropic or related processing")
	}
}

// TestStreamingChunkDelivery tests if streaming splits this content properly
func TestStreamingChunkDelivery(t *testing.T) {
	// Test the exact content from Loki logs with streaming chunking
	content := `✅ PRD created: .claude/prds/installation-outcome-idea-b.md

The PRD defines a refactoring to use an InstallationOutcome record that captures the final result of an iOS app installation. This ensures metrics like app-installation.ios are reported exactly once per request, giving developers accurate visibility into installation success/failure rates, independent of internal fallback attempts.

Ready to create implementation epic? Run: /pm:prd-parse installation-outcome-idea-b`

	// Since splitTextForStreaming is not exposed, we'll test via full streaming transformation
	// This will exercise the complete streaming pipeline including chunking
	openaiResponse := types.OpenAIResponse{
		ID:     "streaming_test",
		Object: "chat.completion", 
		Model:  "claude-sonnet-4-20250514",
		Choices: []types.OpenAIChoice{
			{
				Message: types.OpenAIMessage{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: stringPtr("stop"),
			},
		},
	}

	ctx := internal.WithRequestID(context.Background(), "streaming_chunk_test")
	result, err := proxy.TransformOpenAIToAnthropic(ctx, &openaiResponse, "claude-sonnet-4-20250514", getTestConfig())
	require.NoError(t, err)

	actualContent := result.Content[0].Text
	
	t.Logf("Original content length: %d", len(content))
	t.Logf("Transformed content length: %d", len(actualContent))
	
	// Verify no content loss during transformation (which includes chunking logic)
	assert.Equal(t, content, actualContent, "Streaming transformation should not lose any content")
	
	// Verify the critical end command is preserved
	assert.Contains(t, actualContent, "/pm:prd-parse installation-outcome-idea-b", 
		"The end command should be preserved in streaming transformation")
	
	// Verify the response ends with the complete command
	assert.True(t, strings.HasSuffix(actualContent, "/pm:prd-parse installation-outcome-idea-b"),
		"Response should end with the complete command")
}

// TestClientStreamingTimeout simulates streaming timeout scenario
func TestClientStreamingTimeout(t *testing.T) {
	// This test demonstrates how streaming timeout could cause truncation
	content := `✅ PRD created: .claude/prds/installation-outcome-idea-b.md

The PRD defines a refactoring to use an InstallationOutcome record that captures the final result of an iOS app installation. This ensures metrics like app-installation.ios are reported exactly once per request, giving developers accurate visibility into installation success/failure rates, independent of internal fallback attempts.

Ready to create implementation epic? Run: /pm:prd-parse installation-outcome-idea-b`

	// Simulate what happens if client stops reading after certain amount of content
	maxClientReadLength := 300 // Simulate client timeout/buffer limit
	
	clientReceivedContent := content
	if len(content) > maxClientReadLength {
		clientReceivedContent = content[:maxClientReadLength]
		t.Logf("⚠️ Simulated client timeout - received only %d/%d characters", 
			maxClientReadLength, len(content))
	}
	
	// Check if the missing command would be lost due to client timeout
	if !strings.Contains(clientReceivedContent, "/pm:prd-parse installation-outcome-idea-b") {
		t.Logf("❌ CONFIRMED: Client timeout would lose the end command")
		t.Logf("Client received: %q", clientReceivedContent)
		t.Logf("Missing content: %q", content[len(clientReceivedContent):])
	} else {
		t.Logf("✅ Command would survive client timeout at %d chars", maxClientReadLength)
	}
	
	// Test with different timeout lengths to find critical point
	criticalLengths := []int{250, 300, 350, 400, 450}
	for _, length := range criticalLengths {
		truncated := content
		if len(content) > length {
			truncated = content[:length]
		}
		
		hasCommand := strings.Contains(truncated, "/pm:prd-parse installation-outcome-idea-b")
		t.Logf("Length %d: Command present = %v", length, hasCommand)
	}
}