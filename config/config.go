package config

import (
	"bufio"
	"claude-proxy/circuitbreaker"
	"claude-proxy/internal"
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete proxy configuration, containing all settings
// loaded from environment variables, YAML override files, and default values.
//
// This struct serves as the central configuration hub for the proxy service,
// providing access to all operational parameters including model endpoints,
// API keys, feature flags, and behavioral settings.
//
// Configuration sources (in order of precedence):
//   1. Environment variables from .env file (required)
//   2. YAML override files (optional): tools_override.yaml, system_overrides.yaml
//   3. Default values (fallback)
//
// Key configuration areas:
//   - Model routing: BigModel, SmallModel, CorrectionModel with endpoints
//   - Tool management: SkipTools, ToolDescriptions, correction settings
//   - Logging: Conversation logging, debug flags, sensitive data masking
//   - Performance: Connection timeouts, circuit breaker integration
//   - Features: Harmony parsing, system message overrides
//
// The Config struct is thread-safe for read operations and includes mutex
// protection for endpoint rotation and health management operations.
type Config struct {
	Port string `json:"port"`

	// Tool correction settings
	ToolCorrectionEnabled bool `json:"tool_correction_enabled"`

	// Empty message handling
	HandleEmptyToolResults  bool `json:"handle_empty_tool_results"`  // Replace empty tool results with descriptive messages
	HandleEmptyUserMessages bool `json:"handle_empty_user_messages"` // Replace empty user messages with placeholder content

	// Tool filtering settings
	SkipTools []string `json:"skip_tools"` // Tools to skip/filter out from requests

	// Tool description overrides (loaded from tools_override.yaml)
	ToolDescriptions map[string]string `json:"tool_descriptions"`

	// Debug settings
	PrintSystemMessage           bool `json:"print_system_message"`            // Print system messages to logs
	PrintToolSchemas             bool `json:"print_tool_schemas"`              // Print tool schemas from Claude Code for debugging
	DisableSmallModelLogging     bool `json:"disable_small_model_logging"`     // Disable logging for small model (Haiku) requests
	DisableToolCorrectionLogging bool `json:"disable_tool_correction_logging"` // Disable logging for tool correction operations

	// Conversation logging settings
	ConversationLoggingEnabled bool   `json:"conversation_logging_enabled"` // Enable full conversation logging
	ConversationLogLevel       string `json:"conversation_log_level"`       // Log level for conversation logs (DEBUG, INFO, WARN, ERROR)
	ConversationMaskSensitive  bool   `json:"conversation_mask_sensitive"`  // Mask sensitive data in conversation logs
	ConversationLogFullTools   bool   `json:"conversation_log_full_tools"`  // Log full tool definitions vs tool names only
	ConversationTruncation     int    `json:"conversation_truncation"`      // Maximum message length (0 = disabled)

	// Connection timeout settings
	DefaultConnectionTimeout int `json:"default_connection_timeout"` // Connection timeout in seconds for all endpoints

	// Tool choice correction and necessity detection
	EnableToolChoiceCorrection bool `json:"enable_tool_choice_correction"` // Enable tool choice correction and necessity detection

	// System message overrides (loaded from system_overrides.yaml)
	SystemMessageOverrides SystemMessageOverrides `json:"system_message_overrides"`

	// Harmony parsing settings
	HarmonyParsingEnabled bool `json:"harmony_parsing_enabled"` // Enable Harmony format parsing
	HarmonyDebug          bool `json:"harmony_debug"`           // Enable detailed Harmony debug logging
	HarmonyStrictMode     bool `json:"harmony_strict_mode"`     // Strict error handling for malformed Harmony content

	// Model configuration (.env configurable)
	BigModel        string `json:"big_model"`        // For Claude Sonnet requests
	SmallModel      string `json:"small_model"`      // For Claude Haiku requests
	CorrectionModel string `json:"correction_model"` // For tool correction service

	// Endpoint configuration (.env configurable) - supports multiple endpoints
	BigModelEndpoints       []string `json:"big_model_endpoints"`       // Endpoints for BIG_MODEL (comma-separated)
	SmallModelEndpoints     []string `json:"small_model_endpoints"`     // Endpoints for SMALL_MODEL (comma-separated)
	ToolCorrectionEndpoints []string `json:"tool_correction_endpoints"` // Endpoints for TOOL_CORRECTION_LLM (comma-separated)

	// API Key configuration (.env configurable)
	BigModelAPIKey       string `json:"big_model_api_key"`       // API Key for BIG_MODEL
	SmallModelAPIKey     string `json:"small_model_api_key"`     // API Key for SMALL_MODEL
	ToolCorrectionAPIKey string `json:"tool_correction_api_key"` // API Key for TOOL_CORRECTION_LLM

	// Endpoint rotation state (not serialized)
	bigModelIndex       int        `json:"-"`
	smallModelIndex     int        `json:"-"`
	toolCorrectionIndex int        `json:"-"`
	mutex               sync.Mutex `json:"-"`

	// Circuit breaker health manager
	HealthManager *circuitbreaker.HealthManager `json:"-"`
	
	// Observability logger (optional, can be nil during initial config loading)
	obsLogger interface {
		Info(component, category, requestID, message string, fields map[string]interface{})
		Warn(component, category, requestID, message string, fields map[string]interface{})
		Error(component, category, requestID, message string, fields map[string]interface{})
	} `json:"-"`
}

// SetObservabilityLogger configures structured logging for the configuration
// system, enabling detailed tracking of configuration operations and health management.
//
// This method establishes the logging infrastructure used throughout the
// configuration system for monitoring endpoint health, rotation decisions,
// and configuration changes. The logger interface provides structured
// logging with component, category, and field-based organization.
//
// The observability logger enables:
//   - Configuration change tracking
//   - Endpoint health monitoring
//   - Circuit breaker state logging
//   - Performance metrics collection
//   - Error and warning reporting
//
// Parameters:
//   - obsLogger: Structured logger implementing Info, Warn, and Error methods
//
// Thread Safety: This method is safe to call concurrently.
func (c *Config) SetObservabilityLogger(obsLogger interface {
	Info(component, category, requestID, message string, fields map[string]interface{})
	Warn(component, category, requestID, message string, fields map[string]interface{})
	Error(component, category, requestID, message string, fields map[string]interface{})
}) {
	c.obsLogger = obsLogger
	// Also set the obsLogger on the HealthManager for circuit breaker logging
	if c.HealthManager != nil {
		c.HealthManager.SetObservabilityLogger(obsLogger)
	}
}

// logInfo logs an info message with structured data if obsLogger is available
func (c *Config) logInfo(component, category, requestID, message string, fields map[string]interface{}) {
	if c.obsLogger != nil {
		c.obsLogger.Info(component, category, requestID, message, fields)
	}
}

// logWarn logs a warning message with structured data if obsLogger is available
func (c *Config) logWarn(component, category, requestID, message string, fields map[string]interface{}) {
	if c.obsLogger != nil {
		c.obsLogger.Warn(component, category, requestID, message, fields)
	}
}

// logError logs an error message with structured data if obsLogger is available
func (c *Config) logError(component, category, requestID, message string, fields map[string]interface{}) {
	if c.obsLogger != nil {
		c.obsLogger.Error(component, category, requestID, message, fields)
	}
}

// GetDefaultConfig returns a Config instance populated with sensible default
// values for testing and development environments.
//
// This function provides a baseline configuration that can be used for:
//   - Unit testing without external dependencies
//   - Development environment bootstrapping
//   - Configuration validation and comparison
//   - Fallback when environment configuration fails
//
// Default configuration includes:
//   - Standard port (3456) for proxy service
//   - All optional features disabled for predictable behavior
//   - Conservative logging and performance settings
//   - Empty collections for tools and overrides
//   - Circuit breaker with default configuration
//
// Note: This configuration requires .env file completion for production use,
// as model endpoints and API keys are not provided in defaults.
//
// Returns:
//   - A fully initialized Config struct with default values
//
// Example:
//
//	config := GetDefaultConfig()
//	// Customize for testing
//	config.BigModel = "test-model"
func GetDefaultConfig() *Config {
	return &Config{
		Port:                         "3456",
		ToolCorrectionEnabled:        true,
		SkipTools:                    []string{},               // Empty array by default
		ToolDescriptions:             make(map[string]string),  // Empty map by default
		PrintSystemMessage:           false,                    // Disabled by default
		PrintToolSchemas:             false,                    // Disabled by default
		DisableSmallModelLogging:     false,                    // Enabled by default (normal logging)
		DisableToolCorrectionLogging: false,                    // Enabled by default (normal logging)
		ConversationLoggingEnabled:   false,                    // Disabled by default
		ConversationLogLevel:         "INFO",                   // Default to INFO level
		ConversationMaskSensitive:    true,                     // Enable sensitive data masking by default
		EnableToolChoiceCorrection:   false,                    // Disable tool choice correction by default
		SystemMessageOverrides:       SystemMessageOverrides{}, // Empty by default
		HarmonyParsingEnabled:        true,                      // Enable by default
		HarmonyDebug:                 false,                     // Disabled by default
		HarmonyStrictMode:            false,                     // Lenient by default
		BigModel:                     "",                       // Will be set from .env
		SmallModel:                   "",                       // Will be set from .env
		CorrectionModel:              "",                       // Will be set from .env
		BigModelEndpoints:            []string{},               // Will be set from .env
		SmallModelEndpoints:          []string{},               // Will be set from .env
		ToolCorrectionEndpoints:      []string{},               // Will be set from .env
		BigModelAPIKey:               "",                       // Will be set from .env
		SmallModelAPIKey:             "",                       // Will be set from .env
		ToolCorrectionAPIKey:         "",                       // Will be set from .env
		HealthManager:                circuitbreaker.NewHealthManager(circuitbreaker.DefaultConfig()),
	}
}

// LoadConfigWithEnv loads complete proxy configuration from environment variables
// and optional YAML override files, providing production-ready configuration.
//
// This function performs comprehensive configuration loading:
//   1. Environment variable validation from .env file (required)
//   2. Model endpoint parsing with multi-endpoint support
//   3. API key validation and secure masking
//   4. Optional YAML file loading with graceful degradation
//   5. Circuit breaker initialization for health management
//   6. Configuration validation and error reporting
//
// Required .env variables:
//   - BIG_MODEL, SMALL_MODEL, CORRECTION_MODEL: Model identifiers
//   - BIG_MODEL_ENDPOINT, SMALL_MODEL_ENDPOINT, TOOL_CORRECTION_ENDPOINT: Provider URLs
//   - BIG_MODEL_API_KEY, SMALL_MODEL_API_KEY, TOOL_CORRECTION_API_KEY: Authentication
//   - LOG_FULL_TOOLS, CONVERSATION_TRUNCATION: Logging configuration
//
// Optional configurations:
//   - Feature flags: HARMONY_PARSING_ENABLED, PRINT_SYSTEM_MESSAGE
//   - Performance: DEFAULT_CONNECTION_TIMEOUT, SKIP_TOOLS
//   - Logging: CONVERSATION_LOGGING_ENABLED, various debug flags
//
// The function provides detailed error messages for missing or invalid
// configuration values, enabling rapid deployment troubleshooting.
//
// Returns:
//   - A fully configured Config instance ready for production use
//   - An error if any required configuration is missing or invalid
//
// Performance: Configuration loading is optimized for startup time with
// one-time regex compilation and efficient parsing.
//
// Example:
//
//	config, err := LoadConfigWithEnv()
//	if err != nil {
//		log.Fatal("Configuration error:", err)
//	}
//	// Use config for proxy initialization
func LoadConfigWithEnv() (*Config, error) {
	// Load .env file for complete configuration - REQUIRED
	envVars, err := loadEnvFile()
	if err != nil {
		return nil, fmt.Errorf(".env file is required for configuration: %v", err)
	}

	// Create new config with defaults
	cfg := &Config{
		Port:                       "3456",                   // Default port
		ToolCorrectionEnabled:      true,                     // Enable by default
		HandleEmptyToolResults:     true,                     // Enable by default for API compliance
		SkipTools:                  []string{},               // Empty by default
		ToolDescriptions:           make(map[string]string),  // Empty by default
		PrintSystemMessage:         false,                    // Disabled by default
		PrintToolSchemas:           false,                    // Disabled by default
		ConversationLoggingEnabled: false,                    // Disabled by default
		ConversationLogLevel:       "INFO",                   // Default to INFO level
		ConversationMaskSensitive:  true,                     // Enable sensitive data masking by default
		ConversationLogFullTools:     false,                    // Log tool names only by default
		ConversationTruncation:       0,                        // No truncation by default
		DefaultConnectionTimeout:     30,                       // 30 seconds default connection timeout
		EnableToolChoiceCorrection:   false,                    // Disable tool choice correction by default
		SystemMessageOverrides:       SystemMessageOverrides{}, // Empty by default
		HarmonyParsingEnabled:        true,                      // Enable by default
		HarmonyDebug:                 false,                     // Disabled by default
		HarmonyStrictMode:            false,                     // Lenient by default
		HealthManager:              circuitbreaker.NewHealthManager(circuitbreaker.DefaultConfig()),
	}

	// All models and endpoints are required when .env exists - no fallbacks
	if bigModel, exists := envVars["BIG_MODEL"]; exists && bigModel != "" {
		cfg.BigModel = bigModel
		cfg.logInfo("configuration", "request", "", "Configured BIG_MODEL", map[string]interface{}{
			"model": bigModel,
		})
	} else {
		return nil, fmt.Errorf("BIG_MODEL must be set in .env file")
	}

	if smallModel, exists := envVars["SMALL_MODEL"]; exists && smallModel != "" {
		cfg.SmallModel = smallModel
		cfg.logInfo("configuration", "request", "", "Configured SMALL_MODEL", map[string]interface{}{
			"model": smallModel,
		})
	} else {
		return nil, fmt.Errorf("SMALL_MODEL must be set in .env file")
	}

	if correctionModel, exists := envVars["CORRECTION_MODEL"]; exists && correctionModel != "" {
		cfg.CorrectionModel = correctionModel
		cfg.logInfo("configuration", "request", "", "Configured CORRECTION_MODEL", map[string]interface{}{
			"model": correctionModel,
		})
	} else {
		return nil, fmt.Errorf("CORRECTION_MODEL must be set in .env file")
	}

	// Parse BIG_MODEL_ENDPOINT (comma-separated list)
	if bigEndpoints, exists := envVars["BIG_MODEL_ENDPOINT"]; exists && bigEndpoints != "" {
		endpoints := strings.Split(bigEndpoints, ",")
		for i, endpoint := range endpoints {
			endpoints[i] = strings.TrimSpace(endpoint)
		}
		// Filter out empty strings
		filteredEndpoints := make([]string, 0, len(endpoints))
		for _, endpoint := range endpoints {
			if endpoint != "" {
				filteredEndpoints = append(filteredEndpoints, endpoint)
			}
		}
		cfg.BigModelEndpoints = filteredEndpoints
		cfg.logInfo("configuration", "request", "", "Configured BIG_MODEL_ENDPOINT", map[string]interface{}{
			"endpoints": cfg.BigModelEndpoints,
			"endpoint_count": len(cfg.BigModelEndpoints),
		})
	} else {
		return nil, fmt.Errorf("BIG_MODEL_ENDPOINT must be set in .env file")
	}

	// Parse SMALL_MODEL_ENDPOINT (comma-separated list)
	if smallEndpoints, exists := envVars["SMALL_MODEL_ENDPOINT"]; exists && smallEndpoints != "" {
		endpoints := strings.Split(smallEndpoints, ",")
		for i, endpoint := range endpoints {
			endpoints[i] = strings.TrimSpace(endpoint)
		}
		// Filter out empty strings
		filteredEndpoints := make([]string, 0, len(endpoints))
		for _, endpoint := range endpoints {
			if endpoint != "" {
				filteredEndpoints = append(filteredEndpoints, endpoint)
			}
		}
		cfg.SmallModelEndpoints = filteredEndpoints
		cfg.logInfo("configuration", "request", "", "Configured SMALL_MODEL_ENDPOINT", map[string]interface{}{
			"endpoints": cfg.SmallModelEndpoints,
			"endpoint_count": len(cfg.SmallModelEndpoints),
		})
	} else {
		return nil, fmt.Errorf("SMALL_MODEL_ENDPOINT must be set in .env file")
	}

	if bigAPIKey, exists := envVars["BIG_MODEL_API_KEY"]; exists && bigAPIKey != "" {
		cfg.BigModelAPIKey = bigAPIKey
		cfg.logInfo("configuration", "request", "", "Configured BIG_MODEL_API_KEY", map[string]interface{}{
			"api_key_masked": maskAPIKey(bigAPIKey),
		})
	} else {
		return nil, fmt.Errorf("BIG_MODEL_API_KEY must be set in .env file")
	}

	if smallAPIKey, exists := envVars["SMALL_MODEL_API_KEY"]; exists && smallAPIKey != "" {
		cfg.SmallModelAPIKey = smallAPIKey
		cfg.logInfo("configuration", "request", "", "Configured SMALL_MODEL_API_KEY", map[string]interface{}{
			"api_key_masked": maskAPIKey(smallAPIKey),
		})
	} else {
		return nil, fmt.Errorf("SMALL_MODEL_API_KEY must be set in .env file")
	}

	// Parse TOOL_CORRECTION_ENDPOINT (comma-separated list)
	if toolCorrectionEndpoints, exists := envVars["TOOL_CORRECTION_ENDPOINT"]; exists && toolCorrectionEndpoints != "" {
		endpoints := strings.Split(toolCorrectionEndpoints, ",")
		for i, endpoint := range endpoints {
			endpoints[i] = strings.TrimSpace(endpoint)
		}
		// Filter out empty strings
		filteredEndpoints := make([]string, 0, len(endpoints))
		for _, endpoint := range endpoints {
			if endpoint != "" {
				filteredEndpoints = append(filteredEndpoints, endpoint)
			}
		}
		cfg.ToolCorrectionEndpoints = filteredEndpoints
		cfg.logInfo("configuration", "request", "", "Configured TOOL_CORRECTION_ENDPOINT", map[string]interface{}{
			"endpoints": cfg.ToolCorrectionEndpoints,
			"endpoint_count": len(cfg.ToolCorrectionEndpoints),
		})
	} else {
		return nil, fmt.Errorf("TOOL_CORRECTION_ENDPOINT must be set in .env file")
	}

	if toolCorrectionAPIKey, exists := envVars["TOOL_CORRECTION_API_KEY"]; exists && toolCorrectionAPIKey != "" {
		cfg.ToolCorrectionAPIKey = toolCorrectionAPIKey
		cfg.logInfo("configuration", "request", "", "Configured TOOL_CORRECTION_API_KEY", map[string]interface{}{
			"api_key_masked": maskAPIKey(toolCorrectionAPIKey),
		})
	} else {
		return nil, fmt.Errorf("TOOL_CORRECTION_API_KEY must be set in .env file")
	}

	// Parse SKIP_TOOLS (optional, comma-separated list)
	if skipTools, exists := envVars["SKIP_TOOLS"]; exists && skipTools != "" {
		// Split by comma and trim whitespace
		tools := strings.Split(skipTools, ",")
		for i, tool := range tools {
			tools[i] = strings.TrimSpace(tool)
		}
		// Filter out empty strings
		filteredTools := make([]string, 0, len(tools))
		for _, tool := range tools {
			if tool != "" {
				filteredTools = append(filteredTools, tool)
			}
		}
		cfg.SkipTools = filteredTools
		cfg.logInfo("configuration", "request", "", "Configured SKIP_TOOLS", map[string]interface{}{
			"skip_tools": cfg.SkipTools,
		})
	}

	// Parse PRINT_SYSTEM_MESSAGE (optional, defaults to false)
	if printSystemMessage, exists := envVars["PRINT_SYSTEM_MESSAGE"]; exists {
		if printSystemMessage == "true" || printSystemMessage == "1" {
			cfg.PrintSystemMessage = true
			cfg.logInfo("configuration", "request", "", "Configured PRINT_SYSTEM_MESSAGE", map[string]interface{}{
				"enabled": true,
			})
		} else {
			cfg.PrintSystemMessage = false
			cfg.logInfo("configuration", "request", "", "Configured PRINT_SYSTEM_MESSAGE", map[string]interface{}{
				"enabled": false,
			})
		}
	}

	// Parse PRINT_TOOL_SCHEMAS (optional, defaults to false)
	if printToolSchemas, exists := envVars["PRINT_TOOL_SCHEMAS"]; exists {
		if printToolSchemas == "true" || printToolSchemas == "1" {
			cfg.PrintToolSchemas = true
			cfg.logInfo("configuration", "request", "", "Configured PRINT_TOOL_SCHEMAS", map[string]interface{}{
				"enabled": true,
			})
		} else {
			cfg.PrintToolSchemas = false
			cfg.logInfo("configuration", "request", "", "Configured PRINT_TOOL_SCHEMAS", map[string]interface{}{
				"enabled": false,
			})
		}
	}

	// Parse DISABLE_SMALL_MODEL_LOGGING (optional, defaults to false)
	if disableSmallLogging, exists := envVars["DISABLE_SMALL_MODEL_LOGGING"]; exists {
		if disableSmallLogging == "true" || disableSmallLogging == "1" {
			cfg.DisableSmallModelLogging = true
			cfg.logInfo("configuration", "request", "", "Configured DISABLE_SMALL_MODEL_LOGGING", map[string]interface{}{
				"enabled": true,
				"description": "Haiku logging disabled",
			})
		} else {
			cfg.DisableSmallModelLogging = false
			cfg.logInfo("configuration", "request", "", "Configured DISABLE_SMALL_MODEL_LOGGING", map[string]interface{}{
				"enabled": false,
				"description": "normal logging",
			})
		}
	}

	// Parse DISABLE_TOOL_CORRECTION_LOGGING (optional, defaults to false)
	if disableToolCorrectionLogging, exists := envVars["DISABLE_TOOL_CORRECTION_LOGGING"]; exists {
		if disableToolCorrectionLogging == "true" || disableToolCorrectionLogging == "1" {
			cfg.DisableToolCorrectionLogging = true
			cfg.logInfo("configuration", "request", "", "Configured DISABLE_TOOL_CORRECTION_LOGGING", map[string]interface{}{
				"enabled": true,
				"description": "tool correction logging disabled",
			})
		} else {
			cfg.DisableToolCorrectionLogging = false
			cfg.logInfo("configuration", "request", "", "Configured DISABLE_TOOL_CORRECTION_LOGGING", map[string]interface{}{
				"enabled": false,
				"description": "normal logging",
			})
		}
	}

	// Parse HANDLE_EMPTY_TOOL_RESULTS (optional, defaults to true)
	if handleEmptyResults, exists := envVars["HANDLE_EMPTY_TOOL_RESULTS"]; exists {
		if handleEmptyResults == "false" || handleEmptyResults == "0" {
			cfg.HandleEmptyToolResults = false
			cfg.logInfo("configuration", "request", "", "Configured HANDLE_EMPTY_TOOL_RESULTS", map[string]interface{}{
				"enabled": false,
			})
		} else {
			cfg.HandleEmptyToolResults = true
			cfg.logInfo("configuration", "request", "", "Configured HANDLE_EMPTY_TOOL_RESULTS", map[string]interface{}{
				"enabled": true,
			})
		}
	}

	// Parse HANDLE_EMPTY_USER_MESSAGES (optional, defaults to false)
	if handleEmptyUser, exists := envVars["HANDLE_EMPTY_USER_MESSAGES"]; exists {
		if handleEmptyUser == "true" || handleEmptyUser == "1" {
			cfg.HandleEmptyUserMessages = true
			cfg.logInfo("configuration", "request", "", "Configured HANDLE_EMPTY_USER_MESSAGES", map[string]interface{}{
				"enabled": true,
			})
		} else {
			cfg.HandleEmptyUserMessages = false
			cfg.logInfo("configuration", "request", "", "Configured HANDLE_EMPTY_USER_MESSAGES", map[string]interface{}{
				"enabled": false,
			})
		}
	}

	// Parse CONVERSATION_LOGGING_ENABLED (optional, defaults to false)
	if conversationLogging, exists := envVars["CONVERSATION_LOGGING_ENABLED"]; exists {
		if conversationLogging == "true" || conversationLogging == "1" {
			cfg.ConversationLoggingEnabled = true
			cfg.logInfo("configuration", "request", "", "Configured CONVERSATION_LOGGING_ENABLED", map[string]interface{}{
				"enabled": true,
			})
		} else {
			cfg.ConversationLoggingEnabled = false
			cfg.logInfo("configuration", "request", "", "Configured CONVERSATION_LOGGING_ENABLED", map[string]interface{}{
				"enabled": false,
			})
		}
	}

	// Parse CONVERSATION_LOG_LEVEL (optional, defaults to INFO)
	if logLevel, exists := envVars["CONVERSATION_LOG_LEVEL"]; exists {
		validLevels := map[string]bool{"DEBUG": true, "INFO": true, "WARN": true, "ERROR": true}
		if validLevels[strings.ToUpper(logLevel)] {
			cfg.ConversationLogLevel = strings.ToUpper(logLevel)
			cfg.logInfo("configuration", "request", "", "Configured CONVERSATION_LOG_LEVEL", map[string]interface{}{
				"log_level": cfg.ConversationLogLevel,
			})
		} else {
			cfg.logWarn("configuration", "warning", "", "Invalid CONVERSATION_LOG_LEVEL, using default", map[string]interface{}{
				"invalid_level": logLevel,
				"default_level": "INFO",
			})
			cfg.ConversationLogLevel = "INFO"
		}
	}

	// Parse CONVERSATION_MASK_SENSITIVE (optional, defaults to true)
	if maskSensitive, exists := envVars["CONVERSATION_MASK_SENSITIVE"]; exists {
		if maskSensitive == "false" || maskSensitive == "0" {
			cfg.ConversationMaskSensitive = false
			cfg.logInfo("configuration", "request", "", "Configured CONVERSATION_MASK_SENSITIVE", map[string]interface{}{
				"enabled": false,
			})
		} else {
			cfg.ConversationMaskSensitive = true
			cfg.logInfo("configuration", "request", "", "Configured CONVERSATION_MASK_SENSITIVE", map[string]interface{}{
				"enabled": true,
			})
		}
	}

	// Parse LOG_FULL_TOOLS (required)
	if logFullTools, exists := envVars["LOG_FULL_TOOLS"]; exists {
		if logFullTools == "true" || logFullTools == "1" {
			cfg.ConversationLogFullTools = true
			cfg.logInfo("configuration", "request", "", "Configured LOG_FULL_TOOLS", map[string]interface{}{
				"enabled": true,
				"description": "full tool definitions",
			})
		} else if logFullTools == "false" || logFullTools == "0" {
			cfg.ConversationLogFullTools = false
			cfg.logInfo("configuration", "request", "", "Configured LOG_FULL_TOOLS", map[string]interface{}{
				"enabled": false,
				"description": "tool names only",
			})
		} else {
			return nil, fmt.Errorf("LOG_FULL_TOOLS must be 'true' or 'false', got: %s", logFullTools)
		}
	} else {
		return nil, fmt.Errorf("LOG_FULL_TOOLS must be set in .env file")
	}

	// Parse CONVERSATION_TRUNCATION (required)
	if truncation, exists := envVars["CONVERSATION_TRUNCATION"]; exists {
		if truncation == "false" || truncation == "0" {
			cfg.ConversationTruncation = 0
			cfg.logInfo("configuration", "request", "", "Configured CONVERSATION_TRUNCATION", map[string]interface{}{
				"enabled": false,
			})
		} else {
			// Try to parse as integer
			var truncationValue int
			if n, err := fmt.Sscanf(truncation, "%d", &truncationValue); n != 1 || err != nil {
				return nil, fmt.Errorf("CONVERSATION_TRUNCATION must be 'false' or a positive number, got: %s", truncation)
			}
			if truncationValue < 0 {
				return nil, fmt.Errorf("CONVERSATION_TRUNCATION must be a positive number, got: %d", truncationValue)
			}
			cfg.ConversationTruncation = truncationValue
			cfg.logInfo("configuration", "request", "", "Configured CONVERSATION_TRUNCATION", map[string]interface{}{
				"enabled": true,
				"max_characters": truncationValue,
			})
		}
	} else {
		return nil, fmt.Errorf("CONVERSATION_TRUNCATION must be set in .env file")
	}

	// Parse DEFAULT_CONNECTION_TIMEOUT (optional, defaults to 30 seconds)
	if connectionTimeout, exists := envVars["DEFAULT_CONNECTION_TIMEOUT"]; exists {
		var timeoutValue int
		if n, err := fmt.Sscanf(connectionTimeout, "%d", &timeoutValue); n != 1 || err != nil {
			return nil, fmt.Errorf("DEFAULT_CONNECTION_TIMEOUT must be a positive number, got: %s", connectionTimeout)
		}
		if timeoutValue < 1 {
			return nil, fmt.Errorf("DEFAULT_CONNECTION_TIMEOUT must be a positive number, got: %d", timeoutValue)
		}
		cfg.DefaultConnectionTimeout = timeoutValue
		cfg.logInfo("configuration", "request", "", "Configured DEFAULT_CONNECTION_TIMEOUT", map[string]interface{}{
			"timeout_seconds": timeoutValue,
		})
	} else {
		cfg.DefaultConnectionTimeout = 30 // Default to 30 seconds if not specified
		cfg.logInfo("configuration", "request", "", "Using default DEFAULT_CONNECTION_TIMEOUT", map[string]interface{}{
			"timeout_seconds": 30,
		})
	}

	// Parse ENABLE_TOOL_CHOICE_CORRECTION (optional, defaults to false)
	if enableToolChoiceCorrection, exists := envVars["ENABLE_TOOL_CHOICE_CORRECTION"]; exists {
		if enableToolChoiceCorrection == "true" || enableToolChoiceCorrection == "1" {
			cfg.EnableToolChoiceCorrection = true
			cfg.logInfo("configuration", "request", "", "Configured ENABLE_TOOL_CHOICE_CORRECTION", map[string]interface{}{
				"enabled": true,
				"description": "tool choice correction enabled",
			})
		} else {
			cfg.EnableToolChoiceCorrection = false
			cfg.logInfo("configuration", "request", "", "Configured ENABLE_TOOL_CHOICE_CORRECTION", map[string]interface{}{
				"enabled": false,
				"description": "tool choice correction disabled",
			})
		}
	}

	// Parse HARMONY_PARSING_ENABLED (optional, defaults to true)
	if harmonyParsingEnabled, exists := envVars["HARMONY_PARSING_ENABLED"]; exists {
		if harmonyParsingEnabled == "false" || harmonyParsingEnabled == "0" {
			cfg.HarmonyParsingEnabled = false
			cfg.logInfo("configuration", "request", "", "Configured HARMONY_PARSING_ENABLED", map[string]interface{}{
				"enabled": false,
				"description": "Harmony parsing disabled",
			})
		} else {
			cfg.HarmonyParsingEnabled = true
			cfg.logInfo("configuration", "request", "", "Configured HARMONY_PARSING_ENABLED", map[string]interface{}{
				"enabled": true,
				"description": "Harmony parsing enabled",
			})
		}
	}

	// Parse HARMONY_DEBUG (optional, defaults to false)
	if harmonyDebug, exists := envVars["HARMONY_DEBUG"]; exists {
		if harmonyDebug == "true" || harmonyDebug == "1" {
			cfg.HarmonyDebug = true
			cfg.logInfo("configuration", "request", "", "Configured HARMONY_DEBUG", map[string]interface{}{
				"enabled": true,
				"description": "Harmony debug logging enabled",
			})
		} else {
			cfg.HarmonyDebug = false
			cfg.logInfo("configuration", "request", "", "Configured HARMONY_DEBUG", map[string]interface{}{
				"enabled": false,
				"description": "Harmony debug logging disabled",
			})
		}
	}

	// Parse HARMONY_STRICT_MODE (optional, defaults to false)
	if harmonyStrictMode, exists := envVars["HARMONY_STRICT_MODE"]; exists {
		if harmonyStrictMode == "true" || harmonyStrictMode == "1" {
			cfg.HarmonyStrictMode = true
			cfg.logInfo("configuration", "request", "", "Configured HARMONY_STRICT_MODE", map[string]interface{}{
				"enabled": true,
				"description": "Harmony strict error handling enabled",
			})
		} else {
			cfg.HarmonyStrictMode = false
			cfg.logInfo("configuration", "request", "", "Configured HARMONY_STRICT_MODE", map[string]interface{}{
				"enabled": false,
				"description": "Harmony lenient error handling (default)",
			})
		}
	}

	// Load tool description overrides from YAML file
	toolDescriptions, err := LoadToolDescriptions()
	if err != nil {
		cfg.logWarn("configuration", "warning", "", "Failed to load tool descriptions from tools_override.yaml", map[string]interface{}{
			"error": err.Error(),
		})
		// Continue with empty tool descriptions instead of failing
	} else {
		cfg.ToolDescriptions = toolDescriptions
	}

	// Load system message overrides from YAML file
	systemOverrides, err := LoadSystemMessageOverrides()
	if err != nil {
		cfg.logWarn("configuration", "warning", "", "Failed to load system message overrides from system_overrides.yaml", map[string]interface{}{
			"error": err.Error(),
		})
		// Continue with empty system overrides instead of failing
	} else {
		cfg.SystemMessageOverrides = systemOverrides
	}

	// Initialize circuit breaker health tracking
	cfg.HealthManager = circuitbreaker.NewHealthManager(circuitbreaker.DefaultConfig())
	allEndpoints := append(cfg.BigModelEndpoints, cfg.SmallModelEndpoints...)
	allEndpoints = append(allEndpoints, cfg.ToolCorrectionEndpoints...)
	cfg.HealthManager.InitializeEndpoints(allEndpoints)

	return cfg, nil
}

// maskAPIKey masks an API key for safe logging
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "***"
	}
	return apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
}

