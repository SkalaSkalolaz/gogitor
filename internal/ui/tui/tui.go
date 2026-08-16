package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
    "path/filepath"
	"strings"
	"time"
    "encoding/base64"
    "os/exec"
    "encoding/json"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"gogitor/internal/textutil"
    "gogitor/internal/i18n"
	"gogitor/internal/app"
	"gogitor/internal/config"
	"gogitor/internal/domain"
)

const (
	inputTextAreaHeight     = 3
	inputHorizontalOverhead = 2
	inputVerticalOverhead   = 2
	headerHeight            = 2
	statusHeight            = 1
)

type logKind int

const (
	logPlain logKind = iota
	logInfo
	logSuccess
	logWarn
	logError
	logCreated
	logModified
	logGit
	logIntent
	logAgent
	logMarkdown
	logPlanHeader    
	logPlanPending   
	logPlanRunning   
	logPlanDone      
	logPlanWarn      
	logPlanFailed    
	logPlanSkipped
	logDiffAdd     
	logDiffDel     
	logDiffHunk    
	logDiffMeta
)

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("75")).
			MarginBottom(1)

	intentStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99"))

	agentStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("63"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	inputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1)
	diffAddStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))
	diffDelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))
	diffHunkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("75")).
			Bold(true)
	diffMetaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))
	suggestionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	successStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("42"))

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("196"))

	warnStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("214"))

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	createdStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	modifiedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	gitStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	planHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("75"))

	planPendingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	planRunningStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39"))

	planDoneStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("42"))

	planWarnStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("214"))

	planFailedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("196"))

	planSkippedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))
)

var commandSuggestions = []string{
	":help",
	":clear",
	":cls",
	":save",
	":code",
	":fast",
	":fix",
	":history",
	":task-diff",
	":agent",
	":workflow",
	":workflow interview",
	":workflow reflect",
	":workflow pr",
	":ask",
	":analyze",
	":search",
	":run",
	":test",
	":git",
	":decisions",
	":suggest",
	":vet",
	":todo",
    ":reasoning",	
	":computer",
	":article",
	":quit",
}

var gitSubcommandSuggestions = []string{
	"status",
	"diff",
	"diff-task",
	"commit",
	"init",
	"log",
	"checkout",
	"branch",
	"merge",
	"revert",      
	"reset",     
	"push",
	"pull",
	"fetch",
	"clone",
	"remote",
	"create",
	"pr",
	"issue",
	"changelog",
	"pr-comment",
}

var testSubcommandSuggestions = []string{
	"lint",
}

var reasoningSubcommandSuggestions = []string{
	"on",
	"off",
}

type eventMsg domain.Event

type logEntry struct {
	text string
	kind logKind
}

type model struct {
	svc *app.Service
	cfg *config.Config
	log *slog.Logger

	taskStage       domain.TaskStage
	taskStartedAt   time.Time
	taskQuery       string
	taskIteration   int
	taskMaxIteration int
    taskTimeline []string

	viewport viewport.Model
	input    textarea.Model

	entries   []logEntry
	wrapped   strings.Builder
	wrapWidth int

	mdRenderer      *glamour.TermRenderer
	mdRendererWidth int
	mdRendererStyle string

	suggestions []string

	status      string
	running     bool
	focusOutput bool

	prog *tea.Program
	mouseFree bool

	cancel context.CancelFunc
	events chan domain.Event

	width  int
	height int

    history      []string
	historyIndex int
	savedInput   string

	planItems   []string
	planEntries []int    

	streaming   bool
	streamBuf   strings.Builder
	justStreamed bool

	progressStart time.Time
	progressETA   time.Duration

	planCurrent int
	planETA     map[int]time.Duration
	planStart   map[int]time.Time

	agentRole     string
	agentRequests int
	agentTokens   int
	agentDuration string

    agentStartTime time.Time

    lastResult    *domain.Result

	suggestionIdx int
}


func Run(cfg *config.Config, log *slog.Logger) error {
	svc := app.New(cfg, log)
	defer svc.Close()

	m := newModel(svc, cfg, log)

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	m.prog = p

	_, err := p.Run()
	return err
}



func newModel(svc *app.Service, cfg *config.Config, log *slog.Logger) *model {
    ta := textarea.New()
    ta.Prompt = ""
    ta.Placeholder = i18n.T("Task: enter a task or :help")

	ta.SetHeight(inputTextAreaHeight)
	vp := viewport.New(80, 20)
	m := &model{
		svc:         svc,
		cfg:         cfg,
		log:         log,
		viewport:    vp,
		input:       ta,
		focusOutput: false,
		historyIndex: -1,
	}
	m.updateStatus()

    m.appendInfo(i18n.T("Gogitor TUI ready."))
    m.appendInfo(i18n.T("Type :help for commands."))
    m.appendInfo(i18n.T("Alt+Enter adds a line. Up/Down move between lines. Tab switches to output."))
    m.appendInfo(i18n.T("F2 - mode for selecting text with the mouse for copying."))
	// Сканируем TODO при старте (без LLM, мгновенно).
	go func() {
		todoItems := svc.WS.ScanTODOs(10)
		if len(todoItems) == 0 {
			return
		}
		counts := map[string]int{}
		for _, item := range todoItems {
			counts[item.Kind]++
		}
		var parts []string
		for _, kind := range []string{"TODO", "FIXME", "HACK", "BUG"} {
			if counts[kind] > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", counts[kind], kind))
			}
		}
		if len(parts) == 0 {
			return
		}
		m.appendInfo(i18n.T(
			"💡 Found %s in project. Type :todo to see details.",
			strings.Join(parts, ", "),
		))
	}()
	m.appendInfo(i18n.T("Ctrl+A - copy all output to clipboard."))
    m.appendInfo(i18n.T("PgUp/PgDn - browse command history."))

    m.planETA = make(map[int]time.Duration)
    m.planStart = make(map[int]time.Time)
    
	return m
}

