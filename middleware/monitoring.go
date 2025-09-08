package middleware

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MonitoringMiddleware handles performance monitoring and metrics collection
type MonitoringMiddleware struct {
	config MonitoringConfig
	stats  *MonitoringStats
	mutex  sync.RWMutex
}

// MonitoringConfig represents monitoring middleware configuration
type MonitoringConfig struct {
	Enabled             bool          `json:"enabled"`              // Enable monitoring
	TrackLatency        bool          `json:"track_latency"`        // Track operation latency
	TrackThroughput     bool          `json:"track_throughput"`     // Track throughput metrics
	TrackErrors         bool          `json:"track_errors"`         // Track error rates
	TrackMemory         bool          `json:"track_memory"`         // Track memory usage
	TrackConcurrency    bool          `json:"track_concurrency"`    // Track concurrent operations
	MetricsInterval     time.Duration `json:"metrics_interval"`     // How often to log metrics
	EnableAlerts        bool          `json:"enable_alerts"`        // Enable performance alerts
	LatencyThreshold    time.Duration `json:"latency_threshold"`    // Alert if latency exceeds this
	ErrorThreshold      float64       `json:"error_threshold"`      // Alert if error rate exceeds this (0.0-1.0)
	ThroughputThreshold int64         `json:"throughput_threshold"` // Alert if throughput drops below this
}

// MonitoringStats represents collected monitoring statistics
type MonitoringStats struct {
	// Operation counters
	TotalOperations int64 `json:"total_operations"`
	SuccessfulOps   int64 `json:"successful_ops"`
	FailedOps       int64 `json:"failed_ops"`

	// Latency metrics
	TotalLatency time.Duration `json:"total_latency"`
	MinLatency   time.Duration `json:"min_latency"`
	MaxLatency   time.Duration `json:"max_latency"`
	AvgLatency   time.Duration `json:"avg_latency"`

	// Throughput metrics
	BytesProcessed int64 `json:"bytes_processed"`
	FilesProcessed int64 `json:"files_processed"`

	// Error tracking
	ErrorCounts map[string]int64 `json:"error_counts"`

	// Operation-specific stats
	OperationStats map[string]*OperationStats `json:"operation_stats"`

	// Timestamps
	StartTime time.Time `json:"start_time"`
	LastReset time.Time `json:"last_reset"`
}

// OperationStats represents statistics for a specific operation
type OperationStats struct {
	Count          int64         `json:"count"`
	SuccessCount   int64         `json:"success_count"`
	ErrorCount     int64         `json:"error_count"`
	TotalLatency   time.Duration `json:"total_latency"`
	MinLatency     time.Duration `json:"min_latency"`
	MaxLatency     time.Duration `json:"max_latency"`
	BytesProcessed int64         `json:"bytes_processed"`
	LastOperation  time.Time     `json:"last_operation"`
}

// NewMonitoringMiddleware creates a new monitoring middleware
func NewMonitoringMiddleware(config MonitoringConfig) *MonitoringMiddleware {
	stats := &MonitoringStats{
		ErrorCounts:    make(map[string]int64),
		OperationStats: make(map[string]*OperationStats),
		StartTime:      time.Now(),
		LastReset:      time.Now(),
	}

	middleware := &MonitoringMiddleware{
		config: config,
		stats:  stats,
	}

	// Start metrics logging if enabled
	if config.MetricsInterval > 0 {
		go middleware.startMetricsLogging()
	}

	return middleware
}

// Name returns the middleware name
func (m *MonitoringMiddleware) Name() string {
	return "monitoring"
}

// Process processes the request through monitoring middleware
func (m *MonitoringMiddleware) Process(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
	if !m.config.Enabled {
		return next(ctx, req)
	}

	start := time.Now()

	// Track concurrent operations
	if m.config.TrackConcurrency {
		m.incrementConcurrency()
		defer m.decrementConcurrency()
	}

	// Process with next middleware
	response, err := next(ctx, req)

	// Calculate latency
	latency := time.Since(start)

	// Update statistics
	m.updateStats(req.Operation, response, err, latency, req.FileSize)

	// Check for alerts
	if m.config.EnableAlerts {
		m.checkAlerts()
	}

	return response, err
}

// updateStats updates monitoring statistics
func (m *MonitoringMiddleware) updateStats(operation string, response *StorageResponse, err error, latency time.Duration, fileSize int64) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Update global counters
	m.stats.TotalOperations++
	if err != nil || (response != nil && !response.Success) {
		m.stats.FailedOps++
		if err != nil {
			errorType := fmt.Sprintf("%T", err)
			m.stats.ErrorCounts[errorType]++
		}
	} else {
		m.stats.SuccessfulOps++
	}

	// Update latency metrics
	if m.config.TrackLatency {
		m.stats.TotalLatency += latency
		if m.stats.MinLatency == 0 || latency < m.stats.MinLatency {
			m.stats.MinLatency = latency
		}
		if latency > m.stats.MaxLatency {
			m.stats.MaxLatency = latency
		}
		m.stats.AvgLatency = m.stats.TotalLatency / time.Duration(m.stats.TotalOperations)
	}

	// Update throughput metrics
	if m.config.TrackThroughput && fileSize > 0 {
		m.stats.BytesProcessed += fileSize
		m.stats.FilesProcessed++
	}

	// Update operation-specific stats
	if m.stats.OperationStats[operation] == nil {
		m.stats.OperationStats[operation] = &OperationStats{
			MinLatency: latency,
		}
	}

	opStats := m.stats.OperationStats[operation]
	opStats.Count++
	opStats.LastOperation = time.Now()

	if err != nil || (response != nil && !response.Success) {
		opStats.ErrorCount++
	} else {
		opStats.SuccessCount++
	}

	opStats.TotalLatency += latency
	if opStats.MinLatency == 0 || latency < opStats.MinLatency {
		opStats.MinLatency = latency
	}
	if latency > opStats.MaxLatency {
		opStats.MaxLatency = latency
	}

	if fileSize > 0 {
		opStats.BytesProcessed += fileSize
	}
}

