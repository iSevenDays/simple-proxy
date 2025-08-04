package logger

import (
	"claude-proxy/config"
	"context"
)

// ConfigAdapter adapts the existing config.Config to implement LoggerConfig
type ConfigAdapter struct {
	config *config.Config
}

// NewConfigAdapter creates a new ConfigAdapter
func NewConfigAdapter(cfg *config.Config) LoggerConfig {
	return &ConfigAdapter{config: cfg}
}

// ShouldLogForModel determines if logging should be enabled for the given model
func (c *ConfigAdapter) ShouldLogForModel(model string) bool {
	// If small model logging is disabled and this is a small model, don't log
	if c.config.DisableSmallModelLogging && c.isSmallModelSimple(model) {
		return false
	}
	return true
}

// isSmallModelSimple checks if the given model name maps to the small model configuration
// without requiring context (to avoid [unknown] request IDs in logs)
func (c *ConfigAdapter) isSmallModelSimple(model string) bool {
	// Check direct matches first (most common cases)
	if model == "claude-3-5-haiku-20241022" || model == c.config.SmallModel {
		return true
	}
	
	// Check the known mapping without calling MapModelName to avoid [unknown] logs
	// This is a simplified version that covers the main use case
	if model == "claude-3-5-haiku-20241022" {
		return true // Haiku always maps to small model
	}
	
	return false
}

// GetMinLogLevel returns the minimum log level (currently always DEBUG for backwards compatibility)
func (c *ConfigAdapter) GetMinLogLevel() Level {
	// For now, maintain backwards compatibility by allowing all levels
	// In the future, this could be configurable via environment variables
	return DEBUG
}

// ShouldMaskAPIKeys returns whether API keys should be masked in logs
func (c *ConfigAdapter) ShouldMaskAPIKeys() bool {
	// Always mask API keys for security
	return true
}

// Note: Request ID functions moved to use existing internal package
// This avoids duplicate context key definitions

// NewFromConfig creates a new logger using the existing config
func NewFromConfig(ctx context.Context, cfg *config.Config) Logger {
	loggerConfig := NewConfigAdapter(cfg)
	return New(ctx, loggerConfig)
}

// ContextLoggerFromConfig creates a logger and stores it in context for easy access
func ContextLoggerFromConfig(ctx context.Context, cfg *config.Config) (context.Context, Logger) {
	logger := NewFromConfig(ctx, cfg)
	newCtx := context.WithValue(ctx, loggerContextKey, logger)
	return newCtx, logger
}