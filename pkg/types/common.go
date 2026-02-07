package types

import "time"

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

// Common error codes
const (
	ErrCodeNotFound         = "NOT_FOUND"
	ErrCodeAlreadyExists    = "ALREADY_EXISTS"
	ErrCodeInvalidArgument  = "INVALID_ARGUMENT"
	ErrCodePermissionDenied = "PERMISSION_DENIED"
	ErrCodeInternal         = "INTERNAL"
	ErrCodeUnavailable      = "UNAVAILABLE"
	ErrCodeTimeout          = "TIMEOUT"
)
