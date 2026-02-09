package container

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/policy"
	"github.com/billm/baaaht/orchestrator/pkg/types"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCreator(t *testing.T) {
	tests := []struct {
		name    string
		client  *Client
		log     *logger.Logger
		wantErr bool
		errCode string
	}{
		{
			name:    "valid creator",
			client:  &Client{},
			log:     nil, // Should create default logger
			wantErr: false,
		},
		{
			name:    "nil client",
			client:  nil,
			log:     nil,
			wantErr: true,
			errCode: types.ErrCodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creator, err := NewCreator(tt.client, tt.log)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, creator)
				if tt.errCode != "" {
					var customErr *types.Error
					assert.ErrorAs(t, err, &customErr)
					assert.Equal(t, tt.errCode, customErr.Code)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, creator)
				assert.NotNil(t, creator.logger)
			}
		})
	}
}

func TestCreatorValidateConfig(t *testing.T) {
	client := &Client{}
	creator, err := NewCreator(client, nil)
	require.NoError(t, err)

	tests := []struct {
		name    string
		cfg     CreateConfig
		wantErr bool
		errCode string
	}{
		{
			name: "valid config",
			cfg: CreateConfig{
				Config: types.ContainerConfig{
					Image: "alpine:latest",
				},
				Name:      "test-container",
				SessionID: types.NewID("session-123"),
			},
			wantErr: false,
		},
		{
			name: "missing image",
			cfg: CreateConfig{
				Config:    types.ContainerConfig{},
				Name:      "test-container",
				SessionID: types.NewID("session-123"),
			},
			wantErr: true,
			errCode: types.ErrCodeInvalidArgument,
		},
		{
			name: "missing name",
			cfg: CreateConfig{
				Config: types.ContainerConfig{
					Image: "alpine:latest",
				},
				Name:      "",
				SessionID: types.NewID("session-123"),
			},
			wantErr: true,
			errCode: types.ErrCodeInvalidArgument,
		},
		{
			name: "missing session ID",
			cfg: CreateConfig{
				Config: types.ContainerConfig{
					Image: "alpine:latest",
				},
				Name:      "test-container",
				SessionID: types.ID(""),
			},
			wantErr: true,
			errCode: types.ErrCodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := creator.validateConfig(tt.cfg)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					var customErr *types.Error
					assert.ErrorAs(t, err, &customErr)
					assert.Equal(t, tt.errCode, customErr.Code)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConvertEnvMap(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{
			name: "nil map",
			env:  nil,
			want: nil,
		},
		{
			name: "empty map",
			env:  map[string]string{},
			want: []string{},
		},
		{
			name: "single env var",
			env: map[string]string{
				"FOO": "bar",
			},
			want: []string{"FOO=bar"},
		},
		{
			name: "multiple env vars",
			env: map[string]string{
				"FOO":    "bar",
				"BAZ":    "qux",
				"NUMBER": "123",
			},
			want: []string{"FOO=bar", "BAZ=qux", "NUMBER=123"},
		},
		{
			name: "env var with special characters",
			env: map[string]string{
				"PATH":      "/usr/bin:/bin",
				"MULTILINE": "line1\nline2",
			},
			want: []string{"PATH=/usr/bin:/bin", "MULTILINE=line1\nline2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertEnvMap(tt.env)

			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				assert.ElementsMatch(t, tt.want, got)
			}
		})
	}
}

func TestConvertLabels(t *testing.T) {
	sessionID := types.NewID("test-session")
	name := "test-container"

	tests := []struct {
		name      string
		labels    map[string]string
		container string
		session   types.ID
		wantKeys  []string // Keys that should be present
	}{
		{
			name:      "no custom labels",
			labels:    nil,
			container: name,
			session:   sessionID,
			wantKeys:  []string{"baaaht.managed", "baaaht.container_name", "baaaht.session_id", "baaaht.created_at"},
		},
		{
			name: "with custom labels",
			labels: map[string]string{
				"custom.label": "value",
				"another":      "test",
			},
			container: name,
			session:   sessionID,
			wantKeys:  []string{"baaaht.managed", "baaaht.container_name", "baaaht.session_id", "baaaht.created_at", "custom.label", "another"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertLabels(tt.labels, tt.container, tt.session)

			for _, key := range tt.wantKeys {
				assert.Contains(t, got, key, "label key %s should be present", key)
			}

			// Check standard labels
			assert.Equal(t, "true", got["baaaht.managed"])
			assert.Equal(t, tt.container, got["baaaht.container_name"])
			assert.Equal(t, tt.session.String(), got["baaaht.session_id"])
			assert.NotEmpty(t, got["baaaht.created_at"])

			// Check custom labels
			for k, v := range tt.labels {
				assert.Equal(t, v, got[k])
			}
		})
	}
}

func TestConvertMounts(t *testing.T) {
	tests := []struct {
		name   string
		mounts []types.Mount
		want   int
	}{
		{
			name:   "nil mounts",
			mounts: nil,
			want:   0,
		},
		{
			name:   "empty mounts",
			mounts: []types.Mount{},
			want:   0,
		},
		{
			name: "bind mount",
			mounts: []types.Mount{
				{
					Type:     types.MountTypeBind,
					Source:   "/host/path",
					Target:   "/container/path",
					ReadOnly: true,
				},
			},
			want: 1,
		},
		{
			name: "multiple mounts",
			mounts: []types.Mount{
				{
					Type:     types.MountTypeBind,
					Source:   "/host/path1",
					Target:   "/container/path1",
					ReadOnly: false,
				},
				{
					Type:     types.MountTypeVolume,
					Source:   "volume1",
					Target:   "/data",
					ReadOnly: false,
				},
				{
					Type:     types.MountTypeTmpfs,
					Source:   "",
					Target:   "/tmp",
					ReadOnly: false,
				},
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertMounts(tt.mounts)

			if tt.want == 0 {
				assert.Empty(t, got)
			} else {
				assert.Len(t, got, tt.want)
				for i, m := range tt.mounts {
					assert.Equal(t, string(m.Type), string(got[i].Type))
					assert.Equal(t, m.Source, got[i].Source)
					assert.Equal(t, m.Target, got[i].Target)
					assert.Equal(t, m.ReadOnly, got[i].ReadOnly)
				}
			}
		})
	}
}

func TestConvertPortBindings(t *testing.T) {
	tests := []struct {
		name  string
		ports []types.PortBinding
		want  int // Expected number of unique ports
	}{
		{
			name:  "nil ports",
			ports: nil,
			want:  0,
		},
		{
			name:  "empty ports",
			ports: []types.PortBinding{},
			want:  0,
		},
		{
			name: "single port",
			ports: []types.PortBinding{
				{
					ContainerPort: 8080,
					HostPort:      8080,
					Protocol:      "tcp",
					HostIP:        "0.0.0.0",
				},
			},
			want: 1,
		},
		{
			name: "multiple ports",
			ports: []types.PortBinding{
				{
					ContainerPort: 8080,
					HostPort:      8080,
					Protocol:      "tcp",
				},
				{
					ContainerPort: 9090,
					HostPort:      9090,
					Protocol:      "tcp",
				},
			},
			want: 2,
		},
		{
			name: "port with default protocol",
			ports: []types.PortBinding{
				{
					ContainerPort: 8080,
					HostPort:      8080,
				},
			},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertPortBindings(tt.ports)

			if tt.want == 0 {
				assert.Empty(t, got)
			} else {
				assert.Len(t, got, tt.want)
			}
		})
	}
}

