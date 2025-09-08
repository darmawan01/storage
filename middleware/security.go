package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

// SecurityMiddleware handles security-related operations
type SecurityMiddleware struct {
	config SecurityConfig
	client *minio.Client
}

// SecurityConfig represents security middleware configuration
type SecurityConfig struct {
	// Access control
	RequireAuth  bool     `json:"require_auth,omitempty"`
	RequireOwner bool     `json:"require_owner,omitempty"`
	RequireRole  []string `json:"require_role,omitempty"`

	// File security
	EncryptAtRest     bool `json:"encrypt_at_rest,omitempty"`
	GenerateThumbnail bool `json:"generate_thumbnail,omitempty"`

	// URL security
	PresignedURLExpiry time.Duration `json:"presigned_url_expiry,omitempty"`
	MaxDownloadCount   int           `json:"max_download_count,omitempty"`
}

// NewSecurityMiddleware creates a new security middleware
func NewSecurityMiddleware(config SecurityConfig, client *minio.Client) *SecurityMiddleware {
	return &SecurityMiddleware{
		config: config,
		client: client,
	}
}

// Name returns the middleware name
func (m *SecurityMiddleware) Name() string {
	return "security"
}

// Process processes the request through security middleware
func (m *SecurityMiddleware) Process(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
	// Apply security checks based on operation
	switch req.Operation {
	case "upload":
		return m.processUpload(ctx, req, next)
	case "download":
		return m.processDownload(ctx, req, next)
	case "delete":
		return m.processDelete(ctx, req, next)
	case "preview":
		return m.processPreview(ctx, req, next)
	default:
		return next(ctx, req)
	}
}

// processUpload handles security for upload operations
func (m *SecurityMiddleware) processUpload(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
	// Check authentication requirement
	if m.config.RequireAuth && req.UserID == "" {
		return &StorageResponse{
			Success: false,
			Error:   fmt.Errorf("authentication required for upload"),
		}, nil
	}

	// Check owner requirement
	if m.config.RequireOwner && req.UserID == "" {
		return &StorageResponse{
			Success: false,
			Error:   fmt.Errorf("owner information required for upload"),
		}, nil
	}

	// Check role requirement
	if len(m.config.RequireRole) > 0 {
		userRoles := m.getUserRoles(ctx, req.UserID)
		if !m.hasRequiredRole(userRoles, m.config.RequireRole) {
			return &StorageResponse{
				Success: false,
				Error:   fmt.Errorf("insufficient permissions for upload"),
			}, nil
		}
	}

	// Process with next middleware
	response, err := next(ctx, req)
	if err != nil {
		return response, err
	}

	// Apply post-upload security measures
	if m.config.EncryptAtRest {
		// TODO: Implement encryption at rest
		// This would involve encrypting the file before storage
	}

	return response, nil
}

// processDownload handles security for download operations
func (m *SecurityMiddleware) processDownload(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
	// Check authentication requirement
	if m.config.RequireAuth && req.UserID == "" {
		return &StorageResponse{
			Success: false,
			Error:   fmt.Errorf("authentication required for download"),
		}, nil
	}

	// Check file access permissions
	if err := m.checkFileAccess(ctx, req); err != nil {
		return &StorageResponse{
			Success: false,
			Error:   err,
		}, nil
	}

	// Check download count limits
	if m.config.MaxDownloadCount > 0 {
		if err := m.checkDownloadLimit(ctx, req); err != nil {
			return &StorageResponse{
				Success: false,
				Error:   err,
			}, nil
		}
	}

	return next(ctx, req)
}

// processDelete handles security for delete operations
func (m *SecurityMiddleware) processDelete(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
	// Check authentication requirement
	if m.config.RequireAuth && req.UserID == "" {
		return &StorageResponse{
			Success: false,
			Error:   fmt.Errorf("authentication required for delete"),
		}, nil
	}

	// Check owner requirement
	if m.config.RequireOwner {
		if err := m.checkFileOwnership(ctx, req); err != nil {
			return &StorageResponse{
				Success: false,
				Error:   err,
			}, nil
		}
	}

	return next(ctx, req)
}

// processPreview handles security for preview operations
func (m *SecurityMiddleware) processPreview(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
	// Check authentication requirement
	if m.config.RequireAuth && req.UserID == "" {
		return &StorageResponse{
			Success: false,
			Error:   fmt.Errorf("authentication required for preview"),
		}, nil
	}

	// Check file access permissions
	if err := m.checkFileAccess(ctx, req); err != nil {
		return &StorageResponse{
			Success: false,
			Error:   err,
		}, nil
	}

	return next(ctx, req)
}

