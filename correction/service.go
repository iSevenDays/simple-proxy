package correction

import (
	"bytes"
	"claude-proxy/internal"
	"claude-proxy/logger"
	"claude-proxy/types"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// ConfigProvider provides endpoint configuration for the correction service
type ConfigProvider interface {
	GetToolCorrectionEndpoint() string
	GetHealthyToolCorrectionEndpoint() string
	RecordEndpointFailure(endpoint string)
	RecordEndpointSuccess(endpoint string)
	// GetEnableToolChoiceCorrection returns whether tool choice correction is enabled
	GetEnableToolChoiceCorrection() bool
}

// Service handles tool call correction using configurable model
type Service struct {
	config                     ConfigProvider
	apiKey                     string
	enabled                    bool
	modelName                  string                      // Configurable model for corrections
	disableLogging             bool                        // Disable tool correction logging
	enableToolChoiceCorrection bool                        // Enable tool choice correction and necessity detection
	validator                  types.ToolValidator         // Injected tool validator
	registry                   types.SchemaRegistry        // Injected schema registry
	classifier                 *HybridClassifier           // Two-stage hybrid classifier for tool necessity
	obsLogger                  *logger.ObservabilityLogger // Structured logging
}

// logInfo logs an info message with structured data if obsLogger is available
func (s *Service) logInfo(component, category, requestID, message string, fields map[string]interface{}) {
	if s.obsLogger != nil {
		s.obsLogger.Info(component, category, requestID, message, fields)
	}
}

// logWarn logs a warning message with structured data if obsLogger is available
func (s *Service) logWarn(component, category, requestID, message string, fields map[string]interface{}) {
	if s.obsLogger != nil {
		s.obsLogger.Warn(component, category, requestID, message, fields)
	}
}

// logError logs an error message with structured data if obsLogger is available
func (s *Service) logError(component, category, requestID, message string, fields map[string]interface{}) {
	if s.obsLogger != nil {
		s.obsLogger.Error(component, category, requestID, message, fields)
	}
}

// NewService creates a new tool correction service with default components
func NewService(config ConfigProvider, apiKey string, enabled bool, modelName string, disableLogging bool, obsLogger *logger.ObservabilityLogger) *Service {
	return &Service{
		config:                     config,
		apiKey:                     apiKey,
		enabled:                    enabled,
		modelName:                  modelName,
		disableLogging:             disableLogging,
		enableToolChoiceCorrection: config.GetEnableToolChoiceCorrection(),
		validator:                  types.NewStandardToolValidator(),  // Default validator for backward compatibility
		registry:                   types.NewStandardSchemaRegistry(), // Default registry for backward compatibility
		classifier:                 NewHybridClassifier(),             // Two-stage hybrid classifier
		obsLogger:                  obsLogger,
	}
}

// NewServiceWithValidator creates a new tool correction service with custom validator
func NewServiceWithValidator(config ConfigProvider, apiKey string, enabled bool, modelName string, disableLogging bool, validator types.ToolValidator) *Service {
	return &Service{
		config:                     config,
		apiKey:                     apiKey,
		enabled:                    enabled,
		modelName:                  modelName,
		disableLogging:             disableLogging,
		enableToolChoiceCorrection: config.GetEnableToolChoiceCorrection(),
		validator:                  validator,
		registry:                   types.NewStandardSchemaRegistry(), // Default registry
		classifier:                 NewHybridClassifier(),             // Two-stage hybrid classifier
	}
}

// NewServiceWithComponents creates a new tool correction service with custom components
func NewServiceWithComponents(config ConfigProvider, apiKey string, enabled bool, modelName string, disableLogging bool, validator types.ToolValidator, registry types.SchemaRegistry) *Service {
	return &Service{
		config:                     config,
		apiKey:                     apiKey,
		enabled:                    enabled,
		modelName:                  modelName,
		disableLogging:             disableLogging,
		enableToolChoiceCorrection: config.GetEnableToolChoiceCorrection(),
		validator:                  validator,
		registry:                   registry,
		classifier:                 NewHybridClassifier(), // Two-stage hybrid classifier
	}
}

// shouldLog determines if logging should be enabled for tool correction
func (s *Service) shouldLog() bool {
	return !s.disableLogging
}

// boolToString converts boolean to string for logging
func boolToString(b bool) string {
	if b {
		return "required"
	}
	return "optional"
}

// logParameterChanges logs detailed information about what parameters were changed during correction
func (s *Service) logParameterChanges(requestID string, original, corrected types.Content) {
	// Show basic tool name change (if any)
	if original.Name != corrected.Name {
		s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Tool name correction", map[string]interface{}{
			"original_name":  original.Name,
			"corrected_name": corrected.Name,
		})
	}

	// Compare parameters and show changes
	originalParams := original.Input
	correctedParams := corrected.Input

	// Find added parameters
	for key, value := range correctedParams {
		if _, exists := originalParams[key]; !exists {
			s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Added parameter", map[string]interface{}{
				"parameter": key,
				"value":     value,
			})
		}
	}

	// Find removed parameters
	for key, value := range originalParams {
		if _, exists := correctedParams[key]; !exists {
			s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Removed parameter", map[string]interface{}{
				"parameter": key,
				"value":     value,
			})
		}
	}

	// Find changed parameters
	for key, newValue := range correctedParams {
		if oldValue, exists := originalParams[key]; exists {
			// Convert to strings for comparison to handle different types
			oldStr := fmt.Sprintf("%v", oldValue)
			newStr := fmt.Sprintf("%v", newValue)
			if oldStr != newStr {
				s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Changed parameter", map[string]interface{}{
					"parameter": key,
					"old_value": oldValue,
					"new_value": newValue,
				})
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
			s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Tool correction applied (structural fix)", map[string]interface{}{
				"corrected_name": corrected.Name,
			})
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

		// Memory management: Track original for potential reset
		originalCall := call

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
						s.logInfo(logger.ComponentToolCorrection, logger.CategorySuccess, requestID, "Tool call corrected after retries", map[string]interface{}{
							"retry_count": retryCount,
							"tool_name":   currentCall.Name,
						})
					} else {
						s.logInfo(logger.ComponentToolCorrection, logger.CategorySuccess, requestID, "Tool call valid", map[string]interface{}{
							"tool_name": currentCall.Name,
						})
					}
				}
				correctedCalls = append(correctedCalls, currentCall)
				break // Exit retry loop
			}

			// Circuit breaker: Check if we've exceeded max retries
			if retryCount >= maxRetries {
				s.logError(logger.ComponentToolCorrection, logger.CategoryError, requestID, "Circuit breaker activated - correction attempts exceeded", map[string]interface{}{
					"tool_name":      currentCall.Name,
					"max_retries":    maxRetries,
					"missing_params": validation.MissingParams,
					"invalid_params": validation.InvalidParams,
				})

				// Memory management: Reset to original and clear accumulated state
				currentCall = originalCall
				correctedCalls = append(correctedCalls, originalCall) // Use original call
				break                                                 // Exit retry loop
			}

			if s.shouldLog() && retryCount > 0 {
				s.logInfo(logger.ComponentToolCorrection, logger.CategoryRequest, requestID, "Retry attempt for tool correction", map[string]interface{}{
					"retry_count": retryCount,
					"max_retries": maxRetries,
					"tool_name":   currentCall.Name,
				})
			}

			// Stage 1: Fix tool name issues (direct correction, no LLM)
			if validation.HasCaseIssue || validation.HasToolNameIssue {
				if validation.HasCaseIssue {
					if s.shouldLog() {
						s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Tool case correction", map[string]interface{}{
							"original_name":  currentCall.Name,
							"corrected_name": validation.CorrectToolName,
						})
					}
					currentCall = s.correctToolName(ctx, currentCall, validation.CorrectToolName)
				} else if validation.HasToolNameIssue {
					// Check if this is a semantic issue that needs rule-based correction
					if correctedCall, success := s.CorrectSemanticIssue(ctx, currentCall, availableTools); success {
						s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Semantic correction applied (architectural fix)", map[string]interface{}{
							"original_tool":   currentCall.Name,
							"corrected_tool":  correctedCall.Name,
							"correction_type": "semantic",
						})
						currentCall = correctedCall
					} else {
						if s.shouldLog() {
							s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Tool name correction", map[string]interface{}{
								"original_name":  currentCall.Name,
								"corrected_name": validation.CorrectToolName,
							})
						}
						// Apply both tool name and input corrections for slash commands
						currentCall = s.correctToolNameAndInput(ctx, currentCall, validation.CorrectToolName, validation.CorrectedInput)
					}
				}

				// Re-validate after name correction
				validation = s.ValidateToolCall(ctx, currentCall, availableTools)
				if validation.IsValid {
					if s.shouldLog() {
						s.logInfo(logger.ComponentToolCorrection, logger.CategorySuccess, requestID, "Tool name correction successful", nil)
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
				s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Rule-based parameter correction successful", map[string]interface{}{
					"tool_name":       currentCall.Name,
					"correction_type": "rule-based",
				})

				// Re-validate rule-based correction
				ruleValidation := s.ValidateToolCall(ctx, ruleBasedCall, availableTools)
				if ruleValidation.IsValid {
					s.logInfo(logger.ComponentToolCorrection, logger.CategorySuccess, requestID, "Rule-based parameter correction passed validation", map[string]interface{}{
						"tool_name":         currentCall.Name,
						"validation_result": "passed",
					})
					correctedCalls = append(correctedCalls, ruleBasedCall)
					break // Exit retry loop - success
				} else {
					s.logWarn(logger.ComponentToolCorrection, logger.CategoryWarning, requestID, "Rule-based correction failed validation, continuing with LLM", map[string]interface{}{
						"tool_name":      currentCall.Name,
						"missing_params": ruleValidation.MissingParams,
						"invalid_params": ruleValidation.InvalidParams,
					})
					// Update currentCall to the rule-based attempt for potential LLM correction
					currentCall = ruleBasedCall
					validation = ruleValidation
				}
			}

			// Stage 1.6: Try rule-based TodoWrite correction before LLM
			if currentCall.Name == "TodoWrite" {
				if ruleBasedCall, success := s.AttemptRuleBasedTodoWriteCorrection(ctx, currentCall); success {
					s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Rule-based TodoWrite correction successful", map[string]interface{}{
						"tool_name":       "TodoWrite",
						"correction_type": "rule-based",
						"input_params":    ruleBasedCall.Input,
					})

					// Re-validate rule-based correction
					ruleValidation := s.ValidateToolCall(ctx, ruleBasedCall, availableTools)
					if ruleValidation.IsValid {
						s.logInfo(logger.ComponentToolCorrection, logger.CategorySuccess, requestID, "Rule-based TodoWrite correction passed validation", map[string]interface{}{
							"tool_name":         "TodoWrite",
							"validation_result": "passed",
						})
						correctedCalls = append(correctedCalls, ruleBasedCall)
						break // Exit retry loop - success
					} else {
						s.logWarn(logger.ComponentToolCorrection, logger.CategoryWarning, requestID, "Rule-based TodoWrite correction failed validation, falling back to LLM", map[string]interface{}{
							"tool_name":      "TodoWrite",
							"missing_params": ruleValidation.MissingParams,
							"invalid_params": ruleValidation.InvalidParams,
						})
						// Update currentCall to the rule-based attempt for LLM correction
						currentCall = ruleBasedCall
						validation = ruleValidation
					}
				}
			}

			// Stage 1.7: Try rule-based MultiEdit correction before LLM
			if currentCall.Name == "MultiEdit" {
				if ruleBasedCall, success := s.AttemptRuleBasedMultiEditCorrection(ctx, currentCall); success {
					s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Rule-based MultiEdit correction successful", map[string]interface{}{
						"tool_name":       "MultiEdit",
						"correction_type": "rule-based",
						"input_params":    ruleBasedCall.Input,
					})

					// Re-validate rule-based correction
					ruleValidation := s.ValidateToolCall(ctx, ruleBasedCall, availableTools)
					if ruleValidation.IsValid {
						s.logInfo(logger.ComponentToolCorrection, logger.CategorySuccess, requestID, "Rule-based MultiEdit correction passed validation", map[string]interface{}{
							"tool_name":         "MultiEdit",
							"validation_result": "passed",
						})
						correctedCalls = append(correctedCalls, ruleBasedCall)
						break // Exit retry loop - success
					} else {
						s.logWarn(logger.ComponentToolCorrection, logger.CategoryWarning, requestID, "Rule-based MultiEdit correction failed validation, falling back to LLM", map[string]interface{}{
							"tool_name":      "MultiEdit",
							"missing_params": ruleValidation.MissingParams,
							"invalid_params": ruleValidation.InvalidParams,
						})
						// Update currentCall to the rule-based attempt for LLM correction
						currentCall = ruleBasedCall
						validation = ruleValidation
					}
				}
			}

			// Stage 2: Fix parameter issues (LLM correction)
			if len(validation.MissingParams) > 0 || len(validation.InvalidParams) > 0 {
				s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Starting LLM parameter correction", map[string]interface{}{
					"tool_name":       currentCall.Name,
					"original_input":  currentCall.Input,
					"missing_params":  validation.MissingParams,
					"invalid_params":  validation.InvalidParams,
					"correction_type": "llm",
				})
				correctedCall, err := s.correctToolCall(ctx, currentCall, availableTools)
				if err != nil {
					s.logError(logger.ComponentToolCorrection, logger.CategoryError, requestID, "Parameter correction failed", map[string]interface{}{
						"tool_name":   currentCall.Name,
						"error":       err.Error(),
						"retry_count": retryCount,
						"will_retry":  true,
					})
					// Memory management: Reset to original on failure to prevent accumulation
					currentCall = originalCall
					retryCount++
					continue // Retry with original call
				} else {
					s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "LLM correction completed", map[string]interface{}{
						"tool_name":       correctedCall.Name,
						"corrected_input": correctedCall.Input,
						"original_tool":   currentCall.Name,
					})
					// Re-validate corrected call to verify it's actually fixed
					revalidation := s.ValidateToolCall(ctx, correctedCall, availableTools)
					if !revalidation.IsValid {
						s.logWarn(logger.ComponentToolCorrection, logger.CategoryWarning, requestID, "LLM correction failed validation - will retry", map[string]interface{}{
							"tool_name":      correctedCall.Name,
							"missing_params": revalidation.MissingParams,
							"invalid_params": revalidation.InvalidParams,
							"retry_count":    retryCount,
						})
					} else {
						s.logInfo(logger.ComponentToolCorrection, logger.CategorySuccess, requestID, "LLM correction passed validation", map[string]interface{}{
							"tool_name":         correctedCall.Name,
							"validation_result": "passed",
						})
					}
					// Log detailed parameter changes
					s.logParameterChanges(requestID, currentCall, correctedCall)

					// Check if correction was successful
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
				s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Attempting full LLM correction", map[string]interface{}{
					"tool_name":       currentCall.Name,
					"correction_type": "full_llm",
				})
				correctedCall, err := s.correctToolCall(ctx, currentCall, availableTools)
				if err != nil {
					s.logError(logger.ComponentToolCorrection, logger.CategoryError, requestID, "Full LLM correction failed", map[string]interface{}{
						"tool_name":   currentCall.Name,
						"error":       err.Error(),
						"retry_count": retryCount,
					})
					// Memory management: Reset to original on failure to prevent accumulation
					currentCall = originalCall
					retryCount++
					continue // Retry with original call
				} else {
					if s.shouldLog() {
						// Log detailed parameter changes
						s.logParameterChanges(requestID, currentCall, correctedCall)
					}

					// Check if correction was successful
					fullRevalidation := s.ValidateToolCall(ctx, correctedCall, availableTools)
					if fullRevalidation.IsValid {
						correctedCalls = append(correctedCalls, correctedCall)
						break // Exit retry loop - success
					} else {
						// Correction failed, update for retry
						currentCall = correctedCall
						validation = fullRevalidation
						retryCount++
						continue
					}
				}
			}
		} // End retry loop
	}

	return correctedCalls, nil
}

