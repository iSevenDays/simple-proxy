package test

import (
	"claude-proxy/correction"
	"claude-proxy/internal"
	"claude-proxy/types"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSimpleRealLLM tests that we can use real LLM endpoints for validation
func TestSimpleRealLLM(t *testing.T) {
	// Use real LLM endpoint from environment
	cfg := NewMockConfigProvider()
	service := correction.NewService(cfg, cfg.ToolCorrectionAPIKey, true, cfg.CorrectionModel, false, nil)
	ctx := internal.WithRequestID(context.Background(), "test-simple-real-llm")

	// Test a simple completion case that should be blocked
	completionToolCall := types.Content{
		Type: "tool_use",
		Name: "ExitPlanMode",
		Input: map[string]interface{}{
			"plan": "✅ All tasks completed successfully! The implementation is finished and ready for production.",
		},
	}

	messages := []types.OpenAIMessage{
		{Role: "user", Content: "Implement user authentication"},
		{Role: "assistant", Content: "I'll implement user authentication for you."},
	}

	shouldBlock, reason := service.ValidateExitPlanMode(ctx, completionToolCall, messages)
	
	t.Logf("Real LLM response: shouldBlock=%v, reason=%s", shouldBlock, reason)
	
	// The real LLM should detect this as a completion summary and block it
	assert.True(t, shouldBlock, "Real LLM should block completion summaries")
	assert.Contains(t, reason, "inappropriate usage detected by LLM analysis", "Should have the expected reason format")
}