package test

import (
	"claude-proxy/correction"
	"claude-proxy/internal"
	"claude-proxy/types"
	"context"
	"testing"
)

// TDD RED PHASE: Define how the correction service should integrate with ToolValidator

// TestCorrectionService_WithInjectedValidator tests that the service accepts a ToolValidator
func TestCorrectionService_WithInjectedValidator(t *testing.T) {
	validator := types.NewStandardToolValidator()
	
	// This should fail initially - NewServiceWithValidator doesn't exist yet
	mockConfig := NewMockConfigProvider("http://test:8080")
	service := correction.NewServiceWithValidator(
		mockConfig,
		"test-key",
		true,
		"test-model",
		false,
		validator,
	)
	
	if service == nil {
		t.Fatal("Expected service to be created with validator")
	}
}

// TestCorrectionService_UsesInjectedValidator tests that ValidateToolCall uses the injected validator
func TestCorrectionService_UsesInjectedValidator(t *testing.T) {
	// Create mock validator that we can verify was called
	mockValidator := &MockToolValidator{
		validateFunc: func(ctx context.Context, call types.Content, schema types.ToolSchema) types.ValidationResult {
			// Return a specific result that we can verify
			return types.ValidationResult{
				IsValid:       false,
				MissingParams: []string{"mock_missing_param"},
				InvalidParams: []string{"mock_invalid_param"},
			}
		},
		normalizeFunc: func(toolName string) (string, bool) {
			if toolName == "test_tool" {
				return "TestTool", true
			}
			return "", false
		},
	}
	
	// This should fail initially - NewServiceWithValidator doesn't exist yet
	mockConfig2 := NewMockConfigProvider("http://test:8080")
	service := correction.NewServiceWithValidator(
		mockConfig2,
		"test-key",
		true,
		"test-model",
		false,
		mockValidator,
	)
	
	ctx := context.WithValue(context.Background(), internal.RequestIDKey, "test-req")
	
	call := types.Content{
		Type: "tool_use",
		Name: "test_tool",
		Input: map[string]interface{}{
			"param": "value",
		},
	}
	
	availableTools := []types.Tool{
		{
			Name: "TestTool",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"required_param": {Type: "string", Description: "Required parameter"},
				},
				Required: []string{"required_param"},
			},
		},
	}
	
	// This should use the injected validator and return the mock result
	result := service.ValidateToolCall(ctx, call, availableTools)
	
	// Verify the mock validator was used
	if len(result.MissingParams) != 1 || result.MissingParams[0] != "mock_missing_param" {
		t.Errorf("Expected mock validator result, got missing params: %v", result.MissingParams)
	}
	
	if len(result.InvalidParams) != 1 || result.InvalidParams[0] != "mock_invalid_param" {
		t.Errorf("Expected mock validator result, got invalid params: %v", result.InvalidParams)
	}
}

// TestCorrectionService_BackwardCompatibility tests that existing constructor still works
func TestCorrectionService_BackwardCompatibility(t *testing.T) {
	// Existing constructor should still work and use StandardToolValidator internally
	mockConfig3 := NewMockConfigProvider("http://test:8080")
	service := correction.NewService(
		mockConfig3,
		"test-key",
		true,
		"test-model",
		false,
	)
	
	ctx := context.WithValue(context.Background(), internal.RequestIDKey, "test-req")
	
	call := types.Content{
		Type: "tool_use",
		Name: "Read",
		Input: map[string]interface{}{
			"file_path": "/test/file.go",
		},
	}
	
	availableTools := []types.Tool{
		{
			Name: "Read",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"file_path": {Type: "string", Description: "The file path"},
				},
				Required: []string{"file_path"},
			},
		},
	}
	
	// This should work and return valid result using internal StandardToolValidator
	result := service.ValidateToolCall(ctx, call, availableTools)
	
	if !result.IsValid {
		t.Errorf("Expected valid result from backward compatible service, got: %+v", result)
	}
}

// TestCorrectionService_ValidatorIntegration_ToolNameNormalization tests tool name normalization integration
func TestCorrectionService_ValidatorIntegration_ToolNameNormalization(t *testing.T) {
	validator := types.NewStandardToolValidator()
	mockConfig4 := NewMockConfigProvider("http://test:8080")
	service := correction.NewServiceWithValidator(
		mockConfig4,
		"test-key",
		true,
		"test-model",
		false,
		validator,
	)
	
	ctx := context.WithValue(context.Background(), internal.RequestIDKey, "test-req")
	
	// Use lowercase tool name that should be normalized
	call := types.Content{
		Type: "tool_use",
		Name: "read", // lowercase - should be normalized to "Read"
		Input: map[string]interface{}{
			"file_path": "/test/file.go",
		},
	}
	
	availableTools := []types.Tool{
		{
			Name: "Read", // Correct casing
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"file_path": {Type: "string", Description: "The file path"},
				},
				Required: []string{"file_path"},
			},
		},
	}
	
	result := service.ValidateToolCall(ctx, call, availableTools)
	
	// Should detect case issue and provide correct tool name
	if !result.HasCaseIssue {
		t.Error("Expected case issue to be detected")
	}
	
	if result.CorrectToolName != "Read" {
		t.Errorf("Expected correct tool name 'Read', got '%s'", result.CorrectToolName)
	}
}

// TestCorrectionService_ValidatorIntegration_ParameterValidation tests parameter validation integration
func TestCorrectionService_ValidatorIntegration_ParameterValidation(t *testing.T) {
	validator := types.NewStandardToolValidator()
	mockConfig5 := NewMockConfigProvider("http://test:8080")
	service := correction.NewServiceWithValidator(
		mockConfig5,
		"test-key",
		true,
		"test-model",
		false,
		validator,
	)
	
	ctx := context.WithValue(context.Background(), internal.RequestIDKey, "test-req")
	
	call := types.Content{
		Type: "tool_use",
		Name: "Write",
		Input: map[string]interface{}{
			"invalid_param": "should not be here",
			// Missing required file_path and content
		},
	}
	
	availableTools := []types.Tool{
		{
			Name: "Write",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"file_path": {Type: "string", Description: "The file path"},
					"content":   {Type: "string", Description: "File content"},
				},
				Required: []string{"file_path", "content"},
			},
		},
	}
	
	result := service.ValidateToolCall(ctx, call, availableTools)
	
	// Should detect missing and invalid parameters
	if result.IsValid {
		t.Error("Expected validation to fail")
	}
	
	expectedMissing := map[string]bool{"file_path": true, "content": true}
	for _, missing := range result.MissingParams {
		if !expectedMissing[missing] {
			t.Errorf("Unexpected missing parameter: %s", missing)
		}
		delete(expectedMissing, missing)
	}
	if len(expectedMissing) > 0 {
		t.Errorf("Missing expected missing parameters: %v", expectedMissing)
	}
	
	if len(result.InvalidParams) != 1 || result.InvalidParams[0] != "invalid_param" {
		t.Errorf("Expected invalid parameter 'invalid_param', got: %v", result.InvalidParams)
	}
}