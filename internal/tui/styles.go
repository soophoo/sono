package tui

import "github.com/charmbracelet/lipgloss"

const pathLine = `export PATH="$HOME/.sono/current/bin:$HOME/.sono/shims:$PATH"`

var (
	colorAccent  = lipgloss.AdaptiveColor{Light: "#1c5dd8", Dark: "#6fa8ff"}
	colorSuccess = lipgloss.AdaptiveColor{Light: "#127a2f", Dark: "#4fce74"}
	colorWarn    = lipgloss.AdaptiveColor{Light: "#a15c00", Dark: "#e0a54a"}
	colorError   = lipgloss.AdaptiveColor{Light: "#b3261e", Dark: "#ff6b61"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#6b6b6b", Dark: "#8a8a8a"}
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

	taglineStyle = lipgloss.NewStyle().Foreground(colorMuted)

	tabActiveStyle = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(colorAccent).Padding(0, 2)

	tabInactiveStyle = lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 2)

	summaryStyle = lipgloss.NewStyle().Foreground(colorMuted)

	ltsNoteStyle = lipgloss.NewStyle().Foreground(colorSuccess)

	bannerTitleStyle = lipgloss.NewStyle().Bold(true)

	warnStyle = lipgloss.NewStyle().Foreground(colorWarn)

	pathCodeStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

	confirmStyle = lipgloss.NewStyle().Foreground(colorWarn).Bold(true)

	errorStyle = lipgloss.NewStyle().Foreground(colorError).Bold(true)

	statusSuccessStyle = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)

	statusErrorStyle = lipgloss.NewStyle().Foreground(colorError).Bold(true)

	helpBarStyle = lipgloss.NewStyle().Foreground(colorMuted)
)

func tabBar(active int) string {
	node := "Node"
	pm := "Package managers"
	if active == 0 {
		return lipgloss.JoinHorizontal(lipgloss.Bottom, tabActiveStyle.Render(node), tabInactiveStyle.Render(pm))
	}
	return lipgloss.JoinHorizontal(lipgloss.Bottom, tabInactiveStyle.Render(node), tabActiveStyle.Render(pm))
}

func headerView(active int) string {
	title := titleStyle.Render("sono") + "  " + taglineStyle.Render("Node.js & package manager toolkit")
	return title + "\n" + tabBar(active) + "\n"
}
