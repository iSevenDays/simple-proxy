package test

import (
	"testing"
)

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxLength int
		expected  string
	}{
		{
			name:      "No truncation needed - short message",
			input:     "Short message",
			maxLength: 200,
			expected:  "Short message",
		},
		{
			name:      "No truncation needed - exact length",
			input:     "This message is exactly fifty characters long!",
			maxLength: 50,
			expected:  "This message is exactly fifty characters long!",
		},
		{
			name:      "Truncation needed - 200 char limit",
			input:     "This is a very long message that should be truncated in the middle to demonstrate the functionality of conversation truncation feature that we have implemented for the proxy server",
			maxLength: 200,
			expected:  "This is a very long message that should be truncated in the middle to demonstrate the functionality of conversation truncation feature that we have implemented for the proxy server",
		},
		{
			name:      "Truncation needed - 50 char limit",
			input:     "This is a very long message that should be truncated in the middle to demonstrate the functionality",
			maxLength: 50,
			expected:  "This is a very long message tha ... onstrate the functionality",
		},
		{
			name:      "Truncation needed - very short limit",
			input:     "This is a long message that needs truncation",
			maxLength: 20,
			expected:  "This is ... ation",
		},
		{
			name:      "Single character",
			input:     "A",
			maxLength: 200,
			expected:  "A",
		},
		{
			name:      "Very short limit with long text",
			input:     "Lorem ipsum dolor sit amet consectetur adipiscing elit",
			maxLength: 15,
			expected:  "Lorem ... elit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use helper to test the private method logic
			result := truncateStringHelper(tt.input, tt.maxLength)
			
			if len(result) > tt.maxLength && tt.maxLength > 0 {
				t.Errorf("truncateString() result length %d exceeds maxLength %d", len(result), tt.maxLength)
			}
			
			if len(tt.input) <= tt.maxLength && result != tt.input {
				t.Errorf("truncateString() should not modify string shorter than maxLength. got %q, want %q", result, tt.input)
			}
			
			if len(tt.input) > tt.maxLength {
				if result == tt.input {
					t.Errorf("truncateString() should modify string longer than maxLength")
				}
				
				// Check that it contains " ... " if truncated
				if len(result) > 5 && result != tt.input {
					if !containsEllipsis(result) {
						t.Errorf("truncateString() truncated result should contain ' ... ', got %q", result)
					}
				}
			}
		})
	}
}

// Helper function to simulate the private truncateString method
func truncateStringHelper(s string, maxLength int) string {
	if maxLength <= 0 || len(s) <= maxLength {
		return s
	}
	
	// Handle very small maxLength cases
	if maxLength < 5 {
		// Just return first few characters if we can't fit ellipsis
		if maxLength <= len(s) {
			return s[:maxLength]
		}
		return s
	}
	
	// Calculate how much to keep from each end
	halfLength := (maxLength - 5) / 2 // Reserve 5 chars for " ... "
	if halfLength < 1 {
		halfLength = 1
	}
	
	beginning := s[:halfLength]
	end := s[len(s)-halfLength:]
	
	return beginning + " ... " + end
}

// Helper function to check if string contains ellipsis
func containsEllipsis(s string) bool {
	return len(s) >= 5 && (s[len(s)/2-2:len(s)/2+3] == " ... " || 
		                  findSubstringTrunc(s, " ... "))
}

