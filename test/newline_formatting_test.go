package test

import (
	"claude-proxy/config"
	"claude-proxy/internal"
	"claude-proxy/parser"
	"claude-proxy/proxy"
	"claude-proxy/types"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewlineFormattingPreservation tests that markdown content with newlines is preserved
// This reproduces the issue observed in Claude Code UI where formatted text appears as one continuous line
func TestNewlineFormattingPreservation(t *testing.T) {
	tests := []struct {
		name            string
		openaiResponse  types.OpenAIResponse
		expectedContent string
		description     string
	}{
		{
			name: "markdown_with_newlines_preserved",
			openaiResponse: types.OpenAIResponse{
				ID:     "resp_newline_test",
				Object: "chat.completion",
				Model:  "test-model",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role: "assistant",
							Content: "**Solution A – Central‑only‑call helper**\n\nAdd a private method `recordMetrics(String method, boolean success, Duration duration, long fileSize)` that invokes `reportInstallationSpeed` and `reportInstallation`. Remove all direct calls to those two methods from the low‑level install implementations (`installAppViaGoIos`, `installAppViaInstallProxy`, `installAppViaZipConduit`). Each low‑level method keeps its current signature but returns a plain `boolean success` (or throws). The outermost install flow (`install` / `installAppViaGoIos`) determines which method succeeded, measures the overall duration, and calls `recordMetrics` once.\n\n*Pros*\n- Minimal API changes – only internal calls are adjusted.\n- Quick to implement; existing tests stay valid.\n- Keeps metric logic in one place, making future tweaks easy.\n\n*Cons*\n- Each low‑level method must still manage its own `success` flag and duration, so the code still carries duplicate bookkeeping.\n- New install paths added later could forget to use the helper, re‑introducing double reporting.\n\n---\n\n**Solution B – Return an `InstallationOutcome` record**\n\nCreate `record InstallationOutcome(String method, boolean success, Duration duration, long fileSize)`. Change every low‑level install method (`installAppViaGoIos`, `installAppViaInstallProxy`, `installAppViaZipConduit`) to return an `InstallationOutcome` instead of calling metric helpers. The top‑level flow (`install` / `installAppViaGoIos`) invokes the next method(s) and, on fallback, simply returns the first successful `InstallationOutcome`. Finally, a single call `recordMetrics(outcome)` (which internally calls the two metric methods) records metrics exactly once.\n\n*Pros*\n- All data needed for metrics is packaged together, eliminating scattered flags and duration handling.\n- Fallback logic becomes clearer—just propagate the outcome of the successful step.\n- Adding new install methods automatically fits the pattern; metrics stay centralized.\n\n*Cons*\n- Requires changing method signatures and updating all callers (including tests) across the class.\n- Slightly larger refactor, increasing risk of compile errors if any call sites are missed.",
						},
						FinishReason: stringPtr("stop"),
					},
				},
				Usage: types.OpenAIUsage{
					PromptTokens:     50,
					CompletionTokens: 300,
				},
			},
			expectedContent: "**Solution A – Central‑only‑call helper**\n\nAdd a private method `recordMetrics(String method, boolean success, Duration duration, long fileSize)` that invokes `reportInstallationSpeed` and `reportInstallation`. Remove all direct calls to those two methods from the low‑level install implementations (`installAppViaGoIos`, `installAppViaInstallProxy`, `installAppViaZipConduit`). Each low‑level method keeps its current signature but returns a plain `boolean success` (or throws). The outermost install flow (`install` / `installAppViaGoIos`) determines which method succeeded, measures the overall duration, and calls `recordMetrics` once.\n\n*Pros*\n- Minimal API changes – only internal calls are adjusted.\n- Quick to implement; existing tests stay valid.\n- Keeps metric logic in one place, making future tweaks easy.\n\n*Cons*\n- Each low‑level method must still manage its own `success` flag and duration, so the code still carries duplicate bookkeeping.\n- New install paths added later could forget to use the helper, re‑introducing double reporting.\n\n---\n\n**Solution B – Return an `InstallationOutcome` record**\n\nCreate `record InstallationOutcome(String method, boolean success, Duration duration, long fileSize)`. Change every low‑level install method (`installAppViaGoIos`, `installAppViaInstallProxy`, `installAppViaZipConduit`) to return an `InstallationOutcome` instead of calling metric helpers. The top‑level flow (`install` / `installAppViaGoIos`) invokes the next method(s) and, on fallback, simply returns the first successful `InstallationOutcome`. Finally, a single call `recordMetrics(outcome)` (which internally calls the two metric methods) records metrics exactly once.\n\n*Pros*\n- All data needed for metrics is packaged together, eliminating scattered flags and duration handling.\n- Fallback logic becomes clearer—just propagate the outcome of the successful step.\n- Adding new install methods automatically fits the pattern; metrics stay centralized.\n\n*Cons*\n- Requires changing method signatures and updating all callers (including tests) across the class.\n- Slightly larger refactor, increasing risk of compile errors if any call sites are missed.",
			description: "Should preserve newlines and markdown formatting in response content",
		},
		{
			name: "simple_text_with_newlines",
			openaiResponse: types.OpenAIResponse{
				ID: "resp_simple_newlines",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant",
							Content: "Line 1\nLine 2\n\nLine 3 after blank line",
						},
						FinishReason: stringPtr("stop"),
					},
				},
			},
			expectedContent: "Line 1\nLine 2\n\nLine 3 after blank line",
			description:     "Should preserve simple newlines in text content",
		},
		{
			name: "bullet_points_with_newlines",
			openaiResponse: types.OpenAIResponse{
				ID: "resp_bullets",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant",
							Content: "Here are the options:\n\n- Option A: First choice\n- Option B: Second choice\n- Option C: Third choice\n\nPlease select one.",
						},
						FinishReason: stringPtr("stop"),
					},
				},
			},
			expectedContent: "Here are the options:\n\n- Option A: First choice\n- Option B: Second choice\n- Option C: Third choice\n\nPlease select one.",
			description:     "Should preserve bullet point formatting with newlines",
		},
		{
			name: "code_blocks_with_newlines",
			openaiResponse: types.OpenAIResponse{
				ID: "resp_code",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant",
							Content: "Here's the code:\n\n```go\nfunc main() {\n    fmt.Println(\"Hello\")\n    fmt.Println(\"World\")\n}\n```\n\nThis should work correctly.",
						},
						FinishReason: stringPtr("stop"),
					},
				},
			},
			expectedContent: "Here's the code:\n\n```go\nfunc main() {\n    fmt.Println(\"Hello\")\n    fmt.Println(\"World\")\n}\n```\n\nThis should work correctly.",
			description:     "Should preserve code block formatting with newlines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := internal.WithRequestID(context.Background(), "newline_format_test")
			
			// Transform OpenAI response to Anthropic format
			result, err := proxy.TransformOpenAIToAnthropic(ctx, &tt.openaiResponse, "test-model", getTestConfig())
			require.NoError(t, err, "Transform should not fail for: %s", tt.description)

			// Verify that we have content
			require.NotEmpty(t, result.Content, "Response should have content")
			require.Equal(t, "text", result.Content[0].Type, "First content item should be text")

			// This is the key assertion - newlines should be preserved exactly
			actualContent := result.Content[0].Text
			assert.Equal(t, tt.expectedContent, actualContent, "Newlines and formatting should be preserved exactly: %s", tt.description)

			// Additional checks to ensure we didn't lose any important structure
			expectedNewlineCount := countNewlines(tt.expectedContent)
			actualNewlineCount := countNewlines(actualContent)
			assert.Equal(t, expectedNewlineCount, actualNewlineCount, "Number of newlines should match")

			// Log for debugging
			if actualContent != tt.expectedContent {
				t.Logf("Expected content:\n%q", tt.expectedContent)
				t.Logf("Actual content:\n%q", actualContent)
				t.Logf("Expected newlines: %d, Actual newlines: %d", expectedNewlineCount, actualNewlineCount)
			}
		})
	}
}

