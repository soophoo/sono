package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"sophonie/sono/internal/config"
	"sophonie/sono/internal/nodedist"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.New()
	if err != nil {
		t.Fatalf("config.New: %v", err)
	}
	return cfg
}

func fakeIndex() nodedist.Index {
	return nodedist.Index{
		{Version: "v22.5.1", Date: "2024-07-01", LTS: nodedist.LTSFlag{}},
		{Version: "v20.15.0", Date: "2024-06-20", LTS: nodedist.LTSFlag{IsLTS: true, Codename: "Iron"}},
		{Version: "v20.11.0", Date: "2024-01-09", LTS: nodedist.LTSFlag{IsLTS: true, Codename: "Iron"}},
	}
}

func loadedNode(t *testing.T) nodeModel {
	m := newNodeModel(testConfig(t))
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = m.Update(indexLoadedMsg{index: fakeIndex()})
	return m
}

func TestNodeRendersVersionsCompact(t *testing.T) {
	m := loadedNode(t)
	view := m.View()
	for _, want := range []string{"v22.5.1", "v20.15.0", "Iron", "2 versions"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "v20.11.0") {
		t.Fatalf("compact view should collapse v20.11.0\n%s", view)
	}
}

func TestNodeViewAllShowsEveryPatch(t *testing.T) {
	m := loadedNode(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	view := m.View()
	if m.view != "all" {
		t.Fatalf("view = %q, want all", m.view)
	}
	for _, want := range []string{"v20.15.0", "v20.11.0", "3 versions"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q\n%s", want, view)
		}
	}
}

func TestNodeFilterLTS(t *testing.T) {
	m := loadedNode(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if m.filter != "lts" {
		t.Fatalf("filter = %q, want lts", m.filter)
	}
	view := m.View()
	if strings.Contains(view, "v22.5.1") {
		t.Fatalf("LTS filter should hide v22.5.1\n%s", view)
	}
	if !strings.Contains(view, "v20.15.0") {
		t.Fatalf("LTS filter should show v20.15.0\n%s", view)
	}
}

func TestNodeSearch(t *testing.T) {
	m := loadedNode(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !m.searching {
		t.Fatal("expected searching mode")
	}
	for _, r := range "22" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.query != "22" {
		t.Fatalf("query = %q, want 22", m.query)
	}
	if got := len(m.filtered); got != 1 {
		t.Fatalf("filtered = %d, want 1", got)
	}
}

func TestNodeInstallProgressCell(t *testing.T) {
	m := loadedNode(t)
	m.installs["v20.15.0"] = &installState{stage: "downloading", downloaded: 50, total: 100}
	m.rebuild()
	if !strings.Contains(m.View(), "downloading 50%") {
		t.Fatalf("expected progress cell\n%s", m.View())
	}
}

func TestPmSwitch(t *testing.T) {
	m := newPmModel(testConfig(t))
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = m.Update(pmLoadedMsg{pm: "pnpm", versions: []string{"9.15.0", "9.14.0"}})
	if m.loadedPm != "pnpm" {
		t.Fatalf("loadedPm = %q", m.loadedPm)
	}
	if !strings.Contains(m.View(), "9.15.0") {
		t.Fatalf("expected pnpm versions\n%s", m.View())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if m.current().Name != "yarn" {
		t.Fatalf("expected yarn after switch, got %s", m.current().Name)
	}
}