func (m *model) Init() tea.Cmd {
	return m.input.Focus()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
    case clipboardMsg:
    	if msg.success {
    		m.appendSuccess(i18n.T("📋 Copied to clipboard (%d bytes).", msg.size))
    	} else {
    		m.appendWarn(i18n.T("Clipboard copy failed."))
    	}
    	return m, nil
	case tickMsg:
    	if m.running {
    		m.updateProgressDisplay()
    		return m, tickCmd()
    	}
    	return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.applyLayout()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.running && m.cancel != nil {
				m.cancel()
				m.status = "Cancelling..."
				return m, nil
			}
			return m, tea.Quit

		case "ctrl+d":
			if !m.running {
				return m, tea.Quit
			}
			return m, nil


        case "tab":
        	if !m.running && !m.focusOutput && len(m.suggestions) > 0 {
				m.applyNextSuggestion()
        		// m.applyFirstSuggestion()
        		return m, nil
        	}
        
        	m.focusOutput = !m.focusOutput
        
        	var cmd tea.Cmd
        	if m.focusOutput {
        		m.input.Blur()
        	} else if !m.running {
        		cmd = m.input.Focus()
        	}
        
        	m.updateStatus()
        	return m, cmd


		case "esc":
			if m.focusOutput {
				m.focusOutput = false

				var cmd tea.Cmd
				if !m.running {
					cmd = m.input.Focus()
				}

				m.updateStatus()
				return m, cmd
			}
			return m, nil

        case "f2":
        	return m, m.toggleSelectionMode()
		case "ctrl+a":
			return m, m.copyAllToClipboard()
		case "alt+enter":
			if !m.focusOutput && !m.running {
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(tea.KeyMsg{
					Type: tea.KeyEnter,
				})
				return m, cmd
			}
			return m, nil

        case "enter":
        	if !m.focusOutput && !m.running {
        		if len(m.suggestions) == 1 {
					m.applyNextSuggestion()
        			// m.applyFirstSuggestion()
        		}
        
        		q := strings.TrimSpace(m.input.Value())
        		m.input.Reset()
        		m.updateSuggestions()
        
        		if q == "" {
        			return m, nil
        		}

                m.taskQuery = q
                m.taskStartedAt = time.Now()
                m.taskStage = domain.TaskStageExecuting
                m.taskIteration = 0
                m.taskMaxIteration = 0
                m.taskTimeline = nil        

        		return m, m.submit(q)
        	}
        	return m, nil
		}

		if m.running {
			if m.focusOutput || isOutputScrollKey(msg.String()) {
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			}
			return m, nil
		}

		// Режим прокрутки вывода.
		if m.focusOutput {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

    	if isPageScrollKey(msg.String()) {
    		if msg.String() == "pgup" {
    			m.historyPrev()
    		} else {
    			m.historyNext()
    		}
    		return m, nil
    	}

		if isLineScrollKey(msg.String()) && !m.inputHasLines() {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
        m.updateSuggestions()
		return m, cmd

	case tea.MouseMsg:
		if m.running || m.focusOutput {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case eventMsg:
		e := domain.Event(msg)

        if e.TaskStage != "" {
        	m.setTaskStage(e.TaskStage)
        	m.addTimeline(e.TaskStage, e.Message)        
		}		        

        if e.Type == domain.EventDone {
        	m.agentStartTime = time.Time{}
        
        	if e.Result != nil && e.Result.Success {
        		m.setTaskStage(domain.TaskStageCompleted)
        	} else {
        		m.setTaskStage(domain.TaskStageFailed)
        	}

        	if m.streaming {
        		final := ""
        		if e.Result != nil {
        			final = e.Result.Response
        		}
        		m.finishStreaming(final)
        	}
        
        	m.running = false
        	if m.cancel != nil {
        		m.cancel()
        		m.cancel = nil
        	}
        
        	m.focusOutput = false
        
        	m.progressStart = time.Time{}
        	m.progressETA = 0
        	m.planCurrent = 0

			m.agentRole = ""
			m.agentRequests = 0
			m.agentTokens = 0
			m.agentDuration = ""

            if e.Result != nil {
            	m.appendResult(e.Result)
            	m.appendQualityGates(e.Result)
            } else {
            	m.appendLog("Done.")
            }

            if e.Result != nil {
            	m.saveTaskHistory(e.Result)
            }
       
        	m.updateStatus()
        
        	m.input.Reset()
        	m.updateSuggestions()
        	return m, m.input.Focus()
        }

		// Раскрашиваем события по типу.
		switch e.Type {
		case domain.EventWarn:
			m.appendWarn(fmt.Sprintf("[%s] %s", e.Type, e.Message))
		case domain.EventError:
			m.appendError(fmt.Sprintf("[%s] %s", e.Type, e.Message))
		case domain.EventIntent:
			m.appendIntent(e.Message)
        case domain.EventAgent:
        	if e.Agent != nil {
        		m.agentRole = e.Agent.Role
        		m.agentRequests = e.Agent.Requests
        		m.agentTokens = e.Agent.Tokens
        		if m.agentStartTime.IsZero() {
        			m.agentStartTime = time.Now()
        		}
        		if e.Agent.Kind == "done" {
        			m.agentDuration = e.Agent.Duration
        		}
        	}
        	m.updateStatus()
        case domain.EventPlan:
        	m.applyPlanEvent(e)
        case domain.EventToken:
        	m.appendStreamToken(e.Message)
        	return m, listenEvents(m.events)
        
        case domain.EventProgress:
        	m.applyProgressEvent(e)
        	return m, listenEvents(m.events)

		default:
			m.appendLog(fmt.Sprintf("[%s] %s", e.Type, e.Message))
		}

		return m, listenEvents(m.events)
	}

	return m, nil
}

func (m *model) addTimeline(stage domain.TaskStage, message string) {
	if message == "" {
		message = strings.ToUpper(string(stage))
	}

	line := fmt.Sprintf(
		"%s %s — %s",
		stage.Symbol(),
		strings.ToUpper(string(stage)),
		message,
	)

	m.taskTimeline = append(m.taskTimeline, line)

	if len(m.taskTimeline) > 20 {
		m.taskTimeline = m.taskTimeline[len(m.taskTimeline)-20:]
	}
}

func (m *model) appendAgent(s string) {
	m.appendEntry(s, logAgent)
}

func (m *model) appendIntent(s string) {
	if strings.TrimSpace(s) == "" {
		return
	}
	m.appendEntry("▸ "+s, logIntent)
}


func (m *model) View() string {
    if m.width == 0 || m.height == 0 {
        return i18n.T("Loading...")
    }
    headerText := fmt.Sprintf(
        "Gogitor TUI | %s/%s | %s",
        m.cfg.Provider,
        m.cfg.Model,
        m.cfg.WorkDir,
    )
    header := renderSingleLine(titleStyle, headerText, m.width)
    input := inputStyle.
        Width(m.inputContentWidth()).
        Render(m.input.View())
    statusText, statusTextStyle := m.statusLine()
    status := renderSingleLine(statusTextStyle, statusText, m.width)
    timeline := m.renderTaskTimeline()

    timelineHeight := 0
    if timeline != "" {
        timelineHeight = 1
    }
    inputHeight := inputTextAreaHeight + inputVerticalOverhead
    availableHeight := m.height - headerHeight - timelineHeight - inputHeight - statusHeight
    if availableHeight < 1 {
        availableHeight = 1
    }
    if m.viewport.Height > availableHeight {
        m.viewport.Height = availableHeight
    }

    return lipgloss.JoinVertical(
        lipgloss.Left,
        header,
        timeline,
        m.viewport.View(),
        input,
        status,
    )
}

func (m *model) renderTaskTimeline() string {
	if len(m.taskTimeline) == 0 {
		return ""
	}

	const maxVisible = 5

	start := 0
	if len(m.taskTimeline) > maxVisible {
		start = len(m.taskTimeline) - maxVisible
	}

	lines := m.taskTimeline[start:]

	return lipgloss.NewStyle().
		PaddingLeft(1).
		PaddingRight(1).
		Render(strings.Join(lines, "  │  "))
}

// applyPlanEvent строит доску плана и обновляет статусы пунктов по месту.
func (m *model) applyPlanEvent(e domain.Event) {
	p := e.Plan
	if p == nil {
		m.appendLog(e.Message)
		return
	}

	// Пришёл новый план — строим доску заново.
	if len(p.Items) > 0 {
		m.planItems = append([]string(nil), p.Items...)
		m.planEntries = make([]int, 0, len(p.Items))

		m.appendEntry("📋 "+i18n.T("Execution plan (goal: %s)", p.Goal), logPlanHeader)
		if len(p.Acceptance) > 0 {
			m.appendEntry(
				"   "+i18n.T("Acceptance criteria: %s", strings.Join(p.Acceptance, "; ")),
				logInfo,
			)
		}
		for i, item := range p.Items {
			m.planEntries = append(m.planEntries, len(m.entries))
			m.appendEntry(planItemLine(domain.PlanPending, i+1, item, ""), logPlanPending)
		}
		return
	}

	// Плановое сообщение (например, итог выполнения).
	if p.ItemIndex <= 0 {
		if p.Status != "" {
			m.appendEntry(p.Status.Symbol()+" "+e.Message, planKindForStatus(p.Status))
		} else {
			m.appendLog(e.Message)
		}
		return
	}

	// Обновление статуса одного пункта прямо в доске.
	idx := p.ItemIndex - 1
	if idx >= len(m.planItems) || idx >= len(m.planEntries) {
		m.appendLog(e.Message)
		return
	}
	entryIdx := m.planEntries[idx]
	if entryIdx >= len(m.entries) {
		m.appendLog(e.Message)
		return
	}

	m.entries[entryIdx] = logEntry{
		text: planItemLine(p.Status, p.ItemIndex, m.planItems[idx], p.Note),
		kind: planKindForStatus(p.Status),
	}
	m.rewrapAll()
}

func planItemLine(st domain.PlanStatus, num int, task, note string) string {
	line := fmt.Sprintf("%s %d. %s", st.Symbol(), num, task)
	if strings.TrimSpace(note) != "" {
		line += " — " + note
	}
	return line
}

func planKindForStatus(st domain.PlanStatus) logKind {
	switch st {
	case domain.PlanRunning:
		return logPlanRunning
	case domain.PlanDone:
		return logPlanDone
	case domain.PlanWarn:
		return logPlanWarn
	case domain.PlanFailed:
		return logPlanFailed
	case domain.PlanSkipped:
		return logPlanSkipped
	default:
		return logPlanPending
	}
}

func (m *model) applyLayout() {
	viewportHeight := m.height - headerHeight - inputTextAreaHeight - inputVerticalOverhead - statusHeight
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	m.viewport.Width = m.width
	m.viewport.Height = viewportHeight

	m.input.SetWidth(m.inputContentWidth())
	m.input.SetHeight(inputTextAreaHeight)

	m.setWrapWidth(m.width)
}

func (m *model) inputContentWidth() int {
	w := m.width - inputHorizontalOverhead
	if w < 4 {
		w = 4
	}
	return w
}

func (m *model) submit(q string) tea.Cmd {
	if q == ":quit" || q == ":q" {
		return tea.Quit
	}

	if q == ":cls" {
		m.clearScreen()
		return nil
	}
    if strings.HasPrefix(q, ":save") {
    		m.handleSave(q)
    		return nil
    	}
	m.historyAdd(q)
	m.appendLog("> " + q)

	// На время выполнения запроса убираем фокус из поля ввода.
	m.input.Blur()
	m.focusOutput = false

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	m.updateStatus()

	ch := make(chan domain.Event, 256)
	m.events = ch

	go func() {
		send := func(e domain.Event) {
			select {
			case ch <- e:
			case <-ctx.Done():
			}
		}

		doneSent := false

		defer func() {
			if r := recover(); r != nil {
				send(domain.Event{
					Type:    domain.EventError,
					Message: fmt.Sprintf("panic: %v", r),
				})
			}

			if !doneSent {
				send(domain.Event{
					Type: domain.EventDone,
				})
			}

			close(ch)
		}()

		res := m.svc.ProcessEvents(ctx, q, func(e domain.Event) {
			send(e)
		})

		send(domain.Event{
			Type:   domain.EventDone,
			Result: &res,
		})
		doneSent = true
	}()

    return tea.Batch(listenEvents(ch), tickCmd())
}

func (m *model) appendStreamToken(s string) {
	if s == "" {
		return
	}

	if !m.streaming {
		m.streaming = true
		m.streamBuf.Reset()
		m.appendEntry("▍", logPlain)
	}

	m.streamBuf.WriteString(s)
	m.setLastEntry(m.streamBuf.String()+" ▍", logPlain)
}

func (m *model) finishStreaming(final string) {
	if !m.streaming {
		return
	}

	m.streaming = false

	text := final
	if strings.TrimSpace(text) == "" {
		text = m.streamBuf.String()
	}

	if len(m.entries) > 0 {
		m.entries[len(m.entries)-1] = logEntry{
			text: text,
			kind: logMarkdown,
		}
	}

	m.rewrapAll()
	m.streamBuf.Reset()
	m.justStreamed = strings.TrimSpace(final) != ""
}

func (m *model) setLastEntry(text string, kind logKind) {
	if len(m.entries) == 0 {
		m.appendEntry(text, kind)
		return
	}

	m.entries[len(m.entries)-1] = logEntry{
		text: text,
		kind: kind,
	}

	m.rewrapAll()
}

func (m *model) applyProgressEvent(e domain.Event) {
	if e.Progress == nil {
		return
	}

	p := e.Progress

	eta := time.Duration(p.ETASeconds) * time.Second

	m.progressStart = time.Now()
	m.progressETA = eta

	if p.ItemIndex > 0 {
		m.planCurrent = p.ItemIndex

		if m.planETA == nil {
			m.planETA = make(map[int]time.Duration)
		}
		if m.planStart == nil {
			m.planStart = make(map[int]time.Time)
		}

		m.planETA[p.ItemIndex] = eta
		m.planStart[p.ItemIndex] = time.Now()
	}

	m.updateStatus()
}

func (m *model) updateProgressDisplay() {
	if !m.running {
		return
	}

	if m.agentRole != "" {
		m.updateStatus()
	}

	if m.progressStart.IsZero() {
		return
	}
	elapsed := time.Since(m.progressStart)

	if m.planCurrent > 0 {
		idx := m.planCurrent - 1

		if idx >= 0 && idx < len(m.planItems) && idx < len(m.planEntries) {
			entryIdx := m.planEntries[idx]

			if entryIdx < len(m.entries) {
				eta := m.planETA[m.planCurrent]
				note := formatProgressNote(elapsed, eta)

				m.entries[entryIdx] = logEntry{
					text: planItemLine(
						domain.PlanRunning,
						m.planCurrent,
						m.planItems[idx],
						note,
					),
					kind: logPlanRunning,
				}

				m.rewrapAll()
			}
		}
	}

	m.updateStatus()
}

func formatProgressNote(elapsed, eta time.Duration) string {
	if eta <= 0 {
		return elapsed.Round(time.Second).String()
	}

	pct := float64(elapsed) / float64(eta)
	if pct > 0.95 {
		pct = 0.95
	}

	return fmt.Sprintf(
		"%s / ~%s (%.0f%%)",
		elapsed.Round(time.Second),
		eta.Round(time.Second),
		pct*100,
	)
}

func listenEvents(ch chan domain.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return eventMsg(domain.Event{
				Type: domain.EventDone,
			})
		}
		return eventMsg(e)
	}
}

