package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sophonie/sono/internal/config"
	"sophonie/sono/internal/manager"
	"sophonie/sono/internal/nodedist"
)

type installState struct {
	stage      string
	downloaded int64
	total      int64
	ch         chan installEvent
}

type confirmKind int

const (
	confirmNone confirmKind = iota
	confirmUninstall
	confirmClearCache
)

type nodeModel struct {
	cfg *config.Config

	index    nodedist.Index
	schedule nodedist.Schedule
	loaded   bool
	loadErr  string

	filter string
	view   string
	query  string

	input     textinput.Model
	searching bool

	table    table.Model
	filtered nodedist.Index

	installs map[string]*installState
	spinner  spinner.Model

	settings         config.Settings
	active           string
	resolvedPath     string
	currentBinOnPath bool
	pathOK           bool
	latestLTS        string
	cacheCount       int
	cacheBytes       int64

	confirm     confirmKind
	confirmText string

	status      string
	statusLevel string

	width  int
	height int
}

func newNodeModel(cfg *config.Config) nodeModel {
	input := textinput.New()
	input.Placeholder = "version prefix (e.g. 20.11)"
	input.Prompt = "search: "

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	t := table.New(table.WithFocused(true))

	return nodeModel{
		cfg:      cfg,
		filter:   "all",
		view:     "compact",
		input:    input,
		table:    t,
		spinner:  sp,
		installs: map[string]*installState{},
	}
}

func (m nodeModel) init() tea.Cmd {
	return loadIndexCmd(m.cfg)
}

func (m nodeModel) Update(msg tea.Msg) (nodeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil

	case indexLoadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err.Error()
			return m, nil
		}
		m.index = msg.index
		m.schedule = msg.schedule
		m.loaded = true
		m.loadErr = ""
		m.rebuild()
		return m, nil

	case spinner.TickMsg:
		if len(m.installs) == 0 {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case installEvent:
		return m.handleInstallEvent(msg)

	case copiedMsg:
		m.setStatus("success", "PATH line copied")
		return m, nil

	case tea.KeyMsg:
		if m.searching {
			return m.updateSearch(msg)
		}
		if m.confirm != confirmNone {
			return m.updateConfirm(msg)
		}
		return m.updateKeys(msg)
	}
	return m, nil
}

func (m nodeModel) updateSearch(msg tea.KeyMsg) (nodeModel, tea.Cmd) {
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

func (m nodeModel) updateConfirm(msg tea.KeyMsg) (nodeModel, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		kind := m.confirm
		m.confirm = confirmNone
		m.confirmText = ""
		return m.runConfirmed(kind)
	default:
		m.confirm = confirmNone
		m.confirmText = ""
		return m, nil
	}
}

func (m nodeModel) updateKeys(msg tea.KeyMsg) (nodeModel, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Search):
		m.searching = true
		m.input.Focus()
		return m, textinput.Blink

	case key.Matches(msg, keys.Filter):
		m.filter = nextFilter(m.filter)
		m.rebuild()
		return m, nil

	case key.Matches(msg, keys.View):
		if m.view == "compact" {
			m.view = "all"
		} else {
			m.view = "compact"
		}
		m.rebuild()
		return m, nil

	case key.Matches(msg, keys.Install):
		return m.install()

	case key.Matches(msg, keys.Activate):
		return m.activate()

	case key.Matches(msg, keys.Uninstall):
		return m.askUninstall()

	case key.Matches(msg, keys.Clear):
		return m.askClearCache()

	case key.Matches(msg, keys.AutoPurge):
		m.settings.AutoPurgeEnabled = !m.settings.AutoPurgeEnabled
		_ = config.SaveSettings(m.cfg, m.settings)
		m.setStatus("success", "Settings saved")
		return m, nil

	case key.Matches(msg, keys.Yank):
		return m, copyCmd(pathLine)
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m nodeModel) selected() (nodedist.Release, bool) {
	cursor := m.table.Cursor()
	if cursor < 0 || cursor >= len(m.filtered) {
		return nodedist.Release{}, false
	}
	return m.filtered[cursor], true
}

func (m nodeModel) install() (nodeModel, tea.Cmd) {
	release, ok := m.selected()
	if !ok {
		return m, nil
	}
	target := release.Version
	if updates := manager.AvailableUpdates(m.cfg, m.index); updates[release.Version] != "" {
		target = updates[release.Version]
	}
	if m.installedSet()[target] {
		return m, nil
	}
	if _, running := m.installs[target]; running {
		return m, nil
	}
	ch := startInstall(m.cfg, target)
	m.installs[target] = &installState{stage: manager.StageDownloading, ch: ch}
	m.rebuild()
	return m, tea.Batch(waitInstallCmd(ch), m.spinner.Tick)
}

