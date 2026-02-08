package logger

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/billm/baaaht/orchestrator/internal/config"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.LoggingConfig
		wantErr bool
	}{
		{
			name: "valid json config to stdout",
			cfg: config.LoggingConfig{
				Level:  "info",
				Format: "json",
				Output: "stdout",
			},
			wantErr: false,
		},
		{
			name: "valid text config to stderr",
			cfg: config.LoggingConfig{
				Level:  "debug",
				Format: "text",
				Output: "stderr",
			},
			wantErr: false,
		},
		{
			name: "invalid log level",
			cfg: config.LoggingConfig{
				Level:  "invalid",
				Format: "json",
				Output: "stdout",
			},
			wantErr: true,
		},
		{
			name: "invalid log format",
			cfg: config.LoggingConfig{
				Level:  "info",
				Format: "invalid",
				Output: "stdout",
			},
			wantErr: true,
		},
		{
			name: "empty output defaults to stdout",
			cfg: config.LoggingConfig{
				Level:  "info",
				Format: "json",
				Output: "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := New(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && logger == nil {
				t.Error("New() returned nil logger without error")
			}
		})
	}
}

func TestNewDefaultLogger(t *testing.T) {
	logger, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error = %v", err)
	}
	if logger == nil {
		t.Fatal("NewDefault() returned nil logger")
	}
	if logger.GetLevel() != LevelInfo {
		t.Errorf("NewDefault() level = %v, want %v", logger.GetLevel(), LevelInfo)
	}
}

func TestLoggerLevels(t *testing.T) {
	cfg := config.LoggingConfig{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
	}
	_, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	tests := []struct {
		name string
		cfg  string
		want Level
	}{
		{"debug level", "debug", LevelDebug},
		{"info level", "info", LevelInfo},
		{"warn level", "warn", LevelWarn},
		{"error level", "error", LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg.Level = tt.cfg
			logger, err := New(cfg)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if logger.GetLevel() != tt.want {
				t.Errorf("GetLevel() = %v, want %v", logger.GetLevel(), tt.want)
			}
		})
	}
}

func TestLoggerOutput(t *testing.T) {
	// Test with stdout (buffered)
	cfg := config.LoggingConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Log messages
	logger.Info("test message", "key", "value")
	logger.Debug("debug message", "key", "value")
	logger.Warn("warn message", "key", "value")
	logger.Error("error message", "key", "value")

	// If we got here without panicking, the test passes
	if logger.String() == "" {
		t.Error("Logger.String() returned empty string")
	}
}

func TestLoggerWith(t *testing.T) {
	cfg := config.LoggingConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	}
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Test With()
	logger2 := logger.With("service", "test")
	if logger2 == nil {
		t.Fatal("With() returned nil logger")
	}

	// Test WithGroup()
	logger3 := logger.WithGroup("test-group")
	if logger3 == nil {
		t.Fatal("WithGroup() returned nil logger")
	}
}

func TestLoggerSetLevel(t *testing.T) {
	cfg := config.LoggingConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	}
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Test SetLevel()
	logger.SetLevel(LevelDebug)
	if logger.GetLevel() != LevelDebug {
		t.Errorf("SetLevel() did not change level, got %v, want %v", logger.GetLevel(), LevelDebug)
	}

	logger.SetLevel(LevelError)
	if logger.GetLevel() != LevelError {
		t.Errorf("SetLevel() did not change level, got %v, want %v", logger.GetLevel(), LevelError)
	}
}

func TestLoggerEnabled(t *testing.T) {
	cfg := config.LoggingConfig{
		Level:  "warn",
		Format: "json",
		Output: "stdout",
	}
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	tests := []struct {
		name     string
		level    Level
		enabled  bool
	}{
		{"debug disabled at warn", LevelDebug, false},
		{"info disabled at warn", LevelInfo, false},
		{"warn enabled at warn", LevelWarn, true},
		{"error enabled at warn", LevelError, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := logger.Enabled(tt.level); got != tt.enabled {
				t.Errorf("Enabled(%v) = %v, want %v", tt.level, got, tt.enabled)
			}
		})
	}
}

func TestLoggerFlush(t *testing.T) {
	cfg := config.LoggingConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	}
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Flush should not error
	if err := logger.Flush(); err != nil {
		t.Errorf("Flush() error = %v", err)
	}
}

