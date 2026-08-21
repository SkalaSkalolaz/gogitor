package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

    "gogitor/internal/i18n"
	"gogitor/internal/app"
	"gogitor/internal/config"
	"gogitor/internal/domain"
	"gogitor/internal/ui/tui"
)

type flagSpec struct {
	Name  string
	Short string
	Bool  bool
}

var commonFlagSpecs = []flagSpec{
	{Name: "provider", Short: "p"},
	{Name: "model", Short: "m"},
    {Name: "output", Short: "o"},
	{Name: "key", Short: "k"},
	{Name: "repo", Short: "r"},
	{Name: "github"},
	{Name: "key-github"},
	{Name: "key_github"},
    {Name: "max-context"},
	{Name: "debug", Bool: true},
	{Name: "raw", Bool: true},
	{Name: "pretty", Bool: true},
	{Name: "auto-search", Bool: true},
	{Name: "help", Short: "h", Bool: true},
	{Name: "computer", Bool: true},
    {Name: "reasoning", Bool: true},
    {Name: "reasoning-effort"},
    {Name: "reasoning-budget"},
    {Name: "reasoning-show", Bool: true},
	{Name: "reasoning-router", Bool: true},
}

func buildFlagSpecs(extra ...flagSpec) []flagSpec {
	out := make([]flagSpec, 0, len(commonFlagSpecs)+len(extra))
	out = append(out, commonFlagSpecs...)
	out = append(out, extra...)
	return out
}

type commonFlags struct {
	provider *string
	model    *string
	key      *string
	repo     *string
	githubURL *string
	githubKey *string
	debug    *bool
	raw      *bool
	pretty   *bool
	help     *bool
	maxCtx    *int
    autoSearch *bool
	output   *string
    computer *bool
    reasoning       *bool
    reasoningEffort *string
    reasoningBudget *int
    reasoningShow   *bool
    reasoningRouter *bool
}