func (m nodeModel) activate() (nodeModel, tea.Cmd) {
	release, ok := m.selected()
	if !ok || !m.installedSet()[release.Version] {
		return m, nil
	}
	if err := manager.SetActive(m.cfg, release.Version); err != nil {
		m.setStatus("error", release.Version+" could not be activated: "+err.Error())
	} else {
		m.setStatus("success", release.Version+" is now active")
	}
	m.rebuild()
	return m, nil
}

func (m nodeModel) askUninstall() (nodeModel, tea.Cmd) {
	release, ok := m.selected()
	if !ok || !m.installedSet()[release.Version] {
		return m, nil
	}
	m.confirm = confirmUninstall
	m.confirmText = "Uninstall " + release.Version + "? (y/n)"
	return m, nil
}

func (m nodeModel) askClearCache() (nodeModel, tea.Cmd) {
	if m.cacheCount == 0 {
		return m, nil
	}
	m.confirm = confirmClearCache
	m.confirmText = fmt.Sprintf("Clear cache (%d tarball%s · %s)? (y/n)", m.cacheCount, plural(m.cacheCount), humanSize(m.cacheBytes))
	return m, nil
}

func (m nodeModel) runConfirmed(kind confirmKind) (nodeModel, tea.Cmd) {
	switch kind {
	case confirmUninstall:
		release, ok := m.selected()
		if !ok {
			return m, nil
		}
		if err := manager.Uninstall(m.cfg, release.Version); err != nil {
			m.setStatus("error", err.Error())
		} else {
			m.setStatus("success", release.Version+" removed")
		}
	case confirmClearCache:
		count, _ := manager.PurgeCache(m.cfg)
		m.setStatus("success", fmt.Sprintf("Cache cleared (%d tarball%s)", count, plural(count)))
	}
	m.rebuild()
	return m, nil
}

func (m nodeModel) handleInstallEvent(ev installEvent) (nodeModel, tea.Cmd) {
	state := m.installs[ev.version]
	if state == nil {
		return m, nil
	}
	if ev.errMsg != "" {
		delete(m.installs, ev.version)
		m.setStatus("error", ev.version+" failed: "+ev.errMsg)
		m.rebuild()
		return m, nil
	}
	if ev.done {
		delete(m.installs, ev.version)
		m.setStatus("success", ev.version+" installed")
		m.rebuild()
		return m, nil
	}
	state.stage = ev.stage
	state.downloaded = ev.downloaded
	state.total = ev.total
	m.rebuild()
	return m, waitInstallCmd(state.ch)
}

func (m *nodeModel) rebuild() {
	if !m.loaded {
		return
	}
	installed := m.installedSet()
	m.active, _ = manager.Active(m.cfg)
	m.resolvedPath = manager.ResolvedOnPath()
	m.currentBinOnPath = manager.CurrentBinOnPath(m.cfg)
	m.pathOK = m.active != "" && m.resolvedPath == m.active
	m.latestLTS = m.index.LatestLTS()
	m.cacheCount, m.cacheBytes = manager.CacheInfo(m.cfg)
	m.settings = config.LoadSettings(m.cfg)
	updates := manager.AvailableUpdates(m.cfg, m.index)

	filtered := m.index
	switch m.filter {
	case "lts":
		filtered = filtered.LTS()
	case "nonlts":
		filtered = filtered.NonLTS()
	case "installed":
		filtered = onlyInstalled(m.index, installed)
	}
	if m.query != "" {
		filtered = filtered.SearchPrefix(m.query)
	} else if m.view == "compact" && m.filter != "installed" {
		filtered = filtered.LatestPerMajor()
	}
	filtered = filtered.Sorted()
	m.filtered = filtered

	rows := make([]table.Row, 0, len(filtered))
	for _, release := range filtered {
		endOfLife := ""
		if m.schedule != nil {
			endOfLife = m.schedule.EndOfLife(release.Version)
		}
		lts := "-"
		if release.LTS.IsLTS {
			lts = release.LTS.Codename
		}
		eol := "-"
		if endOfLife != "" {
			eol = endOfLife
			if isExpired(endOfLife) {
				eol += " (expired)"
			}
		}
		rows = append(rows, table.Row{
			release.Version,
			release.Date,
			lts,
			eol,
			m.statusCell(release, installed, updates),
		})
	}
	m.table.SetRows(rows)
}

func (m nodeModel) statusCell(release nodedist.Release, installed map[string]bool, updates map[string]string) string {
	if state := m.installs[release.Version]; state != nil {
		if state.stage == manager.StageDownloading && state.total > 0 {
			return fmt.Sprintf("downloading %d%%", state.downloaded*100/state.total)
		}
		return strings.ToLower(stageLabel(state.stage)) + "…"
	}
	switch {
	case m.active != "" && release.Version == m.active:
		return "active"
	case installed[release.Version]:
		if updates[release.Version] != "" {
			return "installed ⬆ " + updates[release.Version]
		}
		return "installed"
	case updates[release.Version] != "":
		return "⬆ " + updates[release.Version]
	default:
		return "-"
	}
}

