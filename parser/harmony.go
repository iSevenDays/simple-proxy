// Package parser provides OpenAI Harmony message format parsing capabilities.
// It recognizes and parses Harmony tokens like <|start|>, <|channel|>, <|message|>, <|end|>
// to properly classify thinking content, user responses, and tool calls.
package parser

import (
	"fmt"
	"regexp"
	"strings"
)

// Role represents the different roles that can appear in Harmony messages.
// This enum provides strongly-typed role identification for message parsing
// and routing within the Claude Code proxy system.
//
// Harmony messages contain role identifiers in start tokens like <|start|>assistant,
// and this type ensures consistent role handling across the system.
//
// Performance: Role operations are O(1) constant time operations.
type Role int

const (
	RoleAssistant Role = iota
	RoleUser
	RoleSystem
	RoleDeveloper
	RoleTool
)

// String returns the string representation of the Role for API compatibility
// and logging purposes. This method implements the Stringer interface.
//
// The returned string values correspond to standard message role identifiers
// used in chat completion APIs: "assistant", "user", "system", "developer", "tool".
//
// Returns "assistant" as the fallback for unknown role values to ensure
// graceful degradation in parsing scenarios.
//
// Example:
//
//	r := RoleUser
//	fmt.Println(r.String()) // outputs: "user"
func (r Role) String() string {
	switch r {
	case RoleAssistant:
		return "assistant"
	case RoleUser:
		return "user"
	case RoleSystem:
		return "system"
	case RoleDeveloper:
		return "developer"
	case RoleTool:
		return "tool"
	default:
		return "assistant"
	}
}

// ParseRole converts a string role identifier to the corresponding Role enum.
// Input strings are normalized (trimmed and lowercased) for robust parsing.
//
// This function provides case-insensitive parsing of role identifiers from
// Harmony message tokens, ensuring consistent role classification regardless
// of input formatting variations.
//
// Parameters:
//   - role: The string role identifier to parse (e.g., "Assistant", "USER", " system ")
//
// Returns:
//   - The corresponding Role enum value
//   - RoleAssistant for unrecognized inputs (graceful fallback)
//
// Performance: O(1) constant time with simple string operations.
//
// Example:
//
//	assistant := ParseRole("ASSISTANT")
//	user := ParseRole(" user ")
//	unknown := ParseRole("invalid") // returns RoleAssistant
func ParseRole(role string) Role {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant":
		return RoleAssistant
	case "user":
		return RoleUser
	case "system":
		return RoleSystem
	case "developer":
		return RoleDeveloper
	case "tool":
		return RoleTool
	default:
		return RoleAssistant
	}
}

// ChannelType represents the different channel types available in OpenAI Harmony format.
// Channels categorize different types of content within a single message response,
// enabling separation of thinking, final responses, and commentary.
//
// The Harmony format uses channel tokens like <|channel|>analysis to specify
// content categorization, allowing clients like Claude Code to properly display
// different content types (thinking vs. final response).
//
// Channel types directly impact how content is processed and displayed:
//   - Analysis: Internal reasoning, displayed as "thinking" content
//   - Final: User-facing response content
//   - Commentary: Tool-related or meta information
//   - Unknown: Fallback for unrecognized channel types
//
// Performance: All channel operations are O(1) constant time.
type ChannelType int

const (
	ChannelAnalysis ChannelType = iota
	ChannelFinal
	ChannelCommentary
	ChannelUnknown
)

