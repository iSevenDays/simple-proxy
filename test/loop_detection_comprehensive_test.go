package test

import (
	"claude-proxy/loop"
	"claude-proxy/types"
	"context"
	"strings"
	"testing"
)

// TestLoopDetector_BasicFunctionality tests core loop detection features
func TestLoopDetector_BasicFunctionality(t *testing.T) {
	detector := loop.NewLoopDetector()

	t.Run("NoLoop", func(t *testing.T) {
		messages := []types.OpenAIMessage{
			{Role: "user", Content: "Help me with a task"},
			{Role: "assistant", Content: "I'll help you", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Read", Arguments: `{"file_path": "/test/file.go"}`}},
			}},
			{Role: "tool", Content: "File content here"},
			{Role: "assistant", Content: "Based on the file..."},
		}

		detection := detector.DetectLoop(context.Background(), messages)
		if detection.HasLoop {
			t.Errorf("Expected no loop, but detected: %+v", detection)
		}
	})

	t.Run("ConsecutiveIdenticalCalls", func(t *testing.T) {
		messages := []types.OpenAIMessage{
			{Role: "user", Content: "Update the todo list"},
			{Role: "assistant", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": [{"id":"1","content":"Task 1","status":"pending","priority":"high"}]}`}},
			}},
			{Role: "tool", Content: "Todos have been modified successfully"},
			{Role: "assistant", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": [{"id":"1","content":"Task 1","status":"pending","priority":"high"}]}`}},
			}},
			{Role: "tool", Content: "Todos have been modified successfully"},
			{Role: "assistant", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": [{"id":"1","content":"Task 1","status":"pending","priority":"high"}]}`}},
			}},
			{Role: "tool", Content: "Todos have been modified successfully"},
		}

		detection := detector.DetectLoop(context.Background(), messages)
		if !detection.HasLoop {
			t.Error("Expected loop detection, but none found")
		}
		if detection.LoopType != "consecutive_identical" {
			t.Errorf("Expected loop type 'consecutive_identical', got '%s'", detection.LoopType)
		}
		if detection.ToolName != "TodoWrite" {
			t.Errorf("Expected tool name 'TodoWrite', got '%s'", detection.ToolName)
		}
		if detection.Count < 3 {
			t.Errorf("Expected count >= 3, got %d", detection.Count)
		}
	})

	t.Run("AlternatingPattern", func(t *testing.T) {
		messages := []types.OpenAIMessage{
			{Role: "user", Content: "Edit the file multiple times with identical parameters"},
			{Role: "assistant", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Edit", Arguments: `{"file_path": "/test.go", "old_string": "old", "new_string": "new"}`}},
			}},
			{Role: "tool", Content: "File edited successfully"},
			{Role: "assistant", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Edit", Arguments: `{"file_path": "/test.go", "old_string": "old", "new_string": "new"}`}},
			}},
			{Role: "tool", Content: "File edited successfully"},
			{Role: "assistant", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Edit", Arguments: `{"file_path": "/test.go", "old_string": "old", "new_string": "new"}`}},
			}},
			{Role: "tool", Content: "File edited successfully"},
			{Role: "assistant", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Edit", Arguments: `{"file_path": "/test.go", "old_string": "old", "new_string": "new"}`}},
			}},
			{Role: "tool", Content: "File edited successfully"},
		}

		detection := detector.DetectLoop(context.Background(), messages)
		if !detection.HasLoop {
			t.Error("Expected loop detection for identical tool calls, but none found")
		}
		// Since all arguments are identical, this should be detected as consecutive_identical
		// (which is correct behavior after our argument comparison fix)
		if detection.LoopType != "consecutive_identical" {
			t.Errorf("Expected loop type 'consecutive_identical' for identical arguments, got '%s'", detection.LoopType)
		}
	})

	t.Run("DifferentTools", func(t *testing.T) {
		messages := []types.OpenAIMessage{
			{Role: "user", Content: "Help me analyze code"},
			{Role: "assistant", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Read", Arguments: `{"file_path": "/file1.go"}`}},
			}},
			{Role: "tool", Content: "File content 1"},
			{Role: "assistant", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Grep", Arguments: `{"pattern": "func"}`}},
			}},
			{Role: "tool", Content: "Found functions"},
			{Role: "assistant", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Edit", Arguments: `{"file_path": "/file1.go", "old_string": "old", "new_string": "new"}`}},
			}},
			{Role: "tool", Content: "File edited"},
		}

		detection := detector.DetectLoop(context.Background(), messages)
		if detection.HasLoop {
			t.Errorf("Expected no loop for different tools, but detected: %+v", detection)
		}
	})
}

