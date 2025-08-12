package test

import (
	"claude-proxy/config"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnvConfigLoading tests configuration loading from .env file
// Following SPARC principles: Simple, focused test with clear assertions
func TestEnvConfigLoading(t *testing.T) {
	tests := []struct {
		name          string
		envContent    string
		expectedBig   string
		expectedSmall string
		expectError   bool
	}{
		{
			name: "valid_env_loads_correctly",
			envContent: `BIG_MODEL=kimi-k2
BIG_MODEL_ENDPOINT=http://192.168.0.24:8080/v1/chat/completions
BIG_MODEL_API_KEY=sk-12345
SMALL_MODEL=qwen2.5-coder:latest
SMALL_MODEL_ENDPOINT=http://192.168.0.46:11434/v1/chat/completions
SMALL_MODEL_API_KEY=ollama
TOOL_CORRECTION_ENDPOINT=http://192.168.0.46:11434/v1/chat/completions
TOOL_CORRECTION_API_KEY=ollama
CORRECTION_MODEL=qwen2.5-coder:latest
LOG_FULL_TOOLS=false
CONVERSATION_TRUNCATION=200`,
			expectedBig:   "http://192.168.0.24:8080/v1/chat/completions",
			expectedSmall: "http://192.168.0.46:11434/v1/chat/completions",
			expectError:   false,
		},
		{
			name: "missing_big_model_fails",
			envContent: `SMALL_MODEL=qwen2.5-coder:latest
SMALL_MODEL_ENDPOINT=http://192.168.0.46:11434/v1/chat/completions
SMALL_MODEL_API_KEY=ollama
TOOL_CORRECTION_ENDPOINT=http://192.168.0.46:11434/v1/chat/completions
TOOL_CORRECTION_API_KEY=ollama
CORRECTION_MODEL=qwen2.5-coder:latest`,
			expectError: true,
		},
		{
			name: "missing_small_model_fails",
			envContent: `BIG_MODEL=kimi-k2
BIG_MODEL_ENDPOINT=http://192.168.0.24:8080/v1/chat/completions
BIG_MODEL_API_KEY=sk-12345
CORRECTION_MODEL=qwen2.5-coder:latest`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary .env file
			tempDir, err := os.MkdirTemp("", "claude-proxy-test")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			// Change to temp directory
			originalWd, _ := os.Getwd()
			os.Chdir(tempDir)
			defer os.Chdir(originalWd)

			envPath := filepath.Join(tempDir, ".env")
			err = os.WriteFile(envPath, []byte(tt.envContent), 0644)
			require.NoError(t, err)

			// Test config loading
			cfg, err := config.LoadConfigWithEnv()

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Len(t, cfg.BigModelEndpoints, 1)
			require.Len(t, cfg.SmallModelEndpoints, 1)
			assert.Equal(t, tt.expectedBig, cfg.BigModelEndpoints[0])
			assert.Equal(t, tt.expectedSmall, cfg.SmallModelEndpoints[0])
			assert.Equal(t, "3456", cfg.Port)
			assert.True(t, cfg.ToolCorrectionEnabled)
		})
	}
}

// TestDefaultConfig tests default configuration structure
func TestDefaultConfig(t *testing.T) {
	cfg := config.GetDefaultConfig()

	assert.Empty(t, cfg.BigModelEndpoints)         // Empty until set from .env
	assert.Empty(t, cfg.SmallModelEndpoints)       // Empty until set from .env
	assert.Empty(t, cfg.ToolCorrectionEndpoints)   // Empty until set from .env
	assert.Equal(t, "3456", cfg.Port)
	assert.True(t, cfg.ToolCorrectionEnabled)
	assert.False(t, cfg.DisableSmallModelLogging) // Enabled by default (normal logging)
	assert.False(t, cfg.DisableToolCorrectionLogging) // Enabled by default (normal logging)
	assert.Empty(t, cfg.SkipTools) // Empty array by default
}