func Run(args []string, cfg *config.Config, log *slog.Logger, logPath string) error {
	if len(args) == 0 {
		return runTUI(nil, cfg, log)
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
    case "article":
        return runArticle(rest, cfg, log)
	case "suggest":
		return runSuggest(rest, cfg, log)
	case "vet":
		return runVet(rest, cfg, log)
	case "todo":
		return runTODO(rest, cfg, log)
	case "computer":
		return runComputer(rest, cfg, log)
	case "tui":
		return runTUI(rest, cfg, log)

	case "code":
		return runCode(rest, cfg, log)

	case "fix":                            
		return runFix(rest, cfg, log)    
	case "ask":
		return runAsk(rest, cfg, log)

	case "analyze":
		return runAnalyze(rest, cfg, log)

	case "search":
		return runSearch(rest, cfg, log)

	case "run":
		return runRun(rest, cfg, log)

	case "test":
		return runTest(rest, cfg, log)

	case "git":
		return runGit(rest, cfg, log)

	case "task", "file":
		return runTaskFile(rest, cfg, log)

	case "doctor":
		return runDoctor(rest, cfg, logPath)

	case "help":
		printHelp()
		return nil
	case "decisions", "journal":
		return runDecisions(rest, cfg, log)
	default:
		printHelp()
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func runComputer(args []string, cfg *config.Config, log *slog.Logger) error {
	fs := flag.NewFlagSet("computer", flag.ContinueOnError)
	common := registerCommonFlags(fs, cfg)
	jsonOut := fs.Bool("json", false, "print result as JSON")
	dryRun := fs.Bool("dry-run", false, "show plan without executing")
	allowSudo := fs.Bool("allow-sudo", false, "allow sudo commands")
	specs := buildFlagSpecs(
		flagSpec{Name: "json", Bool: true},
		flagSpec{Name: "dry-run", Bool: true},
		flagSpec{Name: "allow-sudo", Bool: true},
	)
	flags, positional, err := reorderArgs(args, specs)
	if err != nil {
		return err
	}
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if common.help != nil && *common.help {
		printHelp()
		return nil
	}
	task := taskFromArgsOrStdin(positional)
	if task == "" {
		return fmt.Errorf("usage: gogitor computer <task> [--dry-run] [--allow-sudo] [flags]")
	}
	applyCommonFlags(common, cfg)
	if *allowSudo {
		cfg.ComputerAllowSudo = true
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	raw := rawEnabled(common, cfg)
	svc := app.New(cfg, log)
	defer svc.Close()

	if *dryRun {
		// Dry-run: только показываем план
		fmt.Fprintf(os.Stderr, "[dry-run] Computer mode plan for: %s\n", task)
		fmt.Fprintf(os.Stderr, "[dry-run] OS: %s %s, pkg: %s\n",
			svc.ComputerOS.OS, svc.ComputerOS.Distro, svc.ComputerOS.PkgManager)
		fmt.Fprintln(os.Stderr, "[dry-run] No commands will be executed.")
		return nil
	}

	res := svc.ExecuteComputer(context.Background(), task, emitCLI(*jsonOut, raw))
	printResult(res, *jsonOut, raw)
	saveErr := saveOutputIfRequested(res, cfg)
	if !res.Success {
		return fmt.Errorf("computer task failed")
	}
	return saveErr
}

func runArticle(args []string, cfg *config.Config, log *slog.Logger) error {
	fs := flag.NewFlagSet("article", flag.ContinueOnError)
	common := registerCommonFlags(fs, cfg)
	jsonOut := fs.Bool("json", false, "print result as JSON")
	fullMode := fs.Bool("full", false, "complex article mode (multi-section)")
	specs := buildFlagSpecs(
		flagSpec{Name: "json", Bool: true},
		flagSpec{Name: "full", Bool: true},
	)
	flags, positional, err := reorderArgs(args, specs)
	if err != nil {
		return err
	}
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if common.help != nil && *common.help {
		printHelp()
		return nil
	}
	topic := taskFromArgsOrStdin(positional)
	if topic == "" {
		return fmt.Errorf("usage: gogitor article <topic> [--full] [flags]")
	}
	applyCommonFlags(common, cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}
	raw := rawEnabled(common, cfg)
	svc := app.New(cfg, log)
	defer svc.Close()

	mode := app.ArticleModeSimple
	if *fullMode {
		mode = app.ArticleModeComplex
	}

	res := svc.Article(context.Background(), topic, app.ArticleOptions{Mode: mode}, emitCLI(*jsonOut, raw))
    printResult(res, *jsonOut, raw)
	saveErr := saveOutputIfRequested(res, cfg)
	if !res.Success {
		return fmt.Errorf("article generation failed")
	}
	return saveErr
}


func runTUI(args []string, cfg *config.Config, log *slog.Logger) error {
	if args == nil {
		args = []string{}
	}

	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	common := registerCommonFlags(fs, cfg)

	flags, _, err := reorderArgs(args, buildFlagSpecs())
	if err != nil {
		return err
	}

	if err := fs.Parse(flags); err != nil {
		return err
	}

	if common.help != nil && *common.help {
		printHelp()
		return nil
	}

	applyCommonFlags(common, cfg)

	if err := cfg.Validate(); err != nil {
		return err
	}

	return tui.Run(cfg, log)
}

func runTaskFile(args []string, cfg *config.Config, log *slog.Logger) error {
	fs := flag.NewFlagSet("task", flag.ContinueOnError)
	common := registerCommonFlags(fs, cfg)

	jsonOut := fs.Bool("json", false, "print result as JSON")
	codeOnly := fs.Bool("code", false, "force code mode instead of automatic intent detection")

	specs := buildFlagSpecs(
		flagSpec{Name: "json", Bool: true},
		flagSpec{Name: "code", Bool: true},
	)

	flags, positional, err := reorderArgs(args, specs)
	if err != nil {
		return err
	}

	if err := fs.Parse(flags); err != nil {
		return err
	}

	if common.help != nil && *common.help {
		printHelp()
		return nil
	}

	if len(positional) == 0 {
		return fmt.Errorf("usage: gogitor task <path/to/file.txt|file.md> [--code] [--json] [flags]")
	}

	applyCommonFlags(common, cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}

	task, err := readTaskFile(expandHome(positional[0]))
	if err != nil {
		return err
	}

	raw := rawEnabled(common, cfg)

	svc := app.New(cfg, log)
	defer svc.Close()

	var res domain.Result
	if *codeOnly {
		res = svc.ExecuteCode(context.Background(), task, app.Options{}, emitCLI(*jsonOut, raw))
	} else {
		res = svc.ProcessEvents(context.Background(), task, emitCLI(*jsonOut, raw))
	}
    printResult(res, *jsonOut, raw)
	saveErr := saveOutputIfRequested(res, cfg)
	if !res.Success {
		return fmt.Errorf("task file failed")
	}
	return saveErr
}


func readTaskFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cannot read task file: %w", err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("task file path is a directory: %s", path)
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".txt" && ext != ".md" {
		return "", fmt.Errorf("task file must have .txt or .md extension: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read task file: %w", err)
	}

	const maxTaskFileSize = 1 << 20 // 1 MB
	if len(data) > maxTaskFileSize {
		return "", fmt.Errorf("task file is too large (%d bytes), max %d bytes", len(data), maxTaskFileSize)
	}

	task := string(data)

	// Удаляем возможный UTF-8 BOM.
	task = strings.TrimPrefix(task, "\ufeff")
	task = strings.TrimSpace(task)

	if task == "" {
		return "", fmt.Errorf("task file is empty: %s", path)
	}

	return task, nil
}

func runCode(args []string, cfg *config.Config, log *slog.Logger) error {
	fs := flag.NewFlagSet("code", flag.ContinueOnError)
	common := registerCommonFlags(fs, cfg)

	dryRun := fs.Bool("dry-run", cfg.DryRun, "validate changes but do not apply")
	noCommit := fs.Bool("no-commit", false, "disable automatic git commit")
	noTests := fs.Bool("no-tests", false, "skip tests")
	jsonOut := fs.Bool("json", false, "print result as JSON")
	noCompare := fs.Bool("no-compare", false, "skip approach comparison for complex tasks")
	mode := fs.String("mode", "", "execution mode: auto|fast|agent|workflow")
	fastMode := fs.Bool("fast", false, "force fast mode")
	agentMode := fs.Bool("agent", false, "force multi-agent mode")
	workflowMode := fs.Bool("workflow", false, "force workflow mode")
	workflowDir := fs.String("workflow-dir", cfg.WorkflowDir, "directory for workflow artifacts, e.g. docs/workflow")

	specs := buildFlagSpecs(
		flagSpec{Name: "dry-run", Bool: true},
		flagSpec{Name: "no-commit", Bool: true},
		flagSpec{Name: "no-tests", Bool: true},
		flagSpec{Name: "no-compare", Bool: true},
		flagSpec{Name: "json", Bool: true},
		flagSpec{Name: "mode"},
		flagSpec{Name: "fast", Bool: true},
		flagSpec{Name: "agent", Bool: true},
		flagSpec{Name: "workflow", Bool: true},
		flagSpec{Name: "workflow-dir"},
	)

	flags, positional, err := reorderArgs(args, specs)
	if err != nil {
		return err
	}

	if err := fs.Parse(flags); err != nil {
		return err
	}

	if common.help != nil && *common.help {
		printHelp()
		return nil
	}

	task := taskFromArgsOrStdin(positional)
	if task == "" {
		return fmt.Errorf("usage: gogitor code <task> [--provider <name>] [--model <model>] [--key <key>] [--repo <path>]")
	}

	applyCommonFlags(common, cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}

	raw := rawEnabled(common, cfg)

	svc := app.New(cfg, log)
	defer svc.Close()

	opts := app.Options{
		DryRun:    *dryRun,
		NoCommit:  *noCommit,
		NoTests:   *noTests,
		NoCompare: *noCompare,
		Mode:      *mode,
		WorkflowDir: *workflowDir,
	}

	if *fastMode {
		opts.Mode = "fast"
	}

	if *agentMode {
		opts.Mode = "agent"
	}

	if *workflowMode {
		opts.Mode = "workflow"
	}

	res := svc.ExecuteCode(context.Background(), task, opts, emitCLI(*jsonOut, raw))

    printResult(res, *jsonOut, raw)
	saveErr := saveOutputIfRequested(res, cfg)
	if !res.Success {
		return fmt.Errorf("code task failed")
	}
	return saveErr
}

func runAsk(args []string, cfg *config.Config, log *slog.Logger) error {
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	common := registerCommonFlags(fs, cfg)
	jsonOut := fs.Bool("json", false, "print result as JSON")
	imagePath := fs.String("image", "", "path to image file for vision analysis")
	specs := buildFlagSpecs(
		flagSpec{Name: "json", Bool: true},
		flagSpec{Name: "image"},
	)
	flags, positional, err := reorderArgs(args, specs)
	if err != nil {
		return err
	}
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if common.help != nil && *common.help {
		printHelp()
		return nil
	}
	query := taskFromArgsOrStdin(positional)
	if query == "" {
		return fmt.Errorf("usage: gogitor ask <question> [--image <path>] [flags]")
	}
	applyCommonFlags(common, cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}
	raw := rawEnabled(common, cfg)
	svc := app.New(cfg, log)
	defer svc.Close()

	var res domain.Result
	if *imagePath != "" {
		images, err := app.ReadImageFile(*imagePath)
		if err != nil {
			return fmt.Errorf("cannot read image: %w", err)
		}
		res = svc.AnalyzeWithImages(context.Background(), query, images, emitCLI(*jsonOut, raw))
	} else {
		res = svc.Chat(context.Background(), query, emitCLI(*jsonOut, raw))
	}
	printResult(res, *jsonOut, raw)
	saveErr := saveOutputIfRequested(res, cfg)
	if !res.Success {
		return fmt.Errorf("ask failed")
	}
	return saveErr
}

func runAnalyze(args []string, cfg *config.Config, log *slog.Logger) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	common := registerCommonFlags(fs, cfg)
	jsonOut := fs.Bool("json", false, "print result as JSON")
	imagePath := fs.String("image", "", "path to image file for vision analysis")
	specs := buildFlagSpecs(
		flagSpec{Name: "json", Bool: true},
		flagSpec{Name: "image"},
	)
	flags, positional, err := reorderArgs(args, specs)
	if err != nil {
		return err
	}
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if common.help != nil && *common.help {
		printHelp()
		return nil
	}
	query := taskFromArgsOrStdin(positional)
	if query == "" {
		return fmt.Errorf("usage: gogitor analyze <question> [--image <path>] [flags]")
	}
	applyCommonFlags(common, cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}
	raw := rawEnabled(common, cfg)
	svc := app.New(cfg, log)
	defer svc.Close()

	var res domain.Result
	if *imagePath != "" {
		images, err := app.ReadImageFile(*imagePath)
		if err != nil {
			return fmt.Errorf("cannot read image: %w", err)
		}
		res = svc.AnalyzeWithImages(context.Background(), query, images, emitCLI(*jsonOut, raw))
	} else {
		res = svc.Analyze(context.Background(), query, emitCLI(*jsonOut, raw))
	}
	printResult(res, *jsonOut, raw)
	saveErr := saveOutputIfRequested(res, cfg)
	if !res.Success {
		return fmt.Errorf("analyze failed")
	}
	return saveErr
}

func runSearch(args []string, cfg *config.Config, log *slog.Logger) error {
	return runQueryCommand(
		"search",
		args,
		cfg,
		log,
		"usage: gogitor search <query> [--provider <name>] [--model <model>] [--key <key>]",
		func(svc *app.Service, ctx context.Context, query string, emit func(domain.Event)) domain.Result {
			return svc.SearchAnswer(ctx, query, emit)
		},
	)
}

func runQueryCommand(
	name string,
	args []string,
	cfg *config.Config,
	log *slog.Logger,
	usage string,
	run func(svc *app.Service, ctx context.Context, query string, emit func(domain.Event)) domain.Result,
) error {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	common := registerCommonFlags(fs, cfg)

	jsonOut := fs.Bool("json", false, "print result as JSON")

	specs := buildFlagSpecs(
		flagSpec{Name: "json", Bool: true},
	)

	flags, positional, err := reorderArgs(args, specs)
	if err != nil {
		return err
	}

	if err := fs.Parse(flags); err != nil {
		return err
	}

	if common.help != nil && *common.help {
		printHelp()
		return nil
	}

	query := taskFromArgsOrStdin(positional)
	if query == "" {
		return fmt.Errorf("%s", usage)
	}

	applyCommonFlags(common, cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}

	raw := rawEnabled(common, cfg)

	svc := app.New(cfg, log)
	defer svc.Close()

	res := run(svc, context.Background(), query, emitCLI(*jsonOut, raw))

	printResult(res, *jsonOut, raw)
    saveErr := saveOutputIfRequested(res, cfg)
	if !res.Success {
		return fmt.Errorf("%s failed", name)
	}
	return saveErr
}

func runSuggest(args []string, cfg *config.Config, log *slog.Logger) error {
	return runQueryCommand(
		"suggest", args, cfg, log,
		"usage: gogitor suggest [flags]",
		func(svc *app.Service, ctx context.Context, _ string, emit func(domain.Event)) domain.Result {
			return svc.Suggest(ctx, emit)
		},
	)
}

func runVet(args []string, cfg *config.Config, log *slog.Logger) error {
	return runQueryCommand(
		"vet", args, cfg, log,
		"usage: gogitor vet [flags]",
		func(svc *app.Service, ctx context.Context, _ string, emit func(domain.Event)) domain.Result {
			return svc.RunVet(ctx, emit)
		},
	)
}

func runTODO(args []string, cfg *config.Config, log *slog.Logger) error {
	return runQueryCommand(
		"todo", args, cfg, log,
		"usage: gogitor todo [flags]",
		func(svc *app.Service, ctx context.Context, _ string, emit func(domain.Event)) domain.Result {
			return svc.ScanTODO(ctx, emit)
		},
	)
}

func runRun(args []string, cfg *config.Config, log *slog.Logger) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	common := registerCommonFlags(fs, cfg)

	jsonOut := fs.Bool("json", false, "print result as JSON")

	specs := buildFlagSpecs(
		flagSpec{Name: "json", Bool: true},
	)

	flags, positional, err := reorderArgs(args, specs)
	if err != nil {
		return err
	}

	if err := fs.Parse(flags); err != nil {
		return err
	}

	if common.help != nil && *common.help {
		printHelp()
		return nil
	}

	file := ""
	if len(positional) > 0 {
		file = positional[0]
	}

	applyCommonFlags(common, cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}

	raw := rawEnabled(common, cfg)

	svc := app.New(cfg, log)
	defer svc.Close()

	res := svc.RunFile(context.Background(), file, emitCLI(*jsonOut, raw))

	printResult(res, *jsonOut, raw)
    saveErr := saveOutputIfRequested(res, cfg)
	if !res.Success {
		return fmt.Errorf("run failed")
	}
	return saveErr
}

