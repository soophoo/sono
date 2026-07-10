package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Config struct {
	Root           string
	VersionsDir    string
	CacheDir       string
	CurrentSymlink string
	IndexCache     string
	ScheduleCache  string
	SettingsFile   string
	PmDir          string
	ShimsDir       string
}

func New() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolving home directory: %w", err)
	}

	root := filepath.Join(home, ".sono")
	cfg := &Config{
		Root:           root,
		VersionsDir:    filepath.Join(root, "versions"),
		CacheDir:       filepath.Join(root, "cache"),
		CurrentSymlink: filepath.Join(root, "current"),
		IndexCache:     filepath.Join(root, "index.json"),
		ScheduleCache:  filepath.Join(root, "schedule.json"),
		SettingsFile:   filepath.Join(root, "config.json"),
		PmDir:          filepath.Join(root, "pm"),
		ShimsDir:       filepath.Join(root, "shims"),
	}

	for _, dir := range []string{cfg.VersionsDir, cfg.CacheDir, cfg.PmDir, cfg.ShimsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating %s: %w", dir, err)
		}
	}

	return cfg, nil
}

var platformByOSArch = map[string]string{
	"linux/amd64":   "linux-x64",
	"linux/arm64":   "linux-arm64",
	"darwin/arm64":  "darwin-arm64",
	"windows/amd64": "win-x64",
}

func Platform() (string, error) {
	key := runtime.GOOS + "/" + runtime.GOARCH
	platform, ok := platformByOSArch[key]
	if !ok {
		return "", fmt.Errorf("unsupported platform: %s", key)
	}
	return platform, nil
}
