package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"sophonie/sono/internal/config"
	"sophonie/sono/internal/manager"
	"sophonie/sono/internal/nodedist"
)

func Run(cfg *config.Config, args []string) int {
	command := args[0]
	rest := args[1:]

	switch command {
	case "install", "i":
		return cmdInstall(cfg, rest)
	case "use", "activate":
		return cmdUse(cfg, rest)
	case "uninstall", "rm", "remove":
		return cmdUninstall(cfg, rest)
	case "ls", "list", "installed":
		return cmdList(cfg)
	case "ls-remote", "available":
		return cmdListRemote(cfg, rest)
	case "current":
		return cmdCurrent(cfg)
	case "path":
		fmt.Println(pathLine)
		return 0
	case "cache":
		return cmdCache(cfg, rest)
	case "pm":
		return cmdPm(cfg, rest)
	case "help", "-h", "--help":
		printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", command)
		printUsage()
		return 2
	}
}

const pathLine = `export PATH="$HOME/.sono/current/bin:$HOME/.sono/shims:$PATH"`

func loadIndex(cfg *config.Config) (nodedist.Index, nodedist.Schedule, error) {
	index, err := nodedist.LoadIndex(cfg.IndexCache)
	if err != nil {
		return nil, nil, err
	}
	schedule, _ := nodedist.LoadSchedule(cfg.ScheduleCache)
	return index, schedule, nil
}

func installedSet(cfg *config.Config) map[string]bool {
	set := map[string]bool{}
	installed, err := manager.ListInstalled(cfg)
	if err != nil {
		return set
	}
	for _, version := range installed {
		set[version] = true
	}
	return set
}

func cmdInstall(cfg *config.Config, args []string) int {
	flags, positional := splitArgs(args)
	if len(positional) == 0 {
		return fail("usage: sono install <version> [--use]")
	}
	index, _, err := loadIndex(cfg)
	if err != nil {
		return fail("could not load the version index: %v", err)
	}
	version, err := resolveRemoteNode(index, positional[0])
	if err != nil {
		return fail("%v", err)
	}

	if installedSet(cfg)[version] {
		fmt.Printf("%s is already installed\n", version)
	} else {
		fmt.Printf("Installing %s\n", version)
		if err := manager.Install(cfg, version, nodeProgress()); err != nil {
			return fail("install failed: %v", err)
		}
		fmt.Printf("Installed %s\n", version)
	}

	if flags["use"] {
		if err := manager.SetActive(cfg, version); err != nil {
			return fail("could not activate %s: %v", version, err)
		}
		fmt.Printf("%s is now active\n", version)
	}
	return 0
}

func cmdUse(cfg *config.Config, args []string) int {
	if len(args) == 0 {
		return fail("usage: sono use <version>")
	}
	installed, err := manager.ListInstalled(cfg)
	if err != nil {
		return fail("%v", err)
	}
	version, err := resolveInstalledNode(installed, args[0])
	if err != nil {
		return fail("%v", err)
	}
	if err := manager.SetActive(cfg, version); err != nil {
		return fail("could not activate %s: %v", version, err)
	}
	fmt.Printf("%s is now active\n", version)
	return 0
}

func cmdUninstall(cfg *config.Config, args []string) int {
	if len(args) == 0 {
		return fail("usage: sono uninstall <version>")
	}
	installed, err := manager.ListInstalled(cfg)
	if err != nil {
		return fail("%v", err)
	}
	version, err := resolveInstalledNode(installed, args[0])
	if err != nil {
		return fail("%v", err)
	}
	if err := manager.Uninstall(cfg, version); err != nil {
		return fail("%v", err)
	}
	fmt.Printf("%s removed\n", version)
	return 0
}

