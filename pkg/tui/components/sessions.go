package components

import (
	"strings"

	"github.com/billm/baaaht/orchestrator/pkg/tui/styles"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SessionItem represents a single session in the list.
// It implements the list.Item interface from bubbles/list.
type SessionItem struct {
	SessionID string
	Name      string
	CreatedAt string
	IsActive  bool
}

// Title returns the display title for the session item.
func (s SessionItem) Title() string {
	if s.Name != "" {
		return s.Name
	}
	return s.shortSessionID()
}

// Description returns the description for the session item.
func (s SessionItem) Description() string {
	var parts []string
	if s.CreatedAt != "" {
		parts = append(parts, "Created: "+s.CreatedAt)
	}
	if s.IsActive {
		parts = append(parts, "Active")
	}
	return strings.Join(parts, " • ")
}

// shortSessionID returns a shortened version of the session ID.
func (s SessionItem) shortSessionID() string {
	if len(s.SessionID) > 8 {
		return s.SessionID[:8] + "..."
	}
	return s.SessionID
}

// FilterValue returns the value to use for filtering.
func (s SessionItem) FilterValue() string {
	return s.SessionID + " " + s.Name
}

// SessionsModel represents the state of the sessions list component.
// It uses bubbles/list for displaying and selecting sessions.
type SessionsModel struct {
	list     list.Model
	visible  bool
	width    int
	height   int
	sessions []SessionItem
}

// NewSessionsModel creates a new sessions list model with default values.
// Uses bubbles/list component for the list interface.
func NewSessionsModel() SessionsModel {
	// Create the list delegate with custom styling
	delegate := list.NewDefaultDelegate()

	// Customize the selected and normal styles
	delegate.Styles.SelectedTitle = styles.Styles.SessionSelected
	delegate.Styles.SelectedDesc = styles.Styles.SessionSelected.Copy().Foreground(lipgloss.Color("251"))
	delegate.Styles.NormalTitle = styles.Styles.SessionNormal
	delegate.Styles.NormalDesc = styles.Styles.SessionNormal.Copy().Faint(true)

	// Set spacing between items
	delegate.ShowDescription = true
	delegate.SetSpacing(1)

	// Create the list with default items
	l := list.New([]list.Item{}, delegate, 0, 0)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Title = "Sessions"
	l.Styles.Title = styles.Styles.HeaderText
	l.Styles.PaginationStyle = styles.Styles.Muted
	l.Styles.HelpStyle = styles.Styles.Muted

	return SessionsModel{
		list:     l,
		visible:  false,
		width:    0,
		height:   0,
		sessions: []SessionItem{},
	}
}

// Init initializes the sessions model.
// Part of the tea.Model interface.
func (m SessionsModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the sessions component.
// Part of the tea.Model interface.
func (m SessionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Update the list dimensions
		if m.width > 0 && m.height > 0 {
			m.list.SetWidth(m.width)
			m.list.SetHeight(m.height)
		}
		return m, nil

	case SessionsShowMsg:
		m.visible = true
		return m, nil

	case SessionsHideMsg:
		m.visible = false
		return m, nil

	case SessionsSetItemsMsg:
		m.sessions = msg.Items
		// Convert SessionItem slice to list.Item slice
		items := make([]list.Item, len(msg.Items))
		for i, item := range msg.Items {
			items[i] = item
		}
		m.list.SetItems(items)
		return m, nil

	case SessionsSelectNextMsg:
		if m.visible {
			m.list.CursorDown()
		}
		return m, nil

	case SessionsSelectPrevMsg:
		if m.visible {
			m.list.CursorUp()
		}
		return m, nil

	case SessionsSelectFirstMsg:
		if m.visible {
			m.list.Select(0)
		}
		return m, nil

	case SessionsSelectLastMsg:
		if m.visible {
			itemCount := m.list.Width()
			if itemCount > 0 {
				m.list.Select(itemCount - 1)
			}
		}
		return m, nil
	}

	// Pass all other messages to the list if visible
	if m.visible {
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View renders the sessions component.
// Part of the tea.Model interface.
func (m SessionsModel) View() string {
	if !m.visible || m.width <= 0 || m.height <= 0 {
		return ""
	}

	// If no sessions, show placeholder
	if len(m.sessions) == 0 {
		placeholder := styles.Styles.Muted.Render("No sessions available.")
		return styles.Styles.SessionBorder.
			Width(m.width).
			Height(m.height).
			Render(placeholder)
	}

	// Render the list with border
	content := m.list.View()
	return styles.Styles.SessionBorder.
		Width(m.width).
		Height(m.height).
		Render(content)
}

// Show sets the sessions list to visible state.
func (m *SessionsModel) Show() {
	m.visible = true
}

// Hide sets the sessions list to hidden state.
func (m *SessionsModel) Hide() {
	m.visible = false
}

// IsVisible returns whether the sessions list is currently visible.
func (m SessionsModel) IsVisible() bool {
	return m.visible
}

// SetItems sets the current list of sessions.
func (m *SessionsModel) SetItems(items []SessionItem) {
	m.sessions = items
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}
	m.list.SetItems(listItems)
}

// Items returns the current list of sessions.
func (m SessionsModel) Items() []SessionItem {
	return m.sessions
}

// SelectedItem returns the currently selected session item.
func (m SessionsModel) SelectedItem() (SessionItem, bool) {
	if !m.visible || len(m.sessions) == 0 {
		return SessionItem{}, false
	}

	selected := m.list.SelectedItem()
	if selected == nil {
		return SessionItem{}, false
	}

	sessionItem, ok := selected.(SessionItem)
	if !ok {
		return SessionItem{}, false
	}

	return sessionItem, true
}

// SelectedSessionID returns the session ID of the currently selected item.
func (m SessionsModel) SelectedSessionID() (string, bool) {
	item, ok := m.SelectedItem()
	if !ok {
		return "", false
	}
	return item.SessionID, true
}

// CursorDown moves the selection down one item.
func (m *SessionsModel) CursorDown() {
	if m.visible {
		m.list.CursorDown()
	}
}

// CursorUp moves the selection up one item.
func (m *SessionsModel) CursorUp() {
	if m.visible {
		m.list.CursorUp()
	}
}

// Select selects the session at the given index.
func (m *SessionsModel) Select(index int) {
	if m.visible && index >= 0 && index < len(m.sessions) {
		m.list.Select(index)
	}
}

// Messages for updating sessions state.

// SessionsShowMsg is sent when the sessions list should be shown.
type SessionsShowMsg struct{}

// SessionsHideMsg is sent when the sessions list should be hidden.
type SessionsHideMsg struct{}

// SessionsSetItemsMsg is sent when the list of sessions should be updated.
type SessionsSetItemsMsg struct {
	Items []SessionItem
}

// SessionsSelectNextMsg is sent when the selection should move to the next item.
type SessionsSelectNextMsg struct{}

// SessionsSelectPrevMsg is sent when the selection should move to the previous item.
type SessionsSelectPrevMsg struct{}

// SessionsSelectFirstMsg is sent when the selection should move to the first item.
type SessionsSelectFirstMsg struct{}

// SessionsSelectLastMsg is sent when the selection should move to the last item.
type SessionsSelectLastMsg struct{}

// SessionsSelectedMsg is sent when a session is selected (via enter key).
type SessionsSelectedMsg struct {
	SessionID string
	Name      string
}

// Commands for sessions state changes.

// SessionsShowCmd returns a command that sends a SessionsShowMsg.
func SessionsShowCmd() tea.Cmd {
	return func() tea.Msg {
		return SessionsShowMsg{}
	}
}

// SessionsHideCmd returns a command that sends a SessionsHideMsg.
func SessionsHideCmd() tea.Cmd {
	return func() tea.Msg {
		return SessionsHideMsg{}
	}
}

// SessionsSetItemsCmd returns a command that sends a SessionsSetItemsMsg.
func SessionsSetItemsCmd(items []SessionItem) tea.Cmd {
	return func() tea.Msg {
		return SessionsSetItemsMsg{Items: items}
	}
}

// SessionsSelectNextCmd returns a command that sends a SessionsSelectNextMsg.
func SessionsSelectNextCmd() tea.Cmd {
	return func() tea.Msg {
		return SessionsSelectNextMsg{}
	}
}

// SessionsSelectPrevCmd returns a command that sends a SessionsSelectPrevMsg.
func SessionsSelectPrevCmd() tea.Cmd {
	return func() tea.Msg {
		return SessionsSelectPrevMsg{}
	}
}

// SessionsSelectFirstCmd returns a command that sends a SessionsSelectFirstMsg.
func SessionsSelectFirstCmd() tea.Cmd {
	return func() tea.Msg {
		return SessionsSelectFirstMsg{}
	}
}

// SessionsSelectLastCmd returns a command that sends a SessionsSelectLastMsg.
func SessionsSelectLastCmd() tea.Cmd {
	return func() tea.Msg {
		return SessionsSelectLastMsg{}
	}
}

// SessionsSelectedCmd returns a command that sends a SessionsSelectedMsg.
func SessionsSelectedCmd(sessionID, name string) tea.Cmd {
	return func() tea.Msg {
		return SessionsSelectedMsg{SessionID: sessionID, Name: name}
	}
}