func runTest(args []string, cfg *config.Config, log *slog.Logger) error {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	common := registerCommonFlags(fs, cfg)
	jsonOut := fs.Bool("json", false, "print result as JSON")
	specs := buildFlagSpecs(
		flagSpec{Name: "json", Bool: true},
	)
	flags, positional, err := reorderArgs(args, specs)
	if err != nil {
		return err
	}
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if common.help != nil && *common.help {
		printHelp()
		return nil
	}
	applyCommonFlags(common, cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}
	raw := rawEnabled(common, cfg)
	svc := app.New(cfg, log)
	defer svc.Close()

	var res domain.Result
	if len(positional) > 0 && strings.ToLower(positional[0]) == "lint" {
		res = svc.RunLint(context.Background(), emitCLI(*jsonOut, raw))
	} else {
		res = svc.RunTests(context.Background(), emitCLI(*jsonOut, raw))
	}
	printResult(res, *jsonOut, raw)
    saveErr := saveOutputIfRequested(res, cfg)
	if !res.Success {
		return fmt.Errorf("tests failed")
	}
	return saveErr
}

func runGit(args []string, cfg *config.Config, log *slog.Logger) error {
	fs := flag.NewFlagSet("git", flag.ContinueOnError)
	common := registerCommonFlags(fs, cfg)

	jsonOut := fs.Bool("json", false, "print result as JSON")

	specs := buildFlagSpecs(
		flagSpec{Name: "json", Bool: true},
	)

	flags, positional, err := reorderArgs(args, specs)
	if err != nil {
		return err
	}

	if err := fs.Parse(flags); err != nil {
		return err
	}

	if common.help != nil && *common.help {
		printHelp()
		return nil
	}

	sub := "status"
	if len(positional) > 0 {
		sub = positional[0]
	}

	applyCommonFlags(common, cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}

	raw := rawEnabled(common, cfg)

	svc := app.New(cfg, log)
	defer svc.Close()

	var res domain.Result

	switch strings.ToLower(sub) {

	case "pr":
		res = svc.GitPR(context.Background(), emitCLI(*jsonOut, raw))
	case "issue", "issues":
		res = svc.GitIssue(context.Background(), emitCLI(*jsonOut, raw))
	case "changelog":
		res = svc.GitChangelog(context.Background(), emitCLI(*jsonOut, raw))
	case "pr-comment":
		res = svc.GitPRComment(context.Background(), positional[1:], emitCLI(*jsonOut, raw))
	case "status":
		res = svc.GitStatus(context.Background(), emitCLI(*jsonOut, raw))
	case "diff":
		res = svc.GitDiff(context.Background(), emitCLI(*jsonOut, raw))
    case "commit":
        splitFiles, hasSplit := app.ParseCommitSplitArgs(positional[1:])
		if hasSplit {
			res = svc.GitCommitSplit(context.Background(), splitFiles, emitCLI(*jsonOut, raw))
		} else {
			res = svc.GitCommit(context.Background(), emitCLI(*jsonOut, raw))
		}
	case "init":
		res = svc.GitInit(context.Background(), emitCLI(*jsonOut, raw))
	case "log":
		res = svc.GitLog(context.Background(), emitCLI(*jsonOut, raw))
    case "create":
        res = svc.GitCreate(context.Background(), positional[1:], emitCLI(*jsonOut, raw))

    case "checkout":
    	res = svc.GitCheckout(context.Background(), positional[1:], emitCLI(*jsonOut, raw))
	case "branch":
		res = svc.GitBranch(context.Background(), positional[1:], emitCLI(*jsonOut, raw))
	case "merge":
		branch := ""
		if len(positional) > 1 {
			branch = positional[1]
		}
		res = svc.GitMerge(context.Background(), branch, emitCLI(*jsonOut, raw))
	case "revert":
		res = svc.GitRevert(context.Background(), positional[1:], emitCLI(*jsonOut, raw))
	case "reset":
		res = svc.GitReset(context.Background(), positional[1:], emitCLI(*jsonOut, raw))
	case "push":
		branch := ""
		if len(positional) > 1 {
			branch = positional[1]
		}
		res = svc.GitPush(context.Background(), branch, emitCLI(*jsonOut, raw))
	case "pull":
		branch := ""
		if len(positional) > 1 {
			branch = positional[1]
		}
		res = svc.GitPull(context.Background(), branch, emitCLI(*jsonOut, raw))
	case "fetch":
		res = svc.GitFetch(context.Background(), emitCLI(*jsonOut, raw))
	case "clone":
		url := ""
		if len(positional) > 1 {
			url = positional[1]
		}
		res = svc.GitClone(context.Background(), url, emitCLI(*jsonOut, raw))
	case "remote":
		res = svc.GitRemote(context.Background(), positional[1:], emitCLI(*jsonOut, raw))
	default:
		return fmt.Errorf("unknown git subcommand: %s (supported: status, diff, commit, init, log, checkout, branch, merge, revert, reset, push, pull, fetch, clone, remote)", sub)
	}
	printResult(res, *jsonOut, raw)
    saveErr := saveOutputIfRequested(res, cfg)
	if !res.Success {
		return fmt.Errorf("git command failed")
	}
	return saveErr
}

