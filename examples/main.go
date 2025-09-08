package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/darmawan01/storage/category"
	"github.com/darmawan01/storage/config"
	"github.com/darmawan01/storage/handler"
	"github.com/darmawan01/storage/interfaces"
	"github.com/darmawan01/storage/middleware"
	"github.com/darmawan01/storage/registry"

	_ "github.com/darmawan01/storage/docs" // This will be generated
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

var storageRegistry *registry.Registry
var metadataStorage *ExampleMetadataStorage

// @title           Cat & Dog Photo Storage API
// @version         1.0
// @description     A simple and clean file storage API built with MinIO, supporting cat and dog photo uploads with automatic thumbnail generation, metadata callbacks, and direct/presigned downloads.
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT

// @host      localhost:8080
// @BasePath  /api/v1

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-User-ID

// @externalDocs.description  OpenAPI
// @externalDocs.url          https://swagger.io/resources/open-api/

// UploadResponse represents the response structure for file uploads
type UploadResponse struct {
	FileID      string                 `json:"file_id" example:"user_123_profile_abc123.jpg"`
	FileName    string                 `json:"file_name" example:"profile.jpg"`
	FileSize    int64                  `json:"file_size" example:"1024000"`
	ContentType string                 `json:"content_type" example:"image/jpeg"`
	Category    string                 `json:"category" example:"profile"`
	EntityType  string                 `json:"entity_type" example:"user"`
	EntityID    string                 `json:"entity_id" example:"123"`
	UploadedAt  time.Time              `json:"uploaded_at" example:"2023-12-01T10:30:00Z"`
	Metadata    map[string]interface{} `json:"metadata"`
	Thumbnails  []ThumbnailInfo        `json:"thumbnails,omitempty"`
}

// ThumbnailInfo represents thumbnail information
type ThumbnailInfo struct {
	Size     string `json:"size" example:"150x150"`
	URL      string `json:"url" example:"/api/v1/files/abc123/thumbnail?size=150x150"`
	Width    int    `json:"width" example:"150"`
	Height   int    `json:"height" example:"150"`
	FileSize int64  `json:"file_size" example:"25600"`
}

