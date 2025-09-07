package middleware

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// CDNMiddleware handles CDN integration
type CDNMiddleware struct {
	config CDNConfig
}

// CDNConfig represents CDN middleware configuration
type CDNConfig struct {
	Enabled       bool              `json:"enabled"`
	CDNEndpoint   string            `json:"cdn_endpoint"`
	CDNProvider   string            `json:"cdn_provider"` // "cloudflare", "aws_cloudfront", "custom"
	CacheTTL      int               `json:"cache_ttl"`    // seconds
	PurgeOnUpdate bool              `json:"purge_on_update"`
	Headers       map[string]string `json:"headers,omitempty"`
	Transform     CDNTransform      `json:"transform,omitempty"`
}

// CDNTransform represents CDN transformation settings
type CDNTransform struct {
	EnableWebP     bool     `json:"enable_webp,omitempty"`
	EnableAVIF     bool     `json:"enable_avif,omitempty"`
	Quality        int      `json:"quality,omitempty"`
	Width          int      `json:"width,omitempty"`
	Height         int      `json:"height,omitempty"`
	Format         string   `json:"format,omitempty"`
	AllowedFormats []string `json:"allowed_formats,omitempty"`
}

// NewCDNMiddleware creates a new CDN middleware
func NewCDNMiddleware(config CDNConfig) *CDNMiddleware {
	return &CDNMiddleware{
		config: config,
	}
}

// Name returns the middleware name
func (m *CDNMiddleware) Name() string {
	return "cdn"
}

// Process processes the request through CDN middleware
func (m *CDNMiddleware) Process(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
	// Check if CDN is enabled
	if !m.config.Enabled {
		return next(ctx, req)
	}

	// Process with next middleware first
	response, err := next(ctx, req)
	if err != nil {
		return response, err
	}

	// Apply CDN transformations
	if response.Success {
		m.applyCDNTransformations(response, req)
	}

	return response, nil
}

// applyCDNTransformations applies CDN transformations to the response
func (m *CDNMiddleware) applyCDNTransformations(response *StorageResponse, req *StorageRequest) {
	// Generate CDN URL for the file
	if response.FileURL != "" {
		response.FileURL = m.generateCDNURL(response.FileURL)
	}

	// Generate CDN URLs for thumbnails
	for i, thumbnail := range response.Thumbnails {
		response.Thumbnails[i].URL = m.generateCDNURL(thumbnail.URL)
	}

	// Add CDN headers to metadata
	if response.Metadata == nil {
		response.Metadata = make(map[string]interface{})
	}
	response.Metadata["cdn_enabled"] = true
	response.Metadata["cdn_endpoint"] = m.config.CDNEndpoint
	response.Metadata["cache_ttl"] = m.config.CacheTTL
}

// generateCDNURL generates a CDN URL for the given file URL
func (m *CDNMiddleware) generateCDNURL(fileURL string) string {
	// Parse the original URL
	originalURL, err := url.Parse(fileURL)
	if err != nil {
		return fileURL // Return original if parsing fails
	}

	// Extract the path from the original URL
	path := originalURL.Path
	if originalURL.RawQuery != "" {
		path += "?" + originalURL.RawQuery
	}

	// Generate CDN URL based on provider
	switch m.config.CDNProvider {
	case "cloudflare":
		return m.generateCloudflareURL(path)
	case "aws_cloudfront":
		return m.generateCloudFrontURL(path)
	case "custom":
		return m.generateCustomURL(path)
	default:
		return fileURL // Return original if provider not supported
	}
}

// generateCloudflareURL generates a Cloudflare CDN URL
func (m *CDNMiddleware) generateCloudflareURL(path string) string {
	// Remove leading slash if present
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}

	// Construct Cloudflare URL
	cdnURL := strings.TrimSuffix(m.config.CDNEndpoint, "/")
	return fmt.Sprintf("%s/%s", cdnURL, path)
}

// generateCloudFrontURL generates an AWS CloudFront CDN URL
func (m *CDNMiddleware) generateCloudFrontURL(path string) string {
	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Construct CloudFront URL
	cdnURL := strings.TrimSuffix(m.config.CDNEndpoint, "/")
	return fmt.Sprintf("%s%s", cdnURL, path)
}

