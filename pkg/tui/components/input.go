package components

import (
	"strings"

	"github.com/billm/baaaht/orchestrator/pkg/tui/styles"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// InputModel represents the state of the text input component.
// It wraps bubbles/textinput to provide a styled input field for chat messages.
type InputModel struct {
	textInput   textinput.Model
	focused     bool
	placeholder string
	width       int
	err         error
}

// NewInputModel creates a new text input model with default values.
// Uses bubbles.NewTextInput component for input handling.
func NewInputModel() InputModel {
	ti := textinput.New()
	ti.Placeholder = "Type your message..."
	ti.Focus()
	ti.EchoMode = textinput.EchoNormal
	ti.EchoCharacter = '•'

	return InputModel{
		textInput:   ti,
		focused:     true,
		placeholder: "Type your message...",
		width:       0,
		err:         nil,
	}
}

// Init initializes the input model.
// Part of the tea.Model interface.
func (m InputModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages for the input component.
// Part of the tea.Model interface.
func (m InputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		// Update the textinput width to match the window
		if m.width > 0 {
			w := m.width - 4 // Account for padding/borders
			if w < 1 {
				w = 1
			}
			m.textInput.Width = w
		}
		return m, nil

	case tea.KeyMsg:
		// Pass all keys to textinput - special keys are handled at parent level
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd

	case InputFocusMsg:
		m.focused = true
		m.textInput.Focus()
		return m, textinput.Blink

	case InputBlurMsg:
		m.focused = false
		m.textInput.Blur()
		return m, nil

	case InputClearMsg:
		m.textInput.Reset()
		return m, nil

	case InputPlaceholderMsg:
		m.placeholder = msg.Text
		m.textInput.Placeholder = msg.Text
		return m, nil

	case InputErrorMsg:
		m.err = msg.Err
		return m, nil
	}

	// Pass all other messages to the textinput
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// View renders the input component.
// Part of the tea.Model interface.
func (m InputModel) View() string {
	if m.width <= 0 {
		return ""
	}

	// Build the input view with styling
	var sb strings.Builder

	// Add error display if present
	if m.err != nil {
		sb.WriteString(styles.Styles.InputError.Render("⚠ " + m.err.Error()))
		sb.WriteString("\n")
	}

	// Apply border if focused
	if m.focused {
		sb.WriteString(styles.Styles.AppBorder.
			Width(m.width).
			Render(m.textInput.View()))
	} else {
		sb.WriteString(m.textInput.View())
	}

	return sb.String()
}

// Focus sets the input to focused state.
func (m *InputModel) Focus() {
	m.focused = true
	m.textInput.Focus()
}

// Blur removes focus from the input.
func (m *InputModel) Blur() {
	m.focused = false
	m.textInput.Blur()
}

// SetValue sets the current text input value.
func (m *InputModel) SetValue(value string) {
	m.textInput.SetValue(value)
}

// Value returns the current text input value.
func (m InputModel) Value() string {
	return m.textInput.Value()
}

// Reset clears the input value and returns the updated model with blink command.
func (m *InputModel) Reset() tea.Cmd {
	m.textInput.Reset()
	// Return blink command to keep cursor visible
	return textinput.Blink
}

// SetPlaceholder sets the placeholder text.
func (m *InputModel) SetPlaceholder(text string) {
	m.placeholder = text
	m.textInput.Placeholder = text
}

// SetError sets the current error state.
func (m *InputModel) SetError(err error) {
	m.err = err
}

// IsFocused returns whether the input is currently focused.
func (m InputModel) IsFocused() bool {
	return m.focused
}

// CursorPosition returns the current cursor position.
// Note: This is a placeholder as the underlying textinput doesn't expose this directly.
func (m InputModel) CursorPosition() int {
	return 0
}

// SetCursorPosition sets the cursor position.
func (m *InputModel) SetCursorPosition(pos int) {
	m.textInput.SetCursor(pos)
}

// Messages for updating input state.

// InputFocusMsg is sent when the input should be focused.
type InputFocusMsg struct{}

// InputBlurMsg is sent when the input should lose focus.
type InputBlurMsg struct{}

// InputClearMsg is sent when the input should be cleared.
type InputClearMsg struct{}

// InputPlaceholderMsg is sent when the placeholder text should change.
type InputPlaceholderMsg struct {
	Text string
}

// InputErrorMsg is sent when an input error occurs.
type InputErrorMsg struct {
	Err error
}

// InputSubmitMsg is sent when the user submits the input (presses enter).
type InputSubmitMsg struct {
	Text string
}

// Commands for input state changes.

// InputFocusCmd returns a command that sends an InputFocusMsg.
func InputFocusCmd() tea.Cmd {
	return func() tea.Msg {
		return InputFocusMsg{}
	}
}

// InputBlurCmd returns a command that sends an InputBlurMsg.
func InputBlurCmd() tea.Cmd {
	return func() tea.Msg {
		return InputBlurMsg{}
	}
}

// InputClearCmd returns a command that sends an InputClearMsg.
func InputClearCmd() tea.Cmd {
	return func() tea.Msg {
		return InputClearMsg{}
	}
}

// InputPlaceholderCmd returns a command that sends an InputPlaceholderMsg.
func InputPlaceholderCmd(text string) tea.Cmd {
	return func() tea.Msg {
		return InputPlaceholderMsg{Text: text}
	}
}

// InputErrorCmd returns a command that sends an InputErrorMsg.
func InputErrorCmd(err error) tea.Cmd {
	return func() tea.Msg {
		return InputErrorMsg{Err: err}
	}
}

// InputSubmitCmd returns a command that sends an InputSubmitMsg.
func InputSubmitCmd(text string) tea.Cmd {
	return func() tea.Msg {
		return InputSubmitMsg{Text: text}
	}
}
