package container

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

func TestDetectRuntime(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		platform string
	}{
		{
			name:     "Detect runtime on current platform",
			platform: runtime.GOOS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected := DetectRuntime(ctx)

			// Verify we get a valid runtime type
			if detected != types.RuntimeTypeDocker &&
				detected != types.RuntimeTypeAppleContainers &&
				detected != types.RuntimeTypeAuto {
				t.Errorf("DetectRuntime() returned invalid type: %s", detected)
			}

			// On Linux, we should get Docker or Auto
			if tt.platform == "linux" {
				if detected != types.RuntimeTypeDocker && detected != types.RuntimeTypeAuto {
					t.Errorf("On Linux, expected Docker or Auto, got %s", detected)
				}
			}

			// On macOS, we should get Apple, Docker, or Auto
			if tt.platform == "darwin" {
				if detected != types.RuntimeTypeAppleContainers &&
					detected != types.RuntimeTypeDocker &&
					detected != types.RuntimeTypeAuto {
					t.Errorf("On macOS, expected Apple, Docker, or Auto, got %s", detected)
				}
			}
		})
	}
}

func TestIsDockerAvailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	available := IsDockerAvailable(ctx)

	// If Docker is available, the result should be true
	// If not, the result should be false
	// We can't make hard assertions here since Docker may or may not be installed
	t.Logf("Docker available: %v", available)
}

func TestIsAppleContainersAvailable(t *testing.T) {
	available := IsAppleContainersAvailable()

	// Apple Containers is not yet available, so this should be false
	if available {
		t.Error("Expected Apple Containers to be unavailable (not yet implemented)")
	}

	// On non-macOS platforms, this should definitely be false
	if runtime.GOOS != "darwin" && available {
		t.Error("Apple Containers should only be available on macOS")
	}
}

func TestDetectRuntimeWithConfig(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		runtimeType string
		want        types.RuntimeType
	}{
		{
			name:        "Explicit Docker runtime",
			runtimeType: string(types.RuntimeTypeDocker),
			want:        types.RuntimeTypeDocker,
		},
		{
			name:        "Explicit Apple runtime",
			runtimeType: string(types.RuntimeTypeAppleContainers),
			want:        types.RuntimeTypeAppleContainers,
		},
		{
			name:        "Auto detection",
			runtimeType: string(types.RuntimeTypeAuto),
			want:        DetectRuntime(ctx),
		},
		{
			name:        "Empty string defaults to auto",
			runtimeType: "",
			want:        DetectRuntime(ctx),
		},
		{
			name:        "Invalid type falls back to auto",
			runtimeType: "invalid",
			want:        DetectRuntime(ctx),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectRuntimeWithConfig(ctx, tt.runtimeType)
			if got != tt.want {
				t.Errorf("DetectRuntimeWithConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetPlatform(t *testing.T) {
	platform := GetPlatform()

	if platform != runtime.GOOS {
		t.Errorf("GetPlatform() = %s, want %s", platform, runtime.GOOS)
	}
}

func TestGetArchitecture(t *testing.T) {
	arch := GetArchitecture()

	if arch != runtime.GOARCH {
		t.Errorf("GetArchitecture() = %s, want %s", arch, runtime.GOARCH)
	}
}

func TestDetectRuntimeConcurrency(t *testing.T) {
	// Test that DetectRuntime is safe to call concurrently
	ctx := context.Background()
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			DetectRuntime(ctx)
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		select {
		case <-done:
			// OK
		case <-time.After(5 * time.Second):
			t.Fatal("DetectRuntime did not complete concurrently")
		}
	}
}
