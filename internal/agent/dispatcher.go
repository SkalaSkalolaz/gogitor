package agent

import (
	"context"
	"errors"
	"fmt"
	"io"       
	"net"        
	"regexp"     
	"strings"
	"sync"
	"syscall"
	"time"
)


type Role string

const (
	RoleDefault  Role = "default"
	RoleRouter   Role = "router"
	RolePlanner  Role = "planner"
	RoleCoder    Role = "coder"
	RoleReviewer Role = "reviewer"
	RoleTester   Role = "tester"
	RoleVerifier Role = "verifier"
	RoleSecurity Role = "security"
	RoleDocs     Role = "docs"
	RoleSearcher Role = "searcher"
)

// Priority — приоритет доступа к LLM.
type Priority int

// StatusKind — тип статусного события диспетчера.
type StatusKind string

// StreamLLM — опциональный интерфейс для LLM с потоковой генерацией.
type StreamLLM interface {
	Stream(ctx context.Context, prompt string, onToken func(string)) (string, error)
}

// MultimodalLLM — опциональный интерфейс для моделей с vision.
type MultimodalLLM interface {
	SendWithImages(ctx context.Context, prompt string, images [][]byte) (string, error)
}

// StreamMultimodalLLM — потоковый вариант multimodal.
type StreamMultimodalLLM interface {
	StreamWithImages(ctx context.Context, prompt string, images [][]byte, onToken func(string)) (string, error)
}

const (
	StatusQueued StatusKind = "queued"
	StatusStart  StatusKind = "start"
	StatusDone   StatusKind = "done"
	StatusRetry  StatusKind = "retry"
)

// StatusEvent — информация о состоянии LLM-запроса.
type StatusEvent struct {
	Kind    StatusKind
	Role    Role
	Purpose string
	Queue   int
	Session Usage
	Err     error
}

// StatusFunc — callback для отправки статусных событий.
type StatusFunc func(StatusEvent)

const (
	PriorityUnknown  Priority = 0
	PriorityLow      Priority = 10
	PriorityNormal   Priority = 100
	PriorityHigh     Priority = 200
	PriorityCritical Priority = 300
)

var (
	ErrBudgetExceeded   = errors.New("agent dispatcher: budget exceeded")
	ErrDispatcherClosed = errors.New("agent dispatcher: closed")
	ErrQueueFull        = errors.New("agent dispatcher: queue full")
)

var httpRetryStatusRE = regexp.MustCompile(`\bHTTP\s+(429|502|503|504)\b`)

type LLM interface {
	Send(ctx context.Context, prompt string) (string, error)
}

type Usage struct {
	Requests        int           `json:"requests"`
	EstimatedTokens int           `json:"estimated_tokens"`
	Duration        time.Duration `json:"duration"`
}

func (u Usage) Add(v Usage) Usage {
	return Usage{
		Requests:        u.Requests + v.Requests,
		EstimatedTokens: u.EstimatedTokens + v.EstimatedTokens,
		Duration:        u.Duration + v.Duration,
	}
}

// RoleQuota — ограничения и бонусы для конкретной роли.
type RoleQuota struct {
	MaxRequests   int
	MaxTokens     int
	MaxDuration   time.Duration
	PriorityBoost Priority
}

// Config — конфигурация диспетчера.
type Config struct {
	// DefaultTimeout — максимальное время одного LLM-запроса.
	DefaultTimeout time.Duration

	// MaxSessionRequests — максимум запросов за всю сессию диспетчера.
	MaxSessionRequests int

	// MaxSessionTokens — примерный максимум токенов за сессию.
	MaxSessionTokens int

	// MaxSessionDuration — максимум суммарного времени LLM-запросов.
	MaxSessionDuration time.Duration

	// MaxQueue — максимальный размер очереди ожидания.
	MaxQueue int

	// AgingPerSecond — насколько растёт приоритет задачи за каждую секунду ожидания.
	// Нужно, чтобы низкоприоритетные агенты не голодали.
	AgingPerSecond float64

	// RoleQuotas — квоты и бонусы по ролям.
	RoleQuotas map[Role]RoleQuota

    // MaxRetries — максимальное число повторных попыток при
    // транзитивных ошибках (0 = без retry).
    MaxRetries int
    // RetryBaseDelay — начальная задержка перед первым повтором.
    RetryBaseDelay time.Duration
    // RetryMaxDelay — потолок задержки (backoff не растёт выше).
    RetryMaxDelay time.Duration
    // RetryMultiplier — множитель экспоненциального роста.
    RetryMultiplier float64

	// StatsHook вызывается после завершения LLM-запроса.
	// Используется для накопления статистики и оценки ETA.
	StatsHook func(Request, Usage, error)
	// ReasoningEnabled — включён ли режим размышления (thinking).
	// Используется для увеличения оценки токенов ответа.
	ReasoningEnabled bool
}

