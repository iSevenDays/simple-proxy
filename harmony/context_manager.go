// Package harmony provides context management for OpenAI Harmony format compliance,
// specifically implementing the analysis message preservation requirements from the
// OpenAI Harmony guide for tool calling scenarios.
package harmony

import (
	"claude-proxy/parser"
	"claude-proxy/types"
	"fmt"
	"strings"
	"sync"
)

// ContextManager manages conversation history and implements OpenAI Harmony format
// requirements for analysis message preservation during tool calling scenarios.
//
// According to the OpenAI Harmony guide: "The exception for this is tool/function calling.
// The model is able to call tools as part of its chain-of-thought and because of that,
// we should pass the previous chain-of-thought back in as input for subsequent sampling."
type ContextManager struct {
	mu                  sync.RWMutex // Protects all fields for thread safety
	history             []types.Message
	preservedAnalysis   []string
	lastMessageType     MessageType
	lastFinalMessageIdx int // Index of the last message with final channel
}

// Constants for memory management
const (
	// MaxHistoryLength defines the maximum number of messages to keep in history
	// This prevents unbounded memory growth in long conversations
	MaxHistoryLength = 50
)

// MessageType represents the type of the last assistant message for preservation logic
type MessageType int

const (
	MessageTypeUnknown MessageType = iota
	MessageTypeToolCall
	MessageTypeFinal
	MessageTypeAnalysisOnly
	MessageTypeCommentary
)

// String returns the string representation of MessageType
func (m MessageType) String() string {
	switch m {
	case MessageTypeToolCall:
		return "tool_call"
	case MessageTypeFinal:
		return "final"
	case MessageTypeAnalysisOnly:
		return "analysis_only"
	case MessageTypeCommentary:
		return "commentary"
	default:
		return "unknown"
	}
}

// NewContextManager creates a new ContextManager instance for managing
// Harmony format conversation state and analysis preservation requirements.
//
// The context manager tracks conversation history and implements the OpenAI
// Harmony guide requirement to preserve analysis messages when the last
// assistant message contained tool calls.
//
// Returns:
//   - A new ContextManager instance ready for use
func NewContextManager() *ContextManager {
	return &ContextManager{
		history:             make([]types.Message, 0),
		preservedAnalysis:   make([]string, 0),
		lastMessageType:     MessageTypeUnknown,
		lastFinalMessageIdx: -1,
	}
}

// UpdateHistory updates the conversation history with a new message and
// recalculates preservation state according to Harmony format requirements.
//
// This method should be called after each message to maintain accurate
// conversation state for proper analysis message preservation behavior.
//
// According to the OpenAI Harmony guide:
// - Drop analysis messages after final channel messages (normal case)
// - Preserve analysis messages after tool calls until next final message
//
// Parameters:
//   - message: The new message to add to conversation history
func (cm *ContextManager) UpdateHistory(message types.Message) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	cm.history = append(cm.history, message)
	
	// Only analyze assistant messages for preservation logic
	if message.Role != "assistant" {
		return
	}
	
	// Analyze message content to determine type and update preservation state
	messageType, hasToolCall, hasFinal, analysisContent := cm.analyzeMessage(message)
	cm.lastMessageType = messageType
	
	// If this message has a final channel, update the last final message index
	if hasFinal {
		cm.lastFinalMessageIdx = len(cm.history) - 1
		// Clear preserved analysis when a new final message is issued
		cm.preservedAnalysis = make([]string, 0)
	}
	
	// If this message has tool calls, preserve analysis content from after the last final message
	if hasToolCall {
		cm.updatePreservedAnalysis(analysisContent)
	}
	
	// Enforce memory bounds - keep only the most recent messages
	cm.enforceMemoryBounds()
}

// analyzeMessage examines a message to determine its type and extract relevant content
// for preservation logic according to OpenAI Harmony format requirements.
//
// Returns:
//   - messageType: The determined MessageType for this message
//   - hasToolCall: Whether the message contains any tool calls
//   - hasFinal: Whether the message contains final channel content
//   - analysisContent: Analysis/thinking content found in the message
func (cm *ContextManager) analyzeMessage(message types.Message) (MessageType, bool, bool, string) {
	var hasToolCall, hasFinal bool
	var analysisContent strings.Builder
	
	// Examine each content block in the message
	// Handle both string and []Content formats with proper type safety
	var contents []types.Content
	switch content := message.Content.(type) {
	case []types.Content:
		// Handle structured content array
		contents = content
	case string:
		// Handle string content case - assume it's final response
		if content != "" {
			hasFinal = true
		}
		return MessageTypeFinal, hasToolCall, hasFinal, analysisContent.String()
	default:
		// Unknown content type, treat as final for safety
		return MessageTypeFinal, hasToolCall, true, analysisContent.String()
	}
	
	for _, content := range contents {
		switch content.Type {
		case "tool_use":
			hasToolCall = true
		case "thinking":
			// Thinking content (analysis channel) should be preserved if followed by tool calls
			if analysisContent.Len() > 0 {
				analysisContent.WriteString("\n")
			}
			analysisContent.WriteString(content.Text)
		case "commentary":
			// Commentary channel content should also be preserved according to Harmony spec
			// "Function calls to the commentary channel can remain"
			if analysisContent.Len() > 0 {
				analysisContent.WriteString("\n")
			}
			analysisContent.WriteString(content.Text)
		case "text":
			// Check if this is from a final channel (heuristic: non-tool, non-thinking content)
			// In a full implementation, we'd track the original channel information
			hasFinal = true
		}
	}
	
	// Determine message type based on content analysis
	if hasToolCall && hasFinal {
		return MessageTypeToolCall, hasToolCall, hasFinal, analysisContent.String()
	} else if hasToolCall {
		return MessageTypeToolCall, hasToolCall, hasFinal, analysisContent.String()
	} else if hasFinal {
		return MessageTypeFinal, hasToolCall, hasFinal, analysisContent.String()
	} else if analysisContent.Len() > 0 {
		return MessageTypeAnalysisOnly, hasToolCall, hasFinal, analysisContent.String()
	} else {
		return MessageTypeUnknown, hasToolCall, hasFinal, analysisContent.String()
	}
}

