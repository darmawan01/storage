package middleware

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// ValidationMiddleware handles file validation
type ValidationMiddleware struct {
	config ValidationConfig
}

// ValidationConfig represents validation middleware configuration
type ValidationConfig struct {
	// Basic file validation
	MaxFileSize       int64    `json:"max_file_size,omitempty"`
	MinFileSize       int64    `json:"min_file_size,omitempty"`
	AllowedTypes      []string `json:"allowed_types,omitempty"`
	AllowedExtensions []string `json:"allowed_extensions,omitempty"`

	// Image validation
	ImageValidation *ImageValidationConfig `json:"image_validation,omitempty"`

	// PDF validation
	PDFValidation *PDFValidationConfig `json:"pdf_validation,omitempty"`

	// Video validation
	VideoValidation *VideoValidationConfig `json:"video_validation,omitempty"`

	// Audio validation
	AudioValidation *AudioValidationConfig `json:"audio_validation,omitempty"`
}

// ImageValidationConfig represents image-specific validation
type ImageValidationConfig struct {
	MinWidth           int      `json:"min_width,omitempty"`
	MaxWidth           int      `json:"max_width,omitempty"`
	MinHeight          int      `json:"min_height,omitempty"`
	MaxHeight          int      `json:"max_height,omitempty"`
	MinQuality         int      `json:"min_quality,omitempty"`
	MaxQuality         int      `json:"max_quality,omitempty"`
	AllowedFormats     []string `json:"allowed_formats,omitempty"`
	MinAspectRatio     float64  `json:"min_aspect_ratio,omitempty"`
	MaxAspectRatio     float64  `json:"max_aspect_ratio,omitempty"`
	AllowedColorSpaces []string `json:"allowed_color_spaces,omitempty"`
}

// PDFValidationConfig represents PDF-specific validation
type PDFValidationConfig struct {
	ValidateStructure bool     `json:"validate_structure,omitempty"`
	MinPages          int      `json:"min_pages,omitempty"`
	MaxPages          int      `json:"max_pages,omitempty"`
	RequireMetadata   bool     `json:"require_metadata,omitempty"`
	RequiredFields    []string `json:"required_fields,omitempty"`
	AllowPassword     bool     `json:"allow_password,omitempty"`
	AllowScripts      bool     `json:"allow_scripts,omitempty"`
}

// VideoValidationConfig represents video-specific validation
type VideoValidationConfig struct {
	MinDuration   int      `json:"min_duration,omitempty"` // seconds
	MaxDuration   int      `json:"max_duration,omitempty"` // seconds
	MinWidth      int      `json:"min_width,omitempty"`
	MaxWidth      int      `json:"max_width,omitempty"`
	MinHeight     int      `json:"min_height,omitempty"`
	MaxHeight     int      `json:"max_height,omitempty"`
	AllowedCodecs []string `json:"allowed_codecs,omitempty"`
	MinFrameRate  int      `json:"min_frame_rate,omitempty"`
	MaxFrameRate  int      `json:"max_frame_rate,omitempty"`
}

// AudioValidationConfig represents audio-specific validation
type AudioValidationConfig struct {
	MinDuration    int      `json:"min_duration,omitempty"` // seconds
	MaxDuration    int      `json:"max_duration,omitempty"` // seconds
	MinBitrate     int      `json:"min_bitrate,omitempty"`  // kbps
	MaxBitrate     int      `json:"max_bitrate,omitempty"`  // kbps
	AllowedFormats []string `json:"allowed_formats,omitempty"`
	MinSampleRate  int      `json:"min_sample_rate,omitempty"`
	MaxSampleRate  int      `json:"max_sample_rate,omitempty"`
}

// NewValidationMiddleware creates a new validation middleware
func NewValidationMiddleware(config ValidationConfig) *ValidationMiddleware {
	return &ValidationMiddleware{
		config: config,
	}
}

// Name returns the middleware name
func (m *ValidationMiddleware) Name() string {
	return "validation"
}

// Process processes the request through validation middleware
func (m *ValidationMiddleware) Process(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
	// Only validate upload operations
	if req.Operation != "upload" {
		return next(ctx, req)
	}

	// Perform validation
	if err := m.validateFile(req); err != nil {
		return &StorageResponse{
			Success: false,
			Error:   err,
		}, nil
	}

	return next(ctx, req)
}

// validateFile performs comprehensive file validation
func (m *ValidationMiddleware) validateFile(req *StorageRequest) error {
	// Basic validation
	if err := m.validateBasicFile(req); err != nil {
		return err
	}

	// Content-type specific validation
	if err := m.validateContentType(req); err != nil {
		return err
	}

	return nil
}

