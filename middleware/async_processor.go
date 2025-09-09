package middleware

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
)

// AsyncProcessor handles background processing of tasks like thumbnail generation
type AsyncProcessor struct {
	jobQueue chan ThumbnailJob
	workers  int
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	client   *minio.Client // MinIO client
	config   AsyncConfig
	bucket   string // Storage bucket name
}

// AsyncConfig represents async processor configuration
type AsyncConfig struct {
	Workers        int           `json:"workers"`         // Number of worker goroutines
	QueueSize      int           `json:"queue_size"`      // Size of job queue
	RetryAttempts  int           `json:"retry_attempts"`  // Number of retry attempts
	RetryDelay     time.Duration `json:"retry_delay"`     // Delay between retries
	MaxConcurrency int           `json:"max_concurrency"` // Maximum concurrent jobs
}

// ThumbnailJob represents a thumbnail generation job
type ThumbnailJob struct {
	ID          string                   `json:"id"`
	FileKey     string                   `json:"file_key"`
	FileData    io.Reader                `json:"-"`
	FileSize    int64                    `json:"file_size"`
	ContentType string                   `json:"content_type"`
	Sizes       []string                 `json:"sizes"`
	BucketName  string                   `json:"bucket_name"`
	Callback    func(*ThumbnailResponse) `json:"-"`
	Metadata    map[string]interface{}   `json:"metadata"`
	CreatedAt   time.Time                `json:"created_at"`
	RetryCount  int                      `json:"retry_count"`
}

// ThumbnailResponse represents the result of thumbnail generation
type ThumbnailResponse struct {
	Success     bool            `json:"success"`
	FileKey     string          `json:"file_key"`
	Thumbnails  []ThumbnailInfo `json:"thumbnails"`
	Error       error           `json:"error,omitempty"`
	ProcessedAt time.Time       `json:"processed_at"`
	Duration    time.Duration   `json:"duration"`
}

// NewAsyncProcessor creates a new async processor
func NewAsyncProcessor(config AsyncConfig, client *minio.Client, bucket string) *AsyncProcessor {
	ctx, cancel := context.WithCancel(context.Background())

	processor := &AsyncProcessor{
		jobQueue: make(chan ThumbnailJob, config.QueueSize),
		workers:  config.Workers,
		ctx:      ctx,
		cancel:   cancel,
		client:   client,
		config:   config,
		bucket:   bucket,
	}

	// Start worker goroutines
	processor.startWorkers()

	return processor
}

// startWorkers starts the worker goroutines
func (p *AsyncProcessor) startWorkers() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// worker processes jobs from the queue
func (p *AsyncProcessor) worker(workerID int) {
	defer p.wg.Done()

	for {
		select {
		case job := <-p.jobQueue:
			p.processJob(job)
		case <-p.ctx.Done():
			return
		}
	}
}

// processJob processes a single thumbnail job
func (p *AsyncProcessor) processJob(job ThumbnailJob) {
	start := time.Now()

	// Process thumbnails
	thumbnails, err := p.generateThumbnails(job)

	duration := time.Since(start)

	// Create response
	response := &ThumbnailResponse{
		Success:     err == nil,
		FileKey:     job.FileKey,
		Thumbnails:  thumbnails,
		Error:       err,
		ProcessedAt: time.Now(),
		Duration:    duration,
	}

	// Call callback if provided
	if job.Callback != nil {
		job.Callback(response)
	}

	// Log processing result
	if err != nil {
		fmt.Printf("❌ Thumbnail generation failed for %s: %v\n", job.FileKey, err)

		// Retry if retry count is below limit
		if job.RetryCount < p.config.RetryAttempts {
			job.RetryCount++
			job.CreatedAt = time.Now()

			// Schedule retry with delay
			go func() {
				time.Sleep(p.config.RetryDelay)
				select {
				case p.jobQueue <- job:
				case <-p.ctx.Done():
				}
			}()
		}
	} else {
		fmt.Printf("✅ Thumbnail generation completed for %s in %v\n", job.FileKey, duration)
	}
}

