package middleware

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// EncryptionMiddleware handles file encryption/decryption
type EncryptionMiddleware struct {
	config EncryptionConfig
}

// EncryptionConfig represents encryption middleware configuration
type EncryptionConfig struct {
	Enabled          bool   `json:"enabled"`
	Algorithm        string `json:"algorithm"`  // "AES-256-GCM"
	KeySource        string `json:"key_source"` // "env", "file", "kms"
	KeyPath          string `json:"key_path,omitempty"`
	KeyEnvVar        string `json:"key_env_var,omitempty"`
	KeyID            string `json:"key_id,omitempty"`
	EncryptAtRest    bool   `json:"encrypt_at_rest"`
	EncryptInTransit bool   `json:"encrypt_in_transit"`
}

// EncryptedData represents encrypted file data
type EncryptedData struct {
	Data      []byte `json:"data"`
	Nonce     []byte `json:"nonce"`
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id,omitempty"`
}

// NewEncryptionMiddleware creates a new encryption middleware
func NewEncryptionMiddleware(config EncryptionConfig) *EncryptionMiddleware {
	return &EncryptionMiddleware{
		config: config,
	}
}

// Name returns the middleware name
func (m *EncryptionMiddleware) Name() string {
	return "encryption"
}

// Process processes the request through encryption middleware
func (m *EncryptionMiddleware) Process(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
	// Check if encryption is enabled
	if !m.config.Enabled {
		return next(ctx, req)
	}

	// Handle different operations
	switch req.Operation {
	case "upload":
		return m.processUpload(ctx, req, next)
	case "download":
		return m.processDownload(ctx, req, next)
	default:
		return next(ctx, req)
	}
}

// processUpload handles encryption for upload operations
func (m *EncryptionMiddleware) processUpload(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
	// Check if we should encrypt at rest
	if !m.config.EncryptAtRest {
		return next(ctx, req)
	}

	// Read the file data
	data, err := io.ReadAll(req.FileData)
	if err != nil {
		return &StorageResponse{
			Success: false,
			Error:   fmt.Errorf("failed to read file data: %w", err),
		}, nil
	}

	// Encrypt the data
	encryptedData, err := m.encryptData(data)
	if err != nil {
		return &StorageResponse{
			Success: false,
			Error:   fmt.Errorf("failed to encrypt data: %w", err),
		}, nil
	}

	// Update the request with encrypted data
	req.FileData = bytes.NewReader(encryptedData.Data)
	req.FileSize = int64(len(encryptedData.Data) + len(encryptedData.Nonce) + len(encryptedData.Algorithm) + 4) // Approximate size

	// Add encryption metadata
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	req.Metadata["encrypted"] = true
	req.Metadata["encryption_algorithm"] = encryptedData.Algorithm
	req.Metadata["encryption_key_id"] = encryptedData.KeyID

	// Process with next middleware
	response, err := next(ctx, req)
	if err != nil {
		return response, err
	}

	// Add encryption info to response metadata
	if response.Metadata == nil {
		response.Metadata = make(map[string]interface{})
	}
	response.Metadata["encrypted"] = true
	response.Metadata["encryption_algorithm"] = encryptedData.Algorithm

	return response, nil
}

// processDownload handles decryption for download operations
func (m *EncryptionMiddleware) processDownload(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
	// Process with next middleware first
	response, err := next(ctx, req)
	if err != nil {
		return response, err
	}

	// Check if the file is encrypted
	if response.Metadata != nil {
		if encrypted, ok := response.Metadata["encrypted"].(bool); ok && encrypted {
			// Read the encrypted data
			data, err := io.ReadAll(response.FileData)
			if err != nil {
				return &StorageResponse{
					Success: false,
					Error:   fmt.Errorf("failed to read encrypted data: %w", err),
				}, nil
			}

			// Decrypt the data
			decryptedData, err := m.decryptData(data)
			if err != nil {
				return &StorageResponse{
					Success: false,
					Error:   fmt.Errorf("failed to decrypt data: %w", err),
				}, nil
			}

			// Update response with decrypted data
			response.FileData = bytes.NewReader(decryptedData)
			response.FileSize = int64(len(decryptedData))
		}
	}

	return response, nil
}

