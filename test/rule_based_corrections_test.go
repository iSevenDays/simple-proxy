package test

import (
	"claude-proxy/correction"
	"claude-proxy/internal"
	"claude-proxy/types"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRuleBasedParameterCorrections tests that common parameter name issues
// are fixed instantly without LLM calls for better performance
func TestRuleBasedParameterCorrections(t *testing.T) {
	service := correction.NewService(NewMockConfigProvider("http://test.com"), "test-key", true, "test-model", false, nil)
	ctx := internal.WithRequestID(context.Background(), "rule_based_test")

	// Available tools for validation (used in integration tests)
	_ = []types.Tool{
		{
			Name:        "Read",
			Description: "Read a file",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"file_path": {Type: "string", Description: "Path to file"},
					"limit":     {Type: "number", Description: "Line limit"},
					"offset":    {Type: "number", Description: "Line offset"},
				},
				Required: []string{"file_path"},
			},
		},
		{
			Name:        "Write",
			Description: "Write a file",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"file_path": {Type: "string", Description: "Path to file"},
					"content":   {Type: "string", Description: "File content"},
				},
				Required: []string{"file_path", "content"},
			},
		},
		{
			Name:        "Grep",
			Description: "Search files",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"pattern":     {Type: "string", Description: "Search pattern"},
					"path":        {Type: "string", Description: "Search path"},
					"glob":        {Type: "string", Description: "File filter"},
					"output_mode": {Type: "string", Description: "Output format"},
				},
				Required: []string{"pattern"},
			},
		},
	}

	testCases := []struct {
		name                string
		inputCall           types.Content
		expectedCall        types.Content
		shouldUseRuleBased  bool
		description         string
	}{
		{
			name: "filename_to_file_path_correction",
			inputCall: types.Content{
				Type: "tool_use",
				ID:   "test1",
				Name: "Read",
				Input: map[string]interface{}{
					"filename": "/path/to/file.txt",
				},
			},
			expectedCall: types.Content{
				Type: "tool_use",
				ID:   "test1",
				Name: "Read",
				Input: map[string]interface{}{
					"file_path": "/path/to/file.txt",
				},
			},
			shouldUseRuleBased: true,
			description:        "filename should be corrected to file_path instantly",
		},
		{
			name: "path_to_file_path_correction_write",
			inputCall: types.Content{
				Type: "tool_use",
				ID:   "test2",
				Name: "Write",
				Input: map[string]interface{}{
					"path":    "/path/to/file.txt",
					"content": "Hello world",
				},
			},
			expectedCall: types.Content{
				Type: "tool_use",
				ID:   "test2",
				Name: "Write",
				Input: map[string]interface{}{
					"file_path": "/path/to/file.txt",
					"content":   "Hello world",
				},
			},
			shouldUseRuleBased: true,
			description:        "path should be corrected to file_path for Write tool",
		},
		{
			name: "text_to_content_correction",
			inputCall: types.Content{
				Type: "tool_use",
				ID:   "test3",
				Name: "Write",
				Input: map[string]interface{}{
					"file_path": "/path/to/file.txt",
					"text":      "Hello world",
				},
			},
			expectedCall: types.Content{
				Type: "tool_use",
				ID:   "test3",
				Name: "Write",
				Input: map[string]interface{}{
					"file_path": "/path/to/file.txt",
					"content":   "Hello world",
				},
			},
			shouldUseRuleBased: true,
			description:        "text should be corrected to content",
		},
		{
			name: "query_to_pattern_for_grep",
			inputCall: types.Content{
				Type: "tool_use",
				ID:   "test4",
				Name: "Grep",
				Input: map[string]interface{}{
					"query": "function.*main",
					"path":  "/src",
				},
			},
			expectedCall: types.Content{
				Type: "tool_use",
				ID:   "test4",
				Name: "Grep",
				Input: map[string]interface{}{
					"pattern": "function.*main",
					"path":    "/src",
				},
			},
			shouldUseRuleBased: true,
			description:        "query should be corrected to pattern for Grep tool",
		},
		{
			name: "search_to_pattern_for_grep",
			inputCall: types.Content{
				Type: "tool_use",
				ID:   "test5",
				Name: "Grep",
				Input: map[string]interface{}{
					"search": "TODO.*FIXME",
				},
			},
			expectedCall: types.Content{
				Type: "tool_use",
				ID:   "test5",
				Name: "Grep",
				Input: map[string]interface{}{
					"pattern": "TODO.*FIXME",
				},
			},
			shouldUseRuleBased: true,
			description:        "search should be corrected to pattern for Grep tool",
		},
		{
			name: "filter_to_glob_for_grep",
			inputCall: types.Content{
				Type: "tool_use",
				ID:   "test6",
				Name: "Grep",
				Input: map[string]interface{}{
					"pattern": "error",
					"filter":  "*.js",
				},
			},
			expectedCall: types.Content{
				Type: "tool_use",
				ID:   "test6",
				Name: "Grep",
				Input: map[string]interface{}{
					"pattern": "error",
					"glob":    "*.js",
				},
			},
			shouldUseRuleBased: true,
			description:        "filter should be corrected to glob for Grep tool",
		},
		{
			name: "multiple_parameter_corrections",
			inputCall: types.Content{
				Type: "tool_use",
				ID:   "test7",
				Name: "Write",
				Input: map[string]interface{}{
					"filename": "/path/to/file.txt",
					"text":     "Content here",
				},
			},
			expectedCall: types.Content{
				Type: "tool_use",
				ID:   "test7",
				Name: "Write",
				Input: map[string]interface{}{
					"file_path": "/path/to/file.txt",
					"content":   "Content here",
				},
			},
			shouldUseRuleBased: true,
			description:        "Multiple parameters should be corrected in one pass",
		},
		{
			name: "no_correction_needed",
			inputCall: types.Content{
				Type: "tool_use",
				ID:   "test8",
				Name: "Read",
				Input: map[string]interface{}{
					"file_path": "/path/to/file.txt",
				},
			},
			expectedCall: types.Content{
				Type: "tool_use",
				ID:   "test8",
				Name: "Read",
				Input: map[string]interface{}{
					"file_path": "/path/to/file.txt",
				},
			},
			shouldUseRuleBased: false,
			description:        "Already correct parameters should not be changed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test rule-based correction directly
			correctedCall, wasRuleBased := service.AttemptRuleBasedParameterCorrection(ctx, tc.inputCall)
			
			// Verify if rule-based correction was used
			assert.Equal(t, tc.shouldUseRuleBased, wasRuleBased, tc.description)
			
			// Verify the corrected call matches expected
			assert.Equal(t, tc.expectedCall.Type, correctedCall.Type, "Type should be preserved")
			assert.Equal(t, tc.expectedCall.ID, correctedCall.ID, "ID should be preserved")
			assert.Equal(t, tc.expectedCall.Name, correctedCall.Name, "Name should be preserved")
			
			// Check each expected parameter
			for key, expectedValue := range tc.expectedCall.Input {
				actualValue, exists := correctedCall.Input[key]
				assert.True(t, exists, "Parameter %s should exist", key)
				assert.Equal(t, expectedValue, actualValue, "Parameter %s should have correct value", key)
			}
			
			// Verify no unexpected parameters were added
			assert.Equal(t, len(tc.expectedCall.Input), len(correctedCall.Input), "Parameter count should match")
		})
	}
}

