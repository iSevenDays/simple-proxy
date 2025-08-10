package test

import (
	"claude-proxy/circuitbreaker"
	"claude-proxy/config"
	"testing"
	"time"
)

// TestCircuitBreakerBasicFunctionality tests basic circuit breaker operations
func TestCircuitBreakerBasicFunctionality(t *testing.T) {
	// Create config with default circuit breaker settings
	cfg := config.GetDefaultConfig()
	cfg.ToolCorrectionEndpoints = []string{
		"http://192.168.0.46:11434/v1/chat/completions",
		"http://192.168.0.50:11434/v1/chat/completions",
	}
	cfg.HealthManager = circuitbreaker.NewHealthManager(circuitbreaker.DefaultConfig())
	
	// Initialize health map
	err := initializeConfigForTesting(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}
	
	endpoint1 := cfg.ToolCorrectionEndpoints[0]
	endpoint2 := cfg.ToolCorrectionEndpoints[1]
	
	t.Run("InitiallyHealthy", func(t *testing.T) {
		if !cfg.IsEndpointHealthy(endpoint1) {
			t.Error("Endpoint should be initially healthy")
		}
		if !cfg.IsEndpointHealthy(endpoint2) {
			t.Error("Endpoint should be initially healthy")
		}
	})
	
	t.Run("RecordSingleFailure", func(t *testing.T) {
		cfg.RecordEndpointFailure(endpoint1)
		
		// Should still be healthy after one failure (threshold is 2)
		if !cfg.IsEndpointHealthy(endpoint1) {
			t.Error("Endpoint should still be healthy after one failure")
		}
	})
	
	t.Run("CircuitOpensAfterThreshold", func(t *testing.T) {
		cfg.RecordEndpointFailure(endpoint1) // Second failure
		
		// Circuit should now be open
		if cfg.IsEndpointHealthy(endpoint1) {
			t.Error("Circuit should be open after reaching failure threshold")
		}
	})
	
	t.Run("CircuitClosesOnSuccess", func(t *testing.T) {
		// Simulate time passing to allow retry
		time.Sleep(100 * time.Millisecond)
		
		// Record success to close circuit
		cfg.RecordEndpointSuccess(endpoint1)
		
		// Circuit should now be closed
		if !cfg.IsEndpointHealthy(endpoint1) {
			t.Error("Circuit should be closed after success")
		}
	})
}

// TestCircuitBreakerBackoff tests the backoff behavior
func TestCircuitBreakerBackoff(t *testing.T) {
	cfg := config.GetDefaultConfig()
	cfg.ToolCorrectionEndpoints = []string{"http://test:8080"}
	
	// Use shorter backoff for testing
	testConfig := circuitbreaker.Config{
		FailureThreshold:   1,                       // Open after 1 failure for faster testing
		BackoffDuration:    100 * time.Millisecond,  // Very short backoff
		MaxBackoffDuration: 10 * time.Second,        // Set a proper max for testing
		ResetTimeout:       1 * time.Minute,
	}
	cfg.HealthManager = circuitbreaker.NewHealthManager(testConfig)
	
	
	err := initializeConfigForTesting(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}
	
	endpoint := cfg.ToolCorrectionEndpoints[0]
	
	t.Run("BackoffPreventsRetry", func(t *testing.T) {
		// Debug: check initial health
		t.Logf("Initial endpoint health: %v", cfg.IsEndpointHealthy(endpoint))
		
		// Trigger circuit breaker
		cfg.RecordEndpointFailure(endpoint)
		
		// Debug: check the health status and internal state
		t.Logf("Circuit breaker config: threshold=%d, backoff=%v", testConfig.FailureThreshold, testConfig.BackoffDuration)
		
		// Inspect the endpoint health directly
		failures, circuitOpen, nextRetry, exists := cfg.GetEndpointHealthDebug(endpoint)
		if exists {
			t.Logf("Endpoint health state: failures=%d, circuitOpen=%v, nextRetry=%v", 
				failures, circuitOpen, nextRetry)
		} else {
			t.Logf("No health record found for endpoint")
		}
		
		// Should be unhealthy immediately
		healthy := cfg.IsEndpointHealthy(endpoint)
		t.Logf("Endpoint healthy after failure: %v", healthy)
		if healthy {
			t.Error("Endpoint should be unhealthy after circuit opens")
		}
		
		// Wait half the backoff period
		time.Sleep(50 * time.Millisecond)
		
		// Should still be unhealthy
		healthy = cfg.IsEndpointHealthy(endpoint)
		t.Logf("Endpoint healthy after 50ms: %v", healthy)
		if healthy {
			t.Error("Endpoint should still be unhealthy during backoff")
		}
	})
	
	t.Run("BackoffAllowsRetryAfterTimeout", func(t *testing.T) {
		// Wait for full backoff period
		time.Sleep(100 * time.Millisecond)
		
		// Should now be healthy for retry attempt
		if !cfg.IsEndpointHealthy(endpoint) {
			t.Error("Endpoint should be healthy after backoff period")
		}
	})
}