// TestNewlineFormattingInHarmonyContent tests newline preservation in proper Harmony-formatted content
// This reproduces the exact issue from Claude Code UI logs where formatted content appears as one line
func TestNewlineFormattingInHarmonyContent(t *testing.T) {
	tests := []struct {
		name           string
		openaiResponse types.OpenAIResponse
		harmonyEnabled bool
		expectedText   string
		description    string
	}{
		{
			name: "proper_harmony_format_with_analysis_and_final_channels",
			openaiResponse: types.OpenAIResponse{
				ID: "resp_harmony_proper",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role: "assistant",
							Content: `<|start|>assistant<|channel|>analysis<|message|>The user is asking for two best ideas to solve duplication issue with pros/cons. I need to provide structured solutions.<|end|>
<|start|>assistant<|channel|>final<|message|>**Solution A – Central‑only‑call helper**

Add a private method recordMetrics(String method, boolean success, Duration duration, long fileSize) that invokes reportInstallationSpeed and reportInstallation. Remove all direct calls to those two methods from the low‑level install implementations (installAppViaGoIos, installAppViaInstallProxy, installAppViaZipConduit). Each low‑level method keeps its current signature but returns a plain boolean success (or throws). The outermost install flow (install / installAppViaGoIos) determines which method succeeded, measures the overall duration, and calls recordMetrics once.

*Pros*
- Minimal API changes – only internal calls are adjusted.
- Quick to implement; existing tests stay valid.
- Keeps metric logic in one place, making future tweaks easy.

*Cons*
- Each low‑level method must still manage its own success flag and duration, so the code still carries duplicate bookkeeping.
- New install paths added later could forget to use the helper, re‑introducing double reporting.

---

**Solution B – Return an InstallationOutcome record**

Create record InstallationOutcome(String method, boolean success, Duration duration, long fileSize). Change every low‑level install method (installAppViaGoIos, installAppViaInstallProxy, installAppViaZipConduit) to return an InstallationOutcome instead of calling metric helpers. The top‑level flow (install / installAppViaGoIos) invokes the next method(s) and, on fallback, simply returns the first successful InstallationOutcome. Finally, a single call recordMetrics(outcome) (which internally calls the two metric methods) records metrics exactly once.

*Pros*
- All data needed for metrics is packaged together, eliminating scattered flags and duration handling.
- Fallback logic becomes clearer—just propagate the outcome of the successful step.
- Adding new install methods automatically fits the pattern; metrics stay centralized.

*Cons*
- Requires changing method signatures and updating all callers (including tests) across the class.
- Slightly larger refactor, increasing risk of compile errors if any call sites are missed.<|return|>`,
						},
						FinishReason: stringPtr("stop"),
					},
				},
			},
			harmonyEnabled: true,
			expectedText: `**Solution A – Central‑only‑call helper**

Add a private method recordMetrics(String method, boolean success, Duration duration, long fileSize) that invokes reportInstallationSpeed and reportInstallation. Remove all direct calls to those two methods from the low‑level install implementations (installAppViaGoIos, installAppViaInstallProxy, installAppViaZipConduit). Each low‑level method keeps its current signature but returns a plain boolean success (or throws). The outermost install flow (install / installAppViaGoIos) determines which method succeeded, measures the overall duration, and calls recordMetrics once.

*Pros*
- Minimal API changes – only internal calls are adjusted.
- Quick to implement; existing tests stay valid.
- Keeps metric logic in one place, making future tweaks easy.

*Cons*
- Each low‑level method must still manage its own success flag and duration, so the code still carries duplicate bookkeeping.
- New install paths added later could forget to use the helper, re‑introducing double reporting.

---

**Solution B – Return an InstallationOutcome record**

Create record InstallationOutcome(String method, boolean success, Duration duration, long fileSize). Change every low‑level install method (installAppViaGoIos, installAppViaInstallProxy, installAppViaZipConduit) to return an InstallationOutcome instead of calling metric helpers. The top‑level flow (install / installAppViaGoIos) invokes the next method(s) and, on fallback, simply returns the first successful InstallationOutcome. Finally, a single call recordMetrics(outcome) (which internally calls the two metric methods) records metrics exactly once.

*Pros*
- All data needed for metrics is packaged together, eliminating scattered flags and duration handling.
- Fallback logic becomes clearer—just propagate the outcome of the successful step.
- Adding new install methods automatically fits the pattern; metrics stay centralized.

*Cons*
- Requires changing method signatures and updating all callers (including tests) across the class.
- Slightly larger refactor, increasing risk of compile errors if any call sites are missed.`,
			description: "Should extract final channel content with preserved newlines from proper Harmony format",
		},
		{
			name: "malformed_harmony_fallback_preserves_newlines", 
			openaiResponse: types.OpenAIResponse{
				ID: "resp_malformed_harmony",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant",
							Content: "**Solution A – Central‑only‑call helper**\n\nThis is the first solution.\n\n**Solution B**\n\nThis is the second solution.",
						},
						FinishReason: stringPtr("stop"),
					},
				},
			},
			harmonyEnabled: true,
			expectedText:   "**Solution A – Central‑only‑call helper**\n\nThis is the first solution.\n\n**Solution B**\n\nThis is the second solution.",
			description:    "Non-Harmony content should fallback gracefully with preserved newlines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create config with Harmony parsing setting
			cfg := &config.Config{
				HarmonyParsingEnabled: tt.harmonyEnabled,
			}

			ctx := internal.WithRequestID(context.Background(), "harmony_newline_test")
			result, err := proxy.TransformOpenAIToAnthropic(ctx, &tt.openaiResponse, "test-model", cfg)
			require.NoError(t, err)

			// Find text content (might be after thinking content for Harmony)
			var actualText string
			for _, content := range result.Content {
				if content.Type == "text" {
					actualText = content.Text
					break
				}
			}

			assert.Equal(t, tt.expectedText, actualText, tt.description)

			// Log for debugging
			if actualText != tt.expectedText {
				t.Logf("Expected text:\n%q", tt.expectedText)
				t.Logf("Actual text:\n%q", actualText)
				t.Logf("Expected newlines: %d, Actual newlines: %d", countNewlines(tt.expectedText), countNewlines(actualText))
			}
		})
	}
}

