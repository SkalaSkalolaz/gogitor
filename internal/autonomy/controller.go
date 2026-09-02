package autonomy

import (
	"log/slog"

	"gogitor/internal/config"
	"gogitor/internal/runner"
	"gogitor/internal/workspace"
)

// Controller — единая точка доступа к подсистеме автономии.
// Агрегирует монитор, очередь, мутатор и генератор тестов.
type Controller struct {
	Monitor *Monitor
	Queue   *TaskQueue
	Mutator *Mutator
	TestGen *TestGenerator
	Enabled bool
}

// NewController создаёт контроллер автономии.
// Вызывается один раз при создании Service.
func NewController(
	cfg *config.Config,
	ws *workspace.Workspace,
	r *runner.Runner,
	llm LLMClient,
	log *slog.Logger,
) *Controller {
	queue := NewTaskQueue()
	return &Controller{
		Monitor: NewMonitor(ws, queue, cfg, log),
		Queue:   queue,
		Mutator: NewMutator(ws, r, cfg.AutonomyMutationLimit),
		TestGen: NewTestGenerator(ws, llm),
		Enabled: cfg.AutonomyEnabled,
	}
}

// Close останавливает фоновый мониторинг.
// Вызывается при закрытии Service.
func (c *Controller) Close() {
	if c.Monitor != nil {
		c.Monitor.Stop()
	}
}