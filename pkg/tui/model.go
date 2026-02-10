package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Model represents the main TUI application state.
// It implements the tea.Model interface following the Elm architecture.
type Model struct {
	// Configuration
	socketPath string
	verbose    bool

	// Application state
	quitting bool
	err      error

	// Components will be added in subsequent subtasks:
	// - status bar for connection state
	// - chat viewport for messages
	// - text input for user input
	// - session list for navigation

	// gRPC client will be added in phase-3-grpc
	// client *OrchestratorClient
}

// NewModel creates a new TUI model with the specified configuration.
func NewModel(socketPath string, verbose bool) Model {
	return Model{
		socketPath: socketPath,
		verbose:    verbose,
		quitting:   false,
		err:        nil,
	}
}

// Init initializes the model and returns the initial command.
// This is called once when the program starts.
// Part of the tea.Model interface.
func (m Model) Init() tea.Cmd {
	// Initial commands will be added in subsequent subtasks:
	// - Connect to gRPC server (phase-3-grpc)
	// - Create initial session (phase-5-session)
	// - Start health check ticker (phase-3-grpc)

	return nil
}
