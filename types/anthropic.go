package types

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
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// Usage represents token usage
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
