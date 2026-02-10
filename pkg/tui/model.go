package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/tui/components"
	"github.com/billm/baaaht/orchestrator/proto"
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

	// gRPC client and session
	client    *OrchestratorClient
	sessionID string
	logger    *logger.Logger
}

// NewModel creates a new TUI model with the specified configuration.
func NewModel(socketPath string, verbose bool) Model {
	// Create logger
	log, err := logger.NewDefault()
	if err != nil {
		// If logger creation fails, continue without logger
		log = nil
	}

	// Create gRPC client
	client, err := NewOrchestratorClient(socketPath, ClientConfig{}, log)
	if err != nil {
		// If client creation fails, store the error for display
		return Model{
			socketPath: socketPath,
			verbose:    verbose,
			quitting:   false,
			err:        err,
			status:     components.NewStatusModel(),
			chat:       components.NewChatModel(),
			input:      components.NewInputModel(),
			width:      0,
			height:     0,
			client:     nil,
			sessionID:  "",
			logger:     log,
		}
	}

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
		client:     client,
		sessionID:  "",
		logger:     log,
	}
}

// Init initializes the model and returns the initial command.
// This is called once when the program starts.
// Part of the tea.Model interface.
func (m Model) Init() tea.Cmd {
	// Initialize the input component's cursor blink and create a session
	return tea.Sequence(m.input.Init, m.createSessionCmd())
}

// Session creation messages

// SessionCreatedMsg is sent when a session is successfully created.
type SessionCreatedMsg struct {
	SessionID string
}

// SessionCreateFailedMsg is sent when session creation fails.
type SessionCreateFailedMsg struct {
	Err error
}

// createSessionCmd returns a command that creates a new session via gRPC.
func (m Model) createSessionCmd() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return SessionCreateFailedMsg{Err: logger.NewError("client not initialized")}
		}

		// Connect to the gRPC server
		ctx := context.Background()
		if err := m.client.Dial(ctx); err != nil {
			return SessionCreateFailedMsg{Err: err}
		}

		// Create the session request
		req := &proto.CreateSessionRequest{
			Metadata: &proto.SessionMetadata{
				Name:    "tui-session",
				Labels:  map[string]string{"client": "tui"},
			},
		}

		// Call CreateSession
		resp, err := m.client.CreateSession(ctx, req)
		if err != nil {
			return SessionCreateFailedMsg{Err: err}
		}

		return SessionCreatedMsg{SessionID: resp.SessionId}
	}
}
