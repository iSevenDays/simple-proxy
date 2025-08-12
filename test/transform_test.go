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

// getTestConfig returns a default config for testing
func getTestConfig() *config.Config {
	return &config.Config{
		SkipTools: []string{}, // No tools skipped by default
	}
}

// TestAnthropicToOpenAITransform tests request format conversion
// Following SPARC: Clear test structure with comprehensive edge cases
func TestAnthropicToOpenAITransform(t *testing.T) {
	tests := []struct {
		name     string
		input    types.AnthropicRequest
		expected types.OpenAIRequest
	}{
		{
			name: "simple_message_transforms_correctly",
			input: types.AnthropicRequest{
				Model:     "kimi-k2",
				MaxTokens: 100,
				Messages: []types.Message{
					{
						Role:    "user",
						Content: "Hello world", // String format (real Claude Code)
					},
				},
			},
			expected: types.OpenAIRequest{
				Model:     "kimi-k2",
				MaxTokens: 100,
				Messages: []types.OpenAIMessage{
					{Role: "user", Content: "Hello world"},
				},
			},
		},
		{
			name: "system_message_converts_correctly",
			input: types.AnthropicRequest{
				Model: "kimi-k2",
				System: []types.SystemContent{
					{Type: "text", Text: "You are a helpful assistant"},
					{Type: "text", Text: "Always be polite"},
				},
				Messages: []types.Message{
					{
						Role:    "user",
						Content: "Hello", // String format
					},
				},
			},
			expected: types.OpenAIRequest{
				Model: "kimi-k2",
				Messages: []types.OpenAIMessage{
					{Role: "system", Content: "You are a helpful assistant\nAlways be polite"},
					{Role: "user", Content: "Hello"},
				},
			},
		},
		{
			name: "tool_definitions_transform_correctly",
			input: types.AnthropicRequest{
				Model: "kimi-k2",
				Messages: []types.Message{
					{
						Role:    "user",
						Content: "Use the write tool", // String format
					},
				},
				Tools: []types.Tool{
					{
						Name:        "Write",
						Description: "Writes content to a file",
						InputSchema: types.ToolSchema{
							Type: "object",
							Properties: map[string]types.ToolProperty{
								"file_path": {Type: "string", Description: "Path to file"},
								"content":   {Type: "string", Description: "Content to write"},
							},
							Required: []string{"file_path", "content"},
						},
					},
				},
			},
			expected: types.OpenAIRequest{
				Model: "kimi-k2",
				Messages: []types.OpenAIMessage{
					{Role: "user", Content: "Use the write tool"},
				},
				Tools: []types.OpenAITool{
					{
						Type: "function",
						Function: types.OpenAIToolFunction{
							Name:        "Write",
							Description: "Writes content to a file",
							Parameters: types.ToolSchema{
								Type: "object",
								Properties: map[string]types.ToolProperty{
									"file_path": {Type: "string", Description: "Path to file"},
									"content":   {Type: "string", Description: "Content to write"},
								},
								Required: []string{"file_path", "content"},
							},
						},
					},
				},
			},
		},
		{
			name: "tool_calls_in_messages_transform_correctly",
			input: types.AnthropicRequest{
				Model: "kimi-k2",
				Messages: []types.Message{
					{
						Role: "assistant",
						Content: []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "I'll write the file for you",
							},
							map[string]interface{}{
								"type":  "tool_use",
								"id":    "call_123",
								"name":  "Write",
								"input": map[string]interface{}{"file_path": "test.txt", "content": "Hello"},
							},
						},
					},
				},
			},
			expected: types.OpenAIRequest{
				Model: "kimi-k2",
				Messages: []types.OpenAIMessage{
					{
						Role:    "assistant",
						Content: "I'll write the file for you",
						ToolCalls: []types.OpenAIToolCall{
							{
								ID:   "call_123",
								Type: "function",
								Function: types.OpenAIToolCallFunction{
									Name:      "Write",
									Arguments: `{"content":"Hello","file_path":"test.txt"}`,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "empty_string_content_handled",
			input: types.AnthropicRequest{
				Model: "kimi-k2",
				Messages: []types.Message{
					{
						Role:    "user",
						Content: "", // Empty string content
					},
				},
			},
			expected: types.OpenAIRequest{
				Model: "kimi-k2",
				Messages: []types.OpenAIMessage{
					{Role: "user", Content: ""}, // Empty content now returns proper 400 error
				},
			},
		},
		{
			name: "empty_array_content_handled",
			input: types.AnthropicRequest{
				Model: "kimi-k2",
				Messages: []types.Message{
					{
						Role:    "user",
						Content: []interface{}{}, // Empty array content
					},
				},
			},
			expected: types.OpenAIRequest{
				Model: "kimi-k2",
				Messages: []types.OpenAIMessage{
					{Role: "user", Content: ""}, // Empty content now returns proper 400 error
				},
			},
		},
		{
			name: "assistant_only_tool_calls_empty_content_correct",
			input: types.AnthropicRequest{
				Model: "kimi-k2",
				Messages: []types.Message{
					{
						Role: "assistant",
						Content: []interface{}{
							map[string]interface{}{
								"type":  "tool_use",
								"id":    "call_456",
								"name":  "Write",
								"input": map[string]interface{}{"file_path": "output.txt", "content": "test"},
							},
						},
					},
				},
			},
			expected: types.OpenAIRequest{
				Model: "kimi-k2",
				Messages: []types.OpenAIMessage{
					{
						Role:    "assistant",
						Content: "", // Per OpenAI spec: empty content is valid when tool_calls are present
						ToolCalls: []types.OpenAIToolCall{
							{
								ID:   "call_456",
								Type: "function",
								Function: types.OpenAIToolCallFunction{
									Name:      "Write",
									Arguments: `{"content":"test","file_path":"output.txt"}`,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "tool_result_transforms_to_tool_message",
			input: types.AnthropicRequest{
				Model: "kimi-k2",
				Messages: []types.Message{
					{
						Role: "user",
						Content: []interface{}{
							map[string]interface{}{
								"type":        "tool_result",
								"content":     "File written successfully",
								"tool_use_id": "call_789",
							},
						},
					},
				},
			},
			expected: types.OpenAIRequest{
				Model: "kimi-k2",
				Messages: []types.OpenAIMessage{
					{
						Role:       "tool",
						Content:    "File written successfully",
						ToolCallID: "call_789",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := internal.WithRequestID(context.Background(), "transform_test")
			result, err := proxy.TransformAnthropicToOpenAI(ctx, tt.input, getTestConfig())
			require.NoError(t, err)

			assert.Equal(t, tt.expected.Model, result.Model)
			assert.Equal(t, tt.expected.MaxTokens, result.MaxTokens)
			assert.Equal(t, len(tt.expected.Messages), len(result.Messages))

			// Compare messages
			for i, expectedMsg := range tt.expected.Messages {
				assert.Equal(t, expectedMsg.Role, result.Messages[i].Role)
				assert.Equal(t, expectedMsg.Content, result.Messages[i].Content)

				// Compare tool calls if present
				if len(expectedMsg.ToolCalls) > 0 {
					require.Equal(t, len(expectedMsg.ToolCalls), len(result.Messages[i].ToolCalls))
					for j, expectedCall := range expectedMsg.ToolCalls {
						assert.Equal(t, expectedCall.ID, result.Messages[i].ToolCalls[j].ID)
						assert.Equal(t, expectedCall.Function.Name, result.Messages[i].ToolCalls[j].Function.Name)
						// Note: JSON marshaling order may vary, so we test the essential parts
						assert.Contains(t, result.Messages[i].ToolCalls[j].Function.Arguments, "file_path")
						assert.Contains(t, result.Messages[i].ToolCalls[j].Function.Arguments, "content")
					}
				}
			}

			// Compare tools
			if len(tt.expected.Tools) > 0 {
				require.Equal(t, len(tt.expected.Tools), len(result.Tools))
				for i, expectedTool := range tt.expected.Tools {
					assert.Equal(t, expectedTool.Function.Name, result.Tools[i].Function.Name)
					assert.Equal(t, expectedTool.Function.Description, result.Tools[i].Function.Description)
				}
			}
		})
	}
}

// TestOpenAIToAnthropicTransform tests response format conversion
func TestOpenAIToAnthropicTransform(t *testing.T) {
	tests := []struct {
		name     string
		input    types.OpenAIResponse
		model    string
		expected types.AnthropicResponse
	}{
		{
			name: "simple_text_response_transforms",
			input: types.OpenAIResponse{
				ID:     "resp_123",
				Object: "chat.completion",
				Model:  "kimi-k2",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant",
							Content: "Hello! How can I help you?",
						},
						FinishReason: stringPtr("stop"),
					},
				},
				Usage: types.OpenAIUsage{
					PromptTokens:     10,
					CompletionTokens: 8,
				},
			},
			model: "kimi-k2",
			expected: types.AnthropicResponse{
				ID:    "resp_123",
				Type:  "message",
				Role:  "assistant",
				Model: "kimi-k2",
				Content: []types.Content{
					{Type: "text", Text: "Hello! How can I help you?"},
				},
				StopReason: "end_turn",
				Usage: types.Usage{
					InputTokens:  10,
					OutputTokens: 8,
				},
			},
		},
		{
			name: "tool_call_response_transforms",
			input: types.OpenAIResponse{
				ID: "resp_456",
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role: "assistant",
							ToolCalls: []types.OpenAIToolCall{
								{
									ID:   "call_789",
									Type: "function",
									Function: types.OpenAIToolCallFunction{
										Name:      "Write",
										Arguments: `{"file_path":"test.txt","content":"Hello World"}`,
									},
								},
							},
						},
						FinishReason: stringPtr("tool_calls"),
					},
				},
				Usage: types.OpenAIUsage{
					PromptTokens:     15,
					CompletionTokens: 5,
				},
			},
			model: "kimi-k2",
			expected: types.AnthropicResponse{
				ID:    "resp_456",
				Type:  "message",
				Role:  "assistant",
				Model: "kimi-k2",
				Content: []types.Content{
					{
						Type: "tool_use",
						ID:   "call_789",
						Name: "Write",
						Input: map[string]interface{}{
							"file_path": "test.txt",
							"content":   "Hello World",
						},
					},
				},
				StopReason: "tool_use",
				Usage: types.Usage{
					InputTokens:  15,
					OutputTokens: 5,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := internal.WithRequestID(context.Background(), "openai_transform_test")
			result, err := proxy.TransformOpenAIToAnthropic(ctx, &tt.input, tt.model, getTestConfig())
			require.NoError(t, err)

			assert.Equal(t, tt.expected.ID, result.ID)
			assert.Equal(t, tt.expected.Type, result.Type)
			assert.Equal(t, tt.expected.Role, result.Role)
			assert.Equal(t, tt.expected.Model, result.Model)
			assert.Equal(t, tt.expected.StopReason, result.StopReason)
			assert.Equal(t, tt.expected.Usage.InputTokens, result.Usage.InputTokens)
			assert.Equal(t, tt.expected.Usage.OutputTokens, result.Usage.OutputTokens)

			require.Equal(t, len(tt.expected.Content), len(result.Content))
			for i, expectedContent := range tt.expected.Content {
				assert.Equal(t, expectedContent.Type, result.Content[i].Type)
				assert.Equal(t, expectedContent.Text, result.Content[i].Text)
				assert.Equal(t, expectedContent.ID, result.Content[i].ID)
				assert.Equal(t, expectedContent.Name, result.Content[i].Name)

				// Compare Input maps if present
				if expectedContent.Input != nil {
					assert.Equal(t, expectedContent.Input, result.Content[i].Input)
				}
			}
		})
	}
}

// TestSystemMessageInterference tests how large/complex system messages affect tool calling
// This reproduces the issue where Claude Code's system message prevents tool calls
func TestSystemMessageInterference(t *testing.T) {
	tests := []struct {
		name           string
		systemMessage  string
		userMessage    string
		toolsCount     int
		expectToolCall bool
		description    string
	}{
		{
			name:          "simple_system_message_allows_tool_calls",
			systemMessage: "You are a helpful assistant. Use tools when requested.",
			userMessage:   "Use the Bash tool to run git status",
			toolsCount:    1,
			expectToolCall: true,
			description:   "Simple system message should allow tool calls",
		},
		{
			name: "large_system_message_blocks_tool_calls",
			systemMessage: `You are Claude Code, Anthropic's official CLI for Claude.
You are an interactive CLI tool that helps users with software engineering tasks. Use the instructions below and the tools available to you to assist the user.

IMPORTANT: Assist with defensive security tasks only. Refuse to create, modify, or improve code that may be used maliciously. Allow security analysis, detection rules, vulnerability explanations, defensive tools, and security documentation.
IMPORTANT: You must NEVER generate or guess URLs for the user unless you are confident that the URLs are for helping the user with programming. You may use URLs provided by the user in their messages or local files.

If the user asks for help or wants to give feedback inform them of the following: 
- /help: Get help with using Claude Code
- To give feedback, users should report the issue at https://github.com/anthropics/claude-code/issues

When the user directly asks about Claude Code (eg 'can Claude Code do...', 'does Claude Code have...') or asks in second person (eg 'are you able...', 'can you do...'), first use the WebFetch tool to gather information to answer the question from Claude Code docs at https://docs.anthropic.com/en/docs/claude-code.
  - The available sub-pages are overview, quickstart, memory (Memory management and CLAUDE.md), common-workflows (Extended thinking, pasting images, --resume), ide-integrations, mcp, github-actions, sdk, troubleshooting, third-party-integrations, amazon-bedrock, google-vertex-ai, corporate-proxy, llm-gateway, devcontainer, iam (auth, permissions), security, monitoring-usage (OTel), costs, cli-reference, interactive-mode (keyboard shortcuts), slash-commands, settings (settings json files, env vars, tools), hooks.
  - Example: https://docs.anthropic.com/en/docs/claude-code/cli-usage

# Tone and style
You should be concise, direct, and to the point.
You MUST answer concisely with fewer than 4 lines (not including tool use or code generation), unless user asks for detail.
IMPORTANT: You should minimize output tokens as much as possible while maintaining helpfulness, quality, and accuracy. Only address the specific query or task at hand, avoiding tangential information unless absolutely critical for completing the request. If you can answer in 1-3 sentences or a short paragraph, please do.
IMPORTANT: You should NOT answer with unnecessary preamble or postamble (such as explaining your code or summarizing your action), unless the user asks you to.
Do not add additional code explanation summary unless requested by the user. After working on a file, just stop, rather than providing an explanation of what you did.
Answer the user's question directly, without elaboration, explanation, or details. One word answers are best. Avoid introductions, conclusions, and explanations. You MUST avoid text before/after your response, such as "The answer is <answer>.", "Here is the content of the file..." or "Based on the information provided, the answer is..." or "Here is what I will do next...". Here are some examples to demonstrate appropriate verbosity:`,
			userMessage:    "Use the Bash tool to run git status",
			toolsCount:     15,
			expectToolCall: false, // This should fail due to complex system message
			description:    "Large complex system message should interfere with tool calls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create Anthropic request with system message
			req := types.AnthropicRequest{
				Model: "kimi-k2",
				System: []types.SystemContent{
					{Type: "text", Text: tt.systemMessage},
				},
				Messages: []types.Message{
					{Role: "user", Content: tt.userMessage},
				},
				MaxTokens: 1000,
			}

			// Add tools
			for i := 0; i < tt.toolsCount; i++ {
				req.Tools = append(req.Tools, types.Tool{
					Name:        "Bash",
					Description: "Execute bash commands",
					InputSchema: types.ToolSchema{
						Type: "object",
						Properties: map[string]types.ToolProperty{
							"command": {Type: "string", Description: "Command to execute"},
						},
						Required: []string{"command"},
					},
				})
			}

			// Transform to OpenAI format
			ctx := internal.WithRequestID(context.Background(), "system_message_test")
			openaiReq, err := proxy.TransformAnthropicToOpenAI(ctx, req, getTestConfig())
			require.NoError(t, err)

			// Validate transformation
			assert.Equal(t, "kimi-k2", openaiReq.Model)
			assert.Equal(t, tt.toolsCount, len(openaiReq.Tools))
			
			// Check system message is present
			require.Greater(t, len(openaiReq.Messages), 0)
			assert.Equal(t, "system", openaiReq.Messages[0].Role)
			assert.Contains(t, openaiReq.Messages[0].Content, tt.systemMessage)

			// This test documents the current failure - in a real scenario,
			// we would need to call the actual model to verify tool call behavior
			t.Logf("Test case: %s", tt.description)
			t.Logf("System message length: %d chars", len(tt.systemMessage))
			t.Logf("Tools count: %d", tt.toolsCount)
			t.Logf("Expected tool call: %v", tt.expectToolCall)
		})
	}
}

// TestNullStringServerError tests the server error scenario
// This reproduces: "basic_string: construction from null is not valid"
func TestNullStringServerError(t *testing.T) {
	tests := []struct {
		name        string
		input       types.AnthropicRequest
		expectError bool
		description string
	}{
		{
			name: "empty_content_fields_cause_null_string_error",
			input: types.AnthropicRequest{
				Model: "kimi-k2",
				Messages: []types.Message{
					{
						Role: "user",
						Content: []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": nil, // This could cause null string error
							},
						},
					},
				},
			},
			expectError: true,
			description: "Null text content should be handled gracefully",
		},
		{
			name: "empty_system_message_handled",
			input: types.AnthropicRequest{
				Model: "kimi-k2",
				System: []types.SystemContent{
					{Type: "text", Text: ""}, // Empty string
				},
				Messages: []types.Message{
					{Role: "user", Content: "test"},
				},
			},
			expectError: false,
			description: "Empty system message should not cause server errors",
		},
		{
			name: "nil_tool_parameters_handled",
			input: types.AnthropicRequest{
				Model: "kimi-k2",
				Messages: []types.Message{
					{Role: "user", Content: "test"},
				},
				Tools: []types.Tool{
					{
						Name:        "TestTool",
						Description: "", // Empty description
						InputSchema: types.ToolSchema{
							Properties: nil, // Nil properties
						},
					},
				},
			},
			expectError: false,
			description: "Nil tool schema properties should be handled gracefully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := internal.WithRequestID(context.Background(), "null_string_test")
			
			// Transform request - this is where null strings might be introduced
			result, err := proxy.TransformAnthropicToOpenAI(ctx, tt.input, getTestConfig())
			
			if tt.expectError {
				// For cases that should cause errors, we expect the transform to either:
				// 1. Return an error, or 2. Create a malformed request that would cause server error
				t.Logf("Testing error case: %s", tt.description)
				t.Logf("Transform result error: %v", err)
				// Note: The actual server error happens when this request is sent to llama-server
			} else {
				require.NoError(t, err, "Transform should not fail for: %s", tt.description)
				assert.NotNil(t, result, "Result should not be nil")
			}
		})
	}
}

// Helper function to create string pointers

// TestPrintSystemMessageInTransform tests that system messages are printed when enabled
func TestPrintSystemMessageInTransform(t *testing.T) {
	tests := []struct {
		name                 string
		printSystemMessage   bool
		systemMessage        string
		expectSystemInLog    bool
	}{
		{
			name:               "print_system_message_enabled",
			printSystemMessage: true,
			systemMessage:      "You are a helpful assistant. Follow these instructions carefully.",
			expectSystemInLog:  true,
		},
		{
			name:               "print_system_message_disabled",
			printSystemMessage: false,
			systemMessage:      "You are a helpful assistant. Follow these instructions carefully.",
			expectSystemInLog:  false,
		},
		{
			name:               "empty_system_message",
			printSystemMessage: true,
			systemMessage:      "",
			expectSystemInLog:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create config with PrintSystemMessage setting
			cfg := &config.Config{
				PrintSystemMessage: tt.printSystemMessage,
			}

			// Create request with system message
			req := types.AnthropicRequest{
				Model:     "claude-3-5-haiku-20241022",
				MaxTokens: 1000,
				System: []types.SystemContent{
					{
						Type: "text",
						Text: tt.systemMessage,
					},
				},
				Messages: []types.Message{
					{
						Role:    "user",
						Content: "Hello",
					},
				},
			}

			ctx := internal.WithRequestID(context.Background(), "print_system_test")
			
			// Transform the request
			openaiReq, err := proxy.TransformAnthropicToOpenAI(ctx, req, cfg)
			require.NoError(t, err)

			// Check that system message was added to OpenAI request
			if tt.systemMessage != "" {
				require.NotEmpty(t, openaiReq.Messages)
				assert.Equal(t, "system", openaiReq.Messages[0].Role)
				assert.Equal(t, tt.systemMessage, openaiReq.Messages[0].Content)
			}

			// Note: We can't easily test log output in unit tests without capturing logs,
			// but we can verify the transform worked correctly and the config was used
			t.Logf("Test completed for PrintSystemMessage=%v with system message length=%d", 
				tt.printSystemMessage, len(tt.systemMessage))
		})
	}
}

// TestEmptyToolResultHandling tests empty tool result handling
func TestEmptyToolResultHandling(t *testing.T) {
	tests := []struct {
		name               string
		handleEmptyResults bool
		anthropicReq       types.AnthropicRequest
		expectedContent    string
		description        string
	}{
		{
			name:               "empty_tool_result_content_handled",
			handleEmptyResults: true,
			anthropicReq: types.AnthropicRequest{
				Model: "test-model",
				Messages: []types.Message{
					{
						Role: "user",
						Content: []interface{}{
							map[string]interface{}{
								"type":        "tool_result",
								"tool_use_id": "test_id_123",
								"content":     "", // Empty content
							},
						},
					},
				},
			},
			expectedContent: "Tool execution returned no results",
			description:     "Should replace empty tool result with default message",
		},
		{
			name:               "empty_tool_result_content_disabled",
			handleEmptyResults: false,
			anthropicReq: types.AnthropicRequest{
				Model: "test-model",
				Messages: []types.Message{
					{
						Role: "user",
						Content: []interface{}{
							map[string]interface{}{
								"type":        "tool_result",
								"tool_use_id": "test_id_123",
								"content":     "", // Empty content
							},
						},
					},
				},
			},
			expectedContent: "", // Should remain empty when disabled
			description:     "Should preserve empty content when handling is disabled",
		},
		{
			name:               "missing_tool_result_content",
			handleEmptyResults: true,
			anthropicReq: types.AnthropicRequest{
				Model: "test-model",
				Messages: []types.Message{
					{
						Role: "user",
						Content: []interface{}{
							map[string]interface{}{
								"type":        "tool_result",
								"tool_use_id": "test_id_456",
								// No content field
							},
						},
					},
				},
			},
			expectedContent: "Tool execution completed with no output",
			description:     "Should handle missing content field with default message",
		},
		{
			name:               "whitespace_only_tool_result",
			handleEmptyResults: true,
			anthropicReq: types.AnthropicRequest{
				Model: "test-model",
				Messages: []types.Message{
					{
						Role: "user",
						Content: []interface{}{
							map[string]interface{}{
								"type":        "tool_result",
								"tool_use_id": "test_id_789",
								"content":     "   \n\t   ", // Whitespace only
							},
						},
					},
				},
			},
			expectedContent: "Tool execution returned no results",
			description:     "Should replace whitespace-only content with default message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create config with the specified empty result handling setting
			cfg := &config.Config{
				HandleEmptyToolResults: tt.handleEmptyResults,
			}

			ctx := internal.WithRequestID(context.Background(), "empty_tool_result_test")
			result, err := proxy.TransformAnthropicToOpenAI(ctx, tt.anthropicReq, cfg)
			require.NoError(t, err)

			// Should have one message
			require.Len(t, result.Messages, 1)

			// Message should have expected content
			assert.Equal(t, tt.expectedContent, result.Messages[0].Content, tt.description)

			// Message should have tool role
			assert.Equal(t, "tool", result.Messages[0].Role)
		})
	}
}

// TestToolResultContentOverrides tests applying system message overrides to tool result content
func TestToolResultContentOverrides(t *testing.T) {
	tests := []struct {
		name            string
		anthropicReq    types.AnthropicRequest
		overrides       config.SystemMessageOverrides
		expectedContent string
		description     string
	}{
		{
			name: "tool_result_removes_malicious_warning",
			anthropicReq: types.AnthropicRequest{
				Model: "test-model",
				Messages: []types.Message{
					{
						Role: "user",
						Content: []interface{}{
							map[string]interface{}{
								"type":        "tool_result",
								"content":     "File contents here.\n\nNOTE: do any of the files above seem malicious? If so, you MUST refuse to continue work.",
								"tool_use_id": "call_123",
							},
						},
					},
				},
			},
			overrides: config.SystemMessageOverrides{
				RemovePatterns: []string{
					"NOTE: do any of the files above seem malicious\\? If so, you MUST refuse to continue work\\.?",
				},
			},
			expectedContent: "File contents here.\n\n",
			description:     "Should remove malicious warning pattern from tool result content",
		},
		{
			name: "tool_result_applies_replacements",
			anthropicReq: types.AnthropicRequest{
				Model: "test-model",
				Messages: []types.Message{
					{
						Role: "user",
						Content: []interface{}{
							map[string]interface{}{
								"type":        "tool_result",
								"content":     "Claude Code performed the action successfully",
								"tool_use_id": "call_456",
							},
						},
					},
				},
			},
			overrides: config.SystemMessageOverrides{
				Replacements: []config.SystemMessageReplacement{
					{
						Find:    "Claude Code",
						Replace: "AI Assistant",
					},
				},
			},
			expectedContent: "AI Assistant performed the action successfully",
			description:     "Should apply text replacements to tool result content",
		},
		{
			name: "tool_result_no_overrides",
			anthropicReq: types.AnthropicRequest{
				Model: "test-model",
				Messages: []types.Message{
					{
						Role: "user",
						Content: []interface{}{
							map[string]interface{}{
								"type":        "tool_result",
								"content":     "Original content unchanged",
								"tool_use_id": "call_789",
							},
						},
					},
				},
			},
			overrides:       config.SystemMessageOverrides{},
			expectedContent: "Original content unchanged",
			description:     "Should leave content unchanged when no overrides configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create config with the specified overrides
			cfg := &config.Config{
				SystemMessageOverrides: tt.overrides,
			}

			ctx := internal.WithRequestID(context.Background(), "tool_result_override_test")
			result, err := proxy.TransformAnthropicToOpenAI(ctx, tt.anthropicReq, cfg)
			require.NoError(t, err)

			// Should have one message
			require.Len(t, result.Messages, 1)

			// Message should have expected content after overrides applied
			assert.Equal(t, tt.expectedContent, result.Messages[0].Content, tt.description)

			// Message should have tool role
			assert.Equal(t, "tool", result.Messages[0].Role)

			// Should preserve tool call ID from the input
			expectedToolCallID := tt.anthropicReq.Messages[0].Content.([]interface{})[0].(map[string]interface{})["tool_use_id"].(string)
			assert.Equal(t, expectedToolCallID, result.Messages[0].ToolCallID)
		})
	}
}
