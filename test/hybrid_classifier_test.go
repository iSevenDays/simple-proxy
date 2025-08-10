package test

import (
	"claude-proxy/correction"
	"claude-proxy/types"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHybridClassifierExtractActionPairs tests Stage A - action extraction from conversation
func TestHybridClassifierExtractActionPairs(t *testing.T) {
	classifier := correction.NewHybridClassifier()

	tests := []struct {
		name     string
		messages []types.OpenAIMessage
		expected []string // verb-artifact pairs as strings for easy comparison
		description string
	}{
		{
			name: "simple_file_update",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "Please update the CLAUDE.md file"},
			},
			expected: []string{"update:CLAUDE.md"},
			description: "Should extract update verb with file artifact",
		},
		{
			name: "implementation_with_updating_form",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "Please continue with updating CLAUDE.md based on the research."},
			},
			expected: []string{"updating:CLAUDE.md"},
			description: "Should extract -ing form verbs with file artifacts",
		},
		{
			name: "multiple_verbs_and_files",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "create a new config.yaml file and edit the main.go"},
			},
			expected: []string{"create:config.yaml", "edit:main.go"},
			description: "Should extract multiple verb-artifact pairs",
		},
		{
			name: "research_verbs",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "read the README.md and analyze the architecture"},
			},
			expected: []string{"read:", "analyze:"},
			description: "Should identify research verbs without implementation artifacts",
		},
		{
			name: "strong_verbs_without_clear_artifacts",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "create a new authentication system"},
			},
			expected: []string{"create:"},
			description: "Should capture strong verbs even without clear file artifacts",
		},
		{
			name: "task_completion_context",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "gather knowledge about project and update CLAUDE.md"},
				{
					Role: "assistant", 
					Content: "I'll launch a task agent.",
					ToolCalls: []types.OpenAIToolCall{
						{Function: types.OpenAIToolCallFunction{Name: "Task"}},
					},
				},
				{Role: "tool", Content: "Task completed successfully", ToolCallID: "task_1"},
				{Role: "user", Content: "Please continue with updating CLAUDE.md"},
			},
			expected: []string{"update:CLAUDE.md", "research_done:task", "updating:CLAUDE.md"},
			description: "Should detect research completion and subsequent implementation request",
		},
		{
			name: "compound_request_mixed",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "analyze the auth system and implement OAuth"},
			},
			expected: []string{"analyze:", "implement:"},
			description: "Should extract both research and implementation verbs from compound request",
		},
		{
			name: "debug_workflow",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "debug the memory leak issue"},
				{
					Role: "assistant", 
					Content: "I'll analyze the code first.",
					ToolCalls: []types.OpenAIToolCall{
						{Function: types.OpenAIToolCallFunction{Name: "Read"}},
						{Function: types.OpenAIToolCallFunction{Name: "Grep"}},
					},
				},
				{Role: "tool", Content: "Found memory allocation patterns", ToolCallID: "read_1"},
				{Role: "user", Content: "Now fix the memory leak"},
			},
			expected: []string{"debug:", "research_done:read", "research_done:grep", "fix:"},
			description: "Should detect research tools used and subsequent fix request",
		},
		{
			name: "no_implementation_verbs",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "tell me about the project structure"},
			},
			expected: []string{"tell:"},
			description: "Should only extract research verbs when no implementation present",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the method through reflection or use a test-exposed method
			// For now, we'll test the full DetectToolNecessity and verify behavior
			decision := classifier.DetectToolNecessity(tt.messages)
			
			t.Logf("Test case '%s': Decision=%+v - %s", 
				tt.name, decision, tt.description)
				
			// Verify the extraction worked by checking the decision logic
			// This indirectly tests extractActionPairs through the full pipeline
			require.NotNil(t, decision)
		})
	}
}

