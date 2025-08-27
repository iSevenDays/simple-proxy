package parser

import (
	"errors"
	"regexp"
)

// Harmony token constants used for parsing OpenAI Harmony format messages
const (
	// Token patterns for OpenAI Harmony format
	StartToken   = "<|start|>"
	ChannelToken = "<|channel|>"
	MessageToken = "<|message|>"
	EndToken     = "<|end|>"
	ReturnToken  = "<|return|>"
)

// Role represents the role of a message in the Harmony format
type Role string

// Role constants matching OpenAI Harmony specification
const (
	RoleAssistant Role = "assistant"
	RoleUser      Role = "user"
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer"
	RoleTool      Role = "tool"
)

// ChannelType represents the type of channel in the Harmony format
type ChannelType string

// Channel type constants with their corresponding content classification
const (
	ChannelAnalysis    ChannelType = "analysis"    // Internal thinking/reasoning content
	ChannelFinal       ChannelType = "final"       // User-facing response content
	ChannelCommentary  ChannelType = "commentary"  // Tool calls and intermediate processing
	ChannelUnknown     ChannelType = "unknown"     // Fallback for unrecognized channels
)

// ContentType represents how the content should be classified and handled
type ContentType string

// Content type constants for response transformation
const (
	ContentTypeThinking  ContentType = "thinking"   // Analysis content for thinking panel
	ContentTypeResponse  ContentType = "response"   // Main user-facing response
	ContentTypeToolCall  ContentType = "tool_call"  // Tool call related content
	ContentTypeRegular   ContentType = "regular"    // Standard content without special handling
)

// HarmonyParseError represents errors that occur during Harmony format parsing
type HarmonyParseError struct {
	Message string
	Token   string
	Content string
}

// Error implements the error interface for HarmonyParseError
func (e *HarmonyParseError) Error() string {
	if e.Token != "" {
		return "harmony parsing error: " + e.Message + " (token: " + e.Token + ")"
	}
	return "harmony parsing error: " + e.Message
}

// Common harmony parsing errors
var (
	ErrMalformedToken     = errors.New("malformed harmony token")
	ErrMissingStartToken  = errors.New("missing start token")
	ErrMissingEndToken    = errors.New("missing end token")
	ErrInvalidRole        = errors.New("invalid role")
	ErrInvalidChannel     = errors.New("invalid channel type")
	ErrEmptyContent       = errors.New("empty message content")
	ErrIncompleteMessage  = errors.New("incomplete harmony message")
)

// Channel represents a parsed Harmony channel with its metadata
type Channel struct {
	Type        ChannelType `json:"type"`         // Channel type (analysis, final, commentary)
	Role        Role        `json:"role"`         // Message role (assistant, user, etc.)
	Content     string      `json:"content"`      // Raw content from the channel
	ContentType ContentType `json:"content_type"` // How content should be classified
	RawTokens   string      `json:"raw_tokens"`   // Original token sequence for debugging
}

// IsThinking returns true if this channel contains thinking/analysis content
func (c *Channel) IsThinking() bool {
	return c.ContentType == ContentTypeThinking || c.Type == ChannelAnalysis
}

// IsResponse returns true if this channel contains user-facing response content
func (c *Channel) IsResponse() bool {
	return c.ContentType == ContentTypeResponse || c.Type == ChannelFinal
}

// IsToolCall returns true if this channel contains tool call content
func (c *Channel) IsToolCall() bool {
	return c.ContentType == ContentTypeToolCall || c.Type == ChannelCommentary
}

// HarmonyMessage represents a complete parsed Harmony message with multiple channels
type HarmonyMessage struct {
	Channels      []Channel `json:"channels"`       // All channels found in the message
	ThinkingText  string    `json:"thinking_text"`  // Combined thinking/analysis content
	ResponseText  string    `json:"response_text"`  // Combined user-facing response content
	ToolCallText  string    `json:"tool_call_text"` // Combined tool call content
	RawContent    string    `json:"raw_content"`    // Original unparsed content
	HasHarmony    bool      `json:"has_harmony"`    // Whether Harmony tokens were detected
	ParseErrors   []error   `json:"parse_errors"`   // Any errors encountered during parsing
}

// HasThinking returns true if the message contains thinking/analysis content
func (h *HarmonyMessage) HasThinking() bool {
	return h.ThinkingText != "" || h.hasChannelType(ChannelAnalysis)
}

// HasResponse returns true if the message contains user-facing response content
func (h *HarmonyMessage) HasResponse() bool {
	return h.ResponseText != "" || h.hasChannelType(ChannelFinal)
}

