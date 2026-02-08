//go:build !darwin

package container

import (
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// newAppleRuntimeFromConfig returns an error on non-darwin platforms
// Apple Containers is only available on macOS
func newAppleRuntimeFromConfig(cfg RuntimeConfig) (Runtime, error) {
	return nil, types.NewError(types.ErrCodeUnavailable,
		"Apple Containers runtime is only available on macOS (darwin)")
}
