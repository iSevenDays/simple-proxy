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

// Config represents the proxy configuration - all settings from .env
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

	// System message overrides (loaded from system_overrides.yaml)
	SystemMessageOverrides SystemMessageOverrides `json:"system_message_overrides"`

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

// SetObservabilityLogger sets the observability logger for structured logging
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

// GetDefaultConfig returns a default configuration for testing
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
		SystemMessageOverrides:       SystemMessageOverrides{}, // Empty by default
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

// LoadConfigWithEnv loads configuration from .env file only - no CCR dependency
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
		ConversationLogFullTools:   false,                    // Log tool names only by default
		ConversationTruncation:     0,                        // No truncation by default
		DefaultConnectionTimeout:   30,                       // 30 seconds default connection timeout
		SystemMessageOverrides:     SystemMessageOverrides{}, // Empty by default
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

// MapModelName translates Claude Code model names to configured provider-specific names
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

// GetToolDescription returns the override description if available, otherwise returns original
func (c *Config) GetToolDescription(toolName, originalDescription string) string {
	return GetToolDescription(c.ToolDescriptions, toolName, originalDescription)
}

// ToolDescriptionsYAML represents the structure of tools_override.yaml
type ToolDescriptionsYAML struct {
	ToolDescriptions map[string]string `yaml:"toolDescriptions"`
}

// LoadToolDescriptions loads tool description overrides from tools_override.yaml
// Returns empty map if file doesn't exist (no error)
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

// GetToolDescription returns the override description if available, otherwise returns original
func GetToolDescription(overrides map[string]string, toolName, originalDescription string) string {
	if override, exists := overrides[toolName]; exists {
		return override
	}
	return originalDescription
}

// SystemMessageReplacement represents a find/replace operation for system messages
type SystemMessageReplacement struct {
	Find    string `yaml:"find"`
	Replace string `yaml:"replace"`
}

// SystemMessageOverrides represents system message modification configuration
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

// LoadSystemMessageOverrides loads system message overrides from system_overrides.yaml
// Returns empty struct if file doesn't exist (no error)
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

// ApplySystemMessageOverrides applies system message modifications
// Operations are applied in order: removePatterns -> replacements -> prepend/append
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

// GetBigModelEndpoint returns the next BIG_MODEL endpoint with simple round-robin
// Note: Big model endpoints bypass circuit breaker since 30min+ processing time is normal
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

// GetSmallModelEndpoint returns the next SMALL_MODEL endpoint with round-robin failover
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

// GetToolCorrectionEndpoint returns the next TOOL_CORRECTION endpoint with round-robin failover
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

// IsEndpointHealthy checks if an endpoint is available (circuit closed)
func (c *Config) IsEndpointHealthy(endpoint string) bool {
	return c.HealthManager.IsHealthy(endpoint)
}

// GetEndpointHealthDebug returns debug information about an endpoint's health
func (c *Config) GetEndpointHealthDebug(endpoint string) (failureCount int, circuitOpen bool, nextRetryTime time.Time, exists bool) {
	return c.HealthManager.GetHealthDebug(endpoint)
}

// RecordEndpointFailure marks an endpoint as failed and potentially opens its circuit
func (c *Config) RecordEndpointFailure(endpoint string) {
	c.HealthManager.RecordFailure(endpoint)
}

// RecordEndpointSuccess marks an endpoint as successful and potentially closes its circuit
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
