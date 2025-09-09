package handler

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/darmawan01/storage/category"
	"github.com/darmawan01/storage/errors"
	"github.com/darmawan01/storage/interfaces"
	"github.com/darmawan01/storage/middleware"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

// Handler represents a storage handler for a specific service/namespace
type Handler struct {
	Name string

	Config      *HandlerConfig
	Client      *minio.Client
	BucketName  string                                 // Global bucket name from registry config
	Categories  map[string]string                      // category -> bucket name (now all use same bucket)
	Middlewares map[string]*middleware.MiddlewareChain // category -> middleware chain
}

// initialize sets up the handler and creates necessary buckets
func (h *Handler) Initialize() error {
	h.Categories = make(map[string]string)
	h.Middlewares = make(map[string]*middleware.MiddlewareChain)

	// All categories now use the same bucket
	for category, categoryConfig := range h.Config.Categories {
		h.Categories[category] = h.BucketName

		// Setup middlewares for this category
		if err := h.setupMiddlewares(category, categoryConfig); err != nil {
			return fmt.Errorf("failed to setup middlewares for category %s: %w", category, err)
		}
	}

	return nil
}

// GenerateFileKey creates a structured file key
func (h *Handler) GenerateFileKey(entityType, entityID, fileType, filename string) string {
	timestamp := time.Now().Unix()

	ext := filepath.Ext(filename)
	return fmt.Sprintf("%s/%s/%s/%d_%s%s",
		entityType, entityID, fileType, timestamp, uuid.NewString(), ext)
}

