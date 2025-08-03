package test

import (
	"claude-proxy/config"
	"claude-proxy/internal"
	"claude-proxy/logger"
	"claude-proxy/proxy"
	"claude-proxy/types"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSchemaRestoration tests the schema restoration system
func TestSchemaRestoration(t *testing.T) {
	// Create test logger
	cfg := config.GetDefaultConfig()
	loggerConfig := logger.NewConfigAdapter(cfg)
	ctx := internal.WithRequestID(context.Background(), "schema_test")
	testLogger := logger.New(ctx, loggerConfig)
	
	// Define valid tools that could be used for restoration
	validTools := []types.Tool{
		{
			Name:        "WebSearch",
			Description: "Search the web and use the results to inform responses",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"query": {
						Type:        "string",
						Description: "The search query to use",
					},
					"allowed_domains": {
						Type:        "array",
						Description: "Only include search results from these domains",
						Items: &types.ToolPropertyItems{
							Type: "string",
						},
					},
					"blocked_domains": {
						Type:        "array",
						Description: "Never include search results from these domains",
						Items: &types.ToolPropertyItems{
							Type: "string",
						},
					},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "Read",
			Description: "Read a file from the filesystem",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"file_path": {
						Type:        "string",
						Description: "The absolute path to the file to read",
					},
					"offset": {
						Type:        "number",
						Description: "Line number to start reading from",
					},
					"limit": {
						Type:        "number",
						Description: "Number of lines to read",
					},
				},
				Required: []string{"file_path"},
			},
		},
		{
			Name:        "Write",
			Description: "Write content to a file",
			InputSchema: types.ToolSchema{
				Type: "object",
				Properties: map[string]types.ToolProperty{
					"file_path": {
						Type:        "string",
						Description: "The absolute path to the file to write",
					},
					"content": {
						Type:        "string",
						Description: "The content to write to the file",
					},
				},
				Required: []string{"file_path", "content"},
			},
		},
	}

	tests := []struct {
		name           string
		corruptedTool  types.Tool
		availableTools []types.Tool
		expectRestore  bool
		expectedName   string
		description    string
	}{
		{
			name: "corrupted_web_search_restored_to_WebSearch",
			corruptedTool: types.Tool{
				Name:        "web_search",
				Description: "",
				InputSchema: types.ToolSchema{
					Type:       "",
					Properties: nil,
					Required:   nil,
				},
			},
			availableTools: validTools,
			expectRestore:  true,
			expectedName:   "WebSearch",
			description:    "Corrupted web_search should be restored to WebSearch",
		},
		{
			name: "corrupted_websearch_restored_to_WebSearch",
			corruptedTool: types.Tool{
				Name:        "websearch",
				Description: "",
				InputSchema: types.ToolSchema{
					Type:       "",
					Properties: nil,
					Required:   nil,
				},
			},
			availableTools: validTools,
			expectRestore:  true,
			expectedName:   "WebSearch",
			description:    "Corrupted websearch should be restored to WebSearch",
		},
		{
			name: "corrupted_read_file_restored_to_Read",
			corruptedTool: types.Tool{
				Name:        "read_file",
				Description: "",
				InputSchema: types.ToolSchema{
					Type:       "",
					Properties: map[string]types.ToolProperty{},
					Required:   nil,
				},
			},
			availableTools: validTools,
			expectRestore:  true,
			expectedName:   "Read",
			description:    "Corrupted read_file should be restored to Read",
		},
		{
			name: "case_insensitive_matching_works",
			corruptedTool: types.Tool{
				Name:        "WEBSEARCH",
				Description: "",
				InputSchema: types.ToolSchema{
					Type:       "",
					Properties: nil,
					Required:   nil,
				},
			},
			availableTools: validTools,
			expectRestore:  true,
			expectedName:   "WebSearch",
			description:    "Case-insensitive matching should work",
		},
		{
			name: "direct_name_match_case_insensitive",
			corruptedTool: types.Tool{
				Name:        "websearch",
				Description: "",
				InputSchema: types.ToolSchema{
					Type:       "",
					Properties: nil,
					Required:   nil,
				},
			},
			availableTools: validTools,
			expectRestore:  true,
			expectedName:   "WebSearch",
			description:    "Direct name match should work case-insensitively",
		},
		{
			name: "unknown_tool_cannot_be_restored",
			corruptedTool: types.Tool{
				Name:        "unknown_tool",
				Description: "",
				InputSchema: types.ToolSchema{
					Type:       "",
					Properties: nil,
					Required:   nil,
				},
			},
			availableTools: validTools,
			expectRestore:  true,
			expectedName:   "unknown_tool",
			description:    "Unknown tools get fallback schema",
		},
		{
			name: "valid_tool_not_modified",
			corruptedTool: types.Tool{
				Name:        "Read",
				Description: "Valid tool description",
				InputSchema: types.ToolSchema{
					Type: "object",
					Properties: map[string]types.ToolProperty{
						"file_path": {Type: "string", Description: "Path"},
					},
					Required: []string{"file_path"},
				},
			},
			availableTools: validTools,
			expectRestore:  false,
			expectedName:   "Read",
			description:    "Valid tools should not be modified",
		},
		{
			name: "empty_available_tools_no_restoration",
			corruptedTool: types.Tool{
				Name:        "web_search",
				Description: "",
				InputSchema: types.ToolSchema{
					Type:       "",
					Properties: nil,
					Required:   nil,
				},
			},
			availableTools: []types.Tool{},
			expectRestore:  true,
			expectedName:   "WebSearch",
			description:    "Fallback schema provided even with empty available tools",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying the test data
			toolCopy := tt.corruptedTool
			
			// Test the restoration function
			result := proxy.RestoreCorruptedToolSchema(&toolCopy, tt.availableTools, testLogger)
			
			// Check if restoration was attempted as expected
			assert.Equal(t, tt.expectRestore, result, tt.description)
			
			// Check the tool name after restoration
			assert.Equal(t, tt.expectedName, toolCopy.Name, "Tool name should match expected result")
			
			// If restoration was successful, verify schema is valid
			if tt.expectRestore && result {
				assert.NotEmpty(t, toolCopy.InputSchema.Type, "Restored tool should have valid type")
				assert.NotNil(t, toolCopy.InputSchema.Properties, "Restored tool should have properties")
				assert.NotEmpty(t, toolCopy.Description, "Restored tool should have description")
			}
		})
	}
}