func (m *model) appendEntry(s string, kind logKind) {
	if s == "" {
		return
	}

	s = strings.ReplaceAll(s, "\r", "")

	entry := logEntry{
		text: s,
		kind: kind,
	}

	m.entries = append(m.entries, entry)
	m.renderEntry(entry)
	m.updateViewportContent()
}

func (m *model) appendLog(s string) {
	m.appendEntry(s, logPlain)
}

func (m *model) appendInfo(s string) {
	m.appendEntry(s, logInfo)
}

func (m *model) appendSuccess(s string) {
	m.appendEntry(s, logSuccess)
}

func (m *model) appendWarn(s string) {
	m.appendEntry(s, logWarn)
}

func (m *model) appendError(s string) {
	m.appendEntry(s, logError)
}

func (m *model) appendCreated(s string) {
	m.appendEntry(s, logCreated)
}

func (m *model) appendModified(s string) {
	m.appendEntry(s, logModified)
}

func (m *model) appendGit(s string) {
	m.appendEntry(s, logGit)
}

func (m *model) appendMarkdown(s string) {
	if strings.TrimSpace(s) == "" {
		return
	}

	s = strings.ReplaceAll(s, "\r", "")

	entry := logEntry{
		text: s,
		kind: logMarkdown,
	}

	m.entries = append(m.entries, entry)
	m.renderEntry(entry)
	m.updateViewportContent()
}



