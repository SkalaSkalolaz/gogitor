package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gogitor/internal/config"
	"gogitor/internal/i18n"
	"gogitor/internal/logging"
	"gogitor/internal/ui/tui"
)

const Version = "2.0.0"

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: config load: %v\n", err)
	}

	i18n.SetLang(i18n.Detect())

	options, err := config.ParseLaunchArgs(cfg, os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "Gogitor: %v\n\n", err)
		config.PrintLaunchUsage(os.Stderr)
		os.Exit(2)
	}

	if options.Version {
		fmt.Printf("Gogitor %s\n", Version)
		return
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: config validation: %v\n", err)
	}

	if cfg.WorkDir == "" {
		wd, wdErr := os.Getwd()
		if wdErr != nil {
			fmt.Fprintf(os.Stderr, "error: cannot get current directory: %v\n", wdErr)
			os.Exit(1)
		}
		cfg.WorkDir = filepath.Clean(wd)
	}

	if options.SaveConfig {
		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot save config: %v\n", err)
		}
	}

	logger, _, err := logging.Init(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: logger init: %v\n", err)
	}
	if logger == nil {
		logger = slog.Default()
	}

	if err := tui.Run(cfg, logger); err != nil {
		fmt.Fprintf(os.Stderr, "Gogitor TUI error: %v\n", err)
		os.Exit(1)
	}
}
