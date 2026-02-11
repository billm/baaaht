package main

import (
	"context"
	"fmt"
	"os"

	"github.com/billm/baaaht/orchestrator/pkg/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

const (
	// DefaultVersion is the default version string
	DefaultVersion = "0.1.0"
)

var (
	// CLI flags
	logLevel      string
	socketPath    string
	versionFlag   bool
	verbose       bool

	// Global variables
	rootLog *noopLogger
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tui",
	Short: "Baaaht TUI - Terminal User Interface for interactive sessions",
	Long: `TUI is the terminal interface for baaaht that provides an interactive
command-line experience for managing sessions, sending messages, and viewing
streaming responses from the AI agent.

Connects to the Orchestrator via gRPC over Unix Domain Socket.`,
	Version: DefaultVersion,
	RunE:    runTUI,
}

// runTUI executes the main TUI logic
func runTUI(cmd *cobra.Command, args []string) error {
	_ = context.Background() // ctx will be used for gRPC client initialization

	// Show version if requested
	if versionFlag {
		fmt.Printf("tui version %s\n", DefaultVersion)
		return nil
	}

	// Initialize logger
	if err := initLogger(); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	rootLog.Info("Starting baaaht TUI", "version", DefaultVersion)

	// TODO: Initialize gRPC client connection
	// This will be implemented in phase-3-grpc

	// TODO: Create and start Bubbletea application
	// This will be implemented in subtask-2-2 when the model is created
	// For now, just show a placeholder message
	rootLog.Info("TUI initialization complete", "socket", socketPath)

	// Create the TUI model
	model := tui.NewModel(socketPath, verbose, DefaultVersion)

	// Start the Bubbletea program
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		rootLog.Error("Failed to run TUI", "error", err)
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	rootLog.Info("TUI shutdown complete")
	return nil
}

// initLogger initializes the global logger based on CLI flags
func initLogger() error {
	// For now, use a no-op logger until we integrate the full logger
	// This avoids dependency on the orchestrator's internal logger
	rootLog = &noopLogger{}
	return nil
}

// noopLogger is a simple no-op logger for initial TUI implementation
type noopLogger struct{}

func (l *noopLogger) Debug(msg string, args ...interface{}) {}
func (l *noopLogger) Info(msg string, args ...interface{})  {}
func (l *noopLogger) Warn(msg string, args ...interface{})  {}
func (l *noopLogger) Error(msg string, args ...interface{}) {}

func main() {
	// Logging flags
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info",
		"Log level: debug, info, warn, error (default: info)")

	// gRPC socket path flag
	rootCmd.PersistentFlags().StringVar(&socketPath, "socket-path", "/tmp/baaaht-grpc.sock",
		"gRPC socket path for connecting to orchestrator")

	// Verbose flag
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false,
		"Enable verbose output")

	// Version flag
	rootCmd.Flags().BoolVar(&versionFlag, "version", false,
		"Show version information")

	// Execute the command
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Command execution failed:", err)
		os.Exit(1)
	}

	// Ensure clean exit
	os.Exit(0)
}
