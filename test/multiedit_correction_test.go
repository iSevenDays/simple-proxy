package test

import (
	"claude-proxy/correction"
	"claude-proxy/internal"
	"claude-proxy/types"
	"context"
	"fmt"
	"testing"
)

// TestMultiEditFallbackSchema tests that MultiEdit has a proper fallback schema
func TestMultiEditFallbackSchema(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		expected bool
	}{
		{"MultiEdit exact case", "MultiEdit", true},
		{"multiedit lowercase", "multiedit", true},
		{"MULTIEDIT uppercase", "MULTIEDIT", true},
		{"multi_edit underscore", "multi_edit", true},
		{"unknown tool", "UnknownTool", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := types.GetFallbackToolSchema(tt.toolName)
			
			if tt.expected {
				if schema == nil {
					t.Errorf("Expected schema for %s, got nil", tt.toolName)
					return
				}
				
				// Verify schema structure
				if schema.Name != "MultiEdit" {
					t.Errorf("Expected name 'MultiEdit', got '%s'", schema.Name)
				}
				
				// Verify required parameters
				expectedRequired := []string{"file_path", "edits"}
				if len(schema.InputSchema.Required) != len(expectedRequired) {
					t.Errorf("Expected %d required params, got %d", len(expectedRequired), len(schema.InputSchema.Required))
				}
				
				for _, req := range expectedRequired {
					found := false
					for _, actual := range schema.InputSchema.Required {
						if actual == req {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Required parameter '%s' not found in schema", req)
					}
				}
				
				// Verify properties exist
				if _, exists := schema.InputSchema.Properties["file_path"]; !exists {
					t.Error("file_path property not found in schema")
				}
				if _, exists := schema.InputSchema.Properties["edits"]; !exists {
					t.Error("edits property not found in schema")
				}
				
				// Verify edits is array type
				editsProperty := schema.InputSchema.Properties["edits"]
				if editsProperty.Type != "array" {
					t.Errorf("Expected edits type 'array', got '%s'", editsProperty.Type)
				}
				
			} else {
				if schema != nil {
					t.Errorf("Expected nil schema for %s, got %+v", tt.toolName, schema)
				}
			}
		})
	}
}

// TestMultiEditValidation tests MultiEdit tool call validation
func TestMultiEditValidation(t *testing.T) {
	service := correction.NewService("http://test", "test-key", true, "test-model", false)
	ctx := context.WithValue(context.Background(), internal.RequestIDKey, "test-req")
	
	// Get fallback schema for MultiEdit
	fallbackSchema := types.GetFallbackToolSchema("MultiEdit")
	if fallbackSchema == nil {
		t.Fatal("MultiEdit fallback schema not found - implement GetFallbackToolSchema first")
	}
	
	availableTools := []types.Tool{*fallbackSchema}
	
	tests := []struct {
		name       string
		toolCall   types.Content
		wantValid  bool
		wantIssues []string // missing or invalid parameters
	}{
		{
			name: "Valid MultiEdit call",
			toolCall: types.Content{
				Type: "tool_use",
				ID:   "test-1",
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
			},
			wantValid: true,
		},
		{
			name: "Missing file_path parameter",
			toolCall: types.Content{
				Type: "tool_use",
				ID:   "test-2", 
				Name: "MultiEdit",
				Input: map[string]interface{}{
					"edits": []interface{}{
						map[string]interface{}{
							"old_string": "old text",
							"new_string": "new text",
						},
					},
				},
			},
			wantValid:  false,
			wantIssues: []string{"file_path"},
		},
		{
			name: "Missing edits parameter",
			toolCall: types.Content{
				Type: "tool_use",
				ID:   "test-3",
				Name: "MultiEdit", 
				Input: map[string]interface{}{
					"file_path": "/test/file.go",
				},
			},
			wantValid:  false,
			wantIssues: []string{"edits"},
		},
		{
			name: "Invalid extra parameter",
			toolCall: types.Content{
				Type: "tool_use",
				ID:   "test-4",
				Name: "MultiEdit",
				Input: map[string]interface{}{
					"file_path":    "/test/file.go",
					"edits":        []interface{}{},
					"extra_param":  "should not be here",
				},
			},
			wantValid:  false,
			wantIssues: []string{"extra_param"},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.ValidateToolCall(ctx, tt.toolCall, availableTools)
			
			if result.IsValid != tt.wantValid {
				t.Errorf("Expected IsValid=%v, got %v", tt.wantValid, result.IsValid)
			}
			
			if tt.wantIssues != nil {
				// Check for expected missing parameters
				for _, expectedMissing := range tt.wantIssues {
					foundInMissing := false
					for _, missing := range result.MissingParams {
						if missing == expectedMissing {
							foundInMissing = true
							break
						}
					}
					foundInInvalid := false
					for _, invalid := range result.InvalidParams {
						if invalid == expectedMissing {
							foundInInvalid = true
							break
						}
					}
					
					if !foundInMissing && !foundInInvalid {
						t.Errorf("Expected parameter issue '%s' not found in MissingParams:%v or InvalidParams:%v", 
							expectedMissing, result.MissingParams, result.InvalidParams)
					}
				}
			}
		})
	}
}

