package registry

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/darmawan01/storage/config"
	"github.com/darmawan01/storage/errors"
	"github.com/darmawan01/storage/handler"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Registry manages multiple storage handlers with shared MinIO connection
type Registry struct {
	client   *minio.Client
	config   config.StorageConfig
	handlers map[string]*handler.Handler
	mutex    sync.RWMutex
}

// NewRegistry creates a new storage registry
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]*handler.Handler),
	}
}

// Initialize sets up the MinIO client and validates configuration
func (r *Registry) Initialize(config config.StorageConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}

	// Create HTTP transport with performance optimizations
	transport := &http.Transport{
		MaxIdleConns:        config.MaxConnections,
		MaxIdleConnsPerHost: config.MaxConnections / 2,
		IdleConnTimeout:     time.Duration(config.ConnectionTimeout) * time.Second,
		DisableCompression:  false,
		DisableKeepAlives:   false,
	}

	// Initialize MinIO client with performance optimizations
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure:    config.UseSSL,
		Region:    config.Region,
		Transport: transport,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize MinIO client: %w", err)
	}

	r.client = client
	r.config = config

	// Test connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.ConnectionTimeout)*time.Second)
	defer cancel()

	_, err = client.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to MinIO: %w", err)
	}

	return nil
}

// Register creates a new storage handler with the given configuration
func (r *Registry) Register(name string, config *handler.HandlerConfig) (*handler.Handler, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Check if handler already exists
	if _, exists := r.handlers[name]; exists {
		return nil, &errors.StorageError{Code: "HANDLER_EXISTS", Message: "Handler " + name + " already exists"}
	}

	handler := &handler.Handler{
		Name:   name,
		Config: config,
		Client: r.client,
		Region: r.config.Region,
	}

	// Initialize handler
	if err := handler.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize handler %s: %w", name, err)
	}

	r.handlers[name] = handler
	return handler, nil
}

// GetHandler retrieves a registered handler by name
func (r *Registry) GetHandler(name string) (*handler.Handler, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	handler, exists := r.handlers[name]
	if !exists {
		return nil, &errors.StorageError{Code: "HANDLER_NOT_FOUND", Message: "Handler " + name + " not found"}
	}

	return handler, nil
}

// ListHandlers returns all registered handler names
func (r *Registry) ListHandlers() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	return names
}

// GetConfig returns the storage configuration
func (r *Registry) GetConfig() config.StorageConfig {
	return r.config
}

// GetClient returns the MinIO client
func (r *Registry) GetClient() *minio.Client {
	return r.client
}

// Close closes all handlers and the MinIO client
func (r *Registry) Close() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Close all handlers
	for _, handler := range r.handlers {
		if err := handler.Close(); err != nil {
			// Log error but continue closing other handlers
			fmt.Printf("Error closing handler %s: %v\n", handler.Name, err)
		}
	}

	// Clear handlers map
	r.handlers = make(map[string]*handler.Handler)

	return nil
}

// HealthCheck performs a health check on the storage system
func (r *Registry) HealthCheck(ctx context.Context) error {
	if r.client == nil {
		return &errors.StorageError{Code: "NOT_INITIALIZED", Message: "Registry not initialized"}
	}

	// Test MinIO connection
	_, err := r.client.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("MinIO health check failed: %w", err)
	}

	// Check all handlers
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for name, handler := range r.handlers {
		if err := handler.HealthCheck(ctx); err != nil {
			return fmt.Errorf("handler %s health check failed: %w", name, err)
		}
	}

	return nil
}

// GetStats returns statistics about the registry
func (r *Registry) GetStats() map[string]interface{} {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	stats := map[string]interface{}{
		"handlers_count": len(r.handlers),
		"handlers":       make([]string, 0, len(r.handlers)),
		"config":         r.config,
	}

	for name := range r.handlers {
		stats["handlers"] = append(stats["handlers"].([]string), name)
	}

	return stats
}

// executeWithRetry executes a function with retry logic
func (r *Registry) executeWithRetry(ctx context.Context, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= r.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(r.config.RetryDelay) * time.Millisecond):
			}
		}

		lastErr = operation()
		if lastErr == nil {
			return nil
		}

		// Check if error is retryable
		if !r.isRetryableError(lastErr) {
			return lastErr
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", r.config.RetryAttempts+1, lastErr)
}

// isRetryableError determines if an error is retryable
func (r *Registry) isRetryableError(err error) bool {
	// Network errors, timeouts, and temporary failures are retryable
	if err == nil {
		return false
	}

	// Check for common retryable error patterns
	errorStr := err.Error()
	retryablePatterns := []string{
		"timeout",
		"connection refused",
		"connection reset",
		"temporary failure",
		"network is unreachable",
		"no route to host",
		"i/o timeout",
	}

	for _, pattern := range retryablePatterns {
		if contains(errorStr, pattern) {
			return true
		}
	}

	return false
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) && containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