// Request — запрос агента к LLM.
type Request struct {
	Role     Role
	Purpose  string
	Prompt   string
	Priority Priority
	Timeout  time.Duration
	StreamFunc func(string)
	Images     [][]byte
}

// Result — результат выполнения LLM-запроса.
type Result struct {
	Text  string
	Usage Usage
	Err   error
}

type ticket struct {
	id      int
	ctx     context.Context
	req     Request
	created time.Time
	result  chan Result
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ECONNRESET, syscall.ECONNREFUSED, syscall.ECONNABORTED,
			syscall.EPIPE, syscall.ETIMEDOUT, syscall.EHOSTUNREACH,
			syscall.ENETUNREACH, syscall.ENETRESET:
			return true
		}
	}

	if httpRetryStatusRE.MatchString(err.Error()) {
		return true
	}

	msg := err.Error()
	if strings.Contains(msg, "model not found") ||
		strings.Contains(msg, "try pulling it first") {
		return true
	}

	return false
}

func (t *ticket) deliver(res Result) {
	select {
	case t.result <- res:
	default:
	}
}

// Dispatcher — единая точка доступа агентов к одной LLM.
type Dispatcher struct {
	cfg Config
	llm LLM

	mu      sync.Mutex
	queue   []*ticket
	seq     int
	session Usage
	roles   map[Role]Usage
	closed  bool

	notify chan struct{}
	stop   chan struct{}
	once   sync.Once
	wg     sync.WaitGroup
}

// NewDispatcher создаёт диспетчер и запускает фоновый планировщик.
//
// Планировщик намеренно выполняет запросы последовательно.
// Для одной локальной LLM это обычно безопаснее и предсказуемее,
// чем конкурентные запросы.
func NewDispatcher(llm LLM, cfg Config) *Dispatcher {
	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = 3000 * time.Second
	}
	if cfg.MaxQueue <= 0 {
		cfg.MaxQueue = 128
	}
	if cfg.AgingPerSecond <= 0 {
		cfg.AgingPerSecond = 5
	}
	if cfg.RoleQuotas == nil {
		cfg.RoleQuotas = map[Role]RoleQuota{}
	}
    // ─── Retry defaults ──────────────────────────────────────
    if cfg.MaxRetries < 0 {
        cfg.MaxRetries = 0
    }
    if cfg.MaxRetries == 0 && cfg.RetryBaseDelay == 0 {
        // По умолчанию: 2 retry, если пользователь ничего не задал.
        cfg.MaxRetries = 2
    }
    if cfg.RetryBaseDelay <= 0 {
        cfg.RetryBaseDelay = 1 * time.Second
    }
    if cfg.RetryMaxDelay <= 0 {
        cfg.RetryMaxDelay = 10 * time.Second
    }
    if cfg.RetryMultiplier <= 1 {
        cfg.RetryMultiplier = 2.0
    }


	d := &Dispatcher{
		cfg:    cfg,
		llm:    llm,
		roles:  map[Role]Usage{},
		notify: make(chan struct{}, 1),
		stop:   make(chan struct{}),
	}

	d.wg.Add(1)
	go d.run()

	return d
}

