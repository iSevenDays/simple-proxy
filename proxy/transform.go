package proxy

import (
	"claude-proxy/config"
	"claude-proxy/logger"
	"claude-proxy/types"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// NOTE: isSmallModel and shouldLogForModel functions removed
// This logic is now handled by the logger.ConfigAdapter

// getEmptyToolResultMessage returns an appropriate message for empty tool results
func getEmptyToolResultMessage(contentMap map[string]interface{}) string {
	// Try to determine tool type from error or other context
	if errorMsg, ok := contentMap["error"].(string); ok && errorMsg != "" {
		return fmt.Sprintf("Tool execution error: %s", errorMsg)
	}
	
	// Check if we can infer tool type from tool_use_id pattern
	if toolUseID, ok := contentMap["tool_use_id"].(string); ok {
		// This is a simple heuristic - could be enhanced with actual tool tracking
		toolType := inferToolTypeFromID(toolUseID)
		return getToolSpecificEmptyMessage(toolType)
	}
	
	// Generic fallback
	return "Tool execution returned no results"
}

// inferToolTypeFromID attempts to determine tool type from tool_use_id
func inferToolTypeFromID(toolUseID string) string {
	// This is a simple heuristic - in practice, we'd want to track
	// the actual tool calls to know which tool this result corresponds to
	return "unknown"
}

// getToolSpecificEmptyMessage returns tool-specific messages for empty results
func getToolSpecificEmptyMessage(toolType string) string {
	switch toolType {
	case "WebSearch":
		return "No search results found for the given query"
	case "Read":
		return "File not found or empty"
	case "Bash":
		return "Command executed with no output"
	case "Grep":
		return "No matches found"
	case "LS":
		return "Directory is empty or not found"
	case "Glob":
		return "No files matching pattern found"
	default:
		return "Tool execution returned no results"
	}
}

// FindValidToolSchema attempts to find a valid schema for a corrupted tool
// by looking for a similar tool name in the available tools list, or providing a fallback schema
func FindValidToolSchema(corruptedTool types.Tool, availableTools []types.Tool) *types.Tool {
	// First, check common mapping patterns for known corrupted tools
	nameMapping := map[string]string{
		"web_search": "WebSearch",
		"websearch":  "WebSearch", 
		"read_file":  "Read",
		"write_file": "Write",
		"bash_command": "Bash",
		"grep_search": "Grep",
	}
	
	if mappedName, exists := nameMapping[strings.ToLower(corruptedTool.Name)]; exists {
		for _, validTool := range availableTools {
			if validTool.Name == mappedName && isValidToolSchema(validTool) {
				return &validTool
			}
		}
	}
	
	// Direct name match (case-insensitive) - only if it has a valid schema
	for _, validTool := range availableTools {
		if strings.EqualFold(corruptedTool.Name, validTool.Name) && isValidToolSchema(validTool) {
			return &validTool
		}
	}
	
	// If no valid tool found in availableTools, provide a fallback schema for known tools
	return types.GetFallbackToolSchema(corruptedTool.Name)
}


// isValidToolSchema checks if a tool has a valid schema
func isValidToolSchema(tool types.Tool) bool {
	return tool.InputSchema.Type != "" && tool.InputSchema.Properties != nil
}

// RestoreCorruptedToolSchema attempts to fix corrupted tool schemas
func RestoreCorruptedToolSchema(tool *types.Tool, availableTools []types.Tool, logger logger.Logger) bool {
	// Check if schema is actually corrupted
	if tool.InputSchema.Type != "" && tool.InputSchema.Properties != nil {
		return false // Not corrupted
	}
	
	logger.Debug("🔍 Attempting to restore corrupted schema for tool: %s", tool.Name)
	
	// Try to find a valid schema for this tool (now includes fallback schemas)
	validTool := FindValidToolSchema(*tool, availableTools)
	if validTool != nil {
		originalName := tool.Name
		*tool = *validTool // Copy the entire valid tool definition
		logger.Info("✅ Schema restored: %s → %s (with fallback schema)", 
			originalName, tool.Name)
		return true
	}
	
	logger.Warn("⚠️ Could not restore schema for tool: %s (no valid schema found)", 
		tool.Name)
	return false
}

// TransformAnthropicToOpenAI converts Anthropic request format to OpenAI format
func TransformAnthropicToOpenAI(ctx context.Context, req types.AnthropicRequest, cfg *config.Config) (types.OpenAIRequest, error) {
	// Get logger from context
	loggerConfig := logger.NewConfigAdapter(cfg)
	loggerInstance := logger.FromContext(ctx, loggerConfig)
	
	openaiReq := types.OpenAIRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Stream:      req.Stream,
		CachePrompt: true,
		Messages:    []types.OpenAIMessage{},
	}

	// Handle system messages - convert from Anthropic array to OpenAI string
	if len(req.System) > 0 {
		var systemParts []string
		for _, sys := range req.System {
			if sys.Type == "text" && sys.Text != "" {
				systemParts = append(systemParts, sys.Text)
			}
		}

		if len(systemParts) > 0 {
			systemContent := strings.Join(systemParts, "\n")
			
			// Apply system message overrides if any are configured
			if len(cfg.SystemMessageOverrides.RemovePatterns) > 0 || 
			   len(cfg.SystemMessageOverrides.Replacements) > 0 ||
			   cfg.SystemMessageOverrides.Prepend != "" ||
			   cfg.SystemMessageOverrides.Append != "" {
				originalContent := systemContent
				systemContent = config.ApplySystemMessageOverrides(systemContent, cfg.SystemMessageOverrides)
				
				logger.LogSystemOverride(ctx, loggerInstance, len(originalContent), len(systemContent))
			}
			
			// Print system message if enabled
			if cfg.PrintSystemMessage {
				logger.LogSystemMessage(ctx, loggerInstance, len(systemContent), systemContent)
			}
			
			openaiReq.Messages = append(openaiReq.Messages, types.OpenAIMessage{
				Role:    "system",
				Content: systemContent,
			})
		}
	}

	// Transform messages
	for i, msg := range req.Messages {
		openaiMsg := types.OpenAIMessage{
			Role: msg.Role,
		}
		
		// Log details of potentially problematic messages for debugging
		if msg.Role == "assistant" {
			hasText := false
			hasToolUse := false
			contentCount := 0
			
			switch content := msg.Content.(type) {
			case []interface{}:
				contentCount = len(content)
				for _, item := range content {
					if contentMap, ok := item.(map[string]interface{}); ok {
						if contentType, _ := contentMap["type"].(string); contentType == "text" {
							if text, ok := contentMap["text"].(string); ok && text != "" {
								hasText = true
							}
						} else if contentType == "tool_use" {
							hasToolUse = true
						}
					}
				}
			case string:
				if content != "" {
					hasText = true
					contentCount = 1
				}
			}
			
			if !hasText && !hasToolUse {
				loggerInstance.Warn("⚠️ Assistant message %d has no text or tool_use content (%d content items)", 
					i, contentCount)
				// Log the actual content structure for debugging
				if contentBytes, err := json.Marshal(msg.Content); err == nil {
					loggerInstance.Warn("⚠️ Assistant message %d content: %s", i, string(contentBytes))
				}
			}
		}

		// Log user messages to track what they requested
		if msg.Role == "user" {
			modelLogger := loggerInstance.WithModel(req.Model)
			switch content := msg.Content.(type) {
			case string:
				contentLength := len(content)
				logger.LogUserRequest(ctx, modelLogger, contentLength)
			case []interface{}:
				for _, item := range content {
					if contentMap, ok := item.(map[string]interface{}); ok {
						if contentType, _ := contentMap["type"].(string); contentType == "text" {
							if text, ok := contentMap["text"].(string); ok {
								textLength := len(text)
								modelLogger.Debug("👤 User request (inner): %d", textLength)
							}
						}
					}
				}
			}
		}

		// Handle flexible content format (string or []Content)
		switch content := msg.Content.(type) {
		case string:
			// Simple string content (real Claude Code format)
			openaiMsg.Content = content
		case []interface{}:
			// Array format, need to convert to []Content
			var textParts []string
			var toolCalls []types.OpenAIToolCall

			for _, item := range content {
				if contentMap, ok := item.(map[string]interface{}); ok {
					contentType, _ := contentMap["type"].(string)
					switch contentType {
					case "text":
						if text, ok := contentMap["text"].(string); ok {
							textParts = append(textParts, text)
						}
					case "tool_use":
						// Handle tool_use conversion
						id, _ := contentMap["id"].(string)
						name, _ := contentMap["name"].(string)
						input, _ := contentMap["input"].(map[string]interface{})

						argsJson, _ := json.Marshal(input)
						toolCall := types.OpenAIToolCall{
							ID:   id,
							Type: "function",
							Function: types.OpenAIToolCallFunction{
								Name:      name,
								Arguments: string(argsJson),
							},
						}
						toolCalls = append(toolCalls, toolCall)
					case "tool_result":
						// Convert tool result to OpenAI format
						openaiMsg.Role = "tool"
						if text, ok := contentMap["content"].(string); ok {
							// Handle empty tool results to maintain OpenAI API compliance
							if cfg.HandleEmptyToolResults && strings.TrimSpace(text) == "" {
								// Determine tool-specific error message based on tool_use_id or content
								openaiMsg.Content = getEmptyToolResultMessage(contentMap)
								logger.LogEmptyToolResult(ctx, loggerInstance, openaiMsg.Content)
							} else {
								openaiMsg.Content = text
							}
						} else if cfg.HandleEmptyToolResults {
							// No content field - provide default message
							openaiMsg.Content = "Tool execution completed with no output"
							logger.LogMissingToolContent(ctx, loggerInstance)
						}
						if toolUseID, ok := contentMap["tool_use_id"].(string); ok {
							openaiMsg.ToolCallID = toolUseID
						}
					}
				}
			}

			// Set content
			if len(textParts) > 0 {
				openaiMsg.Content = strings.Join(textParts, "\n")
			}

			// Set tool calls
			if len(toolCalls) > 0 {
				openaiMsg.ToolCalls = toolCalls
			}
		default:
			// Fallback for unexpected format
			loggerInstance.Warn("⚠️ Unexpected content format: %T", content)
		}

		// Note: llama.cpp workarounds removed (as of llama.cpp latest release)
		// The server now properly handles OpenAI API specification:
		// - Assistant messages with tool_calls can have empty content (returns 200 OK)
		// - Empty user/system messages return proper 400 Bad Request (not 500 Server Error)
		// - No more infinite hangs on empty content (returns immediate 400 error)
		// - Requires updated llama.cpp server version with OpenAI API compliance fixes

		// Handle empty messages based on configuration
		if openaiMsg.Content == "" && len(openaiMsg.ToolCalls) == 0 {
			shouldAddContent := false
			var defaultContent string
			
			switch openaiMsg.Role {
			case "tool":
				if cfg.HandleEmptyToolResults {
					shouldAddContent = true
					defaultContent = "Tool execution completed with no output"
				}
			case "user":
				if cfg.HandleEmptyUserMessages {
					shouldAddContent = true
					defaultContent = "[Empty user message]"
				}
			case "assistant":
				// Assistant messages with no content and no tool calls are typically invalid
				// Let provider handle validation (will return proper error)
				loggerInstance.Warn("⚠️ Empty assistant message (letting provider validate)")
			}
			
			if shouldAddContent {
				openaiMsg.Content = defaultContent
				logger.LogDefaultContent(ctx, loggerInstance, openaiMsg.Role)
			} else if openaiMsg.Content == "" && len(openaiMsg.ToolCalls) == 0 {
				// Log but don't modify - let provider handle validation
				loggerInstance.Warn("⚠️ Empty message: role=%s (letting provider validate)", 
					openaiMsg.Role)
			}
		}

		openaiReq.Messages = append(openaiReq.Messages, openaiMsg)
	}

	// Debug logging: print all messages being sent
	modelLogger := loggerInstance.WithModel(req.Model)
	modelLogger.Debug("🔍 Final message list (%d messages):", len(openaiReq.Messages))
	for i, msg := range openaiReq.Messages {
		contentPreview := msg.Content
		// Show full content for validation errors, otherwise truncate at 100 chars
		if len(contentPreview) > 100 && !strings.Contains(contentPreview, "InputValidationError") {
			contentPreview = contentPreview[:100] + "..."
		}
		
		// Build message info
		msgInfo := fmt.Sprintf("🔍   Message %d: role=%s", i, msg.Role)
		
		// Add content_len if there's content
		if len(msg.Content) > 0 {
			msgInfo += fmt.Sprintf(", content_len=%d", len(msg.Content))
		}
		
		// Add tool_calls if there are any
		if len(msg.ToolCalls) > 0 {
			msgInfo += fmt.Sprintf(", tool_calls=%d", len(msg.ToolCalls))
		}
		
		// Add tool_call_id if present
		if msg.ToolCallID != "" {
			msgInfo += fmt.Sprintf(", tool_call_id=%s", msg.ToolCallID)
		}
		
		modelLogger.Debug(msgInfo)
		modelLogger.Debug("🔍     Content preview: %q", contentPreview)
	}

	// Transform tools
	if len(req.Tools) > 0 {
		// Filter tools based on skip list
		var filteredTools []types.Tool
		var skippedTools []string
		
		for _, tool := range req.Tools {
			shouldSkip := false
			for _, skipTool := range cfg.SkipTools {
				if tool.Name == skipTool {
					shouldSkip = true
					skippedTools = append(skippedTools, tool.Name)
					break
				}
			}
			if !shouldSkip {
				filteredTools = append(filteredTools, tool)
			}
		}
		
		// Log skipped tools if any
		if len(skippedTools) > 0 {
			logger.LogToolsSkipped(ctx, loggerInstance, len(skippedTools), skippedTools)
		}
		
		// Print tool schemas if enabled (before transformation to see original Claude Code schemas)
		if cfg.PrintToolSchemas && len(filteredTools) > 0 {
			logger.LogToolSchemas(ctx, loggerInstance, filteredTools)
		}
		
		// Transform filtered tools
		if len(filteredTools) > 0 {
			openaiReq.Tools = make([]types.OpenAITool, len(filteredTools))
			for i, tool := range filteredTools {
				// Attempt to restore corrupted tool schemas before processing
				if tool.InputSchema.Type == "" || tool.InputSchema.Properties == nil {
					loggerInstance.Warn("⚠️ Malformed tool schema detected for %s, attempting restoration", tool.Name)
					
					// Try to restore the schema by finding a valid tool in the original request
					if RestoreCorruptedToolSchema(&tool, req.Tools, loggerInstance) {
						// Schema was restored, continue with processing
						filteredTools[i] = tool // Update the tool in the slice
					} else {
						// Could not restore, log the corruption details
						loggerInstance.Error("❌ Schema restoration failed for %s: type=%q, properties=%v, required=%v", 
							tool.Name, tool.InputSchema.Type, tool.InputSchema.Properties, tool.InputSchema.Required)
						loggerInstance.Debug("🔍 Original corrupted tool: %+v", tool)
					}
				}
				
				// Use YAML override description if available, otherwise use original
				description := cfg.GetToolDescription(tool.Name, tool.Description)
				
				openaiReq.Tools[i] = types.OpenAITool{
					Type: "function",
					Function: types.OpenAIToolFunction{
						Name:        tool.Name,
						Description: description,
						Parameters:  tool.InputSchema,
					},
				}
				
				// Verify transformation preserved schema correctly
				if openaiReq.Tools[i].Function.Parameters.Type == "" && tool.InputSchema.Type != "" {
					loggerInstance.Error("❌ Schema corruption during assignment for %s: input_type=%q → output_type=%q", 
						tool.Name, tool.InputSchema.Type, openaiReq.Tools[i].Function.Parameters.Type)
				}
			}

			logger.LogToolsTransformed(ctx, modelLogger, len(openaiReq.Tools), len(req.Tools))
			
			// Log first few tool names for debugging
			toolNames := make([]string, len(openaiReq.Tools))
			for i, tool := range openaiReq.Tools {
				toolNames[i] = tool.Function.Name
			}
			logger.LogToolNames(ctx, modelLogger, toolNames)
		} else {
			loggerInstance.Info("🚫 All %d tools were skipped", len(req.Tools))
		}
	}

	return openaiReq, nil
}

