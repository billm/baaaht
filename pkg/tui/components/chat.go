package components

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/billm/baaaht/orchestrator/pkg/tui"
)

// MessageType represents the type of message in the chat.
type MessageType string

const (
	MessageTypeUser      MessageType = "user"
	MessageTypeAssistant MessageType = "assistant"
	MessageTypeSystem    MessageType = "system"
	MessageTypeError     MessageType = "error"
)

// Message represents a single chat message.
type Message struct {
	Type    MessageType
	Content string
}

// ChatModel represents the state of the chat display component.
// It uses a viewport for scrolling through chat messages.
type ChatModel struct {
	viewport  viewport.Model
	messages  []Message
	width     int
	height    int
	autoscroll bool
}

// NewChatModel creates a new chat model with default values.
// Uses bubbles/viewport component for scrolling.
func NewChatModel() ChatModel {
	vp := viewport.New(0, 0)

	return ChatModel{
		viewport:   vp,
		messages:   []Message{},
		width:      0,
		height:     0,
		autoscroll: true,
	}
}

// Init initializes the chat model.
// Part of the tea.Model interface.
func (m ChatModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the chat component.
// Part of the tea.Model interface.
func (m ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = m.width
		m.viewport.Height = m.height
		// Re-render content with new dimensions
		m.viewport.SetContent(m.renderMessages())
		return m, nil

	case ChatAppendMsg:
		m.messages = append(m.messages, Message{
			Type:    msg.Type,
			Content: msg.Content,
		})
		m.viewport.SetContent(m.renderMessages())
		// Auto-scroll to bottom when autoscroll is enabled
		if m.autoscroll {
			m.viewport.GotoBottom()
		}
		return m, nil

	case ChatClearMsg:
		m.messages = []Message{}
		m.viewport.SetContent("")
		return m, nil

	case ChatScrollTopMsg:
		m.viewport.GotoTop()
		return m, nil

	case ChatScrollBottomMsg:
		m.viewport.GotoBottom()
		return m, nil

	case ChatScrollLineUpMsg:
		m.viewport.LineUp(1)
		return m, nil

	case ChatScrollLineDownMsg:
		m.viewport.LineDown(1)
		return m, nil

	case ChatScrollPageUpMsg:
		m.viewport.HalfViewUp()
		return m, nil

	case ChatScrollPageDownMsg:
		m.viewport.HalfViewDown()
		return m, nil

	case ChatSetAutoscrollMsg:
		m.autoscroll = msg.Autoscroll
		return m, nil
	}

	// Pass all other messages to the viewport
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the chat component.
// Part of the tea.Model interface.
func (m ChatModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	// If no messages, show placeholder
	if len(m.messages) == 0 {
		placeholder := tui.Styles.Muted.Render("No messages yet. Start a conversation!")
		return tui.Styles.ChatBorder.
			Width(m.width).
			Height(m.height).
			Render(placeholder)
	}

	// Render the viewport with border
	content := m.viewport.View()
	return tui.Styles.ChatBorder.
		Width(m.width).
		Height(m.height).
		Render(content)
}

// renderMessages renders all messages into a single string for the viewport.
func (m ChatModel) renderMessages() string {
	if len(m.messages) == 0 {
		return ""
	}

	var lines []string
	for _, msg := range m.messages {
		lines = append(lines, m.renderMessage(msg))
		lines = append(lines, "") // Add blank line between messages
	}

	return strings.Join(lines, "\n")
}

// renderMessage renders a single message with appropriate styling.
func (m ChatModel) renderMessage(msg Message) string {
	// Get the appropriate style for this message type
	style := tui.MessageTypeStyle(string(msg.Type))

	// Create the message header
	var prefix string
	switch msg.Type {
	case MessageTypeUser:
		prefix = "You:"
	case MessageTypeAssistant:
		prefix = "Assistant:"
	case MessageTypeSystem:
		prefix = "System:"
	case MessageTypeError:
		prefix = "Error:"
	}

	// Render the message content with word wrapping
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return ""
	}

	// Build the final message
	header := style.Render(prefix)
	body := style.Render(content)

	// Handle code blocks in the message
	body = m.formatCodeBlocks(body)

	return header + "\n" + body
}

// formatCodeBlocks formats code blocks within a message.
// This is a placeholder for more sophisticated syntax highlighting
// which will be implemented in subtask-6-3.
func (m ChatModel) formatCodeBlocks(content string) string {
	// For now, just return the content as-is
	// Syntax highlighting will be added in phase 6
	return content
}

// AppendMessage adds a new message to the chat.
func (m *ChatModel) AppendMessage(msgType MessageType, content string) {
	m.messages = append(m.messages, Message{
		Type:    msgType,
		Content: content,
	})
	m.viewport.SetContent(m.renderMessages())
	if m.autoscroll {
		m.viewport.GotoBottom()
	}
}

