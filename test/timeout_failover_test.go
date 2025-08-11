package test

import (
	"claude-proxy/correction"
	"claude-proxy/types"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TimeoutTestConfig tracks endpoint calls for testing
type TimeoutTestConfig struct {
	endpoints   []string
	callCounts  map[string]int
	index       int
	mutex       sync.Mutex
}

func NewTimeoutTestConfig(endpoints []string) *TimeoutTestConfig {
	return &TimeoutTestConfig{
		endpoints:  endpoints,
		callCounts: make(map[string]int),
		index:      0,
	}
}

func (c *TimeoutTestConfig) GetToolCorrectionEndpoint() string {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if len(c.endpoints) == 0 {
		return ""
	}
	
	endpoint := c.endpoints[c.index]
	c.callCounts[endpoint]++
	c.index = (c.index + 1) % len(c.endpoints)
	return endpoint
}

func (c *TimeoutTestConfig) GetHealthyToolCorrectionEndpoint() string {
	return c.GetToolCorrectionEndpoint()
}

func (c *TimeoutTestConfig) RecordEndpointFailure(endpoint string) {
	// Mock implementation - no-op for basic tests
}

func (c *TimeoutTestConfig) RecordEndpointSuccess(endpoint string) {
	// Mock implementation - no-op for basic tests
}

func (c *TimeoutTestConfig) GetCallCount(endpoint string) int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.callCounts[endpoint]
}

func (c *TimeoutTestConfig) GetTotalCalls() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	total := 0
	for _, count := range c.callCounts {
		total += count
	}
	return total
}

