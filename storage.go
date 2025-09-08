package storage

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/darmawan01/storage/config"
	"github.com/darmawan01/storage/handler"
	"github.com/darmawan01/storage/interfaces"
	"github.com/darmawan01/storage/registry"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

// New creates a new storage client with the given configuration
func New(config *config.StorageConfig) (*registry.Registry, error) {
	// Create registry
	registry := registry.NewRegistry()

	// Initialize with configuration
	if err := registry.Initialize(*config); err != nil {
		return nil, fmt.Errorf("failed to initialize storage registry: %w", err)
	}

	// Return the registry
	return registry, nil
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

// NewHandler creates a new storage handler for a specific service
func NewHandler(name string, config *handler.HandlerConfig, client interface{}) (*handler.Handler, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	handler := &handler.Handler{
		Name:   name,
		Config: config,
		Client: client.(*minio.Client),
	}

	// Initialize handler
	if err := handler.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize handler: %w", err)
	}

	return handler, nil
}

// Utility functions

// IsImageType checks if a content type is an image
func IsImageType(contentType string) bool {
	imageTypes := []string{
		"image/jpeg", "image/jpg", "image/png", "image/gif",
		"image/webp", "image/bmp", "image/tiff", "image/svg+xml",
	}
	for _, t := range imageTypes {
		if contentType == t {
			return true
		}
	}
	return false
}

// IsVideoType checks if a content type is a video
func IsVideoType(contentType string) bool {
	videoTypes := []string{
		"video/mp4", "video/webm", "video/avi", "video/mov",
		"video/wmv", "video/flv", "video/3gp", "video/quicktime",
	}
	for _, t := range videoTypes {
		if contentType == t {
			return true
		}
	}
	return false
}

// IsAudioType checks if a content type is audio
func IsAudioType(contentType string) bool {
	audioTypes := []string{
		"audio/mpeg", "audio/mp3", "audio/wav", "audio/ogg",
		"audio/aac", "audio/flac", "audio/m4a",
	}
	for _, t := range audioTypes {
		if contentType == t {
			return true
		}
	}
	return false
}

// IsDocumentType checks if a content type is a document
func IsDocumentType(contentType string) bool {
	documentTypes := []string{
		"application/pdf", "application/msword", "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.ms-excel", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.ms-powerpoint", "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"text/plain", "text/csv", "application/rtf",
	}
	for _, t := range documentTypes {
		if contentType == t {
			return true
		}
	}
	return false
}

// GetFileCategory determines the file category based on content type
func GetFileCategory(contentType string) interfaces.FileCategory {
	if IsImageType(contentType) {
		return interfaces.CategoryProfile
	}
	if IsDocumentType(contentType) {
		return interfaces.CategoryDocument
	}
	if IsVideoType(contentType) || IsAudioType(contentType) {
		return interfaces.CategoryAttachment
	}
	return interfaces.CategoryAttachment // Default category
}

// FormatFileSize formats a file size in bytes to human readable format
func FormatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ParseThumbnailSize parses a thumbnail size string (e.g., "150x150")
func ParseThumbnailSize(size string) (width, height int, err error) {
	_, err = fmt.Sscanf(size, "%dx%d", &width, &height)
	return
}

// ValidateThumbnailSize validates a thumbnail size string
func ValidateThumbnailSize(size string) error {
	width, height, err := ParseThumbnailSize(size)
	if err != nil {
		return fmt.Errorf("invalid thumbnail size format: %s", size)
	}
	if width <= 0 || height <= 0 {
		return fmt.Errorf("thumbnail dimensions must be positive: %s", size)
	}
	if width > 4096 || height > 4096 {
		return fmt.Errorf("thumbnail dimensions too large: %s", size)
	}
	return nil
}

// GenerateFileKey generates a structured file key
func GenerateFileKey(basePath, entityType, entityID, category, filename string) string {
	timestamp := time.Now().Unix()

	ext := filepath.Ext(filename)
	return fmt.Sprintf("%s/%s/%s/%s/%d_%s%s",
		basePath, entityType, entityID, category, timestamp, uuid.New().String(), ext)
}

// ExtractFileInfo extracts file information from a file key
func ExtractFileInfo(fileKey string) (basePath, entityType, entityID, category, filename string, err error) {
	parts := strings.Split(fileKey, "/")
	if len(parts) < 5 {
		return "", "", "", "", "", fmt.Errorf("invalid file key format")
	}

	basePath = parts[0]
	entityType = parts[1]
	entityID = parts[2]
	category = parts[3]
	filename = parts[4]

	return
}

// Constants
const (
	// Default thumbnail sizes
	DefaultThumbnailSizes = "150x150,300x300,600x600"

	// Default file size limits
	DefaultMaxFileSize = 25 * 1024 * 1024 // 25MB
	DefaultMinFileSize = 1024             // 1KB

	// Default timeouts
	DefaultUploadTimeout   = 300 // 5 minutes
	DefaultDownloadTimeout = 60  // 1 minute

	// Default presigned URL expiry
	DefaultPresignedURLExpiry = 24 * time.Hour
)

// Version information
const (
	Version = "1.0.0"
	Build   = "2024-01-01"
)