func TestLoggerClose(t *testing.T) {
	cfg := config.LoggingConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	}
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Close should not error
	if err := logger.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestGlobalLogger(t *testing.T) {
	// Reset global logger
	globalLogger = nil
	globalOnce = sync.Once{}

	// Test Global() creates default logger
	logger := Global()
	if logger == nil {
		t.Fatal("Global() returned nil logger")
	}

	// Test SetGlobal()
	newLogger, err := New(config.LoggingConfig{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	SetGlobal(newLogger)

	// Verify global logger was updated
	if Global().GetLevel() != LevelDebug {
		t.Errorf("Global() level = %v, want %v", Global().GetLevel(), LevelDebug)
	}
}

func TestGlobalConvenienceFunctions(t *testing.T) {
	// Reset global logger
	globalLogger = nil
	globalOnce = sync.Once{}

	// These should not panic
	Debug("test debug", "key", "value")
	Info("test info", "key", "value")
	Warn("test warn", "key", "value")
	Error("test error", "key", "value")

	// Test With and WithGroup
	logger2 := With("service", "test")
	if logger2 == nil {
		t.Fatal("With() returned nil logger")
	}

	logger3 := WithGroup("test-group")
	if logger3 == nil {
		t.Fatal("WithGroup() returned nil logger")
	}

	// Test SetLevel and GetLevel
	SetLevel(LevelDebug)
	if GetLevel() != LevelDebug {
		t.Errorf("GetLevel() = %v, want %v", GetLevel(), LevelDebug)
	}
}

func TestLoggerFileOutput(t *testing.T) {
	// Create a temporary file for logging
	tmpFile := "/tmp/test-orchestrator-log.txt"
	defer os.Remove(tmpFile)

	cfg := config.LoggingConfig{
		Level:  "info",
		Format: "json",
		Output: tmpFile,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Log a message
	logger.Info("test message", "key", "value")

	// Close the logger
	if err := logger.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify the file was created
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Error("Log file was not created")
	}

	// Verify the file has content
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("Log file is empty")
	}

	// Verify JSON format
	var logEntry map[string]interface{}
	if err := json.Unmarshal(data, &logEntry); err != nil {
		t.Errorf("Log entry is not valid JSON: %v", err)
	}

	// Verify the message is present
	if msg, ok := logEntry["msg"].(string); !ok || msg != "test message" {
		t.Errorf("Log message = %v, want 'test message'", logEntry["msg"])
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		want    Level
		wantErr bool
	}{
		{"debug", "debug", LevelDebug, false},
		{"info", "info", LevelInfo, false},
		{"warn", "warn", LevelWarn, false},
		{"warning", "warning", LevelWarn, false},
		{"error", "error", LevelError, false},
		{"invalid", "invalid", LevelInfo, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLevel(tt.level)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseLevel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.level.String(); got != tt.want {
				t.Errorf("Level.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoggerTextFormat(t *testing.T) {
	// Create a temporary file for logging
	tmpFile := "/tmp/test-orchestrator-log-text.txt"
	defer os.Remove(tmpFile)

	cfg := config.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: tmpFile,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Log a message
	logger.Info("test message", "key", "value")

	// Close the logger
	if err := logger.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify the file was created
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	// Verify text format (should contain key=value pairs)
	content := string(data)
	if !strings.Contains(content, "test message") {
		t.Error("Log entry does not contain the message")
	}
	if !strings.Contains(content, "key=value") {
		t.Error("Log entry does not contain key=value pair")
	}
}

func TestInitGlobal(t *testing.T) {
	// Reset global logger
	globalLogger = nil
	globalOnce = sync.Once{}

	cfg := config.LoggingConfig{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
	}

	err := InitGlobal(cfg)
	if err != nil {
		t.Fatalf("InitGlobal() error = %v", err)
	}

	if globalLogger == nil {
		t.Fatal("InitGlobal() did not set globalLogger")
	}

	if globalLogger.GetLevel() != LevelDebug {
		t.Errorf("InitGlobal() level = %v, want %v", globalLogger.GetLevel(), LevelDebug)
	}

	// Test that calling InitGlobal again doesn't reset
	cfg.Level = "error"
	err = InitGlobal(cfg)
	if err != nil {
		t.Errorf("InitGlobal() second call error = %v", err)
	}

	// Level should still be debug from first call
	if globalLogger.GetLevel() != LevelDebug {
		t.Errorf("InitGlobal() second call changed level, got %v, want %v", globalLogger.GetLevel(), LevelDebug)
	}
}

// TestDerivedLoggerCloser tests that derived loggers don't have closer
func TestDerivedLoggerCloser(t *testing.T) {
	// Create a temporary file for logging
	tmpFile := "/tmp/test-derived-logger.txt"
	defer os.Remove(tmpFile)

	cfg := config.LoggingConfig{
		Level:  "info",
		Format: "json",
		Output: tmpFile,
	}

	rootLogger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Create derived loggers
	derivedLogger := rootLogger.With("component", "test")
	derivedLogger2 := rootLogger.WithGroup("testgroup")

	// Verify derived loggers have nil closer
	if derivedLogger.closer != nil {
		t.Error("Derived logger from With() should have nil closer")
	}
	if derivedLogger2.closer != nil {
		t.Error("Derived logger from WithGroup() should have nil closer")
	}

	// Closing derived loggers should be a no-op
	if err := derivedLogger.Close(); err != nil {
		t.Errorf("Closing derived logger should not error: %v", err)
	}
	if err := derivedLogger2.Close(); err != nil {
		t.Errorf("Closing derived logger2 should not error: %v", err)
	}

	// Log with derived loggers to ensure they still work
	derivedLogger.Info("test from derived")
	derivedLogger2.Info("test from derived2")

	// Close the root logger
	if err := rootLogger.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify the file was created and has content
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("Log file is empty")
	}

	// Verify both messages are present
	content := string(data)
	if !strings.Contains(content, "test from derived") {
		t.Error("Log from derived logger not found")
	}
	if !strings.Contains(content, "test from derived2") {
		t.Error("Log from derived logger2 not found")
	}
}
