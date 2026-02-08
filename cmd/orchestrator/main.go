package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/grpc"
	"github.com/billm/baaaht/orchestrator/pkg/orchestrator"
	"github.com/spf13/cobra"
)

var (
	// CLI flags
	cfgFile     string
	logLevel    string
	logFormat   string
	logOutput   string
	dockerHost  string
	apiHost     string
	apiPort     int
	versionFlag bool

	// gRPC CLI flags
	grpcSocketPath     string
	grpcMaxRecvMsgSize int
	grpcMaxSendMsgSize int
	grpcTimeout        string
	grpcMaxConnections int

	// Global variables
	rootLog   *logger.Logger
	orch      *orchestrator.Orchestrator
	shutdown  *orchestrator.ShutdownManager
	cfgReloader *config.Reloader
	grpcResult *grpc.BootstrapResult
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "orchestrator",
	Short: "Baaaht Orchestrator - Container lifecycle and event management system",
	Long: `Orchestrator is the central nervous system of baaaht that manages container
lifecycles, routes events between components, manages sessions, brokers IPC,
schedules tasks, and enforces security policies.

This is the host process that coordinates all subsystems and provides the
security boundary for agent containers.`,
	Version: orchestrator.DefaultVersion,
	RunE:    runOrchestrator,
}

// runOrchestrator executes the main orchestrator logic
func runOrchestrator(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Show version if requested
	if versionFlag {
		fmt.Printf("orchestrator version %s\n", orchestrator.GetVersion())
		return nil
	}

	// Initialize logger
	if err := initLogger(); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	rootLog.Info("Starting baaaht orchestrator",
		"version", orchestrator.GetVersion())

	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Create bootstrap config
	bootstrapCfg := orchestrator.BootstrapConfig{
		Config:              *cfg,
		Logger:              rootLog,
		Version:             orchestrator.DefaultVersion,
		ShutdownTimeout:     cfg.Orchestrator.ShutdownTimeout,
		EnableHealthCheck:   true,
		HealthCheckInterval: 30 * time.Second,
	}

	// Bootstrap orchestrator
	rootLog.Info("Bootstrapping orchestrator...")
	result, err := orchestrator.Bootstrap(ctx, bootstrapCfg)
	if err != nil {
		rootLog.Error("Failed to bootstrap orchestrator", "error", err)
		return err
	}
	orch = result.Orchestrator

	rootLog.Info("Orchestrator initialized successfully",
		"duration", result.Duration(),
		"version", result.Version)

	// Bootstrap gRPC server
	rootLog.Info("Bootstrapping gRPC server...")
	grpcBootstrapCfg := grpc.BootstrapConfig{
		Config:              cfg.GRPC,
		Logger:              rootLog,
		SessionManager:      orch.SessionManager(),
		EventBus:            orch.EventBus(),
		Version:             orchestrator.DefaultVersion,
		ShutdownTimeout:     cfg.Orchestrator.ShutdownTimeout,
		EnableHealthCheck:   true,
		HealthCheckInterval: 30 * time.Second,
	}
	grpcResult, err = grpc.Bootstrap(ctx, grpcBootstrapCfg)
	if err != nil {
		rootLog.Error("Failed to bootstrap gRPC server", "error", err)
		return err
	}

	rootLog.Info("gRPC server initialized successfully",
		"socket_path", grpcResult.SocketPath,
		"duration", grpcResult.Duration())

	// Create shutdown manager
	shutdown = orchestrator.NewShutdownManager(
		orch,
		cfg.Orchestrator.ShutdownTimeout,
		rootLog,
	)

	// Get config path for reloader
	configPath := cfgFile
	if configPath == "" {
		configPath = getDefaultConfigPath()
	}

	// Create and start config reloader
	cfgReloader = config.NewReloader(configPath, cfg)
	cfgReloader.AddCallback(func(ctx context.Context, newConfig *config.Config) error {
		rootLog.Info("Configuration reloaded, applying to orchestrator")
		if err := orch.UpdateConfig(*newConfig); err != nil {
			rootLog.Error("Failed to apply reloaded configuration", "error", err)
			return err
		}
		rootLog.Info("Successfully applied reloaded configuration")
		return nil
	})
	cfgReloader.Start()
	rootLog.Info("Config reloader started, send SIGHUP to reload configuration",
		"config_path", configPath)

	// Add shutdown hook for graceful cleanup
	shutdown.AddHook(func(ctx context.Context) error {
		rootLog.Info("Executing shutdown hook")
		// Stop gRPC server
		if grpcResult != nil && grpcResult.Server != nil {
			if grpcResult.Health != nil {
				grpcResult.Health.Shutdown()
			}
			if err := grpcResult.Server.Stop(); err != nil {
				rootLog.Error("Failed to stop gRPC server", "error", err)
			} else {
				rootLog.Info("gRPC server stopped")
			}
		}
		// Stop config reloader
		if cfgReloader != nil {
			cfgReloader.Stop()
			rootLog.Info("Config reloader stopped")
		}
		return nil
	})

	// Start signal handling
	shutdown.Start()
	rootLog.Info("Orchestrator is running. Press Ctrl+C to stop.")

	// Wait for shutdown signal
	waitForShutdown()

	// Stop signal handling
	shutdown.Stop()

	rootLog.Info("Orchestrator shutdown complete")
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

// loadConfig loads the configuration from environment variables and CLI overrides
func loadConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	// Apply CLI overrides (highest precedence)
	cfg.ApplyOverrides(config.OverrideOptions{
		DockerHost:         dockerHost,
		APIServerHost:      apiHost,
		APIServerPort:      apiPort,
		LogLevel:           logLevel,
		LogFormat:          logFormat,
		LogOutput:          logOutput,
		GRPCSocketPath:     grpcSocketPath,
		GRPCMaxRecvMsgSize: grpcMaxRecvMsgSize,
		GRPCMaxSendMsgSize: grpcMaxSendMsgSize,
		GRPCTimeout:        grpcTimeout,
		GRPCMaxConnections: grpcMaxConnections,
	})

	return cfg, nil
}