// TestSkipToolsParsing tests SKIP_TOOLS parsing from .env
func TestSkipToolsParsing(t *testing.T) {
	tests := []struct {
		name         string
		skipTools    string
		expectedTools []string
	}{
		{
			name:         "single_tool",
			skipTools:    "NotebookRead",
			expectedTools: []string{"NotebookRead"},
		},
		{
			name:         "multiple_tools",
			skipTools:    "NotebookRead,NotebookEdit,SomeTool",
			expectedTools: []string{"NotebookRead", "NotebookEdit", "SomeTool"},
		},
		{
			name:         "tools_with_spaces",
			skipTools:    "NotebookRead, NotebookEdit , SomeTool",
			expectedTools: []string{"NotebookRead", "NotebookEdit", "SomeTool"},
		},
		{
			name:         "empty_string",
			skipTools:    "",
			expectedTools: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "claude-proxy-test")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			originalWd, _ := os.Getwd()
			os.Chdir(tempDir)
			defer os.Chdir(originalWd)

			envContent := `BIG_MODEL=test-big
BIG_MODEL_ENDPOINT=http://test:8080/v1/chat/completions
BIG_MODEL_API_KEY=test-key
SMALL_MODEL=test-small
SMALL_MODEL_ENDPOINT=http://test:11434/v1/chat/completions
SMALL_MODEL_API_KEY=test-key
TOOL_CORRECTION_ENDPOINT=http://test:11434/v1/chat/completions
TOOL_CORRECTION_API_KEY=test-key
CORRECTION_MODEL=test-correction
LOG_FULL_TOOLS=false
CONVERSATION_TRUNCATION=200`
			if tt.skipTools != "" {
				envContent += "\nSKIP_TOOLS=" + tt.skipTools
			}

			envPath := filepath.Join(tempDir, ".env")
			err = os.WriteFile(envPath, []byte(envContent), 0644)
			require.NoError(t, err)

			cfg, err := config.LoadConfigWithEnv()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedTools, cfg.SkipTools)
		})
	}
}

// TestYAMLToolDescriptionsLoading tests loading tool descriptions from YAML file
func TestYAMLToolDescriptionsLoading(t *testing.T) {
	tests := []struct {
		name                string
		yamlContent         string
		expectedDescriptions map[string]string
		expectError         bool
	}{
		{
			name: "valid_yaml_loads_correctly",
			yamlContent: `toolDescriptions:
  Task: "Custom Task description"
  Bash: "Custom Bash description"`,
			expectedDescriptions: map[string]string{
				"Task": "Custom Task description",
				"Bash": "Custom Bash description",
			},
			expectError: false,
		},
		{
			name: "multiline_descriptions_supported",
			yamlContent: `toolDescriptions:
  Task: |
    This is a multi-line
    task description
    with multiple lines`,
			expectedDescriptions: map[string]string{
				"Task": "This is a multi-line\ntask description\nwith multiple lines",
			},
			expectError: false,
		},
		{
			name: "empty_yaml_returns_empty_map",
			yamlContent: `toolDescriptions: {}`,
			expectedDescriptions: map[string]string{},
			expectError: false,
		},
		{
			name: "invalid_yaml_returns_error",
			yamlContent: `toolDescriptions:
  Task: "Valid"
    InvalidIndent: "This should fail"`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "claude-proxy-yaml-test")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			originalWd, _ := os.Getwd()
			os.Chdir(tempDir)
			defer os.Chdir(originalWd)

			yamlPath := filepath.Join(tempDir, "tools_override.yaml")
			err = os.WriteFile(yamlPath, []byte(tt.yamlContent), 0644)
			require.NoError(t, err)

			descriptions, err := config.LoadToolDescriptions()

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedDescriptions, descriptions)
		})
	}
}

// TestYAMLToolDescriptionsFileNotExists tests behavior when YAML file doesn't exist
func TestYAMLToolDescriptionsFileNotExists(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "claude-proxy-yaml-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalWd)

	// No YAML file exists
	descriptions, err := config.LoadToolDescriptions()
	
	// Should return empty map and no error when file doesn't exist
	assert.NoError(t, err)
	assert.Empty(t, descriptions)
}

