package main

import (
	"claude-proxy/config"
	"claude-proxy/logger"
	"claude-proxy/proxy"
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	// Load configuration with .env support
	cfg, err := config.LoadConfigWithEnv()
	if err != nil {
		log.Fatalf("❌ Failed to load config: %v", err)
	}

	log.Printf("🚀 Starting Claude Code Proxy...")
	log.Printf("🛠️ Tool correction: %v", cfg.ToolCorrectionEnabled)
	log.Printf("🤖 BIG_MODEL: %s → %s", cfg.BigModel, cfg.BigModelEndpoint)
	log.Printf("🤖 SMALL_MODEL: %s → %s", cfg.SmallModel, cfg.SmallModelEndpoint)
	log.Printf("🤖 CORRECTION_MODEL: %s → %s", cfg.CorrectionModel, cfg.ToolCorrectionEndpoint)
	log.Printf("🌐 Listening on port: %s", cfg.Port)

	// Initialize conversation logger if enabled
	var conversationLogger *logger.ConversationLogger
	if cfg.ConversationLoggingEnabled {
		logLevel := logger.ParseLevel(cfg.ConversationLogLevel)
		conversationLogger, err = logger.NewConversationLogger("logs", logLevel, cfg.ConversationMaskSensitive)
		if err != nil {
			log.Fatalf("❌ Failed to initialize conversation logger: %v", err)
		}
		log.Printf("💬 Conversation logging initialized: level=%s, session=%s", cfg.ConversationLogLevel, conversationLogger.GetSessionID())
		defer conversationLogger.Close()
	}

	// Create proxy handler
	proxyHandler := proxy.NewHandler(cfg, conversationLogger)

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

	log.Printf("✅ Claude Code Proxy started on http://localhost:%s", cfg.Port)
	log.Printf("📍 Anthropic endpoint: http://localhost:%s/v1/messages", cfg.Port)

	// Start server
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("❌ Server failed to start: %v", err)
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