// FileInfo represents file information in listings
type FileInfo struct {
	FileID      string                 `json:"file_id" example:"user_123_profile_abc123.jpg"`
	FileName    string                 `json:"file_name" example:"profile.jpg"`
	FileSize    int64                  `json:"file_size" example:"1024000"`
	ContentType string                 `json:"content_type" example:"image/jpeg"`
	Category    string                 `json:"category" example:"profile"`
	UploadedAt  time.Time              `json:"uploaded_at" example:"2023-12-01T10:30:00Z"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ListFilesResponse represents the response structure for file listings
type ListFilesResponse struct {
	Files  []FileInfo `json:"files"`
	Total  int        `json:"total" example:"25"`
	Limit  int        `json:"limit" example:"50"`
	Offset int        `json:"offset" example:"0"`
}

// ErrorResponse represents error response structure
type ErrorResponse struct {
	Error   string `json:"error" example:"File not found"`
	Message string `json:"message,omitempty" example:"The requested file could not be found"`
	Code    int    `json:"code,omitempty" example:"404"`
}

// SuccessResponse represents success response structure
type SuccessResponse struct {
	Success bool        `json:"success" example:"true"`
	Message string      `json:"message" example:"Operation completed successfully"`
	Data    interface{} `json:"data,omitempty"`
}

// PresignedURLRequest represents a presigned URL request
type PresignedURLRequest struct {
	FileKey     string        `json:"file_key" example:"cat/cat/123/photo/image.jpg"`
	UserID      string        `json:"user_id" example:"demo-user-123"`
	Expires     time.Duration `json:"expires" example:"3600000000000"` // 1 hour in nanoseconds
	Action      string        `json:"action" example:"PUT"`            // "GET", "PUT", "DELETE"
	ContentType string        `json:"content_type,omitempty" example:"image/jpeg"`
	MaxSize     int64         `json:"max_size,omitempty" example:"5242880"` // 5MB
}

// PresignedURLResponse represents a presigned URL response
type PresignedURLResponse struct {
	Success   bool      `json:"success" example:"true"`
	URL       string    `json:"url" example:"https://localhost:9000/bucket/file?X-Amz-Algorithm=..."`
	ExpiresAt time.Time `json:"expires_at" example:"2023-12-01T11:30:00Z"`
	FileKey   string    `json:"file_key" example:"cat/cat/123/photo/image.jpg"`
}

// BatchPresignedURLRequest represents a batch presigned URL request
type BatchPresignedURLRequest struct {
	Files  []PresignedURLRequest `json:"files"`
	UserID string                `json:"user_id" example:"demo-user-123"`
}

// BatchPresignedURLResponse represents a batch presigned URL response
type BatchPresignedURLResponse struct {
	Success      bool                   `json:"success" example:"true"`
	URLs         []PresignedURLResponse `json:"urls"`
	SuccessCount int                    `json:"success_count" example:"3"`
	TotalCount   int                    `json:"total_count" example:"3"`
}

// BatchUploadRequest represents a batch upload request
type BatchUploadRequest struct {
	Files  []BatchFile `json:"files"`
	UserID string      `json:"user_id" example:"demo-user-123"`
}

// BatchFile represents a file in batch operations
type BatchFile struct {
	FileName    string                 `json:"file_name" example:"image1.jpg"`
	ContentType string                 `json:"content_type" example:"image/jpeg"`
	FileSize    int64                  `json:"file_size" example:"1024000"`
	Category    string                 `json:"category" example:"photo"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// BatchUploadResponse represents a batch upload response
type BatchUploadResponse struct {
	Success      bool             `json:"success" example:"true"`
	Results      []UploadResponse `json:"results"`
	SuccessCount int              `json:"success_count" example:"2"`
	TotalCount   int              `json:"total_count" example:"3"`
}

// BatchDownloadRequest represents a batch download request
type BatchDownloadRequest struct {
	FileKeys []string `json:"file_keys" example:"cat/cat/123/photo/image1.jpg,cat/cat/123/photo/image2.jpg"`
	UserID   string   `json:"user_id" example:"demo-user-123"`
}

// BatchDownloadResponse represents a batch download response
type BatchDownloadResponse struct {
	Success      bool       `json:"success" example:"true"`
	Results      []FileInfo `json:"results"`
	SuccessCount int        `json:"success_count" example:"2"`
	TotalCount   int        `json:"total_count" example:"3"`
}

// BatchDeleteRequest represents a batch delete request
type BatchDeleteRequest struct {
	FileKeys []string `json:"file_keys" example:"cat/cat/123/photo/image1.jpg,cat/cat/123/photo/image2.jpg"`
	UserID   string   `json:"user_id" example:"demo-user-123"`
}

// BatchDeleteResponse represents a batch delete response
type BatchDeleteResponse struct {
	Success      bool           `json:"success" example:"true"`
	Results      []DeleteResult `json:"results"`
	SuccessCount int            `json:"success_count" example:"2"`
	TotalCount   int            `json:"total_count" example:"3"`
}

// DeleteResult represents a single delete operation result
type DeleteResult struct {
	FileKey string `json:"file_key" example:"cat/cat/123/photo/image1.jpg"`
	Success bool   `json:"success" example:"true"`
	Error   string `json:"error,omitempty" example:"File not found"`
}

// PerformanceStats represents performance monitoring statistics
type PerformanceStats struct {
	TotalOperations int64                  `json:"total_operations" example:"1250"`
	SuccessRate     float64                `json:"success_rate" example:"0.95"`
	AvgLatency      float64                `json:"avg_latency_ms" example:"125.5"`
	Throughput      ThroughputStats        `json:"throughput"`
	OperationStats  map[string]interface{} `json:"operation_stats"`
	Uptime          float64                `json:"uptime_seconds" example:"3600.0"`
}

// ThroughputStats represents throughput statistics
type ThroughputStats struct {
	FilesProcessed int64   `json:"files_processed" example:"500"`
	BytesProcessed int64   `json:"bytes_processed" example:"52428800"`
	FilesPerSecond float64 `json:"files_per_second" example:"0.14"`
	BytesPerSecond float64 `json:"bytes_per_second" example:"14563.56"`
}

func main() {
	// Initialize storage registry
	initStorage()

	// Setup Gin router
	router := gin.Default()

	// CORS middleware
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-User-ID")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// API routes
	api := router.Group("/api/v1")
	{
		// Health check
		api.GET("/health", healthCheck)

		// Cat file operations
		cats := api.Group("/cats")
		{
			cats.POST("/:id/upload", uploadCatFile)
			cats.GET("/:id/files/*fileId", getCatFile)
			cats.DELETE("/:id/files/*fileId", deleteCatFile)
			cats.GET("/:id/files", listCatFiles)

			// Cat presigned URLs
			cats.POST("/:id/presigned/upload", generateCatPresignedUploadURL)
			cats.POST("/:id/presigned/download", generateCatPresignedDownloadURL)

			// Cat batch operations
			cats.POST("/:id/batch/upload", batchUploadCatFiles)
			cats.POST("/:id/batch/download", batchDownloadCatFiles)
			cats.POST("/:id/batch/delete", batchDeleteCatFiles)
		}

		// Dog file operations
		dogs := api.Group("/dogs")
		{
			dogs.POST("/:id/upload", uploadDogFile)
			dogs.GET("/:id/files/*fileId", getDogFile)
			dogs.DELETE("/:id/files/*fileId", deleteDogFile)
			dogs.GET("/:id/files", listDogFiles)

			// Dog presigned URLs
			dogs.POST("/:id/presigned/upload", generateDogPresignedUploadURL)
			dogs.POST("/:id/presigned/download", generateDogPresignedDownloadURL)

			// Dog batch operations
			dogs.POST("/:id/batch/upload", batchUploadDogFiles)
			dogs.POST("/:id/batch/download", batchDownloadDogFiles)
			dogs.POST("/:id/batch/delete", batchDeleteDogFiles)
		}

		// File preview operations
		files := api.Group("/files")
		{
			files.GET("/:fileId/preview", previewFile)
			files.GET("/:fileId/thumbnail", getThumbnail)
			files.GET("/:fileId/stream", streamFile)
		}
	}

	// Start server
	log.Println("🐱🐶 Starting Cat & Dog Photo Storage API Server on :8080")
	log.Println("🔍 Health Check: http://localhost:8080/api/v1/health")
	log.Println("")
	log.Println("🐱 Cat Operations:")
	log.Println("  Upload: POST /api/v1/cats/{id}/upload")
	log.Println("  Download: GET /api/v1/cats/{id}/files/{fileId}")
	log.Println("  Delete: DELETE /api/v1/cats/{id}/files/{fileId}")
	log.Println("  List: GET /api/v1/cats/{id}/files")
	log.Println("  Presigned Upload: POST /api/v1/cats/{id}/presigned/upload")
	log.Println("  Presigned Download: POST /api/v1/cats/{id}/presigned/download")
	log.Println("  Batch Upload: POST /api/v1/cats/{id}/batch/upload")
	log.Println("  Batch Download: POST /api/v1/cats/{id}/batch/download")
	log.Println("  Batch Delete: POST /api/v1/cats/{id}/batch/delete")
	log.Println("")
	log.Println("🐶 Dog Operations:")
	log.Println("  Upload: POST /api/v1/dogs/{id}/upload")
	log.Println("  Download: GET /api/v1/dogs/{id}/files/{fileId}")
	log.Println("  Delete: DELETE /api/v1/dogs/{id}/files/{fileId}")
	log.Println("  List: GET /api/v1/dogs/{id}/files")
	log.Println("  Presigned Upload: POST /api/v1/dogs/{id}/presigned/upload")
	log.Println("  Presigned Download: POST /api/v1/dogs/{id}/presigned/download")
	log.Println("  Batch Upload: POST /api/v1/dogs/{id}/batch/upload")
	log.Println("  Batch Download: POST /api/v1/dogs/{id}/batch/download")
	log.Println("  Batch Delete: POST /api/v1/dogs/{id}/batch/delete")
	router.Run(":8080")
}

// initStorage initializes the storage registry
func initStorage() {
	storageRegistry = registry.NewRegistry()

	// Initialize MinIO connection with performance optimizations
	err := storageRegistry.Initialize(config.StorageConfig{
		Endpoint:        "localhost:9000",
		AccessKey:       "minioadmin",
		SecretKey:       "minioadmin",
		UseSSL:          false,
		Region:          "us-east-1",
		BucketName:      "myapp-storage",
		MaxFileSize:     25 * 1024 * 1024, // 25MB
		UploadTimeout:   300,              // 5 minutes
		DownloadTimeout: 60,               // 1 minute

		// Performance optimization settings
		MaxConnections:    100, // Max concurrent connections
		ConnectionTimeout: 30,  // 30 seconds
		RequestTimeout:    60,  // 60 seconds
		RetryAttempts:     3,   // 3 retry attempts
		RetryDelay:        100, // 100ms delay between retries
	})
	if err != nil {
		log.Printf("⚠️  Warning: Failed to initialize storage (MinIO not running): %v", err)
		log.Println("💡 The API will work in demo mode without actual file storage")
		// Continue without storage for demo purposes
		return
	}

	// Example metadata storage (in-memory for demo)
	metadataStorage = &ExampleMetadataStorage{
		metadata: make(map[string]*interfaces.FileMetadata),
	}

	// Register cat storage handler with metadata callback
	_, err = storageRegistry.Register("cat", &handler.HandlerConfig{
		BasePath: "cat",
		Categories: map[string]category.CategoryConfig{
			"photo": {
				BucketSuffix: "images",
				IsPublic:     false,
				MaxSize:      5 * 1024 * 1024,
				AllowedTypes: []string{"image/jpeg", "image/png", "image/webp"},
				Validation: category.ValidationConfig{
					MaxFileSize:       5 * 1024 * 1024,
					MinFileSize:       1024,
					AllowedTypes:      []string{"image/jpeg", "image/png", "image/webp"},
					AllowedExtensions: []string{".jpg", ".jpeg", ".png", ".webp"},
					ImageValidation: &category.ImageValidationConfig{
						MinWidth:           100,
						MaxWidth:           2048,
						MinHeight:          100,
						MaxHeight:          2048,
						MinQuality:         60,
						MaxQuality:         95,
						AllowedFormats:     []string{"jpeg", "png", "webp"},
						MinAspectRatio:     0.5,
						MaxAspectRatio:     2.0,
						AllowedColorSpaces: []string{"RGB", "RGBA"},
					},
				},
				Security: middleware.SecurityConfig{
					RequireAuth:       true,
					RequireOwner:      true,
					GenerateThumbnail: true,
				},
				Preview: category.PreviewConfig{
					GenerateThumbnails: true,
					ThumbnailSizes:     []string{"150x150", "300x300", "600x600"},
					EnablePreview:      true,
					PreviewFormats:     []string{"image"},
				},
			},
			"thumbnail": {
				BucketSuffix: "thumbnails",
				IsPublic:     true,            // Thumbnails are typically public
				MaxSize:      1 * 1024 * 1024, // 1MB for thumbnails
				AllowedTypes: []string{"image/jpeg", "image/png"},
				Validation: category.ValidationConfig{
					MaxFileSize:  1 * 1024 * 1024,
					MinFileSize:  1024, // 1KB minimum
					AllowedTypes: []string{"image/jpeg", "image/png"},
				},
				Security: middleware.SecurityConfig{
					RequireAuth:  false, // Thumbnails are public
					RequireOwner: false,
				},
				Preview: category.PreviewConfig{
					GenerateThumbnails: false, // Don't generate thumbnails of thumbnails
					EnablePreview:      true,
					PreviewFormats:     []string{"image"},
				},
			},
		},
		// Set the metadata callback
		MetadataCallback: metadataStorage.StoreFileMetadata,
	})
	if err != nil {
		log.Printf("⚠️  Warning: Failed to register cat storage: %v", err)
	}

	// Register dog storage handler with metadata callback
	_, err = storageRegistry.Register("dog", &handler.HandlerConfig{
		BasePath: "dog",
		Categories: map[string]category.CategoryConfig{
			"photo": {
				BucketSuffix: "images",
				IsPublic:     false,
				MaxSize:      5 * 1024 * 1024,
				AllowedTypes: []string{"image/jpeg", "image/png", "image/webp"},
				Validation: category.ValidationConfig{
					MaxFileSize:       5 * 1024 * 1024,
					MinFileSize:       1024,
					AllowedTypes:      []string{"image/jpeg", "image/png", "image/webp"},
					AllowedExtensions: []string{".jpg", ".jpeg", ".png", ".webp"},
					ImageValidation: &category.ImageValidationConfig{
						MinWidth:           100,
						MaxWidth:           2048,
						MinHeight:          100,
						MaxHeight:          2048,
						MinQuality:         60,
						MaxQuality:         95,
						AllowedFormats:     []string{"jpeg", "png", "webp"},
						MinAspectRatio:     0.5,
						MaxAspectRatio:     2.0,
						AllowedColorSpaces: []string{"RGB", "RGBA"},
					},
				},
				Security: middleware.SecurityConfig{
					RequireAuth:       true,
					RequireOwner:      true,
					GenerateThumbnail: true,
				},
				Preview: category.PreviewConfig{
					GenerateThumbnails: true,
					ThumbnailSizes:     []string{"150x150", "300x300", "600x600"},
					EnablePreview:      true,
					PreviewFormats:     []string{"image"},
				},
			},
			"thumbnail": {
				BucketSuffix: "thumbnails",
				IsPublic:     true,            // Thumbnails are typically public
				MaxSize:      1 * 1024 * 1024, // 1MB for thumbnails
				AllowedTypes: []string{"image/jpeg", "image/png"},
				Validation: category.ValidationConfig{
					MaxFileSize:  1 * 1024 * 1024,
					MinFileSize:  1024, // 1KB minimum
					AllowedTypes: []string{"image/jpeg", "image/png"},
				},
				Security: middleware.SecurityConfig{
					RequireAuth:  false, // Thumbnails are public
					RequireOwner: false,
				},
				Preview: category.PreviewConfig{
					GenerateThumbnails: false, // Don't generate thumbnails of thumbnails
					EnablePreview:      true,
					PreviewFormats:     []string{"image"},
				},
			},
		},
		// Set the metadata callback
		MetadataCallback: metadataStorage.StoreFileMetadata,
	})
	if err != nil {
		log.Printf("⚠️  Warning: Failed to register dog storage: %v", err)
	}

	log.Println("✅ Storage registry initialized successfully")
}

// HealthCheck godoc
// @Summary      Health Check
// @Description  Check the health status of the storage API service
// @Tags         System
// @Accept       json
// @Produce      json
// @Success      200 {object} map[string]interface{} "Service is healthy"
// @Failure      503 {object} map[string]interface{} "Service is unhealthy"
// @Router       /health [get]
func healthCheck(c *gin.Context) {
	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"error":  "storage not initialized",
			"mode":   "demo",
		})
		return
	}

	// Perform health check
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if err := storageRegistry.HealthCheck(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"error":  err.Error(),
			"mode":   "demo",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
		"mode":   "production",
	})
}

