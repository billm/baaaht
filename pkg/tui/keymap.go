package tui

import (
	"github.com/billm/baaaht/orchestrator/pkg/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// KeyMap defines keyboard shortcuts for the TUI.
// This provides a centralized location for managing keybindings,
// making it easier to display help text and customize shortcuts.
type KeyMap struct {
	// Keys are stored as string representations for easy comparison
	Quit            string
	SendMessage     string
	NextSession     string
	PreviousSession string
	ListSessions    string
	Cancel          string
	ScrollUp        string
	ScrollDown      string
	ScrollTop       string
	ScrollBottom    string
	TextInput       string
	RetryConnection string
}

// DefaultKeyMap returns the default keybindings for the TUI.
// These bindings follow common CLI conventions:
// - ctrl+c/ctrl+x for quit (ctrl+x is safer than 'q' which can be typed in text)
// - enter for sending messages (shift+enter is preferred, but doesn't seem to work in ghostty, so using plain enter for now)
// - ctrl+n/ctrl+p for next/previous (like Emacs/Vim)
// - ctrl+l for listing sessions (shows overlay modal)
// - esc for cancel (standard UI convention)
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit:            "ctrl+x",
		SendMessage:     "enter",
		NextSession:     "ctrl+n",
		PreviousSession: "ctrl+p",
		ListSessions:    "ctrl+l",
		Cancel:          "esc",
		ScrollUp:        "up",
		ScrollDown:      "down",
		ScrollTop:       "home",
		ScrollBottom:    "end",
		TextInput:       "i",
		RetryConnection: "ctrl+r",
	}
}

// HelpEntry represents a single keybinding help entry.
type HelpEntry struct {
	Key   string         // Display text for the key
	Desc  string         // Description of what the key does
	Style lipgloss.Style // Optional style for the key
}

// HelpEntries returns a slice of help entries for the keymap.
// This is useful for displaying keyboard shortcuts in the UI.
func (k KeyMap) HelpEntries() []HelpEntry {
	return []HelpEntry{
		{Key: k.SendMessage, Desc: "Send message", Style: styles.Styles.HelpKey},
		{Key: k.NextSession, Desc: "Next session", Style: styles.Styles.HelpKey},
		{Key: k.PreviousSession, Desc: "Prev session", Style: styles.Styles.HelpKey},
		{Key: k.ListSessions, Desc: "List sessions", Style: styles.Styles.HelpKey},
		{Key: k.Cancel, Desc: "Cancel", Style: styles.Styles.HelpKey},
		{Key: k.Quit, Desc: "Quit", Style: styles.Styles.HelpKey},
	}
}

// ShortHelp returns essential help entries for the footer.
func (k KeyMap) ShortHelp() []HelpEntry {
	return []HelpEntry{
		{Key: "enter", Desc: "Send", Style: styles.Styles.HelpKey},
		{Key: k.RetryConnection, Desc: "Retry", Style: styles.Styles.HelpKey},
		{Key: k.ListSessions, Desc: "Sessions", Style: styles.Styles.HelpKey},
		{Key: k.Quit, Desc: "Quit", Style: styles.Styles.HelpKey},
	}
}

// FullHelp returns all help entries including navigation shortcuts.
func (k KeyMap) FullHelp() []HelpEntry {
	return append(k.HelpEntries(),
		HelpEntry{Key: k.ScrollUp, Desc: "Scroll up", Style: styles.Styles.HelpKey},
		HelpEntry{Key: k.ScrollDown, Desc: "Scroll down", Style: styles.Styles.HelpKey},
		HelpEntry{Key: k.ScrollTop, Desc: "Top", Style: styles.Styles.HelpKey},
		HelpEntry{Key: k.ScrollBottom, Desc: "Bottom", Style: styles.Styles.HelpKey},
	)
}

// IsQuitKey checks if the given key matches any quit keybinding.
func (k KeyMap) IsQuitKey(msg tea.KeyMsg) bool {
	s := msg.String()
	return s == "ctrl+c" || s == "ctrl+x"
}

// IsSendKey checks if the given key is the send message keybinding.
func (k KeyMap) IsSendKey(msg tea.KeyMsg) bool {
	return msg.String() == "enter"
}

// IsCancelKey checks if the given key is the cancel keybinding.
func (k KeyMap) IsCancelKey(msg tea.KeyMsg) bool {
	s := msg.String()
	return s == "esc"
}

// IsNextSessionKey checks if the given key is the next session keybinding.
func (k KeyMap) IsNextSessionKey(msg tea.KeyMsg) bool {
	s := msg.String()
	return s == "ctrl+n"
}

// IsPreviousSessionKey checks if the given key is the previous session keybinding.
func (k KeyMap) IsPreviousSessionKey(msg tea.KeyMsg) bool {
	s := msg.String()
	return s == "ctrl+p"
}

// IsListSessionsKey checks if the given key is the list sessions keybinding.
func (k KeyMap) IsListSessionsKey(msg tea.KeyMsg) bool {
	s := msg.String()
	return s == "ctrl+l"
}

// IsRetryConnectionKey checks if the given key is the retry connection keybinding.
func (k KeyMap) IsRetryConnectionKey(msg tea.KeyMsg) bool {
	s := msg.String()
	return s == "ctrl+r"
}
