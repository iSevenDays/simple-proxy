package main

import (
	"claude-proxy/config"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	// Backup original .env file if it exists
	if _, err := os.Stat(".env"); err == nil {
		err := os.Rename(".env", ".env.backup")
		if err != nil {
			log.Fatalf("Failed to backup .env: %v", err)
		}
		defer os.Rename(".env.backup", ".env")
	}

	// Copy test config to .env
	err := os.Rename("test_endpoints.env", ".env")
	if err != nil {
		log.Fatalf("Failed to setup test .env: %v", err)
	}
	defer os.Remove(".env")

	// Load configuration
	cfg, err := config.LoadConfigWithEnv()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("✅ Configuration loaded successfully!\n")
	fmt.Printf("🔧 BIG_MODEL endpoints: %v (%d)\n", cfg.BigModelEndpoints, len(cfg.BigModelEndpoints))
	fmt.Printf("🔧 SMALL_MODEL endpoints: %v (%d)\n", cfg.SmallModelEndpoints, len(cfg.SmallModelEndpoints))
	fmt.Printf("🔧 TOOL_CORRECTION endpoints: %v (%d)\n", cfg.ToolCorrectionEndpoints, len(cfg.ToolCorrectionEndpoints))

	// Test round-robin functionality
	fmt.Printf("\n🔄 Testing round-robin functionality:\n")
	for i := 0; i < 6; i++ {
		big := cfg.GetBigModelEndpoint()
		small := cfg.GetSmallModelEndpoint()
		correction := cfg.GetToolCorrectionEndpoint()
		// Extract IP from URL for cleaner display
		bigIP := extractIP(big)
		smallIP := extractIP(small)
		correctionIP := extractIP(correction)
		fmt.Printf("Round %d: BIG=%s, SMALL=%s, CORRECTION=%s\n", i+1, bigIP, smallIP, correctionIP)
	}

	fmt.Printf("\n✅ Multi-endpoint functionality test completed successfully!\n")
}

// extractIP extracts the IP address from a URL for cleaner display
func extractIP(url string) string {
	// Extract IP from URL like "http://192.168.0.46:11434/v1/chat/completions"
	if strings.Contains(url, "://") {
		parts := strings.Split(url, "://")
		if len(parts) > 1 {
			hostPart := strings.Split(parts[1], "/")[0]
			return strings.Split(hostPart, ":")[0] // Return just the IP
		}
	}
	return url
}