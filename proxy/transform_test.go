package proxy

import (
	"claude-proxy/config"
	"claude-proxy/types"
	"context"
	"testing"
)

// TestTransformOpenAIToAnthropic_HarmonyProcessing tests the Harmony format processing
// in the TransformOpenAIToAnthropic function
func TestTransformOpenAIToAnthropic_HarmonyProcessing(t *testing.T) {
	// Setup test configuration with Harmony parsing enabled
	cfg := &config.Config{
		HarmonyParsingEnabled: true,
		HarmonyDebug:          true,
	}

	tests := []struct {
		name           string
		openAIResp     *types.OpenAIResponse
		expectedText   string
		expectHarmony  bool
		expectChannels int
	}{
		{
			name: "Issue #8 - Partial Harmony sequence",
			openAIResp: &types.OpenAIResponse{
				ID: "test-response-1",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Content: `<|channel|>analysis<|message|>The conversation: user asks "interesting!" (possibly a comment). We need to respond concisely, following guidelines: less than 4 lines, minimal. Likely respond with a short statement. Might ask if they have any specific request? The instruction says to be concise, no preamble. Possibly respond "What would you like to work on?" Keep under 4 lines. Use minimal text.

<|end|>What would you like to work on today?`,
						},
						FinishReason: func() *string { s := "stop"; return &s }(),
					},
				},
			},
			expectedText:   "What would you like to work on today?",
			expectHarmony:  true,
			expectChannels: 1,
		},
		{
			name: "Complete Harmony sequence",
			openAIResp: &types.OpenAIResponse{
				ID: "test-response-2",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Content: `<|start|>assistant<|channel|>analysis<|message|>User is asking for help with code. I should provide assistance.<|end|><|start|>assistant<|channel|>final<|message|>I'd be happy to help you with your code. What specific issue are you working on?<|end|>`,
						},
						FinishReason: func() *string { s := "stop"; return &s }(),
					},
				},
			},
			expectedText:   "I'd be happy to help you with your code. What specific issue are you working on?",
			expectHarmony:  true,
			expectChannels: 2,
		},
		{
			name: "Non-Harmony content",
			openAIResp: &types.OpenAIResponse{
				ID: "test-response-3",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Content: "This is just regular text without any Harmony tokens.",
						},
						FinishReason: func() *string { s := "stop"; return &s }(),
					},
				},
			},
			expectedText:   "This is just regular text without any Harmony tokens.",
			expectHarmony:  false,
			expectChannels: 0,
		},
		{
			name: "Harmony tokens detected but no valid channels",
			openAIResp: &types.OpenAIResponse{
				ID: "test-response-4",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Content: `<|start|>assistant but malformed content without proper structure`,
						},
						FinishReason: func() *string { s := "stop"; return &s }(),
					},
				},
			},
			expectedText:   `<|start|>assistant but malformed content without proper structure`,
			expectHarmony:  false, // Should fallback to non-Harmony
			expectChannels: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create context
			ctx := context.Background()

			// Transform response
			result, err := TransformOpenAIToAnthropic(ctx, tt.openAIResp, "test-model", cfg)

			// Check for errors
			if err != nil {
				t.Fatalf("TransformOpenAIToAnthropic() returned error: %v", err)
			}

			// Check result is not nil
			if result == nil {
				t.Fatal("TransformOpenAIToAnthropic() returned nil result")
			}

			// Check content
			if len(result.Content) == 0 {
				t.Fatal("TransformOpenAIToAnthropic() returned no content")
			}

			// Check text content
			if result.Content[0].Type != "text" {
				t.Errorf("Expected content type 'text', got %s", result.Content[0].Type)
			}

			if result.Content[0].Text != tt.expectedText {
				t.Errorf("Expected text %q, got %q", tt.expectedText, result.Content[0].Text)
			}

			// Additional checks for basic response structure
			if result.ID != tt.openAIResp.ID {
				t.Errorf("Expected ID %s, got %s", tt.openAIResp.ID, result.ID)
			}

			if result.Type != "message" {
				t.Errorf("Expected type 'message', got %s", result.Type)
			}

			if result.Role != "assistant" {
				t.Errorf("Expected role 'assistant', got %s", result.Role)
			}

			if result.StopReason != "end_turn" {
				t.Errorf("Expected stop reason 'end_turn', got %s", result.StopReason)
			}
		})
	}
}

// TestTransformOpenAIToAnthropic_HarmonyDisabled tests that Harmony processing
// is skipped when disabled in configuration
func TestTransformOpenAIToAnthropic_HarmonyDisabled(t *testing.T) {
	// Setup test configuration with Harmony parsing DISABLED
	cfg := &config.Config{
		HarmonyParsingEnabled: false,
	}

	harmonyContent := `<|channel|>analysis<|message|>This should not be processed as Harmony<|end|>Final response`

	openAIResp := &types.OpenAIResponse{
		ID: "test-response-disabled",
		Choices: []types.OpenAIChoice{
			{
				Message: types.OpenAIMessage{
					Content: harmonyContent,
				},
				FinishReason: func() *string { s := "stop"; return &s }(),
			},
		},
	}

	// Create context
	ctx := context.Background()

	// Transform response
	result, err := TransformOpenAIToAnthropic(ctx, openAIResp, "test-model", cfg)

	// Check for errors
	if err != nil {
		t.Fatalf("TransformOpenAIToAnthropic() returned error: %v", err)
	}

	// Check that the content is unchanged (not processed as Harmony)
	if result.Content[0].Text != harmonyContent {
		t.Errorf("Expected unchanged content when Harmony disabled, got processed content")
	}
}