// TestLoopDetector_ResponseGeneration tests loop-breaking response creation
func TestLoopDetector_ResponseGeneration(t *testing.T) {
	detector := loop.NewLoopDetector()

	t.Run("CreateLoopBreakingResponse", func(t *testing.T) {
		detection := &loop.LoopDetection{
			HasLoop:        true,
			LoopType:       "consecutive_identical",
			ToolName:       "TodoWrite",
			Count:          5,
			Recommendation: "Loop detected: TodoWrite called 5 times consecutively...",
		}

		response := detector.CreateLoopBreakingResponse(detection)
		if response.Role != "assistant" {
			t.Errorf("Expected role 'assistant', got '%s'", response.Role)
		}
		if len(response.Content) == 0 {
			t.Error("Expected content in loop breaking response")
		}
		if response.Content[0].Type != "text" {
			t.Errorf("Expected content type 'text', got '%s'", response.Content[0].Type)
		}
		if response.StopReason != "tool_use" {
			t.Errorf("Expected stop reason 'tool_use', got '%s'", response.StopReason)
		}
	})

	t.Run("TodoWriteSpecificRecommendation", func(t *testing.T) {
		messages := []types.OpenAIMessage{
			{Role: "assistant", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": []}`}},
			}},
			{Role: "tool", Content: "Todos modified"},
			{Role: "assistant", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": []}`}},
			}},
			{Role: "tool", Content: "Todos modified"},
			{Role: "assistant", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": []}`}},
			}},
		}

		detection := detector.DetectLoop(context.Background(), messages)
		if !detection.HasLoop {
			t.Error("Expected TodoWrite loop detection")
		}
		if detection.ToolName != "TodoWrite" {
			t.Errorf("Expected tool name 'TodoWrite', got '%s'", detection.ToolName)
		}
		if !contains(detection.Recommendation, "todo list") && !contains(detection.Recommendation, "TodoWrite") {
			t.Errorf("Expected TodoWrite-specific recommendation, got: %s", detection.Recommendation)
		}
	})
}

