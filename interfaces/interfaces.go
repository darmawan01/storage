package interfaces

import (
	"context"
	"io"
	"time"
)

// StorageClient defines the main interface for storage operations
type StorageClient interface {
	// Basic operations
	Upload(ctx context.Context, req *UploadRequest) (*UploadResponse, error)
	Download(ctx context.Context, req *DownloadRequest) (*DownloadResponse, error)
	Delete(ctx context.Context, req *DeleteRequest) error

	// Preview operations
	Preview(ctx context.Context, req *PreviewRequest) (*PreviewResponse, error)
	Thumbnail(ctx context.Context, req *ThumbnailRequest) (*ThumbnailResponse, error)
	Stream(ctx context.Context, req *StreamRequest) (*StreamResponse, error)

	// Security operations
	GeneratePresignedURL(ctx context.Context, req *PresignedURLRequest) (*PresignedURLResponse, error)

	// Management operations
	ListFiles(ctx context.Context, req *ListRequest) (*ListResponse, error)
	GetFileInfo(ctx context.Context, req *InfoRequest) (*FileInfo, error)
	UpdateMetadata(ctx context.Context, req *UpdateMetadataRequest) error
}

// MetadataCallback defines a simple callback for storing file metadata after upload
// This allows users to store metadata in their preferred storage system (database, Redis, etc.)
type MetadataCallback func(ctx context.Context, metadata *FileMetadata) error

// Request/Response structures
type UploadRequest struct {
	FileData    io.Reader              `json:"-"`
	FileSize    int64                  `json:"file_size"`
	ContentType string                 `json:"content_type"`
	FileName    string                 `json:"file_name"`
	Category    string                 `json:"category"`
	EntityType  string                 `json:"entity_type"`
	EntityID    string                 `json:"entity_id"`
	UserID      string                 `json:"user_id"`
	Metadata    map[string]interface{} `json:"metadata"`
	Config      map[string]interface{} `json:"config"`
}

type UploadResponse struct {
	Success     bool                   `json:"success"`
	FileKey     string                 `json:"file_key"`
	FileURL     string                 `json:"file_url,omitempty"`
	FileSize    int64                  `json:"file_size"`
	ContentType string                 `json:"content_type"`
	Metadata    map[string]interface{} `json:"metadata"`
	Thumbnails  []ThumbnailInfo        `json:"thumbnails,omitempty"`
	Error       error                  `json:"error,omitempty"`
}

type DownloadRequest struct {
	FileKey string `json:"file_key"`
	UserID  string `json:"user_id"`
}

type DownloadResponse struct {
	Success     bool                   `json:"success"`
	FileData    io.Reader              `json:"-"`
	FileSize    int64                  `json:"file_size"`
	ContentType string                 `json:"content_type"`
	Metadata    map[string]interface{} `json:"metadata"`
	Error       error                  `json:"error,omitempty"`
}

type DeleteRequest struct {
	FileKey string `json:"file_key"`
	UserID  string `json:"user_id"`
}

type PreviewRequest struct {
	FileKey string `json:"file_key"`
	UserID  string `json:"user_id"`
	Size    string `json:"size,omitempty"` // e.g., "300x300"
}

type PreviewResponse struct {
	Success     bool                   `json:"success"`
	PreviewURL  string                 `json:"preview_url"`
	ContentType string                 `json:"content_type"`
	FileSize    int64                  `json:"file_size"`
	Metadata    map[string]interface{} `json:"metadata"`
	Error       error                  `json:"error,omitempty"`
}

type ThumbnailRequest struct {
	FileKey string `json:"file_key"`
	UserID  string `json:"user_id"`
	Size    string `json:"size"` // e.g., "150x150", "300x300"
}

type ThumbnailResponse struct {
	Success      bool                   `json:"success"`
	ThumbnailURL string                 `json:"thumbnail_url"`
	Size         string                 `json:"size"`
	ContentType  string                 `json:"content_type"`
	Metadata     map[string]interface{} `json:"metadata"`
	Error        error                  `json:"error,omitempty"`
}

type StreamRequest struct {
	FileKey string `json:"file_key"`
	UserID  string `json:"user_id"`
	Range   string `json:"range,omitempty"` // HTTP Range header
}

type StreamResponse struct {
	Success     bool                   `json:"success"`
	FileData    io.Reader              `json:"-"`
	FileSize    int64                  `json:"file_size"`
	ContentType string                 `json:"content_type"`
	Range       string                 `json:"range,omitempty"`
	Metadata    map[string]interface{} `json:"metadata"`
	Error       error                  `json:"error,omitempty"`
}

type PresignedURLRequest struct {
	FileKey string        `json:"file_key"`
	UserID  string        `json:"user_id"`
	Expires time.Duration `json:"expires"`
	Action  string        `json:"action"` // "GET", "PUT", "DELETE"
}

