package ui

import "github.com/charmbracelet/lipgloss"

var (
	_ = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 0).
		MarginRight(1)

	_ = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("14")).
		Foreground(lipgloss.Color("14")).
		Bold(true).
		Padding(0, 0).
		MarginRight(1)

	listItemActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Bold(true).
				PaddingLeft(2)

	boldWhite  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	dimText    = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	greenText  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true).Italic(true)
	cyanText   = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	yellowText = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))

	tableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	rowStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dismissalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Italic(true)

	commentaryBallStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	commentaryWicketStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	commentaryBoundaryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	commentaryOverSepStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)

	hintKeyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	hintLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))

	helpOverlayStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("14")).
				Padding(1, 3).
				Align(lipgloss.Left)
)
