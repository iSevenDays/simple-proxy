package test

import (
	"claude-proxy/types"
	"context"
	"testing"
)

// TestToolValidator_EdgeCases tests edge cases that exist in the current correction service
func TestToolValidator_EdgeCases(t *testing.T) {
	validator := types.NewStandardToolValidator()
	ctx := context.Background()

	t.Run("empty schema", func(t *testing.T) {
		schema := types.ToolSchema{
			Type:       "object",
			Properties: map[string]types.ToolProperty{},
			Required:   []string{},
		}

		call := types.Content{
			Type:  "tool_use",
			Name:  "EmptyTool",
			Input: map[string]interface{}{},
		}

		result := validator.ValidateParameters(ctx, call, schema)
		if !result.IsValid {
			t.Error("Expected empty schema validation to pass")
		}
	})

	t.Run("nil input map", func(t *testing.T) {
		schema := types.ToolSchema{
			Type: "object",
			Properties: map[string]types.ToolProperty{
				"param": {Type: "string", Description: "A parameter"},
			},
			Required: []string{"param"},
		}

		call := types.Content{
			Type:  "tool_use",
			Name:  "TestTool",
			Input: nil, // This could happen in malformed requests
		}

		result := validator.ValidateParameters(ctx, call, schema)
		if result.IsValid {
			t.Error("Expected validation to fail for nil input")
		}
		if len(result.MissingParams) != 1 || result.MissingParams[0] != "param" {
			t.Errorf("Expected missing param 'param', got: %v", result.MissingParams)
		}
	})

	t.Run("multiple missing and invalid params", func(t *testing.T) {
		schema := types.ToolSchema{
			Type: "object",
			Properties: map[string]types.ToolProperty{
				"required1": {Type: "string"},
				"required2": {Type: "string"},
				"optional":  {Type: "string"},
			},
			Required: []string{"required1", "required2"},
		}

		call := types.Content{
			Type: "tool_use",
			Name: "TestTool",
			Input: map[string]interface{}{
				"required1":    "present",
				"invalid1":     "should not be here",
				"invalid2":     "also invalid",
			},
		}

		result := validator.ValidateParameters(ctx, call, schema)
		if result.IsValid {
			t.Error("Expected validation to fail")
		}

		// Check missing parameters
		if len(result.MissingParams) != 1 || result.MissingParams[0] != "required2" {
			t.Errorf("Expected missing params [required2], got: %v", result.MissingParams)
		}

		// Check invalid parameters
		expectedInvalid := map[string]bool{"invalid1": true, "invalid2": true}
		if len(result.InvalidParams) != 2 {
			t.Errorf("Expected 2 invalid params, got %d: %v", len(result.InvalidParams), result.InvalidParams)
		}
		for _, invalid := range result.InvalidParams {
			if !expectedInvalid[invalid] {
				t.Errorf("Unexpected invalid param: %s", invalid)
			}
		}
	})
}

// TestToolValidator_RealWorldSchemas tests with actual Claude Code tool schemas
func TestToolValidator_RealWorldSchemas(t *testing.T) {
	validator := types.NewStandardToolValidator()
	ctx := context.Background()

	t.Run("Read tool validation", func(t *testing.T) {
		// Use the actual Read tool schema from GetFallbackToolSchema
		readTool := types.GetFallbackToolSchema("Read")
		if readTool == nil {
			t.Fatal("Could not get Read tool schema")
		}

		// Valid Read call
		validCall := types.Content{
			Type: "tool_use",
			Name: "Read",
			Input: map[string]interface{}{
				"file_path": "/test/file.go",
			},
		}

		result := validator.ValidateParameters(ctx, validCall, readTool.InputSchema)
		if !result.IsValid {
			t.Errorf("Expected valid Read call to pass, got missing: %v, invalid: %v",
				result.MissingParams, result.InvalidParams)
		}

		// Invalid Read call - missing file_path
		invalidCall := types.Content{
			Type: "tool_use",
			Name: "Read",
			Input: map[string]interface{}{
				"invalid_param": "test",
			},
		}

		result = validator.ValidateParameters(ctx, invalidCall, readTool.InputSchema)
		if result.IsValid {
			t.Error("Expected invalid Read call to fail")
		}
	})

	t.Run("WebSearch tool validation", func(t *testing.T) {
		webSearchTool := types.GetFallbackToolSchema("WebSearch")
		if webSearchTool == nil {
			t.Fatal("Could not get WebSearch tool schema")
		}

		// Valid WebSearch call
		validCall := types.Content{
			Type: "tool_use",
			Name: "WebSearch", 
			Input: map[string]interface{}{
				"query": "test query",
				"allowed_domains": []string{"example.com"},
			},
		}

		result := validator.ValidateParameters(ctx, validCall, webSearchTool.InputSchema)
		if !result.IsValid {
			t.Errorf("Expected valid WebSearch call to pass, got missing: %v, invalid: %v",
				result.MissingParams, result.InvalidParams)
		}
	})
}

// TestToolValidator_NormalizeToolName_Comprehensive tests all tool name mappings
func TestToolValidator_NormalizeToolName_Comprehensive(t *testing.T) {
	validator := types.NewStandardToolValidator()

	tests := []struct {
		input    string
		expected string
		found    bool
	}{
		// Canonical names
		{"WebSearch", "WebSearch", true},
		{"Read", "Read", true},
		{"Write", "Write", true},
		{"Edit", "Edit", true},
		{"MultiEdit", "MultiEdit", true},

		// Lowercase versions
		{"websearch", "WebSearch", true},
		{"read", "Read", true},
		{"write", "Write", true},
		{"edit", "Edit", true},
		{"multiedit", "MultiEdit", true},

		// Alias versions
		{"web_search", "WebSearch", true},
		{"read_file", "Read", true},
		{"write_file", "Write", true},
		{"multi_edit", "MultiEdit", true},
		{"bash_command", "Bash", true},
		{"grep_search", "Grep", true},

		// Case insensitive aliases
		{"WEB_SEARCH", "WebSearch", true},
		{"READ_FILE", "Read", true},
		{"MULTI_EDIT", "MultiEdit", true},

		// Unknown tools
		{"UnknownTool", "", false},
		{"random_tool", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			normalized, found := validator.NormalizeToolName(tt.input)

			if found != tt.found {
				t.Errorf("Expected found=%v, got %v", tt.found, found)
			}

			if found && normalized != tt.expected {
				t.Errorf("Expected normalized name '%s', got '%s'", tt.expected, normalized)
			}

			if !found && normalized != "" {
				t.Errorf("Expected empty normalized name for not found, got '%s'", normalized)
			}
		})
	}
}

// TestToolValidator_Benchmark tests performance of validation
func TestToolValidator_Benchmark(t *testing.T) {
	validator := types.NewStandardToolValidator()
	ctx := context.Background()

	schema := types.ToolSchema{
		Type: "object",
		Properties: map[string]types.ToolProperty{
			"param1": {Type: "string"},
			"param2": {Type: "number"},
			"param3": {Type: "array"},
		},
		Required: []string{"param1", "param2"},
	}

	call := types.Content{
		Type: "tool_use",
		Name: "TestTool",
		Input: map[string]interface{}{
			"param1": "value1",
			"param2": 42,
			"param3": []string{"a", "b", "c"},
		},
	}

	// Run validation multiple times to ensure it's deterministic and fast
	for i := 0; i < 100; i++ {
		result := validator.ValidateParameters(ctx, call, schema)
		if !result.IsValid {
			t.Errorf("Validation failed on iteration %d: missing=%v, invalid=%v", 
				i, result.MissingParams, result.InvalidParams)
		}
	}
}