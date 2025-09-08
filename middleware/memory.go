package middleware

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryMiddleware handles memory management and prevents memory leaks
type MemoryMiddleware struct {
	config MemoryConfig
	mutex  sync.RWMutex
}

// MemoryConfig represents memory middleware configuration
type MemoryConfig struct {
	MaxMemoryUsage   int64         `json:"max_memory_usage"`  // Maximum memory usage in bytes
	CurrentUsage     int64         `json:"current_usage"`     // Current memory usage in bytes
	CleanupInterval  time.Duration `json:"cleanup_interval"`  // How often to cleanup
	MaxFileSize      int64         `json:"max_file_size"`     // Maximum file size to process
	EnableMonitoring bool          `json:"enable_monitoring"` // Enable memory monitoring
	AlertThreshold   float64       `json:"alert_threshold"`   // Alert when usage exceeds this percentage (0.0-1.0)
}

// NewMemoryMiddleware creates a new memory middleware
func NewMemoryMiddleware(config MemoryConfig) *MemoryMiddleware {
	middleware := &MemoryMiddleware{
		config: config,
	}

	// Start cleanup routine if enabled
	if config.CleanupInterval > 0 {
		go middleware.startCleanupRoutine()
	}

	return middleware
}

// Name returns the middleware name
func (m *MemoryMiddleware) Name() string {
	return "memory"
}

// Process processes the request through memory middleware
func (m *MemoryMiddleware) Process(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
	// Check if file size exceeds maximum allowed
	if req.FileSize > m.config.MaxFileSize {
		return &StorageResponse{
			Success: false,
			Error:   fmt.Errorf("file size %d exceeds maximum allowed %d", req.FileSize, m.config.MaxFileSize),
		}, nil
	}

	// Check if we have enough memory available
	if !m.checkMemoryAvailability(req.FileSize) {
		return &StorageResponse{
			Success: false,
			Error:   fmt.Errorf("insufficient memory available for file size %d", req.FileSize),
		}, nil
	}

	// Track memory usage
	m.addMemoryUsage(req.FileSize)
	defer m.removeMemoryUsage(req.FileSize)

	// Process with next middleware
	response, err := next(ctx, req)

	// Update memory usage based on response
	if response != nil && response.FileSize > 0 {
		m.addMemoryUsage(response.FileSize)
	}

	return response, err
}

// checkMemoryAvailability checks if there's enough memory available
func (m *MemoryMiddleware) checkMemoryAvailability(fileSize int64) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Check if adding this file would exceed memory limit
	return m.config.CurrentUsage+fileSize <= m.config.MaxMemoryUsage
}

// addMemoryUsage adds to current memory usage
func (m *MemoryMiddleware) addMemoryUsage(size int64) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.config.CurrentUsage += size

	// Check if we need to alert
	if m.config.EnableMonitoring && m.config.AlertThreshold > 0 {
		usagePercentage := float64(m.config.CurrentUsage) / float64(m.config.MaxMemoryUsage)
		if usagePercentage >= m.config.AlertThreshold {
			fmt.Printf("⚠️  Memory usage alert: %.2f%% (%.2f MB / %.2f MB)\n",
				usagePercentage*100,
				float64(m.config.CurrentUsage)/(1024*1024),
				float64(m.config.MaxMemoryUsage)/(1024*1024))
		}
	}
}

// removeMemoryUsage removes from current memory usage
func (m *MemoryMiddleware) removeMemoryUsage(size int64) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.config.CurrentUsage -= size
	if m.config.CurrentUsage < 0 {
		m.config.CurrentUsage = 0
	}
}

// GetMemoryStats returns current memory statistics
func (m *MemoryMiddleware) GetMemoryStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	usagePercentage := float64(m.config.CurrentUsage) / float64(m.config.MaxMemoryUsage)

	return map[string]interface{}{
		"current_usage":      m.config.CurrentUsage,
		"max_usage":          m.config.MaxMemoryUsage,
		"usage_percentage":   usagePercentage,
		"available_memory":   m.config.MaxMemoryUsage - m.config.CurrentUsage,
		"max_file_size":      m.config.MaxFileSize,
		"monitoring_enabled": m.config.EnableMonitoring,
		"alert_threshold":    m.config.AlertThreshold,
	}
}

// startCleanupRoutine starts a background cleanup routine
func (m *MemoryMiddleware) startCleanupRoutine() {
	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		m.performCleanup()
	}
}

// performCleanup performs memory cleanup
func (m *MemoryMiddleware) performCleanup() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Reset current usage to 0 (this is a simple cleanup strategy)
	// In a real implementation, you might want to track individual file usage
	// and clean up based on file access patterns
	oldUsage := m.config.CurrentUsage
	m.config.CurrentUsage = 0

	if oldUsage > 0 {
		fmt.Printf("🧹 Memory cleanup: freed %.2f MB\n", float64(oldUsage)/(1024*1024))
	}
}

// DefaultMemoryConfig returns a default memory configuration
func DefaultMemoryConfig() MemoryConfig {
	return MemoryConfig{
		MaxMemoryUsage:   100 * 1024 * 1024, // 100MB
		CurrentUsage:     0,
		CleanupInterval:  5 * time.Minute,  // Cleanup every 5 minutes
		MaxFileSize:      25 * 1024 * 1024, // 25MB max file size
		EnableMonitoring: true,
		AlertThreshold:   0.8, // Alert at 80% usage
	}
}
