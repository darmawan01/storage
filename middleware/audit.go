package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// AuditMiddleware handles audit logging for storage operations
type AuditMiddleware struct {
	config AuditConfig
	logger Logger
}

// AuditConfig represents audit middleware configuration
type AuditConfig struct {
	Enabled     bool     `json:"enabled"`
	LogLevel    string   `json:"log_level"`   // "info", "warn", "error"
	LogFormat   string   `json:"log_format"`  // "json", "text"
	Operations  []string `json:"operations"`  // ["upload", "download", "delete", "preview"]
	Fields      []string `json:"fields"`      // ["user_id", "file_key", "operation", "timestamp"]
	Destination string   `json:"destination"` // "stdout", "file", "database"
	FilePath    string   `json:"file_path,omitempty"`
}

// Logger interface for audit logging
type Logger interface {
	Info(msg string, fields map[string]interface{})
	Warn(msg string, fields map[string]interface{})
	Error(msg string, fields map[string]interface{})
}

// DefaultLogger implements the Logger interface using Go's standard log package
type DefaultLogger struct{}

func (l *DefaultLogger) Info(msg string, fields map[string]interface{}) {
	l.log("INFO", msg, fields)
}

func (l *DefaultLogger) Warn(msg string, fields map[string]interface{}) {
	l.log("WARN", msg, fields)
}

func (l *DefaultLogger) Error(msg string, fields map[string]interface{}) {
	l.log("ERROR", msg, fields)
}

func (l *DefaultLogger) log(level, msg string, fields map[string]interface{}) {
	timestamp := time.Now().Format(time.RFC3339)

	// Create log entry
	entry := map[string]interface{}{
		"timestamp": timestamp,
		"level":     level,
		"message":   msg,
	}

	// Add fields
	for k, v := range fields {
		entry[k] = v
	}

	// Convert to JSON
	jsonData, err := json.Marshal(entry)
	if err != nil {
		log.Printf("Failed to marshal audit log: %v", err)
		return
	}

	// Log to stdout
	log.Println(string(jsonData))
}