// Close останавливает диспетчер.
func (d *Dispatcher) Close() {
	d.once.Do(func() {
		d.mu.Lock()
		d.closed = true
		d.mu.Unlock()

		close(d.stop)
		d.wake()
	})

	d.wg.Wait()
}

// Send реализует интерфейс LLM.
//
// Это позволяет использовать Dispatcher как drop-in замену llm.Client:
//
//	s.LLM.Send(ctx, prompt)
//
// Роль агента берётся из context.Context.
func (d *Dispatcher) Send(ctx context.Context, prompt string) (string, error) {
	role := RoleFromContext(ctx)
	if role == "" {
		role = RoleDefault
	}

	text, _, err := d.Request(ctx, Request{
		Role:     role,
		Purpose:  PurposeFromContext(ctx),
		Prompt:   prompt,
		Priority: PriorityFromContext(ctx),
	})

	return text, err
}

// SendWithImages реализует запрос с изображениями через очередь.
func (d *Dispatcher) SendWithImages(ctx context.Context, prompt string, images [][]byte) (string, error) {
	role := RoleFromContext(ctx)
	if role == "" {
		role = RoleDefault
	}
	text, _, err := d.Request(ctx, Request{
		Role:     role,
		Purpose:  PurposeFromContext(ctx),
		Prompt:   prompt,
		Priority: PriorityFromContext(ctx),
		Images:   images,
	})
	return text, err
}

// StreamWithImages — потоковый запрос с изображениями.
func (d *Dispatcher) StreamWithImages(ctx context.Context, prompt string, images [][]byte, onToken func(string)) (string, error) {
	role := RoleFromContext(ctx)
	if role == "" {
		role = RoleDefault
	}
	text, _, err := d.Request(ctx, Request{
		Role:       role,
		Purpose:    PurposeFromContext(ctx),
		Prompt:     prompt,
		Priority:   PriorityFromContext(ctx),
		StreamFunc: onToken,
		Images:     images,
	})
	return text, err
}

// Request ставит запрос в очередь и ждёт выполнения.
func (d *Dispatcher) Request(ctx context.Context, req Request) (string, Usage, error) {
	if req.Role == "" {
		req.Role = RoleDefault
	}
	if req.Priority == PriorityUnknown {
		req.Priority = PriorityNormal
	}

	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return "", Usage{}, ErrDispatcherClosed
	}

	if len(d.queue) >= d.cfg.MaxQueue {
		d.mu.Unlock()
		return "", Usage{}, ErrQueueFull
	}

	d.seq++
	t := &ticket{
		id:      d.seq,
		ctx:     ctx,
		req:     req,
		created: time.Now(),
		result:  make(chan Result, 1),
	}

	d.queue = append(d.queue, t)
	d.mu.Unlock()

	d.wake()

    d.emitStatus(ctx, StatusEvent{
    		Kind:    StatusQueued,
    		Role:    req.Role,
    		Purpose: req.Purpose,
    	})

	select {
	case res := <-t.result:
		return res.Text, res.Usage, res.Err
	case <-ctx.Done():
		d.remove(t.id)
		return "", Usage{}, ctx.Err()
	}
}

// Snapshot возвращает текущее использование LLM.
func (d *Dispatcher) Snapshot() (Usage, map[Role]Usage) {
	d.mu.Lock()
	defer d.mu.Unlock()

	roles := make(map[Role]Usage, len(d.roles))
	for k, v := range d.roles {
		roles[k] = v
	}

	return d.session, roles
}

// QueueLen возвращает текущую длину очереди.
func (d *Dispatcher) QueueLen() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.queue)
}

func (d *Dispatcher) run() {
	defer d.wg.Done()

	for {
		select {
		case <-d.stop:
			d.drain(ErrDispatcherClosed)
			return
		case <-d.notify:
		}

		for {
			select {
			case <-d.stop:
				d.drain(ErrDispatcherClosed)
				return
			default:
			}

			t := d.popBest()
			if t == nil {
				break
			}

			if err := t.ctx.Err(); err != nil {
				t.deliver(Result{Err: err})
				continue
			}

			d.execute(t)
		}
	}
}

