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

	var cmds []tea.Cmd

	// Update all components with the message
	statusModel, statusCmd := m.status.Update(msg)
	m.status = statusModel.(components.StatusModel)
	if statusCmd != nil {
		cmds = append(cmds, statusCmd)
	}

	chatModel, chatCmd := m.chat.Update(msg)
	m.chat = chatModel.(components.ChatModel)
	if chatCmd != nil {
		cmds = append(cmds, chatCmd)
	}

	inputModel, inputCmd := m.input.Update(msg)
	m.input = inputModel.(components.InputModel)
	if inputCmd != nil {
		cmds = append(cmds, inputCmd)
	}

	sessionsModel, sessionsCmd := m.sessions.Update(msg)
	m.sessions = sessionsModel.(components.SessionsModel)
	if sessionsCmd != nil {
		cmds = append(cmds, sessionsCmd)
	}

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
		// Add user message to chat and send via gRPC stream
		m.chat.AppendMessage(components.MessageTypeUser, msg.Text)
		m.input.Reset()
		return m, m.sendMessageCmd(msg.Text)

	// Handle session creation success
	case SessionCreatedMsg:
		m.sessionID = msg.SessionID
		// Update status bar with session ID
		sm, _ := m.status.Update(components.StatusSessionMsg{SessionID: msg.SessionID})
		m.status = sm.(components.StatusModel)
		// Update status bar to connected state
		sm, _ = m.status.Update(components.StatusConnectedMsg{})
		m.status = sm.(components.StatusModel)
		// Initialize stream for new session
		return m, m.initStreamCmd()

	// Handle session creation failure
	case SessionCreateFailedMsg:
		// Connection errors are recoverable - don't set m.err
		// Update status bar with error so user can see what happened
		sm, _ := m.status.Update(components.StatusErrorMsg{Err: msg.Err})
		m.status = sm.(components.StatusModel)
		sm, _ = m.status.Update(components.StatusDisconnectedMsg{})
		m.status = sm.(components.StatusModel)
		return m, nil

	// Handle stream initialization
	case StreamInitializedMsg:
		m.stream = msg.Stream
		return m, nil

	// Handle streaming response messages
	case StreamMsgReceivedMsg:
		// Convert role to message type
		var msgType components.MessageType
		switch msg.Role {
		case "assistant":
			msgType = components.MessageTypeAssistant
		case "system":
			msgType = components.MessageTypeSystem
		case "tool":
			msgType = components.MessageTypeSystem
		default:
			msgType = components.MessageTypeAssistant
		}
		m.chat.AppendMessage(msgType, msg.Content)
		// Continue waiting for more messages
		return m, m.waitForStreamMsgCmd()

	// Handle stream send failure
	case StreamSendFailedMsg:
		// Stream errors are recoverable - don't set m.err
		// Update status bar with error so user can see what happened
		sm, _ := m.status.Update(components.StatusErrorMsg{Err: msg.Err})
		m.status = sm.(components.StatusModel)
		sm, _ = m.status.Update(components.StatusDisconnectedMsg{})
		m.status = sm.(components.StatusModel)
		return m, nil

	// Handle shutdown complete
	case ShutdownCompleteMsg:
		return m, tea.Quit

	// Handle shutdown failed
	case ShutdownFailedMsg:
		m.err = msg.Err
		// Log the error but still quit
		return m, tea.Quit

	// Handle connection retry (clear error state and allow retry)
	case ConnectionRetryMsg:
		// Connection retry is handled by the command itself
		return m, nil

	// Handle component-specific messages for passthrough
	// These will be expanded in subsequent subtasks:
	// - gRPC connection status messages (phase-3-grpc)
	// - Session messages (phase-5-session)
	// - Streaming response messages (phase-6-streaming)
	// - Health check tick messages (phase-3-grpc)
	}

	return m, tea.Batch(cmds...)
}

// handleKeyMsg handles keyboard input.
func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Retry connection on ctrl+r when in error state
	if m.keys.IsRetryConnectionKey(msg) {
		if m.status.GetError() != nil {
			// Clear error and retry connection
			sm, _ := m.status.Update(components.StatusErrorMsg{Err: nil})
			m.status = sm.(components.StatusModel)
			sm, _ = m.status.Update(components.StatusConnectingMsg{Reconnecting: true})
			m.status = sm.(components.StatusModel)
			return m, m.retryConnectionCmd()
		}
	}

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
		return m, m.shutdownCmd()
	}

	// Send message key
	if m.keys.IsSendKey(msg) {
		// Submit the current input
		text := m.input.Value()
		if text != "" {
			m.chat.AppendMessage(components.MessageTypeUser, text)
			m.input.Reset()
			return m, m.sendMessageCmd(text)
		}
		return m, nil
	}

	// Additional key bindings will be added in subsequent subtasks:
	// - esc: cancel current operation

	return m, nil
}

// handleWindowSize handles terminal resize events.
func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	// Status bar uses full width
	sm, _ := m.status.Update(msg)
	m.status = sm.(components.StatusModel)

	// Calculate available height for chat and sessions (header + status + input = 4 lines)
	contentHeight := m.height - 4
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Update chat viewport with proper dimensions
	chatMsg := tea.WindowSizeMsg{Width: m.width, Height: contentHeight}
	cm, _ := m.chat.Update(chatMsg)
	m.chat = cm.(components.ChatModel)

	// Update sessions list with proper dimensions
	sessionsMsg := tea.WindowSizeMsg{Width: m.width, Height: contentHeight}
	sessm, _ := m.sessions.Update(sessionsMsg)
	m.sessions = sessm.(components.SessionsModel)

	// Input uses full width
	im, _ := m.input.Update(tea.WindowSizeMsg{Width: m.width, Height: 1})
	m.input = im.(components.InputModel)

	return m, nil
}
