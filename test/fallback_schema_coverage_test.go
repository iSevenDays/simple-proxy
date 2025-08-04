package test

import (
	"claude-proxy/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFallbackSchemaCoverage tests that all common Claude Code tools have fallback schemas
// This addresses the "Unknown tool" errors that cause retry loops
func TestFallbackSchemaCoverage(t *testing.T) {
	// Test cases for all common Claude Code tools that can appear in corrupted requests
	testCases := []struct {
		toolName           string
		expectedName       string
		expectedRequired   []string
		mustHaveProperties []string
		description        string
	}{
		// WebSearch variants (already implemented)
		{"websearch", "WebSearch", []string{"query"}, []string{"query", "allowed_domains", "blocked_domains"}, "WebSearch tool"},
		{"web_search", "WebSearch", []string{"query"}, []string{"query", "allowed_domains", "blocked_domains"}, "WebSearch with underscore"},
		
		// File operations
		{"read", "Read", []string{"file_path"}, []string{"file_path", "limit", "offset"}, "Read file tool"},
		{"read_file", "Read", []string{"file_path"}, []string{"file_path", "limit", "offset"}, "Read file with underscore"},
		{"write", "Write", []string{"file_path", "content"}, []string{"file_path", "content"}, "Write file tool"},
		{"write_file", "Write", []string{"file_path", "content"}, []string{"file_path", "content"}, "Write file with underscore"},
		{"edit", "Edit", []string{"file_path", "old_string", "new_string"}, []string{"file_path", "old_string", "new_string", "replace_all"}, "Edit file tool"},
		
		// Command execution
		{"bash", "Bash", []string{"command"}, []string{"command", "description", "timeout"}, "Bash command tool"},
		{"bash_command", "Bash", []string{"command"}, []string{"command", "description", "timeout"}, "Bash command with underscore"},
		
		// Search tools
		{"grep", "Grep", []string{"pattern"}, []string{"pattern", "path", "glob", "type", "output_mode"}, "Grep search tool"},
		{"grep_search", "Grep", []string{"pattern"}, []string{"pattern", "path", "glob", "type", "output_mode"}, "Grep search with underscore"},
		{"glob", "Glob", []string{"pattern"}, []string{"pattern", "path"}, "Glob file matching tool"},
		
		// Directory operations  
		{"ls", "LS", []string{"path"}, []string{"path", "ignore"}, "List directory tool"},
		
		// Agent tools
		{"task", "Task", []string{"description", "prompt"}, []string{"description", "prompt"}, "Task agent tool"},
		{"todowrite", "TodoWrite", []string{"todos"}, []string{"todos"}, "Todo management tool"},
		
		// Web operations
		{"webfetch", "WebFetch", []string{"url", "prompt"}, []string{"url", "prompt"}, "Web fetch tool"},
	}

	for _, tc := range testCases {
		t.Run(tc.toolName+"_fallback_schema", func(t *testing.T) {
			// Test that fallback schema exists
			fallbackTool := types.GetFallbackToolSchema(tc.toolName)
			require.NotNil(t, fallbackTool, "Should have fallback schema for %s", tc.toolName)
			
			// Test tool name mapping
			assert.Equal(t, tc.expectedName, fallbackTool.Name, "Tool name should be properly mapped")
			
			// Test schema validity
			assert.Equal(t, "object", fallbackTool.InputSchema.Type, "Schema should have object type")
			assert.NotNil(t, fallbackTool.InputSchema.Properties, "Schema should have properties")
			assert.NotEmpty(t, fallbackTool.Description, "Tool should have description")
			
			// Test required parameters
			assert.Equal(t, tc.expectedRequired, fallbackTool.InputSchema.Required, "Required parameters should match")
			
			// Test that all required properties exist
			for _, prop := range tc.mustHaveProperties {
				assert.Contains(t, fallbackTool.InputSchema.Properties, prop, "Should have %s property", prop)
				propDef := fallbackTool.InputSchema.Properties[prop]
				assert.NotEmpty(t, propDef.Type, "Property %s should have type", prop)
				assert.NotEmpty(t, propDef.Description, "Property %s should have description", prop)
			}
		})
	}
}

// TestFallbackSchemaConsistency tests that fallback schemas are consistent with existing patterns
func TestFallbackSchemaConsistency(t *testing.T) {
	testCases := []struct {
		toolName     string
		variants     []string
		description  string
	}{
		{"WebSearch", []string{"websearch", "web_search"}, "WebSearch variants should return same schema"},
		{"Read", []string{"read", "read_file"}, "Read variants should return same schema"},
		{"Write", []string{"write", "write_file"}, "Write variants should return same schema"},
		{"Bash", []string{"bash", "bash_command"}, "Bash variants should return same schema"},
		{"Grep", []string{"grep", "grep_search"}, "Grep variants should return same schema"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.toolName+"_consistency", func(t *testing.T) {
			var schemas []*types.Tool
			
			// Get schemas for all variants
			for _, variant := range tc.variants {
				schema := types.GetFallbackToolSchema(variant)
				require.NotNil(t, schema, "Should have schema for variant %s", variant)
				schemas = append(schemas, schema)
			}
			
			// All variants should map to same tool name
			baseName := schemas[0].Name
			for i, schema := range schemas {
				assert.Equal(t, baseName, schema.Name, "Variant %s should map to %s", tc.variants[i], baseName)
				assert.Equal(t, schemas[0].InputSchema.Required, schema.InputSchema.Required, "Required params should be consistent")
				assert.Equal(t, len(schemas[0].InputSchema.Properties), len(schema.InputSchema.Properties), "Property count should be consistent")
			}
		})
	}
}

// TestUnknownToolFallback tests behavior for truly unknown tools
func TestUnknownToolFallback(t *testing.T) {
	unknownTools := []string{"UnknownTool", "FakeTool", "NonExistentTool", ""}
	
	for _, toolName := range unknownTools {
		t.Run("unknown_tool_"+toolName, func(t *testing.T) {
			fallbackTool := types.GetFallbackToolSchema(toolName)
			assert.Nil(t, fallbackTool, "Unknown tools should return nil, got schema for %s", toolName)
		})
	}
}