func uploadCatFile(c *gin.Context) {
	catID := c.Param("id")

	// Get file from form
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}

	// Open file
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer src.Close()

	// Check if storage is available
	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage not available",
			"mode":  "demo",
		})
		return
	}

	// Get cat storage handler
	catHandler, err := storageRegistry.GetHandler("cat")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage not available"})
		return
	}

	// Create upload request
	uploadReq := &interfaces.UploadRequest{
		FileData:    src,
		FileSize:    file.Size,
		ContentType: file.Header.Get("Content-Type"),
		FileName:    file.Filename,
		Category:    "photo",
		EntityType:  "cat",
		EntityID:    catID,
		UserID:      getCurrentUserID(c),
		Metadata: map[string]interface{}{
			"original_filename": file.Filename,
			"upload_source":     "web",
		},
	}

	// Upload file
	response, err := catHandler.Upload(c.Request.Context(), uploadReq)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Show stored metadata if available
	var metadataInfo interface{}
	if metadata, err := metadataStorage.GetFileMetadata(response.FileKey); err == nil {
		metadataInfo = map[string]interface{}{
			"id":          metadata.ID,
			"file_name":   metadata.FileName,
			"file_key":    metadata.FileKey,
			"category":    metadata.Category,
			"entity_type": metadata.EntityType,
			"entity_id":   metadata.EntityID,
			"uploaded_by": metadata.UploadedBy,
			"uploaded_at": metadata.UploadedAt,
			"is_public":   metadata.IsPublic,
			"thumbnails":  metadata.Thumbnails,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"data":     response,
		"message":  "Cat photo uploaded successfully",
		"metadata": metadataInfo,
	})
}

// UploadDogFile handles dog photo uploads
// @Summary      Upload Dog Photo
// @Description  Upload a photo for a specific dog with validation and thumbnail generation
// @Tags         Dogs
// @Accept       multipart/form-data
// @Produce      json
// @Param        id   path      string  true  "Dog ID"
// @Param        file formData  file    true  "Dog photo file (JPEG, PNG, WebP)"
// @Success      200  {object}  SuccessResponse{data=UploadResponse}  "File uploaded successfully"
// @Failure      400  {object}  map[string]interface{}  "Bad request - validation error"
// @Failure      401  {object}  map[string]interface{}  "Unauthorized"
// @Failure      413  {object}  map[string]interface{}  "File too large"
// @Failure      503  {object}  map[string]interface{}  "Service unavailable"
// @Security     ApiKeyAuth
// @Router       /dogs/{id}/upload [post]
func uploadDogFile(c *gin.Context) {
	dogID := c.Param("id")

	// Get file from form
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}

	// Open file
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer src.Close()

	// Check if storage is available
	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage not available",
			"mode":  "demo",
		})
		return
	}

	// Get dog storage handler
	dogHandler, err := storageRegistry.GetHandler("dog")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage not available"})
		return
	}

	// Create upload request
	uploadReq := &interfaces.UploadRequest{
		FileData:    src,
		FileSize:    file.Size,
		ContentType: file.Header.Get("Content-Type"),
		FileName:    file.Filename,
		Category:    "photo",
		EntityType:  "dog",
		EntityID:    dogID,
		UserID:      getCurrentUserID(c),
		Metadata: map[string]interface{}{
			"original_filename": file.Filename,
			"upload_source":     "web",
		},
	}

	// Upload file
	response, err := dogHandler.Upload(c.Request.Context(), uploadReq)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Show stored metadata if available
	var metadataInfo interface{}
	if metadata, err := metadataStorage.GetFileMetadata(response.FileKey); err == nil {
		metadataInfo = map[string]interface{}{
			"id":          metadata.ID,
			"file_name":   metadata.FileName,
			"file_key":    metadata.FileKey,
			"category":    metadata.Category,
			"entity_type": metadata.EntityType,
			"entity_id":   metadata.EntityID,
			"uploaded_by": metadata.UploadedBy,
			"uploaded_at": metadata.UploadedAt,
			"is_public":   metadata.IsPublic,
			"thumbnails":  metadata.Thumbnails,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"data":     response,
		"message":  "Dog photo uploaded successfully",
		"metadata": metadataInfo,
	})
}

