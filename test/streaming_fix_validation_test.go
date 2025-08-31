package test

import (
	"claude-proxy/internal"
	"claude-proxy/proxy"
	"claude-proxy/types"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreamingFixValidation verifies the splitTextForStreaming fix works correctly
func TestStreamingFixValidation(t *testing.T) {
	// Use the exact content that was being truncated
	content := `✅ PRD created: .claude/prds/installation-outcome-idea-b.md

The PRD defines a refactoring to use an InstallationOutcome record that captures the final result of an iOS app installation. This ensures metrics like app-installation.ios are reported exactly once per request, giving developers accurate visibility into installation success/failure rates, independent of internal fallback attempts.

Ready to create implementation epic? Run: /pm:prd-parse installation-outcome-idea-b`

	openaiResponse := types.OpenAIResponse{
		ID:     "streaming_fix_test",
		Object: "chat.completion",
		Model:  "claude-sonnet-4-20250514",
		Choices: []types.OpenAIChoice{
			{
				Message: types.OpenAIMessage{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: stringPtr("stop"),
			},
		},
		Usage: types.OpenAIUsage{
			PromptTokens:     0,
			CompletionTokens: 0,
		},
	}

	ctx := internal.WithRequestID(context.Background(), "streaming_fix_validation")
	result, err := proxy.TransformOpenAIToAnthropic(ctx, &openaiResponse, "claude-sonnet-4-20250514", getTestConfig())
	require.NoError(t, err)

	actualContent := result.Content[0].Text

	t.Logf("Original content length: %d", len(content))
	t.Logf("Transformed content length: %d", len(actualContent))

	// PRIMARY ASSERTIONS: Fix should preserve all content
	assert.Equal(t, content, actualContent, "Fix should preserve complete content without character loss")

	// Verify critical elements are preserved
	assert.Contains(t, actualContent, "final result of an iOS app installation", 
		"Should contain the full descriptive text that was being lost")
	
	assert.Contains(t, actualContent, "This ensures metrics like app-installation.ios are reported exactly once per request", 
		"Should contain the complete sentence that was being truncated")
	
	assert.Contains(t, actualContent, "giving developers accurate visibility", 
		"Should contain the complete phrase, not truncated 'curate visibility'")
	
	assert.Contains(t, actualContent, "/pm:prd-parse installation-outcome-idea-b",
		"Should contain the complete command that was being lost")

	// Verify response ends correctly
	assert.True(t, strings.HasSuffix(actualContent, "/pm:prd-parse installation-outcome-idea-b"),
		"Response should end with the complete command")

	// Verify no text corruption occurred - check that "accurate" is not truncated to "curate"
	assert.Contains(t, actualContent, "accurate visibility", 
		"Should contain 'accurate visibility' not truncated 'curate visibility'")
	assert.NotContains(t, actualContent, " curate visibility", 
		"Should not contain corrupted text like ' curate visibility' (with leading space)")

	t.Logf("✅ STREAMING FIX VALIDATED: All content preserved correctly")
}

// TestMultipleCommandsPreserved tests that multiple commands at different positions are preserved
func TestMultipleCommandsPreserved(t *testing.T) {
	testCases := []struct {
		name    string
		content string
		commands []string
	}{
		{
			name: "command_at_end",
			content: `Analysis complete. Here are the findings and recommendations.

## Summary
The system is working correctly but needs optimization.

## Next Steps
To proceed with implementation: /pm:task-create optimization-improvements`,
			commands: []string{"/pm:task-create optimization-improvements"},
		},
		{
			name: "multiple_commands",
			content: `Code review completed successfully.

## Phase 1
Start the review process: /pm:review-start code-analysis

## Phase 2  
After review completion, create the deployment plan: /deploy:prepare production-v2.0

## Phase 3
Finally, execute the deployment: /deploy:execute production-v2.0`,
			commands: []string{
				"/pm:review-start code-analysis", 
				"/deploy:prepare production-v2.0",
				"/deploy:execute production-v2.0",
			},
		},
		{
			name: "long_content_with_end_command",
			content: `# Comprehensive Analysis Report

## Executive Summary
This detailed analysis covers multiple aspects of the system architecture, performance metrics, security considerations, and deployment strategies. The findings indicate that while the current implementation is functional, there are several opportunities for improvement that could significantly enhance both performance and maintainability.

## Architecture Review
The current architecture follows a microservices pattern with clear separation of concerns. Each service handles its specific domain responsibilities, communicating through well-defined APIs. The database layer uses a combination of relational and document storage to optimize for different data access patterns.

## Performance Analysis
Load testing reveals that the system can handle current traffic loads effectively, but there are bottlenecks that become apparent under stress conditions. The primary issues are related to database query optimization and caching strategies.

## Security Assessment
Security measures are generally robust, with proper authentication and authorization mechanisms in place. However, there are some areas where additional hardening would be beneficial, particularly around input validation and rate limiting.

## Deployment Strategy
The current deployment process works well for the development environment but needs refinement for production scaling. Container orchestration and automated rollback capabilities would improve reliability.

## Recommendations
Based on this comprehensive analysis, the recommended next step is to create an implementation plan: /pm:epic-create system-optimization-v2.1`,
			commands: []string{"/pm:epic-create system-optimization-v2.1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			openaiResponse := types.OpenAIResponse{
				ID: "multi_cmd_" + tc.name,
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant",
							Content: tc.content,
						},
						FinishReason: stringPtr("stop"),
					},
				},
			}

			ctx := internal.WithRequestID(context.Background(), tc.name)
			result, err := proxy.TransformOpenAIToAnthropic(ctx, &openaiResponse, "claude-sonnet-4-20250514", getTestConfig())
			require.NoError(t, err)

			actualContent := result.Content[0].Text

			// Verify complete content preservation
			assert.Equal(t, tc.content, actualContent, "Content should be fully preserved")

			// Verify all commands are present
			for _, cmd := range tc.commands {
				assert.Contains(t, actualContent, cmd, "Command %s should be preserved", cmd)
			}

			t.Logf("✅ %s: All %d commands preserved", tc.name, len(tc.commands))
		})
	}
}