// TestGetToolDescription tests getting individual tool descriptions
func TestGetToolDescription(t *testing.T) {
	tests := []struct {
		name                string
		toolDescriptions    map[string]string
		toolName            string
		originalDescription string
		expectedDescription string
	}{
		{
			name: "override_exists_returns_override",
			toolDescriptions: map[string]string{
				"Task": "Custom Task description",
			},
			toolName:            "Task",
			originalDescription: "Original Task description",
			expectedDescription: "Custom Task description",
		},
		{
			name: "no_override_returns_original",
			toolDescriptions: map[string]string{
				"Bash": "Custom Bash description",
			},
			toolName:            "Task",
			originalDescription: "Original Task description",
			expectedDescription: "Original Task description",
		},
		{
			name: "empty_overrides_returns_original",
			toolDescriptions:    map[string]string{},
			toolName:            "Task",
			originalDescription: "Original Task description",
			expectedDescription: "Original Task description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			description := config.GetToolDescription(tt.toolDescriptions, tt.toolName, tt.originalDescription)
			assert.Equal(t, tt.expectedDescription, description)
		})
	}
}

// TestYAMLToolDescriptionIntegration tests the complete integration of YAML tool descriptions
func TestYAMLToolDescriptionIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "claude-proxy-integration-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalWd)

	// Create a complete .env file
	envContent := `BIG_MODEL=test-big
BIG_MODEL_ENDPOINT=http://test:8080/v1/chat/completions
BIG_MODEL_API_KEY=test-key
SMALL_MODEL=test-small
SMALL_MODEL_ENDPOINT=http://test:11434/v1/chat/completions
SMALL_MODEL_API_KEY=test-key
TOOL_CORRECTION_ENDPOINT=http://test:11434/v1/chat/completions
TOOL_CORRECTION_API_KEY=test-key
CORRECTION_MODEL=test-correction
LOG_FULL_TOOLS=false
CONVERSATION_TRUNCATION=200
`
	envPath := filepath.Join(tempDir, ".env")
	err = os.WriteFile(envPath, []byte(envContent), 0644)
	require.NoError(t, err)

	// Create a YAML file with tool description overrides
	yamlContent := `toolDescriptions:
  Task: "Custom Task description from YAML"
  Bash: "Custom Bash description from YAML"
  Write: "Custom Write description from YAML"
`
	yamlPath := filepath.Join(tempDir, "tools_override.yaml")
	err = os.WriteFile(yamlPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	// Load configuration (this should load both .env and YAML)
	cfg, err := config.LoadConfigWithEnv()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify that tool descriptions were loaded
	assert.Equal(t, 3, len(cfg.ToolDescriptions))
	assert.Equal(t, "Custom Task description from YAML", cfg.ToolDescriptions["Task"])
	assert.Equal(t, "Custom Bash description from YAML", cfg.ToolDescriptions["Bash"])
	assert.Equal(t, "Custom Write description from YAML", cfg.ToolDescriptions["Write"])

	// Test the GetToolDescription method
	assert.Equal(t, "Custom Task description from YAML", cfg.GetToolDescription("Task", "Original Task description"))
	assert.Equal(t, "Custom Bash description from YAML", cfg.GetToolDescription("Bash", "Original Bash description"))
	assert.Equal(t, "Original Read description", cfg.GetToolDescription("Read", "Original Read description")) // No override
}

