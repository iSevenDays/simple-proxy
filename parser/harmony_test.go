package parser

import (
	"testing"
)

// Test the main ParseHarmonyMessage function
func TestParseHarmonyMessage(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantHarmony  bool
		wantChannels int
		wantThinking string
		wantResponse string
		wantToolCall string
	}{
		{
			name:         "empty input",
			input:        "",
			wantHarmony:  false,
			wantChannels: 0,
			wantThinking: "",
			wantResponse: "",
			wantToolCall: "",
		},
		{
			name:         "regular text without harmony",
			input:        "This is just regular text without any Harmony tokens.",
			wantHarmony:  false,
			wantChannels: 0,
			wantThinking: "",
			wantResponse: "",
			wantToolCall: "",
		},
		{
			name: "analysis channel only",
			input: `<|start|>assistant<|channel|>analysis<|message|>I need to analyze the user's request for implementing a search feature. Let me break down the requirements:
1. Full-text search across documents
2. Filter by date range
3. Sort by relevance
4. Return structured results

The user wants this integrated with the existing API, so I'll need to extend the current endpoints.<|end|>`,
			wantHarmony:  true,
			wantChannels: 1,
			wantThinking: "I need to analyze the user's request for implementing a search feature. Let me break down the requirements:\n1. Full-text search across documents\n2. Filter by date range\n3. Sort by relevance\n4. Return structured results\n\nThe user wants this integrated with the existing API, so I'll need to extend the current endpoints.",
			wantResponse: "",
			wantToolCall: "",
		},
		{
			name: "final channel only",
			input: `<|start|>assistant<|channel|>final<|message|>I'll implement a search feature with the following components:

1. **Search Endpoint**: ` + "`POST /api/search`" + `
2. **Query Parameters**: text, dateRange, sortBy
3. **Response Format**: JSON with results array

Here's the implementation plan:<|end|>`,
			wantHarmony:  true,
			wantChannels: 1,
			wantThinking: "",
			wantResponse: "I'll implement a search feature with the following components:\n\n1. **Search Endpoint**: `POST /api/search`\n2. **Query Parameters**: text, dateRange, sortBy\n3. **Response Format**: JSON with results array\n\nHere's the implementation plan:",
			wantToolCall: "",
		},
		{
			name: "commentary channel only",
			input: `<|start|>assistant<|channel|>commentary<|message|>I'll use the file search tool to find existing search implementations in the codebase.<|end|>`,
			wantHarmony:  true,
			wantChannels: 1,
			wantThinking: "",
			wantResponse: "",
			wantToolCall: "I'll use the file search tool to find existing search implementations in the codebase.",
		},
		{
			name: "mixed channels",
			input: `<|start|>assistant<|channel|>analysis<|message|>The user wants to refactor the authentication system. I need to consider:
- Current JWT implementation
- Session management
- Security implications
- Backward compatibility<|end|>
<|start|>assistant<|channel|>final<|message|>I'll help you refactor the authentication system. Here's my recommended approach:

## Current Assessment
Your JWT implementation is solid, but we can improve session management.

## Proposed Changes
1. Extract auth logic into dedicated service
2. Implement refresh token rotation
3. Add session cleanup job<|end|>`,
			wantHarmony:  true,
			wantChannels: 2,
			wantThinking: "The user wants to refactor the authentication system. I need to consider:\n- Current JWT implementation\n- Session management\n- Security implications\n- Backward compatibility",
			wantResponse: "I'll help you refactor the authentication system. Here's my recommended approach:\n\n## Current Assessment\nYour JWT implementation is solid, but we can improve session management.\n\n## Proposed Changes\n1. Extract auth logic into dedicated service\n2. Implement refresh token rotation\n3. Add session cleanup job",
			wantToolCall: "",
		},
		{
			name: "all channel types",
			input: `<|start|>assistant<|channel|>analysis<|message|>Analyzing the request to implement authentication...<|end|>
<|start|>assistant<|channel|>commentary<|message|>Using search tool to find existing auth patterns<|end|>
<|start|>assistant<|channel|>final<|message|>Here's the authentication implementation:<|end|>`,
			wantHarmony:  true,
			wantChannels: 3,
			wantThinking: "Analyzing the request to implement authentication...",
			wantResponse: "Here's the authentication implementation:",
			wantToolCall: "Using search tool to find existing auth patterns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, err := ParseHarmonyMessage(tt.input)
			if err != nil {
				t.Errorf("ParseHarmonyMessage() error = %v", err)
				return
			}

			if message.HasHarmony != tt.wantHarmony {
				t.Errorf("ParseHarmonyMessage() HasHarmony = %v, want %v", message.HasHarmony, tt.wantHarmony)
			}

			if len(message.Channels) != tt.wantChannels {
				t.Errorf("ParseHarmonyMessage() channels = %d, want %d", len(message.Channels), tt.wantChannels)
			}

			if message.ThinkingText != tt.wantThinking {
				t.Errorf("ParseHarmonyMessage() ThinkingText = %q, want %q", message.ThinkingText, tt.wantThinking)
			}

			if message.ResponseText != tt.wantResponse {
				t.Errorf("ParseHarmonyMessage() ResponseText = %q, want %q", message.ResponseText, tt.wantResponse)
			}

			if message.ToolCallText != tt.wantToolCall {
				t.Errorf("ParseHarmonyMessage() ToolCallText = %q, want %q", message.ToolCallText, tt.wantToolCall)
			}

			if message.RawContent != tt.input {
				t.Errorf("ParseHarmonyMessage() RawContent doesn't match input")
			}
		})
	}
}

