package correction

import (
	"bytes"
	"claude-proxy/internal"
	"claude-proxy/types"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// getRequestID retrieves the request ID from context using internal package
func getRequestID(ctx context.Context) string {
	return internal.GetRequestID(ctx)
}

// getPropertyNames extracts property names from tool schema for debugging
func getPropertyNames(properties map[string]types.ToolProperty) []string {
	var names []string
	for name := range properties {
		names = append(names, name)
	}
	return names
}

// Service handles tool call correction using configurable model
type Service struct {
	endpoint      string
	apiKey        string
	enabled       bool
	modelName     string // Configurable model for corrections
	disableLogging bool   // Disable tool correction logging
}

// NewService creates a new tool correction service
func NewService(endpoint, apiKey string, enabled bool, modelName string, disableLogging bool) *Service {
	return &Service{
		endpoint:       endpoint,
		apiKey:         apiKey,
		enabled:        enabled,
		modelName:      modelName,
		disableLogging: disableLogging,
	}
}

// shouldLog determines if logging should be enabled for tool correction
func (s *Service) shouldLog() bool {
	return !s.disableLogging
}

// logParameterChanges logs detailed information about what parameters were changed during correction
func (s *Service) logParameterChanges(requestID string, original, corrected types.Content) {
	// Show basic tool name change (if any)
	if original.Name != corrected.Name {
		log.Printf("🔧[%s] Tool name correction: %s -> %s", requestID, original.Name, corrected.Name)
	}

	// Compare parameters and show changes
	originalParams := original.Input
	correctedParams := corrected.Input

	// Find added parameters
	for key, value := range correctedParams {
		if _, exists := originalParams[key]; !exists {
			log.Printf("➕[%s] Added parameter: %s = %v", requestID, key, value)
		}
	}

	// Find removed parameters
	for key, value := range originalParams {
		if _, exists := correctedParams[key]; !exists {
			log.Printf("➖[%s] Removed parameter: %s = %v", requestID, key, value)
		}
	}

	// Find changed parameters
	for key, newValue := range correctedParams {
		if oldValue, exists := originalParams[key]; exists {
			// Convert to strings for comparison to handle different types
			oldStr := fmt.Sprintf("%v", oldValue)
			newStr := fmt.Sprintf("%v", newValue)
			if oldStr != newStr {
				log.Printf("🔄[%s] Changed parameter: %s = %v -> %v", requestID, key, oldValue, newValue)
			}
		}
	}

	// If no parameter changes but names are same, it might be a structural fix
	if original.Name == corrected.Name && len(originalParams) == len(correctedParams) {
		hasChanges := false
		for key := range originalParams {
			if _, exists := correctedParams[key]; !exists {
				hasChanges = true
				break
			}
		}
		if !hasChanges {
			// Check if values changed
			for key, oldValue := range originalParams {
				if newValue, exists := correctedParams[key]; exists {
					if fmt.Sprintf("%v", oldValue) != fmt.Sprintf("%v", newValue) {
						hasChanges = true
						break
					}
				}
			}
		}
		if !hasChanges {
			log.Printf("🔧[%s] Tool correction applied (structural fix): %s", requestID, corrected.Name)
		}
	}
}

// CorrectToolCalls validates and corrects tool calls using two-stage approach
func (s *Service) CorrectToolCalls(ctx context.Context, toolCalls []types.Content, availableTools []types.Tool) ([]types.Content, error) {
	if !s.enabled {
		return toolCalls, nil
	}
	requestID := getRequestID(ctx)

	var correctedCalls []types.Content

	for _, call := range toolCalls {
		if call.Type != "tool_use" {
			correctedCalls = append(correctedCalls, call)
			continue
		}

		// Stage 0: Comprehensive validation
		validation := s.validateToolCall(ctx, call, availableTools)
		
		// If already valid, keep as-is
		if validation.IsValid && !validation.HasCaseIssue {
			if s.shouldLog() {
				log.Printf("✅[%s] Tool call valid: %s", requestID, call.Name)
			}
			correctedCalls = append(correctedCalls, call)
			continue
		}

		var currentCall = call
		
		// Stage 1: Fix tool name case issues (direct correction, no LLM)
		if validation.HasCaseIssue {
			if s.shouldLog() {
				log.Printf("🔧[%s] Tool correction: %s -> %s", requestID, currentCall.Name, validation.CorrectToolName)
			}
			currentCall = s.correctToolName(ctx, currentCall, validation.CorrectToolName)
			
			// Re-validate after name correction
			validation = s.validateToolCall(ctx, currentCall, availableTools)
			if validation.IsValid {
				correctedCalls = append(correctedCalls, currentCall)
				continue
			}
		}

		// Stage 2: Fix parameter issues (LLM correction)
		if len(validation.MissingParams) > 0 || len(validation.InvalidParams) > 0 {
			if s.shouldLog() {
				log.Printf("🔧[%s] Correcting parameters with LLM", requestID)
			}
			correctedCall, err := s.correctToolCall(ctx, currentCall, availableTools)
			if err != nil {
				if s.shouldLog() {
					log.Printf("❌[%s] Parameter correction failed: %v", requestID, err)
				}
				correctedCalls = append(correctedCalls, currentCall)
			} else {
				if s.shouldLog() {
					// Log detailed parameter changes
					s.logParameterChanges(requestID, currentCall, correctedCall)
				}
				correctedCalls = append(correctedCalls, correctedCall)
			}
		} else {
			// Unknown issue - fall back to original LLM correction
			if s.shouldLog() {
				log.Printf("🔧[%s] Full LLM correction attempt", requestID)
			}
			correctedCall, err := s.correctToolCall(ctx, currentCall, availableTools)
			if err != nil {
				if s.shouldLog() {
					log.Printf("❌[%s] LLM correction failed: %v", requestID, err)
				}
				correctedCalls = append(correctedCalls, currentCall)
			} else {
				if s.shouldLog() {
					// Log detailed parameter changes
					s.logParameterChanges(requestID, currentCall, correctedCall)
				}
				correctedCalls = append(correctedCalls, correctedCall)
			}
		}
	}

	return correctedCalls, nil
}

// DetectToolNecessity analyzes user message to determine if tools should be required
func (s *Service) DetectToolNecessity(ctx context.Context, userMessage string, availableTools []types.Tool) (bool, error) {
	if !s.enabled {
		return false, nil
	}
	
	requestID := getRequestID(ctx)
	
	// Build analysis prompt
	prompt := s.buildToolNecessityPrompt(userMessage, availableTools)
	
	// Create request to correction model
	req := types.OpenAIRequest{
		Model: s.modelName,
		Messages: []types.OpenAIMessage{
			{
				Role:    "system",
				Content: "You are a tool necessity analyzer. Your job is to determine if a user request requires tools to be executed. Respond with ONLY 'YES' if tools should be required, or 'NO' if the request can be answered without tools.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens:   10, // Very short response needed
		Temperature: 0.1,
	}
	
	// Send request
	response, err := s.sendCorrectionRequest(req)
	if err != nil {
		if s.shouldLog() {
			log.Printf("⚠️[%s] Tool necessity detection failed: %v", requestID, err)
		}
		return false, err
	}
	
	// Parse response
	if len(response.Choices) == 0 {
		return false, fmt.Errorf("no response from tool necessity detection")
	}
	
	content := strings.TrimSpace(strings.ToUpper(response.Choices[0].Message.Content))
	shouldRequire := strings.HasPrefix(content, "YES")
	
	if s.shouldLog() {
		log.Printf("🎯[%s] Tool necessity analysis: %s -> %v", requestID, strings.ToLower(content), shouldRequire)
	}
	
	return shouldRequire, nil
}

// buildToolNecessityPrompt creates the prompt for tool necessity analysis
func (s *Service) buildToolNecessityPrompt(userMessage string, availableTools []types.Tool) string {
	// Build available tools list
	var toolNames []string
	for _, tool := range availableTools {
		toolNames = append(toolNames, tool.Name)
	}
	
	return fmt.Sprintf(`Analyze this user request and determine if it requires using tools to complete:

USER REQUEST: "%s"

AVAILABLE TOOLS: %s

Examples of requests that REQUIRE tools (answer YES):
- "check the file contents"
- "read the code in main.go" 
- "search for function definitions"
- "run this command"
- "find all instances of X"
- "create/write/edit a file"
- "execute tests"
- "grep for patterns"

Examples of requests that DON'T require tools (answer NO):
- "explain how X works"
- "what is the difference between X and Y"
- "help me understand this concept"
- "describe the architecture"
- "what are best practices for X"

Respond with ONLY "YES" or "NO".`, userMessage, strings.Join(toolNames, ", "))
}

// findToolByName finds a tool by exact name match
func (s *Service) findToolByName(name string, availableTools []types.Tool) *types.Tool {
	for _, t := range availableTools {
		if t.Name == name {
			return &t
		}
	}
	return nil
}

// findToolByCaseInsensitiveName finds a tool by case-insensitive name match
func (s *Service) findToolByCaseInsensitiveName(name string, availableTools []types.Tool) *types.Tool {
	for _, t := range availableTools {
		if strings.EqualFold(t.Name, name) {
			return &t
		}
	}
	return nil
}

// ValidationResult represents the result of tool call validation
type ValidationResult struct {
	IsValid          bool
	HasCaseIssue     bool
	CorrectToolName  string
	MissingParams    []string
	InvalidParams    []string
}

// validateToolCall performs comprehensive tool call validation
func (s *Service) validateToolCall(ctx context.Context, call types.Content, availableTools []types.Tool) ValidationResult {
	requestID := getRequestID(ctx)
	result := ValidationResult{IsValid: false}

	// Try exact match first
	tool := s.findToolByName(call.Name, availableTools)
	if tool == nil {
		// Try case-insensitive match
		tool = s.findToolByCaseInsensitiveName(call.Name, availableTools)
		if tool != nil {
			result.HasCaseIssue = true
			result.CorrectToolName = tool.Name
			if s.shouldLog() {
				log.Printf("⚠️[%s] Tool name case issue: '%s' should be '%s'", requestID, call.Name, tool.Name)
			}
		} else {
			if s.shouldLog() {
				log.Printf("⚠️[%s] Unknown tool: %s", requestID, call.Name)
			}
			return result
		}
	}

	// Check required parameters
	for _, required := range tool.InputSchema.Required {
		if _, exists := call.Input[required]; !exists {
			result.MissingParams = append(result.MissingParams, required)
		}
	}

	// Check for invalid parameters
	for param := range call.Input {
		if _, exists := tool.InputSchema.Properties[param]; !exists {
			result.InvalidParams = append(result.InvalidParams, param)
			if s.shouldLog() {
				log.Printf("🔍[%s] Parameter '%s' not found in schema for %s. Available: %v", 
					requestID, param, tool.Name, getPropertyNames(tool.InputSchema.Properties))
			}
		}
	}

	if len(result.MissingParams) > 0 && s.shouldLog() {
		log.Printf("⚠️[%s] Missing required parameters in %s: %v", requestID, call.Name, result.MissingParams)
	}
	if len(result.InvalidParams) > 0 && s.shouldLog() {
		log.Printf("⚠️[%s] Invalid parameters in %s: %v", requestID, call.Name, result.InvalidParams)
	}

	// Valid if no issues found (or only case issue which we can fix easily)
	result.IsValid = len(result.MissingParams) == 0 && len(result.InvalidParams) == 0
	return result
}

// isValidToolCall checks if a tool call matches available tool schemas (backward compatibility)
func (s *Service) isValidToolCall(ctx context.Context, call types.Content, availableTools []types.Tool) bool {
	result := s.validateToolCall(ctx, call, availableTools)
	return result.IsValid && !result.HasCaseIssue
}

// correctToolName fixes tool name case issues directly without LLM
func (s *Service) correctToolName(ctx context.Context, call types.Content, correctName string) types.Content {
	// Create corrected call with proper tool name
	correctedCall := call
	correctedCall.Name = correctName
	
	return correctedCall
}

// correctToolCall uses qwen2.5-coder to fix invalid tool calls
func (s *Service) correctToolCall(ctx context.Context, call types.Content, availableTools []types.Tool) (types.Content, error) {
	requestID := getRequestID(ctx)
	
	// Build correction prompt
	prompt := s.buildCorrectionPrompt(call, availableTools)

	// Create request to configured correction model
	req := types.OpenAIRequest{
		Model: s.modelName,
		Messages: []types.OpenAIMessage{
			{
				Role:    "system",
				Content: "You are a tool call correction expert. Fix the invalid tool call by correcting parameter names and values according to the provided schema. Respond with ONLY the corrected JSON tool call, no explanation.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens:   500,
		Temperature: 0.1, // Low temperature for consistent corrections
	}

	// Send request
	response, err := s.sendCorrectionRequest(req)
	if err != nil {
		return call, fmt.Errorf("[%s] correction request failed: %v", requestID, err)
	}

	// Parse corrected tool call
	correctedCall, err := s.parseCorrectedResponse(response, call)
	if err != nil {
		return call, fmt.Errorf("[%s] failed to parse correction: %v", requestID, err)
	}

	return correctedCall, nil
}

// buildCorrectionPrompt creates the prompt for qwen2.5-coder
func (s *Service) buildCorrectionPrompt(call types.Content, availableTools []types.Tool) string {
	// Find the correct tool schema
	var toolSchema types.Tool
	for _, tool := range availableTools {
		if tool.Name == call.Name {
			toolSchema = tool
			break
		}
	}

	// Serialize call and schema for the prompt
	callJson, _ := json.MarshalIndent(map[string]interface{}{
		"name":  call.Name,
		"input": call.Input,
	}, "", "  ")

	schemaJson, _ := json.MarshalIndent(toolSchema.InputSchema, "", "  ")

	// Check if this appears to be a TodoWrite-related correction
	todoExample := ""
	callStr := strings.ToLower(string(callJson))
	if strings.Contains(callStr, "todo") || strings.Contains(strings.ToLower(call.Name), "todo") {
		todoExample = `

COMPLEX SCHEMA EXAMPLE (TodoWrite):
INCORRECT: {"name": "TodoWrite", "input": {"todo": "Review MR", "status": "pending"}}
CORRECT: {"name": "TodoWrite", "input": {"todos": [{"content": "Review MR", "status": "pending", "priority": "medium", "id": "review-mr-task"}]}}

Key points for TodoWrite:
- Use 'todos' array, not individual 'todo' parameter  
- Each todo object needs: content, status, priority, id fields
- Generate missing required fields with sensible defaults (priority: "medium", id: descriptive slug)
- Preserve semantic information from original parameters`
	}

	return fmt.Sprintf(`Fix this invalid tool call to match the required schema:

INVALID TOOL CALL:
%s

REQUIRED SCHEMA:
%s

Common fixes needed:
- 'filename' should be 'file_path'
- 'path' should be 'file_path' 
- 'text' should be 'content'
- 'filter' should be 'glob' (for Grep tool file filtering)
- 'search' should be 'pattern' (for Grep tool)
- 'query' should be 'pattern' (for Grep tool)
- Ensure all required parameters are present%s

Return ONLY the corrected tool call in this exact JSON format:
{
  "name": "ToolName",
  "input": {
    "parameter1": "value1",
    "parameter2": "value2"
  }
}`, string(callJson), string(schemaJson), todoExample)
}

// sendCorrectionRequest sends request to qwen2.5-coder
func (s *Service) sendCorrectionRequest(req types.OpenAIRequest) (*types.OpenAIResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", s.endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.apiKey)

	client := &http.Client{
		Timeout: 10 * time.Minute, // 10 minute timeout for correction requests
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response types.OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return &response, nil
}

// parseCorrectedResponse extracts the corrected tool call from qwen2.5-coder response
func (s *Service) parseCorrectedResponse(response *types.OpenAIResponse, originalCall types.Content) (types.Content, error) {
	if len(response.Choices) == 0 {
		return originalCall, fmt.Errorf("no response from correction service")
	}

	content := response.Choices[0].Message.Content

	// Extract JSON from response (may have Markdown code blocks)
	jsonStart := strings.Index(content, "{")
	jsonEnd := strings.LastIndex(content, "}") + 1

	if jsonStart == -1 || jsonEnd <= jsonStart {
		return originalCall, fmt.Errorf("no valid JSON in correction response")
	}

	jsonStr := content[jsonStart:jsonEnd]

	// Parse corrected tool call
	var corrected struct {
		Name  string                 `json:"name"`
		Input map[string]interface{} `json:"input"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &corrected); err != nil {
		return originalCall, fmt.Errorf("failed to parse corrected JSON: %v", err)
	}

	// Create corrected content
	return types.Content{
		Type:  "tool_use",
		ID:    originalCall.ID, // Preserve original ID
		Name:  corrected.Name,
		Input: corrected.Input,
	}, nil
}
