package tui

import "github.com/charmbracelet/lipgloss"

// ---------------------------------------------------------------------------
// Color palette (go-chat inspired)
// ---------------------------------------------------------------------------

var (
	blue      = lipgloss.Color("69")
	white     = lipgloss.Color("255")
	gray      = lipgloss.Color("245")
	userClr   = lipgloss.Color("39")  // cyan for user label
	bubbleClr = lipgloss.Color("33")  // blue for user bubble border
)

// ---------------------------------------------------------------------------
// Component styles
// ---------------------------------------------------------------------------

// headerStyle matches the go-chat InLineBlock with
// background: blue, foreground: white, weight: fill, alignment: center.
var headerStyle = lipgloss.NewStyle().
	Background(blue).
	Foreground(white).
	Bold(true).
	Align(lipgloss.Center)

// sessionStyle returns a style for a sidebar session item.
// Focused (active) sessions get a blue background; others are plain white.
func sessionStyle(focused bool) lipgloss.Style {
	if focused {
		return lipgloss.NewStyle().
			Background(blue).
			Foreground(white).
			Bold(true)
	}
	return lipgloss.NewStyle().
		Foreground(white)
}

// messageBlockStyle returns a style for a chat message block with a
// rounded border in the given color.
func messageBlockStyle(borderColor lipgloss.Color, width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(width)
}

// senderLabelStyle returns a bold sender label style.
func senderLabelStyle(clr lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(clr).Bold(true)
}

// ---------------------------------------------------------------------------
// Panel and container styles
// ---------------------------------------------------------------------------

var (
	infoStyle     = lipgloss.NewStyle().Foreground(gray).Italic(true)
	greenDot      = "●"
	leftPanelBg   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	rightPanelBg  = lipgloss.NewStyle()
	vpBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	taBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())

	// Side panel
	sidePanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	// Chat messages container
	messagesStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	// Text input container
	inputAreaStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	// Focused border variant — pink, used when a component is active
	focusedBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("212")).
				Padding(0, 1)

	// User message style
	userMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	// Assistant message style
	assistantMsgStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255"))

	// Tool message style
	toolMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Faint(true)
)
