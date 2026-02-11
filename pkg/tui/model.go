package tui

import (
	"context"
	"fmt"

	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/tui/components"
	"github.com/billm/baaaht/orchestrator/proto"
	tea "github.com/charmbracelet/bubbletea"
)

// Model represents the main TUI application state.
// It implements the tea.Model interface following the Elm architecture.
type Model struct {
	// Configuration
	socketPath string
	verbose    bool
	version    string

	// Application state
	quitting bool
	err      error

	// UI Components
	status   components.StatusModel
	chat     components.ChatModel
	input    components.InputModel
	sessions components.SessionsModel

	// Layout
	width  int
	height int

	// Keyboard shortcuts
	keys KeyMap

	// gRPC client and session
	client    *OrchestratorClient
	sessionID string
	logger    *logger.Logger

	// Streaming state
	stream proto.OrchestratorService_StreamMessagesClient
}

// NewModel creates a new TUI model with the specified configuration.
func NewModel(socketPath string, verbose bool, version string) Model {
	// Create logger only if verbose is enabled
	var log *logger.Logger
	if verbose {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			// If logger creation fails, continue without logger
			log = nil
		}
	}

	// Create gRPC client
	client, err := NewOrchestratorClient(socketPath, ClientConfig{}, log)
	if err != nil {
		// If client creation fails, store the error for display
		return Model{
			socketPath: socketPath,
			verbose:    verbose,
			version:    version,
			quitting:   false,
			err:        err,
			status:     components.NewStatusModel(),
			chat:       components.NewChatModel(),
			input:      components.NewInputModel(),
			sessions:   components.NewSessionsModel(),
			width:      0,
			height:     0,
			keys:       DefaultKeyMap(),
			client:     nil,
			sessionID:  "",
			logger:     log,
		}
	}

	return Model{
		socketPath: socketPath,
		verbose:    verbose,
		version:    version,
		quitting:   false,
		err:        nil,
		status:     components.NewStatusModel(),
		chat:       components.NewChatModel(),
		input:      components.NewInputModel(),
		sessions:   components.NewSessionsModel(),
		width:      0,
		height:     0,
		keys:       DefaultKeyMap(),
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
	// Note: initStreamCmd is called after session creation succeeds (see SessionCreatedMsg handler)
	return tea.Sequence(m.input.Init(), m.createSessionCmd())
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

// ConnectionRetryMsg is sent when a connection retry is attempted.
type ConnectionRetryMsg struct{}

// Streaming messages

// StreamMsgReceivedMsg is sent when a streaming response message is received.
type StreamMsgReceivedMsg struct {
	Role    string // "assistant", "system", "tool"
	Content string
}

// StreamSendFailedMsg is sent when sending a message via the stream fails.
type StreamSendFailedMsg struct {
	Err error
}

// createSessionCmd returns a command that creates a new session via gRPC.
func (m Model) createSessionCmd() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return SessionCreateFailedMsg{Err: fmt.Errorf("client not initialized")}
		}

		// Connect to the gRPC server
		ctx := context.Background()
		if err := m.client.Dial(ctx); err != nil {
			return SessionCreateFailedMsg{Err: err}
		}

		// Create the session request
		req := &proto.CreateSessionRequest{
			Metadata: &proto.SessionMetadata{
				Name:   "tui-session",
				Labels: map[string]string{"client": "tui"},
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

// initStreamCmd initializes the streaming connection.
func (m Model) initStreamCmd() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil || m.sessionID == "" {
			return StreamSendFailedMsg{Err: fmt.Errorf("client or session not initialized")}
		}

		ctx := context.Background()
		stream, err := m.client.StreamMessages(ctx)
		if err != nil {
			return StreamSendFailedMsg{Err: err}
		}

		// Store the stream in a message that will update the model
		return StreamInitializedMsg{Stream: stream}
	}
}

// StreamInitializedMsg is sent when the stream is initialized.
type StreamInitializedMsg struct {
	Stream proto.OrchestratorService_StreamMessagesClient
}

// sendMessageCmd returns a command that sends a message via the stream.
func (m Model) sendMessageCmd(content string) tea.Cmd {
	return func() tea.Msg {
		if m.verbose && m.logger != nil {
			m.logger.Info("sendMessageCmd called", "session_id", m.sessionID, "content", content, "stream_nil", m.stream == nil)
		}

		if m.stream == nil {
			err := fmt.Errorf("stream not initialized")
			if m.verbose && m.logger != nil {
				m.logger.Error("Cannot send message - stream not initialized", "error", err)
			}
			return StreamSendFailedMsg{Err: err}
		}

		// Send the user message
		req := &proto.StreamMessageRequest{
			SessionId: m.sessionID,
			Payload: &proto.StreamMessageRequest_Message{
				Message: &proto.Message{
					Role:    proto.MessageRole_MESSAGE_ROLE_USER,
					Content: content,
				},
			},
		}

		if m.verbose && m.logger != nil {
			m.logger.Info("Sending message to stream", "session_id", m.sessionID, "content", content)
		}

		if err := m.stream.Send(req); err != nil {
			if m.verbose && m.logger != nil {
				m.logger.Error("Failed to send message via stream", "error", err)
			}
			return StreamSendFailedMsg{Err: err}
		}

		if m.verbose && m.logger != nil {
			m.logger.Info("Message sent successfully, waiting for response")
		}

		// After sending, wait for response
		return m.waitForStreamMsgCmd()()
	}
}

