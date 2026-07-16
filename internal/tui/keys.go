package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up        key.Binding
	Down      key.Binding
	Search    key.Binding
	Filter    key.Binding
	View      key.Binding
	Install   key.Binding
	Activate  key.Binding
	Uninstall key.Binding
	Clear     key.Binding
	AutoPurge key.Binding
	Yank      key.Binding
	Switch    key.Binding
	Help      key.Binding
	Quit      key.Binding
}

var keys = keyMap{
	Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Search:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	Filter:    key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter")),
	View:      key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view")),
	Install:   key.NewBinding(key.WithKeys("enter", "i"), key.WithHelp("enter/i", "install")),
	Activate:  key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "activate")),
	Uninstall: key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "uninstall")),
	Clear:     key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear cache")),
	AutoPurge: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "auto-purge")),
	Yank:      key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy PATH")),
	Switch:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "section")),
	Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Search, k.Install, k.Activate, k.Switch, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Search, k.Switch},
		{k.Filter, k.View, k.Install, k.Activate, k.Uninstall},
		{k.Clear, k.AutoPurge, k.Yank, k.Help, k.Quit},
	}
}
