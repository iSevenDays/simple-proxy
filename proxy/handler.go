package proxy

import (
	"bytes"
	"claude-proxy/config"
	"claude-proxy/correction"
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
	config            *config.Config
	correctionService *correction.Service
}

// NewHandler creates a new proxy handler
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{
		config: cfg,
		correctionService: correction.NewService(
			cfg.ToolCorrectionEndpoint,
			cfg.ToolCorrectionAPIKey,
			cfg.ToolCorrectionEnabled,
			cfg.CorrectionModel,
			cfg.DisableToolCorrectionLogging,
		),
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
		log.Printf("❌ Failed to read request body: %v", err)
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse Anthropic request
	var anthropicReq types.AnthropicRequest
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		log.Printf("⚠️ Invalid JSON in request: %v", err)
		log.Printf("📋 Raw request body for debugging:")
		log.Printf("%s", string(body))
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Create context with request ID for tracing
	requestID := generateRequestID()
	ctx := withRequestID(r.Context(), requestID)

	originalModel := anthropicReq.Model

	// Handle empty model to avoid server hanging (server workaround)
	if originalModel == "" {
		originalModel = h.config.BigModel // Use configured BIG_MODEL as fallback
		log.Printf("⚠️ [%s] Empty model provided, using fallback: %s (server workaround)", requestID, originalModel)
	}

	if h.shouldLogForModel(ctx, originalModel) {
		log.Printf("📨 [%s] Received request for model: %s, tools: %d",
			requestID, originalModel, len(anthropicReq.Tools))
	}

	// Log available tools for this request
	if len(anthropicReq.Tools) > 0 && h.shouldLogForModel(ctx, originalModel) {
		log.Printf("🔧 [%s] Available tools in request:", requestID)
		for _, tool := range anthropicReq.Tools {
			log.Printf("   - %s", tool.Name)
		}
	}

	// Map model name to provider-specific name using config
	mappedModel := h.config.MapModelName(ctx, originalModel)

	// Transform to OpenAI format with mapped model name
	anthropicReq.Model = mappedModel // Update the request with mapped model
	openaiReq, err := TransformAnthropicToOpenAI(ctx, anthropicReq, h.config)
	if err != nil {
		log.Printf("❌ [%s] Failed to transform request: %v", requestID, err)
		http.Error(w, "Request transformation failed", http.StatusInternalServerError)
		return
	}

	// Apply smart tool choice detection if enabled and tools are available
	if h.config.ToolCorrectionEnabled && len(openaiReq.Tools) > 0 && h.correctionService != nil {
		// Extract the last user message for analysis
		var lastUserMessage string
		for i := len(openaiReq.Messages) - 1; i >= 0; i-- {
			if openaiReq.Messages[i].Role == "user" {
				lastUserMessage = openaiReq.Messages[i].Content
				break
			}
		}
		
		if lastUserMessage != "" {
			// Convert OpenAI tools back to Anthropic format for analysis
			var analysisTools []types.Tool
			for _, openaiTool := range openaiReq.Tools {
				analysisTools = append(analysisTools, types.Tool{
					Name:        openaiTool.Function.Name,
					Description: openaiTool.Function.Description,
					InputSchema: openaiTool.Function.Parameters,
				})
			}
			
			shouldRequireTools, err := h.correctionService.DetectToolNecessity(ctx, lastUserMessage, analysisTools)
			if err != nil {
				log.Printf("⚠️[%s] Tool necessity detection failed: %v", requestID, err)
			} else if shouldRequireTools {
				openaiReq.ToolChoice = "required"
				log.Printf("🎯[%s] Tool choice set to 'required' based on request analysis", requestID)
			} else {
				log.Printf("🎯[%s] Tool choice remains optional based on request analysis", requestID)
			}
		}
	}

	// Route to appropriate provider based on mapped model (for endpoint selection)
	endpoint, apiKey := h.selectProvider(mappedModel)
	log.Printf("🎯 [%s] Model %s → Endpoint: %s", requestID, mappedModel, endpoint)

	// ALWAYS log message analysis for debugging - regardless of model type
	log.Printf("🔍 [%s] MESSAGE ANALYSIS:", requestID)
	log.Printf("   - Total messages: %d", len(openaiReq.Messages))

	// Extract and log role sequence
	roles := extractRoles(openaiReq.Messages)
	log.Printf("   - Role sequence: %v", roles)

	// Check for user messages and analyze conversation structure
	hasUser := false
	hasSystem := false
	hasAssistant := false
	for _, msg := range openaiReq.Messages {
		if msg.Role == "user" { hasUser = true }
		if msg.Role == "system" { hasSystem = true }
		if msg.Role == "assistant" { hasAssistant = true }
	}

	// Log conversation structure (matching the bug report format)
	log.Printf("🔍 [%s] CONVERSATION STRUCTURE: system=%s, user=%s, assistant=%s", 
		requestID, 
		boolToYesNo(hasSystem), 
		boolToYesNo(hasUser), 
		boolToYesNo(hasAssistant))

	// Reject conversations without user messages (prevents infinite tool call loops)
	if !hasUser && len(openaiReq.Messages) > 1 {
		roles := extractRoles(openaiReq.Messages)
		
		// Enhanced debugging information
		log.Printf("🚨 [%s] INVALID CONVERSATION: Missing user message", requestID)
		log.Printf("🔍 [%s] Conversation details:", requestID)
		log.Printf("   - Message count: %d", len(openaiReq.Messages))
		log.Printf("   - Role sequence: %v", roles)
		log.Printf("   - Tools available: %d", len(openaiReq.Tools))
		
		// Analyze message content for debugging
		for i, msg := range openaiReq.Messages {
			contentLen := len(msg.Content)
			toolCallCount := len(msg.ToolCalls)
			log.Printf("   - Message %d: role=%s, content_len=%d, tool_calls=%d", 
				i, msg.Role, contentLen, toolCallCount)
			
			// Log tool calls if present (key for tool continuation scenarios)
			if toolCallCount > 0 {
				for j, tc := range msg.ToolCalls {
					log.Printf("     - Tool %d: %s (id=%s)", j, tc.Function.Name, tc.ID)
				}
			}
		}
		
		// Log request headers for origin tracking
		log.Printf("🔍 [%s] Request origin analysis:", requestID)
		log.Printf("   - User-Agent: %s", r.Header.Get("User-Agent"))
		log.Printf("   - Content-Length: %s", r.Header.Get("Content-Length"))
		log.Printf("   - Remote-Addr: %s", r.RemoteAddr)
		
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
				log.Printf("❌[%s] Invalid assistant message %d: empty content and no tool_calls", requestID, i)
				invalidMessages++
			}
		case "user", "system":
			// User and system messages are valid (content field always exists now)
			// Note: Server accepts empty content as long as the field is present
		}
	}
	
	// Log validation summary if there are issues or very large conversations
	if invalidMessages > 0 {
		log.Printf("⚠️[%s] Found %d potentially invalid messages out of %d total", requestID, invalidMessages, len(openaiReq.Messages))
	} else if len(openaiReq.Messages) > 30 {
		log.Printf("📊[%s] Large conversation: %d messages", requestID, len(openaiReq.Messages))
	}

	// Proxy to selected provider - handle streaming if requested
	response, err := h.proxyToProviderEndpoint(ctx, openaiReq, endpoint, apiKey)
	if err != nil {
		log.Printf("❌ [%s] Proxy request failed: %v", requestID, err)
		http.Error(w, "Proxy request failed", http.StatusBadGateway)
		return
	}

	// Transform response back to Anthropic format (use original model name)
	anthropicResp, err := TransformOpenAIToAnthropic(ctx, response, originalModel)
	if err != nil {
		log.Printf("❌ [%s] Failed to transform response: %v", requestID, err)
		http.Error(w, "Response transformation failed", http.StatusInternalServerError)
		return
	}

	// Apply tool correction if needed
	if len(anthropicResp.Content) > 0 && h.config.ToolCorrectionEnabled {
		correctedContent, err := h.correctionService.CorrectToolCalls(ctx, anthropicResp.Content, anthropicReq.Tools)
		if err != nil {
			log.Printf("⚠️ [%s] Tool correction failed: %v", requestID, err)
			// Continue with original content if correction fails
		} else {
			anthropicResp.Content = correctedContent
		}
	}

	// Enhanced logging for response summary
	textItemCount := 0
	toolCallCount := 0
	for _, content := range anthropicResp.Content {
		if content.Type == "text" {
			textItemCount++
		} else if content.Type == "tool_use" {
			toolCallCount++
			if h.shouldLogForModel(ctx, originalModel) {
				log.Printf("🎯 [%s] Tool used in response: %s(id=%s)", requestID, content.Name, content.ID)
			}
		}
	}
	if h.shouldLogForModel(ctx, originalModel) {
		log.Printf("✅ [%s] Response summary: %d text_items, %d tool_calls, stop_reason=%s",
			requestID, textItemCount, toolCallCount, anthropicResp.StopReason)
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(anthropicResp); err != nil {
		log.Printf("❌ [%s] Failed to encode response: %v", requestID, err)
	}
}