// Upload uploads a file to the appropriate bucket
func (h *Handler) Upload(ctx context.Context, req *interfaces.UploadRequest) (*interfaces.UploadResponse, error) {
	// Get category configuration
	_, exists := h.Config.Categories[req.Category]
	if !exists {
		return nil, &errors.StorageError{Code: "CATEGORY_NOT_FOUND", Message: "Category " + req.Category + " not found"}
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
	middlewareChain, exists := h.Middlewares[req.Category]
	if !exists {
		return nil, fmt.Errorf("middleware chain not found for category %s", req.Category)
	}

	// Generate file key first
	fileKey := h.GenerateFileKey(req.EntityType, req.EntityID, req.Category, req.FileName)

	// Set the file key in the middleware request
	middlewareReq.FileKey = fileKey

	// Process through middleware chain
	middlewareResp, err := middlewareChain.Process(ctx, middlewareReq)
	if err != nil {
		return nil, fmt.Errorf("middleware processing failed: %w", err)
	}

	if !middlewareResp.Success {
		return &interfaces.UploadResponse{
			Success: false,
			Error:   middlewareResp.Error,
		}, nil
	}

	// Upload to MinIO
	_, err = h.Client.PutObject(ctx, h.BucketName, fileKey, req.FileData, req.FileSize, minio.PutObjectOptions{
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
	var thumbnails []interfaces.ThumbnailInfo
	for _, thumb := range middlewareResp.Thumbnails {
		thumbnails = append(thumbnails, interfaces.ThumbnailInfo{
			Size:     thumb.Size,
			URL:      thumb.URL,
			Width:    thumb.Width,
			Height:   thumb.Height,
			FileSize: thumb.FileSize,
		})
	}

	// Create file metadata for callback
	fileMetadata := &interfaces.FileMetadata{
		ID:          uuid.NewString(),
		FileName:    req.FileName,
		FileKey:     fileKey,
		FileSize:    req.FileSize,
		ContentType: req.ContentType,
		EntityType:  req.EntityType,
		EntityID:    req.EntityID,
		UploadedBy:  req.UserID,
		UploadedAt:  time.Now(),
		Thumbnails:  thumbnails,
		Version:     1,
		Checksum:    "", // Could be calculated if needed
	}

	// Call metadata callback if provided
	if h.Config.MetadataCallback != nil {
		if err := h.Config.MetadataCallback(ctx, fileMetadata); err != nil {
			// Log error but don't fail the upload
			// Users can handle this error in their callback implementation
			fmt.Printf("Warning: metadata callback failed: %v\n", err)
		}
	}

	return &interfaces.UploadResponse{
		Success:     true,
		FileKey:     fileKey,
		FileSize:    req.FileSize,
		ContentType: req.ContentType,
		Metadata:    req.Metadata,
		Thumbnails:  thumbnails,
	}, nil
}

// Download downloads a file from the appropriate bucket
func (h *Handler) Download(ctx context.Context, req *interfaces.DownloadRequest) (*interfaces.DownloadResponse, error) {
	// Find the file in buckets
	_, bucketName, err := h.findFile(ctx, req.FileKey)
	if err != nil {
		return nil, err
	}

	// Download from MinIO
	object, err := h.Client.GetObject(ctx, bucketName, req.FileKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}

	// Get object info for proper metadata
	objInfo, err := object.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get object info: %w", err)
	}

	return &interfaces.DownloadResponse{
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
func (h *Handler) Delete(ctx context.Context, req *interfaces.DeleteRequest) error {
	// Find the file in buckets
	_, bucketName, err := h.findFile(ctx, req.FileKey)
	if err != nil {
		return err
	}

	// Delete from MinIO
	err = h.Client.RemoveObject(ctx, bucketName, req.FileKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// Note: For metadata cleanup, users should implement their own cleanup logic
	// in their metadata storage system (database, Redis, etc.)
	// This library focuses only on MinIO operations

	return nil
}

// Preview generates a preview URL for a file
func (h *Handler) Preview(ctx context.Context, req *interfaces.PreviewRequest) (*interfaces.PreviewResponse, error) {
	// Find the file in buckets
	fileInfo, bucketName, err := h.findFile(ctx, req.FileKey)
	if err != nil {
		return nil, err
	}

	// Get object info for proper metadata
	objInfo := fileInfo.(*minio.ObjectInfo)

	// Generate presigned URL for preview (expires in 1 hour)
	previewURL, err := h.Client.PresignedGetObject(ctx, bucketName, req.FileKey, time.Hour, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate preview URL: %w", err)
	}

	return &interfaces.PreviewResponse{
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

// Stream streams a file from the appropriate bucket
func (h *Handler) Stream(ctx context.Context, req *interfaces.StreamRequest) (*interfaces.StreamResponse, error) {
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
		start, end, err := h.parseRangeHeader(req.Range, objInfo.Size)
		if err != nil {
			return nil, fmt.Errorf("invalid range header: %w", err)
		}
		opts.SetRange(start, end)
	}

	object, err := h.Client.GetObject(ctx, bucketName, req.FileKey, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to stream file: %w", err)
	}

	return &interfaces.StreamResponse{
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
func (h *Handler) GeneratePresignedURL(ctx context.Context, req *interfaces.PresignedURLRequest) (*interfaces.PresignedURLResponse, error) {
	// Find the file in buckets
	_, bucketName, err := h.findFile(ctx, req.FileKey)
	if err != nil {
		return nil, err
	}

	// Generate presigned URL based on action
	var url *url.URL
	switch req.Action {
	case "GET":
		url, err = h.Client.PresignedGetObject(ctx, bucketName, req.FileKey, req.Expires, nil)
	case "PUT":
		url, err = h.Client.PresignedPutObject(ctx, bucketName, req.FileKey, req.Expires)
	default:
		return nil, fmt.Errorf("unsupported action: %s", req.Action)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return &interfaces.PresignedURLResponse{
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
// Note: This is a simplified implementation that returns empty results
// For file listing, users should implement their own metadata storage and querying
func (h *Handler) ListFiles(ctx context.Context, req *interfaces.ListRequest) (*interfaces.ListResponse, error) {
	// This library focuses on MinIO operations only
	// File listing requires external metadata storage (database, Redis, etc.)
	return &interfaces.ListResponse{
		Success: true,
		Files:   []interfaces.FileInfo{},
		Total:   0,
		Limit:   req.Limit,
		Offset:  req.Offset,
	}, nil
}

// GetFileInfo retrieves file information from MinIO
func (h *Handler) GetFileInfo(ctx context.Context, req *interfaces.InfoRequest) (*interfaces.FileInfo, error) {
	// Find the file in buckets
	fileInfo, bucketName, err := h.findFile(ctx, req.FileKey)
	if err != nil {
		return nil, err
	}

	// Get object info for proper metadata
	objInfo := fileInfo.(*minio.ObjectInfo)

	// Convert to FileInfo
	return &interfaces.FileInfo{
		ID:          uuid.NewString(),
		FileName:    objInfo.Key,
		FileKey:     objInfo.Key,
		FileSize:    objInfo.Size,
		ContentType: objInfo.ContentType,
		UploadedAt:  objInfo.LastModified,
		Metadata: map[string]interface{}{
			"bucket_name": bucketName,
			"uploaded_at": objInfo.LastModified,
			"etag":        objInfo.ETag,
		},
	}, nil
}

// Helper methods

func (h *Handler) findFile(ctx context.Context, fileKey string) (interface{}, string, error) {
	// Since all categories use the same bucket, directly check that bucket
	object, err := h.Client.StatObject(ctx, h.BucketName, fileKey, minio.StatObjectOptions{})
	if err == nil {
		return &object, h.BucketName, nil
	}

	// Handle specific MinIO errors
	if minio.ToErrorResponse(err).Code == "NoSuchKey" {
		return nil, "", &errors.StorageError{Code: "FILE_NOT_FOUND", Message: "File not found"}
	}

	return nil, "", fmt.Errorf("failed to check file existence: %w", err)
}

func (h *Handler) HealthCheck(ctx context.Context) error {
	// Check if the global bucket exists
	exists, err := h.Client.BucketExists(ctx, h.BucketName)
	if err != nil {
		return fmt.Errorf("failed to check bucket %s: %w", h.BucketName, err)
	}
	if !exists {
		return fmt.Errorf("bucket %s does not exist", h.BucketName)
	}
	return nil
}

func (h *Handler) Close() error {
	// Cleanup resources if needed
	return nil
}

// setupMiddlewares sets up middleware chains for a category
func (h *Handler) setupMiddlewares(category string, categoryConfig category.CategoryConfig) error {
	chain := middleware.NewMiddlewareChain()

	// Get middleware names for this category
	middlewareNames := categoryConfig.Middlewares
	if len(middlewareNames) == 0 {
		// Use default middlewares from handler config
		middlewareNames = h.Config.Middlewares
	}

	// Add middlewares to chain
	for _, middlewareName := range middlewareNames {
		middleware, err := h.createMiddleware(middlewareName, category, categoryConfig)
		if err != nil {
			return fmt.Errorf("failed to create middleware %s: %w", middlewareName, err)
		}
		chain.Add(middleware)
	}

	h.Middlewares[category] = chain
	return nil
}

// createMiddleware creates a middleware instance
func (h *Handler) createMiddleware(name, category string, categoryConfig category.CategoryConfig) (middleware.Middleware, error) {
	switch name {
	case "security":
		securityConfig := categoryConfig.Security
		if !securityConfig.RequireAuth && !securityConfig.RequireOwner {
			// Use handler default security config
			securityConfig = h.Config.Security
		}

		return middleware.NewSecurityMiddleware(securityConfig, h.Client), nil

	case "validation":
		validationConfig := categoryConfig.Validation
		// Convert storage.ValidationConfig to middleware.ValidationConfig
		middlewareValidationConfig := middleware.ValidationConfig{
			MaxFileSize:       validationConfig.MaxFileSize,
			MinFileSize:       validationConfig.MinFileSize,
			AllowedTypes:      validationConfig.AllowedTypes,
			AllowedExtensions: validationConfig.AllowedExtensions,
		}

		if validationConfig.ImageValidation != nil {
			middlewareValidationConfig.ImageValidation = (*middleware.ImageValidationConfig)(validationConfig.ImageValidation)
		}

		if validationConfig.PDFValidation != nil {
			middlewareValidationConfig.PDFValidation = (*middleware.PDFValidationConfig)(validationConfig.PDFValidation)
		}

		if validationConfig.VideoValidation != nil {
			middlewareValidationConfig.VideoValidation = (*middleware.VideoValidationConfig)(validationConfig.VideoValidation)
		}

		if validationConfig.AudioValidation != nil {
			middlewareValidationConfig.AudioValidation = (*middleware.AudioValidationConfig)(validationConfig.AudioValidation)
		}

		return middleware.NewValidationMiddleware(middlewareValidationConfig), nil

	case "thumbnail":
		previewConfig := categoryConfig.Preview
		if !previewConfig.GenerateThumbnails {
			// Use handler default preview config
			previewConfig = h.Config.Preview
		}
		thumbnailConfig := middleware.ThumbnailConfig{
			GenerateThumbnails: previewConfig.GenerateThumbnails,
			ThumbnailSizes:     previewConfig.ThumbnailSizes,
			ThumbnailBucket:    h.BucketName, // Use the same bucket as original files
			ThumbnailPrefix:    "thumbnails",
			AsyncProcessing:    true, // Enable async processing by default
			AsyncConfig:        middleware.DefaultAsyncConfig(),
		}
		return middleware.NewThumbnailMiddleware(thumbnailConfig, h.Client), nil

	case "encryption":
		securityConfig := categoryConfig.Security
		if !securityConfig.EncryptAtRest {
			// Use handler default security config
			securityConfig = h.Config.Security
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
			previewConfig = h.Config.Preview
		}
		cdnConfig := middleware.CDNConfig{
			Enabled:     previewConfig.UseCDN,
			CDNEndpoint: previewConfig.CDNEndpoint,
			CDNProvider: "custom",
			CacheTTL:    3600, // 1 hour
		}
		return middleware.NewCDNMiddleware(cdnConfig), nil

	case "memory":
		memoryConfig := middleware.DefaultMemoryConfig()
		// Override with category-specific settings if available
		if categoryConfig.MaxSize > 0 {
			memoryConfig.MaxFileSize = categoryConfig.MaxSize
		}
		return middleware.NewMemoryMiddleware(memoryConfig), nil

	case "cache":
		cacheConfig := middleware.DefaultCacheConfig()
		return middleware.NewCacheMiddleware(cacheConfig), nil

	case "monitoring":
		monitoringConfig := middleware.DefaultMonitoringConfig()
		return middleware.NewMonitoringMiddleware(monitoringConfig), nil

	default:
		return nil, fmt.Errorf("unknown middleware: %s", name)
	}
}

// BatchUpload uploads multiple files in a single operation
func (h *Handler) BatchUpload(ctx context.Context, req *interfaces.BatchUploadRequest) (*interfaces.BatchUploadResponse, error) {
	if len(req.Files) == 0 {
		return &interfaces.BatchUploadResponse{
			Success: false,
			Error:   &errors.StorageError{Code: "INVALID_REQUEST", Message: "No files provided"},
		}, nil
	}

	// Limit batch size to prevent memory issues
	maxBatchSize := 10
	if len(req.Files) > maxBatchSize {
		return &interfaces.BatchUploadResponse{
			Success: false,
			Error:   &errors.StorageError{Code: "BATCH_SIZE_EXCEEDED", Message: fmt.Sprintf("Batch size %d exceeds maximum %d", len(req.Files), maxBatchSize)},
		}, nil
	}

	results := make([]*interfaces.UploadResponse, len(req.Files))
	successCount := 0

	// Process files concurrently
	type result struct {
		index int
		resp  *interfaces.UploadResponse
		err   error
	}

	resultChan := make(chan result, len(req.Files))

	for i, file := range req.Files {
		go func(index int, file interfaces.BatchFile) {
			uploadReq := &interfaces.UploadRequest{
				FileData:    file.FileData,
				FileSize:    file.FileSize,
				ContentType: file.ContentType,
				FileName:    file.FileName,
				Category:    file.Category,
				UserID:      req.UserID,
				Metadata:    file.Metadata,
			}

			resp, err := h.Upload(ctx, uploadReq)
			// resp is already an UploadResponse, no conversion needed
			resultChan <- result{index: index, resp: resp, err: err}
		}(i, file)
	}

	// Collect results
	for i := 0; i < len(req.Files); i++ {
		res := <-resultChan
		results[res.index] = res.resp
		if res.resp != nil && res.resp.Success {
			successCount++
		}
	}

	return &interfaces.BatchUploadResponse{
		Success:      successCount > 0,
		Results:      results,
		SuccessCount: successCount,
		TotalCount:   len(req.Files),
	}, nil
}

// BatchDelete deletes multiple files in a single operation
func (h *Handler) BatchDelete(ctx context.Context, req *interfaces.BatchDeleteRequest) (*interfaces.BatchDeleteResponse, error) {
	if len(req.FileKeys) == 0 {
		return &interfaces.BatchDeleteResponse{
			Success: false,
			Error:   &errors.StorageError{Code: "INVALID_REQUEST", Message: "No file keys provided"},
		}, nil
	}

	// Limit batch size to prevent memory issues
	maxBatchSize := 50
	if len(req.FileKeys) > maxBatchSize {
		return &interfaces.BatchDeleteResponse{
			Success: false,
			Error:   &errors.StorageError{Code: "BATCH_SIZE_EXCEEDED", Message: fmt.Sprintf("Batch size %d exceeds maximum %d", len(req.FileKeys), maxBatchSize)},
		}, nil
	}

	results := make([]*interfaces.DeleteResponse, len(req.FileKeys))
	successCount := 0

	// Process deletions concurrently
	type result struct {
		index int
		resp  *interfaces.DeleteResponse
		err   error
	}

	resultChan := make(chan result, len(req.FileKeys))

	for i, fileKey := range req.FileKeys {
		go func(index int, fileKey string) {
			deleteReq := &interfaces.DeleteRequest{
				FileKey: fileKey,
				UserID:  req.UserID,
			}

			err := h.Delete(ctx, deleteReq)
			// Convert error to DeleteResponse
			deleteResp := &interfaces.DeleteResponse{
				Success: err == nil,
				Error:   err,
			}
			resultChan <- result{index: index, resp: deleteResp, err: err}
		}(i, fileKey)
	}

	// Collect results
	for i := 0; i < len(req.FileKeys); i++ {
		res := <-resultChan
		results[res.index] = res.resp
		if res.resp != nil && res.resp.Success {
			successCount++
		}
	}

	return &interfaces.BatchDeleteResponse{
		Success:      successCount > 0,
		Results:      results,
		SuccessCount: successCount,
		TotalCount:   len(req.FileKeys),
	}, nil
}

// BatchGet retrieves multiple files in a single operation
func (h *Handler) BatchGet(ctx context.Context, req *interfaces.BatchGetRequest) (*interfaces.BatchGetResponse, error) {
	if len(req.FileKeys) == 0 {
		return &interfaces.BatchGetResponse{
			Success: false,
			Error:   &errors.StorageError{Code: "INVALID_REQUEST", Message: "No file keys provided"},
		}, nil
	}

	// Limit batch size to prevent memory issues
	maxBatchSize := 20
	if len(req.FileKeys) > maxBatchSize {
		return &interfaces.BatchGetResponse{
			Success: false,
			Error:   &errors.StorageError{Code: "BATCH_SIZE_EXCEEDED", Message: fmt.Sprintf("Batch size %d exceeds maximum %d", len(req.FileKeys), maxBatchSize)},
		}, nil
	}

	results := make([]*interfaces.DownloadResponse, len(req.FileKeys))
	successCount := 0

	// Process downloads concurrently
	type result struct {
		index int
		resp  *interfaces.DownloadResponse
		err   error
	}

	resultChan := make(chan result, len(req.FileKeys))

	for i, fileKey := range req.FileKeys {
		go func(index int, fileKey string) {
			downloadReq := &interfaces.DownloadRequest{
				FileKey: fileKey,
				UserID:  req.UserID,
			}

			resp, err := h.Download(ctx, downloadReq)
			// resp is already a DownloadResponse, no conversion needed
			resultChan <- result{index: index, resp: resp, err: err}
		}(i, fileKey)
	}

	// Collect results
	for i := 0; i < len(req.FileKeys); i++ {
		res := <-resultChan
		results[res.index] = res.resp
		if res.resp != nil && res.resp.Success {
			successCount++
		}
	}

	return &interfaces.BatchGetResponse{
		Success:      successCount > 0,
		Results:      results,
		SuccessCount: successCount,
		TotalCount:   len(req.FileKeys),
	}, nil
}

// parseRangeHeader parses HTTP Range header and returns start and end positions
func (h *Handler) parseRangeHeader(rangeHeader string, fileSize int64) (int64, int64, error) {
	// Remove "bytes=" prefix if present
	rangeStr := strings.TrimPrefix(rangeHeader, "bytes=")

	// Handle different range formats
	if strings.Contains(rangeStr, ",") {
		// Multiple ranges not supported, use first one
		ranges := strings.Split(rangeStr, ",")
		rangeStr = strings.TrimSpace(ranges[0])
	}

	// Parse range
	parts := strings.Split(rangeStr, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range format: %s", rangeHeader)
	}

	var start, end int64
	var err error

	// Parse start position
	if parts[0] != "" {
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid start position: %s", parts[0])
		}
	} else {
		// Suffix range: -500 means last 500 bytes
		suffix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid suffix range: %s", parts[1])
		}
		start = fileSize - suffix
		if start < 0 {
			start = 0
		}
	}

	// Parse end position
	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid end position: %s", parts[1])
		}
	} else {
		// Prefix range: 0- means from start to end
		end = fileSize - 1
	}

	// Validate range
	if start < 0 || end < 0 || start > end || start >= fileSize {
		return 0, 0, fmt.Errorf("invalid range: start=%d, end=%d, fileSize=%d", start, end, fileSize)
	}

	// Ensure end doesn't exceed file size
	if end >= fileSize {
		end = fileSize - 1
	}

	return start, end, nil
}
