package main

import (
	"claude-proxy/config"
	"claude-proxy/logger"
	"claude-proxy/proxy"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	// Load configuration with .env support
	cfg, err := config.LoadConfigWithEnv()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize observability logger for structured Loki ingestion
	logDir := os.Getenv("LOG_DIR")
	if logDir == "" {
		logDir = "./observability/logs" // Default to observability directory
	}
	obsLogger, err := logger.NewObservabilityLogger(logDir)
	if err != nil {
		fmt.Printf("Warning: Failed to initialize observability logger: %v (continuing with fallback logging)\n", err)
	} else {
		defer obsLogger.Close()
		// Set observability logger on config for structured logging
		cfg.SetObservabilityLogger(obsLogger)
	}

	if obsLogger != nil {
		obsLogger.Info(logger.ComponentProxy, logger.CategoryRequest, "", "Claude Code Proxy configuration loaded", map[string]interface{}{
			"tool_correction_enabled": cfg.ToolCorrectionEnabled,
			"big_model": cfg.BigModel,
			"big_model_endpoints": len(cfg.BigModelEndpoints),
			"small_model": cfg.SmallModel,
			"small_model_endpoints": len(cfg.SmallModelEndpoints),
			"correction_model": cfg.CorrectionModel,
			"correction_endpoints": len(cfg.ToolCorrectionEndpoints),
			"port": cfg.Port,
		})
	}

	// Log startup to structured logger
	if obsLogger != nil {
		obsLogger.Info(logger.ComponentProxy, logger.CategoryRequest, "", "Claude Code Proxy starting", map[string]interface{}{
			"port": cfg.Port,
			"tool_correction_enabled": cfg.ToolCorrectionEnabled,
			"big_model_endpoints": len(cfg.BigModelEndpoints),
			"small_model_endpoints": len(cfg.SmallModelEndpoints),
			"correction_endpoints": len(cfg.ToolCorrectionEndpoints),
		})
	}

	// Initialize conversation logger if enabled
	var conversationLogger *logger.ConversationLogger
	if cfg.ConversationLoggingEnabled {
		logLevel := logger.ParseLevel(cfg.ConversationLogLevel)
		conversationLogger, err = logger.NewConversationLogger("logs", logLevel, cfg.ConversationMaskSensitive, cfg.ConversationLogFullTools, cfg.ConversationTruncation)
		if err != nil {
			if obsLogger != nil {
				obsLogger.Error(logger.ComponentProxy, logger.CategoryError, "", "Failed to initialize conversation logger", map[string]interface{}{"error": err.Error()})
			}
			log.Fatalf("Failed to initialize conversation logger: %v", err)
		}
		if obsLogger != nil {
			obsLogger.Info(logger.ComponentProxy, logger.CategoryRequest, "", "Conversation logging initialized", map[string]interface{}{
				"level": cfg.ConversationLogLevel,
				"session_id": conversationLogger.GetSessionID(),
			})
		}
		defer conversationLogger.Close()
	}

	// Create proxy handler
	proxyHandler := proxy.NewHandler(cfg, conversationLogger, obsLogger)

	// Setup HTTP routes
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/v1/messages", proxyHandler.HandleAnthropicRequest)

	// Setup HTTP server with reasonable timeouts
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second, // Long timeout for streaming responses
		IdleTimeout:  60 * time.Second,
	}

	if obsLogger != nil {
		obsLogger.Info(logger.ComponentProxy, logger.CategoryRequest, "", "Claude Code Proxy started", map[string]interface{}{
			"address": fmt.Sprintf("http://localhost:%s", cfg.Port),
			"endpoint": fmt.Sprintf("http://localhost:%s/v1/messages", cfg.Port),
		})
	}

	// Start server
	if err := server.ListenAndServe(); err != nil {
		if obsLogger != nil {
			obsLogger.Error(logger.ComponentProxy, logger.CategoryError, "", "Server failed to start", map[string]interface{}{"error": err.Error()})
		}
		log.Fatalf("Server failed to start: %v", err)
	}
}

// handleRoot provides basic information about the proxy
func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{
	"service": "Claude Code Proxy",
	"version": "1.0.0",
	"status": "running",
	"endpoints": [
		"GET /health - Health check",
		"POST /v1/messages - Anthropic-compatible chat completions"
	]
}`)
}

// handleHealth provides a simple health check endpoint
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{
	"status": "ok",
	"timestamp": "%s"
}`, time.Now().UTC().Format(time.RFC3339))
}