// loadEnvFile loads environment variables from .env file in current directory
func loadEnvFile() (map[string]string, error) {
	envVars := make(map[string]string)

	file, err := os.Open(".env")
	if err != nil {
		return envVars, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE format
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove comments from value
		if commentIndex := strings.Index(value, "#"); commentIndex != -1 {
			value = strings.TrimSpace(value[:commentIndex])
		}

		envVars[key] = value
	}

	return envVars, scanner.Err()
}

// MapModelName translates Claude Code model identifiers to configured provider-specific
// model names, enabling flexible model routing without hardcoded dependencies.
//
// This method provides the critical translation layer between Claude Code's
// standardized model names and the actual model identifiers used by configured
// providers. The mapping enables provider flexibility while maintaining
// consistent model selection logic.
//
// Current model mappings:
//   - claude-3-5-haiku-20241022 → SMALL_MODEL (configured via .env)
//   - claude-sonnet-4-20250514 → BIG_MODEL (configured via .env)
//
// The mapping system enables:
//   - Provider-agnostic model selection
//   - Easy provider switching through configuration
//   - Model name abstraction for Claude Code
//   - Centralized model routing logic
//
// Parameters:
//   - ctx: Request context for logging correlation
//   - claudeModel: Claude Code model identifier
//
// Returns:
//   - The corresponding provider-specific model name
//   - Original model name if no mapping exists (graceful fallback)
//
// Performance: O(1) constant time lookup with map access.
//
// Example:
//
//	mappedModel := config.MapModelName(ctx, "claude-sonnet-4-20250514")
//	// mappedModel contains the BIG_MODEL value from .env
func (c *Config) MapModelName(ctx context.Context, claudeModel string) string {
	// Extract request ID from context for logging (if available)
	requestID := internal.GetRequestID(ctx)

	modelMap := map[string]string{
		"claude-3-5-haiku-20241022": c.SmallModel, // Haiku → SMALL_MODEL
		"claude-sonnet-4-20250514":  c.BigModel,   // Sonnet → BIG_MODEL
		// Add other mappings as needed
	}

	if mapped, exists := modelMap[claudeModel]; exists {
		// Only log model mapping if it's not a small model (to avoid spam from disabled small model logging)
		if !c.DisableSmallModelLogging || mapped != c.SmallModel {
			c.logInfo("configuration", "request", requestID, "Model mapping applied", map[string]interface{}{
				"from_model": claudeModel,
				"to_model": mapped,
			})
		}
		return mapped
	}

	// Default to original name if no mapping exists
	return claudeModel
}