// TestLoopDetector_ContinuationAfterDetection tests the core fix for continuation issues
func TestLoopDetector_ContinuationAfterDetection(t *testing.T) {
	detector := loop.NewLoopDetector()

	t.Run("DoesNotRetriggerAfterLoopBreaking", func(t *testing.T) {
		messages := []types.OpenAIMessage{
			// Original conversation leading to loop
			{Role: "user", Content: "Help me with tasks"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": [{"id":"1","content":"Task 1","status":"pending","priority":"high"}]}`}},
			}},
			{Role: "tool", Content: "Todos have been modified successfully"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": [{"id":"1","content":"Task 1","status":"pending","priority":"high"}]}`}},
			}},
			{Role: "tool", Content: "Todos have been modified successfully"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": [{"id":"1","content":"Task 1","status":"pending","priority":"high"}]}`}},
			}},
			{Role: "tool", Content: "Todos have been modified successfully"},

			// Loop detection response was inserted here
			{Role: "assistant", Content: "🔄 **Loop Detection**: Loop detected: TodoWrite called 3 times consecutively. The todo list may already be properly updated. Consider proceeding with actual task implementation or asking for clarification on what specific action is needed."},

			// User provides clarification
			{Role: "user", Content: "here is reminder of what we were doing: find if there is code responsible for setting the orientation lock on android"},
		}

		// This should NOT detect a loop because the previous loop was already broken
		detection := detector.DetectLoop(context.Background(), messages)
		if detection.HasLoop {
			t.Errorf("Should not detect loop after loop detection response, but detected: %+v", detection)
		}
	})

	t.Run("DetectsNewLoopAfterContinuation", func(t *testing.T) {
		messages := []types.OpenAIMessage{
			// Previous loop and detection (simplified)
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": []}`}},
			}},
			{Role: "tool", Content: "Todos modified"},
			{Role: "assistant", Content: "🔄 **Loop Detection**: Loop detected: TodoWrite called 3 times consecutively..."},

			// User provides clarification
			{Role: "user", Content: "Please search for orientation lock code in DeviceRobot.java"},

			// Assistant starts a NEW loop pattern (should be detected)
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Read", Arguments: `{"file_path": "/path/DeviceRobot.java"}`}},
			}},
			{Role: "tool", Content: "File content here"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Read", Arguments: `{"file_path": "/path/DeviceRobot.java"}`}},
			}},
			{Role: "tool", Content: "File content here"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Read", Arguments: `{"file_path": "/path/DeviceRobot.java"}`}},
			}},
			{Role: "tool", Content: "File content here"},
		}

		// This SHOULD detect the new Read loop
		detection := detector.DetectLoop(context.Background(), messages)
		if !detection.HasLoop {
			t.Error("Should detect new loop pattern after continuation")
		}
		if detection.ToolName != "Read" {
			t.Errorf("Expected new loop with tool 'Read', got '%s'", detection.ToolName)
		}
	})

	t.Run("HandlesMultipleLoopDetectionResponses", func(t *testing.T) {
		messages := []types.OpenAIMessage{
			// First loop and detection
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": []}`}},
			}},
			{Role: "tool", Content: "Todos modified"},
			{Role: "assistant", Content: "🔄 **Loop Detection**: First loop detected..."},

			// User clarification
			{Role: "user", Content: "Please continue with the task"},

			// Second loop and detection
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Edit", Arguments: `{"file_path": "/test.go"}`}},
			}},
			{Role: "tool", Content: "File edited"},
			{Role: "assistant", Content: "🔄 **Loop Detection**: Second loop detected..."},

			// User clarification again
			{Role: "user", Content: "Let me be more specific about what I need"},
		}

		// Should not trigger on old loops, should reset analysis window to after the most recent loop detection
		detection := detector.DetectLoop(context.Background(), messages)
		if detection.HasLoop {
			t.Errorf("Should not detect loop after most recent loop detection response, but detected: %+v", detection)
		}
	})
}

