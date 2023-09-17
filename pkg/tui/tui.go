package tui

import "github.com/charmbracelet/lipgloss"

var (
	Red    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	Green  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	Gray   = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	Yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	Faint = lipgloss.NewStyle().Faint(true)
)