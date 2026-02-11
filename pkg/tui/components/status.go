package components

import (
	"strings"

	"github.com/billm/baaaht/orchestrator/pkg/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// StatusModel represents the state of the status bar component.
// It displays connection state, session information, and any active errors.
type StatusModel struct {
	connected    bool
	connecting   bool
	sessionID    string
	error        error
	thinking     bool
	reconnecting bool
	width        int
}

// NewStatusModel creates a new status bar model with default values.
func NewStatusModel() StatusModel {
	return StatusModel{
		connected:    false,
		connecting:   false,
		sessionID:    "",
		error:        nil,
		thinking:     false,
		reconnecting: false,
		width:        0,
	}
}

// Init initializes the status bar model.
// Part of the tea.Model interface.
func (m StatusModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the status bar.
// Part of the tea.Model interface.
func (m StatusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case StatusConnectedMsg:
		m.connected = true
		m.connecting = false
		m.reconnecting = false
		m.error = nil
		return m, nil

	case StatusDisconnectedMsg:
		m.connected = false
		m.connecting = false
		m.reconnecting = false
		return m, nil

	case StatusConnectingMsg:
		m.connecting = true
		m.reconnecting = msg.Reconnecting
		return m, nil

	case StatusSessionMsg:
		m.sessionID = msg.SessionID
		return m, nil

	case StatusErrorMsg:
		m.error = msg.Err
		return m, nil

	case StatusThinkingMsg:
		m.thinking = msg.Thinking
		return m, nil
	}

	return m, nil
}

// View renders the status bar.
// Part of the tea.Model interface.
func (m StatusModel) View() string {
	if m.width <= 0 {
		return ""
	}

	// Build the status bar from left to right
	var sections []string

	// Connection status section (left)
	connStyle := styles.StatusStyle(m.connected, m.thinking, m.error)
	connText := m.connectionText()
	sections = append(sections, connStyle.Render(connText))

	// Session info section (center)
	if m.sessionID != "" {
		sessionStyle := styles.Styles.Muted
		if m.connected {
			sessionStyle = styles.Styles.StatusInfo
		}
		sections = append(sections, sessionStyle.Render("Session: "+m.shortSessionID()))
	}

	// Error section (right, if present)
	if m.error != nil {
		errorStyle := styles.Styles.StatusError
		errorText := strings.TrimSpace(m.error.Error())
		if len(errorText) > 30 {
			errorText = errorText[:27] + "..."
		}
		sections = append(sections, errorStyle.Render("⚠ "+errorText))
	}

	// Join sections with spacing
	var renderedSections []string
	for _, section := range sections {
		if section != "" {
			renderedSections = append(renderedSections, section)
		}
	}

	if len(renderedSections) == 0 {
		return ""
	}

	// Create the status bar with border
	statusBar := strings.Join(renderedSections, styles.Styles.HelpSeparator.Render(" • "))

	// Apply border style
	return styles.Styles.StatusBorder.
		Width(m.width).
		Render(statusBar)
}

// connectionText returns the connection status text based on current state.
func (m StatusModel) connectionText() string {
	switch {
	case m.error != nil:
		return "✕ Error"
	case m.connecting && m.reconnecting:
		return "↻ Reconnecting..."
	case m.connecting:
		return "⟳ Connecting..."
	case m.connected && m.thinking:
		return "● Thinking..."
	case m.connected:
		return "✓ Connected"
	default:
		return "✕ Disconnected"
	}
}

// shortSessionID returns a shortened version of the session ID for display.
func (m StatusModel) shortSessionID() string {
	if m.sessionID == "" {
		return ""
	}
	// Show first 8 characters of session ID
	if len(m.sessionID) > 8 {
		return m.sessionID[:8] + "..."
	}
	return m.sessionID
}

// SetConnected sets the connection status.
func (m *StatusModel) SetConnected(connected bool) {
	m.connected = connected
	m.connecting = false
	m.reconnecting = false
	if connected {
		m.error = nil
	}
}

// SetConnecting sets the connecting status.
func (m *StatusModel) SetConnecting(connecting, reconnecting bool) {
	m.connecting = connecting
	m.reconnecting = reconnecting
}

// SetSessionID sets the current session ID.
func (m *StatusModel) SetSessionID(sessionID string) {
	m.sessionID = sessionID
}

// SetError sets the current error.
func (m *StatusModel) SetError(err error) {
	m.error = err
}

// SetThinking sets the thinking state.
func (m *StatusModel) SetThinking(thinking bool) {
	m.thinking = thinking
}

// IsConnected returns the current connection status.
func (m StatusModel) IsConnected() bool {
	return m.connected
}

// IsConnecting returns whether we are currently connecting.
func (m StatusModel) IsConnecting() bool {
	return m.connecting
}

// IsReconnecting returns whether we are currently reconnecting.
func (m StatusModel) IsReconnecting() bool {
	return m.reconnecting
}

// GetSessionID returns the current session ID.
func (m StatusModel) GetSessionID() string {
	return m.sessionID
}

// GetError returns the current error.
func (m StatusModel) GetError() error {
	return m.error
}

// IsThinking returns whether the system is currently thinking.
func (m StatusModel) IsThinking() bool {
	return m.thinking
}

// Messages for updating status state.

// StatusConnectedMsg is sent when the connection is established.
type StatusConnectedMsg struct{}

// StatusDisconnectedMsg is sent when the connection is lost.
type StatusDisconnectedMsg struct{}

// StatusConnectingMsg is sent when a connection attempt is in progress.
type StatusConnectingMsg struct {
	Reconnecting bool
}

// StatusSessionMsg is sent when the session ID changes.
type StatusSessionMsg struct {
	SessionID string
}

// StatusErrorMsg is sent when an error occurs.
type StatusErrorMsg struct {
	Err error
}

// StatusThinkingMsg is sent when the thinking state changes.
type StatusThinkingMsg struct {
	Thinking bool
}

// StatusConnectedCmd returns a command that sends a StatusConnectedMsg.
func StatusConnectedCmd() tea.Cmd {
	return func() tea.Msg {
		return StatusConnectedMsg{}
	}
}

// StatusDisconnectedCmd returns a command that sends a StatusDisconnectedMsg.
func StatusDisconnectedCmd() tea.Cmd {
	return func() tea.Msg {
		return StatusDisconnectedMsg{}
	}
}

// StatusConnectingCmd returns a command that sends a StatusConnectingMsg.
func StatusConnectingCmd(reconnecting bool) tea.Cmd {
	return func() tea.Msg {
		return StatusConnectingMsg{Reconnecting: reconnecting}
	}
}

// StatusSessionCmd returns a command that sends a StatusSessionMsg.
func StatusSessionCmd(sessionID string) tea.Cmd {
	return func() tea.Msg {
		return StatusSessionMsg{SessionID: sessionID}
	}
}

// StatusErrorCmd returns a command that sends a StatusErrorMsg.
func StatusErrorCmd(err error) tea.Cmd {
	return func() tea.Msg {
		return StatusErrorMsg{Err: err}
	}
}

// StatusThinkingCmd returns a command that sends a StatusThinkingMsg.
func StatusThinkingCmd(thinking bool) tea.Cmd {
	return func() tea.Msg {
		return StatusThinkingMsg{Thinking: thinking}
	}
}
