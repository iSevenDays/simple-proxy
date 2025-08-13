package test

import (
	"claude-proxy/correction"
	"claude-proxy/types"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHybridClassifierObservability tests that the hybrid classifier logging works correctly
func TestHybridClassifierObservability(t *testing.T) {
	fmt.Println("Testing Hybrid Classifier Observability...")
	
	// Create hybrid classifier
	classifier := correction.NewHybridClassifier()
	
	// Create a simple logger function that captures logs
	var logMessages []string
	logFunc := func(component, category, requestID, message string, fields map[string]interface{}) {
		logEntry := fmt.Sprintf("[%s][%s][%s] %s - %v", component, category, requestID, message, fields)
		logMessages = append(logMessages, logEntry)
		fmt.Println(logEntry)
	}
	
	// Test messages that should trigger different stages
	messages := []types.OpenAIMessage{
		{Role: "user", Content: "create a new file called test.go"},
	}
	
	// Call the hybrid classifier with logging
	decision := classifier.DetectToolNecessity(messages, logFunc, "test-request-123")
	
	fmt.Printf("\nFinal Decision: RequireTools=%v, Confident=%v, Reason=%s\n", 
		decision.RequireTools, decision.Confident, decision.Reason)
	
	// Verify we got observability logs
	stageALogs := 0
	stageBLogs := 0
	
	for _, log := range logMessages {
		if strings.Contains(log, "Stage A:") {
			stageALogs++
		}
		if strings.Contains(log, "Stage B:") {
			stageBLogs++
		}
	}
	
	// Verify we got the expected observability logs
	t.Logf("Observability Check:")
	t.Logf("- Stage A logs: %d", stageALogs)
	t.Logf("- Stage B logs: %d", stageBLogs)
	t.Logf("- Total log entries: %d", len(logMessages))
	
	// Assertions to verify observability is working
	assert.True(t, stageALogs > 0, "Should have Stage A logs")
	assert.True(t, stageBLogs > 0, "Should have Stage B logs")
	assert.True(t, decision.RequireTools, "Should require tools for file creation")
	assert.True(t, decision.Confident, "Should be confident about the decision")
	assert.Contains(t, decision.Reason, "create", "Reason should mention the create verb")
	
	// Verify specific log entries exist
	foundExtraction := false
	foundRuleEval := false
	for _, log := range logMessages {
		if strings.Contains(log, "Stage A: Action pair extraction complete") {
			foundExtraction = true
		}
		if strings.Contains(log, "Stage B: Rule engine evaluation complete") {
			foundRuleEval = true
		}
	}
	
	assert.True(t, foundExtraction, "Should log completion of action pair extraction")
	assert.True(t, foundRuleEval, "Should log completion of rule evaluation")
}