func runDoctor(args []string, cfg *config.Config, logPath string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	common := registerCommonFlags(fs, cfg)

	flags, _, err := reorderArgs(args, buildFlagSpecs())
	if err != nil {
		return err
	}

	if err := fs.Parse(flags); err != nil {
		return err
	}

	if common.help != nil && *common.help {
		printHelp()
		return nil
	}

	applyCommonFlags(common, cfg)
	_ = cfg.Validate()

	fmt.Println("Gogitor doctor")
	fmt.Printf("Provider:        %s\n", cfg.Provider)
	fmt.Printf("Model:           %s\n", cfg.Model)
    fmt.Printf("Max context:     %d tokens\n", cfg.EffectiveContextTokens())
	fmt.Printf("Ollama URL:      %s\n", cfg.OllamaURL)
	fmt.Printf("WorkDir:         %s\n", cfg.WorkDir)
	fmt.Printf("Config path:     %s\n", config.Path())
	fmt.Printf("Log path:        %s\n", logPath)
	fmt.Printf("LLM timeout:     %d sec\n", cfg.LLMTimeout)
	fmt.Printf("Max iterations:  %d\n", cfg.MaxIterations)
	fmt.Printf("Auto git commit: %v\n", cfg.AutoGitCommit)
	fmt.Printf("Git auto init:   %v\n", cfg.GitAutoInit)
	fmt.Printf("Multi-agent:     %v\n", cfg.MultiAgent)
	fmt.Printf("Auto search:     %v\n", cfg.AutoSearch)
	fmt.Printf("Dry run:         %v\n", cfg.DryRun)
	fmt.Printf("Debug:           %v\n", cfg.Debug)
	fmt.Printf("Reasoning:       %v (effort: %s, budget: %d, router: %v)", cfg.ReasoningEnabled, cfg.ReasoningEffort, cfg.ReasoningBudget, cfg.ReasoningRouter)
	return nil
}