func (d *Dispatcher) drain(err error) {
	d.mu.Lock()
	queue := d.queue
	d.queue = nil
	d.closed = true
	d.mu.Unlock()

	for _, t := range queue {
		t.deliver(Result{Err: err})
	}
}

func (d *Dispatcher) wake() {
	select {
	case d.notify <- struct{}{}:
	default:
	}
}

func (d *Dispatcher) remove(id int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := range d.queue {
		if d.queue[i].id == id {
			d.queue = append(d.queue[:i], d.queue[i+1:]...)
			return
		}
	}
}

// popBest выбирает наиболее приоритетную задачу с учётом старения.
func (d *Dispatcher) popBest() *ticket {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.queue) == 0 {
		return nil
	}

	now := time.Now()
	best := 0
	bestScore := d.score(d.queue[0], now)

	for i := 1; i < len(d.queue); i++ {
		score := d.score(d.queue[i], now)
		if score > bestScore {
			best = i
			bestScore = score
		}
	}

	t := d.queue[best]
	d.queue = append(d.queue[:best], d.queue[best+1:]...)
	return t
}

func (d *Dispatcher) score(t *ticket, now time.Time) float64 {
	base := float64(t.req.Priority) + float64(d.roleBoost(t.req.Role))
	waitSeconds := now.Sub(t.created).Seconds()
	return base + waitSeconds*d.cfg.AgingPerSecond
}

func (d *Dispatcher) roleBoost(r Role) Priority {
	if q, ok := d.cfg.RoleQuotas[r]; ok {
		return q.PriorityBoost
	}
	return 0
}

