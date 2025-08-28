package proxy

import (
	"claude-proxy/internal"
	"context"
	"fmt"
	"time"
)

// withRequestID adds a request ID to the context (wraps internal function)
func withRequestID(ctx context.Context, requestID string) context.Context {
	return internal.WithRequestID(ctx, requestID)
}

// GetRequestID retrieves the request ID from context (wraps internal function)
func GetRequestID(ctx context.Context) string {
	return internal.GetRequestID(ctx)
}

// generateRequestID creates a unique request ID
func generateRequestID() string {
	return fmt.Sprintf("req_%d", time.Now().UnixNano()%10000)
}
