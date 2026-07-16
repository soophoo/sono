package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"sophonie/sono/internal/config"
	"sophonie/sono/internal/manager"
	"sophonie/sono/internal/pkgmgr"
)

type pmModel struct {
	cfg *config.Config

	pmIndex  int
	versions []string
	loadedPm string
	loadErr  string

	query     string
	input     textinput.Model
	searching bool

	table    table.Model
	filtered []string

	installing map[string]bool
	spinner    spinner.Model

	active      string
	shimsOnPath bool
	installed   map[string]bool

	confirm     bool
	confirmText string
	confirmVer  string

	status      string
	statusLevel string

	width  int
	height int
}

func newPmModel(cfg *config.Config) pmModel {
	input := textinput.New()
	input.Placeholder = "version prefix (e.g. 9.15)"
	input.Prompt = "search: "

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return pmModel{
		cfg:        cfg,
		input:      input,
		table:      table.New(table.WithFocused(true)),
		spinner:    sp,
		installing: map[string]bool{},
		installed:  map[string]bool{},
	}
}

func (m pmModel) current() pkgmgr.PackageManager {
	return pkgmgr.Supported[m.pmIndex]
}

func (m pmModel) init() tea.Cmd {
	return loadPmCmd(m.cfg, m.current())
}

func (m pmModel) Update(msg tea.Msg) (pmModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil

	case pmLoadedMsg:
		if msg.pm != m.current().Name {
			return m, nil
		}
		if msg.err != nil {
			m.loadErr = msg.err.Error()
			return m, nil
		}
		m.loadErr = ""
		m.versions = msg.versions
		m.loadedPm = msg.pm
		m.rebuild()
		return m, nil

	case pmActionMsg:
		m.installing = map[string]bool{}
		m.setStatus(msg.level, msg.text)
		m.rebuild()
		return m, nil

	case copiedMsg:
		m.setStatus("success", "PATH line copied")
		return m, nil

	case spinner.TickMsg:
		if len(m.installing) == 0 {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if m.searching {
			return m.updateSearch(msg)
		}
		if m.confirm {
			return m.updateConfirm(msg)
		}
		return m.updateKeys(msg)
	}
	return m, nil
}

func (m pmModel) updateSearch(msg tea.KeyMsg) (pmModel, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		m.searching = false
		m.input.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != m.query {
		m.query = m.input.Value()
		m.rebuild()
	}
	return m, cmd
}

func (m pmModel) updateConfirm(msg tea.KeyMsg) (pmModel, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.confirm = false
		version := m.confirmVer
		m.confirmText = ""
		if err := pkgmgr.Uninstall(m.cfg, m.current(), version); err != nil {
			m.setStatus("error", err.Error())
		} else {
			m.setStatus("success", m.current().Name+" "+version+" removed")
		}
		m.rebuild()
		return m, nil
	default:
		m.confirm = false
		m.confirmText = ""
		return m, nil
	}
}

func (m pmModel) updateKeys(msg tea.KeyMsg) (pmModel, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Search):
		m.searching = true
		m.input.Focus()
		return m, textinput.Blink

	case key.Matches(msg, keys.Filter):
		m.pmIndex = (m.pmIndex + 1) % len(pkgmgr.Supported)
		m.query = ""
		m.input.Reset()
		m.versions = nil
		m.loadedPm = ""
		m.loadErr = ""
		return m, loadPmCmd(m.cfg, m.current())

	case key.Matches(msg, keys.Install):
		return m.install()

	case key.Matches(msg, keys.Activate):
		return m.activate()

	case key.Matches(msg, keys.Uninstall):
		return m.askUninstall()

	case key.Matches(msg, keys.Yank):
		return m, copyCmd(pathLine)
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m pmModel) selected() (string, bool) {
	cursor := m.table.Cursor()
	if cursor < 0 || cursor >= len(m.filtered) {
		return "", false
	}
	return m.filtered[cursor], true
}

func (m pmModel) install() (pmModel, tea.Cmd) {
	version, ok := m.selected()
	if !ok || m.installed[version] || m.installing[version] {
		return m, nil
	}
	m.installing[version] = true
	m.rebuild()
	return m, tea.Batch(pmInstallCmd(m.cfg, m.current(), version), m.spinner.Tick)
}

