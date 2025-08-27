package config

import (
	"os"
	"testing"
)

// TestHarmonyConfigurationDefaults tests that Harmony configuration has correct defaults
func TestHarmonyConfigurationDefaults(t *testing.T) {
	cfg := GetDefaultConfig()

	// Test default values
	if !cfg.HarmonyParsingEnabled {
		t.Error("Expected HarmonyParsingEnabled to be true by default")
	}
	if cfg.HarmonyDebug {
		t.Error("Expected HarmonyDebug to be false by default")
	}
	if cfg.HarmonyStrictMode {
		t.Error("Expected HarmonyStrictMode to be false by default")
	}

	// Test API methods
	if !cfg.IsHarmonyParsingEnabled() {
		t.Error("IsHarmonyParsingEnabled() should return true by default")
	}
	if cfg.IsHarmonyDebugEnabled() {
		t.Error("IsHarmonyDebugEnabled() should return false by default")
	}
	if cfg.IsHarmonyStrictModeEnabled() {
		t.Error("IsHarmonyStrictModeEnabled() should return false by default")
	}

	// Test combined getter
	enabled, debug, strict := cfg.GetHarmonyConfiguration()
	if !enabled || debug || strict {
		t.Errorf("GetHarmonyConfiguration() returned incorrect defaults: enabled=%v, debug=%v, strict=%v", enabled, debug, strict)
	}
}

// TestHarmonyEnvironmentVariables tests environment variable parsing
func TestHarmonyEnvironmentVariables(t *testing.T) {
	// Create a temporary .env file for testing
	envContent := `BIG_MODEL=gpt-4o
SMALL_MODEL=gpt-4o-mini
CORRECTION_MODEL=gpt-4o-mini
BIG_MODEL_ENDPOINT=https://api.openai.com/v1/chat/completions
SMALL_MODEL_ENDPOINT=https://api.openai.com/v1/chat/completions
TOOL_CORRECTION_ENDPOINT=https://api.openai.com/v1/chat/completions
BIG_MODEL_API_KEY=test-key
SMALL_MODEL_API_KEY=test-key
TOOL_CORRECTION_API_KEY=test-key
LOG_FULL_TOOLS=false
CONVERSATION_TRUNCATION=0
HARMONY_PARSING_ENABLED=false
HARMONY_DEBUG=true
HARMONY_STRICT_MODE=true
`

	// Write test .env file
	err := os.WriteFile(".env", []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test .env file: %v", err)
	}
	defer os.Remove(".env")

	// Load configuration
	cfg, err := LoadConfigWithEnv()
	if err != nil {
		t.Fatalf("LoadConfigWithEnv() failed: %v", err)
	}

	// Test that environment variables were parsed correctly
	if cfg.HarmonyParsingEnabled {
		t.Error("Expected HarmonyParsingEnabled to be false when HARMONY_PARSING_ENABLED=false")
	}
	if !cfg.HarmonyDebug {
		t.Error("Expected HarmonyDebug to be true when HARMONY_DEBUG=true")
	}
	if !cfg.HarmonyStrictMode {
		t.Error("Expected HarmonyStrictMode to be true when HARMONY_STRICT_MODE=true")
	}

	// Test API methods reflect the environment variables
	if cfg.IsHarmonyParsingEnabled() {
		t.Error("IsHarmonyParsingEnabled() should return false when disabled via env")
	}
	if !cfg.IsHarmonyDebugEnabled() {
		t.Error("IsHarmonyDebugEnabled() should return true when enabled via env")
	}
	if !cfg.IsHarmonyStrictModeEnabled() {
		t.Error("IsHarmonyStrictModeEnabled() should return true when enabled via env")
	}
}

// TestHarmonyEnvironmentVariableDefaults tests default behavior when env vars are missing
func TestHarmonyEnvironmentVariableDefaults(t *testing.T) {
	// Create a minimal .env file without Harmony settings
	envContent := `BIG_MODEL=gpt-4o
SMALL_MODEL=gpt-4o-mini
CORRECTION_MODEL=gpt-4o-mini
BIG_MODEL_ENDPOINT=https://api.openai.com/v1/chat/completions
SMALL_MODEL_ENDPOINT=https://api.openai.com/v1/chat/completions
TOOL_CORRECTION_ENDPOINT=https://api.openai.com/v1/chat/completions
BIG_MODEL_API_KEY=test-key
SMALL_MODEL_API_KEY=test-key
TOOL_CORRECTION_API_KEY=test-key
LOG_FULL_TOOLS=false
CONVERSATION_TRUNCATION=0
`

	// Write test .env file
	err := os.WriteFile(".env", []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test .env file: %v", err)
	}
	defer os.Remove(".env")

	// Load configuration
	cfg, err := LoadConfigWithEnv()
	if err != nil {
		t.Fatalf("LoadConfigWithEnv() failed: %v", err)
	}

	// Test that defaults are used when environment variables are not set
	if !cfg.HarmonyParsingEnabled {
		t.Error("Expected HarmonyParsingEnabled to default to true when not specified")
	}
	if cfg.HarmonyDebug {
		t.Error("Expected HarmonyDebug to default to false when not specified")
	}
	if cfg.HarmonyStrictMode {
		t.Error("Expected HarmonyStrictMode to default to false when not specified")
	}
}