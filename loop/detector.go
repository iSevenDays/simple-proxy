package loop

import (
	"claude-proxy/types"
	"context"
	"crypto/md5"
	"fmt"
	"strings"
	"time"
)

// LoopDetector tracks recent tool calls to detect repetitive patterns
type LoopDetector struct {
	recentCalls []toolCallRecord
	maxHistory  int
	timeWindow  time.Duration
}

type toolCallRecord struct {
	toolName    string
	paramHash   string
	timestamp   time.Time
	messageRole string
}

// NewLoopDetector creates a new loop detector
func NewLoopDetector() *LoopDetector {
	return &LoopDetector{
		recentCalls: make([]toolCallRecord, 0),
		maxHistory:  20,              // Track last 20 tool calls
		timeWindow:  5 * time.Minute, // Consider calls within 5 minutes
	}
}

// DetectLoop analyzes the conversation for repetitive tool call patterns
func (ld *LoopDetector) DetectLoop(ctx context.Context, messages []types.OpenAIMessage) *LoopDetection {
	// Get filtered messages (after any previous loop detection)
	filteredMessages := ld.getFilteredMessages(messages)

	// Extract tool calls from filtered messages
	toolCalls := ld.extractToolCallsFromFilteredMessages(filteredMessages)

	// Look for repetitive patterns in the filtered messages
	if len(toolCalls) < 3 {
		return &LoopDetection{HasLoop: false}
	}

	// Check for consecutive identical tool calls
	consecutiveCount := ld.countConsecutiveIdenticalCalls(toolCalls)
	if consecutiveCount >= 3 {
		lastCall := toolCalls[len(toolCalls)-1]
		return &LoopDetection{
			HasLoop:        true,
			LoopType:       "consecutive_identical",
			ToolName:       lastCall.toolName,
			Count:          consecutiveCount,
			Recommendation: ld.generateRecommendation(lastCall.toolName, consecutiveCount),
		}
	}

	// Check for alternating patterns (tool -> result -> tool -> result)
	alternatingCount := ld.countAlternatingPattern(filteredMessages)
	if alternatingCount >= 4 { // At least 2 complete cycles
		return &LoopDetection{
			HasLoop:        true,
			LoopType:       "alternating_pattern",
			ToolName:       ld.getLastToolName(toolCalls),
			Count:          alternatingCount,
			Recommendation: "Breaking repetitive tool execution pattern. Consider providing more specific instructions or trying a different approach.",
		}
	}

	return &LoopDetection{HasLoop: false}
}

// LoopDetection represents the result of loop detection
type LoopDetection struct {
	HasLoop        bool
	LoopType       string // "consecutive_identical", "alternating_pattern"
	ToolName       string
	Count          int
	Recommendation string
}

// extractToolCallsFromMessages extracts tool calls from recent messages
func (ld *LoopDetector) extractToolCallsFromMessages(messages []types.OpenAIMessage) []toolCallRecord {
	var toolCalls []toolCallRecord
	now := time.Now()

	// Find the last loop detection response to avoid re-detecting the same loop
	lastLoopDetectionIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == "assistant" && strings.Contains(msg.Content, "Loop Detection") {
			lastLoopDetectionIdx = i
			break
		}
	}

	// If we found a loop detection response, only analyze messages after it
	startIdx := 0
	if lastLoopDetectionIdx >= 0 {
		startIdx = lastLoopDetectionIdx + 1
	} else {
		// Look at the last 15 messages to detect patterns (original logic)
		startIdx = len(messages) - 15
		if startIdx < 0 {
			startIdx = 0
		}
	}

	for i := startIdx; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				paramHash := ld.hashParameters(tc.Function.Arguments)
				toolCalls = append(toolCalls, toolCallRecord{
					toolName:    tc.Function.Name,
					paramHash:   paramHash,
					timestamp:   now, // Approximate timestamp
					messageRole: msg.Role,
				})
			}
		}
	}

	return toolCalls
}

// getFilteredMessages returns the same message range used for tool call extraction
func (ld *LoopDetector) getFilteredMessages(messages []types.OpenAIMessage) []types.OpenAIMessage {
	// Find the last loop detection response to avoid re-detecting the same loop
	lastLoopDetectionIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == "assistant" && strings.Contains(msg.Content, "Loop Detection") {
			lastLoopDetectionIdx = i
			break
		}
	}

	// If we found a loop detection response, only analyze messages after it
	startIdx := 0
	if lastLoopDetectionIdx >= 0 {
		startIdx = lastLoopDetectionIdx + 1
	} else {
		// Look at the last 15 messages to detect patterns (original logic)
		startIdx = len(messages) - 15
		if startIdx < 0 {
			startIdx = 0
		}
	}

	// Return the filtered slice
	if startIdx >= len(messages) {
		return []types.OpenAIMessage{}
	}
	return messages[startIdx:]
}

// extractToolCallsFromFilteredMessages extracts tool calls from already filtered messages
func (ld *LoopDetector) extractToolCallsFromFilteredMessages(messages []types.OpenAIMessage) []toolCallRecord {
	var toolCalls []toolCallRecord
	now := time.Now()

	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				paramHash := ld.hashParameters(tc.Function.Arguments)
				toolCalls = append(toolCalls, toolCallRecord{
					toolName:    tc.Function.Name,
					paramHash:   paramHash,
					timestamp:   now, // Approximate timestamp
					messageRole: msg.Role,
				})
			}
		}
	}

	return toolCalls
}

