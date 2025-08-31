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

// TestResponseTruncationSimple tests the specific truncation issue from the user's example
func TestResponseTruncationSimple(t *testing.T) {
	// This reproduces the exact issue: response ending with command gets truncated
	originalContent := "✅ Document created: .claude/specs/feature-implementation-v2.md\n\nThe specification defines a refactoring to use a DataResult record that captures the final result of a processing operation. This ensures metrics like process-execution.count are reported exactly once per request, giving developers accurate visibility into success/failure rates, independent of internal retry attempts.\n\nReady to create implementation epic? Run: /pm:spec-parse feature-implementation-v2"

	openaiResponse := types.OpenAIResponse{
		ID:     "chatcmpl-8rpOza3M2jwODmmRJz49egkyFsBs4Q2T",
		Object: "chat.completion",
		Model:  "claude-sonnet-4-20250514",
		Choices: []types.OpenAIChoice{
			{
				Message: types.OpenAIMessage{
					Role:    "assistant",
					Content: originalContent,
				},
				FinishReason: stringPtr("stop"),
			},
		},
		Usage: types.OpenAIUsage{
			PromptTokens:     0,
			CompletionTokens: 0,
		},
	}

	ctx := internal.WithRequestID(context.Background(), "truncation_test")
	result, err := proxy.TransformOpenAIToAnthropic(ctx, &openaiResponse, "claude-sonnet-4-20250514", getTestConfig())
	require.NoError(t, err)

	actualContent := result.Content[0].Text

	t.Logf("Original length: %d", len(originalContent))
	t.Logf("Actual length: %d", len(actualContent))
	
	// Primary assertion - content should match exactly
	assert.Equal(t, originalContent, actualContent, "Content should be preserved exactly")

	// Verify the specific parts that were missing in Claude Code UI
	missingParts := []string{
		"/pm:spec-parse feature-implementation-v2",
		"giving developers accurate visibility into success/failure rates",
		"Ready to create implementation epic? Run:",
	}

	for _, part := range missingParts {
		assert.Contains(t, actualContent, part, "Expected part should be present: %q", part)
	}

	// Verify the response ends correctly (common truncation point)
	assert.True(t, strings.HasSuffix(actualContent, "/pm:spec-parse feature-implementation-v2"),
		"Response should end with the complete command")
}

// TestLongResponseWithEndCommands tests responses that end with commands
func TestLongResponseWithEndCommands(t *testing.T) {
	testCases := []struct {
		name        string
		content     string
		expectedEnd string
		description string
	}{
		{
			name: "response_ending_with_slash_command",
			content: `Analysis complete. The data processing metrics issue has been identified and a solution designed.

## Problem Summary:
The data-processing.count metric is being incremented multiple times per processing request due to fallback logic in the processor methods. This causes inaccurate reporting where one processing request can generate 2-3 metric events.

## Root Cause:
Each processing method (processViaServiceA, processViaServiceB, processViaServiceC) calls reportProcessing() independently, and the fallback logic means multiple methods may be attempted for a single request.

## Solution: DataResult Record Pattern
Implement a record-based approach where:
1. Each low-level method returns DataResult(method, success, duration, dataSize)
2. Top-level process() method selects the appropriate outcome
3. Single recordMetrics(result) call eliminates double-counting

## Implementation Ready
The solution has been documented and is ready for implementation.

To proceed: /pm:epic-create data-processing-fix`,
			expectedEnd: "/pm:epic-create data-processing-fix",
			description: "Response ending with project management command should be preserved",
		},
		{
			name: "response_ending_with_deployment_command",
			content: `Deployment analysis complete. The data processing metrics refactor is ready for production deployment.

## Pre-deployment Checklist:
✅ Unit tests passing (all 247 tests)
✅ Integration tests verified 
✅ Performance regression tests completed
✅ Database migration scripts validated
✅ Monitoring dashboards updated
✅ Rollback procedures documented

## Deployment Strategy:
1. **Canary Phase**: Deploy to 5% of data processing requests
2. **Validation Phase**: Monitor metrics for 2 hours, verify single emission
3. **Full Rollout**: Gradual increase to 100% over 24 hours
4. **Cleanup Phase**: Remove legacy metric calls after 1 week

## Monitoring:
- Watch data-processing.count metric counts (should decrease by ~60%)
- Monitor processing success rates (should remain stable)
- Track processing duration P95 (should improve by ~200ms)

All systems ready for deployment.

Execute deployment: /deploy:canary data-processing-v1.0.0`,
			expectedEnd: "/deploy:canary data-processing-v1.0.0",
			description: "Response ending with deployment command should be preserved",
		},
		{
			name: "response_ending_with_test_command",
			content: `Code review complete. The DataResult implementation looks good with one minor issue to address.

## Review Summary:
✅ **Architecture**: Clean separation of concerns, proper encapsulation
✅ **Error Handling**: Comprehensive exception handling and logging
✅ **Performance**: No significant performance impact detected
⚠️ **Testing**: Need additional edge case coverage for timeout scenarios

## Required Changes:
1. Add timeout test cases in DataResultTest.java
2. Verify metric emission during network failures
3. Test result selection logic with partial failures

## Approval Status:
The implementation is approved pending the additional test coverage.

Add missing tests: /test:generate data-result-edge-cases`,
			expectedEnd: "/test:generate data-result-edge-cases",
			description: "Response ending with test generation command should be preserved",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			openaiResponse := types.OpenAIResponse{
				ID: "resp_" + tc.name,
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

			// Verify exact match
			assert.Equal(t, tc.content, actualContent, tc.description)

			// Verify the command at the end is preserved
			assert.True(t, strings.HasSuffix(actualContent, tc.expectedEnd),
				"Response should end with: %s", tc.expectedEnd)

			// Log for debugging if it fails
			if actualContent != tc.content {
				t.Logf("Expected (%d chars): %q", len(tc.content), tc.content)
				t.Logf("Actual (%d chars): %q", len(actualContent), actualContent)
			}
		})
	}
}