// TestChunkingBoundaryScenarios tests edge cases around chunking boundaries
func TestChunkingBoundaryScenarios(t *testing.T) {
	// Create content that will hit different chunking scenarios
	scenarios := []struct {
		name string
		content string
		description string
	}{
		{
			name: "space_exactly_at_150_chars",
			content: strings.Repeat("a", 149) + " final_command",
			description: "Space exactly at chunk boundary",
		},
		{
			name: "newline_at_boundary", 
			content: strings.Repeat("b", 149) + "\nfinal_command",
			description: "Newline at chunk boundary",
		},
		{
			name: "no_spaces_in_range",
			content: strings.Repeat("c", 200) + " final_command", 
			description: "No word boundaries in lookback range",
		},
		{
			name: "multiple_chunks_with_commands",
			content: strings.Repeat("d", 300) + " middle_command " + strings.Repeat("e", 300) + " final_command",
			description: "Multiple chunks each with commands",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			openaiResponse := types.OpenAIResponse{
				ID: "boundary_" + scenario.name,
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Role:    "assistant", 
							Content: scenario.content,
						},
						FinishReason: stringPtr("stop"),
					},
				},
			}

			ctx := internal.WithRequestID(context.Background(), scenario.name)
			result, err := proxy.TransformOpenAIToAnthropic(ctx, &openaiResponse, "claude-sonnet-4-20250514", getTestConfig())
			require.NoError(t, err)

			actualContent := result.Content[0].Text

			// Verify no content loss at chunk boundaries
			assert.Equal(t, scenario.content, actualContent, 
				"Chunking boundary scenario should not lose content: %s", scenario.description)

			t.Logf("✅ %s: Content preserved (%d chars)", scenario.name, len(actualContent))
		})
	}
}