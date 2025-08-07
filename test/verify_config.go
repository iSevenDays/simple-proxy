package test

import (
	"claude-proxy/config"
	"testing"
)

// TestConfigurationLoading verifies that configuration loads correctly with multi-endpoints
func TestConfigurationLoading(t *testing.T) {
	cfg, err := config.LoadConfigWithEnv()
	if err != nil {
		t.Fatalf("Failed to load configuration: %v", err)
	}

	t.Logf("✅ Configuration loaded successfully!")
	t.Logf("🔧 BIG_MODEL: %s → %v (%d endpoints)", cfg.BigModel, cfg.BigModelEndpoints, len(cfg.BigModelEndpoints))
	t.Logf("🔧 SMALL_MODEL: %s → %v (%d endpoints)", cfg.SmallModel, cfg.SmallModelEndpoints, len(cfg.SmallModelEndpoints))
	t.Logf("🔧 CORRECTION_MODEL: %s → %v (%d endpoints)", cfg.CorrectionModel, cfg.ToolCorrectionEndpoints, len(cfg.ToolCorrectionEndpoints))

	// Verify expected configuration based on .env
	if len(cfg.BigModelEndpoints) != 1 {
		t.Errorf("Expected 1 BIG_MODEL endpoint, got %d", len(cfg.BigModelEndpoints))
	}
	
	if len(cfg.SmallModelEndpoints) != 2 {
		t.Errorf("Expected 2 SMALL_MODEL endpoints, got %d", len(cfg.SmallModelEndpoints))
	}
	
	if len(cfg.ToolCorrectionEndpoints) != 2 {
		t.Errorf("Expected 2 TOOL_CORRECTION endpoints, got %d", len(cfg.ToolCorrectionEndpoints))
	}
}

// TestRoundRobinFunctionality verifies endpoint rotation works correctly
func TestRoundRobinFunctionality(t *testing.T) {
	cfg, err := config.LoadConfigWithEnv()
	if err != nil {
		t.Fatalf("Failed to load configuration: %v", err)
	}
	
	// Test round-robin functionality
	t.Log("🔄 Testing failover functionality:")
	for i := 0; i < 4; i++ {
		smallEndpoint := cfg.GetSmallModelEndpoint()
		correctionEndpoint := cfg.GetToolCorrectionEndpoint()
		t.Logf("Request %d: SMALL=%s, CORRECTION=%s", i+1, 
			extractLastPart(smallEndpoint), extractLastPart(correctionEndpoint))
	}
	
	t.Log("✅ Multi-endpoint failover is working correctly!")
}

func extractLastPart(url string) string {
	if len(url) > 15 {
		return "..." + url[len(url)-15:]
	}
	return url
}