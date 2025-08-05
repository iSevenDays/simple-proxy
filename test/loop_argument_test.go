package test

import (
	"claude-proxy/loop"
	"claude-proxy/types"
	"context"
	"testing"
)

func TestLoopDetector_ArgumentComparison(t *testing.T) {
	detector := loop.NewLoopDetector()

	t.Run("SameToolDifferentArguments_ShouldNotDetectLoop", func(t *testing.T) {
		// This represents a legitimate search workflow, not a loop
		messages := []types.OpenAIMessage{
			{Role: "user", Content: "Find all function definitions, class definitions, imports, and exports"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Grep", Arguments: `{"pattern": "function"}`}},
			}},
			{Role: "tool", Content: "Found 5 functions"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Grep", Arguments: `{"pattern": "class"}`}},
			}},
			{Role: "tool", Content: "Found 3 classes"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Grep", Arguments: `{"pattern": "import"}`}},
			}},
			{Role: "tool", Content: "Found 10 imports"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Grep", Arguments: `{"pattern": "export"}`}},
			}},
			{Role: "tool", Content: "Found 7 exports"},
		}

		detection := detector.DetectLoop(context.Background(), messages)
		if detection.HasLoop {
			t.Errorf("Should NOT detect loop for same tool with different arguments. This is legitimate search workflow.")
			t.Errorf("Detection: %+v", detection)
			t.Errorf("Tool calls: Grep(function) -> Grep(class) -> Grep(import) -> Grep(export)")
		}
	})

	t.Run("SameToolSameArguments_ShouldDetectLoop", func(t *testing.T) {
		// This represents a genuine loop - same grep pattern repeated
		messages := []types.OpenAIMessage{
			{Role: "user", Content: "Search for functions"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Grep", Arguments: `{"pattern": "function"}`}},
			}},
			{Role: "tool", Content: "Found 5 functions"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Grep", Arguments: `{"pattern": "function"}`}},
			}},
			{Role: "tool", Content: "Found 5 functions"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Grep", Arguments: `{"pattern": "function"}`}},
			}},
			{Role: "tool", Content: "Found 5 functions"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Grep", Arguments: `{"pattern": "function"}`}},
			}},
			{Role: "tool", Content: "Found 5 functions"},
		}

		detection := detector.DetectLoop(context.Background(), messages)
		if !detection.HasLoop {
			t.Error("Should detect loop for same tool with same arguments repeated")
		}
		if detection.ToolName != "Grep" {
			t.Errorf("Expected tool name 'Grep', got '%s'", detection.ToolName)
		}
	})

	t.Run("SameToolSlightlyDifferentArguments_ShouldNotDetectLoop", func(t *testing.T) {
		// Progressive file reading - legitimate workflow
		messages := []types.OpenAIMessage{
			{Role: "user", Content: "Read different parts of the file"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Read", Arguments: `{"file_path": "/test.go", "offset": 1, "limit": 50}`}},
			}},
			{Role: "tool", Content: "Lines 1-50"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Read", Arguments: `{"file_path": "/test.go", "offset": 51, "limit": 50}`}},
			}},
			{Role: "tool", Content: "Lines 51-100"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Read", Arguments: `{"file_path": "/test.go", "offset": 101, "limit": 50}`}},
			}},
			{Role: "tool", Content: "Lines 101-150"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Read", Arguments: `{"file_path": "/test.go", "offset": 151, "limit": 50}`}},
			}},
			{Role: "tool", Content: "Lines 151-200"},
		}

		detection := detector.DetectLoop(context.Background(), messages)
		if detection.HasLoop {
			t.Errorf("Should NOT detect loop for progressive file reading with different offsets")
			t.Errorf("Detection: %+v", detection)
			t.Errorf("This is legitimate pagination, not a loop")
		}
	})

	t.Run("DebugCurrentAlternatingDetection", func(t *testing.T) {
		// Debug the current alternating detection to see what it's actually comparing
		messages := []types.OpenAIMessage{
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Grep", Arguments: `{"pattern": "function"}`}},
			}},
			{Role: "tool", Content: "Found functions"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Grep", Arguments: `{"pattern": "class"}`}},
			}},
			{Role: "tool", Content: "Found classes"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Grep", Arguments: `{"pattern": "import"}`}},
			}},
			{Role: "tool", Content: "Found imports"},
			{Role: "assistant", Content: "", ToolCalls: []types.OpenAIToolCall{
				{Function: types.OpenAIToolCallFunction{Name: "Grep", Arguments: `{"pattern": "export"}`}},
			}},
			{Role: "tool", Content: "Found exports"},
		}

		detection := detector.DetectLoop(context.Background(), messages)

		t.Logf("Detection result: %+v", detection)
		t.Logf("This helps us understand what the current algorithm considers a 'loop'")

		// This test documents current behavior - we expect it to detect a false positive
		// because hasSimilarToolCalls only compares tool names, not arguments
		if detection.HasLoop && detection.LoopType == "alternating_pattern" {
			t.Logf("CONFIRMED: Current algorithm incorrectly detects alternating pattern based on tool name only")
			t.Logf("Arguments were: function -> class -> import -> export (all different, legitimate workflow)")
		}
	})
}