// GetToolDescription returns the appropriate tool description, using override
// descriptions when available or falling back to the original description.
//
// This method enables tool description customization through the tools_override.yaml
// configuration file, allowing administrators to:
//   - Customize tool descriptions for specific environments
//   - Add organization-specific tool usage guidance
//   - Override tool descriptions without code changes
//   - Maintain tool functionality while adapting documentation
//
// The method provides a clean abstraction over the tool description resolution
// logic, ensuring consistent behavior across the proxy system.
//
// Parameters:
//   - toolName: The name of the tool to look up
//   - originalDescription: The default tool description
//
// Returns:
//   - Override description if configured in tools_override.yaml
//   - Original description if no override is configured
//
// Performance: O(1) constant time map lookup.
//
// Example:
//
//	description := config.GetToolDescription("WebSearch", defaultDescription)
//	// Returns customized description if configured, original otherwise
func (c *Config) GetToolDescription(toolName, originalDescription string) string {
	return GetToolDescription(c.ToolDescriptions, toolName, originalDescription)
}

// ToolDescriptionsYAML represents the structure of tools_override.yaml
type ToolDescriptionsYAML struct {
	ToolDescriptions map[string]string `yaml:"toolDescriptions"`
}

// LoadToolDescriptions loads tool description overrides from tools_override.yaml,
// providing customizable tool documentation without requiring code changes.
//
// This function enables runtime customization of tool descriptions through
// YAML configuration, supporting environment-specific tool documentation
// and organizational customization requirements.
//
// The function provides graceful handling of missing configuration:
//   - Returns empty map (no error) if tools_override.yaml doesn't exist
//   - Enables optional tool customization without breaking basic functionality
//   - Supports incremental tool override adoption
//
// YAML file structure:
//   toolDescriptions:
//     WebSearch: "Custom search tool description"
//     Read: "Custom file reading tool description"
//
// Error handling:
//   - Missing file: Returns empty map, no error (graceful degradation)
//   - Invalid YAML: Returns error with parsing details
//   - File access issues: Returns error with file operation details
//
// Returns:
//   - Map of tool names to override descriptions
//   - Empty map if file doesn't exist (successful case)
//   - Error only for file access or parsing issues
//
// Performance: File I/O with YAML parsing, cached after initial load.
//
// Example:
//
//	overrides, err := LoadToolDescriptions()
//	if err != nil {
//		log.Printf("Tool override loading failed: %v", err)
//		// Continue with empty overrides
//	}
func LoadToolDescriptions() (map[string]string, error) {
	file, err := os.Open("tools_override.yaml")
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist - return empty map, no error
			// No structured logging available during config loading phase
			// This will be logged via main.go after obsLogger is available
			return make(map[string]string), nil
		}
		return nil, fmt.Errorf("failed to open tools_override.yaml: %v", err)
	}
	defer file.Close()

	var yamlData ToolDescriptionsYAML
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&yamlData); err != nil {
		return nil, fmt.Errorf("failed to parse tools_override.yaml: %v", err)
	}

	if yamlData.ToolDescriptions == nil {
		yamlData.ToolDescriptions = make(map[string]string)
	}

	// Tool descriptions will be logged by caller after obsLogger is available
	for range yamlData.ToolDescriptions {
		// Individual tool description details will be logged by caller
	}

	return yamlData.ToolDescriptions, nil
}