// TestPrintSystemMessageConfig tests PRINT_SYSTEM_MESSAGE configuration
func TestPrintSystemMessageConfig(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		expectedValue  bool
		shouldHaveVar  bool
	}{
		{
			name:          "print_system_message_true",
			envValue:      "true",
			expectedValue: true,
			shouldHaveVar: true,
		},
		{
			name:          "print_system_message_1",
			envValue:      "1",
			expectedValue: true,
			shouldHaveVar: true,
		},
		{
			name:          "print_system_message_false",
			envValue:      "false",
			expectedValue: false,
			shouldHaveVar: true,
		},
		{
			name:          "print_system_message_omitted",
			envValue:      "",
			expectedValue: false,
			shouldHaveVar: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "claude-proxy-print-system-test")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			originalWd, _ := os.Getwd()
			os.Chdir(tempDir)
			defer os.Chdir(originalWd)

			// Create a complete .env file
			envContent := `BIG_MODEL=test-big
BIG_MODEL_ENDPOINT=http://test:8080/v1/chat/completions
BIG_MODEL_API_KEY=test-key
SMALL_MODEL=test-small
SMALL_MODEL_ENDPOINT=http://test:11434/v1/chat/completions
SMALL_MODEL_API_KEY=test-key
TOOL_CORRECTION_ENDPOINT=http://test:11434/v1/chat/completions
TOOL_CORRECTION_API_KEY=test-key
CORRECTION_MODEL=test-correction
LOG_FULL_TOOLS=false
CONVERSATION_TRUNCATION=200
`
			if tt.shouldHaveVar {
				envContent += "PRINT_SYSTEM_MESSAGE=" + tt.envValue + "\n"
			}

			envPath := filepath.Join(tempDir, ".env")
			err = os.WriteFile(envPath, []byte(envContent), 0644)
			require.NoError(t, err)

			// Load configuration
			cfg, err := config.LoadConfigWithEnv()
			require.NoError(t, err)
			require.NotNil(t, cfg)

			// Verify the PrintSystemMessage setting
			assert.Equal(t, tt.expectedValue, cfg.PrintSystemMessage)
		})
	}
}

// TestDisableSmallModelLoggingConfig tests DISABLE_SMALL_MODEL_LOGGING configuration
func TestDisableSmallModelLoggingConfig(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		expectedValue  bool
		shouldHaveVar  bool
	}{
		{
			name:          "disable_small_model_logging_true",
			envValue:      "true",
			expectedValue: true,
			shouldHaveVar: true,
		},
		{
			name:          "disable_small_model_logging_1",
			envValue:      "1", 
			expectedValue: true,
			shouldHaveVar: true,
		},
		{
			name:          "disable_small_model_logging_false",
			envValue:      "false",
			expectedValue: false,
			shouldHaveVar: true,
		},
		{
			name:          "disable_small_model_logging_omitted",
			envValue:      "",
			expectedValue: false,
			shouldHaveVar: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "claude-proxy-disable-small-logging-test")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			originalWd, _ := os.Getwd()
			os.Chdir(tempDir)
			defer os.Chdir(originalWd)

			// Create a complete .env file
			envContent := `BIG_MODEL=test-big
BIG_MODEL_ENDPOINT=http://test:8080/v1/chat/completions
BIG_MODEL_API_KEY=test-key
SMALL_MODEL=test-small
SMALL_MODEL_ENDPOINT=http://test:11434/v1/chat/completions
SMALL_MODEL_API_KEY=test-key
TOOL_CORRECTION_ENDPOINT=http://test:11434/v1/chat/completions
TOOL_CORRECTION_API_KEY=test-key
CORRECTION_MODEL=test-correction
LOG_FULL_TOOLS=false
CONVERSATION_TRUNCATION=200
`
			if tt.shouldHaveVar {
				envContent += "DISABLE_SMALL_MODEL_LOGGING=" + tt.envValue + "\n"
			}

			envPath := filepath.Join(tempDir, ".env")
			err = os.WriteFile(envPath, []byte(envContent), 0644)
			require.NoError(t, err)

			// Load configuration
			cfg, err := config.LoadConfigWithEnv()
			require.NoError(t, err)
			require.NotNil(t, cfg)

			// Verify the DisableSmallModelLogging setting
			assert.Equal(t, tt.expectedValue, cfg.DisableSmallModelLogging)
		})
	}
}

