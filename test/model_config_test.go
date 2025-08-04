package test

import (
	"claude-proxy/config"
	"claude-proxy/internal"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnvFileLoading tests .env file loading functionality
// Following TDD: These tests will initially FAIL until we implement .env loading
func TestEnvFileLoading(t *testing.T) {
	tests := []struct {
		name        string
		envContent  string
		expectedBig string
		expectedSmall string
		expectError bool
		description string
	}{
		{
			name: "valid_env_file_loads_correctly",
			envContent: `BIG_MODEL=custom-big-model
BIG_MODEL_ENDPOINT=http://test:8080/v1/chat/completions
BIG_MODEL_API_KEY=test-key
SMALL_MODEL=custom-small-model
SMALL_MODEL_ENDPOINT=http://test:11434/v1/chat/completions
SMALL_MODEL_API_KEY=test-key
CORRECTION_MODEL=custom-correction-model
TOOL_CORRECTION_ENDPOINT=http://test:11434/v1/chat/completions
TOOL_CORRECTION_API_KEY=test-key
`,
			expectedBig:   "custom-big-model",
			expectedSmall: "custom-small-model",
			expectError:   false,
			description:   "Valid .env file should load both models correctly",
		},
		{
			name: "partial_env_file_missing_small_model",
			envContent: `BIG_MODEL=only-big-configured
BIG_MODEL_ENDPOINT=http://test:8080/v1/chat/completions
BIG_MODEL_API_KEY=test-key
CORRECTION_MODEL=test-correction
`,
			expectError: true,
			description: "Missing SMALL_MODEL should cause error",
		},
		{
			name: "empty_env_file_causes_error",
			envContent: `# This is an empty .env file
`,
			expectError: true,
			description: "Empty .env file should cause error due to missing required fields",
		},
		{
			name: "comments_and_whitespace_handled",
			envContent: `# Model configuration
BIG_MODEL=big-with-comments   # This is the big model
BIG_MODEL_ENDPOINT=http://test:8080/v1/chat/completions
BIG_MODEL_API_KEY=test-key
SMALL_MODEL=small-with-comments
SMALL_MODEL_ENDPOINT=http://test:11434/v1/chat/completions
SMALL_MODEL_API_KEY=test-key
CORRECTION_MODEL=correction-with-comments
TOOL_CORRECTION_ENDPOINT=http://test:11434/v1/chat/completions
TOOL_CORRECTION_API_KEY=test-key
# End of config
`,
			expectedBig:   "big-with-comments",
			expectedSmall: "small-with-comments",
			expectError:   false,
			description:   "Comments and whitespace should be handled properly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test
			tempDir, err := ioutil.TempDir("", "proxy-test-")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			// Write .env file
			envPath := filepath.Join(tempDir, ".env")
			err = ioutil.WriteFile(envPath, []byte(tt.envContent), 0644)
			require.NoError(t, err)

			// Change to temp directory to simulate running from proxy folder
			originalDir, err := os.Getwd()
			require.NoError(t, err)
			defer os.Chdir(originalDir)
			
			err = os.Chdir(tempDir)
			require.NoError(t, err)

			// Load config with .env support (THIS WILL FAIL until implemented)
			cfg, err := config.LoadConfigWithEnv()
			
			if tt.expectError {
				assert.Error(t, err, tt.description)
				return
			}
			
			require.NoError(t, err, tt.description)
			require.NotNil(t, cfg, "Config should not be nil")
			
			// Verify model configuration
			assert.Equal(t, tt.expectedBig, cfg.BigModel, "BIG_MODEL should match expected")
			assert.Equal(t, tt.expectedSmall, cfg.SmallModel, "SMALL_MODEL should match expected")
			
			t.Logf("Test: %s", tt.description)
			t.Logf("BIG_MODEL: %s", cfg.BigModel)
			t.Logf("SMALL_MODEL: %s", cfg.SmallModel)
		})
	}
}

