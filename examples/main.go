package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/darmawan01/storage"

	"github.com/gin-gonic/gin"
)

var storageRegistry *storage.Registry

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

	// API routes
	api := router.Group("/api/v1")
	{
		// Health check
		api.GET("/health", healthCheck)

		// User file operations
		users := api.Group("/users")
		{
			users.POST("/:id/profile/upload", uploadUserProfile)
			users.POST("/:id/documents/upload", uploadUserDocument)
			users.GET("/:id/files/:category/:fileId", getUserFile)
			users.DELETE("/:id/files/:category/:fileId", deleteUserFile)
			users.GET("/:id/files", listUserFiles)
		}

		// Courier file operations
		couriers := api.Group("/couriers")
		{
			couriers.POST("/:id/vehicle/upload", uploadCourierVehicle)
			couriers.POST("/:id/document/upload", uploadCourierDocument)
			couriers.GET("/:id/files/:fileId", getCourierFile)
			couriers.DELETE("/:id/files/:fileId", deleteCourierFile)
			couriers.GET("/:id/files", listCourierFiles)
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
		}
	}

	// Start server
	log.Println("🚀 Starting MinIO Storage API Server on :8080")
	log.Println("📚 API Documentation: http://localhost:8080/api/v1/health")
	log.Println("🧪 Test endpoints: http://localhost:8080/api/v1/test/upload")
	router.Run(":8080")
}

// initStorage initializes the storage registry
func initStorage() {
	storageRegistry = storage.NewRegistry()

	// Initialize MinIO connection
	err := storageRegistry.Initialize(storage.StorageConfig{
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

	// Register user storage handler
	_, err = storageRegistry.Register("user", &storage.HandlerConfig{
		BasePath: "user",
		Categories: map[string]storage.CategoryConfig{
			"profile": {
				BucketSuffix: "images",
				IsPublic:     false,
				MaxSize:      5 * 1024 * 1024,
				AllowedTypes: []string{"image/jpeg", "image/png", "image/webp"},
				Validation: storage.ValidationConfig{
					MaxFileSize:       5 * 1024 * 1024,
					MinFileSize:       1024,
					AllowedTypes:      []string{"image/jpeg", "image/png", "image/webp"},
					AllowedExtensions: []string{".jpg", ".jpeg", ".png", ".webp"},
					ImageValidation: &storage.ImageValidationConfig{
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
				Security: storage.SecurityConfig{
					RequireAuth:       true,
					RequireOwner:      true,
					GenerateThumbnail: true,
				},
				Preview: storage.PreviewConfig{
					GenerateThumbnails: true,
					ThumbnailSizes:     []string{"150x150", "300x300", "600x600"},
					EnablePreview:      true,
					PreviewFormats:     []string{"image"},
				},
			},
			"document": {
				BucketSuffix: "documents",
				IsPublic:     false,
				MaxSize:      10 * 1024 * 1024,
				AllowedTypes: []string{"application/pdf", "image/jpeg", "image/png"},
				Validation: storage.ValidationConfig{
					MaxFileSize:       10 * 1024 * 1024,
					MinFileSize:       1024,
					AllowedTypes:      []string{"application/pdf", "image/jpeg", "image/png"},
					AllowedExtensions: []string{".pdf", ".jpg", ".jpeg", ".png"},
				},
				Security: storage.SecurityConfig{
					RequireAuth:        true,
					RequireOwner:       true,
					EncryptAtRest:      true,
					PresignedURLExpiry: 24 * time.Hour,
				},
			},
		},
	})
	if err != nil {
		log.Printf("⚠️  Warning: Failed to register user storage: %v", err)
	}

	// Register courier storage handler
	_, err = storageRegistry.Register("courier", &storage.HandlerConfig{
		BasePath: "courier",

		Categories: map[string]storage.CategoryConfig{
			"profile": {
				BucketSuffix: "images",
				IsPublic:     false,
				MaxSize:      5 * 1024 * 1024,
			},
			"vehicle": {
				BucketSuffix: "images",
				IsPublic:     false,
				MaxSize:      5 * 1024 * 1024,
			},
			"document": {
				BucketSuffix: "documents",
				IsPublic:     false,
				MaxSize:      10 * 1024 * 1024,
			},
		},
	})
	if err != nil {
		log.Printf("⚠️  Warning: Failed to register courier storage: %v", err)
	}

	log.Println("✅ Storage registry initialized successfully")
}

// HealthCheck godoc
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
func testUpload(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Test upload endpoint",
		"instructions": []string{
			"Use POST /api/v1/users/{id}/profile/upload",
			"Send multipart/form-data with 'file' field",
			"Supported formats: JPEG, PNG, WebP",
			"Max size: 5MB",
		},
		"example_curl": "curl -X POST -F 'file=@image.jpg' http://localhost:8080/api/v1/users/123/profile/upload",
	})
}

func testDownload(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Test download endpoint",
		"instructions": []string{
			"Use GET /api/v1/users/{id}/files/{category}/{fileId}",
			"FileId is returned from upload response",
			"Category can be 'profile' or 'document'",
		},
		"example_curl": "curl -O http://localhost:8080/api/v1/users/123/files/profile/file123.jpg",
	})
}

