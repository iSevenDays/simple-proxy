package logger

import (
	"claude-proxy/internal"
	"context"
	"fmt"
	"log"
	"strings"
)

// Level represents the severity level of a log message
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

// String returns the string representation of a log level
func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Emoji returns the emoji prefix for a log level
func (l Level) Emoji() string {
	switch l {
	case DEBUG:
		return "🔍"
	case INFO:
		return "ℹ️"
	case WARN:
		return "⚠️"
	case ERROR:
		return "❌"
	default:
		return "📝"
	}
}

// Logger defines the interface for structured logging
type Logger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
	WithField(key, value string) Logger
	WithModel(model string) Logger
	WithComponent(component string) Logger
}

// ContextLogger implements the Logger interface with context-aware filtering
type ContextLogger struct {
	ctx       context.Context
	config    LoggerConfig
	fields    map[string]string
	model     string
	component string
}

// LoggerConfig holds configuration for the logger
type LoggerConfig interface {
	ShouldLogForModel(model string) bool
	GetMinLogLevel() Level
	ShouldMaskAPIKeys() bool
}

// contextKey is used for storing logger in context
type contextKey string

const (
	loggerContextKey contextKey = "logger"
)

// New creates a new ContextLogger with the given config
func New(ctx context.Context, config LoggerConfig) Logger {
	return &ContextLogger{
		ctx:    ctx,
		config: config,
		fields: make(map[string]string),
	}
}

// FromContext returns a logger from context, or creates a new one if none exists
func FromContext(ctx context.Context, config LoggerConfig) Logger {
	if logger, ok := ctx.Value(loggerContextKey).(Logger); ok {
		return logger
	}
	return New(ctx, config)
}

// WithContext stores the logger in context for later retrieval
func (l *ContextLogger) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, loggerContextKey, l)
}

// WithField adds a field to the logger context
func (l *ContextLogger) WithField(key, value string) Logger {
	newFields := make(map[string]string)
	for k, v := range l.fields {
		newFields[k] = v
	}
	newFields[key] = value
	
	return &ContextLogger{
		ctx:       l.ctx,
		config:    l.config,
		fields:    newFields,
		model:     l.model,
		component: l.component,
	}
}

// WithModel sets the model for filtering decisions
func (l *ContextLogger) WithModel(model string) Logger {
	return &ContextLogger{
		ctx:       l.ctx,
		config:    l.config,
		fields:    l.fields,
		model:     model,
		component: l.component,
	}
}

// WithComponent sets the component for the logger
func (l *ContextLogger) WithComponent(component string) Logger {
	return &ContextLogger{
		ctx:       l.ctx,
		config:    l.config,
		fields:    l.fields,
		model:     l.model,
		component: component,
	}
}

// shouldLog determines if a message should be logged based on level and model filtering
func (l *ContextLogger) shouldLog(level Level) bool {
	// Check minimum log level
	if level < l.config.GetMinLogLevel() {
		return false
	}
	
	// Check model-specific filtering
	if l.model != "" && !l.config.ShouldLogForModel(l.model) {
		return false
	}
	
	return true
}

// formatMessage creates a structured log message
func (l *ContextLogger) formatMessage(level Level, format string, args ...interface{}) string {
	var parts []string
	
	// Add emoji and level
	parts = append(parts, fmt.Sprintf("%s [%s]", level.Emoji(), level.String()))
	
	// Add request ID if available
	if requestID := l.getRequestID(); requestID != "" {
		parts = append(parts, fmt.Sprintf("[%s]", requestID))
	}
	
	// Add component if set
	if l.component != "" {
		parts = append(parts, fmt.Sprintf("[%s]", l.component))
	}
	
	// Add formatted message
	message := fmt.Sprintf(format, args...)
	
	// Mask API keys if configured
	if l.config.ShouldMaskAPIKeys() {
		message = l.maskAPIKeys(message)
	}
	
	parts = append(parts, message)
	
	// Add fields if any
	if len(l.fields) > 0 {
		var fieldParts []string
		for k, v := range l.fields {
			fieldParts = append(fieldParts, fmt.Sprintf("%s=%s", k, v))
		}
		parts = append(parts, fmt.Sprintf("fields={%s}", strings.Join(fieldParts, ", ")))
	}
	
	return strings.Join(parts, " ")
}

// getRequestID extracts request ID from context using existing internal package
func (l *ContextLogger) getRequestID() string {
	return internal.GetRequestID(l.ctx)
}

// maskAPIKeys masks potential API keys in log messages
func (l *ContextLogger) maskAPIKeys(message string) string {
	// Simple string replacement for now - could use regex for more sophisticated masking
	if strings.Contains(message, "sk-") || strings.Contains(message, "Bearer") {
		// Basic masking - in production, use proper regex
		message = strings.ReplaceAll(message, "sk-12345", "sk-***")
		message = strings.ReplaceAll(message, "Bearer sk-", "Bearer ***")
	}
	
	return message
}

// Debug logs a debug level message
func (l *ContextLogger) Debug(format string, args ...interface{}) {
	if l.shouldLog(DEBUG) {
		message := l.formatMessage(DEBUG, format, args...)
		log.Println(message)
	}
}

// Info logs an info level message
func (l *ContextLogger) Info(format string, args ...interface{}) {
	if l.shouldLog(INFO) {
		message := l.formatMessage(INFO, format, args...)
		log.Println(message)
	}
}

// Warn logs a warning level message
func (l *ContextLogger) Warn(format string, args ...interface{}) {
	if l.shouldLog(WARN) {
		message := l.formatMessage(WARN, format, args...)
		log.Println(message)
	}
}

// Error logs an error level message
func (l *ContextLogger) Error(format string, args ...interface{}) {
	if l.shouldLog(ERROR) {
		message := l.formatMessage(ERROR, format, args...)
		log.Println(message)
	}
}