// Test channel classification logic
func TestDetermineContentType(t *testing.T) {
	tests := []struct {
		channel ChannelType
		want    ContentType
	}{
		{ChannelAnalysis, ContentTypeThinking},
		{ChannelFinal, ContentTypeResponse},
		{ChannelCommentary, ContentTypeToolCall},
		{ChannelUnknown, ContentTypeRegular},
	}

	for _, tt := range tests {
		t.Run(tt.channel.String(), func(t *testing.T) {
			got := DetermineContentType(tt.channel)
			if got != tt.want {
				t.Errorf("DetermineContentType(%v) = %v, want %v", tt.channel, got, tt.want)
			}
		})
	}
}

// Test IsHarmonyFormat function
func TestIsHarmonyFormat(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "regular text",
			input: "This is just regular text",
			want:  false,
		},
		{
			name:  "has start token",
			input: "Some text <|start|>assistant and more text",
			want:  true,
		},
		{
			name:  "has end token",
			input: "Some text <|end|> and more text",
			want:  true,
		},
		{
			name:  "complete harmony message",
			input: "<|start|>assistant<|channel|>analysis<|message|>thinking<|end|>",
			want:  true,
		},
		{
			name:  "malformed but detectable",
			input: "<|start|>assistant thinking without proper end",
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsHarmonyFormat(tt.input)
			if got != tt.want {
				t.Errorf("IsHarmonyFormat(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// Test ExtractChannels function
func TestExtractChannels(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "no harmony tokens",
			input: "regular text",
			want:  0,
		},
		{
			name:  "single complete channel",
			input: "<|start|>assistant<|channel|>analysis<|message|>content<|end|>",
			want:  1,
		},
		{
			name: "multiple channels",
			input: `<|start|>assistant<|channel|>analysis<|message|>thinking<|end|>
<|start|>assistant<|channel|>final<|message|>response<|end|>`,
			want: 2,
		},
		{
			name:  "malformed tokens",
			input: "<|start|>assistant<|channel|>analysis<|message|>incomplete",
			want:  0, // Should not extract incomplete tokens
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channels := ExtractChannels(tt.input)
			if len(channels) != tt.want {
				t.Errorf("ExtractChannels(%q) returned %d channels, want %d", tt.input, len(channels), tt.want)
			}
		})
	}
}

