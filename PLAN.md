# Generic MinIO Storage Architecture Design

## Overview
This document outlines a generic, configurable MinIO storage package architecture that can be used across different services and contexts. The design focuses on flexibility, security, organization, preview capabilities, and scalability through a middleware-based approach.

## Current Issues
1. **Security**: Public bucket access, no RBAC, limited file-level permissions
2. **Organization**: Single bucket, no namespace separation, basic file categorization
3. **Preview**: No direct preview capabilities, no thumbnails, no content-specific handling
4. **Scalability**: No caching, no CDN, no versioning, no cleanup policies

## Centralized Storage Architecture

### 1. Centralized Storage Initialization

The storage system will be initialized once in `main.go` with a central registry that manages multiple storage handlers:

```go
// main.go
func main() {
    // Initialize central storage registry
    storageRegistry := storage.NewRegistry()
    
    // Register different storage handlers
    userStorage := storageRegistry.Register("user", &storage.HandlerConfig{
        BasePath: "user",
        Middlewares: []string{"security", "validation", "thumbnail"},
        Categories: map[string]storage.CategoryConfig{
            "profile": {BucketSuffix: "images", IsPublic: false, MaxSize: 5 * 1024 * 1024},
            "document": {BucketSuffix: "documents", IsPublic: false, MaxSize: 10 * 1024 * 1024},
        },
    })
    
    courierStorage := storageRegistry.Register("courier", &storage.HandlerConfig{
        BasePath: "courier", 
        Middlewares: []string{"security", "validation", "thumbnail", "audit"},
        Categories: map[string]storage.CategoryConfig{
            "profile": {BucketSuffix: "images", IsPublic: false, MaxSize: 5 * 1024 * 1024},
            "vehicle": {BucketSuffix: "images", IsPublic: false, MaxSize: 5 * 1024 * 1024},
            "document": {BucketSuffix: "documents", IsPublic: false, MaxSize: 10 * 1024 * 1024},
        },
    })
    
    // Pass registry to controllers
    userController := user.NewController(storageRegistry)
    courierController := courier.NewController(storageRegistry)
}
```

### 2. Storage Registry System

The registry manages multiple storage handlers with shared MinIO connection:

```go
// Central storage configuration (MinIO connection)
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
}

// Handler-specific configuration
type HandlerConfig struct {
    BasePath    string                       `json:"base_path"`
    Middlewares []string                     `json:"middlewares"`  // Default middlewares for all categories
    Categories  map[string]CategoryConfig    `json:"categories"`
    Security    SecurityConfig               `json:"security,omitempty"`
    Preview     PreviewConfig                `json:"preview,omitempty"`
}

type CategoryConfig struct {
    BucketSuffix string                    `json:"bucket_suffix"`
    IsPublic     bool                      `json:"is_public"`
    MaxSize      int64                     `json:"max_size"`
    AllowedTypes []string                  `json:"allowed_types"`
    
    // Basic validation handled by storage package
    Validation   BasicValidationConfig     `json:"validation,omitempty"`
    
    // Category-specific middlewares (overrides handler defaults)
    Middlewares  []string                  `json:"middlewares,omitempty"`
    
    // Category-specific security
    Security     SecurityConfig            `json:"security,omitempty"`
    
    // Category-specific preview settings
    Preview      PreviewConfig             `json:"preview,omitempty"`
}

// Comprehensive validation handled by storage package
type ValidationConfig struct {
    // Basic file validation
    MaxFileSize      int64    `json:"max_file_size,omitempty"`
    MinFileSize      int64    `json:"min_file_size,omitempty"`
    AllowedTypes     []string `json:"allowed_types,omitempty"`
    AllowedExtensions []string `json:"allowed_extensions,omitempty"`
    
    // Image validation (only applied if AllowedTypes contains image types)
    ImageValidation  *ImageValidationConfig `json:"image_validation,omitempty"`
    
    // PDF validation (only applied if AllowedTypes contains application/pdf)
    PDFValidation    *PDFValidationConfig   `json:"pdf_validation,omitempty"`
    
    // Video validation (only applied if AllowedTypes contains video types)
    VideoValidation  *VideoValidationConfig `json:"video_validation,omitempty"`
}

type ImageValidationConfig struct {
    // Dimension validation
    MinWidth         int    `json:"min_width,omitempty"`
    MaxWidth         int    `json:"max_width,omitempty"`
    MinHeight        int    `json:"min_height,omitempty"`
    MaxHeight        int    `json:"max_height,omitempty"`
    
    // Quality validation
    MinQuality       int    `json:"min_quality,omitempty"`  // 1-100
    MaxQuality       int    `json:"max_quality,omitempty"`  // 1-100
    
    // Format validation
    AllowedFormats   []string `json:"allowed_formats,omitempty"`  // ["jpeg", "png", "webp"]
    
    // Aspect ratio validation
    MinAspectRatio   float64 `json:"min_aspect_ratio,omitempty"`
    MaxAspectRatio   float64 `json:"max_aspect_ratio,omitempty"`
    
    // Color space validation
    AllowedColorSpaces []string `json:"allowed_color_spaces,omitempty"`  // ["RGB", "RGBA", "GRAY"]
}

type PDFValidationConfig struct {
    // PDF structure validation
    ValidateStructure bool `json:"validate_structure,omitempty"`
    
    // Page count validation
    MinPages         int  `json:"min_pages,omitempty"`
    MaxPages         int  `json:"max_pages,omitempty"`
    
    // Metadata validation
    RequireMetadata  bool     `json:"require_metadata,omitempty"`
    RequiredFields   []string `json:"required_fields,omitempty"`  // ["title", "author"]
    
    // Security validation
    AllowPassword    bool `json:"allow_password,omitempty"`
    AllowScripts     bool `json:"allow_scripts,omitempty"`
}

type VideoValidationConfig struct {
    // Duration validation
    MinDuration      int    `json:"min_duration,omitempty"`  // seconds
    MaxDuration      int    `json:"max_duration,omitempty"`  // seconds
    
    // Resolution validation
    MinWidth         int    `json:"min_width,omitempty"`
    MaxWidth         int    `json:"max_width,omitempty"`
    MinHeight        int    `json:"min_height,omitempty"`
    MaxHeight        int    `json:"max_height,omitempty"`
    
    // Codec validation
    AllowedCodecs    []string `json:"allowed_codecs,omitempty"`  // ["h264", "h265", "vp9"]
    
    // Frame rate validation
    MinFrameRate     int    `json:"min_frame_rate,omitempty"`
    MaxFrameRate     int    `json:"max_frame_rate,omitempty"`
}

type SecurityConfig struct {
    // Access control
    RequireAuth     bool     `json:"require_auth,omitempty"`
    RequireOwner    bool     `json:"require_owner,omitempty"`
    RequireRole     []string `json:"require_role,omitempty"`
    
    // File security
    EncryptAtRest   bool     `json:"encrypt_at_rest,omitempty"`
    GenerateThumbnail bool   `json:"generate_thumbnail,omitempty"`
    
    // URL security
    PresignedURLExpiry time.Duration `json:"presigned_url_expiry,omitempty"`
    MaxDownloadCount   int           `json:"max_download_count,omitempty"`
}

type PreviewConfig struct {
    // Thumbnail settings
    GenerateThumbnails bool     `json:"generate_thumbnails,omitempty"`
    ThumbnailSizes    []string `json:"thumbnail_sizes,omitempty"`  // ["150x150", "300x300", "600x600"]
    
    // Preview settings
    EnablePreview     bool     `json:"enable_preview,omitempty"`
    PreviewFormats    []string `json:"preview_formats,omitempty"`  // ["image", "pdf", "video"]
    
    // CDN settings
    UseCDN           bool     `json:"use_cdn,omitempty"`
    CDNEndpoint      string   `json:"cdn_endpoint,omitempty"`
}

// Storage Registry
type Registry struct {
    client   *minio.Client
    config   StorageConfig
    handlers map[string]*Handler
    mutex    sync.RWMutex
}

func NewRegistry() *Registry {
    return &Registry{
        handlers: make(map[string]*Handler),
    }
}

func (r *Registry) Initialize(config StorageConfig) error {
    // Initialize MinIO client once
    client, err := minio.New(config.Endpoint, &minio.Options{
        Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
        Secure: config.UseSSL,
        Region: config.Region,
    })
    if err != nil {
        return err
    }
    
    r.client = client
    r.config = config
    return nil
}

func (r *Registry) Register(name string, config *HandlerConfig) *Handler {
    r.mutex.Lock()
    defer r.mutex.Unlock()
    
    handler := &Handler{
        name:   name,
        config: config,
        client: r.client,
        registry: r,
    }
    
    r.handlers[name] = handler
    return handler
}

func (r *Registry) GetHandler(name string) (*Handler, error) {
    r.mutex.RLock()
    defer r.mutex.RUnlock()
    
    handler, exists := r.handlers[name]
    if !exists {
        return nil, fmt.Errorf("handler %s not found", name)
    }
    
    return handler, nil
}
```

