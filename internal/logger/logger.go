package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"log/slog"

	"github.com/baaaht/orchestrator/internal/config"
)

// Level represents the log level
type Level slog.Level

const (
	LevelDebug Level = Level(slog.LevelDebug)
	LevelInfo  Level = Level(slog.LevelInfo)
	LevelWarn  Level = Level(slog.LevelWarn)
	LevelError Level = Level(slog.LevelError)
)

// String returns the string representation of the log level
func (l Level) String() string {
	return slog.Level(l).String()
}

// Logger wraps slog.Logger with additional functionality
type Logger struct {
	logger *slog.Logger
	mu     sync.RWMutex
	level  Level
}

// New creates a new logger with the specified configuration
func New(cfg config.LoggingConfig) (*Logger, error) {
	// Parse the log level
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}

	// Determine the output writer
	var writer io.Writer
	switch cfg.Output {
	case "stdout":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	case "":
		writer = os.Stdout
	default:
		// File output
		if err := os.MkdirAll(filepath.Dir(cfg.Output), 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
		file, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		writer = file
	}

	// Create the handler options
	opts := &slog.HandlerOptions{
		Level: slog.Level(level),
	}

	// Create the handler based on format
	var handler slog.Handler
	switch cfg.Format {
	case "json":
		handler = slog.NewJSONHandler(writer, opts)
	case "text":
		handler = slog.NewTextHandler(writer, opts)
	default:
		return nil, fmt.Errorf("invalid log format: %s (must be json or text)", cfg.Format)
	}

	// Create the logger with a default handler
	sl := slog.New(handler)

	return &Logger{
		logger: sl,
		level:  level,
	}, nil
}

// NewDefault creates a new logger with default settings
func NewDefault() (*Logger, error) {
	cfg := config.DefaultLoggingConfig()
	return New(cfg)
}

// parseLevel converts a string log level to a Level
func parseLevel(level string) (Level, error) {
	switch level {
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	default:
		return LevelInfo, fmt.Errorf("unknown log level: %s", level)
	}
}

// With returns a new logger with additional key-value pairs
func (l *Logger) With(args ...any) *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return &Logger{
		logger: l.logger.With(args...),
		level:  l.level,
	}
}

// WithGroup returns a new logger with a group prefix
func (l *Logger) WithGroup(name string) *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return &Logger{
		logger: l.logger.WithGroup(name),
		level:  l.level,
	}
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, args ...any) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.logger.Debug(msg, args...)
}

// Info logs an info message
func (l *Logger) Info(msg string, args ...any) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.logger.Info(msg, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, args ...any) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.logger.Warn(msg, args...)
}

// Error logs an error message
func (l *Logger) Error(msg string, args ...any) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.logger.Error(msg, args...)
}

// DebugCtx logs a debug message with context
func (l *Logger) DebugCtx(msg string, args ...any) {
	l.Debug(msg, args...)
}

// InfoCtx logs an info message with context
func (l *Logger) InfoCtx(msg string, args ...any) {
	l.Info(msg, args...)
}

// WarnCtx logs a warning message with context
func (l *Logger) WarnCtx(msg string, args ...any) {
	l.Warn(msg, args...)
}

// ErrorCtx logs an error message with context
func (l *Logger) ErrorCtx(msg string, args ...any) {
	l.Error(msg, args...)
}

// SetLevel changes the log level dynamically
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level

	// Update the handler's level if it's a Leveler
	if h, ok := l.logger.Handler().(*slog.JSONHandler); ok {
		// We need to recreate the handler with the new level
		opts := &slog.HandlerOptions{Level: slog.Level(level)}
		l.logger = slog.New(h.Handler())
	}
}

// GetLevel returns the current log level
func (l *Logger) GetLevel() Level {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.level
}

// Enabled returns true if logging is enabled for the given level
func (l *Logger) Enabled(level Level) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return level >= l.level
}

// String returns a string representation of the logger
func (l *Logger) String() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return fmt.Sprintf("Logger{Level: %s}", l.level)
}

// Flush ensures all buffered logs are written (for compatibility with future buffering)
func (l *Logger) Flush() error {
	// slog writes immediately, so this is a no-op
	// Kept for API compatibility
	return nil
}

// Close closes any open resources (file handles, etc.)
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if the handler is writing to a file
	// and close it if necessary
	if h, ok := l.logger.Handler().(*slog.JSONHandler); ok {
		// We can't easily access the underlying writer from slog
		// This is a limitation of the current implementation
		// In production, you might want to track the writer separately
	}
	return nil
}

// global logger instance
var (
	globalLogger *Logger
	globalOnce   sync.Once
)

// InitGlobal initializes the global logger with the specified configuration
func InitGlobal(cfg config.LoggingConfig) error {
	var initErr error
	globalOnce.Do(func() {
		logger, err := New(cfg)
		if err != nil {
			initErr = err
			return
		}
		globalLogger = logger
	})
	return initErr
}

// Global returns the global logger instance
func Global() *Logger {
	if globalLogger == nil {
		// Initialize with default settings if not already initialized
		logger, err := NewDefault()
		if err != nil {
			// Fall back to a basic logger
			globalLogger = &Logger{
				logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
					Level: slog.LevelInfo,
				})),
				level: LevelInfo,
			}
		} else {
			globalLogger = logger
		}
	}
	return globalLogger
}

// SetGlobal sets the global logger instance
func SetGlobal(l *Logger) {
	globalLogger = l
	globalOnce = sync.Once{}
}

// Package-level convenience functions that use the global logger

// Debug logs a debug message using the global logger
func Debug(msg string, args ...any) {
	Global().Debug(msg, args...)
}

// Info logs an info message using the global logger
func Info(msg string, args ...any) {
	Global().Info(msg, args...)
}

// Warn logs a warning message using the global logger
func Warn(msg string, args ...any) {
	Global().Warn(msg, args...)
}

// Error logs an error message using the global logger
func Error(msg string, args ...any) {
	Global().Error(msg, args...)
}

// With returns a new global logger with additional key-value pairs
func With(args ...any) *Logger {
	return Global().With(args...)
}

// WithGroup returns a new global logger with a group prefix
func WithGroup(name string) *Logger {
	return Global().WithGroup(name)
}

// SetLevel changes the global logger's log level
func SetLevel(level Level) {
	Global().SetLevel(level)
}

// GetLevel returns the global logger's current log level
func GetLevel() Level {
	return Global().GetLevel()
}
