package main

import (
	"claude-proxy/config"
	"claude-proxy/logger"
	"claude-proxy/proxy"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Simple logger config implementation
type simpleLoggerConfig struct {
	minLevel    logger.Level
	maskAPIKeys bool
}

func (s *simpleLoggerConfig) ShouldLogForModel(model string) bool {
	return true // Log for all models
}

func (s *simpleLoggerConfig) GetMinLogLevel() logger.Level {
	return s.minLevel
}

func (s *simpleLoggerConfig) ShouldMaskAPIKeys() bool {
	return s.maskAPIKeys
}

func main() {
	// Print version information
	fmt.Println(GetBuildInfo())
	fmt.Println()

	// Load configuration with .env support
	cfg, err := config.LoadConfigWithEnv()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize direct Loki HTTP logging
	lokiURL := os.Getenv("LOKI_URL")
	if lokiURL == "" {
		lokiURL = "http://localhost:3100"
	}
	
	// Create a simple config adapter
	loggerCfg := &simpleLoggerConfig{
		minLevel:    logger.INFO,
		maskAPIKeys: true,
	}
	
	lokiLogger, err := logger.NewLokiLogger(context.Background(), loggerCfg, lokiURL)
	if err != nil {
		log.Fatalf("Failed to initialize Loki logger: %v", err)
	}
	
	// Wrap for config interface compatibility
	obsLogger := &logger.LokiObservabilityLogger{LokiLogger: lokiLogger.(*logger.LokiLogger)}
	cfg.SetObservabilityLogger(obsLogger)
	fmt.Printf("✅ Direct Loki logging enabled at %s\n", lokiURL)

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
			"version": GetVersionInfo(),
			"git_commit": GetGitCommit(),
		})
	}

	// Initialize conversation session ID if conversation logging enabled
	var conversationSessionID string
	if cfg.ConversationLoggingEnabled && obsLogger != nil {
		conversationSessionID = fmt.Sprintf("session_%d", time.Now().UnixNano()%100000)
		
		// Log conversation session start
		obsLogger.LokiLogger.WithField("event", "session_start").
			WithField("session_id", conversationSessionID).
			WithField("category", "session").
			Info("📋 Conversation session started")
		
		obsLogger.Info(logger.ComponentProxy, logger.CategoryRequest, "", "Conversation logging initialized", map[string]interface{}{
			"level": cfg.ConversationLogLevel,
			"session_id": conversationSessionID,
		})
		
		defer func() {
			if obsLogger != nil {
				obsLogger.LokiLogger.WithField("event", "session_end").
					WithField("session_id", conversationSessionID).
					WithField("category", "session").
					Info("📋 Conversation session ended")
			}
		}()
	}

	// Create proxy handler  
	proxyHandler := proxy.NewHandler(cfg, obsLogger, conversationSessionID)

	// Setup HTTP routes
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/v1/messages", proxyHandler.HandleAnthropicRequest)
	http.Handle("/metrics", promhttp.Handler())

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