// TestHarmonyMultiChannelParsing tests parsing of different Harmony channel types
func TestHarmonyMultiChannelParsing(t *testing.T) {
	tests := []struct {
		name           string
		openaiResponse types.OpenAIResponse
		expectedText   string
		expectedType   string
		description    string
	}{
		{
			name: "analysis_channel_parsing",
			openaiResponse: types.OpenAIResponse{
				ID: "resp_analysis",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant",
							Content: `<|start|>assistant<|channel|>analysis<|message|>This is internal analysis content that should be extracted properly.<|end|>`,
						},
						FinishReason: stringPtr("stop"),
					},
				},
			},
			expectedText: "This is internal analysis content that should be extracted properly.",
			expectedType: "thinking",
			description:  "Should extract analysis channel content as thinking content",
		},
		{
			name: "final_channel_parsing",
			openaiResponse: types.OpenAIResponse{
				ID: "resp_final",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant",
							Content: "<|start|>assistant<|channel|>final<|message|>This is user-facing final content.\n\nWith proper formatting.<|return|>",
						},
						FinishReason: stringPtr("stop"),
					},
				},
			},
			expectedText: "This is user-facing final content.\n\nWith proper formatting.",
			expectedType: "text", 
			description:  "Should extract final channel content with newlines preserved",
		},
		{
			name: "multi_channel_response",
			openaiResponse: types.OpenAIResponse{
				ID: "resp_multi",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role: "assistant",
							Content: `<|start|>assistant<|channel|>analysis<|message|>Internal thinking about the request.<|end|>
<|start|>assistant<|channel|>final<|message|>**Final Answer**

This is the user-facing response with:
- Multiple lines
- Proper formatting
- Preserved structure<|return|>`,
						},
						FinishReason: stringPtr("stop"),
					},
				},
			},
			expectedText: "**Final Answer**\n\nThis is the user-facing response with:\n- Multiple lines\n- Proper formatting\n- Preserved structure",
			expectedType: "text",
			description:  "Should extract final channel from multi-channel response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				HarmonyParsingEnabled: true,
			}

			ctx := internal.WithRequestID(context.Background(), "multi_channel_test")
			result, err := proxy.TransformOpenAIToAnthropic(ctx, &tt.openaiResponse, "test-model", cfg)
			require.NoError(t, err)
			require.NotEmpty(t, result.Content)

			// Find the text content (might be after thinking content)
			var actualText string
			for _, content := range result.Content {
				if content.Type == tt.expectedType {
					actualText = content.Text
					break
				}
			}

			assert.Equal(t, tt.expectedText, actualText, tt.description)
		})
	}
}

