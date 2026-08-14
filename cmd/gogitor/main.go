package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"strconv"

	"golang.org/x/term"

    "gogitor/internal/i18n"
	"gogitor/internal/config"
	"gogitor/internal/logging"
	"gogitor/internal/ui/cli"
	"gogitor/internal/ui/tui"
)

const Version = "0.9.0.3"


func main() {
    cfg, err := config.Load()
    if err != nil {
    	fmt.Fprintf(os.Stderr, "%s\n", i18n.Localize(fmt.Sprintf("warning: config load: %v", err)))
    }

    i18n.SetLang(i18n.Detect())
	args := os.Args[1:]
	args, exit := parseGlobalFlags(args, cfg)
	if exit {
		return
	}

    logger, logPath, err := logging.Init(cfg)
    if err != nil {
    	fmt.Fprintf(os.Stderr, "%s\n", i18n.Localize(fmt.Sprintf("warning: logger init: %v", err)))
    }

	if logger == nil {
		logger = slog.Default()
	}

	if cfg.WorkDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot get current directory: %v\n", err)
			os.Exit(1)
		}
		cfg.WorkDir = wd
	}

    if err := cfg.Validate(); err != nil {
    	fmt.Fprintf(os.Stderr, "%s\n", i18n.Localize(fmt.Sprintf("warning: config validation: %v", err)))
    }

	if len(args) == 0 {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			if err := tui.Run(cfg, logger); err != nil {
				fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
				os.Exit(1)
			}
			return
		}

		task := readStdinTask()
		if task == "" {
			_ = cli.Run([]string{"help"}, cfg, logger, logPath)
			return
		}

		if err := cli.Run([]string{"ask", task}, cfg, logger, logPath); err != nil {
            fmt.Fprintf(os.Stderr, "%s\n", i18n.Localize(err.Error()))
			os.Exit(1)
		}
		return
	}

	first := args[0]

	switch first {
	case "help", "--help", "-h":
		if err := cli.Run([]string{"help"}, cfg, logger, logPath); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		return

	case "version", "--version", "-v":
		fmt.Printf("gogitor %s\n", Version)
		return
	}

	if isKnownCommand(first) {
		if err := cli.Run(args, cfg, logger, logPath); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		return
	}

	if strings.HasPrefix(first, "-") {
		_ = cli.Run([]string{"help"}, cfg, logger, logPath)
		return
	}

	cfg.Provider = first
	if len(args) > 1 {
		cfg.Model = args[1]
	}
	if len(args) > 2 {
		cfg.APIKey = args[2]
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: config validation: %v\n", err)
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		if err := tui.Run(cfg, logger); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	_ = cli.Run([]string{"help"}, cfg, logger, logPath)
}

func isKnownCommand(name string) bool {
    switch name {
    case "tui", "code", "fix", "ask", "analyze", "run", "test", "search",
        "git", "doctor", "help", "task", "file", "decisions", "journal",
        "article", "suggest", "vet", "todo", "computer":
        return true
    }
    return false
}

func parseGlobalFlags(args []string, cfg *config.Config) ([]string, bool) {
	remaining := make([]string, 0, len(args))

	i := 0
	for i < len(args) {
		arg := args[i]

		if !strings.HasPrefix(arg, "-") {
			remaining = append(remaining, args[i:]...)
			break
		}

		name, val, hasVal := splitFlag(arg)

		switch name {
        case "--computer":
            if hasVal {
                cfg.ComputerEnabled = parseBool(val)
            } else {
                cfg.ComputerEnabled = true
            }
            i++
        case "--no-compare":
        	if hasVal {
        		cfg.CompareApproaches = !parseBool(val)
        	} else {
        		cfg.CompareApproaches = false
        	}
        	i++
		case "--auto-search":
			if hasVal {
				cfg.AutoSearch = parseBool(val)
			} else {
				cfg.AutoSearch = true
			}
			i++
		case "--version", "-v":
			fmt.Printf("gogitor %s\n", Version)
			return nil, true

		case "--help", "-h":
			return []string{"help"}, false

		case "--debug":
			if hasVal {
				cfg.Debug = parseBool(val)
			} else {
				cfg.Debug = true
			}

			if cfg.Debug {
				cfg.LogLevel = "debug"
			}

			i++

		case "--dry-run":
			if hasVal {
				cfg.DryRun = parseBool(val)
			} else {
				cfg.DryRun = true
			}

			i++

		case "--raw":
			if hasVal {
				cfg.Raw = parseBool(val)
			} else {
				cfg.Raw = true
			}
			i++

		case "--pretty":
			pretty := true
			if hasVal {
				pretty = parseBool(val)
			}
			if pretty {
				cfg.Raw = false
			}
			i++

		case "--provider", "-p":
			v, next, ok := flagValue(args, i, hasVal, val, name)
			if !ok {
				return nil, true
			}
			cfg.Provider = v
			i = next

		case "--model", "-m":
			v, next, ok := flagValue(args, i, hasVal, val, name)
			if !ok {
				return nil, true
			}
			cfg.Model = v
			i = next

		case "--key", "-k":
			v, next, ok := flagValue(args, i, hasVal, val, name)
			if !ok {
				return nil, true
			}
			cfg.APIKey = v
			i = next

		case "--repo", "-r":
			v, next, ok := flagValue(args, i, hasVal, val, name)
			if !ok {
				return nil, true
			}
			cfg.WorkDir = v
			i = next
    	case "--github":
    		v, next, ok := flagValue(args, i, hasVal, val, name)
    		if !ok {
    			return nil, true
    		}
    		cfg.GitHubURL = v
    		i = next
    	case "--key-github", "--key_github":
    		v, next, ok := flagValue(args, i, hasVal, val, name)
    		if !ok {
    			return nil, true
    		}
    		cfg.GitHubToken = v
    		i = next
        case "--max-context":
            v, next, ok := flagValue(args, i, hasVal, val, name)
            if !ok {
                return nil, true
            }
            if n, err := strconv.Atoi(v); err == nil && n > 0 {
                cfg.MaxContextTokens = n
            }
            i = next
		default:
			// Останавливаем разбор глобальных флагов.
			// Дальше флаги будет разбирать конкретная подкоманда.
			remaining = append(remaining, args[i:]...)
			return remaining, false
		}
	}

	return remaining, false
}

func splitFlag(arg string) (name string, value string, hasValue bool) {
	if idx := strings.Index(arg, "="); idx != -1 {
		return arg[:idx], arg[idx+1:], true
	}

	return arg, "", false
}

func flagValue(args []string, i int, hasValue bool, value string, flagName string) (string, int, bool) {
	if hasValue {
		return value, i + 1, true
	}

	if i+1 >= len(args) {
		fmt.Fprintf(os.Stderr, "error: flag %s requires a value\n", flagName)
		return "", 0, false
	}

	return args[i+1], i + 2, true
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func readStdinTask() string {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return ""
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}