// HasToolCalls returns true if the message contains tool call content
func (h *HarmonyMessage) HasToolCalls() bool {
	return h.ToolCallText != "" || h.hasChannelType(ChannelCommentary)
}

// hasChannelType checks if any channel has the specified type
func (h *HarmonyMessage) hasChannelType(channelType ChannelType) bool {
	for _, channel := range h.Channels {
		if channel.Type == channelType {
			return true
		}
	}
	return false
}

// GetChannelsByType returns all channels of the specified type
func (h *HarmonyMessage) GetChannelsByType(channelType ChannelType) []Channel {
	var channels []Channel
	for _, channel := range h.Channels {
		if channel.Type == channelType {
			channels = append(channels, channel)
		}
	}
	return channels
}

// GetChannelsByContentType returns all channels of the specified content type
func (h *HarmonyMessage) GetChannelsByContentType(contentType ContentType) []Channel {
	var channels []Channel
	for _, channel := range h.Channels {
		if channel.ContentType == contentType {
			channels = append(channels, channel)
		}
	}
	return channels
}

// TokenRecognizer contains compiled regex patterns for efficient Harmony token recognition
type TokenRecognizer struct {
	StartPattern   *regexp.Regexp // Matches <|start|>role pattern
	ChannelPattern *regexp.Regexp // Matches <|channel|>type pattern  
	MessagePattern *regexp.Regexp // Matches <|message|> token
	EndPattern     *regexp.Regexp // Matches <|end|> token
	ReturnPattern  *regexp.Regexp // Matches <|return|> token
	FullPattern    *regexp.Regexp // Matches complete Harmony message structure
}

// NewTokenRecognizer creates a new TokenRecognizer with compiled regex patterns
func NewTokenRecognizer() (*TokenRecognizer, error) {
	startPattern, err := regexp.Compile(`<\|start\|>([^<]+)`)
	if err != nil {
		return nil, err
	}

	channelPattern, err := regexp.Compile(`<\|channel\|>([^<]+)`)
	if err != nil {
		return nil, err
	}

	messagePattern, err := regexp.Compile(`<\|message\|>`)
	if err != nil {
		return nil, err
	}

	endPattern, err := regexp.Compile(`<\|end\|>`)
	if err != nil {
		return nil, err
	}

	returnPattern, err := regexp.Compile(`<\|return\|>`)
	if err != nil {
		return nil, err
	}

	// Pattern to match complete Harmony message structure with multiline support
	fullPattern, err := regexp.Compile(`(?s)<\|start\|>([^<]+)(?:<\|channel\|>([^<]+))?<\|message\|>(.*?)(?:<\|end\|>|<\|return\|>)`)
	if err != nil {
		return nil, err
	}

	return &TokenRecognizer{
		StartPattern:   startPattern,
		ChannelPattern: channelPattern,
		MessagePattern: messagePattern,
		EndPattern:     endPattern,
		ReturnPattern:  returnPattern,
		FullPattern:    fullPattern,
	}, nil
}

// HasHarmonyTokens checks if the content contains any Harmony tokens
func (tr *TokenRecognizer) HasHarmonyTokens(content string) bool {
	return tr.StartPattern.MatchString(content) ||
		tr.ChannelPattern.MatchString(content) ||
		tr.MessagePattern.MatchString(content) ||
		tr.EndPattern.MatchString(content)
}

