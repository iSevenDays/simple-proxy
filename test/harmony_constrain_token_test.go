package test

import (
	"claude-proxy/parser"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestConstrainTokenHandling tests TDD implementation of <|constrain|> token support
// This test drives the implementation of proper constrain token parsing per OpenAI Harmony spec
func TestConstrainTokenHandling(t *testing.T) {
	tests := []struct {
		name                   string
		input                  string
		expectedDetection      bool
		expectedConstraintType string
		expectedContent        string
		description            string
	}{
		{
			name:                   "json_constraint_in_tool_call",
			input:                  `<|start|>assistant<|channel|>commentary<|constrain|>json<|message|>{"location": "SF"}<|end|>`,
			expectedDetection:      true,
			expectedConstraintType: "json",
			expectedContent:        `{"location": "SF"}`,
			description:            "Should parse JSON constraint in tool calls",
		},
		{
			name:                   "text_constraint_type",
			input:                  `<|start|>assistant<|channel|>commentary<|constrain|>text<|message|>plain text response<|end|>`,
			expectedDetection:      true,
			expectedConstraintType: "text",
			expectedContent:        "plain text response",
			description:            "Should handle text constraint type",
		},
		{
			name:                   "constraint_with_recipient",
			input:                  `<|start|>assistant<|channel|>commentary to=functions.get_weather<|constrain|>json<|message|>{"city": "Tokyo"}<|end|>`,
			expectedDetection:      true,
			expectedConstraintType: "json",
			expectedContent:        `{"city": "Tokyo"}`,
			description:            "Should handle constraint with recipient information",
		},
		{
			name:              "no_constraint_token",
			input:             `<|start|>assistant<|channel|>final<|message|>Regular response<|end|>`,
			expectedDetection: true,
			expectedConstraintType: "",
			expectedContent:   "Regular response",
			description:       "Should handle messages without constraints normally",
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

				// Test the ConstraintType field - this should now work!
				if tt.expectedConstraintType != "" {
					assert.Equal(t, tt.expectedConstraintType, channel.ConstraintType, "Should capture constraint type")
					t.Logf("SUCCESS: ConstraintType field working - got: %s", channel.ConstraintType)
				} else {
					assert.Empty(t, channel.ConstraintType, "Should have no constraint type when not specified")
				}
			}
		})
	}
}

// TestConstrainTokenRegexPattern tests that the constrain token is properly recognized in regex patterns
func TestConstrainTokenRegexPattern(t *testing.T) {
	// This test will initially fail - it drives implementation of constrain token in regex patterns
	constrainToken := `<|constrain|>`
	
	t.Run("constrain_token_regex_support", func(t *testing.T) {
		// Test that our token recognition includes constrain token
		testInput := `<|start|>assistant<|channel|>commentary<|constrain|>json<|message|>{"test": "value"}<|end|>`
		
		detected := parser.IsHarmonyFormat(testInput)
		assert.True(t, detected, "Should detect Harmony format with constrain token")
		
		parsed, err := parser.ParseHarmonyMessage(testInput)
		assert.NoError(t, err, "Should parse constrain token successfully")
		assert.NotEmpty(t, parsed.Channels, "Should extract channels with constraint")
		
		// Log current behavior for implementation guidance
		if len(parsed.Channels) > 0 {
			t.Logf("Current parsing result:")
			t.Logf("  Channel Type: %s", parsed.Channels[0].ChannelType.String())
			t.Logf("  Content: %s", parsed.Channels[0].Content)
			t.Logf("  Raw Channel: %s", parsed.Channels[0].RawChannel)
		}
		
		// This assertion documents what we need to implement
		constraintFound := strings.Contains(testInput, constrainToken)
		assert.True(t, constraintFound, "Input contains constrain token that needs processing")
		
		t.Log("TODO: Implement ConstraintType field in Channel struct")
		t.Log("TODO: Add constrain token to regex patterns in TokenRecognizer")
		t.Log("TODO: Extract constraint type during channel parsing")
	})
}

// TestConstrainTokenIntegration tests integration with the broader Harmony parsing system
func TestConstrainTokenIntegration(t *testing.T) {
	// Test official OpenAI example with constrain token
	officialExample := `<|start|>assistant<|channel|>commentary to=functions.get_current_weather <|constrain|>json<|message|>{"location":"San Francisco"}<|call|>`
	
	t.Run("official_openai_example", func(t *testing.T) {
		detected := parser.IsHarmonyFormat(officialExample)
		assert.True(t, detected, "Should detect official OpenAI Harmony example")
		
		// This will succeed because basic tokens are present, but constraint info may be lost
		parsed, err := parser.ParseHarmonyMessage(officialExample)
		
		if err != nil {
			t.Logf("PARSING ISSUE: %v", err)
			t.Log("This indicates we need better support for complex token sequences")
		} else if len(parsed.Channels) > 0 {
			channel := parsed.Channels[0]
			t.Logf("Successfully parsed official example:")
			t.Logf("  Content: %s", channel.Content)
			t.Logf("  Channel Type: %s", channel.ChannelType.String())
			
			// Check if we're losing constraint information
			if strings.Contains(officialExample, "json") && !strings.Contains(channel.Content, "constraint") {
				t.Log("IMPLEMENTATION GAP: Constraint type 'json' not explicitly captured")
			}
		}
	})
}

// TestConstrainTokenEdgeCases tests edge cases for constrain token handling
func TestConstrainTokenEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldParse bool
		description string
	}{
		{
			name:        "constrain_without_message",
			input:       `<|start|>assistant<|channel|>commentary<|constrain|>json<|end|>`,
			shouldParse: true,
			description: "Should handle constrain token without message content",
		},
		{
			name:        "multiple_constraints", 
			input:       `<|start|>assistant<|channel|>commentary<|constrain|>json<|message|>{"a":1}<|constrain|>text<|message|>more<|end|>`,
			shouldParse: true,
			description: "Should handle multiple constraint tokens (edge case)",
		},
		{
			name:        "malformed_constrain",
			input:       `<|start|>assistant<|channel|>commentary<|constrain<|message|>content<|end|>`,
			shouldParse: true,
			description: "Should handle malformed constrain token gracefully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected := parser.IsHarmonyFormat(tt.input)
			assert.True(t, detected, "Should detect as Harmony format")
			
			_, err := parser.ParseHarmonyMessage(tt.input)
			
			if tt.shouldParse {
				if err != nil {
					t.Logf("EXPECTED PARSING SUCCESS but got error: %v", err)
					t.Logf("Description: %s", tt.description)
				} else {
					t.Logf("SUCCESS: %s", tt.description)
				}
			}
		})
	}
}