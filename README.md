# Cat & Dog Photo Storage API

A simple and clean MinIO storage API with automatic thumbnail generation, metadata callbacks, and direct/presigned downloads. Perfect for photo storage with clear cat and dog examples.

## 🚀 Quick Start

### 1. Start MinIO

```bash
# Start MinIO with Docker Compose
docker-compose up -d
```

### 2. Start API Server

```bash
# Run the API server
go run examples/main.go
```

### 3. Test the API

```bash
# Check health
curl http://localhost:8080/api/v1/health

# Upload cat photo
curl -X POST -F 'file=@cat.jpg' http://localhost:8080/api/v1/cats/123/upload

# Upload dog photo
curl -X POST -F 'file=@dog.jpg' http://localhost:8080/api/v1/dogs/456/upload

# Run the test script
./test_cat_dog.sh
```

## 📁 Project Structure

```
storage/
├── pkg/storage/           # Core storage package
│   ├── interfaces.go      # Core interfaces and types
│   ├── config.go          # Configuration structures
│   ├── registry.go        # Storage registry management
│   ├── handler.go         # Storage handler implementation
│   ├── validation.go      # File validation logic
│   ├── middleware.go      # Middleware system
│   └── storage.go         # Main package entry point
├── examples/              # Example implementations
│   ├── gin_server.go      # Complete Gin web server
│   └── test.html          # Web testing interface
├── docker-compose.yml     # MinIO setup
├── go.mod                 # Go module definition
└── README.md             # This file
```

## 🔧 Configuration

### MinIO Setup

The `docker-compose.yml` sets up MinIO with:
- **API Port**: 9000
- **Console Port**: 9001
- **Credentials**: minioadmin/minioadmin
- **Default Bucket**: myapp-storage

### API Configuration

The API is configured for:
- **Profile Images**: 5MB max, JPEG/PNG/WebP, 100x100 to 2048x2048
- **Documents**: 10MB max, PDF/JPEG/PNG, metadata required
- **Validation**: Comprehensive file validation
- **Security**: Authentication and authorization ready

### Metadata Callback Support

The library focuses purely on MinIO operations and provides a simple callback mechanism for metadata storage:

```go
// Define your metadata storage callback
func (s *MyMetadataStorage) StoreFileMetadata(ctx context.Context, metadata *interfaces.FileMetadata) error {
    // Store metadata in your preferred system (database, Redis, etc.)
    return s.database.Save(metadata)
}

// Use in handler configuration
handlerConfig := &handler.HandlerConfig{
    BasePath: "user",
    Categories: map[string]category.CategoryConfig{...},
    MetadataCallback: myMetadataStorage.StoreFileMetadata,
}
```

**Note**: The library does not provide built-in metadata storage. Users must implement their own metadata storage system (database, Redis, etc.) and use the callback to store file metadata after successful uploads.

## 📋 API Endpoints

### Health & Test
- `GET /api/v1/health` - Health check
- `GET /api/v1/test/upload` - Upload instructions
- `GET /api/v1/test/validation` - Validation rules
- `GET /api/v1/test/metadata` - Show stored file metadata

### Cat Photo Operations
- `POST /api/v1/cats/{id}/upload` - Upload cat photo
- `GET /api/v1/cats/{id}/files/{fileId}` - Download cat file
- `DELETE /api/v1/cats/{id}/files/{fileId}` - Delete cat file
- `GET /api/v1/cats/{id}/files` - List cat files

### Dog Photo Operations
- `POST /api/v1/dogs/{id}/upload` - Upload dog photo
- `GET /api/v1/dogs/{id}/files/{fileId}` - Download dog file
- `DELETE /api/v1/dogs/{id}/files/{fileId}` - Delete dog file
- `GET /api/v1/dogs/{id}/files` - List dog files

### File Preview Operations
- `GET /api/v1/files/{fileId}/preview` - Preview file
- `GET /api/v1/files/{fileId}/thumbnail` - Get thumbnail
- `GET /api/v1/files/{fileId}/stream` - Stream file

## 🧪 Testing

### Web Interface

1. Open `examples/test.html` in your browser
2. Click "Check Health" to verify the API
3. Upload test files using the web form
4. Check responses for file keys and URLs

### Curl Examples

```bash
# Check health
curl http://localhost:8080/api/v1/health

# Upload profile image
curl -X POST -F 'file=@image.jpg' http://localhost:8080/api/v1/users/123/profile/upload

# Upload document
curl -X POST -F 'file=@document.pdf' -F 'title=Test Document' -F 'author=Test Author' http://localhost:8080/api/v1/users/123/documents/upload

# Download file
curl -O http://localhost:8080/api/v1/users/123/files/profile/file123.jpg
```

## 🛡️ Security Features

- **Access Control**: Role-based access control
- **File Validation**: Comprehensive file type and size validation
- **Encryption**: File encryption at rest
- **Audit Logging**: Request/response logging
- **Authentication**: JWT token support

## 📊 Validation Rules

### Profile Images
- **Max Size**: 5MB
- **Min Size**: 1KB
- **Allowed Types**: image/jpeg, image/png, image/webp
- **Dimensions**: 100x100 to 2048x2048 pixels
- **Quality**: 60-95%

### Documents
- **Max Size**: 10MB
- **Min Size**: 1KB
- **Allowed Types**: application/pdf, image/jpeg, image/png
- **PDF Requirements**: Valid structure, 1-100 pages, metadata required

## 🚀 Production Deployment

### Environment Variables

```bash
export MINIO_ENDPOINT=localhost:9000
export MINIO_ACCESS_KEY=minioadmin
export MINIO_SECRET_KEY=minioadmin
export MINIO_USE_SSL=false
export MINIO_REGION=us-east-1
export MINIO_BUCKET_NAME=myapp-storage
```

### Docker Production

```bash
# Build the API image
docker build -t storage-api .

# Run with MinIO
docker-compose -f docker-compose.prod.yml up -d
```

## 🔍 Monitoring

### Health Checks

- MinIO connectivity
- Storage handler status
- File validation system
- API response times

### Logging

- Structured logging with context
- Request/response logging
- Error tracking
- Audit trails

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## 📄 License

MIT License - see LICENSE file for details.

---

**Built with ❤️ for clean, scalable file storage**