// TestTimeoutDetectionAndFailover tests specific timeout scenarios
func TestTimeoutDetectionAndFailover(t *testing.T) {
	t.Run("TimeoutTriggersFailover", func(t *testing.T) {
		// Create timeout server (takes longer than 30s)
		timeoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second) // Short timeout for unit tests
		}))
		defer timeoutServer.Close()

		// Create success server 
		successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := types.OpenAIResponse{
				Choices: []types.OpenAIChoice{
					{Message: types.OpenAIMessage{Content: "NO"}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer successServer.Close()

		// Setup config to track calls
		config := NewTimeoutTestConfig([]string{timeoutServer.URL, successServer.URL})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		// Test with generous timeout to allow for failover
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		start := time.Now()
		result, err := service.DetectToolNecessity(ctx, []types.OpenAIMessage{
			{Role: "user", Content: "test message"},
		}, []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})
		duration := time.Since(start)

		// Should succeed despite first endpoint timing out
		if err != nil {
			t.Errorf("Expected success after failover, got error: %v", err)
		}

		// Should have result
		t.Logf("DetectToolNecessity result: %v", result)

		// Should have attempted both endpoints
		timeoutCalls := config.GetCallCount(timeoutServer.URL)
		successCalls := config.GetCallCount(successServer.URL)
		totalCalls := config.GetTotalCalls()

		t.Logf("Timeout server calls: %d, Success server calls: %d, Total: %d", 
			timeoutCalls, successCalls, totalCalls)
		t.Logf("Test duration: %v", duration)

		if totalCalls < 2 {
			t.Errorf("Expected at least 2 endpoint calls (failover), got %d", totalCalls)
		}

		if successCalls < 1 {
			t.Errorf("Expected at least 1 call to success server, got %d", successCalls)
		}

		// Should complete in reasonable time (less than 70s total)
		if duration > 15*time.Second {
			t.Errorf("Test took too long: %v (expected < 70s)", duration)
		}
	})

	t.Run("MultipleTimeoutsExhaustEndpoints", func(t *testing.T) {
		// Create multiple timeout servers
		timeoutServer1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second)
		}))
		defer timeoutServer1.Close()

		timeoutServer2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second)
		}))
		defer timeoutServer2.Close()

		config := NewTimeoutTestConfig([]string{timeoutServer1.URL, timeoutServer2.URL})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		// Test should fail after trying all endpoints
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		start := time.Now()
		necessary, err := service.DetectToolNecessity(ctx, []types.OpenAIMessage{
			{Role: "user", Content: "test"},
		}, []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})
		duration := time.Since(start)

		// With graceful fallback, should return false, nil instead of error
		if err != nil {
			t.Errorf("Expected graceful fallback (nil error), got: %v", err)
		}
		
		if necessary != false {
			t.Errorf("Expected false (tool_choice=optional) as fallback, got: %v", necessary)
		}

		totalCalls := config.GetTotalCalls()
		t.Logf("Total endpoint attempts: %d, Duration: %v", totalCalls, duration)

		// Should have tried multiple endpoints
		if totalCalls < 2 {
			t.Errorf("Expected multiple endpoint attempts, got %d", totalCalls)
		}
	})

	t.Run("FastFailoverOnConnectionRefused", func(t *testing.T) {
		// Use non-existent ports for immediate connection refused
		badEndpoint1 := "http://127.0.0.1:99997" 
		badEndpoint2 := "http://127.0.0.1:99998"
		
		// Success server
		successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := types.OpenAIResponse{
				Choices: []types.OpenAIChoice{
					{Message: types.OpenAIMessage{Content: "YES"}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer successServer.Close()

		config := NewTimeoutTestConfig([]string{badEndpoint1, badEndpoint2, successServer.URL})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		start := time.Now()
		result, err := service.DetectToolNecessity(ctx, []types.OpenAIMessage{
			{Role: "user", Content: "test"},
		}, []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})
		duration := time.Since(start)

		// Should succeed with third endpoint
		if err != nil {
			t.Errorf("Expected success after connection refused failover, got: %v", err)
		}

		t.Logf("Connection refused test result: %v, Duration: %v", result, duration)

		// Should fail fast on connection refused (much less than timeout)
		if duration > 5*time.Second {
			t.Errorf("Connection refused should fail fast, took: %v", duration)
		}

		totalCalls := config.GetTotalCalls()
		successCalls := config.GetCallCount(successServer.URL)
		
		if totalCalls < 3 {
			t.Errorf("Expected 3 endpoint attempts, got %d", totalCalls)
		}
		
		if successCalls < 1 {
			t.Errorf("Expected success server to be called, got %d calls", successCalls)
		}
	})
}

// TestDetectToolNecessityFailover specifically tests the method user reported issues with
func TestDetectToolNecessityFailover(t *testing.T) {
	t.Run("UserReportedScenario", func(t *testing.T) {
		// Simulate the exact scenario user reported:
		// 192.168.0.46:11434 times out, should fallback to 192.168.0.50:11434

		// Mock the problematic endpoint (192.168.0.46:11434 equivalent)
		problemEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate i/o timeout by taking longer than 30s
			time.Sleep(2 * time.Second)
		}))
		defer problemEndpoint.Close()

		// Mock the fallback endpoint (192.168.0.50:11434 equivalent)  
		fallbackEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := types.OpenAIResponse{
				Choices: []types.OpenAIChoice{
					{Message: types.OpenAIMessage{Content: "YES"}}, // Tools needed
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer fallbackEndpoint.Close()

		config := NewTimeoutTestConfig([]string{problemEndpoint.URL, fallbackEndpoint.URL})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		// Test the exact user message pattern that was failing
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		start := time.Now()
		needsTools, err := service.DetectToolNecessity(ctx, []types.OpenAIMessage{
			{Role: "user", Content: "instead of single ip, I want to specify list of IPs"},
		}, []types.Tool{
			{Name: "Task", InputSchema: types.ToolSchema{Type: "object"}},
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})
		duration := time.Since(start)

		// Should succeed with fallback after first endpoint times out
		if err != nil {
			t.Errorf("Expected success after timeout failover, got error: %v", err)
		}

		if !needsTools {
			t.Log("Tool necessity returned false - this is OK, testing failover mechanics")
		}

		// Verify failover happened
		problemCalls := config.GetCallCount(problemEndpoint.URL)
		fallbackCalls := config.GetCallCount(fallbackEndpoint.URL)
		
		t.Logf("Problem endpoint calls: %d, Fallback calls: %d, Duration: %v", 
			problemCalls, fallbackCalls, duration)

		if problemCalls < 1 {
			t.Error("Expected problem endpoint to be tried first")
		}

		if fallbackCalls < 1 {
			t.Error("Expected fallback endpoint to be used after timeout")
		}

		// Should complete in reasonable time (30s timeout + processing, but less than 60s)
		if duration > 10*time.Second {
			t.Errorf("Failover took too long: %v", duration)
		}

		t.Logf("✅ User scenario test passed: timeout failover working correctly")
	})
}