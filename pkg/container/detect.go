package container

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
	"github.com/docker/docker/client"
)

// DetectRuntime detects the best available container runtime for the current platform
// It returns the recommended RuntimeType based on platform and runtime availability
func DetectRuntime(ctx context.Context) types.RuntimeType {
	// Detect the platform
	platform := runtime.GOOS

	// On Linux, Docker is the primary runtime
	if platform == "linux" {
		if IsDockerAvailable(ctx) {
			return types.RuntimeTypeDocker
		}
		return types.RuntimeTypeAuto // No runtime available
	}

	// On macOS (darwin), prefer Apple Containers if available, fall back to Docker
	if platform == "darwin" {
		if IsAppleContainersAvailable() {
			return types.RuntimeTypeAppleContainers
		}
		if IsDockerAvailable(ctx) {
			return types.RuntimeTypeDocker
		}
		return types.RuntimeTypeAuto // No runtime available
	}

	// On other platforms, try Docker as a fallback
	if IsDockerAvailable(ctx) {
		return types.RuntimeTypeDocker
	}

	return types.RuntimeTypeAuto // No runtime available
}

// IsDockerAvailable checks if the Docker daemon is running and accessible
func IsDockerAvailable(ctx context.Context) bool {
	// Check if DOCKER_HOST is set or if the default socket exists
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		// Check for default Unix socket
		if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
			return false
		}
	}

	// Verify the daemon is actually responding
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false
	}
	defer cli.Close()

	// Use a short timeout for the ping
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, err = cli.Ping(timeoutCtx)
	return err == nil
}

// IsAppleContainersAvailable checks if Apple Containers runtime is available
// On macOS, this would check if the Apple Containers framework is available
// Since Apple Containers API is not yet widely available, this always returns false
// When the Apple Containers SDK becomes available, this can be updated
func IsAppleContainersAvailable() bool {
	// Apple Containers is only available on macOS (darwin)
	if runtime.GOOS != "darwin" {
		return false
	}

	// TODO: Implement actual Apple Containers availability check
	// This would typically:
	// 1. Check if the Apple Containers framework is installed
	// 2. Verify the framework is accessible
	// 3. Check if we have the required permissions
	//
	// Example stub implementation (when SDK is available):
	//
	// // Check if the Apple Containers socket exists
	// if _, err := os.Stat("/var/run/applecontainers.sock"); os.IsNotExist(err) {
	//     return false
	// }
	//
	// // Try to create a test connection
	// cli, err := applecontainers.NewClient()
	// if err != nil {
	//     return false
	// }
	// defer cli.Close()
	//
	// ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	// defer cancel()
	// if err := cli.Ping(ctx); err != nil {
	//     return false
	// }
	//
	// return true

	// For now, Apple Containers is not yet available
	return false
}

// DetectRuntimeWithConfig detects the best available container runtime
// considering both platform and provided configuration
// If the config specifies a non-auto type, it returns that type
func DetectRuntimeWithConfig(ctx context.Context, runtimeType string) types.RuntimeType {
	// If a specific runtime is requested, return it
	// The caller is responsible for validating availability
	switch runtimeType {
	case string(types.RuntimeTypeDocker):
		return types.RuntimeTypeDocker
	case string(types.RuntimeTypeAppleContainers):
		return types.RuntimeTypeAppleContainers
	case string(types.RuntimeTypeAuto), "":
		return DetectRuntime(ctx)
	default:
		// Invalid runtime type, fall back to auto-detection
		return DetectRuntime(ctx)
	}
}

// GetPlatform returns the current operating system platform
func GetPlatform() string {
	return runtime.GOOS
}

// GetArchitecture returns the system architecture
func GetArchitecture() string {
	return runtime.GOARCH
}