// TestDisableToolCorrectionLoggingConfig tests DISABLE_TOOL_CORRECTION_LOGGING configuration
func TestDisableToolCorrectionLoggingConfig(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		expectedValue  bool
		shouldHaveVar  bool
	}{
		{
			name:          "disable_tool_correction_logging_true",
			envValue:      "true",
			expectedValue: true,
			shouldHaveVar: true,
		},
		{
			name:          "disable_tool_correction_logging_1",
			envValue:      "1", 
			expectedValue: true,
			shouldHaveVar: true,
		},
		{
			name:          "disable_tool_correction_logging_false",
			envValue:      "false",
			expectedValue: false,
			shouldHaveVar: true,
		},
		{
			name:          "disable_tool_correction_logging_omitted",
			envValue:      "",
			expectedValue: false,
			shouldHaveVar: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "claude-proxy-disable-tool-correction-logging-test")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			originalWd, _ := os.Getwd()
			os.Chdir(tempDir)
			defer os.Chdir(originalWd)

			// Create a complete .env file
			envContent := `BIG_MODEL=test-big
BIG_MODEL_ENDPOINT=http://test:8080/v1/chat/completions
BIG_MODEL_API_KEY=test-key
SMALL_MODEL=test-small
SMALL_MODEL_ENDPOINT=http://test:11434/v1/chat/completions
SMALL_MODEL_API_KEY=test-key
TOOL_CORRECTION_ENDPOINT=http://test:11434/v1/chat/completions
TOOL_CORRECTION_API_KEY=test-key
CORRECTION_MODEL=test-correction
LOG_FULL_TOOLS=false
CONVERSATION_TRUNCATION=200
`
			if tt.shouldHaveVar {
				envContent += "DISABLE_TOOL_CORRECTION_LOGGING=" + tt.envValue + "\n"
			}

			envPath := filepath.Join(tempDir, ".env")
			err = os.WriteFile(envPath, []byte(envContent), 0644)
			require.NoError(t, err)

			// Load configuration
			cfg, err := config.LoadConfigWithEnv()
			require.NoError(t, err)
			require.NotNil(t, cfg)

			// Verify the DisableToolCorrectionLogging setting
			assert.Equal(t, tt.expectedValue, cfg.DisableToolCorrectionLogging)
		})
	}
}

// TestSystemMessageOverrideLoading tests loading system message overrides from YAML file
func TestSystemMessageOverrideLoading(t *testing.T) {
	tests := []struct {
		name                   string
		yamlContent            string
		expectedRemovePatterns []string
		expectedReplacements   []config.SystemMessageReplacement
		expectedPrepend        string
		expectedAppend         string
		expectError            bool
	}{
		{
			name: "valid_system_overrides_loads_correctly",
			yamlContent: `systemMessageOverrides:
  removePatterns:
    - "IMPORTANT: Assist with defensive security.*"
    - "You must NEVER generate.*"
  replacements:
    - find: "Claude Code"
      replace: "AI Assistant"
    - find: "Anthropic"
      replace: "Company"
  prepend: |
    You are an expert coding assistant.
  append: |
    Always prioritize quality.`,
			expectedRemovePatterns: []string{
				"IMPORTANT: Assist with defensive security.*",
				"You must NEVER generate.*",
			},
			expectedReplacements: []config.SystemMessageReplacement{
				{Find: "Claude Code", Replace: "AI Assistant"},
				{Find: "Anthropic", Replace: "Company"},
			},
			expectedPrepend: "You are an expert coding assistant.\n",
			expectedAppend:  "Always prioritize quality.",
			expectError:     false,
		},
		{
			name: "empty_overrides_returns_empty_struct",
			yamlContent: `systemMessageOverrides:
  removePatterns: []
  replacements: []
  prepend: ""
  append: ""`,
			expectedRemovePatterns: []string{},
			expectedReplacements:   []config.SystemMessageReplacement{},
			expectedPrepend:        "",
			expectedAppend:         "",
			expectError:            false,
		},
		{
			name: "invalid_yaml_returns_error",
			yamlContent: `systemMessageOverrides:
  removePatterns:
    - "Valid pattern"
      invalidIndent: "This should fail"`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "claude-proxy-system-override-test")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			originalWd, _ := os.Getwd()
			os.Chdir(tempDir)
			defer os.Chdir(originalWd)

			yamlPath := filepath.Join(tempDir, "system_overrides.yaml")
			err = os.WriteFile(yamlPath, []byte(tt.yamlContent), 0644)
			require.NoError(t, err)

			overrides, err := config.LoadSystemMessageOverrides()

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedRemovePatterns, overrides.RemovePatterns)
			assert.Equal(t, tt.expectedReplacements, overrides.Replacements)
			assert.Equal(t, tt.expectedPrepend, overrides.Prepend)
			assert.Equal(t, tt.expectedAppend, overrides.Append)
		})
	}
}