// TestHealthyEndpointSelection tests that healthy endpoints are preferred
func TestHealthyEndpointSelection(t *testing.T) {
	cfg := config.GetDefaultConfig()
	cfg.ToolCorrectionEndpoints = []string{
		"http://unhealthy:8080",
		"http://healthy:8080",
	}
	testConfig := circuitbreaker.Config{
		FailureThreshold:   1,                  // Trigger on first failure
		BackoffDuration:    2 * time.Second,   // Set backoff duration
		MaxBackoffDuration: 10 * time.Second,  // Set proper max backoff
		ResetTimeout:       1 * time.Minute,
	}
	cfg.HealthManager = circuitbreaker.NewHealthManager(testConfig)
	
	err := initializeConfigForTesting(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}
	
	unhealthyEndpoint := cfg.ToolCorrectionEndpoints[0]
	healthyEndpoint := cfg.ToolCorrectionEndpoints[1]
	
	// Make first endpoint unhealthy
	cfg.RecordEndpointFailure(unhealthyEndpoint)
	
	// GetHealthyToolCorrectionEndpoint should prefer the healthy one
	selectedEndpoint := cfg.GetHealthyToolCorrectionEndpoint()
	
	if selectedEndpoint != healthyEndpoint {
		t.Errorf("Expected healthy endpoint %s, got %s", healthyEndpoint, selectedEndpoint)
	}
}

// TestAllEndpointsUnhealthy tests behavior when all endpoints are unhealthy
func TestAllEndpointsUnhealthy(t *testing.T) {
	cfg := config.GetDefaultConfig()
	cfg.ToolCorrectionEndpoints = []string{
		"http://unhealthy1:8080",
		"http://unhealthy2:8080",
	}
	testConfig := circuitbreaker.Config{
		FailureThreshold:   1,                  // Trigger on first failure
		BackoffDuration:    2 * time.Second,   // Set backoff duration
		MaxBackoffDuration: 10 * time.Second,  // Set proper max backoff
		ResetTimeout:       1 * time.Minute,
	}
	cfg.HealthManager = circuitbreaker.NewHealthManager(testConfig)
	
	err := initializeConfigForTesting(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize config: %v", err)
	}
	
	// Make all endpoints unhealthy
	for _, endpoint := range cfg.ToolCorrectionEndpoints {
		cfg.RecordEndpointFailure(endpoint)
	}
	
	// Should still return an endpoint (last resort behavior)
	selectedEndpoint := cfg.GetHealthyToolCorrectionEndpoint()
	if selectedEndpoint == "" {
		t.Error("Should return an endpoint even when all are unhealthy")
	}
	
	// Should be one of the configured endpoints
	found := false
	for _, endpoint := range cfg.ToolCorrectionEndpoints {
		if selectedEndpoint == endpoint {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Returned endpoint %s not in configured list", selectedEndpoint)
	}
}

// initializeConfigForTesting initializes the config for testing (similar to real initialization)
func initializeConfigForTesting(cfg *config.Config) error {
	// Initialize the health map like LoadConfigWithEnv does
	// Initialize endpoints in health manager
	allEndpoints := append(cfg.BigModelEndpoints, cfg.SmallModelEndpoints...)
	allEndpoints = append(allEndpoints, cfg.ToolCorrectionEndpoints...)
	cfg.HealthManager.InitializeEndpoints(allEndpoints)
	return nil
}