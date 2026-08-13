package i18n

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

type Lang string

const (
	EN Lang = "en"
	RU Lang = "ru"
)

var (
	mu      sync.RWMutex
	current Lang = EN
)

func init() {
	SetLang(Detect())
}

// Detect определяет язык пользователя по окружению.
// Дополнительно поддерживается GOGITOR_LANG=ru/en.
func Detect() Lang {
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("GOGITOR_LANG"))); v != "" {
		if strings.HasPrefix(v, "ru") {
			return RU
		}
		if strings.HasPrefix(v, "en") {
			return EN
		}
	}

	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := strings.TrimSpace(os.Getenv(env))
		if v == "" {
			continue
		}

		lower := strings.ToLower(v)
		if lower == "c" || lower == "posix" {
			continue
		}

		if strings.HasPrefix(lower, "ru") {
			return RU
		}

		return EN
	}

	return EN
}

func SetLang(l Lang) {
	if l == "" {
		return
	}

	mu.Lock()
	current = l
	mu.Unlock()
}

func Current() Lang {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// T переводит строку-ключ.
// Английская строка используется как ключ, например:
//
//	i18n.T("Mode: %s", mode)
func T(key string, args ...any) string {
	return Tr(Current(), key, args...)
}

// Tr переводит строку для конкретного языка.
func Tr(l Lang, key string, args ...any) string {
	mu.RLock()
	defer mu.RUnlock()

	if msg, ok := messages[l][key]; ok {
		return sprintf(msg, args...)
	}

	if l != EN {
		if msg, ok := messages[EN][key]; ok {
			return sprintf(msg, args...)
		}
	}

	return sprintf(key, args...)
}

func sprintf(tpl string, args ...any) string {
	if len(args) == 0 {
		return tpl
	}
	return fmt.Sprintf(tpl, args...)
}

// Localize переводит уже сформированное сообщение, если оно состоит
// из известных шаблонов. Это нужно для совместимости со старыми местами,
// где сообщение уже склеено через fmt.Sprintf.
func Localize(msg string) string {
	if msg == "" {
		return msg
	}

	lang := Current()
	if lang != RU {
		return msg
	}

	lines := strings.Split(msg, "\n")
	for i, line := range lines {
		lines[i] = localizeLine(lang, line)
	}

	return strings.Join(lines, "\n")
}

type pattern struct {
	re   *regexp.Regexp
	repl string
}

func localizeLine(l Lang, line string) string {
	if strings.TrimSpace(line) == "" {
		return line
	}

	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	trimmed := strings.TrimSpace(line)

	out := ""

	if tr, ok := messages[l][trimmed]; ok {
		out = indent + sprintf(tr)
	} else {
		out = line

		for _, p := range ruPatterns {
			if p.re.MatchString(trimmed) {
				out = indent + p.re.ReplaceAllString(trimmed, p.repl)
				break
			}
		}
	}

	// Дополнительные inline-замены для коротких фрагментов,
	// например "2 created" -> "2 создано".
	for _, p := range ruInlinePatterns {
		out = p.re.ReplaceAllString(out, p.repl)
	}

	return out
}

// messages содержит переводы для явных ключей.
// Английский текст используется как ключ.
var messages = map[Lang]map[string]string{
	EN: map[string]string{},
	RU: map[string]string{
        " | Enter send | :help": " | Enter отправить | :help",
        // ─── TUI: сохранение и история ─────────────────────────────
        "usage: :save <filename>":
            "использование: :save <имя-файла>",
        "No result to save. Run a task first.":
            "Нет результата для сохранения. Сначала выполните задачу.",
        "Result saved to: %s":
            "Результат сохранён в: %s",
        "No completed task yet.":
            "Завершённых задач пока нет.",
        "The last task produced no Git diff.":
            "Последняя задача не создала Git diff.",
        "Task history is empty.":
            "История задач пуста.",
        "Auto-save failed: %v":
            "Автосохранение не удалось: %v",
        "Cannot read task history: %v":
            "Не удалось прочитать историю задач: %v",
        
        // ─── TUI: заголовки блоков ─────────────────────────────────
        "── Task History ─────────────────────":
            "── История задач ─────────────────────",
        "── Task Diff: +%d / -%d ─────────────────":
            "── Diff задачи: +%d / -%d ─────────────────",
        "── End Task Diff ─────────────────────":
            "── Конец Diff задачи ─────────────────────",
        "Quality Gates":
            "Контроль качества",
        "... (%d more lines, use :git diff-task for full diff)":
            "... (ещё %d строк, используйте :git diff-task для полного diff)",
        
        // ─── Computer Mode ─────────────────────────────────────────
        "Detecting OS...":
            "Определение ОС...",
        "Generating execution plan...":
            "Формирование плана выполнения...",
        "Step %d/%d: %s":
            "Шаг %d/%d: %s",
        "Command failed, attempting error recovery...":
            "Команда не выполнена, попытка восстановления...",
        "Recovery: %s → %s":
            "Восстановление: %s → %s",
        "Verifying task completion...":
            "Проверка завершения задачи...",
        "Computer task completed: %d steps executed.\n%s":
            "Задача выполнена: %d шагов завершено.\n%s",
        "computer mode is disabled; use --computer flag, set GOGITOR_COMPUTER_ENABLED=true, or \"computer_enabled\": true in .gogitor.json":
            "режим управления компьютером отключён; используйте флаг --computer, установите GOGITOR_COMPUTER_ENABLED=true или \"computer_enabled\": true в .gogitor.json",
        "usage: :computer <task>":
            "использование: :computer <задача>",
        "step %d FORBIDDEN: %s (%s)":
            "шаг %d ЗАПРЕЩЁН: %s (%s)",
        "step %d failed: %v":
            "шаг %d не выполнен: %v",
        "step %d failed with exit code %d":
            "шаг %d не выполнен, код выхода %d",
        "Generating commit message for: %s":       "Генерация сообщения коммита для: %s",
        "Committed %s (%s)":                       "Закоммичено %s (%s)",
        "Generating general commit message for %d remaining file(s)...": "Генерация общего сообщения для %d оставшихся файлов...",
        "Committed remaining %d file(s) (%s)":     "Закоммичено %d оставшихся файлов (%s)",
        "file '%s' has no changes, skipping":      "файл '%s' не имеет изменений, пропуск",
        "none of the specified files have changes": "ни один из указанных файлов не имеет изменений",
        "Specify files to split. Changed files:":  "Укажите файлы для разделения. Изменённые файлы:",
        "Reverting commit %s...":              "Отмена коммита %s...",
        "Reverted %s. A new revert commit was created.": "Коммит %s отменён. Создан новый revert-коммит.",
        "Revert conflict detected. Resolve conflicts manually, then run ':git commit'.": "Обнаружен конфликт при отмене. Разрешите конфликты вручную, затем выполните ':git commit'.",
        "Resetting to %s (%s). This rewrites branch history.": "Откат к %s (%s). История ветки будет переписана.",
        "Reset to %s (%s).":                  "Выполнен откат к %s (%s).",
        "WARNING: '--hard' will DISCARD all uncommitted changes:": "ВНИМАНИЕ: '--hard' УДАЛИТ все незакоммиченные изменения:",
        "usage: :git reset [--hard] <commit-hash>": "использование: :git reset [--hard] <хеш-коммита>",
        "Running git command":     "Выполнение git-команды",
        "Running go program":      "Запуск Go-программы",
        "current stage: article":  "текущий этап: статья",
        "current stage: suggest":  "текущий этап: анализ проекта",
        "Scanning TODO markers":   "Сканирование TODO-маркеров",
        "Reading decision journal": "Чтение журнала решений",
		// === Дополнительные переводы для лог-сообщений ===
		"Parsing error trace":                                    "Разбор трассировки ошибки",
		"Preparing sandbox":                                      "Подготовка песочницы",
		"Running go build":                                       "Выполнение go build",
		"Running go test":                                        "Выполнение go test",
		"Running go vet":                                         "Выполнение go vet",
		"Running golangci-lint":                                  "Выполнение golangci-lint",
		"Running go run .":                                       "Запуск go run .",
		"Applying changes to project":                            "Применение изменений к проекту",
		"Reading project files":                                  "Чтение файлов проекта",
		"Sending chat request to LLM":                            "Отправка запроса чата в LLM",
		"Sending analysis request to LLM":                        "Отправка запроса анализа в LLM",
		"Generating search query":                                "Формирование поискового запроса",
		"Generating answer from search results":                  "Формирование ответа по результатам поиска",
		"Generating commit message from diff...":                 "Генерация сообщения коммита по diff...",
		"Generating comparative analysis of approaches...":       "Генерация сравнительного анализа подходов...",
		"LLM request":                                            "Запрос к LLM",
		"Orchestrator disabled (simple task)":                    "Оркестратор отключён (простая задача)",
		"Orchestrator enabled":                                   "Оркестратор включён",
		"Complex task detected: multi-agent mode enabled":        "Обнаружена сложная задача: включён мультиагентный режим",
		"Analysis-only task detected: no file changes will be made": "Обнаружена задача только на анализ: файлы не будут изменены",
		"Using patch mode for existing project files":            "Используется режим патчей для существующих файлов проекта",
		"Using modification mode based on existing project files": "Используется режим изменения на основе существующих файлов проекта",
		"Using create-in-existing-project mode":                  "Используется режим создания в существущем проекте",
		"Using create mode for empty project":                    "Используется режим создания для пустого проекта",
		"Falling back to full-file mode":                         "Откат к режиму полной перезаписи файлов",
		"Patch mode failed: ":                                    "Режим патчей не сработал: ",
		"Full agent mode: planner + coder + reviewer + verifier": "Полный агентный режим: планировщик + кодер + ревьюер + верификатор",
		"Planner started":                                        "Панировщик запущен",
		"Coder started":                                          "Кодер запущен",
		"Reviewer started":                                       "Ревьюер запущен",
		"Verifier started":                                       "Верификатор запущен",
		"Reviewer agent: analyzing changes":                      "Агент-ревьюер: анализ изменений",
		"Reviewer agent finished":                                "Агент-ревьюер завершил работу",
		"Pushing to remote...":                                   "Отправка на удалённый репозиторий...",
		"Pulling from remote...":                                 "Получение с удалённого репозитория...",
		"Fetching from remote...":                                "Загрузка с удалённого репозитория...",
		"Cloning %s ...":                                         "Клонирование %s ...",
		"Creating repository %q on GitHub...":                    "Создание репозитория %q на GitHub...",
		"Reading commit history...":                              "Чтение истории коммитов...",
		"Running tests to collect failures...":                   "Запуск тестов для сбора ошибок...",
		"Creating issue on GitHub...":                            "Создание issue на GitHub...",
		"Generating PR description from diff...":                 "Генерация описания PR по diff...",
		"Creating PR: %s → %s ...":                               "Создание PR: %s → %s ...",
		"Adding comment to PR #%d...":                            "Добавление комментария к PR #%d...",
		"Merging '%s' into '%s'...":                              "Слияние '%s' в '%s'...",
		"Checking out commit %s (detached HEAD). Use ':git checkout <branch>' to return.": "Переключение на коммит %s (detached HEAD). Используйте ':git checkout <ветка>' для возврата.",
		"Specify a commit hash. Recent commits:":                 "Укажите хеш коммита. Последние коммиты:",
		"No previous task diff available. Run a code task first.": "Нет diff предыдущей задачи. Сначала выполните задачу.",
		"Decision journal is empty. Decisions are recorded automatically during multi-agent tasks.": "Журнал решений пуст. Решения записываются автоматически при выполнении мультиагентных задач.",
		"Analyzing decision debt with LLM...":                    "Анализ долга решений через LLM...",
		"Reading project files for health review":                "Чтение файлов проекта для анализа состояния",
		"Sending suggest request to LLM":                         "Отправка запроса предложений в LLM",
		"Auto-search: looking up reference material for subtask...": "Автопоиск: поиск справочных материалов для подзадачи...",
		"Web search context added to subtask":                    "Контекст веб-поиска добавлен к подзадаче",
		"Interpreting approach selection via LLM...":             "Интерпретация выбора подхода через LLM...",
		"Selection reason: %s":                                   "Причина выбора: %s",
		"Commit message: %s":                                     "Сообщение коммита: %s",
		"All tests passed — nothing to report as an issue.":      "Все тесты пройдены — нечего сообщать как issue.",
		"Pull successful.":                                       "Получение выполнено.",
		"Push successful.":                                       "Отправка выполнена.",
		"Fetch complete.":                                        "Загрузка завершена.",
		"Already up to date.":                                    "Уже актуально.",
    	"Everything up-to-date":                                  "Всё актуально.",
    	"Everything up-to-date.":                                 "Всё актуально.",
    	"No commits found.":                                      "Коммиты не найдены.",
		"Working tree clean.":                                    "Рабочая директория чиста.",
		"Not a git repository.":                                  "Не является git-репозиторием.",
		"No diff.":                                               "Нет различий.",
		"No changes.":                                            "Нет изменений.",
		"Nothing to commit.":                                     "Нечего коммитить.",
		"Git repository initialized.":                            "Git-репозиторий инициализирован.",
		"No commits yet.":                                        "Коммитов пока нет.",
		"No branches found.":                                     "Ветки не найдены.",
		"No remotes configured.":                                 "Удалённые репозитории не настроены.",
		"go vet: no issues found.":                               "go vet: проблем не найдено.",
		"go vet found issues":                                    "go vet обнаружил проблемы",
		"golangci-lint: no issues found.":                        "golangci-lint: проблем не найдено.",
		"No TODO/FIXME/HACK markers found. Project is clean.":    "Маркеры TODO/FIXME/HACK не найдены. Проект чист.",
		"Conversation context cleared.":                          "Контекст разговора очищен.",
		"Done.":                                                  "Готово.",
		"unknown error":                                          "неизвестная ошибка",
		"empty command":                                          "пустая команда",
		"empty query":                                            "пустой запрос",

        "usage: :article <topic>":                    "использование: :article <тема>",
        "Generating simple article":                  "Генерация простой статьи",
        "Generating article plan":                    "Генерация плана статьи",
        "Plan ready: %d sections":                    "План готов: %d секций",
        "Writing section %d/%d: %s":                  "Написание секции %d/%d: %s",
        "Section %d failed: %v":                      "Секция %d не удалась: %v",
        "JSON plan failed, trying text fallback":     "JSON-план не удался, пробуем текстовый формат",
        "current stage: article (simple)":            "текущий этап: статья (простая)",
        "current stage: article (complex)":           "текущий этап: статья (сложная)",
        "Plan generation failed, falling back to simple mode: %v": "Генерация плана не удалась, переключение на простой режим: %v",
        "Article mode: genre=%s, web_search=%v, project_context=%v": "Режим статьи: жанр=%s, веб-поиск=%v, контекст проекта=%v",
        "Searching web for article context...":       "Поиск в интернете для контекста статьи...",
        "Web search: found %d source(s)":             "Веб-поиск: найдено источников: %d",
        "Web search failed (non-fatal): %v":          "Веб-поиск не удался (некритично): %v",
        "article generation failed":                  "генерация статьи завершилась с ошибкой",
        "Article saved to: %s":                       "Статья сохранена в: %s",
		"Found %s:":          "Найдено %s:",
		"... and %d more":    "... и ещё %d",
		"Type ':todo' to see all, or ask to fix any of them.": "Введите ':todo', чтобы посмотреть все, или попросите исправить любую из них.",
		"Loading...":                    "Загрузка...",
		"Approach":      "Подход",
		"Complexity":    "Сложность",
		"Performance":   "Производительность",
		"Readability":   "Читаемость",
		"Dependencies":  "Зависимости",
		"Testability":   "Тестируемость",
		"Variant %d: %s": "Вариант %d: %s",
		"type a number (1-%d), or describe your own approach, or type \"yes\" to accept the recommendation.": "введите номер (1-%d), или опишите свой подход, или введите \"да\" для принятия рекомендации.",
		"usage: :code <task>":                                "использование: :code <задача>",
		"usage: :ask <question>":                             "использование: :ask <вопрос>",
		"usage: :analyze <question>":                         "использование: :analyze <вопрос>",
		"usage: :search <query>":                             "использование: :search <запрос>",
		"usage: :git merge <branch-name>":                    "использование: :git merge <имя-ветки>",
		"usage: :git branch -d <branch-name>":                "использование: :git branch -d <имя-ветки>",
		"usage: :git checkout -b <new-branch>":               "использование: :git checkout -b <новая-ветка>",
		"usage: :git checkout <commit-hash>":                 "использование: :git checkout <хеш-коммита>",
		"usage: :git remote add <name> <url>":                "использование: :git remote add <имя> <url>",
		"usage: :git remote remove <name>":                   "использование: :git remote remove <имя>",
		"usage: :git remote set-url <name> <url>":            "использование: :git remote set-url <имя> <url>",
		"usage: :git remote [add <name> <url> | remove <name> | set-url <name> <url>]": "использование: :git remote [add <имя> <url> | remove <имя> | set-url <имя> <url>]",
		"usage: :git create <repo-name> [--private] [--desc <description>]":            "использование: :git create <имя-репо> [--private] [--desc <описание>]",
		"usage: :git clone <url> or set --github <url>":      "использование: :git clone <url> или задайте --github <url>",
		"usage: :git pr-comment <PR-number> [text]":          "использование: :git pr-comment <номер-PR> [текст]",
		"GitHub token is required. Use --key-github <token>":                       "Требуется токен GitHub. Используйте --key-github <токен>",
		"cannot determine current branch":                                          "не удалось определить текущую ветку",
		"refusing to create PR from main/master; switch to a feature branch":       "создание PR из main/master запрещено; переключитесь на отдельную ветку",
		"remote 'origin' not found; use ':git remote add origin <url>'":            "remote 'origin' не найден; используйте ':git remote add origin <url>'",
		"remote 'origin' not found":                                                "remote 'origin' не найден",
		"PR number must be a positive integer":                                     "номер PR должен быть положительным целым числом",
		"no remote configured. Use ':git remote add <url>' or --github <url>":      "remote не настроен. Используйте ':git remote add <url>' или --github <url>",
		"changes were rolled back to pre-agent state":                          "изменения были откатаны к состоянию до запуска агента",
		"agent finished unsuccessfully; changes were not automatically rolled back": "агент завершился с ошибкой; изменения не были автоматически откатаны",
		"LLM did not return file blocks. Expected format: --- File: path ---":                                  "LLM не вернула файловые блоки. Ожидаемый формат: --- File: путь ---",
		"LLM did not return valid patch/file blocks. Expected format: --- Patch: path --- or --- File: path ---": "LLM не вернула корректные patch/файловые блоки. Ожидаемый формат: --- Patch: путь --- или --- File: путь ---",
		"task file failed":   "ошибка выполнения файла задачи",
		"code task failed":   "ошибка выполнения задачи генерации кода",
		"tests failed":       "тесты завершились с ошибкой",
		"git command failed": "команда git завершилась с ошибкой",
		"fix failed":         "исправление завершилось с ошибкой",
		"run failed":         "запуск завершился с ошибкой",
		"decisions failed":   "не удалось получить журнал решений",
		"ask failed":         "запрос завершился с ошибкой",
		"analyze failed":     "анализ завершился с ошибкой",
		"search failed":      "поиск завершился с ошибкой",
		"suggest failed":     "анализ проекта завершился с ошибкой",
		"vet failed":         "go vet завершился с ошибкой",
		"todo failed":        "поиск TODO завершился с ошибкой",
		"💡 Found %s in project. Type :todo to see details.": "💡 Найдено %s в проекте. Введите :todo для подробностей.",
        "No project files found in trace; using general project context": "Файлы проекта не найдены в трассировке; используется общий контекст проекта",
        "Mode: fix (auto-detected error trace)": "Режим: fix (автоопределение трассировки ошибки)",
        "usage: :fix <error output / stack trace>": "использование: :fix <вывод ошибки / stack trace>",
        "Approach selection LLM failed, falling back: %v": "LLM для выбора подхода завершилась с ошибкой, используем запасной сценарий: %v",
        "Cannot parse approach selection response: %v": "Не удалось разобрать ответ выбора подхода: %v",
        "LLM returned invalid approach_id=%d (valid: 1-%d)": "LLM вернула недопустимый approach_id=%d (допустимо: 1-%d)",
        "Ctrl+A - copy all output to clipboard.": "Ctrl+A — копировать весь вывод в буфер обмена.",
        "📋 Copied to clipboard (%d bytes).": "Скопировано в буфер обмена (%d байт).",
        "Nothing to copy: output is empty.": "Нечего копировать: вывод пуст.",
        "Clipboard copy failed.": "Не удалось скопировать в буфер обмена.",
        "Auto-search: found %d source(s)": "Автопоиск: найдено источников: %d",
        "Auto-search failed (non-fatal): %v": "Автопоиск не удался (некритично): %v",
		"LLM analysis failed: %v (showing raw journal)": "Не удалось проанализировать через LLM: %v (показан сырой журнал)",
        "working": "работа",
        "running": "выполняется",
        "Tab: output | Ctrl+C: cancel | F2: select": "Tab: вывод | Ctrl+C: отмена | F2: выделение",
        "Tab: output | Ctrl+C: cancel | F2: select text": "Tab: вывод | Ctrl+C: отмена | F2: выделение текста",
        "%s | %d req | ≈%d tok | %s | Tab: output | Ctrl+C: cancel | F2: select": "%s | %d зап | ≈%d ток | %s | Tab: вывод | Ctrl+C: отмена | F2: выделение",
        "%s | Tab: output | Ctrl+C: cancel | F2: select text": "%s | Tab: вывод | Ctrl+C: отмена | F2: выделение текста",
        "Running... | Tab: output | Ctrl+C: cancel | F2: select text": "Выполняется... | Tab: вывод | Ctrl+C: отмена | F2: выделение текста",
        "LLM usage: %d requests, %d tokens, %s": "Использование LLM: %d запросов, %d токенов, %s",
        "current stage: approach comparison":       "текущий этап: сравнение подходов",
        "Selected approach: %s":                    "Выбранный подход: %s",
        "Comparison generation failed, proceeding normally: %v": "Не удалось сгенерировать сравнение, продолжаем обычно: %v",
        "Select a variant:":                        "Выберите вариант:",
        "RECOMMENDED":                              "РЕКОМЕНДУЕТСЯ",
        "Comparative Analysis of Approaches":       "Сравнительный анализ подходов",
        "Recommendation":                           "Рекомендация",
        "Trade-offs:":                              "Компромиссы:",
        "Justification:":                           "Обоснование:",
        "Details":                                  "Детали",
        "Task changes (cumulative diff):": "Изменения задачи (накопительный diff):",
        "LLM retry: agent %s%s — %v": "LLM повтор: агент %s%s — %v",
        "golangci-lint found %d issue(s), sending to LLM for fixing": "golangci-lint нашёл %d проблем(ы), отправка в LLM для исправления",
        "Lint: passed (0 issues)":                            "Линтер: пройдено (0 проблем)",
        "Lint: %d issue(s) found":                            "Линтер: найдено %d проблем(ы)",
        "golangci-lint is not installed; install it with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest": "golangci-lint не установлен; установите: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest",
		"Mode: %s":                             "Режим: %s",
		"Refined task: %s":                     "Уточнённая задача: %s",
		"Intent reason: %s":                    "Причина выбора режима: %s",
		"Intent detection failed: %v":          "Не удалось определить намерение: %v",
		"Cannot parse intent response: %v":       "Не удалось разобрать ответ намерения: %v",
		"current stage: %s":                    "текущий этап: %s",
		"Searching web: %s":                    "Поиск в интернете: %s",
		"Sources:":                             "Источники:",

		"Dry-run mode enabled: changes will be validated but not applied": "Включён режим dry-run: изменения будут проверены, но не применены",
		"orchestrator disabled (simple task)":                             "оркестратор отключён (простая задача)",
		"orchestrator enabled":                                            "оркестратор включён",
		"current stage: coder":                                            "текущий этап: кодер",
		"coder started":                                                   "кодер запущен",
		"Agent mode: creating plan":                                       "Агентный режим: создание плана",
		"Subtask %d/%d: %s":                                               "Подзадача %d/%d: %s",
		"Iteration %d/%d":                                                 "Итерация %d/%d",

		"Dry-run DIFF patch: %s":          "Dry-run DIFF-патч: %s",
		"Dry-run full file rewrite: %s":   "Dry-run полная перезапись файла: %s",
		"Dry-run create new file: %s":     "Dry-run создание нового файла: %s",
		"Applied DIFF patch: %s":          "Применён DIFF-патч: %s",
		"Applied full file rewrite: %s":   "Применена полная перезапись файла: %s",
		"Created new file: %s":            "Создан новый файл: %s",
		"Applied changes: %s.":            "Применены изменения: %s.",
		"Applied %d patch/file change(s).": "Применено изменений: %d.",
		"Applied %d file(s).":              "Применено файлов: %d.",
		"Dry-run validated: %s.":           "Dry-run проверено: %s.",
		"Dry-run: patch changes were validated in sandbox but not applied.": "Dry-run: patch-изменения проверены в песочнице, но не применены.",
		"Dry-run: changes were validated in sandbox but not applied.":       "Dry-run: изменения проверены в песочнице, но не применены.",

		"Git commit created: %s": "Создан git-коммит: %s",

		"Committed %s":                 "Закоммичено %s",
		"Current branch: %s":           "Текущая ветка: %s",
		"Deleted branch %s.":           "Удалена ветка %s.",
		"Created branch '%s'.":         "Создана ветка '%s'.",
		"Use ':git checkout %s' to switch to it.": "Используйте ':git checkout %s' для переключения.",
		"Merged '%s' into '%s'.":                  "Выполнено слияние '%s' в '%s'.",

		"Remote '%s' added: %s":         "Удалённый репозиторий '%s' добавлен: %s",
		"Remote '%s' removed.":          "Удалённый репозиторий '%s' удалён.",
		"Remote '%s' URL set to %s":     "URL удалённого репозитория '%s' установлен на %s",
		"Using GitHub token (%s)":       "Используется токен GitHub (%s)",
		"No GitHub token configured. Private repos will fail.": "Токен GitHub не настроен. Приватные репозитории будут недоступны.",
		"Switched working directory to %s":                     "Рабочая директория переключена на %s",
		"Cloned into %s":                                       "Склонировано в %s",
		"Repository created: %s (%s)":                          "Репозиторий создан: %s (%s)",
		"Remote 'origin' set to %s":                            "Удалённый репозиторий 'origin' установлен на %s",
		"Clone URL: %s":                                        "URL для клонирования: %s",

		"current stage: planning":                                "текущий этап: планирование",
		"planner started":                                        "планировщик запущен",
		"planner completed: goal=%s":                             "планировщик завершён: цель=%s",
		"Plan contains %d subtasks":                              "План содержит %d подзадач(и)",
		"Agent subtask %d/%d: %s":                                "Агентная подзадача %d/%d: %s",
		"current subtask %d/%d: %s":                              "текущая подзадача %d/%d: %s",
		"current stage: reviewer":                                "текущий этап: ревьюер",
		"reviewer started":                                       "ревьюер запущен",
		"current stage: reviewer (skipped, no changes)":          "текущий этап: ревьюер (пропущено, нет изменений)",
		"Reviewer found critical issues: %s":                     "Ревьюер нашёл критические проблемы: %s",
		"Reviewer suggestions: %s":                               "Предложения ревьюера: %s",
		"current stage: coder (correction of reviewer comments)": "текущий этап: кодер (исправление замечаний ревьюера)",
		"current stage: verifier":                                "текущий этап: верификатор",
		"verifier started":                                       "верификатор запущен",
		"Verifier: task not fully completed: %s":                 "Верификатор: задача выполнена не полностью: %s",
		"current stage: coder (fix verifier)":                    "текущий этап: кодер (исправление после верификатора)",
		"Rollback: %s":                                           "Откат: %s",
		"checkpoint failed: %v":                                  "не удалось создать контрольную точку: %v",
		"reviewer failed: %v":                                    "ошибка ревьюера: %v",
		"verifier failed: %v":                                    "ошибка верификатора: %v",
		"structured plan failed, fallback to legacy plan: %v":    "не удалось построить структурированный план, используется упрощённый: %v",

		"LLM dispatcher usage: requests=%d estimated_tokens=%d duration=%s queue=%d": "Использование LLM-диспетчера: запросов=%d, примерных токенов=%d, время=%s, очередь=%d",
		"budget after stage %s: requests=%d, tokens≈%d, duration=%s, queue=%d":       "бюджет после этапа %s: запросов=%d, токенов≈%d, время=%s, очередь=%d",
		"requests=%d, tokens≈%d, duration=%s":                                        "запросов=%d, токенов≈%d, время=%s",

		"LLM queue: agent %s%s waiting; queue=%d; budget: %s":        "LLM очередь: агент %s%s ожидает запрос; очередь=%d; бюджет: %s",
		"LLM request: agent %s%s is using LLM; queue=%d; budget: %s": "LLM запрос: агент %s%s сейчас использует LLM; очередь=%d; бюджет: %s",
		"LLM request: agent %s%s finished with error; budget: %s":    "LLM запрос: агент %s%s завершил запрос с ошибкой; бюджет: %s",
		"LLM request: agent %s%s finished; budget: %s":               "LLM запрос: агент %s%s завершил запрос; бюджет: %s",
		"LLM status: agent %s%s; queue=%d; budget: %s":               "LLM статус: агент %s%s; очередь=%d; бюджет: %s",

		"Resolving external Go dependencies (go mod tidy)...": "Разрешение внешних Go-зависимостей (go mod tidy)...",
		"⚠ go mod tidy failed: %s":                            "⚠ go mod tidy завершился с ошибкой: %s",
		"✓ Dependencies resolved:\n%s":                        "✓ Зависимости разрешены:\n%s",
		"✓ Dependencies resolved.":                            "✓ Зависимости разрешены.",

		"Failed tests:":                   "Упавшие тесты:",
		"- %s -> function %s (%s:%d)":     "- %s -> функция %s (%s:%d)",
		"Raw output:":                     "Исходный вывод:",
		"No Go test files found.":         "Go-тесты не найдены.",
		"Tests failed: %d passed, %d failed%s": "Тесты упали: пройдено %d, упало %d%s",
		"Tests passed: %d%s":                   "Тесты пройдены: %d%s",
		"Tests: passed=%d failed=%d":           "Тесты: пройдено=%d, упало=%d",
		"Tests: passed=%d failed=%d%s":         "Тесты: пройдено=%d, упало=%d%s",
		"Tests: passed=%d failed=%d (%s)":      "Тесты: пройдено=%d, упало=%d (%s)",
		"Tests: skipped":                       "Тесты: пропущено",
		" (coverage: %.1f%%)":                  " (покрытие: %.1f%%)",

		"Created files:":          "Созданные файлы:",
		"Modified files:":         "Изменённые файлы:",
		"Patched files (DIFF):":   "Пропатченные файлы (DIFF):",
		"Full rewritten files:":   "Полностью переписанные файлы:",
		"Warnings:":               "Предупреждения:",
		"Errors:":                 "Ошибки:",
		"Git commit:":             "Git-коммит:",
		"SUCCESS":                 "УСПЕХ",
		"FAILED":                  "ОШИБКА",
		"Refined task:":           "Уточнённая задача:",

        "Execution plan (goal: %s)": "План выполнения (цель: %s)",
        "Acceptance criteria: %s":   "Критерии приёмки: %s",
        "Plan completed: %d/%d":     "План выполнен: %d/%d",

		"Gogitor TUI ready.": "Gogitor TUI готов.",
		"Type :help for commands.": "Введите :help для списка команд.",
		"Alt+Enter adds a line. Up/Down move between lines. Tab switches to output.": "Alt+Enter — новая строка. Up/Down — перемещение между строками. Tab — переключение на вывод.",
		"F2 - mode for selecting text with the mouse for copying.":                   "F2 — режим выделения текста мышью для копирования.",
		"PgUp/PgDn - browse command history.":                                        "PgUp/PgDn — просмотр истории команд.",
		"Task: enter a task or :help":                                                "Task: введите задачу или :help",

		"Running... Ctrl+C cancel | Tab: output/input | PgUp/PgDn scroll | F2: select text": "Выполняется... Ctrl+C отмена | Tab: вывод/ввод | PgUp/PgDn прокрутка | F2: выделение текста",
		"Output focus: arrows/PgUp/PgDn/mouse scroll | Tab or Esc: back to input | F2: select text | Ctrl+C quit": "Фокус на выводе: стрелки/PgUp/PgDn/мышь | Tab или Esc: назад ко вводу | F2: выделение текста | Ctrl+C выход",
		"provider=%s model=%s | Enter send | Alt+Enter newline | PgUp/PgDn history | F2: select text | Ctrl+C quit": "провайдер=%s модель=%s | Enter отправить | Alt+Enter новая строка | PgUp/PgDn история | F2: выделение текста | Ctrl+C выход",
		"Text selection: select with mouse and copy via terminal | PgUp/PgDn scroll | F2 back": "Выделение текста: выделите мышкой и скопируйте через терминал | PgUp/PgDn прокрутка | F2 — обратно",
        // ─── Workflow: общие ────────────────────────────────────────────
        "Workflow dry-run is not fully supported; falling back to simple execution":
        	"Dry-run в режиме workflow поддерживается не полностью; переключение на простое выполнение",
        "Cannot create workflow dir %s: %v; falling back to .gogitor/workflow":
        	"Не удалось создать директорию workflow %s: %v; используется .gogitor/workflow",
        "Cannot create fallback workflow dir %s: %v; falling back to simple execution":
        	"Не удалось создать резервную директорию workflow %s: %v; переключение на простое выполнение",
        "Workflow artifacts dir: %s":
        	"Директория артефактов workflow: %s",
        "Cannot write inbox.md: %v":
        	"Не удалось записать inbox.md: %v",
        "Cannot write research.md: %v":
        	"Не удалось записать research.md: %v",
        "PRD validation failed: %v; rebuilding fallback PRD":
        	"Валидация PRD не удалась: %v; пересоздание резервного PRD",
        "Cannot write plan.md: %v":
        	"Не удалось записать plan.md: %v",
        "Cannot write prd.json: %v":
        	"Не удалось записать prd.json: %v",
        "Running workflow quality gates":
        	"Выполнение quality gates workflow",
        "Cannot write gate report: %v":
        	"Не удалось записать отчёт quality gates: %v",
        "previous workflow tasks may already have been committed":
        	"предыдущие задачи workflow могли уже быть закоммичены",
        "changes from the failed task remain applied but uncommitted":
        	"изменения из неудачной задачи применены, но не закоммичены",
        "workflow task %d commit failed: %v":
        	"коммит задачи workflow %d не удался: %v",
        "Workflow completed %d task(s).":
        	"Workflow завершил %d задач(у).",
        "Invalid workflow dir %s: %v; using .gogitor/workflow":
        	"Некорректная директория workflow %s: %v; используется .gogitor/workflow",
        
        // ─── Workflow: interview ────────────────────────────────────────
        "Generating clarifying questions...":
        	"Формирование уточняющих вопросов...",
        "Interview question generation failed: %v; proceeding without interview":
        	"Не удалось сформировать вопросы: %v; продолжаем без интервью",
        "No clarifying questions needed.":
        	"Уточняющие вопросы не требуются.",
        "Interview questions ready.":
        	"Вопросы интервью готовы.",
        "Processing interview answers...":
        	"Обработка ответов интервью...",
        "Task refined based on interview answers.":
        	"Задача уточнена на основе ответов интервью.",
        "Interview refinement failed: %v; using original task":
        	"Не удалось уточнить задачу: %v; используется исходная задача",
        
        // ─── Workflow: reflect ──────────────────────────────────────────
        "Analyzing workflow artifacts: %s":
        	"Анализ артефактов workflow: %s",
        "Generating workflow reflection...":
        	"Формирование ретроспективы workflow...",
        "LLM reflection failed: %v; showing raw artifacts":
        	"Не удалось сформировать ретроспективу: %v; показаны сырые артефакты",
        "Cannot save reflection.md: %v":
        	"Не удалось сохранить reflection.md: %v",
        "Reflection saved: %s":
        	"Ретроспектива сохранена: %s",
        
        // ─── Workflow: PR ───────────────────────────────────────────────
        "not a git repository; run :git init first":
        	"не git-репозиторий; выполните :git init",
        "no workflow artifacts found: %v":
        	"артефакты workflow не найдены: %v",
        "Workflow artifacts: %s":
        	"Артефакты workflow: %s",
        "workflow goal not found in inbox.md":
        	"цель workflow не найдена в inbox.md",
        "Creating branch '%s' from '%s'...":
        	"Создание ветки '%s' из '%s'...",
        "cannot create branch: %v":
        	"не удалось создать ветку: %v",
        "Using existing branch '%s'":
        	"Используется существующая ветка '%s'",
        "Using current branch '%s'":
        	"Используется текущая ветка '%s'",
        "Pushing branch '%s' to origin...":
        	"Отправка ветки '%s' в origin...",
        "push failed: %v":
        	"ошибка отправки: %v",
        "Pull Request created: #%d\n%s\nTitle: %s\nBranch: %s → %s":
        	"Pull Request создан: #%d\n%s\nЗаголовок: %s\nВетка: %s → %s",
        
        // ─── Workflow: usage ────────────────────────────────────────────
        "usage: :workflow <task> | :workflow interview <task> | :workflow reflect | :workflow pr":
        	"использование: :workflow <задача> | :workflow interview <задача> | :workflow reflect | :workflow pr",
        "usage: :workflow interview <task>":
        	"использование: :workflow interview <задача>",
        "Interview skipped by user, using original task with defaults.":
        "Интервью пропущено пользователем, используется исходная задача со значениями по умолчанию.",
        
        "gofmt gate skipped (small model profile)":
        "Проверка gofmt пропущена (профиль малой модели)",
        
        "golangci-lint gate skipped (small model profile)":
        "Проверка golangci-lint пропущена (профиль малой модели)",
        
        "Processing plan review feedback...":
        "Обработка обратной связи по плану...",
        
        "Refining plan based on user feedback...":
        "Корректировка плана на основе обратной связи пользователя...",
        
        "Plan refined based on feedback.":
        "План скорректирован на основе обратной связи.",
        
        "Extracting lessons from reflection...":
        "Извлечение уроков из ретроспективы...",
	},
}

var ruPatterns = []pattern{
    // ─── Computer Mode ─────────────────────────────────────────
    {regexp.MustCompile(`^OS: (\S+) (\S+) (\S+) \| pkg: (\S+) \| shell: (\S+) \| sudo: (\S+)$`),
        "ОС: $1 $2 $3 | пакетный менеджер: $4 | оболочка: $5 | sudo: $6"},
    {regexp.MustCompile(`^computer mode is disabled; use --computer flag, set GOGITOR_COMPUTER_ENABLED=true, or "computer_enabled": true in \.gogitor\.json$`),
        "режим управления компьютером отключён; используйте флаг --computer, установите GOGITOR_COMPUTER_ENABLED=true или \"computer_enabled\": true в .gogitor.json"},
    {regexp.MustCompile(`^plan generation failed: (.*)$`),
        "не удалось сформировать план: $1"},
    {regexp.MustCompile(`^plan contains no steps$`),
        "план не содержит шагов"},
    {regexp.MustCompile(`^FORBIDDEN: (.*) \((.*)\)$`),
        "ЗАПРЕЩЕНО: $1 ($2)"},
    {regexp.MustCompile(`^sudo not allowed: (.*)$`),
        "sudo не разрешён: $1"},
    {regexp.MustCompile(`^doas not allowed: (.*)$`),
        "doas не разрешён: $1"},
    {regexp.MustCompile(`^pkexec not allowed: (.*)$`),
        "pkexec не разрешён: $1"},
    {regexp.MustCompile(`^runuser not allowed: (.*)$`),
        "runuser не разрешён: $1"},
    {regexp.MustCompile(`^su -c not allowed: (.*)$`),
        "su -c не разрешён: $1"},
    {regexp.MustCompile(`^HIGH risk requires confirmation: (.*)$`),
        "Высокий риск требует подтверждения: $1"},
    {regexp.MustCompile(`^command rejected by user: (.*)$`),
        "Команда отклонена пользователем: $1"},
    {regexp.MustCompile(`^write to (.*) is not allowed \(outside permitted directories\)$`),
        "Запись в $1 не разрешена (вне допустимых директорий)"},
    {regexp.MustCompile(`^verifier: task may be incomplete: (.*)$`),
        "верификатор: задача может быть не завершена: $1"},
    {regexp.MustCompile(`^side effects: (.*)$`),
        "побочные эффекты: $1"},
    {regexp.MustCompile(`^risks: (.*)$`),
        "риски: $1"},
	// --- Сырой вывод git push / pull / fetch ---
	{regexp.MustCompile(`^To (.*)$`), "В $1"},
	{regexp.MustCompile(`^From (.*)$`), "Из $1"},
	{regexp.MustCompile(`^\s+([a-f0-9]+)\.\.([a-f0-9]+)\s+(\S+)\s+->\s+(\S+)$`), "   $1..$2 $3 -> $4"},
	{regexp.MustCompile(`^Updating ([a-f0-9]+)\.\.([a-f0-9]+)$`), "Обновление $1..$2"},
	{regexp.MustCompile(`^Fast-forward$`), "Fast-forward"},
	{regexp.MustCompile(`^\s+\*\s+branch\s+(\S+)\s+->\s+(\S+)$`), "   * ветка $1 -> $2"},
	
	// --- Сырой вывод git clone ---
	{regexp.MustCompile(`^Cloning into '(.*)'\.\.\.$`), "Клонирование в '$1'..."},
	{regexp.MustCompile(`^Receiving objects:\s+(\d+)% \((\d+)/(\d+)\)(, done\.)?$`), "Получение объектов: $1% ($2/$3)$4"},
	{regexp.MustCompile(`^Resolving deltas:\s+(\d+)% \((\d+)/(\d+)\)(, done\.)?$`), "Разрешение дельт: $1% ($2/$3)$4"},
	{regexp.MustCompile(`^remote: Enumerating objects: (\d+)(, done\.)?$`), "remote: Перечисление объектов: $1$2"},
	{regexp.MustCompile(`^remote: Counting objects:\s+(\d+)% \((\d+)/(\d+)\)(, done\.)?$`), "remote: Подсчёт объектов: $1% ($2/$3)$4"},
	{regexp.MustCompile(`^remote: Compressing objects:\s+(\d+)% \((\d+)/(\d+)\)(, done\.)?$`), "remote: Сжатие объектов: $1% ($2/$3)$4"},
	{regexp.MustCompile(`^remote: Total (\d+) \(delta (\d+)\), reused (\d+) \(delta (\d+)\), pack-reused (\d+)$`), "remote: Всего $1 (изменений $2), переиспользовано $3 (изменений $4), pack-reused $5"},
	
	// --- Ответы GitHub API и прочие системные фразы ---
	{regexp.MustCompile(`^Pull Request created: #(\d+)$`), "Pull Request создан: #$1"},
	{regexp.MustCompile(`^Issue created: #(\d+)$`), "Issue создан: #$1"},
	{regexp.MustCompile(`^Title: (.*)$`), "Заголовок: $1"},
	{regexp.MustCompile(`^CHANGELOG\.md generated from (\d+) commits \((\d+) categorized\)\.$`), "CHANGELOG.md сгенерирован из $1 коммитов ($2 категоризировано)."},
	{regexp.MustCompile(`^Comment added to PR #(\d+)\.$`), "Комментарий добавлен к PR #$1."},
	{regexp.MustCompile(`^Usage: :git checkout <hash>$`), "Использование: :git checkout <хеш>"},
	{regexp.MustCompile(`^Full agent completed (\d+) subtasks\.$`), "Полный агент завершил $1 подзадач."},
	{regexp.MustCompile(`^Commit message: (.*)$`), "Сообщение коммита: $1"},

	{regexp.MustCompile(`^build failed:$`), "ошибка сборки:"},
	{regexp.MustCompile(`^no main package found: (.*)$`), "main-пакет не найден: $1"},
	{regexp.MustCompile(`^Using selected approach: (.*)$`), "Используется выбранный подход: $1"},
	{regexp.MustCompile(`^Patch mode failed: (.*)$`), "Режим патчей не сработал: $1"},
	{regexp.MustCompile(`^patch target files do not exist: (.*)$`), "целевые файлы патча не существуют: $1"},
	{regexp.MustCompile(`^git commit failed: (.*)$`), "git commit завершился с ошибкой: $1"},
	{regexp.MustCompile(`^cannot set remote origin: (.*)$`), "не удалось настроить remote origin: $1"},
	{regexp.MustCompile(`^GitHub token validation failed: (.*)$`), "проверка токена GitHub не удалась: $1"},
	{regexp.MustCompile(`^cannot parse repo URL: (.*)$`), "не удалось разобрать URL репозитория: $1"},
	{regexp.MustCompile(`^cannot parse remote URL: (.*)$`), "не удалось разобрать URL remote: $1"},
	{regexp.MustCompile(`^cannot get repo info: (.*)$`), "не удалось получить информацию о репозитории: $1"},
	{regexp.MustCompile(`^directory (.*) already exists$`), "директория $1 уже существует"},
	{regexp.MustCompile(`^Created and switched to branch '(.*)'\.$`), "Создана ветка '$1' и выполнено переключение на неё."},
	{regexp.MustCompile(`^Checked out (.*)\.$`), "Выполнен checkout $1."},
	{regexp.MustCompile(`^Specify a commit hash\. Recent commits:$`), "Укажите хеш коммита. Последние коммиты:"},
	{regexp.MustCompile(`^golangci-lint issues fixed \((\d+) issue\(s\) were found\)\. (.*)$`), "Проблемы golangci-lint исправлены (было найдено $1). $2"},
	{regexp.MustCompile(`^golangci-lint found (\d+) issue\(s\), but automatic fix failed\.$`), "golangci-lint нашёл $1 проблем(ы), но автоматическое исправление не удалось."},
	{regexp.MustCompile(`^unknown command: (.*)$`), "неизвестная команда: $1"},
	{regexp.MustCompile(`^unknown git subcommand: (.*)$`), "неизвестная подкоманда git: $1"},
	{regexp.MustCompile(`^unknown flag: (.*)$`), "неизвестный флаг: $1"},
	{regexp.MustCompile(`^flag --(.*) requires a value$`), "флаг --$1 требует значения"},
	{regexp.MustCompile(`^cannot read task file: (.*)$`), "не удалось прочитать файл задачи: $1"},
	{regexp.MustCompile(`^task file is empty: (.*)$`), "файл задачи пуст: $1"},
	{regexp.MustCompile(`^task file path is a directory: (.*)$`), "путь к файлу задачи является директорией: $1"},
	{regexp.MustCompile(`^task file must have \.txt or \.md extension: (.*)$`), "файл задачи должен иметь расширение .txt или .md: $1"},
	{regexp.MustCompile(`^warning: config load: (.*)$`), "предупреждение: загрузка конфига: $1"},
	{regexp.MustCompile(`^warning: logger init: (.*)$`), "предупреждение: инициализация логгера: $1"},
	{regexp.MustCompile(`^warning: config validation: (.*)$`), "предупреждение: валидация конфига: $1"},
	{regexp.MustCompile(`^TUI error: (.*)$`), "ошибка TUI: $1"},
	{regexp.MustCompile(`^error: cannot get current directory: (.*)$`), "ошибка: не удалось получить текущую директорию: $1"},
	{regexp.MustCompile(`^All tests passed\.$`), "Все тесты пройдены."},
	{regexp.MustCompile(`^TEST FAILURES \(fix ONLY listed issues\):$`), "ПАДЕНИЯ ТЕСТОВ (исправьте ТОЛЬКО перечисленные):"},
	{regexp.MustCompile(`^- test: (.*)$`), "- тест: $1"},
	{regexp.MustCompile(`^function: (.*)$`), "функция: $1"},
	{regexp.MustCompile(`^location: (.*)$`), "расположение: $1"},
	{regexp.MustCompile(`^Raw output \(truncated\):$`), "Исходный вывод (обрезан):"},
    {regexp.MustCompile(`^Error type: (\S+) \| frames: (\d+)$`), "Тип ошибки: $1 | кадров: $2"},
    {regexp.MustCompile(`^Identified source files: (.*)$`), "Определены файлы: $1"},
    {regexp.MustCompile(`^Auto-detected stack trace, switching to fix mode$`), "Обнаружена трассировка ошибки, переключение в режим исправления"},
	{regexp.MustCompile(`^Analyzing decision debt with LLM\.\.\.$`), "Анализ долга решений через LLM..."},
    {regexp.MustCompile(`^Selected approach: (.*)$`), "Выбранный подход: $1"},
    {regexp.MustCompile(`^current stage: approach comparison$`), "текущий этап: сравнение подходов"},
    {regexp.MustCompile(`^LLM retry: agent (\S+)(.*) — (.*)$`), "LLM повтор: агент $1$2 — $3"},
    {regexp.MustCompile(`^golangci-lint found (\d+) issue\(s\), sending to LLM for fixing$`), "golangci-lint нашёл $1 проблем(ы), отправка в LLM для исправления"},
    {regexp.MustCompile(`^Execution plan \(goal: (.*)\)$`), "План выполнения (цель: $1)"},
    {regexp.MustCompile(`^Plan completed: (\d+)/(\d+)$`), "План выполнен: $1/$2"},
	{regexp.MustCompile(`^Mode: (.*)$`), "Режим: $1"},
	{regexp.MustCompile(`^Refined task: (.*)$`), "Уточнённая задача: $1"},
	{regexp.MustCompile(`^Intent reason: (.*)$`), "Причина выбора режима: $1"},
	{regexp.MustCompile(`^Intent detection failed: (.*)$`), "Не удалось определить намерение: $1"},
	{regexp.MustCompile(`^Cannot parse intent response: (.*)$`), "Не удалось разобрать ответ намерения: $1"},
	{regexp.MustCompile(`^current stage: (.*)$`), "текущий этап: $1"},
	{regexp.MustCompile(`^Searching web: (.*)$`), "Поиск в интернете: $1"},

	{regexp.MustCompile(`^Subtask (\d+)/(\d+): (.*)$`), "Подзадача $1/$2: $3"},
	{regexp.MustCompile(`^Iteration (\d+)/(\d+)$`), "Итерация $1/$2"},

	{regexp.MustCompile(`^Dry-run DIFF patch: (.*)$`), "Dry-run DIFF-патч: $1"},
	{regexp.MustCompile(`^Dry-run full file rewrite: (.*)$`), "Dry-run полная перезапись файла: $1"},
	{regexp.MustCompile(`^Dry-run create new file: (.*)$`), "Dry-run создание нового файла: $1"},
	{regexp.MustCompile(`^Applied DIFF patch: (.*)$`), "Применён DIFF-патч: $1"},
	{regexp.MustCompile(`^Applied full file rewrite: (.*)$`), "Применена полная перезапись файла: $1"},
	{regexp.MustCompile(`^Created new file: (.*)$`), "Создан новый файл: $1"},
	{regexp.MustCompile(`^Applied changes: (.*)\.$`), "Применены изменения: $1."},
	{regexp.MustCompile(`^Applied (\d+) patch/file change\(s\)\.$`), "Применено изменений: $1."},
	{regexp.MustCompile(`^Applied (\d+) file\(s\)\.$`), "Применено файлов: $1."},
	{regexp.MustCompile(`^Dry-run validated: (.*)\.$`), "Dry-run проверено: $1."},

	{regexp.MustCompile(`^Git commit created: (.*)$`), "Создан git-коммит: $1"},
	{regexp.MustCompile(`^Committed (.*)$`), "Закоммичено $1"},

	{regexp.MustCompile(`^Current branch: (.*)$`), "Текущая ветка: $1"},
	{regexp.MustCompile(`^Deleted branch (.*)\.$`), "Удалена ветка $1."},
	{regexp.MustCompile(`^Created branch '(.*)'\.$`), "Создана ветка '$1'."},
	{regexp.MustCompile(`^Use ':git checkout (.*)' to switch to it\.$`), "Используйте ':git checkout $1' для переключения."},
	{regexp.MustCompile(`^Merging '(.*)' into '(.*)'\.\.\.$`), "Слияние '$1' в '$2'..."},
	{regexp.MustCompile(`^Merged '(.*)' into '(.*)'\.$`), "Выполнено слияние '$1' в '$2'."},

	{regexp.MustCompile(`^Remote '(.*)' added: (.*)$`), "Удалённый репозиторий '$1' добавлен: $2"},
	{regexp.MustCompile(`^Remote '(.*)' removed\.$`), "Удалённый репозиторий '$1' удалён."},
	{regexp.MustCompile(`^Remote '(.*)' URL set to (.*)$`), "URL удалённого репозитория '$1' установлен на $2"},

	{regexp.MustCompile(`^Cloning (.*) \.\.\.$`), "Клонирование $1 ..."},
	{regexp.MustCompile(`^Using GitHub token \((.*)\)$`), "Используется токен GitHub ($1)"},
	{regexp.MustCompile(`^Switched working directory to (.*)$`), "Рабочая директория переключена на $1"},
	{regexp.MustCompile(`^Cloned into (.*)$`), "Склонировано в $1"},

	{regexp.MustCompile(`^Creating repository "(.*)" on GitHub\.\.\.$`), "Создание репозитория \"$1\" на GitHub..."},
	{regexp.MustCompile(`^Repository created: (.*) \((.*)\)$`), "Репозиторий создан: $1 ($2)"},
	{regexp.MustCompile(`^Remote 'origin' set to (.*)$`), "Удалённый репозиторий 'origin' установлен на $1"},
	{regexp.MustCompile(`^Clone URL: (.*)$`), "URL для клонирования: $1"},

	{regexp.MustCompile(`^planner completed: goal=(.*)$`), "планировщик завершён: цель=$1"},
	{regexp.MustCompile(`^Plan contains (\d+) subtasks$`), "План содержит $1 подзадач(и)"},
	{regexp.MustCompile(`^Agent subtask (\d+)/(\d+): (.*)$`), "Агентная подзадача $1/$2: $3"},
	{regexp.MustCompile(`^current subtask (\d+)/(\d+): (.*)$`), "текущая подзадача $1/$2: $3"},

	{regexp.MustCompile(`^Reviewer found critical issues: (.*)$`), "Ревьюер нашёл критические проблемы: $1"},
	{regexp.MustCompile(`^Reviewer suggestions: (.*)$`), "Предложения ревьюера: $1"},
	{regexp.MustCompile(`^Verifier: task not fully completed: (.*)$`), "Верификатор: задача выполнена не полностью: $1"},
	{regexp.MustCompile(`^Rollback: (.*)$`), "Откат: $1"},

	{regexp.MustCompile(`^checkpoint failed: (.*)$`), "не удалось создать контрольную точку: $1"},
	{regexp.MustCompile(`^reviewer failed: (.*)$`), "ошибка ревьюера: $1"},
	{regexp.MustCompile(`^verifier failed: (.*)$`), "ошибка верификатора: $1"},
	{regexp.MustCompile(`^structured plan failed, fallback to legacy plan: (.*)$`), "не удалось построить структурированный план, используется упрощённый: $1"},

	{regexp.MustCompile(`^LLM dispatcher usage: requests=(\d+) estimated_tokens=(\d+) duration=(\S+) queue=(\d+)$`), "Использование LLM-диспетчера: запросов=$1, примерных токенов=$2, время=$3, очередь=$4"},

	{regexp.MustCompile(`^Tests failed: (\d+) passed, (\d+) failed(.*)$`), "Тесты упали: пройдено $1, упало $2$3"},
	{regexp.MustCompile(`^Tests passed: (\d+)(.*)$`), "Тесты пройдены: $1$2"},

	{regexp.MustCompile(`^build failed: (.*)$`), "ошибка сборки: $1"},
	{regexp.MustCompile(`^go mod init failed: (.*)$`), "не удалось выполнить go mod init: $1"},
	{regexp.MustCompile(`^no Go files found$`), "Go-файлы не найдены"},
	{regexp.MustCompile(`^no main package found$`), "main-пакет не найден"},

	{regexp.MustCompile(`^- (.*) -> function (.*) \((.*):(\d+)\)$`), "- $1 -> функция $2 ($3:$4)"},
	// ─── Workflow: сообщения с параметрами ──────────────────────
	{regexp.MustCompile(`^Cannot write inbox\.md: (.*)$`),
		"Не удалось записать inbox.md: $1"},
	{regexp.MustCompile(`^Cannot write research\.md: (.*)$`),
		"Не удалось записать research.md: $1"},
	{regexp.MustCompile(`^Cannot write plan\.md: (.*)$`),
		"Не удалось записать plan.md: $1"},
	{regexp.MustCompile(`^Cannot write prd\.json: (.*)$`),
		"Не удалось записать prd.json: $1"},
	{regexp.MustCompile(`^Cannot write gate report: (.*)$`),
		"Не удалось записать отчёт quality gates: $1"},
	{regexp.MustCompile(`^PRD validation failed: (.*); rebuilding fallback PRD$`),
		"Валидация PRD не удалась: $1; пересоздание резервного PRD"},
	{regexp.MustCompile(`^workflow task (\d+) commit failed: (.*)$`),
		"Коммит задачи workflow $1 не удался: $2"},
	{regexp.MustCompile(`^Workflow completed (\d+) task\(s\)\.$`),
		"Workflow завершил $1 задач(у)."},
	{regexp.MustCompile(`^Workflow artifacts dir: (.*)$`),
		"Директория артефактов workflow: $1"},
	{regexp.MustCompile(`^Interview question generation failed: (.*); proceeding without interview$`),
		"Не удалось сформировать вопросы: $1; продолжаем без интервью"},
	{regexp.MustCompile(`^Interview refinement failed: (.*); using original task$`),
		"Не удалось уточнить задачу: $1; используется исходная задача"},
	{regexp.MustCompile(`^Analyzing workflow artifacts: (.*)$`),
		"Анализ артефактов workflow: $1"},
	{regexp.MustCompile(`^LLM reflection failed: (.*); showing raw artifacts$`),
		"Не удалось сформировать ретроспективу: $1; показаны сырые артефакты"},
	{regexp.MustCompile(`^Cannot save reflection\.md: (.*)$`),
		"Не удалось сохранить reflection.md: $1"},
	{regexp.MustCompile(`^Reflection saved: (.*)$`),
		"Ретроспектива сохранена: $1"},
	{regexp.MustCompile(`^no workflow artifacts found: (.*)$`),
		"Артефакты workflow не найдены: $1"},
	{regexp.MustCompile(`^Workflow artifacts: (.*)$`),
		"Артефакты workflow: $1"},
	{regexp.MustCompile(`^Plan review failed \(non-fatal\): (.*)$`),
		"Рецензирование плана не удалось (некритично): $1"},
	{regexp.MustCompile(`^Cannot rewrite plan\.md: (.*)$`),
		"Не удалось перезаписать plan.md: $1"},
	{regexp.MustCompile(`^Cannot rewrite prd\.json: (.*)$`),
		"Не удалось перезаписать prd.json: $1"},
	{regexp.MustCompile(`^Plan refinement failed: (.*); using original plan$`),
		"Не удалось скорректировать план: $1; используется исходный план"},
	{regexp.MustCompile(`^Cannot save lessons to agent memory: (.*)$`),
		"Не удалось сохранить уроки в память агента: $1"},
	{regexp.MustCompile(`^Extracted (\d+) lesson\(s\) for future workflows\.$`),
		"Извлечено уроков для будущих воркфлоу: $1."},
	{regexp.MustCompile(`^Lesson extraction failed \(non-fatal\): (.*)$`),
		"Не удалось извлечь уроки (некритично): $1"},
	{regexp.MustCompile(`^go vet found issues \(non-blocking for small/medium model\): (.*)$`),
		"go vet обнаружил проблемы (не блокирует для малых/средних моделей): $1"},
	{regexp.MustCompile(`^Invalid workflow dir (.*): (.*); using \.gogitor/workflow$`),
		"Некорректная директория workflow $1: $2; используется .gogitor/workflow"},
	{regexp.MustCompile(`^Cannot create workflow dir (.*): (.*); falling back to \.gogitor/workflow$`),
		"Не удалось создать директорию workflow $1: $2; используется .gogitor/workflow"},
	{regexp.MustCompile(`^Cannot create fallback workflow dir (.*): (.*); falling back to simple execution$`),
		"Не удалось создать резервную директорию workflow $1: $2; переключение на простое выполнение"},
	{regexp.MustCompile(`^Creating branch '(.*)' from '(.*)'\.\.\.$`),
		"Создание ветки '$1' из '$2'..."},
	{regexp.MustCompile(`^cannot create branch: (.*)$`),
		"Не удалось создать ветку: $1"},
	{regexp.MustCompile(`^Using existing branch '(.*)'$`),
		"Используется существующая ветка '$1'"},
	{regexp.MustCompile(`^Using current branch '(.*)'$`),
		"Используется текущая ветка '$1'"},
	{regexp.MustCompile(`^Pushing branch '(.*)' to origin\.\.\.$`),
		"Отправка ветки '$1' в origin..."},
	{regexp.MustCompile(`^push failed: (.*)$`),
		"Ошибка отправки: $1"},
	{regexp.MustCompile(`^Creating PR: (.*) → (.*) \.\.\.$`),
		"Создание PR: $1 → $2 ..."},
    {regexp.MustCompile(`^Execution strategy: (\S+) \(source=(\S+), reason=(.*)\)$`),
        "Стратегия выполнения: $1 (источник=$2, причина=$3)"},
    {regexp.MustCompile(`explicit execution mode requested`),
        "явно запрошен режим выполнения"},
    {regexp.MustCompile(`local model and high complexity score (\d+)`),
        "локальная модель и высокая сложность (балл $1)"},
    {regexp.MustCompile(`local model and medium complexity score (\d+)`),
        "локальная модель и средняя сложность (балл $1)"},
    {regexp.MustCompile(`low complexity: (.*)`),
        "низкая сложность: $1"},
    {regexp.MustCompile(`default heuristic`),
        "эвристика по умолчанию"},
}

// ruInlinePatterns аккуратно переводит короткие фрагменты внутри строк.
var ruInlinePatterns = []pattern{
	{regexp.MustCompile(`(\d+) patched \(DIFF\)`), "$1 пропатчено (DIFF)"},
	{regexp.MustCompile(`(\d+) fully rewritten`), "$1 полностью переписано"},
	{regexp.MustCompile(`(\d+) created`), "$1 создано"},
	{regexp.MustCompile(`coverage: ([0-9.]+)%`), "покрытие: $1%"},
	{regexp.MustCompile(`passed=(\d+) failed=(\d+)`), "пройдено=$1, упало=$2"},
}