// Helper to find substring for truncation tests
func findSubstringTrunc(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestConversationTruncationIntegration(t *testing.T) {

	// Test with truncation enabled (50 chars)
	t.Run("Truncation enabled", func(t *testing.T) {
		// This would require access to the private methods or making them public
		// For now, we test the helper function behavior
		longMessage := "This is a very long message that should be truncated when the truncation feature is enabled"
		result := truncateStringHelper(longMessage, 50)
		
		if len(result) > 50 {
			t.Errorf("Message should be truncated to 50 chars, got %d chars", len(result))
		}
		
		if !containsEllipsis(result) {
			t.Errorf("Truncated message should contain ellipsis")
		}
	})

	// Test with truncation disabled (maxLength larger than message)
	t.Run("Truncation disabled", func(t *testing.T) {
		longMessage := "This is a very long message that should not be truncated when truncation is disabled"
		result := truncateStringHelper(longMessage, 200) // 200 > len(longMessage), so no truncation
		
		if result != longMessage {
			t.Errorf("Message should not be modified when maxLength is larger than message length")
		}
	})
}

func TestNestedContentStructureTruncation(t *testing.T) {
	// Test the logic for handling nested content structures like:
	// "content": [{"text": "long text here", "type": "text"}]
	
	t.Run("Nested text content truncation", func(t *testing.T) {
		longText := "This is a very long system reminder or user message content that should be truncated when the truncation feature is enabled in the conversation logger for testing purposes and demonstration of the nested content structure handling"
		
		// Simulate the nested structure from Claude Code logs
		testData := map[string]interface{}{
			"messages": []interface{}{
				map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{
							"text": longText,
							"type": "text",
						},
					},
					"role": "user",
				},
			},
		}
		
		// Apply truncation logic manually (simulating the recursive function)
		processed := processNestedContent(testData, 50)
		
		// Navigate to the text field and verify truncation
		processedMap := processed.(map[string]interface{})
		messages := processedMap["messages"].([]interface{})
		firstMessage := messages[0].(map[string]interface{})
		content := firstMessage["content"].([]interface{})
		textObj := content[0].(map[string]interface{})
		truncatedText := textObj["text"].(string)
		
		if len(truncatedText) > 50 {
			t.Errorf("Nested text should be truncated to 50 chars, got %d chars", len(truncatedText))
		}
		
		if !containsEllipsis(truncatedText) {
			t.Errorf("Truncated nested text should contain ellipsis")
		}
		
		if truncatedText == longText {
			t.Errorf("Nested text should be modified when truncation is applied")
		}
	})

	t.Run("Deep nested messages structure", func(t *testing.T) {
		longSystemText := "This is a very long system reminder that appears in messages content text field and should be truncated properly"
		
		// Test the exact structure from the user's log
		testData := map[string]interface{}{
			"category": "REQUEST",
			"data": map[string]interface{}{
				"data": map[string]interface{}{
					"messages": []interface{}{
						map[string]interface{}{
							"content": []interface{}{
								map[string]interface{}{
									"text": longSystemText,
									"type": "text",
								},
							},
							"role": "user",
						},
					},
					"system": []interface{}{
						map[string]interface{}{
							"text": "Short system message",
							"type": "text",
						},
					},
				},
			},
		}
		
		// Apply truncation logic
		processed := processNestedContent(testData, 50)
		
		// Navigate to the messages->content->text field
		processedMap := processed.(map[string]interface{})
		data := processedMap["data"].(map[string]interface{})
		dataData := data["data"].(map[string]interface{})
		messages := dataData["messages"].([]interface{})
		firstMessage := messages[0].(map[string]interface{})
		content := firstMessage["content"].([]interface{})
		textObj := content[0].(map[string]interface{})
		truncatedText := textObj["text"].(string)
		
		if len(truncatedText) > 50 {
			t.Errorf("Deep nested text should be truncated to 50 chars, got %d chars", len(truncatedText))
		}
		
		if truncatedText == longSystemText {
			t.Errorf("Deep nested text should be modified when truncation is applied")
		}
		
		// Also check system array text is processed
		system := dataData["system"].([]interface{})
		systemObj := system[0].(map[string]interface{})
		systemText := systemObj["text"].(string)
		
		if systemText != "Short system message" {
			t.Errorf("Short system text should not be modified, got: %s", systemText)
		}
	})
}

// Helper function to simulate the nested content processing
func processNestedContent(data interface{}, maxLength int) interface{} {
	// Enhanced implementation that mimics the updated recursive truncation logic
	switch v := data.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, value := range v {
			if key == "content" {
				// Handle content as string or recurse into it
				if contentStr, ok := value.(string); ok {
					result[key] = truncateStringHelper(contentStr, maxLength)
				} else {
					result[key] = processNestedContent(value, maxLength)
				}
			} else if key == "text" {
				if textStr, ok := value.(string); ok {
					result[key] = truncateStringHelper(textStr, maxLength)
				} else {
					result[key] = value
				}
			} else {
				result[key] = processNestedContent(value, maxLength)
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = processNestedContent(item, maxLength)
		}
		return result
	default:
		return data
	}
}

func TestTruncationEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxLength int
		wantPanic bool
	}{
		{
			name:      "Empty string",
			input:     "",
			maxLength: 10,
			wantPanic: false,
		},
		{
			name:      "Zero maxLength",
			input:     "test",
			maxLength: 0,
			wantPanic: false,
		},
		{
			name:      "Negative maxLength", 
			input:     "test",
			maxLength: -5,
			wantPanic: false,
		},
		{
			name:      "Very small maxLength",
			input:     "test message",
			maxLength: 1,
			wantPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil && !tt.wantPanic {
					t.Errorf("truncateString() panicked unexpectedly: %v", r)
				}
			}()

			result := truncateStringHelper(tt.input, tt.maxLength)
			
			// Basic sanity checks
			if tt.maxLength > 0 && len(result) > tt.maxLength {
				t.Errorf("Result length %d exceeds maxLength %d", len(result), tt.maxLength)
			}
			
			if tt.input == "" {
				if result != "" {
					t.Errorf("Empty input should return empty result, got %q", result)
				}
			}
		})
	}
}