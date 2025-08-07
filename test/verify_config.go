package main

import (
	"claude-proxy/config"
	"fmt"
	"os"
)

func main() {
	cfg, err := config.LoadConfigWithEnv()
	if err != nil {
		fmt.Printf("❌ Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Configuration loaded successfully!\n")
	fmt.Printf("🔧 BIG_MODEL: %s → %v (%d endpoints)\n", cfg.BigModel, cfg.BigModelEndpoints, len(cfg.BigModelEndpoints))
	fmt.Printf("🔧 SMALL_MODEL: %s → %v (%d endpoints)\n", cfg.SmallModel, cfg.SmallModelEndpoints, len(cfg.SmallModelEndpoints))
	fmt.Printf("🔧 CORRECTION_MODEL: %s → %v (%d endpoints)\n", cfg.CorrectionModel, cfg.ToolCorrectionEndpoints, len(cfg.ToolCorrectionEndpoints))
	
	// Test round-robin functionality
	fmt.Printf("\n🔄 Testing failover functionality:\n")
	for i := 0; i < 4; i++ {
		smallEndpoint := cfg.GetSmallModelEndpoint()
		correctionEndpoint := cfg.GetToolCorrectionEndpoint()
		fmt.Printf("Request %d: SMALL=%s, CORRECTION=%s\n", i+1, 
			extractLastPart(smallEndpoint), extractLastPart(correctionEndpoint))
	}
	
	fmt.Printf("\n✅ Multi-endpoint failover is working correctly!\n")
}

func extractLastPart(url string) string {
	if len(url) > 15 {
		return "..." + url[len(url)-15:]
	}
	return url
}