// DetectToolNecessity analyzes conversation context to determine if tools should be required
// DetectToolNecessity is the PRIMARY API for tool necessity detection
// This is the main entry point that should be used by all callers (proxy, etc.)
// It includes context handling, error management, and LLM fallback
func (s *Service) DetectToolNecessity(ctx context.Context, messages []types.OpenAIMessage, availableTools []types.Tool) (bool, error) {
	if !s.enabled {
		return false, nil
	}

	requestID := getRequestID(ctx)

	// Check if tool choice correction is disabled
	if !s.enableToolChoiceCorrection {
		if s.shouldLog() {
			s.logInfo(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Tool choice correction disabled", map[string]interface{}{
				"enabled": false,
				"reason":  "ENABLE_TOOL_CHOICE_CORRECTION=false",
			})
		}
		return false, nil
	}

	// Stage A & B: Use hybrid classifier for deterministic analysis with logging
	decision := s.classifier.DetectToolNecessity(messages, s.logInfo, requestID)

	if s.shouldLog() {
		s.logInfo(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Tool necessity decision", map[string]interface{}{
			"decision":  boolToString(decision.RequireTools),
			"confident": decision.Confident,
			"reason":    decision.Reason,
		})
	}

	// If classifier is confident, use its decision (Stage B complete)
	if decision.Confident {
		return decision.RequireTools, nil
	}

	// Stage C: LLM fallback for ambiguous cases only
	return s.llmFallbackAnalysis(ctx, messages, availableTools, requestID)
}

