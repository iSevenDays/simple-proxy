package types

import (
	"context"
	"strings"
)

// ToolValidator interface for tool call validation
// Extracted from correction service to enable better testing and reusability
type ToolValidator interface {
	// ValidateParameters performs basic parameter validation against a tool schema
	ValidateParameters(ctx context.Context, call Content, schema ToolSchema) ValidationResult
	
	// NormalizeToolName handles case-insensitive tool name resolution
	NormalizeToolName(toolName string) (normalized string, found bool)
}

// ValidationResult represents the outcome of tool validation
type ValidationResult struct {
	IsValid         bool
	MissingParams   []string
	InvalidParams   []string
	HasCaseIssue    bool
	CorrectToolName string
}

// StandardToolValidator is the default implementation of ToolValidator
type StandardToolValidator struct {
	// Tool name mappings for case-insensitive and alias resolution
	toolNameMappings map[string]string
}

// NewStandardToolValidator creates a new StandardToolValidator with default mappings
func NewStandardToolValidator() *StandardToolValidator {
	return &StandardToolValidator{
		toolNameMappings: map[string]string{
			// Exact matches (canonical names)
			"websearch":  "WebSearch",
			"read":       "Read",
			"write":      "Write", 
			"edit":       "Edit",
			"multiedit":  "MultiEdit",
			"bash":       "Bash",
			"grep":       "Grep",
			"glob":       "Glob",
			"ls":         "LS",
			"task":       "Task",
			"todowrite":  "TodoWrite",
			"webfetch":   "WebFetch",
			
			// Alias mappings
			"web_search": "WebSearch",
			"read_file":  "Read",
			"write_file": "Write",
			"multi_edit": "MultiEdit",
			"bash_command": "Bash",
			"grep_search": "Grep",
		},
	}
}

// ValidateParameters performs comprehensive parameter validation
func (v *StandardToolValidator) ValidateParameters(ctx context.Context, call Content, schema ToolSchema) ValidationResult {
	result := ValidationResult{
		IsValid:       false,
		MissingParams: []string{},
		InvalidParams: []string{},
	}
	
	// Check required parameters
	for _, required := range schema.Required {
		if _, exists := call.Input[required]; !exists {
			result.MissingParams = append(result.MissingParams, required)
		}
	}
	
	// Check for invalid parameters
	for param := range call.Input {
		if _, exists := schema.Properties[param]; !exists {
			result.InvalidParams = append(result.InvalidParams, param)
		}
	}
	
	// Tool is valid if no missing or invalid parameters
	result.IsValid = len(result.MissingParams) == 0 && len(result.InvalidParams) == 0
	
	return result
}

// NormalizeToolName resolves tool names to their canonical form
func (v *StandardToolValidator) NormalizeToolName(toolName string) (normalized string, found bool) {
	// Try exact match first
	if canonical, exists := v.toolNameMappings[toolName]; exists {
		return canonical, true
	}
	
	// Try case-insensitive match
	lowerName := strings.ToLower(toolName)
	if canonical, exists := v.toolNameMappings[lowerName]; exists {
		return canonical, true
	}
	
	// Try exact match with different casing patterns for common tools
	switch strings.ToLower(toolName) {
	case "websearch":
		return "WebSearch", true
	case "web_search":
		return "WebSearch", true
	case "read":
		return "Read", true
	case "read_file":
		return "Read", true
	case "write":
		return "Write", true
	case "write_file":
		return "Write", true
	case "edit":
		return "Edit", true
	case "multiedit":
		return "MultiEdit", true
	case "multi_edit":
		return "MultiEdit", true
	case "bash":
		return "Bash", true
	case "bash_command":
		return "Bash", true
	case "grep":
		return "Grep", true
	case "grep_search":
		return "Grep", true
	case "glob":
		return "Glob", true
	case "ls":
		return "LS", true
	case "task":
		return "Task", true
	case "todowrite":
		return "TodoWrite", true
	case "webfetch":
		return "WebFetch", true
	}
	
	// No match found
	return "", false
}