package handler

import (
	"time"

	"github.com/darmawan01/storage/category"
	"github.com/darmawan01/storage/errors"
	"github.com/darmawan01/storage/interfaces"
	"github.com/darmawan01/storage/middleware"
)

// HandlerConfig represents handler-specific configuration
type HandlerConfig struct {
	BasePath    string                             `json:"base_path"`
	Middlewares []string                           `json:"middlewares"` // Default middlewares for all categories
	Categories  map[string]category.CategoryConfig `json:"categories"`
	Security    middleware.SecurityConfig          `json:"security,omitempty"`
	Preview     category.PreviewConfig             `json:"preview,omitempty"`
	// MetadataCallback provides a callback for storing file metadata after upload
	// If not provided, metadata will only be stored in MinIO object metadata
	MetadataCallback interfaces.MetadataCallback `json:"-"`
}

func DefaultHandlerConfig(basePath string) HandlerConfig {
	return HandlerConfig{
		BasePath:   basePath,
		Categories: make(map[string]category.CategoryConfig),
		Security: middleware.SecurityConfig{
			RequireAuth:        true,
			PresignedURLExpiry: 24 * time.Hour,
			MaxDownloadCount:   100,
		},
		Preview: category.PreviewConfig{
			GenerateThumbnails: true,
			ThumbnailSizes:     []string{"150x150", "300x300", "600x600"},
			EnablePreview:      true,
			PreviewFormats:     []string{"image", "pdf"},
		},
	}
}

func (c *HandlerConfig) Validate() error {
	if c.BasePath == "" {
		return &errors.StorageError{Code: "INVALID_CONFIG", Message: "BasePath is required"}
	}
	if len(c.Categories) == 0 {
		return &errors.StorageError{Code: "INVALID_CONFIG", Message: "At least one category must be defined"}
	}

	for name, category := range c.Categories {
		if err := category.Validate(); err != nil {
			return &errors.StorageError{Code: "INVALID_CONFIG", Message: "Category " + name + " is invalid: " + err.Error()}
		}
	}

	return nil
}