func testValidation(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Validation rules",
		"profile_images": map[string]interface{}{
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
		},
		"documents": map[string]interface{}{
			"max_size":      "10MB",
			"min_size":      "1KB",
			"allowed_types": []string{"application/pdf", "image/jpeg", "image/png"},
			"pdf_requirements": []string{
				"Valid PDF structure",
				"1-100 pages",
				"Title and author metadata required",
				"No password protection",
				"No scripts",
			},
		},
	})
}

// UploadUserProfile handles profile image uploads
func uploadUserProfile(c *gin.Context) {
	userID := c.Param("id")

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

	// Get user storage handler
	userHandler, err := storageRegistry.GetHandler("user")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage not available"})
		return
	}

	// Create upload request
	uploadReq := &storage.UploadRequest{
		FileData:    src,
		FileSize:    file.Size,
		ContentType: file.Header.Get("Content-Type"),
		FileName:    file.Filename,
		Category:    "profile",
		EntityType:  "user",
		EntityID:    userID,
		UserID:      getCurrentUserID(c),
		Metadata: map[string]interface{}{
			"original_filename": file.Filename,
			"upload_source":     "web",
		},
	}

	// Upload file
	response, err := userHandler.Upload(c.Request.Context(), uploadReq)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "File uploaded successfully",
	})
}

// UploadUserDocument handles document uploads
func uploadUserDocument(c *gin.Context) {
	userID := c.Param("id")

	// Get file from form
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}

	// Get required metadata
	title := c.PostForm("title")
	author := c.PostForm("author")
	if title == "" || author == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Title and author are required"})
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

	// Get user storage handler
	userHandler, err := storageRegistry.GetHandler("user")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage not available"})
		return
	}

	// Create upload request
	uploadReq := &storage.UploadRequest{
		FileData:    src,
		FileSize:    file.Size,
		ContentType: file.Header.Get("Content-Type"),
		FileName:    file.Filename,
		Category:    "document",
		EntityType:  "user",
		EntityID:    userID,
		UserID:      getCurrentUserID(c),
		Metadata: map[string]interface{}{
			"title":       title,
			"author":      author,
			"description": c.PostForm("description"),
		},
	}

	// Upload file
	response, err := userHandler.Upload(c.Request.Context(), uploadReq)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Document uploaded successfully",
	})
}

// GetUserFile handles file downloads
func getUserFile(c *gin.Context) {
	fileID := c.Param("fileId")

	// Check if storage is available
	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage not available",
			"mode":  "demo",
		})
		return
	}

	// Get user storage handler
	userHandler, err := storageRegistry.GetHandler("user")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage not available"})
		return
	}

	// Create download request
	downloadReq := &storage.DownloadRequest{
		FileKey: fileID,
		UserID:  getCurrentUserID(c),
	}

	// Download file
	response, err := userHandler.Download(c.Request.Context(), downloadReq)
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

// DeleteUserFile handles file deletion
func deleteUserFile(c *gin.Context) {
	fileID := c.Param("fileId")

	// Check if storage is available
	if storageRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage not available",
			"mode":  "demo",
		})
		return
	}

	// Get user storage handler
	userHandler, err := storageRegistry.GetHandler("user")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage not available"})
		return
	}

	// Create delete request
	deleteReq := &storage.DeleteRequest{
		FileKey: fileID,
		UserID:  getCurrentUserID(c),
	}

	// Delete file
	err = userHandler.Delete(c.Request.Context(), deleteReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "File deleted successfully",
	})
}

// ListUserFiles handles file listing
func listUserFiles(c *gin.Context) {
	userID := c.Param("id")
	category := c.Query("category")
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

	// Get user storage handler
	userHandler, err := storageRegistry.GetHandler("user")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Storage not available"})
		return
	}

	// Create list request
	listReq := &storage.ListRequest{
		EntityType: "user",
		EntityID:   userID,
		Category:   category,
		UserID:     getCurrentUserID(c),
		Limit:      limit,
		Offset:     offset,
	}

	// List files
	response, err := userHandler.ListFiles(c.Request.Context(), listReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
	})
}

// Courier endpoints (simplified)
func uploadCourierVehicle(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Courier vehicle upload endpoint",
		"status":  "not implemented in demo",
	})
}

func uploadCourierDocument(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Courier document upload endpoint",
		"status":  "not implemented in demo",
	})
}

func getCourierFile(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Courier file download endpoint",
		"status":  "not implemented in demo",
	})
}

func deleteCourierFile(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Courier file delete endpoint",
		"status":  "not implemented in demo",
	})
}

func listCourierFiles(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Courier file list endpoint",
		"status":  "not implemented in demo",
	})
}

// File preview endpoints
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
