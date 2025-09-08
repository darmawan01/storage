package errors

// Error types
type StorageError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e *StorageError) Error() string {
	return e.Message
}

var (
	ErrFileNotFound     = &StorageError{Code: "FILE_NOT_FOUND", Message: "File not found"}
	ErrAccessDenied     = &StorageError{Code: "ACCESS_DENIED", Message: "Access denied"}
	ErrInvalidFile      = &StorageError{Code: "INVALID_FILE", Message: "Invalid file"}
	ErrFileTooLarge     = &StorageError{Code: "FILE_TOO_LARGE", Message: "File too large"}
	ErrUnsupportedType  = &StorageError{Code: "UNSUPPORTED_TYPE", Message: "Unsupported file type"}
	ErrValidationFailed = &StorageError{Code: "VALIDATION_FAILED", Message: "Validation failed"}
	ErrBucketNotFound   = &StorageError{Code: "BUCKET_NOT_FOUND", Message: "Bucket not found"}
	ErrUploadFailed     = &StorageError{Code: "UPLOAD_FAILED", Message: "Upload failed"}
	ErrDownloadFailed   = &StorageError{Code: "DOWNLOAD_FAILED", Message: "Download failed"}
	ErrDeleteFailed     = &StorageError{Code: "DELETE_FAILED", Message: "Delete failed"}
)
