package storage

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/darmawan01/storage/middleware"
	"github.com/minio/minio-go/v7"
)

// Handler represents a storage handler for a specific service/namespace
type Handler struct {
	name        string
	config      *HandlerConfig
	client      *minio.Client
	registry    *Registry
	buckets     map[string]string                      // category -> bucket name
	middlewares map[string]*middleware.MiddlewareChain // category -> middleware chain
}

// initialize sets up the handler and creates necessary buckets
func (h *Handler) initialize() error {
	h.buckets = make(map[string]string)
	h.middlewares = make(map[string]*middleware.MiddlewareChain)

	// Create buckets for each category
	for category, categoryConfig := range h.config.Categories {
		bucketName := h.GetBucketName(category)
		h.buckets[category] = bucketName

		// Create bucket if it doesn't exist
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		exists, err := h.client.BucketExists(ctx, bucketName)
		if err != nil {
			return fmt.Errorf("failed to check bucket existence: %w", err)
		}

		if !exists {
			err = h.client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{
				Region: h.registry.config.Region,
			})
			if err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bucketName, err)
			}
		}

		// Set bucket policy based on category configuration
		if err := h.setBucketPolicy(ctx, bucketName, categoryConfig); err != nil {
			return fmt.Errorf("failed to set bucket policy for %s: %w", bucketName, err)
		}

		// Setup middlewares for this category
		if err := h.setupMiddlewares(category, categoryConfig); err != nil {
			return fmt.Errorf("failed to setup middlewares for category %s: %w", category, err)
		}
	}

	return nil
}

// GetBucketName generates the bucket name for a category
func (h *Handler) GetBucketName(category string) string {
	categoryConfig, exists := h.config.Categories[category]
	if !exists {
		return fmt.Sprintf("%s-%s", h.config.BasePath, category)
	}
	return fmt.Sprintf("%s-%s", h.config.BasePath, categoryConfig.BucketSuffix)
}

// GenerateFileKey creates a structured file key
func (h *Handler) GenerateFileKey(entityType, entityID, fileType, filename string) string {
	timestamp := time.Now().Unix()
	uuid := generateUUID()
	ext := filepath.Ext(filename)
	return fmt.Sprintf("%s/%s/%s/%s/%d_%s%s",
		h.config.BasePath, entityType, entityID, fileType, timestamp, uuid, ext)
}