// Test channel methods
func TestChannelMethods(t *testing.T) {
	thinkingChannel := Channel{
		ContentType: ContentTypeThinking,
	}
	responseChannel := Channel{
		ContentType: ContentTypeResponse,
	}
	toolChannel := Channel{
		ContentType: ContentTypeToolCall,
	}

	// Test IsThinking
	if !thinkingChannel.IsThinking() {
		t.Error("thinking channel should return true for IsThinking()")
	}
	if responseChannel.IsThinking() {
		t.Error("response channel should return false for IsThinking()")
	}

	// Test IsResponse
	if !responseChannel.IsResponse() {
		t.Error("response channel should return true for IsResponse()")
	}
	if thinkingChannel.IsResponse() {
		t.Error("thinking channel should return false for IsResponse()")
	}

	// Test IsToolCall
	if !toolChannel.IsToolCall() {
		t.Error("tool channel should return true for IsToolCall()")
	}
	if responseChannel.IsToolCall() {
		t.Error("response channel should return false for IsToolCall()")
	}
}

// Test HarmonyMessage methods
func TestHarmonyMessageMethods(t *testing.T) {
	message := &HarmonyMessage{
		Channels: []Channel{
			{ChannelType: ChannelAnalysis, ContentType: ContentTypeThinking, Content: "thinking1"},
			{ChannelType: ChannelFinal, ContentType: ContentTypeResponse, Content: "response1"},
			{ChannelType: ChannelAnalysis, ContentType: ContentTypeThinking, Content: "thinking2"},
			{ChannelType: ChannelCommentary, ContentType: ContentTypeToolCall, Content: "tool1"},
		},
	}

	// Test GetChannelsByType
	analysisChannels := message.GetChannelsByType(ChannelAnalysis)
	if len(analysisChannels) != 2 {
		t.Errorf("expected 2 analysis channels, got %d", len(analysisChannels))
	}

	// Test GetThinkingChannels
	thinkingChannels := message.GetThinkingChannels()
	if len(thinkingChannels) != 2 {
		t.Errorf("expected 2 thinking channels, got %d", len(thinkingChannels))
	}

	// Test GetResponseChannels
	responseChannels := message.GetResponseChannels()
	if len(responseChannels) != 1 {
		t.Errorf("expected 1 response channel, got %d", len(responseChannels))
	}
}

