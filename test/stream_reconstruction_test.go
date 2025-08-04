package test

import (
	"claude-proxy/internal"
	"claude-proxy/proxy"
	"claude-proxy/types"
	"context"
	"testing"
)


func TestStreamingToolCallReconstruction_PartialChunks(t *testing.T) {
	// Test the exact scenario from the logs:
	// - Multiple chunks with partial tool call data
	// - First chunk has ID and name, empty/malformed arguments
	// - Second chunk has arguments but no ID/name
	chunks := []types.OpenAIStreamChunk{
		{
			ID:      "chatcmpl-test",
			Object:  "chat.completion.chunk",
			Created: 1234567890,
			Model:   "test-model",
			Choices: []types.OpenAIStreamChoice{
				{
					Index: 0,
					Delta: types.OpenAIStreamDelta{
						ToolCalls: []types.OpenAIToolCall{
							{
								Index: 0,
								ID:    "BcRYh1bKJfWBKZwMVgwyi1oA7XJjMOMn",
								Type:  "function",
								Function: types.OpenAIToolCallFunction{
									Name:      "TodoWrite",
									Arguments: "", // Empty arguments in first chunk
								},
							},
						},
					},
				},
			},
		},
		{
			ID:      "chatcmpl-test",
			Object:  "chat.completion.chunk",
			Created: 1234567890,
			Model:   "test-model",
			Choices: []types.OpenAIStreamChoice{
				{
					Index: 0,
					Delta: types.OpenAIStreamDelta{
						ToolCalls: []types.OpenAIToolCall{
							{
								Index: 0,
								ID:    "", // Empty ID in second chunk
								Type:  "",
								Function: types.OpenAIToolCallFunction{
									Name:      "", // Empty name in second chunk
									Arguments: `{"todos":[{"content":"Test task","id":"test-1","priority":"medium","status":"pending"}]}`,
								},
							},
						},
					},
				},
			},
		},
	}

	finalChunk := &types.OpenAIStreamChunk{
		ID:      "chatcmpl-test",
		Object:  "chat.completion.chunk",
		Created: 1234567890,
		Model:   "test-model",
		Choices: []types.OpenAIStreamChoice{
			{
				Index:        0,
				FinishReason: stringPtr("tool_calls"),
			},
		},
	}

	ctx := internal.WithRequestID(context.Background(), "stream_test")
	response, err := proxy.ReconstructResponseFromChunks(ctx, chunks, finalChunk)

	if err != nil {
		t.Fatalf("Expected successful reconstruction, got error: %v", err)
	}

	// This test verifies the fix works correctly:
	// 1. Creates 1 tool call instead of 2
	// 2. Tool call has complete data from both chunks
	// 3. All fields are properly merged
	
	if len(response.Choices) == 0 {
		t.Fatal("Expected at least one choice in response")
	}

	toolCalls := response.Choices[0].Message.ToolCalls
	
	// Verify exactly 1 tool call is created from the partial chunks
	if len(toolCalls) != 1 {
		t.Errorf("Expected 1 tool call, got %d", len(toolCalls))
		for i, tc := range toolCalls {
			t.Logf("Tool call %d: ID=%s, Name=%s, Args=%s", i, tc.ID, tc.Function.Name, tc.Function.Arguments)
		}
		return
	}

	// Verify all fields are properly merged from both chunks
	toolCall := toolCalls[0]
	if toolCall.ID != "BcRYh1bKJfWBKZwMVgwyi1oA7XJjMOMn" {
		t.Errorf("Expected tool call ID 'BcRYh1bKJfWBKZwMVgwyi1oA7XJjMOMn', got '%s'", toolCall.ID)
	}
	
	if toolCall.Function.Name != "TodoWrite" {
		t.Errorf("Expected tool call name 'TodoWrite', got '%s'", toolCall.Function.Name)
	}
	
	expectedArgs := `{"todos":[{"content":"Test task","id":"test-1","priority":"medium","status":"pending"}]}`
	if toolCall.Function.Arguments != expectedArgs {
		t.Errorf("Expected arguments '%s', got '%s'", expectedArgs, toolCall.Function.Arguments)
	}
}