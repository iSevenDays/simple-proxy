package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"claude-proxy/internal"
)

// Component constants for consistent labeling
const (
	ComponentProxy         = "proxy_core"
	ComponentCircuitBreaker = "circuit_breaker"
	ComponentHybridClassifier = "hybrid_classifier" 
	ComponentToolCorrection = "tool_correction"
	ComponentExitPlanMode  = "exitplanmode_validation"
	ComponentSchemaCorrection = "schema_correction"
	ComponentEndpointManagement = "endpoint_management"
	ComponentConfig        = "configuration"
)

// Category constants for log classification
const (
	CategoryRequest       = "request"
	CategoryTransformation = "transformation"
	CategorySuccess       = "success"
	CategoryWarning       = "warning"
	CategoryError         = "error"
	CategoryHealth        = "health"
	CategoryFailover      = "failover"
	CategoryClassification = "classification"
	CategoryValidation     = "validation"
	CategoryDebug         = "debug"
	CategoryBlocked       = "blocked"
)

// LokiLogger implements Logger interface with direct HTTP push to Loki
type LokiLogger struct {
	ctx       context.Context
	config    LoggerConfig
	lokiURL   string
	client    *http.Client
	fields    map[string]string
	model     string
	component string
}

// LokiLogEntry represents a Loki log entry
type LokiLogEntry struct {
	Streams []LokiStream `json:"streams"`
}

type LokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// NewLokiLogger creates a new Loki HTTP logger
func NewLokiLogger(ctx context.Context, config LoggerConfig, lokiURL string) (Logger, error) {
	if lokiURL == "" {
		lokiURL = "http://localhost:3100"
	}
	
	return &LokiLogger{
		ctx:     ctx,
		config:  config,
		lokiURL: lokiURL + "/loki/api/v1/push",
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		fields: make(map[string]string),
	}, nil
}

// Close shuts down the logger
func (l *LokiLogger) Close() error {
	l.client.CloseIdleConnections()
	return nil
}

// WithField adds a field to the logger context
func (l *LokiLogger) WithField(key, value string) Logger {
	newFields := make(map[string]string)
	for k, v := range l.fields {
		newFields[k] = v
	}
	newFields[key] = value
	
	return &LokiLogger{
		ctx:       l.ctx,
		config:    l.config,
		lokiURL:   l.lokiURL,
		client:    l.client,
		fields:    newFields,
		model:     l.model,
		component: l.component,
	}
}

// WithModel sets the model for filtering decisions
func (l *LokiLogger) WithModel(model string) Logger {
	newFields := make(map[string]string)
	for k, v := range l.fields {
		newFields[k] = v
	}
	
	return &LokiLogger{
		ctx:       l.ctx,
		config:    l.config,
		lokiURL:   l.lokiURL,
		client:    l.client,
		fields:    newFields,
		model:     model,
		component: l.component,
	}
}

// WithComponent sets the component for the logger
func (l *LokiLogger) WithComponent(component string) Logger {
	newFields := make(map[string]string)
	for k, v := range l.fields {
		newFields[k] = v
	}
	
	return &LokiLogger{
		ctx:       l.ctx,
		config:    l.config,
		lokiURL:   l.lokiURL,
		client:    l.client,
		fields:    newFields,
		model:     l.model,
		component: component,
	}
}

// shouldLog determines if a message should be logged
func (l *LokiLogger) shouldLog(level Level) bool {
	// Always log if no config provided (simplified approach)
	if l.config == nil {
		return true
	}
	
	if level < l.config.GetMinLogLevel() {
		return false
	}
	
	if l.model != "" && !l.config.ShouldLogForModel(l.model) {
		return false
	}
	
	return true
}

