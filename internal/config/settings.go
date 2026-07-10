package config

import (
	"encoding/json"
	"os"
)

type Settings struct {
	AutoPurgeEnabled bool `json:"autoPurgeEnabled"`
	CacheMaxAgeDays  int  `json:"cacheMaxAgeDays"`
}

func DefaultSettings() Settings {
	return Settings{AutoPurgeEnabled: true, CacheMaxAgeDays: 30}
}

func LoadSettings(cfg *Config) Settings {
	data, err := os.ReadFile(cfg.SettingsFile)
	if err != nil {
		return DefaultSettings()
	}
	settings := DefaultSettings()
	if err := json.Unmarshal(data, &settings); err != nil {
		return DefaultSettings()
	}
	return settings
}

func SaveSettings(cfg *Config, settings Settings) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfg.SettingsFile, data, 0o644)
}
