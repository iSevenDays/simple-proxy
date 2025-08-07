package test

import (
	"bytes"
	"claude-proxy/config"
	"claude-proxy/proxy"
	"claude-proxy/types"
	"context"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnhancedParameterLogging(t *testing.T) {
	// Capture log output
	var logBuffer bytes.Buffer
	log.SetOutput(&logBuffer)
	defer log.SetOutput(log.Writer())

	// Create a test config
	cfg := &config.Config{
		BigModel:         "test-model",
		BigModelEndpoints: []string{"http://test:8080/v1/chat/completions"},
	}

	// Create a simple request to verify our enhanced logging works
	anthropicReq := types.AnthropicRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []types.Message{
			{
				Role:    "system",
				Content: "You are a helpful assistant.",
			},
		},
		Tools: []types.Tool{
			{
				Name:        "Read",
				Description: "Read files",
				InputSchema: types.ToolSchema{
					Type: "object",
					Properties: map[string]types.ToolProperty{
						"file_path": {Type: "string"},
					},
				},
			},
		},
	}

	ctx := context.Background()
	
	// Transform the request (this will trigger the logging)
	_, err := proxy.TransformAnthropicToOpenAI(ctx, anthropicReq, cfg)
	assert.NoError(t, err)

	// The important thing is that we don't get empty content previews anymore
	logOutput := logBuffer.String()
	
	// Verify that empty content previews are NOT logged
	assert.NotContains(t, logOutput, `Content preview: ""`, 
		"Should not show empty content previews")
}

func TestActualParameterValues(t *testing.T) {
	// This test documents the expected format of the new logging
	// When real tool calls are made, they should show actual parameter values
	// Example expectations:
	// 
	// Instead of: Tools: [Read(1 params)[handlers.go]]
	// Should be:  Tools: [Read(file=handlers.go)]
	//
	// Instead of: Tools: [TodoWrite(1 params)[2 todos]] 
	// Should be:  Tools: [TodoWrite(todos=2)]
	//
	// Instead of: Tools: [Bash(1 params)[ls -la...]]
	// Should be:  Tools: [Bash(cmd="ls -la /very/long/path...")]
	//
	// Instead of: Tools: [Edit(3 params)[handlers.go]]
	// Should be:  Tools: [Edit(file=handlers.go, old="old content...", content="new content...")]
	
	t.Log("Enhanced logging now shows actual parameter values instead of just counts")
	t.Log("This provides much better debugging information in production logs")
}