// GetToolDescription provides package-level tool description resolution with
// explicit override map parameter for flexible usage patterns.
//
// This function offers a pure, stateless approach to tool description resolution,
// enabling custom override map usage without requiring Config instance access.
// Useful for testing, custom override sources, and modular tool processing.
//
// The function implements the core tool description resolution logic:
//   - Check override map for custom description
//   - Return custom description if found
//   - Return original description if no override exists
//
// Parameters:
//   - overrides: Map of tool names to override descriptions
//   - toolName: The tool name to resolve
//   - originalDescription: Fallback description if no override exists
//
// Returns:
//   - Override description if present in the map
//   - Original description if no override is configured
//
// Performance: O(1) constant time map lookup.
//
// Example:
//
//	overrides := map[string]string{"WebSearch": "Custom search"}
//	description := GetToolDescription(overrides, "WebSearch", "Default search")
//	// Returns "Custom search"
func GetToolDescription(overrides map[string]string, toolName, originalDescription string) string {
	if override, exists := overrides[toolName]; exists {
		return override
	}
	return originalDescription
}

// SystemMessageReplacement represents a single find-and-replace operation
// for system message content modification, enabling precise text transformations.
//
// This struct defines atomic text replacement operations that can be applied
// to system messages, supporting content customization, branding updates,
// and environment-specific message modifications.
//
// The replacement operation:
//   - Find: Exact text string to locate and replace
//   - Replace: Replacement text to substitute
//
// Multiple replacements can be chained together for complex transformations,
// with each operation applied sequentially to the system message content.
type SystemMessageReplacement struct {
	Find    string `yaml:"find"`
	Replace string `yaml:"replace"`
}

