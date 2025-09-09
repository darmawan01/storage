package middleware

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "image/gif"

	"github.com/minio/minio-go/v7"
)

// ThumbnailMiddleware handles thumbnail generation
type ThumbnailMiddleware struct {
	config         ThumbnailConfig
	client         *minio.Client
	asyncProcessor *AsyncProcessor
}

// ThumbnailConfig represents thumbnail middleware configuration
type ThumbnailConfig struct {
	// Thumbnail settings
	GenerateThumbnails bool     `json:"generate_thumbnails,omitempty"`
	ThumbnailSizes     []string `json:"thumbnail_sizes,omitempty"` // ["150x150", "300x300", "600x600"]

	// Quality settings
	JPEGQuality int `json:"jpeg_quality,omitempty"` // 1-100, default 85
	PNGQuality  int `json:"png_quality,omitempty"`  // 1-100, default 100

	// Storage settings
	ThumbnailBucket string `json:"thumbnail_bucket,omitempty"`
	ThumbnailPrefix string `json:"thumbnail_prefix,omitempty"`

	// Async processing settings
	AsyncProcessing bool        `json:"async_processing,omitempty"` // Enable async thumbnail generation
	AsyncConfig     AsyncConfig `json:"async_config,omitempty"`     // Async processor configuration
}

// NewThumbnailMiddleware creates a new thumbnail middleware
func NewThumbnailMiddleware(config ThumbnailConfig, client *minio.Client) *ThumbnailMiddleware {
	// Set default values
	if config.JPEGQuality == 0 {
		config.JPEGQuality = 85
	}
	if config.PNGQuality == 0 {
		config.PNGQuality = 100
	}
	if config.ThumbnailPrefix == "" {
		config.ThumbnailPrefix = "thumbnails"
	}

	// Initialize async processor if async processing is enabled
	var asyncProcessor *AsyncProcessor
	if config.AsyncProcessing {
		asyncConfig := config.AsyncConfig
		if asyncConfig.Workers == 0 {
			asyncConfig = DefaultAsyncConfig()
		}
		asyncProcessor = NewAsyncProcessor(asyncConfig, client, config.ThumbnailBucket)
	}

	return &ThumbnailMiddleware{
		config:         config,
		client:         client,
		asyncProcessor: asyncProcessor,
	}
}

// Name returns the middleware name
func (m *ThumbnailMiddleware) Name() string {
	return "thumbnail"
}

// Process processes the request through thumbnail middleware
func (m *ThumbnailMiddleware) Process(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
	// Only process upload operations for thumbnail generation
	if req.Operation != "upload" {
		return next(ctx, req)
	}

	// Check if thumbnail generation is enabled
	if !m.config.GenerateThumbnails {
		return next(ctx, req)
	}

	// Check if the file type supports thumbnail generation
	if !m.supportsThumbnail(req.ContentType) {
		return next(ctx, req)
	}

	// Process with next middleware first
	response, err := next(ctx, req)
	if err != nil {
		return response, err
	}

	// Set the file key in the response if not already set
	if response.FileKey == "" && req.FileKey != "" {
		response.FileKey = req.FileKey
	}

	// Generate thumbnails after successful upload
	if response.Success && response.FileKey != "" {
		// Generate "fake" thumbnail info immediately with predictable keys
		// This allows users to construct thumbnail URLs even before async processing completes
		var thumbnails []ThumbnailInfo
		for _, size := range m.config.ThumbnailSizes {
			thumbnailKey := m.generateThumbnailKey(response.FileKey, size)

			// Parse size to get width and height
			width, height, _ := parseThumbnailSize(size)

			thumbnails = append(thumbnails, ThumbnailInfo{
				Size:     size,
				URL:      thumbnailKey, // Just the thumbnail key, not a full URL
				Width:    width,
				Height:   height,
				FileSize: 0, // Will be updated when async processing completes
			})
		}
		response.Thumbnails = thumbnails

		if m.config.AsyncProcessing && m.asyncProcessor != nil {
			// Submit thumbnail job for async processing
			job := ThumbnailJob{
				FileKey:     response.FileKey,
				FileData:    req.FileData,
				FileSize:    req.FileSize,
				ContentType: req.ContentType,
				Sizes:       m.config.ThumbnailSizes,
				BucketName:  m.config.ThumbnailBucket,
				Metadata:    req.Metadata,
			}

			// Set callback to update response when thumbnails are ready
			job.Callback = func(thumbResponse *ThumbnailResponse) {
				if thumbResponse.Success {
					// Update the existing thumbnails with actual file sizes
					for i, newThumb := range thumbResponse.Thumbnails {
						if i < len(response.Thumbnails) {
							response.Thumbnails[i].FileSize = newThumb.FileSize
						}
					}
				}
			}

			if err := m.asyncProcessor.SubmitJob(job); err != nil {
				// Log error but don't fail the upload
			}
		} else {
			// Synchronous thumbnail generation
			thumbnails, err := m.generateThumbnails(ctx, req, response.FileKey)
			if err != nil {
				// Log error but don't fail the upload
			} else {
				response.Thumbnails = thumbnails
			}
		}
	}

	return response, nil
}