// TestHarmonyMalformedContentHandling tests graceful handling of malformed Harmony content
func TestHarmonyMalformedContentHandling(t *testing.T) {
	tests := []struct {
		name           string
		openaiResponse types.OpenAIResponse
		expectedText   string
		description    string
	}{
		{
			name: "missing_end_tag",
			openaiResponse: types.OpenAIResponse{
				ID: "resp_missing_end",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant", 
							Content: `<|start|>assistant<|channel|>final<|message|>Content without proper end tag`,
						},
						FinishReason: stringPtr("stop"),
					},
				},
			},
			expectedText: "Content without proper end tag",
			description:  "Should handle missing end tag gracefully",
		},
		{
			name: "malformed_channel_tag",
			openaiResponse: types.OpenAIResponse{
				ID: "resp_malformed",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant",
							Content: `<|start|>assistant<|channel|>invalid_channel<|message|>Content with invalid channel type<|end|>`,
						},
						FinishReason: stringPtr("stop"),
					},
				},
			},
			expectedText: "Content with invalid channel type",
			description:  "Should handle invalid channel types gracefully",
		},
		{
			name: "no_harmony_content",
			openaiResponse: types.OpenAIResponse{
				ID: "resp_no_harmony",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant",
							Content: "Regular content without Harmony formatting\n\nShould be preserved as-is",
						},
						FinishReason: stringPtr("stop"),
					},
				},
			},
			expectedText: "Regular content without Harmony formatting\n\nShould be preserved as-is",
			description:  "Should handle non-Harmony content without modification",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				HarmonyParsingEnabled: true,
			}

			ctx := internal.WithRequestID(context.Background(), "malformed_test")
			result, err := proxy.TransformOpenAIToAnthropic(ctx, &tt.openaiResponse, "test-model", cfg)
			require.NoError(t, err)
			require.NotEmpty(t, result.Content)

			actualText := result.Content[0].Text
			assert.Equal(t, tt.expectedText, actualText, tt.description)
		})
	}
}

// TestRobustHarmonyParsing provides comprehensive testing of robust
// Harmony parser functions with various malformed content scenarios.
//
// This test suite validates the multi-level graceful degradation implemented
// for Issue #22, ensuring that malformed Harmony content is handled robustly
// without causing crashes or data loss.
//
// Test coverage includes:
//   - Missing end tags
//   - Invalid channel identifiers  
//   - Incomplete token structures
//   - Mixed malformed content
//   - Performance with large responses
//   - Recovery strategies and fallback mechanisms
func TestRobustHarmonyParsing(t *testing.T) {
	t.Run("MissingEndTags", testMissingEndTags)
	t.Run("InvalidChannels", testInvalidChannels)
	t.Run("IncompleteStructures", testIncompleteStructures)
	t.Run("MixedContent", testMixedContent)
	t.Run("LargeResponsePerformance", testLargeResponsePerformance)
	t.Run("RobustTokenExtraction", testRobustTokenExtraction)
	t.Run("ContentCleaningStrategies", testContentCleaningStrategies)
	t.Run("ParseHarmonyMessageRobust", testParseHarmonyMessageRobust)
	t.Run("ExtractChannelsRobust", testExtractChannelsRobust)
	t.Run("GracefulDegradation", testGracefulDegradation)
}

