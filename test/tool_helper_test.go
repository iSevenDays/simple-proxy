package test

import (
	"testing"
)

// TestGetStandardTestTool verifies the standardized tool helper works correctly
func TestGetStandardTestTool(t *testing.T) {
	t.Run("ValidTool", func(t *testing.T) {
		tool := GetStandardTestTool("Read")
		if tool.Name != "Read" {
			t.Errorf("Expected Read tool, got %s", tool.Name)
		}
		if tool.InputSchema.Type != "object" {
			t.Errorf("Expected object schema type, got %s", tool.InputSchema.Type)
		}
		if len(tool.InputSchema.Properties) == 0 {
			t.Error("Expected Read tool to have properties")
		}
	})
	
	t.Run("PanicsOnUnknownTool", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for unknown tool, but didn't panic")
			}
		}()
		GetStandardTestTool("NonExistentTool")
	})
}

// TestGetStandardTestTools verifies the standard tool set
func TestGetStandardTestTools(t *testing.T) {
	tools := GetStandardTestTools()
	
	if len(tools) == 0 {
		t.Fatal("Expected standard tools, got empty slice")
	}
	
	// Check that all tools have proper schemas
	for _, tool := range tools {
		if tool.Name == "" {
			t.Error("Tool missing name")
		}
		if tool.InputSchema.Type == "" {
			t.Errorf("Tool %s missing schema type", tool.Name)
		}
	}
	
	// Check for essential tools
	essentialTools := []string{"Read", "Write", "Edit", "Bash", "ExitPlanMode"}
	for _, essential := range essentialTools {
		found := false
		for _, tool := range tools {
			if tool.Name == essential {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Missing essential tool: %s", essential)
		}
	}
}