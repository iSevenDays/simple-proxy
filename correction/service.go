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

// getInputParamNames extracts parameter names from tool call input for debugging
func getInputParamNames(input map[string]interface{}) []string {
	var names []string
	for name := range input {
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
	validator     types.ToolValidator // Injected tool validator
}

// NewService creates a new tool correction service with default StandardToolValidator
func NewService(endpoint, apiKey string, enabled bool, modelName string, disableLogging bool) *Service {
	return &Service{
		endpoint:       endpoint,
		apiKey:         apiKey,
		enabled:        enabled,
		modelName:      modelName,
		disableLogging: disableLogging,
		validator:      types.NewStandardToolValidator(), // Default validator for backward compatibility
	}
}

// NewServiceWithValidator creates a new tool correction service with custom validator
func NewServiceWithValidator(endpoint, apiKey string, enabled bool, modelName string, disableLogging bool, validator types.ToolValidator) *Service {
	return &Service{
		endpoint:       endpoint,
		apiKey:         apiKey,
		enabled:        enabled,
		modelName:      modelName,
		disableLogging: disableLogging,
		validator:      validator,
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

		// Circuit breaker: Initialize retry tracking for this tool call
		const maxRetries = 3
		retryCount := 0
		var currentCall = call

		for retryCount <= maxRetries {
			// Stage 0: Comprehensive validation
			validation := s.ValidateToolCall(ctx, currentCall, availableTools)
			
			// Check for structural mismatches that OpenAI validation misses
			needsStructuralCorrection := false
			if validation.IsValid {
				needsStructuralCorrection = s.HasStructuralMismatch(currentCall, availableTools)
			}
			
			// If already valid and doesn't need structural correction, keep as-is
			if validation.IsValid && !validation.HasCaseIssue && !validation.HasToolNameIssue && !needsStructuralCorrection {
				if s.shouldLog() {
					if retryCount > 0 {
						log.Printf("✅[%s] Tool call corrected after %d retries: %s", requestID, retryCount, currentCall.Name)
					} else {
						log.Printf("✅[%s] Tool call valid: %s", requestID, currentCall.Name)
					}
				}
				correctedCalls = append(correctedCalls, currentCall)
				break // Exit retry loop
			}

			// Circuit breaker: Check if we've exceeded max retries
			if retryCount >= maxRetries {
				if s.shouldLog() {
					log.Printf("❌[%s] Circuit breaker activated: %s exceeded %d correction attempts", 
						requestID, currentCall.Name, maxRetries)
					log.Printf("🔧[%s] Final attempt had missing: %v, invalid: %v", 
						requestID, validation.MissingParams, validation.InvalidParams)
				}
				correctedCalls = append(correctedCalls, call) // Use original call
				break // Exit retry loop
			}

			if s.shouldLog() && retryCount > 0 {
				log.Printf("🔄[%s] Retry attempt %d/%d for tool: %s", requestID, retryCount, maxRetries, currentCall.Name)
			}
		
		// Stage 1: Fix tool name issues (direct correction, no LLM)
		if validation.HasCaseIssue || validation.HasToolNameIssue {
			if validation.HasCaseIssue {
				if s.shouldLog() {
					log.Printf("🔧[%s] Tool case correction: %s -> %s", requestID, currentCall.Name, validation.CorrectToolName)
				}
				currentCall = s.correctToolName(ctx, currentCall, validation.CorrectToolName)
			} else if validation.HasToolNameIssue {
				// Check if this is a semantic issue that needs rule-based correction
				if correctedCall, success := s.CorrectSemanticIssue(ctx, currentCall, availableTools); success {
					if s.shouldLog() {
						log.Printf("🔧[%s] Semantic correction: %s -> %s (architectural fix)", requestID, currentCall.Name, correctedCall.Name)
					}
					currentCall = correctedCall
				} else {
					if s.shouldLog() {
						log.Printf("🔧[%s] Tool name correction: %s -> %s", requestID, currentCall.Name, validation.CorrectToolName)
					}
					// Apply both tool name and input corrections for slash commands
					currentCall = s.correctToolNameAndInput(ctx, currentCall, validation.CorrectToolName, validation.CorrectedInput)
				}
			}
			
			// Re-validate after name correction
			validation = s.ValidateToolCall(ctx, currentCall, availableTools)
			if validation.IsValid {
				if s.shouldLog() {
					log.Printf("✅[%s] Tool name correction successful", requestID)
				}
				correctedCalls = append(correctedCalls, currentCall)
				break // Exit retry loop - correction successful
			}
			
			// If still invalid after name correction, continue with retry
			retryCount++
			continue
		}

		// Stage 1.5: Try rule-based parameter corrections before LLM
		if ruleBasedCall, success := s.AttemptRuleBasedParameterCorrection(ctx, currentCall); success {
			if s.shouldLog() {
				log.Printf("🔧[%s] Rule-based parameter correction successful", requestID)
			}
			
			// Re-validate rule-based correction
			ruleValidation := s.ValidateToolCall(ctx, ruleBasedCall, availableTools)
			if ruleValidation.IsValid {
				if s.shouldLog() {
					log.Printf("✅[%s] Rule-based parameter correction passed validation", requestID)
				}
				correctedCalls = append(correctedCalls, ruleBasedCall)
				break // Exit retry loop - success
			} else {
				if s.shouldLog() {
					log.Printf("⚠️[%s] Rule-based correction failed validation, continuing with LLM", requestID)
				}
				// Update currentCall to the rule-based attempt for potential LLM correction
				currentCall = ruleBasedCall
				validation = ruleValidation
			}
		}

		// Stage 1.6: Try rule-based TodoWrite correction before LLM
		if currentCall.Name == "TodoWrite" {
			if ruleBasedCall, success := s.AttemptRuleBasedTodoWriteCorrection(ctx, currentCall); success {
				if s.shouldLog() {
					log.Printf("🔧[%s] Rule-based TodoWrite correction successful", requestID)
					log.Printf("🔧[%s] Rule-based corrected: %+v", requestID, ruleBasedCall.Input)
				}
				
				// Re-validate rule-based correction
				ruleValidation := s.ValidateToolCall(ctx, ruleBasedCall, availableTools)
				if ruleValidation.IsValid {
					if s.shouldLog() {
						log.Printf("✅[%s] Rule-based TodoWrite correction passed validation", requestID)
					}
					correctedCalls = append(correctedCalls, ruleBasedCall)
					break // Exit retry loop - success
				} else {
					if s.shouldLog() {
						log.Printf("⚠️[%s] Rule-based correction failed validation, falling back to LLM", requestID)
					}
					// Update currentCall to the rule-based attempt for LLM correction
					currentCall = ruleBasedCall
					validation = ruleValidation
				}
			}
		}

		// Stage 2: Fix parameter issues (LLM correction)
		if len(validation.MissingParams) > 0 || len(validation.InvalidParams) > 0 {
			if s.shouldLog() {
				log.Printf("🔧[%s] Correcting parameters with LLM", requestID)
				log.Printf("🔍[%s] Original tool call: name=%s, input=%v", requestID, currentCall.Name, currentCall.Input)
				log.Printf("🔍[%s] Missing params: %v, Invalid params: %v", requestID, validation.MissingParams, validation.InvalidParams)
			}
			correctedCall, err := s.correctToolCall(ctx, currentCall, availableTools)
			if err != nil {
				if s.shouldLog() {
					log.Printf("❌[%s] Parameter correction failed: %v", requestID, err)
					log.Printf("🔍[%s] Retrying with current call", requestID)
				}
				retryCount++
				continue // Retry with current call
			} else {
				if s.shouldLog() {
					log.Printf("🔍[%s] Corrected tool call: name=%s, input=%v", requestID, correctedCall.Name, correctedCall.Input)
					// Re-validate corrected call to verify it's actually fixed
					revalidation := s.ValidateToolCall(ctx, correctedCall, availableTools)
					if !revalidation.IsValid {
						log.Printf("⚠️[%s] Correction FAILED validation - still missing: %v, invalid: %v", 
							requestID, revalidation.MissingParams, revalidation.InvalidParams)
						log.Printf("🔄[%s] Will retry correction", requestID)
					} else {
						log.Printf("✅[%s] Correction passed validation", requestID)
					}
					// Log detailed parameter changes
					s.logParameterChanges(requestID, currentCall, correctedCall)
				}
				
				// Check if correction was successful
				revalidation := s.ValidateToolCall(ctx, correctedCall, availableTools)
				if revalidation.IsValid {
					correctedCalls = append(correctedCalls, correctedCall)
					break // Exit retry loop - success
				} else {
					// Correction failed, update for retry
					currentCall = correctedCall
					validation = revalidation
					retryCount++
					continue
				}
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
				retryCount++
				continue // Retry
			} else {
				if s.shouldLog() {
					// Log detailed parameter changes
					s.logParameterChanges(requestID, currentCall, correctedCall)
				}
				
				// Check if correction was successful
				revalidation := s.ValidateToolCall(ctx, correctedCall, availableTools)
				if revalidation.IsValid {
					correctedCalls = append(correctedCalls, correctedCall)
					break // Exit retry loop - success
				} else {
					// Correction failed, update for retry
					currentCall = correctedCall
					validation = revalidation
					retryCount++
					continue
				}
			}
		}
		} // End retry loop
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
// Deprecated: Use validator.NormalizeToolName() instead
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
	IsValid           bool
	HasCaseIssue      bool
	HasToolNameIssue  bool                   // New: indicates tool name was corrected (e.g., slash command)
	CorrectToolName   string
	MissingParams     []string
	InvalidParams     []string
	CorrectedInput    map[string]interface{} // New: corrected input parameters
}

// ValidateToolCall performs comprehensive tool call validation using injected ToolValidator
// Made public for testing slash command correction functionality
func (s *Service) ValidateToolCall(ctx context.Context, call types.Content, availableTools []types.Tool) ValidationResult {
	requestID := getRequestID(ctx)
	result := ValidationResult{IsValid: false}

	// Enhanced logging: Log validation start
	if s.shouldLog() {
		log.Printf("🔍[%s] Validating tool call: %s with %d parameters", requestID, call.Name, len(call.Input))
	}

	// Try exact match first
	tool := s.findToolByName(call.Name, availableTools)
	if tool == nil {
		// Try case-insensitive match using validator
		if normalizedName, found := s.validator.NormalizeToolName(call.Name); found {
			tool = s.findToolByName(normalizedName, availableTools)
			if tool != nil {
				result.HasCaseIssue = true
				result.CorrectToolName = tool.Name
				if s.shouldLog() {
					log.Printf("⚠️[%s] Tool name case issue: '%s' should be '%s'", requestID, call.Name, tool.Name)
				}
			}
		}
		
		if tool == nil {
			// Check if this is a slash command that should be converted to Task
			if s.isSlashCommand(call.Name) {
				return s.correctSlashCommandToTask(ctx, call, availableTools)
			}
			
			// Try fallback schema for Claude Code built-in tools
			if fallbackTool := types.GetFallbackToolSchema(call.Name); fallbackTool != nil {
				tool = fallbackTool
				if s.shouldLog() {
					log.Printf("🔧[%s] Using fallback schema for Claude Code tool: %s", requestID, call.Name)
				}
			} else {
				if s.shouldLog() {
					log.Printf("❌[%s] Unknown tool: %s", requestID, call.Name)
				}
				return result
			}
		}
	}

	// Enhanced logging: Log tool schema details for TodoWrite
	if s.shouldLog() && call.Name == "TodoWrite" {
		log.Printf("🔍[%s] TodoWrite validation - Required params: %v", requestID, tool.InputSchema.Required)
		log.Printf("🔍[%s] TodoWrite validation - Available properties: %v", requestID, getPropertyNames(tool.InputSchema.Properties))
		log.Printf("🔍[%s] TodoWrite validation - Input params: %v", requestID, getInputParamNames(call.Input))
	}

	// Use injected validator for parameter validation
	validatorResult := s.validator.ValidateParameters(ctx, call, tool.InputSchema)
	
	// Copy validator results to correction service result format
	result.MissingParams = validatorResult.MissingParams
	result.InvalidParams = validatorResult.InvalidParams
	
	// For Task tools, allow additional parameters (they may come from slash commands)
	if tool.Name == "Task" && len(result.InvalidParams) > 0 {
		var filteredInvalid []string
		for _, invalid := range result.InvalidParams {
			if s.shouldLog() {
				log.Printf("🔧[%s] Allowing additional parameter for Task: %s", requestID, invalid)
			}
			// Don't add to filteredInvalid - we're allowing it
		}
		result.InvalidParams = filteredInvalid
	}

	// Enhanced logging: Detailed validation results
	if len(result.MissingParams) > 0 && s.shouldLog() {
		log.Printf("❌[%s] Missing required parameters in %s: %v", requestID, call.Name, result.MissingParams)
		if call.Name == "TodoWrite" {
			log.Printf("🔍[%s] TodoWrite expects 'todos' array but received: %v", requestID, getInputParamNames(call.Input))
		}
	}
	if len(result.InvalidParams) > 0 && s.shouldLog() {
		log.Printf("❌[%s] Invalid parameters in %s: %v", requestID, call.Name, result.InvalidParams)
		if call.Name == "TodoWrite" {
			for _, invalid := range result.InvalidParams {
				if value, exists := call.Input[invalid]; exists {
					log.Printf("🔍[%s] TodoWrite invalid param '%s' = %v (type: %T)", requestID, invalid, value, value)
				}
			}
		}
	}

	// Semantic validation: Check for common tool misuse patterns
	if validatorResult.IsValid && s.DetectSemanticIssue(ctx, call) {
		// Mark as having a tool name issue to trigger correction
		result.HasToolNameIssue = true
		result.IsValid = false
		if correctTool := s.suggestCorrectTool(ctx, call, availableTools); correctTool != "" {
			result.CorrectToolName = correctTool
			if s.shouldLog() {
				log.Printf("🔧[%s] Semantic tool issue detected: %s should use %s", requestID, call.Name, correctTool)
			}
		}
	}

	// Valid if no issues found (or only case issue which we can fix easily)
	result.IsValid = len(result.MissingParams) == 0 && len(result.InvalidParams) == 0 && !result.HasToolNameIssue
	
	// Enhanced logging: Log final validation result
	if s.shouldLog() {
		if result.IsValid {
			log.Printf("✅[%s] Tool call validation passed: %s", requestID, call.Name)
		} else {
			log.Printf("❌[%s] Tool call validation failed: %s (missing: %v, invalid: %v)", 
				requestID, call.Name, result.MissingParams, result.InvalidParams)
		}
	}
	
	return result
}

// isValidToolCall checks if a tool call matches available tool schemas (backward compatibility)
func (s *Service) isValidToolCall(ctx context.Context, call types.Content, availableTools []types.Tool) bool {
	result := s.ValidateToolCall(ctx, call, availableTools)
	return result.IsValid && !result.HasCaseIssue
}

// correctToolName fixes tool name case issues directly without LLM
func (s *Service) correctToolName(ctx context.Context, call types.Content, correctName string) types.Content {
	// Create corrected call with proper tool name
	correctedCall := call
	correctedCall.Name = correctName
	
	return correctedCall
}

// correctToolNameAndInput fixes both tool name and input parameters (for slash commands)
func (s *Service) correctToolNameAndInput(ctx context.Context, call types.Content, correctName string, correctedInput map[string]interface{}) types.Content {
	// Create corrected call with proper tool name and input
	correctedCall := call
	correctedCall.Name = correctName
	correctedCall.Input = correctedInput
	
	return correctedCall
}

// correctToolCall uses qwen2.5-coder to fix invalid tool calls
func (s *Service) correctToolCall(ctx context.Context, call types.Content, availableTools []types.Tool) (types.Content, error) {
	requestID := getRequestID(ctx)
	
	// Enhanced logging: Log original call details
	if s.shouldLog() {
		log.Printf("🔧[%s] Starting LLM correction for tool: %s", requestID, call.Name)
		log.Printf("🔧[%s] Original parameters: %+v", requestID, call.Input)
		if call.Name == "TodoWrite" {
			log.Printf("🔧[%s] TodoWrite correction attempt - analyzing input structure", requestID)
		}
	}
	
	// Build correction prompt
	prompt := s.buildCorrectionPrompt(call, availableTools)
	
	// Enhanced logging: Log prompt details (truncated for security)
	if s.shouldLog() {
		truncatedPrompt := prompt
		if len(prompt) > 300 {
			truncatedPrompt = prompt[:300] + "... [truncated]"
		}
		log.Printf("🔧[%s] Correction prompt (first 300 chars): %s", requestID, truncatedPrompt)
	}

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

	// Enhanced logging: Log LLM request details
	if s.shouldLog() {
		log.Printf("🔧[%s] Sending correction request to model: %s", requestID, s.modelName)
	}

	// Send request
	response, err := s.sendCorrectionRequest(req)
	if err != nil {
		if s.shouldLog() {
			log.Printf("❌[%s] LLM correction request failed: %v", requestID, err)
		}
		return call, fmt.Errorf("[%s] correction request failed: %v", requestID, err)
	}

	// Enhanced logging: Log raw LLM response
	if s.shouldLog() && len(response.Choices) > 0 {
		log.Printf("🔧[%s] LLM correction response received (length: %d chars)", requestID, len(response.Choices[0].Message.Content))
		log.Printf("🔍[%s] Raw correction response: %s", requestID, response.Choices[0].Message.Content)
	}

	// Parse corrected tool call
	correctedCall, err := s.parseCorrectedResponse(response, call)
	if err != nil {
		if s.shouldLog() {
			log.Printf("❌[%s] Failed to parse LLM correction response", requestID)
			log.Printf("🔍[%s] Parse error details: %v", requestID, err)
			if len(response.Choices) > 0 {
				log.Printf("🔍[%s] Failed to parse response: %s", requestID, response.Choices[0].Message.Content)
			}
		}
		return call, fmt.Errorf("[%s] failed to parse correction: %v", requestID, err)
	}

	// Enhanced logging: Log successful correction details
	if s.shouldLog() {
		log.Printf("✅[%s] LLM correction successful - tool: %s", requestID, correctedCall.Name)
		log.Printf("🔧[%s] Corrected parameters: %+v", requestID, correctedCall.Input)
		
		// Special logging for TodoWrite corrections
		if call.Name == "TodoWrite" {
			if todos, exists := correctedCall.Input["todos"]; exists {
				if todosArray, ok := todos.([]interface{}); ok {
					log.Printf("🔧[%s] TodoWrite correction produced %d todo items", requestID, len(todosArray))
				} else {
					log.Printf("⚠️[%s] TodoWrite correction: todos is not an array (type: %T)", requestID, todos)
				}
			} else {
				log.Printf("⚠️[%s] TodoWrite correction: missing 'todos' parameter", requestID)
			}
		}
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

	// Enhanced TodoWrite examples and instructions
	todoExample := ""
	callStr := strings.ToLower(string(callJson))
	if strings.Contains(callStr, "todo") || strings.Contains(strings.ToLower(call.Name), "todo") {
		todoExample = `

TODOWRITE TRANSFORMATION EXAMPLES:

EXAMPLE 1 - Single todo string:
INCORRECT: {"name": "TodoWrite", "input": {"todo": "Review code"}}
CORRECT: {"name": "TodoWrite", "input": {"todos": [{"content": "Review code", "status": "pending", "priority": "medium", "id": "review-code"}]}}

EXAMPLE 2 - Missing parameters:
INCORRECT: {"name": "TodoWrite", "input": {"task": "Fix bug", "priority": "high"}}
CORRECT: {"name": "TodoWrite", "input": {"todos": [{"content": "Fix bug", "status": "pending", "priority": "high", "id": "fix-bug"}]}}

EXAMPLE 3 - Multiple items:
INCORRECT: {"name": "TodoWrite", "input": {"items": ["Task 1", "Task 2"]}}
CORRECT: {"name": "TodoWrite", "input": {"todos": [{"content": "Task 1", "status": "pending", "priority": "medium", "id": "task-1"}, {"content": "Task 2", "status": "pending", "priority": "medium", "id": "task-2"}]}}

EXAMPLE 4 - No parameters:
INCORRECT: {"name": "TodoWrite", "input": {}}
CORRECT: {"name": "TodoWrite", "input": {"todos": [{"content": "New task", "status": "pending", "priority": "medium", "id": "new-task"}]}}

CRITICAL TODOWRITE RULES:
- ALWAYS use 'todos' parameter (array), never 'todo', 'task', 'items', etc.
- Each todo object MUST have exactly these 4 fields: content, status, priority, id
- content: string (preserve original semantic meaning)
- status: must be "pending", "in_progress", or "completed" (default: "pending")
- priority: must be "high", "medium", or "low" (default: "medium")
- id: string (generate from content: lowercase, replace spaces with hyphens)
- If no meaningful content exists, use "New task" as content
- Always preserve the user's original intent and information`
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

	// Enhanced JSON extraction with multiple fallback strategies
	var jsonStr string

	// Strategy 1: Extract JSON from response (may have Markdown code blocks)
	jsonStart := strings.Index(content, "{")
	jsonEnd := strings.LastIndex(content, "}") + 1

	if jsonStart != -1 && jsonEnd > jsonStart {
		jsonStr = content[jsonStart:jsonEnd]
	} else {
		// Strategy 2: Look for JSON in code blocks
		codeBlockStart := strings.Index(content, "```json")
		if codeBlockStart != -1 {
			codeBlockStart += 7 // Skip "```json"
			codeBlockEnd := strings.Index(content[codeBlockStart:], "```")
			if codeBlockEnd != -1 {
				jsonStr = strings.TrimSpace(content[codeBlockStart : codeBlockStart+codeBlockEnd])
			}
		} else {
			// Strategy 3: Look for any code block
			codeBlockStart = strings.Index(content, "```")
			if codeBlockStart != -1 {
				codeBlockStart += 3
				codeBlockEnd := strings.Index(content[codeBlockStart:], "```")
				if codeBlockEnd != -1 {
					possibleJson := strings.TrimSpace(content[codeBlockStart : codeBlockStart+codeBlockEnd])
					if strings.HasPrefix(possibleJson, "{") && strings.HasSuffix(possibleJson, "}") {
						jsonStr = possibleJson
					}
				}
			}
		}
	}

	if jsonStr == "" {
		return originalCall, fmt.Errorf("no valid JSON found in correction response")
	}

	// Parse corrected tool call with enhanced error handling
	var corrected struct {
		Name  string                 `json:"name"`
		Input map[string]interface{} `json:"input"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &corrected); err != nil {
		return originalCall, fmt.Errorf("failed to parse corrected JSON: %v (JSON: %s)", err, jsonStr)
	}

	// Enhanced validation for TodoWrite
	if corrected.Name == "TodoWrite" {
		if err := s.validateTodoWriteCorrection(corrected.Input); err != nil {
			return originalCall, fmt.Errorf("TodoWrite correction validation failed: %v", err)
		}
	}

	// Create corrected content
	return types.Content{
		Type:  "tool_use",
		ID:    originalCall.ID, // Preserve original ID
		Name:  corrected.Name,
		Input: corrected.Input,
	}, nil
}

// validateTodoWriteCorrection validates that a TodoWrite correction has proper structure
func (s *Service) validateTodoWriteCorrection(input map[string]interface{}) error {
	// Check for todos parameter
	todos, exists := input["todos"]
	if !exists {
		return fmt.Errorf("missing 'todos' parameter")
	}

	// Check that todos is an array
	todosArray, ok := todos.([]interface{})
	if !ok {
		return fmt.Errorf("'todos' must be an array, got %T", todos)
	}

	if len(todosArray) == 0 {
		return fmt.Errorf("'todos' array cannot be empty")
	}

	// Validate each todo item structure
	for i, todoItem := range todosArray {
		todoMap, ok := todoItem.(map[string]interface{})
		if !ok {
			return fmt.Errorf("todo item %d must be an object, got %T", i, todoItem)
		}

		// Check required fields
		requiredFields := []string{"content", "status", "priority", "id"}
		for _, field := range requiredFields {
			if _, exists := todoMap[field]; !exists {
				return fmt.Errorf("todo item %d missing required field: %s", i, field)
			}
		}

		// Validate field types
		if content, ok := todoMap["content"].(string); !ok || content == "" {
			return fmt.Errorf("todo item %d 'content' must be a non-empty string", i)
		}

		if status, ok := todoMap["status"].(string); !ok || !isValidStatus(status) {
			return fmt.Errorf("todo item %d 'status' must be 'pending', 'in_progress', or 'completed'", i)
		}

		if priority, ok := todoMap["priority"].(string); !ok || !isValidPriority(priority) {
			return fmt.Errorf("todo item %d 'priority' must be 'high', 'medium', or 'low'", i)
		}

		if id, ok := todoMap["id"].(string); !ok || id == "" {
			return fmt.Errorf("todo item %d 'id' must be a non-empty string", i)
		}
	}

	return nil
}

// isValidStatus checks if a status value is valid
func isValidStatus(status string) bool {
	validStatuses := []string{"pending", "in_progress", "completed"}
	for _, valid := range validStatuses {
		if status == valid {
			return true
		}
	}
	return false
}

// isValidPriority checks if a priority value is valid
func isValidPriority(priority string) bool {
	validPriorities := []string{"high", "medium", "low"}
	for _, valid := range validPriorities {
		if priority == valid {
			return true
		}
	}
	return false
}

// AttemptRuleBasedTodoWriteCorrection tries to fix TodoWrite calls using predefined rules
func (s *Service) AttemptRuleBasedTodoWriteCorrection(ctx context.Context, call types.Content) (types.Content, bool) {
	requestID := getRequestID(ctx)
	
	if call.Name != "TodoWrite" || call.Type != "tool_use" {
		return call, false
	}

	if s.shouldLog() {
		log.Printf("🔧[%s] Attempting rule-based TodoWrite correction", requestID)
		log.Printf("🔧[%s] Input parameters: %+v", requestID, call.Input)
	}

	correctedInput := make(map[string]interface{})
	var todos []interface{}

	// Check if already has valid todos parameter - but validate structure
	if existingTodos, exists := call.Input["todos"]; exists {
		if todosArray, ok := existingTodos.([]interface{}); ok && len(todosArray) > 0 {
			// Validate that each todo item has correct structure
			hasValidStructure := true
			for _, todoItem := range todosArray {
				if todoMap, ok := todoItem.(map[string]interface{}); ok {
					// Check if it has the required fields with correct names
					if _, hasContent := todoMap["content"]; !hasContent {
						hasValidStructure = false
						break
					}
					if _, hasPriority := todoMap["priority"]; !hasPriority {
						hasValidStructure = false
						break
					}
					// Allow status and id to be missing (they have defaults) but check names are correct
				} else {
					hasValidStructure = false
					break
				}
			}
			
			if hasValidStructure {
				if s.shouldLog() {
					log.Printf("🔧[%s] Already has valid todos array with correct structure, no correction needed", requestID)
				}
				return call, false
			} else {
				if s.shouldLog() {
					log.Printf("🔧[%s] Found todos array but with invalid structure, attempting correction", requestID)
				}
				// Process malformed todos array immediately
				for _, todoItem := range todosArray {
					if todoMap, ok := todoItem.(map[string]interface{}); ok {
						// Extract content from various possible field names
						content := ""
						if desc, exists := todoMap["description"]; exists {
							if descStr, ok := desc.(string); ok {
								content = descStr
								if s.shouldLog() {
									log.Printf("🔧[%s] Transformed 'description' → 'content': %s", requestID, descStr)
								}
							}
						}
						if task, exists := todoMap["task"]; exists {
							if taskStr, ok := task.(string); ok {
								content = taskStr
								if s.shouldLog() {
									log.Printf("🔧[%s] Transformed 'task' → 'content': %s", requestID, taskStr)
								}
							}
						}
						if cont, exists := todoMap["content"]; exists {
							if contStr, ok := cont.(string); ok {
								content = contStr
							}
						}
						
						// Extract other fields with defaults
						status := "pending"
						if s, exists := todoMap["status"]; exists {
							if sStr, ok := s.(string); ok && isValidStatus(sStr) {
								status = sStr
							}
						}
						
						priority := "medium"
						if p, exists := todoMap["priority"]; exists {
							if pStr, ok := p.(string); ok && isValidPriority(pStr) {
								priority = pStr
							}
						} else {
							if s.shouldLog() {
								log.Printf("🔧[%s] Added missing 'priority': %s", requestID, priority)
							}
						}
						
						id := ""
						if i, exists := todoMap["id"]; exists {
							if iStr, ok := i.(string); ok {
								id = iStr
							}
						}
						if id == "" && content != "" {
							id = s.GenerateTodoID(content)
						}
						if id == "" {
							id = s.GenerateTodoID("todo")
						}
						
						if content != "" {
							correctedTodo := map[string]interface{}{
								"content":  content,
								"status":   status,
								"priority": priority,
								"id":       id,
							}
							todos = append(todos, correctedTodo)
						}
					}
				}
				
				if len(todos) > 0 {
					if s.shouldLog() {
						log.Printf("🔧[%s] Rule 0: Corrected malformed todos array (%d items)", requestID, len(todos))
					}
					correctedInput["todos"] = todos
					
					// Validate the correction
					if err := s.validateTodoWriteCorrection(correctedInput); err == nil {
						if s.shouldLog() {
							log.Printf("✅[%s] Malformed todos correction passed validation", requestID)
						}
						return types.Content{
							Type:  "tool_use",
							ID:    call.ID,
							Name:  "TodoWrite",
							Input: correctedInput,
						}, true
					} else {
						if s.shouldLog() {
							log.Printf("❌[%s] Malformed todos correction failed validation: %v", requestID, err)
						}
					}
				}
				// If correction failed, continue with other rules
			}
		}
	}

	// Rule 1: Handle single 'todo' string parameter
	if todo, exists := call.Input["todo"]; exists {
		if todoStr, ok := todo.(string); ok && todoStr != "" {
			todoItem := map[string]interface{}{
				"content":  todoStr,
				"status":   "pending",
				"priority": "medium",
				"id":       s.GenerateTodoID(todoStr),
			}
			todos = append(todos, todoItem)
			if s.shouldLog() {
				log.Printf("🔧[%s] Rule 1: Converted 'todo' string to todos array", requestID)
			}
		}
	}

	// Rule 2: Handle 'task' parameter
	if task, exists := call.Input["task"]; exists {
		if taskStr, ok := task.(string); ok && taskStr != "" {
			priority := "medium"
			if p, exists := call.Input["priority"]; exists {
				if pStr, ok := p.(string); ok && isValidPriority(pStr) {
					priority = pStr
				}
			}
			
			todoItem := map[string]interface{}{
				"content":  taskStr,
				"status":   "pending",
				"priority": priority,
				"id":       s.GenerateTodoID(taskStr),
			}
			todos = append(todos, todoItem)
			if s.shouldLog() {
				log.Printf("🔧[%s] Rule 2: Converted 'task' to todos array", requestID)
			}
		}
	}

	// Rule 3: Handle 'items' array
	if items, exists := call.Input["items"]; exists {
		if itemsArray, ok := items.([]interface{}); ok {
			for _, item := range itemsArray {
				if itemStr, ok := item.(string); ok && itemStr != "" {
					todoItem := map[string]interface{}{
						"content":  itemStr,
						"status":   "pending",
						"priority": "medium",
						"id":       s.GenerateTodoID(itemStr),
					}
					todos = append(todos, todoItem)
				}
			}
			if len(todos) > 0 && s.shouldLog() {
				log.Printf("🔧[%s] Rule 3: Converted 'items' array to todos array (%d items)", requestID, len(todos))
			}
		}
	}

	// Rule 4: Handle multiple individual parameters (content, description, etc.)
	if len(todos) == 0 {
		content := ""
		priority := "medium"
		status := "pending"

		// Look for content in various parameter names
		for _, paramName := range []string{"content", "description", "text", "message", "title"} {
			if val, exists := call.Input[paramName]; exists {
				if str, ok := val.(string); ok && str != "" {
					content = str
					break
				}
			}
		}

		// Check for explicit priority and status
		if p, exists := call.Input["priority"]; exists {
			if pStr, ok := p.(string); ok && isValidPriority(pStr) {
				priority = pStr
			}
		}
		if s, exists := call.Input["status"]; exists {
			if sStr, ok := s.(string); ok && isValidStatus(sStr) {
				status = sStr
			}
		}

		if content != "" {
			todoItem := map[string]interface{}{
				"content":  content,
				"status":   status,
				"priority": priority,
				"id":       s.GenerateTodoID(content),
			}
			todos = append(todos, todoItem)
			if s.shouldLog() {
				log.Printf("🔧[%s] Rule 4: Created todo from found content: %s", requestID, content)
			}
		}
	}

	// Rule 5: Handle empty input - create default todo
	if len(todos) == 0 {
		todoItem := map[string]interface{}{
			"content":  "New task",
			"status":   "pending",
			"priority": "medium",
			"id":       "new-task",
		}
		todos = append(todos, todoItem)
		if s.shouldLog() {
			log.Printf("🔧[%s] Rule 5: Created default todo for empty input", requestID)
		}
	}

	// If we successfully created todos, build the corrected call
	if len(todos) > 0 {
		correctedInput["todos"] = todos
		
		correctedCall := types.Content{
			Type:  call.Type,
			ID:    call.ID,
			Name:  call.Name,
			Input: correctedInput,
		}

		if s.shouldLog() {
			log.Printf("✅[%s] Rule-based correction successful: created %d todo items", requestID, len(todos))
		}

		return correctedCall, true
	}

	if s.shouldLog() {
		log.Printf("⚠️[%s] Rule-based correction failed: no valid todos could be generated", requestID)
	}
	return call, false
}

// GenerateTodoID creates a valid ID from todo content
func (s *Service) GenerateTodoID(content string) string {
	// Convert to lowercase and replace problematic characters
	id := strings.ToLower(content)
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, "_", "-")
	
	// Remove non-alphanumeric characters except hyphens
	var result strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	
	id = result.String()
	
	// Clean up multiple consecutive hyphens
	for strings.Contains(id, "--") {
		id = strings.ReplaceAll(id, "--", "-")
	}
	
	// Trim leading/trailing hyphens
	id = strings.Trim(id, "-")
	
	// Ensure ID is not empty and not too long
	if id == "" {
		id = "task"
	}
	if len(id) > 50 {
		id = id[:50]
		id = strings.Trim(id, "-")
	}
	
	return id
}

// isSlashCommand detects if a tool name is a slash command that should be converted to Task
func (s *Service) isSlashCommand(toolName string) bool {
	return strings.HasPrefix(toolName, "/")
}

// correctSlashCommandToTask converts a slash command to a proper Task tool call
func (s *Service) correctSlashCommandToTask(ctx context.Context, call types.Content, availableTools []types.Tool) ValidationResult {
	requestID := getRequestID(ctx)
	
	// Ensure this is a tool_use call
	if call.Type != "tool_use" {
		return ValidationResult{IsValid: false}
	}
	
	// Find Task tool in available tools
	taskTool := s.findToolByName("Task", availableTools)
	if taskTool == nil {
		if s.shouldLog() {
			log.Printf("⚠️[%s] Cannot correct slash command '%s' - Task tool not available", requestID, call.Name)
		}
		return ValidationResult{IsValid: false}
	}
	
	// Generate description from slash command
	description := s.generateDescriptionFromSlashCommand(call.Name)
	
	// Build corrected input parameters
	correctedInput := make(map[string]interface{})
	correctedInput["description"] = description
	correctedInput["prompt"] = call.Name
	
	// Preserve all existing parameters for slash commands
	// Claude Code expects additional parameters like subagent_type to be passed through
	for key, value := range call.Input {
		if key != "description" && key != "prompt" {
			correctedInput[key] = value
			if s.shouldLog() {
				// Check if this parameter exists in Task tool schema for logging purposes
				if _, exists := taskTool.InputSchema.Properties[key]; !exists {
					log.Printf("🔧[%s] Preserving additional parameter for Task: %s", requestID, key)
				}
			}
		}
	}
	
	if s.shouldLog() {
		log.Printf("🔧[%s] Corrected slash command '%s' to Task tool call", requestID, call.Name)
		log.Printf("🔧[%s] Generated description: '%s'", requestID, description)
	}
	
	return ValidationResult{
		IsValid:           true,
		HasToolNameIssue:  true,
		CorrectToolName:   "Task",
		CorrectedInput:    correctedInput,
	}
}

// generateDescriptionFromSlashCommand creates a human-readable description from a slash command
func (s *Service) generateDescriptionFromSlashCommand(command string) string {
	// Remove leading slash
	if len(command) <= 1 {
		// Handle edge case of just "/"
		return strings.Title(strings.TrimPrefix(command, "/"))
	}
	
	// Handle commands like "/code-reviewer" -> "Code Reviewer"
	name := strings.TrimPrefix(command, "/")
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	
	// Capitalize first letter of each word
	words := strings.Fields(name)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	
	return strings.Join(words, " ")
}

// DetectSemanticIssue checks for common tool misuse patterns that violate architecture
// Following SPARC: Simple, focused detection logic for architectural issues
func (s *Service) DetectSemanticIssue(ctx context.Context, call types.Content) bool {
	// Check WebFetch misuse for local files - architectural violation
	if call.Name == "WebFetch" || call.Name == "Fetch" {
		if url, exists := call.Input["url"]; exists {
			if urlStr, ok := url.(string); ok {
				// Detect file:// URLs that should use Read tool instead
				if strings.HasPrefix(urlStr, "file://") {
					return true
				}
			}
		}
	}
	
	// Add more semantic checks here as needed
	// Example: Edit tool with file paths that should use Write
	// Example: Bash commands that should use specific tools
	
	return false
}

// suggestCorrectTool recommends the appropriate tool based on semantic analysis
// Following SPARC: Focused tool suggestion logic
func (s *Service) suggestCorrectTool(ctx context.Context, call types.Content, availableTools []types.Tool) string {
	// Handle WebFetch -> Read conversion for local files
	if call.Name == "WebFetch" || call.Name == "Fetch" {
		if url, exists := call.Input["url"]; exists {
			if urlStr, ok := url.(string); ok && strings.HasPrefix(urlStr, "file://") {
				// Check if Read tool is available
				if s.findToolByName("Read", availableTools) != nil {
					return "Read"
				}
			}
		}
	}
	
	// Add more correction suggestions here
	// Return empty string if no correction is suggested
	return ""
}

// CorrectSemanticIssue performs rule-based correction for semantic tool misuse
// Following SPARC: Simple rule-based transformation without LLM calls
func (s *Service) CorrectSemanticIssue(ctx context.Context, call types.Content, availableTools []types.Tool) (types.Content, bool) {
	requestID := getRequestID(ctx)
	
	// Handle WebFetch -> Read conversion for local files
	if call.Name == "WebFetch" || call.Name == "Fetch" {
		if url, exists := call.Input["url"]; exists {
			if urlStr, ok := url.(string); ok && strings.HasPrefix(urlStr, "file://") {
				// Check if Read tool is available
				if s.findToolByName("Read", availableTools) != nil {
					// Extract file path from file:// URL
					filePath := strings.TrimPrefix(urlStr, "file://")
					
					// Create corrected tool call
					correctedCall := call
					correctedCall.Name = "Read"
					correctedCall.Input = map[string]interface{}{
						"file_path": filePath,
					}
					
					if s.shouldLog() {
						log.Printf("🔧[%s] ARCHITECTURE FIX: WebFetch(file://) -> Read(file_path)", requestID)
						log.Printf("   Original: %s(url='%s')", call.Name, urlStr)
						log.Printf("   Corrected: Read(file_path='%s')", filePath) 
						log.Printf("   Reason: Claude Code (client) and Simple Proxy (server) on different machines")
					}
					
					return correctedCall, true
				}
			}
		}
	}
	
	// Add more semantic corrections here as needed
	return call, false
}

// HasStructuralMismatch detects when a tool call has structural issues that OpenAI validation misses
// This is a generic approach that works for any tool with complex parameter structures
func (s *Service) HasStructuralMismatch(call types.Content, availableTools []types.Tool) bool {
	// Find the tool schema
	var toolSchema *types.Tool
	for _, tool := range availableTools {
		if tool.Name == call.Name {
			toolSchema = &tool
			break
		}
	}
	
	if toolSchema == nil {
		return false // Unknown tool - let normal validation handle it
	}
	
	// Special handling for TodoWrite - this is the only tool we know has structural issues
	// In the future, this could be expanded to other tools or made more generic
	if call.Name == "TodoWrite" {
		return s.checkTodoWriteStructure(call)
	}
	
	// For other tools, we could add generic structural checks here
	// For now, no structural validation for other tools
	return false
}

// AttemptRuleBasedParameterCorrection tries to fix common parameter name issues instantly
// without LLM calls for better performance. This handles the most common correction patterns.
func (s *Service) AttemptRuleBasedParameterCorrection(ctx context.Context, call types.Content) (types.Content, bool) {
	requestID := getRequestID(ctx)
	
	if call.Type != "tool_use" {
		return call, false
	}

	// Tool-specific parameter mappings based on frequent correction patterns
	// These are extracted from the actual LLM correction prompt patterns
	toolSpecificMappings := map[string]map[string]string{
		// File operations: path-related parameters become file_path
		"Read":      {"filename": "file_path", "path": "file_path"},
		"Write":     {"filename": "file_path", "path": "file_path", "text": "content"},
		"Edit":      {"filename": "file_path", "path": "file_path"},
		"MultiEdit": {"filename": "file_path", "path": "file_path", "filepath": "file_path"},
		
		// Search operations: query/search -> pattern, filter -> glob
		"Grep": {"search": "pattern", "query": "pattern", "filter": "glob"},
		"Glob": {"search": "pattern", "query": "pattern"},
		
		// WebSearch: Don't change query (it's correct for WebSearch)
		// Other tools: Add as needed
	}
	
	// Get mappings for this specific tool
	mappings, exists := toolSpecificMappings[call.Name]
	if !exists {
		// No specific mappings for this tool, return unchanged
		return call, false
	}

	// Create a copy of the input to avoid modifying the original
	correctedInput := make(map[string]interface{})
	for key, value := range call.Input {
		correctedInput[key] = value
	}
	
	// Apply parameter mappings for this specific tool
	changed := false
	for oldParam, newParam := range mappings {
		if value, exists := correctedInput[oldParam]; exists {
			// Only apply mapping if the new parameter doesn't already exist
			if _, hasNew := correctedInput[newParam]; !hasNew {
				delete(correctedInput, oldParam)
				correctedInput[newParam] = value
				changed = true
				
				if s.shouldLog() {
					log.Printf("🔧[%s] Rule-based parameter correction: %s.%s -> %s.%s", 
						requestID, call.Name, oldParam, call.Name, newParam)
				}
			}
		}
	}

	if !changed {
		return call, false
	}

	// Create corrected call
	correctedCall := types.Content{
		Type:  call.Type,
		ID:    call.ID,
		Name:  call.Name,
		Input: correctedInput,
	}

	if s.shouldLog() {
		log.Printf("✅[%s] Rule-based correction successful for %s: %d parameter(s) fixed", 
			requestID, call.Name, len(mappings))
	}

	return correctedCall, true
}

// checkTodoWriteStructure validates TodoWrite internal structure
func (s *Service) checkTodoWriteStructure(call types.Content) bool {
	if todos, exists := call.Input["todos"]; exists {
		if todosArray, ok := todos.([]interface{}); ok && len(todosArray) > 0 {
			// Check if any todo item has wrong field names or missing required fields
			for _, todoItem := range todosArray {
				if todoMap, ok := todoItem.(map[string]interface{}); ok {
					// Check for invalid field names that should be 'content'
					if _, hasDesc := todoMap["description"]; hasDesc {
						return true
					}
					if _, hasTask := todoMap["task"]; hasTask {
						return true
					}
					// Check for missing required fields
					if _, hasContent := todoMap["content"]; !hasContent {
						return true
					}
					if _, hasPriority := todoMap["priority"]; !hasPriority {
						return true
					}
				}
			}
		}
	}
	return false
}