// TestSystemMessageOverrideFileNotExists tests behavior when system_overrides.yaml doesn't exist
func TestSystemMessageOverrideFileNotExists(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "claude-proxy-system-override-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalWd)

	// No YAML file exists
	overrides, err := config.LoadSystemMessageOverrides()
	
	// Should return empty struct and no error when file doesn't exist
	assert.NoError(t, err)
	assert.Empty(t, overrides.RemovePatterns)
	assert.Empty(t, overrides.Replacements)
	assert.Empty(t, overrides.Prepend)
	assert.Empty(t, overrides.Append)
}

// TestApplySystemMessageOverrides tests applying overrides to system messages
func TestApplySystemMessageOverrides(t *testing.T) {
	tests := []struct {
		name            string
		originalMessage string
		overrides       config.SystemMessageOverrides
		expectedMessage string
	}{
		{
			name: "remove_patterns_works",
			originalMessage: "You are Claude Code. IMPORTANT: Assist with defensive security tasks only. You must NEVER generate URLs. Help users with code.",
			overrides: config.SystemMessageOverrides{
				RemovePatterns: []string{
					"IMPORTANT: Assist with defensive security tasks only\\.",
					"You must NEVER generate URLs\\.",
				},
			},
			expectedMessage: "You are Claude Code.   Help users with code.",
		},
		{
			name: "replacements_work",
			originalMessage: "You are Claude Code, Anthropic's official CLI tool.",
			overrides: config.SystemMessageOverrides{
				Replacements: []config.SystemMessageReplacement{
					{Find: "Claude Code", Replace: "AI Assistant"},
					{Find: "Anthropic's official CLI", Replace: "Your AI Assistant"},
				},
			},
			expectedMessage: "You are AI Assistant, Your AI Assistant tool.",
		},
		{
			name: "prepend_and_append_work",
			originalMessage: "You are a helpful assistant.",
			overrides: config.SystemMessageOverrides{
				Prepend: "CUSTOM PREFIX: You are an expert coder.\n",
				Append:  "\nCUSTOM SUFFIX: Always prioritize quality.",
			},
			expectedMessage: "CUSTOM PREFIX: You are an expert coder.\nYou are a helpful assistant.\nCUSTOM SUFFIX: Always prioritize quality.",
		},
		{
			name: "all_operations_combined",
			originalMessage: "You are Claude Code. IMPORTANT: Assist with defensive security tasks only. Help users.",
			overrides: config.SystemMessageOverrides{
				RemovePatterns: []string{"IMPORTANT: Assist with defensive security tasks only\\."},
				Replacements: []config.SystemMessageReplacement{
					{Find: "Claude Code", Replace: "AI Assistant"},
				},
				Prepend: "Expert coder: ",
				Append:  " Focus on quality.",
			},
			expectedMessage: "Expert coder: You are AI Assistant.  Help users. Focus on quality.",
		},
		{
			name: "empty_overrides_no_change",
			originalMessage: "You are a helpful assistant.",
			overrides: config.SystemMessageOverrides{},
			expectedMessage: "You are a helpful assistant.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.ApplySystemMessageOverrides(tt.originalMessage, tt.overrides)
			assert.Equal(t, tt.expectedMessage, result)
		})
	}
}

