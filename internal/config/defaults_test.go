package config

import (
	"testing"
	"time"
)

// TestDefaultSessionConfig verifies that the default session configuration
// has persistence enabled by default
func TestDefaultSessionConfig(t *testing.T) {
	config := DefaultSessionConfig()

	// Verify persistence is enabled by default
	if !config.PersistenceEnabled {
		t.Errorf("Expected PersistenceEnabled to be true by default, got false")
	}

	// Verify other default values are correct
	if config.Timeout != DefaultSessionTimeout {
		t.Errorf("Expected Timeout to be %v, got %v", DefaultSessionTimeout, config.Timeout)
	}

	if config.MaxSessions != DefaultMaxSessions {
		t.Errorf("Expected MaxSessions to be %d, got %d", DefaultMaxSessions, config.MaxSessions)
	}

	if config.CleanupInterval != 5*time.Minute {
		t.Errorf("Expected CleanupInterval to be 5m, got %v", config.CleanupInterval)
	}

	if config.IdleTimeout != 10*time.Minute {
		t.Errorf("Expected IdleTimeout to be 10m, got %v", config.IdleTimeout)
	}

	// Verify storage path is set
	if config.StoragePath == "" {
		t.Errorf("Expected StoragePath to be set, got empty string")
	}
}