// checkFileAccess checks if the user has access to the file
func (m *SecurityMiddleware) checkFileAccess(ctx context.Context, req *StorageRequest) error {
	// Get user ID from context
	userID, ok := ctx.Value("user_id").(string)
	if !ok {
		return fmt.Errorf("user ID not found in context")
	}

	// Check if file is public
	if req.Metadata != nil {
		if isPublic, ok := req.Metadata["is_public"].(bool); ok && isPublic {
			return nil // Public files are accessible to everyone
		}
	}

	// For private files, check if user has access
	// Basic implementation: allow access if user ID matches uploader
	if req.Metadata != nil {
		if uploadedBy, ok := req.Metadata["uploaded_by"].(string); ok && uploadedBy == userID {
			return nil
		}
	}

	// Check if user has admin role
	roles := m.getUserRoles(ctx, userID)
	for _, role := range roles {
		if role == "admin" || role == "moderator" {
			return nil
		}
	}

	return fmt.Errorf("access denied: insufficient permissions")
}

// checkFileOwnership checks if the user owns the file
func (m *SecurityMiddleware) checkFileOwnership(ctx context.Context, req *StorageRequest) error {
	// Get user ID from context
	userID, ok := ctx.Value("user_id").(string)
	if !ok {
		return fmt.Errorf("user ID not found in context")
	}

	// Check if user owns the file
	if req.Metadata != nil {
		if uploadedBy, ok := req.Metadata["uploaded_by"].(string); ok && uploadedBy == userID {
			return nil
		}
	}

	// Check if user has admin role
	roles := m.getUserRoles(ctx, userID)
	for _, role := range roles {
		if role == "admin" {
			return nil
		}
	}

	return fmt.Errorf("access denied: user does not own this file")
}

// checkDownloadLimit checks if the download limit has been exceeded
func (m *SecurityMiddleware) checkDownloadLimit(ctx context.Context, req *StorageRequest) error {
	// Get user ID from context
	userID, ok := ctx.Value("user_id").(string)
	if !ok {
		return fmt.Errorf("user ID not found in context")
	}

	// Check download limits based on user role
	roles := m.getUserRoles(ctx, userID)

	// Admin users have no limits
	for _, role := range roles {
		if role == "admin" {
			return nil
		}
	}

	// Basic implementation: limit large file downloads
	if req.FileSize > 100*1024*1024 { // 100MB limit
		// Check if user has premium role
		hasPremium := false
		for _, role := range roles {
			if role == "premium" || role == "vip" {
				hasPremium = true
				break
			}
		}

		if !hasPremium {
			return fmt.Errorf("download limit exceeded: file too large for basic users")
		}
	}

	return nil
}

// getUserRoles retrieves user roles from context or external service
func (m *SecurityMiddleware) getUserRoles(ctx context.Context, userID string) []string {
	// First try to get roles from context
	if roles, ok := ctx.Value("user_roles").([]string); ok {
		return roles
	}

	// Basic implementation: return roles based on user ID patterns
	// In a real implementation, this would query a user service or database
	roles := []string{"user"} // Default role

	// Add additional roles based on user ID patterns (for demo purposes)
	if strings.HasPrefix(userID, "admin-") {
		roles = append(roles, "admin")
	} else if strings.HasPrefix(userID, "premium-") {
		roles = append(roles, "premium")
	} else if strings.HasPrefix(userID, "vip-") {
		roles = append(roles, "vip")
	} else if strings.HasPrefix(userID, "mod-") {
		roles = append(roles, "moderator")
	}

	return roles
}

// hasRequiredRole checks if the user has any of the required roles
func (m *SecurityMiddleware) hasRequiredRole(userRoles, requiredRoles []string) bool {
	for _, requiredRole := range requiredRoles {
		for _, userRole := range userRoles {
			if userRole == requiredRole {
				return true
			}
		}
	}
	return false
}

// GeneratePresignedURL generates a secure presigned URL
func (m *SecurityMiddleware) GeneratePresignedURL(ctx context.Context, bucketName, objectName string, expires time.Duration) (string, error) {
	if m.config.PresignedURLExpiry > 0 && expires > m.config.PresignedURLExpiry {
		expires = m.config.PresignedURLExpiry
	}

	url, err := m.client.PresignedGetObject(ctx, bucketName, objectName, expires, nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return url.String(), nil
}

// ValidateAccess validates user access to a resource
func (m *SecurityMiddleware) ValidateAccess(ctx context.Context, userID, resourceID, action string) error {
	// TODO: Implement comprehensive access validation
	// This would involve checking user permissions against resource policies

	return nil
}
