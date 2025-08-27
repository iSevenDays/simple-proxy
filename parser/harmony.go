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

	// Pattern to match complete Harmony message structure
	fullPattern, err := regexp.Compile(`<\|start\|>([^<]+)(?:<\|channel\|>([^<]+))?<\|message\|>(.*?)(?:<\|end\|>|<\|return\|>|$)`)
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