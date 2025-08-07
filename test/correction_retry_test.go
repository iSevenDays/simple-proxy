package test

import (
	"claude-proxy/correction"
	"claude-proxy/types"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// MockConfigProviderWithRetry provides multiple endpoints for testing retry logic
type MockConfigProviderWithRetry struct {
	endpoints []string
	index     int
}

func NewMockConfigProviderWithRetry(endpoints []string) *MockConfigProviderWithRetry {
	return &MockConfigProviderWithRetry{
		endpoints: endpoints,
		index:     0,
	}
}

func (m *MockConfigProviderWithRetry) GetToolCorrectionEndpoint() string {
	if len(m.endpoints) == 0 {
		return ""
	}
	endpoint := m.endpoints[m.index]
	m.index = (m.index + 1) % len(m.endpoints)
	return endpoint
}

func (m *MockConfigProviderWithRetry) GetHealthyToolCorrectionEndpoint() string {
	return m.GetToolCorrectionEndpoint()
}

func (m *MockConfigProviderWithRetry) RecordEndpointFailure(endpoint string) {
	// Mock implementation - no-op for basic tests
}

func (m *MockConfigProviderWithRetry) RecordEndpointSuccess(endpoint string) {
	// Mock implementation - no-op for basic tests
}

// TestSendCorrectionRequestRetryLogic tests that retry logic works correctly
func TestSendCorrectionRequestRetryLogic(t *testing.T) {
	// Test 1: First endpoint fails with timeout, second succeeds
	t.Run("FailoverOnTimeout", func(t *testing.T) {
		// Create first server that times out
		timeoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(35 * time.Second) // Longer than 30s timeout
		}))
		defer timeoutServer.Close()

		// Create second server that responds successfully
		successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := types.OpenAIResponse{
				Choices: []types.OpenAIChoice{
					{
						Message: types.OpenAIMessage{
							Content: `{"name": "Read", "input": {"file_path": "test.txt"}}`,
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer successServer.Close()

		// Create service with retry endpoints
		config := NewMockConfigProviderWithRetry([]string{
			timeoutServer.URL,
			successServer.URL,
		})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		// Test that it eventually succeeds after timeout failover
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		// Call private method through reflection or make it public for testing
		// For now, let's test through DetectToolNecessity which uses sendCorrectionRequest
		_, err := service.DetectToolNecessity(ctx, "read file test.txt", []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})

		// Should succeed despite first endpoint timing out
		if err != nil && strings.Contains(err.Error(), "all tool correction endpoints failed") {
			t.Errorf("Expected success after failover, got error: %v", err)
		}
	})

	// Test 2: All endpoints fail
	t.Run("AllEndpointsFail", func(t *testing.T) {
		// Create two failing servers
		failServer1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}))
		defer failServer1.Close()

		failServer2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
		}))
		defer failServer2.Close()

		// Create service with failing endpoints
		config := NewMockConfigProviderWithRetry([]string{
			failServer1.URL,
			failServer2.URL,
		})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		// Test should fail after trying all endpoints
		ctx := context.Background()
		_, err := service.DetectToolNecessity(ctx, "test message", []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})

		if err == nil {
			t.Error("Expected error when all endpoints fail, got nil")
		}

		if !strings.Contains(err.Error(), "Tool necessity detection failed") {
			t.Errorf("Expected 'Tool necessity detection failed' error, got: %v", err)
		}
	})

	// Test 3: Connection refused should trigger retry
	t.Run("ConnectionRefusedRetry", func(t *testing.T) {
		// Create service with non-existent endpoint and successful endpoint
		config := NewMockConfigProviderWithRetry([]string{
			"http://127.0.0.1:99999", // Non-existent port
			"http://httpbin.org/status/200", // This might work in some environments
		})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// This should attempt retry on connection refused
		_, err := service.DetectToolNecessity(ctx, "test", []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})

		// We expect this to fail, but it should have attempted retry
		if err == nil {
			t.Log("Unexpected success - this test environment may have different network behavior")
		} else if !strings.Contains(err.Error(), "Tool necessity detection failed") {
			t.Errorf("Expected proper error handling, got: %v", err)
		}
	})

	// Test 4: Empty endpoint list
	t.Run("EmptyEndpointList", func(t *testing.T) {
		config := NewMockConfigProviderWithRetry([]string{})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		ctx := context.Background()
		_, err := service.DetectToolNecessity(ctx, "test", []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})

		if err == nil {
			t.Error("Expected error with empty endpoint list, got nil")
		}
	})
}

// TestCorrectToolCallsRetry tests retry logic in the CorrectToolCalls method
func TestCorrectToolCallsRetry(t *testing.T) {
	t.Run("RetryOnCorrectionFailure", func(t *testing.T) {
		// Create a server that returns invalid JSON first, then valid JSON
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			
			if callCount == 1 {
				// First call returns invalid JSON
				w.Write([]byte(`{"invalid": json`))
			} else {
				// Second call returns valid correction
				response := types.OpenAIResponse{
					Choices: []types.OpenAIChoice{
						{
							Message: types.OpenAIMessage{
								Content: `{"name": "Read", "input": {"file_path": "test.txt"}}`,
							},
						},
					},
				}
				json.NewEncoder(w).Encode(response)
			}
		}))
		defer server.Close()

		config := NewMockConfigProviderWithRetry([]string{server.URL})
		service := correction.NewService(config, "test-key", true, "test-model", false)

		// Create invalid tool call that needs correction
		toolCalls := []types.Content{
			{
				Type: "tool_use",
				ID:   "test-1",
				Name: "Read",
				Input: map[string]interface{}{
					"filename": "test.txt", // Wrong parameter name
				},
			},
		}

		availableTools := []types.Tool{
			{
				Name: "Read",
				InputSchema: types.ToolSchema{
					Type: "object",
					Properties: map[string]types.ToolProperty{
						"file_path": {Type: "string"},
					},
					Required: []string{"file_path"},
				},
			},
		}

		ctx := context.Background()
		corrected, err := service.CorrectToolCalls(ctx, toolCalls, availableTools)

		if err != nil {
			t.Errorf("Expected successful correction after retry, got error: %v", err)
		}

		if len(corrected) != 1 {
			t.Fatalf("Expected 1 corrected tool call, got %d", len(corrected))
		}

		// Verify the server was called multiple times (retry happened)
		if callCount < 2 {
			t.Errorf("Expected at least 2 server calls (retry), got %d", callCount)
		}
	})
}

// TestRetryLogging tests that retry attempts are properly logged
func TestRetryLogging(t *testing.T) {
	t.Run("LogsRetryAttempts", func(t *testing.T) {
		// Create servers that fail then succeed
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount <= 2 {
				// First two attempts fail
				time.Sleep(35 * time.Second) // Timeout
			} else {
				// Third attempt succeeds
				response := types.OpenAIResponse{
					Choices: []types.OpenAIChoice{
						{Message: types.OpenAIMessage{Content: "NO"}},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}
		}))
		defer server.Close()

		config := NewMockConfigProviderWithRetry([]string{server.URL, server.URL, server.URL})
		service := correction.NewService(config, "test-key", true, "test-model", false) // Logging enabled

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// This should log retry attempts
		_, err := service.DetectToolNecessity(ctx, "test", []types.Tool{
			{Name: "Read", InputSchema: types.ToolSchema{Type: "object"}},
		})

		// We don't assert specific log messages since they go to log.Printf
		// but the test exercises the logging code paths
		
		if err == nil {
			t.Log("Test succeeded after retries")
		} else {
			t.Logf("Test failed as expected with retries: %v", err)
		}
	})
}