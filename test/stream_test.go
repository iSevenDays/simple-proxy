package test

import (
	"claude-proxy/internal"
	"claude-proxy/proxy"
	"claude-proxy/types"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)


// TestStreamingResponseReconstruction tests chunk-by-chunk processing
// Following SPARC: Focused test for the core streaming issue we solved
func TestStreamingResponseReconstruction(t *testing.T) {
	tests := []struct {
		name           string
		chunks         []types.OpenAIStreamChunk
		finalChunk     *types.OpenAIStreamChunk
		expectedMsg    string
		expectedTools  []types.OpenAIToolCall
		expectedFinish *string
	}{
		{
			name: "simple_text_streaming_reconstructs",
			chunks: []types.OpenAIStreamChunk{
				{
					ID:      "stream_123",
					Created: 1640995200,
					Model:   "kimi-k2",
					Choices: []types.OpenAIStreamChoice{
						{Delta: types.OpenAIStreamDelta{Content: "Hello "}},
					},
				},
				{
					ID:      "stream_123",
					Created: 1640995200,
					Model:   "kimi-k2",
					Choices: []types.OpenAIStreamChoice{
						{Delta: types.OpenAIStreamDelta{Content: "world!"}},
					},
				},
			},
			finalChunk: &types.OpenAIStreamChunk{
				ID:      "stream_123",
				Created: 1640995200,
				Model:   "kimi-k2",
				Choices: []types.OpenAIStreamChoice{
					{FinishReason: stringPtr("stop")},
				},
			},
			expectedMsg:    "Hello world!",
			expectedFinish: stringPtr("stop"),
		},
		{
			name: "tool_call_streaming_reconstructs_correctly",
			chunks: []types.OpenAIStreamChunk{
				{
					ID:      "stream_456",
					Created: 1640995200,
					Model:   "kimi-k2",
					Choices: []types.OpenAIStreamChoice{
						{
							Delta: types.OpenAIStreamDelta{
								ToolCalls: []types.OpenAIToolCall{
									{
										ID:   "call_789",
										Type: "function",
										Function: types.OpenAIToolCallFunction{
											Name:      "Write",
											Arguments: `{"file_path":`,
										},
									},
								},
							},
						},
					},
				},
				{
					ID:      "stream_456",
					Created: 1640995200,
					Model:   "kimi-k2",
					Choices: []types.OpenAIStreamChoice{
						{
							Delta: types.OpenAIStreamDelta{
								ToolCalls: []types.OpenAIToolCall{
									{
										ID: "call_789",
										Function: types.OpenAIToolCallFunction{
											Arguments: `"test.txt","content":"Hello"}`,
										},
									},
								},
							},
						},
					},
				},
			},
			finalChunk: &types.OpenAIStreamChunk{
				ID:      "stream_456",
				Created: 1640995200,
				Model:   "kimi-k2",
				Choices: []types.OpenAIStreamChoice{
					{FinishReason: stringPtr("tool_calls")},
				},
			},
			expectedMsg: "",
			expectedTools: []types.OpenAIToolCall{
				{
					ID:   "call_789",
					Type: "function",
					Function: types.OpenAIToolCallFunction{
						Name:      "Write",
						Arguments: `{"file_path":"test.txt","content":"Hello"}`,
					},
				},
			},
			expectedFinish: stringPtr("tool_calls"),
		},
		{
			name: "mixed_content_and_tool_streaming",
			chunks: []types.OpenAIStreamChunk{
				{
					ID:      "stream_789",
					Created: 1640995200,
					Model:   "kimi-k2",
					Choices: []types.OpenAIStreamChoice{
						{Delta: types.OpenAIStreamDelta{Content: "I'll write the file. "}},
					},
				},
				{
					ID:      "stream_789",
					Created: 1640995200,
					Model:   "kimi-k2",
					Choices: []types.OpenAIStreamChoice{
						{
							Delta: types.OpenAIStreamDelta{
								ToolCalls: []types.OpenAIToolCall{
									{
										ID:   "call_abc",
										Type: "function",
										Function: types.OpenAIToolCallFunction{
											Name:      "Write",
											Arguments: `{"file_path":"output.txt","content":"data"}`,
										},
									},
								},
							},
						},
					},
				},
			},
			finalChunk: &types.OpenAIStreamChunk{
				ID:      "stream_789",
				Created: 1640995200,
				Model:   "kimi-k2",
				Choices: []types.OpenAIStreamChoice{
					{FinishReason: stringPtr("tool_calls")},
				},
			},
			expectedMsg: "I'll write the file. ",
			expectedTools: []types.OpenAIToolCall{
				{
					ID:   "call_abc",
					Type: "function",
					Function: types.OpenAIToolCallFunction{
						Name:      "Write",
						Arguments: `{"file_path":"output.txt","content":"data"}`,
					},
				},
			},
			expectedFinish: stringPtr("tool_calls"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a handler to call the method
			cfg := getTestConfig()
			handler := proxy.NewHandler(cfg, nil, nil)
			
			ctx := internal.WithRequestID(context.Background(), "stream_reconstruction_test")
			result, err := handler.ReconstructResponseFromChunks(ctx, tt.chunks, tt.finalChunk)
			require.NoError(t, err)

			// Verify basic response structure
			assert.Equal(t, tt.chunks[0].ID, result.ID)
			assert.Equal(t, tt.chunks[0].Model, result.Model)
			assert.Equal(t, "chat.completion", result.Object)
			require.Len(t, result.Choices, 1)

			choice := result.Choices[0]

			// Verify message content
			assert.Equal(t, "assistant", choice.Message.Role)
			assert.Equal(t, tt.expectedMsg, choice.Message.Content)

			// Verify finish reason
			if tt.expectedFinish != nil {
				require.NotNil(t, choice.FinishReason)
				assert.Equal(t, *tt.expectedFinish, *choice.FinishReason)
			}

			// Verify tool calls
			if len(tt.expectedTools) > 0 {
				require.Equal(t, len(tt.expectedTools), len(choice.Message.ToolCalls))
				for i, expectedTool := range tt.expectedTools {
					assert.Equal(t, expectedTool.ID, choice.Message.ToolCalls[i].ID)
					assert.Equal(t, expectedTool.Type, choice.Message.ToolCalls[i].Type)
					assert.Equal(t, expectedTool.Function.Name, choice.Message.ToolCalls[i].Function.Name)
					assert.Equal(t, expectedTool.Function.Arguments, choice.Message.ToolCalls[i].Function.Arguments)
				}
			}
		})
	}
}