func (d *Dispatcher) execute(t *ticket) {
    defer func() {
        if r := recover(); r != nil {
            t.deliver(Result{
                Err: fmt.Errorf("agent dispatcher: llm panic: %v", r),
            })
        }
    }()

    if err := t.ctx.Err(); err != nil {
        t.deliver(Result{Err: err})
        return
    }

    d.mu.Lock()
    if d.closed {
        d.mu.Unlock()
        t.deliver(Result{Err: ErrDispatcherClosed})
        return
    }
    if err := d.checkBudgetLocked(t.req); err != nil {
        d.mu.Unlock()
        t.deliver(Result{Err: err})
        return
    }
    timeout := t.req.Timeout
    if timeout <= 0 {
        timeout = d.cfg.DefaultTimeout
    }
    d.mu.Unlock()

    d.emitStatus(t.ctx, StatusEvent{
        Kind:    StatusStart,
        Role:    t.req.Role,
        Purpose: t.req.Purpose,
    })

    // ─── Retry loop ──────────────────────────────────────────
    maxAttempts := 1 + d.cfg.MaxRetries
    backoff := d.cfg.RetryBaseDelay
    
    var lastErr error
    var text string
    var totalDuration time.Duration
    
    gotToken := false
    var onToken func(string)
    
    if t.req.StreamFunc != nil {
    	onToken = func(s string) {
    		gotToken = true
    		t.req.StreamFunc(s)
    	}
    }
    
    for attempt := 1; attempt <= maxAttempts; attempt++ {
    	if t.ctx.Err() != nil {
    		t.deliver(Result{Err: t.ctx.Err()})
    		return
    	}
    
    	attemptCtx, cancel := context.WithTimeout(t.ctx, timeout)
    
    	// Передаём роль и purpose дальше, чтобы можно было собирать статистику.
    	attemptCtx = WithRole(attemptCtx, t.req.Role)
    	attemptCtx = WithPurpose(attemptCtx, t.req.Purpose)
    	attemptCtx = WithPriority(attemptCtx, t.req.Priority)
    
    	start := time.Now()

    	if len(t.req.Images) > 0 {
    		// Multimodal path: изображения присутствуют
    		if t.req.StreamFunc != nil {
    			if sml, ok := d.llm.(StreamMultimodalLLM); ok {
    				text, lastErr = sml.StreamWithImages(attemptCtx, t.req.Prompt, t.req.Images, onToken)
    			} else if ml, ok := d.llm.(MultimodalLLM); ok {
    				text, lastErr = ml.SendWithImages(attemptCtx, t.req.Prompt, t.req.Images)
    			} else {
    				// Fallback: модель не поддерживает vision
    				text, lastErr = d.llm.Send(attemptCtx, t.req.Prompt)
    			}
    		} else {
    			if ml, ok := d.llm.(MultimodalLLM); ok {
    				text, lastErr = ml.SendWithImages(attemptCtx, t.req.Prompt, t.req.Images)
    			} else {
    				text, lastErr = d.llm.Send(attemptCtx, t.req.Prompt)
    			}
    		}
    	} else if t.req.StreamFunc != nil {
    		if sl, ok := d.llm.(StreamLLM); ok {
    			text, lastErr = sl.Stream(attemptCtx, t.req.Prompt, onToken)
    		} else {
    			text, lastErr = d.llm.Send(attemptCtx, t.req.Prompt)
    		}
    	} else {
    		text, lastErr = d.llm.Send(attemptCtx, t.req.Prompt)
    	}    
    	elapsed := time.Since(start)
    	totalDuration += elapsed
    	cancel()
    
    	if lastErr == nil {
    		break
    	}
    
    	// Если уже начался стриминг, retry невозможен: часть ответа могла уйти пользователю.
    	if gotToken || attempt == maxAttempts || !isRetryable(lastErr) {
    		break
    	}
    
    	d.emitStatus(t.ctx, StatusEvent{
    		Kind:    StatusRetry,
    		Role:    t.req.Role,
    		Purpose: t.req.Purpose,
    		Err: fmt.Errorf(
    			"attempt %d/%d failed: %v; retry in %s",
    			attempt, maxAttempts, lastErr, backoff.Round(time.Millisecond),
    		),
    	})
    
    	select {
    	case <-time.After(backoff):
    	case <-t.ctx.Done():
    		t.deliver(Result{Err: t.ctx.Err()})
    		return
    	}
    
    	backoff = time.Duration(float64(backoff) * d.cfg.RetryMultiplier)
    	if backoff > d.cfg.RetryMaxDelay {
    		backoff = d.cfg.RetryMaxDelay
    	}
    }

    estimatedResponseTokens := estimateTokens(text)
    if d.cfg.ReasoningEnabled {
        estimatedResponseTokens *= 3
    }
    usage := Usage{
        Requests:        1,
        EstimatedTokens: estimateTokens(t.req.Prompt) + estimatedResponseTokens,
        Duration:        totalDuration,
    }
    
    d.mu.Lock()
    d.session = d.session.Add(usage)
    d.roles[t.req.Role] = d.roles[t.req.Role].Add(usage)
    d.mu.Unlock()
    
    if d.cfg.StatsHook != nil {
    	d.cfg.StatsHook(t.req, usage, lastErr)
    }
    
    t.deliver(Result{
    	Text:  text,
    	Usage: usage,
    	Err:   lastErr,
    })
    
    d.emitStatus(t.ctx, StatusEvent{
    	Kind:    StatusDone,
    	Role:    t.req.Role,
    	Purpose: t.req.Purpose,
    	Err:     lastErr,
    })
}

func (d *Dispatcher) checkBudgetLocked(req Request) error {
	if d.cfg.MaxSessionRequests > 0 && d.session.Requests >= d.cfg.MaxSessionRequests {
		return fmt.Errorf(
			"%w: session requests limit %d",
			ErrBudgetExceeded,
			d.cfg.MaxSessionRequests,
		)
	}

	if d.cfg.MaxSessionTokens > 0 && d.session.EstimatedTokens >= d.cfg.MaxSessionTokens {
		return fmt.Errorf(
			"%w: session estimated tokens limit %d",
			ErrBudgetExceeded,
			d.cfg.MaxSessionTokens,
		)
	}

	if d.cfg.MaxSessionDuration > 0 && d.session.Duration >= d.cfg.MaxSessionDuration {
		return fmt.Errorf(
			"%w: session duration limit %s",
			ErrBudgetExceeded,
			d.cfg.MaxSessionDuration,
		)
	}

	q := d.roleQuota(req.Role)
	ru := d.roles[req.Role]

	if q.MaxRequests > 0 && ru.Requests >= q.MaxRequests {
		return fmt.Errorf(
			"%w: role %s requests limit %d",
			ErrBudgetExceeded,
			req.Role,
			q.MaxRequests,
		)
	}

	if q.MaxTokens > 0 && ru.EstimatedTokens >= q.MaxTokens {
		return fmt.Errorf(
			"%w: role %s estimated tokens limit %d",
			ErrBudgetExceeded,
			req.Role,
			q.MaxTokens,
		)
	}

	if q.MaxDuration > 0 && ru.Duration >= q.MaxDuration {
		return fmt.Errorf(
			"%w: role %s duration limit %s",
			ErrBudgetExceeded,
			req.Role,
			q.MaxDuration,
		)
	}

	return nil
}