// SystemMessageOverrides represents comprehensive system message modification
// configuration, enabling content customization through multiple transformation types.
//
// This struct provides complete control over system message content through:
//   - Pattern-based removal using regular expressions
//   - Find-and-replace text transformations
//   - Content prepending and appending operations
//
// Transformation order (applied sequentially):
//   1. RemovePatterns: Regex-based content removal
//   2. Replacements: Find-and-replace text substitutions
//   3. Prepend: Content added to message beginning
//   4. Append: Content added to message end
//
// This configuration enables comprehensive system message customization
// for branding, environment-specific instructions, and content filtering.
type SystemMessageOverrides struct {
	RemovePatterns []string                   `yaml:"removePatterns"`
	Replacements   []SystemMessageReplacement `yaml:"replacements"`
	Prepend        string                     `yaml:"prepend"`
	Append         string                     `yaml:"append"`
}

// SystemMessageOverridesYAML represents the structure of system_overrides.yaml
type SystemMessageOverridesYAML struct {
	SystemMessageOverrides SystemMessageOverrides `yaml:"systemMessageOverrides"`
}

// LoadSystemMessageOverrides loads system message modification configuration
// from system_overrides.yaml, enabling runtime system message customization.
//
// This function provides flexible system message customization through YAML
// configuration, supporting environment-specific message modifications,
// branding updates, and content filtering without code changes.
//
// The function handles missing configuration gracefully:
//   - Returns empty struct (no error) if system_overrides.yaml doesn't exist
//   - Enables optional system message customization
//   - Supports incremental override adoption
//
// YAML file structure:
//   systemMessageOverrides:
//     removePatterns:
//       - "pattern1"
//       - "pattern2"
//     replacements:
//       - find: "old text"
//         replace: "new text"
//     prepend: "Additional instructions\n"
//     append: "\nFooter content"
//
// Error handling:
//   - Missing file: Returns empty struct, no error (graceful)
//   - Invalid YAML: Returns error with parsing details
//   - File access issues: Returns error with operation details
//
// Returns:
//   - SystemMessageOverrides struct with loaded configuration
//   - Empty struct if file doesn't exist (successful case)
//   - Error only for file access or parsing issues
//
// Performance: File I/O with YAML parsing, cached after initial load.
func LoadSystemMessageOverrides() (SystemMessageOverrides, error) {
	file, err := os.Open("system_overrides.yaml")
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist - return empty struct, no error
			// No structured logging available during config loading phase
			// This will be logged via main.go after obsLogger is available
			return SystemMessageOverrides{}, nil
		}
		return SystemMessageOverrides{}, fmt.Errorf("failed to open system_overrides.yaml: %v", err)
	}
	defer file.Close()

	var yamlData SystemMessageOverridesYAML
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&yamlData); err != nil {
		return SystemMessageOverrides{}, fmt.Errorf("failed to parse system_overrides.yaml: %v", err)
	}

	overrides := yamlData.SystemMessageOverrides
	// System message override details will be logged by caller after obsLogger is available

	return overrides, nil
}

