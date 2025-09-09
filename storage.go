package storage

import (
	"fmt"

	"github.com/darmawan01/storage/config"
	"github.com/darmawan01/storage/handler"
	"github.com/darmawan01/storage/registry"
)

// Global registry
var Registry *registry.Registry

// New creates a new storage client with the given configuration
func New(config *config.StorageConfig) error {
	Registry = registry.NewRegistry()

	return Registry.Initialize(*config)
}

// NewWithHandlers creates a new storage client with pre-configured handlers
func NewWithHandlers(config config.StorageConfig, handlers map[string]*handler.HandlerConfig) (*registry.Registry, error) {
	// Create registry
	registry := registry.NewRegistry()

	// Initialize with configuration
	if err := registry.Initialize(config); err != nil {
		return nil, fmt.Errorf("failed to initialize storage registry: %w", err)
	}

	// Register handlers
	for name, handlerConfig := range handlers {
		_, err := registry.Register(name, handlerConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to register handler %s: %w", name, err)
		}
	}

	return registry, nil
}
