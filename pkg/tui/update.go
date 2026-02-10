package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/billm/baaaht/orchestrator/pkg/tui/components"
)

// Update handles incoming messages and updates the model state.
// It returns the updated model and a command to execute.
// Part of the tea.Model interface.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle quit state
	if m.quitting {
		return m, tea.Quit
	}

	var cmd tea.Cmd

	// Update all components with the message
	m.status, _ = m.status.Update(msg)
	m.chat, cmd = m.chat.Update(msg)
	m.input, cmd = tea.Sequence(cmd, m.input.Update(msg))
	m.sessions, cmd = tea.Sequence(cmd, m.sessions.Update(msg))

	switch msg := msg.(type) {
	// Handle key presses
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	// Handle window resize
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)

	// Handle errors
	case error:
		m.err = msg
		return m, tea.Quit

	// Handle input submission
	case components.InputSubmitMsg:
		// For now, just add user message to chat
		// In phase-6-streaming, this will be sent to the gRPC server
		m.chat.AppendMessage(components.MessageTypeUser, msg.Text)
		m.input.Reset()
		return m, nil

	// Handle session creation success
	case SessionCreatedMsg:
		m.sessionID = msg.SessionID
		// Update status bar with session ID
		m.status, _ = m.status.Update(components.StatusSessionMsg{SessionID: msg.SessionID})
		// Update status bar to connected state
		m.status, _ = m.status.Update(components.StatusConnectedMsg{})
		return m, nil

	// Handle session creation failure
	case SessionCreateFailedMsg:
		m.err = msg.Err
		// Update status bar with error
		m.status, _ = m.status.Update(components.StatusErrorMsg{Err: msg.Err})
		return m, nil

	// Handle component-specific messages for passthrough
	// These will be expanded in subsequent subtasks:
	// - gRPC connection status messages (phase-3-grpc)
	// - Session messages (phase-5-session)
	// - Streaming response messages (phase-6-streaming)
	// - Health check tick messages (phase-3-grpc)
	}

	return m, cmd
}

// handleKeyMsg handles keyboard input.
func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Import components package for session commands
	// Session navigation keys (phase-5-session)
	if m.keys.IsNextSessionKey(msg) {
		return m, components.SessionsSelectNextCmd()
	}
	if m.keys.IsPreviousSessionKey(msg) {
		return m, components.SessionsSelectPrevCmd()
	}
	if m.keys.IsListSessionsKey(msg) {
		// Toggle sessions list visibility
		if m.sessions.IsVisible() {
			return m, components.SessionsHideCmd()
		}
		return m, components.SessionsShowCmd()
	}

	// Quit keys
	if m.keys.IsQuitKey(msg) {
		m.quitting = true
		return m, tea.Quit
	}

	// Additional key bindings will be added in subsequent subtasks:
	// - ctrl+enter: send message (phase-6-streaming)
	// - esc: cancel current operation

	return m, nil
}

// handleWindowSize handles terminal resize events.
func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	// Status bar uses full width
	m.status, _ = m.status.Update(msg)

	// Calculate available height for chat (header + status + input = 4 lines)
	chatHeight := m.height - 4
	if chatHeight < 1 {
		chatHeight = 1
	}

	// Update chat viewport with proper dimensions
	chatMsg := tea.WindowSizeMsg{Width: m.width, Height: chatHeight}
	m.chat, _ = m.chat.Update(chatMsg)

	// Input uses full width
	m.input, _ = m.input.Update(tea.WindowSizeMsg{Width: m.width, Height: 1})

	return m, nil
}