// Upload uploads a file to the appropriate bucket
func (h *Handler) Upload(ctx context.Context, req *UploadRequest) (*UploadResponse, error) {
	// Get category configuration
	_, exists := h.config.Categories[req.Category]
	if !exists {
		return nil, &StorageError{Code: "CATEGORY_NOT_FOUND", Message: "Category " + req.Category + " not found"}
	}

	// Convert to middleware request
	middlewareReq := &middleware.StorageRequest{
		Operation:   "upload",
		FileName:    req.FileName,
		FileData:    req.FileData,
		FileSize:    req.FileSize,
		ContentType: req.ContentType,
		Category:    req.Category,
		EntityType:  req.EntityType,
		EntityID:    req.EntityID,
		UserID:      req.UserID,
		Metadata:    req.Metadata,
		Config:      req.Config,
	}

	// Get middleware chain for this category
	middlewareChain, exists := h.middlewares[req.Category]
	if !exists {
		return nil, fmt.Errorf("middleware chain not found for category %s", req.Category)
	}

	// Process through middleware chain
	middlewareResp, err := middlewareChain.Process(ctx, middlewareReq)
	if err != nil {
		return nil, fmt.Errorf("middleware processing failed: %w", err)
	}

	if !middlewareResp.Success {
		return &UploadResponse{
			Success: false,
			Error:   middlewareResp.Error,
		}, nil
	}

	// Generate file key
	fileKey := h.GenerateFileKey(req.EntityType, req.EntityID, req.Category, req.FileName)

	// Upload to MinIO
	bucketName := h.GetBucketName(req.Category)
	_, err = h.client.PutObject(ctx, bucketName, fileKey, req.FileData, req.FileSize, minio.PutObjectOptions{
		ContentType: req.ContentType,
		UserMetadata: map[string]string{
			"original-filename": req.FileName,
			"entity-type":       req.EntityType,
			"entity-id":         req.EntityID,
			"category":          req.Category,
			"uploaded-by":       req.UserID,
			"uploaded-at":       time.Now().Format(time.RFC3339),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	// Convert middleware thumbnails to storage thumbnails
	var thumbnails []ThumbnailInfo
	for _, thumb := range middlewareResp.Thumbnails {
		thumbnails = append(thumbnails, ThumbnailInfo{
			Size:     thumb.Size,
			URL:      thumb.URL,
			Width:    thumb.Width,
			Height:   thumb.Height,
			FileSize: thumb.FileSize,
		})
	}

	return &UploadResponse{
		Success:     true,
		FileKey:     fileKey,
		FileSize:    req.FileSize,
		ContentType: req.ContentType,
		Metadata:    req.Metadata,
		Thumbnails:  thumbnails,
	}, nil
}

// Download downloads a file from the appropriate bucket
func (h *Handler) Download(ctx context.Context, req *DownloadRequest) (*DownloadResponse, error) {
	// Find the file in buckets
	_, bucketName, err := h.findFile(ctx, req.FileKey)
	if err != nil {
		return nil, err
	}

	// Download from MinIO
	object, err := h.client.GetObject(ctx, bucketName, req.FileKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}

	// Get object info for proper metadata
	objInfo, err := object.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get object info: %w", err)
	}

	return &DownloadResponse{
		Success:     true,
		FileData:    object,
		FileSize:    objInfo.Size,
		ContentType: objInfo.ContentType,
		Metadata: map[string]interface{}{
			"file_name":    objInfo.Key,
			"uploaded_at":  objInfo.LastModified,
			"content_type": objInfo.ContentType,
		},
	}, nil
}

// Delete deletes a file from the appropriate bucket
func (h *Handler) Delete(ctx context.Context, req *DeleteRequest) error {
	// Find the file in buckets
	_, bucketName, err := h.findFile(ctx, req.FileKey)
	if err != nil {
		return err
	}

	// Delete from MinIO
	err = h.client.RemoveObject(ctx, bucketName, req.FileKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

// Preview generates a preview URL for a file
func (h *Handler) Preview(ctx context.Context, req *PreviewRequest) (*PreviewResponse, error) {
	// Find the file in buckets
	fileInfo, bucketName, err := h.findFile(ctx, req.FileKey)
	if err != nil {
		return nil, err
	}

	// Get object info for proper metadata
	objInfo := fileInfo.(*minio.ObjectInfo)

	// Generate presigned URL for preview (expires in 1 hour)
	previewURL, err := h.client.PresignedGetObject(ctx, bucketName, req.FileKey, time.Hour, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate preview URL: %w", err)
	}

	return &PreviewResponse{
		Success:     true,
		PreviewURL:  previewURL.String(),
		ContentType: objInfo.ContentType,
		FileSize:    objInfo.Size,
		Metadata: map[string]interface{}{
			"file_name":    objInfo.Key,
			"uploaded_at":  objInfo.LastModified,
			"content_type": objInfo.ContentType,
		},
	}, nil
}

// Thumbnail generates a thumbnail for a file
func (h *Handler) Thumbnail(ctx context.Context, req *ThumbnailRequest) (*ThumbnailResponse, error) {
	// Find the file in buckets
	fileInfo, bucketName, err := h.findFile(ctx, req.FileKey)
	if err != nil {
		return nil, err
	}

	// Get object info for proper metadata
	objInfo := fileInfo.(*minio.ObjectInfo)

	// Check if thumbnail generation is supported for this file type
	if !h.supportsThumbnail(objInfo.ContentType) {
		return nil, &StorageError{Code: "THUMBNAIL_NOT_SUPPORTED", Message: "Thumbnail generation not supported for this file type"}
	}

	// Generate presigned URL for thumbnail (expires in 1 hour)
	thumbnailURL, err := h.client.PresignedGetObject(ctx, bucketName, req.FileKey, time.Hour, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate thumbnail URL: %w", err)
	}

	return &ThumbnailResponse{
		Success:      true,
		ThumbnailURL: thumbnailURL.String(),
		Size:         req.Size,
		ContentType:  "image/jpeg", // Thumbnails are typically JPEG
		Metadata: map[string]interface{}{
			"original_file":  req.FileKey,
			"thumbnail_size": req.Size,
		},
	}, nil
}

// Stream streams a file from the appropriate bucket
func (h *Handler) Stream(ctx context.Context, req *StreamRequest) (*StreamResponse, error) {
	// Find the file in buckets
	fileInfo, bucketName, err := h.findFile(ctx, req.FileKey)
	if err != nil {
		return nil, err
	}

	// Get object info for proper metadata
	objInfo := fileInfo.(*minio.ObjectInfo)

	// Stream from MinIO
	opts := minio.GetObjectOptions{}
	if req.Range != "" {
		// Parse range header for partial content requests
		opts.SetRange(0, 0) // Placeholder - would parse actual range
	}

	object, err := h.client.GetObject(ctx, bucketName, req.FileKey, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to stream file: %w", err)
	}

	return &StreamResponse{
		Success:     true,
		FileData:    object,
		FileSize:    objInfo.Size,
		ContentType: objInfo.ContentType,
		Range:       req.Range,
		Metadata: map[string]interface{}{
			"file_name":    objInfo.Key,
			"uploaded_at":  objInfo.LastModified,
			"content_type": objInfo.ContentType,
		},
	}, nil
}

// GeneratePresignedURL generates a presigned URL for a file
func (h *Handler) GeneratePresignedURL(ctx context.Context, req *PresignedURLRequest) (*PresignedURLResponse, error) {
	// Find the file in buckets
	_, bucketName, err := h.findFile(ctx, req.FileKey)
	if err != nil {
		return nil, err
	}

	// Generate presigned URL based on action
	var url *url.URL
	switch req.Action {
	case "GET":
		url, err = h.client.PresignedGetObject(ctx, bucketName, req.FileKey, req.Expires, nil)
	case "PUT":
		url, err = h.client.PresignedPutObject(ctx, bucketName, req.FileKey, req.Expires)
	default:
		return nil, fmt.Errorf("unsupported action: %s", req.Action)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return &PresignedURLResponse{
		Success:   true,
		URL:       url.String(),
		ExpiresAt: time.Now().Add(req.Expires),
		Metadata: map[string]interface{}{
			"file_name":  req.FileKey,
			"action":     req.Action,
			"expires_at": time.Now().Add(req.Expires),
		},
	}, nil
}

// ListFiles lists files for a specific entity
func (h *Handler) ListFiles(ctx context.Context, req *ListRequest) (*ListResponse, error) {
	// This is a simplified implementation
	// In a real implementation, you would query a metadata database
	return &ListResponse{
		Success: true,
		Files:   []FileInfo{},
		Total:   0,
		Limit:   req.Limit,
		Offset:  req.Offset,
	}, nil
}

// GetFileInfo retrieves file information
func (h *Handler) GetFileInfo(ctx context.Context, req *InfoRequest) (*FileInfo, error) {
	// Find the file in buckets
	fileInfo, bucketName, err := h.findFile(ctx, req.FileKey)
	if err != nil {
		return nil, err
	}

	// Get object info for proper metadata
	objInfo := fileInfo.(*minio.ObjectInfo)

	// Convert to FileInfo
	return &FileInfo{
		ID:          generateUUID(),
		FileName:    objInfo.Key,
		FileKey:     objInfo.Key,
		FileSize:    objInfo.Size,
		ContentType: objInfo.ContentType,
		UploadedAt:  objInfo.LastModified,
		IsPublic:    h.isPublicFile(bucketName),
		Metadata: map[string]interface{}{
			"bucket_name": bucketName,
			"uploaded_at": objInfo.LastModified,
			"etag":        objInfo.ETag,
		},
	}, nil
}

// UpdateMetadata updates file metadata
func (h *Handler) UpdateMetadata(ctx context.Context, req *UpdateMetadataRequest) error {
	// Find the file in buckets
	_, bucketName, err := h.findFile(ctx, req.FileKey)
	if err != nil {
		return err
	}

	// Update metadata in MinIO
	// Note: This is a simplified implementation
	// In a real implementation, you would update metadata in a database
	_ = bucketName // Suppress unused variable warning

	return nil
}

// Helper methods

func (h *Handler) findFile(ctx context.Context, fileKey string) (interface{}, string, error) {
	// Search through all buckets for the file
	for _, bucketName := range h.buckets {
		object, err := h.client.StatObject(ctx, bucketName, fileKey, minio.StatObjectOptions{})
		if err == nil {
			return &object, bucketName, nil
		}
		// Continue searching if file not found in this bucket
	}

	return nil, "", &StorageError{Code: "FILE_NOT_FOUND", Message: "File not found"}
}

func (h *Handler) isPublicFile(bucketName string) bool {
	// Check if the bucket is configured as public
	for _, categoryConfig := range h.config.Categories {
		if h.GetBucketName(categoryConfig.BucketSuffix) == bucketName {
			return categoryConfig.IsPublic
		}
	}
	return false
}

func (h *Handler) supportsThumbnail(contentType string) bool {
	thumbnailTypes := []string{"image/jpeg", "image/png", "image/gif", "image/webp"}
	for _, t := range thumbnailTypes {
		if strings.HasPrefix(contentType, t) {
			return true
		}
	}
	return false
}

func (h *Handler) setBucketPolicy(ctx context.Context, bucketName string, categoryConfig CategoryConfig) error {
	if categoryConfig.IsPublic {
		// Set public read policy
		policy := `{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Principal": "*",
					"Action": "s3:GetObject",
					"Resource": "arn:aws:s3:::` + bucketName + `/*"
				}
			]
		}`
		return h.client.SetBucketPolicy(ctx, bucketName, policy)
	}
	// For private buckets, no policy is set (default private)
	return nil
}

func (h *Handler) HealthCheck(ctx context.Context) error {
	// Check if all required buckets exist
	for category, bucketName := range h.buckets {
		exists, err := h.client.BucketExists(ctx, bucketName)
		if err != nil {
			return fmt.Errorf("failed to check bucket %s: %w", bucketName, err)
		}
		if !exists {
			return fmt.Errorf("bucket %s for category %s does not exist", bucketName, category)
		}
	}
	return nil
}

func (h *Handler) Close() error {
	// Cleanup resources if needed
	return nil
}

// setupMiddlewares sets up middleware chains for a category
func (h *Handler) setupMiddlewares(category string, categoryConfig CategoryConfig) error {
	chain := middleware.NewMiddlewareChain()

	// Get middleware names for this category
	middlewareNames := categoryConfig.Middlewares
	if len(middlewareNames) == 0 {
		// Use default middlewares from handler config
		middlewareNames = h.config.Middlewares
	}

	// Add middlewares to chain
	for _, middlewareName := range middlewareNames {
		middleware, err := h.createMiddleware(middlewareName, category, categoryConfig)
		if err != nil {
			return fmt.Errorf("failed to create middleware %s: %w", middlewareName, err)
		}
		chain.Add(middleware)
	}

	h.middlewares[category] = chain
	return nil
}

// createMiddleware creates a middleware instance
func (h *Handler) createMiddleware(name, category string, categoryConfig CategoryConfig) (middleware.Middleware, error) {
	switch name {
	case "security":
		securityConfig := categoryConfig.Security
		if securityConfig.RequireAuth == false && securityConfig.RequireOwner == false {
			// Use handler default security config
			securityConfig = h.config.Security
		}
		return middleware.NewSecurityMiddleware(securityConfig, h.client), nil

	case "validation":
		validationConfig := categoryConfig.Validation
		return middleware.NewValidationMiddleware(validationConfig), nil

	case "thumbnail":
		previewConfig := categoryConfig.Preview
		if !previewConfig.GenerateThumbnails {
			// Use handler default preview config
			previewConfig = h.config.Preview
		}
		thumbnailConfig := middleware.ThumbnailConfig{
			GenerateThumbnails: previewConfig.GenerateThumbnails,
			ThumbnailSizes:     previewConfig.ThumbnailSizes,
			ThumbnailBucket:    h.GetBucketName("thumbnail"),
			ThumbnailPrefix:    "thumbnails",
		}
		return middleware.NewThumbnailMiddleware(thumbnailConfig, h.client), nil

	case "encryption":
		securityConfig := categoryConfig.Security
		if !securityConfig.EncryptAtRest {
			// Use handler default security config
			securityConfig = h.config.Security
		}
		encryptionConfig := middleware.EncryptionConfig{
			Enabled:       securityConfig.EncryptAtRest,
			Algorithm:     "AES-256-GCM",
			KeySource:     "env",
			EncryptAtRest: securityConfig.EncryptAtRest,
		}
		return middleware.NewEncryptionMiddleware(encryptionConfig), nil

	case "audit":
		auditConfig := middleware.AuditConfig{
			Enabled:     true,
			LogLevel:    "info",
			LogFormat:   "json",
			Operations:  []string{"upload", "download", "delete", "preview"},
			Fields:      []string{"user_id", "file_key", "operation", "timestamp", "success"},
			Destination: "stdout",
		}
		return middleware.NewAuditMiddleware(auditConfig, nil), nil

	case "cdn":
		previewConfig := categoryConfig.Preview
		if !previewConfig.UseCDN {
			// Use handler default preview config
			previewConfig = h.config.Preview
		}
		cdnConfig := middleware.CDNConfig{
			Enabled:     previewConfig.UseCDN,
			CDNEndpoint: previewConfig.CDNEndpoint,
			CDNProvider: "custom",
			CacheTTL:    3600, // 1 hour
		}
		return middleware.NewCDNMiddleware(cdnConfig), nil

	default:
		return nil, fmt.Errorf("unknown middleware: %s", name)
	}
}

// Utility functions

func generateUUID() string {
	// Simple UUID generation - in production, use a proper UUID library
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
