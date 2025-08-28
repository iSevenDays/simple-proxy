package types

// OpenAIRequest represents a complete request structure formatted for OpenAI-compatible
// providers, created through transformation from Anthropic format requests.
//
// This struct serves as the target format for the proxy's request transformation
// pipeline, containing all necessary information for communication with OpenAI-compatible
// endpoints including model routing, conversation history, and tool configurations.
//
// The request structure supports:
//   - Model selection through the Model field
//   - Multi-turn conversations through Messages
//   - Function calling through Tools and ToolChoice
//   - Response control through MaxTokens and Temperature
//   - Streaming responses through Stream flag
//   - Caching optimizations through CachePrompt
//
// OpenAIRequest is designed to be compatible with the OpenAI Chat Completions API
// while supporting various OpenAI-compatible providers configured via environment variables.
type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
	ToolChoice  interface{}     `json:"tool_choice,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	CachePrompt bool            `json:"cache_prompt,omitempty"`
}

// OpenAIResponse represents a complete response from OpenAI-compatible providers,
// containing the generated content, metadata, and usage statistics.
//
// This struct serves as the input for the proxy's response transformation pipeline,
// containing the provider's response in OpenAI format before conversion back to
// Anthropic format for Claude Code consumption.
//
// The response structure includes:
//   - Standard OpenAI response fields (ID, Object, Created, Model)
//   - Generated content through Choices array
//   - Token usage statistics through Usage
//   - Provider-specific metadata and timing information
//
// OpenAIResponse supports both single-choice and multiple-choice responses,
// with the proxy typically processing the first choice for standard operations.
type OpenAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
}

// OpenAIStreamChunk represents individual chunks in a streaming response from
// OpenAI-compatible providers, enabling real-time response processing.
//
// Streaming responses are delivered as a series of chunks, each containing
// incremental content updates. The chunk structure mirrors the complete
// response format but contains delta information rather than complete content.
//
// Stream chunks enable:
//   - Real-time response display in Claude Code UI
//   - Progressive content loading and rendering
//   - Early response termination if needed
//   - Efficient bandwidth utilization for long responses
//
// The proxy reconstructs complete responses from stream chunks before
// transforming back to Anthropic format, maintaining response completeness
// while supporting streaming performance benefits.
type OpenAIStreamChunk struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []OpenAIStreamChoice `json:"choices"`
}

// OpenAIMessage represents a single message within an OpenAI-format conversation,
// supporting text content, function calls, and tool interactions.
//
// This message format serves as the target structure for transforming Anthropic
// messages into OpenAI-compatible format, maintaining conversation context
// and tool interaction history for provider communication.
//
// The message structure supports:
//   - Standard chat roles (user, assistant, system, tool)
//   - Plain text content through Content field
//   - Function/tool calls through ToolCalls array
//   - Tool call responses through ToolCallID linking
//   - Message attribution through optional Name field
//
// OpenAIMessage enables full conversation fidelity during format transformation,
// ensuring that complex multi-turn interactions with tools are preserved.
type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	Name       string           `json:"name,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// OpenAIChoice represents a single response alternative from an OpenAI-compatible
// provider, containing the generated content and completion metadata.
//
// Providers may return multiple choices for a single request, though the proxy
// typically processes only the first choice for standard operations. Each choice
// contains a complete message and finish reason indication.
//
// The choice structure includes:
//   - Index: Position of this choice in the response
//   - Message: Complete generated message content
//   - FinishReason: Explanation of why generation stopped
//
// FinishReason values include "stop" (natural completion), "length" (max tokens),
// "tool_calls" (function call generated), or provider-specific reasons.
type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason *string       `json:"finish_reason"`
}

// OpenAIStreamChoice represents a single choice within a streaming response chunk,
// containing incremental content updates and completion status.
//
// Streaming choices contain delta information rather than complete content,
// enabling progressive response building during streaming operations.
// The proxy accumulates deltas across chunks to reconstruct complete responses.
//
// The streaming choice structure includes:
//   - Index: Position of this choice in the response stream
//   - Delta: Incremental content updates for this chunk
//   - FinishReason: Completion status (non-nil when stream ends)
//
// Delta content is accumulated across chunks to build the complete response
// before transformation back to Anthropic format.
type OpenAIStreamChoice struct {
	Index        int               `json:"index"`
	Delta        OpenAIStreamDelta `json:"delta"`
	FinishReason *string           `json:"finish_reason"`
}