// pushToLoki sends log to Loki via HTTP following best practices
func (l *LokiLogger) pushToLoki(level Level, message string) {
	// Build labels (stream identifiers) - KEEP LOW CARDINALITY
	labels := map[string]string{
		"service": "simple-proxy",
		"level":   level.String(),
		"job":     "simple-proxy",
	}
	
	// Add component as label only if it exists (low cardinality)
	if l.component != "" {
		labels["component"] = l.component
	}
	
	// Build structured data for JSON extraction (high cardinality OK)
	structuredData := make(map[string]interface{})
	
	// Add all fields to structured data
	for k, v := range l.fields {
		structuredData[k] = v
	}
	
	// Add standard fields
	if requestID := internal.GetRequestID(l.ctx); requestID != "" {
		structuredData["request_id"] = requestID
	}
	
	if l.model != "" {
		structuredData["model"] = l.model
	}
	
	// Add timestamp for structured data
	structuredData["timestamp"] = time.Now().Format(time.RFC3339Nano)
	
	// Create human-readable log line with structured data
	logLine := l.formatReadableLogLine(level, message, structuredData)
	
	// Create Loki payload
	entry := LokiLogEntry{
		Streams: []LokiStream{
			{
				Stream: labels,
				Values: [][]string{
					{fmt.Sprintf("%d", time.Now().UnixNano()), logLine},
				},
			},
		},
	}
	
	// Send to Loki (async)
	go l.sendAsync(entry)
}

// formatReadableLogLine creates a readable log line with embedded structured data
func (l *LokiLogger) formatReadableLogLine(level Level, message string, structuredData map[string]interface{}) string {
	// Start with human-readable format
	timestamp := time.Now().Format("15:04:05.000")
	
	var parts []string
	parts = append(parts, fmt.Sprintf("[%s]", timestamp))
	parts = append(parts, fmt.Sprintf("[%s]", level.String()))
	
	// Add component if available
	if l.component != "" {
		parts = append(parts, fmt.Sprintf("[%s]", l.component))
	}
	
	// Add request ID if available
	if requestID, ok := structuredData["request_id"].(string); ok && requestID != "" {
		parts = append(parts, fmt.Sprintf("[req:%s]", requestID))
	}
	
	// Add main message
	parts = append(parts, message)
	
	// Add key structured fields for visibility
	var keyFields []string
	for key, value := range structuredData {
		// Skip already displayed fields
		if key == "request_id" || key == "timestamp" || key == "component" {
			continue
		}
		
		// Add important fields inline
		if key == "endpoint" || key == "tool_name" || key == "decision" || key == "error" {
			keyFields = append(keyFields, fmt.Sprintf("%s=%v", key, value))
		}
	}
	
	if len(keyFields) > 0 {
		parts = append(parts, fmt.Sprintf("| %s", strings.Join(keyFields, " ")))
	}
	
	humanLine := strings.Join(parts, " ")
	
	// Add structured data as JSON on new line for LogQL parsing
	if len(structuredData) > 0 {
		jsonData, _ := json.Marshal(structuredData)
		return fmt.Sprintf("%s\n%s", humanLine, string(jsonData))
	}
	
	return humanLine
}