// countConsecutiveIdenticalCalls counts consecutive identical tool calls
func (ld *LoopDetector) countConsecutiveIdenticalCalls(toolCalls []toolCallRecord) int {
	if len(toolCalls) == 0 {
		return 0
	}

	lastCall := toolCalls[len(toolCalls)-1]
	count := 1

	// Count backwards while calls are identical
	for i := len(toolCalls) - 2; i >= 0; i-- {
		call := toolCalls[i]
		if call.toolName == lastCall.toolName && call.paramHash == lastCall.paramHash {
			count++
		} else {
			break
		}
	}

	return count
}

// countAlternatingPattern detects assistant->tool->assistant->tool patterns
func (ld *LoopDetector) countAlternatingPattern(messages []types.OpenAIMessage) int {
	if len(messages) < 4 {
		return 0
	}

	// Look for pattern: assistant (with tool_calls) -> tool -> assistant (with tool_calls) -> tool
	count := 0
	i := len(messages) - 1

	// Start from the end and work backwards
	for i >= 3 {
		// Check if we have the pattern: tool -> assistant -> tool -> assistant
		if messages[i].Role == "tool" &&
			messages[i-1].Role == "assistant" && len(messages[i-1].ToolCalls) > 0 &&
			messages[i-2].Role == "tool" &&
			messages[i-3].Role == "assistant" && len(messages[i-3].ToolCalls) > 0 {

			// Check if the assistant messages have the same tool call
			if ld.hasSimilarToolCalls(messages[i-1], messages[i-3]) {
				count += 2 // Count both messages in the pattern
				i -= 2     // Skip to next potential pattern
			} else {
				break
			}
		} else {
			break
		}
	}

	return count
}

// hasSimilarToolCalls checks if two assistant messages have similar tool calls
func (ld *LoopDetector) hasSimilarToolCalls(msg1, msg2 types.OpenAIMessage) bool {
	if len(msg1.ToolCalls) == 0 || len(msg2.ToolCalls) == 0 {
		return false
	}

	// Compare both tool name AND arguments to avoid false positives
	tool1 := msg1.ToolCalls[0].Function.Name
	tool2 := msg2.ToolCalls[0].Function.Name
	args1 := msg1.ToolCalls[0].Function.Arguments
	args2 := msg2.ToolCalls[0].Function.Arguments

	// Tools are similar only if both name and arguments match
	if tool1 != tool2 {
		return false
	}

	// Hash the arguments for comparison (same logic as consecutive detection)
	hash1 := ld.hashParameters(args1)
	hash2 := ld.hashParameters(args2)

	return hash1 == hash2
}

// getLastToolName gets the name of the last tool call
func (ld *LoopDetector) getLastToolName(toolCalls []toolCallRecord) string {
	if len(toolCalls) == 0 {
		return "unknown"
	}
	return toolCalls[len(toolCalls)-1].toolName
}

// hashParameters creates a hash of tool call parameters for comparison
func (ld *LoopDetector) hashParameters(params string) string {
	// Normalize JSON by removing whitespace for comparison
	normalized := strings.ReplaceAll(params, " ", "")
	normalized = strings.ReplaceAll(normalized, "\n", "")
	normalized = strings.ReplaceAll(normalized, "\t", "")

	hash := md5.Sum([]byte(normalized))
	return fmt.Sprintf("%x", hash)
}

// generateRecommendation creates a helpful recommendation for breaking the loop
func (ld *LoopDetector) generateRecommendation(toolName string, count int) string {
	switch toolName {
	case "TodoWrite":
		return fmt.Sprintf("Loop detected: %s called %d times consecutively. The todo list may already be properly updated. Consider proceeding with actual task implementation or asking for clarification on what specific action is needed.", toolName, count)
	case "Edit", "MultiEdit":
		return fmt.Sprintf("Loop detected: %s called %d times consecutively. The file may already be properly edited. Consider reviewing the changes or trying a different approach.", toolName, count)
	case "Write":
		return fmt.Sprintf("Loop detected: %s called %d times consecutively. The file may already exist or there may be a permission issue. Consider checking the file status or trying a different approach.", toolName, count)
	case "Read":
		return fmt.Sprintf("Loop detected: %s called %d times consecutively. The file content may already be available. Consider proceeding with analysis or asking for clarification.", toolName, count)
	default:
		return fmt.Sprintf("Loop detected: %s called %d times consecutively. Consider providing more specific instructions or trying a different approach.", toolName, count)
	}
}

// CreateLoopBreakingResponse creates a response that breaks the loop
func (ld *LoopDetector) CreateLoopBreakingResponse(detection *LoopDetection) types.AnthropicResponse {
	return types.AnthropicResponse{
		ID:    "loop-break-" + fmt.Sprintf("%d", time.Now().Unix()),
		Type:  "message",
		Role:  "assistant",
		Model: "loop-detector",
		Content: []types.Content{
			{
				Type: "text",
				Text: fmt.Sprintf("🔄 **Loop Detection**: %s\n\nI've detected a repetitive pattern and am breaking the loop to prevent infinite execution. Please provide more specific guidance or let me know if you need help with a different approach.", detection.Recommendation),
			},
		},
		StopReason: "tool_use",
		Usage: types.Usage{
			InputTokens:  0,
			OutputTokens: 50,
		},
	}
}