// TestStreamingPreservesEndContent tests that streaming doesn't truncate content at the end
func TestStreamingPreservesEndContent(t *testing.T) {
	// Create content that would be split across multiple chunks
	longContent := `# Data Processing Metrics Analysis Report

## Executive Summary
The data processing system currently exhibits double-counting in the data-processing.count metric due to architectural issues in the fallback logic implementation.

## Detailed Findings
After analyzing the DataProcessor.java codebase, we identified that each processing method calls reportProcessing() independently:

### Method Analysis:
1. **processViaServiceA()** - Calls reportProcessing() at line 142
2. **processViaServiceB()** - Calls reportProcessing() at line 287  
3. **processViaServiceC()** - Calls reportProcessing() at line 451

### Fallback Logic Impact:
When the primary processing method fails, the system attempts fallback methods, each generating its own metric event. A single user processing request can result in 2-3 metric increments.

## Proposed Solution: DataResult Pattern
Implement a record-based approach that consolidates metric reporting:

### Design:
` + "```java" + `
record DataResult(String method, boolean success, Duration duration, long dataSize) {}
` + "```" + `

### Implementation Strategy:
1. Modify all processing methods to return DataResult instead of calling metrics
2. Update top-level process() method to handle result selection
3. Implement single recordMetrics(result) call per request
4. Remove individual reportProcessing() calls from helper methods

### Benefits:
- **Single Emission**: One metric event per processing request
- **Accurate Reporting**: True success/failure rates independent of retry attempts  
- **Clean Architecture**: Centralized metric logic
- **Future-Proof**: Easy to extend for additional metrics

## Implementation Timeline
- **Phase 1**: Create DataResult record and tests (1 day)
- **Phase 2**: Refactor processing methods (2 days)
- **Phase 3**: Integration testing and validation (1 day)
- **Phase 4**: Production deployment with monitoring (1 day)

## Risk Assessment
- **Low Risk**: Changes are isolated to metric reporting logic
- **Backward Compatible**: No changes to public API
- **Testable**: Comprehensive unit test coverage possible
- **Rollback Ready**: Easy to revert if issues arise

The implementation is thoroughly planned and ready to proceed.

Start implementation: /pm:task-create "DataResult record implementation"`

	openaiResponse := types.OpenAIResponse{
		ID: "resp_streaming_long",
		Choices: []types.OpenAIChoice{
			{
				Message: types.OpenAIMessage{
					Role:    "assistant",
					Content: longContent,
				},
				FinishReason: stringPtr("stop"),
			},
		},
	}

	ctx := internal.WithRequestID(context.Background(), "streaming_long_test")
	result, err := proxy.TransformOpenAIToAnthropic(ctx, &openaiResponse, "claude-sonnet-4-20250514", getTestConfig())
	require.NoError(t, err)

	actualContent := result.Content[0].Text

	// Verify complete content preservation
	assert.Equal(t, longContent, actualContent, "Long streaming content should be preserved exactly")

	// Verify the critical end command is preserved
	assert.True(t, strings.HasSuffix(actualContent, `/pm:task-create "DataResult record implementation"`),
		"Response should end with the complete command including quotes")

	// Verify key sections are preserved
	keyParts := []string{
		"Executive Summary",
		"processViaServiceA()** - Calls reportProcessing() at line 142",
		"Single Emission**: One metric event per processing request",
		"Start implementation: /pm:task-create \"DataResult record implementation\"",
	}

	for _, part := range keyParts {
		assert.Contains(t, actualContent, part, "Key part should be preserved: %q", part)
	}

	t.Logf("Content length - Expected: %d, Actual: %d", len(longContent), len(actualContent))
}