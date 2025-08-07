package test

// stringPtr helper function for creating string pointers
// Shared across all test files to avoid duplication
func stringPtr(s string) *string {
	return &s
}

// MockConfigProvider provides a mock ConfigProvider for testing
type MockConfigProvider struct {
	Endpoint string
}

func (m *MockConfigProvider) GetToolCorrectionEndpoint() string {
	return m.Endpoint
}

func (m *MockConfigProvider) GetHealthyToolCorrectionEndpoint() string {
	return m.Endpoint
}

func (m *MockConfigProvider) RecordEndpointFailure(endpoint string) {
	// Mock implementation - no-op for basic tests
}

func (m *MockConfigProvider) RecordEndpointSuccess(endpoint string) {
	// Mock implementation - no-op for basic tests
}

// NewMockConfigProvider creates a new mock config provider
func NewMockConfigProvider(endpoint string) *MockConfigProvider {
	return &MockConfigProvider{Endpoint: endpoint}
}