package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
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
			cats.GET("/:id/files/:fileId", getCatFile)
			cats.DELETE("/:id/files/:fileId", deleteCatFile)
			cats.GET("/:id/files", listCatFiles)
		}

		// Dog file operations
		dogs := api.Group("/dogs")
		{
			dogs.POST("/:id/upload", uploadDogFile)
			dogs.GET("/:id/files/:fileId", getDogFile)
			dogs.DELETE("/:id/files/:fileId", deleteDogFile)
			dogs.GET("/:id/files", listDogFiles)
		}

		// File preview operations
		files := api.Group("/files")
		{
			files.GET("/:fileId/preview", previewFile)
			files.GET("/:fileId/thumbnail", getThumbnail)
			files.GET("/:fileId/stream", streamFile)
		}

		// Test endpoints
		test := api.Group("/test")
		{
			test.GET("/upload", testUpload)
			test.GET("/download", testDownload)
			test.GET("/validation", testValidation)
			test.GET("/metadata", showMetadata)
		}
	}

	// Start server
	log.Println("🐱🐶 Starting Cat & Dog Photo Storage API Server on :8080")
	log.Println("📚 API Documentation: http://localhost:8080/swagger/index.html")
	log.Println("🔍 Health Check: http://localhost:8080/api/v1/health")
	log.Println("🧪 Test endpoints: http://localhost:8080/api/v1/test/upload")
	log.Println("📋 Metadata endpoint: http://localhost:8080/api/v1/test/metadata")
	log.Println("🐱 Cat upload: POST /api/v1/cats/{id}/upload")
	log.Println("🐶 Dog upload: POST /api/v1/dogs/{id}/upload")
	router.Run(":8080")
}

// initStorage initializes the storage registry
func initStorage() {
	storageRegistry = registry.NewRegistry()

	// Initialize MinIO connection
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

// Test endpoints
// @Summary      Test Upload Endpoint
// @Description  Get instructions for testing file upload functionality
// @Tags         Test
// @Accept       json
// @Produce      json
// @Success      200 {object} map[string]interface{} "Upload test instructions"
// @Router       /test/upload [get]
func testUpload(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Test upload endpoint",
		"instructions": []string{
			"Cat photos: POST /api/v1/cats/{id}/upload",
			"Dog photos: POST /api/v1/dogs/{id}/upload",
			"Send multipart/form-data with 'file' field",
			"Supported formats: JPEG, PNG, WebP",
			"Max size: 5MB",
			"Thumbnails generated automatically",
		},
		"examples": map[string]string{
			"cat_upload": "curl -X POST -F 'file=@cat.jpg' http://localhost:8080/api/v1/cats/123/upload",
			"dog_upload": "curl -X POST -F 'file=@dog.jpg' http://localhost:8080/api/v1/dogs/456/upload",
		},
	})
}

// @Summary      Test Download Endpoint
// @Description  Get instructions for testing file download functionality
// @Tags         Test
// @Accept       json
// @Produce      json
// @Success      200 {object} map[string]interface{} "Download test instructions"
// @Router       /test/download [get]
func testDownload(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Test download endpoint",
		"instructions": []string{
			"Cat files: GET /api/v1/cats/{id}/files/{fileId}",
			"Dog files: GET /api/v1/dogs/{id}/files/{fileId}",
			"FileId is returned from upload response",
			"Direct download or presigned URL available",
		},
		"examples": map[string]string{
			"cat_download": "curl -O http://localhost:8080/api/v1/cats/123/files/cat_photo_123.jpg",
			"dog_download": "curl -O http://localhost:8080/api/v1/dogs/456/files/dog_photo_456.jpg",
		},
	})
}

// @Summary      Test Validation Endpoint
// @Description  Get validation rules and requirements for file uploads
// @Tags         Test
// @Accept       json
// @Produce      json
// @Success      200 {object} map[string]interface{} "Validation rules and requirements"
// @Router       /test/validation [get]
func testValidation(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Validation rules for cat and dog photos",
		"photo_validation": map[string]interface{}{
			"max_size":      "5MB",
			"min_size":      "1KB",
			"allowed_types": []string{"image/jpeg", "image/png", "image/webp"},
			"dimensions": map[string]int{
				"min_width":  100,
				"max_width":  2048,
				"min_height": 100,
				"max_height": 2048,
			},
			"quality": map[string]int{
				"min": 60,
				"max": 95,
			},
			"features": []string{
				"Automatic thumbnail generation",
				"Image validation and optimization",
				"Metadata callback support",
				"Direct download and presigned URLs",
			},
		},
		"endpoints": map[string]interface{}{
			"cat_upload":   "POST /api/v1/cats/{id}/upload",
			"cat_download": "GET /api/v1/cats/{id}/files/{fileId}",
			"cat_list":     "GET /api/v1/cats/{id}/files",
			"dog_upload":   "POST /api/v1/dogs/{id}/upload",
			"dog_download": "GET /api/v1/dogs/{id}/files/{fileId}",
			"dog_list":     "GET /api/v1/dogs/{id}/files",
		},
	})
}