// llmFallbackAnalysis handles Stage C - LLM fallback for ambiguous cases
func (s *Service) llmFallbackAnalysis(ctx context.Context, messages []types.OpenAIMessage, availableTools []types.Tool, requestID string) (bool, error) {
	if s.shouldLog() {
		s.logInfo(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage C: Starting LLM fallback analysis", map[string]interface{}{
			"stage":          "C_llm_fallback",
			"messages_count": len(messages),
			"tools_count":    len(availableTools),
			"reason":         "Rule engine not confident, requiring LLM analysis",
		})
	}

	// Use simplified prompt since rules handle clear cases
	prompt := s.buildSimplifiedToolNecessityPrompt(messages, availableTools)
	
	if s.shouldLog() {
		s.logInfo(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage C: Generated analysis prompt", map[string]interface{}{
			"stage":        "C_llm_fallback",
			"prompt_length": len(prompt),
			"full_prompt":   prompt,
		})
	}

	// Create request to correction model
	systemMsg := "Analyze if this ambiguous request requires tools. Focus on user intent and context. Respond only 'YES' or 'NO'."
	req := types.OpenAIRequest{
		Model: s.modelName,
		Messages: []types.OpenAIMessage{
			{
				Role:    "system",
				Content: systemMsg,
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens:   10, // Very short response needed
		Temperature: 0.1,
	}

	if s.shouldLog() {
		s.logInfo(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage C: Prepared LLM request", map[string]interface{}{
			"stage":        "C_llm_fallback",
			"model":        s.modelName,
			"max_tokens":   req.MaxTokens,
			"temperature":  req.Temperature,
			"system_message": systemMsg,
		})
	}

	// Send request
	if s.shouldLog() {
		s.logInfo(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage C: Sending request to LLM", map[string]interface{}{
			"stage": "C_llm_fallback",
			"model": s.modelName,
		})
	}
	
	response, err := s.sendCorrectionRequest(req)
	if err != nil {
		if s.shouldLog() {
			s.logWarn(logger.ComponentHybridClassifier, logger.CategoryWarning, requestID, "Stage C: LLM request failed", map[string]interface{}{
				"stage": "C_llm_fallback",
				"error": err.Error(),
				"fallback_decision": "no_tools",
				"reason": "Fail safe: don't force tools if analysis fails",
			})
		}
		return false, nil // Fail safe: don't force tools if analysis fails
	}

	if s.shouldLog() {
		s.logInfo(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage C: Received LLM response", map[string]interface{}{
			"stage": "C_llm_fallback",
			"choices_count": len(response.Choices),
			"response_id": response.ID,
		})
	}

	// Parse response
	if len(response.Choices) == 0 {
		if s.shouldLog() {
			s.logWarn(logger.ComponentHybridClassifier, logger.CategoryWarning, requestID, "Stage C: Empty LLM response", map[string]interface{}{
				"stage": "C_llm_fallback",
				"fallback_decision": "no_tools",
				"reason": "Fail safe: don't force tools if no response choices",
			})
		}
		return false, nil // Fail safe: don't force tools if no response
	}

	rawContent := response.Choices[0].Message.Content
	content := strings.TrimSpace(strings.ToUpper(rawContent))
	shouldRequire := strings.HasPrefix(content, "YES")

	if s.shouldLog() {
		s.logInfo(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage C: Parsing LLM response", map[string]interface{}{
			"stage": "C_llm_fallback",
			"raw_content": rawContent,
			"normalized_content": content,
			"starts_with_yes": strings.HasPrefix(content, "YES"),
			"decision_logic": "YES prefix check",
		})
	}

	if s.shouldLog() {
		s.logInfo(logger.ComponentHybridClassifier, logger.CategoryClassification, requestID, "Stage C: LLM fallback analysis completed", map[string]interface{}{
			"stage": "C_llm_fallback",
			"final_decision": boolToString(shouldRequire),
			"analysis_result": strings.ToLower(content),
			"raw_response": rawContent,
			"confident": true, // LLM analysis is considered confident
		})
	}

	return shouldRequire, nil
}

// buildExitPlanModeValidationPrompt creates the prompt for ExitPlanMode usage validation
func (s *Service) buildExitPlanModeValidationPrompt(planContent string, messages []types.OpenAIMessage) string {
	// Get recent tool names for context
	recentTools := s.getRecentToolNames(messages, 10) // Get last 10 tools used

	return fmt.Sprintf(`Analyze this ExitPlanMode usage and determine if it's appropriate:

PLAN CONTENT:
"%s"

CONVERSATION CONTEXT:
- Recent tools used: %s
- Total messages in conversation: %d

RULES FOR EXITPLANMODE:
✅ APPROPRIATE USAGE (respond with ALLOW):
- Planning future implementation steps
- Outlining approach before starting work
- Requesting approval for implementation plan
- Forward-looking language: "I will...", "Here's my plan...", "I propose..."

❌ INAPPROPRIATE USAGE (respond with BLOCK):
- Summarizing completed work
- Reporting finished implementation 
- Using past tense to describe what was done: "I've implemented...", "The implementation included..."
- Completion language: "successfully completed", "all tasks finished", "ready for production"

ANALYSIS CRITERIA:
1. Language tense: Future-focused planning vs past-tense completion summary
2. Content purpose: Outlining upcoming work vs reporting finished work
3. Context: Is this planning before work or summarizing after work?

Respond with ONLY "BLOCK" or "ALLOW".`,
		planContent,
		strings.Join(recentTools, ", "),
		len(messages))
}