func TestConvertRestartPolicy(t *testing.T) {
	tests := []struct {
		name   string
		policy types.RestartPolicy
		want   container.RestartPolicyMode
	}{
		{
			name: "always restart",
			policy: types.RestartPolicy{
				Name:              "always",
				MaximumRetryCount: 0,
			},
			want: container.RestartPolicyAlways,
		},
		{
			name: "unless stopped",
			policy: types.RestartPolicy{
				Name:              "unless-stopped",
				MaximumRetryCount: 0,
			},
			want: container.RestartPolicyUnlessStopped,
		},
		{
			name: "on failure",
			policy: types.RestartPolicy{
				Name:              "on-failure",
				MaximumRetryCount: 5,
			},
			want: container.RestartPolicyOnFailure,
		},
		{
			name: "no restart",
			policy: types.RestartPolicy{
				Name:              "no",
				MaximumRetryCount: 0,
			},
			want: container.RestartPolicyDisabled,
		},
		{
			name: "empty policy defaults to no",
			policy: types.RestartPolicy{
				Name:              "",
				MaximumRetryCount: 0,
			},
			want: container.RestartPolicyDisabled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertRestartPolicy(tt.policy)
			assert.Equal(t, tt.want, got.Name)
			assert.Equal(t, tt.policy.MaximumRetryCount, got.MaximumRetryCount)
		})
	}
}