### 2. Dynamic Bucket Organization

Instead of hardcoded buckets, the system will dynamically organize files based on configuration:

```
{basePath}-{category}/
├── user-documents/          # When basePath="user"
├── user-images/            
├── user-attachments/       
├── user-temp/             
├── user-thumbnails/       
└── user-public/           

# Or for courier service:
├── courier-documents/       # When basePath="courier"
├── courier-images/         
├── courier-attachments/    
├── courier-temp/           
├── courier-thumbnails/     
└── courier-public/         
```

### 3. Storage Handler Interface

Each registered handler provides a clean interface for file operations:

```go
type Handler struct {
    name     string
    config   *HandlerConfig
    client   *minio.Client
    registry *Registry
    middlewares []Middleware
}

// Handler interface for file operations
func (h *Handler) Upload(ctx context.Context, req *UploadRequest) (*UploadResponse, error) {
    // Apply middlewares
    // Generate file key based on basePath
    // Upload to appropriate bucket
    // Return response
}

func (h *Handler) Download(ctx context.Context, req *DownloadRequest) (*DownloadResponse, error) {
    // Validate access
    // Download from bucket
    // Apply middlewares
    // Return response
}

func (h *Handler) Delete(ctx context.Context, req *DeleteRequest) error {
    // Validate access
    // Delete from bucket
    // Apply middlewares
}

func (h *Handler) Preview(ctx context.Context, req *PreviewRequest) (*PreviewResponse, error) {
    // Generate preview URL
    // Apply security checks
    // Return preview response
}

func (h *Handler) GetFileInfo(ctx context.Context, req *InfoRequest) (*FileInfo, error) {
    // Get file metadata
    // Apply access control
    // Return file info
}

// Helper methods
func (h *Handler) GetBucketName(category string) string {
    return fmt.Sprintf("%s-%s", h.config.BasePath, h.config.Categories[category].BucketSuffix)
}

func (h *Handler) GenerateFileKey(entityType, entityID, fileType, filename string) string {
    timestamp := time.Now().Unix()
    uuid := generateUUID()
    ext := filepath.Ext(filename)
    return fmt.Sprintf("%s/%s/%s/%s/%d_%s%s", 
        h.config.BasePath, entityType, entityID, fileType, timestamp, uuid, ext)
}
```

### 4. Controller Integration

Controllers receive the registry and use specific handlers:

```go
// User Controller
type UserController struct {
    storage *storage.Handler
}

func NewUserController(registry *storage.Registry) *UserController {
    handler, _ := registry.GetHandler("user")
    return &UserController{storage: handler}
}

func (c *UserController) RegisterRoutes(router *gin.RouterGroup) {
    group := router.Group("/users")
    {
        group.Use(authentication.Authentication())
        
        group.POST("/:id/profile/upload", c.UploadProfile)
        group.GET("/:id/profile/:fileId", c.GetProfileImage)
        group.DELETE("/:id/profile/:fileId", c.DeleteProfileImage)
    }
}

func (c *UserController) UploadProfile(ctx *gin.Context) {
    userID := ctx.Param("id")
    
    // Parse multipart form
    file, err := ctx.FormFile("file")
    if err != nil {
        http_response.SendBadRequestResponse(ctx, "Invalid file")
        return
    }
    
    // Create upload request
    uploadReq := &storage.UploadRequest{
        FileData:    file,
        FileSize:    file.Size,
        ContentType: file.Header.Get("Content-Type"),
        Category:    "profile",
        EntityType:  "user",
        EntityID:    userID,
        UserID:      getCurrentUserID(ctx),
    }
    
    // Upload using handler
    response, err := c.storage.Upload(ctx, uploadReq)
    if err != nil {
        http_response.SendInternalServerErrorResponse(ctx, err.Error())
        return
    }
    
    http_response.SendSuccess(ctx, response, nil, nil)
}

// Courier Controller
type CourierController struct {
    storage *storage.Handler
}

func NewCourierController(registry *storage.Registry) *CourierController {
    handler, _ := registry.GetHandler("courier")
    return &CourierController{storage: handler}
}

func (c *CourierController) RegisterRoutes(router *gin.RouterGroup) {
    group := router.Group("/couriers")
    {
        group.Use(authentication.Authentication())
        
        group.POST("/:id/vehicle/upload", c.UploadVehicleImage)
        group.POST("/:id/document/upload", c.UploadDocument)
        group.GET("/:id/files/:fileId", c.GetFile)
    }
}
```