// GetCatFile handles cat file downloads
// @Summary      Download Cat File
// @Description  Download a file belonging to a specific cat. Accepts either the metadata ID or the file key from upload response.
// @Tags         Cats
// @Accept       json
// @Produce      application/octet-stream
// @Param        id     path      string  true  "Cat ID"
// @Param        fileId path      string  true  "File identifier - can be either metadata ID (e.g., '9498ac3a-57b3-4a6e-9dad-9ec915ffa1b9') or file key (e.g., 'cat/cat/123/photo/1757314047_50a7135e-6612-48ff-b3a6-16f3f1595cbe.png')"
// @Success      200    {file}    file    "File content"
// @Failure      401    {object}  map[string]interface{}  "Unauthorized"
// @Failure      403    {object}  map[string]interface{}  "Forbidden - access denied"
// @Failure      404    {object}  map[string]interface{}  "File not found"
// @Failure      503    {object}  map[string]interface{}  "Service unavailable"
// @Security     ApiKeyAuth
// @Router       /cats/{id}/files/{fileId} [get]
func getCatFile(c *gin.Context) {
	fileID := c.Param("fileId")

	// Remove leading slash from wildcard parameter
	if len(fileID) > 0 && fileID[0] == '/' {
		fileID = fileID[1:]
	}

	// URL decode the file ID in case it contains encoded characters
	decodedFileID, err := url.QueryUnescape(fileID)
	if err != nil {
		// If decoding fails, use the original
		decodedFileID = fileID
	}

	// Check if storage is available
	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage not available",
			"mode":  "demo",
		})
		return
	}

	// Get cat storage handler
	catHandler, err := storageRegistry.GetHandler("cat")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage not available"})
		return
	}

	// Try to find the file key by metadata ID first
	fileKey := decodedFileID
	if metadata, err := metadataStorage.GetFileMetadata(decodedFileID); err == nil {
		// Use the file key from metadata
		fileKey = metadata.FileKey
	} else {
		// If not found in metadata, assume the fileID is actually the file key
		// This allows direct download using the file key from upload response
		fileKey = decodedFileID
	}

	// Create download request
	downloadReq := &interfaces.DownloadRequest{
		FileKey: fileKey,
		UserID:  getCurrentUserID(c),
	}

	// Download file
	response, err := catHandler.Download(c.Request.Context(), downloadReq)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":          "File not found",
			"message":        "Make sure you're using the correct file key from the upload response",
			"file_key_used":  fileKey,
			"original_param": fileID,
			"decoded_param":  decodedFileID,
			"hint":           "Use the 'file_key' field from the upload response, not the metadata 'id'",
		})
		return
	}

	// Set headers
	c.Header("Content-Type", response.ContentType)
	c.Header("Content-Length", strconv.FormatInt(response.FileSize, 10))

	// Stream file
	io.Copy(c.Writer, response.FileData)
}

// DeleteCatFile handles cat file deletion
// @Summary      Delete Cat File
// @Description  Delete a file belonging to a specific cat. Accepts either the metadata ID or the file key from upload response.
// @Tags         Cats
// @Accept       json
// @Produce      json
// @Param        id     path      string  true  "Cat ID"
// @Param        fileId path      string  true  "File identifier - can be either metadata ID (e.g., '9498ac3a-57b3-4a6e-9dad-9ec915ffa1b9') or file key (e.g., 'cat/cat/123/photo/1757314047_50a7135e-6612-48ff-b3a6-16f3f1595cbe.png')"
// @Success      200    {object}  SuccessResponse  "File deleted successfully"
// @Failure      401    {object}  map[string]interface{}  "Unauthorized"
// @Failure      403    {object}  map[string]interface{}  "Forbidden - access denied"
// @Failure      404    {object}  map[string]interface{}  "File not found"
// @Failure      500    {object}  map[string]interface{}  "Internal server error"
// @Failure      503    {object}  map[string]interface{}  "Service unavailable"
// @Security     ApiKeyAuth
// @Router       /cats/{id}/files/{fileId} [delete]
func deleteCatFile(c *gin.Context) {
	fileID := c.Param("fileId")

	// Remove leading slash from wildcard parameter
	if len(fileID) > 0 && fileID[0] == '/' {
		fileID = fileID[1:]
	}

	// Check if storage is available
	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage not available",
			"mode":  "demo",
		})
		return
	}

	// Get cat storage handler
	catHandler, err := storageRegistry.GetHandler("cat")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage not available"})
		return
	}

	// Create delete request
	deleteReq := &interfaces.DeleteRequest{
		FileKey: fileID,
		UserID:  getCurrentUserID(c),
	}

	// Delete file
	err = catHandler.Delete(c.Request.Context(), deleteReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Cat file deleted successfully",
	})
}

// ListCatFiles handles cat file listing
// @Summary      List Cat Files
// @Description  List all files belonging to a specific cat
// @Tags         Cats
// @Accept       json
// @Produce      json
// @Param        id     path      string  true   "Cat ID"
// @Param        limit  query     int     false  "Maximum number of files to return (default: 50)"
// @Param        offset query     int     false  "Number of files to skip (default: 0)"
// @Success      200    {object}  SuccessResponse{data=ListFilesResponse}  "List of cat files"
// @Failure      401    {object}  map[string]interface{}  "Unauthorized"
// @Failure      403    {object}  map[string]interface{}  "Forbidden - access denied"
// @Failure      500    {object}  map[string]interface{}  "Internal server error"
// @Failure      503    {object}  map[string]interface{}  "Service unavailable"
// @Security     ApiKeyAuth
// @Router       /cats/{id}/files [get]
func listCatFiles(c *gin.Context) {
	catID := c.Param("id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	// Check if storage is available
	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage not available",
			"mode":  "demo",
		})
		return
	}

	// Get cat storage handler
	catHandler, err := storageRegistry.GetHandler("cat")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage not available"})
		return
	}

	// Create list request
	listReq := &interfaces.ListRequest{
		EntityType: "cat",
		EntityID:   catID,
		Category:   "photo",
		UserID:     getCurrentUserID(c),
		Limit:      limit,
		Offset:     offset,
	}

	// List files
	response, err := catHandler.ListFiles(c.Request.Context(), listReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
	})
}

// Dog file operations
// @Summary      Download Dog File
// @Description  Download a file belonging to a specific dog. Accepts either the metadata ID or the file key from upload response.
// @Tags         Dogs
// @Accept       json
// @Produce      application/octet-stream
// @Param        id     path      string  true  "Dog ID"
// @Param        fileId path      string  true  "File identifier - can be either metadata ID (e.g., '9498ac3a-57b3-4a6e-9dad-9ec915ffa1b9') or file key (e.g., 'dog/dog/456/photo/1757314047_50a7135e-6612-48ff-b3a6-16f3f1595cbe.png')"
// @Success      200    {file}    file    "File content"
// @Failure      401    {object}  map[string]interface{}  "Unauthorized"
// @Failure      403    {object}  map[string]interface{}  "Forbidden - access denied"
// @Failure      404    {object}  map[string]interface{}  "File not found"
// @Failure      503    {object}  map[string]interface{}  "Service unavailable"
// @Security     ApiKeyAuth
// @Router       /dogs/{id}/files/{fileId} [get]
func getDogFile(c *gin.Context) {
	fileID := c.Param("fileId")

	// Remove leading slash from wildcard parameter
	if len(fileID) > 0 && fileID[0] == '/' {
		fileID = fileID[1:]
	}

	// URL decode the file ID in case it contains encoded characters
	decodedFileID, err := url.QueryUnescape(fileID)
	if err != nil {
		// If decoding fails, use the original
		decodedFileID = fileID
	}

	// Check if storage is available
	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage not available",
			"mode":  "demo",
		})
		return
	}

	// Get dog storage handler
	dogHandler, err := storageRegistry.GetHandler("dog")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage not available"})
		return
	}

	// Try to find the file key by metadata ID first
	fileKey := decodedFileID
	if metadata, err := metadataStorage.GetFileMetadata(decodedFileID); err == nil {
		// Use the file key from metadata
		fileKey = metadata.FileKey
	} else {
		// If not found in metadata, assume the fileID is actually the file key
		// This allows direct download using the file key from upload response
		fileKey = decodedFileID
	}

	// Create download request
	downloadReq := &interfaces.DownloadRequest{
		FileKey: fileKey,
		UserID:  getCurrentUserID(c),
	}

	// Debug logging
	fmt.Printf("🔍 Download Debug Info:\n")
	fmt.Printf("  Original param: %s\n", fileID)
	fmt.Printf("  Decoded param: %s\n", decodedFileID)
	fmt.Printf("  File key used: %s\n", fileKey)
	fmt.Printf("  User ID: %s\n", getCurrentUserID(c))

	// Download file
	response, err := dogHandler.Download(c.Request.Context(), downloadReq)
	if err != nil {
		fmt.Printf("❌ Download failed: %v\n", err)
		c.JSON(http.StatusNotFound, gin.H{
			"error":          "File not found",
			"message":        "Make sure you're using the correct file key from the upload response",
			"file_key_used":  fileKey,
			"original_param": fileID,
			"decoded_param":  decodedFileID,
			"hint":           "Use the 'file_key' field from the upload response, not the metadata 'id'",
			"debug_error":    err.Error(),
		})
		return
	}

	fmt.Printf("✅ Download successful: %s\n", fileKey)

	// Set headers
	c.Header("Content-Type", response.ContentType)
	c.Header("Content-Length", strconv.FormatInt(response.FileSize, 10))

	// Stream file
	io.Copy(c.Writer, response.FileData)
}