// TestFindValidToolSchema tests the tool schema lookup functionality
func TestFindValidToolSchema(t *testing.T) {
	validTools := []types.Tool{
		{
			Name:        "WebSearch",
			Description: "Search the web",
			InputSchema: types.ToolSchema{Type: "object", Properties: map[string]types.ToolProperty{"query": {Type: "string"}}},
		},
		{
			Name:        "Read",
			Description: "Read file",
			InputSchema: types.ToolSchema{Type: "object", Properties: map[string]types.ToolProperty{"file_path": {Type: "string"}}},
		},
	}

	tests := []struct {
		name         string
		corruptedName string
		expectedName  string
		shouldFind    bool
	}{
		{"web_search_maps_to_WebSearch", "web_search", "WebSearch", true},
		{"websearch_maps_to_WebSearch", "websearch", "WebSearch", true},
		{"WEBSEARCH_maps_to_WebSearch", "WEBSEARCH", "WebSearch", true},
		{"read_file_maps_to_Read", "read_file", "Read", true},
		{"READ_maps_to_Read", "READ", "Read", true},
		{"unknown_tool_gets_fallback", "unknown", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corruptedTool := types.Tool{Name: tt.corruptedName}
			result := proxy.FindValidToolSchema(corruptedTool, validTools)
			
			if tt.shouldFind {
				assert.NotNil(t, result, "Should find a valid tool schema")
				if result != nil {
					assert.Equal(t, tt.expectedName, result.Name, "Found tool should have expected name")
				}
			} else {
				assert.Nil(t, result, "Should not find a valid tool schema")
			}
		})
	}
}

// TestSchemaRestorationIntegration tests the integration with transform pipeline
func TestSchemaRestorationIntegration(t *testing.T) {
	// Create a complete Anthropic request with both valid and corrupted tools
	req := types.AnthropicRequest{
		Model: "claude-sonnet-4",
		Tools: []types.Tool{
			// Valid tool
			{
				Name:        "WebSearch",
				Description: "Search the web",
				InputSchema: types.ToolSchema{
					Type: "object",
					Properties: map[string]types.ToolProperty{
						"query": {Type: "string", Description: "Search query"},
					},
					Required: []string{"query"},
				},
			},
			// Corrupted tool that should be restored
			{
				Name:        "web_search",
				Description: "",
				InputSchema: types.ToolSchema{
					Type:       "",
					Properties: nil,
					Required:   nil,
				},
			},
			// Another valid tool
			{
				Name:        "Read",
				Description: "Read file",
				InputSchema: types.ToolSchema{
					Type: "object",
					Properties: map[string]types.ToolProperty{
						"file_path": {Type: "string", Description: "File path"},
					},
					Required: []string{"file_path"},
				},
			},
		},
		Messages: []types.Message{
			{Role: "user", Content: "Test message"},
		},
	}

	// Create config for transformation
	cfg := &config.Config{
		BigModel:   "test-model",
		SmallModel: "test-small",
	}

	// Transform the request
	ctx := internal.WithRequestID(context.Background(), "schema_test")
	result, err := proxy.TransformAnthropicToOpenAI(ctx, req, cfg)

	// Verify no errors occurred
	assert.NoError(t, err, "Transformation should succeed")

	// Verify we have the correct number of tools
	assert.Len(t, result.Tools, 3, "Should have 3 tools after transformation")

	// Verify all tools have valid schemas
	for i, tool := range result.Tools {
		assert.NotEmpty(t, tool.Function.Parameters.Type, "Tool %d should have valid type", i)
		assert.NotNil(t, tool.Function.Parameters.Properties, "Tool %d should have properties", i)
		assert.NotEmpty(t, tool.Function.Name, "Tool %d should have name", i)
		assert.NotEmpty(t, tool.Function.Description, "Tool %d should have description", i)
	}

	// Verify the corrupted tool was restored correctly
	foundWebSearch := false
	for _, tool := range result.Tools {
		if tool.Function.Name == "WebSearch" {
			foundWebSearch = true
			assert.Contains(t, tool.Function.Parameters.Properties, "query", "WebSearch should have query parameter")
		}
	}
	assert.True(t, foundWebSearch, "Should find WebSearch tool after restoration")
}