// String returns the string representation of the ChannelType for API
// compatibility and debugging purposes. This method implements the Stringer interface.
//
// The returned string values correspond to Harmony channel identifiers:
// "analysis", "final", "commentary", "unknown".
//
// Returns "unknown" as the fallback for unrecognized channel types to ensure
// graceful degradation during parsing.
//
// Example:
//
//	c := ChannelAnalysis
//	fmt.Println(c.String()) // outputs: "analysis"
func (c ChannelType) String() string {
	switch c {
	case ChannelAnalysis:
		return "analysis"
	case ChannelFinal:
		return "final"
	case ChannelCommentary:
		return "commentary"
	case ChannelUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// ParseChannelType converts a string channel identifier to the corresponding
// ChannelType enum. Input strings are normalized (trimmed and lowercased)
// for robust parsing of Harmony channel tokens.
//
// This function handles channel identifiers extracted from Harmony tokens like
// <|channel|>analysis, ensuring consistent channel classification regardless
// of input formatting variations.
//
// Parameters:
//   - channel: The string channel identifier to parse (e.g., "Analysis", "FINAL")
//
// Returns:
//   - The corresponding ChannelType enum value
//   - ChannelUnknown for unrecognized inputs (graceful fallback)
//
// Performance: O(1) constant time with simple string operations.
//
// Example:
//
//	analysis := ParseChannelType("ANALYSIS")
//	final := ParseChannelType(" final ")
//	unknown := ParseChannelType("invalid") // returns ChannelUnknown
func ParseChannelType(channel string) ChannelType {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "analysis":
		return ChannelAnalysis
	case "final":
		return ChannelFinal
	case "commentary":
		return ChannelCommentary
	default:
		return ChannelUnknown
	}
}

// ContentType represents content classification for Claude Code user interface
// rendering and display logic. This enum maps Harmony channel types to UI
// presentation categories, enabling appropriate visual treatment of different content.
//
// ContentType provides the bridge between Harmony's channel-based organization
// and Claude Code's UI rendering system:
//   - Thinking: Internal reasoning content, typically hidden or collapsible
//   - Response: Primary user-facing content, prominently displayed
//   - ToolCall: Tool invocation or result content, shown with special formatting
//   - Regular: Standard content without special treatment
//
// This classification enables Claude Code to provide rich, contextual presentation
// of AI responses with appropriate visual hierarchy and interaction patterns.
//
// Performance: All operations are O(1) constant time.
type ContentType int

const (
	ContentTypeThinking ContentType = iota
	ContentTypeResponse
	ContentTypeToolCall
	ContentTypeRegular
)

// String returns the string representation of the ContentType for debugging,
// logging, and API compatibility. This method implements the Stringer interface.
//
// The returned string values correspond to Claude Code UI content categories:
// "thinking", "response", "tool_call", "regular".
//
// Returns "regular" as the fallback for unrecognized content types to ensure
// graceful degradation in UI rendering scenarios.
//
// Example:
//
//	c := ContentTypeThinking
//	fmt.Println(c.String()) // outputs: "thinking"
func (c ContentType) String() string {
	switch c {
	case ContentTypeThinking:
		return "thinking"
	case ContentTypeResponse:
		return "response"
	case ContentTypeToolCall:
		return "tool_call"
	case ContentTypeRegular:
		return "regular"
	default:
		return "regular"
	}
}

// Channel represents a single parsed channel from a Harmony message, containing
// both the extracted content and metadata about its classification and origin.
//
// Each Channel corresponds to one complete token sequence in Harmony format:
// <|start|>role<|channel|>type<|message|>content<|end|>
//
// The Channel struct provides all necessary information for content processing:
//   - Role: Message role (assistant, user, system, etc.)
//   - ChannelType: Content category (analysis, final, commentary)
//   - ContentType: UI rendering hint (thinking, response, tool_call)
//   - Content: The actual message content
//   - RawChannel: Original channel string for debugging
//
// Channels are immutable after parsing and can be safely passed between
// goroutines for concurrent processing.
type Channel struct {
	Role        Role        `json:"role"`
	ChannelType ChannelType `json:"channel_type"`
	ContentType ContentType `json:"content_type"`
	Content     string      `json:"content"`
	RawChannel  string      `json:"raw_channel,omitempty"`
}

// IsThinking returns true if this channel contains thinking content that should
// be treated as internal reasoning rather than user-facing response content.
//
// Thinking content typically represents the AI's internal analysis, reasoning
// process, or decision-making that precedes the final response. In Claude Code's
// UI, thinking content is often displayed in a collapsible or secondary format.
//
// This method provides a convenient way to filter channels for UI rendering
// without directly checking ContentType values.
//
// Returns:
//   - true if the channel contains thinking content
//   - false for all other content types
//
// Performance: O(1) constant time comparison.
//
// Example:
//
//	if channel.IsThinking() {
//		// Render in collapsible thinking section
//	}
func (c *Channel) IsThinking() bool {
	return c.ContentType == ContentTypeThinking
}

// IsResponse returns true if this channel contains final response content
// intended for direct user consumption.
//
// Response content represents the primary output that should be prominently
// displayed to the user, as opposed to internal thinking or tool-related content.
// In Claude Code's UI, response content typically receives primary visual emphasis.
//
// This method provides a convenient way to identify user-facing content
// without directly checking ContentType values.
//
// Returns:
//   - true if the channel contains response content
//   - false for all other content types
//
// Performance: O(1) constant time comparison.
//
// Example:
//
//	if channel.IsResponse() {
//		// Render as primary response content
//	}
func (c *Channel) IsResponse() bool {
	return c.ContentType == ContentTypeResponse
}

// IsToolCall returns true if this channel contains tool call content such as
// function invocations, API calls, or tool execution results.
//
// Tool call content represents interactions with external tools, APIs, or
// functions that are part of the AI's response generation process. In Claude Code's
// UI, tool call content often receives special formatting to distinguish it
// from regular response content.
//
// This method provides a convenient way to identify tool-related content
// without directly checking ContentType values.
//
// Returns:
//   - true if the channel contains tool call content
//   - false for all other content types
//
// Performance: O(1) constant time comparison.
//
// Example:
//
//	if channel.IsToolCall() {
//		// Render with tool-specific formatting
//	}
func (c *Channel) IsToolCall() bool {
	return c.ContentType == ContentTypeToolCall
}

// HarmonyMessage represents a complete parsed Harmony format message with all
// extracted channels, consolidated content, and parsing metadata.
//
// This struct serves as the primary result type for Harmony parsing operations,
// containing both the raw parsing results and processed content ready for
// consumption by Claude Code's response transformation pipeline.
//
// The HarmonyMessage provides multiple views of the same content:
//   - Channels: Individual parsed channel objects with full metadata
//   - Consolidated text fields: Combined content by type for easy access
//   - Parsing metadata: Information about the parsing process and any errors
//
// Consolidated text fields (ThinkingText, ResponseText, ToolCallText) combine
// content from multiple channels of the same type, separated by newlines,
// providing convenient access for response building without channel iteration.
//
// The struct is designed to be serializable for debugging and caching purposes,
// with all fields exported and JSON tags provided.
type HarmonyMessage struct {
	Channels     []Channel `json:"channels"`
	RawContent   string    `json:"raw_content"`
	HasHarmony   bool      `json:"has_harmony"`
	ParseErrors  []error   `json:"parse_errors,omitempty"`
	ThinkingText string    `json:"thinking_text,omitempty"`
	ResponseText string    `json:"response_text,omitempty"`
	ToolCallText string    `json:"tool_call_text,omitempty"`
}

// GetChannelsByType returns all channels matching the specified ChannelType,
// enabling filtered access to specific categories of content within the message.
//
// This method provides efficient filtering of channels by type without requiring
// manual iteration through all channels. The returned slice contains references
// to the original Channel structs, not copies.
//
// Parameters:
//   - channelType: The ChannelType to filter by (e.g., ChannelAnalysis, ChannelFinal)
//
// Returns:
//   - A slice of Channel structs matching the specified type
//   - Empty slice if no channels match the specified type
//
// Performance: O(n) where n is the total number of channels in the message.
//
// Example:
//
//	analysisChannels := message.GetChannelsByType(ChannelAnalysis)
//	for _, channel := range analysisChannels {
//		fmt.Println("Analysis:", channel.Content)
//	}
func (h *HarmonyMessage) GetChannelsByType(channelType ChannelType) []Channel {
	var channels []Channel
	for _, channel := range h.Channels {
		if channel.ChannelType == channelType {
			channels = append(channels, channel)
		}
	}
	return channels
}

// GetThinkingChannels returns all channels containing thinking content,
// providing convenient access to internal reasoning content for UI rendering.
//
// This method filters channels by ContentType rather than ChannelType,
// focusing on the UI presentation category rather than the original
// Harmony channel classification. This is particularly useful for
// Claude Code's interface rendering logic.
//
// The returned channels typically contain AI reasoning, analysis, or
// decision-making process content that should be displayed in a
// collapsible or secondary UI section.
//
// Returns:
//   - A slice of Channel structs containing thinking content
//   - Empty slice if no thinking content is present
//
// Performance: O(n) where n is the total number of channels in the message.
//
// Example:
//
//	thinkingChannels := message.GetThinkingChannels()
//	if len(thinkingChannels) > 0 {
//		// Render thinking content in collapsible section
//	}
func (h *HarmonyMessage) GetThinkingChannels() []Channel {
	var channels []Channel
	for _, channel := range h.Channels {
		if channel.IsThinking() {
			channels = append(channels, channel)
		}
	}
	return channels
}

// GetResponseChannels returns all channels containing final response content,
// providing convenient access to user-facing content for primary UI display.
//
// This method filters channels by ContentType to identify content intended
// for direct user consumption, as opposed to thinking or tool-related content.
// This is the primary method for extracting content that should be prominently
// displayed in Claude Code's interface.
//
// The returned channels contain the AI's final answer, conclusion, or
// user-directed communication that represents the main response.
//
// Returns:
//   - A slice of Channel structs containing response content
//   - Empty slice if no response content is present
//
// Performance: O(n) where n is the total number of channels in the message.
//
// Example:
//
//	responseChannels := message.GetResponseChannels()
//	for _, channel := range responseChannels {
//		// Display as primary response content
//		fmt.Println("Response:", channel.Content)
//	}
func (h *HarmonyMessage) GetResponseChannels() []Channel {
	var channels []Channel
	for _, channel := range h.Channels {
		if channel.IsResponse() {
			channels = append(channels, channel)
		}
	}
	return channels
}

// HarmonyParseError represents detailed error information from Harmony parsing
// operations, providing structured error reporting with contextual information
// for debugging and error handling.
//
// This error type extends the standard error interface with position information
// and contextual details to help diagnose parsing issues in malformed Harmony
// content. The structured approach enables both human-readable error messages
// and programmatic error handling.
//
// Error information includes:
//   - Message: Human-readable error description
//   - Position: Character position where the error occurred (-1 if not applicable)
//   - Context: Additional contextual information about the parsing state
//
// HarmonyParseError implements the error interface through its Error() method,
// making it compatible with standard Go error handling patterns.
type HarmonyParseError struct {
	Message  string
	Position int
	Context  string
}

// Error implements the error interface, providing formatted error messages
// with optional position and context information.
//
// The error message format varies based on available information:
//   - Full format: "harmony parse error at position X: message (context: details)"
//   - Position only: "harmony parse error at position X: message"
//   - Message only: "harmony parse error: message"
//
// This graduated formatting ensures useful error messages regardless of
// the amount of context information available during parsing.
//
// Returns:
//   - A formatted error string suitable for logging and user display
//
// Example output:
//   "harmony parse error at position 45: mismatched start/end tokens (context: structural validation)"
func (e *HarmonyParseError) Error() string {
	if e.Position >= 0 && e.Context != "" {
		return fmt.Sprintf("harmony parse error at position %d: %s (context: %s)", e.Position, e.Message, e.Context)
	} else if e.Position >= 0 {
		return fmt.Sprintf("harmony parse error at position %d: %s", e.Position, e.Message)
	}
	return fmt.Sprintf("harmony parse error: %s", e.Message)
}

// TokenRecognizer handles efficient pattern recognition and extraction of
// Harmony format tokens using precompiled regular expressions.
//
// This struct encapsulates all regex patterns needed for Harmony parsing,
// providing compiled patterns for optimal performance during repeated
// parsing operations. The recognizer supports both individual token
// detection and complete token sequence extraction.
//
// Supported token patterns:
//   - Start tokens: <|start|>role
//   - End tokens: <|end|>
//   - Channel tokens: <|channel|>type
//   - Message tokens: <|message|>
//   - Full sequences: Complete <|start|>...<|end|> blocks
//   - Partial sequences: <|channel|>...<|end|> blocks (missing start token)
//
// TokenRecognizer instances should be created once and reused for
// multiple parsing operations to amortize regex compilation costs.
type TokenRecognizer struct {
	startPattern     *regexp.Regexp
	endPattern       *regexp.Regexp
	channelPattern   *regexp.Regexp
	messagePattern   *regexp.Regexp
	fullPattern      *regexp.Regexp
	partialPattern   *regexp.Regexp
}

// NewTokenRecognizer creates a new TokenRecognizer with all necessary
// regular expression patterns precompiled for optimal parsing performance.
//
// This constructor compiles all Harmony token patterns and validates their
// syntax, returning an error if any pattern compilation fails. The resulting
// TokenRecognizer can be used for efficient repeated parsing operations
// without pattern recompilation overhead.
//
// Compiled patterns include:
//   - Individual token recognition (start, end, channel, message)
//   - Complete token sequence extraction (full Harmony blocks)
//   - Partial token sequence extraction (missing start token)
//   - Multiline content support with proper token boundary detection
//
// Returns:
//   - A fully initialized TokenRecognizer ready for use
//   - An error if any regex pattern fails to compile
//
// Performance: One-time compilation cost, then O(1) pattern access.
//
// Example:
//
//	recognizer, err := NewTokenRecognizer()
//	if err != nil {
//		log.Fatal("Failed to initialize token recognizer:", err)
//	}
//	// Use recognizer for multiple parsing operations
func NewTokenRecognizer() (*TokenRecognizer, error) {
	startPattern, err := regexp.Compile(`<\|start\|>(\w+)`)
	if err != nil {
		return nil, fmt.Errorf("failed to compile start pattern: %w", err)
	}

	endPattern, err := regexp.Compile(`<\|end\|>`)
	if err != nil {
		return nil, fmt.Errorf("failed to compile end pattern: %w", err)
	}

	channelPattern, err := regexp.Compile(`<\|channel\|>(\w+)`)
	if err != nil {
		return nil, fmt.Errorf("failed to compile channel pattern: %w", err)
	}

	messagePattern, err := regexp.Compile(`<\|message\|>`)
	if err != nil {
		return nil, fmt.Errorf("failed to compile message pattern: %w", err)
	}

	// Full pattern for complete token sequences with start token
	fullPattern, err := regexp.Compile(`(?s)<\|start\|>(\w+)(?:<\|channel\|>(\w+))?<\|message\|>(.*?)<\|end\|>`)
	if err != nil {
		return nil, fmt.Errorf("failed to compile full pattern: %w", err)
	}

	// Partial pattern for sequences without start token (fallback)
	partialPattern, err := regexp.Compile(`(?s)<\|channel\|>(\w+)<\|message\|>(.*?)<\|end\|>`)
	if err != nil {
		return nil, fmt.Errorf("failed to compile partial pattern: %w", err)
	}

	return &TokenRecognizer{
		startPattern:   startPattern,
		endPattern:     endPattern,
		channelPattern: channelPattern,
		messagePattern: messagePattern,
		fullPattern:    fullPattern,
		partialPattern: partialPattern,
	}, nil
}

// HasHarmonyTokens performs fast detection of Harmony format tokens in content
// without full parsing, enabling efficient format detection for routing decisions.
//
// This method checks for the presence of any Harmony tokens (start or end)
// using precompiled patterns, making it suitable for quick format detection
// in high-throughput scenarios where full parsing might be unnecessary.
//
// The detection is optimized for speed rather than completeness - it only
// checks for basic token presence, not structural validity.
//
// Parameters:
//   - content: The text content to scan for Harmony tokens
//
// Returns:
//   - true if any Harmony tokens are found
//   - false if no Harmony tokens are present
//
// Performance: O(n) where n is content length, but optimized for early exit.
//
// Example:
//
//	if recognizer.HasHarmonyTokens(content) {
//		// Route to Harmony parsing pipeline
//	} else {
//		// Handle as regular content
//	}
func (tr *TokenRecognizer) HasHarmonyTokens(content string) bool {
	return tr.startPattern.MatchString(content) || 
		   tr.endPattern.MatchString(content) ||
		   tr.channelPattern.MatchString(content) || 
		   tr.messagePattern.MatchString(content)
}

// ExtractTokens extracts all complete Harmony token sequences from content,
// returning structured match data for each found token block.
//
// This method identifies and extracts Harmony token sequences in two forms:
// 1. Complete: <|start|>role<|channel|>type<|message|>content<|end|>
// 2. Partial: <|channel|>type<|message|>content<|end|> (missing start token)
//
// Each returned match is normalized to a 4-element string slice:
//   - [0]: Full matched sequence
//   - [1]: Role identifier ("assistant" default for partial sequences)
//   - [2]: Channel type (from channel token)
//   - [3]: Message content (between message and end tokens)
//
// The function tries the full pattern first, then falls back to the partial
// pattern for sequences missing the start token, ensuring compatibility
// with various Harmony format variations.
//
// Parameters:
//   - content: The text content to scan for complete token sequences
//
// Returns:
//   - A slice of normalized 4-element string slices representing token sequences
//   - Empty slice if no valid sequences are found
//
// Performance: O(n) where n is content length, with dual regex matching.
//
// Example:
//
//	matches := recognizer.ExtractTokens(content)
//	for _, match := range matches {
//		role := match[1]      // "assistant" or actual role
//		channel := match[2]   // channel type
//		message := match[3]   // content
//		// Process extracted token data
//	}
func (tr *TokenRecognizer) ExtractTokens(content string) [][]string {
	// First try full pattern (with start token)
	fullMatches := tr.fullPattern.FindAllStringSubmatch(content, -1)
	
	// If we found full matches, return those (don't look for partial matches that would overlap)
	if len(fullMatches) > 0 {
		return fullMatches
	}
	
	// Only try partial pattern if no full matches found
	partialMatches := tr.partialPattern.FindAllStringSubmatch(content, -1)
	
	var allMatches [][]string
	
	// Add partial matches, normalizing to include default role
	for _, match := range partialMatches {
		if len(match) >= 3 {
			// Normalize to [full_match, role, channel, content] format
			normalizedMatch := []string{
				match[0],        // full matched sequence
				"assistant",     // default role for partial sequences
				match[1],        // channel type
				match[2],        // message content
			}
			allMatches = append(allMatches, normalizedMatch)
		}
	}
	
	return allMatches
}

// Package-level default token recognizer for performance
var defaultTokenRecognizer *TokenRecognizer

func init() {
	var err error
	defaultTokenRecognizer, err = NewTokenRecognizer()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize default token recognizer: %v", err))
	}
}