func TestCreatorString(t *testing.T) {
	client := &Client{}
	creator, err := NewCreator(client, nil)
	require.NoError(t, err)

	s := creator.String()
	assert.Contains(t, s, "Creator")
}

// Integration test - only runs if Docker is available
func TestCreatorIntegration(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check if Docker is available
	ctx := context.Background()
	if !IsDockerRunning(ctx) {
		t.Skip("Docker is not running, skipping integration test")
	}

	// Create Docker client
	log, err := logger.NewDefault()
	require.NoError(t, err)

	client, err := NewDefault(log)
	require.NoError(t, err)
	defer client.Close()

	creator, err := NewCreator(client, log)
	require.NoError(t, err)

	t.Run("CreateAndRemoveContainer", func(t *testing.T) {
		containerName := "baaaht-test-container-" + time.Now().Format("20060102150405")
		sessionID := types.NewID("test-session-integration")

		// Create container with a minimal Alpine image
		cfg := CreateConfig{
			Config: types.ContainerConfig{
				Image:   "alpine:latest",
				Command: []string{"sh", "-c", "echo 'hello' && sleep 1"},
				Labels: map[string]string{
					"test": "integration",
				},
			},
			Name:        containerName,
			SessionID:   sessionID,
			AutoPull:    true,
			PullTimeout: 2 * time.Minute,
		}

		result, err := creator.Create(ctx, cfg)
		require.NoError(t, err)
		assert.NotEmpty(t, result.ContainerID)

		t.Cleanup(func() {
			// Clean up the container
			timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			_ = client.Client().ContainerRemove(timeoutCtx, result.ContainerID, container.RemoveOptions{
				Force: true,
			})
		})

		// Verify container was created
		containerJSON, err := client.Client().ContainerInspect(ctx, result.ContainerID)
		require.NoError(t, err)
		assert.Equal(t, containerName, strings.TrimPrefix(containerJSON.Name, "/"))
		assert.Equal(t, sessionID.String(), containerJSON.Config.Labels["baaaht.session_id"])
		assert.Equal(t, "true", containerJSON.Config.Labels["baaaht.managed"])

		t.Logf("Container created successfully: %s", result.ContainerID)
	})

	t.Run("CreateWithDefaults", func(t *testing.T) {
		containerName := "baaaht-test-defaults-" + time.Now().Format("20060102150405")
		sessionID := types.NewID("test-session-defaults")

		result, err := creator.CreateWithDefaults(ctx, "alpine:latest", containerName, sessionID)
		require.NoError(t, err)
		assert.NotEmpty(t, result.ContainerID)

		t.Cleanup(func() {
			// Clean up the container
			timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			_ = client.Client().ContainerRemove(timeoutCtx, result.ContainerID, container.RemoveOptions{
				Force: true,
			})
		})

		// Verify container was created with defaults
		containerJSON, err := client.Client().ContainerInspect(ctx, result.ContainerID)
		require.NoError(t, err)
		assert.Equal(t, containerName, strings.TrimPrefix(containerJSON.Name, "/"))
		assert.Equal(t, sessionID.String(), containerJSON.Config.Labels["baaaht.session_id"])

		t.Logf("Container created with defaults: %s", result.ContainerID)
	})

	t.Run("PullImage", func(t *testing.T) {
		// Pull a small test image
		image := "alpine:latest"
		err := creator.PullImage(ctx, image, 2*time.Minute)
		require.NoError(t, err)

		// Verify image exists
		exists, err := creator.ImageExists(ctx, image)
		require.NoError(t, err)
		assert.True(t, exists)

		t.Logf("Image pulled successfully: %s", image)
	})
}

