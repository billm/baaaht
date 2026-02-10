package tui

import (
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

	// Additional message types will be added in subsequent subtasks:
	// - gRPC connection status messages (phase-3-grpc)
	// - Session messages (phase-5-session)
	// - Streaming response messages (phase-6-streaming)
	// - Health check tick messages (phase-3-grpc)
	}

	return m, nil
}

// handleKeyMsg handles keyboard input.
func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	// Quit keys
	case "q", "ctrl+c", "ctrl+d":
		m.quitting = true
		return m, tea.Quit

	// Additional key bindings will be added in subsequent subtasks:
	// - ctrl+enter: send message (phase-6-streaming)
	// - ctrl+n: next session (phase-5-session)
	// - ctrl+p: previous session (phase-5-session)
	// - ctrl+l: list sessions (phase-5-session)
	// - esc: cancel current operation
	}

	return m, nil
}

// handleWindowSize handles terminal resize events.
func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	// Layout adjustments will be added in subtask-4-4 when components are integrated
	// For now, just store the width/height if needed
	_ = msg.Width
	_ = msg.Height
	return m, nil
}