// OpenAIStreamDelta represents incremental content updates within a streaming
// response chunk, containing the new content added in this specific chunk.
//
// Stream deltas enable efficient streaming by transmitting only the incremental
// changes rather than the complete content with each chunk. The proxy accumulates
// these deltas to reconstruct the complete response.
//
// Delta content types include:
//   - Role: Message role (typically only in first chunk)
//   - Content: Incremental text content
//   - ToolCalls: Incremental function/tool call information
//
// The delta structure enables real-time response building while maintaining
// full response fidelity for complex interactions involving tools and functions.
type OpenAIStreamDelta struct {
	Role      string           `json:"role,omitempty"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
}

// OpenAITool represents a function/tool definition in OpenAI format, created
// through transformation from Anthropic tool definitions for provider communication.
//
// This structure provides the OpenAI-compatible representation of Claude Code
// tools, enabling function calling capabilities with OpenAI-compatible providers
// while maintaining tool functionality and parameter validation.
//
// The tool structure includes:
//   - Type: Tool type identifier (typically "function")
//   - Function: Complete function definition with schema
//
// OpenAITool serves as the target format for tool transformation, ensuring
// that Claude Code's tool capabilities are preserved during provider communication.
type OpenAITool struct {
	Type     string             `json:"type"`
	Function OpenAIToolFunction `json:"function"`
}

// OpenAIToolFunction represents the function definition portion of an OpenAI tool,
// containing the function signature, documentation, and parameter schema.
//
// This structure provides the detailed function specification needed for
// OpenAI-compatible providers to understand and invoke tools correctly,
// including parameter validation and usage guidance.
//
// The function definition includes:
//   - Name: Unique function identifier
//   - Description: Human-readable function documentation
//   - Parameters: JSON Schema for parameter validation
//
// OpenAIToolFunction enables full tool functionality preservation during
// format transformation from Anthropic to OpenAI tool specifications.
type OpenAIToolFunction struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  ToolSchema `json:"parameters"`
}

// OpenAIToolCall represents a function/tool invocation in OpenAI format,
// containing the call identifier, type, and function execution details.
//
// Tool calls enable the model to invoke functions during response generation,
// with each call containing all necessary information for function execution
// and result correlation.
//
// The tool call structure includes:
//   - ID: Unique identifier for correlating with results
//   - Type: Call type identifier (typically "function")
//   - Function: Function name and argument specification
//   - Index: Position in streaming responses (optional)
//
// OpenAIToolCall enables full function calling capability during provider
// communication while maintaining call tracking and result correlation.
type OpenAIToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function OpenAIToolCallFunction `json:"function"`
	Index    int                    `json:"index,omitempty"`
}

// OpenAIToolCallFunction represents the execution details for a specific
// function/tool call, containing the function name and serialized arguments.
//
// This structure provides the execution specification for tool calls,
// with arguments serialized as JSON strings for provider communication.
// The proxy handles argument parsing and validation before tool execution.
//
// The function call details include:
//   - Name: The function name to invoke
//   - Arguments: JSON-serialized function arguments
//
// Arguments are maintained as JSON strings to preserve complex parameter
// structures during provider communication and tool call processing.
type OpenAIToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAIUsage represents detailed token consumption statistics from OpenAI-compatible
// providers, providing billing and performance monitoring information.
//
// This structure captures provider-reported token usage in OpenAI's standard
// format, enabling accurate cost calculation and performance monitoring
// during proxy operations.
//
// The usage statistics include:
//   - PromptTokens: Tokens consumed by input prompt and context
//   - CompletionTokens: Tokens generated in the response
//   - TotalTokens: Sum of prompt and completion tokens
//
// Token counts are used for:
//   - Cost calculation and billing transparency
//   - Performance optimization and monitoring
//   - Rate limiting and quota management
//   - Usage analytics and capacity planning
//
// The proxy transforms these statistics to Anthropic format before
// returning them to Claude Code for display and monitoring.
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
