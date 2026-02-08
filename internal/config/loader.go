package config

import (
	"os"

	"github.com/billm/baaaht/orchestrator/pkg/types"
	"gopkg.in/yaml.v3"
)

// LoadFromFile loads configuration from a YAML file
func LoadFromFile(path string) (*Config, error) {
	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, types.WrapError(types.ErrCodeNotFound, "configuration file not found", err)
		}
		return nil, types.WrapError(types.ErrCodeInvalidArgument, "failed to read configuration file", err)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, types.WrapError(types.ErrCodeInvalid, "failed to parse YAML configuration", err)
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, types.WrapError(types.ErrCodeInvalid, "configuration validation failed", err)
	}

	return &cfg, nil
}