// generateCustomURL generates a custom CDN URL
func (m *CDNMiddleware) generateCustomURL(path string) string {
	// Remove leading slash if present
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}

	// Construct custom URL
	cdnURL := strings.TrimSuffix(m.config.CDNEndpoint, "/")
	return fmt.Sprintf("%s/%s", cdnURL, path)
}

// generateTransformedURL generates a URL with CDN transformations
func (m *CDNMiddleware) generateTransformedURL(baseURL string, transform CDNTransform) string {
	if !m.config.Transform.EnableWebP && !m.config.Transform.EnableAVIF && transform.Quality == 0 {
		return baseURL
	}

	// Parse the base URL
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}

	// Add transformation parameters
	params := u.Query()

	if transform.EnableWebP {
		params.Set("format", "webp")
	} else if transform.EnableAVIF {
		params.Set("format", "avif")
	}

	if transform.Quality > 0 {
		params.Set("quality", fmt.Sprintf("%d", transform.Quality))
	}

	if transform.Width > 0 {
		params.Set("width", fmt.Sprintf("%d", transform.Width))
	}

	if transform.Height > 0 {
		params.Set("height", fmt.Sprintf("%d", transform.Height))
	}

	if transform.Format != "" {
		params.Set("format", transform.Format)
	}

	u.RawQuery = params.Encode()
	return u.String()
}

// PurgeCache purges the CDN cache for a specific URL
func (m *CDNMiddleware) PurgeCache(ctx context.Context, fileURL string) error {
	if !m.config.Enabled {
		return nil
	}

	// Generate CDN URL
	cdnURL := m.generateCDNURL(fileURL)

	// Purge based on provider
	switch m.config.CDNProvider {
	case "cloudflare":
		return m.purgeCloudflareCache(ctx, cdnURL)
	case "aws_cloudfront":
		return m.purgeCloudFrontCache(ctx, cdnURL)
	case "custom":
		return m.purgeCustomCache(ctx, cdnURL)
	default:
		return fmt.Errorf("unsupported CDN provider: %s", m.config.CDNProvider)
	}
}

// purgeCloudflareCache purges Cloudflare cache
func (m *CDNMiddleware) purgeCloudflareCache(ctx context.Context, url string) error {
	// TODO: Implement Cloudflare cache purging
	// This would involve calling the Cloudflare API
	return nil
}

// purgeCloudFrontCache purges AWS CloudFront cache
func (m *CDNMiddleware) purgeCloudFrontCache(ctx context.Context, url string) error {
	// TODO: Implement CloudFront cache invalidation
	// This would involve calling the AWS CloudFront API
	return nil
}

// purgeCustomCache purges custom CDN cache
func (m *CDNMiddleware) purgeCustomCache(ctx context.Context, url string) error {
	// TODO: Implement custom CDN cache purging
	// This would involve calling the custom CDN API
	return nil
}

// GetCacheHeaders returns cache headers for the CDN
func (m *CDNMiddleware) GetCacheHeaders() map[string]string {
	headers := make(map[string]string)

	// Add default cache headers
	headers["Cache-Control"] = fmt.Sprintf("public, max-age=%d", m.config.CacheTTL)
	headers["CDN-Cache-Control"] = fmt.Sprintf("max-age=%d", m.config.CacheTTL)

	// Add custom headers
	for k, v := range m.config.Headers {
		headers[k] = v
	}

	return headers
}

// IsCDNEnabled checks if CDN is enabled
func (m *CDNMiddleware) IsCDNEnabled() bool {
	return m.config.Enabled
}

// GetCDNEndpoint returns the CDN endpoint
func (m *CDNMiddleware) GetCDNEndpoint() string {
	return m.config.CDNEndpoint
}

// GetCacheTTL returns the cache TTL
func (m *CDNMiddleware) GetCacheTTL() time.Duration {
	return time.Duration(m.config.CacheTTL) * time.Second
}

// ShouldPurgeOnUpdate checks if cache should be purged on update
func (m *CDNMiddleware) ShouldPurgeOnUpdate() bool {
	return m.config.PurgeOnUpdate
}