### 3. Security Model

#### Access Control Levels
1. **Public** - No authentication required
2. **Authenticated** - Requires valid JWT token
3. **Owner** - Only file owner can access
4. **Role-based** - Based on user roles (admin, courier, customer)
5. **Temporary** - Time-limited access with presigned URLs

#### Permission Matrix
| File Type | Public | Authenticated | Owner | Admin | Courier | Customer |
|-----------|--------|---------------|-------|-------|---------|----------|
| Profile   | ❌     | ✅            | ✅    | ✅    | ✅      | ❌       |
| Document  | ❌     | ❌            | ✅    | ✅    | ✅      | ❌       |
| Attachment| ❌     | ✅            | ✅    | ✅    | ✅      | ✅       |
| Temp      | ❌     | ❌            | ✅    | ✅    | ❌      | ❌       |
| Thumbnail | ✅     | ✅            | ✅    | ✅    | ✅      | ✅       |

### 4. Preview System

#### Direct Preview Endpoints
- `GET /files/{id}/preview` - Direct file preview
- `GET /files/{id}/thumbnail` - Thumbnail generation
- `GET /files/{id}/stream` - Streaming for large files

#### Content-Type Support
- **Images**: JPEG, PNG, WebP, GIF
- **Documents**: PDF, DOC, DOCX
- **Videos**: MP4, WebM (streaming)
- **Audio**: MP3, WAV, OGG

#### Thumbnail Generation
- Automatic thumbnail creation for images
- Multiple sizes: 150x150, 300x300, 600x600
- Lazy generation on first request
- Caching with Redis

### 4. Middleware System

#### Middleware Interface
```go
type Middleware interface {
    Name() string
    Process(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error)
}

type MiddlewareFunc func(ctx context.Context, req *StorageRequest) (*StorageResponse, error)

type StorageRequest struct {
    Operation   string                 `json:"operation"`   // upload, download, delete, preview
    FileKey     string                 `json:"file_key"`
    FileData    io.Reader              `json:"-"`
    FileSize    int64                  `json:"file_size"`
    ContentType string                 `json:"content_type"`
    Category    string                 `json:"category"`
    EntityType  string                 `json:"entity_type"`
    EntityID    string                 `json:"entity_id"`
    UserID      string                 `json:"user_id"`
    Metadata    map[string]interface{} `json:"metadata"`
    Config      map[string]interface{} `json:"config"`
}

type StorageResponse struct {
    Success     bool                   `json:"success"`
    FileKey     string                 `json:"file_key"`
    FileURL     string                 `json:"file_url,omitempty"`
    FileSize    int64                  `json:"file_size"`
    ContentType string                 `json:"content_type"`
    Metadata    map[string]interface{} `json:"metadata"`
    Error       error                  `json:"error,omitempty"`
}
```

#### Built-in Middlewares
```go
// Security middleware
type SecurityMiddleware struct {
    config SecurityConfig
}

// Thumbnail generation middleware
type ThumbnailMiddleware struct {
    config ThumbnailConfig
}

// Validation middleware
type ValidationMiddleware struct {
    config ValidationConfig
}

// Audit logging middleware
type AuditMiddleware struct {
    logger Logger
}

// Encryption middleware
type EncryptionMiddleware struct {
    config EncryptionConfig
}

// CDN middleware
type CDNMiddleware struct {
    config CDNConfig
}
```

### 5. Generic Storage Client

#### Core Interfaces
```go
type StorageClient interface {
    // Basic operations
    Upload(ctx context.Context, req *UploadRequest) (*UploadResponse, error)
    Download(ctx context.Context, req *DownloadRequest) (*DownloadResponse, error)
    Delete(ctx context.Context, req *DeleteRequest) error
    
    // Preview operations
    Preview(ctx context.Context, req *PreviewRequest) (*PreviewResponse, error)
    Thumbnail(ctx context.Context, req *ThumbnailRequest) (*ThumbnailResponse, error)
    Stream(ctx context.Context, req *StreamRequest) (*StreamResponse, error)
    
    // Security operations
    GeneratePresignedURL(ctx context.Context, req *PresignedURLRequest) (*PresignedURLResponse, error)
    ValidateAccess(ctx context.Context, req *AccessRequest) (*AccessResponse, error)
    
    // Management operations
    ListFiles(ctx context.Context, req *ListRequest) (*ListResponse, error)
    GetFileInfo(ctx context.Context, req *InfoRequest) (*FileInfo, error)
    UpdateMetadata(ctx context.Context, req *UpdateMetadataRequest) error
}

// Generic constructor
func New(config *StorageConfig) (StorageClient, error) {
    // Initialize MinIO client
    // Setup middlewares based on config
    // Configure buckets dynamically
    // Return configured client
}
```

#### File Metadata Structure
```go
type FileMetadata struct {
    ID          string            `json:"id"`
    FileName    string            `json:"file_name"`
    FileKey     string            `json:"file_key"`
    FileSize    int64             `json:"file_size"`
    ContentType string            `json:"content_type"`
    Category    FileCategory      `json:"category"`
    Namespace   string            `json:"namespace"`
    EntityType  string            `json:"entity_type"`
    EntityID    string            `json:"entity_id"`
    UploadedBy  string            `json:"uploaded_by"`
    UploadedAt  time.Time         `json:"uploaded_at"`
    IsPublic    bool              `json:"is_public"`
    Permissions []Permission      `json:"permissions"`
    Tags        []string          `json:"tags"`
    Thumbnails  []ThumbnailInfo   `json:"thumbnails"`
    Version     int               `json:"version"`
    Checksum    string            `json:"checksum"`
    ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
}
```