func (m *nodeModel) layout() {
	if m.width == 0 {
		return
	}
	versionW, dateW, ltsW, eolW := 14, 12, 10, 22
	statusW := m.width - versionW - dateW - ltsW - eolW - 10
	if statusW < 12 {
		statusW = 12
	}
	m.table.SetColumns([]table.Column{
		{Title: "Version", Width: versionW},
		{Title: "Date", Width: dateW},
		{Title: "LTS", Width: ltsW},
		{Title: "End of support", Width: eolW},
		{Title: "Status", Width: statusW},
	})
	m.table.SetWidth(m.width)
	m.table.SetHeight(m.tableHeight())
}

func (m nodeModel) tableHeight() int {
	height := m.height - nodeChromeHeight
	if height < 3 {
		height = 3
	}
	return height
}

const nodeChromeHeight = 12

func (m *nodeModel) setStatus(level, text string) {
	m.statusLevel = level
	m.status = text
}

func (m nodeModel) installedSet() map[string]bool {
	set := map[string]bool{}
	installed, err := manager.ListInstalled(m.cfg)
	if err != nil {
		return set
	}
	for _, version := range installed {
		set[version] = true
	}
	return set
}

func nextFilter(current string) string {
	switch current {
	case "all":
		return "lts"
	case "lts":
		return "nonlts"
	case "nonlts":
		return "installed"
	default:
		return "all"
	}
}

func (m nodeModel) View() string {
	if m.loadErr != "" {
		return errorStyle.Render("Could not load the version list: "+m.loadErr) + "\n"
	}
	if !m.loaded {
		return "Loading Node.js versions…\n"
	}

	var b strings.Builder

	if m.searching {
		b.WriteString(m.input.View() + "\n")
	} else {
		b.WriteString(summaryStyle.Render(fmt.Sprintf("filter: %s · view: %s", m.filter, m.view)) + "\n")
	}

	summary := fmt.Sprintf("%d version%s", len(m.filtered), plural(len(m.filtered)))
	if m.latestLTS != "" {
		summary += "   " + ltsNoteStyle.Render("Latest LTS: "+m.latestLTS)
	}
	b.WriteString(summaryStyle.Render(summary) + "\n")

	b.WriteString(m.table.View() + "\n")

	b.WriteString(m.pathBanner())
	b.WriteString(m.cacheFooter())

	if m.confirm != confirmNone {
		b.WriteString(confirmStyle.Render(m.confirmText) + "\n")
	} else if m.status != "" {
		b.WriteString(m.statusView() + "\n")
	}
	return b.String()
}

func (m nodeModel) statusView() string {
	if m.statusLevel == "error" {
		return statusErrorStyle.Render("✗ " + m.status)
	}
	return statusSuccessStyle.Render("✓ " + m.status)
}

func (m nodeModel) pathBanner() string {
	if m.pathOK {
		return ""
	}
	var b strings.Builder
	if m.active != "" {
		b.WriteString(bannerTitleStyle.Render("Active version: "+m.active) + "\n")
		switch {
		case !m.currentBinOnPath:
			b.WriteString(warnStyle.Render("⚠ ~/.sono/current/bin is not on PATH. Add the line below to ~/.bashrc, then restart from a new terminal.") + "\n")
		case m.resolvedPath != "":
			b.WriteString(warnStyle.Render("⚠ Another node ("+m.resolvedPath+") takes precedence before ~/.sono/current/bin.") + "\n")
		default:
			b.WriteString(warnStyle.Render("⚠ node not found on PATH.") + "\n")
		}
	} else {
		b.WriteString(warnStyle.Render("No active version. Activate one, then add the line below to ~/.bashrc.") + "\n")
	}
	b.WriteString(pathCodeStyle.Render(pathLine) + "  " + helpBarStyle.Render("(y to copy)") + "\n")
	return b.String()
}

func (m nodeModel) cacheFooter() string {
	autopurge := "off"
	if m.settings.AutoPurgeEnabled {
		autopurge = "on"
	}
	cache := "Cache empty"
	if m.cacheCount > 0 {
		cache = fmt.Sprintf("Cache: %d tarball%s · %s", m.cacheCount, plural(m.cacheCount), humanSize(m.cacheBytes))
	}
	line := fmt.Sprintf("%s   auto-purge: %s (after %d days)", cache, autopurge, clampMaxAge(m.settings.CacheMaxAgeDays))
	return lipgloss.NewStyle().Foreground(colorMuted).Render(line) + "\n"
}