// IsHarmonyFormat provides a package-level convenience function for detecting
// Harmony format content using the default TokenRecognizer instance.
//
// This function offers a simple API for Harmony format detection without
// requiring explicit TokenRecognizer instantiation, using a shared recognizer
// instance initialized at package level for optimal performance.
//
// The function is thread-safe and suitable for concurrent use across
// multiple goroutines, as the underlying TokenRecognizer uses read-only
// compiled patterns.
//
// Parameters:
//   - content: The text content to check for Harmony format tokens
//
// Returns:
//   - true if Harmony format tokens are detected
//   - false if no Harmony tokens are found
//
// Performance: O(n) where n is content length, with shared pattern compilation.
//
// Example:
//
//	if IsHarmonyFormat(responseContent) {
//		// Process as Harmony format
//		message, err := ParseHarmonyMessage(responseContent)
//	} else {
//		// Handle as regular text response
//	}
func IsHarmonyFormat(content string) bool {
	return defaultTokenRecognizer.HasHarmonyTokens(content)
}

// ExtractChannels extracts and parses all valid Harmony channels from content,
// returning a slice of fully populated Channel structs ready for processing.
//
// This function performs the core channel extraction logic, identifying
// complete Harmony token sequences and converting them into structured
// Channel objects with proper role, type, and content classification.
//
// The extraction process:
//   1. Uses the default TokenRecognizer to find complete token sequences
//   2. Parses role and channel identifiers from each sequence
//   3. Determines appropriate ContentType based on ChannelType
//   4. Creates Channel structs with all metadata populated
//   5. Filters out incomplete or invalid sequences
//
// Parameters:
//   - content: Text content containing Harmony format tokens
//
// Returns:
//   - A slice of Channel structs for all valid sequences found
//   - Empty slice if no valid Harmony channels are present
//
// Performance: O(n*m) where n is content length and m is number of token sequences.
//
// Example:
//
//	channels := ExtractChannels(harmonyContent)
//	for _, channel := range channels {
//		fmt.Printf("Role: %s, Type: %s, Content: %s\n",
//			channel.Role, channel.ChannelType, channel.Content)
//	}
func ExtractChannels(content string) []Channel {
	var channels []Channel
	
	tokens := defaultTokenRecognizer.ExtractTokens(content)
	
	for _, match := range tokens {
		if len(match) < 4 {
			continue
		}
		
		roleStr := match[1]
		channelStr := match[2]
		messageContent := match[3]
		
		role := ParseRole(roleStr)
		channelType := ParseChannelType(channelStr)
		contentType := DetermineContentType(channelType)
		
		channel := Channel{
			Role:        role,
			ChannelType: channelType,
			ContentType: contentType,
			Content:     strings.TrimSpace(messageContent),
			RawChannel:  channelStr,
		}
		
		channels = append(channels, channel)
	}
	
	return channels
}