### 6. Security Enhancements

#### Bucket Policies
- Separate policies per bucket
- Role-based access control
- IP whitelisting for admin operations
- Time-based access restrictions

#### File-Level Security
- Encryption at rest
- Signed URLs with expiration
- Audit logging for all operations
- Content scanning for malware

#### Access Validation
```go
type AccessValidator interface {
    ValidateAccess(ctx context.Context, user *User, file *FileMetadata, action string) error
    CheckPermissions(ctx context.Context, user *User, file *FileMetadata) ([]Permission, error)
    GeneratePresignedURL(ctx context.Context, user *User, file *FileMetadata, expires time.Duration) (string, error)
}
```

### 7. Performance Optimizations

#### Caching Strategy
- Redis cache for file metadata
- CDN integration for public files
- Thumbnail caching
- Presigned URL caching

#### Streaming Support
- Chunked upload for large files
- Progressive download
- Range requests support
- Background processing for thumbnails

#### Cleanup Policies
- Automatic temp file cleanup
- Orphaned file detection
- Storage quota management
- Lifecycle policies per bucket

### 8. Monitoring & Observability

#### Metrics
- Upload/download success rates
- File size distributions
- Access patterns
- Storage utilization
- Error rates by operation type

#### Logging
- Structured logging for all operations
- Audit trail for security events
- Performance metrics
- Error tracking with context

#### Health Checks
- MinIO connectivity
- Bucket accessibility
- Cache health
- CDN status

## Implementation Plan

### Phase 1: Core Infrastructure
1. Refactor storage client with new interfaces
2. Implement multi-bucket strategy
3. Add enhanced file metadata structure
4. Implement basic security model

### Phase 2: Preview System
1. Add preview endpoints
2. Implement thumbnail generation
3. Add streaming support
4. Content-type specific handling

### Phase 3: Security & Access Control
1. Implement RBAC system
2. Add file-level permissions
3. Implement audit logging
4. Add encryption support

### Phase 4: Performance & Monitoring
1. Add caching layer
2. Implement CDN integration
3. Add monitoring and metrics
4. Implement cleanup policies

### Phase 5: Advanced Features
1. File versioning
2. Advanced search and filtering
3. Bulk operations
4. Migration tools

## Migration Strategy

### Backward Compatibility
- Maintain existing API endpoints
- Gradual migration of file organization
- Data migration scripts
- Feature flags for new functionality

### Testing Strategy
- Unit tests for all new components
- Integration tests with MinIO
- Performance testing
- Security testing

## Benefits

1. **Enhanced Security**: Role-based access, file-level permissions, audit logging
2. **Better Organization**: Namespace separation, categorized storage, structured file keys
3. **Preview Capabilities**: Direct preview, thumbnails, streaming support
4. **Improved Performance**: Caching, CDN, optimized operations
5. **Better Monitoring**: Comprehensive metrics and logging
6. **Scalability**: Multi-bucket strategy, cleanup policies, lifecycle management

## Complete Implementation Example

