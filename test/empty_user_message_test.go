package test

import (
	"claude-proxy/config"
	"claude-proxy/internal"
	"claude-proxy/proxy"
	"claude-proxy/types"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmptyUserMessageHandling(t *testing.T) {
	tests := []struct {
		name                    string
		handleEmptyUserMessages bool
		anthropicReq            types.AnthropicRequest
		expectedContent         string
		description             string
	}{
		{
			name:                    "empty_user_message_handled",
			handleEmptyUserMessages: true,
			anthropicReq: types.AnthropicRequest{
				Model: "test-model",
				Messages: []types.Message{
					{
						Role:    "user",
						Content: "", // Empty content
					},
				},
			},
			expectedContent: "[Empty user message]",
			description:     "Should replace empty user message with placeholder when enabled",
		},
		{
			name:                    "empty_user_message_preserved",
			handleEmptyUserMessages: false,
			anthropicReq: types.AnthropicRequest{
				Model: "test-model",
				Messages: []types.Message{
					{
						Role:    "user",
						Content: "", // Empty content
					},
				},
			},
			expectedContent: "", // Should remain empty when disabled
			description:     "Should preserve empty user message when handling is disabled",
		},
		{
			name:                    "non_empty_user_message_unchanged",
			handleEmptyUserMessages: true,
			anthropicReq: types.AnthropicRequest{
				Model: "test-model",
				Messages: []types.Message{
					{
						Role:    "user",
						Content: "Hello world",
					},
				},
			},
			expectedContent: "Hello world",
			description:     "Should not modify non-empty user messages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				HandleEmptyUserMessages: tt.handleEmptyUserMessages,
			}

			ctx := internal.WithRequestID(context.Background(), "empty_user_message_test")
			result, err := proxy.TransformAnthropicToOpenAI(ctx, tt.anthropicReq, cfg)
			require.NoError(t, err)

			// Should have one message
			require.Len(t, result.Messages, 1)

			// Message should have expected content
			assert.Equal(t, tt.expectedContent, result.Messages[0].Content, tt.description)

			// Message should have user role
			assert.Equal(t, "user", result.Messages[0].Role)
		})
	}
}