func cmdList(cfg *config.Config) int {
	installed, err := manager.ListInstalled(cfg)
	if err != nil {
		return fail("%v", err)
	}
	if len(installed) == 0 {
		fmt.Println("No Node.js versions installed.")
		return 0
	}
	active, _ := manager.Active(cfg)
	var index nodedist.Index
	for _, version := range installed {
		index = append(index, nodedist.Release{Version: version})
	}

	updates := map[string]string{}
	if remote, _, err := loadIndex(cfg); err == nil {
		updates = manager.AvailableUpdates(cfg, remote)
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, release := range index.Sorted() {
		marker := " "
		if release.Version == active {
			marker = "*"
		}
		note := ""
		if updates[release.Version] != "" {
			note = "update → " + updates[release.Version]
		}
		fmt.Fprintf(writer, "%s %s\t%s\n", marker, release.Version, note)
	}
	writer.Flush()
	return 0
}

func cmdListRemote(cfg *config.Config, args []string) int {
	flags, positional := splitArgs(args)
	index, schedule, err := loadIndex(cfg)
	if err != nil {
		return fail("could not load the version index: %v", err)
	}

	filtered := index
	switch {
	case flags["lts"]:
		filtered = filtered.LTS()
	case flags["nonlts"]:
		filtered = filtered.NonLTS()
	}

	prefix := ""
	if len(positional) > 0 {
		prefix = positional[0]
	}
	if prefix != "" {
		filtered = filtered.SearchPrefix(prefix)
	} else if !flags["all"] {
		filtered = filtered.LatestPerMajor()
	}
	filtered = filtered.Sorted()

	installed := installedSet(cfg)
	active, _ := manager.Active(cfg)

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "VERSION\tLTS\tDATE\tEND OF SUPPORT\tSTATUS")
	for _, release := range filtered {
		lts := "-"
		if release.LTS.IsLTS {
			lts = release.LTS.Codename
		}
		eol := "-"
		if schedule != nil {
			if end := schedule.EndOfLife(release.Version); end != "" {
				eol = end
			}
		}
		status := "-"
		switch {
		case release.Version == active:
			status = "active"
		case installed[release.Version]:
			status = "installed"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", release.Version, lts, release.Date, eol, status)
	}
	writer.Flush()
	return 0
}

func cmdCurrent(cfg *config.Config) int {
	active, err := manager.Active(cfg)
	if err != nil {
		return fail("%v", err)
	}
	if active == "" {
		fmt.Println("none")
		return 0
	}
	fmt.Println(active)
	return 0
}

func cmdCache(cfg *config.Config, args []string) int {
	sub := "info"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "info":
		count, bytes := manager.CacheInfo(cfg)
		fmt.Printf("%d tarball%s · %s\n", count, plural(count), humanSize(bytes))
		return 0
	case "purge", "clear":
		count, err := manager.PurgeCache(cfg)
		if err != nil {
			return fail("%v", err)
		}
		fmt.Printf("Cache cleared (%d tarball%s)\n", count, plural(count))
		return 0
	default:
		return fail("usage: sono cache [info|purge]")
	}
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func nodeProgress() func(string, int64, int64) {
	tty := isTerminal(os.Stderr)
	lastStage := ""
	return func(stage string, downloaded, total int64) {
		stageChanged := stage != lastStage
		if tty && lastStage == manager.StageDownloading && stageChanged {
			fmt.Fprintln(os.Stderr)
		}
		lastStage = stage
		switch stage {
		case manager.StageDownloading:
			if !tty {
				if stageChanged {
					fmt.Fprintln(os.Stderr, "  downloading…")
				}
				return
			}
			if total > 0 {
				fmt.Fprintf(os.Stderr, "\r  downloading %3d%%", downloaded*100/total)
			} else {
				fmt.Fprint(os.Stderr, "\r  downloading…")
			}
		case manager.StageVerifying:
			fmt.Fprintln(os.Stderr, "  verifying…")
		case manager.StageExtracting:
			fmt.Fprintln(os.Stderr, "  extracting…")
		}
	}
}

func printUsage() {
	fmt.Print(usage)
}

const usage = `sono — Node.js & package manager toolkit

Run without arguments to launch the interactive TUI, or use a subcommand:

Node.js
  sono install <version> [--use]   install a version (e.g. 20, 20.11, v20.11.0, lts, latest)
  sono use <version>               activate an installed version
  sono uninstall <version>         remove an installed version
  sono ls                          list installed versions (* = active)
  sono ls-remote [prefix]          list available versions
                                     flags: --lts, --nonlts, --all
  sono current                     print the active version

Package managers (pnpm, yarn)
  sono pm ls <pm>                  list installed versions
  sono pm ls-remote <pm> [prefix]  list available versions (--all)
  sono pm install <pm> <version> [--use]
  sono pm use <pm> <version>
  sono pm uninstall <pm> <version>

Cache
  sono cache info                  show cached tarball count and size
  sono cache purge                 delete all cached tarballs

Other
  sono path                        print the PATH export line
  sono help                        show this help
`