// @Summary      Delete Dog File
// @Description  Delete a file belonging to a specific dog. Accepts either the metadata ID or the file key from upload response.
// @Tags         Dogs
// @Accept       json
// @Produce      json
// @Param        id     path      string  true  "Dog ID"
// @Param        fileId path      string  true  "File identifier - can be either metadata ID (e.g., '9498ac3a-57b3-4a6e-9dad-9ec915ffa1b9') or file key (e.g., 'dog/dog/456/photo/1757314047_50a7135e-6612-48ff-b3a6-16f3f1595cbe.png')"
// @Success      200    {object}  SuccessResponse  "File deleted successfully"
// @Failure      401    {object}  map[string]interface{}  "Unauthorized"
// @Failure      403    {object}  map[string]interface{}  "Forbidden - access denied"
// @Failure      404    {object}  map[string]interface{}  "File not found"
// @Failure      500    {object}  map[string]interface{}  "Internal server error"
// @Failure      503    {object}  map[string]interface{}  "Service unavailable"
// @Security     ApiKeyAuth
// @Router       /dogs/{id}/files/{fileId} [delete]
func deleteDogFile(c *gin.Context) {
	fileID := c.Param("fileId")

	// Remove leading slash from wildcard parameter
	if len(fileID) > 0 && fileID[0] == '/' {
		fileID = fileID[1:]
	}

	// Check if storage is available
	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage not available",
			"mode":  "demo",
		})
		return
	}

	// Get dog storage handler
	dogHandler, err := storageRegistry.GetHandler("dog")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage not available"})
		return
	}

	// Create delete request
	deleteReq := &interfaces.DeleteRequest{
		FileKey: fileID,
		UserID:  getCurrentUserID(c),
	}

	// Delete file
	err = dogHandler.Delete(c.Request.Context(), deleteReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Dog file deleted successfully",
	})
}

// @Summary      List Dog Files
// @Description  List all files belonging to a specific dog
// @Tags         Dogs
// @Accept       json
// @Produce      json
// @Param        id     path      string  true   "Dog ID"
// @Param        limit  query     int     false  "Maximum number of files to return (default: 50)"
// @Param        offset query     int     false  "Number of files to skip (default: 0)"
// @Success      200    {object}  SuccessResponse{data=ListFilesResponse}  "List of dog files"
// @Failure      401    {object}  map[string]interface{}  "Unauthorized"
// @Failure      403    {object}  map[string]interface{}  "Forbidden - access denied"
// @Failure      500    {object}  map[string]interface{}  "Internal server error"
// @Failure      503    {object}  map[string]interface{}  "Service unavailable"
// @Security     ApiKeyAuth
// @Router       /dogs/{id}/files [get]
func listDogFiles(c *gin.Context) {
	dogID := c.Param("id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	// Check if storage is available
	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage not available",
			"mode":  "demo",
		})
		return
	}

	// Get dog storage handler
	dogHandler, err := storageRegistry.GetHandler("dog")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage not available"})
		return
	}

	// Create list request
	listReq := &interfaces.ListRequest{
		EntityType: "dog",
		EntityID:   dogID,
		Category:   "photo",
		UserID:     getCurrentUserID(c),
		Limit:      limit,
		Offset:     offset,
	}

	// List files
	response, err := dogHandler.ListFiles(c.Request.Context(), listReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
	})
}

// File preview endpoints
// @Summary      Preview File
// @Description  Get a preview of a file with specified size (not implemented in demo)
// @Tags         Files
// @Accept       json
// @Produce      json
// @Param        fileId path      string  true   "File ID"
// @Param        size   query     string  false  "Preview size (default: 300x300)"
// @Success      200    {object}  map[string]interface{}  "Not implemented in demo"
// @Router       /files/{fileId}/preview [get]
func previewFile(c *gin.Context) {
	fileID := c.Param("fileId")
	size := c.DefaultQuery("size", "300x300")

	c.JSON(http.StatusOK, gin.H{
		"message": "File preview endpoint",
		"file_id": fileID,
		"size":    size,
		"status":  "not implemented in demo",
	})
}

// @Summary      Get File Thumbnail
// @Description  Get a thumbnail of a file with specified size
// @Tags         Files
// @Accept       json
// @Produce      json
// @Param        fileId path      string  true   "File ID"
// @Param        size   query     string  false  "Thumbnail size (default: 150x150)"
// @Success      200    {object}  map[string]interface{}  "Thumbnail information"
// @Failure      404    {object}  map[string]interface{}  "File or thumbnail not found"
// @Failure      500    {object}  map[string]interface{}  "Internal server error"
// @Router       /files/{fileId}/thumbnail [get]
func getThumbnail(c *gin.Context) {
	fileID := c.Param("fileId")
	size := c.DefaultQuery("size", "150x150")

	// In demo mode, return a mock response with proper structure
	if storageRegistry == nil {
		c.JSON(http.StatusOK, gin.H{
			"success":       true,
			"message":       "Thumbnail endpoint (demo mode)",
			"file_id":       fileID,
			"size":          size,
			"thumbnail_url": fmt.Sprintf("/api/v1/files/%s/thumbnail?size=%s", fileID, size),
			"status":        "demo_mode",
		})
		return
	}

	// Try to get thumbnail from both cat and dog handlers
	var thumbnailURL string

	// Try cat handler first
	catHandler, err := storageRegistry.GetHandler("cat")
	if err == nil && catHandler != nil {
		req := &interfaces.ThumbnailRequest{
			FileKey: fileID,
			Size:    size,
		}
		resp, err := catHandler.Thumbnail(c.Request.Context(), req)
		if err == nil && resp.Success {
			thumbnailURL = resp.ThumbnailURL
		}
	}

	// If not found in cat handler, try dog handler
	if thumbnailURL == "" {
		dogHandler, err := storageRegistry.GetHandler("dog")
		if err == nil && dogHandler != nil {
			req := &interfaces.ThumbnailRequest{
				FileKey: fileID,
				Size:    size,
			}
			resp, err := dogHandler.Thumbnail(c.Request.Context(), req)
			if err == nil && resp.Success {
				thumbnailURL = resp.ThumbnailURL
			}
		}
	}

	// If still not found, return error
	if thumbnailURL == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Thumbnail not found",
			"file_id": fileID,
			"size":    size,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"file_id":       fileID,
		"size":          size,
		"thumbnail_url": thumbnailURL,
		"content_type":  "image/jpeg",
	})
}

// @Summary      Stream File
// @Description  Stream a file with range support for partial content (not implemented in demo)
// @Tags         Files
// @Accept       json
// @Produce      application/octet-stream
// @Param        fileId path      string  true  "File ID"
// @Param        Range  header    string  false "Range header for partial content"
// @Success      200    {file}    file    "File content"
// @Success      206    {file}    file    "Partial file content"
// @Router       /files/{fileId}/stream [get]
func streamFile(c *gin.Context) {
	fileID := c.Param("fileId")
	rangeHeader := c.GetHeader("Range")

	c.JSON(http.StatusOK, gin.H{
		"message": "File stream endpoint",
		"file_id": fileID,
		"range":   rangeHeader,
		"status":  "not implemented in demo",
	})
}

// Helper function to get current user ID (mock implementation)
func getCurrentUserID(c *gin.Context) string {
	// In a real implementation, extract from JWT token
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = "demo-user-123" // Default for demo
	}
	return userID
}