// mapModelName is now handled by config.MapModelName() method
// This function has been removed in favor of configurable model mapping

// selectProvider determines which endpoint to use based on mapped model
func (h *Handler) selectProvider(mappedModel string) (endpoint, apiKey string) {
	// Route based on configured SMALL_MODEL to small model endpoint
	if mappedModel == h.config.SmallModel {
		return h.config.SmallModelEndpoint, h.config.SmallModelAPIKey
	}

	// Default to big model endpoint for BIG_MODEL and others
	return h.config.BigModelEndpoint, h.config.BigModelAPIKey
}

// proxyToProviderEndpoint sends the OpenAI request to a specific provider endpoint
func (h *Handler) proxyToProviderEndpoint(ctx context.Context, req types.OpenAIRequest, endpoint, apiKey string) (*types.OpenAIResponse, error) {
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

	requestID := GetRequestID(ctx)
	if h.shouldLogForModel(ctx, req.Model) {
		log.Printf("🚀 [%s] Proxying to: %s (streaming: %v)", requestID, endpoint, req.Stream)
	}
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
		log.Printf("🌊 [%s] Processing streaming response...", requestID)
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

		if h.shouldLogForModel(ctx, req.Model) {
			log.Printf("✅ [%s] Received non-streaming response with %d choices", requestID, len(openaiResp.Choices))
		}
		return &openaiResp, nil
	}
}

// isSmallModel checks if the given Claude model name maps to the small model
func (h *Handler) isSmallModel(ctx context.Context, claudeModel string) bool {
	// Check if the model maps to small model (Haiku)
	return claudeModel == "claude-3-5-haiku-20241022" || 
		   h.config.MapModelName(ctx, claudeModel) == h.config.SmallModel
}

// shouldLogForModel determines if logging should be enabled for the given model
func (h *Handler) shouldLogForModel(ctx context.Context, claudeModel string) bool {
	// If small model logging is disabled and this is a small model, don't log
	if h.config.DisableSmallModelLogging && h.isSmallModel(ctx, claudeModel) {
		return false
	}
	return true
}

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
