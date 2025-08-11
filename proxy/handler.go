package proxy

import (
	"bytes"
	"claude-proxy/config"
	"claude-proxy/correction"
	"claude-proxy/logger"
	"claude-proxy/loop"
	"claude-proxy/types"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Handler handles HTTP proxy requests
type Handler struct {
	config               *config.Config
	correctionService    *correction.Service
	loggerConfig         logger.LoggerConfig
	conversationLogger   *logger.ConversationLogger
	loopDetector         *loop.LoopDetector
}

// NewHandler creates a new proxy handler
func NewHandler(cfg *config.Config, conversationLogger *logger.ConversationLogger) *Handler {
	return &Handler{
		config: cfg,
		correctionService: correction.NewService(
			cfg,
			cfg.ToolCorrectionAPIKey,
			cfg.ToolCorrectionEnabled,
			cfg.CorrectionModel,
			cfg.DisableToolCorrectionLogging,
		),
		loggerConfig:       logger.NewConfigAdapter(cfg),
		conversationLogger: conversationLogger,
		loopDetector:       loop.NewLoopDetector(),
	}
}

// HandleAnthropicRequest handles incoming Anthropic format requests
func (h *Handler) HandleAnthropicRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		// Early error - no context yet, use basic logging
		log.Printf("❌ Failed to read request body: %v", err)
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse Anthropic request
	var anthropicReq types.AnthropicRequest
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		// Early error - no context yet, use basic logging
		log.Printf("⚠️ Invalid JSON in request: %v", err)
		log.Printf("📋 Raw request body for debugging:")
		log.Printf("%s", string(body))
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Create context with request ID for tracing
	requestID := generateRequestID()
	ctx := withRequestID(r.Context(), requestID)
	
	// Set up logger context - request ID already set by withRequestID above
	loggerInstance := logger.New(ctx, h.loggerConfig)

	// Log conversation if enabled
	if h.conversationLogger != nil {
		h.conversationLogger.LogRequest(ctx, requestID, anthropicReq)
	}

	originalModel := anthropicReq.Model

	// Handle empty model to avoid server hanging (server workaround)
	if originalModel == "" {
		originalModel = h.config.BigModel // Use configured BIG_MODEL as fallback
		loggerInstance.WithModel(originalModel).Warn("Empty model provided, using fallback: %s (server workaround)", originalModel)
	}

	logger.LogRequest(ctx, loggerInstance.WithModel(originalModel), originalModel, len(anthropicReq.Tools))

	// Log available tools for this request
	if len(anthropicReq.Tools) > 0 {
		modelLogger := loggerInstance.WithModel(originalModel)
		modelLogger.Info("🔧 Available tools in request:")
		for _, tool := range anthropicReq.Tools {
			modelLogger.Debug("   - %s", tool.Name)
		}
	}

	// Map model name to provider-specific name using config
	mappedModel := h.config.MapModelName(ctx, originalModel)

	// Transform to OpenAI format with mapped model name
	anthropicReq.Model = mappedModel // Update the request with mapped model
	openaiReq, err := TransformAnthropicToOpenAI(ctx, anthropicReq, h.config)
	if err != nil {
		loggerInstance.Error("❌ Failed to transform request: %v", err)
		http.Error(w, "Request transformation failed", http.StatusInternalServerError)
		return
	}

	// Check for loop patterns in the conversation
	if h.loopDetector != nil {
		detection := h.loopDetector.DetectLoop(ctx, openaiReq.Messages)
		if detection.HasLoop {
			loggerInstance.Warn("🔄 Loop detected: %s (tool: %s, count: %d)", detection.LoopType, detection.ToolName, detection.Count)
			loggerInstance.Info("🔄 Breaking loop with recommendation: %s", detection.Recommendation)
			
			// Log conversation loop if enabled
			if h.conversationLogger != nil {
				h.conversationLogger.LogCorrection(ctx, requestID, nil, nil, fmt.Sprintf("loop_detection_%s_%s_%d", detection.LoopType, detection.ToolName, detection.Count))
			}
			
			// Return loop-breaking response immediately
			loopBreakResponse := h.loopDetector.CreateLoopBreakingResponse(detection)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(loopBreakResponse); err != nil {
				loggerInstance.Error("❌ Failed to encode loop-breaking response: %v", err)
			}
			return
		}
	}

	// Apply smart tool choice detection if enabled and tools are available
	if h.config.ToolCorrectionEnabled && len(openaiReq.Tools) > 0 && h.correctionService != nil {
		// Extract last N messages for context-aware analysis (max 10 messages)
		const maxContextMessages = 10
		contextMessages := openaiReq.Messages
		if len(contextMessages) > maxContextMessages {
			contextMessages = contextMessages[len(contextMessages)-maxContextMessages:]
		}
		
		// Convert OpenAI tools back to Anthropic format for analysis
		var analysisTools []types.Tool
		for _, openaiTool := range openaiReq.Tools {
			analysisTools = append(analysisTools, types.Tool{
				Name:        openaiTool.Function.Name,
				Description: openaiTool.Function.Description,
				InputSchema: openaiTool.Function.Parameters,
			})
		}
		
		shouldRequireTools, err := h.correctionService.DetectToolNecessity(ctx, contextMessages, analysisTools)
		if err != nil {
			loggerInstance.Warn("Tool necessity detection failed: %v", err)
		} else if shouldRequireTools {
			openaiReq.ToolChoice = "required"
			loggerInstance.Info("🎯 Tool choice set to 'required' based on conversation analysis")
		} else {
			loggerInstance.Info("🎯 Tool choice remains optional based on conversation analysis")
		}
	}

	// Route to appropriate provider based on mapped model (for endpoint selection)
	endpoint, apiKey := h.selectProvider(mappedModel)
	logger.LogModelRouting(ctx, loggerInstance.WithModel(originalModel), mappedModel, endpoint)

	// Analyze conversation structure for debugging
	hasUser := false
	for _, msg := range openaiReq.Messages {
		if msg.Role == "user" { hasUser = true }
	}

	// Only log if there are unusual conversation patterns
	if len(openaiReq.Messages) > 30 {
		modelLogger := loggerInstance.WithModel(originalModel)
		modelLogger.Debug("🔍 Large conversation: %d messages", len(openaiReq.Messages))
	}

	// Reject conversations without user messages (prevents infinite tool call loops)
	if !hasUser && len(openaiReq.Messages) > 1 {
		roles := extractRoles(openaiReq.Messages)
		
		// Enhanced debugging information
		loggerInstance.Error("🚨 INVALID CONVERSATION: Missing user message")
		loggerInstance.Error("🔍 Conversation details:")
		loggerInstance.Error("   - Message count: %d", len(openaiReq.Messages))
		loggerInstance.Error("   - Role sequence: %v", roles)
		loggerInstance.Error("   - Tools available: %d", len(openaiReq.Tools))
		
		// Analyze message content for debugging
		for i, msg := range openaiReq.Messages {
			contentLen := len(msg.Content)
			toolCallCount := len(msg.ToolCalls)
			loggerInstance.Error("   - Message %d: role=%s, content_len=%d, tool_calls=%d", 
				i, msg.Role, contentLen, toolCallCount)
			
			// Log tool calls if present (key for tool continuation scenarios)
			if toolCallCount > 0 {
				for j, tc := range msg.ToolCalls {
					loggerInstance.Error("     - Tool %d: %s (id=%s)", j, tc.Function.Name, tc.ID)
				}
			}
		}
		
		// Log request headers for origin tracking
		loggerInstance.Error("🔍 Request origin analysis:")
		loggerInstance.Error("   - User-Agent: %s", r.Header.Get("User-Agent"))
		loggerInstance.Error("   - Content-Length: %s", r.Header.Get("Content-Length"))
		loggerInstance.Error("   - Remote-Addr: %s", r.RemoteAddr)
		
		http.Error(w, "Invalid conversation: missing user message", http.StatusBadRequest)
		return
	}

	// Validate messages before sending to provider - role-aware validation
	invalidMessages := 0
	for i, msg := range openaiReq.Messages {
		switch msg.Role {
		case "tool":
			// Tool messages are always valid - they can have empty content
			// (empty tool results are now handled with placeholder messages)
			continue
		case "assistant":
			// Assistant messages are valid if they have content OR tool_calls
			if msg.Content == "" && len(msg.ToolCalls) == 0 {
				loggerInstance.Error("❌ Invalid assistant message %d: empty content and no tool_calls", i)
				invalidMessages++
			}
		case "user", "system":
			// User and system messages are valid (content field always exists now)
			// Note: Server accepts empty content as long as the field is present
		}
	}
	
	// Log validation summary if there are issues or very large conversations
	if invalidMessages > 0 {
		logger.LogInvalidMessages(ctx, loggerInstance, invalidMessages, len(openaiReq.Messages))
	} else if len(openaiReq.Messages) > 30 {
		logger.LogLargeConversation(ctx, loggerInstance, len(openaiReq.Messages))
	}

	// Validate ExitPlanMode usage before sending to provider
	for _, msg := range openaiReq.Messages {
		if msg.ToolCalls != nil {
			for _, toolCall := range msg.ToolCalls {
				if toolCall.Function.Name == "ExitPlanMode" {
					// Convert to types.Content for validation
					exitPlanCall := types.Content{
						Type: "tool_use",
						ID:   toolCall.ID,
						Name: toolCall.Function.Name,
						Input: make(map[string]interface{}),
					}
					
					// Parse arguments JSON to map for validation
					var args map[string]interface{}
					if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err == nil {
						exitPlanCall.Input = args
					}
					
					// Validate ExitPlanMode usage
					shouldBlock, reason := h.correctionService.ValidateExitPlanMode(ctx, exitPlanCall, openaiReq.Messages)
					if shouldBlock {
						loggerInstance.Error("🚫 ExitPlanMode usage blocked: %s", reason)
						
						// Return educational response instead of forwarding to provider
						educationalResponse := &types.AnthropicResponse{
							ID:         "blocked-exitplanmode",
							Type:       "message",
							Role:       "assistant", 
							Model:      originalModel,
							Content:    []types.Content{{
								Type: "text",
								Text: fmt.Sprintf("I understand you want to use ExitPlanMode, but this tool should only be used for **planning before implementation**, not as a completion summary.\n\n**Issue detected**: %s\n\n**Proper ExitPlanMode usage:**\n- Use it BEFORE starting any implementation work\n- Use it to present a plan for user approval\n- Use it when you need to outline steps you will take\n\n**Avoid using ExitPlanMode for:**\n- Summarizing completed work\n- Reporting finished tasks\n- Indicating that implementation is done\n\nWould you like me to help you with the next steps instead?", reason),
							}},
							Usage:      types.Usage{InputTokens: 0, OutputTokens: 0},
							StopReason: "end_turn",
						}
						
						// Send educational response
						w.Header().Set("Content-Type", "application/json")
						if err := json.NewEncoder(w).Encode(educationalResponse); err != nil {
							loggerInstance.Error("❌ Failed to encode educational response: %v", err)
							http.Error(w, "Response encoding failed", http.StatusInternalServerError)
						}
						return
					}
				}
			}
		}
	}

	// Proxy to selected provider - handle streaming if requested
	response, err := h.proxyToProviderEndpoint(ctx, openaiReq, endpoint, apiKey, originalModel)
	if err != nil {
		loggerInstance.Error("❌ Proxy request failed: %v", err)
		http.Error(w, "Proxy request failed", http.StatusBadGateway)
		return
	}

	// Transform response back to Anthropic format (use original model name)
	anthropicResp, err := TransformOpenAIToAnthropic(ctx, response, originalModel, h.config)
	if err != nil {
		loggerInstance.Error("❌ Failed to transform response: %v", err)
		http.Error(w, "Response transformation failed", http.StatusInternalServerError)
		return
	}

	// Apply tool correction if needed - only if there are actual tool calls that need correction
	if HasToolCalls(anthropicResp.Content) && h.config.ToolCorrectionEnabled && NeedsCorrection(ctx, anthropicResp.Content, anthropicReq.Tools, h.correctionService, h.loggerConfig) {
		loggerInstance.Info("🔧 Starting tool correction for %d content items", len(anthropicResp.Content))
		originalContent := anthropicResp.Content
		correctedContent, err := h.correctionService.CorrectToolCalls(ctx, anthropicResp.Content, anthropicReq.Tools)
		if err != nil {
			loggerInstance.Warn("⚠️ Tool correction failed: %v", err)
			// Continue with original content if correction fails
		} else {
			// Log if any changes were made
			if len(correctedContent) != len(originalContent) {
				loggerInstance.Info("🔧 Tool correction changed content count: %d -> %d", len(originalContent), len(correctedContent))
			}
			
			// Check for actual changes in tool calls
			changesDetected := false
			for i, corrected := range correctedContent {
				if i < len(originalContent) && corrected.Type == "tool_use" && originalContent[i].Type == "tool_use" {
					if corrected.Name != originalContent[i].Name {
						loggerInstance.Info("🔧 Tool name changed: %s -> %s", originalContent[i].Name, corrected.Name)
						changesDetected = true
					}
					if len(corrected.Input) != len(originalContent[i].Input) {
						loggerInstance.Info("🔧 Tool input changed for %s: %d -> %d params", corrected.Name, len(originalContent[i].Input), len(corrected.Input))
						changesDetected = true
					}
				}
			}
			
			if !changesDetected {
				loggerInstance.Info("🔧 Tool correction completed - no changes detected")
			}
			
			// Log conversation correction if enabled
			if h.conversationLogger != nil && changesDetected {
				h.conversationLogger.LogCorrection(ctx, requestID, originalContent, correctedContent, "tool_correction")
			}
			
			anthropicResp.Content = correctedContent
		}
	}

	// Enhanced logging for response summary
	textItemCount := 0
	toolCallCount := 0
	modelLogger := loggerInstance.WithModel(originalModel)
	for _, content := range anthropicResp.Content {
		if content.Type == "text" {
			textItemCount++
		} else if content.Type == "tool_use" {
			toolCallCount++
			logger.LogToolUsed(ctx, modelLogger, content.Name, content.ID)
			
			// Log conversation tool call if enabled
			if h.conversationLogger != nil {
				h.conversationLogger.LogToolCall(ctx, requestID, content.Name, content.Input, nil) // Result will be from next request
			}
		}
	}
	logger.LogResponseSummary(ctx, modelLogger, textItemCount, toolCallCount, anthropicResp.StopReason)

	// Log conversation response if enabled
	if h.conversationLogger != nil {
		h.conversationLogger.LogResponse(ctx, requestID, anthropicResp)
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(anthropicResp); err != nil {
		loggerInstance.Error("❌ Failed to encode response: %v", err)
	}
}

// mapModelName is now handled by config.MapModelName() method
// This function has been removed in favor of configurable model mapping

// selectProvider determines which endpoint to use based on mapped model with failover support
func (h *Handler) selectProvider(mappedModel string) (endpoint, apiKey string) {
	// Route based on configured SMALL_MODEL to small model endpoint
	if mappedModel == h.config.SmallModel {
		return h.config.GetSmallModelEndpoint(), h.config.SmallModelAPIKey
	}

	// Default to big model endpoint for BIG_MODEL and others
	return h.config.GetBigModelEndpoint(), h.config.BigModelAPIKey
}

// proxyToProviderEndpoint sends the OpenAI request to a specific provider endpoint
func (h *Handler) proxyToProviderEndpoint(ctx context.Context, req types.OpenAIRequest, endpoint, apiKey, originalModel string) (*types.OpenAIResponse, error) {
	// Serialize request
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	// Create HTTP request with context for timeout/cancellation
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	// Get logger from context and use it for logging
	proxyLogger := logger.FromContext(ctx, h.loggerConfig).WithModel(originalModel)
	logger.LogProxyRequest(ctx, proxyLogger, endpoint, req.Stream)
	//too much verbosity log.Printf("📤 [%s] Request JSON: %s", requestID, string(reqBody))

	// Send request with timeout to prevent hanging (defensive programming)
	client := &http.Client{
		Timeout: 10 * time.Minute, // 10 minute timeout for long-running requests
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read error response
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Handle streaming vs non-streaming responses
	if req.Stream {
		logger.LogStreamingResponse(ctx, proxyLogger)
		return ProcessStreamingResponse(ctx, resp)
	} else {
		// Handle non-streaming response (current logic)
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %v", err)
		}

		var openaiResp types.OpenAIResponse
		if err := json.Unmarshal(respBody, &openaiResp); err != nil {
			return nil, fmt.Errorf("failed to parse response: %v", err)
		}

		logger.LogNonStreamingResponse(ctx, proxyLogger, len(openaiResp.Choices))
		return &openaiResp, nil
	}
}

// NOTE: isSmallModel and shouldLogForModel functions removed
// This logic is now handled by the logger.ConfigAdapter

// extractRoles returns a slice of message roles for debugging conversation structure
func extractRoles(messages []types.OpenAIMessage) []string {
	roles := make([]string, len(messages))
	for i, msg := range messages {
		roles[i] = msg.Role
	}
	return roles
}

// boolToYesNo converts boolean to YES/NO string for conversation structure logging
func boolToYesNo(b bool) string {
	if b { 
		return "YES" 
	}
	return "NO"
}

// HasToolCalls checks if the content contains any tool_use items
func HasToolCalls(content []types.Content) bool {
	for _, item := range content {
		if item.Type == "tool_use" {
			return true
		}
	}
	return false
}

// NeedsCorrection quickly checks if any tool calls need correction without doing the full correction process
func NeedsCorrection(ctx context.Context, content []types.Content, availableTools []types.Tool, correctionService *correction.Service, loggerConfig logger.LoggerConfig) bool {
	loggerInstance := logger.New(ctx, loggerConfig)
	
	for _, item := range content {
		if item.Type == "tool_use" {
			// Quick validation - if any tool call is invalid, correction is needed
			validation := correctionService.ValidateToolCall(ctx, item, availableTools)
			if !validation.IsValid || validation.HasCaseIssue || validation.HasToolNameIssue {
				return true
			}
			
			// Check for structural mismatches using generic approach
			if validation.IsValid && correctionService.HasStructuralMismatch(item, availableTools) {
				loggerInstance.Debug("🔍 NeedsCorrection: %s has structural mismatch, needs correction", item.Name)
				return true
			}
		}
	}
	return false
}
