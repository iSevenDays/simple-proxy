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
	service := correction.NewService(NewMockConfigProvider("http://test"), "test-key", true, "test-model", false)
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
	service := correction.NewService(NewMockConfigProvider("http://test"), "test-key", true, "test-model", false)
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
	service := correction.NewService(NewMockConfigProvider("http://test"), "test-key", true, "test-model", false)
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

// TestMultiEditStructuralCorrection tests correction of MultiEdit calls where file_path is incorrectly nested in edits
func TestMultiEditStructuralCorrection(t *testing.T) {
	service := correction.NewService(NewMockConfigProvider("http://test"), "test-key", true, "test-model", false)
	ctx := context.WithValue(context.Background(), internal.RequestIDKey, "test-req")
	
	// Get fallback schema
	fallbackSchema := types.GetFallbackToolSchema("MultiEdit")
	if fallbackSchema == nil {
		t.Fatal("MultiEdit fallback schema required for this test")
	}
	availableTools := []types.Tool{*fallbackSchema}
	
	tests := []struct {
		name          string
		input         types.Content
		expectFixed   bool
		expectedPath  string
		expectedEdits []map[string]interface{}
	}{
		{
			name: "file_path nested in edits - should be extracted to top level",
			input: types.Content{
				Type: "tool_use",
				ID:   "test-nested",
				Name: "MultiEdit",
				Input: map[string]interface{}{
					"file_path": "/project/main.go",  // This should remain
					"edits": []interface{}{
						map[string]interface{}{
							"file_path":  "/project/other.go",  // This should be removed
							"old_string": "package main",
							"new_string": "package main",
						},
						map[string]interface{}{
							"file_path":  "/project/helper.go",  // This should be removed too
							"old_string": "func helper() {", 
							"new_string": "func helper() {",
						},
					},
				},
			},
			expectFixed:  true,
			expectedPath: "/project/main.go",  // Should keep top-level path
			expectedEdits: []map[string]interface{}{
				{
					"old_string": "package main",
					"new_string": "package main",
				},
				{
					"old_string": "func helper() {",
					"new_string": "func helper() {", 
				},
			},
		},
		{
			name: "file_path only in edits - should be extracted to top level",
			input: types.Content{
				Type: "tool_use",
				ID:   "test-extract",
				Name: "MultiEdit",
				Input: map[string]interface{}{
					"edits": []interface{}{
						map[string]interface{}{
							"file_path":  "/project/source.go",  // Should become top-level
							"old_string": "import \"fmt\"",
							"new_string": "import \"fmt\"",
						},
						map[string]interface{}{
							"file_path":  "/project/source.go",  // Same path, should be removed
							"old_string": "func main() {",
							"new_string": "func main() {",
						},
					},
				},
			},
			expectFixed:  true,
			expectedPath: "/project/source.go",  // Extracted from edits
			expectedEdits: []map[string]interface{}{
				{
					"old_string": "import \"fmt\"",
					"new_string": "import \"fmt\"",
				},
				{
					"old_string": "func main() {",
					"new_string": "func main() {",
				},
			},
		},
		{
			name: "Anonymized real log example - matches actual failure case",
			input: types.Content{
				Type: "tool_use", 
				ID:   "test-real-case",
				Name: "MultiEdit",
				Input: map[string]interface{}{
					"file_path": "/test/project/handlers.go",
					"edits": []interface{}{
						map[string]interface{}{
							"file_path":  "/test/project/handlers.go",
							"old_string": "func handleRequest() {",
							"new_string": "func handleRequest() {",
						},
						map[string]interface{}{
							"file_path":  "/test/project/service.go",
							"old_string": "func processData() error {",
							"new_string": "func processData() error {",
						},
					},
				},
			},
			expectFixed:  true,
			expectedPath: "/test/project/handlers.go",  // Keep top-level
			expectedEdits: []map[string]interface{}{
				{
					"old_string": "func handleRequest() {",
					"new_string": "func handleRequest() {",
				},
				{
					"old_string": "func processData() error {",
					"new_string": "func processData() error {",
				},
			},
		},
		{
			name: "Multiple file parameter variations - should remove all",
			input: types.Content{
				Type: "tool_use",
				ID:   "test-variations",
				Name: "MultiEdit",
				Input: map[string]interface{}{
					"file_path": "/project/main.go",  // Correct top-level
					"edits": []interface{}{
						map[string]interface{}{
							"filename":   "/project/file1.go",  // Wrong variation 1
							"old_string": "package main",
							"new_string": "package main",
						},
						map[string]interface{}{
							"filepath":   "/project/file2.go",  // Wrong variation 2
							"old_string": "import \"fmt\"",
							"new_string": "import \"fmt\"",
						},
						map[string]interface{}{
							"path":       "/project/file3.go",  // Wrong variation 3
							"old_string": "func test() {",
							"new_string": "func test() {",
						},
					},
				},
			},
			expectFixed:  true,
			expectedPath: "/project/main.go",
			expectedEdits: []map[string]interface{}{
				{
					"old_string": "package main",
					"new_string": "package main",
				},
				{
					"old_string": "import \"fmt\"",
					"new_string": "import \"fmt\"",
				},
				{
					"old_string": "func test() {",
					"new_string": "func test() {",
				},
			},
		},
		{
			name: "Edit with only file_path parameter - should be discarded",
			input: types.Content{
				Type: "tool_use",
				ID:   "test-discard-empty",
				Name: "MultiEdit",
				Input: map[string]interface{}{
					"file_path": "/project/main.go",
					"edits": []interface{}{
						map[string]interface{}{
							"old_string": "package main",
							"new_string": "package main",
						},
						map[string]interface{}{
							"file_path": "/project/other.go",  // Only has file_path, no old_string/new_string
						},
						map[string]interface{}{
							"old_string": "import \"fmt\"",
							"new_string": "import \"fmt\"",
							"file_path":  "/project/helper.go",  // Has required params + file_path
						},
					},
				},
			},
			expectFixed:  true,
			expectedPath: "/project/main.go",
			expectedEdits: []map[string]interface{}{
				{
					"old_string": "package main",
					"new_string": "package main",
				},
				{
					"old_string": "import \"fmt\"",
					"new_string": "import \"fmt\"",
				},
			},
		},
		{
			name: "Edit becomes empty after removing file_path - should be discarded (real log case)",
			input: types.Content{
				Type: "tool_use",
				ID:   "test-real-empty-case", 
				Name: "MultiEdit",
				Input: map[string]interface{}{
					"file_path": "/project/handlers_test.go",
					"edits": []interface{}{
						map[string]interface{}{
							"old_string": "func TestInstallApp() {",
							"new_string": "func TestInstallApp() {",
						},
						map[string]interface{}{
							"old_string": "func executeInstallAppRequest() {",
							"new_string": "func executeInstallAppRequest() {",
						},
						map[string]interface{}{
							"file_path": "/project/handlers_test.go",  // Only file_path - becomes empty after removal
						},
					},
				},
			},
			expectFixed:  true,
			expectedPath: "/project/handlers_test.go",
			expectedEdits: []map[string]interface{}{
				{
					"old_string": "func TestInstallApp() {",
					"new_string": "func TestInstallApp() {",
				},
				{
					"old_string": "func executeInstallAppRequest() {",
					"new_string": "func executeInstallAppRequest() {",
				},
			},
		},
		{
			name: "All edits only have file_path - correction should fail",
			input: types.Content{
				Type: "tool_use",
				ID:   "test-all-invalid",
				Name: "MultiEdit",
				Input: map[string]interface{}{
					"file_path": "/project/main.go",
					"edits": []interface{}{
						map[string]interface{}{
							"file_path": "/project/file1.go",  // Only file_path
						},
						map[string]interface{}{
							"file_path": "/project/file2.go",  // Only file_path
						},
					},
				},
			},
			expectFixed: false,  // Should fail because no valid edits remain
		},
		{
			name: "Already correct structure - no changes needed",
			input: types.Content{
				Type: "tool_use",
				ID:   "test-correct",
				Name: "MultiEdit",
				Input: map[string]interface{}{
					"file_path": "/project/main.go",
					"edits": []interface{}{
						map[string]interface{}{
							"old_string": "package main",
							"new_string": "package main", 
						},
					},
				},
			},
			expectFixed: false,  // No correction needed
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First, verify the input is invalid (for cases that should be fixed)
			if tt.expectFixed {
				validation := service.ValidateToolCall(ctx, tt.input, availableTools)
				if validation.IsValid {
					t.Errorf("Expected input to be invalid, but validation passed")
				}
				// Verify we detected structural issues with nested file/path parameters
				if len(validation.InvalidParams) == 0 {
					t.Errorf("Expected validation to detect invalid nested parameters, got none")
				}
			}
			
			// Attempt the rule-based correction
			result, fixed := service.AttemptRuleBasedMultiEditCorrection(ctx, tt.input)
			
			if fixed != tt.expectFixed {
				t.Errorf("Expected fixed=%v, got %v", tt.expectFixed, fixed)
			}
			
			if tt.expectFixed {
				// Verify the corrected structure
				if actualPath, exists := result.Input["file_path"]; !exists {
					t.Error("Expected file_path parameter in corrected result")
				} else if actualPath != tt.expectedPath {
					t.Errorf("Expected file_path=%s, got %s", tt.expectedPath, actualPath)
				}
				
				if actualEdits, exists := result.Input["edits"]; !exists {
					t.Error("Expected edits parameter in corrected result")
				} else {
					editsArray, ok := actualEdits.([]interface{})
					if !ok {
						t.Error("Expected edits to be an array")
					} else if len(editsArray) != len(tt.expectedEdits) {
						t.Errorf("Expected %d edits, got %d", len(tt.expectedEdits), len(editsArray))
					} else {
						// Check each edit doesn't have file_path
						for i, edit := range editsArray {
							editMap, ok := edit.(map[string]interface{})
							if !ok {
								t.Errorf("Edit %d is not a map", i)
								continue
							}
							if _, hasFilePath := editMap["file_path"]; hasFilePath {
								t.Errorf("Edit %d still has file_path parameter - correction failed", i)
							}
							// Verify expected content
							if i < len(tt.expectedEdits) {
								expected := tt.expectedEdits[i]
								for key, expectedValue := range expected {
									if actualValue, exists := editMap[key]; !exists {
										t.Errorf("Edit %d missing expected key %s", i, key)
									} else if actualValue != expectedValue {
										t.Errorf("Edit %d key %s: expected %v, got %v", i, key, expectedValue, actualValue)
									}
								}
							}
						}
					}
				}
				
				// Verify the corrected call passes validation  
				validation := service.ValidateToolCall(ctx, result, availableTools)
				if !validation.IsValid {
					t.Errorf("Corrected call failed validation. Missing: %v, Invalid: %v", 
						validation.MissingParams, validation.InvalidParams)
				}
			}
		})
	}
}