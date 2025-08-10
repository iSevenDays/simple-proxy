package test

import (
	"claude-proxy/config"
	"claude-proxy/internal"
	"claude-proxy/proxy"
	"claude-proxy/types"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTestConfigWithRealLLM returns a config configured for testing with real LLM endpoint from .env
func getTestConfigWithRealLLM() *config.Config {
	return NewMockConfigProvider() // Use the shared helper
}

// TestContextAwareToolFiltering tests the root cause fix:
// ExitPlanMode should be automatically filtered out for research/analysis requests
func TestContextAwareToolFiltering(t *testing.T) {
	// Use real LLM endpoint from environment
	ctx := internal.WithRequestID(context.Background(), "test-context-filtering")
	cfg := getTestConfigWithRealLLM()

	tests := []struct {
		name                      string
		userMessage               string
		expectExitPlanModeSkipped bool
		description               string
	}{
		{
			name:                      "research_request_filters_exitplanmode",
			userMessage:               "read architecture md and check recent changes to tool correction related to exit plan mode",
			expectExitPlanModeSkipped: true,
			description:               "Research requests should filter out ExitPlanMode",
		},
		{
			name:                      "documentation_reading_filters_exitplanmode", 
			userMessage:               "show me the contents of the README file",
			expectExitPlanModeSkipped: true,
			description:               "Simple documentation reading should filter ExitPlanMode",
		},
		{
			name:                      "planning_analysis_keeps_exitplanmode",
			userMessage:               "help me plan the implementation of a new feature",
			expectExitPlanModeSkipped: false,
			description:               "Planning requests should keep ExitPlanMode available",
		},
		{
			name:                      "implementation_request_keeps_exitplanmode",
			userMessage:               "implement a new user authentication system with database integration",
			expectExitPlanModeSkipped: false,
			description:               "Implementation requests should keep ExitPlanMode available",
		},
		{
			name:                      "create_request_keeps_exitplanmode",
			userMessage:               "create a new API endpoint for user management",
			expectExitPlanModeSkipped: false,
			description:               "Creation requests should keep ExitPlanMode available",
		},
		{
			name:                      "mixed_request_research_dominant_filters",
			userMessage:               "read the current auth code, analyze the patterns, and then implement improvements",
			expectExitPlanModeSkipped: true,
			description:               "Mixed requests with dominant research should filter ExitPlanMode",
		},
		{
			name:                      "mixed_request_implementation_dominant_keeps",
			userMessage:               "implement user registration, create the database tables, and add validation",
			expectExitPlanModeSkipped: false,
			description:               "Mixed requests with dominant implementation should keep ExitPlanMode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create Anthropic request with ExitPlanMode in tools
			anthropicReq := types.AnthropicRequest{
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 1024,
				Messages: []types.Message{
					{
						Role: "user",
						Content: []types.Content{
							{
								Type: "text",
								Text: tt.userMessage,
							},
						},
					},
				},
				Tools: []types.Tool{
					{
						Name:        "Read",
						Description: "Read files",
						InputSchema: types.ToolSchema{Type: "object"},
					},
					{
						Name:        "ExitPlanMode",
						Description: "Use this tool when you are in plan mode and have finished presenting your plan and are ready to code. This will prompt the user to exit plan mode. \nIMPORTANT: Only use this tool when the task requires planning the implementation steps of a task that requires writing code. For research tasks where you're gathering information, searching files, reading files or in general trying to understand the codebase - do NOT use this tool.\n\nEg. \n1. Initial task: \"Search for and understand the implementation of vim mode in the codebase\" - Do not use the exit plan mode tool because you are not planning the implementation steps of a task.\n2. Initial task: \"Help me implement yank mode for vim\" - Use the exit plan mode tool after you have finished planning the implementation steps of the task.\n",
						InputSchema: types.ToolSchema{
							Type: "object",
							Properties: map[string]types.ToolProperty{
								"plan": {
									Type:        "string",
									Description: "The plan you came up with, that you want to run by the user for approval. Supports markdown. The plan should be pretty concise.",
								},
							},
							Required: []string{"plan"},
						},
					},
					{
						Name:        "Write",
						Description: "Write files",
						InputSchema: types.ToolSchema{Type: "object"},
					},
				},
			}

			// Transform the request
			openaiReq, err := proxy.TransformAnthropicToOpenAI(ctx, anthropicReq, cfg)
			require.NoError(t, err)

			// Check if ExitPlanMode was filtered out
			hasExitPlanMode := false
			for _, tool := range openaiReq.Tools {
				if tool.Function.Name == "ExitPlanMode" {
					hasExitPlanMode = true
					break
				}
			}

			if tt.expectExitPlanModeSkipped {
				assert.False(t, hasExitPlanMode,
					"Test case %s: ExitPlanMode should be filtered out for %s. %s",
					tt.name, tt.userMessage, tt.description)

				// Verify other tools are still available
				assert.True(t, len(openaiReq.Tools) >= 2,
					"Other tools should still be available after filtering ExitPlanMode")
			} else {
				assert.True(t, hasExitPlanMode,
					"Test case %s: ExitPlanMode should be available for %s. %s",
					tt.name, tt.userMessage, tt.description)
			}

			t.Logf("✅ Test %s: userMessage='%s', hasExitPlanMode=%v, expected=%v",
				tt.name, tt.userMessage, hasExitPlanMode, !tt.expectExitPlanModeSkipped)
		})
	}
}

