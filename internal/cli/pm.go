package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"sophonie/sono/internal/config"
	"sophonie/sono/internal/pkgmgr"
)

const pmRemoteDefaultLimit = 40

func cmdPm(cfg *config.Config, args []string) int {
	if len(args) == 0 {
		return fail("usage: sono pm <ls|ls-remote|install|use|uninstall> <pm> [version]")
	}
	sub := args[0]
	rest := args[1:]

	switch sub {
	case "ls", "list", "installed":
		return pmList(cfg, rest)
	case "ls-remote", "available":
		return pmListRemote(cfg, rest)
	case "install", "i":
		return pmInstall(cfg, rest)
	case "use", "activate":
		return pmUse(cfg, rest)
	case "uninstall", "rm", "remove":
		return pmUninstall(cfg, rest)
	default:
		return fail("unknown pm command: %s", sub)
	}
}

func findPm(name string) (pkgmgr.PackageManager, error) {
	pm, ok := pkgmgr.Find(name)
	if !ok {
		var names []string
		for _, supported := range pkgmgr.Supported {
			names = append(names, supported.Name)
		}
		return pkgmgr.PackageManager{}, fmt.Errorf("unknown package manager %q (supported: %s)", name, strings.Join(names, ", "))
	}
	return pm, nil
}

func pmInstalledSet(cfg *config.Config, pm pkgmgr.PackageManager) map[string]bool {
	set := map[string]bool{}
	installed, err := pkgmgr.Installed(cfg, pm)
	if err != nil {
		return set
	}
	for _, version := range installed {
		set[version] = true
	}
	return set
}

func pmList(cfg *config.Config, args []string) int {
	if len(args) == 0 {
		return fail("usage: sono pm ls <pm>")
	}
	pm, err := findPm(args[0])
	if err != nil {
		return fail("%v", err)
	}
	installed, err := pkgmgr.Installed(cfg, pm)
	if err != nil {
		return fail("%v", err)
	}
	if len(installed) == 0 {
		fmt.Printf("No %s versions installed.\n", pm.Name)
		return 0
	}
	active, _ := pkgmgr.Active(cfg, pm)
	for _, version := range installed {
		marker := " "
		if version == active {
			marker = "*"
		}
		fmt.Printf("%s %s\n", marker, version)
	}
	return 0
}

func pmListRemote(cfg *config.Config, args []string) int {
	flags, positional := splitArgs(args)
	if len(positional) == 0 {
		return fail("usage: sono pm ls-remote <pm> [prefix]")
	}
	pm, err := findPm(positional[0])
	if err != nil {
		return fail("%v", err)
	}
	versions, err := pkgmgr.ListStableVersions(cfg, pm)
	if err != nil {
		return fail("%v", err)
	}

	prefix := ""
	if len(positional) > 1 {
		prefix = positional[1]
	}
	installed := pmInstalledSet(cfg, pm)
	active, _ := pkgmgr.Active(cfg, pm)

	var matched []string
	for _, version := range versions {
		if prefix != "" && !strings.HasPrefix(version, prefix) {
			continue
		}
		matched = append(matched, version)
	}

	limited := prefix == "" && !flags["all"] && len(matched) > pmRemoteDefaultLimit
	shown := matched
	if limited {
		shown = matched[:pmRemoteDefaultLimit]
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, version := range shown {
		status := "-"
		switch {
		case version == active:
			status = "active"
		case installed[version]:
			status = "installed"
		}
		fmt.Fprintf(writer, "%s\t%s\n", version, status)
	}
	writer.Flush()
	if limited {
		fmt.Printf("… showing %d of %d (use --all)\n", pmRemoteDefaultLimit, len(matched))
	}
	return 0
}

func pmInstall(cfg *config.Config, args []string) int {
	flags, positional := splitArgs(args)
	if len(positional) < 2 {
		return fail("usage: sono pm install <pm> <version> [--use]")
	}
	pm, err := findPm(positional[0])
	if err != nil {
		return fail("%v", err)
	}
	versions, err := pkgmgr.ListStableVersions(cfg, pm)
	if err != nil {
		return fail("%v", err)
	}
	version, ok := resolvePmVersion(positional[1], versions)
	if !ok {
		return fail("no %s version matches %q", pm.Name, positional[1])
	}

	if pmInstalledSet(cfg, pm)[version] {
		fmt.Printf("%s %s is already installed\n", pm.Name, version)
	} else {
		fmt.Printf("Installing %s %s\n", pm.Name, version)
		if err := pkgmgr.Install(cfg, pm, version); err != nil {
			return fail("install failed: %v", err)
		}
		fmt.Printf("Installed %s %s\n", pm.Name, version)
	}

	if flags["use"] {
		if err := pkgmgr.Activate(cfg, pm, version); err != nil {
			return fail("could not activate %s %s: %v", pm.Name, version, err)
		}
		fmt.Printf("%s %s is now active\n", pm.Name, version)
	}
	return 0
}

func pmUse(cfg *config.Config, args []string) int {
	if len(args) < 2 {
		return fail("usage: sono pm use <pm> <version>")
	}
	pm, err := findPm(args[0])
	if err != nil {
		return fail("%v", err)
	}
	installed, err := pkgmgr.Installed(cfg, pm)
	if err != nil {
		return fail("%v", err)
	}
	version, ok := resolvePmVersion(args[1], installed)
	if !ok {
		return fail("no installed %s version matches %q", pm.Name, args[1])
	}
	if err := pkgmgr.Activate(cfg, pm, version); err != nil {
		return fail("could not activate %s %s: %v", pm.Name, version, err)
	}
	fmt.Printf("%s %s is now active\n", pm.Name, version)
	return 0
}

func pmUninstall(cfg *config.Config, args []string) int {
	if len(args) < 2 {
		return fail("usage: sono pm uninstall <pm> <version>")
	}
	pm, err := findPm(args[0])
	if err != nil {
		return fail("%v", err)
	}
	installed, err := pkgmgr.Installed(cfg, pm)
	if err != nil {
		return fail("%v", err)
	}
	version, ok := resolvePmVersion(args[1], installed)
	if !ok {
		return fail("no installed %s version matches %q", pm.Name, args[1])
	}
	if err := pkgmgr.Uninstall(cfg, pm, version); err != nil {
		return fail("%v", err)
	}
	fmt.Printf("%s %s removed\n", pm.Name, version)
	return 0
}
