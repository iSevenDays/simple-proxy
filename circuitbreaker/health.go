package circuitbreaker

import (
	"sync"
	"time"
)

// EndpointHealth tracks the health status of an endpoint
type EndpointHealth struct {
	URL               string    `json:"url"`
	FailureCount      int       `json:"failure_count"`
	SuccessCount      int       `json:"success_count"`
	TotalRequests     int       `json:"total_requests"`
	LastFailureTime   time.Time `json:"last_failure_time"`
	LastSuccessTime   time.Time `json:"last_success_time"`
	CircuitOpen       bool      `json:"circuit_open"`
	NextRetryTime     time.Time `json:"next_retry_time"`
	LastReorderCheck  time.Time `json:"last_reorder_check"`
}

// Config controls circuit breaker behavior
type Config struct {
	FailureThreshold   int           `json:"failure_threshold"`    // Number of failures before opening circuit
	BackoffDuration    time.Duration `json:"backoff_duration"`     // How long to wait before retrying failed endpoint
	MaxBackoffDuration time.Duration `json:"max_backoff_duration"` // Maximum backoff time
	ResetTimeout       time.Duration `json:"reset_timeout"`        // Time to reset failure count after success
}

// DefaultConfig returns sensible defaults for circuit breaker
func DefaultConfig() Config {
	return Config{
		FailureThreshold:   2,                // Open circuit after 2 consecutive failures
		BackoffDuration:    30 * time.Second, // Initial 30s backoff
		MaxBackoffDuration: 5 * time.Minute,  // Max 5min backoff
		ResetTimeout:       1 * time.Minute,  // Reset failure count after 1min of success
	}
}

// HealthManager manages endpoint health tracking
type HealthManager struct {
	config      Config
	healthMap   map[string]*EndpointHealth
	healthMutex sync.RWMutex
	obsLogger   interface {
		Info(component, category, requestID, message string, fields map[string]interface{})
		Warn(component, category, requestID, message string, fields map[string]interface{})
		Error(component, category, requestID, message string, fields map[string]interface{})
	}
}

// NewHealthManager creates a new health manager
func NewHealthManager(config Config) *HealthManager {
	return &HealthManager{
		config:    config,
		healthMap: make(map[string]*EndpointHealth),
	}
}

// SetObservabilityLogger sets the observability logger for structured logging
func (hm *HealthManager) SetObservabilityLogger(obsLogger interface {
	Info(component, category, requestID, message string, fields map[string]interface{})
	Warn(component, category, requestID, message string, fields map[string]interface{})
	Error(component, category, requestID, message string, fields map[string]interface{})
}) {
	hm.obsLogger = obsLogger
}

// InitializeEndpoints initializes health tracking for all endpoints
func (hm *HealthManager) InitializeEndpoints(endpoints []string) {
	hm.healthMutex.Lock()
	defer hm.healthMutex.Unlock()

	for _, endpoint := range endpoints {
		if _, exists := hm.healthMap[endpoint]; !exists {
			hm.healthMap[endpoint] = &EndpointHealth{
				URL:          endpoint,
				FailureCount: 0,
				CircuitOpen:  false,
			}
		}
	}
}

// IsHealthy checks if an endpoint is available (circuit closed)
func (hm *HealthManager) IsHealthy(endpoint string) bool {
	hm.healthMutex.RLock()
	defer hm.healthMutex.RUnlock()

	health, exists := hm.healthMap[endpoint]
	if !exists {
		return true // Unknown endpoints are assumed healthy
	}

	// If circuit is open, check if it's time to retry
	if health.CircuitOpen {
		if time.Now().After(health.NextRetryTime) {
			return true // Time to test the endpoint again
		}
		return false // Still in backoff period
	}

	return true // Circuit is closed, endpoint is healthy
}

// GetHealthDebug returns debug information about an endpoint's health
func (hm *HealthManager) GetHealthDebug(endpoint string) (failureCount int, circuitOpen bool, nextRetryTime time.Time, exists bool) {
	hm.healthMutex.RLock()
	defer hm.healthMutex.RUnlock()

	health, exists := hm.healthMap[endpoint]
	if !exists {
		return 0, false, time.Time{}, false
	}

	return health.FailureCount, health.CircuitOpen, health.NextRetryTime, true
}

// CalculateSuccessRate calculates the success rate for an endpoint
func (hm *HealthManager) CalculateSuccessRate(endpoint string) float64 {
	hm.healthMutex.RLock()
	defer hm.healthMutex.RUnlock()

	health, exists := hm.healthMap[endpoint]
	if !exists || health.TotalRequests == 0 {
		return 0.5 // Default neutral rate for new endpoints
	}

	return float64(health.SuccessCount) / float64(health.TotalRequests)
}