// TestRuleBasedCorrectionIntegration tests rule-based corrections in the full correction pipeline
func TestRuleBasedCorrectionIntegration(t *testing.T) {
	service := correction.NewService(NewMockConfigProvider("http://test.com"), "test-key", true, "test-model", false, nil)
	ctx := internal.WithRequestID(context.Background(), "rule_integration_test")

	// Available tools for validation
	availableTools := []types.Tool{
		{
			Name:        "Read",
			Description: "Read a file",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"file_path": {Type: "string", Description: "Path to file"},
				},
				Required: []string{"file_path"},
			},
		},
	}

	// Test case: parameter name issue that should be fixed by rule-based correction
	inputCalls := []types.Content{
		{
			Type: "tool_use",
			ID:   "test_integration",
			Name: "Read",
			Input: map[string]interface{}{
				"filename": "/path/to/file.txt", // Wrong parameter name
			},
		},
	}

	// This should be corrected by rule-based correction before any LLM call
	correctedCalls, err := service.CorrectToolCalls(ctx, inputCalls, availableTools)
	require.NoError(t, err, "Correction should succeed")
	require.Len(t, correctedCalls, 1, "Should return one corrected call")

	correctedCall := correctedCalls[0]
	assert.Equal(t, "Read", correctedCall.Name, "Tool name should be preserved")
	
	// The key test: filename should be corrected to file_path
	assert.Contains(t, correctedCall.Input, "file_path", "Should have file_path parameter")
	assert.NotContains(t, correctedCall.Input, "filename", "Should not have filename parameter")
	assert.Equal(t, "/path/to/file.txt", correctedCall.Input["file_path"], "file_path should have correct value")

	// Verify the corrected call validates successfully
	validation := service.ValidateToolCall(ctx, correctedCall, availableTools)
	assert.True(t, validation.IsValid, "Rule-based corrected call should be valid")
	assert.Empty(t, validation.MissingParams, "Should have no missing parameters")
	assert.Empty(t, validation.InvalidParams, "Should have no invalid parameters")
}

// TestRuleBasedCorrectionPerformance tests that rule-based corrections avoid LLM calls
func TestRuleBasedCorrectionPerformance(t *testing.T) {
	// This test verifies that rule-based corrections happen without LLM calls
	// We can't easily mock the LLM call in this test, but we can verify the behavior
	// by checking that corrections happen instantly and produce valid results
	
	service := correction.NewService(NewMockConfigProvider("http://invalid-endpoint"), "invalid-key", true, "test-model", false, nil)
	ctx := internal.WithRequestID(context.Background(), "performance_test")

	availableTools := []types.Tool{
		{
			Name:        "Write",
			Description: "Write a file",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"file_path": {Type: "string", Description: "Path to file"},
					"content":   {Type: "string", Description: "File content"},
				},
				Required: []string{"file_path", "content"},
			},
		},
	}

	// Input with parameter name issues that should be fixed by rule-based correction
	inputCall := types.Content{
		Type: "tool_use",
		ID:   "perf_test",
		Name: "Write",
		Input: map[string]interface{}{
			"filename": "/test.txt",  // Should become file_path
			"text":     "test data",   // Should become content
		},
	}

	// Test rule-based correction directly (should succeed even with invalid endpoint)
	correctedCall, wasRuleBased := service.AttemptRuleBasedParameterCorrection(ctx, inputCall)
	
	assert.True(t, wasRuleBased, "Should use rule-based correction")
	assert.Equal(t, "/test.txt", correctedCall.Input["file_path"], "filename should become file_path")
	assert.Equal(t, "test data", correctedCall.Input["content"], "text should become content")
	assert.NotContains(t, correctedCall.Input, "filename", "filename should be removed")
	assert.NotContains(t, correctedCall.Input, "text", "text should be removed")

	// Verify the result validates correctly
	validation := service.ValidateToolCall(ctx, correctedCall, availableTools)
	assert.True(t, validation.IsValid, "Rule-based correction should produce valid result")
}