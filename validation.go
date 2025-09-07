package storage

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"path/filepath"
	"strings"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// validateFile performs basic file validation
func (h *Handler) validateFile(req *UploadRequest, config ValidationConfig) error {
	// Basic validation
	if err := h.validateBasicFile(req, config); err != nil {
		return err
	}

	// Image validation (only if image types are allowed and config provided)
	if h.isImageType(req.ContentType) && config.ImageValidation != nil {
		if err := h.validateImage(req, *config.ImageValidation); err != nil {
			return fmt.Errorf("image validation failed: %w", err)
		}
	}

	return nil
}

// validateBasicFile performs basic file validation
func (h *Handler) validateBasicFile(req *UploadRequest, config ValidationConfig) error {
	// Check file size
	if config.MaxFileSize > 0 && req.FileSize > config.MaxFileSize {
		return &StorageError{
			Code:    "FILE_TOO_LARGE",
			Message: fmt.Sprintf("file size %d exceeds maximum allowed size %d", req.FileSize, config.MaxFileSize),
		}
	}

	if config.MinFileSize > 0 && req.FileSize < config.MinFileSize {
		return &StorageError{
			Code:    "FILE_TOO_SMALL",
			Message: fmt.Sprintf("file size %d is below minimum required size %d", req.FileSize, config.MinFileSize),
		}
	}

	// Check content type
	if len(config.AllowedTypes) > 0 {
		if !contains(config.AllowedTypes, req.ContentType) {
			return &StorageError{
				Code:    "UNSUPPORTED_TYPE",
				Message: fmt.Sprintf("content type %s is not allowed, allowed types: %v", req.ContentType, config.AllowedTypes),
			}
		}
	}

	// Check file extension
	if len(config.AllowedExtensions) > 0 {
		ext := strings.ToLower(filepath.Ext(req.FileName))
		if !contains(config.AllowedExtensions, ext) {
			return &StorageError{
				Code:    "INVALID_EXTENSION",
				Message: fmt.Sprintf("file extension %s is not allowed, allowed extensions: %v", ext, config.AllowedExtensions),
			}
		}
	}

	return nil
}

// validateImage performs image-specific validation
func (h *Handler) validateImage(req *UploadRequest, config ImageValidationConfig) error {
	// Decode image to get properties
	img, format, err := h.decodeImage(req.FileData)
	if err != nil {
		return &StorageError{
			Code:    "INVALID_IMAGE",
			Message: fmt.Sprintf("failed to decode image: %v", err),
		}
	}

	// Check dimensions
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if config.MinWidth > 0 && width < config.MinWidth {
		return &StorageError{
			Code:    "IMAGE_TOO_SMALL",
			Message: fmt.Sprintf("image width %d is below minimum %d", width, config.MinWidth),
		}
	}
	if config.MaxWidth > 0 && width > config.MaxWidth {
		return &StorageError{
			Code:    "IMAGE_TOO_LARGE",
			Message: fmt.Sprintf("image width %d exceeds maximum %d", width, config.MaxWidth),
		}
	}
	if config.MinHeight > 0 && height < config.MinHeight {
		return &StorageError{
			Code:    "IMAGE_TOO_SMALL",
			Message: fmt.Sprintf("image height %d is below minimum %d", height, config.MinHeight),
		}
	}
	if config.MaxHeight > 0 && height > config.MaxHeight {
		return &StorageError{
			Code:    "IMAGE_TOO_LARGE",
			Message: fmt.Sprintf("image height %d exceeds maximum %d", height, config.MaxHeight),
		}
	}

	// Check aspect ratio
	aspectRatio := float64(width) / float64(height)
	if config.MinAspectRatio > 0 && aspectRatio < config.MinAspectRatio {
		return &StorageError{
			Code:    "INVALID_ASPECT_RATIO",
			Message: fmt.Sprintf("image aspect ratio %.2f is below minimum %.2f", aspectRatio, config.MinAspectRatio),
		}
	}
	if config.MaxAspectRatio > 0 && aspectRatio > config.MaxAspectRatio {
		return &StorageError{
			Code:    "INVALID_ASPECT_RATIO",
			Message: fmt.Sprintf("image aspect ratio %.2f exceeds maximum %.2f", aspectRatio, config.MaxAspectRatio),
		}
	}

	// Check format
	if len(config.AllowedFormats) > 0 {
		if !contains(config.AllowedFormats, format) {
			return &StorageError{
				Code:    "UNSUPPORTED_FORMAT",
				Message: fmt.Sprintf("image format %s is not allowed, allowed formats: %v", format, config.AllowedFormats),
			}
		}
	}

	// Check quality (if config provided)
	if config.MinQuality > 0 || config.MaxQuality > 0 {
		quality, err := h.getImageQuality(req.FileData, format)
		if err != nil {
			return &StorageError{
				Code:    "QUALITY_CHECK_FAILED",
				Message: fmt.Sprintf("failed to get image quality: %v", err),
			}
		}

		if config.MinQuality > 0 && quality < config.MinQuality {
			return &StorageError{
				Code:    "QUALITY_TOO_LOW",
				Message: fmt.Sprintf("image quality %d is below minimum %d", quality, config.MinQuality),
			}
		}
		if config.MaxQuality > 0 && quality > config.MaxQuality {
			return &StorageError{
				Code:    "QUALITY_TOO_HIGH",
				Message: fmt.Sprintf("image quality %d exceeds maximum %d", quality, config.MaxQuality),
			}
		}
	}

	// Check color space
	if len(config.AllowedColorSpaces) > 0 {
		colorSpace := h.getImageColorSpace(img)
		if !contains(config.AllowedColorSpaces, colorSpace) {
			return &StorageError{
				Code:    "UNSUPPORTED_COLOR_SPACE",
				Message: fmt.Sprintf("image color space %s is not allowed, allowed color spaces: %v", colorSpace, config.AllowedColorSpaces),
			}
		}
	}

	return nil
}

// Helper functions for content type detection
func (h *Handler) isImageType(contentType string) bool {
	imageTypes := []string{"image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp", "image/tiff"}
	return contains(imageTypes, contentType)
}

// Image processing functions
func (h *Handler) decodeImage(reader io.Reader) (image.Image, string, error) {
	// Read the image data
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, "", err
	}

	// Decode the image
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}

	return img, format, nil
}

func (h *Handler) getImageQuality(reader io.Reader, format string) (int, error) {
	// This is a simplified implementation
	// In a real implementation, you would use a proper image quality detection library
	switch format {
	case "jpeg":
		// For JPEG, quality is typically stored in the file
		// This is a placeholder implementation
		return 85, nil
	case "png":
		// PNG doesn't have quality in the same sense as JPEG
		// Return a default value
		return 100, nil
	default:
		return 100, nil
	}
}

func (h *Handler) getImageColorSpace(img image.Image) string {
	// This is a simplified implementation
	// In a real implementation, you would properly detect the color space
	// For now, return a default value
	return "RGB"
}

// Utility functions
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
