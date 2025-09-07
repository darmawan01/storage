package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Registry manages multiple storage handlers with shared MinIO connection
type Registry struct {
	client   *minio.Client
	config   StorageConfig
	handlers map[string]*Handler
	mutex    sync.RWMutex
}

// NewRegistry creates a new storage registry
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]*Handler),
	}
}

// Initialize sets up the MinIO client and validates configuration
func (r *Registry) Initialize(config StorageConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}

	// Initialize MinIO client
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure: config.UseSSL,
		Region: config.Region,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize MinIO client: %w", err)
	}

	r.client = client
	r.config = config

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = client.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to MinIO: %w", err)
	}

	return nil
}

// Register creates a new storage handler with the given configuration
func (r *Registry) Register(name string, config *HandlerConfig) (*Handler, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Check if handler already exists
	if _, exists := r.handlers[name]; exists {
		return nil, &StorageError{Code: "HANDLER_EXISTS", Message: "Handler " + name + " already exists"}
	}

	handler := &Handler{
		name:     name,
		config:   config,
		client:   r.client,
		registry: r,
	}

	// Initialize handler
	if err := handler.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize handler %s: %w", name, err)
	}

	r.handlers[name] = handler
	return handler, nil
}

// GetHandler retrieves a registered handler by name
func (r *Registry) GetHandler(name string) (*Handler, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	handler, exists := r.handlers[name]
	if !exists {
		return nil, &StorageError{Code: "HANDLER_NOT_FOUND", Message: "Handler " + name + " not found"}
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
func (r *Registry) GetConfig() StorageConfig {
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
			fmt.Printf("Error closing handler %s: %v\n", handler.name, err)
		}
	}

	// Clear handlers map
	r.handlers = make(map[string]*Handler)

	return nil
}

// HealthCheck performs a health check on the storage system
func (r *Registry) HealthCheck(ctx context.Context) error {
	if r.client == nil {
		return &StorageError{Code: "NOT_INITIALIZED", Message: "Registry not initialized"}
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