// testMissingEndTags verifies handling of content with missing end tags
func testMissingEndTags(t *testing.T) {
	testCases := []struct {
		name        string
		content     string
		expectError bool
		expectValid bool
	}{
		{
			name:        "Missing end tag simple",
			content:     "<|start|>assistant<|channel|>final<|message|>Content without proper end tag",
			expectError: false,
			expectValid: true,
		},
		{
			name:        "Missing end tag with analysis channel",
			content:     "<|start|>assistant<|channel|>analysis<|message|>This is thinking content that never ends",
			expectError: false,
			expectValid: true,
		},
		{
			name:        "Multiple missing end tags",
			content:     "<|start|>assistant<|channel|>analysis<|message|>First content<|start|>assistant<|channel|>final<|message|>Second content",
			expectError: false,
			expectValid: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test robust token extraction
			tokens, err := parser.ExtractTokensRobust(tc.content)
			if tc.expectError && err == nil {
				t.Errorf("Expected error for content: %s", tc.content)
			}
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Verify tokens were extracted
			if len(tokens) == 0 && tc.expectValid {
				t.Errorf("Expected tokens to be extracted from: %s", tc.content)
			}

			// Test robust message parsing
			message, err := parser.ParseHarmonyMessageRobust(tc.content)
			if err != nil {
				t.Errorf("ParseHarmonyMessageRobust failed: %v", err)
			}
			if message == nil {
				t.Error("Expected non-nil message")
			}

			// Verify content is preserved (could be in ResponseText, ThinkingText, or channels)
			hasContent := message.ResponseText != "" || message.ThinkingText != "" || len(message.Channels) > 0
			if !hasContent && strings.TrimSpace(tc.content) != "" {
				t.Errorf("Content was lost during parsing. Expected some content to be preserved.")
			}
		})
	}
}

// testInvalidChannels verifies handling of invalid channel identifiers
func testInvalidChannels(t *testing.T) {
	testCases := []struct {
		name    string
		content string
	}{
		{
			name:    "Invalid channel with numbers",
			content: "<|start|>assistant<|channel|>123invalid<|message|>Content<|end|>",
		},
		{
			name:    "Invalid channel with special chars",
			content: "<|start|>assistant<|channel|>invalid@channel<|message|>Content<|end|>",
		},
		{
			name:    "Empty channel identifier",
			content: "<|start|>assistant<|channel|><|message|>Content<|end|>",
		},
		{
			name:    "Channel with spaces",
			content: "<|start|>assistant<|channel|>final analysis<|message|>Content<|end|>",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test robust parsing doesn't crash
			message, err := parser.ParseHarmonyMessageRobust(tc.content)
			if err != nil {
				t.Errorf("ParseHarmonyMessageRobust failed: %v", err)
			}
			if message == nil {
				t.Error("Expected non-nil message")
			}

			// Test robust channel extraction
			channels, err := parser.ExtractChannelsRobust(tc.content)
			if err != nil {
				t.Errorf("ExtractChannelsRobust failed: %v", err)
			}

			// Verify at least one channel was extracted (fallback if needed)
			if len(channels) == 0 {
				t.Errorf("Expected at least one channel from: %s", tc.content)
			}

			// Verify content is preserved
			foundContent := false
			for _, channel := range channels {
				if strings.Contains(channel.Content, "Content") {
					foundContent = true
					break
				}
			}
			if !foundContent && message.ResponseText == "" {
				t.Error("Content was lost during parsing")
			}
		})
	}
}

// testIncompleteStructures verifies handling of incomplete token structures
func testIncompleteStructures(t *testing.T) {
	testCases := []struct {
		name    string
		content string
	}{
		{
			name:    "Only start token",
			content: "<|start|>assistant",
		},
		{
			name:    "Start and channel only",
			content: "<|start|>assistant<|channel|>final",
		},
		{
			name:    "Missing start token",
			content: "<|channel|>final<|message|>Content<|end|>",
		},
		{
			name:    "Mixed partial tokens",
			content: "Some text <|channel|>analysis then <|message|> more content",
		},
		{
			name:    "Truncated at message",
			content: "<|start|>assistant<|channel|>final<|message|>Content that gets cut off mid-sen",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test that parsing doesn't crash
			message, err := parser.ParseHarmonyMessageRobust(tc.content)
			if err != nil {
				t.Errorf("ParseHarmonyMessageRobust failed: %v", err)
			}
			if message == nil {
				t.Error("Expected non-nil message")
			}

			// Verify content is preserved (either in channels or as fallback)
			if message.ResponseText == "" && len(message.Channels) == 0 && strings.TrimSpace(tc.content) != "" {
				t.Errorf("Content was completely lost from: %s", tc.content)
			}

			// Test token extraction
			tokens, err := parser.ExtractTokensRobust(tc.content)
			if err != nil {
				t.Errorf("ExtractTokensRobust failed: %v", err)
			}

			// Should find some tokens or malformed sequences
			if len(tokens) == 0 && strings.Contains(tc.content, "<|") {
				t.Errorf("Expected to find tokens in: %s", tc.content)
			}
		})
	}
}