// BuildExitPlanModeValidationPrompt is a public wrapper for testing
func (s *Service) BuildExitPlanModeValidationPrompt(planContent string, messages []types.OpenAIMessage) string {
	return s.buildExitPlanModeValidationPrompt(planContent, messages)
}

// getRecentToolNames extracts recent tool names for context
func (s *Service) getRecentToolNames(messages []types.OpenAIMessage, limit int) []string {
	var tools []string
	count := 0

	// Go backwards through messages to get most recent tools
	for i := len(messages) - 1; i >= 0 && count < limit; i-- {
		msg := messages[i]
		if msg.ToolCalls != nil {
			for _, toolCall := range msg.ToolCalls {
				if count >= limit {
					break
				}
				tools = append(tools, toolCall.Function.Name)
				count++
			}
		}
	}

	// Reverse to get chronological order
	for i, j := 0, len(tools)-1; i < j; i, j = i+1, j-1 {
		tools[i], tools[j] = tools[j], tools[i]
	}

	return tools
}

// buildToolNecessityPrompt creates the prompt for tool necessity analysis
func (s *Service) buildToolNecessityPrompt(messages []types.OpenAIMessage, availableTools []types.Tool) string {
	// Build available tools list
	var toolNames []string
	for _, tool := range availableTools {
		toolNames = append(toolNames, tool.Name)
	}

	// Build conversation context
	var conversationContext strings.Builder
	conversationContext.WriteString("RECENT CONVERSATION:\n")
	for i, msg := range messages {
		// Truncate very long messages for context
		content := msg.Content
		if len(content) > 200 {
			content = content[:200] + "... [truncated]"
		}

		// Show role and content
		conversationContext.WriteString(fmt.Sprintf("%d. %s: %s\n", i+1, strings.ToUpper(msg.Role), content))

		// Show tool calls if present
		if len(msg.ToolCalls) > 0 {
			var toolCallNames []string
			for _, tc := range msg.ToolCalls {
				toolCallNames = append(toolCallNames, tc.Function.Name)
			}
			conversationContext.WriteString(fmt.Sprintf("   [Used tools: %s]\n", strings.Join(toolCallNames, ", ")))
		}
	}

	// Extract the most recent user message for emphasis
	var lastUserMessage string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserMessage = messages[i].Content
			break
		}
	}

	return fmt.Sprintf(`You are analyzing whether a user request requires tools (YES) or can be handled conversationally (NO).

%s

CURRENT REQUEST: "%s"
AVAILABLE TOOLS: %s

MANDATORY RULE - If request contains ANY of these words, answer YES immediately:
UPDATE/UPDATING, CREATE/CREATING, EDIT/EDITING, WRITE/WRITING, MODIFY/MODIFYING, FIX/FIXING, CHANGE/CHANGING, MAKE/MAKING, BUILD/BUILDING, ADD/ADDING, IMPLEMENT/IMPLEMENTING, INSTALL/INSTALLING, SETUP, RUN/RUNNING, EXECUTE/EXECUTING, LAUNCH/LAUNCHING, START/STARTING, DELETE/DELETING, REMOVE/REMOVING

OVERRIDE RULE: The phrase "updating CLAUDE.md" MUST return YES regardless of context or politeness.

CONTEXT-AWARE DECISION MATRIX:

SCENARIO 1 - CONTINUATION AFTER RESEARCH (Main failing case):
Pattern: Research tools used → High token output → User requests implementation
Example conversation:
  USER: "gather knowledge about project and update CLAUDE.md"
  ASSISTANT: [Used Task tool - 23,000 tokens research output]
  USER: "Please continue with updating CLAUDE.md based on the research"
DECISION: YES (Research complete, now implementation needed)

SCENARIO 2 - DIRECT FILE OPERATIONS:
- "create file", "edit config", "update README", "run tests" → YES
- "write to file", "modify code", "add function" → YES
- Any action on files/code regardless of politeness → YES

SCENARIO 3 - COMPOUND REQUESTS:
- "analyze X and create Y" → YES (contains implementation verb "create")
- "research Z and implement W" → YES (contains implementation verb "implement")
- "gather info and update file" → YES (contains implementation verb "update")

SCENARIO 4 - PURE RESEARCH/ANALYSIS:
- "read file X and tell me what it does" → NO
- "explain the architecture" → NO
- "what does this code do?" → NO

FEW-SHOT EXAMPLES:

EXAMPLE 1 (Target fix):
Context: "Task tool used, 23k tokens output, research complete"
Request: "Please continue with updating CLAUDE.md based on the research"
Contains: "updating" (implementation verb)
Phase: Research done, implementation needed
ANSWER: YES

EXAMPLE 2 (Simple implementation):
Context: None
Request: "create a new config file"
Contains: "create" (implementation verb)  
ANSWER: YES

EXAMPLE 3 (Pure research):
Context: None
Request: "read the architecture docs and explain the design"
Contains: No implementation verbs, asks for explanation
ANSWER: NO

EXAMPLE 4 (Compound with implementation):
Context: None
Request: "analyze the auth system and implement OAuth"
Contains: "implement" (implementation verb)
ANSWER: YES

DECISION ALGORITHM:
1. Does request contain implementation verbs? → YES
2. Does conversation show research complete + user wants action? → YES
3. Is request purely informational/explanatory? → NO
4. When uncertain about file operations → YES

CRITICAL: File operations (update, create, edit, modify) ALWAYS require tools.
Be decisive. Prioritize action verbs over polite language.

Answer only: YES or NO`, conversationContext.String(), lastUserMessage, strings.Join(toolNames, ", "))
}

