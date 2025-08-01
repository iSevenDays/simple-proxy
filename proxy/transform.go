package proxy

import (
	"claude-proxy/config"
	"claude-proxy/types"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// isSmallModel checks if the given model name maps to the small model configuration
func isSmallModel(ctx context.Context, model string, cfg *config.Config) bool {
	// Check if the model is Haiku or maps to small model
	return model == "claude-3-5-haiku-20241022" || 
		   cfg.MapModelName(ctx, model) == cfg.SmallModel ||
		   model == cfg.SmallModel
}

// shouldLogForModel determines if logging should be enabled for the given model
func shouldLogForModel(ctx context.Context, model string, cfg *config.Config) bool {
	// If small model logging is disabled and this is a small model, don't log
	if cfg.DisableSmallModelLogging && isSmallModel(ctx, model, cfg) {
		return false
	}
	return true
}

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

// TransformAnthropicToOpenAI converts Anthropic request format to OpenAI format
func TransformAnthropicToOpenAI(ctx context.Context, req types.AnthropicRequest, cfg *config.Config) (types.OpenAIRequest, error) {
	openaiReq := types.OpenAIRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		Stream:    req.Stream,
		Messages:  []types.OpenAIMessage{},
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
				
				requestID := GetRequestID(ctx)
				log.Printf("🔄 [%s] Applied system message overrides (original: %d chars, modified: %d chars)", 
					requestID, len(originalContent), len(systemContent))
			}
			
			// Print system message if enabled
			if cfg.PrintSystemMessage {
				requestID := GetRequestID(ctx)
				log.Printf("📋 [%s] System message (%d chars):\n%s", requestID, len(systemContent), systemContent)
			}
			
			openaiReq.Messages = append(openaiReq.Messages, types.OpenAIMessage{
				Role:    "system",
				Content: systemContent,
			})
		}
	}

	// Transform messages
	requestID := GetRequestID(ctx)
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
				log.Printf("⚠️[%s] Assistant message %d has no text or tool_use content (%d content items)", 
					requestID, i, contentCount)
				// Log the actual content structure for debugging
				if contentBytes, err := json.Marshal(msg.Content); err == nil {
					log.Printf("⚠️[%s] Assistant message %d content: %s", requestID, i, string(contentBytes))
				}
			}
		}

		// Log user messages to track what they requested
		if msg.Role == "user" {
			switch content := msg.Content.(type) {
			case string:
				contentLength := len(content)
				if shouldLogForModel(ctx, req.Model, cfg) {
					log.Printf("👤 [%s] User request: %d", requestID, contentLength)
				}
			case []interface{}:
				for _, item := range content {
					if contentMap, ok := item.(map[string]interface{}); ok {
						if contentType, _ := contentMap["type"].(string); contentType == "text" {
							if text, ok := contentMap["text"].(string); ok {
								textLength := len(text)
								if shouldLogForModel(ctx, req.Model, cfg) {
									log.Printf("👤 [%s] User request (inner): %d", requestID, textLength)
								}
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
								log.Printf("🔧[%s] Empty tool result replaced with placeholder message", requestID)
							} else {
								openaiMsg.Content = text
							}
						} else if cfg.HandleEmptyToolResults {
							// No content field - provide default message
							openaiMsg.Content = "Tool execution completed with no output"
							log.Printf("🔧[%s] Missing tool result content replaced with default message", requestID)
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
			requestID := GetRequestID(ctx)
			log.Printf("⚠️ [%s] Unexpected content format: %T", requestID, content)
		}

		// Note: llama.cpp workarounds removed (as of llama.cpp latest release)
		// The server now properly handles OpenAI API specification:
		// - Assistant messages with tool_calls can have empty content (returns 200 OK)
		// - Empty user/system messages return proper 400 Bad Request (not 500 Server Error)
		// - No more infinite hangs on empty content (returns immediate 400 error)
		// - Requires updated llama.cpp server version with OpenAI API compliance fixes

		// Validate message before adding to request
		if openaiMsg.Content == "" && len(openaiMsg.ToolCalls) == 0 {
			log.Printf("⚠️[%s] Potentially invalid message: role=%s, empty content and no tool_calls (from Anthropic message %d)", 
				requestID, openaiMsg.Role, i)
		}

		openaiReq.Messages = append(openaiReq.Messages, openaiMsg)
	}

	// Transform tools
	if len(req.Tools) > 0 {
		requestID := GetRequestID(ctx)
		
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
			log.Printf("🚫 [%s] Skipped %d tools: %v", requestID, len(skippedTools), skippedTools)
		}
		
		// Transform filtered tools
		if len(filteredTools) > 0 {
			openaiReq.Tools = make([]types.OpenAITool, len(filteredTools))
			for i, tool := range filteredTools {
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
			}

			if shouldLogForModel(ctx, req.Model, cfg) {
				log.Printf("🔧 [%s] Transformed %d tools to OpenAI format (filtered from %d)", requestID, len(openaiReq.Tools), len(req.Tools))
			}
			// Log first few tool names for debugging
			if shouldLogForModel(ctx, req.Model, cfg) {
				if len(openaiReq.Tools) <= 5 {
					toolNames := make([]string, len(openaiReq.Tools))
					for i, tool := range openaiReq.Tools {
						toolNames[i] = tool.Function.Name
					}
					log.Printf("     [%s] Tools: [%s]", requestID, strings.Join(toolNames, ", "))
				} else {
					log.Printf("     [%s] Tools: [%s, %s, ... and %d more]", requestID,
						openaiReq.Tools[0].Function.Name,
						openaiReq.Tools[1].Function.Name,
						len(openaiReq.Tools)-2)
				}
			}
		} else {
			log.Printf("🚫 [%s] All %d tools were skipped", requestID, len(req.Tools))
		}
	}

	return openaiReq, nil
}

// TransformOpenAIToAnthropic converts OpenAI response format to Anthropic format
func TransformOpenAIToAnthropic(ctx context.Context, resp *types.OpenAIResponse, model string) (*types.AnthropicResponse, error) {
	requestID := GetRequestID(ctx)
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("[%s] no choices in OpenAI response", requestID)
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
			log.Printf("⚠️[%s] Failed to parse tool arguments: %v", requestID, err)
			args = make(map[string]interface{})
		}

		toolContent := types.Content{
			Type:  "tool_use",
			ID:    toolCall.ID,
			Name:  toolCall.Function.Name,
			Input: args,
		}
		content = append(content, toolContent)

		log.Printf("🔧 [%s] Tool call detected in OpenAI response: %s(id=%s) with args: %v",
			requestID, toolCall.Function.Name, toolCall.ID, args)
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

	// Note: This function doesn't have access to config, so we can't conditionally log here
	// The conditional logging is handled in the handler.go for response summary
	log.Printf("✅ [%s] Transformed response: %d content items, stop_reason: %s",
		requestID, len(content), stopReason)

	return anthropicResp, nil
}