// ApplySystemMessageOverrides applies comprehensive system message modifications
// using the provided override configuration, transforming content through multiple stages.
//
// This function implements the complete system message transformation pipeline:
//   1. Pattern removal: Regex-based content filtering
//   2. Text replacement: Find-and-replace substitutions
//   3. Content addition: Prepend and append operations
//
// Transformation sequence (order is significant):
//   - removePatterns: Applied first using compiled regex patterns
//   - replacements: Applied to pattern-filtered content
//   - prepend/append: Applied last to finalize content
//
// The function provides detailed logging of all transformations for debugging
// and monitoring purposes, enabling administrators to verify modification behavior.
//
// Error handling:
//   - Invalid regex patterns: Logged as warnings, pattern skipped
//   - Processing continues with remaining valid patterns
//   - Graceful degradation ensures message processing completion
//
// Parameters:
//   - originalMessage: Source system message content
//   - overrides: Configuration specifying all modifications to apply
//
// Returns:
//   - Transformed message with all applicable modifications applied
//
// Performance: O(n*m) where n is message length and m is number of patterns/replacements.
//
// Example:
//
//	modified := ApplySystemMessageOverrides(original, overrides)
//	// Returns message with all configured transformations applied
func ApplySystemMessageOverrides(originalMessage string, overrides SystemMessageOverrides) string {
	message := originalMessage

	// Apply remove patterns (regex-based removal)
	for _, pattern := range overrides.RemovePatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			// Pattern validation error - use fallback logging since this is runtime processing
			log.Printf("⚠️  Warning: Invalid regex pattern '%s': %v", pattern, err)
			continue
		}

		// Find matches before removing them
		matches := re.FindAllString(message, -1)
		if len(matches) > 0 {
			for _, match := range matches {
				// Match removal - use fallback logging for system message processing
				log.Printf("🔍 removePattern detected, removed '%s' for pattern '%s'", match, pattern)
			}
			message = re.ReplaceAllString(message, "")
		}
	}

	// Apply replacements
	for _, replacement := range overrides.Replacements {
		if strings.Contains(message, replacement.Find) {
			oldMessage := message
			message = strings.ReplaceAll(message, replacement.Find, replacement.Replace)
			// Count occurrences replaced
			occurrences := strings.Count(oldMessage, replacement.Find)
			// Replacement applied - use fallback logging for system message processing
			log.Printf("🔄 replacement applied: '%s' → '%s' (%d occurrences)",
				replacement.Find, replacement.Replace, occurrences)
		}
	}

	// Apply prepend and append
	if overrides.Prepend != "" {
		message = overrides.Prepend + message
		// Prepend applied - use fallback logging for system message processing
		log.Printf("➕ prepend applied: '%s'", strings.TrimSpace(overrides.Prepend))
	}
	if overrides.Append != "" {
		message = message + overrides.Append
		// Append applied - use fallback logging for system message processing
		log.Printf("➕ append applied: '%s'", strings.TrimSpace(overrides.Append))
	}

	// Print updated system prompt
	//log.Printf("Modified system prompt:\n%s", message)

	return message
}

// GetBigModelEndpoint returns the next BIG_MODEL endpoint using simple round-robin
// rotation, optimized for long-running requests with extended processing times.
//
// This method provides endpoint selection for BIG_MODEL requests, which typically
// involve complex reasoning tasks that may require 30+ minutes of processing time.
// Due to these extended processing requirements, big model endpoints bypass
// circuit breaker logic to avoid false failure detection.
//
// Endpoint selection characteristics:
//   - Simple round-robin rotation without health checking
//   - No circuit breaker integration (extended processing time tolerance)
//   - Thread-safe endpoint index management
//   - Automatic wraparound for continuous rotation
//
// The method is optimized for scenarios where extended processing time is
// expected and normal, avoiding premature request termination due to timeouts.
//
// Returns:
//   - Next endpoint in the BIG_MODEL rotation sequence
//   - Empty string if no endpoints are configured
//
// Thread Safety: This method uses mutex protection for safe concurrent access.
//
// Performance: O(1) constant time with mutex synchronization.
//
// Example:
//
//	endpoint := config.GetBigModelEndpoint()
//	if endpoint != "" {
//		// Use endpoint for big model request
//	}
func (c *Config) GetBigModelEndpoint() string {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(c.BigModelEndpoints) == 0 {
		return ""
	}

	// Simple round-robin without circuit breaker for big models
	// (30+ minute processing time is acceptable for big models)
	endpoint := c.BigModelEndpoints[c.bigModelIndex%len(c.BigModelEndpoints)]
	c.bigModelIndex++
	
	return endpoint
}