// buildSimplifiedToolNecessityPrompt creates a simplified prompt for LLM fallback
// Used only for ambiguous cases that rules couldn't handle
func (s *Service) buildSimplifiedToolNecessityPrompt(messages []types.OpenAIMessage, availableTools []types.Tool) string {
	// Build available tools list
	var toolNames []string
	for _, tool := range availableTools {
		toolNames = append(toolNames, tool.Name)
	}

	// Get recent conversation context (last 3 messages)
	var contextMessages []string
	start := len(messages) - 3
	if start < 0 {
		start = 0
	}

	contextMsgCount := 0
	for _, msg := range messages[start:] {
		role := strings.ToUpper(msg.Role)
		content := msg.Content
		originalLength := len(content)
		if len(content) > 150 { // Truncate long messages
			content = content[:150] + "..."
		}
		contextMessages = append(contextMessages, fmt.Sprintf("%s: %s", role, content))
		contextMsgCount++
		
		// Log context message processing if verbose logging is enabled
		if s.shouldLog() {
			s.logInfo(logger.ComponentHybridClassifier, logger.CategoryClassification, "", "Stage C: Processing context message", map[string]interface{}{
				"stage": "C_llm_fallback",
				"message_role": msg.Role,
				"original_length": originalLength,
				"truncated": originalLength > 150,
				"final_length": len(content),
			})
		}
	}

	// Extract current user request
	var currentRequest string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			currentRequest = messages[i].Content
			break
		}
	}

	// Log prompt construction details
	if s.shouldLog() {
		s.logInfo(logger.ComponentHybridClassifier, logger.CategoryClassification, "", "Stage C: Prompt construction details", map[string]interface{}{
			"stage": "C_llm_fallback",
			"context_messages_count": len(contextMessages),
			"available_tools_count": len(toolNames),
			"available_tools": toolNames,
			"current_request_length": len(currentRequest),
			"current_request": currentRequest,
		})
	}

	finalPrompt := fmt.Sprintf(`This is an ambiguous request that needs analysis.

RECENT CONTEXT:
%s

CURRENT REQUEST: "%s"
TOOLS: %s

The request was not clearly classified by rules. Analyze if it requires tools:
- Does it ask for file operations, code changes, or command execution?
- Is it asking to create, modify, or run something?
- Or is it asking for explanation, analysis, or information only?

Answer: YES or NO`,
		strings.Join(contextMessages, "\n"),
		currentRequest,
		strings.Join(toolNames, ", "))

	// Log final prompt details
	if s.shouldLog() {
		s.logInfo(logger.ComponentHybridClassifier, logger.CategoryClassification, "", "Stage C: Final prompt constructed", map[string]interface{}{
			"stage": "C_llm_fallback",
			"final_prompt_length": len(finalPrompt),
		})
	}

	return finalPrompt
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
	IsValid          bool
	HasCaseIssue     bool
	HasToolNameIssue bool // New: indicates tool name was corrected (e.g., slash command)
	CorrectToolName  string
	MissingParams    []string
	InvalidParams    []string
	CorrectedInput   map[string]interface{} // New: corrected input parameters
}