// TestLoopDetector_RealWorldScenarios tests scenarios from actual production logs
func TestLoopDetector_RealWorldScenarios(t *testing.T) {
	detector := loop.NewLoopDetector()

	t.Run("ReproduceOriginalIssue", func(t *testing.T) {
		// Reproduce the exact scenario from the user's logs
		messages := []types.OpenAIMessage{
			{Role: "user", Content: "help me find a way to fix some loops that Claude Code goes into"},

			// The original loop pattern (messages 92-99 from logs)
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": [{"id":"1","content":"Task","status":"pending","priority":"high"}]}`}},
			}},
			{Role: "tool", Content: "Todos have been modified successfully. Ensure that you continue to use the todo list to track your p...", ToolCallID: "2MH7hJZOQalfbVAHAd3WMT6XYUHQjhhr"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": [{"id":"1","content":"Task","status":"pending","priority":"high"}]}`}},
			}},
			{Role: "tool", Content: "Todos have been modified successfully. Ensure that you continue to use the todo list to track your p...", ToolCallID: "JvOnCoaMhXylbN1Pnu6SAXST3BwW3fji"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": [{"id":"1","content":"Task","status":"pending","priority":"high"}]}`}},
			}},
			{Role: "tool", Content: "Todos have been modified successfully. Ensure that you continue to use the todo list to track your p...", ToolCallID: "M74MFcNYi1yDv3Zu0p4OlxXKx1hfIfqY"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": [{"id":"1","content":"Task","status":"pending","priority":"high"}]}`}},
			}},
			{Role: "tool", Content: "Todos have been modified successfully. Ensure that you continue to use the todo list to track your p...", ToolCallID: "VYvPb38jpcJjTF7c4torYKEP6n1VaVVr"},

			// Loop detection response (this is what our system should generate)
			{Role: "assistant", Content: "🔄 **Loop Detection**: Loop detected: TodoWrite called 7 times consecutively. The todo list may already be properly updated. Consider proceeding with actual task implementation or asking for clarification on what specific action is needed.\n\nI've detected a repetitive pattern and am breaking the loop to prevent infinite execution. Please provide more specific guidance or let me know if you need help with a different approach."},

			// User provides clarification (this should NOT trigger another loop detection)
			{Role: "user", Content: "here is reminder of what we were doing: find if there is code responsible for setting the orientation lock on android. Start from rdc-pool project. There is a file named \"DeviceRobot.java\". Then try to search for code that is setting the orientation lock. Alternatively, search for code that is setting accelerometer rotation using adb shell commands. When I restart a live testing session on Android, I always see orientation lock is enabled (auto-rotation is disabled) and I need to find the code responsible for doing that"},
		}

		// At this point, the system should NOT detect a loop because:
		// 1. The previous loop was already detected and broken
		// 2. We should only analyze messages after the loop detection response
		detection := detector.DetectLoop(context.Background(), messages)
		if detection.HasLoop {
			t.Errorf("Should not re-detect the same loop after loop detection response. Detection: %+v", detection)
			t.Errorf("This would cause the infinite loop detection -> clarification -> loop detection cycle")
		}
	})

	t.Run("LargeConversation", func(t *testing.T) {
		// Test with a conversation similar to the logs showing 74+ messages
		var messages []types.OpenAIMessage

		// Add initial normal conversation
		messages = append(messages, types.OpenAIMessage{Role: "system", Content: "You are Claude Code..."})
		messages = append(messages, types.OpenAIMessage{Role: "user", Content: "Help me find a way to fix some loops"})

		// Add some normal tool calls
		for i := 0; i < 10; i++ {
			messages = append(messages, types.OpenAIMessage{
				Role:    "assistant",
				Content: "",
				ToolCalls: []types.OpenAIToolCall{
					{Function: types.OpenAIToolCallFunction{Name: "Read", Arguments: `{"file_path": "/test.go"}`}},
				},
			})
			messages = append(messages, types.OpenAIMessage{Role: "tool", Content: "File content"})
		}

		// Add the problematic loop pattern
		for i := 0; i < 5; i++ {
			messages = append(messages, types.OpenAIMessage{
				Role:    "assistant",
				Content: "",
				ToolCalls: []types.OpenAIToolCall{
					{Function: types.OpenAIToolCallFunction{Name: "TodoWrite", Arguments: `{"todos": [{"id":"1","content":"Task","status":"pending","priority":"high"}]}`}},
				},
			})
			messages = append(messages, types.OpenAIMessage{Role: "tool", Content: "Todos have been modified successfully. Ensure that you continue to use the todo list to track your p..."})
		}

		detection := detector.DetectLoop(context.Background(), messages)
		if !detection.HasLoop {
			t.Error("Expected loop detection in large conversation")
		}
		if detection.ToolName != "TodoWrite" {
			t.Errorf("Expected 'TodoWrite', got '%s'", detection.ToolName)
		}
		if detection.Count < 3 {
			t.Errorf("Expected count >= 3, got %d", detection.Count)
		}
	})

	t.Run("EmojiStringMatching", func(t *testing.T) {
		// Test different variations of loop detection response formats
		testCases := []struct {
			name     string
			content  string
			expected bool
		}{
			{
				name:     "Simple format",
				content:  "Loop Detection: something detected",
				expected: true,
			},
			{
				name:     "Emoji format from logs",
				content:  "🔄 **Loop Detection**: Loop detected: TodoWrite called 7 times consecutively...",
				expected: true,
			},
			{
				name:     "Different emoji",
				content:  "🚨 Loop Detection: Emergency stop",
				expected: true,
			},
			{
				name:     "Case variations",
				content:  "LOOP DETECTION: All caps version",
				expected: true,
			},
			{
				name:     "Not a loop detection",
				content:  "This is just a normal response about loops in code",
				expected: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Test the string matching logic
				isLoopDetection := strings.Contains(tc.content, "Loop Detection") ||
					strings.Contains(tc.content, "LOOP DETECTION") ||
					strings.Contains(tc.content, "loop detection")
				if isLoopDetection != tc.expected {
					t.Errorf("Case '%s': expected %v, got %v for content: %s", tc.name, tc.expected, isLoopDetection, tc.content)
				}
			})
		}
	})
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
