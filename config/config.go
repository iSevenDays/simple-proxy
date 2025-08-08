package config

import (
	"bufio"
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

// EndpointHealth tracks the health status of an endpoint
type EndpointHealth struct {
	URL             string    `json:"url"`
	FailureCount    int       `json:"failure_count"`
	LastFailureTime time.Time `json:"last_failure_time"`
	CircuitOpen     bool      `json:"circuit_open"`
	NextRetryTime   time.Time `json:"next_retry_time"`
}

// CircuitBreakerConfig controls circuit breaker behavior
type CircuitBreakerConfig struct {
	FailureThreshold   int           `json:"failure_threshold"`    // Number of failures before opening circuit
	BackoffDuration    time.Duration `json:"backoff_duration"`     // How long to wait before retrying failed endpoint
	MaxBackoffDuration time.Duration `json:"max_backoff_duration"` // Maximum backoff time
	ResetTimeout       time.Duration `json:"reset_timeout"`        // Time to reset failure count after success
}

// DefaultCircuitBreakerConfig returns sensible defaults for circuit breaker
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:   2,                // Open circuit after 2 consecutive failures
		BackoffDuration:    30 * time.Second, // Initial 30s backoff
		MaxBackoffDuration: 5 * time.Minute,  // Max 5min backoff
		ResetTimeout:       1 * time.Minute,  // Reset failure count after 1min of success
	}
}

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

	// Circuit breaker configuration and health tracking
	CircuitBreaker    CircuitBreakerConfig       `json:"circuit_breaker"`
	EndpointHealthMap map[string]*EndpointHealth `json:"-"`
	healthMutex       sync.RWMutex               `json:"-"`
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
		Port:                       "3456",                  // Default port
		ToolCorrectionEnabled:      true,                    // Enable by default
		HandleEmptyToolResults:     true,                    // Enable by default for API compliance
		SkipTools:                  []string{},              // Empty by default
		ToolDescriptions:           make(map[string]string), // Empty by default
		PrintSystemMessage:         false,                   // Disabled by default
		PrintToolSchemas:           false,                   // Disabled by default
		ConversationLoggingEnabled: false,                   // Disabled by default
		ConversationLogLevel:       "INFO",                  // Default to INFO level
		ConversationMaskSensitive:  true,                    // Enable sensitive data masking by default
		CircuitBreaker:             DefaultCircuitBreakerConfig(),
		SystemMessageOverrides:     SystemMessageOverrides{}, // Empty by default
	}

	// All models and endpoints are required when .env exists - no fallbacks
	if bigModel, exists := envVars["BIG_MODEL"]; exists && bigModel != "" {
		cfg.BigModel = bigModel
		log.Printf("🔧 Configured BIG_MODEL: %s", bigModel)
	} else {
		return nil, fmt.Errorf("BIG_MODEL must be set in .env file")
	}

	if smallModel, exists := envVars["SMALL_MODEL"]; exists && smallModel != "" {
		cfg.SmallModel = smallModel
		log.Printf("🔧 Configured SMALL_MODEL: %s", smallModel)
	} else {
		return nil, fmt.Errorf("SMALL_MODEL must be set in .env file")
	}

	if correctionModel, exists := envVars["CORRECTION_MODEL"]; exists && correctionModel != "" {
		cfg.CorrectionModel = correctionModel
		log.Printf("🔧 Configured CORRECTION_MODEL: %s", correctionModel)
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
		log.Printf("🔧 Configured BIG_MODEL_ENDPOINT: %v (%d endpoints)", cfg.BigModelEndpoints, len(cfg.BigModelEndpoints))
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
		log.Printf("🔧 Configured SMALL_MODEL_ENDPOINT: %v (%d endpoints)", cfg.SmallModelEndpoints, len(cfg.SmallModelEndpoints))
	} else {
		return nil, fmt.Errorf("SMALL_MODEL_ENDPOINT must be set in .env file")
	}

	if bigAPIKey, exists := envVars["BIG_MODEL_API_KEY"]; exists && bigAPIKey != "" {
		cfg.BigModelAPIKey = bigAPIKey
		log.Printf("🔧 Configured BIG_MODEL_API_KEY: %s", maskAPIKey(bigAPIKey))
	} else {
		return nil, fmt.Errorf("BIG_MODEL_API_KEY must be set in .env file")
	}

	if smallAPIKey, exists := envVars["SMALL_MODEL_API_KEY"]; exists && smallAPIKey != "" {
		cfg.SmallModelAPIKey = smallAPIKey
		log.Printf("🔧 Configured SMALL_MODEL_API_KEY: %s", maskAPIKey(smallAPIKey))
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
		log.Printf("🔧 Configured TOOL_CORRECTION_ENDPOINT: %v (%d endpoints)", cfg.ToolCorrectionEndpoints, len(cfg.ToolCorrectionEndpoints))
	} else {
		return nil, fmt.Errorf("TOOL_CORRECTION_ENDPOINT must be set in .env file")
	}

	if toolCorrectionAPIKey, exists := envVars["TOOL_CORRECTION_API_KEY"]; exists && toolCorrectionAPIKey != "" {
		cfg.ToolCorrectionAPIKey = toolCorrectionAPIKey
		log.Printf("🔧 Configured TOOL_CORRECTION_API_KEY: %s", maskAPIKey(toolCorrectionAPIKey))
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
		log.Printf("🚫 Configured SKIP_TOOLS: %v", cfg.SkipTools)
	}

	// Parse PRINT_SYSTEM_MESSAGE (optional, defaults to false)
	if printSystemMessage, exists := envVars["PRINT_SYSTEM_MESSAGE"]; exists {
		if printSystemMessage == "true" || printSystemMessage == "1" {
			cfg.PrintSystemMessage = true
			log.Printf("🖨️  Configured PRINT_SYSTEM_MESSAGE: enabled")
		} else {
			cfg.PrintSystemMessage = false
			log.Printf("🖨️  Configured PRINT_SYSTEM_MESSAGE: disabled")
		}
	}

	// Parse PRINT_TOOL_SCHEMAS (optional, defaults to false)
	if printToolSchemas, exists := envVars["PRINT_TOOL_SCHEMAS"]; exists {
		if printToolSchemas == "true" || printToolSchemas == "1" {
			cfg.PrintToolSchemas = true
			log.Printf("🔧 Configured PRINT_TOOL_SCHEMAS: enabled")
		} else {
			cfg.PrintToolSchemas = false
			log.Printf("🔧 Configured PRINT_TOOL_SCHEMAS: disabled")
		}
	}

	// Parse DISABLE_SMALL_MODEL_LOGGING (optional, defaults to false)
	if disableSmallLogging, exists := envVars["DISABLE_SMALL_MODEL_LOGGING"]; exists {
		if disableSmallLogging == "true" || disableSmallLogging == "1" {
			cfg.DisableSmallModelLogging = true
			log.Printf("🔇 Configured DISABLE_SMALL_MODEL_LOGGING: enabled (Haiku logging disabled)")
		} else {
			cfg.DisableSmallModelLogging = false
			log.Printf("🔊 Configured DISABLE_SMALL_MODEL_LOGGING: disabled (normal logging)")
		}
	}

	// Parse DISABLE_TOOL_CORRECTION_LOGGING (optional, defaults to false)
	if disableToolCorrectionLogging, exists := envVars["DISABLE_TOOL_CORRECTION_LOGGING"]; exists {
		if disableToolCorrectionLogging == "true" || disableToolCorrectionLogging == "1" {
			cfg.DisableToolCorrectionLogging = true
			log.Printf("🔇 Configured DISABLE_TOOL_CORRECTION_LOGGING: enabled (tool correction logging disabled)")
		} else {
			cfg.DisableToolCorrectionLogging = false
			log.Printf("🔊 Configured DISABLE_TOOL_CORRECTION_LOGGING: disabled (normal logging)")
		}
	}

	// Parse HANDLE_EMPTY_TOOL_RESULTS (optional, defaults to true)
	if handleEmptyResults, exists := envVars["HANDLE_EMPTY_TOOL_RESULTS"]; exists {
		if handleEmptyResults == "false" || handleEmptyResults == "0" {
			cfg.HandleEmptyToolResults = false
			log.Printf("🔧 Configured HANDLE_EMPTY_TOOL_RESULTS: disabled")
		} else {
			cfg.HandleEmptyToolResults = true
			log.Printf("🔧 Configured HANDLE_EMPTY_TOOL_RESULTS: enabled")
		}
	}

	// Parse HANDLE_EMPTY_USER_MESSAGES (optional, defaults to false)
	if handleEmptyUser, exists := envVars["HANDLE_EMPTY_USER_MESSAGES"]; exists {
		if handleEmptyUser == "true" || handleEmptyUser == "1" {
			cfg.HandleEmptyUserMessages = true
			log.Printf("🔧 Configured HANDLE_EMPTY_USER_MESSAGES: enabled")
		} else {
			cfg.HandleEmptyUserMessages = false
			log.Printf("🔧 Configured HANDLE_EMPTY_USER_MESSAGES: disabled")
		}
	}

	// Parse CONVERSATION_LOGGING_ENABLED (optional, defaults to false)
	if conversationLogging, exists := envVars["CONVERSATION_LOGGING_ENABLED"]; exists {
		if conversationLogging == "true" || conversationLogging == "1" {
			cfg.ConversationLoggingEnabled = true
			log.Printf("💬 Configured CONVERSATION_LOGGING_ENABLED: enabled")
		} else {
			cfg.ConversationLoggingEnabled = false
			log.Printf("💬 Configured CONVERSATION_LOGGING_ENABLED: disabled")
		}
	}

	// Parse CONVERSATION_LOG_LEVEL (optional, defaults to INFO)
	if logLevel, exists := envVars["CONVERSATION_LOG_LEVEL"]; exists {
		validLevels := map[string]bool{"DEBUG": true, "INFO": true, "WARN": true, "ERROR": true}
		if validLevels[strings.ToUpper(logLevel)] {
			cfg.ConversationLogLevel = strings.ToUpper(logLevel)
			log.Printf("📊 Configured CONVERSATION_LOG_LEVEL: %s", cfg.ConversationLogLevel)
		} else {
			log.Printf("⚠️  Warning: Invalid CONVERSATION_LOG_LEVEL '%s', using default 'INFO'", logLevel)
			cfg.ConversationLogLevel = "INFO"
		}
	}

	// Parse CONVERSATION_MASK_SENSITIVE (optional, defaults to true)
	if maskSensitive, exists := envVars["CONVERSATION_MASK_SENSITIVE"]; exists {
		if maskSensitive == "false" || maskSensitive == "0" {
			cfg.ConversationMaskSensitive = false
			log.Printf("🔒 Configured CONVERSATION_MASK_SENSITIVE: disabled")
		} else {
			cfg.ConversationMaskSensitive = true
			log.Printf("🔒 Configured CONVERSATION_MASK_SENSITIVE: enabled")
		}
	}

	// Load tool description overrides from YAML file
	toolDescriptions, err := LoadToolDescriptions()
	if err != nil {
		log.Printf("⚠️  Warning: Failed to load tool descriptions from tools_override.yaml: %v", err)
		// Continue with empty tool descriptions instead of failing
	} else {
		cfg.ToolDescriptions = toolDescriptions
	}

	// Load system message overrides from YAML file
	systemOverrides, err := LoadSystemMessageOverrides()
	if err != nil {
		log.Printf("⚠️  Warning: Failed to load system message overrides from system_overrides.yaml: %v", err)
		// Continue with empty system overrides instead of failing
	} else {
		cfg.SystemMessageOverrides = systemOverrides
	}

	// Initialize circuit breaker health tracking
	cfg.InitializeEndpointHealthMap()

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
			log.Printf("🔄[%s] Model mapping: %s → %s", requestID, claudeModel, mapped)
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
			log.Printf("📝 tools_override.yaml not found, using original tool descriptions")
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

	log.Printf("📝 Loaded %d tool description overrides from tools_override.yaml", len(yamlData.ToolDescriptions))
	for toolName := range yamlData.ToolDescriptions {
		log.Printf("   - %s: custom description loaded", toolName)
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
			log.Printf("📝 system_overrides.yaml not found, using original system messages")
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
	log.Printf("📝 Loaded system message overrides from system_overrides.yaml:")
	log.Printf("   - Remove patterns: %d", len(overrides.RemovePatterns))
	log.Printf("   - Replacements: %d", len(overrides.Replacements))
	log.Printf("   - Prepend: %t", overrides.Prepend != "")
	log.Printf("   - Append: %t", overrides.Append != "")

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
			log.Printf("⚠️  Warning: Invalid regex pattern '%s': %v", pattern, err)
			continue
		}

		// Find matches before removing them
		matches := re.FindAllString(message, -1)
		if len(matches) > 0 {
			for _, match := range matches {
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
			log.Printf("🔄 replacement applied: '%s' → '%s' (%d occurrences)",
				replacement.Find, replacement.Replace, occurrences)
		}
	}

	// Apply prepend and append
	if overrides.Prepend != "" {
		message = overrides.Prepend + message
		log.Printf("➕ prepend applied: '%s'", strings.TrimSpace(overrides.Prepend))
	}
	if overrides.Append != "" {
		message = message + overrides.Append
		log.Printf("➕ append applied: '%s'", strings.TrimSpace(overrides.Append))
	}

	// Print updated system prompt
	log.Printf("Modified system prompt:\n%s", message)

	return message
}

