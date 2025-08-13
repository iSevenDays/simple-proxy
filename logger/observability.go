package logger

import (
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// ObservabilityLogger provides structured logging using logrus for Loki ingestion
// Following SPARC: Simple, focused logging with industry-standard library
type ObservabilityLogger struct {
	logger *logrus.Logger
	file   *os.File
}

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


// NewObservabilityLogger creates a new structured logger using logrus for Loki ingestion
func NewObservabilityLogger(logDir string) (*ObservabilityLogger, error) {
	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	// Open log file
	logPath := filepath.Join(logDir, "simple-proxy.jsonl")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	// Create logrus logger with JSON formatter
	logger := logrus.New()
	logger.SetOutput(file)
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "level",
			logrus.FieldKeyMsg:   "message",
		},
	})
	logger.SetLevel(logrus.InfoLevel)

	// Add service field to all logs
	logger = logger.WithField("service", "simple-proxy").Logger

	return &ObservabilityLogger{
		logger: logger,
		file:   file,
	}, nil
}

// Close closes the log file
func (o *ObservabilityLogger) Close() error {
	if o.file != nil {
		return o.file.Close()
	}
	return nil
}

// createEntry creates a logrus entry with standard fields
func (o *ObservabilityLogger) createEntry(component, category, requestID string, fields map[string]interface{}) *logrus.Entry {
	entry := o.logger.WithFields(logrus.Fields{
		"component": component,
		"category":  category,
	})
	
	if requestID != "" {
		entry = entry.WithField("request_id", requestID)
	}
	
	if fields != nil {
		entry = entry.WithFields(fields)
	}
	
	return entry
}

// Debug logs a debug message
func (o *ObservabilityLogger) Debug(component, category, requestID, message string, fields map[string]interface{}) {
	o.createEntry(component, category, requestID, fields).Debug(message)
}

// Info logs an info message
func (o *ObservabilityLogger) Info(component, category, requestID, message string, fields map[string]interface{}) {
	o.createEntry(component, category, requestID, fields).Info(message)
}

// Warn logs a warning message
func (o *ObservabilityLogger) Warn(component, category, requestID, message string, fields map[string]interface{}) {
	o.createEntry(component, category, requestID, fields).Warn(message)
}

// Error logs an error message
func (o *ObservabilityLogger) Error(component, category, requestID, message string, fields map[string]interface{}) {
	o.createEntry(component, category, requestID, fields).Error(message)
}

// Request logs request-related events
func (o *ObservabilityLogger) Request(requestID, message string, fields map[string]interface{}) {
	o.Info(ComponentProxy, CategoryRequest, requestID, message, fields)
}

// CircuitBreakerEvent logs circuit breaker state changes
func (o *ObservabilityLogger) CircuitBreakerEvent(requestID, endpoint, message string, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["endpoint"] = endpoint
	o.Info(ComponentCircuitBreaker, CategoryHealth, requestID, message, fields)
}

// ToolCorrection logs tool correction attempts
func (o *ObservabilityLogger) ToolCorrection(requestID, toolName, message string, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["tool_name"] = toolName
	o.Info(ComponentToolCorrection, CategoryTransformation, requestID, message, fields)
}

// ClassificationDecision logs hybrid classifier decisions
func (o *ObservabilityLogger) ClassificationDecision(requestID, decision, reason string, confident bool, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["decision"] = decision
	fields["reason"] = reason
	fields["confident"] = confident
	o.Info(ComponentHybridClassifier, CategoryClassification, requestID, "Tool necessity decision", fields)
}