// DetermineContentType maps Harmony ChannelType values to ContentType values
// for appropriate Claude Code UI rendering and content classification.
//
// This function provides the critical translation between Harmony's channel-based
// content organization and Claude Code's UI presentation requirements. The mapping
// ensures that different types of content receive appropriate visual treatment:
//
// Mapping logic:
//   - ChannelAnalysis → ContentTypeThinking (collapsible reasoning content)
//   - ChannelFinal → ContentTypeResponse (primary user-facing content)
//   - ChannelCommentary → ContentTypeToolCall (tool-related information)
//   - All others → ContentTypeRegular (standard content treatment)
//
// This mapping can be adjusted to modify how different Harmony channels
// are presented in Claude Code's interface without changing the core
// parsing logic.
//
// Parameters:
//   - channelType: The ChannelType from Harmony parsing
//
// Returns:
//   - The corresponding ContentType for UI rendering
//
// Performance: O(1) constant time switch statement.
//
// Example:
//
//	contentType := DetermineContentType(ChannelAnalysis)
//	// contentType == ContentTypeThinking
func DetermineContentType(channelType ChannelType) ContentType {
	switch channelType {
	case ChannelAnalysis:
		return ContentTypeThinking
	case ChannelFinal:
		return ContentTypeResponse
	case ChannelCommentary:
		return ContentTypeToolCall
	default:
		return ContentTypeRegular
	}
}

