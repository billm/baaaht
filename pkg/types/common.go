package types

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Status represents the operational status of components
type Status string

const (
	StatusUnknown    Status = "unknown"
	StatusStarting   Status = "starting"
	StatusRunning    Status = "running"
	StatusStopping   Status = "stopping"
	StatusStopped    Status = "stopped"
	StatusError      Status = "error"
	StatusTerminated Status = "terminated"
)

// Health represents the health state of a component
type Health string

const (
	HealthUnknown   Health = "unknown"
	Healthy         Health = "healthy"
	Unhealthy       Health = "unhealthy"
	Degraded        Health = "degraded"
	HealthChecking  Health = "checking"
)

// ID represents a unique identifier
type ID string

// NewID generates a new ID from a string
func NewID(s string) ID {
	return ID(s)
}

// String returns the string representation of the ID
func (i ID) String() string {
	return string(i)
}

// IsEmpty returns true if the ID is empty
func (i ID) IsEmpty() bool {
	return string(i) == ""
}

// GenerateID generates a new unique identifier
func GenerateID() ID {
	b := make([]byte, 16)
	// Note: In a real scenario, handle the error properly
	// For now, we'll use time-based fallback if rand fails
	if _, err := rand.Read(b); err == nil {
		return ID(hex.EncodeToString(b))
	}
	// Fallback to time-based ID
	return ID(hex.EncodeToString([]byte(time.Now().String())))
}

// Timestamp represents a point in time
type Timestamp struct {
	time.Time
}

// NewTimestamp creates a new timestamp from the current time
func NewTimestamp() Timestamp {
	return Timestamp{Time: time.Now()}
}

// NewTimestampFromTime creates a new timestamp from a time.Time
func NewTimestampFromTime(t time.Time) Timestamp {
	return Timestamp{Time: t}
}

// IsZero returns true if the timestamp is zero
func (t Timestamp) IsZero() bool {
	return t.Time.IsZero()
}

// Error represents an error with additional context
type Error struct {
	Code    string
	Message string
	Err     error
}

// Error returns the error message
func (e *Error) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

// Unwrap returns the underlying error
func (e *Error) Unwrap() error {
	return e.Err
}

// NewError creates a new error with code and message
func NewError(code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// WrapError wraps an existing error with code and message
func WrapError(code, message string, err error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// IsErrCode checks if an error has a specific error code
func IsErrCode(err error, code string) bool {
	if e, ok := err.(*Error); ok {
		return e.Code == code
	}
	return false
}

// GetErrorCode returns the error code from an error
func GetErrorCode(err error) string {
	if e, ok := err.(*Error); ok {
		return e.Code
	}
	return ""
}

// Common error codes
const (
	ErrCodeNotFound           = "NOT_FOUND"
	ErrCodeAlreadyExists      = "ALREADY_EXISTS"
	ErrCodeInvalidArgument    = "INVALID_ARGUMENT"
	ErrCodeInvalid            = "INVALID"
	ErrCodePermission         = "PERMISSION"
	ErrCodePermissionDenied   = "PERMISSION_DENIED"
	ErrCodeInternal           = "INTERNAL"
	ErrCodeUnavailable        = "UNAVAILABLE"
	ErrCodeTimeout            = "TIMEOUT"
	ErrCodeCanceled           = "CANCELED"
	ErrCodeHandlerFailed      = "HANDLER_FAILED"
	ErrCodePartialFailure     = "PARTIAL_FAILURE"
	ErrCodeFiltered           = "FILTERED"
	ErrCodeRateLimited        = "RATE_LIMITED"
	ErrCodeDuplicate          = "DUPLICATE"
	ErrCodeFailedPrecondition = "FAILED_PRECONDITION"
	ErrCodeResourceExhausted  = "RESOURCE_EXHAUSTED"
)