// @Summary      Show Metadata
// @Description  Display all stored file metadata from the callback
// @Tags         Test
// @Accept       json
// @Produce      json
// @Success      200 {object} map[string]interface{} "All stored metadata"
// @Router       /test/metadata [get]
func showMetadata(c *gin.Context) {
	allMetadata := metadataStorage.ListFileMetadata()

	var files []map[string]interface{}
	for _, metadata := range allMetadata {
		files = append(files, map[string]interface{}{
			"id":           metadata.ID,
			"file_name":    metadata.FileName,
			"file_key":     metadata.FileKey,
			"file_size":    metadata.FileSize,
			"content_type": metadata.ContentType,
			"category":     metadata.Category,
			"entity_type":  metadata.EntityType,
			"entity_id":    metadata.EntityID,
			"uploaded_by":  metadata.UploadedBy,
			"uploaded_at":  metadata.UploadedAt,
			"is_public":    metadata.IsPublic,
			"thumbnails":   metadata.Thumbnails,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Stored file metadata",
		"count":   len(files),
		"files":   files,
	})
}

// UploadCatFile handles cat photo uploads
// @Summary      Upload Cat Photo
// @Description  Upload a photo for a specific cat with validation and thumbnail generation
// @Tags         Cats
// @Accept       multipart/form-data
// @Produce      json
// @Param        id   path      string  true  "Cat ID"
// @Param        file formData  file    true  "Cat photo file (JPEG, PNG, WebP)"
// @Success      200  {object}  SuccessResponse{data=UploadResponse}  "File uploaded successfully"
// @Failure      400  {object}  map[string]interface{}  "Bad request - validation error"
// @Failure      401  {object}  map[string]interface{}  "Unauthorized"
// @Failure      413  {object}  map[string]interface{}  "File too large"
// @Failure      503  {object}  map[string]interface{}  "Service unavailable"
// @Security     ApiKeyAuth
// @Router       /cats/{id}/upload [post]
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
// @Description  Download a file belonging to a specific cat by file ID
// @Tags         Cats
// @Accept       json
// @Produce      application/octet-stream
// @Param        id     path      string  true  "Cat ID"
// @Param        fileId path      string  true  "File ID"
// @Success      200    {file}    file    "File content"
// @Failure      401    {object}  map[string]interface{}  "Unauthorized"
// @Failure      403    {object}  map[string]interface{}  "Forbidden - access denied"
// @Failure      404    {object}  map[string]interface{}  "File not found"
// @Failure      503    {object}  map[string]interface{}  "Service unavailable"
// @Security     ApiKeyAuth
// @Router       /cats/{id}/files/{fileId} [get]
func getCatFile(c *gin.Context) {
	fileID := c.Param("fileId")

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

	// Create download request
	downloadReq := &interfaces.DownloadRequest{
		FileKey: fileID,
		UserID:  getCurrentUserID(c),
	}

	// Download file
	response, err := catHandler.Download(c.Request.Context(), downloadReq)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
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
// @Description  Delete a file belonging to a specific cat by file ID
// @Tags         Cats
// @Accept       json
// @Produce      json
// @Param        id     path      string  true  "Cat ID"
// @Param        fileId path      string  true  "File ID"
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
// @Description  Download a file belonging to a specific dog by file ID
// @Tags         Dogs
// @Accept       json
// @Produce      application/octet-stream
// @Param        id     path      string  true  "Dog ID"
// @Param        fileId path      string  true  "File ID"
// @Success      200    {file}    file    "File content"
// @Failure      401    {object}  map[string]interface{}  "Unauthorized"
// @Failure      403    {object}  map[string]interface{}  "Forbidden - access denied"
// @Failure      404    {object}  map[string]interface{}  "File not found"
// @Failure      503    {object}  map[string]interface{}  "Service unavailable"
// @Security     ApiKeyAuth
// @Router       /dogs/{id}/files/{fileId} [get]
func getDogFile(c *gin.Context) {
	fileID := c.Param("fileId")

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

	// Create download request
	downloadReq := &interfaces.DownloadRequest{
		FileKey: fileID,
		UserID:  getCurrentUserID(c),
	}

	// Download file
	response, err := dogHandler.Download(c.Request.Context(), downloadReq)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Set headers
	c.Header("Content-Type", response.ContentType)
	c.Header("Content-Length", strconv.FormatInt(response.FileSize, 10))

	// Stream file
	io.Copy(c.Writer, response.FileData)
}

// @Summary      Delete Dog File
// @Description  Delete a file belonging to a specific dog by file ID
// @Tags         Dogs
// @Accept       json
// @Produce      json
// @Param        id     path      string  true  "Dog ID"
// @Param        fileId path      string  true  "File ID"
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
// @Description  Get a thumbnail of a file with specified size (not implemented in demo)
// @Tags         Files
// @Accept       json
// @Produce      json
// @Param        fileId path      string  true   "File ID"
// @Param        size   query     string  false  "Thumbnail size (default: 150x150)"
// @Success      200    {object}  map[string]interface{}  "Not implemented in demo"
// @Router       /files/{fileId}/thumbnail [get]
func getThumbnail(c *gin.Context) {
	fileID := c.Param("fileId")
	size := c.DefaultQuery("size", "150x150")

	c.JSON(http.StatusOK, gin.H{
		"message": "Thumbnail endpoint",
		"file_id": fileID,
		"size":    size,
		"status":  "not implemented in demo",
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
