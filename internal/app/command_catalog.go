package app

// CommandSpec describes a TUI command. The catalog is intentionally independent
// from the renderer so the TUI, help and completion can share one source of truth.
type CommandSpec struct {
	Name        string
	Aliases     []string
	Description string
}

var commandCatalog = []CommandSpec{
	{Name: ":help", Aliases: []string{":h"}, Description: "show help"},
	{Name: ":clear", Aliases: []string{":cls"}, Description: "clear conversation context"},
	{Name: ":save", Description: "save the last result"},
	{Name: ":load", Description: "load a task from a text/markdown file"},
	{Name: ":code", Description: "automatic code execution strategy"},
	{Name: ":fast", Description: "single-pass code generation"},
	{Name: ":agent", Description: "adaptive multi-agent coding"},
	{Name: ":fix", Description: "repair a build/runtime error"},
	{Name: ":ask", Description: "general LLM chat"},
	{Name: ":analyze", Aliases: []string{":analysis"}, Description: "analyze without modifying files"},
	{Name: ":search", Description: "web search"},
	{Name: ":run", Description: "run the project or a Go file"},
	{Name: ":test", Description: "run tests"},
	{Name: ":vet", Description: "run go vet"},
	{Name: ":todo", Description: "scan TODO/FIXME/HACK/BUG"},
	{Name: ":suggest", Description: "project health suggestions"},
	{Name: ":decisions", Aliases: []string{":journal"}, Description: "show decision journal"},
	{Name: ":task-diff", Description: "show the last task diff"},
	{Name: ":diff-trace", Description: "toggle patch diagnostics"},
	{Name: ":reasoning", Description: "toggle LLM reasoning"},
	{Name: ":article", Description: "generate an article"},
	{Name: ":git", Description: "Git/GitHub operations"},
	{Name: ":computer", Description: "computer-control tasks"},
	{Name: ":autonomy", Description: "autonomous task queue"},
	{Name: ":mutate", Description: "mutation testing"},
	{Name: ":autogen-tests", Description: "generate focused tests"},
	{Name: ":quit", Aliases: []string{":exit", ":q"}, Description: "exit Gogitor"},
}

func CommandCatalog() []CommandSpec {
	out := make([]CommandSpec, len(commandCatalog))
	copy(out, commandCatalog)
	for i := range out {
		out[i].Aliases = append([]string(nil), out[i].Aliases...)
	}
	return out
}

func CommandNames() []string {
	result := make([]string, 0, len(commandCatalog))
	for _, spec := range commandCatalog {
		result = append(result, spec.Name)
	}
	return result
}
