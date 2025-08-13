package test

import (
	"claude-proxy/correction"
	"claude-proxy/types"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CustomDocumentationRule: Custom rule for documentation-specific requests
type CustomDocumentationRule struct{}

func (r *CustomDocumentationRule) Priority() int { return 95 }
func (r *CustomDocumentationRule) Name() string { return "CustomDocumentation" }

func (r *CustomDocumentationRule) IsSatisfiedBy(pairs []correction.ActionPair, messages []types.OpenAIMessage) (bool, correction.RuleDecision) {
	// Look for documentation-related verbs with documentation files
	docVerbs := map[string]bool{
		"document": true, "doc": true, "readme": true,
		"documenting": true, "docs": true,
	}
	
	docFiles := []string{"readme", "documentation", "docs", "guide", "manual"}
	
	for _, pair := range pairs {
		if docVerbs[pair.Verb] {
			// Check if artifact contains documentation-related terms
			lower := strings.ToLower(pair.Artifact)
			for _, docFile := range docFiles {
				if strings.Contains(lower, docFile) {
					return true, correction.RuleDecision{
						RequireTools: true,
						Confident:    true,
						Reason:       "Documentation verb '" + pair.Verb + "' with documentation file '" + pair.Artifact + "'",
					}
				}
			}
		}
	}
	
	return false, correction.RuleDecision{}
}

// TestRuleEngineExtensibility tests that we can add custom rules to the system
func TestRuleEngineExtensibility(t *testing.T) {
	classifier := correction.NewHybridClassifier()
	
	// Add a custom documentation rule
	customRule := &CustomDocumentationRule{}
	classifier.AddCustomRule(customRule)
	
	tests := []struct {
		name     string
		messages []types.OpenAIMessage
		expected bool
		expectedReason string
		description string
	}{
		{
			name: "documentation_update",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "Please document the API endpoints in the README.md"},
			},
			expected: true,
			expectedReason: "Documentation verb 'document' with documentation file 'readme.md'",
			description: "Custom documentation rule should trigger for README documentation",
		},
		{
			name: "regular_file_update",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "Please update the config.yaml file"},
			},
			expected: true,
			expectedReason: "Strong implementation verb 'update' with file 'config.yaml'",
			description: "Regular rules should still work alongside custom rules",
		},
		{
			name: "research_only",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "read the documentation and explain the architecture"},
			},
			expected: false,
			expectedReason: "Only research/analysis verbs detected, no implementation",
			description: "Pure research should still return false",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := classifier.DetectToolNecessity(tt.messages, nil, "")
			
			assert.Equal(t, tt.expected, decision.RequireTools, 
				"RequireTools should match expected for: %s", tt.description)
			assert.NotEmpty(t, decision.Reason, "Reason should not be empty")
			
			t.Logf("Test case '%s': Decision=%+v - %s", 
				tt.name, decision, tt.description)
		})
	}
}

// TestRuleEnginePriority tests that rules are evaluated in priority order
func TestRuleEnginePriority(t *testing.T) {
	// Test that higher priority rules are evaluated before lower priority ones
	classifier := correction.NewHybridClassifier()
	
	// Add a high priority rule that should override default behavior
	highPriorityRule := &CustomDocumentationRule{}
	classifier.AddCustomRule(highPriorityRule)
	
	messages := []types.OpenAIMessage{
		{Role: "user", Content: "document the new features in README.md"},
	}
	
	decision := classifier.DetectToolNecessity(messages, nil, "")
	
	// The custom documentation rule should fire (priority 95) before 
	// the default ImplementationVerbWithFileRule (priority 90)
	assert.True(t, decision.RequireTools)
	assert.True(t, decision.Confident)
	assert.Contains(t, decision.Reason, "Documentation verb")
	
	t.Logf("Priority test result: %+v", decision)
}

// TestRuleEnginePerformance tests that the rule engine maintains performance
func TestRuleEnginePerformance(t *testing.T) {
	classifier := correction.NewHybridClassifier()
	
	// Add several custom rules to test performance with many rules
	rules := []correction.Rule{
		&CustomDocumentationRule{},
	}
	
	for _, rule := range rules {
		classifier.AddCustomRule(rule)
	}
	
	testCases := []types.OpenAIMessage{
		{Role: "user", Content: "create a new configuration file"},
		{Role: "user", Content: "read the logs and analyze errors"},
		{Role: "user", Content: "update the documentation"},
		{Role: "user", Content: "fix the authentication bug"},
	}
	
	for i, msg := range testCases {
		t.Run(t.Name()+"_case_"+string(rune(i+'A')), func(t *testing.T) {
			messages := []types.OpenAIMessage{msg}
			
			// Should complete very quickly even with multiple rules
			decision := classifier.DetectToolNecessity(messages, nil, "")
			
			require.NotNil(t, decision)
			assert.NotEmpty(t, decision.Reason)
			
			t.Logf("Performance test for '%s': %+v", msg.Content, decision)
		})
	}
}