package circuitbreaker

import (
	"time"
)

// RecordFailure marks an endpoint as failed and potentially opens its circuit
func (hm *HealthManager) RecordFailure(endpoint string) {
	hm.healthMutex.Lock()
	defer hm.healthMutex.Unlock()

	health, exists := hm.healthMap[endpoint]
	if !exists {
		health = &EndpointHealth{URL: endpoint}
		hm.healthMap[endpoint] = health
	}

	health.FailureCount++
	health.TotalRequests++
	health.LastFailureTime = time.Now()

	// Open circuit if failure threshold exceeded
	if health.FailureCount >= hm.config.FailureThreshold {
		health.CircuitOpen = true

		// Calculate backoff time with exponential backoff capped at max
		failuresOverThreshold := health.FailureCount - hm.config.FailureThreshold + 1
		if failuresOverThreshold < 1 {
			failuresOverThreshold = 1
		}
		backoff := time.Duration(int64(hm.config.BackoffDuration) * int64(failuresOverThreshold))
		if backoff > hm.config.MaxBackoffDuration {
			backoff = hm.config.MaxBackoffDuration
		}

		now := time.Now()
		health.NextRetryTime = now.Add(backoff)

		if hm.obsLogger != nil {
			hm.obsLogger.Error("circuit_breaker", "error", "", "Circuit breaker opened for endpoint", map[string]interface{}{
				"endpoint": endpoint,
				"failure_count": health.FailureCount,
				"backoff_duration": backoff.String(),
				"next_retry_time": health.NextRetryTime.Format(time.RFC3339),
			})
		}
	} else {
		if hm.obsLogger != nil {
			hm.obsLogger.Warn("circuit_breaker", "warning", "", "Endpoint failure recorded", map[string]interface{}{
				"endpoint": endpoint,
				"failure_count": health.FailureCount,
				"failure_threshold": hm.config.FailureThreshold,
			})
		}
	}
}

// RecordSuccess marks an endpoint as successful and potentially closes its circuit
func (hm *HealthManager) RecordSuccess(endpoint string) {
	hm.healthMutex.Lock()
	defer hm.healthMutex.Unlock()

	health, exists := hm.healthMap[endpoint]
	if !exists {
		health = &EndpointHealth{URL: endpoint}
		hm.healthMap[endpoint] = health
	}

	// Update success metrics
	health.SuccessCount++
	health.TotalRequests++
	health.LastSuccessTime = time.Now()

	// If circuit was open, close it and reset
	if health.CircuitOpen {
		health.CircuitOpen = false
		health.FailureCount = 0
		health.NextRetryTime = time.Time{}
		if hm.obsLogger != nil {
			hm.obsLogger.Info("circuit_breaker", "health", "", "Circuit breaker closed for endpoint", map[string]interface{}{
				"endpoint": endpoint,
				"status": "recovered",
			})
		}
	} else if health.FailureCount > 0 {
		// Gradually reduce failure count on success
		health.FailureCount = 0
		if hm.obsLogger != nil {
			hm.obsLogger.Info("circuit_breaker", "health", "", "Endpoint recovered", map[string]interface{}{
				"endpoint": endpoint,
				"status": "failure_count_reset",
			})
		}
	}
}

// SelectHealthyEndpoint returns the next healthy endpoint from a list
func (hm *HealthManager) SelectHealthyEndpoint(endpoints []string, currentIndex *int) string {
	if len(endpoints) == 0 {
		return ""
	}

	// Try to find a healthy endpoint, starting from current index
	attempts := 0
	maxAttempts := len(endpoints)
	
	for attempts < maxAttempts {
		endpoint := endpoints[*currentIndex]
		*currentIndex = (*currentIndex + 1) % len(endpoints)
		attempts++

		if hm.IsHealthy(endpoint) {
			return endpoint
		} else {
			failureCount, circuitOpen, nextRetry, exists := hm.GetHealthDebug(endpoint)
			if exists {
				if hm.obsLogger != nil {
					hm.obsLogger.Warn("circuit_breaker", "warning", "", "Skipping unhealthy endpoint", map[string]interface{}{
						"endpoint": endpoint,
						"failure_count": failureCount,
						"circuit_open": circuitOpen,
						"next_retry_time": nextRetry.Format(time.RFC3339),
					})
				}
			} else {
				if hm.obsLogger != nil {
					hm.obsLogger.Warn("circuit_breaker", "warning", "", "Skipping endpoint with no health info", map[string]interface{}{
						"endpoint": endpoint,
					})
				}
			}
		}
	}

	// If no healthy endpoints found, return the next one anyway (last resort)
	endpoint := endpoints[*currentIndex]
	*currentIndex = (*currentIndex + 1) % len(endpoints)
	if hm.obsLogger != nil {
		hm.obsLogger.Error("circuit_breaker", "error", "", "No healthy endpoints found, using fallback", map[string]interface{}{
			"fallback_endpoint": endpoint,
		})
	}
	return endpoint
}