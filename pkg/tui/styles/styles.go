package styles

import (
	"github.com/charmbracelet/lipgloss"
)

// Styles contains all Lipgloss style definitions for the TUI.
// These styles provide consistent visual theming across all UI components.
var Styles = &styleDefs{
	// General styles
	Title:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")), // Purple
	Subtitle: lipgloss.NewStyle().Foreground(lipgloss.Color("243")),           // Gray
	Normal:   lipgloss.NewStyle().Foreground(lipgloss.Color("252")),           // White-ish
	Faint:    lipgloss.NewStyle().Faint(true),
	Muted:    lipgloss.NewStyle().Foreground(lipgloss.Color("241")), // Dark gray

	// Status indicator styles
	StatusConnected:    lipgloss.NewStyle().Foreground(lipgloss.Color("86")),              // Green-purple
	StatusDisconnected: lipgloss.NewStyle().Foreground(lipgloss.Color("203")),             // Red-pink
	StatusError:        lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),  // Bright red
	StatusWarning:      lipgloss.NewStyle().Foreground(lipgloss.Color("226")),             // Yellow
	StatusInfo:         lipgloss.NewStyle().Foreground(lipgloss.Color("39")),              // Cyan
	StatusThinking:     lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Faint(true), // Pink

	// Message styles
	MessageUser:      lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true),   // Gold
	MessageAssistant: lipgloss.NewStyle().Foreground(lipgloss.Color("86")),               // Purple
	MessageSystem:    lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Italic(true), // Gray
	MessageError:     lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),   // Red
	MessageCode:      lipgloss.NewStyle().Foreground(lipgloss.Color("117")),              // Light blue

	// Code block styles
	CodeBlock: lipgloss.NewStyle().
		Foreground(lipgloss.Color("231")).
		Background(lipgloss.Color("235")).
		Padding(1),

	CodeBorder: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("59")),

	// Input styles
	InputPlaceholder: lipgloss.NewStyle().Faint(true),
	InputCursor:      lipgloss.NewStyle().Foreground(lipgloss.Color("86")),
	InputText:        lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
	InputError:       lipgloss.NewStyle().Foreground(lipgloss.Color("196")),

	// Component border styles
	AppBorder: lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("238")),

	ChatBorder: lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("241")),

	StatusBorder: lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("59")),

	SessionBorder: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("96")),

	ModalBackground: lipgloss.NewStyle().
		Background(lipgloss.Color("235")).
		Padding(1),

	SessionSelected: lipgloss.NewStyle().
		Foreground(lipgloss.Color("231")).
		Background(lipgloss.Color("62")).
		Bold(true),

	SessionNormal: lipgloss.NewStyle().
		Foreground(lipgloss.Color("244")),

	// Help/Key hint styles
	HelpKey: lipgloss.NewStyle().
		Foreground(lipgloss.Color("228")).
		Background(lipgloss.Color("236")).
		Padding(0, 1),

	HelpDesc: lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")),

	HelpSeparator: lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")),

	// Footer styles
	FooterText: lipgloss.NewStyle().Faint(true),
	FooterError: lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true),

	// Header styles
	HeaderText: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")),

	HeaderVersion: lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")),

	// Error styles
	ErrorTitle: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")),

	ErrorBorder: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")),

	ErrorText: lipgloss.NewStyle().
		Foreground(lipgloss.Color("203")),

	// Scroll indicator styles
	ScrollIndicator: lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Faint(true),
}

// styleDefs defines all style variables used throughout the TUI.
type styleDefs struct {
	// General text styles
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	Normal   lipgloss.Style
	Faint    lipgloss.Style
	Muted    lipgloss.Style

	// Status indicator styles
	StatusConnected    lipgloss.Style
	StatusDisconnected lipgloss.Style
	StatusError        lipgloss.Style
	StatusWarning      lipgloss.Style
	StatusInfo         lipgloss.Style
	StatusThinking     lipgloss.Style

	// Message styles
	MessageUser      lipgloss.Style
	MessageAssistant lipgloss.Style
	MessageSystem    lipgloss.Style
	MessageError     lipgloss.Style
	MessageCode      lipgloss.Style

	// Code block styles
	CodeBlock  lipgloss.Style
	CodeBorder lipgloss.Style

	// Input styles
	InputPlaceholder lipgloss.Style
	InputCursor      lipgloss.Style
	InputText        lipgloss.Style
	InputError       lipgloss.Style

	// Component border styles
	AppBorder       lipgloss.Style
	ChatBorder      lipgloss.Style
	StatusBorder    lipgloss.Style
	SessionBorder   lipgloss.Style
	ModalBackground lipgloss.Style

	// Session list styles
	SessionSelected lipgloss.Style
	SessionNormal   lipgloss.Style

	// Help/Key hint styles
	HelpKey       lipgloss.Style
	HelpDesc      lipgloss.Style
	HelpSeparator lipgloss.Style

	// Footer styles
	FooterText  lipgloss.Style
	FooterError lipgloss.Style

	// Header styles
	HeaderText    lipgloss.Style
	HeaderVersion lipgloss.Style

	// Error styles
	ErrorTitle  lipgloss.Style
	ErrorBorder lipgloss.Style
	ErrorText   lipgloss.Style

	// Scroll indicator styles
	ScrollIndicator lipgloss.Style
}

// Helper functions for common style combinations.

// StatusStyle returns the appropriate style based on connection status.
func StatusStyle(connected bool, thinking bool, err error) lipgloss.Style {
	switch {
	case err != nil:
		return Styles.StatusError
	case thinking:
		return Styles.StatusThinking
	case connected:
		return Styles.StatusConnected
	default:
		return Styles.StatusDisconnected
	}
}

// MessageTypeStyle returns the appropriate style for a message type.
func MessageTypeStyle(msgType string) lipgloss.Style {
	switch msgType {
	case "user":
		return Styles.MessageUser
	case "assistant":
		return Styles.MessageAssistant
	case "system":
		return Styles.MessageSystem
	case "error":
		return Styles.MessageError
	default:
		return Styles.Normal
	}
}