### main.go
```go
package main

import (
    "log"
    "storage"
    "user"
    "courier"
    "order"
)

func main() {
    // Initialize storage registry
    storageRegistry := storage.NewRegistry()
    
    // Initialize MinIO connection
    err := storageRegistry.Initialize(storage.StorageConfig{
        Endpoint:   "localhost:9000",
        AccessKey:  "minioadmin", 
        SecretKey:  "minioadmin",
        UseSSL:     false,
        Region:     "us-east-1",
        BucketName: "myapp-storage",
        MaxFileSize: 25 * 1024 * 1024, // 25MB
    })
    if err != nil {
        log.Fatal("Failed to initialize storage:", err)
    }
    
    // Register storage handlers
    userStorage := storageRegistry.Register("user", &storage.HandlerConfig{
        BasePath: "user",
        Middlewares: []string{"security", "validation"}, // Default middlewares
        Categories: map[string]storage.CategoryConfig{
            "profile": {
                BucketSuffix: "images", 
                IsPublic: false, 
                MaxSize: 5 * 1024 * 1024,
                AllowedTypes: []string{"image/jpeg", "image/png", "image/webp"},
                
                // Comprehensive validation handled by storage package
                Validation: storage.ValidationConfig{
                    MaxFileSize: 5 * 1024 * 1024,
                    MinFileSize: 1024,
                    AllowedTypes: []string{"image/jpeg", "image/png", "image/webp"},
                    AllowedExtensions: []string{".jpg", ".jpeg", ".png", ".webp"},
                    
                    // Image-specific validation (only applied if image types are allowed)
                    ImageValidation: &storage.ImageValidationConfig{
                        MinWidth: 100,
                        MaxWidth: 2048,
                        MinHeight: 100,
                        MaxHeight: 2048,
                        MinQuality: 60,
                        MaxQuality: 95,
                        AllowedFormats: []string{"jpeg", "png", "webp"},
                        MinAspectRatio: 0.5,  // 1:2 ratio
                        MaxAspectRatio: 2.0,  // 2:1 ratio
                        AllowedColorSpaces: []string{"RGB", "RGBA"},
                    },
                },
                
                // Category-specific middlewares (overrides defaults)
                Middlewares: []string{"security", "thumbnail"},
                
                // Category-specific security
                Security: storage.SecurityConfig{
                    RequireAuth: true,
                    RequireOwner: true,
                    GenerateThumbnail: true,
                },
                
                // Category-specific preview
                Preview: storage.PreviewConfig{
                    GenerateThumbnails: true,
                    ThumbnailSizes: []string{"150x150", "300x300", "600x600"},
                    EnablePreview: true,
                    PreviewFormats: []string{"image"},
                },
            },
            
            "document": {
                BucketSuffix: "documents", 
                IsPublic: false, 
                MaxSize: 10 * 1024 * 1024,
                AllowedTypes: []string{"application/pdf", "image/jpeg", "image/png"},
                
                // Comprehensive validation handled by storage package
                Validation: storage.ValidationConfig{
                    MaxFileSize: 10 * 1024 * 1024,
                    MinFileSize: 1024,
                    AllowedTypes: []string{"application/pdf", "image/jpeg", "image/png"},
                    AllowedExtensions: []string{".pdf", ".jpg", ".jpeg", ".png"},
                    
                    // PDF-specific validation (only applied if PDF type is allowed)
                    PDFValidation: &storage.PDFValidationConfig{
                        ValidateStructure: true,
                        MinPages: 1,
                        MaxPages: 100,
                        RequireMetadata: true,
                        RequiredFields: []string{"title", "author"},
                        AllowPassword: false,
                        AllowScripts: false,
                    },
                    
                    // Image validation for document images
                    ImageValidation: &storage.ImageValidationConfig{
                        MinWidth: 200,
                        MaxWidth: 4000,
                        MinHeight: 200,
                        MaxHeight: 4000,
                        MinQuality: 70,
                        AllowedFormats: []string{"jpeg", "png"},
                    },
                },
                
                // Different middlewares for documents
                Middlewares: []string{"security", "encryption"},
                
                // Different security for documents
                Security: storage.SecurityConfig{
                    RequireAuth: true,
                    RequireOwner: true,
                    EncryptAtRest: true,
                    PresignedURLExpiry: 24 * time.Hour,
                },
                
                // No preview for documents
                Preview: storage.PreviewConfig{
                    EnablePreview: false,
                },
            },
        },
    })
    
    courierStorage := storageRegistry.Register("courier", &storage.HandlerConfig{
        BasePath: "courier",
        Middlewares: []string{"security", "validation", "thumbnail", "audit"},
        Categories: map[string]storage.CategoryConfig{
            "profile": {
                BucketSuffix: "images", 
                IsPublic: false, 
                MaxSize: 5 * 1024 * 1024,
            },
            "vehicle": {
                BucketSuffix: "images", 
                IsPublic: false, 
                MaxSize: 5 * 1024 * 1024,
            },
            "document": {
                BucketSuffix: "documents", 
                IsPublic: false, 
                MaxSize: 10 * 1024 * 1024,
            },
        },
    })
    
    orderStorage := storageRegistry.Register("order", &storage.HandlerConfig{
        BasePath: "order",
        Middlewares: []string{"security", "validation"},
        Categories: map[string]storage.CategoryConfig{
            "attachment": {
                BucketSuffix: "attachments", 
                IsPublic: false, 
                MaxSize: 25 * 1024 * 1024,
            },
            "receipt": {
                BucketSuffix: "receipts", 
                IsPublic: true, 
                MaxSize: 5 * 1024 * 1024,
            },
        },
    })
    
    // Initialize controllers with storage registry
    userController := user.NewController(storageRegistry)
    courierController := courier.NewController(storageRegistry)
    orderController := order.NewController(storageRegistry)
    
    // Setup routes
    router := gin.Default()
    api := router.Group("/api/v1")
    
    userController.RegisterRoutes(api)
    courierController.RegisterRoutes(api)
    orderController.RegisterRoutes(api)
    
    // Start server
    router.Run(":8080")
}
```

### Controller Integration with Configurable Validation