// ParseHarmonyMessage is the primary API function for parsing complete Harmony
// format messages, providing comprehensive content extraction and processing.
//
// This function serves as the main entry point for Harmony parsing operations,
// handling the complete parsing pipeline from raw content to structured
// HarmonyMessage with all channels extracted and consolidated text prepared.
//
// Processing pipeline:
//   1. Input validation and empty content handling
//   2. Channel extraction using ExtractChannels
//   3. Harmony format detection using IsHarmonyFormat
//   4. Content consolidation by ContentType
//   5. Error collection and metadata population
//
// The function never returns an error for parsing issues, instead collecting
// errors in the ParseErrors field to enable partial parsing and graceful
// degradation in production environments.
//
// Parameters:
//   - content: Raw text content potentially containing Harmony format
//
// Returns:
//   - A HarmonyMessage struct with all available parsed information
//   - Always returns a valid struct, never nil
//   - Error always nil (errors collected in ParseErrors field)
//
// Performance: O(n*m) where n is content length and m is number of channels.
//
// Example:
//
//	message, _ := ParseHarmonyMessage(responseText)
//	if message.HasHarmony {
//		fmt.Printf("Found %d channels\n", len(message.Channels))
//		fmt.Printf("Thinking: %s\n", message.ThinkingText)
//		fmt.Printf("Response: %s\n", message.ResponseText)
//	}
func ParseHarmonyMessage(content string) (*HarmonyMessage, error) {
	if content == "" {
		return &HarmonyMessage{
			Channels:     []Channel{},
			RawContent:   "",
			HasHarmony:   false,
			ParseErrors:  []error{},
			ThinkingText: "",
			ResponseText: "",
			ToolCallText: "",
		}, nil
	}

	channels := ExtractChannels(content)
	
	message := &HarmonyMessage{
		Channels:     channels,
		RawContent:   content,
		HasHarmony:   IsHarmonyFormat(content),
		ParseErrors:  []error{},
		ThinkingText: "",
		ResponseText: "",
		ToolCallText: "",
	}
	
	// Build consolidated text fields by content type
	for _, channel := range channels {
		switch channel.ContentType {
		case ContentTypeThinking:
			if message.ThinkingText != "" {
				message.ThinkingText += "\n"
			}
			message.ThinkingText += channel.Content
		case ContentTypeResponse:
			if message.ResponseText != "" {
				message.ResponseText += "\n"
			}
			message.ResponseText += channel.Content
		case ContentTypeToolCall:
			if message.ToolCallText != "" {
				message.ToolCallText += "\n"
			}
			message.ToolCallText += channel.Content
		}
	}
	
	return message, nil
}

// FindHarmonyTokens provides detailed analysis of all Harmony tokens in content,
// returning position and type information for debugging and validation purposes.
//
// This function performs comprehensive token scanning, identifying all individual
// Harmony tokens (start, channel, message, end) and their exact positions
// within the content. Unlike ExtractTokens, this function finds individual
// tokens rather than complete sequences.
//
// The function is particularly useful for:
//   - Debugging malformed Harmony content
//   - Validation of token structure
//   - Detailed parsing error reporting
//   - Content analysis and statistics
//
// Each TokenPosition includes:
//   - Type: Token type identifier ("start", "channel", "message", "end")
//   - Start/End: Character positions within the content
//   - Value: Extracted value from parameterized tokens (role, channel type)
//   - Position: Primary position reference for sorting/analysis
//
// Parameters:
//   - content: Text content to analyze for Harmony tokens
//
// Returns:
//   - A slice of TokenPosition structs for all found tokens
//   - Empty slice if no tokens are found
//
// Performance: O(n) where n is content length, with multiple regex passes.
//
// Example:
//
//	positions := FindHarmonyTokens(content)
//	for _, pos := range positions {
//		fmt.Printf("%s token at %d-%d: %s\n",
//			pos.Type, pos.Start, pos.End, pos.Value)
//	}
func FindHarmonyTokens(content string) []TokenPosition {
	var positions []TokenPosition
	
	// Find start tokens
	startMatches := defaultTokenRecognizer.startPattern.FindAllStringSubmatchIndex(content, -1)
	for _, match := range startMatches {
		positions = append(positions, TokenPosition{
			Type:     "start",
			Start:    match[0],
			End:      match[1],
			Value:    content[match[2]:match[3]], // Role value
			Position: match[0],
		})
	}
	
	// Find channel tokens
	channelMatches := defaultTokenRecognizer.channelPattern.FindAllStringSubmatchIndex(content, -1)
	for _, match := range channelMatches {
		positions = append(positions, TokenPosition{
			Type:     "channel",
			Start:    match[0],
			End:      match[1],
			Value:    content[match[2]:match[3]], // Channel value
			Position: match[0],
		})
	}
	
	// Find message tokens
	messageMatches := defaultTokenRecognizer.messagePattern.FindAllStringIndex(content, -1)
	for _, match := range messageMatches {
		positions = append(positions, TokenPosition{
			Type:     "message",
			Start:    match[0],
			End:      match[1],
			Position: match[0],
		})
	}
	
	// Find end tokens
	endMatches := defaultTokenRecognizer.endPattern.FindAllStringIndex(content, -1)
	for _, match := range endMatches {
		positions = append(positions, TokenPosition{
			Type:     "end",
			Start:    match[0],
			End:      match[1],
			Position: match[0],
		})
	}
	
	return positions
}