// GetSmallModelEndpoint returns the next SMALL_MODEL endpoint using intelligent
// round-robin rotation with health-based failover and success rate optimization.
//
// This method provides sophisticated endpoint selection for SMALL_MODEL requests,
// incorporating circuit breaker health management and performance-based routing
// to maximize request success rates and minimize latency.
//
// Endpoint selection features:
//   - Health-aware round-robin with circuit breaker integration
//   - Success rate-based endpoint reordering for optimization
//   - Automatic failover to healthy endpoints
//   - Performance monitoring and adjustment
//
// The method prioritizes endpoints with better success rates while maintaining
// load distribution across healthy endpoints, ensuring optimal performance
// for high-frequency small model requests.
//
// Returns:
//   - Next healthy endpoint in the optimized rotation sequence
//   - Empty string if no endpoints are configured
//
// Thread Safety: This method uses mutex protection for safe concurrent access.
//
// Performance: O(n) where n is the number of endpoints, with health evaluation.
//
// Example:
//
//	endpoint := config.GetSmallModelEndpoint()
//	if endpoint != "" {
//		// Use healthy endpoint for small model request
//	}
func (c *Config) GetSmallModelEndpoint() string {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(c.SmallModelEndpoints) == 0 {
		return ""
	}

	// Reorder endpoints by success rate periodically
	c.HealthManager.ReorderBySuccess(c.SmallModelEndpoints, "SmallModel")

	return c.HealthManager.SelectHealthyEndpoint(c.SmallModelEndpoints, &c.smallModelIndex)
}

// GetToolCorrectionEndpoint returns the next TOOL_CORRECTION endpoint using
// intelligent round-robin rotation with health-based failover for tool validation.
//
// This method provides reliable endpoint selection for tool correction operations,
// ensuring high availability for critical tool call validation and correction
// processes that maintain system functionality.
//
// Endpoint selection features:
//   - Health-aware round-robin with circuit breaker integration
//   - Success rate-based endpoint reordering
//   - Automatic failover to healthy endpoints
//   - Detailed logging for tool correction endpoint selection
//
// Tool correction endpoints are critical for maintaining tool call functionality
// when tool validation fails, making reliable endpoint selection essential
// for system stability.
//
// Returns:
//   - Next healthy endpoint in the tool correction rotation sequence
//   - Empty string if no endpoints are configured
//
// Thread Safety: This method uses mutex protection for safe concurrent access.
//
// Performance: O(n) where n is the number of endpoints, with health evaluation.
//
// Example:
//
//	endpoint := config.GetToolCorrectionEndpoint()
//	if endpoint != "" {
//		// Use healthy endpoint for tool correction
//	}
func (c *Config) GetToolCorrectionEndpoint() string {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(c.ToolCorrectionEndpoints) == 0 {
		return ""
	}

	// Reorder endpoints by success rate periodically
	c.HealthManager.ReorderBySuccess(c.ToolCorrectionEndpoints, "ToolCorrection")

	return c.HealthManager.SelectHealthyEndpoint(c.ToolCorrectionEndpoints, &c.toolCorrectionIndex)
}

// IsEndpointHealthy checks whether the specified endpoint is currently healthy
// and available for request processing based on circuit breaker state.
//
// This method provides real-time health status for endpoints, enabling
// request routing decisions and load balancing based on endpoint availability.
// The health check considers circuit breaker state, failure counts, and
// recovery timing.
//
// Health determination factors:
//   - Circuit breaker state (open/closed/half-open)
//   - Recent failure counts and patterns
//   - Recovery timeouts and retry windows
//   - Success rate trends
//
// Parameters:
//   - endpoint: The endpoint URL to check for health status
//
// Returns:
//   - true if the endpoint is healthy and available for requests
//   - false if the endpoint is unhealthy or circuit breaker is open
//
// Thread Safety: This method is safe for concurrent access.
//
// Performance: O(1) constant time health state lookup.
//
// Example:
//
//	if config.IsEndpointHealthy(endpoint) {
//		// Route request to healthy endpoint
//	} else {
//		// Skip unhealthy endpoint
//	}
func (c *Config) IsEndpointHealthy(endpoint string) bool {
	return c.HealthManager.IsHealthy(endpoint)
}

// GetEndpointHealthDebug provides comprehensive debugging information about
// an endpoint's health status, circuit breaker state, and recovery timing.
//
// This method enables detailed health monitoring and troubleshooting by
// exposing internal health management state for administrative and
// debugging purposes.
//
// Debug information includes:
//   - failureCount: Number of consecutive failures recorded
//   - circuitOpen: Whether the circuit breaker is currently open
//   - nextRetryTime: When the endpoint will next be eligible for retry
//   - exists: Whether the endpoint is known to the health manager
//
// The information supports operational monitoring, capacity planning,
// and troubleshooting of endpoint availability issues.
//
// Parameters:
//   - endpoint: The endpoint URL to retrieve debug information for
//
// Returns:
//   - failureCount: Consecutive failure count for the endpoint
//   - circuitOpen: Circuit breaker open status
//   - nextRetryTime: Next retry attempt time
//   - exists: Whether the endpoint is tracked by health manager
//
// Thread Safety: This method is safe for concurrent access.
//
// Performance: O(1) constant time state lookup.
//
// Example:
//
//	failures, open, retry, exists := config.GetEndpointHealthDebug(endpoint)
//	if exists {
//		log.Printf("Endpoint %s: %d failures, circuit open: %v, retry: %v",
//			endpoint, failures, open, retry)
//	}
func (c *Config) GetEndpointHealthDebug(endpoint string) (failureCount int, circuitOpen bool, nextRetryTime time.Time, exists bool) {
	return c.HealthManager.GetHealthDebug(endpoint)
}

// RecordEndpointFailure registers a failure event for the specified endpoint,
// updating circuit breaker state and potentially opening the circuit for protection.
//
// This method maintains endpoint health tracking by recording failure events
// and triggering circuit breaker logic when failure thresholds are exceeded.
// Proper failure recording is essential for maintaining system stability
// and preventing cascading failures.
//
// Failure recording effects:
//   - Increments consecutive failure count for the endpoint
//   - Evaluates circuit breaker threshold conditions
//   - Opens circuit breaker if failure threshold is exceeded
//   - Updates failure timing for recovery calculations
//
// The method should be called immediately after detecting endpoint failures
// to ensure accurate health tracking and timely circuit breaker activation.
//
// Parameters:
//   - endpoint: The endpoint URL that experienced a failure
//
// Thread Safety: This method is safe for concurrent access.
//
// Performance: O(1) constant time failure recording with minimal overhead.
//
// Example:
//
//	if requestFailed {
//		config.RecordEndpointFailure(endpoint)
//		// Circuit breaker may now be open for this endpoint
//	}
func (c *Config) RecordEndpointFailure(endpoint string) {
	c.HealthManager.RecordFailure(endpoint)
}

// RecordEndpointSuccess registers a success event for the specified endpoint,
// updating circuit breaker state and potentially closing the circuit for recovery.
//
// This method maintains endpoint health tracking by recording successful
// request completions and triggering circuit breaker recovery logic when
// success patterns indicate endpoint recovery.
//
// Success recording effects:
//   - Resets consecutive failure count for the endpoint
//   - Evaluates circuit breaker recovery conditions
//   - Closes circuit breaker if recovery criteria are met
//   - Updates success timing for performance tracking
//
// The method should be called after successful request completion to ensure
// accurate health tracking and timely circuit breaker recovery.
//
// Parameters:
//   - endpoint: The endpoint URL that completed successfully
//
// Thread Safety: This method is safe for concurrent access.
//
// Performance: O(1) constant time success recording with minimal overhead.
//
// Example:
//
//	if requestSucceeded {
//		config.RecordEndpointSuccess(endpoint)
//		// Circuit breaker may now be closed for this endpoint
//	}
func (c *Config) RecordEndpointSuccess(endpoint string) {
	c.HealthManager.RecordSuccess(endpoint)
}