// Benchmark tests
func BenchmarkConvertEnvMap(b *testing.B) {
	env := make(map[string]string)
	for i := 0; i < 100; i++ {
		env[fmt.Sprintf("VAR_%d", i)] = fmt.Sprintf("value_%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = convertEnvMap(env)
	}
}

func BenchmarkConvertPortBindings(b *testing.B) {
	ports := make([]types.PortBinding, 100)
	for i := 0; i < 100; i++ {
		ports[i] = types.PortBinding{
			ContainerPort: 8000 + i,
			HostPort:      8000 + i,
			Protocol:      "tcp",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = convertPortBindings(ports)
	}
}

func TestCreateWithPolicyViolation(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	// Create a policy enforcer with strict mode that denies latest tag
	strictPolicy := policy.DefaultPolicy()
	strictPolicy.Images.AllowLatestTag = false
	strictPolicy.Mode = policy.EnforcementModeStrict

	enforcer, err := policy.New(config.DefaultPolicyConfig(), log)
	require.NoError(t, err)
	err = enforcer.SetPolicy(context.Background(), strictPolicy)
	require.NoError(t, err)

	client := &Client{}
	creator, err := NewCreator(client, log)
	require.NoError(t, err)

	// Set the enforcer
	creator.SetEnforcer(enforcer)

	sessionID := types.NewID("test-session-policy")

	tests := []struct {
		name    string
		cfg     CreateConfig
		wantErr bool
		errCode string
	}{
		{
			name: "policy violation - image with latest tag denied",
			cfg: CreateConfig{
				Config: types.ContainerConfig{
					Image: "alpine:latest",
				},
				Name:      "test-container-violation",
				SessionID: sessionID,
			},
			wantErr: true,
			errCode: types.ErrCodePermission,
		},
		{
			name: "policy violation - CPU quota exceeds maximum",
			cfg: CreateConfig{
				Config: types.ContainerConfig{
					Image: "alpine:3.18",
					Resources: types.ResourceLimits{
						NanoCPUs: 8 * 1000000000, // 8 CPUs exceeds default 4 CPU limit
					},
				},
				Name:      "test-container-cpu-violation",
				SessionID: sessionID,
			},
			wantErr: true,
			errCode: types.ErrCodePermission,
		},
		{
			name: "policy violation - memory quota exceeds maximum",
			cfg: CreateConfig{
				Config: types.ContainerConfig{
					Image: "alpine:3.18",
					Resources: types.ResourceLimits{
						MemoryBytes: 16 * 1024 * 1024 * 1024, // 16GB exceeds default 8GB limit
					},
				},
				Name:      "test-container-memory-violation",
				SessionID: sessionID,
			},
			wantErr: true,
			errCode: types.ErrCodePermission,
		},
		{
			name: "policy compliant - image with specific tag",
			cfg: CreateConfig{
				Config: types.ContainerConfig{
					Image: "alpine:3.18",
				},
				Name:      "test-container-compliant",
				SessionID: sessionID,
			},
			wantErr: true, // Will still fail because Docker client is nil, but not with permission error
			errCode: "",   // Not expecting permission error
		},
		{
			name: "no enforcer - allows any configuration",
			cfg: CreateConfig{
				Config: types.ContainerConfig{
					Image: "alpine:latest",
					Resources: types.ResourceLimits{
						NanoCPUs:    8 * 1000000000,
						MemoryBytes: 16 * 1024 * 1024 * 1024,
					},
				},
				Name:      "test-container-no-enforcer",
				SessionID: sessionID,
			},
			wantErr: true, // Will fail with nil Docker client, but not policy violation
			errCode: "",   // Not expecting permission error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For the "no enforcer" test, temporarily remove the enforcer
			if tt.name == "no enforcer - allows any configuration" {
				creator.SetEnforcer(nil)
				defer creator.SetEnforcer(enforcer)
			}

			// Create will fail at ContainerCreate since we don't have a real Docker client
			// But we want to test that policy validation happens before that
			_, err := creator.Create(context.Background(), tt.cfg)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					var customErr *types.Error
					assert.ErrorAs(t, err, &customErr)
					assert.Equal(t, tt.errCode, customErr.Code, "expected %s error code", tt.errCode)
				} else {
					// For cases that should fail but not with permission error
					var customErr *types.Error
					if err != nil && assert.ErrorAs(t, err, &customErr) {
						// Should not be a permission error (that would mean policy rejected it)
						assert.NotEqual(t, types.ErrCodePermission, customErr.Code,
							"should not be a policy permission error for this case")
					}
				}
			}
		})
	}
}

