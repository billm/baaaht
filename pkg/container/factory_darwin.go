//go:build darwin

package container

import (
	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// newAppleRuntimeFromConfig creates an Apple Containers runtime from RuntimeConfig
func newAppleRuntimeFromConfig(cfg RuntimeConfig) (Runtime, error) {
	// Create logger
	var log *logger.Logger
	if cfg.Logger != nil {
		if l, ok := cfg.Logger.(*logger.Logger); ok {
			log = l
		} else {
			// Logger is not the expected type, create a new one
			var err error
			log, err = logger.NewDefault()
			if err != nil {
				return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
			}
		}
	} else {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Convert RuntimeConfig to RuntimeConfig for Apple Containers
	appleCfg := config.RuntimeConfig{
		Type: string(types.RuntimeTypeAppleContainers),
	}

	// Set timeout
	if cfg.Timeout > 0 {
		appleCfg.Timeout = cfg.Timeout
	} else {
		appleCfg.Timeout = config.DefaultRuntimeConfig().Timeout
	}

	// Set socket path from endpoint if provided
	if cfg.Endpoint != "" {
		appleCfg.SocketPath = cfg.Endpoint
	}

	// Set TLS settings from options if provided
	if cfg.Options != nil {
		if tlsCert, ok := cfg.Options["tls_cert"].(string); ok {
			appleCfg.TLSCertPath = tlsCert
		}
		if tlsKey, ok := cfg.Options["tls_key"].(string); ok {
			appleCfg.TLSKeyPath = tlsKey
		}
		if tlsCA, ok := cfg.Options["tls_ca"].(string); ok {
			appleCfg.TLSCAPath = tlsCA
		}
		if tlsEnabled, ok := cfg.Options["tls_enabled"].(bool); ok {
			appleCfg.TLSEnabled = tlsEnabled
		}
	}

	// Create the Apple Containers runtime
	appleRuntime, err := NewAppleRuntime(appleCfg, log)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInternal, "failed to create Apple Containers runtime", err)
	}

	return appleRuntime, nil
}