// TestMultiEditRuleBasedCorrection tests rule-based parameter corrections for MultiEdit
func TestMultiEditRuleBasedCorrection(t *testing.T) {
	service := correction.NewService("http://test", "test-key", true, "test-model", false)
	ctx := context.WithValue(context.Background(), internal.RequestIDKey, "test-req")
	
	tests := []struct {
		name        string
		input       types.Content
		expectFixed bool
		expectedParams map[string]interface{}
	}{
		{
			name: "Correct filename to file_path",
			input: types.Content{
				Type: "tool_use",
				ID:   "test-1",
				Name: "MultiEdit",
				Input: map[string]interface{}{
					"filename": "/test/file.go",  // Wrong parameter name
					"edits": []interface{}{
						map[string]interface{}{
							"old_string": "old",
							"new_string": "new",
						},
					},
				},
			},
			expectFixed: true,
			expectedParams: map[string]interface{}{
				"file_path": "/test/file.go",  // Corrected parameter name
				"edits": []interface{}{
					map[string]interface{}{
						"old_string": "old",
						"new_string": "new",
					},
				},
			},
		},
		{
			name: "Correct path to file_path",
			input: types.Content{
				Type: "tool_use",
				ID:   "test-2",
				Name: "MultiEdit",
				Input: map[string]interface{}{
					"path": "/test/file.go",  // Wrong parameter name
					"edits": []interface{}{},
				},
			},
			expectFixed: true,
			expectedParams: map[string]interface{}{
				"file_path": "/test/file.go",  // Corrected parameter name
				"edits": []interface{}{},
			},
		},
		{
			name: "Already correct parameters - no change",
			input: types.Content{
				Type: "tool_use",
				ID:   "test-3", 
				Name: "MultiEdit",
				Input: map[string]interface{}{
					"file_path": "/test/file.go",  // Already correct
					"edits": []interface{}{},
				},
			},
			expectFixed: false,  // No changes needed
		},
		{
			name: "Real schema validation - exact Claude Code format",
			input: types.Content{
				Type: "tool_use",
				ID:   "test-4",
				Name: "MultiEdit",
				Input: map[string]interface{}{
					"file_path": "/absolute/path/to/file.go",
					"edits": []interface{}{
						map[string]interface{}{
							"old_string": "original text",
							"new_string": "replacement text",
							"replace_all": false,
						},
						map[string]interface{}{
							"old_string": "another text",
							"new_string": "new text",
						},
					},
				},
			},
			expectFixed: false,  // Already matches exact Claude Code schema
		},
		{
			name: "Handle common misspelling - filepath",
			input: types.Content{
				Type: "tool_use",
				ID:   "test-5",
				Name: "MultiEdit",
				Input: map[string]interface{}{
					"filepath": "/test/file.go",  // Common misspelling
					"edits": []interface{}{},
				},
			},
			expectFixed: true,  // This should be corrected: filepath -> file_path
			expectedParams: map[string]interface{}{
				"file_path": "/test/file.go",  // Corrected parameter name
				"edits": []interface{}{},
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, fixed := service.AttemptRuleBasedParameterCorrection(ctx, tt.input)
			
			if fixed != tt.expectFixed {
				t.Errorf("Expected fixed=%v, got %v", tt.expectFixed, fixed)
			}
			
			if tt.expectFixed && tt.expectedParams != nil {
				for key, expectedValue := range tt.expectedParams {
					if actualValue, exists := result.Input[key]; !exists {
						t.Errorf("Expected parameter '%s' not found in corrected input", key)
					} else {
						// For simple comparison, convert to string
						if fmt.Sprintf("%v", actualValue) != fmt.Sprintf("%v", expectedValue) {
							t.Errorf("Parameter '%s': expected %v, got %v", key, expectedValue, actualValue)
						}
					}
				}
			}
		})
	}
}

// TestMultiEditMalformedStructureCorrection tests correction of malformed MultiEdit calls like the original error
func TestMultiEditMalformedStructureCorrection(t *testing.T) {
	service := correction.NewService("http://test", "test-key", true, "test-model", false)
	ctx := context.WithValue(context.Background(), internal.RequestIDKey, "test-req")
	
	// Get fallback schema
	fallbackSchema := types.GetFallbackToolSchema("MultiEdit")
	if fallbackSchema == nil {
		t.Fatal("MultiEdit fallback schema required for this test")
	}
	availableTools := []types.Tool{*fallbackSchema}
	
	// Test the original error case: MultiEdit with file_path at wrong level
	malformedCall := types.Content{
		Type: "tool_use",
		ID:   "test-malformed",
		Name: "MultiEdit",
		Input: map[string]interface{}{
			"file_path": "/path/to/file.go",  // This is correct for MultiEdit
			"edits": []interface{}{
				map[string]interface{}{
					"old_string": "old text",
					"new_string": "new text",
				},
			},
		},
	}
	
	t.Run("Malformed structure validation", func(t *testing.T) {
		// This should pass validation since file_path at top level is correct for MultiEdit
		result := service.ValidateToolCall(ctx, malformedCall, availableTools)
		
		if !result.IsValid {
			t.Errorf("Expected valid MultiEdit call, got invalid. Missing: %v, Invalid: %v", 
				result.MissingParams, result.InvalidParams)
		}
	})
	
	t.Run("Invalid parameter structure", func(t *testing.T) {
		// Test a truly invalid structure
		invalidCall := types.Content{
			Type: "tool_use",
			ID:   "test-invalid",
			Name: "MultiEdit", 
			Input: map[string]interface{}{
				"file_path": "/path/to/file.go",
				"edits": []interface{}{
					map[string]interface{}{
						"old_string": "old text",
						"new_string": "new text",
					},
				},
				"unexpected_param": "this should be invalid",  // This parameter doesn't exist in schema
			},
		}
		
		result := service.ValidateToolCall(ctx, invalidCall, availableTools)
		
		if result.IsValid {
			t.Error("Expected invalid call due to unexpected parameter")
		}
		
		if len(result.InvalidParams) == 0 {
			t.Error("Expected invalid parameters to be detected")
		}
		
		found := false
		for _, invalid := range result.InvalidParams {
			if invalid == "unexpected_param" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected 'unexpected_param' in invalid parameters, got: %v", result.InvalidParams)
		}
	})
}