// updatePreservedAnalysis updates the preserved analysis content by collecting
// analysis messages from after the last final message until the current point.
//
// This implements the OpenAI Harmony guide requirement to preserve chain-of-thought
// content when tool calls are involved in the conversation flow.
//
// Parameters:
//   - currentAnalysis: Analysis content from the current message to add
func (cm *ContextManager) updatePreservedAnalysis(currentAnalysis string) {
	// Start collecting from after the last final message
	startIdx := cm.lastFinalMessageIdx + 1
	if startIdx < 0 {
		startIdx = 0
	}
	
	// Reset preserved analysis to collect fresh content
	cm.preservedAnalysis = make([]string, 0)
	
	// Collect analysis content from messages after the last final message
	// Note: We collect from the entire range including current message since it's already in history
	for i := startIdx; i < len(cm.history); i++ {
		message := cm.history[i]
		if message.Role == "assistant" {
			// Handle both string and []Content formats with proper type safety
			switch content := message.Content.(type) {
			case []types.Content:
				for _, item := range content {
					// Preserve both thinking (analysis) and commentary channels per Harmony spec
					if (item.Type == "thinking" || item.Type == "commentary") && item.Text != "" {
						cm.preservedAnalysis = append(cm.preservedAnalysis, item.Text)
					}
				}
			case string:
				// String content doesn't have thinking/commentary components
				continue
			default:
				// Skip unknown content types
				continue
			}
		}
	}
	
	// Note: currentAnalysis is already included in the history scan above,
	// so we don't need to add it separately to avoid duplicates
}

// ShouldPreserveAnalysis returns true if analysis messages should be preserved
// in the next sampling turn based on OpenAI Harmony format requirements.
//
// According to the guide: "The exception for this is tool/function calling.
// The model is able to call tools as part of its chain-of-thought and because
// of that, we should pass the previous chain-of-thought back in as input for
// subsequent sampling."
//
// Returns:
//   - true if the last assistant message contained tool calls
//   - false if the last assistant message was a final response
func (cm *ContextManager) ShouldPreserveAnalysis() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	return cm.lastMessageType == MessageTypeToolCall
}

// GetPreservedAnalysis returns the analysis content that should be preserved
// in subsequent sampling according to OpenAI Harmony format requirements.
//
// This returns analysis messages from after the last final channel message
// up to and including the last tool call, as required by the Harmony guide.
//
// Returns:
//   - A slice of strings containing preserved analysis content
//   - Empty slice if no analysis should be preserved
func (cm *ContextManager) GetPreservedAnalysis() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	if cm.lastMessageType != MessageTypeToolCall {
		return make([]string, 0)
	}
	
	// Return a copy to prevent external modification
	result := make([]string, len(cm.preservedAnalysis))
	copy(result, cm.preservedAnalysis)
	return result
}

// ClearPreservedAnalysis clears any preserved analysis content, typically
// called when a new final message is issued according to Harmony format rules.
//
// This implements the normal case where analysis messages are dropped after
// final channel messages, as specified in the OpenAI Harmony guide.
func (cm *ContextManager) ClearPreservedAnalysis() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	cm.preservedAnalysis = make([]string, 0)
	cm.lastMessageType = MessageTypeFinal
}

// GetLastMessageType returns the type of the last assistant message in the
// conversation history for debugging and state inspection purposes.
//
// Returns:
//   - The MessageType of the most recent assistant message
//   - MessageTypeUnknown if no assistant messages exist
func (cm *ContextManager) GetLastMessageType() MessageType {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	return cm.lastMessageType
}

// GetHistoryLength returns the current length of the conversation history
// for debugging and monitoring purposes.
//
// Returns:
//   - The number of messages in the conversation history
func (cm *ContextManager) GetHistoryLength() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	return len(cm.history)
}

// GetPreservedAnalysisCount returns the number of preserved analysis messages
// for debugging and monitoring purposes.
//
// Returns:
//   - The number of analysis messages currently preserved
func (cm *ContextManager) GetPreservedAnalysisCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	return len(cm.preservedAnalysis)
}

