// Package parser provides OpenAI Harmony message format parsing capabilities.
// It recognizes and parses Harmony tokens like <|start|>, <|channel|>, <|message|>, <|end|>
// to properly classify thinking content, user responses, and tool calls.
package parser

import (
	"fmt"
	"regexp"
	"strings"
)

// Role represents the different roles that can appear in Harmony messages
type Role int

const (
	RoleAssistant Role = iota
	RoleUser
	RoleSystem
	RoleDeveloper
	RoleTool
)

// String returns the string representation of the Role
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

// ParseRole converts a string to Role enum with fallback to assistant
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

// ChannelType represents the different channel types in Harmony format
type ChannelType int

const (
	ChannelAnalysis ChannelType = iota
	ChannelFinal
	ChannelCommentary
	ChannelUnknown
)

// String returns the string representation of the ChannelType
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

// ParseChannelType converts a string to ChannelType enum with fallback to unknown
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

// ContentType represents the classification of content for Claude Code UI
type ContentType int

const (
	ContentTypeThinking ContentType = iota
	ContentTypeResponse
	ContentTypeToolCall
	ContentTypeRegular
)

// String returns the string representation of the ContentType
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

// Channel represents a single channel in a Harmony message
type Channel struct {
	Role        Role        `json:"role"`
	ChannelType ChannelType `json:"channel_type"`
	ContentType ContentType `json:"content_type"`
	Content     string      `json:"content"`
	RawChannel  string      `json:"raw_channel,omitempty"`
}

// IsThinking returns true if this channel contains thinking content
func (c *Channel) IsThinking() bool {
	return c.ContentType == ContentTypeThinking
}

// IsResponse returns true if this channel contains response content
func (c *Channel) IsResponse() bool {
	return c.ContentType == ContentTypeResponse
}

// IsToolCall returns true if this channel contains tool call content
func (c *Channel) IsToolCall() bool {
	return c.ContentType == ContentTypeToolCall
}

// HarmonyMessage represents a complete parsed Harmony message
type HarmonyMessage struct {
	Channels     []Channel `json:"channels"`
	RawContent   string    `json:"raw_content"`
	HasHarmony   bool      `json:"has_harmony"`
	ParseErrors  []error   `json:"parse_errors,omitempty"`
	ThinkingText string    `json:"thinking_text,omitempty"`
	ResponseText string    `json:"response_text,omitempty"`
	ToolCallText string    `json:"tool_call_text,omitempty"`
}

// GetChannelsByType returns all channels of the specified type
func (h *HarmonyMessage) GetChannelsByType(channelType ChannelType) []Channel {
	var channels []Channel
	for _, channel := range h.Channels {
		if channel.ChannelType == channelType {
			channels = append(channels, channel)
		}
	}
	return channels
}

// GetThinkingChannels returns all channels containing thinking content
func (h *HarmonyMessage) GetThinkingChannels() []Channel {
	var channels []Channel
	for _, channel := range h.Channels {
		if channel.IsThinking() {
			channels = append(channels, channel)
		}
	}
	return channels
}

// GetResponseChannels returns all channels containing response content
func (h *HarmonyMessage) GetResponseChannels() []Channel {
	var channels []Channel
	for _, channel := range h.Channels {
		if channel.IsResponse() {
			channels = append(channels, channel)
		}
	}
	return channels
}

// HarmonyParseError represents errors that occur during Harmony parsing
type HarmonyParseError struct {
	Message  string
	Position int
	Context  string
}

// Error implements the error interface
func (e *HarmonyParseError) Error() string {
	if e.Position >= 0 && e.Context != "" {
		return fmt.Sprintf("harmony parse error at position %d: %s (context: %s)", e.Position, e.Message, e.Context)
	} else if e.Position >= 0 {
		return fmt.Sprintf("harmony parse error at position %d: %s", e.Position, e.Message)
	}
	return fmt.Sprintf("harmony parse error: %s", e.Message)
}

// TokenRecognizer handles Harmony token pattern recognition
type TokenRecognizer struct {
	startPattern   *regexp.Regexp
	endPattern     *regexp.Regexp
	channelPattern *regexp.Regexp
	messagePattern *regexp.Regexp
	fullPattern    *regexp.Regexp
}

// NewTokenRecognizer creates a new TokenRecognizer with compiled patterns
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

	// Full pattern for complete token sequences
	fullPattern, err := regexp.Compile(`(?s)<\|start\|>(\w+)(?:<\|channel\|>(\w+))?<\|message\|>(.*?)<\|end\|>`)
	if err != nil {
		return nil, fmt.Errorf("failed to compile full pattern: %w", err)
	}

	return &TokenRecognizer{
		startPattern:   startPattern,
		endPattern:     endPattern,
		channelPattern: channelPattern,
		messagePattern: messagePattern,
		fullPattern:    fullPattern,
	}, nil
}

// HasHarmonyTokens returns true if the content contains any Harmony tokens
func (tr *TokenRecognizer) HasHarmonyTokens(content string) bool {
	return tr.startPattern.MatchString(content) || tr.endPattern.MatchString(content)
}

// ExtractTokens extracts all complete Harmony token sequences from content
func (tr *TokenRecognizer) ExtractTokens(content string) [][]string {
	return tr.fullPattern.FindAllStringSubmatch(content, -1)
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

// IsHarmonyFormat returns true if the content contains Harmony format tokens
func IsHarmonyFormat(content string) bool {
	return defaultTokenRecognizer.HasHarmonyTokens(content)
}

// ExtractChannels extracts and parses all channels from Harmony format content
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

// DetermineContentType maps channel types to content types for Claude Code UI
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

// ParseHarmonyMessage is the main API function for parsing Harmony format messages
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

// FindHarmonyTokens returns the positions and types of Harmony tokens in content
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

// TokenPosition represents the position and type of a Harmony token
type TokenPosition struct {
	Type     string `json:"type"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
	Value    string `json:"value,omitempty"`
	Position int    `json:"position"`
}

// ValidateHarmonyStructure validates that Harmony tokens are properly structured
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

// GetHarmonyTokenStats returns statistics about Harmony tokens in content
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

// TokenStats provides statistics about Harmony tokens
type TokenStats struct {
	TotalTokens int            `json:"total_tokens"`
	TokenCounts map[string]int `json:"token_counts"`
}