// Test role parsing
func TestParseRole(t *testing.T) {
	tests := []struct {
		input string
		want  Role
	}{
		{"assistant", RoleAssistant},
		{"user", RoleUser},
		{"system", RoleSystem},
		{"developer", RoleDeveloper},
		{"tool", RoleTool},
		{"unknown", RoleAssistant}, // fallback
		{"", RoleAssistant},        // fallback
		{"ASSISTANT", RoleAssistant}, // case insensitive
		{" user ", RoleUser},       // whitespace handling
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseRole(tt.input)
			if got != tt.want {
				t.Errorf("ParseRole(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// Test channel type parsing
func TestParseChannelType(t *testing.T) {
	tests := []struct {
		input string
		want  ChannelType
	}{
		{"analysis", ChannelAnalysis},
		{"final", ChannelFinal},
		{"commentary", ChannelCommentary},
		{"unknown", ChannelUnknown},
		{"", ChannelUnknown},           // fallback
		{"ANALYSIS", ChannelAnalysis}, // case insensitive
		{" final ", ChannelFinal},     // whitespace handling
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseChannelType(tt.input)
			if got != tt.want {
				t.Errorf("ParseChannelType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// Test real-world example from PRD
func TestRealWorldExample(t *testing.T) {
	input := `<|channel|>analysis<|message|>The user is asking: "to compare changes in
  this branch to main, do the code review, I will create merge request
  then, first I need your review". The user wants code review between
  current branch (feature/auth-improvements) and main branch. The
  repository is a Node.js project, but note from project-specific
  CLAUDE.md that we cannot run npm commands (they will fail) and we should
  rely on static analysis. There's a code review agent optimization: max 5
  tool calls per review, 2-minute analysis target, prioritizing security,
  business logic, critical issues, using git diff to focus on changed files
   only. Thus we need to run git diff between current branch and main, get
  list of changed files and diffs. Then we need to review changes focusing
  on security, business logic, critical issues, static analysis (e.g.,
  scanning for password, secret, token, try/catch, API keys). We also
  need to ensure code quality (e.g., proper error handling, async/await
  patterns, potential memory leaks). Since the branch is named
  feature/auth-improvements, it likely contains authentication changes.
  Let's explore repository. First, list files. Use LS.<|end|>`

	message, err := ParseHarmonyMessage(input)
	if err != nil {
		t.Fatalf("ParseHarmonyMessage() error = %v", err)
	}

	// This input doesn't have proper start tokens, so it should be detected but not fully parsed
	if !message.HasHarmony {
		t.Error("expected HasHarmony to be true for input containing Harmony tokens")
	}

	// Since it's malformed (no start token), channels might be empty
	// But the content should be preserved in RawContent
	if message.RawContent != input {
		t.Error("RawContent should preserve original input")
	}
}

// Test malformed input handling
func TestMalformedInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "missing end token",
			input: "<|start|>assistant<|channel|>analysis<|message|>content without end",
		},
		{
			name:  "missing start token",
			input: "<|channel|>analysis<|message|>content without start<|end|>",
		},
		{
			name:  "missing channel",
			input: "<|start|>assistant<|message|>content without channel<|end|>",
		},
		{
			name:  "empty message",
			input: "<|start|>assistant<|channel|>analysis<|message|><|end|>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, err := ParseHarmonyMessage(tt.input)
			if err != nil {
				t.Errorf("ParseHarmonyMessage() should not error on malformed input, got: %v", err)
			}
			if message == nil {
				t.Error("ParseHarmonyMessage() should return a message even for malformed input")
			}
			// Should preserve raw content even if parsing fails
			if message.RawContent != tt.input {
				t.Error("RawContent should be preserved even for malformed input")
			}
		})
	}
}

// Test multiple channels of same type
func TestMultipleChannelsSameType(t *testing.T) {
	input := `<|start|>assistant<|channel|>analysis<|message|>First analysis<|end|>
<|start|>assistant<|channel|>analysis<|message|>Second analysis<|end|>
<|start|>assistant<|channel|>final<|message|>Final response<|end|>`

	message, err := ParseHarmonyMessage(input)
	if err != nil {
		t.Fatalf("ParseHarmonyMessage() error = %v", err)
	}

	if len(message.Channels) != 3 {
		t.Errorf("expected 3 channels, got %d", len(message.Channels))
	}

	expectedThinking := "First analysis\nSecond analysis"
	if message.ThinkingText != expectedThinking {
		t.Errorf("ThinkingText = %q, want %q", message.ThinkingText, expectedThinking)
	}

	expectedResponse := "Final response"
	if message.ResponseText != expectedResponse {
		t.Errorf("ResponseText = %q, want %q", message.ResponseText, expectedResponse)
	}
}

// Benchmark the main parsing function
func BenchmarkParseHarmonyMessage(b *testing.B) {
	input := `<|start|>assistant<|channel|>analysis<|message|>This is a thinking message that analyzes the user request and considers various approaches before providing a response.<|end|>
<|start|>assistant<|channel|>final<|message|>This is the final response that will be shown to the user as the main answer.<|end|>`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseHarmonyMessage(input)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Test string representations
func TestStringRepresentations(t *testing.T) {
	// Test Role.String()
	if RoleAssistant.String() != "assistant" {
		t.Errorf("RoleAssistant.String() = %q, want %q", RoleAssistant.String(), "assistant")
	}

	// Test ChannelType.String()
	if ChannelAnalysis.String() != "analysis" {
		t.Errorf("ChannelAnalysis.String() = %q, want %q", ChannelAnalysis.String(), "analysis")
	}

	// Test ContentType.String()
	if ContentTypeThinking.String() != "thinking" {
		t.Errorf("ContentTypeThinking.String() = %q, want %q", ContentTypeThinking.String(), "thinking")
	}
}

// Test error formatting
func TestHarmonyParseError(t *testing.T) {
	err := &HarmonyParseError{
		Message:  "test error",
		Position: 42,
		Context:  "test context",
	}

	expected := "harmony parse error at position 42: test error (context: test context)"
	if err.Error() != expected {
		t.Errorf("HarmonyParseError.Error() = %q, want %q", err.Error(), expected)
	}

	// Test error without position
	err2 := &HarmonyParseError{
		Message:  "test error without position",
		Position: -1,
	}

	expected2 := "harmony parse error: test error without position"
	if err2.Error() != expected2 {
		t.Errorf("HarmonyParseError.Error() = %q, want %q", err2.Error(), expected2)
	}
}