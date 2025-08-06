package test

import (
	"claude-proxy/types"
	"context"
	"testing"
)

// TestToolValidator_Interface defines the contract we want to extract
func TestToolValidator_Interface(t *testing.T) {
	// Test that our interface exists and can be implemented
	var _ types.ToolValidator = &MockToolValidator{}
	var _ types.ToolValidator = types.NewStandardToolValidator()
}

// MockToolValidator for testing
type MockToolValidator struct {
	validateFunc    func(ctx context.Context, call types.Content, schema types.ToolSchema) types.ValidationResult
	normalizeFunc   func(toolName string) (string, bool)
}

func (m *MockToolValidator) ValidateParameters(ctx context.Context, call types.Content, schema types.ToolSchema) types.ValidationResult {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, call, schema)
	}
	return types.ValidationResult{IsValid: true}
}

func (m *MockToolValidator) NormalizeToolName(toolName string) (string, bool) {
	if m.normalizeFunc != nil {
		return m.normalizeFunc(toolName)
	}
	return toolName, true
}

// GREEN PHASE: Test required parameter validation
func TestToolValidator_RequiredParameters_Missing(t *testing.T) {
	validator := types.NewStandardToolValidator()
	
	schema := types.ToolSchema{
		Type: "object",
		Properties: map[string]types.ToolProperty{
			"file_path": {Type: "string", Description: "The file path"},
		},
		Required: []string{"file_path"},
	}
	
	call := types.Content{
		Type: "tool_use",
		Name: "Read",
		Input: map[string]interface{}{}, // Missing required file_path
	}
	
	result := validator.ValidateParameters(context.Background(), call, schema)
	
	if result.IsValid {
		t.Error("Expected validation to fail for missing required parameter")
	}
	
	if len(result.MissingParams) != 1 {
		t.Errorf("Expected 1 missing parameter, got %d", len(result.MissingParams))
	}
	
	if result.MissingParams[0] != "file_path" {
		t.Errorf("Expected missing parameter 'file_path', got '%s'", result.MissingParams[0])
	}
}

// GREEN PHASE: Test required parameter validation success  
func TestToolValidator_RequiredParameters_Present(t *testing.T) {
	validator := types.NewStandardToolValidator()
	
	schema := types.ToolSchema{
		Type: "object",
		Properties: map[string]types.ToolProperty{
			"file_path": {Type: "string", Description: "The file path"},
		},
		Required: []string{"file_path"},
	}
	
	call := types.Content{
		Type: "tool_use", 
		Name: "Read",
		Input: map[string]interface{}{
			"file_path": "/test/file.go",
		},
	}
	
	result := validator.ValidateParameters(context.Background(), call, schema)
	
	if !result.IsValid {
		t.Errorf("Expected validation to pass, but got invalid with missing: %v, invalid: %v", 
			result.MissingParams, result.InvalidParams)
	}
}

// GREEN PHASE: Test invalid parameter detection
func TestToolValidator_InvalidParameters(t *testing.T) {
	validator := types.NewStandardToolValidator()
	
	schema := types.ToolSchema{
		Type: "object",
		Properties: map[string]types.ToolProperty{
			"file_path": {Type: "string", Description: "The file path"},
		},
		Required: []string{"file_path"},
	}
	
	call := types.Content{
		Type: "tool_use",
		Name: "Read", 
		Input: map[string]interface{}{
			"file_path":    "/test/file.go",
			"invalid_param": "should not be here",
		},
	}
	
	result := validator.ValidateParameters(context.Background(), call, schema)
	
	if result.IsValid {
		t.Error("Expected validation to fail for invalid parameter")
	}
	
	if len(result.InvalidParams) != 1 {
		t.Errorf("Expected 1 invalid parameter, got %d", len(result.InvalidParams))
	}
	
	if result.InvalidParams[0] != "invalid_param" {
		t.Errorf("Expected invalid parameter 'invalid_param', got '%s'", result.InvalidParams[0])
	}
}

// GREEN PHASE: Test tool name normalization
func TestToolValidator_NormalizeToolName(t *testing.T) {
	validator := types.NewStandardToolValidator()
	
	tests := []struct {
		name     string
		input    string
		expected string
		found    bool
	}{
		{"exact match", "Read", "Read", true},
		{"lowercase", "read", "Read", true},
		{"underscore variant", "read_file", "Read", true},
		{"unknown tool", "UnknownTool", "", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, found := validator.NormalizeToolName(tt.input)
			
			if found != tt.found {
				t.Errorf("Expected found=%v, got %v", tt.found, found)
			}
			
			if found && normalized != tt.expected {
				t.Errorf("Expected normalized name '%s', got '%s'", tt.expected, normalized)
			}
		})
	}
}

// GREEN PHASE: Test complex validation scenario (MultiEdit)
func TestToolValidator_ComplexTool_MultiEdit(t *testing.T) {
	validator := types.NewStandardToolValidator()
	
	schema := types.ToolSchema{
		Type: "object",
		Properties: map[string]types.ToolProperty{
			"file_path": {Type: "string", Description: "The file path"},
			"edits": {
				Type:        "array",
				Description: "Array of edit operations",
				Items:       &types.ToolPropertyItems{Type: "object"},
			},
		},
		Required: []string{"file_path", "edits"},
	}
	
	call := types.Content{
		Type: "tool_use",
		Name: "MultiEdit",
		Input: map[string]interface{}{
			"file_path": "/test/file.go",
			"edits": []interface{}{
				map[string]interface{}{
					"old_string": "old text",
					"new_string": "new text",
				},
			},
		},
	}
	
	result := validator.ValidateParameters(context.Background(), call, schema)
	
	if !result.IsValid {
		t.Errorf("Expected MultiEdit validation to pass, but got missing: %v, invalid: %v",
			result.MissingParams, result.InvalidParams)
	}
}

// StandardToolValidator is now implemented in types/validator.go