// generateThumbnails generates thumbnails for the given job
func (p *AsyncProcessor) generateThumbnails(job ThumbnailJob) ([]ThumbnailInfo, error) {
	var thumbnails []ThumbnailInfo

	// Read the original file data
	originalData, err := p.getOriginalFile(job.FileKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get original file: %w", err)
	}
	defer originalData.Close()

	// Decode the original image
	originalImg, format, err := image.Decode(originalData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Generate thumbnails for each configured size
	for _, sizeStr := range job.Sizes {
		width, height, err := parseThumbnailSize(sizeStr)
		if err != nil {
			fmt.Printf("Invalid thumbnail size %s: %v\n", sizeStr, err)
			continue
		}

		// Generate thumbnail
		thumbnailData, err := p.createThumbnail(originalImg, width, height, format)
		if err != nil {
			fmt.Printf("Failed to create thumbnail %s: %v\n", sizeStr, err)
			continue
		}

		// Upload thumbnail to storage
		thumbnailKey := p.generateThumbnailKey(job.FileKey, sizeStr)
		thumbnailURL, err := p.uploadThumbnail(thumbnailKey, thumbnailData, format)
		if err != nil {
			fmt.Printf("Failed to upload thumbnail %s: %v\n", sizeStr, err)
			continue
		}

		// Add thumbnail info
		thumbnails = append(thumbnails, ThumbnailInfo{
			Size:     sizeStr,
			URL:      thumbnailURL,
			Width:    width,
			Height:   height,
			FileSize: int64(len(thumbnailData)),
		})
	}

	return thumbnails, nil
}

// getOriginalFile retrieves the original file from storage
func (p *AsyncProcessor) getOriginalFile(fileKey string) (io.ReadCloser, error) {
	// Get the object from MinIO
	object, err := p.client.GetObject(context.Background(), p.bucket, fileKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object from MinIO: %w", err)
	}

	return object, nil
}

// createThumbnail creates a thumbnail from the original image
func (p *AsyncProcessor) createThumbnail(originalImg image.Image, width, height int, format string) ([]byte, error) {
	// Resize the image
	resizedImg := p.resizeImage(originalImg, width, height)

	// Encode the resized image
	var buf bytes.Buffer
	switch format {
	case "jpeg":
		err := jpeg.Encode(&buf, resizedImg, &jpeg.Options{Quality: 85})
		if err != nil {
			return nil, fmt.Errorf("failed to encode JPEG thumbnail: %w", err)
		}
	case "png":
		err := png.Encode(&buf, resizedImg)
		if err != nil {
			return nil, fmt.Errorf("failed to encode PNG thumbnail: %w", err)
		}
	default:
		// Default to JPEG for other formats
		err := jpeg.Encode(&buf, resizedImg, &jpeg.Options{Quality: 85})
		if err != nil {
			return nil, fmt.Errorf("failed to encode thumbnail: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// resizeImage resizes an image to the specified dimensions
func (p *AsyncProcessor) resizeImage(img image.Image, width, height int) image.Image {
	// Create a new image with the target dimensions
	bounds := img.Bounds()
	originalWidth := bounds.Dx()
	originalHeight := bounds.Dy()

	// Calculate scaling factors
	scaleX := float64(width) / float64(originalWidth)
	scaleY := float64(height) / float64(originalHeight)

	// Use the smaller scale to maintain aspect ratio
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	// Calculate new dimensions maintaining aspect ratio
	newWidth := int(float64(originalWidth) * scale)
	newHeight := int(float64(originalHeight) * scale)

	// Create the resized image
	resized := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	// Simple nearest neighbor scaling
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			// Map to original image coordinates
			srcX := int(float64(x) / scale)
			srcY := int(float64(y) / scale)

			// Ensure we don't go out of bounds
			if srcX >= originalWidth {
				srcX = originalWidth - 1
			}
			if srcY >= originalHeight {
				srcY = originalHeight - 1
			}

			// Copy pixel
			resized.Set(x, y, img.At(srcX, srcY))
		}
	}

	return resized
}

// uploadThumbnail uploads the thumbnail to storage
func (p *AsyncProcessor) uploadThumbnail(key string, data []byte, format string) (string, error) {
	// Create a reader from the byte data
	reader := bytes.NewReader(data)

	// Determine content type based on format
	contentType := "image/jpeg"
	if format == "png" {
		contentType = "image/png"
	}

	// Upload the thumbnail to MinIO
	_, err := p.client.PutObject(
		context.Background(),
		p.bucket,
		key,
		reader,
		int64(len(data)),
		minio.PutObjectOptions{
			ContentType: contentType,
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to upload thumbnail to MinIO: %w", err)
	}

	// Generate the thumbnail URL
	thumbnailURL := fmt.Sprintf("/api/v1/files/%s/thumbnail?size=%s", key, "thumbnail")
	return thumbnailURL, nil
}

// generateThumbnailKey generates a key for the thumbnail using predictable naming
func (p *AsyncProcessor) generateThumbnailKey(originalKey, size string) string {
	// Use predictable naming pattern: original_file_key_512x512.png
	// This makes it easy for users to construct thumbnail URLs

	// Get the file extension from the original key
	ext := filepath.Ext(originalKey)
	if ext == "" {
		ext = ".jpg" // Default to jpg for thumbnails
	}

	// Remove the extension from the original key
	baseKey := strings.TrimSuffix(originalKey, ext)

	// Create the thumbnail key with size suffix
	thumbnailKey := fmt.Sprintf("%s_%s%s", baseKey, size, ext)

	return thumbnailKey
}

// SubmitJob submits a thumbnail job for processing
func (p *AsyncProcessor) SubmitJob(job ThumbnailJob) error {
	// Set job ID and creation time if not set
	if job.ID == "" {
		job.ID = fmt.Sprintf("thumb_%d", time.Now().UnixNano())
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}

	select {
	case p.jobQueue <- job:
		return nil
	case <-p.ctx.Done():
		return fmt.Errorf("async processor is shutting down")
	default:
		return fmt.Errorf("job queue is full")
	}
}

// GetStats returns processor statistics
func (p *AsyncProcessor) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"workers":         p.workers,
		"queue_size":      len(p.jobQueue),
		"max_queue_size":  p.config.QueueSize,
		"retry_attempts":  p.config.RetryAttempts,
		"retry_delay":     p.config.RetryDelay,
		"max_concurrency": p.config.MaxConcurrency,
		"is_running":      p.ctx.Err() == nil,
	}
}

// Stop stops the async processor
func (p *AsyncProcessor) Stop() {
	p.cancel()
	p.wg.Wait()
	close(p.jobQueue)
}

// DefaultAsyncConfig returns a default async processor configuration
func DefaultAsyncConfig() AsyncConfig {
	return AsyncConfig{
		Workers:        3,               // 3 worker goroutines
		QueueSize:      100,             // Queue up to 100 jobs
		RetryAttempts:  2,               // Retry failed jobs twice
		RetryDelay:     5 * time.Second, // 5 second delay between retries
		MaxConcurrency: 10,              // Maximum 10 concurrent jobs
	}
}