```go
// User Controller with configurable validation
type UserController struct {
    storage *storage.Handler
}

func NewUserController(registry *storage.Registry) *UserController {
    handler, _ := registry.GetHandler("user")
    return &UserController{storage: handler}
}

func (c *UserController) RegisterRoutes(router *gin.RouterGroup) {
    group := router.Group("/users")
    {
        group.Use(authentication.Authentication())
        
        // Profile image upload with specific validation
        group.POST("/:id/profile/upload", c.UploadProfile)
        
        // Document upload with different validation
        group.POST("/:id/documents/upload", c.UploadDocument)
        
        // Get file with category-specific access control
        group.GET("/:id/files/:category/:fileId", c.GetFile)
    }
}

func (c *UserController) UploadProfile(ctx *gin.Context) {
    userID := ctx.Param("id")
    
    // UPLOAD-SPECIFIC VALIDATION - Controller handles upload method validation
    if err := c.validateUploadMethod(ctx); err != nil {
        http_response.SendBadRequestResponse(ctx, err.Error())
        return
    }
    
    // Parse multipart form
    file, err := ctx.FormFile("file")
    if err != nil {
        http_response.SendBadRequestResponse(ctx, "Invalid file")
        return
    }
    
    // Open file
    src, err := file.Open()
    if err != nil {
        http_response.SendInternalServerErrorResponse(ctx, "Failed to open file")
        return
    }
    defer src.Close()
    
    // Create upload request - comprehensive validation handled by storage package
    uploadReq := &storage.UploadRequest{
        FileData:    src,
        FileSize:    file.Size,
        ContentType: file.Header.Get("Content-Type"),
        Category:    "profile", // Comprehensive validation (size, type, dimensions, quality) handled by storage
        EntityType:  "user",
        EntityID:    userID,
        UserID:      getCurrentUserID(ctx),
        Metadata: map[string]interface{}{
            "original_filename": file.Filename,
            "upload_source": "web",
        },
    }
    
    // Upload using handler - comprehensive validation and middleware applied by storage package
    response, err := c.storage.Upload(ctx, uploadReq)
    if err != nil {
        // Error will include comprehensive validation failure details
        http_response.SendBadRequestResponse(ctx, err.Error())
        return
    }
    
    http_response.SendSuccess(ctx, response, nil, nil)
}

// UPLOAD-SPECIFIC VALIDATION - Controller handles upload method validation
func (c *UserController) validateUploadMethod(ctx *gin.Context) error {
    // Check if it's multipart form data
    contentType := ctx.GetHeader("Content-Type")
    if !strings.HasPrefix(contentType, "multipart/form-data") {
        return fmt.Errorf("only multipart/form-data uploads are allowed")
    }
    
    // Check if single file upload (not multiple files)
    form, err := ctx.MultipartForm()
    if err != nil {
        return fmt.Errorf("failed to parse multipart form: %w", err)
    }
    
    files := form.File["file"]
    if len(files) == 0 {
        return fmt.Errorf("no file provided")
    }
    
    if len(files) > 1 {
        return fmt.Errorf("only single file upload is allowed for profile images")
    }
    
    return nil
}

// Alternative: Multiple file upload validation
func (c *UserController) validateMultipleFileUpload(ctx *gin.Context, maxFiles int) error {
    form, err := ctx.MultipartForm()
    if err != nil {
        return fmt.Errorf("failed to parse multipart form: %w", err)
    }
    
    files := form.File["files"] // Note: "files" for multiple uploads
    if len(files) == 0 {
        return fmt.Errorf("no files provided")
    }
    
    if len(files) > maxFiles {
        return fmt.Errorf("maximum %d files allowed, got %d", maxFiles, len(files))
    }
    
    return nil
}

// Alternative: Direct file upload validation (not multipart)
func (c *UserController) validateDirectFileUpload(ctx *gin.Context) error {
    contentType := ctx.GetHeader("Content-Type")
    if strings.HasPrefix(contentType, "multipart/form-data") {
        return fmt.Errorf("multipart uploads not allowed for this endpoint")
    }
    
    // Check if it's a direct file upload
    if !strings.HasPrefix(contentType, "application/octet-stream") && 
       !strings.HasPrefix(contentType, "image/") &&
       !strings.HasPrefix(contentType, "application/pdf") {
        return fmt.Errorf("unsupported content type: %s", contentType)
    }
    
    return nil
}

func (c *UserController) UploadDocument(ctx *gin.Context) {
    userID := ctx.Param("id")
    
    file, err := ctx.FormFile("file")
    if err != nil {
        http_response.SendBadRequestResponse(ctx, "Invalid file")
        return
    }
    
    // CUSTOM VALIDATION - Controller handles custom validation
    if err := c.validateDocument(file, ctx); err != nil {
        http_response.SendBadRequestResponse(ctx, err.Error())
        return
    }
    
    src, err := file.Open()
    if err != nil {
        http_response.SendInternalServerErrorResponse(ctx, "Failed to open file")
        return
    }
    defer src.Close()
    
    // Document upload - basic validation handled by storage package
    uploadReq := &storage.UploadRequest{
        FileData:    src,
        FileSize:    file.Size,
        ContentType: file.Header.Get("Content-Type"),
        Category:    "document", // Basic validation (size, type, extension) handled by storage
        EntityType:  "user",
        EntityID:    userID,
        UserID:      getCurrentUserID(ctx),
        Metadata: map[string]interface{}{
            "title": ctx.PostForm("title"),
            "author": ctx.PostForm("author"),
            "description": ctx.PostForm("description"),
        },
    }
    
    response, err := c.storage.Upload(ctx, uploadReq)
    if err != nil {
        http_response.SendBadRequestResponse(ctx, err.Error())
        return
    }
    
    http_response.SendSuccess(ctx, response, nil, nil)
}

// CUSTOM VALIDATION - Controller handles custom document validation
func (c *UserController) validateDocument(file *multipart.FileHeader, ctx *gin.Context) error {
    // Custom validation: Check required metadata
    title := ctx.PostForm("title")
    author := ctx.PostForm("author")
    if title == "" || author == "" {
        return fmt.Errorf("title and author are required for document upload")
    }
    
    // Custom validation: PDF-specific validation
    if strings.HasSuffix(strings.ToLower(file.Filename), ".pdf") {
        if err := c.validatePDFContent(file); err != nil {
            return fmt.Errorf("PDF validation failed: %w", err)
        }
    }
    
    // Custom validation: Virus scanning (if enabled)
    if c.config.EnableVirusScan {
        if err := c.scanForViruses(file); err != nil {
            return fmt.Errorf("virus scan failed: %w", err)
        }
    }
    
    return nil
}

func (c *UserController) validatePDFContent(file *multipart.FileHeader) error {
    // Custom PDF validation logic
    // e.g., check PDF structure, metadata, etc.
    return nil
}

func (c *UserController) scanForViruses(file *multipart.FileHeader) error {
    // Custom virus scanning logic
    // e.g., integrate with virus scanning service
    return nil
}

// Courier Controller with different validation rules
type CourierController struct {
    storage *storage.Handler
}

func NewCourierController(registry *storage.Registry) *CourierController {
    handler, _ := registry.GetHandler("courier")
    return &CourierController{storage: handler}
}

func (c *CourierController) UploadVehicleImage(ctx *gin.Context) {
    courierID := ctx.Param("id")
    
    file, err := ctx.FormFile("file")
    if err != nil {
        http_response.SendBadRequestResponse(ctx, "Invalid file")
        return
    }
    
    src, err := file.Open()
    if err != nil {
        http_response.SendInternalServerErrorResponse(ctx, "Failed to open file")
        return
    }
    defer src.Close()
    
    // Vehicle image with courier-specific validation
    uploadReq := &storage.UploadRequest{
        FileData:    src,
        FileSize:    file.Size,
        ContentType: file.Header.Get("Content-Type"),
        Category:    "vehicle", // Different validation rules for vehicle images
        EntityType:  "courier",
        EntityID:    courierID,
        UserID:      getCurrentUserID(ctx),
        Metadata: map[string]interface{}{
            "vehicle_type": ctx.PostForm("vehicle_type"),
            "license_plate": ctx.PostForm("license_plate"),
        },
    }
    
    response, err := c.storage.Upload(ctx, uploadReq)
    if err != nil {
        http_response.SendBadRequestResponse(ctx, err.Error())
        return
    }
    
    http_response.SendSuccess(ctx, response, nil, nil)
}
```

### Validation and Middleware Separation

#### Storage Package - Comprehensive Validation
The storage package handles comprehensive validation based on configuration:

