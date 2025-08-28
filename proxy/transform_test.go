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

			// Find the text content block (may not be first due to thinking content)
			var textBlock *types.Content
			for i := range result.Content {
				if result.Content[i].Type == "text" {
					textBlock = &result.Content[i]
					break
				}
			}
			
			if textBlock == nil {
				t.Fatal("No text content block found in response")
			}

			if textBlock.Text != tt.expectedText {
				t.Errorf("Expected text %q, got %q", tt.expectedText, textBlock.Text)
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

// TestTransformOpenAIToAnthropic_HarmonyThinkingContent tests that thinking content
// is properly extracted and assigned to the ThinkingContent field for Claude Code UI
func TestTransformOpenAIToAnthropic_HarmonyThinkingContent(t *testing.T) {
	// Setup test configuration with Harmony parsing ENABLED
	cfg := &config.Config{
		HarmonyParsingEnabled: true,
	}

	// Test case with both thinking content (analysis channel) and response content (final channel)
	// Following official OpenAI Harmony format: https://openai.com/open-models/harmony/
	harmonyContent := `<|start|>assistant<|channel|>analysis<|message|>User asks a question. I need to think about this carefully. This is chain-of-thought reasoning that should be extracted as thinking content for Claude Code UI.<|end|><|start|>assistant<|channel|>final<|message|>This is the final response that should be displayed to the user.<|end|>`

	openAIResp := &types.OpenAIResponse{
		ID: "test-thinking-content",
		Choices: []types.OpenAIChoice{
			{
				Message: types.OpenAIMessage{
					Content: harmonyContent,
				},
				FinishReason: func() *string { s := "stop"; return &s }(),
			},
		},
		Usage: types.OpenAIUsage{
			PromptTokens:     10,
			CompletionTokens: 20,
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

	// Check that main response content is extracted from final channel (second content block)
	expectedResponse := "This is the final response that should be displayed to the user."


	// Check that thinking content is added as first content block with type "thinking"
	expectedThinking := "User asks a question. I need to think about this carefully. This is chain-of-thought reasoning that should be extracted as thinking content for Claude Code UI."
	if len(result.Content) < 2 {
		t.Fatalf("Expected at least 2 content blocks (thinking + response), got %d", len(result.Content))
	}
	
	// First content block should be thinking
	thinkingBlock := result.Content[0]
	if thinkingBlock.Type != "thinking" {
		t.Errorf("Expected first content block type to be 'thinking', got: %q", thinkingBlock.Type)
	}
	if thinkingBlock.Text != expectedThinking {
		t.Errorf("Expected thinking content: %q, got: %q", expectedThinking, thinkingBlock.Text)
	}
	
	// Second content block should be main response
	responseBlock := result.Content[1]
	if responseBlock.Type != "text" {
		t.Errorf("Expected second content block type to be 'text', got: %q", responseBlock.Type)
	}
	if responseBlock.Text != expectedResponse {
		t.Errorf("Expected main response: %q, got: %q", expectedResponse, responseBlock.Text)
	}
}
