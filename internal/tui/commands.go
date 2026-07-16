package tui

import (
	"encoding/base64"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"sophonie/sono/internal/config"
	"sophonie/sono/internal/manager"
	"sophonie/sono/internal/nodedist"
	"sophonie/sono/internal/pkgmgr"
)

type indexLoadedMsg struct {
	index    nodedist.Index
	schedule nodedist.Schedule
	err      error
}

type installEvent struct {
	version    string
	stage      string
	downloaded int64
	total      int64
	done       bool
	errMsg     string
}

type pmLoadedMsg struct {
	pm       string
	versions []string
	err      error
}

type pmActionMsg struct {
	pm    string
	level string
	text  string
}

type copiedMsg struct{}

func loadIndexCmd(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		index, err := nodedist.LoadIndex(cfg.IndexCache)
		if err != nil {
			return indexLoadedMsg{err: err}
		}
		schedule, _ := nodedist.LoadSchedule(cfg.ScheduleCache)
		return indexLoadedMsg{index: index, schedule: schedule}
	}
}

func startInstall(cfg *config.Config, version string) chan installEvent {
	ch := make(chan installEvent, 8)
	go func() {
		err := manager.Install(cfg, version, func(stage string, downloaded, total int64) {
			ch <- installEvent{version: version, stage: stage, downloaded: downloaded, total: total}
		})
		if err != nil {
			ch <- installEvent{version: version, errMsg: err.Error()}
		} else {
			ch <- installEvent{version: version, done: true}
		}
		close(ch)
	}()
	return ch
}

func waitInstallCmd(ch chan installEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return ev
	}
}

func loadPmCmd(cfg *config.Config, pm pkgmgr.PackageManager) tea.Cmd {
	return func() tea.Msg {
		versions, err := pkgmgr.ListStableVersions(cfg, pm)
		return pmLoadedMsg{pm: pm.Name, versions: versions, err: err}
	}
}

func pmInstallCmd(cfg *config.Config, pm pkgmgr.PackageManager, version string) tea.Cmd {
	return func() tea.Msg {
		if err := pkgmgr.Install(cfg, pm, version); err != nil {
			return pmActionMsg{pm: pm.Name, level: "error", text: pm.Name + " " + version + " failed: " + err.Error()}
		}
		return pmActionMsg{pm: pm.Name, level: "success", text: pm.Name + " " + version + " installed"}
	}
}

func copyCmd(text string) tea.Cmd {
	return func() tea.Msg {
		fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(text)))
		return copiedMsg{}
	}
}
