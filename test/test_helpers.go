package test

import (
	"claude-proxy/circuitbreaker"
	"claude-proxy/config"
	"claude-proxy/types"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// stringPtr helper function for creating string pointers
// Shared across all test files to avoid duplication
func stringPtr(s string) *string {
	return &s
}

// getTestConfig helper function for creating test configuration
// Shared across all test files to avoid duplication
func getTestConfig() *config.Config {
	return &config.Config{
		HarmonyParsingEnabled: false,
		SkipTools:            []string{}, // No tools skipped by default
	}
}

// MockConfigProvider provides a mock ConfigProvider for testing
type MockConfigProvider struct {
	Endpoint string
}

func (m *MockConfigProvider) GetToolCorrectionEndpoint() string {
	return m.Endpoint
}

func (m *MockConfigProvider) GetHealthyToolCorrectionEndpoint() string {
	return m.Endpoint
}

func (m *MockConfigProvider) RecordEndpointFailure(endpoint string) {
	// Mock implementation - no-op for basic tests
}

func (m *MockConfigProvider) RecordEndpointSuccess(endpoint string) {
	// Mock implementation - no-op for basic tests
}

func (m *MockConfigProvider) GetEnableToolChoiceCorrection() bool {
	return true // Enable for unit tests as requested
}

// NewMockConfigProvider creates a test-optimized config using real LLM endpoints 
// For backward compatibility, accepts optional endpoint parameter (ignored)
func NewMockConfigProvider(endpoint ...string) *config.Config {
	// Load config from environment variables (this will use .env file)
	// Ignore any endpoint parameter - we use real LLM endpoints from environment
	cfg, err := config.LoadConfigWithEnv()
	if err != nil {
		// Fallback to hardcoded values if env loading fails
		cfg = &config.Config{
			ToolCorrectionEndpoints: []string{"http://192.168.0.46:11434/v1/chat/completions", "http://192.168.0.50:11434/v1/chat/completions"},
			ToolCorrectionAPIKey: "ollama",
			CorrectionModel: "qwen2.5-coder:latest",
			EnableToolChoiceCorrection: true, // Enable for unit tests
			HealthManager: circuitbreaker.NewHealthManager(getTestCircuitBreakerConfig()),
		}
	}
	
	// Ensure we have the tool correction settings
	if len(cfg.ToolCorrectionEndpoints) == 0 {
		cfg.ToolCorrectionEndpoints = []string{"http://192.168.0.46:11434/v1/chat/completions", "http://192.168.0.50:11434/v1/chat/completions"}
	}
	if cfg.ToolCorrectionAPIKey == "" {
		cfg.ToolCorrectionAPIKey = "ollama"
	}
	if cfg.CorrectionModel == "" {
		cfg.CorrectionModel = "qwen2.5-coder:latest"
	}
	
	// OPTIMIZATION FOR TESTS: Reorder endpoints to put healthy ones first
	cfg.ToolCorrectionEndpoints = reorderEndpointsByHealth(cfg.ToolCorrectionEndpoints)
	
	// Use test-optimized circuit breaker settings for faster failover
	if cfg.HealthManager == nil {
		cfg.HealthManager = circuitbreaker.NewHealthManager(getTestCircuitBreakerConfig())
	}
	
	// No tools skipped by default for testing
	cfg.SkipTools = []string{}
	
	// FORCE ENABLE tool choice correction for unit tests regardless of .env file
	cfg.EnableToolChoiceCorrection = true
	
	return cfg
}

// Legacy function name - redirects to new implementation  
func NewMockConfigProviderLegacy(endpoint string) *MockConfigProvider {
	return &MockConfigProvider{Endpoint: endpoint}
}

// Backward compatibility function - ignores the endpoint and uses real LLM  
func NewMockConfigProviderWithEndpoint(endpoint string) *config.Config {
	// Ignore the provided endpoint and use real LLM from environment
	return NewMockConfigProvider()
}

// createMockLLMValidationServer creates a mock server that simulates intelligent LLM validation
func createMockLLMValidationServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req types.OpenAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Logf("Failed to decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		
		// Extract the validation prompt content - look for plan content in the entire prompt
		fullPrompt := ""
		if len(req.Messages) > 1 {
			fullPrompt = req.Messages[1].Content
		}
		
		// Log the full prompt for debugging
		t.Logf("Mock LLM received prompt: %s", fullPrompt)
		
		// Extract just the PLAN CONTENT section to analyze
		planContent := ""
		if strings.Contains(fullPrompt, "PLAN CONTENT:") {
			lines := strings.Split(fullPrompt, "\n")
			inPlanContent := false
			for _, line := range lines {
				if strings.TrimSpace(line) == "PLAN CONTENT:" {
					inPlanContent = true
					continue
				}
				if inPlanContent && strings.HasPrefix(line, "CONVERSATION CONTEXT:") {
					break
				}
				if inPlanContent {
					planContent += line + "\n"
				}
			}
		}
		
		// Clean up the plan content (remove quotes)
		planContent = strings.Trim(planContent, "\" \n")
		t.Logf("Mock LLM analyzing plan content: %s", planContent)
		
		// Simulate intelligent LLM analysis based on the actual plan content only
		var decision string
		if strings.Contains(planContent, "Integration Verification Report") ||
		   strings.Contains(planContent, "Analysis Complete") ||
		   strings.Contains(planContent, "implementation has been completed") ||
		   strings.Contains(planContent, "all tasks completed") ||
		   strings.Contains(planContent, "✅") ||
		   strings.Contains(planContent, "Summary of changes") ||
		   strings.Contains(planContent, "analysis is complete") ||
		   strings.Contains(planContent, "The integration is complete") {
			decision = "BLOCK"
			t.Logf("Mock LLM decision: BLOCK (detected completion language in plan)")
		} else if strings.Contains(planContent, "I will implement") ||
				  strings.Contains(planContent, "implementation plan") ||
				  strings.Contains(planContent, "following approach") ||
				  strings.Contains(planContent, "Here's my approach") {
			decision = "ALLOW"
			t.Logf("Mock LLM decision: ALLOW (detected planning language in plan)")
		} else {
			// Default to ALLOW for ambiguous cases
			decision = "ALLOW"
			t.Logf("Mock LLM decision: ALLOW (default for ambiguous case)")
		}
		
		// Return LLM response
		response := types.OpenAIResponse{
			Choices: []types.OpenAIChoice{
				{
					Message: types.OpenAIMessage{
						Role:    "assistant",
						Content: decision,
					},
				},
			},
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
}

// createMockAlwaysYesServer creates a mock server that always returns "YES" for tool necessity
func createMockAlwaysYesServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req types.OpenAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Logf("Failed to decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		
		// Extract the user request from the prompt
		userPrompt := ""
		if len(req.Messages) > 1 {
			userPrompt = req.Messages[1].Content
		}
		
		// Simulate improved LLM analysis
		var decision string
		if strings.Contains(userPrompt, "read architecture md and check recent changes") ||
		   strings.Contains(userPrompt, "analyze the codebase and explain") ||
		   strings.Contains(userPrompt, "review the code and tell me") ||
		   strings.Contains(userPrompt, "examine") && strings.Contains(userPrompt, "and provide") {
			decision = "NO" // These are analysis requests - should not force tools
			t.Logf("Mock tool necessity: NO (detected analysis/research request)")
		} else if strings.Contains(userPrompt, "Implement") ||
				  strings.Contains(userPrompt, "create") ||
				  strings.Contains(userPrompt, "write") ||
				  strings.Contains(userPrompt, "edit") {
			decision = "YES" // These clearly require tools
			t.Logf("Mock tool necessity: YES (detected implementation request)")
		} else {
			decision = "YES" // Default to YES to demonstrate the old behavior
			t.Logf("Mock tool necessity: YES (default/old behavior)")
		}
		
		response := types.OpenAIResponse{
			Choices: []types.OpenAIChoice{
				{
					Message: types.OpenAIMessage{
						Role:    "assistant",
						Content: decision,
					},
				},
			},
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
}
// getTestCircuitBreakerConfig returns test-optimized circuit breaker settings
// Fails fast to avoid long waits in unit tests
func getTestCircuitBreakerConfig() circuitbreaker.Config {
	return circuitbreaker.Config{
		FailureThreshold:   1,                       // Open circuit after 1 failure (not 2)
		BackoffDuration:    100 * time.Millisecond,  // Very short backoff (100ms)
		MaxBackoffDuration: 1 * time.Second,         // Max 1s wait (not 30s)
		ResetTimeout:       1 * time.Minute,         // Reset failure count after 1min of success
	}
}

// GetStandardTestTool returns a standardized tool definition using centralized schemas
// This ensures consistent tool definitions across all tests and reduces duplication
func GetStandardTestTool(toolName string) types.Tool {
	// Use centralized fallback schema for consistency
	tool := types.GetFallbackToolSchema(toolName)
	if tool == nil {
		panic("Unknown tool requested in test: " + toolName + ". Use types.GetFallbackToolSchema to check available tools.")
	}
	return *tool
}

// GetStandardTestTools returns commonly used test tools with consistent schemas
func GetStandardTestTools() []types.Tool {
	return []types.Tool{
		GetStandardTestTool("Read"),
		GetStandardTestTool("Write"),
		GetStandardTestTool("Edit"),
		GetStandardTestTool("Bash"),
		GetStandardTestTool("Grep"),
		GetStandardTestTool("LS"),
		GetStandardTestTool("Glob"),
		GetStandardTestTool("Task"),
		GetStandardTestTool("TodoWrite"),
		GetStandardTestTool("ExitPlanMode"),
	}
}

// reorderEndpointsByHealth performs quick health checks and puts working endpoints first
// This optimizes test performance by avoiding known-bad endpoints
func reorderEndpointsByHealth(endpoints []string) []string {
	if len(endpoints) <= 1 {
		return endpoints
	}

	healthy := []string{}
	unhealthy := []string{}
	
	// Quick health check with very short timeout
	for _, endpoint := range endpoints {
		if isEndpointQuickHealthy(endpoint) {
			healthy = append(healthy, endpoint)
		} else {
			unhealthy = append(unhealthy, endpoint)
		}
	}
	
	// Return healthy endpoints first, then unhealthy as fallback
	result := append(healthy, unhealthy...)
	
	// Log the reordering for debugging
	if len(healthy) > 0 && len(unhealthy) > 0 {
		// Note: using println instead of log to avoid import cycle in tests
		println("🎯 Test optimization: Reordered endpoints - healthy first:", len(healthy), "working,", len(unhealthy), "failing")
	}
	
	return result
}

// isEndpointQuickHealthy performs a very fast health check (2 second timeout)
func isEndpointQuickHealthy(endpoint string) bool {
	client := &http.Client{
		Timeout: 2 * time.Second, // Very short timeout for test optimization
	}
	
	// Try a simple HEAD request to see if endpoint is responsive
	req, err := http.NewRequest("HEAD", endpoint, nil)
	if err != nil {
		return false
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	
	// Consider any response (even errors) as "healthy" since the endpoint is reachable
	// The actual LLM validation will handle authentication/format issues
	return true
}

// Centralized tool definitions for consistent test usage
// These use the actual Claude Code tool definitions from types.GetFallbackToolSchema()
// Will panic if tool definitions are not found (intentional - no fallbacks)

func CreateReadTool() types.Tool {
	tool := types.GetFallbackToolSchema("read")
	if tool == nil {
		panic("Read tool definition not found in types.GetFallbackToolSchema()")
	}
	return *tool
}

func CreateWriteTool() types.Tool {
	tool := types.GetFallbackToolSchema("write")
	if tool == nil {
		panic("Write tool definition not found in types.GetFallbackToolSchema()")
	}
	return *tool
}

func CreateEditTool() types.Tool {
	tool := types.GetFallbackToolSchema("edit")
	if tool == nil {
		panic("Edit tool definition not found in types.GetFallbackToolSchema()")
	}
	return *tool
}

func CreateGrepTool() types.Tool {
	tool := types.GetFallbackToolSchema("grep")
	if tool == nil {
		panic("Grep tool definition not found in types.GetFallbackToolSchema()")
	}
	return *tool
}

func CreateGlobTool() types.Tool {
	tool := types.GetFallbackToolSchema("glob")
	if tool == nil {
		panic("Glob tool definition not found in types.GetFallbackToolSchema()")
	}
	return *tool
}

func CreateLSTool() types.Tool {
	tool := types.GetFallbackToolSchema("ls")
	if tool == nil {
		panic("LS tool definition not found in types.GetFallbackToolSchema()")
	}
	return *tool
}

func CreateBashTool() types.Tool {
	tool := types.GetFallbackToolSchema("bash")
	if tool == nil {
		panic("Bash tool definition not found in types.GetFallbackToolSchema()")
	}
	return *tool
}

func CreateExitPlanModeTool() types.Tool {
	tool := types.GetFallbackToolSchema("exitplanmode")
	if tool == nil {
		panic("ExitPlanMode tool definition not found in types.GetFallbackToolSchema()")
	}
	return *tool
}

// Helper functions for common tool combinations

func CreateResearchTools() []types.Tool {
	return []types.Tool{
		CreateReadTool(),
		CreateGrepTool(),
		CreateGlobTool(),
		CreateLSTool(),
		CreateExitPlanModeTool(),
	}
}

func CreateImplementationTools() []types.Tool {
	return []types.Tool{
		CreateReadTool(),
		CreateWriteTool(), 
		CreateEditTool(),
		CreateBashTool(),
		CreateExitPlanModeTool(),
	}
}

func CreateAllTools() []types.Tool {
	return []types.Tool{
		CreateReadTool(),
		CreateWriteTool(),
		CreateEditTool(),
		CreateGrepTool(),
		CreateGlobTool(),
		CreateLSTool(),
		CreateBashTool(),
		CreateExitPlanModeTool(),
	}
}