// ClassifyChannelType determines the content type based on channel type
func ClassifyChannelType(channelType ChannelType) ContentType {
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

// ParseRole validates and normalizes role strings
func ParseRole(roleStr string) (Role, error) {
	role := Role(roleStr)
	switch role {
	case RoleAssistant, RoleUser, RoleSystem, RoleDeveloper, RoleTool:
		return role, nil
	default:
		return RoleAssistant, &HarmonyParseError{
			Message: "invalid role, defaulting to assistant",
			Token:   roleStr,
		}
	}
}

// ParseChannelType validates and normalizes channel type strings
func ParseChannelType(channelStr string) ChannelType {
	channelType := ChannelType(channelStr)
	switch channelType {
	case ChannelAnalysis, ChannelFinal, ChannelCommentary:
		return channelType
	default:
		return ChannelUnknown
	}
}

// ValidateRole checks if a role string is valid
func ValidateRole(role string) bool {
	switch Role(role) {
	case RoleAssistant, RoleUser, RoleSystem, RoleDeveloper, RoleTool:
		return true
	default:
		return false
	}
}

// ValidateChannelType checks if a channel type string is valid
func ValidateChannelType(channelType string) bool {
	switch ChannelType(channelType) {
	case ChannelAnalysis, ChannelFinal, ChannelCommentary:
		return true
	default:
		return false
	}
}

// ============================================================================
// STREAM B: TOKEN RECOGNITION ENGINE
// ============================================================================

// defaultTokenRecognizer is a package-level instance for performance
var defaultTokenRecognizer *TokenRecognizer

// init initializes the default token recognizer
func init() {
	var err error
	defaultTokenRecognizer, err = NewTokenRecognizer()
	if err != nil {
		// This should never happen with static patterns, but handle gracefully
		panic("failed to initialize harmony token recognizer: " + err.Error())
	}
}

// IsHarmonyFormat performs quick detection of Harmony tokens in content
// This is optimized for performance and provides a fast way to determine
// if content contains Harmony formatting before full parsing
func IsHarmonyFormat(content string) bool {
	return defaultTokenRecognizer.HasHarmonyTokens(content)
}

// ExtractChannels parses content and extracts all Harmony channels
// Returns a slice of Channel structs with parsed metadata and content
// Handles both complete and streaming responses with graceful error handling
func ExtractChannels(content string) []Channel {
	if !IsHarmonyFormat(content) {
		return []Channel{}
	}

	var channels []Channel
	
	// Find all potential Harmony message blocks using the full pattern
	matches := defaultTokenRecognizer.FullPattern.FindAllStringSubmatch(content, -1)
	
	for _, match := range matches {
		if len(match) < 4 {
			continue // Malformed match, skip
		}
		
		roleStr := match[1]
		channelStr := match[2] 
		messageContent := match[3]
		
		// Parse and validate role
		role, err := ParseRole(roleStr)
		if err != nil {
			// Log warning but continue with default role
			role = RoleAssistant
		}
		
		// Parse channel type - empty channel defaults to final
		var channelType ChannelType
		if channelStr == "" {
			channelType = ChannelFinal
		} else {
			channelType = ParseChannelType(channelStr)
		}
		
		// Skip empty content
		if messageContent == "" {
			continue
		}
		
		// Classify content type based on channel
		contentType := ClassifyChannelType(channelType)
		
		// Create channel with extracted data
		channel := Channel{
			Type:        channelType,
			Role:        role,
			Content:     messageContent,
			ContentType: contentType,
			RawTokens:   match[0], // Store original token sequence
		}
		
		channels = append(channels, channel)
	}
	
	// Handle partial/streaming content by looking for incomplete tokens
	channels = append(channels, extractPartialChannels(content)...)
	
	return channels
}

// extractPartialChannels handles incomplete Harmony tokens for streaming responses
// This allows parsing of partial content where messages may be cut off mid-stream
func extractPartialChannels(content string) []Channel {
	var partialChannels []Channel
	
	// Look for start tokens that don't have complete end tokens
	startMatches := defaultTokenRecognizer.StartPattern.FindAllStringSubmatchIndex(content, -1)
	
	for _, startMatch := range startMatches {
		startPos := startMatch[0]
		
		// Extract everything from start position to end of content
		remaining := content[startPos:]
		
		// Check if this looks like an incomplete Harmony message
		if isIncompleteHarmonyMessage(remaining) {
			channel := parseIncompleteChannel(remaining)
			if channel != nil {
				partialChannels = append(partialChannels, *channel)
			}
		}
	}
	
	return partialChannels
}

// isIncompleteHarmonyMessage checks if content appears to be a partial Harmony message
func isIncompleteHarmonyMessage(content string) bool {
	// Must have start token but missing end token
	hasStart := defaultTokenRecognizer.StartPattern.MatchString(content)
	hasEnd := defaultTokenRecognizer.EndPattern.MatchString(content)
	
	return hasStart && !hasEnd
}

// parseIncompleteChannel attempts to parse a partial Harmony channel
func parseIncompleteChannel(content string) *Channel {
	// Try to extract what we can from the incomplete content
	roleMatch := defaultTokenRecognizer.StartPattern.FindStringSubmatch(content)
	if len(roleMatch) < 2 {
		return nil
	}
	
	role, err := ParseRole(roleMatch[1])
	if err != nil {
		role = RoleAssistant
	}
	
	// Look for channel specification
	var channelType ChannelType = ChannelFinal // Default
	channelMatch := defaultTokenRecognizer.ChannelPattern.FindStringSubmatch(content)
	if len(channelMatch) >= 2 {
		channelType = ParseChannelType(channelMatch[1])
	}
	
	// Extract content after message token
	messageTokenIdx := defaultTokenRecognizer.MessagePattern.FindStringIndex(content)
	var messageContent string
	if messageTokenIdx != nil {
		messageContent = content[messageTokenIdx[1]:]
	}
	
	// Don't create channel for completely empty content
	if messageContent == "" {
		return nil
	}
	
	return &Channel{
		Type:        channelType,
		Role:        role,
		Content:     messageContent,
		ContentType: ClassifyChannelType(channelType),
		RawTokens:   content,
	}
}

// FindHarmonyTokens returns all harmony token positions in the content
// Useful for detailed analysis and debugging of token recognition
func FindHarmonyTokens(content string) map[string][]int {
	tokens := make(map[string][]int)
	
	// Find all start tokens
	startIndices := defaultTokenRecognizer.StartPattern.FindAllStringIndex(content, -1)
	if len(startIndices) > 0 {
		tokens["start"] = make([]int, len(startIndices))
		for i, match := range startIndices {
			tokens["start"][i] = match[0]
		}
	}
	
	// Find all channel tokens
	channelIndices := defaultTokenRecognizer.ChannelPattern.FindAllStringIndex(content, -1)
	if len(channelIndices) > 0 {
		tokens["channel"] = make([]int, len(channelIndices))
		for i, match := range channelIndices {
			tokens["channel"][i] = match[0]
		}
	}
	
	// Find all message tokens
	messageIndices := defaultTokenRecognizer.MessagePattern.FindAllStringIndex(content, -1)
	if len(messageIndices) > 0 {
		tokens["message"] = make([]int, len(messageIndices))
		for i, match := range messageIndices {
			tokens["message"][i] = match[0]
		}
	}
	
	// Find all end tokens
	endIndices := defaultTokenRecognizer.EndPattern.FindAllStringIndex(content, -1)
	if len(endIndices) > 0 {
		tokens["end"] = make([]int, len(endIndices))
		for i, match := range endIndices {
			tokens["end"][i] = match[0]
		}
	}
	
	return tokens
}

// ValidateHarmonyStructure checks if Harmony tokens are properly structured
// Returns validation errors for malformed token sequences
func ValidateHarmonyStructure(content string) []error {
	var errors []error
	
	if !IsHarmonyFormat(content) {
		return errors // No harmony content to validate
	}
	
	tokens := FindHarmonyTokens(content)
	
	// Check for basic structure issues
	startCount := len(tokens["start"])
	endCount := len(tokens["end"])
	
	if startCount > endCount {
		errors = append(errors, &HarmonyParseError{
			Message: "more start tokens than end tokens",
			Content: content,
		})
	}
	
	if startCount == 0 && len(tokens["channel"]) > 0 {
		errors = append(errors, &HarmonyParseError{
			Message: "channel tokens without start token",
			Content: content,
		})
	}
	
	if len(tokens["message"]) == 0 && (startCount > 0 || len(tokens["channel"]) > 0) {
		errors = append(errors, &HarmonyParseError{
			Message: "harmony tokens without message content",
			Content: content,
		})
	}
	
	return errors
}

// CleanHarmonyContent removes or fixes malformed Harmony tokens
// Returns cleaned content that can be safely processed
func CleanHarmonyContent(content string) string {
	if !IsHarmonyFormat(content) {
		return content
	}
	
	// For now, just return the content as-is
	// More sophisticated cleaning logic can be added later
	return content
}

// GetHarmonyTokenStats returns statistics about Harmony tokens in content
// Useful for monitoring and debugging token recognition performance
func GetHarmonyTokenStats(content string) map[string]int {
	stats := make(map[string]int)
	
	tokens := FindHarmonyTokens(content)
	
	stats["start_tokens"] = len(tokens["start"])
	stats["channel_tokens"] = len(tokens["channel"])
	stats["message_tokens"] = len(tokens["message"])
	stats["end_tokens"] = len(tokens["end"])
	stats["total_tokens"] = stats["start_tokens"] + stats["channel_tokens"] + 
		stats["message_tokens"] + stats["end_tokens"]
	
	// Calculate completeness ratio
	if stats["start_tokens"] > 0 {
		stats["completeness_ratio"] = stats["end_tokens"] * 100 / stats["start_tokens"]
	} else {
		stats["completeness_ratio"] = 100
	}
	
	return stats
}