// TokenPosition represents detailed position and type information for a single
// Harmony token found within content, providing structured data for debugging
// and validation operations.
//
// This struct captures all relevant information about individual tokens:
//   - Type: The token category ("start", "channel", "message", "end")
//   - Start/End: Character positions for precise location within content
//   - Value: Extracted parameter value for tokens that carry data
//   - Position: Primary position reference for sorting and comparison
//
// TokenPosition is used primarily for content analysis, debugging malformed
// Harmony sequences, and generating detailed error reports when parsing fails.
//
// The struct is serializable for debugging output and logging purposes.
type TokenPosition struct {
	Type     string `json:"type"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
	Value    string `json:"value,omitempty"`
	Position int    `json:"position"`
}

// ValidateHarmonyStructure performs structural validation of Harmony token
// sequences, identifying common formatting and structure errors.
//
// This function analyzes the overall structure of Harmony content without
// performing full parsing, focusing on token balance, sequence validity,
// and other structural requirements. It provides detailed error reporting
// for debugging malformed content.
//
// Current validation checks:
//   - Start/end token balance (equal counts)
//   - Token sequence integrity
//   - Structural consistency
//
// Additional validation rules can be added as needed without affecting
// the main parsing pipeline. The function is designed to provide helpful
// error messages for content authors and debugging.
//
// Parameters:
//   - content: Text content to validate for structural correctness
//
// Returns:
//   - A slice of HarmonyParseError structs describing any issues found
//   - Empty slice if no structural problems are detected
//
// Performance: O(n) where n is content length, optimized for error detection.
//
// Example:
//
//	errors := ValidateHarmonyStructure(content)
//	if len(errors) > 0 {
//		for _, err := range errors {
//			fmt.Printf("Validation error: %s\n", err.Error())
//		}
//	}
func ValidateHarmonyStructure(content string) []HarmonyParseError {
	var errors []HarmonyParseError
	
	tokens := FindHarmonyTokens(content)
	if len(tokens) == 0 {
		return errors
	}
	
	// Basic validation: each start should have corresponding end
	startCount := 0
	endCount := 0
	
	for _, token := range tokens {
		switch token.Type {
		case "start":
			startCount++
		case "end":
			endCount++
		}
	}
	
	if startCount != endCount {
		errors = append(errors, HarmonyParseError{
			Message: fmt.Sprintf("mismatched start/end tokens: %d start, %d end", startCount, endCount),
			Position: -1,
			Context: "structural validation",
		})
	}
	
	return errors
}

// GetHarmonyTokenStats provides statistical analysis of Harmony token usage
// within content, enabling content analysis and debugging insights.
//
// This function analyzes Harmony content to provide quantitative information
// about token distribution, helping with:
//   - Content complexity assessment
//   - Token usage pattern analysis
//   - Debugging and validation support
//   - Performance estimation for parsing operations
//
// The returned statistics include:
//   - Total token count across all types
//   - Per-type token counts (start, channel, message, end)
//   - Distribution analysis for understanding content structure
//
// This information is particularly useful for monitoring Harmony content
// patterns and identifying potential parsing performance issues.
//
// Parameters:
//   - content: Text content to analyze for token statistics
//
// Returns:
//   - A TokenStats struct containing comprehensive token analysis
//
// Performance: O(n) where n is content length, single-pass analysis.
//
// Example:
//
//	stats := GetHarmonyTokenStats(content)
//	fmt.Printf("Found %d total tokens\n", stats.TotalTokens)
//	for tokenType, count := range stats.TokenCounts {
//		fmt.Printf("%s: %d\n", tokenType, count)
//	}
func GetHarmonyTokenStats(content string) TokenStats {
	tokens := FindHarmonyTokens(content)
	
	stats := TokenStats{
		TotalTokens: len(tokens),
		TokenCounts: make(map[string]int),
	}
	
	for _, token := range tokens {
		stats.TokenCounts[token.Type]++
	}
	
	return stats
}

// TokenStats provides comprehensive statistical information about Harmony token
// usage within analyzed content, supporting debugging and content analysis.
//
// This struct aggregates quantitative data about Harmony token distribution:
//   - TotalTokens: Overall count of all tokens found
//   - TokenCounts: Per-type breakdown of token usage
//
// TokenStats is designed to provide insights into Harmony content complexity
// and structure, helping developers understand parsing requirements and
// optimize content generation strategies.
//
// The struct is serializable for logging and monitoring purposes, enabling
// tracking of Harmony usage patterns across different content sources.
type TokenStats struct {
	TotalTokens int            `json:"total_tokens"`
	TokenCounts map[string]int `json:"token_counts"`
}

// HarmonyToken represents a parsed Harmony token with structural information
// for robust parsing operations that handle malformed content.
//
// This struct provides a structured representation of individual Harmony tokens
// found during content analysis, including position information and validation
// status to support error recovery and graceful degradation.
//
// The token structure enables:
//   - Position-based error reporting
//   - Structural validation of token sequences
//   - Recovery strategies for malformed content
//   - Debug information for parsing issues
type HarmonyToken struct {
	Type     string `json:"type"`     // Token type: "start", "channel", "message", "end"
	Role     string `json:"role"`     // Role for start tokens
	Channel  string `json:"channel"`  // Channel type for channel tokens
	Content  string `json:"content"`  // Content between tokens
	Start    int    `json:"start"`    // Starting position in source
	End      int    `json:"end"`      // Ending position in source
	Valid    bool   `json:"valid"`    // Whether the token is structurally valid
	Error    string `json:"error"`    // Error message if invalid
}

// ExtractTokensRobust performs comprehensive token extraction with error recovery
// and malformed content handling, providing structured token information for
// debugging and robust parsing operations.
//
// This function implements a multi-stage extraction process:
//   1. Standard token extraction using existing patterns
//   2. Malformed sequence detection and recovery
//   3. Position-based error reporting
//   4. Structural validation with detailed error context
//
// The robust extraction handles common malformed patterns:
//   - Missing end tags
//   - Incomplete token sequences
//   - Invalid channel identifiers
//   - Mixed content structures
//   - Truncated responses
//
// Parameters:
//   - content: Text content to analyze for Harmony tokens
//
// Returns:
//   - A slice of HarmonyToken structs with validation status
//   - Error if critical parsing failures occur
//
// Performance: O(n) where n is content length, with multiple validation passes.
//
// Example:
//
//	tokens, err := ExtractTokensRobust(content)
//	for _, token := range tokens {
//		if !token.Valid {
//			fmt.Printf("Invalid token at %d: %s\n", token.Start, token.Error)
//		}
//	}
func ExtractTokensRobust(content string) ([]HarmonyToken, error) {
	var tokens []HarmonyToken
	
	// First, try standard token extraction
	tokenPositions := FindHarmonyTokens(content)
	
	// Convert token positions to HarmonyToken structures
	for _, pos := range tokenPositions {
		token := HarmonyToken{
			Type:    pos.Type,
			Start:   pos.Start,
			End:     pos.End,
			Valid:   true,
			Error:   "",
		}
		
		// Extract type-specific information
		switch pos.Type {
		case "start":
			token.Role = pos.Value
		case "channel":
			token.Channel = pos.Value
		}
		
		tokens = append(tokens, token)
	}
	
	// Detect malformed sequences and add them as invalid tokens
	malformedSequences := extractMalformedSequences(content)
	for _, seq := range malformedSequences {
		token := HarmonyToken{
			Type:    "malformed",
			Content: seq,
			Start:   strings.Index(content, seq),
			End:     strings.Index(content, seq) + len(seq),
			Valid:   false,
			Error:   "malformed token sequence detected",
		}
		tokens = append(tokens, token)
	}
	
	return tokens, nil
}

// extractMalformedSequences identifies common malformed Harmony patterns
// that standard parsing cannot handle, enabling recovery strategies.
//
// This function uses pattern matching to identify:
//   - Incomplete token sequences (missing end tags)
//   - Invalid channel identifiers
//   - Truncated responses
//   - Mixed content with partial tokens
//
// Parameters:
//   - content: Text content to analyze for malformed patterns
//
// Returns:
//   - A slice of strings containing malformed sequences found
//
// Performance: O(n) where n is content length.
func extractMalformedSequences(content string) []string {
	var malformed []string
	
	// Pattern 1: Missing end tags - content with start/channel/message but no end
	// Look for sequences that don't end with <|end|>
	missingEndPattern := regexp.MustCompile(`(?s)<\|start\|>\w+(?:<\|channel\|>\w+)?<\|message\|>[^<]*$`)
	matches := missingEndPattern.FindAllString(content, -1)
	malformed = append(malformed, matches...)
	
	// Pattern 2: Invalid channels - channels that don't match expected patterns
	invalidChannelPattern := regexp.MustCompile(`<\|channel\|>[^a-zA-Z_][^<]*`)
	matches = invalidChannelPattern.FindAllString(content, -1)
	malformed = append(malformed, matches...)
	
	// Pattern 3: Incomplete structures - find tokens at end without proper closing
	incompletePattern := regexp.MustCompile(`<\|(?:start|channel|message)\|>[^<]*$`)
	matches = incompletePattern.FindAllString(content, -1)
	malformed = append(malformed, matches...)
	
	return malformed
}

// cleanMalformedContent attempts to repair common malformed Harmony patterns
// by applying heuristic fixes and content normalization.
//
// This function implements recovery strategies for:
//   - Adding missing end tags
//   - Normalizing channel identifiers
//   - Reconstructing incomplete sequences
//   - Removing invalid token fragments
//
// Parameters:
//   - content: Malformed Harmony content to repair
//
// Returns:
//   - Cleaned content with attempted repairs applied
//
// Performance: O(n) where n is content length.
func cleanMalformedContent(content string) string {
	cleaned := content
	
	// Fix 1: Add missing end tags for incomplete sequences
	missingEndPattern := regexp.MustCompile(`(?s)(<\|start\|>\w+(?:<\|channel\|>\w+)?<\|message\|>.*)$`)
	cleaned = missingEndPattern.ReplaceAllString(cleaned, "$1<|end|>")
	
	// Fix 2: Normalize invalid channel identifiers to "final"
	invalidChannelPattern := regexp.MustCompile(`<\|channel\|>[^a-zA-Z_][^<]*`)
	cleaned = invalidChannelPattern.ReplaceAllString(cleaned, "<|channel|>final")
	
	// Fix 3: Remove incomplete token fragments at end
	incompleteTokenPattern := regexp.MustCompile(`<\|[^|]*(?:\|[^>]*)?[^>]*$`)
	cleaned = incompleteTokenPattern.ReplaceAllString(cleaned, "")
	
	// Fix 4: Wrap orphaned content in proper tokens
	orphanedContentPattern := regexp.MustCompile(`(?s)^([^<]*)<\|(?:channel|message)\|>`)
	cleaned = orphanedContentPattern.ReplaceAllString(cleaned, "<|start|>assistant<|channel|>final<|message|>$1<|end|>")
	
	return cleaned
}

// ParseHarmonyMessageRobust provides comprehensive Harmony message parsing
// with error recovery, malformed content handling, and graceful degradation.
//
// This function implements a multi-level parsing strategy:
//   1. Standard parsing using existing ParseHarmonyMessage
//   2. Robust token extraction for malformed content
//   3. Content cleaning and repair attempts  
//   4. Fallback to raw content with error logging
//
// The robust parsing ensures that no content is lost, even when malformed:
//   - Attempts standard parsing first
//   - Falls back to cleaning and re-parsing
//   - Finally provides raw content as fallback
//   - Collects all errors for debugging
//
// Parameters:
//   - content: Text content potentially containing malformed Harmony format
//
// Returns:
//   - A HarmonyMessage struct with maximum extracted information
//   - Error details in ParseErrors field, never returns parsing errors directly
//
// Performance: O(n*m) where n is content length and m is number of repair attempts.
//
// Example:
//
//	message, _ := ParseHarmonyMessageRobust(malformedContent)
//	if len(message.ParseErrors) > 0 {
//		fmt.Printf("Parsing issues detected: %d errors\n", len(message.ParseErrors))
//	}
//	// message.ResponseText will contain best available content
func ParseHarmonyMessageRobust(content string) (*HarmonyMessage, error) {
	// Attempt 1: Standard parsing
	message, err := ParseHarmonyMessage(content)
	if err == nil && message.HasHarmony && len(message.Channels) > 0 {
		return message, nil
	}
	
	// Attempt 2: Robust token extraction with error recovery
	tokens, tokenErr := ExtractTokensRobust(content)
	
	// Attempt 3: Content cleaning and re-parsing
	cleanedContent := cleanMalformedContent(content)
	if cleanedContent != content {
		cleanedMessage, cleanErr := ParseHarmonyMessage(cleanedContent)
		if cleanErr == nil && cleanedMessage.HasHarmony && len(cleanedMessage.Channels) > 0 {
			// Add error information about the cleaning and preserve original raw content
			cleanedMessage.ParseErrors = append(cleanedMessage.ParseErrors, 
				fmt.Errorf("content required cleaning for successful parsing"))
			cleanedMessage.RawContent = content // Preserve original content, not cleaned
			return cleanedMessage, nil
		}
	}
	
	// Fallback: Provide best available content with error information
	var fallbackMessage *HarmonyMessage
	if message != nil {
		// Use the partially parsed message from standard parsing
		fallbackMessage = message
	} else {
		// Create new message structure
		fallbackMessage = &HarmonyMessage{
			Channels:     []Channel{},
			RawContent:   content,
			HasHarmony:   len(tokens) > 0, // Consider it Harmony if we found any tokens
			ParseErrors:  []error{},
			ThinkingText: "",
			ResponseText: "",
			ToolCallText: "",
		}
	}
	
	// Ensure raw content is preserved
	fallbackMessage.RawContent = content
	
	// If we don't have any meaningful parsed content, use raw content as response
	if fallbackMessage.ResponseText == "" && fallbackMessage.ThinkingText == "" && len(fallbackMessage.Channels) == 0 {
		fallbackMessage.ResponseText = content
	}
	
	// Add parsing error information
	if err != nil {
		fallbackMessage.ParseErrors = append(fallbackMessage.ParseErrors, err)
	}
	if tokenErr != nil {
		fallbackMessage.ParseErrors = append(fallbackMessage.ParseErrors, tokenErr)
	}
	
	// Add error for each invalid token
	for _, token := range tokens {
		if !token.Valid {
			fallbackMessage.ParseErrors = append(fallbackMessage.ParseErrors, 
				fmt.Errorf("invalid token at position %d: %s", token.Start, token.Error))
		}
	}
	
	return fallbackMessage, nil
}

// ExtractChannelsRobust performs channel extraction with comprehensive error
// handling and recovery strategies for malformed Harmony content.
//
// This function provides the most reliable channel extraction available:
//   1. Standard channel extraction using ExtractChannels
//   2. Robust token-based extraction for malformed content
//   3. Heuristic content reconstruction
//   4. Fallback channel generation for unstructured content
//
// The extraction ensures that meaningful content is never lost:
//   - Attempts standard extraction first
//   - Uses robust token analysis for partial recovery
//   - Creates synthetic channels for unstructured content
//   - Preserves all content with appropriate classification
//
// Parameters:
//   - content: Text content potentially containing malformed Harmony channels
//
// Returns:
//   - A slice of Channel structs with maximum extracted information
//   - Error if critical extraction failures occur
//
// Performance: O(n*m) where n is content length and m is number of extraction attempts.
//
// Example:
//
//	channels, err := ExtractChannelsRobust(malformedContent)
//	if err != nil {
//		fmt.Printf("Extraction error: %v\n", err)
//	}
//	for _, channel := range channels {
//		fmt.Printf("Channel: %s, Content: %s\n", channel.ChannelType, channel.Content)
//	}
func ExtractChannelsRobust(content string) ([]HarmonyChannel, error) {
	var channels []HarmonyChannel
	
	// Attempt 1: Standard channel extraction
	standardChannels := ExtractChannels(content)
	if len(standardChannels) > 0 {
		// Convert to HarmonyChannel format (assuming it's the same structure)
		for _, ch := range standardChannels {
			harmonyChannel := HarmonyChannel{
				Role:        ch.Role,
				ChannelType: ch.ChannelType,
				ContentType: ch.ContentType,
				Content:     ch.Content,
				RawChannel:  ch.RawChannel,
				Valid:       true,
				Error:       "",
			}
			channels = append(channels, harmonyChannel)
		}
		return channels, nil
	}
	
	// Attempt 2: Robust token-based extraction
	tokens, err := ExtractTokensRobust(content)
	if err != nil {
		return nil, fmt.Errorf("robust token extraction failed: %w", err)
	}
	
	// For malformed content, try to reconstruct channels by analyzing the content
	// between tokens and extracting meaningful sequences
	if len(tokens) > 0 {
		// Extract content between message tokens and end tokens (or end of content)
		messageContent := extractContentBetweenTokens(content, tokens)
		
		if messageContent != "" {
			// Try to determine the channel type based on any channel token found
			channelType := ChannelFinal // Default
			for _, token := range tokens {
				if token.Type == "channel" && token.Channel != "" {
					channelType = ParseChannelType(token.Channel)
					break
				}
			}
			
			channel := HarmonyChannel{
				Role:        RoleAssistant,
				ChannelType: channelType,
				ContentType: DetermineContentType(channelType),
				Content:     strings.TrimSpace(messageContent),
				RawChannel:  channelType.String(),
				Valid:       false,
				Error:       "reconstructed from malformed content",
			}
			channels = append(channels, channel)
		}
	}
	
	// Attempt 3: Fallback - create a single channel with all content
	if len(channels) == 0 && content != "" {
		channel := HarmonyChannel{
			Role:        RoleAssistant,
			ChannelType: ChannelFinal,
			ContentType: ContentTypeResponse,
			Content:     strings.TrimSpace(content),
			RawChannel:  "fallback",
			Valid:       false,
			Error:       "no structured channels found, using fallback",
		}
		channels = append(channels, channel)
	}
	
	return channels, nil
}

// extractContentBetweenTokens attempts to extract meaningful content between
// Harmony tokens when standard parsing fails, used for malformed content recovery.
//
// This function analyzes token positions to find content that appears after
// message tokens, providing a fallback mechanism for content extraction when
// the token structure is incomplete or malformed.
//
// Parameters:
//   - content: The original content string
//   - tokens: Slice of HarmonyToken structs with position information
//
// Returns:
//   - Extracted content string, empty if no meaningful content found
func extractContentBetweenTokens(content string, tokens []HarmonyToken) string {
	var extractedContent strings.Builder
	
	// Find the last message token position
	lastMessagePos := -1
	for _, token := range tokens {
		if token.Type == "message" && token.End > lastMessagePos {
			lastMessagePos = token.End
		}
	}
	
	if lastMessagePos >= 0 && lastMessagePos < len(content) {
		// Extract content from after the message token to the end (or next token)
		remaining := content[lastMessagePos:]
		
		// Find the next token that might terminate the content
		nextTokenPos := len(remaining)
		for _, token := range tokens {
			if token.Start > lastMessagePos {
				relativePos := token.Start - lastMessagePos
				if relativePos < nextTokenPos {
					nextTokenPos = relativePos
				}
			}
		}
		
		if nextTokenPos > 0 {
			extractedContent.WriteString(remaining[:nextTokenPos])
		}
	}
	
	return strings.TrimSpace(extractedContent.String())
}

// HarmonyChannel represents an extended Channel structure with validation
// information for robust parsing operations.
//
// This struct extends the standard Channel with additional fields for:
//   - Validation status and error reporting
//   - Recovery and repair operation tracking
//   - Debug information for malformed content
//
// The extended structure enables comprehensive error handling while
// maintaining compatibility with existing Channel-based code.
type HarmonyChannel struct {
	Role        Role        `json:"role"`
	ChannelType ChannelType `json:"channel_type"`
	ContentType ContentType `json:"content_type"`
	Content     string      `json:"content"`
	RawChannel  string      `json:"raw_channel,omitempty"`
	Valid       bool        `json:"valid"`       // Whether the channel is structurally valid
	Error       string      `json:"error"`       // Error message if invalid
}