// TransformOpenAIToAnthropic converts OpenAI response format to Anthropic format
func TransformOpenAIToAnthropic(ctx context.Context, resp *types.OpenAIResponse, model string, cfg *config.Config) (*types.AnthropicResponse, error) {
	// Set up logger for this function
	loggerConfig := logger.NewConfigAdapter(cfg)
	loggerInstance := logger.FromContext(ctx, loggerConfig)
	
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in OpenAI response")
	}

	choice := resp.Choices[0]

	// Convert content
	var content []types.Content

	// Add text content if present
	if choice.Message.Content != "" {
		content = append(content, types.Content{
			Type: "text",
			Text: choice.Message.Content,
		})
	}

	// Add tool calls if present
	for _, toolCall := range choice.Message.ToolCalls {
		// Parse arguments back to map
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			loggerInstance.Warn("⚠️ Failed to parse tool arguments: %v", err)
			args = make(map[string]interface{})
		}

		toolContent := types.Content{
			Type:  "tool_use",
			ID:    toolCall.ID,
			Name:  toolCall.Function.Name,
			Input: args,
		}
		content = append(content, toolContent)

		loggerInstance.Debug("🔧 Tool call detected in OpenAI response: %s(id=%s) with args: %v",
			toolCall.Function.Name, toolCall.ID, args)
	}

	// Determine stop reason
	stopReason := "end_turn"
	if choice.FinishReason != nil {
		switch *choice.FinishReason {
		case "tool_calls":
			stopReason = "tool_use"
		case "stop":
			stopReason = "end_turn"
		case "length":
			stopReason = "max_tokens"
		}
	}

	// Create Anthropic response
	anthropicResp := &types.AnthropicResponse{
		ID:           resp.ID,
		Type:         "message",
		Role:         "assistant",
		Model:        model,
		Content:      content,
		StopReason:   stopReason,
		StopSequence: nil,
		Usage: types.Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}

	// Log response transformation summary
	loggerInstance.Debug("✅ Transformed response: %d content items, stop_reason: %s",
		len(content), stopReason)

	return anthropicResp, nil
}