// ValidateToolCall performs comprehensive tool call validation using injected ToolValidator
// Made public for testing slash command correction functionality
func (s *Service) ValidateToolCall(ctx context.Context, call types.Content, availableTools []types.Tool) ValidationResult {
	requestID := getRequestID(ctx)
	result := ValidationResult{IsValid: false}

	// Enhanced logging: Log validation start
	if s.shouldLog() {
		s.logInfo(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "Validating tool call", map[string]interface{}{
			"tool_name":       call.Name,
			"parameter_count": len(call.Input),
		})
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
					s.logWarn(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "Tool name case issue detected", map[string]interface{}{
						"provided_name": call.Name,
						"correct_name":  tool.Name,
					})
				}
			}
		}

		if tool == nil {
			// Check if this is a slash command that should be converted to Task
			if s.isSlashCommand(call.Name) {
				return s.correctSlashCommandToTask(ctx, call, availableTools)
			}

			// Try registry for schema lookup
			if registryTool, exists := s.registry.GetSchema(call.Name); exists {
				tool = registryTool
				if s.shouldLog() {
					s.logInfo(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "Using registry schema for tool validation", map[string]interface{}{
						"tool_name": call.Name,
					})
				}
			} else {
				if s.shouldLog() {
					s.logError(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "Unknown tool detected", map[string]interface{}{
						"tool_name": call.Name,
					})
				}
				return result
			}
		}
	}

	// Enhanced logging: Log tool schema details for TodoWrite
	if s.shouldLog() && call.Name == "TodoWrite" {
		s.logInfo(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "TodoWrite validation analysis", map[string]interface{}{
			"required_params":      tool.InputSchema.Required,
			"available_properties": getPropertyNames(tool.InputSchema.Properties),
			"input_params":         getInputParamNames(call.Input),
		})
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
				s.logInfo(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "Allowing additional parameter for Task tool", map[string]interface{}{
					"parameter": invalid,
				})
			}
			// Don't add to filteredInvalid - we're allowing it
		}
		result.InvalidParams = filteredInvalid
	}

	// Enhanced logging: Detailed validation results
	if len(result.MissingParams) > 0 && s.shouldLog() {
		s.logWarn(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "Missing required parameters", map[string]interface{}{
			"tool_name":      call.Name,
			"missing_params": result.MissingParams,
		})
		if call.Name == "TodoWrite" {
			s.logInfo(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "TodoWrite parameter mismatch", map[string]interface{}{
				"expected": "todos array",
				"received": getInputParamNames(call.Input),
			})
		}
	}
	if len(result.InvalidParams) > 0 && s.shouldLog() {
		s.logWarn(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "Invalid parameters detected", map[string]interface{}{
			"tool_name":      call.Name,
			"invalid_params": result.InvalidParams,
		})
		if call.Name == "TodoWrite" {
			for _, invalid := range result.InvalidParams {
				if value, exists := call.Input[invalid]; exists {
					s.logInfo(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "TodoWrite invalid parameter details", map[string]interface{}{
						"parameter": invalid,
						"value":     value,
						"type":      fmt.Sprintf("%T", value),
					})
				}
			}
		}
	}

	// MultiEdit structural validation: Check for file/path parameters nested in edits
	// (Run this regardless of basic validation result to catch structural issues)
	if call.Name == "MultiEdit" {
		if editsValue, hasEdits := call.Input["edits"]; hasEdits {
			if editsArray, ok := editsValue.([]interface{}); ok {
				// Common file path parameter variations that shouldn't appear in individual edits
				invalidNestedParams := []string{"file_path", "filepath", "filename", "path", "file", "target_path", "source_path"}

				for i, edit := range editsArray {
					if editMap, ok := edit.(map[string]interface{}); ok {
						for _, paramName := range invalidNestedParams {
							if _, hasParam := editMap[paramName]; hasParam {
								// Found file/path parameter nested in edit - this is a structural violation
								result.InvalidParams = append(result.InvalidParams, paramName)
								result.IsValid = false
								if s.shouldLog() {
									s.logWarn(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "MultiEdit structural violation detected", map[string]interface{}{
										"parameter":  paramName,
										"edit_index": i,
										"issue":      "file/path parameter should only be at top level",
									})
								}
							}
						}
					}
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
				s.logInfo(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "Semantic tool issue detected", map[string]interface{}{
					"current_tool":   call.Name,
					"suggested_tool": correctTool,
				})
			}
		}
	}

	// Valid if no issues found (or only case issue which we can fix easily)
	result.IsValid = len(result.MissingParams) == 0 && len(result.InvalidParams) == 0 && !result.HasToolNameIssue

	// Enhanced logging: Log final validation result
	if s.shouldLog() {
		if result.IsValid {
			s.logInfo(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "Tool call validation passed", map[string]interface{}{
				"tool_name": call.Name,
			})
		} else {
			s.logWarn(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "Tool call validation failed", map[string]interface{}{
				"tool_name":      call.Name,
				"missing_params": result.MissingParams,
				"invalid_params": result.InvalidParams,
			})
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
		s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Starting LLM correction", map[string]interface{}{
			"tool_name":           call.Name,
			"original_parameters": call.Input,
		})
		if call.Name == "TodoWrite" {
			s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "TodoWrite correction attempt", map[string]interface{}{
				"input_structure": "analyzing",
			})
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
		s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Correction prompt prepared", map[string]interface{}{
			"prompt_preview": truncatedPrompt,
			"prompt_length":  len(prompt),
		})
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
		s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Sending correction request to LLM", map[string]interface{}{
			"model":       s.modelName,
			"max_tokens":  req.MaxTokens,
			"temperature": req.Temperature,
		})
	}

	// Send request
	response, err := s.sendCorrectionRequest(req)
	if err != nil {
		if s.shouldLog() {
			s.logError(logger.ComponentToolCorrection, logger.CategoryError, requestID, "LLM correction request failed", map[string]interface{}{
				"error": err.Error(),
			})
		}
		return call, fmt.Errorf("[%s] correction request failed: %v", requestID, err)
	}

	// Enhanced logging: Log raw LLM response
	if s.shouldLog() && len(response.Choices) > 0 {
		s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "LLM correction response received", map[string]interface{}{
			"response_length": len(response.Choices[0].Message.Content),
			"raw_response":    response.Choices[0].Message.Content,
		})
	}

	// Parse corrected tool call
	correctedCall, err := s.parseCorrectedResponse(response, call)
	if err != nil {
		if s.shouldLog() {
			parseData := map[string]interface{}{
				"error": err.Error(),
			}
			if len(response.Choices) > 0 {
				parseData["failed_response"] = response.Choices[0].Message.Content
			}
			s.logError(logger.ComponentToolCorrection, logger.CategoryError, requestID, "Failed to parse LLM correction response", parseData)
		}
		return call, fmt.Errorf("[%s] failed to parse correction: %v", requestID, err)
	}

	// Enhanced logging: Log successful correction details
	if s.shouldLog() {
		s.logInfo(logger.ComponentToolCorrection, logger.CategorySuccess, requestID, "LLM correction successful", map[string]interface{}{
			"tool_name":            correctedCall.Name,
			"corrected_parameters": correctedCall.Input,
		})

		// Special logging for TodoWrite corrections
		if call.Name == "TodoWrite" {
			if todos, exists := correctedCall.Input["todos"]; exists {
				if todosArray, ok := todos.([]interface{}); ok {
					s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "TodoWrite correction completed", map[string]interface{}{
						"todo_count": len(todosArray),
					})
				} else {
					s.logWarn(logger.ComponentToolCorrection, logger.CategoryWarning, requestID, "TodoWrite correction issue", map[string]interface{}{
						"issue": "todos is not an array",
						"type":  fmt.Sprintf("%T", todos),
					})
				}
			} else {
				s.logWarn(logger.ComponentToolCorrection, logger.CategoryWarning, requestID, "TodoWrite correction issue", map[string]interface{}{
					"issue": "missing 'todos' parameter",
				})
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

// sendCorrectionRequest sends request with automatic failover
func (s *Service) sendCorrectionRequest(req types.OpenAIRequest) (*types.OpenAIResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Try up to 3 endpoints for failover using circuit breaker
	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Get healthy endpoint using circuit breaker
		endpoint := s.config.GetHealthyToolCorrectionEndpoint()
		if endpoint == "" {
			return nil, fmt.Errorf("no tool correction endpoints available")
		}

		httpReq, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(reqBody))
		if err != nil {
			return nil, err
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+s.apiKey)

		// Use longer timeout for Task agents that need extensive tool usage
		client := &http.Client{
			Timeout: 60 * time.Second, // Increased to allow Task agents to complete thorough analysis
		}

		resp, err := client.Do(httpReq)
		if err != nil {
			lastErr = err
			// Record endpoint failure for circuit breaker
			s.config.RecordEndpointFailure(endpoint)

			// Retry on timeout/connection errors
			if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "context deadline exceeded") {
				if attempt < maxRetries-1 {
					if s.shouldLog() {
						s.logWarn(logger.ComponentToolCorrection, logger.CategoryFailover, "", "Tool correction endpoint failed, trying next", map[string]interface{}{
							"attempt":     attempt + 1,
							"max_retries": maxRetries,
							"error":       err.Error(),
						})
					}
					continue
				}
			}
			// For non-retryable errors, fail immediately
			if attempt < maxRetries-1 {
				if s.shouldLog() {
					s.logWarn(logger.ComponentToolCorrection, logger.CategoryFailover, "", "Tool correction endpoint error, trying next", map[string]interface{}{
						"error": err.Error(),
					})
				}
				continue
			}
			return nil, fmt.Errorf("tool correction request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("endpoint returned status %d: %s", resp.StatusCode, string(respBody))
			// Record endpoint failure for non-200 status codes
			s.config.RecordEndpointFailure(endpoint)

			if attempt < maxRetries-1 {
				if s.shouldLog() {
					s.logWarn(logger.ComponentToolCorrection, logger.CategoryFailover, "", "Tool correction endpoint returned non-200, trying next", map[string]interface{}{
						"status_code": resp.StatusCode,
					})
				}
				continue
			}
			return nil, lastErr
		}

		var response types.OpenAIResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			lastErr = err
			// Record endpoint failure for JSON parse errors
			s.config.RecordEndpointFailure(endpoint)

			if attempt < maxRetries-1 {
				if s.shouldLog() {
					s.logWarn(logger.ComponentToolCorrection, logger.CategoryFailover, "", "Tool correction response parse failed, trying next", map[string]interface{}{
						"error": err.Error(),
					})
				}
				continue
			}
			return nil, fmt.Errorf("failed to parse response: %v", err)
		}

		// Success - record endpoint success for circuit breaker
		s.config.RecordEndpointSuccess(endpoint)

		if attempt > 0 {
			if s.shouldLog() {
				s.logInfo(logger.ComponentToolCorrection, logger.CategorySuccess, "", "Tool correction succeeded on fallback endpoint", map[string]interface{}{
					"attempt": attempt + 1,
				})
			}
		}
		return &response, nil
	}

	return nil, fmt.Errorf("all tool correction endpoints failed, last error: %v", lastErr)
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
		s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Attempting rule-based TodoWrite correction", map[string]interface{}{
			"input_parameters": call.Input,
		})
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
					s.logInfo(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "TodoWrite already has valid structure", map[string]interface{}{
						"todos_count": len(todosArray),
					})
				}
				return call, false
			} else {
				if s.shouldLog() {
					s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Found todos array with invalid structure", map[string]interface{}{
						"todos_count": len(todosArray),
					})
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
									s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Transformed parameter in todo item", map[string]interface{}{
										"transformation": "description → content",
										"value":          descStr,
									})
								}
							}
						}
						if task, exists := todoMap["task"]; exists {
							if taskStr, ok := task.(string); ok {
								content = taskStr
								if s.shouldLog() {
									s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Transformed parameter in todo item", map[string]interface{}{
										"transformation": "task → content",
										"value":          taskStr,
									})
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
						if statusValue, exists := todoMap["status"]; exists {
							if sStr, ok := statusValue.(string); ok && isValidStatus(sStr) {
								status = sStr
							}
						} else {
							if s.shouldLog() {
								s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Added missing parameter to todo item", map[string]interface{}{
									"parameter": "status",
									"value":     status,
								})
							}
						}

						priority := "medium"
						if p, exists := todoMap["priority"]; exists {
							if pStr, ok := p.(string); ok && isValidPriority(pStr) {
								priority = pStr
							}
						} else {
							if s.shouldLog() {
								s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Added missing parameter to todo item", map[string]interface{}{
									"parameter": "priority",
									"value":     priority,
								})
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
						s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Rule-based TodoWrite correction - Rule 0", map[string]interface{}{
							"rule":       "corrected_malformed_todos_array",
							"item_count": len(todos),
						})
					}
					correctedInput["todos"] = todos

					// Validate the correction
					if err := s.validateTodoWriteCorrection(correctedInput); err == nil {
						if s.shouldLog() {
							s.logInfo(logger.ComponentToolCorrection, logger.CategorySuccess, requestID, "Malformed todos correction passed validation", map[string]interface{}{
								"todos_count": len(todos),
							})
						}
						return types.Content{
							Type:  "tool_use",
							ID:    call.ID,
							Name:  "TodoWrite",
							Input: correctedInput,
						}, true
					} else {
						if s.shouldLog() {
							s.logWarn(logger.ComponentToolCorrection, logger.CategoryWarning, requestID, "Malformed todos correction failed validation", map[string]interface{}{
								"error": err.Error(),
							})
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
				s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Rule-based TodoWrite correction - Rule 1", map[string]interface{}{
					"rule": "converted_todo_string_to_array",
				})
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
				s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Rule-based TodoWrite correction - Rule 2", map[string]interface{}{
					"rule": "converted_task_to_todos_array",
				})
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
				s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Rule-based TodoWrite correction - Rule 3", map[string]interface{}{
					"rule":       "converted_items_array_to_todos",
					"item_count": len(todos),
				})
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
				s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Rule-based TodoWrite correction - Rule 4", map[string]interface{}{
					"rule":    "created_todo_from_content",
					"content": content,
				})
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
			s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Rule-based TodoWrite correction - Rule 5", map[string]interface{}{
				"rule": "created_default_todo_for_empty_input",
			})
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
			s.logInfo(logger.ComponentToolCorrection, logger.CategorySuccess, requestID, "Rule-based TodoWrite correction successful", map[string]interface{}{
				"todo_count": len(todos),
			})
		}

		return correctedCall, true
	}

	if s.shouldLog() {
		s.logWarn(logger.ComponentToolCorrection, logger.CategoryWarning, requestID, "Rule-based TodoWrite correction failed", map[string]interface{}{
			"reason": "no valid todos could be generated",
		})
	}
	return call, false
}

