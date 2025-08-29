package test

import (
	"claude-proxy/config"
	"claude-proxy/internal"
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

