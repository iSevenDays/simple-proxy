package test

import (
	"claude-proxy/config"
	"strings"
	"testing"
)

// TestMultiEndpointConfiguration tests configuration loading with existing .env
func TestMultiEndpointConfiguration(t *testing.T) {
	// Load configuration using existing .env file
	cfg, err := config.LoadConfigWithEnv()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	t.Logf("✅ Configuration loaded successfully!")
	t.Logf("🔧 BIG_MODEL endpoints: %v (%d)", cfg.BigModelEndpoints, len(cfg.BigModelEndpoints))
	t.Logf("🔧 SMALL_MODEL endpoints: %v (%d)", cfg.SmallModelEndpoints, len(cfg.SmallModelEndpoints))
	t.Logf("🔧 TOOL_CORRECTION endpoints: %v (%d)", cfg.ToolCorrectionEndpoints, len(cfg.ToolCorrectionEndpoints))

	// Verify configuration meets requirements
	if len(cfg.BigModelEndpoints) == 0 {
		t.Error("Expected at least 1 BIG_MODEL endpoint")
	}
	if len(cfg.SmallModelEndpoints) == 0 {
		t.Error("Expected at least 1 SMALL_MODEL endpoint")
	}
	if len(cfg.ToolCorrectionEndpoints) == 0 {
		t.Error("Expected at least 1 TOOL_CORRECTION endpoint")
	}

	// Test round-robin functionality
	t.Log("\n🔄 Testing round-robin functionality:")
	for i := 0; i < 6; i++ {
		big := cfg.GetBigModelEndpoint()
		small := cfg.GetSmallModelEndpoint()
		correction := cfg.GetToolCorrectionEndpoint()
		
		// Extract IP from URL for cleaner display
		bigIP := extractIP(big)
		smallIP := extractIP(small)
		correctionIP := extractIP(correction)
		
		t.Logf("Round %d: BIG=%s, SMALL=%s, CORRECTION=%s", i+1, bigIP, smallIP, correctionIP)
		
		// Verify endpoints are valid URLs
		if !strings.HasPrefix(big, "http") {
			t.Errorf("Invalid BIG_MODEL endpoint: %s", big)
		}
		if !strings.HasPrefix(small, "http") {
			t.Errorf("Invalid SMALL_MODEL endpoint: %s", small)
		}
		if !strings.HasPrefix(correction, "http") {
			t.Errorf("Invalid TOOL_CORRECTION endpoint: %s", correction)
		}
	}

	t.Log("✅ Multi-endpoint functionality test completed successfully!")
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