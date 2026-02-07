package container

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if Docker is available
	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	t.Run("create client with default config", func(t *testing.T) {
		cfg := config.DefaultDockerConfig()
		log, err := logger.New(config.DefaultLoggingConfig())
		require.NoError(t, err)

		client, err := New(cfg, log)
		require.NoError(t, err)
		require.NotNil(t, client)
		assert.False(t, client.IsClosed())

		err = client.Close()
		assert.NoError(t, err)
		assert.True(t, client.IsClosed())
	})

	t.Run("create client with custom config", func(t *testing.T) {
		cfg := config.DockerConfig{
			Host:        os.Getenv("DOCKER_HOST"),
			APIVersion:  "1.44",
			Timeout:     10 * time.Second,
			MaxRetries:  2,
			RetryDelay:  500 * time.Millisecond,
			TLSVerify:   false,
		}
		if cfg.Host == "" {
			cfg.Host = config.DefaultDockerHost
		}

		log, err := logger.New(config.DefaultLoggingConfig())
		require.NoError(t, err)

		client, err := New(cfg, log)
		require.NoError(t, err)
		require.NotNil(t, client)

		err = client.Close()
		assert.NoError(t, err)
	})

	t.Run("create client with nil logger uses default", func(t *testing.T) {
		cfg := config.DefaultDockerConfig()

		client, err := New(cfg, nil)
		require.NoError(t, err)
		require.NotNil(t, client)

		err = client.Close()
		assert.NoError(t, err)
	})
}

func TestNewDefaultClient(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	require.NotNil(t, client)

	err = client.Close()
	assert.NoError(t, err)
}

func TestClientPing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	err = client.Ping(ctx)
	assert.NoError(t, err)
}

func TestClientPingWithTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = client.Ping(ctx)
	assert.NoError(t, err)
}

func TestClientVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	version, err := client.Version(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, version)
}

func TestClientInfo(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	info, err := client.Info(ctx)
	require.NoError(t, err)
	assert.NotNil(t, info)
	assert.NotEmpty(t, info.ServerVersion)
	assert.NotEmpty(t, info.APIVersion)
	assert.NotEmpty(t, info.OS)
	assert.Greater(t, info.NCPU, 0)
	assert.Greater(t, info.Memory, int64(0))
}

func TestClientClose(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	t.Run("close client successfully", func(t *testing.T) {
		client, err := NewDefault(nil)
		require.NoError(t, err)

		err = client.Close()
		assert.NoError(t, err)
		assert.True(t, client.IsClosed())
	})

	t.Run("close already closed client is idempotent", func(t *testing.T) {
		client, err := NewDefault(nil)
		require.NoError(t, err)

		err = client.Close()
		assert.NoError(t, err)

		err = client.Close()
		assert.NoError(t, err)
	})

	t.Run("operations fail on closed client", func(t *testing.T) {
		client, err := NewDefault(nil)
		require.NoError(t, err)

		err = client.Close()
		assert.NoError(t, err)

		ctx := context.Background()
		err = client.Ping(ctx)
		assert.Error(t, err)
	})
}

func TestClientConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	cfg := config.DefaultDockerConfig()
	client, err := New(cfg, nil)
	require.NoError(t, err)
	defer client.Close()

	retrievedCfg := client.Config()
	assert.Equal(t, cfg.Host, retrievedCfg.Host)
	assert.Equal(t, cfg.APIVersion, retrievedCfg.APIVersion)
	assert.Equal(t, cfg.Timeout, retrievedCfg.Timeout)
	assert.Equal(t, cfg.MaxRetries, retrievedCfg.MaxRetries)
}

func TestClientWithTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	cfg := config.DefaultDockerConfig()
	client, err := New(cfg, nil)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := client.WithTimeout(context.Background())
	defer cancel()

	_, ok := ctx.Deadline()
	assert.True(t, ok, "WithTimeout should create a context with a deadline")
}

