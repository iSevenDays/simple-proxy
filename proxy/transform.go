package proxy

import (
	"claude-proxy/config"
	"claude-proxy/correction"
	"claude-proxy/logger"
	"claude-proxy/parser"
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
								// Apply system message overrides to tool result content
								processedText := text
								if len(cfg.SystemMessageOverrides.RemovePatterns) > 0 || 
								   len(cfg.SystemMessageOverrides.Replacements) > 0 ||
								   cfg.SystemMessageOverrides.Prepend != "" ||
								   cfg.SystemMessageOverrides.Append != "" {
									processedText = config.ApplySystemMessageOverrides(text, cfg.SystemMessageOverrides)
									if processedText != text {
										logger.LogSystemOverride(ctx, loggerInstance, len(text), len(processedText))
									}
								}
								openaiMsg.Content = processedText
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
		
		// Show more informative logging for different message types
		if len(msg.Content) == 0 && len(msg.ToolCalls) > 0 {
			// Assistant message with only tool calls - show tool info instead of empty content
			toolInfos := make([]string, len(msg.ToolCalls))
			for i, toolCall := range msg.ToolCalls {
				// Show tool name and key parameters for better debugging
				toolInfo := toolCall.Function.Name
				if toolCall.Function.Arguments != "" {
					var args map[string]interface{}
					if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err == nil {
						// Show actual parameter values for better debugging
						var paramPairs []string
						
						// For specific tools, show the most relevant parameters
						switch toolCall.Function.Name {
						case "Read", "Write", "Edit", "MultiEdit":
							if filePath, ok := args["file_path"].(string); ok {
								// Show just filename, not full path
								if parts := strings.Split(filePath, "/"); len(parts) > 0 {
									filename := parts[len(parts)-1]
									paramPairs = append(paramPairs, fmt.Sprintf("file=%s", filename))
								}
							}
							// Show additional relevant params for Write/Edit
							if toolCall.Function.Name != "Read" {
								if content, ok := args["content"].(string); ok && len(content) > 0 {
									// Show first 20 chars of content
									contentPreview := content
									if len(contentPreview) > 20 {
										contentPreview = contentPreview[:20] + "..."
									}
									paramPairs = append(paramPairs, fmt.Sprintf("content=%q", contentPreview))
								}
								if oldString, ok := args["old_string"].(string); ok && len(oldString) > 0 {
									// Show first 15 chars of old_string for Edit
									oldPreview := oldString
									if len(oldPreview) > 15 {
										oldPreview = oldPreview[:15] + "..."
									}
									paramPairs = append(paramPairs, fmt.Sprintf("old=%q", oldPreview))
								}
							}
						case "TodoWrite":
							if todos, ok := args["todos"].([]interface{}); ok {
								paramPairs = append(paramPairs, fmt.Sprintf("todos=%d", len(todos)))
							}
						case "Bash":
							if command, ok := args["command"].(string); ok {
								// Show first 40 chars of command
								commandPreview := command
								if len(commandPreview) > 40 {
									commandPreview = commandPreview[:40] + "..."
								}
								paramPairs = append(paramPairs, fmt.Sprintf("cmd=%q", commandPreview))
							}
						case "Grep":
							if pattern, ok := args["pattern"].(string); ok {
								paramPairs = append(paramPairs, fmt.Sprintf("pattern=%q", pattern))
							}
							if path, ok := args["path"].(string); ok {
								// Show just directory name for path
								if parts := strings.Split(path, "/"); len(parts) > 0 {
									dirname := parts[len(parts)-1]
									if dirname == "" && len(parts) > 1 {
										dirname = parts[len(parts)-2] + "/"
									}
									paramPairs = append(paramPairs, fmt.Sprintf("path=%s", dirname))
								}
							}
						case "Task":
							if prompt, ok := args["prompt"].(string); ok {
								// Show first 30 chars of prompt
								promptPreview := prompt
								if len(promptPreview) > 30 {
									promptPreview = promptPreview[:30] + "..."
								}
								paramPairs = append(paramPairs, fmt.Sprintf("prompt=%q", promptPreview))
							}
							if description, ok := args["description"].(string); ok {
								paramPairs = append(paramPairs, fmt.Sprintf("desc=%q", description))
							}
						case "WebSearch":
							if query, ok := args["query"].(string); ok {
								paramPairs = append(paramPairs, fmt.Sprintf("query=%q", query))
							}
						case "WebFetch":
							if url, ok := args["url"].(string); ok {
								// Show domain from URL
								if parts := strings.Split(url, "/"); len(parts) > 2 {
									domain := parts[2]
									paramPairs = append(paramPairs, fmt.Sprintf("url=%s", domain))
								} else {
									paramPairs = append(paramPairs, fmt.Sprintf("url=%s", url))
								}
							}
						default:
							// For other tools, show first few key parameters
							paramCount := 0
							for key, value := range args {
								if paramCount >= 2 { // Limit to 2 params to avoid log pollution
									break
								}
								valueStr := fmt.Sprintf("%v", value)
								if len(valueStr) > 20 {
									valueStr = valueStr[:20] + "..."
								}
								paramPairs = append(paramPairs, fmt.Sprintf("%s=%s", key, valueStr))
								paramCount++
							}
						}
						
						if len(paramPairs) > 0 {
							toolInfo += fmt.Sprintf("(%s)", strings.Join(paramPairs, ", "))
						}
					}
				}
				toolInfos[i] = toolInfo
			}
			modelLogger.Debug("🔍     Tools: [%s]", strings.Join(toolInfos, ", "))
		} else if len(msg.Content) > 0 {
			// Show content preview for non-empty messages
			modelLogger.Debug("🔍     Content preview: %q", contentPreview)
		}
		// Skip logging entirely for empty content with no tools
	}

	// Transform tools
	if len(req.Tools) > 0 {
		// Context-aware tool filtering: Analyze conversation to determine appropriate tools
		contextBasedSkipTools := make([]string, len(cfg.SkipTools))
		copy(contextBasedSkipTools, cfg.SkipTools)
		
		// Check if conversation suggests research/analysis rather than planning
		if shouldSkipExitPlanMode(ctx, req.Messages, cfg) {
			contextBasedSkipTools = append(contextBasedSkipTools, "ExitPlanMode")
			loggerInstance.Info("🔍 Context analysis: ExitPlanMode filtered out (research/analysis detected)")
		}
		
		// Filter tools based on skip list (including context-based additions)
		var filteredTools []types.Tool
		var skippedTools []string
		
		for _, tool := range req.Tools {
			shouldSkip := false
			for _, skipTool := range contextBasedSkipTools {
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

	// ============================================================================
	// HARMONY DETECTION AND PRE-PROCESSING (Stream B)
	// ============================================================================
	// Chain-of-responsibility pattern: attempt Harmony parsing first, 
	// then fallback to existing transformation logic if not Harmony format
	
	if cfg.IsHarmonyParsingEnabled() && choice.Message.Content != "" {
		if isHarmonyContent, harmonyChannels := detectHarmonyContent(ctx, choice.Message.Content, cfg, loggerInstance); isHarmonyContent {
			// Content contains Harmony formatting - delegate to Stream C for response building
			// This is the integration point where Stream C will build the response with thinking metadata
			
			if cfg.IsHarmonyDebugEnabled() {
				loggerInstance.Debug("🔍 Harmony content detected, channels extracted: %d", len(harmonyChannels))
				for i, channel := range harmonyChannels {
					loggerInstance.Debug("🔍   Channel %d: type=%s, content_type=%s, content_len=%d", 
						i, channel.Type, channel.ContentType, len(channel.Content))
				}
			}
			
			// Store channels in response for Stream C to process
			// Note: Stream C will implement the actual response building logic
			// that maps channels to appropriate response fields with thinking metadata
			return buildHarmonyResponse(ctx, resp, choice, harmonyChannels, model, cfg, loggerInstance)
		}
	}
	
	// ============================================================================
	// STANDARD CONTENT CONVERSION (existing logic preserved)
	// ============================================================================
	// No Harmony content detected or Harmony parsing disabled - use existing transformation
	
	return buildStandardResponse(ctx, resp, choice, model, cfg, loggerInstance)
}

// shouldSkipExitPlanMode analyzes conversation context using LLM to determine if ExitPlanMode should be filtered out
// Root cause fix: Prevent ExitPlanMode availability during research/analysis tasks
func shouldSkipExitPlanMode(ctx context.Context, messages []types.Message, cfg *config.Config) bool {
	// Validate inputs - critical for preventing nil pointer panics
	if cfg == nil || len(messages) == 0 {
		return false
	}
	
	// Extract the user's request (first message)
	var userRequest string
	for _, msg := range messages {
		if msg.Role == "user" {
			userRequest = extractUserText(msg)
			break // Only check the first user message for the initial intent
		}
	}
	
	if strings.TrimSpace(userRequest) == "" {
		return false
	}
	
	// Use LLM analysis for more nuanced understanding
	return shouldSkipExitPlanModeLLM(ctx, strings.TrimSpace(userRequest), cfg)
}

// shouldSkipExitPlanModeLLM uses LLM analysis to determine if ExitPlanMode should be filtered out
func shouldSkipExitPlanModeLLM(ctx context.Context, userRequest string, cfg *config.Config) bool {
	// Validate inputs - cfg already validated by caller, but check userRequest
	if strings.TrimSpace(userRequest) == "" {
		return false
	}
	
	// Use the correction service's dedicated context analysis method
	correctionService := correction.NewService(cfg, cfg.ToolCorrectionAPIKey, true, cfg.CorrectionModel, false, nil)
	
	// Use the specialized AnalyzeRequestContext method
	shouldFilter, err := correctionService.AnalyzeRequestContext(ctx, userRequest)
	if err != nil {
		// If LLM analysis fails, conservative fallback - don't filter
		return false
	}
	
	return shouldFilter
}

// extractUserText extracts text content from a message, handling both string and []Content formats
// This helper function improves maintainability and reduces code duplication
func extractUserText(msg types.Message) string {
	switch content := msg.Content.(type) {
	case string:
		return content
	case []types.Content:
		var texts []string
		for _, c := range content {
			if c.Type == "text" && c.Text != "" {
				texts = append(texts, c.Text)
			}
		}
		return strings.Join(texts, " ")
	}
	return ""
}

// ============================================================================
// HARMONY DETECTION AND PARSING (Stream B Implementation)
// ============================================================================

// detectHarmonyContent implements chain-of-responsibility pattern for Harmony detection
// Returns (isHarmony, channels) tuple where isHarmony indicates if content contains
// Harmony formatting and channels contains the extracted channel data
func detectHarmonyContent(ctx context.Context, content string, cfg *config.Config, logger logger.Logger) (bool, []parser.Channel) {
	// Fast detection first - optimize for performance on non-Harmony content
	if !parser.IsHarmonyFormat(content) {
		if cfg.IsHarmonyDebugEnabled() {
			logger.Debug("🔍 No Harmony tokens detected in content")
		}
		return false, nil
	}
	
	// Harmony tokens detected - perform full parsing
	if cfg.IsHarmonyDebugEnabled() {
		logger.Debug("🔍 Harmony tokens detected, performing full extraction")
	}
	
	channels := parser.ExtractChannels(content)
	
	// Validate extraction results
	if len(channels) == 0 {
		if cfg.IsHarmonyDebugEnabled() {
			logger.Debug("🔍 Harmony tokens found but no channels extracted - treating as non-Harmony")
		}
		return false, nil
	}
	
	// Log extraction summary
	if cfg.IsHarmonyDebugEnabled() {
		thinkingChannels := 0
		responseChannels := 0
		toolCallChannels := 0
		
		for _, channel := range channels {
			switch {
			case channel.IsThinking():
				thinkingChannels++
			case channel.IsResponse():
				responseChannels++
			case channel.IsToolCall():
				toolCallChannels++
			}
		}
		
		logger.Debug("🔍 Harmony extraction complete: %d total channels (thinking=%d, response=%d, tools=%d)", 
			len(channels), thinkingChannels, responseChannels, toolCallChannels)
	}
	
	return true, channels
}

// buildHarmonyResponse builds responses with thinking metadata from parsed Harmony content
// Maps analysis channels to thinking metadata, final channels to main response content,
// and commentary channels to tool calls using the extended types from Issue #3
func buildHarmonyResponse(ctx context.Context, resp *types.OpenAIResponse, choice types.OpenAIChoice, channels []parser.Channel, model string, cfg *config.Config, logger logger.Logger) (*types.AnthropicResponse, error) {
	logger.Debug("🎵 Building Harmony response with %d channels", len(channels))
	
	// Separate channels by content type
	var thinkingChannels []parser.Channel
	var responseChannels []parser.Channel
	var toolCallChannels []parser.Channel
	var regularContent []types.Content
	
	for _, channel := range channels {
		switch {
		case channel.IsThinking():
			thinkingChannels = append(thinkingChannels, channel)
			logger.Debug("🎵   Analysis channel: type=%s, content_len=%d", channel.Type, len(channel.Content))
		case channel.IsResponse():
			responseChannels = append(responseChannels, channel)
			logger.Debug("🎵   Response channel: type=%s, content_len=%d", channel.Type, len(channel.Content))
		case channel.IsToolCall():
			toolCallChannels = append(toolCallChannels, channel)
			logger.Debug("🎵   Tool call channel: type=%s, content_len=%d", channel.Type, len(channel.Content))
		default:
			// Handle unknown channels as regular text content
			if channel.Content != "" {
				regularContent = append(regularContent, types.Content{
					Type: "text",
					Text: channel.Content,
				})
				logger.Debug("🎵   Regular channel: type=%s, content_len=%d", channel.Type, len(channel.Content))
			}
		}
	}
	
	// Build thinking metadata from analysis channels
	var thinkingContent *string
	if len(thinkingChannels) > 0 {
		var thinkingParts []string
		for _, channel := range thinkingChannels {
			if channel.Content != "" {
				thinkingParts = append(thinkingParts, channel.Content)
			}
		}
		if len(thinkingParts) > 0 {
			combinedThinking := strings.Join(thinkingParts, "\n\n")
			thinkingContent = &combinedThinking
			logger.Debug("🎵 Combined thinking content: %d characters", len(combinedThinking))
		}
	}
	
	// Build main response content from final channels
	var content []types.Content
	if len(responseChannels) > 0 {
		var responseParts []string
		for _, channel := range responseChannels {
			if channel.Content != "" {
				responseParts = append(responseParts, channel.Content)
			}
		}
		if len(responseParts) > 0 {
			combinedResponse := strings.Join(responseParts, "\n\n")
			content = append(content, types.Content{
				Type: "text",
				Text: combinedResponse,
			})
			logger.Debug("🎵 Combined response content: %d characters", len(combinedResponse))
		}
	}
	
	// Handle tool calls from commentary channels and original OpenAI tool calls
	toolCallsProcessed := false
	for _, toolCall := range choice.Message.ToolCalls {
		// Parse arguments back to map
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			logger.Warn("⚠️ Failed to parse tool arguments in Harmony response: %v", err)
			args = make(map[string]interface{})
		}

		toolContent := types.Content{
			Type:  "tool_use",
			ID:    toolCall.ID,
			Name:  toolCall.Function.Name,
			Input: args,
		}
		content = append(content, toolContent)
		toolCallsProcessed = true

		logger.Debug("🎵 Tool call processed: %s(id=%s) with args: %v",
			toolCall.Function.Name, toolCall.ID, args)
	}
	
	// Add any commentary channel content as additional context if no tool calls found
	if !toolCallsProcessed && len(toolCallChannels) > 0 {
		for _, channel := range toolCallChannels {
			if channel.Content != "" {
				content = append(content, types.Content{
					Type: "text",
					Text: channel.Content,
				})
				logger.Debug("🎵 Commentary channel added as text: %d characters", len(channel.Content))
			}
		}
	}
	
	// Add any regular content that didn't fit into specific channels
	content = append(content, regularContent...)
	
	// Fallback: if no response channels were found, use original choice content
	if len(responseChannels) == 0 && choice.Message.Content != "" {
		content = append(content, types.Content{
			Type: "text",
			Text: choice.Message.Content,
		})
		logger.Debug("🎵 Fallback: using original choice content (%d characters)", len(choice.Message.Content))
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
	
	// Create Anthropic response with thinking metadata
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
		// Thinking metadata fields from Issue #3
		ThinkingContent: thinkingContent,
		HarmonyChannels: channels, // Store original channels for debugging
	}
	
	// Log transformation summary
	hasThinking := anthropicResp.HasThinking()
	logger.Debug("🎵 Harmony response built: %d content items, thinking=%v, stop_reason=%s",
		len(content), hasThinking, stopReason)
	
	if hasThinking {
		logger.Debug("🎵 Thinking metadata: %d characters", len(anthropicResp.GetThinkingText()))
	}
	
	return anthropicResp, nil
}

// buildStandardResponse handles non-Harmony content using existing transformation logic
// This preserves the original transformation behavior for non-Harmony responses
func buildStandardResponse(ctx context.Context, resp *types.OpenAIResponse, choice types.OpenAIChoice, model string, cfg *config.Config, logger logger.Logger) (*types.AnthropicResponse, error) {
	// Convert content using existing logic
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
			logger.Warn("⚠️ Failed to parse tool arguments: %v", err)
			args = make(map[string]interface{})
		}

		toolContent := types.Content{
			Type:  "tool_use",
			ID:    toolCall.ID,
			Name:  toolCall.Function.Name,
			Input: args,
		}
		content = append(content, toolContent)

		logger.Debug("🔧 Tool call detected in OpenAI response: %s(id=%s) with args: %v",
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
	logger.Debug("✅ Transformed response: %d content items, stop_reason: %s",
		len(content), stopReason)

	return anthropicResp, nil
}