// validateBasicFile performs basic file validation
func (m *ValidationMiddleware) validateBasicFile(req *StorageRequest) error {
	// Check file size
	if m.config.MaxFileSize > 0 && req.FileSize > m.config.MaxFileSize {
		return fmt.Errorf("file size %d exceeds maximum allowed size %d", req.FileSize, m.config.MaxFileSize)
	}

	if m.config.MinFileSize > 0 && req.FileSize < m.config.MinFileSize {
		return fmt.Errorf("file size %d is below minimum required size %d", req.FileSize, m.config.MinFileSize)
	}

	// Check content type
	if len(m.config.AllowedTypes) > 0 {
		if !contains(m.config.AllowedTypes, req.ContentType) {
			return fmt.Errorf("content type %s is not allowed, allowed types: %v", req.ContentType, m.config.AllowedTypes)
		}
	}

	// Check file extension
	if len(m.config.AllowedExtensions) > 0 {
		ext := strings.ToLower(filepath.Ext(req.FileName))
		if !contains(m.config.AllowedExtensions, ext) {
			return fmt.Errorf("file extension %s is not allowed, allowed extensions: %v", ext, m.config.AllowedExtensions)
		}
	}

	return nil
}

// validateContentType performs content-type specific validation
func (m *ValidationMiddleware) validateContentType(req *StorageRequest) error {
	contentType := req.ContentType

	// Image validation
	if m.isImageType(contentType) && m.config.ImageValidation != nil {
		if err := m.validateImage(req, *m.config.ImageValidation); err != nil {
			return fmt.Errorf("image validation failed: %w", err)
		}
	}

	// PDF validation
	if m.isPDFType(contentType) && m.config.PDFValidation != nil {
		if err := m.validatePDF(req, *m.config.PDFValidation); err != nil {
			return fmt.Errorf("PDF validation failed: %w", err)
		}
	}

	// Video validation
	if m.isVideoType(contentType) && m.config.VideoValidation != nil {
		if err := m.validateVideo(req, *m.config.VideoValidation); err != nil {
			return fmt.Errorf("video validation failed: %w", err)
		}
	}

	// Audio validation
	if m.isAudioType(contentType) && m.config.AudioValidation != nil {
		if err := m.validateAudio(req, *m.config.AudioValidation); err != nil {
			return fmt.Errorf("audio validation failed: %w", err)
		}
	}

	return nil
}

// validateImage performs image-specific validation
func (m *ValidationMiddleware) validateImage(req *StorageRequest, config ImageValidationConfig) error {
	// TODO: Implement image validation
	// This would involve decoding the image and checking dimensions, quality, etc.
	// For now, return nil (no validation)
	return nil
}

// validatePDF performs PDF-specific validation
func (m *ValidationMiddleware) validatePDF(req *StorageRequest, config PDFValidationConfig) error {
	// TODO: Implement PDF validation
	// This would involve parsing the PDF and checking structure, pages, metadata, etc.
	// For now, return nil (no validation)
	return nil
}

// validateVideo performs video-specific validation
func (m *ValidationMiddleware) validateVideo(req *StorageRequest, config VideoValidationConfig) error {
	// TODO: Implement video validation
	// This would involve parsing the video and checking duration, resolution, codec, etc.
	// For now, return nil (no validation)
	return nil
}

// validateAudio performs audio-specific validation
func (m *ValidationMiddleware) validateAudio(req *StorageRequest, config AudioValidationConfig) error {
	// TODO: Implement audio validation
	// This would involve parsing the audio and checking duration, bitrate, sample rate, etc.
	// For now, return nil (no validation)
	return nil
}

// Content type detection methods
func (m *ValidationMiddleware) isImageType(contentType string) bool {
	imageTypes := []string{
		"image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp", "image/bmp", "image/tiff",
	}
	return contains(imageTypes, contentType)
}

func (m *ValidationMiddleware) isPDFType(contentType string) bool {
	return contentType == "application/pdf"
}

func (m *ValidationMiddleware) isVideoType(contentType string) bool {
	videoTypes := []string{
		"video/mp4", "video/webm", "video/avi", "video/mov", "video/wmv", "video/flv", "video/3gp", "video/quicktime",
	}
	return contains(videoTypes, contentType)
}

func (m *ValidationMiddleware) isAudioType(contentType string) bool {
	audioTypes := []string{
		"audio/mpeg", "audio/mp3", "audio/wav", "audio/ogg", "audio/aac", "audio/flac", "audio/m4a",
	}
	return contains(audioTypes, contentType)
}

// Utility function
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