func TestClientString(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	cfg := config.DefaultDockerConfig()
	client, err := New(cfg, nil)
	require.NoError(t, err)
	defer client.Close()

	str := client.String()
	assert.Contains(t, str, "DockerClient")
	assert.Contains(t, str, cfg.Host)
	assert.Contains(t, str, "Closed: false")
}

func TestClientRetries(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	cfg := config.DockerConfig{
		Host:       os.Getenv("DOCKER_HOST"),
		APIVersion: "1.44",
		Timeout:    5 * time.Second,
		MaxRetries: 2,
		RetryDelay: 100 * time.Millisecond,
		TLSVerify:  false,
	}
	if cfg.Host == "" {
		cfg.Host = config.DefaultDockerHost
	}

	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err)

	client, err := New(cfg, log)
	require.NoError(t, err)
	defer client.Close()

	// Execute with retry should succeed
	ctx := context.Background()
	version, err := client.Version(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, version)
}

func TestGlobalClient(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	// Reset global client
	globalClient = nil
	globalOnce = sync.Once{}

	t.Run("global client initialization", func(t *testing.T) {
		client := Global()
		assert.NotNil(t, client)
		if client.IsClosed() {
			t.Skip("Global client failed to initialize")
		}

		// Subsequent calls should return the same instance
		client2 := Global()
		assert.Same(t, client, client2)
	})

	t.Run("set global client", func(t *testing.T) {
		cfg := config.DefaultDockerConfig()
		log, err := logger.New(config.DefaultLoggingConfig())
		require.NoError(t, err)

		newClient, err := New(cfg, log)
		require.NoError(t, err)
		defer newClient.Close()

		SetGlobal(newClient)

		client := Global()
		assert.Same(t, newClient, client)
	})

	// Reset global state
	globalClient = nil
	globalOnce = sync.Once{}
}

func TestInitGlobalClient(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	// Reset global client
	globalClient = nil
	globalOnce = sync.Once{}

	cfg := config.DefaultDockerConfig()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err)

	err = InitGlobal(cfg, log)
	assert.NoError(t, err)

	client := Global()
	assert.NotNil(t, client)
	assert.False(t, client.IsClosed())

	// Test that calling InitGlobal again doesn't reset
	cfg2 := config.DockerConfig{
		Host:       "unix:///var/run/invalid.sock",
		APIVersion: "1.44",
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		RetryDelay: 100 * time.Millisecond,
		TLSVerify:  false,
	}
	err = InitGlobal(cfg2, log)
	assert.NoError(t, err)

	// Client should still be using the original config
	client2 := Global()
	assert.Same(t, client, client2)
	assert.False(t, client.IsClosed())

	err = client.Close()
	assert.NoError(t, err)

	// Reset global state
	globalClient = nil
	globalOnce = sync.Once{}
}

func TestCheckEnvironment(t *testing.T) {
	t.Run("environment check with Docker socket", func(t *testing.T) {
		if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
			// Docker socket doesn't exist
			err := CheckEnvironment()
			// Either DOCKER_HOST is set or we get an error
			if os.Getenv("DOCKER_HOST") == "" {
				assert.Error(t, err)
			}
		} else {
			// Docker socket exists
			err := CheckEnvironment()
			assert.NoError(t, err)
		}
	})

	t.Run("environment check with DOCKER_HOST", func(t *testing.T) {
		originalHost := os.Getenv("DOCKER_HOST")
		defer os.Setenv("DOCKER_HOST", originalHost)

		os.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")
		err := CheckEnvironment()
		assert.NoError(t, err)
	})
}

func TestIsDockerRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	if err := CheckEnvironment(); err != nil {
		// Docker not available, should return false
		running := IsDockerRunning(ctx)
		assert.False(t, running)
		return
	}

	// Reset global client to ensure fresh state
	globalClient = nil
	globalOnce = sync.Once{}

	running := IsDockerRunning(ctx)
	// This should be true if Docker is available
	if !running {
		t.Skip("Docker daemon not running")
	}
	assert.True(t, running)
}

func TestClientReconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)

	ctx := context.Background()

	// Initial connection should work
	err = client.Ping(ctx)
	require.NoError(t, err)

	// Reconnect
	err = client.Reconnect(ctx)
	assert.NoError(t, err)

	// Ping should still work after reconnect
	err = client.Ping(ctx)
	assert.NoError(t, err)

	err = client.Close()
	assert.NoError(t, err)
}

func TestClientUnderlyingAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	underlyingClient := client.Client()
	assert.NotNil(t, underlyingClient)

	// Verify we can use the underlying client
	ctx := context.Background()
	_, err = underlyingClient.Ping(ctx)
	assert.NoError(t, err)
}

func TestDockerClient(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	// This is the main test for the verification command
	cfg := config.DefaultDockerConfig()
	log, err := logger.New(config.DefaultLoggingConfig())
	require.NoError(t, err)

	client, err := New(cfg, log)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Test basic operations
	ctx := context.Background()

	// Test Ping
	err = client.Ping(ctx)
	assert.NoError(t, err, "Ping should succeed")

	// Test Version
	version, err := client.Version(ctx)
	assert.NoError(t, err, "Version should succeed")
	assert.NotEmpty(t, version, "Version should not be empty")

	// Test Info
	info, err := client.Info(ctx)
	assert.NoError(t, err, "Info should succeed")
	assert.NotNil(t, info, "Info should not be nil")

	// Test Close
	err = client.Close()
	assert.NoError(t, err, "Close should succeed")
	assert.True(t, client.IsClosed(), "Client should be closed after Close")
}

func TestClientConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	client, err := NewDefault(nil)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	var wg sync.WaitGroup

	// Test concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.Config()
			_ = client.IsClosed()
			_ = client.String()
		}()
	}

	// Test concurrent operations
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = client.Version(ctx)
		}()
	}

	wg.Wait()
}

func TestClientErrors(t *testing.T) {
	t.Run("invalid Docker host", func(t *testing.T) {
		cfg := config.DockerConfig{
			Host:       "unix:///invalid/path/docker.sock",
			APIVersion: "1.44",
			Timeout:    1 * time.Second,
			MaxRetries: 0,
			RetryDelay: 100 * time.Millisecond,
			TLSVerify:  false,
		}

		log, err := logger.New(config.DefaultLoggingConfig())
		require.NoError(t, err)

		_, err = New(cfg, log)
		assert.Error(t, err, "Should fail with invalid host")
	})

	t.Run("operations on closed client return error", func(t *testing.T) {
		client, err := NewDefault(nil)
		if err != nil {
			t.Skip("Cannot create client:", err)
		}

		err = client.Close()
		require.NoError(t, err)

		ctx := context.Background()
		_, err = client.Version(ctx)
		assert.Error(t, err)

		_, err = client.Info(ctx)
		assert.Error(t, err)
	})
}

// TestIsDockerRunningWithoutDocker tests the behavior when Docker is not available
func TestIsDockerRunningWithoutDocker(t *testing.T) {
	// Save and restore global client
	originalClient := globalClient
	defer func() { globalClient = originalClient }()

	// Create a closed client
	closedClient := &Client{closed: true}
	globalClient = closedClient
	globalOnce = sync.Once{}

	ctx := context.Background()
	running := IsDockerRunning(ctx)
	assert.False(t, running, "IsDockerRunning should return false for closed client")
}

func TestNewClientWithoutLogger(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CheckEnvironment(); err != nil {
		t.Skip("Docker not available:", err)
	}

	// Test with nil logger - should create default
	cfg := config.DefaultDockerConfig()
	client, err := New(cfg, nil)
	require.NoError(t, err)
	require.NotNil(t, client)

	err = client.Close()
	assert.NoError(t, err)
}