// incrementConcurrency increments the concurrent operations counter
func (m *MonitoringMiddleware) incrementConcurrency() {
	// This would be implemented with atomic operations in a real implementation
	// For now, we'll just track it in the stats
}

// decrementConcurrency decrements the concurrent operations counter
func (m *MonitoringMiddleware) decrementConcurrency() {
	// This would be implemented with atomic operations in a real implementation
}

// checkAlerts checks for performance alerts
func (m *MonitoringMiddleware) checkAlerts() {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Check latency alert
	if m.config.TrackLatency && m.stats.AvgLatency > m.config.LatencyThreshold {
		fmt.Printf("⚠️  High latency alert: %.2fms (threshold: %.2fms)\n",
			float64(m.stats.AvgLatency.Nanoseconds())/1e6,
			float64(m.config.LatencyThreshold.Nanoseconds())/1e6)
	}

	// Check error rate alert
	if m.stats.TotalOperations > 0 {
		errorRate := float64(m.stats.FailedOps) / float64(m.stats.TotalOperations)
		if errorRate > m.config.ErrorThreshold {
			fmt.Printf("⚠️  High error rate alert: %.2f%% (threshold: %.2f%%)\n",
				errorRate*100, m.config.ErrorThreshold*100)
		}
	}

	// Check throughput alert
	if m.config.TrackThroughput && m.stats.FilesProcessed > 0 {
		avgThroughput := m.stats.BytesProcessed / m.stats.FilesProcessed
		if avgThroughput < m.config.ThroughputThreshold {
			fmt.Printf("⚠️  Low throughput alert: %d bytes/file (threshold: %d bytes/file)\n",
				avgThroughput, m.config.ThroughputThreshold)
		}
	}
}

// startMetricsLogging starts a background metrics logging routine
func (m *MonitoringMiddleware) startMetricsLogging() {
	ticker := time.NewTicker(m.config.MetricsInterval)
	defer ticker.Stop()

	for range ticker.C {
		m.logMetrics()
	}
}

// logMetrics logs current metrics
func (m *MonitoringMiddleware) logMetrics() {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	fmt.Printf("📊 Storage Performance Metrics:\n")
	fmt.Printf("  Operations: %d total, %d successful, %d failed\n",
		m.stats.TotalOperations, m.stats.SuccessfulOps, m.stats.FailedOps)

	if m.config.TrackLatency {
		fmt.Printf("  Latency: avg=%.2fms, min=%.2fms, max=%.2fms\n",
			float64(m.stats.AvgLatency.Nanoseconds())/1e6,
			float64(m.stats.MinLatency.Nanoseconds())/1e6,
			float64(m.stats.MaxLatency.Nanoseconds())/1e6)
	}

	if m.config.TrackThroughput {
		fmt.Printf("  Throughput: %d files, %.2f MB total\n",
			m.stats.FilesProcessed, float64(m.stats.BytesProcessed)/(1024*1024))
	}

	// Log operation-specific stats
	for op, stats := range m.stats.OperationStats {
		fmt.Printf("  %s: %d ops, %.2f%% success rate\n",
			op, stats.Count, float64(stats.SuccessCount)/float64(stats.Count)*100)
	}
}

// GetStats returns current monitoring statistics
func (m *MonitoringMiddleware) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return map[string]interface{}{
		"enabled":          m.config.Enabled,
		"total_operations": m.stats.TotalOperations,
		"successful_ops":   m.stats.SuccessfulOps,
		"failed_ops":       m.stats.FailedOps,
		"success_rate":     float64(m.stats.SuccessfulOps) / float64(m.stats.TotalOperations),
		"avg_latency_ms":   float64(m.stats.AvgLatency.Nanoseconds()) / 1e6,
		"min_latency_ms":   float64(m.stats.MinLatency.Nanoseconds()) / 1e6,
		"max_latency_ms":   float64(m.stats.MaxLatency.Nanoseconds()) / 1e6,
		"bytes_processed":  m.stats.BytesProcessed,
		"files_processed":  m.stats.FilesProcessed,
		"error_counts":     m.stats.ErrorCounts,
		"operation_stats":  m.stats.OperationStats,
		"uptime_seconds":   time.Since(m.stats.StartTime).Seconds(),
	}
}

// ResetStats resets all monitoring statistics
func (m *MonitoringMiddleware) ResetStats() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.stats = &MonitoringStats{
		ErrorCounts:    make(map[string]int64),
		OperationStats: make(map[string]*OperationStats),
		StartTime:      time.Now(),
		LastReset:      time.Now(),
	}
}

// DefaultMonitoringConfig returns a default monitoring configuration
func DefaultMonitoringConfig() MonitoringConfig {
	return MonitoringConfig{
		Enabled:             true,
		TrackLatency:        true,
		TrackThroughput:     true,
		TrackErrors:         true,
		TrackMemory:         false,            // Disabled by default
		TrackConcurrency:    false,            // Disabled by default
		MetricsInterval:     30 * time.Second, // Log every 30 seconds
		EnableAlerts:        true,
		LatencyThreshold:    5 * time.Second, // Alert if latency > 5s
		ErrorThreshold:      0.1,             // Alert if error rate > 10%
		ThroughputThreshold: 1024,            // Alert if avg file size < 1KB
	}
}