// ExampleMetadataStorage demonstrates how to implement metadata storage
// This is a simple in-memory example, but you can implement database, Redis, etc.
type ExampleMetadataStorage struct {
	metadata map[string]*interfaces.FileMetadata
}

// StoreFileMetadata implements the metadata callback
func (s *ExampleMetadataStorage) StoreFileMetadata(ctx context.Context, metadata *interfaces.FileMetadata) error {
	s.metadata[metadata.FileKey] = metadata
	fmt.Printf("✅ Stored metadata for file: %s\n", metadata.FileKey)
	return nil
}

// GetFileMetadata retrieves file metadata (for your own use)
func (s *ExampleMetadataStorage) GetFileMetadata(fileKey string) (*interfaces.FileMetadata, error) {
	metadata, exists := s.metadata[fileKey]
	if !exists {
		return nil, fmt.Errorf("file metadata not found")
	}
	return metadata, nil
}

// ListFileMetadata lists all file metadata (for your own use)
func (s *ExampleMetadataStorage) ListFileMetadata() []*interfaces.FileMetadata {
	var result []*interfaces.FileMetadata
	for _, metadata := range s.metadata {
		result = append(result, metadata)
	}
	return result
}

// convertThumbnails converts interfaces.ThumbnailInfo to ThumbnailInfo
func convertThumbnails(thumbnails []interfaces.ThumbnailInfo) []ThumbnailInfo {
	if thumbnails == nil {
		return []ThumbnailInfo{}
	}

	var result []ThumbnailInfo
	for _, thumb := range thumbnails {
		result = append(result, ThumbnailInfo{
			Size:     thumb.Size,
			URL:      thumb.URL,
			Width:    thumb.Width,
			Height:   thumb.Height,
			FileSize: thumb.FileSize,
		})
	}
	return result
}

// ============================================================================
// CAT PRESIGNED URL ENDPOINTS
// ============================================================================

// generateCatPresignedUploadURL generates a presigned URL for cat file upload
// @Summary      Generate presigned upload URL for cat
// @Description  Generate a presigned URL for direct cat file upload to MinIO
// @Tags         cats
// @Accept       json
// @Produce      json
// @Param        id path string true "Cat ID"
// @Param        request body PresignedURLRequest true "Presigned URL request"
// @Success      200 {object} PresignedURLResponse
// @Failure      400 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /cats/{id}/presigned/upload [post]
func generateCatPresignedUploadURL(c *gin.Context) {
	catID := c.Param("id")
	if catID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Missing cat ID",
			Message: "Cat ID is required",
		})
		return
	}

	var req PresignedURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request",
			Message: err.Error(),
		})
		return
	}

	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error:   "Storage not available",
			Message: "MinIO is not running",
		})
		return
	}

	// Generate file key for cat photo
	fileKey := fmt.Sprintf("cat/cat/%s/photo/%s", catID, req.FileKey)
	expiresAt := time.Now().Add(req.Expires)

	// For demo purposes, return a mock presigned URL
	presignedURL := fmt.Sprintf("https://localhost:9000/myapp-storage/%s?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=minioadmin&X-Amz-Date=%s&X-Amz-Expires=%d&X-Amz-SignedHeaders=host&X-Amz-Signature=mock-signature",
		fileKey,
		time.Now().Format("20060102T150405Z"),
		int(req.Expires.Seconds()),
	)

	c.JSON(http.StatusOK, PresignedURLResponse{
		Success:   true,
		URL:       presignedURL,
		ExpiresAt: expiresAt,
		FileKey:   fileKey,
	})
}

// generateCatPresignedDownloadURL generates a presigned URL for cat file download
// @Summary      Generate presigned download URL for cat
// @Description  Generate a presigned URL for direct cat file download from MinIO
// @Tags         cats
// @Accept       json
// @Produce      json
// @Param        id path string true "Cat ID"
// @Param        request body PresignedURLRequest true "Presigned URL request"
// @Success      200 {object} PresignedURLResponse
// @Failure      400 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /cats/{id}/presigned/download [post]
func generateCatPresignedDownloadURL(c *gin.Context) {
	catID := c.Param("id")
	if catID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Missing cat ID",
			Message: "Cat ID is required",
		})
		return
	}

	var req PresignedURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request",
			Message: err.Error(),
		})
		return
	}

	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error:   "Storage not available",
			Message: "MinIO is not running",
		})
		return
	}

	// Use the provided file key (should be the full path)
	fileKey := req.FileKey
	expiresAt := time.Now().Add(req.Expires)

	// For demo purposes, return a mock presigned URL
	presignedURL := fmt.Sprintf("https://localhost:9000/myapp-storage/%s?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=minioadmin&X-Amz-Date=%s&X-Amz-Expires=%d&X-Amz-SignedHeaders=host&X-Amz-Signature=mock-signature",
		fileKey,
		time.Now().Format("20060102T150405Z"),
		int(req.Expires.Seconds()),
	)

	c.JSON(http.StatusOK, PresignedURLResponse{
		Success:   true,
		URL:       presignedURL,
		ExpiresAt: expiresAt,
		FileKey:   fileKey,
	})
}

// generateBatchPresignedUploadURLs generates multiple presigned URLs for batch upload
// @Summary      Generate batch presigned upload URLs
// @Description  Generate multiple presigned URLs for batch file upload
// @Tags         presigned
// @Accept       json
// @Produce      json
// @Param        request body BatchPresignedURLRequest true "Batch presigned URL request"
// @Success      200 {object} BatchPresignedURLResponse
// @Failure      400 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /presigned/batch-upload [post]
func generateBatchPresignedUploadURLs(c *gin.Context) {
	var req BatchPresignedURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request",
			Message: err.Error(),
		})
		return
	}

	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error:   "Storage not available",
			Message: "MinIO is not running",
		})
		return
	}

	var urls []PresignedURLResponse
	successCount := 0

	for _, fileReq := range req.Files {
		expiresAt := time.Now().Add(fileReq.Expires)

		// Generate mock presigned URL
		presignedURL := fmt.Sprintf("https://localhost:9000/myapp-storage/%s?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=minioadmin&X-Amz-Date=%s&X-Amz-Expires=%d&X-Amz-SignedHeaders=host&X-Amz-Signature=mock-signature",
			fileReq.FileKey,
			time.Now().Format("20060102T150405Z"),
			int(fileReq.Expires.Seconds()),
		)

		urls = append(urls, PresignedURLResponse{
			Success:   true,
			URL:       presignedURL,
			ExpiresAt: expiresAt,
			FileKey:   fileReq.FileKey,
		})
		successCount++
	}

	c.JSON(http.StatusOK, BatchPresignedURLResponse{
		Success:      successCount > 0,
		URLs:         urls,
		SuccessCount: successCount,
		TotalCount:   len(req.Files),
	})
}

// ============================================================================
// CAT BATCH OPERATION ENDPOINTS
// ============================================================================

