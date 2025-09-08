package category

import (
	"github.com/darmawan01/storage/errors"
	"github.com/darmawan01/storage/middleware"
)

// CategoryConfig represents category-specific configuration
type CategoryConfig struct {
	BucketSuffix string   `json:"bucket_suffix"`
	IsPublic     bool     `json:"is_public"`
	MaxSize      int64    `json:"max_size"`
	AllowedTypes []string `json:"allowed_types"`

	// Basic validation handled by storage package
	Validation ValidationConfig `json:"validation,omitempty"`

	// Category-specific middlewares (overrides handler defaults)
	Middlewares []string `json:"middlewares,omitempty"`

	// Category-specific security
	Security middleware.SecurityConfig `json:"security,omitempty"`

	// Category-specific preview settings
	Preview PreviewConfig `json:"preview,omitempty"`
}

// ValidationConfig represents basic validation configuration
type ValidationConfig struct {
	// Basic file validation
	MaxFileSize       int64    `json:"max_file_size,omitempty"`
	MinFileSize       int64    `json:"min_file_size,omitempty"`
	AllowedTypes      []string `json:"allowed_types,omitempty"`
	AllowedExtensions []string `json:"allowed_extensions,omitempty"`

	// Image validation (only applied if AllowedTypes contains image types)
	ImageValidation *ImageValidationConfig `json:"image_validation,omitempty"`

	// PDF validation (only applied if AllowedTypes contains application/pdf)
	PDFValidation *PDFValidationConfig `json:"pdf_validation,omitempty"`

	// Video validation (only applied if AllowedTypes contains video types)
	VideoValidation *VideoValidationConfig `json:"video_validation,omitempty"`

	// Audio validation (only applied if AllowedTypes contains audio types)
	AudioValidation *AudioValidationConfig `json:"audio_validation,omitempty"`
}

// ImageValidationConfig represents image-specific validation
type ImageValidationConfig struct {
	// Dimension validation
	MinWidth  int `json:"min_width,omitempty"`
	MaxWidth  int `json:"max_width,omitempty"`
	MinHeight int `json:"min_height,omitempty"`
	MaxHeight int `json:"max_height,omitempty"`

	// Quality validation
	MinQuality int `json:"min_quality,omitempty"` // 1-100
	MaxQuality int `json:"max_quality,omitempty"` // 1-100

	// Format validation
	AllowedFormats []string `json:"allowed_formats,omitempty"` // ["jpeg", "png", "webp"]

	// Aspect ratio validation
	MinAspectRatio float64 `json:"min_aspect_ratio,omitempty"`
	MaxAspectRatio float64 `json:"max_aspect_ratio,omitempty"`

	// Color space validation
	AllowedColorSpaces []string `json:"allowed_color_spaces,omitempty"` // ["RGB", "RGBA", "GRAY"]
}

// PDFValidationConfig represents PDF-specific validation
type PDFValidationConfig struct {
	// PDF structure validation
	ValidateStructure bool `json:"validate_structure,omitempty"`

	// Page count validation
	MinPages int `json:"min_pages,omitempty"`
	MaxPages int `json:"max_pages,omitempty"`

	// Metadata validation
	RequireMetadata bool     `json:"require_metadata,omitempty"`
	RequiredFields  []string `json:"required_fields,omitempty"` // ["title", "author"]

	// Security validation
	AllowPassword bool `json:"allow_password,omitempty"`
	AllowScripts  bool `json:"allow_scripts,omitempty"`
}

// VideoValidationConfig represents video-specific validation
type VideoValidationConfig struct {
	// Duration validation
	MinDuration int `json:"min_duration,omitempty"` // seconds
	MaxDuration int `json:"max_duration,omitempty"` // seconds

	// Resolution validation
	MinWidth  int `json:"min_width,omitempty"`
	MaxWidth  int `json:"max_width,omitempty"`
	MinHeight int `json:"min_height,omitempty"`
	MaxHeight int `json:"max_height,omitempty"`

	// Codec validation
	AllowedCodecs []string `json:"allowed_codecs,omitempty"` // ["h264", "h265", "vp9"]

	// Frame rate validation
	MinFrameRate int `json:"min_frame_rate,omitempty"`
	MaxFrameRate int `json:"max_frame_rate,omitempty"`
}

// AudioValidationConfig represents audio-specific validation
type AudioValidationConfig struct {
	// Duration validation
	MinDuration int `json:"min_duration,omitempty"` // seconds
	MaxDuration int `json:"max_duration,omitempty"` // seconds

	// Bitrate validation
	MinBitrate int `json:"min_bitrate,omitempty"` // kbps
	MaxBitrate int `json:"max_bitrate,omitempty"` // kbps

	// Format validation
	AllowedFormats []string `json:"allowed_formats,omitempty"` // ["mp3", "wav", "aac", "flac"]

	// Sample rate validation
	MinSampleRate int `json:"min_sample_rate,omitempty"` // Hz
	MaxSampleRate int `json:"max_sample_rate,omitempty"` // Hz
}

// PreviewConfig represents preview configuration
type PreviewConfig struct {
	// Thumbnail settings
	GenerateThumbnails bool     `json:"generate_thumbnails,omitempty"`
	ThumbnailSizes     []string `json:"thumbnail_sizes,omitempty"` // ["150x150", "300x300", "600x600"]

	// Preview settings
	EnablePreview  bool     `json:"enable_preview,omitempty"`
	PreviewFormats []string `json:"preview_formats,omitempty"` // ["image", "pdf", "video"]

	// CDN settings
	UseCDN      bool   `json:"use_cdn,omitempty"`
	CDNEndpoint string `json:"cdn_endpoint,omitempty"`
}

func (c *CategoryConfig) Validate() error {
	if c.BucketSuffix == "" {
		return &errors.StorageError{Code: "INVALID_CONFIG", Message: "BucketSuffix is required"}
	}
	if c.MaxSize <= 0 {
		return &errors.StorageError{Code: "INVALID_CONFIG", Message: "MaxSize must be greater than 0"}
	}
	return nil
}

func DefaultCategoryConfig(bucketSuffix string, isPublic bool, maxSize int64) CategoryConfig {
	return CategoryConfig{
		BucketSuffix: bucketSuffix,
		IsPublic:     isPublic,
		MaxSize:      maxSize,
		AllowedTypes: []string{},
		Validation: ValidationConfig{
			MaxFileSize: maxSize,
			MinFileSize: 1024, // 1KB minimum
		},
		Security: middleware.SecurityConfig{
			RequireAuth:  !isPublic,
			RequireOwner: !isPublic,
		},
		Preview: PreviewConfig{
			GenerateThumbnails: false,
			EnablePreview:      false,
		},
	}
}