// encryptData encrypts the given data
func (m *EncryptionMiddleware) encryptData(data []byte) (*EncryptedData, error) {
	// Get encryption key
	key, err := m.getEncryptionKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt data
	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	return &EncryptedData{
		Data:      ciphertext,
		Nonce:     nonce,
		Algorithm: m.config.Algorithm,
		KeyID:     m.config.KeyID,
	}, nil
}

// decryptData decrypts the given encrypted data
func (m *EncryptionMiddleware) decryptData(data []byte) ([]byte, error) {
	// Get encryption key
	key, err := m.getEncryptionKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Extract nonce and ciphertext
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]

	// Decrypt data
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	return plaintext, nil
}

// getEncryptionKey retrieves the encryption key
func (m *EncryptionMiddleware) getEncryptionKey() ([]byte, error) {
	switch m.config.KeySource {
	case "env":
		return m.getKeyFromEnv()
	case "file":
		return m.getKeyFromFile()
	case "kms":
		return m.getKeyFromKMS()
	default:
		return nil, fmt.Errorf("unsupported key source: %s", m.config.KeySource)
	}
}

// getKeyFromEnv retrieves the key from environment variable
func (m *EncryptionMiddleware) getKeyFromEnv() ([]byte, error) {
	// Get key from environment variable
	keyStr := os.Getenv("STORAGE_ENCRYPTION_KEY")
	if keyStr == "" {
		return nil, fmt.Errorf("STORAGE_ENCRYPTION_KEY environment variable not set")
	}

	// Decode hex string to bytes
	key, err := hex.DecodeString(keyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid hex key in environment variable: %w", err)
	}

	// Validate key length (AES-256 requires 32 bytes)
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes (256 bits) for AES-256")
	}

	return key, nil
}

// getKeyFromFile retrieves the key from file
func (m *EncryptionMiddleware) getKeyFromFile() ([]byte, error) {
	// Get key file path from config or environment
	keyPath := m.config.KeyPath
	if keyPath == "" {
		keyPath = os.Getenv("STORAGE_ENCRYPTION_KEY_FILE")
	}
	if keyPath == "" {
		return nil, fmt.Errorf("encryption key file path not configured")
	}

	// Read key from file
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	// Decode hex string to bytes
	keyStr := strings.TrimSpace(string(keyData))
	key, err := hex.DecodeString(keyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid hex key in file: %w", err)
	}

	// Validate key length (AES-256 requires 32 bytes)
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes (256 bits) for AES-256")
	}

	return key, nil
}

// getKeyFromKMS retrieves the key from KMS
func (m *EncryptionMiddleware) getKeyFromKMS() ([]byte, error) {
	// KMS integration would require AWS SDK or similar
	// For now, return an error indicating KMS is not implemented
	return nil, fmt.Errorf("KMS key retrieval not implemented - requires AWS SDK integration")
}

// generateKey generates a new encryption key
func (m *EncryptionMiddleware) generateKey() ([]byte, error) {
	key := make([]byte, 32) // 256 bits
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	return key, nil
}

// hashKey hashes a key using SHA-256
func (m *EncryptionMiddleware) hashKey(key []byte) []byte {
	hash := sha256.Sum256(key)
	return hash[:]
}

// EncryptString encrypts a string
func (m *EncryptionMiddleware) EncryptString(plaintext string) (string, error) {
	data, err := m.encryptData([]byte(plaintext))
	if err != nil {
		return "", err
	}

	// Convert to base64 or similar for storage
	// TODO: Implement proper encoding
	return string(data.Data), nil
}

// DecryptString decrypts a string
func (m *EncryptionMiddleware) DecryptString(ciphertext string) (string, error) {
	// TODO: Implement proper decoding
	data, err := m.decryptData([]byte(ciphertext))
	if err != nil {
		return "", err
	}

	return string(data), nil
}
