//go:build unix

package policy

import (
	"os/signal"
	"syscall"
)

// setupSignalHandler sets up the signal handler for Unix systems (SIGHUP support)
func (r *Reloader) setupSignalHandler() {
	signal.Notify(r.signalChan, syscall.SIGHUP)
}
