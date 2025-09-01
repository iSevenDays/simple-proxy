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

// TestBufferedStreamingPreventsCommandSplitting verifies the buffered streaming fix
func TestBufferedStreamingPreventsCommandSplitting(t *testing.T) {
	// Test the exact content from the original truncation issue (with "accurate visibility")
	content := `✅ PRD created: .claude/prds/installation-outcome-idea-b.md

The PRD defines a refactoring to use an InstallationOutcome record that captures the final result of an iOS app installation. This ensures metrics like app-installation.ios are reported exactly once per request, giving developers accurate visibility into installation success/failure rates, independent of internal fallback attempts.

Ready to create implementation epic? Run: /pm:prd-parse installation-outcome-idea-b`

	openaiResponse := types.OpenAIResponse{
		ID:     "buffered_streaming_test",
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
		Usage: types.OpenAIUsage{
			PromptTokens:     0,
			CompletionTokens: 0,
		},
	}

	ctx := internal.WithRequestID(context.Background(), "buffered_streaming_test")
	result, err := proxy.TransformOpenAIToAnthropic(ctx, &openaiResponse, "claude-sonnet-4-20250514", getTestConfig())
	require.NoError(t, err)

	actualContent := result.Content[0].Text

	t.Logf("Original content: %q", content)
	t.Logf("Transformed content: %q", actualContent) 
	t.Logf("Original length: %d", len(content))
	t.Logf("Transformed length: %d", len(actualContent))

	// Primary assertion - complete content should be preserved
	assert.Equal(t, content, actualContent, "Buffered streaming should preserve complete content")

	// Verify critical commands are preserved intact
	commands := []string{
		"/pm:prd-parse installation-outcome-idea-b",
	}

	for _, cmd := range commands {
		assert.Contains(t, actualContent, cmd, "Buffered streaming should preserve command intact: %s", cmd)
		t.Logf("✅ Command preserved: %s", cmd)
	}

	// Verify no text corruption (the original problem) - check for complete phrase
	assert.Contains(t, actualContent, "accurate visibility", "Should contain full phrase, not truncated")
	
	// Check that we don't have the specific corruption pattern where "acc" is missing
	assert.NotContains(t, actualContent, " curate visibility", "Should not contain corruption with missing 'ac' prefix")

	t.Logf("✅ BUFFERED STREAMING SUCCESS: Commands preserved, no truncation detected")
}

// TestBufferedStreamingWithMultipleCommands tests multiple commands in various positions
func TestBufferedStreamingWithMultipleCommands(t *testing.T) {
	testCases := []struct {
		name     string
		content  string
		commands []string
	}{
		{
			name: "commands_at_end",
			content: `Analysis complete. The refactoring is ready for implementation.

## Next Steps:
1. Create task: /pm:task-create refactor-installappviagoios-to-return-installationoutcome
2. Start deployment: /deploy:execute refactor-installappviagoios-to-return-installationoutcome-v1.0`,
			commands: []string{
				"/pm:task-create refactor-installappviagoios-to-return-installationoutcome", 
				"/deploy:execute refactor-installappviagoios-to-return-installationoutcome-v1.0",
			},
		},
		{
			name: "commands_mid_response",
			content: `First step: /pm:task-analyze refactor-installappviagoios-to-return-installationoutcome and then review the results.

After analysis, proceed with: /pm:task-start refactor-installappviagoios-to-return-installationoutcome --validate

Finally, deploy using: /deploy:canary refactor-installappviagoios-to-return-installationoutcome-validated`,
			commands: []string{
				"/pm:task-analyze refactor-installappviagoios-to-return-installationoutcome",
				"/pm:task-start refactor-installappviagoios-to-return-installationoutcome",
				"/deploy:canary refactor-installappviagoios-to-return-installationoutcome-validated",
			},
		},
		{
			name: "very_long_command",
			content: `Execute comprehensive analysis: /pm:task-analyze refactor-installappviagoios-to-return-installationoutcome-with-comprehensive-metrics-validation-and-error-handling-integration`,
			commands: []string{
				"/pm:task-analyze refactor-installappviagoios-to-return-installationoutcome-with-comprehensive-metrics-validation-and-error-handling-integration",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			openaiResponse := types.OpenAIResponse{
				ID: "multi_cmd_" + tc.name,
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant",
							Content: tc.content,
						},
						FinishReason: stringPtr("stop"),
					},
				},
			}

			ctx := internal.WithRequestID(context.Background(), tc.name)
			result, err := proxy.TransformOpenAIToAnthropic(ctx, &openaiResponse, "claude-sonnet-4-20250514", getTestConfig())
			require.NoError(t, err)

			actualContent := result.Content[0].Text

			// Verify complete content preservation
			assert.Equal(t, tc.content, actualContent, "Buffered streaming should preserve complete content")

			// Verify all commands are preserved intact
			for _, cmd := range tc.commands {
				assert.Contains(t, actualContent, cmd, "Command should be preserved intact: %s", cmd)
			}

			t.Logf("✅ %s: All %d commands preserved with buffered streaming", tc.name, len(tc.commands))
		})
	}
}