func registerCommonFlags(fs *flag.FlagSet, cfg *config.Config) commonFlags {
	fs.SetOutput(os.Stderr)
	return commonFlags{
		provider:  fs.String("provider", cfg.Provider, "LLM provider: ollama, openai+URL, openai-compatible+URL, or Ollama URL"),
		model:     fs.String("model", cfg.Model, "LLM model"),
        computer: fs.Bool("computer", cfg.ComputerEnabled, "enable computer mode (executes real system commands)"),
		key:       fs.String("key", cfg.APIKey, "API key (optional)"),
		repo:      fs.String("repo", cfg.WorkDir, "project root directory"),
		githubURL: fs.String("github", cfg.GitHubURL, "GitHub repository URL (https://github.com/user/repo)"),
		githubKey: fs.String("key-github", cfg.GitHubToken, "GitHub token (classic PAT ghp_... or fine-grained github_pat_...)"),
		debug:     fs.Bool("debug", cfg.Debug, "enable debug logging"),
		raw:       fs.Bool("raw", cfg.Raw, "output only result content (for pipes/redirection)"),
		pretty:    fs.Bool("pretty", false, "force human-readable output even when stdout is not a terminal"),
		help:      fs.Bool("help", false, "show help"),
        maxCtx:    fs.Int("max-context", cfg.MaxContextTokens, "max model context tokens (0=auto, e.g. 262144 for 256K)"),
		autoSearch: fs.Bool("auto-search", cfg.AutoSearch, "enable automatic web search in multi-agent mode"),
		output:     fs.String("output", cfg.OutputFile, "save result to file (type by extension: .md, .txt, .go, .json)"),
		reasoning:      fs.Bool("reasoning", cfg.ReasoningEnabled, "enable reasoning/thinking mode for models that support it"),
        reasoningEffort: fs.String("reasoning-effort", cfg.ReasoningEffort, "reasoning depth: low, medium, high (for OpenAI o-series)"),
        reasoningBudget: fs.Int("reasoning-budget", cfg.ReasoningBudget, "max tokens for reasoning (0=server default)"),
        reasoningShow:   fs.Bool("reasoning-show", cfg.ReasoningShow, "display thinking content in output"),
		reasoningRouter: fs.Bool("reasoning-router", cfg.ReasoningRouter, "enable reasoning for intent router (off by default)"),
	}
}

