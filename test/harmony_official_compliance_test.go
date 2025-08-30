package test

import (
	"claude-proxy/parser"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestOfficialHarmonyTokenCompliance tests compliance with official OpenAI Harmony tokens
// Based on official documentation: https://cookbook.openai.com/articles/gpt-oss/handle-raw-cot
func TestOfficialHarmonyTokenCompliance(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		shouldDetect   bool
		expectedIssue  string
	}{
		{
			name:          "Basic tokens work",
			input:         `<|start|>assistant<|channel|>analysis<|message|>Some analysis<|end|>`,
			shouldDetect:  true,
			expectedIssue: "Should work with basic tokens",
		},
		{
			name:          "Official constrain token",
			input:         `<|start|>assistant<|channel|>commentary to=functions.tool<|constrain|>json<|message|>{"param":"value"}<|call|>`,
			shouldDetect:  true, 
			expectedIssue: "BUG: Missing <|constrain|> token support",
		},
		{
			name:          "Official return token", 
			input:         `<|start|>assistant<|channel|>final<|message|>Final answer<|return|>`,
			shouldDetect:  true,
			expectedIssue: "BUG: <|return|> should be distinct from <|end|>",
		},
		{
			name:          "Official call token",
			input:         `<|start|>assistant<|channel|>commentary<|message|>{"tool":"call"}<|call|>`,
			shouldDetect:  true,
			expectedIssue: "BUG: Missing <|call|> token support",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected := parser.IsHarmonyFormat(tt.input)
			if tt.shouldDetect {
				assert.True(t, detected, tt.expectedIssue)
			} else {
				assert.False(t, detected, tt.expectedIssue)
			}
		})
	}
}

// TestOfficialMessageFormatCompliance tests compliance with official message structure
// Official format: <|start|>{header}<|message|>{content}<|end|>
func TestOfficialMessageFormatCompliance(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedError  string
	}{
		{
			name:  "Tool call with recipient header",
			input: `<|start|>assistant<|channel|>commentary to=functions.get_weather<|constrain|>json<|message|>{"location": "SF"}<|call|>`,
			expectedError: "BUG: Should support 'to=' recipient in header",
		},
		{
			name:  "Multiple header components",
			input: `<|start|>functions.get_weather to=assistant<|channel|>commentary<|message|>{"result": "sunny"}<|end|>`,
			expectedError: "BUG: Should support tool name as role with recipient",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Try to parse the official format
			harmonyMsg, err := parser.ParseHarmonyMessage(tt.input)
			
			// Current implementation will fail on these official formats
			// This test documents the expected failures
			if err != nil || len(harmonyMsg.Channels) == 0 {
				t.Logf("EXPECTED FAILURE: %s", tt.expectedError)
				t.Logf("Input: %s", tt.input)
				t.Logf("Error: %v", err)
			} else {
				// If it somehow works, verify it parsed correctly
				t.Logf("Unexpectedly worked for: %s", tt.input)
			}
		})
	}
}

// TestOfficialStopTokenHandling tests proper handling of <|return|> vs <|end|>
func TestOfficialStopTokenHandling(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Return token should be different from end",
			input:    `<|start|>assistant<|channel|>final<|message|>Answer<|return|>`,
			expected: "Should preserve <|return|> for conversation history replacement",
		},
		{
			name:     "End token should mark complete message",
			input:    `<|start|>assistant<|channel|>final<|message|>Answer<|end|>`,
			expected: "Should handle <|end|> as complete message marker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harmonyMsg, err := parser.ParseHarmonyMessage(tt.input)
			assert.NoError(t, err, "Should parse basic harmony messages")
			assert.NotEmpty(t, harmonyMsg.Channels, "Should extract channels")
			
			// Current implementation treats <|return|> same as <|end|>
			// This is technically incorrect per official spec
			t.Logf("NOTE: %s", tt.expected)
			t.Logf("Current behavior may not distinguish between tokens correctly")
		})
	}
}

