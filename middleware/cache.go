package middleware

import (
	"context"
	"crypto/md5"
	"fmt"
	"sync"
	"time"
)

// CacheMiddleware handles caching of presigned URLs and other data
type CacheMiddleware struct {
	config CacheConfig
	cache  map[string]*CacheEntry
	mutex  sync.RWMutex
}

// CacheConfig represents cache middleware configuration
type CacheConfig struct {
	Enabled           bool          `json:"enabled"`            // Enable caching
	DefaultTTL        time.Duration `json:"default_ttl"`        // Default TTL for cache entries
	MaxSize           int           `json:"max_size"`           // Maximum number of cache entries
	CleanupInterval   time.Duration `json:"cleanup_interval"`   // How often to cleanup expired entries
	PresignedURLTTL   time.Duration `json:"presigned_url_ttl"`  // TTL for presigned URLs
	MetadataTTL       time.Duration `json:"metadata_ttl"`       // TTL for metadata
	EnableCompression bool          `json:"enable_compression"` // Enable compression for cache values
}

// CacheEntry represents a cache entry
type CacheEntry struct {
	Value        interface{} `json:"value"`
	ExpiresAt    time.Time   `json:"expires_at"`
	CreatedAt    time.Time   `json:"created_at"`
	AccessCount  int64       `json:"access_count"`
	LastAccessed time.Time   `json:"last_accessed"`
}

// NewCacheMiddleware creates a new cache middleware
func NewCacheMiddleware(config CacheConfig) *CacheMiddleware {
	middleware := &CacheMiddleware{
		config: config,
		cache:  make(map[string]*CacheEntry),
	}

	// Start cleanup routine if enabled
	if config.CleanupInterval > 0 {
		go middleware.startCleanupRoutine()
	}

	return middleware
}

// Name returns the middleware name
func (m *CacheMiddleware) Name() string {
	return "cache"
}

// Process processes the request through cache middleware
func (m *CacheMiddleware) Process(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
	if !m.config.Enabled {
		return next(ctx, req)
	}

	// Only cache certain operations
	if req.Operation != "download" && req.Operation != "preview" {
		return next(ctx, req)
	}

	// Generate cache key
	cacheKey := m.generateCacheKey(req)

	// Try to get from cache
	if cached := m.get(cacheKey); cached != nil {
		if response, ok := cached.(*StorageResponse); ok {
			// Update access statistics
			m.updateAccessStats(cacheKey)
			return response, nil
		}
	}

	// Process with next middleware
	response, err := next(ctx, req)
	if err != nil {
		return response, err
	}

	// Cache the response if successful
	if response.Success {
		ttl := m.getTTLForOperation(req.Operation)
		m.set(cacheKey, response, ttl)
	}

	return response, err
}

// generateCacheKey generates a cache key for the request
func (m *CacheMiddleware) generateCacheKey(req *StorageRequest) string {
	// Create a hash of the request parameters
	keyData := fmt.Sprintf("%s:%s:%s:%s:%s",
		req.Operation,
		req.FileKey,
		req.UserID,
		req.ContentType,
		req.Category)

	hash := md5.Sum([]byte(keyData))
	return fmt.Sprintf("%x", hash)
}

// get retrieves a value from cache
func (m *CacheMiddleware) get(key string) interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	entry, exists := m.cache[key]
	if !exists {
		return nil
	}

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		return nil
	}

	// Update access statistics
	entry.AccessCount++
	entry.LastAccessed = time.Now()

	return entry.Value
}

// set stores a value in cache
func (m *CacheMiddleware) set(key string, value interface{}, ttl time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Check if cache is full
	if len(m.cache) >= m.config.MaxSize {
		m.evictLeastRecentlyUsed()
	}

	entry := &CacheEntry{
		Value:        value,
		ExpiresAt:    time.Now().Add(ttl),
		CreatedAt:    time.Now(),
		AccessCount:  0,
		LastAccessed: time.Now(),
	}

	m.cache[key] = entry
}

// updateAccessStats updates access statistics for a cache entry
func (m *CacheMiddleware) updateAccessStats(key string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if entry, exists := m.cache[key]; exists {
		entry.AccessCount++
		entry.LastAccessed = time.Now()
	}
}

// evictLeastRecentlyUsed removes the least recently used entry
func (m *CacheMiddleware) evictLeastRecentlyUsed() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range m.cache {
		if oldestKey == "" || entry.LastAccessed.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.LastAccessed
		}
	}

	if oldestKey != "" {
		delete(m.cache, oldestKey)
	}
}

// getTTLForOperation returns the TTL for a specific operation
func (m *CacheMiddleware) getTTLForOperation(operation string) time.Duration {
	switch operation {
	case "preview":
		return m.config.PresignedURLTTL
	case "download":
		return m.config.MetadataTTL
	default:
		return m.config.DefaultTTL
	}
}

// startCleanupRoutine starts a background cleanup routine
func (m *CacheMiddleware) startCleanupRoutine() {
	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		m.performCleanup()
	}
}

// performCleanup removes expired entries from cache
func (m *CacheMiddleware) performCleanup() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	expiredKeys := make([]string, 0)

	for key, entry := range m.cache {
		if now.After(entry.ExpiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		delete(m.cache, key)
	}

	if len(expiredKeys) > 0 {
		fmt.Printf("🧹 Cache cleanup: removed %d expired entries\n", len(expiredKeys))
	}
}

// GetStats returns cache statistics
func (m *CacheMiddleware) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	totalAccesses := int64(0)
	now := time.Now()
	expiredCount := 0

	for _, entry := range m.cache {
		totalAccesses += entry.AccessCount
		if now.After(entry.ExpiresAt) {
			expiredCount++
		}
	}

	return map[string]interface{}{
		"enabled":           m.config.Enabled,
		"total_entries":     len(m.cache),
		"max_size":          m.config.MaxSize,
		"expired_entries":   expiredCount,
		"total_accesses":    totalAccesses,
		"default_ttl":       m.config.DefaultTTL,
		"presigned_url_ttl": m.config.PresignedURLTTL,
		"metadata_ttl":      m.config.MetadataTTL,
	}
}

// Clear clears all cache entries
func (m *CacheMiddleware) Clear() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.cache = make(map[string]*CacheEntry)
}

// InvalidateKey removes a specific cache entry
func (m *CacheMiddleware) InvalidateKey(key string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.cache, key)
}

// DefaultCacheConfig returns a default cache configuration
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		Enabled:           true,
		DefaultTTL:        5 * time.Minute,  // 5 minutes
		MaxSize:           1000,             // 1000 entries
		CleanupInterval:   1 * time.Minute,  // Cleanup every minute
		PresignedURLTTL:   1 * time.Hour,    // 1 hour for presigned URLs
		MetadataTTL:       10 * time.Minute, // 10 minutes for metadata
		EnableCompression: false,            // Disable compression for now
	}
}