func applyCommonFlags(f commonFlags, cfg *config.Config) {
    if f.reasoning != nil && *f.reasoning {
        cfg.ReasoningEnabled = true
    }
    if f.reasoningEffort != nil {
        if v := strings.TrimSpace(*f.reasoningEffort); v != "" {
            cfg.ReasoningEffort = v
        }
    }
    if f.reasoningBudget != nil && *f.reasoningBudget > 0 {
        cfg.ReasoningBudget = *f.reasoningBudget
    }
    if f.reasoningShow != nil && *f.reasoningShow {
        cfg.ReasoningShow = true
    }
    
    if f.reasoningRouter != nil && *f.reasoningRouter {
        cfg.ReasoningRouter = true
    }
	if f.provider != nil {
		if v := strings.TrimSpace(*f.provider); v != "" {
			cfg.Provider = v
		}
	}
	if f.model != nil {
		if v := strings.TrimSpace(*f.model); v != "" {
			cfg.Model = v
		}
	}

	if f.autoSearch != nil {
		cfg.AutoSearch = *f.autoSearch
	}
    if f.output != nil {
    		if v := strings.TrimSpace(*f.output); v != "" {
    			cfg.OutputFile = expandHome(v)
    		}
	}
    if f.maxCtx != nil && *f.maxCtx > 0 {
        cfg.MaxContextTokens = *f.maxCtx
    }
	if f.key != nil {
		if v := strings.TrimSpace(*f.key); v != "" {
			cfg.APIKey = v
		}
	}
	if f.repo != nil {
		if v := strings.TrimSpace(*f.repo); v != "" {
			cfg.WorkDir = expandHome(v)
		}
	}
	if f.githubURL != nil {
		if v := strings.TrimSpace(*f.githubURL); v != "" {
			cfg.GitHubURL = v
		}
	}
	if f.githubKey != nil {
		if v := strings.TrimSpace(*f.githubKey); v != "" {
			cfg.GitHubToken = v
		}
	}
	if f.debug != nil {
		cfg.Debug = *f.debug
		if *f.debug {
			cfg.LogLevel = "debug"
		}
	}
	if f.raw != nil {
		cfg.Raw = *f.raw
	}
	if f.pretty != nil && *f.pretty {
		cfg.Raw = false
	}
    if f.computer != nil && *f.computer {
        cfg.ComputerEnabled = true
    }
    if f.key != nil && strings.TrimSpace(*f.key) != "" && cfg.APIKey == strings.TrimSpace(*f.key) {
        fmt.Fprintln(os.Stderr, "[warn] API key passed via CLI flag; consider GOGITOR_API_KEY env var")
    }
    if f.githubKey != nil && strings.TrimSpace(*f.githubKey) != "" && cfg.GitHubToken == strings.TrimSpace(*f.githubKey) {
        fmt.Fprintln(os.Stderr, "[warn] GitHub token passed via CLI flag; consider GOGITOR_GITHUB_TOKEN env var")
    }
}


func reorderArgs(args []string, specs []flagSpec) ([]string, []string, error) {
	var flags []string
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}

		name, val, hasVal, isFlag := parseFlagToken(arg)
		if !isFlag {
			positional = append(positional, arg)
			continue
		}

		spec, ok := findFlagSpec(specs, name)
		if !ok {
			return nil, nil, fmt.Errorf("unknown flag: %s (use -- to separate flags from task text)", arg)
		}

		if spec.Bool {
			if hasVal {
				flags = append(flags, "--"+spec.Name+"="+val)
			} else {
				flags = append(flags, "--"+spec.Name)
			}
			continue
		}

		if hasVal {
			flags = append(flags, "--"+spec.Name+"="+val)
			continue
		}

		if i+1 >= len(args) {
			return nil, nil, fmt.Errorf("flag --%s requires a value", spec.Name)
		}

		flags = append(flags, "--"+spec.Name, args[i+1])
		i++
	}

	return flags, positional, nil
}

func parseFlagToken(arg string) (name string, value string, hasValue bool, isFlag bool) {
	if arg == "-" || !strings.HasPrefix(arg, "-") {
		return "", "", false, false
	}

	trimmed := strings.TrimLeft(arg, "-")
	if trimmed == "" {
		return "", "", false, false
	}

	if idx := strings.Index(trimmed, "="); idx != -1 {
		return trimmed[:idx], trimmed[idx+1:], true, true
	}

	return trimmed, "", false, true
}

func findFlagSpec(specs []flagSpec, name string) (flagSpec, bool) {
	for _, spec := range specs {
		if spec.Name == name {
			return spec, true
		}

		if spec.Short != "" && spec.Short == name {
			return spec, true
		}
	}

	return flagSpec{}, false
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}

	return path
}

func emitCLI(jsonOut bool, raw bool) func(domain.Event) {
    return func(e domain.Event) {
    	if e.Type == domain.EventDone ||
    		e.Type == domain.EventToken ||
    		e.Type == domain.EventProgress {
    		return
    	}
    
		msg := i18n.Localize(e.Message)

		if jsonOut {
			fmt.Fprintf(os.Stderr, "[%s] %s\n", e.Type, msg)
			return
		}

		if raw && e.Type != domain.EventWarn && e.Type != domain.EventError {
			return
		}

		switch e.Type {
		case domain.EventAgent:
			fmt.Fprintf(os.Stderr, "▸ %s\n", msg)
		case domain.EventIntent:
			fmt.Fprintf(os.Stderr, "▸ %s\n", msg)
		case domain.EventWarn:
			fmt.Fprintf(os.Stderr, "[warn] %s\n", msg)
		case domain.EventError:
			fmt.Fprintf(os.Stderr, "[error] %s\n", msg)
        case domain.EventPlan:
        	if e.Plan != nil && len(e.Plan.Items) > 0 {
        		fmt.Fprintf(os.Stderr, "▸ %s\n", msg)
        		for _, a := range e.Plan.Acceptance {
        			fmt.Fprintf(os.Stderr, "    • %s\n", a)
        		}
        		for i, item := range e.Plan.Items {
        			fmt.Fprintf(os.Stderr, "    %s %d. %s\n", domain.PlanPending.Symbol(), i+1, item)
        		}
        		return
        	}
        	if e.Plan != nil && e.Plan.Status != "" {
        		fmt.Fprintf(os.Stderr, "%s %s\n", e.Plan.Status.Symbol(), msg)
        		return
        	}
        	fmt.Fprintf(os.Stderr, "%s\n", msg)

		default:
			fmt.Fprintf(os.Stderr, "[%s] %s\n", e.Type, msg)
		}
	}
}

