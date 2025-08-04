package types

import "strings"

// AnthropicRequest represents incoming request from Claude Code
type AnthropicRequest struct {
	Model     string          `json:"model"`
	Messages  []Message       `json:"messages"`
	System    []SystemContent `json:"system,omitempty"`
	Tools     []Tool          `json:"tools,omitempty"`
	MaxTokens int             `json:"max_tokens,omitempty"`
	Stream    bool            `json:"stream,omitempty"`
}

// AnthropicResponse represents response to Claude Code
type AnthropicResponse struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Role         string    `json:"role"`
	Model        string    `json:"model"`
	Content      []Content `json:"content"`
	StopReason   string    `json:"stop_reason"`
	StopSequence *string   `json:"stop_sequence"`
	Usage        Usage     `json:"usage"`
}

// Message represents a chat message
type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // Can be string or []Content
}

// SystemContent represents system message content
type SystemContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Content represents message content (text or tool_use)
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`

	// Tool use fields
	ID    string                 `json:"id,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`

	// Tool result fields
	ToolUseID string `json:"tool_use_id,omitempty"`
}

// Tool represents an Anthropic tool definition
type Tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema ToolSchema `json:"input_schema"`
}

// ToolSchema represents tool parameter schema
type ToolSchema struct {
	Type       string                  `json:"type"`
	Properties map[string]ToolProperty `json:"properties"`
	Required   []string                `json:"required"`
}

type ToolProperty struct {
	Type        string               `json:"type"`
	Description string               `json:"description,omitempty"`
	Items       *ToolPropertyItems   `json:"items,omitempty"`
}

// ToolPropertyItems represents array item schema for tool properties
type ToolPropertyItems struct {
	Type string `json:"type"`
}

// Usage represents token usage
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// GetFallbackToolSchema provides accurate schemas for Claude Code tools when restoration fails
// This prevents "Unknown tool" errors when LLM generates tool calls for valid Claude Code tools
// that weren't included in the original request's tool list
func GetFallbackToolSchema(toolName string) *Tool {
	// Complete coverage of Claude Code tools to prevent "Unknown tool" errors
	switch strings.ToLower(toolName) {
	case "websearch", "web_search":
		return &Tool{
			Name:        "WebSearch",
			Description: "Search the web and use the results to inform responses",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"query": {
						Type:        "string",
						Description: "The search query to use",
					},
					"allowed_domains": {
						Type:        "array",
						Description: "Only include search results from these domains",
						Items:       &ToolPropertyItems{Type: "string"},
					},
					"blocked_domains": {
						Type:        "array", 
						Description: "Never include search results from these domains",
						Items:       &ToolPropertyItems{Type: "string"},
					},
				},
				Required: []string{"query"},
			},
		}
	case "read", "read_file":
		return &Tool{
			Name:        "Read",
			Description: "Reads a file from the local filesystem",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"file_path": {
						Type:        "string",
						Description: "The absolute path to the file to read",
					},
					"limit": {
						Type:        "number",
						Description: "The number of lines to read",
					},
					"offset": {
						Type:        "number",
						Description: "The line number to start reading from",
					},
				},
				Required: []string{"file_path"},
			},
		}
	case "write", "write_file":
		return &Tool{
			Name:        "Write",
			Description: "Writes a file to the local filesystem",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
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
		}
	case "edit":
		return &Tool{
			Name:        "Edit",
			Description: "Performs exact string replacements in files",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"file_path": {
						Type:        "string",
						Description: "The absolute path to the file to modify",
					},
					"old_string": {
						Type:        "string",
						Description: "The text to replace",
					},
					"new_string": {
						Type:        "string",
						Description: "The text to replace it with",
					},
					"replace_all": {
						Type:        "boolean",
						Description: "Replace all occurences of old_string",
					},
				},
				Required: []string{"file_path", "old_string", "new_string"},
			},
		}
	case "bash", "bash_command":
		return &Tool{
			Name:        "Bash",
			Description: "Executes bash commands",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"command": {
						Type:        "string",
						Description: "The command to execute",
					},
					"description": {
						Type:        "string",
						Description: "Description of what the command does",
					},
					"timeout": {
						Type:        "number",
						Description: "Optional timeout in milliseconds",
					},
				},
				Required: []string{"command"},
			},
		}
	case "grep", "grep_search":
		return &Tool{
			Name:        "Grep",
			Description: "A powerful search tool built on ripgrep",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"pattern": {
						Type:        "string",
						Description: "The regular expression pattern to search for",
					},
					"path": {
						Type:        "string",
						Description: "File or directory to search in",
					},
					"glob": {
						Type:        "string",
						Description: "Glob pattern to filter files",
					},
					"type": {
						Type:        "string",
						Description: "File type to search (js, py, rust, go, etc.)",
					},
					"output_mode": {
						Type:        "string",
						Description: "Output mode: content, files_with_matches, count",
					},
				},
				Required: []string{"pattern"},
			},
		}
	case "glob":
		return &Tool{
			Name:        "Glob",
			Description: "Fast file pattern matching tool",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"pattern": {
						Type:        "string",
						Description: "The glob pattern to match files against",
					},
					"path": {
						Type:        "string",
						Description: "The directory to search in",
					},
				},
				Required: []string{"pattern"},
			},
		}
	case "ls":
		return &Tool{
			Name:        "LS",
			Description: "Lists files and directories",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"path": {
						Type:        "string",
						Description: "The absolute path to the directory to list",
					},
					"ignore": {
						Type:        "array",
						Description: "List of glob patterns to ignore",
						Items:       &ToolPropertyItems{Type: "string"},
					},
				},
				Required: []string{"path"},
			},
		}
	case "task":
		return &Tool{
			Name:        "Task",
			Description: "Launch a new agent to handle complex tasks",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"description": {
						Type:        "string",
						Description: "A short description of the task",
					},
					"prompt": {
						Type:        "string",
						Description: "The task for the agent to perform",
					},
				},
				Required: []string{"description", "prompt"},
			},
		}
	case "todowrite":
		return &Tool{
			Name:        "TodoWrite",
			Description: "Create and manage a structured task list",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"todos": {
						Type:        "array",
						Description: "The updated todo list",
						Items:       &ToolPropertyItems{Type: "object"},
					},
				},
				Required: []string{"todos"},
			},
		}
	case "webfetch":
		return &Tool{
			Name:        "WebFetch",
			Description: "Fetches content from a specified URL",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"url": {
						Type:        "string",
						Description: "The URL to fetch content from",
					},
					"prompt": {
						Type:        "string",
						Description: "The prompt to run on the fetched content",
					},
				},
				Required: []string{"url", "prompt"},
			},
		}
	default:
		return nil
	}
}