// TestSystemMessageOverrideIntegration tests the complete system message override integration
func TestSystemMessageOverrideIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "claude-proxy-system-override-integration-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	originalWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalWd)

	// Create a complete .env file
	envContent := `BIG_MODEL=test-big
BIG_MODEL_ENDPOINT=http://test:8080/v1/chat/completions
BIG_MODEL_API_KEY=test-key
SMALL_MODEL=test-small
SMALL_MODEL_ENDPOINT=http://test:11434/v1/chat/completions
SMALL_MODEL_API_KEY=test-key
TOOL_CORRECTION_ENDPOINT=http://test:11434/v1/chat/completions
TOOL_CORRECTION_API_KEY=test-key
CORRECTION_MODEL=test-correction
PRINT_SYSTEM_MESSAGE=true
LOG_FULL_TOOLS=false
CONVERSATION_TRUNCATION=false
`
	envPath := filepath.Join(tempDir, ".env")
	err = os.WriteFile(envPath, []byte(envContent), 0644)
	require.NoError(t, err)

	// Create a system_overrides.yaml file
	yamlContent := `systemMessageOverrides:
  removePatterns:
    - "IMPORTANT: Assist with defensive security.*"
    - "If the user asks for help.*"
  replacements:
    - find: "Claude Code"
      replace: "AI Assistant"
    - find: "Anthropic"
      replace: "Custom Company"
  prepend: |
    CUSTOM PREFIX: You are an expert coding assistant.
  append: |
    CUSTOM SUFFIX: Always prioritize code quality.
`
	yamlPath := filepath.Join(tempDir, "system_overrides.yaml")
	err = os.WriteFile(yamlPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	// Load configuration (this should load both .env and YAML)
	cfg, err := config.LoadConfigWithEnv()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify that system message overrides were loaded
	assert.Equal(t, 2, len(cfg.SystemMessageOverrides.RemovePatterns))
	assert.Equal(t, 2, len(cfg.SystemMessageOverrides.Replacements))
	assert.Contains(t, cfg.SystemMessageOverrides.Prepend, "CUSTOM PREFIX")
	assert.Contains(t, cfg.SystemMessageOverrides.Append, "CUSTOM SUFFIX")

	// Test applying overrides to a system message
	originalMessage := "You are Claude Code, Anthropic's official CLI. IMPORTANT: Assist with defensive security tasks only. If the user asks for help, direct them to support."
	expectedMessage := "CUSTOM PREFIX: You are an expert coding assistant.\nYou are AI Assistant, Custom Company's official CLI. CUSTOM SUFFIX: Always prioritize code quality.\n"
	
	result := config.ApplySystemMessageOverrides(originalMessage, cfg.SystemMessageOverrides)
	assert.Equal(t, expectedMessage, result)
}

// TestSystemMessageOverrideLogging tests detailed logging of override operations
func TestSystemMessageOverrideLogging(t *testing.T) {
	originalMessage := "You are Claude Code, Anthropic's official CLI. IMPORTANT: Assist with defensive security tasks only. Help users with coding tasks."
	
	overrides := config.SystemMessageOverrides{
		RemovePatterns: []string{"IMPORTANT: Assist with defensive security tasks only[^.]*\\."},
		Replacements: []config.SystemMessageReplacement{
			{Find: "Claude Code", Replace: "AI Assistant"},
			{Find: "Anthropic's official CLI", Replace: "Your AI Assistant"},
		},
		Prepend: "CUSTOM PREFIX: You are an expert coder.\n",
		Append: "\nCUSTOM SUFFIX: Always prioritize quality.",
	}

	result := config.ApplySystemMessageOverrides(originalMessage, overrides)
	
	// Verify the transformation worked
	assert.Contains(t, result, "AI Assistant")
	assert.Contains(t, result, "Your AI Assistant")
	assert.Contains(t, result, "CUSTOM PREFIX")
	assert.Contains(t, result, "CUSTOM SUFFIX")
	assert.NotContains(t, result, "IMPORTANT: Assist with defensive security")
	
	// Test logs are written (we can't easily capture them in unit tests, but the function should not error)
	assert.NotEmpty(t, result)
}