// TestOriginalExitPlanModeMisuseScenario tests that the original problematic scenario
// would now be prevented at the source through context-aware filtering
func TestOriginalExitPlanModeMisuseScenario(t *testing.T) {
	// Use real LLM endpoint from environment
	ctx := internal.WithRequestID(context.Background(), "test-original-scenario")
	cfg := getTestConfigWithRealLLM()

	// Recreate the exact original request that led to the misuse
	originalProblematicRequest := types.AnthropicRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		Messages: []types.Message{
			{
				Role: "user",
				Content: []types.Content{
					{
						Type: "text",
						Text: "read architecture md and check recent changes to tool correction related to exit plan mode",
					},
				},
			},
		},
		Tools: []types.Tool{
			{Name: "LS", Description: "List files", InputSchema: types.ToolSchema{Type: "object"}},
			{Name: "Grep", Description: "Search files", InputSchema: types.ToolSchema{Type: "object"}},
			{Name: "Read", Description: "Read files", InputSchema: types.ToolSchema{Type: "object"}},
			{Name: "Bash", Description: "Run commands", InputSchema: types.ToolSchema{Type: "object"}},
			{Name: "Glob", Description: "Find files", InputSchema: types.ToolSchema{Type: "object"}},
			{Name: "ExitPlanMode", Description: "Exit plan mode", InputSchema: types.ToolSchema{Type: "object"}},
		},
	}

	// Transform the request with our root cause fix
	openaiReq, err := proxy.TransformAnthropicToOpenAI(ctx, originalProblematicRequest, cfg)
	require.NoError(t, err)

	// Verify ExitPlanMode was filtered out
	hasExitPlanMode := false
	availableTools := make([]string, 0, len(openaiReq.Tools))
	for _, tool := range openaiReq.Tools {
		availableTools = append(availableTools, tool.Function.Name)
		if tool.Function.Name == "ExitPlanMode" {
			hasExitPlanMode = true
		}
	}

	// The root cause fix should prevent ExitPlanMode from being available
	assert.False(t, hasExitPlanMode, "ExitPlanMode should be filtered out for research requests")

	// Research tools should still be available
	expectedResearchTools := []string{"LS", "Grep", "Read", "Bash", "Glob"}
	for _, expectedTool := range expectedResearchTools {
		found := false
		for _, availableTool := range availableTools {
			if availableTool == expectedTool {
				found = true
				break
			}
		}
		assert.True(t, found, "Research tool %s should still be available", expectedTool)
	}

	t.Logf("🎯 ROOT CAUSE FIX VERIFIED: Original problematic request now filters out ExitPlanMode")
	t.Logf("📋 Available tools after filtering: %v", availableTools)
	t.Logf("🚫 ExitPlanMode correctly filtered out, preventing misuse at source")
}
