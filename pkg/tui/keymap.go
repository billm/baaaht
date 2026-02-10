package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// KeyMap defines keyboard shortcuts for the TUI.
// This provides a centralized location for managing keybindings,
// making it easier to display help text and customize shortcuts.
type KeyMap struct {
	// Quit keys - exit the application
	Quit tea.Key

	// Message input keys
	SendMessage tea.Key // Send the current message

	// Session navigation keys
	NextSession     tea.Key // Switch to next session
	PreviousSession tea.Key // Switch to previous session
	ListSessions    tea.Key // Show session list

	// General keys
	Cancel tea.Key // Cancel current operation or close modal

	// Chat view keys
	ScrollUp     tea.Key // Scroll chat view up
	ScrollDown   tea.Key // Scroll chat view down
	ScrollTop    tea.Key // Jump to top of chat
	ScrollBottom tea.Key // Jump to bottom of chat

	// Input keys
	TextInput tea.Key // Focus text input
}

// DefaultKeyMap returns the default keybindings for the TUI.
// These bindings follow common CLI conventions:
// - ctrl+c/ctrl+d/q for quit
// - ctrl+enter for sending messages
// - ctrl+n/ctrl+p for next/previous (like Emacs/Vim)
// - esc for cancel (standard UI convention)
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: tea.Key{
			Type:  tea.KeyCtrlC,
			Alt:   "",
			Runes: nil,
		},

		SendMessage: tea.Key{
			Type:  tea.KeyEnter,
			Alt:   "ctrl",
			Runes: nil,
		},

		NextSession: tea.Key{
			Type:  tea.KeyCtrlN,
			Alt:   "",
			Runes: nil,
		},

		PreviousSession: tea.Key{
			Type:  tea.KeyCtrlP,
			Alt:   "",
			Runes: nil,
		},

		ListSessions: tea.Key{
			Type:  tea.KeyCtrlL,
			Alt:   "",
			Runes: nil,
		},

		Cancel: tea.Key{
			Type:  tea.KeyEsc,
			Alt:   "",
			Runes: nil,
		},

		ScrollUp: tea.Key{
			Type:  tea.KeyUp,
			Alt:   "",
			Runes: nil,
		},

		ScrollDown: tea.Key{
			Type:  tea.KeyKeyDown,
			Alt:   "",
			Runes: nil,
		},

		ScrollTop: tea.Key{
			Type:  tea.KeyHome,
			Alt:   "",
			Runes: nil,
		},

		ScrollBottom: tea.Key{
			Type:  tea.KeyEnd,
			Alt:   "",
			Runes: nil,
		},

		TextInput: tea.Key{
			Type:  tea.KeyRunes,
			Alt:   "",
			Runes: []rune{'i'},
		},
	}
}

// String representations for help display.
// These constants provide human-readable key labels.

const (
	keyQuit            = "q"
	keyCtrlC           = "ctrl+c"
	keyCtrlD           = "ctrl+d"
	keyCtrlEnter       = "ctrl+enter"
	keyCtrlN           = "ctrl+n"
	keyCtrlP           = "ctrl+p"
	keyCtrlL           = "ctrl+l"
	keyEsc             = "esc"
	keyUp              = "↑"
	keyDown            = "↓"
	keyHome            = "home"
	keyEnd             = "end"
	keyI               = "i"
	keyEnter           = "enter"
)

// HelpEntry represents a single keybinding help entry.
type HelpEntry struct {
	Key   string // Display text for the key
	Desc  string // Description of what the key does
	Style lipgloss.Style // Optional style for the key
}

// HelpEntries returns a slice of help entries for the keymap.
// This is useful for displaying keyboard shortcuts in the UI.
func (k KeyMap) HelpEntries() []HelpEntry {
	return []HelpEntry{
		{Key: keyCtrlEnter, Desc: "Send message", Style: Styles.HelpKey},
		{Key: keyCtrlN, Desc: "Next session", Style: Styles.HelpKey},
		{Key: keyCtrlP, Desc: "Prev session", Style: Styles.HelpKey},
		{Key: keyCtrlL, Desc: "List sessions", Style: Styles.HelpKey},
		{Key: keyEsc, Desc: "Cancel", Style: Styles.HelpKey},
		{Key: keyQuit, Desc: "Quit", Style: Styles.HelpKey},
	}
}

// ShortHelp returns essential help entries for the footer.
func (k KeyMap) ShortHelp() []HelpEntry {
	return []HelpEntry{
		{Key: keyCtrlEnter, Desc: "Send", Style: Styles.HelpKey},
		{Key: keyCtrlL, Desc: "Sessions", Style: Styles.HelpKey},
		{Key: keyQuit, Desc: "Quit", Style: Styles.HelpKey},
	}
}

// FullHelp returns all help entries including navigation shortcuts.
func (k KeyMap) FullHelp() []HelpEntry {
	return append(k.HelpEntries(),
		HelpEntry{Key: keyUp, Desc: "Scroll up", Style: Styles.HelpKey},
		HelpEntry{Key: keyDown, Desc: "Scroll down", Style: Styles.HelpKey},
		HelpEntry{Key: keyHome, Desc: "Top", Style: Styles.HelpKey},
		HelpEntry{Key: keyEnd, Desc: "Bottom", Style: Styles.HelpKey},
	)
}

// IsQuitKey checks if the given key matches any quit keybinding.
func (k KeyMap) IsQuitKey(msg tea.KeyMsg) bool {
	s := msg.String()
	return s == keyQuit || s == keyCtrlC || s == keyCtrlD
}

// IsSendKey checks if the given key is the send message keybinding.
func (k KeyMap) IsSendKey(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyEnter && msg.Alt == "ctrl"
}

// IsCancelKey checks if the given key is the cancel keybinding.
func (k KeyMap) IsCancelKey(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyEsc
}

// IsNextSessionKey checks if the given key is the next session keybinding.
func (k KeyMap) IsNextSessionKey(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyCtrlN
}

// IsPreviousSessionKey checks if the given key is the previous session keybinding.
func (k KeyMap) IsPreviousSessionKey(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyCtrlP
}

// IsListSessionsKey checks if the given key is the list sessions keybinding.
func (k KeyMap) IsListSessionsKey(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyCtrlL
}