func printResult(res domain.Result, jsonOut bool, raw bool) {
	if jsonOut {
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "json marshal error: %v\n", err)
			return
		}
		fmt.Println(string(data))
		return
	}

	if res.Comparison != nil && res.AwaitingSelection {
		if raw {
			fmt.Println(res.Response)
			return
		}
		fmt.Println(res.Response)
		return
	}

	if raw {
		if !res.Success {
			for _, e := range res.Errors {
				fmt.Fprintln(os.Stderr, i18n.Localize(e))
			}
			return
		}

		if res.Mode == "code" {
			fmt.Println(rawCodeOutput(res))
			return
		}

		fmt.Println(strings.TrimSpace(res.Response))
		return
	}

	if res.RefinedTask != "" {
		fmt.Println(i18n.T("Refined task: %s", res.RefinedTask))
	}

	if res.Response != "" {
		if isLLMMode(res.Mode) {
			fmt.Println(res.Response)
		} else {
			fmt.Println(i18n.Localize(res.Response))
		}
	}

	if len(res.FilesCreated) > 0 {
		fmt.Println("\n" + i18n.T("Created files:"))
		for _, f := range res.FilesCreated {
			fmt.Printf("  + %s\n", f)
		}
	}

	if len(res.FilesModified) > 0 {
		fmt.Println("\n" + i18n.T("Modified files:"))
		for _, f := range res.FilesModified {
			fmt.Printf("  ~ %s\n", f)
		}
	}

	if len(res.FilesPatched) > 0 {
		fmt.Println("\n" + i18n.T("Patched files (DIFF):"))
		for _, f := range res.FilesPatched {
			fmt.Printf("  Δ %s\n", f)
		}
	}

	if len(res.FilesFullRewritten) > 0 {
		fmt.Println("\n" + i18n.T("Full rewritten files:"))
		for _, f := range res.FilesFullRewritten {
			fmt.Printf("  ≡ %s\n", f)
		}
	}

	if res.Tests.Run {
		fmt.Printf(
			"\n%s\n",
			i18n.T("Tests: passed=%d failed=%d", res.Tests.Passed, res.Tests.Failed),
		)
	} else if res.Tests.Skipped {
		fmt.Println("\n" + i18n.T("Tests: skipped"))
	}

	if res.GitCommit != "" {
		fmt.Printf("\n%s %s\n", i18n.T("Git commit:"), res.GitCommit)
	}

	if len(res.Warnings) > 0 {
		fmt.Println("\n" + i18n.T("Warnings:"))
		for _, w := range res.Warnings {
			fmt.Printf("  ! %s\n", i18n.Localize(w))
		}
	}

	if len(res.Errors) > 0 {
		fmt.Println("\n" + i18n.T("Errors:"))
		for _, e := range res.Errors {
			fmt.Printf("  × %s\n", i18n.Localize(e))
		}
	}

	if res.Success {
		fmt.Println("\n" + i18n.T("SUCCESS"))
	} else {
		fmt.Println("\n" + i18n.T("FAILED"))
	}
    if res.Lint.Run {
    	if res.Lint.Passed {
    		fmt.Println("\n" + i18n.T("Lint: passed (0 issues)"))
    	} else if res.Lint.Issues > 0 {
    		fmt.Printf("\n%s\n", i18n.T("Lint: %d issue(s) found", res.Lint.Issues))
    	}
    }
}

func isLLMMode(mode string) bool {
	switch mode {
	case "chat", "analyze", "search", "article":
		return true
	default:
		return false
	}
}

// saveOutputIfRequested сохраняет результат в файл, если указан --output.
func saveOutputIfRequested(res domain.Result, cfg *config.Config) error {
	if cfg.OutputFile == "" {
		return nil
	}
	if err := app.SaveResultToFile(res, cfg.OutputFile); err != nil {
		return fmt.Errorf("cannot save to %s: %w", cfg.OutputFile, err)
	}
	fmt.Fprintf(os.Stderr, "Result saved to: %s\n", cfg.OutputFile)
	return nil
}

func printHelp() {
	fmt.Println(`gogitor — AI coding assistant for Go

Usage:
gogitor
gogitor tui [flags]
gogitor code <task> [flags]
gogitor fix <error / stack trace> [flags]
gogitor task <path/to/file.txt|file.md> [flags]
gogitor file <path/to/file.txt|file.md> [flags]
gogitor ask <question> [--image <path>] [flags]
gogitor analyze <question> [--image <path>] [flags]
gogitor search <query> [flags]
gogitor run [file] [flags]
gogitor test [flags]
gogitor test lint [flags]
gogitor suggest [flags]
gogitor vet [flags]
gogitor todo [flags]
gogitor computer <task> [flags]
gogitor decisions [flags]
gogitor git [status|diff|commit|init|log|checkout|branch|merge|push|pull|fetch|clone|remote] [flags]
gogitor article <topic> [--full] [flags]
gogitor doctor [flags]
gogitor help

Image analysis:
  --image <path>         Path to image file (.png, .jpg, .gif, .webp, .bmp)
                         Used with 'ask' and 'analyze' commands.
                         The image is sent to a vision-capable LLM model.

Common flags:
-p, --provider <name> ollama, openai+URL, openai-compatible+URL, or Ollama URL
-m, --model <model> model name
-k, --key <key>     API key
-r, --repo <path>   project root directory
--image <path>      Path to image file for vision analysis (.png, .jpg, .gif, .webp)
--reasoning            Enable reasoning/thinking mode
--reasoning-effort <v> Reasoning depth: low, medium, high
--reasoning-budget <n> Max tokens for reasoning (0=server default)
--reasoning-show       Display thinking content in output
--reasoning-router     Enable reasoning for intent router (off by default)
--github <url>       GitHub repository URL
--key-github <token> GitHub token (ghp_... or github_pat_...)
--max-context <n>    max model context tokens (0=auto, e.g. 262144 for 256K)
--auto-search        enable automatic web search in multi-agent mode
--computer           Enable computer mode (executes real system commands)
--dry-run            (computer) show plan without executing
--allow-sudo         (computer) allow sudo commands
-o, --output <file>  save result to file (type by extension: .md, .txt, .go, .json)
--debug              enable debug logging
--raw                output only result content (for pipes/redirection)
--pretty             force human-readable output
-h, --help           show help

Code flags:
--dry-run            validate but do not apply changes
--no-commit          disable automatic git commit
--no-tests           skip tests
--no-compare         skip approach comparison for complex tasks
--json               JSON output

Workflow subcommands:
:workflow <task>           execute task in workflow mode
:workflow interview <task> interactive interview before workflow
:workflow reflect          reflect on the last workflow execution
:workflow pr               create branch + open PR with workflow artifacts in description

Task file flags:
--code               force code mode instead of automatic intent detection
--json               JSON output

Flags can be placed before or after the task text.
If the task text itself starts with a dash or contains flag-like text, use --:
gogitor code -- --fix something
gogitor ask -- what is --force in git?
gogitor task -- ./-task.txt

Examples:
gogitor
gogitor tui --provider ollama --model gemma4:31b-cloud
gogitor code "напиши программу для умножения матриц" --provider ollama --model gemma4:31b-cloud
gogitor code --provider ollama --model gemma4:31b-cloud "create main.go"
gogitor ask "объясни context.Context" \
    --provider openai+https://api.example.com/v1 \
    --model gpt-4o-mini \
    --key sk-...

  gogitor code "create main.go" \
    --provider openai-compatible+http://localhost:8000/v1 \
    --model local-model

  gogitor code "create main.go" \
    --provider http://192.168.1.10:11434 \
    --model gemma3:4b
gogitor task ./tasks/feature.txt --provider ollama --model gemma3:4b
gogitor file ./tasks/refactor.md --code --json
gogitor ask "объясни context.Context" -p ollama -m gemma3:4b
gogitor analyze "найди ошибки в коде" --repo ./myproject
gogitor search "последняя версия Go" --json
gogitor run main.go --repo ./myproject
gogitor test --json
gogitor test lint
gogitor git commit
gogitor article "как работает garbage collector в Go"
gogitor article "разбор паттерна middleware" --full
gogitor doctor
gogitor ask "что на этом изображении?" --image screenshot.png
gogitor analyze "опиши архитектуру" --image diagram.png
gogitor ask "explain this error screenshot" --image error.png

Pipe examples:
echo "напиши hello world" | gogitor code --raw > code.go
echo "объясни context.Context" | gogitor ask --raw
gogitor code "create main.go" --raw | gofmt
gogitor code "task" --pretty
`)
}

func isStdinTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func isStdoutTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func rawEnabled(common commonFlags, cfg *config.Config) bool {
	if common.pretty != nil && *common.pretty {
		return false
	}

	if common.raw != nil && *common.raw {
		return true
	}

	if cfg.Raw {
		return true
	}

	// Если stdout перенаправлен или piped, включаем raw автоматически.
	return !isStdoutTerminal()
}

func readStdinIfAvailable() string {
	if isStdinTerminal() {
		return ""
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

func taskFromArgsOrStdin(positional []string) string {
	task := strings.TrimSpace(strings.Join(positional, " "))

	if task == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	}

	if task == "" {
		return readStdinIfAvailable()
	}

	return task
}

func rawCodeOutput(res domain.Result) string {
	if len(res.OutputFiles) == 0 {
		return strings.TrimSpace(res.Response)
	}

	if len(res.OutputFiles) == 1 {
		return strings.TrimRight(res.OutputFiles[0].Content, "\n")
	}

	var chosen *domain.OutputFile

	for i := range res.OutputFiles {
		if strings.HasSuffix(res.OutputFiles[i].Path, ".go") {
			chosen = &res.OutputFiles[i]
			break
		}
	}

	if chosen == nil {
		chosen = &res.OutputFiles[0]
	}

	fmt.Fprintf(
		os.Stderr,
		"raw: multiple output files (%d); stdout contains %s\n",
		len(res.OutputFiles),
		chosen.Path,
	)

	return strings.TrimRight(chosen.Content, "\n")
}

func runDecisions(args []string, cfg *config.Config, log *slog.Logger) error {
	fs := flag.NewFlagSet("decisions", flag.ContinueOnError)
	common := registerCommonFlags(fs, cfg)
	jsonOut := fs.Bool("json", false, "print result as JSON")
	specs := buildFlagSpecs(
		flagSpec{Name: "json", Bool: true},
	)
	flags, _, err := reorderArgs(args, specs)
	if err != nil {
		return err
	}
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if common.help != nil && *common.help {
		printHelp()
		return nil
	}
	applyCommonFlags(common, cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}
	raw := rawEnabled(common, cfg)
	svc := app.New(cfg, log)
	defer svc.Close()
	res := svc.DecisionJournal(context.Background(), emitCLI(*jsonOut, raw))
	printResult(res, *jsonOut, raw)
    saveErr := saveOutputIfRequested(res, cfg)
	if !res.Success {
		return fmt.Errorf("decisions failed")
	}
	return saveErr
}

func runFix(args []string, cfg *config.Config, log *slog.Logger) error {
	fs := flag.NewFlagSet("fix", flag.ContinueOnError)
	common := registerCommonFlags(fs, cfg)
	dryRun := fs.Bool("dry-run", cfg.DryRun, "validate changes but do not apply")
	noCommit := fs.Bool("no-commit", false, "disable automatic git commit")
	noTests := fs.Bool("no-tests", false, "skip tests")
	jsonOut := fs.Bool("json", false, "print result as JSON")

	specs := buildFlagSpecs(
		flagSpec{Name: "dry-run", Bool: true},
		flagSpec{Name: "no-commit", Bool: true},
		flagSpec{Name: "no-tests", Bool: true},
		flagSpec{Name: "json", Bool: true},
	)
	flags, positional, err := reorderArgs(args, specs)
	if err != nil {
		return err
	}
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if common.help != nil && *common.help {
		printHelp()
		return nil
	}

	errorText := taskFromArgsOrStdin(positional)
	if errorText == "" {
		return fmt.Errorf(
			"usage: gogitor fix <error output / stack trace>\n" +
				"example: gogitor fix \"panic: runtime error: index out of range [3] with length 2\"",
		)
	}

	applyCommonFlags(common, cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}

	raw := rawEnabled(common, cfg)
	svc := app.New(cfg, log)
	defer svc.Close()

	opts := app.Options{
		DryRun:   *dryRun,
		NoCommit: *noCommit,
		NoTests:  *noTests,
	}

	res := svc.FixError(context.Background(), errorText, opts, emitCLI(*jsonOut, raw))
	printResult(res, *jsonOut, raw)
    saveErr := saveOutputIfRequested(res, cfg)
	if !res.Success {
		return fmt.Errorf("fix failed")
	}
	return saveErr
}
