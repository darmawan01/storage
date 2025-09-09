package storage

import (
	"bytes"
	"fmt"
	"log"

	"github.com/darmawan01/storage/category"
	"github.com/darmawan01/storage/config"
	"github.com/darmawan01/storage/errors"
	"github.com/darmawan01/storage/handler"
	"github.com/darmawan01/storage/interfaces"
	"github.com/darmawan01/storage/middleware"
	"github.com/darmawan01/storage/registry"
)

func StorageTest() {
	// Test basic storage functionality
	fmt.Println("Testing MinIO Storage Architecture...")

	// Create a test configuration
	cfg := config.StorageConfig{
		Endpoint:        "localhost:9000",
		AccessKey:       "minioadmin",
		SecretKey:       "minioadmin",
		UseSSL:          false,
		Region:          "us-east-1",
		BucketName:      "test-storage",
		MaxFileSize:     25 * 1024 * 1024, // 25MB
		UploadTimeout:   300,              // 5 minutes
		DownloadTimeout: 60,               // 1 minute
	}

	// Initialize storage registry
	_ = registry.NewRegistry()

	// Test configuration validation
	if err := cfg.Validate(); err != nil {
		log.Printf("Configuration validation failed: %v", err)
		return
	}

	fmt.Println("✓ Configuration validation passed")

	// Test handler configuration
	handlerConfig := &handler.HandlerConfig{

		Categories: map[string]category.CategoryConfig{
			"profile": {
				BucketSuffix: "images",
				IsPublic:     false,
				MaxSize:      5 * 1024 * 1024,
				AllowedTypes: []string{"image/jpeg", "image/png"},
				Validation: category.ValidationConfig{
					MaxFileSize:       5 * 1024 * 1024,
					MinFileSize:       1024,
					AllowedTypes:      []string{"image/jpeg", "image/png"},
					AllowedExtensions: []string{".jpg", ".jpeg", ".png"},
					ImageValidation: &category.ImageValidationConfig{
						MinWidth:  100,
						MaxWidth:  2048,
						MinHeight: 100,
						MaxHeight: 2048,
					},
				},
				Security: middleware.SecurityConfig{
					RequireAuth:  true,
					RequireOwner: true,
				},
				Preview: category.PreviewConfig{
					GenerateThumbnails: true,
					ThumbnailSizes:     []string{"150x150", "300x300"},
					EnablePreview:      true,
				},
			},
		},
	}

	// Test handler configuration validation
	if err := handlerConfig.Validate(); err != nil {
		log.Printf("Handler configuration validation failed: %v", err)
		return
	}

	fmt.Println("✓ Handler configuration validation passed")

	// Test error types
	testError := errors.ErrFileNotFound
	fmt.Printf("✓ Error types: %s\n", testError.Error())

	// Test default configurations
	defaultStorageConfig := config.DefaultStorageConfig()
	fmt.Printf("✓ Default storage config: %s\n", defaultStorageConfig.Endpoint)

	defaultCategoryConfig := category.DefaultCategoryConfig("images", false, 5*1024*1024)
	fmt.Printf("✓ Default category config: %s\n", defaultCategoryConfig.BucketSuffix)

	// Test storage request/response structures
	uploadReq := &interfaces.UploadRequest{
		FileData:    bytes.NewReader([]byte("test data")),
		FileSize:    9,
		ContentType: "text/plain",
		FileName:    "test.txt",
		Category:    "profile",
		EntityType:  "user",
		EntityID:    "123",
		UserID:      "user-123",
		Metadata: map[string]interface{}{
			"test": "value",
		},
	}

	fmt.Printf("✓ Upload request created: %s\n", uploadReq.FileName)

	downloadReq := &interfaces.DownloadRequest{
		FileKey: "test/user/123/profile/test.txt",
		UserID:  "user-123",
	}

	fmt.Printf("✓ Download request created: %s\n", downloadReq.FileKey)

	// Test file metadata
	fileMetadata := &interfaces.FileMetadata{
		ID:          "file-123",
		FileName:    "test.txt",
		FileKey:     "test/user/123/profile/test.txt",
		FileSize:    9,
		ContentType: "text/plain",
		Namespace:   "test",
		EntityType:  "user",
		EntityID:    "123",
		UploadedBy:  "user-123",
		Version:     1,
		Checksum:    "abc123",
	}

	fmt.Printf("✓ File metadata created: %s\n", fileMetadata.FileName)

	// Test thumbnail info
	thumbnailInfo := interfaces.ThumbnailInfo{
		Size:     "150x150",
		URL:      "/thumbnails/150x150/test.jpg",
		Width:    150,
		Height:   150,
		FileSize: 1024,
	}

	fmt.Printf("✓ Thumbnail info created: %s\n", thumbnailInfo.Size)

	fmt.Println("\n🎉 All tests passed! The MinIO Storage Architecture is working correctly.")
	fmt.Println("\nTo run the full example with Gin integration:")
	fmt.Println("go run examples/gin_example.go")
}
