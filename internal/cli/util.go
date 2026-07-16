package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"sophonie/sono/internal/nodedist"
)

func fail(format string, args ...any) int {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	return 1
}

func humanSize(bytes int64) string {
	if bytes <= 0 {
		return "0 MB"
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func splitArgs(args []string) (map[string]bool, []string) {
	flags := map[string]bool{}
	var positional []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "--") {
			flags[strings.TrimPrefix(arg, "--")] = true
			continue
		}
		positional = append(positional, arg)
	}
	return flags, positional
}

func resolveRemoteNode(index nodedist.Index, input string) (string, error) {
	input = strings.TrimSpace(input)
	switch {
	case input == "":
		return "", fmt.Errorf("no version given")
	case strings.EqualFold(input, "lts"):
		version := index.LatestLTS()
		if version == "" {
			return "", fmt.Errorf("no LTS version found")
		}
		return version, nil
	case strings.EqualFold(input, "latest"):
		sorted := index.Sorted()
		if len(sorted) == 0 {
			return "", fmt.Errorf("version index is empty")
		}
		return sorted[0].Version, nil
	}
	matches := index.SearchPrefix(input).Sorted()
	if len(matches) == 0 {
		return "", fmt.Errorf("no Node.js version matches %q", input)
	}
	return matches[0].Version, nil
}

func resolveInstalledNode(installed []string, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("no version given")
	}
	var index nodedist.Index
	for _, version := range installed {
		index = append(index, nodedist.Release{Version: version})
	}
	matches := index.SearchPrefix(input).Sorted()
	if len(matches) == 0 {
		return "", fmt.Errorf("no installed version matches %q", input)
	}
	return matches[0].Version, nil
}

func resolvePmVersion(prefix string, versions []string) (string, bool) {
	prefix = strings.TrimSpace(prefix)
	best := ""
	for _, version := range versions {
		if prefix != "" && !strings.HasPrefix(version, prefix) {
			continue
		}
		if best == "" || compareSemver(version, best) > 0 {
			best = version
		}
	}
	return best, best != ""
}

func compareSemver(a, b string) int {
	partsA := strings.SplitN(a, ".", 3)
	partsB := strings.SplitN(b, ".", 3)
	for i := 0; i < 3; i++ {
		var x, y int
		if i < len(partsA) {
			x, _ = strconv.Atoi(partsA[i])
		}
		if i < len(partsB) {
			y, _ = strconv.Atoi(partsB[i])
		}
		if x != y {
			return x - y
		}
	}
	return 0
}
