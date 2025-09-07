package middleware

import (
	"context"
	"io"
)

// Middleware defines the interface for storage middlewares
type Middleware interface {
	Name() string
	Process(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error)
}

// MiddlewareFunc represents the next middleware in the chain
type MiddlewareFunc func(ctx context.Context, req *StorageRequest) (*StorageResponse, error)

// StorageRequest represents a request flowing through the middleware chain
type StorageRequest struct {
	Operation   string                 `json:"operation"` // upload, download, delete, preview
	FileKey     string                 `json:"file_key"`
	FileName    string                 `json:"file_name"`
	FileData    io.Reader              `json:"-"`
	FileSize    int64                  `json:"file_size"`
	ContentType string                 `json:"content_type"`
	Category    string                 `json:"category"`
	EntityType  string                 `json:"entity_type"`
	EntityID    string                 `json:"entity_id"`
	UserID      string                 `json:"user_id"`
	Metadata    map[string]interface{} `json:"metadata"`
	Config      map[string]interface{} `json:"config"`
}

// StorageResponse represents a response from the middleware chain
type StorageResponse struct {
	Success     bool                   `json:"success"`
	FileKey     string                 `json:"file_key"`
	FileData    io.Reader              `json:"-"`
	FileURL     string                 `json:"file_url,omitempty"`
	FileSize    int64                  `json:"file_size"`
	ContentType string                 `json:"content_type"`
	Metadata    map[string]interface{} `json:"metadata"`
	Thumbnails  []ThumbnailInfo        `json:"thumbnails,omitempty"`
	Error       error                  `json:"error,omitempty"`
}

// ThumbnailInfo represents thumbnail information
type ThumbnailInfo struct {
	Size     string `json:"size"` // e.g., "150x150"
	URL      string `json:"url"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int64  `json:"file_size"`
}

// MiddlewareType represents different types of middlewares
type MiddlewareType string

const (
	SecurityMiddlewareType   MiddlewareType = "security"
	ThumbnailMiddlewareType  MiddlewareType = "thumbnail"
	EncryptionMiddlewareType MiddlewareType = "encryption"
	AuditMiddlewareType      MiddlewareType = "audit"
	CDNMiddlewareType        MiddlewareType = "cdn"
	ValidationMiddlewareType MiddlewareType = "validation"
)

// MiddlewareConfig represents configuration for a middleware
type MiddlewareConfig struct {
	Name    string                 `json:"name"`
	Enabled bool                   `json:"enabled"`
	Config  map[string]interface{} `json:"config"`
}

// MiddlewareChain represents a chain of middlewares
type MiddlewareChain struct {
	middlewares []Middleware
}

// NewMiddlewareChain creates a new middleware chain
func NewMiddlewareChain() *MiddlewareChain {
	return &MiddlewareChain{
		middlewares: make([]Middleware, 0),
	}
}

// Add adds a middleware to the chain
func (c *MiddlewareChain) Add(middleware Middleware) {
	c.middlewares = append(c.middlewares, middleware)
}

// Process processes a request through the middleware chain
func (c *MiddlewareChain) Process(ctx context.Context, req *StorageRequest) (*StorageResponse, error) {
	if len(c.middlewares) == 0 {
		return &StorageResponse{Success: true}, nil
	}

	// Create the chain by building the next functions
	var next MiddlewareFunc = func(ctx context.Context, req *StorageRequest) (*StorageResponse, error) {
		return &StorageResponse{Success: true}, nil
	}

	// Build the chain in reverse order
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		current := c.middlewares[i]
		nextFunc := next
		next = func(ctx context.Context, req *StorageRequest) (*StorageResponse, error) {
			return current.Process(ctx, req, nextFunc)
		}
	}

	return next(ctx, req)
}

// GetMiddlewareNames returns the names of all middlewares in the chain
func (c *MiddlewareChain) GetMiddlewareNames() []string {
	names := make([]string, len(c.middlewares))
	for i, middleware := range c.middlewares {
		names[i] = middleware.Name()
	}
	return names
}