// Clear removes all messages from the chat.
func (m *ChatModel) Clear() {
	m.messages = []Message{}
	m.viewport.SetContent("")
}

// Messages returns the current list of messages.
func (m ChatModel) Messages() []Message {
	return m.messages
}

// SetAutoscroll sets whether the chat should auto-scroll to new messages.
func (m *ChatModel) SetAutoscroll(autoscroll bool) {
	m.autoscroll = autoscroll
}

// IsAutoscroll returns whether auto-scroll is enabled.
func (m ChatModel) IsAutoscroll() bool {
	return m.autoscroll
}

// ScrollToTop scrolls the chat to the top.
func (m *ChatModel) ScrollToTop() {
	m.viewport.GotoTop()
}

// ScrollToBottom scrolls the chat to the bottom.
func (m *ChatModel) ScrollToBottom() {
	m.viewport.GotoBottom()
}

// ScrollLineUp scrolls up by one line.
func (m *ChatModel) ScrollLineUp() {
	m.viewport.LineUp(1)
}

// ScrollLineDown scrolls down by one line.
func (m *ChatModel) ScrollLineDown() {
	m.viewport.LineDown(1)
}

// ScrollPageUp scrolls up by half a page.
func (m *ChatModel) ScrollPageUp() {
	m.viewport.HalfViewUp()
}

// ScrollPageDown scrolls down by half a page.
func (m *ChatModel) ScrollPageDown() {
	m.viewport.HalfViewDown()
}

// Messages for updating chat state.

// ChatAppendMsg is sent when a new message should be appended to the chat.
type ChatAppendMsg struct {
	Type    MessageType
	Content string
}

// ChatClearMsg is sent when the chat should be cleared.
type ChatClearMsg struct{}

// ChatScrollTopMsg is sent when the chat should scroll to the top.
type ChatScrollTopMsg struct{}

// ChatScrollBottomMsg is sent when the chat should scroll to the bottom.
type ChatScrollBottomMsg struct{}

// ChatScrollLineUpMsg is sent when the chat should scroll up one line.
type ChatScrollLineUpMsg struct{}

// ChatScrollLineDownMsg is sent when the chat should scroll down one line.
type ChatScrollLineDownMsg struct{}

// ChatScrollPageUpMsg is sent when the chat should scroll up half a page.
type ChatScrollPageUpMsg struct{}

// ChatScrollPageDownMsg is sent when the chat should scroll down half a page.
type ChatScrollPageDownMsg struct{}

// ChatSetAutoscrollMsg is sent when auto-scroll behavior should change.
type ChatSetAutoscrollMsg struct {
	Autoscroll bool
}

// Commands for chat state changes.

// ChatAppendCmd returns a command that sends a ChatAppendMsg.
func ChatAppendCmd(msgType MessageType, content string) tea.Cmd {
	return func() tea.Msg {
		return ChatAppendMsg{Type: msgType, Content: content}
	}
}

// ChatClearCmd returns a command that sends a ChatClearMsg.
func ChatClearCmd() tea.Cmd {
	return func() tea.Msg {
		return ChatClearMsg{}
	}
}

// ChatScrollTopCmd returns a command that sends a ChatScrollTopMsg.
func ChatScrollTopCmd() tea.Cmd {
	return func() tea.Msg {
		return ChatScrollTopMsg{}
	}
}

// ChatScrollBottomCmd returns a command that sends a ChatScrollBottomMsg.
func ChatScrollBottomCmd() tea.Cmd {
	return func() tea.Msg {
		return ChatScrollBottomMsg{}
	}
}

// ChatScrollLineUpCmd returns a command that sends a ChatScrollLineUpMsg.
func ChatScrollLineUpCmd() tea.Cmd {
	return func() tea.Msg {
		return ChatScrollLineUpMsg{}
	}
}

// ChatScrollLineDownCmd returns a command that sends a ChatScrollLineDownMsg.
func ChatScrollLineDownCmd() tea.Cmd {
	return func() tea.Msg {
		return ChatScrollLineDownMsg{}
	}
}

// ChatScrollPageUpCmd returns a command that sends a ChatScrollPageUpMsg.
func ChatScrollPageUpCmd() tea.Cmd {
	return func() tea.Msg {
		return ChatScrollPageUpMsg{}
	}
}

// ChatScrollPageDownCmd returns a command that sends a ChatScrollPageDownMsg.
func ChatScrollPageDownCmd() tea.Cmd {
	return func() tea.Msg {
		return ChatScrollPageDownMsg{}
	}
}

// ChatSetAutoscrollCmd returns a command that sends a ChatSetAutoscrollMsg.
func ChatSetAutoscrollCmd(autoscroll bool) tea.Cmd {
	return func() tea.Msg {
		return ChatSetAutoscrollMsg{Autoscroll: autoscroll}
	}
}