// BuildPreservedContext constructs the preserved analysis content in the
// proper OpenAI Harmony format for inclusion in subsequent sampling.
//
// This method formats preserved analysis messages as proper Harmony tokens
// according to the format specified in the OpenAI Harmony guide.
//
// Returns:
//   - Formatted string containing preserved analysis in Harmony format
//   - Empty string if no analysis should be preserved
func (cm *ContextManager) BuildPreservedContext() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	if cm.lastMessageType != MessageTypeToolCall || len(cm.preservedAnalysis) == 0 {
		return ""
	}
	
	var builder strings.Builder
	
	// Format each preserved analysis message as a Harmony analysis channel
	for i, analysis := range cm.preservedAnalysis {
		if i > 0 {
			// Add separator between multiple analysis messages
			builder.WriteString("\n")
		}
		
		// Format as proper Harmony token sequence
		builder.WriteString("<|start|>assistant<|channel|>analysis<|message|>")
		builder.WriteString(strings.TrimSpace(analysis))
		builder.WriteString("<|end|>")
	}
	
	return builder.String()
}

// ValidateHarmonyCompliance performs validation checks to ensure the context
// manager is maintaining proper OpenAI Harmony format compliance.
//
// This method checks for common implementation issues and provides diagnostic
// information for debugging preservation behavior.
//
// Returns:
//   - A slice of validation errors found
//   - Empty slice if all compliance checks pass
func (cm *ContextManager) ValidateHarmonyCompliance() []error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	var errors []error
	
	// Check 1: Verify preservation state consistency
	if cm.lastMessageType == MessageTypeToolCall && len(cm.preservedAnalysis) == 0 {
		errors = append(errors, fmt.Errorf("preservation flag set but no analysis content preserved"))
	}
	
	// Check 2: Verify last message type consistency
	if len(cm.history) > 0 {
		lastAssistantIdx := -1
		for i := len(cm.history) - 1; i >= 0; i-- {
			if cm.history[i].Role == "assistant" {
				lastAssistantIdx = i
				break
			}
		}
		
		if lastAssistantIdx >= 0 {
			actualType, _, _, _ := cm.analyzeMessage(cm.history[lastAssistantIdx])
			if actualType != cm.lastMessageType {
				errors = append(errors, fmt.Errorf("last message type mismatch: stored=%s, actual=%s", 
					cm.lastMessageType.String(), actualType.String()))
			}
		}
	}
	
	// Check 3: Verify final message index consistency
	if cm.lastFinalMessageIdx >= len(cm.history) {
		errors = append(errors, fmt.Errorf("last final message index out of bounds: %d >= %d", 
			cm.lastFinalMessageIdx, len(cm.history)))
	}
	
	return errors
}

// Reset clears all conversation history and preservation state, returning
// the context manager to its initial state.
//
// This method is useful for starting fresh conversations or clearing
// accumulated state for testing purposes.
func (cm *ContextManager) Reset() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	cm.history = make([]types.Message, 0)
	cm.preservedAnalysis = make([]string, 0)
	cm.lastMessageType = MessageTypeUnknown
	cm.lastFinalMessageIdx = -1
}

// ExtractHarmonyChannels analyzes a Harmony format message string and returns
// information about the channels present, useful for integration with the
// existing parser functionality.
//
// This method provides a bridge between the context manager and the existing
// Harmony parsing infrastructure.
//
// Parameters:
//   - content: Raw Harmony format content to analyze
//
// Returns:
//   - Parsed channels from the content
//   - Error if parsing fails
func (cm *ContextManager) ExtractHarmonyChannels(content string) ([]parser.Channel, error) {
	// This method doesn't access ContextManager state, so no mutex needed
	if !parser.IsHarmonyFormat(content) {
		return nil, fmt.Errorf("content is not in Harmony format")
	}
	
	channels := parser.ExtractChannels(content)
	return channels, nil
}

// enforceMemoryBounds limits history size to prevent unbounded memory growth
// Uses sliding window approach to keep most recent messages while maintaining
// index consistency for preservation logic
func (cm *ContextManager) enforceMemoryBounds() {
	if len(cm.history) <= MaxHistoryLength {
		return
	}
	
	// Calculate how many messages to remove from the beginning
	excess := len(cm.history) - MaxHistoryLength
	
	// Adjust lastFinalMessageIdx after truncation
	if cm.lastFinalMessageIdx >= 0 {
		cm.lastFinalMessageIdx -= excess
		// If the final message was truncated, reset the index
		if cm.lastFinalMessageIdx < 0 {
			cm.lastFinalMessageIdx = -1
			// Clear preserved analysis since reference point was lost
			cm.preservedAnalysis = make([]string, 0)
		}
	}
	
	// Truncate history to keep only recent messages
	// Use copy to ensure old messages are eligible for garbage collection
	newHistory := make([]types.Message, MaxHistoryLength)
	copy(newHistory, cm.history[excess:])
	cm.history = newHistory
}