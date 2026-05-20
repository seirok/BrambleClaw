package httpserver

import (
	"sync"
	"time"
)

// MetricsCollector collects in-memory metrics
type MetricsCollector struct {
	mu sync.RWMutex

	requestCount map[string]int64
	latencySum   map[string]time.Duration
	errorCount   map[string]int64

	tokenUsage map[string]int64
}

// NewMetricsCollector creates a new MetricsCollector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		requestCount: make(map[string]int64),
		latencySum:   make(map[string]time.Duration),
		errorCount:   make(map[string]int64),
		tokenUsage:   make(map[string]int64),
	}
}

// RecordRequest records a request
func (m *MetricsCollector) RecordRequest(endpoint string, duration time.Duration, hasError bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requestCount[endpoint]++
	m.latencySum[endpoint] += duration
	if hasError {
		m.errorCount[endpoint]++
	}
}

// RecordTokenUsage records token usage for an agent
func (m *MetricsCollector) RecordTokenUsage(agent string, tokens int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokenUsage[agent] += tokens
}

// GetSummary returns aggregated metrics
func (m *MetricsCollector) GetSummary() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalRequests := int64(0)
	totalLatency := time.Duration(0)
	totalErrors := int64(0)
	totalTokens := int64(0)

	for _, count := range m.requestCount {
		totalRequests += count
	}
	for _, latency := range m.latencySum {
		totalLatency += latency
	}
	for _, count := range m.errorCount {
		totalErrors += count
	}
	for _, tokens := range m.tokenUsage {
		totalTokens += tokens
	}

	avgLatency := time.Duration(0)
	if totalRequests > 0 {
		avgLatency = totalLatency / time.Duration(totalRequests)
	}

	errorRate := float64(0)
	if totalRequests > 0 {
		errorRate = float64(totalErrors) / float64(totalRequests) * 100
	}

	return map[string]interface{}{
		"total_requests": totalRequests,
		"total_errors":   totalErrors,
		"error_rate":     errorRate,
		"avg_latency_ms": avgLatency.Milliseconds(),
		"total_tokens":   totalTokens,
	}
}

// GetPerEndpoint returns per-endpoint metrics
func (m *MetricsCollector) GetPerEndpoint() map[string]map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]map[string]interface{})
	for endpoint, count := range m.requestCount {
		avgLatency := time.Duration(0)
		if count > 0 {
			avgLatency = m.latencySum[endpoint] / time.Duration(count)
		}
		errors := m.errorCount[endpoint]
		errorRate := float64(0)
		if count > 0 {
			errorRate = float64(errors) / float64(count) * 100
		}
		result[endpoint] = map[string]interface{}{
			"requests":    count,
			"errors":      errors,
			"error_rate":  errorRate,
			"avg_latency": avgLatency.Milliseconds(),
		}
	}
	return result
}
