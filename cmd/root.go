package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/baaaht/orchestrator/internal/config"
	"github.com/baaaht/orchestrator/internal/logger"
	"github.com/baaaht/orchestrator/pkg/orchestrator"
	"github.com/baaaht/orchestrator/pkg/types"
	"github.com/spf13/cobra"
)

var (
	// CLI flags
	cfgFile      string
	logLevel     string
	logFormat    string
	logOutput    string
	dockerHost   string
	apiHost      string
	apiPort      int
	versionFlag  bool

	// Global variables
	rootLog *logger.Logger
	orch    *orchestrator.Orchestrator
	shutdown *orchestrator.ShutdownManager
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
		Config:            *cfg,
		Logger:            rootLog,
		Version:           orchestrator.DefaultVersion,
		ShutdownTimeout:   cfg.Orchestrator.ShutdownTimeout,
		EnableHealthCheck: true,
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
		"duration", result.Duration.String(),
		"version", result.Version)

	// Create shutdown manager
	shutdown = orchestrator.NewShutdownManager(
		orch,
		cfg.Orchestrator.ShutdownTimeout,
		rootLog,
	)

	// Start signal handling
	shutdown.Start()
	rootLog.Info("Orchestrator is running. Press Ctrl+C to stop.")

	// Add shutdown hook for graceful cleanup
	shutdown.AddHook(func(ctx context.Context) error {
		rootLog.Info("Executing shutdown hook")
		return nil
	})

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

	// Apply CLI overrides
	if dockerHost != "" {
		cfg.Docker.Host = dockerHost
	}
	if apiHost != "" {
		cfg.APIServer.Host = apiHost
	}
	if apiPort > 0 {
		cfg.APIServer.Port = apiPort
	}

	return cfg, nil
}

// waitForShutdown blocks until a shutdown signal is received
func waitForShutdown() {
	if shutdown == nil {
		return
	}

	// This will block until Shutdown is called
	_ = shutdown.ShutdownAndWait(context.Background(), "signal received")
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		rootLog.Error("Command execution failed", "error", err)
		os.Exit(1)
	}
}

func init() {
	// Config file flag
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"Config file path (default: use environment variables)")

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

	// Version flag
	rootCmd.Flags().BoolVar(&versionFlag, "version", false,
		"Show version information")
}
