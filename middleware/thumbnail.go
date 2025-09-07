package middleware

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strconv"
	"strings"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/minio/minio-go/v7"
)

// ThumbnailMiddleware handles thumbnail generation
type ThumbnailMiddleware struct {
	config ThumbnailConfig
	client *minio.Client
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

	return &ThumbnailMiddleware{
		config: config,
		client: client,
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

	// Generate thumbnails after successful upload
	if response.Success && response.FileKey != "" {
		thumbnails, err := m.generateThumbnails(ctx, req, response.FileKey)
		if err != nil {
			// Log error but don't fail the upload
			fmt.Printf("Failed to generate thumbnails: %v\n", err)
		} else {
			response.Thumbnails = thumbnails
		}
	}

	return response, nil
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
	// TODO: This should be implemented to get the file from the appropriate bucket
	// For now, return an error as this requires integration with the storage system
	return nil, fmt.Errorf("getOriginalFile not implemented")
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
	// TODO: This should be implemented to upload to the appropriate bucket
	// For now, return a placeholder URL
	return fmt.Sprintf("https://storage.example.com/%s", key), nil
}

// generateThumbnailKey generates a key for the thumbnail
func (m *ThumbnailMiddleware) generateThumbnailKey(originalKey, size string) string {
	// Replace the original key with thumbnail prefix and size
	parts := strings.Split(originalKey, "/")
	if len(parts) > 0 {
		parts[0] = m.config.ThumbnailPrefix
		parts = append(parts, size)
	}
	return strings.Join(parts, "/")
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
		// TODO: Implement on-demand thumbnail generation
		return "", fmt.Errorf("thumbnail not found and on-demand generation not implemented")
	}

	// Generate presigned URL for thumbnail
	// TODO: Implement presigned URL generation
	return fmt.Sprintf("https://storage.example.com/%s", thumbnailKey), nil
}

// thumbnailExists checks if a thumbnail exists
func (m *ThumbnailMiddleware) thumbnailExists(ctx context.Context, thumbnailKey string) (bool, error) {
	// TODO: Implement thumbnail existence check
	// This would involve checking the storage system
	return false, nil
}