func (d *Dispatcher) roleQuota(r Role) RoleQuota {
	if q, ok := d.cfg.RoleQuotas[r]; ok {
		return q
	}
	return RoleQuota{}
}

// estimateTokens — грубая оценка токенов по байтам.
//
// Для английского текста и кода часто используют приближение:
//
//	1 token ≈ 4 bytes
//
// Для русского языка это может быть менее точно,
// но для бюджетного диспетчера этого достаточно.
//
// Позже можно заменить на реальный usage из OpenAI/Ollama API.
func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len(s) + 3) / 4
}

// ─── Context helpers ─────────────────────────────────────────────────

type ctxKey int

const (
	roleCtxKey ctxKey = iota
	priorityCtxKey
	purposeCtxKey
	statusCtxKey
)

func WithRole(ctx context.Context, role Role) context.Context {
	return context.WithValue(ctx, roleCtxKey, role)
}

func RoleFromContext(ctx context.Context) Role {
	if ctx == nil {
		return RoleDefault
	}
	if v, ok := ctx.Value(roleCtxKey).(Role); ok && v != "" {
		return v
	}
	return RoleDefault
}

func WithPriority(ctx context.Context, p Priority) context.Context {
	return context.WithValue(ctx, priorityCtxKey, p)
}

func PriorityFromContext(ctx context.Context) Priority {
	if ctx == nil {
		return PriorityNormal
	}
	if v, ok := ctx.Value(priorityCtxKey).(Priority); ok && v != PriorityUnknown {
		return v
	}
	return PriorityNormal
}

func WithPurpose(ctx context.Context, purpose string) context.Context {
	return context.WithValue(ctx, purposeCtxKey, purpose)
}

func PurposeFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(purposeCtxKey).(string); ok {
		return v
	}
	return ""
}


// WithStatusFunc добавляет callback статусных событий в context.
func WithStatusFunc(ctx context.Context, fn StatusFunc) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, statusCtxKey, fn)
}

func statusFuncFromContext(ctx context.Context) StatusFunc {
	if ctx == nil {
		return nil
	}
	if fn, ok := ctx.Value(statusCtxKey).(StatusFunc); ok {
		return fn
	}
	return nil
}

func (d *Dispatcher) sessionUsage() Usage {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.session
}

func (d *Dispatcher) emitStatus(ctx context.Context, ev StatusEvent) {
	fn := statusFuncFromContext(ctx)
	if fn == nil {
		return
	}

	if ev.Role == "" {
		ev.Role = RoleDefault
	}

	ev.Queue = d.QueueLen()
	ev.Session = d.sessionUsage()

	defer func() {
		_ = recover()
	}()

	fn(ev)
}

// Stream реализует потоковый запрос через очередь dispatcher'а.
func (d *Dispatcher) Stream(ctx context.Context, prompt string, onToken func(string)) (string, error) {
	role := RoleFromContext(ctx)
	if role == "" {
		role = RoleDefault
	}

	text, _, err := d.Request(ctx, Request{
		Role:       role,
		Purpose:    PurposeFromContext(ctx),
		Prompt:     prompt,
		Priority:   PriorityFromContext(ctx),
		StreamFunc: onToken,
	})

	return text, err
}