package middleware

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"path/filepath"
	"slices"
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
		if !slices.Contains(m.config.AllowedTypes, req.ContentType) {
			return fmt.Errorf("content type %s is not allowed, allowed types: %v", req.ContentType, m.config.AllowedTypes)
		}
	}

	// Check file extension
	if len(m.config.AllowedExtensions) > 0 {
		ext := strings.ToLower(filepath.Ext(req.FileName))
		if !slices.Contains(m.config.AllowedExtensions, ext) {
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
	// Read the image data
	reader := req.FileData
	if reader == nil {
		return fmt.Errorf("no file data provided for image validation")
	}

	// Decode the image to get dimensions and format
	img, format, err := image.Decode(reader)
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Get image dimensions
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Validate format
	if len(config.AllowedFormats) > 0 {
		formatValid := false
		for _, allowedFormat := range config.AllowedFormats {
			if strings.EqualFold(format, allowedFormat) {
				formatValid = true
				break
			}
		}
		if !formatValid {
			return fmt.Errorf("image format %s not allowed, allowed formats: %v", format, config.AllowedFormats)
		}
	}

	// Validate dimensions
	if config.MinWidth > 0 && width < config.MinWidth {
		return fmt.Errorf("image width %d is below minimum %d", width, config.MinWidth)
	}
	if config.MaxWidth > 0 && width > config.MaxWidth {
		return fmt.Errorf("image width %d exceeds maximum %d", width, config.MaxWidth)
	}
	if config.MinHeight > 0 && height < config.MinHeight {
		return fmt.Errorf("image height %d is below minimum %d", height, config.MinHeight)
	}
	if config.MaxHeight > 0 && height > config.MaxHeight {
		return fmt.Errorf("image height %d exceeds maximum %d", height, config.MaxHeight)
	}

	// Validate aspect ratio
	if config.MinAspectRatio > 0 || config.MaxAspectRatio > 0 {
		aspectRatio := float64(width) / float64(height)
		if config.MinAspectRatio > 0 && aspectRatio < config.MinAspectRatio {
			return fmt.Errorf("image aspect ratio %.2f is below minimum %.2f", aspectRatio, config.MinAspectRatio)
		}
		if config.MaxAspectRatio > 0 && aspectRatio > config.MaxAspectRatio {
			return fmt.Errorf("image aspect ratio %.2f exceeds maximum %.2f", aspectRatio, config.MaxAspectRatio)
		}
	}

	return nil
}

// validatePDF performs PDF-specific validation
func (m *ValidationMiddleware) validatePDF(req *StorageRequest, config PDFValidationConfig) error {
	// Basic PDF validation - check file header
	reader := req.FileData
	if reader == nil {
		return fmt.Errorf("no file data provided for PDF validation")
	}

	// Read first few bytes to check PDF header
	header := make([]byte, 8)
	n, err := reader.Read(header)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read PDF header: %w", err)
	}

	// Check if it starts with PDF signature
	if n < 4 || string(header[:4]) != "%PDF" {
		return fmt.Errorf("invalid PDF file: missing PDF signature")
	}

	// Basic structure validation would require a PDF parser library
	// For now, we just validate the header and basic requirements
	if config.ValidateStructure {
		// This would require a proper PDF parsing library like unidoc/unipdf
		// For now, just return a warning that full validation is not implemented
		return fmt.Errorf("PDF structure validation not fully implemented - requires PDF parsing library")
	}

	return nil
}

// validateVideo performs video-specific validation
func (m *ValidationMiddleware) validateVideo(req *StorageRequest, config VideoValidationConfig) error {
	// Basic video validation - check file extension and basic structure
	reader := req.FileData
	if reader == nil {
		return fmt.Errorf("no file data provided for video validation")
	}

	// Read first few bytes to check video container signature
	header := make([]byte, 12)
	n, err := reader.Read(header)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read video header: %w", err)
	}

	// Basic container format detection
	// This is a simplified check - proper video validation would require ffmpeg or similar
	if n >= 4 {
		// Check for common video container signatures
		signature := string(header[:4])
		validSignatures := []string{
			"\x00\x00\x00\x20", // MP4
			"ftyp",             // MP4/MOV
			"\x1a\x45\xdf\xa3", // Matroska (MKV)
			"RIFF",             // AVI
		}

		valid := false
		for _, sig := range validSignatures {
			if strings.HasPrefix(signature, sig) {
				valid = true
				break
			}
		}

		if !valid {
			return fmt.Errorf("invalid video file: unrecognized container format")
		}
	}

	// Note: Full video validation (duration, resolution, codec) would require
	// a video processing library like ffmpeg or gst-libav
	// For now, we just do basic container validation

	return nil
}

// validateAudio performs audio-specific validation
func (m *ValidationMiddleware) validateAudio(req *StorageRequest, config AudioValidationConfig) error {
	// Basic audio validation - check file extension and basic structure
	reader := req.FileData
	if reader == nil {
		return fmt.Errorf("no file data provided for audio validation")
	}

	// Read first few bytes to check audio format signature
	header := make([]byte, 12)
	n, err := reader.Read(header)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read audio header: %w", err)
	}

	// Basic audio format detection
	// This is a simplified check - proper audio validation would require ffmpeg or similar
	if n >= 4 {
		// Check for common audio format signatures
		signature := string(header[:4])
		validSignatures := []string{
			"RIFF",     // WAV
			"OggS",     // OGG
			"ID3",      // MP3 with ID3 tag
			"\xff\xfb", // MP3 frame sync
			"\xff\xf3", // MP3 frame sync
			"\xff\xf2", // MP3 frame sync
			"fLaC",     // FLAC
		}

		valid := false
		for _, sig := range validSignatures {
			if strings.HasPrefix(signature, sig) {
				valid = true
				break
			}
		}

		// Also check for MP3 without ID3 tag (starts with frame sync)
		if !valid && n >= 2 {
			mp3Sync := []byte{0xff, 0xfb}
			if header[0] == mp3Sync[0] && (header[1]&0xe0) == mp3Sync[1] {
				valid = true
			}
		}

		if !valid {
			return fmt.Errorf("invalid audio file: unrecognized audio format")
		}
	}

	// Note: Full audio validation (duration, bitrate, sample rate) would require
	// an audio processing library like ffmpeg or gst-libav
	// For now, we just do basic format validation

	return nil
}

// Content type detection methods
func (m *ValidationMiddleware) isImageType(contentType string) bool {
	imageTypes := []string{
		"image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp", "image/bmp", "image/tiff",
	}
	return slices.Contains(imageTypes, contentType)
}

func (m *ValidationMiddleware) isPDFType(contentType string) bool {
	return contentType == "application/pdf"
}

func (m *ValidationMiddleware) isVideoType(contentType string) bool {
	videoTypes := []string{
		"video/mp4", "video/webm", "video/avi", "video/mov", "video/wmv", "video/flv", "video/3gp", "video/quicktime",
	}
	return slices.Contains(videoTypes, contentType)
}

func (m *ValidationMiddleware) isAudioType(contentType string) bool {
	audioTypes := []string{
		"audio/mpeg", "audio/mp3", "audio/wav", "audio/ogg", "audio/aac", "audio/flac", "audio/m4a",
	}
	return slices.Contains(audioTypes, contentType)
}