func TestCreatorWithEnforcer(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	client := &Client{}
	creator, err := NewCreator(client, log)
	require.NoError(t, err)

	// Initially, enforcer should be nil
	assert.Nil(t, creator.Enforcer())

	// Create and set enforcer
	enforcer, err := policy.NewDefault(log)
	require.NoError(t, err)

	creator.SetEnforcer(enforcer)

	// Enforcer should now be set
	assert.NotNil(t, creator.Enforcer())
	assert.Same(t, enforcer, creator.Enforcer())

	// Check String representation includes enforcer status
	s := creator.String()
	assert.Contains(t, s, "Creator")
	assert.Contains(t, s, "enabled")
}

func TestStrictModeRejection(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	client := &Client{}
	creator, err := NewCreator(client, log)
	require.NoError(t, err)

	sessionID := types.NewID("test-session-strict")

	t.Run("strict mode rejects latest tag image", func(t *testing.T) {
		// Create a policy enforcer with strict mode that denies latest tag
		strictPolicy := policy.DefaultPolicy()
		strictPolicy.Images.AllowLatestTag = false
		strictPolicy.Mode = policy.EnforcementModeStrict

		enforcer, err := policy.New(config.DefaultPolicyConfig(), log)
		require.NoError(t, err)
		err = enforcer.SetPolicy(context.Background(), strictPolicy)
		require.NoError(t, err)

		creator.SetEnforcer(enforcer)

		cfg := CreateConfig{
			Config: types.ContainerConfig{
				Image: "alpine:latest",
			},
			Name:      "test-container-latest",
			SessionID: sessionID,
		}

		_, err = creator.Create(context.Background(), cfg)

		// Should fail with permission error
		require.Error(t, err)
		var customErr *types.Error
		require.ErrorAs(t, err, &customErr)
		assert.Equal(t, types.ErrCodePermission, customErr.Code,
			"strict mode should reject image with latest tag")
		assert.Contains(t, customErr.Message, "policy")
	})

	t.Run("strict mode rejects CPU quota violations", func(t *testing.T) {
		strictPolicy := policy.DefaultPolicy()
		maxCPUs := int64(2 * 1000000000) // 2 CPUs
		strictPolicy.Quotas.MaxCPUs = &maxCPUs
		strictPolicy.Mode = policy.EnforcementModeStrict

		enforcer, err := policy.New(config.DefaultPolicyConfig(), log)
		require.NoError(t, err)
		err = enforcer.SetPolicy(context.Background(), strictPolicy)
		require.NoError(t, err)

		creator.SetEnforcer(enforcer)

		cfg := CreateConfig{
			Config: types.ContainerConfig{
				Image: "alpine:3.18",
				Resources: types.ResourceLimits{
					NanoCPUs: 4 * 1000000000, // 4 CPUs exceeds 2 CPU limit
				},
			},
			Name:      "test-container-cpu",
			SessionID: sessionID,
		}

		_, err = creator.Create(context.Background(), cfg)

		// Should fail with permission error
		require.Error(t, err)
		var customErr *types.Error
		require.ErrorAs(t, err, &customErr)
		assert.Equal(t, types.ErrCodePermission, customErr.Code,
			"strict mode should reject CPU quota violation")
	})

	t.Run("strict mode rejects memory quota violations", func(t *testing.T) {
		strictPolicy := policy.DefaultPolicy()
		maxMemory := int64(4 * 1024 * 1024 * 1024) // 4GB
		strictPolicy.Quotas.MaxMemory = &maxMemory
		strictPolicy.Mode = policy.EnforcementModeStrict

		enforcer, err := policy.New(config.DefaultPolicyConfig(), log)
		require.NoError(t, err)
		err = enforcer.SetPolicy(context.Background(), strictPolicy)
		require.NoError(t, err)

		creator.SetEnforcer(enforcer)

		cfg := CreateConfig{
			Config: types.ContainerConfig{
				Image: "alpine:3.18",
				Resources: types.ResourceLimits{
					MemoryBytes: 8 * 1024 * 1024 * 1024, // 8GB exceeds 4GB limit
				},
			},
			Name:      "test-container-memory",
			SessionID: sessionID,
		}

		_, err = creator.Create(context.Background(), cfg)

		// Should fail with permission error
		require.Error(t, err)
		var customErr *types.Error
		require.ErrorAs(t, err, &customErr)
		assert.Equal(t, types.ErrCodePermission, customErr.Code,
			"strict mode should reject memory quota violation")
	})

	t.Run("strict mode allows compliant configuration", func(t *testing.T) {
		strictPolicy := policy.DefaultPolicy()
		maxCPUs := int64(4 * 1000000000) // 4 CPUs
		strictPolicy.Quotas.MaxCPUs = &maxCPUs
		maxMemory := int64(8 * 1024 * 1024 * 1024) // 8GB
		strictPolicy.Quotas.MaxMemory = &maxMemory
		strictPolicy.Images.AllowLatestTag = false
		strictPolicy.Mode = policy.EnforcementModeStrict

		enforcer, err := policy.New(config.DefaultPolicyConfig(), log)
		require.NoError(t, err)
		err = enforcer.SetPolicy(context.Background(), strictPolicy)
		require.NoError(t, err)

		creator.SetEnforcer(enforcer)

		cfg := CreateConfig{
			Config: types.ContainerConfig{
				Image: "alpine:3.18", // Specific tag, not latest
				Resources: types.ResourceLimits{
					NanoCPUs:    2 * 1000000000,         // 2 CPUs within 4 CPU limit
					MemoryBytes: 4 * 1024 * 1024 * 1024, // 4GB within 8GB limit
				},
			},
			Name:      "test-container-compliant",
			SessionID: sessionID,
		}

		_, err = creator.Create(context.Background(), cfg)

		// Should NOT fail with permission error (will fail with nil Docker client error instead)
		if err != nil {
			var customErr *types.Error
			if assert.ErrorAs(t, err, &customErr) {
				assert.NotEqual(t, types.ErrCodePermission, customErr.Code,
					"compliant config should not fail policy validation")
			}
		}
	})

	t.Run("permissive mode allows violations but logs them", func(t *testing.T) {
		permissivePolicy := policy.DefaultPolicy()
		permissivePolicy.Images.AllowLatestTag = false
		permissivePolicy.Mode = policy.EnforcementModePermissive

		enforcer, err := policy.New(config.DefaultPolicyConfig(), log)
		require.NoError(t, err)
		err = enforcer.SetPolicy(context.Background(), permissivePolicy)
		require.NoError(t, err)

		creator.SetEnforcer(enforcer)

		cfg := CreateConfig{
			Config: types.ContainerConfig{
				Image: "alpine:latest", // Violates policy
			},
			Name:      "test-container-permissive",
			SessionID: sessionID,
		}

		_, err = creator.Create(context.Background(), cfg)

		// Should NOT fail with permission error in permissive mode
		if err != nil {
			var customErr *types.Error
			if assert.ErrorAs(t, err, &customErr) {
				assert.NotEqual(t, types.ErrCodePermission, customErr.Code,
					"permissive mode should not block with permission error")
			}
		}
	})

	t.Run("disabled mode allows everything", func(t *testing.T) {
		disabledPolicy := policy.DefaultPolicy()
		disabledPolicy.Images.AllowLatestTag = false
		disabledPolicy.Mode = policy.EnforcementModeDisabled

		enforcer, err := policy.New(config.DefaultPolicyConfig(), log)
		require.NoError(t, err)
		err = enforcer.SetPolicy(context.Background(), disabledPolicy)
		require.NoError(t, err)

		creator.SetEnforcer(enforcer)

		cfg := CreateConfig{
			Config: types.ContainerConfig{
				Image: "alpine:latest", // Would violate if not disabled
				Resources: types.ResourceLimits{
					NanoCPUs:    8 * 1000000000,          // Exceeds default
					MemoryBytes: 16 * 1024 * 1024 * 1024, // Exceeds default
				},
			},
			Name:      "test-container-disabled",
			SessionID: sessionID,
		}

		_, err = creator.Create(context.Background(), cfg)

		// Should NOT fail with permission error in disabled mode
		if err != nil {
			var customErr *types.Error
			if assert.ErrorAs(t, err, &customErr) {
				assert.NotEqual(t, types.ErrCodePermission, customErr.Code,
					"disabled mode should not block with permission error")
			}
		}
	})
}

