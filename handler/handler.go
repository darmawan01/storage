package handler

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
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
	Name        string
	Region      string
	Config      *HandlerConfig
	Client      *minio.Client
	Buckets     map[string]string                      // category -> bucket name
	Middlewares map[string]*middleware.MiddlewareChain // category -> middleware chain
}

// initialize sets up the handler and creates necessary buckets
func (h *Handler) Initialize() error {
	h.Buckets = make(map[string]string)
	h.Middlewares = make(map[string]*middleware.MiddlewareChain)

	// Create buckets for each category
	for category, categoryConfig := range h.Config.Categories {
		bucketName := h.GetBucketName(category)
		h.Buckets[category] = bucketName

		// Create bucket if it doesn't exist
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		exists, err := h.Client.BucketExists(ctx, bucketName)
		if err != nil {
			return fmt.Errorf("failed to check bucket existence: %w", err)
		}

		if !exists {
			err = h.Client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{
				Region: h.Region,
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
	categoryConfig, exists := h.Config.Categories[category]
	if !exists {
		return fmt.Sprintf("%s-%s", h.Config.BasePath, category)
	}
	return fmt.Sprintf("%s-%s", h.Config.BasePath, categoryConfig.BucketSuffix)
}

// GenerateFileKey creates a structured file key
func (h *Handler) GenerateFileKey(entityType, entityID, fileType, filename string) string {
	timestamp := time.Now().Unix()

	ext := filepath.Ext(filename)
	return fmt.Sprintf("%s/%s/%s/%s/%d_%s%s",
		h.Config.BasePath, entityType, entityID, fileType, timestamp, uuid.NewString(), ext)
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

	// Generate file key
	fileKey := h.GenerateFileKey(req.EntityType, req.EntityID, req.Category, req.FileName)

	// Upload to MinIO
	bucketName := h.GetBucketName(req.Category)
	_, err = h.Client.PutObject(ctx, bucketName, fileKey, req.FileData, req.FileSize, minio.PutObjectOptions{
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
		Category:    interfaces.FileCategory(req.Category),
		Namespace:   h.Config.BasePath,
		EntityType:  req.EntityType,
		EntityID:    req.EntityID,
		UploadedBy:  req.UserID,
		UploadedAt:  time.Now(),
		IsPublic:    h.isPublicFile(bucketName),
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

// Thumbnail generates a thumbnail for a file
func (h *Handler) Thumbnail(ctx context.Context, req *interfaces.ThumbnailRequest) (*interfaces.ThumbnailResponse, error) {
	// Find the file in buckets
	fileInfo, bucketName, err := h.findFile(ctx, req.FileKey)
	if err != nil {
		return nil, err
	}

	// Get object info for proper metadata
	objInfo := fileInfo.(*minio.ObjectInfo)

	// Check if thumbnail generation is supported for this file type
	if !h.supportsThumbnail(objInfo.ContentType) {
		return nil, &errors.StorageError{Code: "THUMBNAIL_NOT_SUPPORTED", Message: "Thumbnail generation not supported for this file type"}
	}

	// Generate thumbnail key based on original file key and requested size
	thumbnailKey := h.generateThumbnailKey(req.FileKey, req.Size)

	// Check if thumbnail already exists
	thumbnailBucket := h.GetBucketName("thumbnail")
	exists, err := h.thumbnailExists(ctx, thumbnailBucket, thumbnailKey)
	if err != nil {
		return nil, fmt.Errorf("failed to check thumbnail existence: %w", err)
	}

	// If thumbnail doesn't exist, generate it
	if !exists {
		// Get the original file
		originalObject, err := h.Client.GetObject(ctx, bucketName, req.FileKey, minio.GetObjectOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get original file: %w", err)
		}
		defer originalObject.Close()

		// Generate thumbnail
		thumbnailData, err := h.generateThumbnailFromObject(originalObject, req.Size, objInfo.ContentType)
		if err != nil {
			return nil, fmt.Errorf("failed to generate thumbnail: %w", err)
		}

		// Upload thumbnail to storage
		_, err = h.Client.PutObject(ctx, thumbnailBucket, thumbnailKey, bytes.NewReader(thumbnailData), int64(len(thumbnailData)), minio.PutObjectOptions{
			ContentType: "image/jpeg",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to upload thumbnail: %w", err)
		}
	}

	// Generate presigned URL for thumbnail (expires in 24 hours)
	thumbnailURL, err := h.Client.PresignedGetObject(ctx, thumbnailBucket, thumbnailKey, 24*time.Hour, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate thumbnail URL: %w", err)
	}

	return &interfaces.ThumbnailResponse{
		Success:      true,
		ThumbnailURL: thumbnailURL.String(),
		Size:         req.Size,
		ContentType:  "image/jpeg", // Thumbnails are typically JPEG
		Metadata: map[string]interface{}{
			"original_file":  req.FileKey,
			"thumbnail_size": req.Size,
			"thumbnail_key":  thumbnailKey,
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
		IsPublic:    h.isPublicFile(bucketName),
		Metadata: map[string]interface{}{
			"bucket_name": bucketName,
			"uploaded_at": objInfo.LastModified,
			"etag":        objInfo.ETag,
		},
	}, nil
}

// UpdateMetadata updates file metadata
// Note: This is a simplified implementation that only verifies file existence
// For complex metadata operations, users should implement their own metadata storage
func (h *Handler) UpdateMetadata(ctx context.Context, req *interfaces.UpdateMetadataRequest) error {
	// Just verify file exists
	_, _, err := h.findFile(ctx, req.FileKey)
	if err != nil {
		return err
	}

	// Note: This library focuses on MinIO operations only
	// Complex metadata operations should be handled by external systems
	return nil
}

// Helper methods

func (h *Handler) findFile(ctx context.Context, fileKey string) (interface{}, string, error) {
	fmt.Printf("🔍 Handler findFile Debug:\n")
	fmt.Printf("  Looking for file key: %s\n", fileKey)
	fmt.Printf("  Available buckets: %v\n", h.Buckets)

	// Try to determine the most likely bucket based on file key pattern
	if bucketHint := h.getBucketHint(fileKey); bucketHint != "" {
		fmt.Printf("  🎯 Using bucket hint: %s\n", bucketHint)
		if object, err := h.Client.StatObject(ctx, bucketHint, fileKey, minio.StatObjectOptions{}); err == nil {
			fmt.Printf("  ✅ Found file in hinted bucket %s\n", bucketHint)
			return &object, bucketHint, nil
		} else {
			fmt.Printf("  ❌ Not found in hinted bucket %s: %v\n", bucketHint, err)
		}
	}

	// Fallback: search through all buckets for the file
	for category, bucketName := range h.Buckets {
		fmt.Printf("  Checking bucket %s (category: %s)\n", bucketName, category)
		object, err := h.Client.StatObject(ctx, bucketName, fileKey, minio.StatObjectOptions{})
		if err == nil {
			fmt.Printf("  ✅ Found file in bucket %s\n", bucketName)
			return &object, bucketName, nil
		}
		fmt.Printf("  ❌ Not found in bucket %s: %v\n", bucketName, err)
		// Continue searching if file not found in this bucket
	}

	fmt.Printf("  ❌ File not found in any bucket\n")
	return nil, "", &errors.StorageError{Code: "FILE_NOT_FOUND", Message: "File not found"}
}

// getBucketHint determines the most likely bucket based on file key pattern
func (h *Handler) getBucketHint(fileKey string) string {
	// File key format: {basePath}/{entityType}/{entityID}/{category}/{filename}
	// Example: "dog/dog/123/photo/1757314047_abc123.jpg"

	parts := strings.Split(fileKey, "/")
	if len(parts) < 4 {
		return ""
	}

	basePath := parts[0]
	category := parts[3]

	// Check if this matches our handler's base path
	if basePath != h.Config.BasePath {
		return ""
	}

	// Look for a bucket that matches this category
	for cat, bucketName := range h.Buckets {
		if cat == category {
			return bucketName
		}
	}

	return ""
}

func (h *Handler) isPublicFile(bucketName string) bool {
	// Check if the bucket is configured as public
	for _, categoryConfig := range h.Config.Categories {
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

// generateThumbnailKey generates a key for the thumbnail
func (h *Handler) generateThumbnailKey(originalKey, size string) string {
	// Replace the original key with thumbnail prefix and size
	parts := strings.Split(originalKey, "/")
	if len(parts) > 0 {
		parts[0] = "thumbnails"
		parts = append(parts, size)
	}
	return strings.Join(parts, "/")
}

// thumbnailExists checks if a thumbnail exists
func (h *Handler) thumbnailExists(ctx context.Context, bucketName, thumbnailKey string) (bool, error) {
	// Check if the thumbnail exists in MinIO
	_, err := h.Client.StatObject(ctx, bucketName, thumbnailKey, minio.StatObjectOptions{})
	if err != nil {
		// If the object doesn't exist, minio returns a specific error
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("failed to check thumbnail existence: %w", err)
	}

	return true, nil
}

// generateThumbnailFromObject generates a thumbnail from an object
func (h *Handler) generateThumbnailFromObject(object io.Reader, size, contentType string) ([]byte, error) {
	// Parse size (e.g., "150x150")
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid size format: %s", size)
	}

	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid width: %s", parts[0])
	}

	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid height: %s", parts[1])
	}

	// Decode the original image
	img, _, err := image.Decode(object)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Resize the image
	resizedImg := h.resizeImage(img, width, height)

	// Encode the resized image as JPEG
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, resizedImg, &jpeg.Options{Quality: 85})
	if err != nil {
		return nil, fmt.Errorf("failed to encode thumbnail: %w", err)
	}

	return buf.Bytes(), nil
}

// resizeImage resizes an image to the specified dimensions
func (h *Handler) resizeImage(img image.Image, width, height int) image.Image {
	// Get original bounds
	bounds := img.Bounds()
	originalWidth := bounds.Dx()
	originalHeight := bounds.Dy()

	// Calculate scaling factors
	scaleX := float64(width) / float64(originalWidth)
	scaleY := float64(height) / float64(originalHeight)

	// Use the smaller scale to maintain aspect ratio
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	// Calculate new dimensions
	newWidth := int(float64(originalWidth) * scale)
	newHeight := int(float64(originalHeight) * scale)

	// Create new image
	newImg := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	// Simple nearest neighbor resize
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			// Map to original coordinates
			origX := int(float64(x) / scale)
			origY := int(float64(y) / scale)

			// Ensure we don't go out of bounds
			if origX >= originalWidth {
				origX = originalWidth - 1
			}
			if origY >= originalHeight {
				origY = originalHeight - 1
			}

			// Copy pixel
			newImg.Set(x, y, img.At(origX, origY))
		}
	}

	return newImg
}

func (h *Handler) setBucketPolicy(ctx context.Context, bucketName string, categoryConfig category.CategoryConfig) error {
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
		return h.Client.SetBucketPolicy(ctx, bucketName, policy)
	}
	// For private buckets, no policy is set (default private)
	return nil
}

func (h *Handler) HealthCheck(ctx context.Context) error {
	// Check if all required buckets exist
	for category, bucketName := range h.Buckets {
		exists, err := h.Client.BucketExists(ctx, bucketName)
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
			ThumbnailBucket:    h.GetBucketName("thumbnail"),
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