func (m *model) appendResult(res *domain.Result) {
	if res == nil {
		m.appendLog(i18n.T("Done."))
		return
	}
    m.lastResult = res
	if m.cfg.OutputFile != "" {
		if err := app.SaveResultToFile(*res, m.cfg.OutputFile); err != nil {
            m.appendWarn(i18n.T("Auto-save failed: %v", err))
		} else {
			m.appendInfo(i18n.T("Result saved to: %s", m.cfg.OutputFile))
		}
	}

	// Если ответ уже был показан через стриминг, не дублируем его.
	if m.justStreamed && isMarkdownMode(res.Mode) && strings.TrimSpace(res.Response) != "" {
		m.justStreamed = false
		m.appendResultDetails(res)
		return
	}

	m.justStreamed = false

	if res.Comparison != nil && res.AwaitingSelection {
		m.appendMarkdown(res.Response)
		return
	}

	if isMarkdownMode(res.Mode) && strings.TrimSpace(res.Response) != "" {
		m.appendMarkdown(res.Response)
	} else if strings.TrimSpace(res.Response) != "" {
		m.appendLog(i18n.Localize(res.Response))
	}

	m.appendResultDetails(res)
}


func (m *model) appendResultDetails(res *domain.Result) {
	if res == nil {
		return
	}

	if res.SelectedApproach != "" {
		m.appendInfo(i18n.T("Selected approach: %s", truncateStr(res.SelectedApproach, 200)))
	}

	if len(res.FilesCreated) > 0 {
		m.appendCreated(i18n.T("Created files:"))
		for _, f := range res.FilesCreated {
			m.appendCreated("  + " + f)
		}
	}

	if len(res.FilesModified) > 0 {
		m.appendModified(i18n.T("Modified files:"))
		for _, f := range res.FilesModified {
			m.appendModified("  ~ " + f)
		}
	}

	if len(res.FilesPatched) > 0 {
		m.appendModified(i18n.T("Patched files (DIFF):"))
		for _, f := range res.FilesPatched {
			m.appendModified("  Δ " + f)
		}
	}

	if len(res.FilesFullRewritten) > 0 {
		m.appendModified(i18n.T("Full rewritten files:"))
		for _, f := range res.FilesFullRewritten {
			m.appendModified("  ≡ " + f)
		}
	}

	if res.Tests.Run {
		msg := tuiTestsMessage(res.Tests)

		if res.Tests.Failed > 0 {
			m.appendError(msg)
		} else {
			m.appendSuccess(msg)
		}
	} else if res.Tests.Skipped {
		m.appendInfo(i18n.T("Tests: skipped"))
	}
    if res.Lint.Run {
    	if res.Lint.Passed {
    		m.appendSuccess(i18n.T("Lint: passed (0 issues)"))
    	} else if res.Lint.Issues > 0 {
    		m.appendWarn(i18n.T("Lint: %d issue(s) found", res.Lint.Issues))
    	}
    }

	if res.GitCommit != "" {
		m.appendGit(i18n.T("Git commit:") + " " + res.GitCommit)
	}

	if res.CumulativeDiff != "" {
		m.appendGit(i18n.T("Task changes (cumulative diff):"))
		m.appendDiff(res.CumulativeDiff)
	}

	if len(res.Warnings) > 0 {
		m.appendWarn(i18n.T("Warnings:"))
		for _, w := range res.Warnings {
			m.appendWarn("  ! " + i18n.Localize(w))
		}
	}

	if len(res.Errors) > 0 {
		m.appendError(i18n.T("Errors:"))
		for _, e := range res.Errors {
			m.appendError("  × " + i18n.Localize(e))
		}
	}

	if m.svc != nil {
		snap := m.svc.LLMSnapshotData()
		if snap.Requests > 0 {
			m.appendLog(i18n.T(
				"LLM usage: %d requests, %d tokens, %s",
				snap.Requests,
				snap.EstimatedTokens,
				snap.Duration.Round(time.Millisecond),
			))
		}
	}

	if res.Success {
		m.appendSuccess(i18n.T("SUCCESS"))
	} else {
		m.appendError(i18n.T("FAILED"))
	}
}

