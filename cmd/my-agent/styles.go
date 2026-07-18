package main

import "github.com/charmbracelet/lipgloss"

var (
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

	// User message prefix
	userMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	// Assistant message
	assistantMsgStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255"))

	// Tool message
	toolMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Faint(true)

	// (footer now rendered via InlineBlock in renderFooter)
)
