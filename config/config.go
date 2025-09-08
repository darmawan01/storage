package config

import "github.com/darmawan01/storage/errors"

// StorageConfig represents the central storage configuration
type StorageConfig struct {
	Endpoint        string `json:"endpoint"`
	AccessKey       string `json:"access_key"`
	SecretKey       string `json:"secret_key"`
	UseSSL          bool   `json:"use_ssl"`
	Region          string `json:"region"`
	BucketName      string `json:"bucket_name"`
	MaxFileSize     int64  `json:"max_file_size"`
	UploadTimeout   int    `json:"upload_timeout"`
	DownloadTimeout int    `json:"download_timeout"`

	// Performance optimization settings
	MaxConnections    int `json:"max_connections"`    // Max concurrent connections
	ConnectionTimeout int `json:"connection_timeout"` // Connection timeout in seconds
	RequestTimeout    int `json:"request_timeout"`    // Request timeout in seconds
	RetryAttempts     int `json:"retry_attempts"`     // Number of retry attempts
	RetryDelay        int `json:"retry_delay"`        // Delay between retries in milliseconds
}

// Default configurations
func DefaultStorageConfig() StorageConfig {
	return StorageConfig{
		Endpoint:        "localhost:9000",
		AccessKey:       "minioadmin",
		SecretKey:       "minioadmin",
		UseSSL:          false,
		Region:          "us-east-1",
		BucketName:      "myapp-storage",
		MaxFileSize:     25 * 1024 * 1024, // 25MB
		UploadTimeout:   300,              // 5 minutes
		DownloadTimeout: 60,               // 1 minute

		// Performance optimization defaults
		MaxConnections:    100, // Max concurrent connections
		ConnectionTimeout: 30,  // 30 seconds
		RequestTimeout:    60,  // 60 seconds
		RetryAttempts:     3,   // 3 retry attempts
		RetryDelay:        100, // 100ms delay between retries
	}
}

// Helper functions for configuration validation
func (c *StorageConfig) Validate() error {
	if c.Endpoint == "" {
		return &errors.StorageError{Code: "INVALID_CONFIG", Message: "Endpoint is required"}
	}
	if c.AccessKey == "" {
		return &errors.StorageError{Code: "INVALID_CONFIG", Message: "AccessKey is required"}
	}
	if c.SecretKey == "" {
		return &errors.StorageError{Code: "INVALID_CONFIG", Message: "SecretKey is required"}
	}
	if c.MaxFileSize <= 0 {
		return &errors.StorageError{Code: "INVALID_CONFIG", Message: "MaxFileSize must be greater than 0"}
	}
	if c.MaxConnections <= 0 {
		return &errors.StorageError{Code: "INVALID_CONFIG", Message: "MaxConnections must be greater than 0"}
	}
	if c.ConnectionTimeout <= 0 {
		return &errors.StorageError{Code: "INVALID_CONFIG", Message: "ConnectionTimeout must be greater than 0"}
	}
	if c.RequestTimeout <= 0 {
		return &errors.StorageError{Code: "INVALID_CONFIG", Message: "RequestTimeout must be greater than 0"}
	}
	if c.RetryAttempts < 0 {
		return &errors.StorageError{Code: "INVALID_CONFIG", Message: "RetryAttempts must be non-negative"}
	}
	if c.RetryDelay < 0 {
		return &errors.StorageError{Code: "INVALID_CONFIG", Message: "RetryDelay must be non-negative"}
	}
	return nil
}
