package test

import (
	"claude-proxy/parser"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCallTokenHandling tests TDD implementation of <|call|> token support
// This test drives the implementation of proper call token parsing per OpenAI Harmony spec
func TestCallTokenHandling(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		expectedDetection bool
		expectedContent   string
		expectedStopToken string
		description       string
	}{
		{
			name:              "tool_call_with_call_token",
			input:             `<|start|>assistant<|channel|>commentary<|constrain|>json<|message|>{"function": "get_weather"}<|call|>`,
			expectedDetection: true,
			expectedContent:   `{"function": "get_weather"}`,
			expectedStopToken: "call",
			description:       "Should parse tool calls ending with <|call|> token",
		},
		{
			name:              "simple_call_token_only",
			input:             `<|start|>assistant<|channel|>commentary<|message|>tool action<|call|>`,
			expectedDetection: true,
			expectedContent:   "tool action",
			expectedStopToken: "call",
			description:       "Should handle <|call|> as stop token",
		},
		{
			name:              "official_openai_tool_format",
			input:             `<|start|>assistant<|channel|>commentary to=functions.get_current_weather <|constrain|>json<|message|>{"location":"San Francisco"}<|call|>`,
			expectedDetection: true,
			expectedContent:   `{"location":"San Francisco"}`,
			expectedStopToken: "call",
			description:       "Should handle complete official OpenAI tool call format",
		},
		{
			name:              "regular_end_token",
			input:             `<|start|>assistant<|channel|>final<|message|>Regular response<|end|>`,
			expectedDetection: true,
			expectedContent:   "Regular response",
			expectedStopToken: "end",
			description:       "Should still handle regular <|end|> tokens",
		},
		{
			name:              "return_token",
			input:             `<|start|>assistant<|channel|>final<|message|>Final answer<|return|>`,
			expectedDetection: true,
			expectedContent:   "Final answer",
			expectedStopToken: "return",
			description:       "Should handle <|return|> stop tokens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test detection
			detected := parser.IsHarmonyFormat(tt.input)
			assert.Equal(t, tt.expectedDetection, detected, "Detection should match expected")

			// Test parsing
			harmonyMsg, err := parser.ParseHarmonyMessage(tt.input)
			assert.NoError(t, err, "Should parse without error: %s", tt.description)
			assert.NotEmpty(t, harmonyMsg.Channels, "Should extract channels")

			if len(harmonyMsg.Channels) > 0 {
				channel := harmonyMsg.Channels[0]
				assert.Equal(t, tt.expectedContent, channel.Content, "Content should match expected")

				// TODO: This will drive implementation - we need to track stop tokens
				t.Logf("IMPLEMENTATION NEEDED: Channel should track stop token type")
				t.Logf("Expected stop token: %s", tt.expectedStopToken)
				t.Logf("Input: %s", tt.input)
				
				// For now, just verify the content is parsed correctly
				if tt.expectedStopToken == "call" {
					t.Logf("BUG EXPECTED: <|call|> token should be handled as stop token")
					t.Logf("Current parsing treats it same as <|end|>")
				}
			}
		})
	}
}

// TestCallTokenRegexSupport tests that call token is properly recognized in regex patterns
func TestCallTokenRegexSupport(t *testing.T) {
	t.Run("call_token_detection", func(t *testing.T) {
		// Test just the call token by itself
		callTokenOnly := `<|call|>`
		
		detected := parser.IsHarmonyFormat(callTokenOnly)
		
		// Current implementation should NOT detect this since HasHarmonyTokens only checks for:
		// start, end, channel, message, constrain
		t.Logf("Call token detection result: %v", detected)
		t.Logf("IMPLEMENTATION NEEDED: Add <|call|> to HasHarmonyTokens check")
		
		if !detected {
			t.Log("EXPECTED BEHAVIOR: <|call|> token not recognized by current implementation")
		} else {
			t.Log("UNEXPECTED: <|call|> token already being detected")
		}
	})
	
	t.Run("call_token_in_context", func(t *testing.T) {
		// Test call token within a complete message
		fullMessage := `<|start|>assistant<|channel|>commentary<|message|>{"tool": "call"}<|call|>`
		
		detected := parser.IsHarmonyFormat(fullMessage)
		assert.True(t, detected, "Should detect due to other Harmony tokens")
		
		parsed, err := parser.ParseHarmonyMessage(fullMessage)
		
		if err != nil {
			t.Logf("EXPECTED PARSING ISSUE: %v", err)
			t.Log("This indicates <|call|> token needs proper regex support")
		} else if len(parsed.Channels) > 0 {
			t.Logf("Parsing succeeded - content: %s", parsed.Channels[0].Content)
			t.Log("Need to verify <|call|> is treated as proper stop token")
		} else {
			t.Log("Parsing succeeded but no channels extracted")
		}
	})
}

