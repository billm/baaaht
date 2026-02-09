//go:build windows

package policy

// setupSignalHandler is a no-op on Windows as SIGHUP is not available
func (r *Reloader) setupSignalHandler() {
	// SIGHUP is not available on Windows
	// Signal-based reload is not supported on this platform
	r.logger.Warn("Signal-based policy reload is not supported on Windows")
}