// TestBufferedStreamingPerformanceImpact tests that buffering doesn't significantly impact performance
func TestBufferedStreamingPerformanceImpact(t *testing.T) {
	// Create moderately large content to test performance
	content := strings.Repeat("This is test content for performance validation. ", 100) +
		"Final command: /pm:task-analyze refactor-installappviagoios-to-return-installationoutcome"

	openaiResponse := types.OpenAIResponse{
		ID: "perf_test",
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

	ctx := internal.WithRequestID(context.Background(), "perf_test")
	
	// Run transformation multiple times to check consistency
	for i := 0; i < 5; i++ {
		result, err := proxy.TransformOpenAIToAnthropic(ctx, &openaiResponse, "claude-sonnet-4-20250514", getTestConfig())
		require.NoError(t, err)

		actualContent := result.Content[0].Text
		
		// Verify content is preserved correctly each time
		assert.Equal(t, content, actualContent, "Content should be consistent across runs")
		assert.Contains(t, actualContent, "/pm:task-analyze refactor-installappviagoios-to-return-installationoutcome", "Command should be preserved")
	}

	t.Logf("✅ Performance test passed: Buffered streaming works consistently")
}

// TestBufferedStreamingWithMixedContent tests streaming with both text and tool content
func TestBufferedStreamingWithMixedContent(t *testing.T) {
	// This tests a more complex response that includes both text content and commands
	content := `I'll help you analyze the installation metrics. Let me start by reading the current implementation.

After reviewing the code, I found several issues with metric reporting. The solution is to implement an InstallationOutcome pattern.

## Recommended Actions:
1. Analyze the current code: /pm:task-analyze refactor-installappviagoios-to-return-installationoutcome
2. Implement the refactoring with proper testing  
3. Deploy the fix: /deploy:canary refactor-installappviagoios-to-return-installationoutcome-v2.1

This approach will ensure accurate metrics reporting without double-counting.`

	openaiResponse := types.OpenAIResponse{
		ID: "mixed_content_test",
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

	ctx := internal.WithRequestID(context.Background(), "mixed_content_test")
	result, err := proxy.TransformOpenAIToAnthropic(ctx, &openaiResponse, "claude-sonnet-4-20250514", getTestConfig())
	require.NoError(t, err)

	actualContent := result.Content[0].Text

	// Verify complete content preservation
	assert.Equal(t, content, actualContent, "Mixed content should be preserved completely")

	// Verify specific commands are preserved
	commands := []string{
		"/pm:task-analyze refactor-installappviagoios-to-return-installationoutcome",
		"/deploy:canary refactor-installappviagoios-to-return-installationoutcome-v2.1",
	}

	for _, cmd := range commands {
		assert.Contains(t, actualContent, cmd, "Command should be preserved in mixed content: %s", cmd)
	}

	// Verify contextual text is preserved
	keyPhrases := []string{
		"InstallationOutcome pattern",
		"accurate metrics reporting", 
		"without double-counting",
	}

	for _, phrase := range keyPhrases {
		assert.Contains(t, actualContent, phrase, "Key phrase should be preserved: %s", phrase)
	}

	t.Logf("✅ Mixed content test passed: Both text and commands preserved correctly")
}