// GetBigModelEndpoint returns the next BIG_MODEL endpoint with round-robin failover
func (c *Config) GetBigModelEndpoint() string {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(c.BigModelEndpoints) == 0 {
		return ""
	}

	endpoint := c.BigModelEndpoints[c.bigModelIndex]
	c.bigModelIndex = (c.bigModelIndex + 1) % len(c.BigModelEndpoints)
	return endpoint
}

// GetSmallModelEndpoint returns the next SMALL_MODEL endpoint with round-robin failover
func (c *Config) GetSmallModelEndpoint() string {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(c.SmallModelEndpoints) == 0 {
		return ""
	}

	endpoint := c.SmallModelEndpoints[c.smallModelIndex]
	c.smallModelIndex = (c.smallModelIndex + 1) % len(c.SmallModelEndpoints)
	return endpoint
}

// GetToolCorrectionEndpoint returns the next TOOL_CORRECTION endpoint with round-robin failover
func (c *Config) GetToolCorrectionEndpoint() string {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(c.ToolCorrectionEndpoints) == 0 {
		return ""
	}

	endpoint := c.ToolCorrectionEndpoints[c.toolCorrectionIndex]
	c.toolCorrectionIndex = (c.toolCorrectionIndex + 1) % len(c.ToolCorrectionEndpoints)
	return endpoint
}