// AttemptRuleBasedMultiEditCorrection tries to fix MultiEdit calls with structural issues like file_path nested in edits
func (s *Service) AttemptRuleBasedMultiEditCorrection(ctx context.Context, call types.Content) (types.Content, bool) {
	requestID := getRequestID(ctx)

	if call.Name != "MultiEdit" || call.Type != "tool_use" {
		return call, false
	}

	if s.shouldLog() {
		s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Attempting rule-based MultiEdit correction", map[string]interface{}{
			"input_parameters": call.Input,
		})
	}

	// Create a copy of the input to avoid modifying the original
	correctedInput := make(map[string]interface{})
	for key, value := range call.Input {
		correctedInput[key] = value
	}

	// Check if we have edits parameter
	editsValue, hasEdits := correctedInput["edits"]
	if !hasEdits {
		if s.shouldLog() {
			s.logInfo(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "MultiEdit: No edits parameter found", map[string]interface{}{
				"status": "no_correction_needed",
			})
		}
		return call, false
	}

	editsArray, ok := editsValue.([]interface{})
	if !ok {
		if s.shouldLog() {
			s.logWarn(logger.ComponentToolCorrection, logger.CategoryWarning, requestID, "MultiEdit: Edits parameter is not an array", map[string]interface{}{
				"type": fmt.Sprintf("%T", editsValue),
			})
		}
		return call, false
	}

	// Check if any edit has file/path parameters (this is the structural issue we're fixing)
	needsCorrection := false
	extractedFilePath := ""
	invalidNestedParams := []string{"file_path", "filepath", "filename", "path", "file", "target_path", "source_path"}

	for i, edit := range editsArray {
		if editMap, ok := edit.(map[string]interface{}); ok {
			for _, paramName := range invalidNestedParams {
				if pathValue, hasParam := editMap[paramName]; hasParam {
					needsCorrection = true
					if pathStr, ok := pathValue.(string); ok && pathStr != "" && extractedFilePath == "" {
						extractedFilePath = pathStr
						if s.shouldLog() {
							s.logInfo(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "MultiEdit: Found file/path parameter in edit", map[string]interface{}{
								"parameter":  paramName,
								"edit_index": i,
								"value":      pathStr,
							})
						}
					}
				}
			}
		}
	}

	if !needsCorrection {
		if s.shouldLog() {
			s.logInfo(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "MultiEdit: No file/path parameters found in edits", map[string]interface{}{
				"status": "no_correction_needed",
			})
		}
		return call, false
	}

	if s.shouldLog() {
		s.logInfo(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "MultiEdit: Detected structural issue", map[string]interface{}{
			"issue": "file/path parameters nested in edits",
		})
	}

	// Fix the structure: remove all file/path parameters from individual edits
	var correctedEdits []interface{}
	removedParams := []string{}

	for i, edit := range editsArray {
		if editMap, ok := edit.(map[string]interface{}); ok {
			// Create new edit map without any file/path parameters
			correctedEdit := make(map[string]interface{})
			for key, value := range editMap {
				// Check if this key is a file/path parameter that should be removed
				shouldRemove := false
				for _, invalidParam := range invalidNestedParams {
					if key == invalidParam {
						shouldRemove = true
						if !contains(removedParams, key) {
							removedParams = append(removedParams, key)
						}
						break
					}
				}
				if !shouldRemove {
					correctedEdit[key] = value
				}
			}

			// Only include the edit if it has the required parameters (old_string, new_string)
			if hasRequiredEditParams(correctedEdit) {
				correctedEdits = append(correctedEdits, correctedEdit)
			} else {
				if s.shouldLog() {
					s.logWarn(logger.ComponentToolCorrection, logger.CategoryWarning, requestID, "MultiEdit: Discarding edit after correction", map[string]interface{}{
						"edit_index": i,
						"reason":     "missing required parameters",
					})
				}
			}
		} else {
			// Keep non-map edits as-is (though this is unusual for MultiEdit)
			correctedEdits = append(correctedEdits, edit)
		}
	}

	// Ensure we have valid edits remaining after correction
	if len(correctedEdits) == 0 {
		if s.shouldLog() {
			s.logError(logger.ComponentToolCorrection, logger.CategoryError, requestID, "MultiEdit: No valid edits remaining after correction", map[string]interface{}{})
		}
		return call, false
	}

	// Update the corrected input
	correctedInput["edits"] = correctedEdits

	// Ensure top-level file_path exists
	if _, hasTopLevelFilePath := correctedInput["file_path"]; !hasTopLevelFilePath {
		if extractedFilePath != "" {
			correctedInput["file_path"] = extractedFilePath
			if s.shouldLog() {
				s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "MultiEdit: Extracted file_path to top level", map[string]interface{}{
					"file_path": extractedFilePath,
				})
			}
		} else {
			if s.shouldLog() {
				s.logWarn(logger.ComponentToolCorrection, logger.CategoryWarning, requestID, "MultiEdit: Could not extract valid file_path from edits", map[string]interface{}{})
			}
			return call, false
		}
	}

	// Create the corrected call
	correctedCall := types.Content{
		Type:  call.Type,
		ID:    call.ID,
		Name:  call.Name,
		Input: correctedInput,
	}

	if s.shouldLog() {
		s.logInfo(logger.ComponentToolCorrection, logger.CategorySuccess, requestID, "MultiEdit structural correction successful", map[string]interface{}{
			"removed_parameters": removedParams,
			"edit_count":         len(editsArray),
		})
	}

	return correctedCall, true
}