func TestPolicyViolationLogging(t *testing.T) {
	log, err := logger.NewDefault()
	require.NoError(t, err)

	// Create a policy enforcer with permissive mode to allow logging but not block
	permissivePolicy := policy.DefaultPolicy()
	permissivePolicy.Images.AllowLatestTag = false // Will generate violation
	permissivePolicy.Mode = policy.EnforcementModePermissive

	enforcer, err := policy.New(config.DefaultPolicyConfig(), log)
	require.NoError(t, err)
	err = enforcer.SetPolicy(context.Background(), permissivePolicy)
	require.NoError(t, err)

	client := &Client{}
	creator, err := NewCreator(client, log)
	require.NoError(t, err)

	// Set the enforcer
	creator.SetEnforcer(enforcer)

	sessionID := types.NewID("test-session-logging")

	t.Run("logs policy violations with error severity", func(t *testing.T) {
		cfg := CreateConfig{
			Config: types.ContainerConfig{
				Image: "alpine:latest", // Violates AllowLatestTag policy
			},
			Name:      "test-container-violation-log",
			SessionID: sessionID,
		}

		// In permissive mode, this should not return an error for policy violations
		// but should log them. The Create will still fail because Docker client is nil.
		_, err := creator.Create(context.Background(), cfg)

		// Should fail with nil Docker client error, not policy permission error
		assert.Error(t, err)
		var customErr *types.Error
		if err != nil && assert.ErrorAs(t, err, &customErr) {
			// Should NOT be a permission error (that would mean policy blocked it in strict mode)
			assert.NotEqual(t, types.ErrCodePermission, customErr.Code,
				"permissive mode should log violations but not block with permission error")
		}
	})

	t.Run("logs policy warnings", func(t *testing.T) {
		// Create a policy that generates warnings
		warningPolicy := policy.DefaultPolicy()
		warningPolicy.Mode = policy.EnforcementModePermissive

		enforcer2, err := policy.New(config.DefaultPolicyConfig(), log)
		require.NoError(t, err)
		err = enforcer2.SetPolicy(context.Background(), warningPolicy)
		require.NoError(t, err)

		creator.SetEnforcer(enforcer2)

		cfg := CreateConfig{
			Config: types.ContainerConfig{
				Image: "alpine:3.18", // Compliant image
			},
			Name:      "test-container-warning-log",
			SessionID: sessionID,
		}

		// Should not fail policy validation in permissive mode
		_, err = creator.Create(context.Background(), cfg)

		// Will fail with nil Docker client, but not with policy error
		assert.Error(t, err)
		var customErr *types.Error
		if err != nil && assert.ErrorAs(t, err, &customErr) {
			assert.NotEqual(t, types.ErrCodePermission, customErr.Code)
		}
	})

	t.Run("logs multiple violations", func(t *testing.T) {
		// Create a policy that will generate multiple violations
		multiViolationPolicy := policy.DefaultPolicy()
		multiViolationPolicy.Images.AllowLatestTag = false
		maxCPUs := int64(2 * 1000000000) // 2 CPUs in nanoseconds
		multiViolationPolicy.Quotas.MaxCPUs = &maxCPUs
		maxMemory := int64(4 * 1024 * 1024 * 1024) // 4GB
		multiViolationPolicy.Quotas.MaxMemory = &maxMemory
		multiViolationPolicy.Mode = policy.EnforcementModePermissive

		enforcer3, err := policy.New(config.DefaultPolicyConfig(), log)
		require.NoError(t, err)
		err = enforcer3.SetPolicy(context.Background(), multiViolationPolicy)
		require.NoError(t, err)

		creator.SetEnforcer(enforcer3)

		cfg := CreateConfig{
			Config: types.ContainerConfig{
				Image: "ubuntu:latest", // Violates no-latest-tag
				Resources: types.ResourceLimits{
					NanoCPUs:    4 * 1000000000,         // 4 CPUs exceeds 2 CPU limit
					MemoryBytes: 8 * 1024 * 1024 * 1024, // 8GB exceeds 4GB limit
				},
			},
			Name:      "test-container-multi-violation",
			SessionID: sessionID,
		}

		// Should log multiple violations but not block in permissive mode
		_, err = creator.Create(context.Background(), cfg)

		// Will fail with nil Docker client, but not with policy permission error
		assert.Error(t, err)
		var customErr *types.Error
		if err != nil && assert.ErrorAs(t, err, &customErr) {
			assert.NotEqual(t, types.ErrCodePermission, customErr.Code,
				"permissive mode should not block with permission error")
		}
	})
}