// getDefaultConfigPath returns the default config file path
func getDefaultConfigPath() string {
	if path, err := config.GetDefaultConfigPath(); err == nil {
		return path
	}
	return "~/.config/baaaht/config.yaml"
}

// waitForShutdown blocks until a shutdown signal is received and shutdown completes
func waitForShutdown() {
	if shutdown == nil {
		return
	}

	// Block until shutdown is completed (triggered by signal handler)
	_ = shutdown.WaitCompletion(context.Background())
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

	// Docker flags
	rootCmd.PersistentFlags().StringVar(&dockerHost, "docker-host", "",
		"Docker daemon host (default: unix:///var/run/docker.sock)")

	// API server flags
	rootCmd.PersistentFlags().StringVar(&apiHost, "api-host", "",
		"API server host (default: 0.0.0.0)")
	rootCmd.PersistentFlags().IntVar(&apiPort, "api-port", 0,
		"API server port (default: from config or env)")

	// gRPC flags
	rootCmd.PersistentFlags().StringVar(&grpcSocketPath, "grpc-socket-path", "",
		"gRPC server socket path (default: /tmp/baaaht-grpc.sock)")
	rootCmd.PersistentFlags().IntVar(&grpcMaxRecvMsgSize, "grpc-max-recv-msg-size", 0,
		"gRPC max receive message size in bytes (default: 104857600)")
	rootCmd.PersistentFlags().IntVar(&grpcMaxSendMsgSize, "grpc-max-send-msg-size", 0,
		"gRPC max send message size in bytes (default: 104857600)")
	rootCmd.PersistentFlags().StringVar(&grpcTimeout, "grpc-timeout", "",
		"gRPC connection timeout (default: 30s)")
	rootCmd.PersistentFlags().IntVar(&grpcMaxConnections, "grpc-max-connections", 0,
		"gRPC max connections (default: 100)")

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
