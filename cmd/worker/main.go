package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/worker"
	"github.com/spf13/cobra"
)

var (
	// CLI flags
	cfgFile        string
	logLevel       string
	logFormat      string
	logOutput      string
	orchestratorAddr string
	workerName     string
	versionFlag    bool

	// Worker CLI flags
	dialTimeout           string
	rpcTimeout            string
	maxRecvMsgSize        int
	maxSendMsgSize        int
	reconnectInterval     string
	reconnectMaxAttempts  int
	heartbeatInterval     string

	// Global variables
	rootLog     *logger.Logger
	workerAgent *worker.Agent
	shutdownCtx context.Context
	shutdownCancel context.CancelFunc
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "worker",
	Short: "Baaaht Worker - Tool execution agent for containerized operations",
	Long: `Worker is a baaaht agent that executes tools within isolated containers.
It connects to the orchestrator via gRPC, registers itself, and listens for
task execution requests.

The worker runs tool containers (file operations, web requests, etc.) with
proper policy enforcement and resource isolation.`,
	Version: worker.DefaultVersion,
	RunE:    runWorker,
}

// runWorker executes the main worker logic
func runWorker(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Show version if requested
	if versionFlag {
		fmt.Printf("worker version %s\n", worker.GetVersion())
		return nil
	}

	// Initialize logger
	if err := initLogger(); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	rootLog.Info("Starting baaaht worker",
		"version", worker.GetVersion())

	// Create shutdown context
	shutdownCtx, shutdownCancel = context.WithCancel(ctx)
	defer shutdownCancel()

	// Get orchestrator address (priority: CLI flag > ORCHESTRATOR_URL env > default)
	addr := orchestratorAddr
	if addr == "" {
		addr = os.Getenv("ORCHESTRATOR_URL")
	}
	if addr == "" {
		addr = "unix:///tmp/baaaht-grpc.sock"
	}

	// Get worker name
	name := workerName
	if name == "" {
		hostname, err := os.Hostname()
		if err != nil {
			rootLog.Warn("Failed to get hostname, using default", "error", err)
			name = "worker-" + fmt.Sprintf("%d", time.Now().Unix())
		} else {
			name = "worker-" + hostname
		}
	}

	// Parse timeout durations
	dialTimeoutDuration, err := parseDuration(dialTimeout, worker.DefaultDialTimeout)
	if err != nil {
		return fmt.Errorf("invalid dial-timeout: %w", err)
	}
	rpcTimeoutDuration, err := parseDuration(rpcTimeout, worker.DefaultRPCTimeout)
	if err != nil {
		return fmt.Errorf("invalid rpc-timeout: %w", err)
	}
	reconnectIntervalDuration, err := parseDuration(reconnectInterval, worker.DefaultReconnectInterval)
	if err != nil {
		return fmt.Errorf("invalid reconnect-interval: %w", err)
	}
	heartbeatIntervalDuration, err := parseDuration(heartbeatInterval, worker.DefaultHeartbeatInterval)
	if err != nil {
		return fmt.Errorf("invalid heartbeat-interval: %w", err)
	}

	// Create bootstrap config
	bootstrapCfg := worker.BootstrapConfig{
		Logger:               rootLog,
		Version:              worker.DefaultVersion,
		OrchestratorAddr:     addr,
		WorkerName:           name,
		DialTimeout:          dialTimeoutDuration,
		RPCTimeout:           rpcTimeoutDuration,
		MaxRecvMsgSize:       maxRecvMsgSize,
		MaxSendMsgSize:       maxSendMsgSize,
		ReconnectInterval:    reconnectIntervalDuration,
		ReconnectMaxAttempts: reconnectMaxAttempts,
		HeartbeatInterval:    heartbeatIntervalDuration,
		ShutdownTimeout:      worker.DefaultShutdownTimeout,
		EnableHealthCheck:    true,
	}

	// Bootstrap worker
	rootLog.Info("Bootstrapping worker...")
	result, err := worker.Bootstrap(ctx, bootstrapCfg)
	if err != nil {
		rootLog.Error("Failed to bootstrap worker", "error", err)
		return err
	}
	workerAgent = result.Agent

	rootLog.Info("Worker initialized successfully",
		"duration", result.Duration(),
		"version", result.Version,
		"worker_name", result.WorkerName,
		"agent_id", workerAgent.GetAgentID())

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	rootLog.Info("Worker is running. Press Ctrl+C to stop.")
	select {
	case sig := <-sigCh:
		rootLog.Info("Received shutdown signal", "signal", sig)
	case <-shutdownCtx.Done():
		rootLog.Info("Shutdown requested")
	}

	// Graceful shutdown
	rootLog.Info("Shutting down worker...")
	shutdownCtx, shutdownCancel = context.WithTimeout(context.Background(), worker.DefaultShutdownTimeout)
	defer shutdownCancel()

	if err := workerAgent.Close(); err != nil {
		rootLog.Error("Error during worker shutdown", "error", err)
		return err
	}

	rootLog.Info("Worker shutdown complete")
	return nil
}

