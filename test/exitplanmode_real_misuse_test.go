package test

import (
	"claude-proxy/correction"
	"claude-proxy/internal"
	"claude-proxy/types"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExitPlanModeRealWorldMisuse tests the specific misuse case from the log:
// ExitPlanMode used as a completion summary after extensive research work
// This follows the TDD approach to reproduce and verify the bug fix
func TestExitPlanModeRealWorldMisuse(t *testing.T) {
	// Use real LLM endpoint from environment
	cfg := NewMockConfigProvider()
	service := correction.NewService(cfg, cfg.ToolCorrectionAPIKey, true, cfg.CorrectionModel, false)
	ctx := internal.WithRequestID(context.Background(), "req_2000")

	// Recreate the exact scenario from the log where ExitPlanMode was incorrectly used
	// The conversation shows extensive research work (LS, Grep, Read, Bash, Glob) 
	// followed by ExitPlanMode with a completion summary
	realWorldMisuseCase := struct {
		name              string
		toolCall          types.Content
		messages          []types.OpenAIMessage
		shouldBlock       bool
		expectedReason    string
		description       string
	}{
		name: "real_world_integration_analysis_misuse",
		toolCall: types.Content{
			Type: "tool_use",
			Name: "ExitPlanMode",
			Input: map[string]interface{}{
				"plan": "# Integration Verification Report\n\n## Summary\nI have examined the complete integration flow between ...The integration appears to be properly implemented and tested.\n\n## Key Integration Points Verified\n...\n\n## Architecture Summary\nThe system implements a clean three-tier architecture:\n...\n\nThe integration is complete and functional according to the examined code and tests.",
			},
		},
		messages: buildRealWorldAnalysisMessages(),
		shouldBlock: true,
		expectedReason: "inappropriate usage detected by LLM analysis", 
		description: "ExitPlanMode used as analysis completion summary should be blocked",
	}

	t.Run(realWorldMisuseCase.name, func(t *testing.T) {
		shouldBlock, reason := service.ValidateExitPlanMode(ctx, realWorldMisuseCase.toolCall, realWorldMisuseCase.messages)
		
		assert.Equal(t, realWorldMisuseCase.shouldBlock, shouldBlock, 
			"Test case %s: Expected shouldBlock=%v, got %v. %s", 
			realWorldMisuseCase.name, realWorldMisuseCase.shouldBlock, shouldBlock, realWorldMisuseCase.description)
		
		if realWorldMisuseCase.shouldBlock {
			assert.Contains(t, reason, realWorldMisuseCase.expectedReason,
				"Test case %s: Expected reason to contain '%s', got '%s'", 
				realWorldMisuseCase.name, realWorldMisuseCase.expectedReason, reason)
			assert.NotEmpty(t, reason,
				"Test case %s: Blocked calls should have a reason", realWorldMisuseCase.name)
		} else {
			assert.Empty(t, reason,
				"Test case %s: Allowed calls should not have a reason", realWorldMisuseCase.name)
		}
	})
}

// buildRealWorldAnalysisMessages recreates the conversation pattern from the log
// This represents a typical analysis/research task that should NOT conclude with ExitPlanMode
func buildRealWorldAnalysisMessages() []types.OpenAIMessage {
	messages := []types.OpenAIMessage{
		{Role: "user", Content: "read architecture md and check recent changes to tool correction related to exit plan mode"},
	}

	// Recreate the extensive research pattern from the log (46 messages total)
	// This includes multiple file reads, searches, and analysis commands
	researchActivities := []struct {
		toolName  string
		response  string
	}{
		{"LS", "Listed project directory contents"},
		{"Grep", "Found 64 files with installApp pattern"},
		{"Read", "Read IosInstaller.java implementation"},
		{"Read", "Read AppInstallationResource.java"},
		{"Read", "Read IOSControllerClient.java"},
		{"Read", "Read DeviceBridgeClient.java"},
		{"Glob", "Searched for device-controller files"},
		{"Glob", "Found restapi files"},
		{"Read", "Read app_endpoints.go"},
		{"Read", "Read IosInstallerTest.java"},
		{"Grep", "Searched for installApp.*udid pattern"},
		{"Grep", "Found /installApp endpoints"},
		{"Read", "Read routes.go"},
		{"Read", "Read handlers.go"},
		{"Bash", "Searched for Go files with installApp"},
		{"Read", "Attempted to read device_control.go (not found)"},
		{"Read", "Attempted to read device.go (not found)"},
		{"Glob", "Found devices/*.go files"},
		{"Read", "Read devices.go with 24639 characters"},
		{"Bash", "Searched for test files containing installApp"},
		{"Bash", "Found installapp_test.go"},
		{"Read", "Read installapp_test.go"},
	}

	// Build conversation with extensive research work
	for i, activity := range researchActivities {
		toolCallID := mustGenerateID(i + 1)
		
		// Add assistant message with tool call
		messages = append(messages, types.OpenAIMessage{
			Role:    "assistant",
			Content: "",
			ToolCalls: []types.OpenAIToolCall{
				{
					ID:   toolCallID,
					Type: "function",
					Function: types.OpenAIToolCallFunction{
						Name:      activity.toolName,
						Arguments: mustMarshalJSON(generateToolArgs(activity.toolName)),
					},
				},
			},
		})

		// Add tool response
		messages = append(messages, types.OpenAIMessage{
			Role:       "tool",
			Content:    activity.response,
			ToolCallID: toolCallID,
		})
	}

	return messages
}

// Helper to generate appropriate tool arguments for different tools
func generateToolArgs(toolName string) map[string]interface{} {
	switch toolName {
	case "LS":
		return map[string]interface{}{
			"path": "/Users/seven/projects/rdc-pool",
		}
	case "Grep":
		return map[string]interface{}{
			"pattern": "installApp",
		}
	case "Read":
		return map[string]interface{}{
			"file_path": "/path/to/file.java",
		}
	case "Glob":
		return map[string]interface{}{
			"pattern": "**/device-controller*",
		}
	case "Bash":
		return map[string]interface{}{
			"command": "find /Users/seven/projects -name \"*.go\" -exec grep -l \"installApp\" {} \\;",
		}
	default:
		return map[string]interface{}{
			"param": "value",
		}
	}
}

// Helper to generate unique tool call IDs similar to the log
func mustGenerateID(index int) string {
	ids := []string{
		"UsP7yrcNHmMG8XXPGjlvdQfW",
		"doahbkzClcvriuCn99XNaZBJ", 
		"88sSZYEH3ncG6lJHFrC14PjN",
		"C6QXOovw3XrrcjiNq3KilOEB",
		"My32fLtrSloUjdfJRxcnP7HP",
		"3eAdWZKOUTH2R5fQylVvBad1",
		"Es0w1sZjxBGGLGpsRS5IoumO",
		"htgpqG6vs1AhHyq3YR0osVYK",
		"1ttMluGka6vDy8tPeTUONpYm",
		"SZxkaqjiFxppDyU3HILAGkiN",
		"SeVR6enNsiYS5Tyvloi4CZ2H",
		"VMLBjTGLQbr8vTp86AYRtlqb",
		"qWsMyrAdG0w14W2JZoV2737P",
		"QRVr6HTtVNDUlPsonSiftpPK",
		"FwUePsyRe2C3lOB3LBzxt4aZ",
		"aKux6HTkEM8ICd72APmtodzr",
		"MEJo9d3m97FUREjeKPunwsbG",
		"9MfecapCZP6umHQoVX3yy6W2",
		"gqggTI8LYzJFqMwHZdAHl6ca",
		"FO4KwrFkZoyUewIxcqfIuArw",
		"UypPS915rzoCo1B7Wc7Eq70R",
		"PYoVPgmXBp7J0E30BcWJHMb",
	}
	
	if index <= len(ids) {
		return ids[index-1] + "nPtswACk"
	}
	return "GeneratedID" + mustMarshalJSON(index)
}

// TestExitPlanModeAnalysisPattern tests detection of analysis completion patterns
func TestExitPlanModeAnalysisPattern(t *testing.T) {
	// Use real LLM endpoint from environment
	cfg := NewMockConfigProvider()
	service := correction.NewService(cfg, cfg.ToolCorrectionAPIKey, true, cfg.CorrectionModel, false)
	ctx := internal.WithRequestID(context.Background(), "test-analysis-pattern")

	analysisPatternTests := []struct {
		name        string
		planContent string
		shouldBlock bool
		description string
	}{
		{
			name:        "integration_report_blocked",
			planContent: "# Integration Verification Report\n\n## Summary\nI have examined the complete integration flow... The integration appears to be properly implemented and tested.",
			shouldBlock: true,
			description: "Integration reports indicate completed analysis",
		},
		{
			name:        "architecture_summary_blocked", 
			planContent: "## Architecture Summary\nThe system implements a clean three-tier architecture... The integration is complete and functional according to the examined code.",
			shouldBlock: true,
			description: "Architecture summaries indicate completed analysis",
		},
		{
			name:        "examination_complete_blocked",
			planContent: "I have examined all the relevant files and verified the implementation. The analysis is complete.",
			shouldBlock: true,
			description: "Examination completion language should be blocked",
		},
		{
			name:        "analysis_findings_blocked",
			planContent: "Based on my analysis of the codebase, here are my findings... The review is now complete.",
			shouldBlock: true,
			description: "Analysis findings with completion should be blocked",
		},
		{
			name:        "future_analysis_allowed",
			planContent: "I will analyze the codebase using the following approach:\n1. Examine architecture documents\n2. Review implementation patterns\n3. Identify integration points",
			shouldBlock: false,
			description: "Future analysis plans should be allowed",
		},
		{
			name:        "investigation_plan_allowed",
			planContent: "Here's my investigation strategy:\n1. Start with configuration files\n2. Trace request flow\n3. Validate error handling",
			shouldBlock: false,
			description: "Investigation plans should be allowed",
		},
	}

	for _, tt := range analysisPatternTests {
		t.Run(tt.name, func(t *testing.T) {
			toolCall := types.Content{
				Type: "tool_use",
				Name: "ExitPlanMode",
				Input: map[string]interface{}{
					"plan": tt.planContent,
				},
			}

			// Minimal context for content-focused testing
			messages := []types.OpenAIMessage{
				{Role: "user", Content: "Analyze the system architecture and integration patterns"},
			}

			shouldBlock, _ := service.ValidateExitPlanMode(ctx, toolCall, messages)
			
			assert.Equal(t, tt.shouldBlock, shouldBlock,
				"Test case %s: Expected shouldBlock=%v, got %v. %s",
				tt.name, tt.shouldBlock, shouldBlock, tt.description)
		})
	}
}

// TestExitPlanModeResearchContextPattern tests context-based detection after research activities
func TestExitPlanModeResearchContextPattern(t *testing.T) {
	// Use real LLM endpoint from environment
	cfg := NewMockConfigProvider()
	service := correction.NewService(cfg, cfg.ToolCorrectionAPIKey, true, cfg.CorrectionModel, false)
	ctx := internal.WithRequestID(context.Background(), "test-research-context")

	researchContextTests := []struct {
		name         string
		messages     []types.OpenAIMessage
		shouldBlock  bool
		description  string
	}{
		{
			name:        "extensive_research_analysis_blocked",
			messages:    buildRealWorldAnalysisMessages(), // 46 messages of research work
			shouldBlock: true,
			description: "ExitPlanMode after extensive research should be blocked",
		},
		{
			name: "minimal_research_planning_allowed",
			messages: []types.OpenAIMessage{
				{Role: "user", Content: "Help me understand this codebase"},
				{
					Role:    "assistant",
					Content: "",
					ToolCalls: []types.OpenAIToolCall{
						{
							ID:   "read-001",
							Type: "function",
							Function: types.OpenAIToolCallFunction{
								Name:      "Read",
								Arguments: mustMarshalJSON(map[string]interface{}{"file_path": "README.md"}),
							},
						},
					},
				},
				{Role: "tool", Content: "File contents", ToolCallID: "read-001"},
			},
			shouldBlock: false,
			description: "ExitPlanMode after minimal research should be allowed for planning",
		},
	}

	for _, tt := range researchContextTests {
		t.Run(tt.name, func(t *testing.T) {
			// Use completion-style content for blocked cases, planning content for allowed cases  
			var planContent string
			if tt.shouldBlock {
				planContent = "# Analysis Complete\n\nI have thoroughly examined the codebase and can confirm the integration is properly implemented. The analysis is finished."
			} else {
				planContent = "Based on my initial review, here's my implementation plan:\n1. Study the current patterns\n2. Design the new feature\n3. Implement incrementally"
			}

			toolCall := types.Content{
				Type: "tool_use", 
				Name: "ExitPlanMode",
				Input: map[string]interface{}{
					"plan": planContent,
				},
			}

			shouldBlock, _ := service.ValidateExitPlanMode(ctx, toolCall, tt.messages)
			
			assert.Equal(t, tt.shouldBlock, shouldBlock,
				"Test case %s: Expected shouldBlock=%v, got %v. %s",
				tt.name, tt.shouldBlock, shouldBlock, tt.description)
		})
	}
}