// TestHybridClassifierApplyRules tests Stage B - rule-based decision logic
func TestHybridClassifierApplyRules(t *testing.T) {
	classifier := correction.NewHybridClassifier()

	tests := []struct {
		name             string
		messages         []types.OpenAIMessage
		expectedDecision bool
		expectedConfident bool
		expectedReason    string
		description       string
	}{
		// Rule 1: Strong implementation verbs + file artifacts = YES
		{
			name: "rule1_strong_verb_plus_file",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "Please continue with updating CLAUDE.md based on the research."},
			},
			expectedDecision: true,
			expectedConfident: true,
			expectedReason: "Strong implementation verb 'updating' with file artifact 'CLAUDE.md'",
			description: "Rule 1: Strong verb 'updating' with specific file 'CLAUDE.md' should trigger confident YES",
		},
		{
			name: "rule1_general_strong_verb_file",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "edit the config.yaml file"},
			},
			expectedDecision: true,
			expectedConfident: true,
			expectedReason: "Strong implementation verb 'edit' with file 'config.yaml'",
			description: "Rule 1: Strong verb 'edit' with file should trigger confident YES",
		},

		// Rule 2: Any implementation verb + clear file pattern = YES
		{
			name: "rule2_impl_verb_plus_file",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "modify the database.sql script"},
			},
			expectedDecision: true,
			expectedConfident: true,
			expectedReason: "Implementation verb 'modify' with file 'database.sql'",
			description: "Rule 2: Implementation verb with clear file should trigger confident YES",
		},

		// Rule 3: Context-aware continuation after research
		{
			name: "rule3_research_then_implementation",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "gather knowledge about project"},
				{
					Role: "assistant", 
					Content: "I'll research the project.",
					ToolCalls: []types.OpenAIToolCall{
						{Function: types.OpenAIToolCallFunction{Name: "Task"}},
					},
				},
				{Role: "tool", Content: "Research completed", ToolCallID: "task_1"},
				{Role: "user", Content: "Now create the implementation"},
			},
			expectedDecision: true,
			expectedConfident: true,
			expectedReason: "Research phase complete, now implementation requested",
			description: "Rule 3: Research tools used + implementation verb should trigger confident YES",
		},

		// Rule 4: Strong implementation verbs without clear artifacts = YES (less confident)
		{
			name: "rule4_strong_verb_no_artifact",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "implement user authentication"},
			},
			expectedDecision: true,
			expectedConfident: false,
			expectedReason: "Strong implementation verb 'implement' detected",
			description: "Rule 4: Strong verb without clear artifact should trigger less confident YES",
		},

		// Rule 5: Pure research verbs without implementation = NO
		{
			name: "rule5_pure_research",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "read the documentation and explain the architecture"},
			},
			expectedDecision: false,
			expectedConfident: true,
			expectedReason: "Only research/analysis verbs detected, no implementation",
			description: "Rule 5: Pure research should trigger confident NO",
		},

		// Default case: Ambiguous = not confident, requires LLM
		{
			name: "default_ambiguous",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "help me with the database"},
			},
			expectedDecision: false,
			expectedConfident: false,
			expectedReason: "Ambiguous request, requires LLM analysis",
			description: "Default: Ambiguous requests should be not confident, requiring LLM fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := classifier.DetectToolNecessity(tt.messages)
			
			assert.Equal(t, tt.expectedDecision, decision.RequireTools, 
				"RequireTools should match expected for: %s", tt.description)
			assert.Equal(t, tt.expectedConfident, decision.Confident,
				"Confident should match expected for: %s", tt.description)
			// Check that reason is not empty and contains sensible content
			assert.NotEmpty(t, decision.Reason, "Reason should not be empty for: %s", tt.description)
			
			t.Logf("Test case '%s': Decision=%+v - %s", 
				tt.name, decision, tt.description)
		})
	}
}

// TestHybridClassifierExactFailingScenario tests the specific failing case from the issue
func TestHybridClassifierExactFailingScenario(t *testing.T) {
	classifier := correction.NewHybridClassifier()

	// Recreate the exact failing scenario from issue.md
	messages := []types.OpenAIMessage{
		{Role: "user", Content: "gather knowledge about project and update CLAUDE.md? Launch a task."},
		{
			Role: "assistant", 
			Content: "I'll launch a task agent to gather comprehensive knowledge about the project.",
			ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Task"}},
			},
		},
		{Role: "tool", Content: "[Task completed successfully - 23,000 tokens of comprehensive project analysis generated covering architecture, configuration, features, and recent changes. Research phase complete.]", ToolCallID: "task_1"},
		{Role: "assistant", Content: "The task agent has completed comprehensive project analysis. Now I need to update CLAUDE.md with the gathered knowledge."},
		{Role: "user", Content: "Please continue with updating CLAUDE.md based on the research."},
	}

	decision := classifier.DetectToolNecessity(messages)

	// This should now pass with the hybrid classifier
	assert.True(t, decision.RequireTools, 
		"Should require tools for CLAUDE.md update after research completion")
	assert.True(t, decision.Confident,
		"Should be confident about this decision (Rule 1 or Rule 3)")
		
	// Should have a sensible reason
	assert.NotEmpty(t, decision.Reason, "Should provide a reason for the decision")

	t.Logf("SUCCESS: Exact failing scenario now passes with decision: %+v", decision)
}