func truncateStr(s string, max int) string {
	return textutil.LimitRunes(s, max, "...")
}

func tuiTestsMessage(t domain.TestsStatus) string {
	suffix := ""

	if strings.TrimSpace(t.CoverageOutput) != "" {
		suffix = fmt.Sprintf(" (%s)", strings.TrimSpace(t.CoverageOutput))
		suffix = i18n.Localize(suffix)
	} else if t.Coverage > 0 {
		suffix = i18n.T(" (coverage: %.1f%%)", t.Coverage)
	}

	return i18n.T("Tests: passed=%d failed=%d%s", t.Passed, t.Failed, suffix)
}

func (m *model) renderEntry(entry logEntry) {
	if entry.kind == logMarkdown {
		rendered := m.renderMarkdown(entry.text)
		m.wrapped.WriteString(rendered)
		m.wrapped.WriteByte('\n')
		return
	}

	lines := strings.Split(strings.TrimRight(entry.text, "\n"), "\n")
	for _, line := range lines {
		m.wrapped.WriteString(m.renderLogLine(line, entry.kind))
		m.wrapped.WriteByte('\n')
	}
}

func (m *model) renderLogLine(line string, kind logKind) string {
	if line == "" {
		return ""
	}

	wrapped := m.wrapLine(line)

	if kind == logPlain {
		return wrapped
	}

	return styleForKind(kind).Render(wrapped)
}

func styleForKind(kind logKind) lipgloss.Style {
	switch kind {
	case logInfo:
		return infoStyle
	case logSuccess:
		return successStyle
	case logWarn:
		return warnStyle
	case logError:
		return errorStyle
	case logCreated:
		return createdStyle
	case logModified:
		return modifiedStyle
	case logGit:
		return gitStyle
	case logIntent:
		return intentStyle
	case logAgent:
		return agentStyle
	case logPlanHeader:
		return planHeaderStyle
	case logPlanPending:
		return planPendingStyle
	case logPlanRunning:
		return planRunningStyle
	case logPlanDone:
		return planDoneStyle
	case logPlanWarn:
		return planWarnStyle
	case logPlanFailed:
		return planFailedStyle
	case logPlanSkipped:
		return planSkippedStyle
	case logDiffAdd:
		return diffAddStyle
	case logDiffDel:
		return diffDelStyle
	case logDiffHunk:
		return diffHunkStyle
	case logDiffMeta:
		return diffMetaStyle
	default:
		return lipgloss.NewStyle()
	}
}

func (m *model) clearScreen() {
	m.entries = nil
	m.planItems = nil
	m.planEntries = nil  
	m.wrapped.Reset()
	m.viewport.SetContent("")
	m.viewport.GotoTop()
}

func (m *model) setWrapWidth(width int) {
	if width <= 0 {
		return
	}

	if width == m.wrapWidth {
		return
	}

	m.wrapWidth = width
	m.rewrapAll()
}

func (m *model) rewrapAll() {
	m.wrapped.Reset()

	for _, entry := range m.entries {
		m.renderEntry(entry)
	}

	m.updateViewportContent()
}

func (m *model) wrapLine(line string) string {
	if m.wrapWidth <= 0 {
		return line
	}

	if line == "" {
		return ""
	}

	return lipgloss.NewStyle().
		Width(m.wrapWidth).
		Render(line)
}

func (m *model) renderMarkdown(text string) string {
	width := m.wrapWidth
	if width <= 0 {
		width = 80
	}

	renderer, err := m.markdownRenderer(width)
	if err != nil {
		return m.renderPlain(text)
	}

	out, err := renderer.Render(text)
	if err != nil || strings.TrimSpace(out) == "" {
		return m.renderPlain(text)
	}

	return strings.TrimRight(out, "\n")
}

func (m *model) markdownRenderer(width int) (*glamour.TermRenderer, error) {
	if width <= 0 {
		width = 80
	}

	style := markdownStyleName()

	if m.mdRenderer != nil && m.mdRendererWidth == width && m.mdRendererStyle == style {
		return m.mdRenderer, nil
	}

	// Важно: не используем glamour.WithAutoStyle(), потому что автоопределение
	// темы может отправлять терминалу запросы и получать ответы, которые
	// визуально попадают в поле ввода.
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		renderer, err = glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(width),
		)
		if err != nil {
			return nil, err
		}
		style = "dark"
	}

	m.mdRenderer = renderer
	m.mdRendererWidth = width
	m.mdRendererStyle = style

	return renderer, nil
}

func (m *model) renderPlain(text string) string {
	var b strings.Builder

	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for _, line := range lines {
		b.WriteString(m.wrapLine(line))
		b.WriteByte('\n')
	}

	return strings.TrimRight(b.String(), "\n")
}

func (m *model) updateViewportContent() {
	if m.viewport.Height <= 0 {
		return
	}

	atBottom := m.viewport.AtBottom()
	oldY := m.viewport.YOffset

	m.viewport.SetContent(m.wrapped.String())

	if atBottom {
		m.viewport.GotoBottom()
		return
	}

	m.viewport.YOffset = oldY
}

