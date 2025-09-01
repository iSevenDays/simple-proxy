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

// TestLongCommandSplitting tests the specific issue where long commands get split
func TestLongCommandSplitting(t *testing.T) {
	// This reproduces the exact issue from Claude Code UI
	content := `⏺ ❌ No analysis found for task #refactor-installappviagoios-to-return-installationoutcome.Run: /pm:task-analyze refactor-installappviagoios-to-return-installationoutcome firstOr: /pm:task-start refactor-installappviagoios-to-return-installationoutcome --analyze to do both.`

	openaiResponse := types.OpenAIResponse{
		ID:     "long_command_test",
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

	ctx := internal.WithRequestID(context.Background(), "long_command_splitting")
	result, err := proxy.TransformOpenAIToAnthropic(ctx, &openaiResponse, "claude-sonnet-4-20250514", getTestConfig())
	require.NoError(t, err)

	actualContent := result.Content[0].Text

	t.Logf("Original content: %q", content)
	t.Logf("Transformed content: %q", actualContent)
	t.Logf("Original length: %d", len(content))
	t.Logf("Transformed length: %d", len(actualContent))

	// Primary assertion - content should be preserved exactly
	assert.Equal(t, content, actualContent, "Long commands should not be truncated or split")

	// Verify specific long commands are preserved intact
	commands := []string{
		"/pm:task-analyze refactor-installappviagoios-to-return-installationoutcome",
		"/pm:task-start refactor-installappviagoios-to-return-installationoutcome",
	}

	for _, cmd := range commands {
		assert.Contains(t, actualContent, cmd, "Long command should be preserved intact: %s", cmd)
		
		// Make sure command is not split (doesn't appear as fragments)
		if strings.Contains(actualContent, cmd) {
			t.Logf("✅ Command preserved intact: %s", cmd)
		} else {
			// Look for evidence of splitting
			cmdParts := []string{
				strings.Split(cmd, "-")[0], // e.g., "/pm:task-analyze refactor"
				"installationoutcome",      // common suffix that might get separated
			}
			
			splitFound := false
			for _, part := range cmdParts {
				if strings.Contains(actualContent, part) && !strings.Contains(actualContent, cmd) {
					t.Logf("❌ Command appears to be split: found '%s' but not complete '%s'", part, cmd)
					splitFound = true
				}
			}
			if !splitFound {
				t.Logf("❓ Command missing entirely: %s", cmd)
			}
		}
	}
}

// TestCommandBoundaryDetection tests various command patterns
func TestCommandBoundaryDetection(t *testing.T) {
	testCases := []struct {
		name    string
		content string
		command string
		desc    string
	}{
		{
			name: "very_long_command_no_spaces",
			content: strings.Repeat("a", 140) + " /pm:task-analyze refactor-installappviagoios-to-return-installationoutcome-with-metrics",
			command: "/pm:task-analyze refactor-installappviagoios-to-return-installationoutcome-with-metrics",
			desc: "Very long command without spaces should not be split",
		},
		{
			name: "multiple_long_commands", 
			content: "Analysis complete. Use: /pm:task-analyze refactor-installappviagoios-to-return-installationoutcome or /deploy:canary refactor-installappviagoios-to-return-installationoutcome-v2.1 depending on requirements.",
			command: "/pm:task-analyze refactor-installappviagoios-to-return-installationoutcome",
			desc: "Multiple long commands should be preserved",
		},
		{
			name: "command_at_chunk_boundary",
			content: strings.Repeat("b", 149) + "/pm:task-analyze refactor-installappviagoios-to-return-installationoutcome",
			command: "/pm:task-analyze refactor-installappviagoios-to-return-installationoutcome",
			desc: "Command exactly at chunk boundary should not split",
		},
		{
			name: "command_crossing_boundary", 
			content: strings.Repeat("c", 100) + " The solution is ready. Execute: /pm:task-analyze refactor-installappviagoios-to-return-installationoutcome-with-validation-metrics",
			command: "/pm:task-analyze refactor-installappviagoios-to-return-installationoutcome-with-validation-metrics",
			desc: "Command crossing chunk boundary should remain intact",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			openaiResponse := types.OpenAIResponse{
				ID: "cmd_boundary_" + tc.name,
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
			assert.Equal(t, tc.content, actualContent, tc.desc)

			// Verify specific command is preserved
			assert.Contains(t, actualContent, tc.command, "Command should be preserved intact")

			t.Logf("✅ %s: Command preserved (%d chars)", tc.name, len(actualContent))
		})
	}
}

// TestChunkingWithCommandPatterns tests chunking behavior with slash commands
func TestChunkingWithCommandPatterns(t *testing.T) {
	// Test content that will trigger the chunking issue
	content := "Here is the analysis result. " + strings.Repeat("Details ", 20) + "Execute: /pm:task-analyze refactor-installappviagoios-to-return-installationoutcome-with-comprehensive-validation-and-metrics-tracking"
	
	t.Logf("Test content length: %d", len(content))
	
	openaiResponse := types.OpenAIResponse{
		ID: "chunking_patterns",
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

	ctx := internal.WithRequestID(context.Background(), "chunking_patterns")
	result, err := proxy.TransformOpenAIToAnthropic(ctx, &openaiResponse, "claude-sonnet-4-20250514", getTestConfig())
	require.NoError(t, err)

	actualContent := result.Content[0].Text

	// The key test - command should not be split
	longCommand := "/pm:task-analyze refactor-installappviagoios-to-return-installationoutcome-with-comprehensive-validation-and-metrics-tracking"
	
	assert.Equal(t, content, actualContent, "Content with long command should be preserved exactly")
	assert.Contains(t, actualContent, longCommand, "Long command should appear intact")
	
	// Additional debugging
	if !strings.Contains(actualContent, longCommand) {
		t.Logf("❌ Long command not found intact")
		t.Logf("Looking for fragments...")
		
		fragments := []string{
			"/pm:task-analyze",
			"refactor-installappviagoios",
			"installationoutcome",
			"comprehensive-validation",
		}
		
		for _, fragment := range fragments {
			if strings.Contains(actualContent, fragment) {
				t.Logf("Found fragment: %s", fragment)
			}
		}
	} else {
		t.Logf("✅ Long command found intact")
	}
}