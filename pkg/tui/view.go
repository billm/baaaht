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

	// If dimensions aren't set yet, return a placeholder
	if m.width <= 0 || m.height <= 0 {
		return "Initializing..."
	}

	// Build the main view
	var b strings.Builder

	// Header (no border, just text)
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

	mainView := b.String()

	// If sessions list is visible, render it as an overlay
	if m.sessions.IsVisible() {
		return m.renderSessionsOverlay(mainView)
	}

	return mainView
}

// headerView renders the header section with full width.
func (m Model) headerView() string {
	title := styles.Styles.HeaderText.Render("Baaaht TUI")
	version := styles.Styles.HeaderVersion.Render("v0.1.0")

	// Join the title and version
	header := lipgloss.JoinHorizontal(lipgloss.Top, title, " ", version)

	// Ensure the header fills the full width (no border on header, so width - 1)
	width := m.width - 1
	if width < 1 {
		width = 1
	}
	return lipgloss.NewStyle().Width(width).Render(header)
}

// chatView renders the chat viewport component.
func (m Model) chatView() string {
	// Calculate chat height accounting for:
	// - header (1 line) + newline (1)
	// - status with border (3 lines) + newline (1)
	// - input with border (3 lines) + newline (1)
	// - footer (1 line)
	// Total overhead: 11 lines
	chatHeight := m.height - 11
	if chatHeight < 5 {
		chatHeight = 5
	}

	// Update chat component with the new size (width - 2 for left+right borders)
	width := m.width - 2
	if width < 1 {
		width = 1
	}
	chatMsg := tea.WindowSizeMsg{Width: width, Height: chatHeight}
	cm, _ := m.chat.Update(chatMsg)
	m.chat = cm.(components.ChatModel)

	return m.chat.View()
}

// sessionsView renders the sessions list component.
func (m Model) sessionsView() string {
	// Calculate sessions height (same as chat)
	sessionsHeight := m.height - 11
	if sessionsHeight < 5 {
		sessionsHeight = 5
	}

	// Update sessions component with the new size (width - 2 for left+right borders)
	width := m.width - 2
	if width < 1 {
		width = 1
	}
	sessionsMsg := tea.WindowSizeMsg{Width: width, Height: sessionsHeight}
	sessm, _ := m.sessions.Update(sessionsMsg)
	m.sessions = sessm.(components.SessionsModel)

	return m.sessions.View()
}

// footerView renders the footer with help text and dimensions.
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
	// Footer has no border, so width - 1
	width := m.width - 1
	if width < 1 {
		width = 1
	}
	return styles.Styles.FooterText.Width(width).Render(helpText)
}

// errorView renders the error state.
func (m Model) errorView() string {
	title := styles.Styles.ErrorTitle.Render("Error")
	message := styles.Styles.ErrorText.Render(m.err.Error())

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", message)
	return styles.Styles.ErrorBorder.Width(m.width).Render(content)
}

// renderSessionsOverlay renders the sessions list as a modal overlay.
func (m Model) renderSessionsOverlay(mainView string) string {
	// Calculate modal dimensions (smaller than full screen for modal effect)
	// Leave room for borders and padding, and ensure it fits within terminal
	margin := 10
	maxModalWidth := m.width - margin*2
	maxModalHeight := m.height - margin*2

	// Set minimum and maximum dimensions
	modalWidth := maxModalWidth
	if modalWidth > 80 {
		modalWidth = 80
	}
	if modalWidth < 50 {
		modalWidth = 50
	}

	modalHeight := maxModalHeight
	if modalHeight > 25 {
		modalHeight = 25
	}
	if modalHeight < 15 {
		modalHeight = 15
	}

	// Ensure modal doesn't exceed terminal bounds
	if modalWidth > m.width-4 {
		modalWidth = m.width - 4
	}
	if modalHeight > m.height-4 {
		modalHeight = m.height - 4
	}

	// Update sessions component with modal dimensions (minus border/padding)
	innerWidth := modalWidth - 4   // Account for border
	innerHeight := modalHeight - 2 // Account for border
	if innerWidth < 10 {
		innerWidth = 10
	}
	if innerHeight < 5 {
		innerHeight = 5
	}

	sessionsMsg := tea.WindowSizeMsg{Width: innerWidth, Height: innerHeight}
	sessm, _ := m.sessions.Update(sessionsMsg)
	m.sessions = sessm.(components.SessionsModel)

	// Get the sessions list content
	sessionsContent := m.sessions.View()

	// Create the bordered modal content
	borderedContent := styles.Styles.SessionBorder.
		Width(modalWidth).
		Height(modalHeight).
		Render(sessionsContent)

	// Add instructions at the bottom (outside the modal border)
	instructions := styles.Styles.Muted.Render("Press ctrl+l or esc to close")
	modalWithInstructions := lipgloss.JoinVertical(
		lipgloss.Center,
		borderedContent,
		"",
		instructions,
	)

	// Use lipgloss.Place to center the modal on the full screen
	// This ensures the modal stays within the terminal bounds
	modal := lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		modalWithInstructions,
	)

	return modal
}