```go
// Inside storage handler - COMPREHENSIVE VALIDATION
func (h *Handler) Upload(ctx context.Context, req *UploadRequest) (*UploadResponse, error) {
    // Get category configuration
    categoryConfig, exists := h.config.Categories[req.Category]
    if !exists {
        return nil, fmt.Errorf("category %s not found", req.Category)
    }
    
    // Apply COMPREHENSIVE validation (size, type, extension, dimensions, quality, etc.)
    if err := h.validateFile(req, categoryConfig.Validation); err != nil {
        return nil, fmt.Errorf("validation failed: %w", err)
    }
    
    // Apply category-specific middlewares (security, thumbnail, encryption, etc.)
    middlewares := categoryConfig.Middlewares
    if len(middlewares) == 0 {
        middlewares = h.config.Middlewares // Use default middlewares
    }
    
    for _, middlewareName := range middlewares {
        if err := h.applyMiddleware(ctx, req, middlewareName, categoryConfig); err != nil {
            return nil, fmt.Errorf("middleware %s failed: %w", middlewareName, err)
        }
    }
    
    // Continue with upload...
}

// Comprehensive validation in storage package
func (h *Handler) validateFile(req *UploadRequest, config ValidationConfig) error {
    // Basic validation
    if err := h.validateBasicFile(req, config); err != nil {
        return err
    }
    
    // Image validation (only if image types are allowed and config provided)
    if h.isImageType(req.ContentType) && config.ImageValidation != nil {
        if err := h.validateImage(req, *config.ImageValidation); err != nil {
            return fmt.Errorf("image validation failed: %w", err)
        }
    }
    
    // PDF validation (only if PDF type is allowed and config provided)
    if h.isPDFType(req.ContentType) && config.PDFValidation != nil {
        if err := h.validatePDF(req, *config.PDFValidation); err != nil {
            return fmt.Errorf("PDF validation failed: %w", err)
        }
    }
    
    // Video validation (only if video types are allowed and config provided)
    if h.isVideoType(req.ContentType) && config.VideoValidation != nil {
        if err := h.validateVideo(req, *config.VideoValidation); err != nil {
            return fmt.Errorf("video validation failed: %w", err)
        }
    }
    
    return nil
}

// Basic validation
func (h *Handler) validateBasicFile(req *UploadRequest, config ValidationConfig) error {
    // Check file size
    if config.MaxFileSize > 0 && req.FileSize > config.MaxFileSize {
        return fmt.Errorf("file size %d exceeds maximum allowed size %d", req.FileSize, config.MaxFileSize)
    }
    
    if config.MinFileSize > 0 && req.FileSize < config.MinFileSize {
        return fmt.Errorf("file size %d is below minimum required size %d", req.FileSize, config.MinFileSize)
    }
    
    // Check content type
    if len(config.AllowedTypes) > 0 {
        if !contains(config.AllowedTypes, req.ContentType) {
            return fmt.Errorf("content type %s is not allowed, allowed types: %v", req.ContentType, config.AllowedTypes)
        }
    }
    
    // Check file extension
    if len(config.AllowedExtensions) > 0 {
        ext := strings.ToLower(filepath.Ext(req.FileName))
        if !contains(config.AllowedExtensions, ext) {
            return fmt.Errorf("file extension %s is not allowed, allowed extensions: %v", ext, config.AllowedExtensions)
        }
    }
    
    return nil
}

// Image validation
func (h *Handler) validateImage(req *UploadRequest, config ImageValidationConfig) error {
    // Decode image to get properties
    img, format, err := h.decodeImage(req.FileData)
    if err != nil {
        return fmt.Errorf("failed to decode image: %w", err)
    }
    
    // Check dimensions
    if config.MinWidth > 0 && img.Width < config.MinWidth {
        return fmt.Errorf("image width %d is below minimum %d", img.Width, config.MinWidth)
    }
    if config.MaxWidth > 0 && img.Width > config.MaxWidth {
        return fmt.Errorf("image width %d exceeds maximum %d", img.Width, config.MaxWidth)
    }
    if config.MinHeight > 0 && img.Height < config.MinHeight {
        return fmt.Errorf("image height %d is below minimum %d", img.Height, config.MinHeight)
    }
    if config.MaxHeight > 0 && img.Height > config.MaxHeight {
        return fmt.Errorf("image height %d exceeds maximum %d", img.Height, config.MaxHeight)
    }
    
    // Check aspect ratio
    aspectRatio := float64(img.Width) / float64(img.Height)
    if config.MinAspectRatio > 0 && aspectRatio < config.MinAspectRatio {
        return fmt.Errorf("image aspect ratio %.2f is below minimum %.2f", aspectRatio, config.MinAspectRatio)
    }
    if config.MaxAspectRatio > 0 && aspectRatio > config.MaxAspectRatio {
        return fmt.Errorf("image aspect ratio %.2f exceeds maximum %.2f", aspectRatio, config.MaxAspectRatio)
    }
    
    // Check format
    if len(config.AllowedFormats) > 0 {
        if !contains(config.AllowedFormats, format) {
            return fmt.Errorf("image format %s is not allowed, allowed formats: %v", format, config.AllowedFormats)
        }
    }
    
    // Check quality (if config provided)
    if config.MinQuality > 0 || config.MaxQuality > 0 {
        quality, err := h.getImageQuality(req.FileData, format)
        if err != nil {
            return fmt.Errorf("failed to get image quality: %w", err)
        }
        
        if config.MinQuality > 0 && quality < config.MinQuality {
            return fmt.Errorf("image quality %d is below minimum %d", quality, config.MinQuality)
        }
        if config.MaxQuality > 0 && quality > config.MaxQuality {
            return fmt.Errorf("image quality %d exceeds maximum %d", quality, config.MaxQuality)
        }
    }
    
    return nil
}

// PDF validation
func (h *Handler) validatePDF(req *UploadRequest, config PDFValidationConfig) error {
    // Validate PDF structure
    if config.ValidateStructure {
        if err := h.validatePDFStructure(req.FileData); err != nil {
            return fmt.Errorf("PDF structure validation failed: %w", err)
        }
    }
    
    // Check page count
    if config.MinPages > 0 || config.MaxPages > 0 {
        pages, err := h.getPDFPageCount(req.FileData)
        if err != nil {
            return fmt.Errorf("failed to get PDF page count: %w", err)
        }
        
        if config.MinPages > 0 && pages < config.MinPages {
            return fmt.Errorf("PDF has %d pages, minimum required is %d", pages, config.MinPages)
        }
        if config.MaxPages > 0 && pages > config.MaxPages {
            return fmt.Errorf("PDF has %d pages, maximum allowed is %d", pages, config.MaxPages)
        }
    }
    
    // Check metadata requirements
    if config.RequireMetadata {
        metadata, err := h.getPDFMetadata(req.FileData)
        if err != nil {
            return fmt.Errorf("failed to get PDF metadata: %w", err)
        }
        
        for _, field := range config.RequiredFields {
            if metadata[field] == "" {
                return fmt.Errorf("PDF metadata field '%s' is required but not found", field)
            }
        }
    }
    
    return nil
}
```

