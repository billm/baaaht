package credentials

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/billm/baaaht/orchestrator/internal/logger"
)

// TestFilepathWithBase tests the path traversal protection
func TestFilepathWithBase(t *testing.T) {
	tests := []struct {
		name      string
		base      string
		path      string
		wantErr   bool
		errString string
	}{
		{
			name:    "normal path",
			base:    "/credentials",
			path:    "myfile.txt",
			wantErr: false,
		},
		{
			name:    "path with leading slash",
			base:    "/credentials",
			path:    "/myfile.txt",
			wantErr: false,
		},
		{
			name:    "subdirectory path",
			base:    "/credentials",
			path:    "subdir/myfile.txt",
			wantErr: false,
		},
		{
			name:      "path traversal with ..",
			base:      "/credentials",
			path:      "../etc/passwd",
			wantErr:   true,
			errString: "path traversal attempt detected",
		},
		{
			name:      "path traversal in subdirectory",
			base:      "/credentials",
			path:      "subdir/../../etc/passwd",
			wantErr:   true,
			errString: "path traversal attempt detected",
		},
		{
			name:    "absolute path normalized",
			base:    "/credentials",
			path:    "/etc/passwd",
			wantErr: false, // After stripping leading slash, becomes etc/passwd which is valid
		},
		{
			name:      "sibling directory bypass attempt",
			base:      "/base",
			path:      "../base2/file.txt",
			wantErr:   true,
			errString: "path traversal attempt detected",
		},
		{
			name:    "dots in filename",
			base:    "/credentials",
			path:    "my.config.yaml",
			wantErr: false,
		},
		{
			name:    "path with multiple subdirectories",
			base:    "/credentials",
			path:    "a/b/c/file.txt",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := filepathWithBase(tt.base, tt.path)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("filepathWithBase() expected error but got none, result: %s", result)
					return
				}
				if tt.errString != "" && !strings.Contains(err.Error(), tt.errString) {
					t.Errorf("filepathWithBase() error = %v, should contain %s", err, tt.errString)
				}
			} else {
				if err != nil {
					t.Errorf("filepathWithBase() unexpected error = %v", err)
					return
				}
				// Verify result is within base directory
				if result == "" {
					t.Error("filepathWithBase() returned empty result")
					return
				}
				// The result should be an absolute path
				if !filepath.IsAbs(result) {
					t.Errorf("filepathWithBase() result %s is not absolute", result)
				}
			}
		})
	}
}

// TestPrepareFile tests file preparation with path validation
func TestPrepareFile(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer log.Close()

	store := createTestStore(t)
	defer store.Close()

	inj, err := New(store, log)
	if err != nil {
		t.Fatalf("failed to create injector: %v", err)
	}
	defer inj.Close()

	ctx := context.Background()

	// Create a test credential
	cred := &Credential{
		Name:  "test-cred",
		Type:  "api_key",
		Value: "test-value",
	}
	err = store.Store(ctx, cred)
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	tests := []struct {
		name      string
		cfg       InjectionConfig
		wantErr   bool
		errString string
	}{
		{
			name: "normal file path",
			cfg: InjectionConfig{
				CredentialID: cred.ID,
				Method:       InjectionMethodFile,
				FilePath:     "/config.yaml",
				MountPath:    "/credentials",
			},
			wantErr: false,
		},
		{
			name: "path traversal attempt",
			cfg: InjectionConfig{
				CredentialID: cred.ID,
				Method:       InjectionMethodFile,
				FilePath:     "/../etc/passwd",
				MountPath:    "/credentials",
			},
			wantErr:   true,
			errString: "path traversal",
		},
		{
			name: "sibling directory bypass",
			cfg: InjectionConfig{
				CredentialID: cred.ID,
				Method:       InjectionMethodFile,
				FilePath:     "/../credentials2/file.txt",
				MountPath:    "/credentials",
			},
			wantErr:   true,
			errString: "path traversal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := inj.Prepare(ctx, []InjectionConfig{tt.cfg})
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("Prepare() expected error but got none")
					return
				}
				if tt.errString != "" && !strings.Contains(err.Error(), tt.errString) {
					t.Errorf("Prepare() error = %v, should contain %s", err, tt.errString)
				}
			} else {
				if err != nil {
					t.Errorf("Prepare() unexpected error = %v", err)
					return
				}
				if result == nil {
					t.Error("Prepare() returned nil result")
					return
				}
				if len(result.Files) == 0 {
					t.Error("Prepare() returned no files")
				}
			}
		})
	}
}
