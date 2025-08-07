package types

// SchemaRegistry interface for centralized tool schema management
// Extracted to eliminate duplication and provide centralized access to tool definitions
type SchemaRegistry interface {
	// GetSchema retrieves a tool schema by name
	GetSchema(toolName string) (*Tool, bool)
	
	// ListTools returns all available tool names
	ListTools() []string
	
	// RegisterTool adds or updates a tool schema
	RegisterTool(tool *Tool) error
}

// StandardSchemaRegistry is the default implementation of SchemaRegistry
type StandardSchemaRegistry struct {
	// Tools stores all registered tool schemas
	tools map[string]*Tool
}

// NewStandardSchemaRegistry creates a new StandardSchemaRegistry with all existing fallback tools
func NewStandardSchemaRegistry() *StandardSchemaRegistry {
	registry := &StandardSchemaRegistry{
		tools: make(map[string]*Tool),
	}
	
	// Register all existing fallback tools to maintain backward compatibility
	registry.registerFallbackTools()
	
	return registry
}

// GetSchema retrieves a tool schema by name
func (r *StandardSchemaRegistry) GetSchema(toolName string) (*Tool, bool) {
	tool, exists := r.tools[toolName]
	return tool, exists
}

// ListTools returns all available tool names
func (r *StandardSchemaRegistry) ListTools() []string {
	tools := make([]string, 0, len(r.tools))
	for name := range r.tools {
		tools = append(tools, name)
	}
	return tools
}

// RegisterTool adds or updates a tool schema
func (r *StandardSchemaRegistry) RegisterTool(tool *Tool) error {
	if tool == nil {
		return nil // Ignore nil tools
	}
	r.tools[tool.Name] = tool
	return nil
}

// registerFallbackTools populates the registry with existing fallback tool schemas
// This maintains backward compatibility with the existing GetFallbackToolSchema function
func (r *StandardSchemaRegistry) registerFallbackTools() {
	// All tools from the existing GetFallbackToolSchema function
	fallbackTools := []string{
		"WebSearch", "web_search",
		"Read", "read_file", 
		"Write", "write_file",
		"Edit",
		"MultiEdit", "multi_edit",
		"Bash", "bash_command",
		"Grep", "grep_search",
		"Glob",
		"LS",
		"Task",
		"TodoWrite",
		"WebFetch",
		"ExitPlanMode", "exit_plan_mode",
	}
	
	for _, toolName := range fallbackTools {
		if fallbackTool := GetFallbackToolSchema(toolName); fallbackTool != nil {
			r.tools[toolName] = fallbackTool
		}
	}
}