// TestProcessStreamingResponse tests the full streaming response processing
func TestProcessStreamingResponse(t *testing.T) {
	// Create a mock HTTP response with streaming data
	streamData := `data: {"id":"stream_123","object":"chat.completion.chunk","created":1640995200,"model":"kimi-k2","choices":[{"index":0,"delta":{"content":"Hello "},"finish_reason":null}]}

data: {"id":"stream_123","object":"chat.completion.chunk","created":1640995200,"model":"kimi-k2","choices":[{"index":0,"delta":{"content":"world!"},"finish_reason":null}]}

data: {"id":"stream_123","object":"chat.completion.chunk","created":1640995200,"model":"kimi-k2","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`

	resp := &http.Response{
		Body: &mockReadCloser{strings.NewReader(streamData)},
	}

	// Create a handler to call the method
	cfg := getTestConfig()
	handler := proxy.NewHandler(cfg, nil, nil)
	
	ctx := internal.WithRequestID(context.Background(), "stream_processing_test")
	result, err := handler.ProcessStreamingResponse(ctx, resp)
	require.NoError(t, err)

	assert.Equal(t, "stream_123", result.ID)
	assert.Equal(t, "kimi-k2", result.Model)
	require.Len(t, result.Choices, 1)

	choice := result.Choices[0]
	assert.Equal(t, "assistant", choice.Message.Role)
	assert.Equal(t, "Hello world!", choice.Message.Content)
	require.NotNil(t, choice.FinishReason)
	assert.Equal(t, "stop", *choice.FinishReason)
}

// TestEmptyStreamingResponse tests edge case handling
func TestEmptyStreamingResponse(t *testing.T) {
	// Create a handler to call the method
	cfg := getTestConfig()
	handler := proxy.NewHandler(cfg, nil, nil)
	
	ctx := internal.WithRequestID(context.Background(), "empty_stream_test")
	_, err := handler.ReconstructResponseFromChunks(ctx, []types.OpenAIStreamChunk{}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no chunks received")
}

// mockReadCloser implements io.ReadCloser for testing
type mockReadCloser struct {
	*strings.Reader
}

func (m *mockReadCloser) Close() error {
	return nil
}