// initLogger initializes the global logger based on CLI flags and config
func initLogger() error {
	cfg := config.DefaultLoggingConfig()

	// Override with CLI flags if provided
	if logLevel != "" {
		cfg.Level = logLevel
	}
	if logFormat != "" {
		cfg.Format = logFormat
	}
	if logOutput != "" {
		cfg.Output = logOutput
	}

	log, err := logger.New(cfg)
	if err != nil {
		return err
	}

	rootLog = log
	logger.SetGlobal(log)
	return nil
}

// parseDuration parses a duration string with a fallback default
func parseDuration(s string, defaultDuration time.Duration) (time.Duration, error) {
	if s == "" {
		return defaultDuration, nil
	}
	return time.ParseDuration(s)
}

// getDefaultConfigPath returns the default config file path
func getDefaultConfigPath() string {
	if path, err := config.GetDefaultConfigPath(); err == nil {
		return path
	}
	return "~/.config/baaaht/config.yaml"
}

func main() {
	// Config file flag
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"Config file path (default: ~/.config/baaaht/config.yaml)")

	// Logging flags
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "",
		"Log level: debug, info, warn, error (default: from config or env)")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "",
		"Log format: json, text (default: from config or env)")
	rootCmd.PersistentFlags().StringVar(&logOutput, "log-output", "",
		"Log output: stdout, stderr, or file path (default: from config or env)")

	// Connection flags
	rootCmd.PersistentFlags().StringVar(&orchestratorAddr, "orchestrator-addr", "",
		"Orchestrator gRPC address (default: unix:///tmp/baaaht-grpc.sock)")

	// Worker identity
	rootCmd.PersistentFlags().StringVar(&workerName, "name", "",
		"Worker name (default: worker-<hostname>)")

	// gRPC/Network flags
	rootCmd.PersistentFlags().StringVar(&dialTimeout, "dial-timeout", "",
		"Timeout for dialing orchestrator (default: 30s)")
	rootCmd.PersistentFlags().StringVar(&rpcTimeout, "rpc-timeout", "",
		"Timeout for RPC calls (default: 10s)")
	rootCmd.PersistentFlags().IntVar(&maxRecvMsgSize, "max-recv-msg-size", 0,
		"Maximum received message size in bytes (default: 104857600)")
	rootCmd.PersistentFlags().IntVar(&maxSendMsgSize, "max-send-msg-size", 0,
		"Maximum sent message size in bytes (default: 104857600)")

	// Reconnection flags
	rootCmd.PersistentFlags().StringVar(&reconnectInterval, "reconnect-interval", "",
		"Interval between reconnection attempts (default: 5s)")
	rootCmd.PersistentFlags().IntVar(&reconnectMaxAttempts, "reconnect-max-attempts", 0,
		"Maximum reconnection attempts (default: 0 for infinite)")

	// Heartbeat flags
	rootCmd.PersistentFlags().StringVar(&heartbeatInterval, "heartbeat-interval", "",
		"Interval between heartbeats (default: 30s)")

	// Version flag
	rootCmd.Flags().BoolVar(&versionFlag, "version", false,
		"Show version information")

	// Execute the command
	if err := rootCmd.Execute(); err != nil {
		if rootLog != nil {
			rootLog.Error("Command execution failed", "error", err)
		} else {
			fmt.Fprintln(os.Stderr, "Command execution failed:", err)
		}
		os.Exit(1)
	}

	// Ensure clean exit
	os.Exit(0)
}