// contains checks if a string slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// hasRequiredEditParams checks if an edit has the minimum required parameters for MultiEdit
func hasRequiredEditParams(edit map[string]interface{}) bool {
	// For MultiEdit, each edit needs at least old_string and new_string
	_, hasOld := edit["old_string"]
	_, hasNew := edit["new_string"]
	return hasOld && hasNew
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
			s.logWarn(logger.ComponentToolCorrection, logger.CategoryWarning, requestID, "Cannot correct slash command - Task tool not available", map[string]interface{}{
				"slash_command": call.Name,
			})
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
					s.logInfo(logger.ComponentToolCorrection, logger.CategoryValidation, requestID, "Preserving additional parameter for Task tool", map[string]interface{}{
						"parameter": key,
					})
				}
			}
		}
	}

	if s.shouldLog() {
		s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Corrected slash command to Task tool call", map[string]interface{}{
			"original_command":      call.Name,
			"generated_description": description,
		})
	}

	return ValidationResult{
		IsValid:          true,
		HasToolNameIssue: true,
		CorrectToolName:  "Task",
		CorrectedInput:   correctedInput,
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
						s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Architecture fix: WebFetch file:// to Read", map[string]interface{}{
							"original_tool":       call.Name,
							"original_url":        urlStr,
							"corrected_tool":      "Read",
							"corrected_file_path": filePath,
							"reason":              "Claude Code client and Simple Proxy server on different machines",
						})
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
					s.logInfo(logger.ComponentToolCorrection, logger.CategoryTransformation, requestID, "Rule-based parameter correction", map[string]interface{}{
						"tool_name":       call.Name,
						"original_param":  oldParam,
						"corrected_param": newParam,
					})
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
		s.logInfo(logger.ComponentToolCorrection, logger.CategorySuccess, requestID, "Rule-based correction successful", map[string]interface{}{
			"tool_name":        call.Name,
			"parameters_fixed": len(mappings),
		})
	}

	return correctedCall, true
}

// ValidateExitPlanMode validates ExitPlanMode usage with conversation context using LLM analysis
// Returns (shouldBlock, reason) where shouldBlock=true means the tool call should be blocked
func (s *Service) ValidateExitPlanMode(ctx context.Context, call types.Content, messages []types.OpenAIMessage) (bool, string) {
	requestID := getRequestID(ctx)

	if call.Name != "ExitPlanMode" || call.Type != "tool_use" {
		return false, ""
	}

	if !s.enabled {
		return false, "" // Skip validation if correction service disabled
	}

	if s.shouldLog() {
		s.logInfo(logger.ComponentExitPlanMode, logger.CategoryValidation, requestID, "Validating ExitPlanMode usage with LLM analysis", map[string]interface{}{})
	}

	// Extract plan content
	plan, exists := call.Input["plan"]
	if !exists {
		return false, "" // Let schema validation handle missing plan
	}

	planStr, ok := plan.(string)
	if !ok {
		return false, "" // Let schema validation handle non-string plan
	}

	// Build analysis prompt with conversation context
	prompt := s.buildExitPlanModeValidationPrompt(planStr, messages)

	// Create request to correction model
	req := types.OpenAIRequest{
		Model: s.modelName,
		Messages: []types.OpenAIMessage{
			{
				Role:    "system",
				Content: "You are an ExitPlanMode usage validator. Respond with ONLY 'BLOCK' if the ExitPlanMode is being used inappropriately as a completion summary, or 'ALLOW' if it's legitimate planning usage.",
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
			s.logWarn(logger.ComponentExitPlanMode, logger.CategoryWarning, requestID, "ExitPlanMode LLM validation failed, conservative fallback", map[string]interface{}{
				"error":           err.Error(),
				"fallback_action": "allow_usage",
			})
		}
		// Conservative fallback: allow ExitPlanMode if LLM is unavailable
		return false, ""
	}

	// Parse response
	if len(response.Choices) == 0 {
		if s.shouldLog() {
			s.logWarn(logger.ComponentExitPlanMode, logger.CategoryWarning, requestID, "ExitPlanMode LLM validation no response, conservative fallback", map[string]interface{}{
				"fallback_action": "allow_usage",
			})
		}
		return false, ""
	}

	content := strings.TrimSpace(strings.ToUpper(response.Choices[0].Message.Content))
	shouldBlock := strings.HasPrefix(content, "BLOCK")

	if shouldBlock {
		reason := "inappropriate usage detected by LLM analysis"
		if s.shouldLog() {
			s.logInfo(logger.ComponentExitPlanMode, logger.CategoryBlocked, requestID, "ExitPlanMode blocked by LLM analysis", map[string]interface{}{
				"analysis_result": strings.ToLower(content),
				"reason":          reason,
			})
		}
		return true, reason
	} else {
		if s.shouldLog() {
			s.logInfo(logger.ComponentExitPlanMode, logger.CategoryValidation, requestID, "ExitPlanMode allowed by LLM analysis", map[string]interface{}{
				"analysis_result": strings.ToLower(content),
			})
		}
		return false, ""
	}
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

// AnalyzeRequestContext analyzes the user request to determine if ExitPlanMode should be filtered out
// Returns (shouldFilter, error) where shouldFilter=true means ExitPlanMode should not be available
func (s *Service) AnalyzeRequestContext(ctx context.Context, userRequest string) (bool, error) {
	requestID := getRequestID(ctx)

	if !s.enabled {
		return false, nil // Skip analysis if correction service disabled
	}

	if strings.TrimSpace(userRequest) == "" {
		return false, nil // Don't filter for empty requests
	}

	if s.shouldLog() {
		s.logInfo(logger.ComponentExitPlanMode, logger.CategoryValidation, requestID, "Analyzing request context for ExitPlanMode filtering", map[string]interface{}{})
	}

	// Build analysis prompt
	prompt := fmt.Sprintf(`Analyze this user request: "%s"

ExitPlanMode creates implementation plans BEFORE starting work.

FILTER ExitPlanMode (respond "FILTER") for:
- Research: "read X", "analyze Y", "examine Z"  
- Information: "tell me about", "explain", "what is"
- Investigation: "check", "review", "investigate"

KEEP ExitPlanMode (respond "KEEP") for:  
- Implementation: "implement", "create", "build", "develop"
- Planning: "add feature", "make", "write code"

For mixed requests, consider PRIMARY intent.
Respond only "FILTER" or "KEEP".`, userRequest)

	// Create request to correction model
	req := types.OpenAIRequest{
		Model: s.modelName,
		Messages: []types.OpenAIMessage{
			{
				Role:    "system",
				Content: "You are a context analyzer for tool filtering. Respond with ONLY 'FILTER' if ExitPlanMode should be filtered out for this request type, or 'KEEP' if it should remain available.",
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
			s.logWarn(logger.ComponentExitPlanMode, logger.CategoryWarning, requestID, "Context analysis LLM failed, conservative fallback", map[string]interface{}{
				"error":           err.Error(),
				"fallback_action": "don't_filter",
			})
		}
		// Conservative fallback: don't filter if LLM fails
		return false, nil
	}

	// Parse response
	if len(response.Choices) == 0 {
		if s.shouldLog() {
			s.logWarn(logger.ComponentExitPlanMode, logger.CategoryWarning, requestID, "Context analysis LLM no response, conservative fallback", map[string]interface{}{
				"fallback_action": "don't_filter",
			})
		}
		return false, nil
	}

	content := strings.TrimSpace(strings.ToUpper(response.Choices[0].Message.Content))
	shouldFilter := strings.HasPrefix(content, "FILTER")

	if shouldFilter {
		if s.shouldLog() {
			s.logInfo(logger.ComponentExitPlanMode, logger.CategoryBlocked, requestID, "Context analysis: ExitPlanMode filtered out", map[string]interface{}{
				"reason":          "research/analysis detected",
				"analysis_result": strings.ToLower(content),
			})
		}
		return true, nil
	} else {
		if s.shouldLog() {
			s.logInfo(logger.ComponentExitPlanMode, logger.CategoryValidation, requestID, "Context analysis: ExitPlanMode kept available", map[string]interface{}{
				"reason":          "implementation detected",
				"analysis_result": strings.ToLower(content),
			})
		}
		return false, nil
	}
}