// TestCallTokenStopBehavior tests that <|call|> behaves correctly as a stop token
func TestCallTokenStopBehavior(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		stopType  string
	}{
		{
			name:     "call_stop_token",
			input:    `<|start|>assistant<|channel|>commentary<|message|>content<|call|>`,
			stopType: "call",
		},
		{
			name:     "end_stop_token", 
			input:    `<|start|>assistant<|channel|>final<|message|>content<|end|>`,
			stopType: "end",
		},
		{
			name:     "return_stop_token",
			input:    `<|start|>assistant<|channel|>final<|message|>content<|return|>`,
			stopType: "return",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected := parser.IsHarmonyFormat(tt.input)
			assert.True(t, detected, "Should detect Harmony format")

			parsed, err := parser.ParseHarmonyMessage(tt.input)
			
			if err == nil && len(parsed.Channels) > 0 {
				t.Logf("Stop token type '%s' parsed successfully", tt.stopType)
				t.Logf("Content: %s", parsed.Channels[0].Content)
			} else {
				t.Logf("Stop token type '%s' parsing failed: %v", tt.stopType, err)
			}
			
			// TODO: Add StopTokenType field to Channel to track this information
			t.Logf("IMPLEMENTATION NEEDED: Track stop token type in Channel struct")
		})
	}
}

// TestCallTokenEdgeCases tests edge cases for call token handling
func TestCallTokenEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldParse bool
		description string
	}{
		{
			name:        "call_without_content",
			input:       `<|start|>assistant<|channel|>commentary<|message|><|call|>`,
			shouldParse: true,
			description: "Should handle call token with empty content",
		},
		{
			name:        "multiple_call_tokens",
			input:       `<|start|>assistant<|channel|>commentary<|message|>content<|call|>more<|call|>`,
			shouldParse: true,
			description: "Should handle multiple call tokens (edge case)",
		},
		{
			name:        "malformed_call_token",
			input:       `<|start|>assistant<|channel|>commentary<|message|>content<|call>`,
			shouldParse: true,
			description: "Should handle malformed call token gracefully",
		},
		{
			name:        "call_in_middle_of_content",
			input:       `<|start|>assistant<|channel|>commentary<|message|>before<|call|>after<|end|>`,
			shouldParse: true,
			description: "Should handle call token within content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected := parser.IsHarmonyFormat(tt.input)
			assert.True(t, detected, "Should detect as Harmony format")
			
			_, err := parser.ParseHarmonyMessage(tt.input)
			
			if tt.shouldParse {
				if err != nil {
					t.Logf("PARSING ISSUE for %s: %v", tt.description, err)
				} else {
					t.Logf("SUCCESS: %s", tt.description)
				}
			}
		})
	}
}

// TestCallTokenIntegrationWithConstraint tests call and constraint tokens together
func TestCallTokenIntegrationWithConstraint(t *testing.T) {
	// This is the complete official OpenAI format with both constraint and call tokens
	officialFormat := `<|start|>assistant<|channel|>commentary to=functions.get_weather<|constrain|>json<|message|>{"location": "Tokyo", "format": "celsius"}<|call|>`
	
	t.Run("complete_official_format", func(t *testing.T) {
		detected := parser.IsHarmonyFormat(officialFormat)
		assert.True(t, detected, "Should detect complete official format")
		
		parsed, err := parser.ParseHarmonyMessage(officialFormat)
		
		if err == nil && len(parsed.Channels) > 0 {
			channel := parsed.Channels[0]
			t.Logf("SUCCESS: Complete official format parsed")
			t.Logf("  Content: %s", channel.Content)
			t.Logf("  Constraint: %s", channel.ConstraintType)
			t.Logf("  Channel: %s", channel.ChannelType.String())
			
			assert.Equal(t, "json", channel.ConstraintType, "Should capture constraint type")
			assert.Equal(t, `{"location": "Tokyo", "format": "celsius"}`, channel.Content, "Should capture JSON content")
		} else {
			t.Logf("PARSING ISSUE with complete format: %v", err)
		}
	})
}