func (m *model) updateStatus() {
	if m.mouseFree {
		m.status = i18n.T("Text selection: select with mouse and copy via terminal | PgUp/PgDn scroll | F2 back")
		return
	}

	if m.running {
		if m.agentRole != "" {
			dur := m.agentDuration
			if !m.agentStartTime.IsZero() {
				dur = time.Since(m.agentStartTime).Round(100 * time.Millisecond).String()
			}
			if dur == "" {
				dur = "0s"
			}
			tokRate := ""
			if m.agentTokens > 0 && !m.agentStartTime.IsZero() {
				elapsed := time.Since(m.agentStartTime).Seconds()
				if elapsed > 1 {
					tokRate = fmt.Sprintf(" | %d tok/s", int(float64(m.agentTokens)/elapsed))
				}
			}
			m.status = i18n.T(
				"%s | %d req | ≈%d tok%s | Tab: output | Ctrl+C: cancel | F2: select",
				m.agentRole, m.agentRequests, m.agentTokens, tokRate,
				// "%s | %d req | ≈%d tok%s | %s | Tab: output | Ctrl+C: cancel | F2: select",
				// m.agentRole, m.agentRequests, m.agentTokens, tokRate, dur,
			)
		} else if !m.progressStart.IsZero() && m.progressETA > 0 {
			m.status = i18n.T(
				"%s | Tab: output | Ctrl+C: cancel | F2: select text",
				formatProgressNote(time.Since(m.progressStart), m.progressETA),
			)
		} else {
			m.status = i18n.T("Running... | Tab: output | Ctrl+C: cancel | F2: select text")
		}
		return
	}
	if m.focusOutput {
		m.status = i18n.T("Output focus: arrows/PgUp/PgDn/mouse scroll | Tab or Esc: back to input | F2: select text | Ctrl+C quit")
		return
	}
	m.status = i18n.T(
		"%s / %s | Enter send | Alt+Enter newline | Ctrl+A copy | PgUp/PgDn history | F2: select | Ctrl+C quit",
		m.cfg.Provider,
		m.cfg.Model,
	)
}

func isMarkdownMode(mode string) bool {
	switch mode {
	case "chat", "analyze", "search", "article",
		"workflow-interview", "workflow-reflect", "workflow-pr",
		"workflow-plan-review":
		return true
	default:
		return false
	}
}

func renderSingleLine(style lipgloss.Style, s string, width int) string {
	if width <= 0 {
		return style.Render(s)
	}

	truncated := lipgloss.NewStyle().
		MaxWidth(width).
		Render(s)

	return style.
		Width(width).
		Render(truncated)
}

// isOutputScrollKey используется, когда вывод прокручивается
// в фокусе или во время выполнения запроса.
func isOutputScrollKey(key string) bool {
	switch key {
	case "pgup", "pgdown", "up", "down", "home", "end":
		return true
	default:
		return false
	}
}

func isPageScrollKey(key string) bool {
	return key == "pgup" || key == "pgdown"
}

func isLineScrollKey(key string) bool {
	return key == "up" || key == "down"
}

func (m *model) inputHasLines() bool {
	return strings.Contains(m.input.Value(), "\n")
}

func (m *model) statusLine() (string, lipgloss.Style) {
	if !m.running && !m.focusOutput && len(m.suggestions) > 0 {
		return "Tab: " + strings.Join(m.suggestions, "  "), suggestionStyle
	}
	stage := m.taskStageText()
	if stage != "" {
		if m.status != "" {
			return stage + " | " + m.status, statusStyle
		}
		return stage, statusStyle
	}
	// В режиме ожидания показываем расширенную информацию.
	if !m.running {
		info := fmt.Sprintf("%s/%s", m.cfg.Provider, m.cfg.Model)
		if m.svc != nil {
			snap := m.svc.LLMSnapshotData()
			if snap.Requests > 0 {
				info += fmt.Sprintf(" | %d req | ≈%d tok", snap.Requests, snap.EstimatedTokens)
			}
		}
        info += i18n.T(" | Enter send | :help")
		return info, statusStyle
	}
	return m.status, statusStyle
}

func (m *model) applyNextSuggestion() {
    if len(m.suggestions) == 0 {
        return
    }
    if m.suggestionIdx >= len(m.suggestions) {
        m.suggestionIdx = 0
    }
    current := strings.TrimSpace(m.input.Value())
    chosen := m.suggestions[m.suggestionIdx]
    if len(current) <= len(chosen) || strings.HasPrefix(chosen, current) {
        m.input.SetValue(chosen)
    }
    m.suggestionIdx++
}

func (m *model) applyFirstSuggestion() {
	if len(m.suggestions) == 0 {
		return
	}
	current := strings.TrimSpace(m.input.Value())
	if len(current) > len(m.suggestions[0]) {
		return
	}
	m.input.SetValue(m.suggestions[0])
	m.updateSuggestions()
}

const maxHistorySize = 100

func (m *model) historyAdd(cmd string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	// Не дублируем подряд идущие одинаковые команды.
	if len(m.history) > 0 && m.history[len(m.history)-1] == cmd {
		m.historyIndex = -1
		m.savedInput = ""
		return
	}
	m.history = append(m.history, cmd)
	// Ограничиваем размер кеша.
	if len(m.history) > maxHistorySize {
		m.history = m.history[len(m.history)-maxHistorySize:]
	}
	m.historyIndex = -1
	m.savedInput = ""
}

func (m *model) historyPrev() {
	if len(m.history) == 0 {
		return
	}
	if m.historyIndex == -1 {
		// Первый вход в историю — сохраняем текущий ввод.
		m.savedInput = m.input.Value()
		m.historyIndex = len(m.history) - 1
	} else if m.historyIndex > 0 {
		m.historyIndex--
	} else {
		// Уже на самой старой команде — ничего не делаем.
		return
	}
	m.input.SetValue(m.history[m.historyIndex])
	m.input.CursorEnd()
	m.updateSuggestions()
}

func (m *model) historyNext() {
	if m.historyIndex == -1 {
		// Не в режиме истории — ничего не делаем.
		return
	}
	if m.historyIndex < len(m.history)-1 {
		m.historyIndex++
		m.input.SetValue(m.history[m.historyIndex])
		m.input.CursorEnd()
	} else {
		// Дошли до конца истории — возвращаем сохранённый ввод.
		m.historyIndex = -1
		m.input.SetValue(m.savedInput)
		m.input.CursorEnd()
		m.savedInput = ""
	}
	m.updateSuggestions()
}