// AuditEvent represents an audit event
type AuditEvent struct {
	Timestamp   time.Time              `json:"timestamp"`
	Operation   string                 `json:"operation"`
	UserID      string                 `json:"user_id,omitempty"`
	FileKey     string                 `json:"file_key,omitempty"`
	FileSize    int64                  `json:"file_size,omitempty"`
	ContentType string                 `json:"content_type,omitempty"`
	Category    string                 `json:"category,omitempty"`
	EntityType  string                 `json:"entity_type,omitempty"`
	EntityID    string                 `json:"entity_id,omitempty"`
	Success     bool                   `json:"success"`
	Error       string                 `json:"error,omitempty"`
	IPAddress   string                 `json:"ip_address,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// NewAuditMiddleware creates a new audit middleware
func NewAuditMiddleware(config AuditConfig, logger Logger) *AuditMiddleware {
	if logger == nil {
		logger = &DefaultLogger{}
	}

	return &AuditMiddleware{
		config: config,
		logger: logger,
	}
}

// Name returns the middleware name
func (m *AuditMiddleware) Name() string {
	return "audit"
}

// Process processes the request through audit middleware
func (m *AuditMiddleware) Process(ctx context.Context, req *StorageRequest, next MiddlewareFunc) (*StorageResponse, error) {
	// Check if audit is enabled
	if !m.config.Enabled {
		return next(ctx, req)
	}

	// Check if this operation should be audited
	if !m.shouldAudit(req.Operation) {
		return next(ctx, req)
	}

	// Create audit event
	event := m.createAuditEvent(req)

	// Process with next middleware
	response, err := next(ctx, req)

	// Update event with response information
	event.Success = response != nil && response.Success
	if err != nil {
		event.Error = err.Error()
	}
	if response != nil {
		event.FileSize = response.FileSize
		event.ContentType = response.ContentType
	}

	// Log the audit event
	m.logAuditEvent(event)

	return response, err
}

// shouldAudit checks if the operation should be audited
func (m *AuditMiddleware) shouldAudit(operation string) bool {
	if len(m.config.Operations) == 0 {
		return true // Audit all operations if none specified
	}

	for _, op := range m.config.Operations {
		if op == operation {
			return true
		}
	}
	return false
}

// createAuditEvent creates an audit event from the request
func (m *AuditMiddleware) createAuditEvent(req *StorageRequest) *AuditEvent {
	event := &AuditEvent{
		Timestamp:   time.Now(),
		Operation:   req.Operation,
		UserID:      req.UserID,
		FileKey:     req.FileKey,
		FileSize:    req.FileSize,
		ContentType: req.ContentType,
		Category:    req.Category,
		EntityType:  req.EntityType,
		EntityID:    req.EntityID,
		Metadata:    req.Metadata,
	}

	// Add additional context from the request
	if req.Config != nil {
		if ip, ok := req.Config["ip_address"].(string); ok {
			event.IPAddress = ip
		}
		if ua, ok := req.Config["user_agent"].(string); ok {
			event.UserAgent = ua
		}
	}

	return event
}

// logAuditEvent logs the audit event
func (m *AuditMiddleware) logAuditEvent(event *AuditEvent) {
	// Create log message
	msg := fmt.Sprintf("Storage operation: %s", event.Operation)

	// Create fields map
	fields := make(map[string]interface{})

	// Add fields based on configuration
	for _, field := range m.config.Fields {
		switch field {
		case "user_id":
			fields["user_id"] = event.UserID
		case "file_key":
			fields["file_key"] = event.FileKey
		case "operation":
			fields["operation"] = event.Operation
		case "timestamp":
			fields["timestamp"] = event.Timestamp
		case "file_size":
			fields["file_size"] = event.FileSize
		case "content_type":
			fields["content_type"] = event.ContentType
		case "category":
			fields["category"] = event.Category
		case "entity_type":
			fields["entity_type"] = event.EntityType
		case "entity_id":
			fields["entity_id"] = event.EntityID
		case "success":
			fields["success"] = event.Success
		case "error":
			fields["error"] = event.Error
		case "ip_address":
			fields["ip_address"] = event.IPAddress
		case "user_agent":
			fields["user_agent"] = event.UserAgent
		}
	}

	// Add metadata if present
	if len(event.Metadata) > 0 {
		fields["metadata"] = event.Metadata
	}

	// Log based on success/failure
	if event.Success {
		m.logger.Info(msg, fields)
	} else {
		m.logger.Error(msg, fields)
	}
}

// LogSecurityEvent logs a security-related event
func (m *AuditMiddleware) LogSecurityEvent(eventType, userID, resourceID, action string, success bool, details map[string]interface{}) {
	if !m.config.Enabled {
		return
	}

	msg := fmt.Sprintf("Security event: %s", eventType)
	fields := map[string]interface{}{
		"event_type":  eventType,
		"user_id":     userID,
		"resource_id": resourceID,
		"action":      action,
		"success":     success,
		"timestamp":   time.Now(),
	}

	// Add details
	for k, v := range details {
		fields[k] = v
	}

	if success {
		m.logger.Info(msg, fields)
	} else {
		m.logger.Warn(msg, fields)
	}
}

// LogAccessEvent logs an access-related event
func (m *AuditMiddleware) LogAccessEvent(userID, resourceID, action string, success bool, details map[string]interface{}) {
	if !m.config.Enabled {
		return
	}

	msg := fmt.Sprintf("Access event: %s", action)
	fields := map[string]interface{}{
		"user_id":     userID,
		"resource_id": resourceID,
		"action":      action,
		"success":     success,
		"timestamp":   time.Now(),
	}

	// Add details
	for k, v := range details {
		fields[k] = v
	}

	if success {
		m.logger.Info(msg, fields)
	} else {
		m.logger.Warn(msg, fields)
	}
}

// LogErrorEvent logs an error event
func (m *AuditMiddleware) LogErrorEvent(operation, userID, errorMsg string, details map[string]interface{}) {
	if !m.config.Enabled {
		return
	}

	msg := fmt.Sprintf("Error in operation: %s", operation)
	fields := map[string]interface{}{
		"operation": operation,
		"user_id":   userID,
		"error":     errorMsg,
		"timestamp": time.Now(),
	}

	// Add details
	for k, v := range details {
		fields[k] = v
	}

	m.logger.Error(msg, fields)
}