// testMixedContent verifies handling of mixed valid and malformed content
func testMixedContent(t *testing.T) {
	mixedContent := `<|start|>assistant<|channel|>analysis<|message|>This is valid thinking content.<|end|>

Some regular text without tokens.

<|start|>assistant<|channel|>final<|message|>This is valid final content.<|end|>

<|start|>assistant<|channel|>invalid123<|message|>This has invalid channel

More regular text.

<|channel|>analysis<|message|>Missing start token content<|end|>`

	t.Run("Mixed valid and invalid content", func(t *testing.T) {
		// Test robust parsing
		message, err := parser.ParseHarmonyMessageRobust(mixedContent)
		if err != nil {
			t.Errorf("ParseHarmonyMessageRobust failed: %v", err)
		}
		if message == nil {
			t.Error("Expected non-nil message")
		}

		// Should have detected Harmony format
		if !message.HasHarmony {
			t.Error("Expected HasHarmony to be true")
		}

		// Should have extracted some channels
		if len(message.Channels) == 0 {
			t.Error("Expected to extract some channels")
		}

		// Should have consolidated content
		if message.ThinkingText == "" && message.ResponseText == "" {
			t.Error("Expected some consolidated content")
		}

		// Test channel extraction
		channels, err := parser.ExtractChannelsRobust(mixedContent)
		if err != nil {
			t.Errorf("ExtractChannelsRobust failed: %v", err)
		}

		if len(channels) == 0 {
			t.Error("Expected to extract channels from mixed content")
		}

		// Verify both valid and invalid channels are handled
		hasValidChannel := false
		for _, channel := range channels {
			if channel.Valid {
				hasValidChannel = true
				break
			}
		}
		if !hasValidChannel {
			t.Error("Expected at least one valid channel")
		}
	})
}

// testLargeResponsePerformance verifies performance with large malformed responses
func testLargeResponsePerformance(t *testing.T) {
	// Create a large response with mixed content
	var largeContent strings.Builder
	
	// Add valid content
	largeContent.WriteString("<|start|>assistant<|channel|>analysis<|message|>")
	for i := 0; i < 1000; i++ {
		largeContent.WriteString("This is repeated thinking content. ")
	}
	largeContent.WriteString("<|end|>")
	
	// Add malformed content
	for i := 0; i < 100; i++ {
		largeContent.WriteString("<|start|>assistant<|channel|>final<|message|>Incomplete content " + strings.Repeat("x", 50))
	}
	
	// Add more valid content
	largeContent.WriteString("<|start|>assistant<|channel|>final<|message|>")
	for i := 0; i < 500; i++ {
		largeContent.WriteString("Final response content. ")
	}
	largeContent.WriteString("<|end|>")

	content := largeContent.String()

	t.Run("Large response parsing", func(t *testing.T) {
		// Test that parsing completes in reasonable time
		message, err := parser.ParseHarmonyMessageRobust(content)
		if err != nil {
			t.Errorf("ParseHarmonyMessageRobust failed: %v", err)
		}
		if message == nil {
			t.Error("Expected non-nil message")
		}

		// Should handle the large content
		if message.ResponseText == "" && message.ThinkingText == "" {
			t.Error("Expected some content to be extracted")
		}

		// Test token extraction performance
		tokens, err := parser.ExtractTokensRobust(content)
		if err != nil {
			t.Errorf("ExtractTokensRobust failed: %v", err)
		}

		// Should find tokens despite the size
		if len(tokens) == 0 {
			t.Error("Expected to find tokens in large content")
		}
	})
}