type PresignedURLResponse struct {
	Success   bool                   `json:"success"`
	URL       string                 `json:"url"`
	ExpiresAt time.Time              `json:"expires_at"`
	Metadata  map[string]interface{} `json:"metadata"`
	Error     error                  `json:"error,omitempty"`
}

type ListRequest struct {
	EntityType string            `json:"entity_type"`
	EntityID   string            `json:"entity_id"`
	Category   string            `json:"category,omitempty"`
	UserID     string            `json:"user_id"`
	Filters    map[string]string `json:"filters,omitempty"`
	Limit      int               `json:"limit,omitempty"`
	Offset     int               `json:"offset,omitempty"`
}

type ListResponse struct {
	Success bool       `json:"success"`
	Files   []FileInfo `json:"files"`
	Total   int        `json:"total"`
	Limit   int        `json:"limit"`
	Offset  int        `json:"offset"`
	Error   error      `json:"error,omitempty"`
}

type InfoRequest struct {
	FileKey string `json:"file_key"`
	UserID  string `json:"user_id"`
}

type UpdateMetadataRequest struct {
	FileKey  string                 `json:"file_key"`
	UserID   string                 `json:"user_id"`
	Metadata map[string]interface{} `json:"metadata"`
}

// File metadata structure
type FileMetadata struct {
	ID          string          `json:"id"`
	FileName    string          `json:"file_name"`
	FileKey     string          `json:"file_key"`
	FileSize    int64           `json:"file_size"`
	ContentType string          `json:"content_type"`
	Category    FileCategory    `json:"category"`
	Namespace   string          `json:"namespace"`
	EntityType  string          `json:"entity_type"`
	EntityID    string          `json:"entity_id"`
	UploadedBy  string          `json:"uploaded_by"`
	UploadedAt  time.Time       `json:"uploaded_at"`
	IsPublic    bool            `json:"is_public"`
	Tags        []string        `json:"tags"`
	Thumbnails  []ThumbnailInfo `json:"thumbnails"`
	Version     int             `json:"version"`
	Checksum    string          `json:"checksum"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
}

type FileInfo struct {
	ID          string                 `json:"id"`
	FileName    string                 `json:"file_name"`
	FileKey     string                 `json:"file_key"`
	FileSize    int64                  `json:"file_size"`
	ContentType string                 `json:"content_type"`
	Category    string                 `json:"category"`
	EntityType  string                 `json:"entity_type"`
	EntityID    string                 `json:"entity_id"`
	UploadedBy  string                 `json:"uploaded_by"`
	UploadedAt  time.Time              `json:"uploaded_at"`
	IsPublic    bool                   `json:"is_public"`
	Thumbnails  []ThumbnailInfo        `json:"thumbnails"`
	URL         string                 `json:"url,omitempty"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type FileCategory string

const (
	CategoryProfile    FileCategory = "profile"
	CategoryDocument   FileCategory = "document"
	CategoryAttachment FileCategory = "attachment"
	CategoryTemp       FileCategory = "temp"
	CategoryThumbnail  FileCategory = "thumbnail"
	CategoryPublic     FileCategory = "public"
	CategoryVehicle    FileCategory = "vehicle"
	CategoryReceipt    FileCategory = "receipt"
)

type ThumbnailInfo struct {
	Size     string `json:"size"` // e.g., "150x150"
	URL      string `json:"url"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int64  `json:"file_size"`
}

type User struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Roles    []string `json:"roles"`
}

// Batch operation types
type BatchFile struct {
	FileData    io.Reader              `json:"-"`
	FileName    string                 `json:"file_name"`
	ContentType string                 `json:"content_type"`
	FileSize    int64                  `json:"file_size"`
	Category    string                 `json:"category"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type BatchUploadRequest struct {
	Files  []BatchFile `json:"files"`
	UserID string      `json:"user_id"`
}

type BatchUploadResponse struct {
	Success      bool              `json:"success"`
	Results      []*UploadResponse `json:"results"`
	SuccessCount int               `json:"success_count"`
	TotalCount   int               `json:"total_count"`
	Error        error             `json:"error,omitempty"`
}

type BatchDeleteRequest struct {
	FileKeys []string `json:"file_keys"`
	UserID   string   `json:"user_id"`
}

type BatchDeleteResponse struct {
	Success      bool              `json:"success"`
	Results      []*DeleteResponse `json:"results"`
	SuccessCount int               `json:"success_count"`
	TotalCount   int               `json:"total_count"`
	Error        error             `json:"error,omitempty"`
}

type BatchGetRequest struct {
	FileKeys []string `json:"file_keys"`
	UserID   string   `json:"user_id"`
}

type BatchGetResponse struct {
	Success      bool                `json:"success"`
	Results      []*DownloadResponse `json:"results"`
	SuccessCount int                 `json:"success_count"`
	TotalCount   int                 `json:"total_count"`
	Error        error               `json:"error,omitempty"`
}

// DeleteResponse represents a delete response
type DeleteResponse struct {
	Success bool  `json:"success"`
	Error   error `json:"error,omitempty"`
}