// waitForStreamMsgCmd returns a command that waits for incoming stream messages.
func (m Model) waitForStreamMsgCmd() tea.Cmd {
	return func() tea.Msg {
		if m.stream == nil {
			return StreamSendFailedMsg{Err: fmt.Errorf("stream not initialized")}
		}

		for {
			resp, err := m.stream.Recv()
			if err != nil {
				return StreamSendFailedMsg{Err: err}
			}

			// Handle different response types
			switch payload := resp.Payload.(type) {
			case *proto.StreamMessageResponse_Message:
				msg := payload.Message
				role := "assistant"
				switch msg.Role {
				case proto.MessageRole_MESSAGE_ROLE_ASSISTANT:
					role = "assistant"
				case proto.MessageRole_MESSAGE_ROLE_SYSTEM:
					role = "system"
				case proto.MessageRole_MESSAGE_ROLE_TOOL:
					role = "tool"
				}
				return StreamMsgReceivedMsg{
					Role:    role,
					Content: msg.Content,
				}
			case *proto.StreamMessageResponse_Event:
				// For now, ignore events and continue waiting for the next message
				continue
			case *proto.StreamMessageResponse_Heartbeat:
				// Continue waiting for messages after heartbeat
				continue
			default:
				// Unknown payload type; skip and wait for the next message
				continue
			}
		}
	}
}

// Shutdown messages

// ShutdownCompleteMsg is sent when graceful shutdown completes successfully.
type ShutdownCompleteMsg struct{}

// ShutdownFailedMsg is sent when shutdown encounters an error.
type ShutdownFailedMsg struct {
	Err error
}

// shutdownCmd returns a command that gracefully shuts down the TUI by closing
// the session and gRPC connection before quitting.
func (m Model) shutdownCmd() tea.Cmd {
	return func() tea.Msg {
		// Log shutdown start
		if m.logger != nil {
			m.logger.Info("Starting graceful shutdown", "session_id", m.sessionID)
		}

		ctx := context.Background()

		// Close the stream if active
		if m.stream != nil {
			if err := m.stream.CloseSend(); err != nil {
				if m.logger != nil {
					m.logger.Warn("Failed to close stream send side", "error", err)
				}
				// Continue with shutdown despite stream close error
			}
		}

		// Close the session if we have one
		if m.sessionID != "" && m.client != nil {
			req := &proto.CloseSessionRequest{
				SessionId: m.sessionID,
			}
			_, err := m.client.CloseSession(ctx, req)
			if err != nil {
				if m.logger != nil {
					m.logger.Warn("Failed to close session", "session_id", m.sessionID, "error", err)
				}
				// Continue with shutdown despite session close error
			} else {
				if m.logger != nil {
					m.logger.Info("Closed session", "session_id", m.sessionID)
				}
			}
		}

		// Close the gRPC client connection
		if m.client != nil {
			if err := m.client.Close(); err != nil {
				if m.logger != nil {
					m.logger.Error("Failed to close gRPC client", "error", err)
				}
				return ShutdownFailedMsg{Err: err}
			}
			if m.logger != nil {
				m.logger.Info("Closed gRPC client connection")
			}
		}

		if m.logger != nil {
			m.logger.Info("Graceful shutdown complete")
		}

		return ShutdownCompleteMsg{}
	}
}

// retryConnectionCmd returns a command that retries the connection.
// It attempts to re-establish the connection and create a new session.
func (m Model) retryConnectionCmd() tea.Cmd {
	return func() tea.Msg {
		// Signal that retry is starting
		if m.logger != nil {
			m.logger.Info("Retrying connection", "previous_session_id", m.sessionID)
		}

		// Re-dial if needed
		if m.client != nil {
			ctx := context.Background()
			if err := m.client.Dial(ctx); err != nil {
				if m.logger != nil {
					m.logger.Warn("Retry dial failed", "error", err)
				}
				return SessionCreateFailedMsg{Err: err}
			}
		}

		// Attempt to create a new session
		if m.client == nil {
			return SessionCreateFailedMsg{Err: fmt.Errorf("client not initialized")}
		}

		ctx := context.Background()
		req := &proto.CreateSessionRequest{
			Metadata: &proto.SessionMetadata{
				Name:   "tui-session",
				Labels: map[string]string{"client": "tui"},
			},
		}

		resp, err := m.client.CreateSession(ctx, req)
		if err != nil {
			if m.logger != nil {
				m.logger.Warn("Retry session creation failed", "error", err)
			}
			return SessionCreateFailedMsg{Err: err}
		}

		if m.logger != nil {
			m.logger.Info("Retry connection successful", "session_id", resp.SessionId)
		}

		return SessionCreatedMsg{SessionID: resp.SessionId}
	}
}

// SessionsListMsg is sent when the list of sessions is retrieved.
type SessionsListMsg struct {
	Sessions []*proto.Session
}

// SessionsListFailedMsg is sent when listing sessions fails.
type SessionsListFailedMsg struct {
	Err error
}

// listSessionsCmd returns a command that fetches all sessions from the orchestrator.
func (m Model) listSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return SessionsListFailedMsg{Err: fmt.Errorf("client not initialized")}
		}

		ctx := context.Background()
		req := &proto.ListSessionsRequest{}

		resp, err := m.client.ListSessions(ctx, req)
		if err != nil {
			if m.logger != nil {
				m.logger.Warn("Failed to list sessions", "error", err)
			}
			return SessionsListFailedMsg{Err: err}
		}

		if m.logger != nil {
			m.logger.Info("Successfully listed sessions", "count", len(resp.Sessions))
		}

		return SessionsListMsg{Sessions: resp.Sessions}
	}
}