// testRobustTokenExtraction tests the ExtractTokensRobust function specifically
func testRobustTokenExtraction(t *testing.T) {
	testCases := []struct {
		name           string
		content        string
		expectTokens   int
		expectMalformed bool
	}{
		{
			name:           "Well-formed content",
			content:        "<|start|>assistant<|channel|>final<|message|>Content<|end|>",
			expectTokens:   4, // start, channel, message, end
			expectMalformed: false,
		},
		{
			name:           "Malformed missing end",
			content:        "<|start|>assistant<|channel|>final<|message|>Content",
			expectTokens:   3, // start, channel, message (+ malformed sequences)
			expectMalformed: true,
		},
		{
			name:           "Empty content",
			content:        "",
			expectTokens:   0,
			expectMalformed: false,
		},
		{
			name:           "No Harmony tokens",
			content:        "Regular text without any special tokens",
			expectTokens:   0,
			expectMalformed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tokens, err := parser.ExtractTokensRobust(tc.content)
			if err != nil {
				t.Errorf("ExtractTokensRobust failed: %v", err)
			}

			if len(tokens) < tc.expectTokens {
				t.Errorf("Expected at least %d tokens, got %d", tc.expectTokens, len(tokens))
			}

			// Check for malformed tokens if expected
			hasMalformed := false
			for _, token := range tokens {
				if !token.Valid {
					hasMalformed = true
					break
				}
			}

			if tc.expectMalformed && !hasMalformed {
				t.Error("Expected malformed tokens but none found")
			}
			if !tc.expectMalformed && hasMalformed {
				t.Error("Found unexpected malformed tokens")
			}
		})
	}
}

// testContentCleaningStrategies tests the content cleaning functionality
func testContentCleaningStrategies(t *testing.T) {
	// Note: cleanMalformedContent is not exported, so we test it indirectly
	// through ParseHarmonyMessageRobust
	
	testCases := []struct {
		name    string
		content string
		expectImprovement bool
	}{
		{
			name:    "Missing end tag should be repaired",
			content: "<|start|>assistant<|channel|>final<|message|>Content without end",
			expectImprovement: true,
		},
		{
			name:    "Invalid channel should be normalized",
			content: "<|start|>assistant<|channel|>123invalid<|message|>Content<|end|>",
			expectImprovement: true,
		},
		{
			name:    "Well-formed content should not be changed",
			content: "<|start|>assistant<|channel|>final<|message|>Good content<|end|>",
			expectImprovement: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse with robust parser
			message, err := parser.ParseHarmonyMessageRobust(tc.content)
			if err != nil {
				t.Errorf("ParseHarmonyMessageRobust failed: %v", err)
			}

			// Check if parsing succeeded better than expected
			if tc.expectImprovement {
				// Should have extracted meaningful content
				if message.ResponseText == "" && len(message.Channels) == 0 {
					t.Error("Expected content cleaning to improve parsing results")
				}
			}

			// Content should never be completely lost
			if strings.TrimSpace(tc.content) != "" && message.ResponseText == "" && len(message.Channels) == 0 {
				t.Error("Content was completely lost despite cleaning attempts")
			}
		})
	}
}

// testParseHarmonyMessageRobust tests the robust message parsing function
func testParseHarmonyMessageRobust(t *testing.T) {
	testCases := []struct {
		name    string
		content string
	}{
		{
			name:    "Empty content",
			content: "",
		},
		{
			name:    "Regular text",
			content: "Just some regular text without any special formatting",
		},
		{
			name:    "Well-formed Harmony",
			content: "<|start|>assistant<|channel|>analysis<|message|>Thinking<|end|><|start|>assistant<|channel|>final<|message|>Response<|end|>",
		},
		{
			name:    "Malformed Harmony",
			content: "<|start|>assistant<|channel|>final<|message|>Incomplete",
		},
		{
			name:    "Mixed content",
			content: "Text before <|start|>assistant<|channel|>final<|message|>Good content<|end|> text after",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			message, err := parser.ParseHarmonyMessageRobust(tc.content)
			
			// Should never return an error (errors are collected in ParseErrors)
			if err != nil {
				t.Errorf("ParseHarmonyMessageRobust should not return errors, got: %v", err)
			}

			// Should always return a message
			if message == nil {
				t.Error("Expected non-nil message")
				return
			}

			// Should preserve raw content
			if message.RawContent != tc.content {
				t.Error("RawContent was not preserved")
			}

			// For non-empty content, should have some output
			if strings.TrimSpace(tc.content) != "" {
				if message.ResponseText == "" && len(message.Channels) == 0 {
					t.Error("Expected some parsed content for non-empty input")
				}
			}

			// For empty content, should have empty results
			if tc.content == "" {
				if message.ResponseText != "" || len(message.Channels) != 0 {
					t.Error("Expected empty results for empty content")
				}
			}
		})
	}
}