// batchUploadCatFiles handles batch cat file uploads
// @Summary      Batch cat file upload
// @Description  Upload multiple cat files in a single request
// @Tags         cats
// @Accept       multipart/form-data
// @Produce      json
// @Param        id path string true "Cat ID"
// @Param        files formData file true "Files to upload"
// @Param        user_id formData string true "User ID"
// @Success      200 {object} BatchUploadResponse
// @Failure      400 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /cats/{id}/batch/upload [post]
func batchUploadCatFiles(c *gin.Context) {
	catID := c.Param("id")
	if catID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Missing cat ID",
			Message: "Cat ID is required",
		})
		return
	}

	userID := c.PostForm("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Missing user_id",
			Message: "user_id is required",
		})
		return
	}

	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error:   "Storage not available",
			Message: "MinIO is not running",
		})
		return
	}

	// Get handler
	handler, err := storageRegistry.GetHandler("cat")
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Handler not found",
			Message: err.Error(),
		})
		return
	}

	// Parse multipart form
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid form data",
			Message: err.Error(),
		})
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "No files provided",
			Message: "At least one file is required",
		})
		return
	}

	// Convert to batch files
	var batchFiles []interfaces.BatchFile
	for _, file := range files {
		// Open file
		src, err := file.Open()
		if err != nil {
			continue
		}
		defer src.Close()

		batchFiles = append(batchFiles, interfaces.BatchFile{
			FileName:    file.Filename,
			ContentType: file.Header.Get("Content-Type"),
			FileSize:    file.Size,
			Category:    "photo",
			FileData:    src,
			Metadata: map[string]interface{}{
				"upload_source": "batch_upload",
				"original_name": file.Filename,
				"cat_id":        catID,
			},
		})
	}

	// Create batch upload request
	batchReq := &interfaces.BatchUploadRequest{
		Files:  batchFiles,
		UserID: userID,
	}

	// Perform batch upload
	response, err := handler.BatchUpload(c.Request.Context(), batchReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Batch upload failed",
			Message: err.Error(),
		})
		return
	}

	// Convert to response format - FIX: Use actual count from response
	var results []UploadResponse
	for i, result := range response.Results {
		if result != nil && result.Success && i < len(files) {
			results = append(results, UploadResponse{
				FileID:      result.FileKey,
				FileName:    files[i].Filename, // Use original filename
				FileSize:    result.FileSize,
				ContentType: result.ContentType,
				Category:    "photo",
				EntityType:  "cat",
				EntityID:    catID,
				UploadedAt:  time.Now(),
				Metadata:    result.Metadata,
				Thumbnails:  convertThumbnails(result.Thumbnails),
			})
		}
	}

	// FIX: Use actual counts from the results
	actualSuccessCount := len(results)
	actualTotalCount := len(files)

	c.JSON(http.StatusOK, BatchUploadResponse{
		Success:      actualSuccessCount > 0,
		Results:      results,
		SuccessCount: actualSuccessCount,
		TotalCount:   actualTotalCount,
	})
}

// batchDownloadCatFiles handles batch cat file downloads
// @Summary      Batch cat file download
// @Description  Download multiple cat files in a single request
// @Tags         cats
// @Accept       json
// @Produce      json
// @Param        id path string true "Cat ID"
// @Param        request body BatchDownloadRequest true "Batch download request"
// @Success      200 {object} BatchDownloadResponse
// @Failure      400 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /cats/{id}/batch/download [post]
func batchDownloadCatFiles(c *gin.Context) {
	catID := c.Param("id")
	if catID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Missing cat ID",
			Message: "Cat ID is required",
		})
		return
	}

	var req BatchDownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request",
			Message: err.Error(),
		})
		return
	}

	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error:   "Storage not available",
			Message: "MinIO is not running",
		})
		return
	}

	// Get handler
	handler, err := storageRegistry.GetHandler("cat")
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Handler not found",
			Message: err.Error(),
		})
		return
	}

	// Create batch download request
	batchReq := &interfaces.BatchGetRequest{
		FileKeys: req.FileKeys,
		UserID:   req.UserID,
	}

	// Perform batch download
	response, err := handler.BatchGet(c.Request.Context(), batchReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Batch download failed",
			Message: err.Error(),
		})
		return
	}

	// Convert to response format
	var results []FileInfo
	for i, result := range response.Results {
		if result != nil && result.Success && i < len(req.FileKeys) {
			results = append(results, FileInfo{
				FileID:      req.FileKeys[i],
				FileName:    req.FileKeys[i], // Use file key as filename
				FileSize:    result.FileSize,
				ContentType: result.ContentType,
				Category:    "photo",
				UploadedAt:  time.Now(),
				Metadata:    result.Metadata,
			})
		}
	}

	// Use actual counts
	actualSuccessCount := len(results)
	actualTotalCount := len(req.FileKeys)

	c.JSON(http.StatusOK, BatchDownloadResponse{
		Success:      actualSuccessCount > 0,
		Results:      results,
		SuccessCount: actualSuccessCount,
		TotalCount:   actualTotalCount,
	})
}

// batchDeleteCatFiles handles batch cat file deletions
// @Summary      Batch cat file delete
// @Description  Delete multiple cat files in a single request
// @Tags         cats
// @Accept       json
// @Produce      json
// @Param        id path string true "Cat ID"
// @Param        request body BatchDeleteRequest true "Batch delete request"
// @Success      200 {object} BatchDeleteResponse
// @Failure      400 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /cats/{id}/batch/delete [post]
func batchDeleteCatFiles(c *gin.Context) {
	catID := c.Param("id")
	if catID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Missing cat ID",
			Message: "Cat ID is required",
		})
		return
	}

	var req BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request",
			Message: err.Error(),
		})
		return
	}

	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error:   "Storage not available",
			Message: "MinIO is not running",
		})
		return
	}

	// Get handler
	handler, err := storageRegistry.GetHandler("cat")
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Handler not found",
			Message: err.Error(),
		})
		return
	}

	// Create batch delete request
	batchReq := &interfaces.BatchDeleteRequest{
		FileKeys: req.FileKeys,
		UserID:   req.UserID,
	}

	// Perform batch delete
	response, err := handler.BatchDelete(c.Request.Context(), batchReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Batch delete failed",
			Message: err.Error(),
		})
		return
	}

	// Convert to response format
	var results []DeleteResult
	for i, result := range response.Results {
		fileKey := req.FileKeys[i]
		errorMsg := ""
		if result.Error != nil {
			errorMsg = result.Error.Error()
		}

		results = append(results, DeleteResult{
			FileKey: fileKey,
			Success: result.Success,
			Error:   errorMsg,
		})
	}

	// Use actual counts
	actualSuccessCount := len(results)
	actualTotalCount := len(req.FileKeys)

	c.JSON(http.StatusOK, BatchDeleteResponse{
		Success:      actualSuccessCount > 0,
		Results:      results,
		SuccessCount: actualSuccessCount,
		TotalCount:   actualTotalCount,
	})
}

// ============================================================================
// DOG PRESIGNED URL ENDPOINTS
// ============================================================================

// generateDogPresignedUploadURL generates a presigned URL for dog file upload
// @Summary      Generate presigned upload URL for dog
// @Description  Generate a presigned URL for direct dog file upload to MinIO
// @Tags         dogs
// @Accept       json
// @Produce      json
// @Param        id path string true "Dog ID"
// @Param        request body PresignedURLRequest true "Presigned URL request"
// @Success      200 {object} PresignedURLResponse
// @Failure      400 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /dogs/{id}/presigned/upload [post]
func generateDogPresignedUploadURL(c *gin.Context) {
	dogID := c.Param("id")
	if dogID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Missing dog ID",
			Message: "Dog ID is required",
		})
		return
	}

	var req PresignedURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request",
			Message: err.Error(),
		})
		return
	}

	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error:   "Storage not available",
			Message: "MinIO is not running",
		})
		return
	}

	// Generate file key for dog photo
	fileKey := fmt.Sprintf("dog/dog/%s/photo/%s", dogID, req.FileKey)
	expiresAt := time.Now().Add(req.Expires)

	// For demo purposes, return a mock presigned URL
	presignedURL := fmt.Sprintf("https://localhost:9000/myapp-storage/%s?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=minioadmin&X-Amz-Date=%s&X-Amz-Expires=%d&X-Amz-SignedHeaders=host&X-Amz-Signature=mock-signature",
		fileKey,
		time.Now().Format("20060102T150405Z"),
		int(req.Expires.Seconds()),
	)

	c.JSON(http.StatusOK, PresignedURLResponse{
		Success:   true,
		URL:       presignedURL,
		ExpiresAt: expiresAt,
		FileKey:   fileKey,
	})
}