// TestNoEnvFileFallback tests behavior when .env file doesn't exist
func TestNoEnvFileFallback(t *testing.T) {
	// Create temporary directory without .env file
	tempDir, err := ioutil.TempDir("", "proxy-test-no-env-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Change to temp directory
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)
	
	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Load config without .env file (should error now since .env is required)
	cfg, err := config.LoadConfigWithEnv()
	assert.Error(t, err, "Should error when .env file missing")
	assert.Nil(t, cfg, "Config should be nil when error occurs")
}

// TestModelMapping tests the model mapping logic with configurable models
func TestModelMappingWithConfigurableModels(t *testing.T) {
	tests := []struct {
		name        string
		inputModel  string
		bigModel    string
		smallModel  string
		expectedModel string
		description string
	}{
		{
			name:          "sonnet_maps_to_configured_big_model",
			inputModel:    "claude-sonnet-4-20250514",
			bigModel:      "custom-big-model",
			smallModel:    "custom-small-model",
			expectedModel: "custom-big-model",
			description:   "Sonnet should map to configured BIG_MODEL",
		},
		{
			name:          "haiku_maps_to_configured_small_model",
			inputModel:    "claude-3-5-haiku-20241022",
			bigModel:      "custom-big-model",
			smallModel:    "custom-small-model",
			expectedModel: "custom-small-model",
			description:   "Haiku should map to configured SMALL_MODEL",
		},
		{
			name:          "unknown_model_passes_through",
			inputModel:    "unknown-model",
			bigModel:      "custom-big-model",
			smallModel:    "custom-small-model",
			expectedModel: "unknown-model",
			description:   "Unknown models should pass through unchanged",
		},
		{
			name:          "default_models_work",
			inputModel:    "claude-sonnet-4-20250514",
			bigModel:      "kimi-k2",
			smallModel:    "qwen2.5-coder:latest",
			expectedModel: "kimi-k2",
			description:   "Default model configuration should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create config with test models
			cfg := &config.Config{
				BigModel:   tt.bigModel,
				SmallModel: tt.smallModel,
			}

			// Test model mapping with proper request ID context
			ctx := internal.WithRequestID(context.Background(), "test_mapping")
			mappedModel := cfg.MapModelName(ctx, tt.inputModel)
			
			assert.Equal(t, tt.expectedModel, mappedModel, tt.description)
			
			t.Logf("Input: %s → Mapped: %s", tt.inputModel, mappedModel)
		})
	}
}

// TestIntegrationModelConfiguration tests end-to-end model configuration
func TestIntegrationModelConfiguration(t *testing.T) {
	// Create temporary directory with .env file
	tempDir, err := ioutil.TempDir("", "proxy-integration-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Write .env file with custom models
	envContent := `BIG_MODEL=integration-big-model
BIG_MODEL_ENDPOINT=http://test:8080/v1/chat/completions
BIG_MODEL_API_KEY=test-key
SMALL_MODEL=integration-small-model
SMALL_MODEL_ENDPOINT=http://test:11434/v1/chat/completions
SMALL_MODEL_API_KEY=test-key
TOOL_CORRECTION_ENDPOINT=http://test:11434/v1/chat/completions
TOOL_CORRECTION_API_KEY=test-key
CORRECTION_MODEL=integration-correction-model
`
	envPath := filepath.Join(tempDir, ".env")
	err = ioutil.WriteFile(envPath, []byte(envContent), 0644)
	require.NoError(t, err)

	// Change to temp directory
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)
	
	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Load full configuration
	cfg, err := config.LoadConfigWithEnv()
	require.NoError(t, err, "Integration test should load config successfully")

	// Test complete model mapping workflow
	ctx := internal.WithRequestID(context.Background(), "integration_test")
	
	sonnetResult := cfg.MapModelName(ctx, "claude-sonnet-4-20250514")
	assert.Equal(t, "integration-big-model", sonnetResult, "Sonnet should map to custom big model")
	
	haikuResult := cfg.MapModelName(ctx, "claude-3-5-haiku-20241022")
	assert.Equal(t, "integration-small-model", haikuResult, "Haiku should map to custom small model")

	t.Logf("Integration test successful:")
	t.Logf("  Sonnet → %s", sonnetResult)
	t.Logf("  Haiku → %s", haikuResult)
}