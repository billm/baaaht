package tui

import (
	"github.com/billm/baaaht/orchestrator/pkg/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles incoming messages and updates the model state.
// It returns the updated model and a command to execute.
// Part of the tea.Model interface.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle quit state
	if m.quitting {
		return m, tea.Quit
	}

	// Handle special keys FIRST (before component updates)
	// Only intercept global keys like ctrl+enter, ctrl+l, etc.
	// Let regular typing pass through to components
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		keyStr := keyMsg.String()

		// Debug logging for ALL ctrl+key combinations (only in verbose mode)
		if m.verbose && m.logger != nil && len(keyStr) > 4 && (keyStr[:4] == "ctrl" || keyStr[:4] == "Ctrl") {
			m.logger.Info("Ctrl key pressed", "key", keyStr, "input_value", m.input.Value())
		}

		switch keyStr {
		case "ctrl+enter", "ctrl+enter ", "ctrl+j":
			// Send message
			text := m.input.Value()
			if m.verbose && m.logger != nil {
				m.logger.Info("Sending message via Ctrl+Enter", "text", text, "key", keyStr)
			}
			if text != "" {
				m.chat.AppendMessage(components.MessageTypeUser, text)
				resetCmd := m.input.Reset()
				return m, tea.Batch(m.sendMessageCmd(text), resetCmd)
			}
			return m, nil
		case "enter":
			// TEMPORARY: Also accept plain enter for testing
			// This helps us verify the message sending works
			text := m.input.Value()
			if m.verbose && m.logger != nil {
				m.logger.Info("Sending message via Enter", "text", text)
			}
			if text != "" {
				m.chat.AppendMessage(components.MessageTypeUser, text)
				resetCmd := m.input.Reset()
				return m, tea.Batch(m.sendMessageCmd(text), resetCmd)
			}
			return m, nil
		case "ctrl+c", "ctrl+d", "ctrl+x":
			// Quit
			m.quitting = true
			return m, m.shutdownCmd()
		case "ctrl+l":
			// Toggle sessions list
			if m.sessions.IsVisible() {
				return m, components.SessionsHideCmd()
			}
			// Show sessions list and fetch current sessions from orchestrator
			return m, tea.Batch(components.SessionsShowCmd(), m.listSessionsCmd())
		case "ctrl+n":
			// Next session
			return m, components.SessionsSelectNextCmd()
		case "ctrl+p":
			// Previous session
			return m, components.SessionsSelectPrevCmd()
		case "ctrl+r":
			// Retry connection (only if in error state)
			if m.status.GetError() != nil {
				sm, _ := m.status.Update(components.StatusErrorMsg{Err: nil})
				m.status = sm.(components.StatusModel)
				sm, _ = m.status.Update(components.StatusConnectingMsg{Reconnecting: true})
				m.status = sm.(components.StatusModel)
				return m, m.retryConnectionCmd()
			}
		case "esc":
			// Close sessions modal if visible
			if m.sessions.IsVisible() {
				return m, components.SessionsHideCmd()
			}
		}
		// For all other keys, fall through to component updates below
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
		// Add the current session to the sessions list
		sessionItem := components.SessionItem{
			SessionID: msg.SessionID,
			Name:      "Current Session",
			CreatedAt: "",
			IsActive:  true,
		}
		sessm, _ := m.sessions.Update(components.SessionsSetItemsMsg{Items: []components.SessionItem{sessionItem}})
		m.sessions = sessm.(components.SessionsModel)
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

	// Handle sessions list response
	case SessionsListMsg:
		// Convert proto sessions to SessionItem
		items := make([]components.SessionItem, len(msg.Sessions))
		for i, session := range msg.Sessions {
			createdAt := ""
			if session.CreatedAt != nil {
				createdAt = session.CreatedAt.AsTime().Format("2006-01-02 15:04:05")
			}
			items[i] = components.SessionItem{
				SessionID: session.Id,
				Name:      session.Metadata.Name,
				CreatedAt: createdAt,
				IsActive:  session.Id == m.sessionID,
			}
		}
		// Update the sessions list
		sessm, _ := m.sessions.Update(components.SessionsSetItemsMsg{Items: items})
		m.sessions = sessm.(components.SessionsModel)
		return m, nil

	// Handle sessions list failure
	case SessionsListFailedMsg:
		// Log error but don't show it prominently (sessions list is optional)
		if m.verbose && m.logger != nil {
			m.logger.Warn("Failed to list sessions", "error", msg.Err)
		}
		// Show at least the current session if we have one
		if m.sessionID != "" {
			items := []components.SessionItem{
				{
					SessionID: m.sessionID,
					Name:      "Current Session",
					CreatedAt: "",
					IsActive:  true,
				},
			}
			sessm, _ := m.sessions.Update(components.SessionsSetItemsMsg{Items: items})
			m.sessions = sessm.(components.SessionsModel)
		}
		return m, nil

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

// handleWindowSize handles terminal resize events.
func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	if m.verbose && m.logger != nil {
		m.logger.Info("Window size", "width", m.width, "height", m.height)
	}

	// Update status bar with dimensions
	sm, _ := m.status.Update(components.StatusDimensionsMsg{Width: m.width, Height: m.height})
	m.status = sm.(components.StatusModel)

	// Calculate available height for chat
	// Layout: header(1)+nl(1) + status(3)+nl(1) + chat + input(3)+nl(1) + footer(1) = total
	// Overhead: 11 lines
	contentHeight := m.height - 11
	if contentHeight < 5 {
		contentHeight = 5
	}

	if m.verbose && m.logger != nil {
		m.logger.Info("Chat dimensions", "width", m.width, "height", contentHeight)
	}

	// Adjust width for borders (bordered components need width - 2)
	widthBordered := m.width - 2
	if widthBordered < 1 {
		widthBordered = 1
	}

	// Update chat viewport with proper dimensions
	chatMsg := tea.WindowSizeMsg{Width: widthBordered, Height: contentHeight}
	cm, _ := m.chat.Update(chatMsg)
	m.chat = cm.(components.ChatModel)

	// Update sessions list with proper dimensions
	sessionsMsg := tea.WindowSizeMsg{Width: widthBordered, Height: contentHeight}
	sessm, _ := m.sessions.Update(sessionsMsg)
	m.sessions = sessm.(components.SessionsModel)

	// Input also has border when focused, so use same width
	im, _ := m.input.Update(tea.WindowSizeMsg{Width: widthBordered, Height: 1})
	m.input = im.(components.InputModel)

	return m, nil
}
