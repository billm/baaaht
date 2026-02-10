package tui

import (
	"strings"

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
	// This will be expanded in subsequent subtasks to include:
	// - Status bar (subtask-4-1)
	// - Chat viewport (subtask-4-3)
	// - Input field (subtask-4-2)
	// - Session list overlay (subtask-5-2)

	var b strings.Builder

	// Header
	b.WriteString(m.headerView())
	b.WriteString("\n")

	// Main content area
	b.WriteString(m.contentView())
	b.WriteString("\n")

	// Footer
	b.WriteString(m.footerView())

	return b.String()
}

// headerView renders the header section.
func (m Model) headerView() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")). // Purple
		Render("Baaaht TUI v0.1.0")

	return title
}

// contentView renders the main content area.
// Will be expanded in subtask-4-4 to include chat viewport.
func (m Model) contentView() string {
	// Placeholder content until components are integrated
	return lipgloss.NewStyle().
		Faint(true).
		Render("Initializing TUI components...")
}

// footerView renders the footer with help text.
func (m Model) footerView() string {
	helpText := "Press 'q', Ctrl+C, or Ctrl+D to quit"

	return lipgloss.NewStyle().
		Faint(true).
		Render(helpText)
}

// errorView renders the error state.
func (m Model) errorView() string {
	errorStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")). // Red
		Render("Error: " + m.err.Error())

	return errorStyle
}