// GetHealthySmallModelEndpoint returns the next healthy SMALL_MODEL endpoint
func (c *Config) GetHealthySmallModelEndpoint() string {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(c.SmallModelEndpoints) == 0 {
		return ""
	}

	// Reorder endpoints by success rate periodically
	c.HealthManager.ReorderBySuccess(c.SmallModelEndpoints, "SmallModel")

	return c.HealthManager.SelectHealthyEndpoint(c.SmallModelEndpoints, &c.smallModelIndex)
}

// GetHealthyToolCorrectionEndpoint returns the next healthy TOOL_CORRECTION endpoint
func (c *Config) GetHealthyToolCorrectionEndpoint() string {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(c.ToolCorrectionEndpoints) == 0 {
		return ""
	}

	// Reorder endpoints by success rate periodically
	c.HealthManager.ReorderBySuccess(c.ToolCorrectionEndpoints, "ToolCorrection")

	endpoint := c.HealthManager.SelectHealthyEndpoint(c.ToolCorrectionEndpoints, &c.toolCorrectionIndex)
	if endpoint != "" {
		c.logInfo("configuration", "request", "", "Selected healthy tool correction endpoint", map[string]interface{}{
			"endpoint": endpoint,
		})
	}
	return endpoint
}

// GetEnableToolChoiceCorrection returns whether tool choice correction is enabled
func (c *Config) GetEnableToolChoiceCorrection() bool {
	return c.EnableToolChoiceCorrection
}

// MarkEndpointFailed moves to the next endpoint when the current one fails
func (c *Config) MarkEndpointFailed(endpointType string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	switch endpointType {
	case "big_model":
		if len(c.BigModelEndpoints) > 1 {
			c.bigModelIndex = (c.bigModelIndex + 1) % len(c.BigModelEndpoints)
			c.logWarn("configuration", "warning", "", "Big model endpoint failed, switching to next", map[string]interface{}{
				"new_index": c.bigModelIndex,
				"total_endpoints": len(c.BigModelEndpoints),
			})
		}
	case "small_model":
		if len(c.SmallModelEndpoints) > 1 {
			c.smallModelIndex = (c.smallModelIndex + 1) % len(c.SmallModelEndpoints)
			c.logWarn("configuration", "warning", "", "Small model endpoint failed, switching to next", map[string]interface{}{
				"new_index": c.smallModelIndex,
				"total_endpoints": len(c.SmallModelEndpoints),
			})
		}
	case "tool_correction":
		if len(c.ToolCorrectionEndpoints) > 1 {
			c.toolCorrectionIndex = (c.toolCorrectionIndex + 1) % len(c.ToolCorrectionEndpoints)
			c.logWarn("configuration", "warning", "", "Tool correction endpoint failed, switching to next", map[string]interface{}{
				"new_index": c.toolCorrectionIndex,
				"total_endpoints": len(c.ToolCorrectionEndpoints),
			})
		}
	}
}

// Harmony configuration API methods

// IsHarmonyParsingEnabled returns whether Harmony format parsing is currently
// enabled in the proxy configuration, controlling response processing behavior.
//
// This method provides access to the HARMONY_PARSING_ENABLED configuration
// setting, which determines whether the proxy should attempt to parse and
// process OpenAI Harmony format responses for enhanced Claude Code UI support.
//
// When Harmony parsing is enabled:
//   - Response content is analyzed for Harmony format tokens
//   - Thinking and response content are separated
//   - Channel metadata is preserved for debugging
//   - Enhanced UI rendering is supported
//
// When disabled, responses are processed as standard text without
// Harmony-specific formatting or content separation.
//
// Returns:
//   - true if Harmony parsing is enabled
//   - false if Harmony parsing is disabled
//
// Thread Safety: This method is safe for concurrent access (read-only).
//
// Example:
//
//	if config.IsHarmonyParsingEnabled() {
//		// Parse response for Harmony content
//	} else {
//		// Process as standard response
//	}
func (c *Config) IsHarmonyParsingEnabled() bool {
	return c.HarmonyParsingEnabled
}

// IsHarmonyDebugEnabled returns whether detailed Harmony debug logging is
// currently enabled, controlling the verbosity of Harmony parsing operations.
//
// This method provides access to the HARMONY_DEBUG configuration setting,
// which determines whether the proxy should generate detailed logging
// information during Harmony parsing operations for troubleshooting and monitoring.
//
// When Harmony debug logging is enabled:
//   - Detailed token parsing information is logged
//   - Channel extraction steps are documented
//   - Content transformation details are recorded
//   - Parsing errors and warnings are expanded
//
// Debug logging should typically be enabled only in development or
// troubleshooting scenarios due to increased log volume.
//
// Returns:
//   - true if Harmony debug logging is enabled
//   - false if Harmony debug logging is disabled
//
// Thread Safety: This method is safe for concurrent access (read-only).
//
// Example:
//
//	if config.IsHarmonyDebugEnabled() {
//		log.Printf("Harmony parsing details: %+v", parseResult)
//	}
func (c *Config) IsHarmonyDebugEnabled() bool {
	return c.HarmonyDebug
}

// IsHarmonyStrictModeEnabled returns whether Harmony strict error handling mode
// is currently enabled, controlling parser behavior when encountering malformed content.
//
// This method provides access to the HARMONY_STRICT_MODE configuration setting,
// which determines how the Harmony parser should handle structural errors
// and malformed token sequences.
//
// Strict mode behavior:
//   - Parsing errors cause request failures
//   - Malformed tokens result in error responses
//   - Structural validation is enforced strictly
//   - Invalid content is rejected rather than ignored
//
// Lenient mode behavior (default):
//   - Parsing errors are logged but processing continues
//   - Malformed tokens are skipped gracefully
//   - Partial parsing results are accepted
//   - Best-effort content extraction is performed
//
// Returns:
//   - true if Harmony strict mode is enabled
//   - false if lenient mode is enabled (default)
//
// Thread Safety: This method is safe for concurrent access (read-only).
//
// Example:
//
//	if config.IsHarmonyStrictModeEnabled() {
//		// Fail request on parsing errors
//	} else {
//		// Continue with partial results
//	}
func (c *Config) IsHarmonyStrictModeEnabled() bool {
	return c.HarmonyStrictMode
}

// HarmonyConfiguration represents a complete snapshot of all Harmony-related
// configuration settings, providing a unified view of parsing behavior control.
//
// This struct aggregates all Harmony configuration settings into a single
// structure for convenient access, serialization, and configuration management.
// It enables atomic configuration queries and simplified configuration sharing.
//
// Configuration fields:
//   - ParsingEnabled: Whether Harmony parsing is active
//   - Debug: Whether detailed debug logging is enabled
//   - StrictMode: Whether strict error handling is enforced
//
// The struct is designed for read-only configuration access and can be
// safely serialized for monitoring, logging, and configuration validation.
type HarmonyConfiguration struct {
	ParsingEnabled bool `json:"parsing_enabled"`
	Debug          bool `json:"debug"`
	StrictMode     bool `json:"strict_mode"`
}

// GetHarmonyConfiguration returns a complete snapshot of all Harmony-related
// configuration settings as a unified structure for convenient access.
//
// This method provides atomic access to all Harmony configuration settings,
// ensuring consistent configuration state across related operations and
// enabling simplified configuration management and monitoring.
//
// The returned configuration includes:
//   - Parsing enablement status
//   - Debug logging configuration
//   - Error handling mode settings
//
// Returns:
//   - HarmonyConfiguration struct with current settings
//
// Thread Safety: This method is safe for concurrent access (read-only).
//
// Performance: O(1) constant time configuration access.
//
// Example:
//
//	harmonyConfig := config.GetHarmonyConfiguration()
//	log.Printf("Harmony config: parsing=%v, debug=%v, strict=%v",
//		harmonyConfig.ParsingEnabled, harmonyConfig.Debug, harmonyConfig.StrictMode)
func (c *Config) GetHarmonyConfiguration() HarmonyConfiguration {
	return HarmonyConfiguration{
		ParsingEnabled: c.HarmonyParsingEnabled,
		Debug:          c.HarmonyDebug,
		StrictMode:     c.HarmonyStrictMode,
	}
}
