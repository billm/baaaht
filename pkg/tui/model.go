package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/billm/baaaht/orchestrator/pkg/tui/components"
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

	// UI Components
	status components.StatusModel
	chat   components.ChatModel
	input  components.InputModel

	// Layout
	width  int
	height int

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
		status:     components.NewStatusModel(),
		chat:       components.NewChatModel(),
		input:      components.NewInputModel(),
		width:      0,
		height:     0,
	}
}

// Init initializes the model and returns the initial command.
// This is called once when the program starts.
// Part of the tea.Model interface.
func (m Model) Init() tea.Cmd {
	// Initialize the input component's cursor blink
	return m.input.Init
}