// InitializeEndpointHealthMap initializes health tracking for all endpoints
func (c *Config) InitializeEndpointHealthMap() {
	c.healthMutex.Lock()
	defer c.healthMutex.Unlock()

	if c.EndpointHealthMap == nil {
		c.EndpointHealthMap = make(map[string]*EndpointHealth)
	}

	// Initialize health for all endpoints
	allEndpoints := append(c.BigModelEndpoints, c.SmallModelEndpoints...)
	allEndpoints = append(allEndpoints, c.ToolCorrectionEndpoints...)

	for _, endpoint := range allEndpoints {
		if _, exists := c.EndpointHealthMap[endpoint]; !exists {
			c.EndpointHealthMap[endpoint] = &EndpointHealth{
				URL:          endpoint,
				FailureCount: 0,
				CircuitOpen:  false,
			}
		}
	}
}

// IsEndpointHealthy checks if an endpoint is available (circuit closed)
func (c *Config) IsEndpointHealthy(endpoint string) bool {
	c.healthMutex.RLock()
	defer c.healthMutex.RUnlock()

	health, exists := c.EndpointHealthMap[endpoint]
	if !exists {
		return true // Unknown endpoints are assumed healthy
	}

	// If circuit is open, check if it's time to retry
	if health.CircuitOpen {
		if time.Now().After(health.NextRetryTime) {
			return true // Time to test the endpoint again
		}
		return false // Still in backoff period
	}

	return true // Circuit is closed, endpoint is healthy
}