func (m pmModel) activate() (pmModel, tea.Cmd) {
	version, ok := m.selected()
	if !ok || !m.installed[version] {
		return m, nil
	}
	if err := pkgmgr.Activate(m.cfg, m.current(), version); err != nil {
		m.setStatus("error", m.current().Name+" could not be activated: "+err.Error())
	} else {
		m.setStatus("success", m.current().Name+" "+version+" is now active")
	}
	m.rebuild()
	return m, nil
}

func (m pmModel) askUninstall() (pmModel, tea.Cmd) {
	version, ok := m.selected()
	if !ok || !m.installed[version] {
		return m, nil
	}
	m.confirm = true
	m.confirmVer = version
	m.confirmText = "Uninstall " + m.current().Name + " " + version + "? (y/n)"
	return m, nil
}

func (m *pmModel) rebuild() {
	pm := m.current()
	m.active, _ = pkgmgr.Active(m.cfg, pm)
	m.shimsOnPath = manager.DirOnPath(m.cfg.ShimsDir)
	m.installed = map[string]bool{}
	if installed, err := pkgmgr.Installed(m.cfg, pm); err == nil {
		for _, version := range installed {
			m.installed[version] = true
		}
	}

	filtered := m.versions
	if m.query != "" {
		matched := make([]string, 0, len(m.versions))
		for _, version := range m.versions {
			if strings.HasPrefix(version, m.query) {
				matched = append(matched, version)
			}
		}
		filtered = matched
	}
	m.filtered = filtered

	rows := make([]table.Row, 0, len(filtered))
	for _, version := range filtered {
		rows = append(rows, table.Row{version, m.statusCell(version)})
	}
	m.table.SetRows(rows)
}

func (m pmModel) statusCell(version string) string {
	switch {
	case m.installing[version]:
		return "installing…"
	case m.active != "" && version == m.active:
		return "active"
	case m.installed[version]:
		return "installed"
	default:
		return "-"
	}
}

func (m *pmModel) layout() {
	if m.width == 0 {
		return
	}
	versionW := 20
	statusW := m.width - versionW - 6
	if statusW < 12 {
		statusW = 12
	}
	m.table.SetColumns([]table.Column{
		{Title: "Version", Width: versionW},
		{Title: "Status", Width: statusW},
	})
	m.table.SetWidth(m.width)
	height := m.height - pmChromeHeight
	if height < 3 {
		height = 3
	}
	m.table.SetHeight(height)
}

const pmChromeHeight = 9

func (m *pmModel) setStatus(level, text string) {
	m.statusLevel = level
	m.status = text
}

func (m pmModel) View() string {
	if m.loadErr != "" {
		return errorStyle.Render("Could not load versions: "+m.loadErr) + "\n"
	}
	if m.loadedPm != m.current().Name {
		return fmt.Sprintf("Loading %s versions…\n", m.current().Name)
	}

	var b strings.Builder

	if m.searching {
		b.WriteString(m.input.View() + "\n")
	} else {
		b.WriteString(summaryStyle.Render("package manager: "+m.current().Name+"  (f to switch)") + "\n")
	}
	b.WriteString(summaryStyle.Render(fmt.Sprintf("%d version%s", len(m.filtered), plural(len(m.filtered)))) + "\n")

	b.WriteString(m.table.View() + "\n")

	if !m.shimsOnPath {
		b.WriteString(warnStyle.Render("⚠ Package managers require ~/.sono/shims on PATH. Add the line below to ~/.bashrc, then restart from a new terminal.") + "\n")
		b.WriteString(pathCodeStyle.Render(pathLine) + "  " + helpBarStyle.Render("(y to copy)") + "\n")
	}

	if m.confirm {
		b.WriteString(confirmStyle.Render(m.confirmText) + "\n")
	} else if m.status != "" {
		if m.statusLevel == "error" {
			b.WriteString(statusErrorStyle.Render("✗ "+m.status) + "\n")
		} else {
			b.WriteString(statusSuccessStyle.Render("✓ "+m.status) + "\n")
		}
	}
	return b.String()
}
