package test

import (
	"claude-proxy/correction"
	"claude-proxy/types"
	"testing"
)

// TestSystemMessageContamination demonstrates that system messages should NOT influence
// hybrid classifier decisions. This test should FAIL initially, proving the bug exists.
func TestSystemMessageContamination(t *testing.T) {
	classifier := correction.NewHybridClassifier()

	// Create messages where:
	// 1. System message contains implementation verbs ("update", "index.md") that would trigger tools
	// 2. User message contains only research/informational request that should NOT require tools
	messages := []types.OpenAIMessage{
		{
			Role: "system",
			Content: `You are Claude Code. You help users with software development.
			When the user asks to update files like index.md or README.md, use the appropriate tools.
			Always update documentation files when requested. Create and update files as needed.`,
		},
		{
			Role: "user", 
			Content: "Can you explain what a hybrid classifier is and how it works conceptually?",
		},
	}

	// Mock logger function to capture logs for debugging
	var logEntries []map[string]interface{}
	logFunc := func(component, category, requestID, message string, fields map[string]interface{}) {
		logEntry := make(map[string]interface{})
		logEntry["component"] = component
		logEntry["category"] = category
		logEntry["request_id"] = requestID
		logEntry["message"] = message
		for k, v := range fields {
			logEntry[k] = v
		}
		logEntries = append(logEntries, logEntry)
	}

	// Execute the hybrid classifier
	decision := classifier.DetectToolNecessity(messages, logFunc, "test-system-contamination")

	// EXPECTED BEHAVIOR: Pure research question should NOT require tools
	// The system message should be IGNORED completely
	if decision.RequireTools {
		// Print debug information to understand what triggered tools
		t.Logf("=== DEBUG: System message contamination detected ===")
		t.Logf("Decision: RequireTools=%v, Confident=%v, Reason=%s", 
			decision.RequireTools, decision.Confident, decision.Reason)
			
		// Print action pairs that were extracted
		for _, entry := range logEntries {
			if stage, exists := entry["stage"]; exists && stage == "A_extract_pairs" {
				if pairs, exists := entry["pairs"]; exists {
					t.Logf("Action pairs extracted: %+v", pairs)
				}
			}
		}
		
		// Print rule evaluations
		for _, entry := range logEntries {
			if stage, exists := entry["stage"]; exists && stage == "B_rule_engine" {
				if ruleName, exists := entry["rule_name"]; exists {
					t.Logf("Rule %s: matched=%v", ruleName, entry["matched"])
				}
			}
		}

		t.Errorf("SYSTEM MESSAGE CONTAMINATION BUG: System message influenced tool necessity decision!")
		t.Errorf("User asked pure research question but classifier decided RequireTools=true")
		t.Errorf("This indicates system messages are being analyzed when they should be ignored")
		t.Errorf("Expected: RequireTools=false (research question)")
		t.Errorf("Actual: RequireTools=true (contaminated by system message)")
	}

	// Additional validation: Ensure the reason doesn't reference system message content
	if decision.RequireTools && (
		containsSystemContent(decision.Reason, "update") || 
		containsSystemContent(decision.Reason, "index.md") || 
		containsSystemContent(decision.Reason, "README.md")) {
		t.Errorf("CONTAMINATION DETECTED: Decision reason contains system message content: %s", decision.Reason)
	}
}

// Helper function to check if string contains substring (case-insensitive)
func containsSystemContent(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
		    len(s) > len(substr) && 
		    (s[:len(substr)] == substr || 
		     s[len(s)-len(substr):] == substr || 
		     containsSystemMiddle(s, substr)))
}

func containsSystemMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestSystemMessageShouldBeIgnored verifies that identical user messages 
// produce identical results regardless of system message content
func TestSystemMessageShouldBeIgnored(t *testing.T) {
	classifier := correction.NewHybridClassifier()

	// User message that should NOT require tools
	userMessage := types.OpenAIMessage{
		Role:    "user",
		Content: "What are the benefits of using a circuit breaker pattern?",
	}

	// Test 1: No system message
	messages1 := []types.OpenAIMessage{userMessage}
	decision1 := classifier.DetectToolNecessity(messages1, nil, "test-no-system")

	// Test 2: System message with implementation verbs
	messages2 := []types.OpenAIMessage{
		{
			Role: "system",
			Content: "Create new files, update existing code, edit documentation, write tests, and implement features as needed.",
		},
		userMessage,
	}
	decision2 := classifier.DetectToolNecessity(messages2, nil, "test-with-system")

	// Results should be IDENTICAL regardless of system message
	if decision1.RequireTools != decision2.RequireTools {
		t.Errorf("System message contamination detected!")
		t.Errorf("Same user message produced different results based on system message:")
		t.Errorf("Without system: RequireTools=%v, Reason=%s", decision1.RequireTools, decision1.Reason)
		t.Errorf("With system: RequireTools=%v, Reason=%s", decision2.RequireTools, decision2.Reason)
	}

	if decision1.Confident != decision2.Confident {
		t.Errorf("System message affected confidence level:")
		t.Errorf("Without system: Confident=%v", decision1.Confident)
		t.Errorf("With system: Confident=%v", decision2.Confident)
	}
}