// Stop stops the async processor
func (m *ThumbnailMiddleware) Stop() {
	if m.asyncProcessor != nil {
		m.asyncProcessor.Stop()
	}
}

// GetAsyncStats returns async processor statistics
func (m *ThumbnailMiddleware) GetAsyncStats() map[string]interface{} {
	if m.asyncProcessor == nil {
		return map[string]interface{}{
			"async_enabled": false,
		}
	}

	stats := m.asyncProcessor.GetStats()
	stats["async_enabled"] = true
	return stats
}

// generateThumbnails generates thumbnails for the uploaded file
func (m *ThumbnailMiddleware) generateThumbnails(ctx context.Context, req *StorageRequest, fileKey string) ([]ThumbnailInfo, error) {
	var thumbnails []ThumbnailInfo

	// Get the original file from storage
	originalData, err := m.getOriginalFile(ctx, fileKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get original file: %w", err)
	}
	defer originalData.Close()

	// Decode the original image
	originalImg, format, err := image.Decode(originalData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Generate thumbnails for each configured size
	for _, sizeStr := range m.config.ThumbnailSizes {
		width, height, err := parseThumbnailSize(sizeStr)
		if err != nil {
			fmt.Printf("Invalid thumbnail size %s: %v\n", sizeStr, err)
			continue
		}

		// Generate thumbnail
		thumbnailData, err := m.createThumbnail(originalImg, width, height, format)
		if err != nil {
			fmt.Printf("Failed to create thumbnail %s: %v\n", sizeStr, err)
			continue
		}

		// Upload thumbnail to storage
		thumbnailKey := m.generateThumbnailKey(fileKey, sizeStr)
		thumbnailURL, err := m.uploadThumbnail(ctx, thumbnailKey, thumbnailData, format)
		if err != nil {
			fmt.Printf("Failed to upload thumbnail %s: %v\n", sizeStr, err)
			continue
		}

		// Add thumbnail info
		thumbnails = append(thumbnails, ThumbnailInfo{
			Size:     sizeStr,
			URL:      thumbnailURL,
			Width:    width,
			Height:   height,
			FileSize: int64(len(thumbnailData)),
		})
	}

	return thumbnails, nil
}

// getOriginalFile retrieves the original file from storage
func (m *ThumbnailMiddleware) getOriginalFile(ctx context.Context, fileKey string) (io.ReadCloser, error) {
	// Get the object from MinIO
	object, err := m.client.GetObject(ctx, m.config.ThumbnailBucket, fileKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object from MinIO: %w", err)
	}

	return object, nil
}

// createThumbnail creates a thumbnail from the original image
func (m *ThumbnailMiddleware) createThumbnail(originalImg image.Image, width, height int, format string) ([]byte, error) {
	// Resize the image
	resizedImg := m.resizeImage(originalImg, width, height)

	// Encode the resized image
	var buf bytes.Buffer
	switch format {
	case "jpeg":
		err := jpeg.Encode(&buf, resizedImg, &jpeg.Options{Quality: m.config.JPEGQuality})
		if err != nil {
			return nil, fmt.Errorf("failed to encode JPEG thumbnail: %w", err)
		}
	case "png":
		err := png.Encode(&buf, resizedImg)
		if err != nil {
			return nil, fmt.Errorf("failed to encode PNG thumbnail: %w", err)
		}
	default:
		// Default to JPEG for other formats
		err := jpeg.Encode(&buf, resizedImg, &jpeg.Options{Quality: m.config.JPEGQuality})
		if err != nil {
			return nil, fmt.Errorf("failed to encode thumbnail: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// resizeImage resizes an image to the specified dimensions
func (m *ThumbnailMiddleware) resizeImage(img image.Image, width, height int) image.Image {
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

// uploadThumbnail uploads a thumbnail to storage
func (m *ThumbnailMiddleware) uploadThumbnail(ctx context.Context, key string, data []byte, format string) (string, error) {
	// Create a reader from the byte data
	reader := bytes.NewReader(data)

	// Determine content type based on format
	contentType := "image/jpeg"
	if format == "png" {
		contentType = "image/png"
	}

	// Upload the thumbnail to MinIO
	_, err := m.client.PutObject(
		ctx,
		m.config.ThumbnailBucket,
		key,
		reader,
		int64(len(data)),
		minio.PutObjectOptions{
			ContentType: contentType,
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to upload thumbnail to MinIO: %w", err)
	}

	// Generate the thumbnail URL
	thumbnailURL := fmt.Sprintf("/api/v1/files/%s/thumbnail?size=%s", key, "thumbnail")
	return thumbnailURL, nil
}

// generateThumbnailKey generates a key for the thumbnail using predictable naming
func (m *ThumbnailMiddleware) generateThumbnailKey(originalKey, size string) string {
	// Use predictable naming pattern: original_file_key_512x512.png
	// This makes it easy for users to construct thumbnail URLs

	// Get the file extension from the original key
	ext := filepath.Ext(originalKey)
	if ext == "" {
		ext = ".jpg" // Default to jpg for thumbnails
	}

	// Remove the extension from the original key
	baseKey := strings.TrimSuffix(originalKey, ext)

	// Create the thumbnail key with size suffix
	thumbnailKey := fmt.Sprintf("%s_%s%s", baseKey, size, ext)
	return thumbnailKey
}

// supportsThumbnail checks if the content type supports thumbnail generation
func (m *ThumbnailMiddleware) supportsThumbnail(contentType string) bool {
	supportedTypes := []string{
		"image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp", "image/bmp",
	}

	for _, t := range supportedTypes {
		if strings.HasPrefix(contentType, t) {
			return true
		}
	}
	return false
}

// parseThumbnailSize parses a thumbnail size string (e.g., "150x150")
func parseThumbnailSize(size string) (width, height int, err error) {
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid size format: %s", size)
	}

	width, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid width: %s", parts[0])
	}

	height, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid height: %s", parts[1])
	}

	if width <= 0 || height <= 0 {
		return 0, 0, fmt.Errorf("dimensions must be positive: %s", size)
	}

	return width, height, nil
}

// GetThumbnailURL generates a thumbnail URL for a file
func (m *ThumbnailMiddleware) GetThumbnailURL(ctx context.Context, fileKey, size string) (string, error) {
	// Generate thumbnail key
	thumbnailKey := m.generateThumbnailKey(fileKey, size)

	// Check if thumbnail exists
	exists, err := m.thumbnailExists(ctx, thumbnailKey)
	if err != nil {
		return "", fmt.Errorf("failed to check thumbnail existence: %w", err)
	}

	if !exists {
		// Generate thumbnail on demand
		// This would require the original file, which we don't have in this context
		// For now, return an error indicating the thumbnail needs to be generated
		return "", fmt.Errorf("thumbnail not found - please upload the file first to generate thumbnails")
	}

	// Generate presigned URL for thumbnail
	presignedURL, err := m.client.PresignedGetObject(ctx, m.config.ThumbnailBucket, thumbnailKey, 24*time.Hour, nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedURL.String(), nil
}

// thumbnailExists checks if a thumbnail exists
func (m *ThumbnailMiddleware) thumbnailExists(ctx context.Context, thumbnailKey string) (bool, error) {
	// Check if the thumbnail exists in MinIO
	_, err := m.client.StatObject(ctx, m.config.ThumbnailBucket, thumbnailKey, minio.StatObjectOptions{})
	if err != nil {
		// If the object doesn't exist, minio returns a specific error
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("failed to check thumbnail existence: %w", err)
	}

	return true, nil
}
