package main

import (
	"log"
	"os"

	"sophonie/sono/internal/cli"
	"sophonie/sono/internal/config"
	"sophonie/sono/internal/manager"
	"sophonie/sono/internal/tui"
)

func main() {
	cfg, err := config.New()
	if err != nil {
		log.Fatal(err)
	}

	if settings := config.LoadSettings(cfg); settings.AutoPurgeEnabled && settings.CacheMaxAgeDays > 0 {
		_, _ = manager.PurgeExpired(cfg, settings.CacheMaxAgeDays)
	}

	if args := os.Args[1:]; len(args) > 0 {
		os.Exit(cli.Run(cfg, args))
	}

	if err := tui.Run(cfg); err != nil {
		log.Fatal(err)
	}
}
