package test

import (
	"claude-proxy/correction"
	"claude-proxy/internal"
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

// SCHEMA REGISTRY TESTS

// TestSchemaRegistry_Interface defines the contract for centralized schema management
func TestSchemaRegistry_Interface(t *testing.T) {
	// Test that our interface exists and can be implemented
	var _ types.SchemaRegistry = &MockSchemaRegistry{}
	var _ types.SchemaRegistry = types.NewStandardSchemaRegistry()
}

// MockSchemaRegistry for testing
type MockSchemaRegistry struct {
	schemas map[string]*types.Tool
}

func (m *MockSchemaRegistry) GetSchema(toolName string) (*types.Tool, bool) {
	if m.schemas == nil {
		return nil, false
	}
	schema, exists := m.schemas[toolName]
	return schema, exists
}

func (m *MockSchemaRegistry) ListTools() []string {
	if m.schemas == nil {
		return []string{}
	}
	var tools []string
	for name := range m.schemas {
		tools = append(tools, name)
	}
	return tools
}

func (m *MockSchemaRegistry) RegisterTool(tool *types.Tool) error {
	if m.schemas == nil {
		m.schemas = make(map[string]*types.Tool)
	}
	m.schemas[tool.Name] = tool
	return nil
}

// TestSchemaRegistry_BasicOperation tests schema retrieval
func TestSchemaRegistry_BasicOperation(t *testing.T) {
	registry := types.NewStandardSchemaRegistry()
	
	// Test known tool schema retrieval
	schema, exists := registry.GetSchema("Read")
	if !exists {
		t.Error("Expected 'Read' tool to exist in registry")
	}
	if schema == nil {
		t.Error("Expected non-nil schema for 'Read' tool")
	}
	if schema.Name != "Read" {
		t.Errorf("Expected schema name 'Read', got '%s'", schema.Name)
	}
	
	// Test unknown tool
	schema, exists = registry.GetSchema("UnknownTool")
	if exists {
		t.Error("Expected 'UnknownTool' to not exist in registry")
	}
	if schema != nil {
		t.Error("Expected nil schema for unknown tool")
	}
}

// TestSchemaRegistry_BackwardCompatibility tests compatibility with existing GetFallbackToolSchema
func TestSchemaRegistry_BackwardCompatibility(t *testing.T) {
	registry := types.NewStandardSchemaRegistry()
	
	// Compare registry results with existing GetFallbackToolSchema
	testTools := []string{"Read", "Write", "WebSearch", "Task", "Bash"}
	
	for _, toolName := range testTools {
		t.Run(toolName, func(t *testing.T) {
			// Get from registry
			registrySchema, registryExists := registry.GetSchema(toolName)
			
			// Get from existing function
			fallbackSchema := types.GetFallbackToolSchema(toolName)
			fallbackExists := fallbackSchema != nil
			
			// Should have same existence
			if registryExists != fallbackExists {
				t.Errorf("Existence mismatch for '%s': registry=%v, fallback=%v", 
					toolName, registryExists, fallbackExists)
			}
			
			if registryExists && fallbackExists {
				// Should have same basic properties
				if registrySchema.Name != fallbackSchema.Name {
					t.Errorf("Name mismatch for '%s': registry='%s', fallback='%s'",
						toolName, registrySchema.Name, fallbackSchema.Name)
				}
			}
		})
	}
}

// TestSchemaRegistry_RegisterTool tests dynamic tool registration
func TestSchemaRegistry_RegisterTool(t *testing.T) {
	registry := types.NewStandardSchemaRegistry()
	
	// Create a custom tool
	customTool := &types.Tool{
		Name:        "CustomTool",
		Description: "A custom test tool",
		InputSchema: types.ToolSchema{
			Type: "object",
			Properties: map[string]types.ToolProperty{
				"param1": {Type: "string", Description: "Test parameter"},
			},
			Required: []string{"param1"},
		},
	}
	
	// Register the tool
	err := registry.RegisterTool(customTool)
	if err != nil {
		t.Errorf("Failed to register custom tool: %v", err)
	}
	
	// Verify it can be retrieved
	schema, exists := registry.GetSchema("CustomTool")
	if !exists {
		t.Error("Expected registered 'CustomTool' to be retrievable")
	}
	if schema == nil {
		t.Error("Expected non-nil schema for registered tool")
	}
	if schema.Name != "CustomTool" {
		t.Errorf("Expected retrieved tool name 'CustomTool', got '%s'", schema.Name)
	}
}

// SCHEMA REGISTRY INTEGRATION TESTS

// TestSchemaRegistry_CorrectionServiceIntegration tests registry integration with correction service
func TestSchemaRegistry_CorrectionServiceIntegration(t *testing.T) {
	// Create custom registry with mock tool
	registry := types.NewStandardSchemaRegistry()
	
	// Create custom tool for testing
	customTool := &types.Tool{
		Name:        "TestRegistryTool",
		Description: "Test tool for registry integration",
		InputSchema: types.ToolSchema{
			Type: "object",
			Properties: map[string]types.ToolProperty{
				"test_param": {Type: "string", Description: "Test parameter"},
			},
			Required: []string{"test_param"},
		},
	}
	
	// Register the custom tool
	registry.RegisterTool(customTool)
	
	// Create service with custom registry
	validator := types.NewStandardToolValidator()
	mockConfig := NewMockConfigProvider("http://test:8080")
	service := correction.NewServiceWithComponents(
		mockConfig,
		"test-key",
		true,
		"test-model",
		false,
		validator,
		registry,
	)
	
	// Test that service can find tool through registry
	ctx := context.WithValue(context.Background(), internal.RequestIDKey, "test-req")
	
	call := types.Content{
		Type: "tool_use",
		Name: "TestRegistryTool",
		Input: map[string]interface{}{
			"test_param": "test_value",
		},
	}
	
	// Service should find tool through registry and validate successfully
	result := service.ValidateToolCall(ctx, call, []types.Tool{}) // Empty available tools - should use registry
	
	if !result.IsValid {
		t.Errorf("Expected registry tool validation to pass, got: missing=%v, invalid=%v",
			result.MissingParams, result.InvalidParams)
	}
}

// TestSchemaRegistry_BackwardCompatibilityInService tests that existing behavior is preserved
func TestSchemaRegistry_BackwardCompatibilityInService(t *testing.T) {
	// Create service with defaults (backward compatibility)
	mockConfig2 := NewMockConfigProvider("http://test:8080")
	service := correction.NewService(mockConfig2, "test-key", true, "test-model", false)
	
	ctx := context.WithValue(context.Background(), internal.RequestIDKey, "test-req")
	
	// Test with known fallback tool
	call := types.Content{
		Type: "tool_use",
		Name: "Read",
		Input: map[string]interface{}{
			"file_path": "/test/file.go",
		},
	}
	
	// Should work through registry (which loads fallback tools)
	result := service.ValidateToolCall(ctx, call, []types.Tool{}) // Empty available tools
	
	if !result.IsValid {
		t.Errorf("Expected backward compatibility validation to pass, got: missing=%v, invalid=%v",
			result.MissingParams, result.InvalidParams)
	}
}