// updateSuggestions пересчитывает список подсказок для команд.
func (m *model) updateSuggestions() {
	m.suggestions = nil
	m.suggestionIdx = 0

	value := m.input.Value()
	if value == "" || !strings.HasPrefix(value, ":") || strings.Contains(value, "\n") {
		return
	}
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return
	}
	first := fields[0]
    if first == ":test" && (len(fields) > 1 || strings.HasSuffix(value, " ")) {
    	if len(fields) > 2 {
    		return
    	}
    	prefix := ""
    	if len(fields) > 1 {
    		prefix = fields[1]
    	}
    	for _, sub := range testSubcommandSuggestions {
    		if strings.HasPrefix(sub, prefix) {
    			m.suggestions = append(m.suggestions, ":test "+sub)
    		}
    	}
    	if len(m.suggestions) == 1 && m.suggestions[0] == strings.TrimSpace(value) {
    		m.suggestions = nil
    	}
    	return
    }

    if first == ":reasoning" && (len(fields) > 1 || strings.HasSuffix(value, " ")) {
		if len(fields) > 2 {
			return
		}
		prefix := ""
		if len(fields) > 1 {
			prefix = fields[1]
		}
		for _, sub := range reasoningSubcommandSuggestions {
			if strings.HasPrefix(sub, prefix) {
				m.suggestions = append(m.suggestions, ":reasoning "+sub)
			}
		}
		if len(m.suggestions) == 1 && m.suggestions[0] == strings.TrimSpace(value) {
			m.suggestions = nil
		}
		return
	}
    if first == ":workflow" && (len(fields) > 1 || strings.HasSuffix(value, " ")) {
	    if len(fields) > 2 {
		    return
	    }
	    prefix := ""
	    if len(fields) > 1 {
		    prefix = fields[1]
	    }

        workflowSubs := []string{"interview", "reflect", "pr", "plan-review"}
	    for _, sub := range workflowSubs {
		    if strings.HasPrefix(sub, prefix) {
			    m.suggestions = append(m.suggestions, ":workflow "+sub)
		    }
	    }
        if len(m.suggestions) == 1 && m.suggestions[0] == strings.TrimSpace(value) {
 	       m.suggestions = nil
        }
	    return
	}

	if first == ":git" && (len(fields) > 1 || strings.HasSuffix(value, " ")) {
		if len(fields) > 2 {
			return
		}
		prefix := ""
		if len(fields) > 1 {
			prefix = fields[1]
		}
		for _, sub := range gitSubcommandSuggestions {
			if strings.HasPrefix(sub, prefix) {
				m.suggestions = append(m.suggestions, ":git "+sub)
			}
		}
		if len(m.suggestions) == 1 && m.suggestions[0] == strings.TrimSpace(value) {
			m.suggestions = nil
		}
        // После определения prefix для git-подкоманд:
        if prefix == "commit" || strings.HasPrefix(prefix, "commit ") {
            // Показываем подсказку --split
            m.suggestions = append(m.suggestions, ":git commit --split ")
        }
		return
	}

	if len(fields) > 1 {
		return
	}

	prefix := strings.TrimSpace(value)

	for _, cmd := range commandSuggestions {
		if strings.HasPrefix(cmd, prefix) {
			m.suggestions = append(m.suggestions, cmd)
		}
	}

	// Если введено уже точное имя команды, подсказку можно не показывать.
	if len(m.suggestions) == 1 && m.suggestions[0] == prefix {
		m.suggestions = nil
	}
}

func markdownStyleName() string {
	for _, envName := range []string{
		"GOGITOR_MARKDOWN_STYLE",
		"GLAMOUR_STYLE",
	} {
		v := strings.ToLower(strings.TrimSpace(os.Getenv(envName)))
		switch v {
		case "light", "dark", "notty":
			return v
		}
	}

	// Определяем светлую/тёмную тему без интерактивного запроса к терминалу.
	// COLORFGBG обычно имеет формат: foreground;background
	if v := strings.TrimSpace(os.Getenv("COLORFGBG")); v != "" {
		parts := strings.Split(v, ";")
		bg := strings.TrimSpace(parts[len(parts)-1])

		switch bg {
		case "7", "10", "15":
			return "light"
		default:
			return "dark"
		}
	}

	return "dark"
}

type mouseReleaser interface {
	ReleaseMouse()
}

type mouseCellEnabler interface {
	EnableMouseCellMotion()
}

type mouseToggleMsg struct{}

type clipboardMsg struct {
	success bool
	size    int
}

func (m *model) copyAllToClipboard() tea.Cmd {
	content := m.plainTextContent()
	if strings.TrimSpace(content) == "" {
		m.appendWarn(i18n.T("Nothing to copy: output is empty."))
		return nil
	}
	size := len(content)
	return func() tea.Msg {
		// Сначала пробуем системные утилиты буфера обмена.
		if copyToSystemClipboard(content) {
			return clipboardMsg{success: true, size: size}
		}
		// Fallback: OSC 52 (работает не во всех терминалах).
		encoded := base64.StdEncoding.EncodeToString([]byte(content))
		fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\x07", encoded)
		return clipboardMsg{success: true, size: size}
	}
}