// testExtractChannelsRobust tests the robust channel extraction function
func testExtractChannelsRobust(t *testing.T) {
	testCases := []struct {
		name           string
		content        string
		expectChannels int
		expectFallback bool
	}{
		{
			name:           "Well-formed content",
			content:        "<|start|>assistant<|channel|>final<|message|>Content<|end|>",
			expectChannels: 1,
			expectFallback: false,
		},
		{
			name:           "Malformed content",
			content:        "<|start|>assistant<|channel|>final<|message|>Content",
			expectChannels: 1,
			expectFallback: true, // May use fallback strategies
		},
		{
			name:           "No tokens",
			content:        "Regular text",
			expectChannels: 1, // Should create fallback channel
			expectFallback: true,
		},
		{
			name:           "Empty content",
			content:        "",
			expectChannels: 0,
			expectFallback: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			channels, err := parser.ExtractChannelsRobust(tc.content)
			if err != nil {
				t.Errorf("ExtractChannelsRobust failed: %v", err)
			}

			if len(channels) != tc.expectChannels {
				t.Errorf("Expected %d channels, got %d", tc.expectChannels, len(channels))
			}

			// Check fallback usage
			if tc.expectFallback && len(channels) > 0 {
				foundFallback := false
				for _, channel := range channels {
					if !channel.Valid || channel.RawChannel == "fallback" {
						foundFallback = true
						break
					}
				}
				if !foundFallback && tc.content != "" {
					t.Error("Expected fallback channel for malformed content")
				}
			}

			// Verify content preservation
			if tc.content != "" && len(channels) > 0 {
				hasContent := false
				for _, channel := range channels {
					if strings.TrimSpace(channel.Content) != "" {
						hasContent = true
						break
					}
				}
				if !hasContent {
					t.Error("Expected channels to contain some content")
				}
			}
		})
	}
}

// testGracefulDegradation verifies the multi-level graceful degradation
func testGracefulDegradation(t *testing.T) {
	// Test severely malformed content that should trigger multiple fallback levels
	severelyMalformed := "<|start|>assistant<|channel|>123invalid<|message|>Content that never ends properly and has more <|random|> tokens scattered around"

	t.Run("Graceful degradation levels", func(t *testing.T) {
		// Test that parsing completes without crashing
		message, err := parser.ParseHarmonyMessageRobust(severelyMalformed)
		if err != nil {
			t.Errorf("ParseHarmonyMessageRobust failed: %v", err)
		}
		if message == nil {
			t.Error("Expected non-nil message")
			return
		}

		// Should have preserved content somehow
		if message.ResponseText == "" && len(message.Channels) == 0 {
			t.Error("Graceful degradation should preserve content")
		}

		// Should have error information
		if len(message.ParseErrors) == 0 {
			t.Error("Expected parse errors to be recorded")
		}

		// Test token extraction also handles it
		tokens, err := parser.ExtractTokensRobust(severelyMalformed)
		if err != nil {
			t.Errorf("ExtractTokensRobust failed: %v", err)
		}

		// Should find some tokens
		if len(tokens) == 0 {
			t.Error("Expected to find some tokens")
		}

		// Test channel extraction
		channels, err := parser.ExtractChannelsRobust(severelyMalformed)
		if err != nil {
			t.Errorf("ExtractChannelsRobust failed: %v", err)
		}

		// Should extract at least one channel (even if fallback)
		if len(channels) == 0 {
			t.Error("Expected at least one channel from graceful degradation")
		}
	})
}

// BenchmarkHarmonyParsing benchmarks the performance of Harmony content parsing
func BenchmarkHarmonyParsing(b *testing.B) {
	smallContent := `<|start|>assistant<|channel|>final<|message|>Small response<|return|>`
	
	mediumContent := `<|start|>assistant<|channel|>analysis<|message|>` + strings.Repeat("Analysis content. ", 50) + `<|end|>
<|start|>assistant<|channel|>final<|message|>` + strings.Repeat("Final response content. ", 100) + `<|return|>`

	largeContent := `<|start|>assistant<|channel|>analysis<|message|>` + strings.Repeat("Large analysis content. ", 500) + `<|end|>
<|start|>assistant<|channel|>final<|message|>` + strings.Repeat("Large final response content. ", 1000) + `<|return|>`

	cfg := &config.Config{
		HarmonyParsingEnabled: true,
	}

	benchmarks := []struct {
		name    string
		content string
	}{
		{"Small", smallContent},
		{"Medium", mediumContent}, 
		{"Large", largeContent},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			response := types.OpenAIResponse{
				ID: "bench_" + bm.name,
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant",
							Content: bm.content,
						},
						FinishReason: stringPtr("stop"),
					},
				},
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx := internal.WithRequestID(context.Background(), "benchmark_test")
				_, err := proxy.TransformOpenAIToAnthropic(ctx, &response, "test-model", cfg)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Benchmark tests for performance validation of robust parsing functions
func BenchmarkParseHarmonyMessageRobust(b *testing.B) {
	content := "<|start|>assistant<|channel|>analysis<|message|>Some thinking content<|end|><|start|>assistant<|channel|>final<|message|>Final response"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.ParseHarmonyMessageRobust(content)
	}
}

func BenchmarkExtractTokensRobust(b *testing.B) {
	content := "<|start|>assistant<|channel|>final<|message|>Content without end tag"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.ExtractTokensRobust(content)
	}
}

func BenchmarkExtractChannelsRobust(b *testing.B) {
	content := "Mixed content <|start|>assistant<|channel|>123invalid<|message|>Malformed content"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.ExtractChannelsRobust(content)
	}
}

// Helper function to count newlines in a string
func countNewlines(s string) int {
	count := 0
	for _, char := range s {
		if char == '\n' {
			count++
		}
	}
	return count
}

