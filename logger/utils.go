package logger

import (
	"claude-proxy/types"
	"context"
	"encoding/json"
)

// Common emoji constants for different log types (maintaining existing visual style)
const (
	EmojiReceived   = "📨"
	EmojiTool       = "🔧"
	EmojiTarget     = "🎯"
	EmojiStream     = "🌊"
	EmojiSuccess    = "✅"
	EmojiLaunch     = "🚀"
	EmojiUser       = "👤"
	EmojiSystem     = "📋"
	EmojiOverride   = "🔄"
	EmojiSkip       = "🚫"
	EmojiAlert      = "🚨"
	EmojiStats      = "📊"
	EmojiCorrection = "🔧"
)

// Specialized logging functions for common proxy operations

// LogRequest logs an incoming request with model and tool count
func LogRequest(ctx context.Context, logger Logger, model string, toolCount int) {
	logger.WithModel(model).Info("%s Received request for model: %s, tools: %d", EmojiReceived, model, toolCount)
}

// LogModelRouting logs model routing decisions
func LogModelRouting(ctx context.Context, logger Logger, model, endpoint string) {
	logger.Info("%s Model %s → Endpoint: %s", EmojiTarget, model, endpoint)
}

// LogToolUsed logs when a tool is used in a response
func LogToolUsed(ctx context.Context, logger Logger, toolName, toolID string) {
	logger.Info("%s Tool used in response: %s(id=%s)", EmojiTarget, toolName, toolID)
}

// LogResponseSummary logs a summary of the response
func LogResponseSummary(ctx context.Context, logger Logger, textItems, toolCalls int, stopReason string) {
	logger.Info("%s Response summary: %d text_items, %d tool_calls, stop_reason=%s", 
		EmojiSuccess, textItems, toolCalls, stopReason)
}

// LogProxyRequest logs outgoing proxy requests
func LogProxyRequest(ctx context.Context, logger Logger, endpoint string, streaming bool) {
	logger.Info("%s Proxying to: %s (streaming: %v)", EmojiLaunch, endpoint, streaming)
}

// LogStreamingResponse logs when processing streaming responses
func LogStreamingResponse(ctx context.Context, logger Logger) {
	logger.Info("%s Processing streaming response...", EmojiStream)
}

// LogNonStreamingResponse logs when receiving non-streaming responses
func LogNonStreamingResponse(ctx context.Context, logger Logger, choiceCount int) {
	logger.Info("%s Received non-streaming response with %d choices", EmojiSuccess, choiceCount)
}

// LogUserRequest logs user request size
func LogUserRequest(ctx context.Context, logger Logger, contentLength int) {
	logger.Debug("%s User request: %d", EmojiUser, contentLength)
}

// LogSystemMessage logs system message details
func LogSystemMessage(ctx context.Context, logger Logger, contentLength int, content string) {
	logger.Debug("%s System message (%d chars):\n%s", EmojiSystem, contentLength, content)
}

// LogSystemOverride logs system message overrides
func LogSystemOverride(ctx context.Context, logger Logger, originalLen, modifiedLen int) {
	logger.Info("%s Applied system message overrides (original: %d chars, modified: %d chars)", 
		EmojiOverride, originalLen, modifiedLen)
}

// LogToolsTransformed logs tool transformation results
func LogToolsTransformed(ctx context.Context, logger Logger, transformedCount, originalCount int) {
	logger.Info("%s Transformed %d tools to OpenAI format (filtered from %d)", 
		EmojiTool, transformedCount, originalCount)
}

// LogToolsSkipped logs when tools are filtered out
func LogToolsSkipped(ctx context.Context, logger Logger, count int, toolNames []string) {
	logger.Info("%s Skipped %d tools: %v", EmojiSkip, count, toolNames)
}

// LogToolSchemas logs tool schemas from Claude Code for debugging
func LogToolSchemas(ctx context.Context, logger Logger, tools []types.Tool) {
	logger.Info("🔧 PRINT_TOOL_SCHEMAS enabled - Printing %d tool schemas from Claude Code:", len(tools))
	
	// Print each tool schema for debugging
	for i, tool := range tools {
		// Convert to JSON for pretty printing with indentation
		if toolJSON, err := json.MarshalIndent(tool, "", "  "); err == nil {
			logger.Info("🔧 Tool[%d] Schema (%s):\n%s", i, tool.Name, string(toolJSON))
		} else {
			logger.Warn("🔧 Tool[%d] Schema: Failed to marshal to JSON: %v", i, err)
			logger.Info("🔧 Tool[%d] Schema (%s): %+v", i, tool.Name, tool)
		}
	}
}

// LogToolNames logs the names of tools being processed
func LogToolNames(ctx context.Context, logger Logger, toolNames []string) {
	if len(toolNames) <= 5 {
		logger.Debug("     Tools: [%s]", joinStrings(toolNames, ", "))
	} else {
		logger.Debug("     Tools: [%s, %s, ... and %d more]", 
			toolNames[0], toolNames[1], len(toolNames)-2)
	}
}

// LogEmptyToolResult logs when empty tool results are replaced
func LogEmptyToolResult(ctx context.Context, logger Logger, message string) {
	logger.Debug("%s Empty tool result replaced with placeholder message", EmojiCorrection)
}

// LogMissingToolContent logs when tool result content is missing
func LogMissingToolContent(ctx context.Context, logger Logger) {
	logger.Debug("%s Missing tool result content replaced with default message", EmojiCorrection)
}

// LogDefaultContent logs when default content is added to messages
func LogDefaultContent(ctx context.Context, logger Logger, role string) {
	logger.Debug("%s Added default content for %s message to maintain API compliance", 
		EmojiCorrection, role)
}

// LogProblematicMessage logs when a message has issues
func LogProblematicMessage(ctx context.Context, logger Logger, messageIndex int, reason string) {
	logger.Warn("%s Message %d: %s", EmojiAlert, messageIndex, reason)
}

// LogLargeConversation logs when dealing with large conversations
func LogLargeConversation(ctx context.Context, logger Logger, messageCount int) {
	logger.Info("%s Large conversation: %d messages", EmojiStats, messageCount)
}

// LogInvalidMessages logs when messages fail validation
func LogInvalidMessages(ctx context.Context, logger Logger, invalidCount, totalCount int) {
	logger.Warn("%s Found %d potentially invalid messages out of %d total", 
		EmojiAlert, invalidCount, totalCount)
}

// Helper function to join strings (avoiding external dependencies)
func joinStrings(strs []string, separator string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}
	
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += separator + strs[i]
	}
	return result
}

// ConditionalLogger wraps common pattern of getting logger from context with config
func ConditionalLogger(ctx context.Context, cfg interface{}) Logger {
	// This is a convenience function that can be used when we need to create
	// a logger but don't have one in context yet
	if logger, ok := ctx.Value(loggerContextKey).(Logger); ok {
		return logger
	}
	
	// If no logger in context, create a basic one
	// This maintains backwards compatibility during transition
	return &noOpLogger{}
}

// noOpLogger is a no-operation logger for backwards compatibility
type noOpLogger struct{}

func (n *noOpLogger) Debug(format string, args ...interface{}) {}
func (n *noOpLogger) Info(format string, args ...interface{})  {}
func (n *noOpLogger) Warn(format string, args ...interface{})  {}
func (n *noOpLogger) Error(format string, args ...interface{}) {}
func (n *noOpLogger) WithField(key, value string) Logger       { return n }
func (n *noOpLogger) WithModel(model string) Logger            { return n }
func (n *noOpLogger) WithComponent(component string) Logger    { return n }