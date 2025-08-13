package circuitbreaker

import (
	"time"
)

// endpointScore represents an endpoint with its performance metrics
type endpointScore struct {
	url         string
	successRate float64
	isHealthy   bool
}

// ReorderBySuccess reorders endpoint slices based on success rates
func (hm *HealthManager) ReorderBySuccess(endpoints []string, endpointType string) bool {
	now := time.Now()
	reorderInterval := 5 * time.Minute // Reorder every 5 minutes

	// Check if enough time has passed since last reorder
	hm.healthMutex.RLock()
	shouldReorder := false
	for _, health := range hm.healthMap {
		if now.Sub(health.LastReorderCheck) > reorderInterval {
			shouldReorder = true
			break
		}
	}
	hm.healthMutex.RUnlock()

	if !shouldReorder || len(endpoints) <= 1 {
		return false
	}

	// Calculate scores for each endpoint
	scores := make([]endpointScore, len(endpoints))
	for i, endpoint := range endpoints {
		scores[i] = endpointScore{
			url:         endpoint,
			successRate: hm.CalculateSuccessRate(endpoint),
			isHealthy:   hm.IsHealthy(endpoint),
		}
	}

	// Sort by: 1) healthy status (healthy first), 2) success rate (highest first)
	for i := 0; i < len(scores); i++ {
		for j := i + 1; j < len(scores); j++ {
			// Prioritize healthy endpoints
			if scores[i].isHealthy != scores[j].isHealthy {
				if scores[j].isHealthy && !scores[i].isHealthy {
					scores[i], scores[j] = scores[j], scores[i]
				}
				continue
			}
			// Among same health status, prioritize higher success rate
			if scores[j].successRate > scores[i].successRate {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	// Check if reordering occurred
	hasChanged := false
	for i, score := range scores {
		if endpoints[i] != score.url {
			hasChanged = true
		}
		endpoints[i] = score.url
	}

	// Update reorder timestamps
	hm.healthMutex.Lock()
	for _, health := range hm.healthMap {
		health.LastReorderCheck = now
	}
	hm.healthMutex.Unlock()

	// Log reordering if changes occurred
	if hasChanged {
		if hm.obsLogger != nil {
			// Create endpoint details for logging
			endpointDetails := make([]map[string]interface{}, len(scores))
			for i, score := range scores {
				endpointDetails[i] = map[string]interface{}{
					"position": i + 1,
					"endpoint": score.url,
					"success_rate": score.successRate,
					"is_healthy": score.isHealthy,
				}
			}
			hm.obsLogger.Info("circuit_breaker", "health", "", "Reordered endpoints by success rate", map[string]interface{}{
				"endpoint_type": endpointType,
				"endpoint_details": endpointDetails,
				"total_endpoints": len(scores),
			})
		}
	}

	return hasChanged
}