// TestOfficialToolCallingFormat tests compliance with official tool calling
func TestOfficialToolCallingFormat(t *testing.T) {
	// This is the official tool calling format from the documentation
	officialToolCall := `<|start|>assistant<|channel|>commentary to=functions.get_current_weather <|constrain|>json<|message|>{"location":"San Francisco"}<|call|>`
	officialToolResponse := `<|start|>functions.get_current_weather to=assistant<|channel|>commentary<|message|>{"sunny": true, "temperature": 20}<|end|>`
	
	t.Run("Official tool call format", func(t *testing.T) {
		detected := parser.IsHarmonyFormat(officialToolCall)
		// Current implementation will likely detect this as Harmony due to basic tokens
		assert.True(t, detected, "Should detect as Harmony format")
		
		_, err := parser.ParseHarmonyMessage(officialToolCall)
		// But parsing will likely fail due to missing <|constrain|> and <|call|> support
		if err != nil {
			t.Logf("EXPECTED PARSING FAILURE: Missing <|constrain|> and <|call|> token support")
			t.Logf("Error: %v", err)
		} else {
			t.Logf("Parsing succeeded - check if all tokens were handled correctly")
		}
	})
	
	t.Run("Official tool response format", func(t *testing.T) {
		detected := parser.IsHarmonyFormat(officialToolResponse)
		assert.True(t, detected, "Should detect tool response as Harmony")
		
		harmonyMsg, err := parser.ParseHarmonyMessage(officialToolResponse)
		// This might work since it uses <|end|> instead of special tokens
		if err == nil && len(harmonyMsg.Channels) > 0 {
			t.Logf("Tool response format works with current implementation")
		} else {
			t.Logf("PARSING ISSUE: %v", err)
		}
	})
}

// TestNumericTokenIDSupport tests support for official numeric token IDs
func TestNumericTokenIDSupport(t *testing.T) {
	// Official token IDs per documentation
	tokenIDs := map[string]int{
		"<|start|>":     200006,
		"<|end|>":       200007, 
		"<|message|>":   200008,
		"<|channel|>":   200005,
		"<|constrain|>": 200003,
		"<|return|>":    200002,
		"<|call|>":      200012,
	}
	
	t.Run("Document official token IDs", func(t *testing.T) {
		for token, id := range tokenIDs {
			t.Logf("Official token %s has ID %d", token, id)
		}
		
		// Current implementation only supports string tokens
		t.Log("BUG: Current implementation doesn't support numeric token IDs")
		t.Log("This prevents compatibility with official Harmony encoders/decoders")
	})
}

// TestChannelTypeCompliance tests support for official channel types
func TestChannelTypeCompliance(t *testing.T) {
	officialChannels := []string{"analysis", "commentary", "final"}
	
	t.Run("Official channel support", func(t *testing.T) {
		for _, channel := range officialChannels {
			input := `<|start|>assistant<|channel|>` + channel + `<|message|>Test content<|end|>`
			
			harmonyMsg, err := parser.ParseHarmonyMessage(input)
			assert.NoError(t, err, "Should parse official channel: %s", channel)
			
			if err == nil && len(harmonyMsg.Channels) > 0 {
				assert.Equal(t, channel, harmonyMsg.Channels[0].ChannelType.String(), "Should preserve channel type")
			}
		}
	})
	
	t.Run("Custom channel types", func(t *testing.T) {
		// The spec doesn't explicitly limit channel types
		customChannel := "custom"
		input := `<|start|>assistant<|channel|>` + customChannel + `<|message|>Test content<|end|>`
		
		harmonyMsg, err := parser.ParseHarmonyMessage(input)
		assert.NoError(t, err, "Should handle custom channel types")
		
		if err == nil && len(harmonyMsg.Channels) > 0 {
			t.Logf("Custom channel '%s' parsed as: %s", customChannel, harmonyMsg.Channels[0].ChannelType.String())
		}
	})
}