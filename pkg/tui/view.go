package tui

import (
	"strings"

	"github.com/billm/baaaht/orchestrator/pkg/tui/components"
	"github.com/billm/baaaht/orchestrator/pkg/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// View renders the model state as a string for display.
// Part of the tea.Model interface.
func (m Model) View() string {
	// Handle error state
	if m.err != nil {
		return m.errorView()
	}

	// Build the main view
	var b strings.Builder

	// Header
	b.WriteString(m.headerView())
	b.WriteString("\n")

	// Status bar
	b.WriteString(m.status.View())
	b.WriteString("\n")

	// Chat viewport (main content area)
	b.WriteString(m.chatView())
	b.WriteString("\n")

	// Input field
	b.WriteString(m.input.View())
	b.WriteString("\n")

	// Footer with help text
	b.WriteString(m.footerView())

	return b.String()
}

// headerView renders the header section.
func (m Model) headerView() string {
	title := styles.Styles.HeaderText.Render("Baaaht TUI")
	version := styles.Styles.HeaderVersion.Render("v0.1.0")

	return lipgloss.JoinHorizontal(lipgloss.Top, title, " ", version)
}

// chatView renders the chat viewport component.
func (m Model) chatView() string {
	// Calculate chat height (total height - header - status - input - footer)
	chatHeight := m.height - 4
	if chatHeight < 1 {
		chatHeight = 1
	}

	// Update chat component with the new size
	chatMsg := tea.WindowSizeMsg{Width: m.width, Height: chatHeight}
	cm, _ := m.chat.Update(chatMsg)
	m.chat = cm.(components.ChatModel)

	return m.chat.View()
}

// sessionsView renders the sessions list component.
func (m Model) sessionsView() string {
	// Calculate sessions height (total height - header - status - input - footer)
	sessionsHeight := m.height - 4
	if sessionsHeight < 1 {
		sessionsHeight = 1
	}

	// Update sessions component with the new size
	sessionsMsg := tea.WindowSizeMsg{Width: m.width, Height: sessionsHeight}
	sessm, _ := m.sessions.Update(sessionsMsg)
	m.sessions = sessm.(components.SessionsModel)

	return m.sessions.View()
}

// footerView renders the footer with help text.
func (m Model) footerView() string {
	keymap := DefaultKeyMap()
	entries := keymap.ShortHelp()

	var parts []string
	for _, entry := range entries {
		key := entry.Key
		desc := entry.Desc
		parts = append(parts, entry.Style.Render(key+" "+desc))
	}

	helpText := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	return styles.Styles.FooterText.Width(m.width).Render(helpText)
}

// errorView renders the error state.
func (m Model) errorView() string {
	title := styles.Styles.ErrorTitle.Render("Error")
	message := styles.Styles.ErrorText.Render(m.err.Error())

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", message)
	return styles.Styles.ErrorBorder.Width(m.width).Render(content)
}
