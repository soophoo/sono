package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"sophonie/sono/internal/config"
)

type model struct {
	cfg  *config.Config
	tab  int
	node nodeModel
	pm   pmModel
	help help.Model
}

func newModel(cfg *config.Config) model {
	return model{
		cfg:  cfg,
		node: newNodeModel(cfg),
		pm:   newPmModel(cfg),
		help: help.New(),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.node.init(), m.pm.init())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.help.Width = msg.Width
		return m.forward(msg)

	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	return m.forward(msg)
}

func (m model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.activeSearching() {
		return m.forwardKey(msg)
	}
	switch {
	case key.Matches(msg, keys.Switch):
		m.tab = (m.tab + 1) % 2
		return m, nil
	case key.Matches(msg, keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	}
	return m.forwardKey(msg)
}

func (m model) activeSearching() bool {
	if m.tab == 0 {
		return m.node.searching
	}
	return m.pm.searching
}

func (m model) forwardKey(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.tab == 0 {
		m.node, cmd = m.node.Update(msg)
	} else {
		m.pm, cmd = m.pm.Update(msg)
	}
	return m, cmd
}

func (m model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	var nodeCmd, pmCmd tea.Cmd
	m.node, nodeCmd = m.node.Update(msg)
	m.pm, pmCmd = m.pm.Update(msg)
	return m, tea.Batch(nodeCmd, pmCmd)
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString(headerView(m.tab))
	if m.tab == 0 {
		b.WriteString(m.node.View())
	} else {
		b.WriteString(m.pm.View())
	}
	b.WriteString(helpBarStyle.Render(m.help.View(keys)))
	return b.String()
}

func Run(cfg *config.Config) error {
	program := tea.NewProgram(newModel(cfg), tea.WithAltScreen())
	_, err := program.Run()
	return err
}