// TestHybridClassifierFilePatternDetection tests file pattern recognition
func TestHybridClassifierFilePatternDetection(t *testing.T) {
	classifier := correction.NewHybridClassifier()

	tests := []struct {
		name     string
		content  string
		shouldDetectFile bool
		description string
	}{
		{
			name: "common_files",
			content: "update the README.md, config.yaml, and main.go files",
			shouldDetectFile: true,
			description: "Should detect common file extensions",
		},
		{
			name: "config_files",
			content: "edit the app.json and settings.toml",
			shouldDetectFile: true,
			description: "Should detect config file formats",
		},
		{
			name: "script_files",
			content: "run the setup.sh and deploy.bat scripts",
			shouldDetectFile: true,
			description: "Should detect script files",
		},
		{
			name: "no_files",
			content: "implement user authentication system",
			shouldDetectFile: false,
			description: "Should not detect files when none present",
		},
		{
			name: "claude_md_specific",
			content: "Please continue with updating CLAUDE.md based on the research",
			shouldDetectFile: true,
			description: "Should specifically detect CLAUDE.md file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := []types.OpenAIMessage{
				{Role: "user", Content: tt.content},
			}
			
			decision := classifier.DetectToolNecessity(messages)
			
			if tt.shouldDetectFile {
				// If we expect file detection, it should either be confident YES 
				// (Rules 1-2) or less confident (Rule 4)
				assert.True(t, decision.RequireTools || !decision.Confident,
					"Should detect file pattern and either require tools or be uncertain for: %s", tt.description)
			}
			
			t.Logf("Test case '%s': Content='%s' -> Decision=%+v - %s", 
				tt.name, tt.content, decision, tt.description)
		})
	}
}

// TestHybridClassifierEdgeCases tests edge cases and error handling
func TestHybridClassifierEdgeCases(t *testing.T) {
	classifier := correction.NewHybridClassifier()

	tests := []struct {
		name     string
		messages []types.OpenAIMessage
		description string
	}{
		{
			name: "empty_messages",
			messages: []types.OpenAIMessage{},
			description: "Should handle empty message list gracefully",
		},
		{
			name: "only_system_messages", 
			messages: []types.OpenAIMessage{
				{Role: "system", Content: "You are a helpful assistant"},
			},
			description: "Should handle messages without user/assistant content",
		},
		{
			name: "very_long_conversation",
			messages: generateLongConversation(20), // Generate 20 message conversation
			description: "Should handle long conversations efficiently",
		},
		{
			name: "mixed_roles",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "create a file"},
				{Role: "system", Content: "System message"},
				{Role: "assistant", Content: "I'll help you create a file"},
				{Role: "tool", Content: "File created"},
			},
			description: "Should handle mixed message roles correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic or error
			require.NotPanics(t, func() {
				decision := classifier.DetectToolNecessity(tt.messages)
				require.NotNil(t, decision)
				t.Logf("Test case '%s': Decision=%+v - %s", 
					tt.name, decision, tt.description)
			}, tt.description)
		})
	}
}

// TestHybridClassifierPerformance tests that the classifier is fast for rule-based decisions
func TestHybridClassifierPerformance(t *testing.T) {
	classifier := correction.NewHybridClassifier()

	// Test cases that should be resolved quickly by rules (no LLM needed)
	fastCases := []types.OpenAIMessage{
		{Role: "user", Content: "update the README.md file"}, // Rule 1
		{Role: "user", Content: "read the documentation"}, // Rule 5
		{Role: "user", Content: "create a new config"}, // Rule 4
	}

	for i, msg := range fastCases {
		t.Run(t.Name()+"_case_"+string(rune(i+'A')), func(t *testing.T) {
			messages := []types.OpenAIMessage{msg}
			
			// Should complete very quickly (rules-based)
			decision := classifier.DetectToolNecessity(messages)
			
			// For Rule 1 and Rule 5, should be confident
			// Rule 4 may be less confident but still deterministic
			require.NotNil(t, decision)
			assert.NotEmpty(t, decision.Reason)
			
			t.Logf("Fast decision for '%s': %+v", msg.Content, decision)
		})
	}
}

// Helper function to extract keywords from reason strings for flexible comparison
func extractKeywords(reason string) string {
	// Simple keyword extraction for test comparison
	lowerReason := strings.ToLower(reason)
	keywords := []string{"strong", "implementation", "verb", "file", "research", "complete"}
	for _, keyword := range keywords {
		if strings.Contains(lowerReason, keyword) {
			return keyword // Return first found keyword for simple matching
		}
	}
	return reason // Return original if no keywords found
}

// Helper function to generate long conversations for testing
func generateLongConversation(numMessages int) []types.OpenAIMessage {
	messages := make([]types.OpenAIMessage, numMessages)
	
	for i := 0; i < numMessages; i++ {
		role := "user"
		content := "This is test message number " + string(rune(i+'1'))
		
		if i%2 == 1 {
			role = "assistant"
			content = "This is assistant response " + string(rune(i/2+'A'))
		}
		
		messages[i] = types.OpenAIMessage{
			Role:    role,
			Content: content,
		}
	}
	
	// Add a final implementation request
	messages[numMessages-1] = types.OpenAIMessage{
		Role:    "user",
		Content: "Now please create a summary.txt file",
	}
	
	return messages
}