// GetEndpointHealthDebug returns debug information about an endpoint's health
func (c *Config) GetEndpointHealthDebug(endpoint string) (failureCount int, circuitOpen bool, nextRetryTime time.Time, exists bool) {
	c.healthMutex.RLock()
	defer c.healthMutex.RUnlock()

	health, exists := c.EndpointHealthMap[endpoint]
	if !exists {
		return 0, false, time.Time{}, false
	}

	return health.FailureCount, health.CircuitOpen, health.NextRetryTime, true
}

// RecordEndpointFailure marks an endpoint as failed and potentially opens its circuit
func (c *Config) RecordEndpointFailure(endpoint string) {
	c.healthMutex.Lock()
	defer c.healthMutex.Unlock()

	health, exists := c.EndpointHealthMap[endpoint]
	if !exists {
		health = &EndpointHealth{URL: endpoint}
		c.EndpointHealthMap[endpoint] = health
	}

	health.FailureCount++
	health.LastFailureTime = time.Now()

	// Open circuit if failure threshold exceeded
	if health.FailureCount >= c.CircuitBreaker.FailureThreshold {
		health.CircuitOpen = true

		// Calculate backoff time with exponential backoff capped at max
		// When we hit threshold, we want at least 1x backoff
		failuresOverThreshold := health.FailureCount - c.CircuitBreaker.FailureThreshold + 1
		if failuresOverThreshold < 1 {
			failuresOverThreshold = 1
		}
		backoff := time.Duration(int64(c.CircuitBreaker.BackoffDuration) * int64(failuresOverThreshold))
		if backoff > c.CircuitBreaker.MaxBackoffDuration {
			backoff = c.CircuitBreaker.MaxBackoffDuration
		}

		now := time.Now()
		health.NextRetryTime = now.Add(backoff)

		log.Printf("🚨 Circuit breaker opened for endpoint %s (failures: %d, retry in: %v)",
			endpoint, health.FailureCount, backoff)
	} else {
		log.Printf("⚠️ Endpoint failure recorded: %s (failures: %d/%d)",
			endpoint, health.FailureCount, c.CircuitBreaker.FailureThreshold)
	}
}

