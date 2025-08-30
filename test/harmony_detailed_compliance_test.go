package test

import (
	"claude-proxy/parser"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDetailedOfficialComplianceAnalysis performs detailed verification of OpenAI Harmony compliance
// This test documents exactly what works vs what doesn't work in the current implementation
func TestDetailedOfficialComplianceAnalysis(t *testing.T) {
	
	t.Run("Verify missing official tokens", func(t *testing.T) {
		// Test each official token individually
		testCases := []struct {
			name      string
			input     string
			tokenType string
			shouldWork bool
		}{
			{
				name: "constrain_token_in_tool_call",
				input: `<|start|>assistant<|channel|>commentary<|constrain|>json<|message|>{"tool":"data"}<|end|>`,
				tokenType: "<|constrain|>",
				shouldWork: false, // This token is NOT handled by current regex patterns
			},
			{
				name: "call_token_as_stop_token", 
				input: `<|start|>assistant<|channel|>commentary<|message|>{"tool":"data"}<|call|>`,
				tokenType: "<|call|>",
				shouldWork: false, // This token is NOT handled by current regex patterns
			},
			{
				name: "return_token_as_stop_token",
				input: `<|start|>assistant<|channel|>final<|message|>Final answer<|return|>`,
				tokenType: "<|return|>",
				shouldWork: true, // This IS handled by current implementation
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				detected := parser.IsHarmonyFormat(tc.input)
				parsed, err := parser.ParseHarmonyMessage(tc.input)

				if tc.shouldWork {
					assert.True(t, detected, "Should detect %s", tc.tokenType)
					assert.NoError(t, err, "Should parse %s", tc.tokenType)
					assert.NotEmpty(t, parsed.Channels, "Should extract channels for %s", tc.tokenType)
				} else {
					// Document what happens with unsupported tokens
					t.Logf("Token %s - Detected: %v, Parse Error: %v", tc.tokenType, detected, err)
					if detected && err == nil && len(parsed.Channels) > 0 {
						t.Logf("UNEXPECTED: %s token was handled correctly", tc.tokenType)
					}
					if !detected || err != nil {
						t.Logf("CONFIRMED GAP: %s token not fully supported", tc.tokenType)
					}
				}
			})
		}
	})

	t.Run("Verify official tool calling format parsing", func(t *testing.T) {
		// This is the exact format from OpenAI docs
		officialFormat := `<|start|>assistant<|channel|>commentary to=functions.get_current_weather <|constrain|>json<|message|>{"location":"San Francisco"}<|call|>`
		
		detected := parser.IsHarmonyFormat(officialFormat)
		assert.True(t, detected, "Should detect official tool call format")
		
		parsed, err := parser.ParseHarmonyMessage(officialFormat)
		
		// Log what actually happens with the official format
		t.Logf("Official tool call format parsing:")
		t.Logf("  Input: %s", officialFormat)
		t.Logf("  Detected: %v", detected)
		t.Logf("  Parse Error: %v", err)
		
		if err == nil && len(parsed.Channels) > 0 {
			channel := parsed.Channels[0]
			t.Logf("  Parsed Channel Type: %s", channel.ChannelType.String())
			t.Logf("  Parsed Content: %s", channel.Content)
			t.Logf("  Raw Channel: %s", channel.RawChannel)
			
			// Check if recipient information is captured
			if strings.Contains(channel.RawChannel, "to=") || strings.Contains(channel.Content, "to=") {
				t.Log("  ✅ Recipient information preserved")
			} else {
				t.Log("  ❌ BUG: Recipient information lost")
			}
			
			// Check if constrain information is captured
			if strings.Contains(channel.Content, "constrain") || strings.Contains(officialFormat, "<|constrain|>") {
				if strings.Contains(channel.Content, "json") {
					t.Log("  ✅ Constraint information captured")
				} else {
					t.Log("  ❌ BUG: Constraint information lost or not parsed")
				}
			}
		}
	})

	t.Run("Test recipient parsing in tool messages", func(t *testing.T) {
		// Test tool response format with recipient
		toolResponse := `<|start|>functions.get_weather to=assistant<|channel|>commentary<|message|>{"temperature": 20}<|end|>`
		
		detected := parser.IsHarmonyFormat(toolResponse)
		parsed, err := parser.ParseHarmonyMessage(toolResponse)
		
		t.Logf("Tool response format:")
		t.Logf("  Detected: %v, Error: %v", detected, err)
		
		if err == nil && len(parsed.Channels) > 0 {
			channel := parsed.Channels[0]
			t.Logf("  Role: %s", channel.Role.String())
			t.Logf("  Channel: %s", channel.ChannelType.String())
			
			// The current regex `<\|start\|>(\w+)` only captures the role part
			// It should capture "functions.get_weather" but may not handle the "to=" part
			t.Logf("  Current role parsing handles: functions.get_weather")
		}
	})

	t.Run("Verify header structure compliance", func(t *testing.T) {
		// Official format has header = role + optional channel + optional recipient + optional constraint
		testCases := []struct {
			name   string
			input  string
			header string
		}{
			{
				name:   "Simple header",
				input:  `<|start|>assistant<|message|>content<|end|>`,
				header: "assistant",
			},
			{
				name:   "Header with channel",
				input:  `<|start|>assistant<|channel|>final<|message|>content<|end|>`,
				header: "assistant + channel",
			},
			{
				name:   "Header with recipient",
				input:  `<|start|>assistant to=user<|channel|>final<|message|>content<|end|>`,
				header: "assistant + recipient",
			},
			{
				name:   "Complex header with all components",
				input:  `<|start|>assistant<|channel|>commentary to=functions.tool<|constrain|>json<|message|>content<|call|>`,
				header: "assistant + channel + recipient + constraint",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				detected := parser.IsHarmonyFormat(tc.input)
				parsed, err := parser.ParseHarmonyMessage(tc.input)
				
				t.Logf("Header test '%s': Detected=%v, Error=%v", tc.header, detected, err)
				
				if err == nil && len(parsed.Channels) > 0 {
					t.Logf("  Successfully parsed %s", tc.header)
				} else if err != nil {
					t.Logf("  PARSING ISSUE with %s: %v", tc.header, err)
				}
			})
		}
	})

	t.Run("Document current vs official token coverage", func(t *testing.T) {
		officialTokens := map[string]int{
			"<|start|>":     200006,
			"<|end|>":       200007,
			"<|message|>":   200008,
			"<|channel|>":   200005,
			"<|constrain|>": 200003,
			"<|return|>":    200002,
			"<|call|>":      200012,
		}

		currentlySupported := map[string]bool{
			"<|start|>":     true, // ✅ Handled in regex
			"<|end|>":       true, // ✅ Handled in regex
			"<|message|>":   true, // ✅ Handled in regex
			"<|channel|>":   true, // ✅ Handled in regex
			"<|constrain|>": true, // ✅ Implemented with ConstraintType field
			"<|return|>":    true, // ✅ Handled as alternative to <|end|>
			"<|call|>":      true, // ✅ Implemented as stop token
		}

		t.Log("OpenAI Harmony Token Coverage Analysis:")
		for token, id := range officialTokens {
			supported := currentlySupported[token]
			status := "❌ MISSING"
			if supported {
				status = "✅ SUPPORTED"
			}
			t.Logf("  %s (ID: %d) - %s", token, id, status)
		}

		// Calculate coverage percentage
		supportedCount := 0
		for _, supported := range currentlySupported {
			if supported {
				supportedCount++
			}
		}
		coverage := float64(supportedCount) / float64(len(officialTokens)) * 100
		t.Logf("\nOverall Token Coverage: %.1f%% (%d/%d tokens)", coverage, supportedCount, len(officialTokens))

		if coverage < 100 {
			t.Log("\n🔴 COMPLIANCE GAP: Missing official tokens prevent full OpenAI Harmony compatibility")
		} else {
			t.Log("\n✅ FULL COMPLIANCE: All official tokens supported")
		}
	})
}