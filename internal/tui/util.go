package tui

import (
	"fmt"
	"time"

	"sophonie/sono/internal/manager"
	"sophonie/sono/internal/nodedist"
)

func humanSize(bytes int64) string {
	if bytes <= 0 {
		return "0 MB"
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
}

func isExpired(endDate string) bool {
	if endDate == "" {
		return false
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return false
	}
	return end.Before(time.Now())
}

func stageLabel(stage string) string {
	switch stage {
	case manager.StageDownloading:
		return "Downloading"
	case manager.StageVerifying:
		return "Verifying"
	case manager.StageExtracting:
		return "Extracting"
	default:
		return "Preparing"
	}
}

func clampMaxAge(days int) int {
	if days < 1 {
		return 1
	}
	if days > 3650 {
		return 3650
	}
	return days
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func onlyInstalled(index nodedist.Index, installed map[string]bool) nodedist.Index {
	var out nodedist.Index
	for _, release := range index {
		if installed[release.Version] {
			out = append(out, release)
		}
	}
	return out
}