// sendAsync sends to Loki without blocking
func (l *LokiLogger) sendAsync(entry LokiLogEntry) {
	jsonData, err := json.Marshal(entry)
	if err != nil {
		fmt.Printf("Failed to marshal log: %v\n", err)
		return
	}
	
	req, err := http.NewRequest("POST", l.lokiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Failed to create request: %v\n", err)
		return
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := l.client.Do(req)
	if err != nil {
		// Fallback to stdout if Loki unavailable - show actual error for debugging
		fmt.Printf("Loki unavailable (%v), logging to stdout: %s\n", err, string(jsonData))
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 204 {
		fmt.Printf("Loki returned %d, logging to stdout: %s\n", resp.StatusCode, string(jsonData))
	}
}

// Debug logs a debug level message  
func (l *LokiLogger) Debug(format string, args ...interface{}) {
	if l.shouldLog(DEBUG) {
		message := fmt.Sprintf(format, args...)
		l.pushToLoki(DEBUG, message)
	}
}

// Info logs an info level message (Logger interface)
func (l *LokiLogger) Info(format string, args ...interface{}) {
	if l.shouldLog(INFO) {
		message := fmt.Sprintf(format, args...)
		l.pushToLoki(INFO, message)
	}
}

// Warn logs a warning level message (Logger interface)
func (l *LokiLogger) Warn(format string, args ...interface{}) {
	if l.shouldLog(WARN) {
		message := fmt.Sprintf(format, args...)
		l.pushToLoki(WARN, message)
	}
}

// Error logs an error level message (Logger interface)
func (l *LokiLogger) Error(format string, args ...interface{}) {
	if l.shouldLog(ERROR) {
		message := fmt.Sprintf(format, args...)
		l.pushToLoki(ERROR, message)
	}
}

// Observability methods matching existing interface

// Request logs request-related events
func (l *LokiLogger) Request(requestID, message string, fields map[string]interface{}) {
	logger := l.WithField("request_id", requestID).
		WithField("component", ComponentProxy).
		WithField("category", CategoryRequest)
	
	if fields != nil {
		for k, v := range fields {
			logger = logger.WithField(k, fmt.Sprintf("%v", v))
		}
	}
	
	logger.Info(message)
}

// CircuitBreakerEvent logs circuit breaker state changes
func (l *LokiLogger) CircuitBreakerEvent(requestID, endpoint, message string, fields map[string]interface{}) {
	logger := l.WithField("request_id", requestID).
		WithField("endpoint", endpoint).
		WithField("component", ComponentCircuitBreaker).
		WithField("category", CategoryHealth)
	
	if fields != nil {
		for k, v := range fields {
			logger = logger.WithField(k, fmt.Sprintf("%v", v))
		}
	}
	
	logger.Info(message)
}

// ToolCorrection logs tool correction attempts
func (l *LokiLogger) ToolCorrection(requestID, toolName, message string, fields map[string]interface{}) {
	logger := l.WithField("request_id", requestID).
		WithField("tool_name", toolName).
		WithField("component", ComponentToolCorrection).
		WithField("category", CategoryTransformation)
	
	if fields != nil {
		for k, v := range fields {
			logger = logger.WithField(k, fmt.Sprintf("%v", v))
		}
	}
	
	logger.Info(message)
}

// ClassificationDecision logs hybrid classifier decisions
func (l *LokiLogger) ClassificationDecision(requestID, decision, reason string, confident bool, fields map[string]interface{}) {
	logger := l.WithField("request_id", requestID).
		WithField("decision", decision).
		WithField("reason", reason).
		WithField("confident", fmt.Sprintf("%t", confident)).
		WithField("component", ComponentHybridClassifier).
		WithField("category", CategoryClassification)
	
	if fields != nil {
		for k, v := range fields {
			logger = logger.WithField(k, fmt.Sprintf("%v", v))
		}
	}
	
	logger.Info("Tool necessity decision")
}

// ObservabilityLogger is an alias for backward compatibility
type ObservabilityLogger = LokiObservabilityLogger

// Config interface compatibility - separate struct for this use case
type LokiObservabilityLogger struct {
	*LokiLogger
}

// Info logs info with config interface signature
func (l *LokiObservabilityLogger) Info(component, category, requestID, message string, fields map[string]interface{}) {
	logger := l.LokiLogger.WithField("component", component).
		WithField("category", category)
	
	if requestID != "" {
		logger = logger.WithField("request_id", requestID)
	}
	
	if fields != nil {
		for k, v := range fields {
			logger = logger.WithField(k, fmt.Sprintf("%v", v))
		}
	}
	
	logger.Info(message)
}

// Warn logs warning with config interface
func (l *LokiObservabilityLogger) Warn(component, category, requestID, message string, fields map[string]interface{}) {
	logger := l.LokiLogger.WithField("component", component).
		WithField("category", category)
	
	if requestID != "" {
		logger = logger.WithField("request_id", requestID)
	}
	
	if fields != nil {
		for k, v := range fields {
			logger = logger.WithField(k, fmt.Sprintf("%v", v))
		}
	}
	
	logger.Warn(message)
}

// Error logs error with config interface
func (l *LokiObservabilityLogger) Error(component, category, requestID, message string, fields map[string]interface{}) {
	logger := l.LokiLogger.WithField("component", component).
		WithField("category", category)
	
	if requestID != "" {
		logger = logger.WithField("request_id", requestID)
	}
	
	if fields != nil {
		for k, v := range fields {
			logger = logger.WithField(k, fmt.Sprintf("%v", v))
		}
	}
	
	logger.Error(message)
}

// Conversation logging methods - extending LokiLogger for structured conversation data

// LogConversationStart logs the beginning of a new conversation
func (l *LokiLogger) LogConversationStart(ctx context.Context, requestID, sessionID string) {
	l.WithField("event", "conversation_start").
		WithField("session_id", sessionID).
		WithField("request_id", requestID).
		WithField("category", "conversation").
		Info("🚀 Conversation started")
}

// LogRequest logs a complete incoming request
func (l *LokiLogger) LogRequest(ctx context.Context, requestID, sessionID string, request interface{}) {
	requestJSON, _ := json.Marshal(request)
	
	l.WithField("event", "request").
		WithField("session_id", sessionID).
		WithField("request_id", requestID).
		WithField("category", "conversation").
		WithField("data_type", "request").
		WithField("request_data", string(requestJSON)).
		Info("📨 Incoming request logged")
}

// LogResponse logs a complete outgoing response  
func (l *LokiLogger) LogResponse(ctx context.Context, requestID, sessionID string, response interface{}) {
	responseJSON, _ := json.Marshal(response)
	
	l.WithField("event", "response").
		WithField("session_id", sessionID).
		WithField("request_id", requestID).
		WithField("category", "conversation").
		WithField("data_type", "response").
		WithField("response_data", string(responseJSON)).
		Info("📤 Outgoing response logged")
}

// LogToolCall logs a tool call and its result
func (l *LokiLogger) LogToolCall(ctx context.Context, requestID, sessionID, toolName string, params, result interface{}) {
	paramsJSON, _ := json.Marshal(params)
	resultJSON, _ := json.Marshal(result)
	
	l.WithField("event", "tool_call").
		WithField("session_id", sessionID).
		WithField("request_id", requestID).
		WithField("category", "conversation").
		WithField("data_type", "tool_call").
		WithField("tool_name", toolName).
		WithField("tool_params", string(paramsJSON)).
		WithField("tool_result", string(resultJSON)).
		Info("🔧 Tool call executed")
}

// LogCorrection logs a tool correction attempt
func (l *LokiLogger) LogCorrection(ctx context.Context, requestID, sessionID string, original, corrected interface{}, method string) {
	originalJSON, _ := json.Marshal(original)
	correctedJSON, _ := json.Marshal(corrected)
	
	l.WithField("event", "correction").
		WithField("session_id", sessionID).
		WithField("request_id", requestID).
		WithField("category", "conversation").
		WithField("data_type", "correction").
		WithField("correction_method", method).
		WithField("original_data", string(originalJSON)).
		WithField("corrected_data", string(correctedJSON)).
		Info("🔧 Tool correction applied")
}

// LogConversationEnd logs the end of a conversation
func (l *LokiLogger) LogConversationEnd(ctx context.Context, requestID, sessionID string, stats map[string]interface{}) {
	statsJSON, _ := json.Marshal(stats)
	
	l.WithField("event", "conversation_end").
		WithField("session_id", sessionID).
		WithField("request_id", requestID).
		WithField("category", "conversation").
		WithField("data_type", "conversation_end").
		WithField("conversation_stats", string(statsJSON)).
		Info("🏁 Conversation ended")
}