// generateDogPresignedDownloadURL generates a presigned URL for dog file download
// @Summary      Generate presigned download URL for dog
// @Description  Generate a presigned URL for direct dog file download from MinIO
// @Tags         dogs
// @Accept       json
// @Produce      json
// @Param        id path string true "Dog ID"
// @Param        request body PresignedURLRequest true "Presigned URL request"
// @Success      200 {object} PresignedURLResponse
// @Failure      400 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /dogs/{id}/presigned/download [post]
func generateDogPresignedDownloadURL(c *gin.Context) {
	dogID := c.Param("id")
	if dogID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Missing dog ID",
			Message: "Dog ID is required",
		})
		return
	}

	var req PresignedURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request",
			Message: err.Error(),
		})
		return
	}

	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error:   "Storage not available",
			Message: "MinIO is not running",
		})
		return
	}

	// Use the provided file key (should be the full path)
	fileKey := req.FileKey
	expiresAt := time.Now().Add(req.Expires)

	// For demo purposes, return a mock presigned URL
	presignedURL := fmt.Sprintf("https://localhost:9000/myapp-storage/%s?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=minioadmin&X-Amz-Date=%s&X-Amz-Expires=%d&X-Amz-SignedHeaders=host&X-Amz-Signature=mock-signature",
		fileKey,
		time.Now().Format("20060102T150405Z"),
		int(req.Expires.Seconds()),
	)

	c.JSON(http.StatusOK, PresignedURLResponse{
		Success:   true,
		URL:       presignedURL,
		ExpiresAt: expiresAt,
		FileKey:   fileKey,
	})
}

// ============================================================================
// DOG BATCH OPERATION ENDPOINTS
// ============================================================================

// batchUploadDogFiles handles batch dog file uploads
// @Summary      Batch dog file upload
// @Description  Upload multiple dog files in a single request
// @Tags         dogs
// @Accept       multipart/form-data
// @Produce      json
// @Param        id path string true "Dog ID"
// @Param        files formData file true "Files to upload"
// @Param        user_id formData string true "User ID"
// @Success      200 {object} BatchUploadResponse
// @Failure      400 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /dogs/{id}/batch/upload [post]
func batchUploadDogFiles(c *gin.Context) {
	dogID := c.Param("id")
	if dogID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Missing dog ID",
			Message: "Dog ID is required",
		})
		return
	}

	userID := c.PostForm("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Missing user_id",
			Message: "user_id is required",
		})
		return
	}

	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error:   "Storage not available",
			Message: "MinIO is not running",
		})
		return
	}

	// Get handler
	handler, err := storageRegistry.GetHandler("dog")
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Handler not found",
			Message: err.Error(),
		})
		return
	}

	// Parse multipart form
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid form data",
			Message: err.Error(),
		})
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "No files provided",
			Message: "At least one file is required",
		})
		return
	}

	// Convert to batch files
	var batchFiles []interfaces.BatchFile
	for _, file := range files {
		// Open file
		src, err := file.Open()
		if err != nil {
			continue
		}
		defer src.Close()

		batchFiles = append(batchFiles, interfaces.BatchFile{
			FileName:    file.Filename,
			ContentType: file.Header.Get("Content-Type"),
			FileSize:    file.Size,
			Category:    "photo",
			FileData:    src,
			Metadata: map[string]interface{}{
				"upload_source": "batch_upload",
				"original_name": file.Filename,
				"dog_id":        dogID,
			},
		})
	}

	// Create batch upload request
	batchReq := &interfaces.BatchUploadRequest{
		Files:  batchFiles,
		UserID: userID,
	}

	// Perform batch upload
	response, err := handler.BatchUpload(c.Request.Context(), batchReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Batch upload failed",
			Message: err.Error(),
		})
		return
	}

	// Convert to response format - FIX: Use actual count from response
	var results []UploadResponse
	for i, result := range response.Results {
		if result != nil && result.Success && i < len(files) {
			results = append(results, UploadResponse{
				FileID:      result.FileKey,
				FileName:    files[i].Filename, // Use original filename
				FileSize:    result.FileSize,
				ContentType: result.ContentType,
				Category:    "photo",
				EntityType:  "dog",
				EntityID:    dogID,
				UploadedAt:  time.Now(),
				Metadata:    result.Metadata,
				Thumbnails:  convertThumbnails(result.Thumbnails),
			})
		}
	}

	// FIX: Use actual counts from the results
	actualSuccessCount := len(results)
	actualTotalCount := len(files)

	c.JSON(http.StatusOK, BatchUploadResponse{
		Success:      actualSuccessCount > 0,
		Results:      results,
		SuccessCount: actualSuccessCount,
		TotalCount:   actualTotalCount,
	})
}

// batchDownloadDogFiles handles batch dog file downloads
// @Summary      Batch dog file download
// @Description  Download multiple dog files in a single request
// @Tags         dogs
// @Accept       json
// @Produce      json
// @Param        id path string true "Dog ID"
// @Param        request body BatchDownloadRequest true "Batch download request"
// @Success      200 {object} BatchDownloadResponse
// @Failure      400 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /dogs/{id}/batch/download [post]
func batchDownloadDogFiles(c *gin.Context) {
	dogID := c.Param("id")
	if dogID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Missing dog ID",
			Message: "Dog ID is required",
		})
		return
	}

	var req BatchDownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request",
			Message: err.Error(),
		})
		return
	}

	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error:   "Storage not available",
			Message: "MinIO is not running",
		})
		return
	}

	// Get handler
	handler, err := storageRegistry.GetHandler("dog")
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Handler not found",
			Message: err.Error(),
		})
		return
	}

	// Create batch download request
	batchReq := &interfaces.BatchGetRequest{
		FileKeys: req.FileKeys,
		UserID:   req.UserID,
	}

	// Perform batch download
	response, err := handler.BatchGet(c.Request.Context(), batchReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Batch download failed",
			Message: err.Error(),
		})
		return
	}

	// Convert to response format
	var results []FileInfo
	for i, result := range response.Results {
		if result != nil && result.Success && i < len(req.FileKeys) {
			results = append(results, FileInfo{
				FileID:      req.FileKeys[i],
				FileName:    req.FileKeys[i], // Use file key as filename
				FileSize:    result.FileSize,
				ContentType: result.ContentType,
				Category:    "photo",
				UploadedAt:  time.Now(),
				Metadata:    result.Metadata,
			})
		}
	}

	// Use actual counts
	actualSuccessCount := len(results)
	actualTotalCount := len(req.FileKeys)

	c.JSON(http.StatusOK, BatchDownloadResponse{
		Success:      actualSuccessCount > 0,
		Results:      results,
		SuccessCount: actualSuccessCount,
		TotalCount:   actualTotalCount,
	})
}

// batchDeleteDogFiles handles batch dog file deletions
// @Summary      Batch dog file delete
// @Description  Delete multiple dog files in a single request
// @Tags         dogs
// @Accept       json
// @Produce      json
// @Param        id path string true "Dog ID"
// @Param        request body BatchDeleteRequest true "Batch delete request"
// @Success      200 {object} BatchDeleteResponse
// @Failure      400 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /dogs/{id}/batch/delete [post]
func batchDeleteDogFiles(c *gin.Context) {
	dogID := c.Param("id")
	if dogID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Missing dog ID",
			Message: "Dog ID is required",
		})
		return
	}

	var req BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request",
			Message: err.Error(),
		})
		return
	}

	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error:   "Storage not available",
			Message: "MinIO is not running",
		})
		return
	}

	// Get handler
	handler, err := storageRegistry.GetHandler("dog")
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Handler not found",
			Message: err.Error(),
		})
		return
	}

	// Create batch delete request
	batchReq := &interfaces.BatchDeleteRequest{
		FileKeys: req.FileKeys,
		UserID:   req.UserID,
	}

	// Perform batch delete
	response, err := handler.BatchDelete(c.Request.Context(), batchReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Batch delete failed",
			Message: err.Error(),
		})
		return
	}

	// Convert to response format
	var results []DeleteResult
	for i, result := range response.Results {
		fileKey := req.FileKeys[i]
		errorMsg := ""
		if result.Error != nil {
			errorMsg = result.Error.Error()
		}

		results = append(results, DeleteResult{
			FileKey: fileKey,
			Success: result.Success,
			Error:   errorMsg,
		})
	}

	// Use actual counts
	actualSuccessCount := len(results)
	actualTotalCount := len(req.FileKeys)

	c.JSON(http.StatusOK, BatchDeleteResponse{
		Success:      actualSuccessCount > 0,
		Results:      results,
		SuccessCount: actualSuccessCount,
		TotalCount:   actualTotalCount,
	})
}

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================
