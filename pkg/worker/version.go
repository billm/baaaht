package worker

const (
	// DefaultVersion is the default version of the worker
	DefaultVersion = "0.1.0"
)

// GetVersion returns the version of the worker
func GetVersion() string {
	return DefaultVersion
}

// GetVersionInfo returns detailed version information
func GetVersionInfo() map[string]interface{} {
	return map[string]interface{}{
		"version": DefaultVersion,
	}
}