// RecordEndpointSuccess marks an endpoint as successful and potentially closes its circuit
func (c *Config) RecordEndpointSuccess(endpoint string) {
	c.healthMutex.Lock()
	defer c.healthMutex.Unlock()

	health, exists := c.EndpointHealthMap[endpoint]
	if !exists {
		health = &EndpointHealth{URL: endpoint}
		c.EndpointHealthMap[endpoint] = health
	}

	// If circuit was open, close it and reset
	if health.CircuitOpen {
		health.CircuitOpen = false
		health.FailureCount = 0
		health.NextRetryTime = time.Time{}
		log.Printf("✅ Circuit breaker closed for endpoint %s (recovered)", endpoint)
	} else if health.FailureCount > 0 {
		// Gradually reduce failure count on success
		health.FailureCount = 0
		log.Printf("✅ Endpoint recovered: %s (failure count reset)", endpoint)
	}
}

// GetHealthySmallModelEndpoint returns the next healthy SMALL_MODEL endpoint
func (c *Config) GetHealthySmallModelEndpoint() string {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(c.SmallModelEndpoints) == 0 {
		return ""
	}

	// Try to find a healthy endpoint, starting from current index
	startIndex := c.smallModelIndex
	for i := 0; i < len(c.SmallModelEndpoints); i++ {
		endpoint := c.SmallModelEndpoints[c.smallModelIndex]
		c.smallModelIndex = (c.smallModelIndex + 1) % len(c.SmallModelEndpoints)

		if c.IsEndpointHealthy(endpoint) {
			return endpoint
		}

		// If we've cycled through all endpoints, break to avoid infinite loop
		if c.smallModelIndex == startIndex {
			break
		}
	}

	// If no healthy endpoints found, return the next one anyway (last resort)
	// This handles the case where all endpoints are unhealthy
	endpoint := c.SmallModelEndpoints[c.smallModelIndex]
	c.smallModelIndex = (c.smallModelIndex + 1) % len(c.SmallModelEndpoints)
	return endpoint
}

// GetHealthyToolCorrectionEndpoint returns the next healthy TOOL_CORRECTION endpoint
func (c *Config) GetHealthyToolCorrectionEndpoint() string {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(c.ToolCorrectionEndpoints) == 0 {
		return ""
	}

	// Try to find a healthy endpoint, starting from current index
	startIndex := c.toolCorrectionIndex
	for i := 0; i < len(c.ToolCorrectionEndpoints); i++ {
		endpoint := c.ToolCorrectionEndpoints[c.toolCorrectionIndex]
		c.toolCorrectionIndex = (c.toolCorrectionIndex + 1) % len(c.ToolCorrectionEndpoints)

		if c.IsEndpointHealthy(endpoint) {
			return endpoint
		}

		// If we've cycled through all endpoints, break to avoid infinite loop
		if c.toolCorrectionIndex == startIndex {
			break
		}
	}

	// If no healthy endpoints found, return the next one anyway (last resort)
	endpoint := c.ToolCorrectionEndpoints[c.toolCorrectionIndex]
	c.toolCorrectionIndex = (c.toolCorrectionIndex + 1) % len(c.ToolCorrectionEndpoints)
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
			log.Printf("⚠️ Big model endpoint failed, switching to index %d", c.bigModelIndex)
		}
	case "small_model":
		if len(c.SmallModelEndpoints) > 1 {
			c.smallModelIndex = (c.smallModelIndex + 1) % len(c.SmallModelEndpoints)
			log.Printf("⚠️ Small model endpoint failed, switching to index %d", c.smallModelIndex)
		}
	case "tool_correction":
		if len(c.ToolCorrectionEndpoints) > 1 {
			c.toolCorrectionIndex = (c.toolCorrectionIndex + 1) % len(c.ToolCorrectionEndpoints)
			log.Printf("⚠️ Tool correction endpoint failed, switching to index %d", c.toolCorrectionIndex)
		}
	}
}
