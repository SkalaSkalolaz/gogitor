package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"gogitor/internal/config"
	"gogitor/internal/i18n"
	"gogitor/internal/llama"
	"gogitor/internal/logging"
	"gogitor/internal/ui/tui"
)

const Version = "2.1.14"

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

	// ─── llama-server lifecycle ──────────────────────────────────
	var llamaMgr *llama.Manager

	if cfg.IsLlamaProvider() {
		if cfg.Model == "" || cfg.Model == cfg.LlamaBinPath {
			fmt.Fprintln(os.Stderr,
				"error: --provider llama requires --model to be a path to a .gguf file")
			os.Exit(2)
		}

		fmt.Fprintf(os.Stderr,
			"Starting llama-server with model: %s\n", cfg.Model)
		fmt.Fprintln(os.Stderr,
			"Note: loading a large model can take 1–3 minutes; progress is written to .gogitor/llama-server.log")

		llamaMgr = llama.New(llama.Config{
			BinPath:   cfg.LlamaBinPath,
			ModelPath: cfg.Model,
			Host:      cfg.LlamaHost,
			Port:      cfg.LlamaPort,
			ExtraArgs: cfg.LlamaArgs,
			LogDir:    cfg.WorkDir,
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := llamaMgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot start llama-server: %v\n", err)
			os.Exit(1)
		}

		// Подменяем провайдера на OpenAI-совместимый.
		// LLM-клиент уже умеет работать с openai-compatible+URL.
		originalModel := cfg.Model

		cfg.Provider = "openai-compatible+" + llamaMgr.BaseURL()
		cfg.Model = llama.Alias
    	cfg.LlamaManaged = true 
		if cfg.APIKey == "" {
			cfg.APIKey = "no-key-required"
		}

		// Оставляем человекочитаемые имена для TUI.
		cfg.DisplayProvider = "llama"
		cfg.DisplayModel    = filepath.Base(originalModel)

		// Синхронизируем окно контекста с тем, что задано в -c для llama-server.
		// Иначе Gogitor будет пытаться отправить промпт длиннее, чем может принять сервер.
		if cs := llamaMgr.ContextSize(); cs > 0 {
			cfg.MaxContextTokens = cs
		}

		// Гарантированная остановка по SIGINT/SIGTERM и в defer.
		defer llamaMgr.Stop()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			llamaMgr.Stop()
			os.Exit(0)
		}()
	}
	// ─────────────────────────────────────────────────────────────

	if options.SaveConfig {
		saved := *cfg
		saved.LlamaManaged = false

		if llamaMgr != nil {
			saved.Provider = "llama"
		}
		if err := saved.Save(); err != nil {
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