func copyToSystemClipboard(content string) bool {
	type tool struct {
		name string
		args []string
	}
	tools := []tool{
		{"xclip", []string{"-selection", "clipboard"}},
		{"xsel", []string{"--clipboard", "--input"}},
		{"wl-copy", nil},
		{"pbcopy", nil},
	}
	for _, t := range tools {
		if _, err := exec.LookPath(t.name); err != nil {
			continue
		}
		cmd := exec.Command(t.name, t.args...)
		cmd.Stdin = strings.NewReader(content)
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	return false
}

func (m *model) plainTextContent() string {
	var b strings.Builder
	for _, entry := range m.entries {
		b.WriteString(entry.text)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *model) toggleSelectionMode() tea.Cmd {
	if m.prog == nil {
		return nil
	}

	m.mouseFree = !m.mouseFree

	if m.mouseFree {
		if r, ok := any(m.prog).(mouseReleaser); ok {
			r.ReleaseMouse()
			m.updateStatus()
			return nil
		}

		m.updateStatus()
		return disableMouseReportingCmd()
	}

	if e, ok := any(m.prog).(mouseCellEnabler); ok {
		e.EnableMouseCellMotion()
		m.updateStatus()
		return nil
	}

	m.updateStatus()
	return enableMouseReportingCmd()
}


func disableMouseReportingCmd() tea.Cmd {
	return func() tea.Msg {
		fmt.Fprint(os.Stdout, "\x1b[?1002l\x1b[?1006l\x1b[?1000l")
		return mouseToggleMsg{}
	}
}

func enableMouseReportingCmd() tea.Cmd {
	return func() tea.Msg {
		fmt.Fprint(os.Stdout, "\x1b[?1002h\x1b[?1006h")
		return mouseToggleMsg{}
	}
}

func (m *model) handleSave(q string) {
	parts := strings.Fields(q)
	if len(parts) < 2 {
		m.appendWarn(i18n.T("usage: :save <filename>"))
		return
	}
	path := parts[1]
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	if m.lastResult == nil {
		m.appendWarn(i18n.T("No result to save. Run a task first."))
		return
	}
	if err := app.SaveResultToFile(*m.lastResult, path); err != nil {
		m.appendError(fmt.Sprintf("Cannot save to %s: %v", path, err))
		return
	}
	m.appendSuccess(i18n.T("Result saved to: %s", path))
}

func diffStats(diff string) (added, removed int) {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++") ||
			strings.HasPrefix(line, "---") {
			continue
		}

		switch {
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}

	return added, removed
}

func (m *model) showLastTaskDiff() {
	if m.lastResult == nil {
		m.appendWarn(i18n.T("No completed task yet."))
		return
	}

	diff := strings.TrimSpace(m.lastResult.CumulativeDiff)

	if diff == "" {
		m.appendInfo(i18n.T("The last task produced no Git diff."))
		return
	}

    added, removed := diffStats(diff)

    m.appendEntry(
        i18n.T(
            "── Task Diff: +%d / -%d ─────────────────",
            added,
            removed,
        ),
        logDiffMeta,
    )
    m.appendDiff(diff)
    m.appendEntry(i18n.T("── End Task Diff ─────────────────────"), logDiffMeta)    
}

func (m *model) appendDiff(diff string) {
	if strings.TrimSpace(diff) == "" {
		return
	}
	lines := strings.Split(diff, "\n")
	const maxLines = 60
	for i, line := range lines {
		if i >= maxLines {
            m.appendEntry(
                i18n.T("... (%d more lines, use :git diff-task for full diff)", len(lines)-maxLines),
                logDiffMeta,
            )
			break
		}
		kind := logDiffMeta
		switch {
		case strings.HasPrefix(line, "+"):
			kind = logDiffAdd
		case strings.HasPrefix(line, "-"):
			kind = logDiffDel
		case strings.HasPrefix(line, "@@"):
			kind = logDiffHunk
		case strings.HasPrefix(line, "diff "),
			strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "--- "),
			strings.HasPrefix(line, "+++ "):
			kind = logDiffMeta
		}
		m.appendEntry(line, kind)
	}
}

func (m *model) setTaskStage(stage domain.TaskStage) {
	m.taskStage = stage

	if stage == domain.TaskStageIdle {
		m.taskStartedAt = time.Time{}
		return
	}

	if m.taskStartedAt.IsZero() {
		m.taskStartedAt = time.Now()
	}
}

func (m *model) taskStageText() string {
	if m.taskStage == domain.TaskStageIdle {
		return ""
	}

	text := fmt.Sprintf("%s %s", m.taskStage.Symbol(), strings.ToUpper(string(m.taskStage)))

	if m.taskIteration > 0 && m.taskMaxIteration > 0 {
		text += fmt.Sprintf(" %d/%d", m.taskIteration, m.taskMaxIteration)
	}

	if !m.taskStartedAt.IsZero() {
		text += " • " + formatElapsed(time.Since(m.taskStartedAt))
	}

	return text
}

func formatElapsed(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}

	d = d.Round(time.Second)

	h := d / time.Hour
	d -= h * time.Hour

	min := d / time.Minute
	d -= min * time.Minute

	sec := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, min)
	}

	if min > 0 {
		return fmt.Sprintf("%dm %02ds", min, sec)
	}

	return fmt.Sprintf("%ds", sec)
}

func (m *model) appendQualityGates(result *domain.Result) {
	if result == nil {
		return
	}

	q := result.QualityGates

	if !q.Build &&
		!q.Tests &&
		!q.Vet &&
		!q.Gofmt &&
		!q.LintInstalled {
		return
	}

	m.appendEntry(i18n.T("Quality Gates"), logPlanHeader)
	m.appendEntry(
		fmt.Sprintf("%s build", gateSymbol(q.Build)),
		logInfo,
	)

	m.appendEntry(
		fmt.Sprintf(
			"%s tests (%d passed, %d failed)",
			gateSymbol(q.Tests),
			q.TestsPassed,
			q.TestsFailed,
		),
		logInfo,
	)

	m.appendEntry(
		fmt.Sprintf("%s vet", gateSymbol(q.Vet)),
		logInfo,
	)

	m.appendEntry(
		fmt.Sprintf("%s gofmt", gateSymbol(q.Gofmt)),
		logInfo,
	)

	if q.LintInstalled {
		m.appendEntry(
			fmt.Sprintf(
				"%s lint (%d issues)",
				gateSymbol(q.Lint),
				q.LintIssues,
			),
			logInfo,
		)
	}

	if q.Coverage > 0 {
		m.appendEntry(
			fmt.Sprintf("coverage %.1f%%", q.Coverage),
			logInfo,
		)
	}
}

func gateSymbol(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}

func (m *model) saveTaskHistory(result *domain.Result) {
	if result == nil || strings.TrimSpace(m.taskQuery) == "" {
		return
	}

	added, removed := diffStats(result.CumulativeDiff)

	entry := domain.TaskHistoryEntry{
		Time:         time.Now(),
		Query:        m.taskQuery,
		Mode:         result.Mode,
		Success:      result.Success,
		Iterations:   result.Iterations,
		Files:        append(
			append([]string{}, result.FilesCreated...),
			result.FilesModified...,
		),
		AddedLines:   added,
		RemovedLines: removed,
		GitCommit:    result.GitCommit,
	}

	path := filepath.Join(
		m.cfg.WorkDir,
		".gogitor",
		"task_history.json",
	)

	if err := appendTaskHistory(path, entry); err != nil {
		m.log.Debug("cannot save task history", "err", err)
	}
}

func appendTaskHistory(path string, entry domain.TaskHistoryEntry) error {
	var history []domain.TaskHistoryEntry

	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &history)
	}

	entry.ID = len(history) + 1
	history = append(history, entry)

	const maxEntries = 100

	if len(history) > maxEntries {
		history = history[len(history)-maxEntries:]
	}

	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

func (m *model) showTaskHistory() {
	path := filepath.Join(
		m.cfg.WorkDir,
		".gogitor",
		"task_history.json",
	)

	data, err := os.ReadFile(path)
	if err != nil {
		m.appendInfo(i18n.T("Task history is empty."))
		return
	}

	var history []domain.TaskHistoryEntry
	if err := json.Unmarshal(data, &history); err != nil {
        m.appendError(
            i18n.T("Cannot read task history: %v", err),
        )
		return
	}
    m.appendEntry(
        i18n.T("── Task History ─────────────────────"),
        logPlanHeader,
    )

	start := 0
	if len(history) > 20 {
		start = len(history) - 20
	}

	for _, item := range history[start:] {
		status := "✗"
		if item.Success {
			status = "✓"
		}

		files := len(item.Files)

		m.appendEntry(
			fmt.Sprintf(
				"%s #%d %s — %s [%s, %d files, %d/%d lines]",
				status,
				item.ID,
				item.Time.Format("2006-01-02 15:04"),
				textutil.LimitRunes(item.Query, 80, "..."),
				item.Mode,
				files,
				item.AddedLines,
				item.RemovedLines,
			),
			logInfo,
		)
	}
}