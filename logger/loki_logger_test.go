package logger

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLokiLoggerBasic(t *testing.T) {
	config := &testLoggerConfig{minLevel: DEBUG}
	logger, err := NewLokiLogger(context.Background(), config, "http://localhost:3100")
	
	require.NoError(t, err)
	require.NotNil(t, logger)
	
	// Test basic logging
	logger.Info("test message")
	
	// Cleanup
	if closer, ok := logger.(*LokiLogger); ok {
		closer.Close()
	}
}

func TestLokiLoggerWithFields(t *testing.T) {
	config := &testLoggerConfig{minLevel: DEBUG}
	logger, err := NewLokiLogger(context.Background(), config, "http://localhost:3100")
	require.NoError(t, err)
	defer func() {
		if closer, ok := logger.(*LokiLogger); ok {
			closer.Close()
		}
	}()
	
	// Test with fields
	loggerWithFields := logger.WithField("request_id", "test-123").
		WithComponent("proxy")
	
	loggerWithFields.Info("test with fields")
	
	// Should not panic or error
}

func TestLokiLoggerObservabilityMethods(t *testing.T) {
	config := &testLoggerConfig{minLevel: DEBUG}
	logger, err := NewLokiLogger(context.Background(), config, "http://localhost:3100")
	require.NoError(t, err)
	defer func() {
		if closer, ok := logger.(*LokiLogger); ok {
			closer.Close()
		}
	}()
	
	lokiLogger := logger.(*LokiLogger)
	
	// Test observability methods exist
	lokiLogger.Request("req-123", "test request", nil)
	lokiLogger.CircuitBreakerEvent("req-123", "endpoint", "test cb", nil)
	lokiLogger.ToolCorrection("req-123", "Write", "test correction", nil)
	lokiLogger.ClassificationDecision("req-123", "require", "test reason", true, nil)
}