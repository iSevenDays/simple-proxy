package test

import (
	"claude-proxy/correction"
	"claude-proxy/internal"
	"claude-proxy/types"
	"context"
	"testing"
)

// TestToolValidator_IntegrationWithCorrectionService demonstrates how the new ToolValidator
// interface can be integrated with the existing correction service
func TestToolValidator_IntegrationWithCorrectionService(t *testing.T) {
	// Create validator
	validator := types.NewStandardToolValidator()
	
	// Create correction service (using existing constructor)
	service := correction.NewService("http://test", "test-key", true, "test-model", false)
	
	// Create context with request ID
	ctx := context.WithValue(context.Background(), internal.RequestIDKey, "test-req")
	
	// Create available tools (simulating tools from request)
	availableTools := []types.Tool{
		{
			Name:        "Read",
			Description: "Reads a file",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"file_path": {Type: "string", Description: "The file path"},
				},
				Required: []string{"file_path"},
			},
		},
	}
	
	// Test case 1: Valid tool call - should pass validation with both old and new systems
	t.Run("valid tool call comparison", func(t *testing.T) {
		validCall := types.Content{
			Type: "tool_use",
			Name: "Read",
			Input: map[string]interface{}{
				"file_path": "/test/file.go",
			},
		}
		
		// Test with existing correction service validation
		existingResult := service.ValidateToolCall(ctx, validCall, availableTools)
		
		// Test with new validator interface
		newResult := validator.ValidateParameters(ctx, validCall, availableTools[0].InputSchema)
		
		// Results should be equivalent
		if existingResult.IsValid != newResult.IsValid {
			t.Errorf("Validation results differ: existing=%v, new=%v", 
				existingResult.IsValid, newResult.IsValid)
		}
	})
	
	// Test case 2: Invalid tool call - missing required parameter
	t.Run("invalid tool call comparison", func(t *testing.T) {
		invalidCall := types.Content{
			Type: "tool_use",
			Name: "Read",
			Input: map[string]interface{}{
				"wrong_param": "value",
			},
		}
		
		// Test with existing correction service validation
		existingResult := service.ValidateToolCall(ctx, invalidCall, availableTools)
		
		// Test with new validator interface
		newResult := validator.ValidateParameters(ctx, invalidCall, availableTools[0].InputSchema)
		
		// Both should fail validation
		if existingResult.IsValid {
			t.Error("Expected existing validation to fail")
		}
		if newResult.IsValid {
			t.Error("Expected new validation to fail")
		}
		
		// Both should detect missing file_path
		existingHasMissing := len(existingResult.MissingParams) > 0
		newHasMissing := len(newResult.MissingParams) > 0
		
		if existingHasMissing != newHasMissing {
			t.Errorf("Missing parameter detection differs: existing=%v, new=%v",
				existingHasMissing, newHasMissing)
		}
	})
	
	// Test case 3: Tool name normalization
	t.Run("tool name normalization", func(t *testing.T) {
		testCases := []string{"read", "READ", "read_file", "READ_FILE"}
		
		for _, testName := range testCases {
			normalized, found := validator.NormalizeToolName(testName)
			
			if !found {
				t.Errorf("Expected tool name '%s' to be found", testName)
			}
			
			if normalized != "Read" {
				t.Errorf("Expected '%s' to normalize to 'Read', got '%s'", testName, normalized)
			}
		}
	})
}

// TestToolValidator_FallbackSchemaCompatibility tests compatibility with existing fallback schemas
func TestToolValidator_FallbackSchemaCompatibility(t *testing.T) {
	validator := types.NewStandardToolValidator()
	ctx := context.Background()
	
	// Test all available fallback schemas
	toolNames := []string{
		"WebSearch", "Read", "Write", "Edit", "MultiEdit",
		"Bash", "Grep", "Glob", "LS", "Task", "TodoWrite", "WebFetch",
	}
	
	for _, toolName := range toolNames {
		t.Run(toolName, func(t *testing.T) {
			// Get fallback schema
			fallbackTool := types.GetFallbackToolSchema(toolName)
			if fallbackTool == nil {
				t.Fatalf("Could not get fallback schema for %s", toolName)
			}
			
			// Test that tool name normalization works
			normalized, found := validator.NormalizeToolName(toolName)
			if !found {
				t.Errorf("Tool name normalization failed for %s", toolName)
			}
			
			if normalized != toolName {
				t.Logf("Tool name normalized: %s -> %s", toolName, normalized)
			}
			
			// Create a minimal valid call for this tool
			validInput := make(map[string]interface{})
			for _, required := range fallbackTool.InputSchema.Required {
				// Add dummy values for required parameters
				if prop, exists := fallbackTool.InputSchema.Properties[required]; exists {
					switch prop.Type {
					case "string":
						validInput[required] = "test_value"
					case "array":
						validInput[required] = []interface{}{"item"}
					case "number":
						validInput[required] = 42
					case "boolean":
						validInput[required] = true
					default:
						validInput[required] = "test_value"
					}
				}
			}
			
			validCall := types.Content{
				Type:  "tool_use",
				Name:  toolName,
				Input: validInput,
			}
			
			// Validate using our new validator
			result := validator.ValidateParameters(ctx, validCall, fallbackTool.InputSchema)
			
			if !result.IsValid {
				t.Errorf("Validation failed for %s: missing=%v, invalid=%v",
					toolName, result.MissingParams, result.InvalidParams)
			}
		})
	}
}

// TestToolValidator_PerformanceBenchmark benchmarks the validator performance
func BenchmarkToolValidator_ValidateParameters(b *testing.B) {
	validator := types.NewStandardToolValidator()
	ctx := context.Background()
	
	schema := types.ToolSchema{
		Type: "object",
		Properties: map[string]types.ToolProperty{
			"file_path": {Type: "string", Description: "File path"},
			"content":   {Type: "string", Description: "File content"},
			"optional":  {Type: "boolean", Description: "Optional param"},
		},
		Required: []string{"file_path", "content"},
	}
	
	call := types.Content{
		Type: "tool_use",
		Name: "Write",
		Input: map[string]interface{}{
			"file_path": "/test/file.go",
			"content":   "package main\n\nfunc main() {}",
			"optional":  true,
		},
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := validator.ValidateParameters(ctx, call, schema)
		if !result.IsValid {
			b.Fatalf("Unexpected validation failure")
		}
	}
}

// BenchmarkToolValidator_NormalizeToolName benchmarks tool name normalization
func BenchmarkToolValidator_NormalizeToolName(b *testing.B) {
	validator := types.NewStandardToolValidator()
	
	testNames := []string{
		"WebSearch", "web_search", "WEB_SEARCH",
		"Read", "read", "read_file", "READ_FILE",
		"MultiEdit", "multiedit", "multi_edit", "MULTI_EDIT",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := testNames[i%len(testNames)]
		_, found := validator.NormalizeToolName(name)
		if !found {
			b.Fatalf("Expected to find tool name: %s", name)
		}
	}
}