#### Controller - Upload-Specific Validation
The controller handles only upload method validation:

```go
// Controller handles UPLOAD-SPECIFIC validation only
func (c *UserController) validateUploadMethod(ctx *gin.Context) error {
    // Check if it's multipart form data
    contentType := ctx.GetHeader("Content-Type")
    if !strings.HasPrefix(contentType, "multipart/form-data") {
        return fmt.Errorf("only multipart/form-data uploads are allowed")
    }
    
    // Check if single file upload (not multiple files)
    form, err := ctx.MultipartForm()
    if err != nil {
        return fmt.Errorf("failed to parse multipart form: %w", err)
    }
    
    files := form.File["file"]
    if len(files) == 0 {
        return fmt.Errorf("no file provided")
    }
    
    if len(files) > 1 {
        return fmt.Errorf("only single file upload is allowed for profile images")
    }
    
    return nil
}

// Alternative: Multiple file upload validation
func (c *UserController) validateMultipleFileUpload(ctx *gin.Context, maxFiles int) error {
    form, err := ctx.MultipartForm()
    if err != nil {
        return fmt.Errorf("failed to parse multipart form: %w", err)
    }
    
    files := form.File["files"] // Note: "files" for multiple uploads
    if len(files) == 0 {
        return fmt.Errorf("no files provided")
    }
    
    if len(files) > maxFiles {
        return fmt.Errorf("maximum %d files allowed, got %d", maxFiles, len(files))
    }
    
    return nil
}

// Alternative: Direct file upload validation (not multipart)
func (c *UserController) validateDirectFileUpload(ctx *gin.Context) error {
    contentType := ctx.GetHeader("Content-Type")
    if strings.HasPrefix(contentType, "multipart/form-data") {
        return fmt.Errorf("multipart uploads not allowed for this endpoint")
    }
    
    // Check if it's a direct file upload
    if !strings.HasPrefix(contentType, "application/octet-stream") && 
       !strings.HasPrefix(contentType, "image/") &&
       !strings.HasPrefix(contentType, "application/pdf") {
        return fmt.Errorf("unsupported content type: %s", contentType)
    }
    
    return nil
}
```

#### Middleware Application
Middlewares are applied by the storage package based on configuration:

```go
// Middleware types handled by storage package
type MiddlewareType string

const (
    SecurityMiddleware    MiddlewareType = "security"
    ThumbnailMiddleware   MiddlewareType = "thumbnail"
    EncryptionMiddleware  MiddlewareType = "encryption"
    AuditMiddleware       MiddlewareType = "audit"
    CDNMiddleware         MiddlewareType = "cdn"
)

func (h *Handler) applyMiddleware(ctx context.Context, req *UploadRequest, middlewareName string, config CategoryConfig) error {
    switch middlewareName {
    case "security":
        return h.applySecurityMiddleware(ctx, req, config.Security)
    case "thumbnail":
        return h.applyThumbnailMiddleware(ctx, req, config.Preview)
    case "encryption":
        return h.applyEncryptionMiddleware(ctx, req, config.Security)
    case "audit":
        return h.applyAuditMiddleware(ctx, req)
    case "cdn":
        return h.applyCDNMiddleware(ctx, req, config.Preview)
    default:
        return fmt.Errorf("unknown middleware: %s", middlewareName)
    }
}
```

// Courier Controller  
func (c *CourierController) UploadVehicleImage(ctx *gin.Context) {
    courierID := ctx.Param("id")
    
    file, err := ctx.FormFile("file")
    if err != nil {
        http_response.SendBadRequestResponse(ctx, "Invalid file")
        return
    }
    
    src, err := file.Open()
    if err != nil {
        http_response.SendInternalServerErrorResponse(ctx, "Failed to open file")
        return
    }
    defer src.Close()
    
    uploadReq := &storage.UploadRequest{
        FileData:    src,
        FileSize:    file.Size,
        ContentType: file.Header.Get("Content-Type"),
        Category:    "vehicle",
        EntityType:  "courier",
        EntityID:    courierID,
        UserID:      getCurrentUserID(ctx),
    }
    
    response, err := c.storage.Upload(ctx, uploadReq)
    if err != nil {
        http_response.SendInternalServerErrorResponse(ctx, err.Error())
        return
    }
    
    http_response.SendSuccess(ctx, response, nil, nil)
}
```

### Custom Middleware
```go
// Custom middleware for virus scanning
type VirusScanMiddleware struct {
    scanner VirusScanner
}

func (m *VirusScanMiddleware) Name() string {
    return "virus_scan"
}

func (m *VirusScanMiddleware) Process(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
    if req.Operation == "upload" {
        // Scan file for viruses
        if err := m.scanner.Scan(req.FileData); err != nil {
            return &StorageResponse{Success: false, Error: err}, nil
        }
    }
    
    return next(ctx, req)
}

// Register custom middleware
config.Middlewares = append(config.Middlewares, MiddlewareConfig{
    Name: "virus_scan",
    Enabled: true,
    Config: map[string]interface{}{
        "scanner_endpoint": "http://virus-scanner:8080",
    },
})
```

## File Structure

```

├── client/
│   ├── minio_client.go
│   ├── interfaces.go
│   └── errors.go
├── middleware/
│   ├── interface.go
│   ├── security.go
│   ├── thumbnail.go
│   ├── validation.go
│   ├── audit.go
│   ├── encryption.go
│   └── cdn.go
├── preview/
│   ├── thumbnail.go
│   ├── streaming.go
│   └── content_handler.go
├── metadata/
│   ├── file_metadata.go
│   ├── repository.go
│   └── cache.go
├── buckets/
│   ├── bucket_manager.go
│   ├── policies.go
│   └── lifecycle.go
├── utils/
│   ├── file_utils.go
│   ├── validation.go
│   └── encryption.go
├── config/
│   ├── storage_config.go
│   ├── middleware_config.go
│   └── category_config.go
└── storage.go  // Main New() function
```

This architecture provides a robust, secure, and scalable foundation for file management in the courier service backend while maintaining ease of use and preview capabilities.
