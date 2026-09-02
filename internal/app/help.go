package app

import (
	"strings"

	"gogitor/internal/domain"
	"gogitor/internal/i18n"
)

// helpTopic — раздел точечной помощи.
type helpTopic struct {
	Name    string
	Aliases []string
	En      string
	Ru      string
}

// helpTopics — все разделы точечной помощи.
var helpTopics = []helpTopic{
	{
		Name:    "load",
		Aliases: []string{":load", "load"},
		En: `## :load — Load Task from File
    
### Syntax
:load <path/to/file.txt|file.md>

### Description
Reads a task from a text file and passes it through the intent router,
exactly as if you had typed the task manually. The router decides which
mode to use (code, analyze, search, fix, etc.).

### Rules
- Only .txt and .md files are accepted.
- Maximum file size: 1 MB.
- Path is relative to the project root, or absolute, or ~/...

### Examples
:load tasks/feature.txt
:load ./refactor.md
:load ~/notes/task.md

### Help
:load help — show this help
:help load — same as above`,
		Ru: `## :load — Загрузка задачи из файла

### Синтаксис
:load <путь/к/файлу.txt|файл.md>

### Описание
Читает задачу из текстового файла и передаёт её роутеру намерений —
точно так же, как если бы вы ввели задачу вручную. Роутер сам выбирает
режим (код, анализ, поиск, исправление и т.д.).

### Правила
- Принимаются только файлы .txt и .md.
- Максимальный размер файла: 1 МБ.
- Путь может быть относительным (от корня проекта), абсолютным или с ~.

### Примеры
:load tasks/feature.txt
:load ./refactor.md
:load ~/notes/task.md

### Помощь
:load help — показать эту помощь
:help load — то же самое`,
	},
	{
		Name:    "git",
		Aliases: []string{":git", "git"},
		En: `## :git — Git Operations

### Syntax
:git <subcommand> [args]

### Subcommands
| Command | Description |
|---------|-------------|
| status | Show working tree status |
| diff | Show changes (working dir vs HEAD) |
| diff-task | Show cumulative diff of the last task |
| commit | Commit all changes (single commit) |
| commit --split <f1,f2> | Separate commits per file |
| init | Initialize git repository |
| log | Show commit history |
| checkout <ref> | Checkout commit or branch |
| checkout -b <name> | Create and switch to new branch |
| branch | List branches |
| branch <name> | Create new branch |
| branch -d <name> | Delete branch |
| merge <branch> | Merge branch into current |
| revert [hash] | Revert a commit (safe) |
| reset [--hard] <hash> | Reset to commit |
| push [branch] | Push to remote |
| pull [branch] | Pull from remote |
| fetch | Fetch from remote |
| clone <url> | Clone repository |
| remote | List remotes |
| remote add <name> <url> | Add remote |
| remote remove <name> | Remove remote |
| create <name> [--private] [--desc ] | Create GitHub repo |
| pr | Create Pull Request |
| issue | Create Issue from failing tests |
| changelog | Generate CHANGELOG.md |
| pr-comment <n> [text] | Add PR comment |

### Examples
:git status
:git commit
:git commit --split main.go,internal/app/app.go
:git push
:git create myproject --private --desc "My project"
:git pr

### Help
:git help — show this help
:help git — same as above`,
		Ru: `## :git — Git-операции

### Синтаксис
:git <подкоманда> [аргументы]

### Подкоманды
| Команда | Описание |
|---------|----------|
| status | Показать состояние рабочей директории |
| diff | Показать изменения (рабочая директория vs HEAD) |
| diff-task | Показать накопительный diff последней задачи |
| commit | Закоммитить все изменения (один коммит) |
| commit --split <ф1,ф2> | Раздельные коммиты по файлам |
| init | Инициализировать git-репозиторий |
| log | Показать историю коммитов |
| checkout <ссылка> | Переключиться на коммит или ветку |
| checkout -b <имя> | Создать ветку и переключиться |
| branch | Список веток |
| branch <имя> | Создать новую ветку |
| branch -d <имя> | Удалить ветку |
| merge <ветка> | Слить ветку в текущую |
| revert [хеш] | Отменить коммит (безопасно) |
| reset [--hard] <хеш> | Откатить к коммиту |
| push [ветка] | Отправить на удалённый репозиторий |
| pull [ветка] | Получить с удалённого репозитория |
| fetch | Загрузить с удалённого репозитория |
| clone <url> | Клонировать репозиторий |
| remote | Список удалённых репозиториев |
| remote add <имя> <url> | Добавить remote |
| remote remove <имя> | Удалить remote |
| create <имя> [--private] [--desc ] | Создать репозиторий на GitHub |
| pr | Создать Pull Request |
| issue | Создать Issue из падающих тестов |
| changelog | Сгенерировать CHANGELOG.md |
| pr-comment <n> [текст] | Добавить комментарий к PR |

### Примеры
:git status
:git commit
:git commit --split main.go,internal/app/app.go
:git push
:git create myproject --private --desc "My project"
:git pr

### Помощь
:git help — показать эту помощь
:help git — то же самое`,
	},
	{
		Name:    "autonomy",
		Aliases: []string{":autonomy", "autonomy"},
		En: `## :autonomy — Autonomous Engineering Mode

### Syntax
:autonomy [on|off|status|run|clear]

### Description
Background monitoring of the project with automatic issue detection.
The monitor periodically runs go build, go vet, and TODO scanning.
Found issues are queued as tasks for potential auto-fixing.

### Subcommands
| Command | Description |
|---------|-------------|
| on | Start background monitoring |
| off | Stop background monitoring |
| status | Show monitor state and task queue |
| run | Execute fixable tasks from the queue |
| clear | Clear the task queue |

### Related Commands
| Command | Description |
|---------|-------------|
| :mutate [limit] | Run mutation testing (deterministic) |
| :autogen-tests [n] | Auto-generate unit tests |

### Configuration
Set in .gogitor.json or environment:
- autonomy_enabled: true/false
- autonomy_interval_sec: monitoring interval
- autonomy_mutation_limit: max mutations per run

### Examples
:autonomy on
:autonomy status
:autonomy run
:mutate 10
:autogen-tests 3

### Help
:autonomy help — show this help
:help autonomy — same as above`,
		Ru: `## :autonomy — Автономный режим

### Синтаксис
:autonomy [on|off|status|run|clear]

### Описание
Фоновый мониторинг проекта с автоматическим обнаружением проблем.
Монитор периодически запускает go build, go vet и сканирование TODO.
Найденные проблемы помещаются в очередь для потенциального автоисправления.

### Подкоманды
| Команда | Описание |
|---------|----------|
| on | Запустить фоновый мониторинг |
| off | Остановить фоновый мониторинг |
| status | Показать состояние монитора и очередь |
| run | Выполнить исправляемые задачи из очереди |
| clear | Очистить очередь задач |

### Связанные команды
| Команда | Описание |
|---------|----------|
| :mutate [лимит] | Мутационное тестирование (детерминированно) |
| :autogen-tests [n] | Автогенерация юнит-тестов |

### Конфигурация
Задаётся в .gogitor.json или переменных окружения:
- autonomy_enabled: true/false
- autonomy_interval_sec: интервал мониторинга
- autonomy_mutation_limit: макс. мутаций за запуск

### Примеры
:autonomy on
:autonomy status
:autonomy run
:mutate 10
:autogen-tests 3

### Помощь
:autonomy help — показать эту помощь
:help autonomy — то же самое`,
	},
	{
		Name:    "computer",
		Aliases: []string{":computer", "computer"},
		En: `## :computer — Computer Mode

### Syntax
:computer <task>

### Description
Execute system administration tasks. The assistant plans commands,
validates safety, executes them, and verifies results.

⚠ WARNING: Executes real commands on your system!

### Requirements
- GOGITOR_COMPUTER_ENABLED=true or --computer flag
- "computer_enabled": true in .gogitor.json

### Safety
- FORBIDDEN commands are blocked immediately
- HIGH risk commands require confirmation
- Command substitution is not allowed
- All actions are logged to .gogitor/computer_audit.json

### CLI Flags
| Flag | Description |
|------|-------------|
| --computer | Enable computer mode |
| --dry-run | Show plan without executing |
| --allow-sudo | Allow sudo commands |

### Examples
:computer show disk usage
:computer list largest files in current directory

### Help
:computer help — show this help
:help computer — same as above`,
		Ru: `## :computer — Режим управления компьютером

### Синтаксис
:computer <задача>

### Описание
Выполнение задач по управлению системой. Ассистент планирует команды,
проверяет безопасность, выполняет их и проверяет результаты.

⚠ ВНИМАНИЕ: Выполняет реальные команды в системе!

### Требования
- GOGITOR_COMPUTER_ENABLED=true или флаг --computer
- "computer_enabled": true в .gogitor.json

### Безопасность
- ЗАПРЕЩЁННЫЕ команды блокируются немедленно
- Команды ВЫСОКОГО риска требуют подтверждения
- Подстановка команд не допускается
- Все действия логируются в .gogitor/computer_audit.json

### Флаги CLI
| Флаг | Описание |
|------|----------|
| --computer | Включить режим управления компьютером |
| --dry-run | Показать план без выполнения |
| --allow-sudo | Разрешить sudo |

### Примеры
:computer показать использование дисков
:computer список самых больших файлов в текущей директории

### Помощь
:computer help — показать эту помощь
:help computer — то же самое`,
	},
	{
		Name:    "article",
		Aliases: []string{":article", "article"},
		En: `## :article — Article Generation

### Syntax
:article <topic>
:article --full <topic>

### Description
Generate technical articles, tutorials, stories, and other texts.

### Modes
| Mode | Description |
|------|-------------|
| :article <topic> | Simple article (single LLM call) |
| :article --full <topic> | Complex multi-section article |

### Genres (auto-detected)
technical, news, story, review, howto, code_desc, free

### Examples
:article how garbage collection works in Go
:article --full middleware pattern deep dive

### Help
:article help — show this help
:help article — same as above`,
		Ru: `## :article — Генерация статей

### Синтаксис
:article <тема>
:article --full <тема>

### Описание
Генерация технических статей, инструкций, рассказов и других текстов.

### Режимы
| Режим | Описание |
|-------|----------|
| :article <тема> | Простая статья (один вызов LLM) |
| :article --full <тема> | Сложная многосекционная статья |

### Жанры (определяются автоматически)
technical, news, story, review, howto, code_desc, free

### Примеры
:article как работает сборщик мусора в Go
:article --full подробный разбор паттерна middleware

### Помощь
:article help — показать эту помощь
:help article — то же самое`,
	},
	{
		Name:    "fix",
		Aliases: []string{":fix", "fix"},
		En: `## :fix — Error Fixing

### Syntax
:fix <error output / stack trace>

### Description
Fix errors from compiler output, test failures, panics, or stack traces.
The assistant parses the error, identifies source files, and generates fixes.

### Auto-detection
The assistant automatically detects error traces in regular input:
- "panic:" → fix mode
- "runtime error" → fix mode
- ".go:123" → fix mode
- "--- FAIL" → fix mode

### Examples
:fix panic: runtime error: index out of range [3] with length 2
:fix build error in internal/app/app.go

### Help
:fix help — show this help
:help fix — same as above`,
		Ru: `## :fix — Исправление ошибок

### Синтаксис
:fix <вывод ошибки / stack trace>

### Описание
Исправление ошибок из вывода компилятора, падений тестов, panic или stack trace.
Ассистент разбирает ошибку, определяет исходные файлы и генерирует исправления.

### Автоопределение
Ассистент автоматически определяет трассировки ошибок в обычном вводе:
- "panic:" → режим fix
- "runtime error" → режим fix
- ".go:123" → режим fix
- "--- FAIL" → режим fix

### Примеры
:fix panic: runtime error: index out of range [3] with length 2
:fix ошибка сборки в internal/app/app.go

### Помощь
:fix help — показать эту помощь
:help fix — то же самое`,
	},
	{
		Name:    "agent",
		Aliases: []string{":agent", "agent"},
		En: `## :agent — Multi-Agent Mode

### Syntax
:agent <task> [flags]
:agent deep <task>
:agent interview <task>
:agent reflect

### Description
Forces multi-agent execution regardless of task complexity.
The task goes through a four-stage pipeline:

1. **Planner** — breaks the task into 2–7 subtasks with acceptance criteria
2. **Coder** — implements each subtask, validates with go build / go test
3. **Reviewer** — checks for critical issues (compilation errors, security, regressions)
4. **Verifier** — confirms the original task goal was achieved

### When to use
- Complex refactoring across multiple files
- Tasks requiring architectural decisions
- When you need review and verification guarantees
- When but you want full pipeline

### Differences from :code
| Mode | Pipeline | Use case |
|------|----------|----------|
| :code | Auto-selects agent | Default choice |
| :agent | Full 4-stage pipeline | Complex multi-file tasks |

### Flags
Same as :code — see :code help

### Examples
:agent refactor authentication module into separate package
:agent create REST API with middleware, tests, and documentation

### Help
:agent help — show this help
:help agent — same as above`,
		Ru: `## :agent — Мультиагентный режим

### Синтаксис
:agent <задача> [флаги]
:agent deep <задача>
:agent interview <задача>
:agent reflect

### Описание
Принудительно запускает мультиагентное выполнение независимо от сложности задачи.
Задача проходит четырёхэтапный конвейер:

1. **Планировщик** — разбивает задачу на 2–7 подзадач с критериями приёмки
2. **Кодер** — реализует каждую подзадачу, проверяет через go build / go test
3. **Ревьюер** — проверяет критические проблемы (ошибки компиляции, безопасность, регрессии)
4. **Верификатор** — подтверждает, что исходная цель задачи достигнута

### Когда использовать
- Сложный рефакторинг нескольких файлов
- Задачи, требующие архитектурных решений
- Когда нужны гарантии ревью и верификации
- Когда нужен полный конвейер

### Отличия от :code
| Режим | Конвейер | Когда использовать |
|-------|----------|-------------------|
| :code | Автовыбор agent | Выбор по умолчанию |
| :agent | Полный 4-этапный конвейер | Сложные многофайловые задачи |

### Флаги
Аналогичны :code — см. :code help

### Примеры
:agent отрефакторить модуль аутентификации в отдельный пакет
:agent создать REST API с middleware, тестами и документацией

### Помощь
:agent help — показать эту помощь
:help agent — то же самое`,
	},
	{
		Name:    "mutate",
		Aliases: []string{":mutate", "mutate"},
		En: `## :mutate — Mutation Testing

### Syntax
:mutate [limit]

### Description
Deterministically generates code mutations and checks if your tests catch them.
No LLM is used — mutations are generated by operator substitution.

### Mutation types
| Type | Example |
|------|---------|
| Relational | > → >=, < → <= |
| Logical | && → \|\|, \|\| → && |
| Equality | == → !=, != → == |

### Reading the report
- **Killed** — your test detected the mutation (good)
- **Survived** — your test missed the mutation (weak test)
- **Score** — percentage of killed mutations

### Examples
:mutate          Run with default limit (20 mutations)
:mutate 50       Run with 50 mutations

### Help
:mutate help — show this help
:help mutate — same as above`,
		Ru: `## :mutate — Мутационное тестирование

### Синтаксис
:mutate [лимит]

### Описание
Детерминированно генерирует мутации кода и проверяет, ловят ли их ваши тесты.
LLM не используется — мутации генерируются заменой операторов.

### Типы мутаций
| Тип | Пример |
|-----|--------|
| Отношения | > → >=, < → <= |
| Логика | && → \|\|, \|\| → && |
| Равенство | == → !=, != → == |

### Как читать отчёт
- **Убита** — ваш тест обнаружил мутацию (хорошо)
- **Выжила** — ваш тест пропустил мутацию (слабый тест)
- **Оценка** — процент убитых мутаций

### Примеры
:mutate          Запуск с лимитом по умолчанию (20 мутаций)
:mutate 50       Запуск с 50 мутациями

### Помощь
:mutate help — показать эту помощь
:help mutate — то же самое`,
	},
	{
		Name:    "suggest",
		Aliases: []string{":suggest", "suggest"},
		En: `## :suggest — Project Health Analysis

### Syntax
:suggest

### Description
Analyzes the project health using LLM. Produces actionable improvement
suggestions organized into 5 categories:

- 🔴 **Critical** — bugs, security vulnerabilities, data loss risks
- 🟡 **Tech Debt** — temporary solutions, duplicated code
- 🧪 **Missing Tests** — exported functions without test coverage
- 🧹 **Code Smells** — style issues, naming problems, unused code
- 💡 **Improvements** — performance, architecture improvements

### Behavior
- Each suggestion references a specific file and function/line
- Suggestions from previous sessions are not repeated
- Maximum 5 items per category

### Examples
:suggest

### Help
:suggest help — show this help
:help suggest — same as above`,
		Ru: `## :suggest — Анализ состояния проекта

### Синтаксис
:suggest

### Описание
Анализирует состояние проекта с помощью LLM. Формирует конкретные
предложения по улучшению, организованные в 5 категорий:

- 🔴 **Критические** — баги, уязвимости, риски потери данных
- 🟡 **Технический долг** — временные решения, дублирование кода
- 🧪 **Отсутствующие тесты** — экспортированные функции без покрытия
- 🧹 **Запахи кода** — проблемы стиля, именования, неиспользуемый код
- 💡 **Улучшения** — производительность, архитектура

### Поведение
- Каждое предложение ссылается на конкретный файл и функцию/строку
- Предложения из предыдущих сессий не повторяются
- Максимум 5 пунктов в каждой категории

### Примеры
:suggest

### Помощь
:suggest help — показать эту помощь
:help suggest — то же самое`,
	},
	{
		Name:    "decisions",
		Aliases: []string{":decisions", ":journal", "decisions", "journal"},
		En: `## :decisions — Decision Journal

### Syntax
:decisions

### Description
Shows the project decision journal with LLM analysis of "decision debt".

The journal records important engineering decisions made during
multi-agent sessions, including:
- Decisions made
- Alternatives considered and rejected
- Constraints that forced the decision
- Sources of decisions

### Decision Debt
LLM analyzes the journal to find temporary decisions whose original
constraints may no longer apply, suggesting what to revisit.

### Examples
:decisions
:journal

### Help
:decisions help — show this help
:help decisions — same as above`,
		Ru: `## :decisions — Журнал решений

### Синтаксис
:decisions

### Описание
Показывает журнал инженерных решений проекта с LLM-анализом «долга решений».

Журнал записывает важные инженерные решения, принятые во время
мультиагентных сессий, включая:
- Принятые решения
- Рассмотренные и отклонённые альтернативы
- Ограничения, вынудившие принять решение
- Источники решений

### Долг решений
LLM анализирует журнал, чтобы найти временные решения, ограничения
которых могли перестать быть актуальными, и предлагает, что пересмотреть.

### Примеры
:decisions
:journal

### Помощь
:decisions help — показать эту помощь
:help decisions — то же самое`,
	},
	{
		Name:    "search",
		Aliases: []string{":search", "search"},
		En: `## :search — Web Search

### Syntax
:search <query>

### Description
Performs web search and summarizes results using LLM.

### How it works
1. Your query is rewritten by LLM into an optimized search query
2. Web search is performed (DuckDuckGo)
3. Results are sanitized (prompt injection protection)
4. LLM summarizes the results into a coherent answer
5. Sources are listed at the end of the response

### Safety
- Rate limiting: max 3 searches per session
- Domain whitelist filtering
- SSRF protection
- Prompt injection sanitization

### Examples
:search latest Go version features
:search best practices for Go error handling

### Help
:search help — show this help
:help search — same as above`,
		Ru: `## :search — Веб-поиск

### Синтаксис
:search <запрос>

### Описание
Выполняет веб-поиск и резюмирует результаты с помощью LLM.

### Как это работает
1. Ваш запрос переписывается LLM в оптимизированный поисковый запрос
2. Выполняется веб-поиск (DuckDuckGo)
3. Результаты санитизируются (защита от prompt injection)
4. LLM резюмирует результаты в связный ответ
5. Источники перечисляются в конце ответа

### Безопасность
- Ограничение частоты: макс. 3 поиска за сессию
- Фильтрация по белому списку доменов
- Защита от SSRF
- Санитизация от prompt injection

### Примеры
:search последние возможности Go
:search лучшие практики обработки ошибок в Go

### Помощь
:search help — показать эту помощь
:help search — то же самое`,
	},
	{
		Name:    "vet",
		Aliases: []string{":vet", "vet"},
		En: `## :vet — Go Vet

### Syntax
:vet

### Description
Runs go vet ./... in a sandbox. Fast static analysis without LLM.

### Difference from :test lint
- :vet runs go vet only (fast, no LLM)
- :test lint runs golangci-lint and auto-fixes issues via LLM

### Examples
:vet

### Help
:vet help — show this help
:help vet — same as above`,
		Ru: `## :vet — Go Vet

### Синтаксис
:vet

### Описание
Запускает go vet ./... в песочнице. Быстрый статический анализ без LLM.

### Отличие от :test lint
- :vet запускает только go vet (быстро, без LLM)
- :test lint запускает golangci-lint и автоисправляет проблемы через LLM

### Примеры
:vet

### Помощь
:vet help — показать эту помощь
:help vet — то же самое`,
	},
	{
		Name:    "todo",
		Aliases: []string{":todo", "todo"},
		En: `## :todo — TODO Scanner

### Syntax
:todo

### Description
Scans Go source files for TODO/FIXME/HACK/XXX/BUG markers.
Fast scan without LLM.

### Behavior
- Scans all .go files in the project
- Searches in comments only (not in code)
- Maximum 50 items
- Output format: file:line [MARKER] text

### Examples
:todo

### Help
:todo help — show this help
:help todo — same as above`,
		Ru: `## :todo — Сканер TODO

### Синтаксис
:todo

### Описание
Сканирует Go-файлы проекта на маркеры TODO/FIXME/HACK/XXX/BUG.
Быстрое сканирование без LLM.

### Поведение
- Сканирует все .go файлы проекта
- Ищет только в комментариях (не в коде)
- Максимум 50 элементов
- Формат вывода: файл:строка [МАРКЕР] текст

### Примеры
:todo

### Помощь
:todo help — показать эту помощь
:help todo — то же самое`,
	},
	{
		Name:    "analyze",
		Aliases: []string{":analyze", ":analysis", "analyze", "analysis"},
		En: `## :analyze — Code Analysis

### Syntax
:analyze <question>

### Description
Analyzes the project code without modifying files. Uses the project
index to select the most relevant files for analysis.

### Difference from :ask and :code
- :analyze — analyzes code, does NOT modify files
- :ask — general chat, no project context
- :code — creates or modifies code

### Image analysis
Use --image flag (CLI) or attach image path in the query (TUI):
gogitor analyze "explain this diagram" --image diagram.png

### Examples
:analyze find potential bugs in this project
:analyze explain the authentication flow

### Help
:analyze help — show this help
:help analyze — same as above`,
		Ru: `## :analyze — Анализ кода

### Синтаксис
:analyze <вопрос>

### Описание
Анализирует код проекта без изменения файлов. Использует индекс
проекта для выбора наиболее релевантных файлов для анализа.

### Отличие от :ask и :code
- :analyze — анализирует код, НЕ изменяет файлы
- :ask — общий чат, без контекста проекта
- :code — создаёт или изменяет код

### Анализ изображений
Используйте флаг --image (CLI) или укажите путь к изображению (TUI):
gogitor analyze "объясни эту диаграмму" --image diagram.png

### Примеры
:analyze найди потенциальные баги в проекте
:analyze объясни поток аутентификации

### Помощь
:analyze help — показать эту помощь
:help analyze — то же самое`,
	},
	{
		Name:    "ask",
		Aliases: []string{":ask", "ask"},
		En: `## :ask — Chat Mode

### Syntax
:ask <question>

### Description
General chat mode. Answers questions about Go development,
provides advice, explains concepts. Does NOT modify files.

### Difference from :analyze
- :ask — general chat, conversation history is maintained
- :analyze — analyzes project code with project context

### Image analysis
Use --image flag (CLI) or attach image path in the query (TUI):
gogitor ask "what is on this image?" --image screenshot.png

### Examples
:ask explain context.Context
:ask what is the difference between goroutine and thread?

### Help
:ask help — show this help
:help ask — same as above`,
		Ru: `## :ask — Режим чата

### Синтаксис
:ask <вопрос>

### Описание
Общий режим чата. Отвечает на вопросы о разработке на Go,
даёт советы, объясняет концепции. НЕ изменяет файлы.

### Отличие от :analyze
- :ask — общий чат, ведётся история диалога
- :analyze — анализирует код проекта с контекстом проекта

### Анализ изображений
Используйте флаг --image (CLI) или укажите путь к изображению (TUI):
gogitor ask "что на этом изображении?" --image screenshot.png

### Примеры
:ask объясни context.Context
:ask в чём разница между горутиной и потоком?

### Помощь
:ask help — показать эту помощь
:help ask — то же самое`,
	},
	{
		Name:    "autogen-tests",
		Aliases: []string{":autogen-tests", "autogen-tests"},
		En: `## :autogen-tests — Auto Test Generation

### Syntax
:autogen-tests [count]

### Description
Scans the project AST for exported functions that have no corresponding
_test.go file, then generates table-driven tests via LLM.

### How it works
1. Finds exported functions without test coverage (AST scan, no LLM)
2. For each function, sends a focused prompt to LLM
3. Generated test is written to a _test.go file
4. Tests are run immediately — if they fail, the file is deleted

### What gets generated
- Table-driven tests with t.Run subtests
- At least 3 cases: happy path, boundary, error/edge
- Only standard library "testing" package

### Examples
:autogen-tests       Generate tests for up to 5 functions
:autogen-tests 3     Generate tests for up to 3 functions

### Help
:autogen-tests help — show this help
:help autogen-tests — same as above`,
		Ru: `## :autogen-tests — Автогенерация тестов

### Синтаксис
:autogen-tests [количество]

### Описание
Сканирует AST проекта в поисках экспортированных функций без
соответствующего файла _test.go, затем генерирует табличные тесты через LLM.

### Как это работает
1. Находит экспортированные функции без тестов (сканирование AST, без LLM)
2. Для каждой функции отправляет сфокусированный промпт в LLM
3. Сгенерированный тест записывается в файл _test.go
4. Тесты немедленно запускаются — если падают, файл удаляется

### Что генерируется
- Табличные тесты с подтестами t.Run
- Минимум 3 случая: нормальный, граничный, ошибка/крайний случай
- Только стандартная библиотека "testing"

### Примеры
:autogen-tests       Генерация тестов для 5 функций
:autogen-tests 3     Генерация тестов для 3 функций

### Помощь
:autogen-tests help — показать эту помощь
:help autogen-tests — то же самое`,
	},
	{
		Name:    "reasoning",
		Aliases: []string{":reasoning", "reasoning"},
		En: `## :reasoning — Reasoning Mode

### Syntax
:reasoning [on|off|router [on|off]]

### Description
Control the reasoning/thinking mode for models that support it
(DeepSeek-R1, QwQ, Qwen3, OpenAI o1/o3/o4-mini, etc.)

### Subcommands
| Command | Description |
|---------|-------------|
| :reasoning | Show current state |
| :reasoning on | Enable reasoning mode |
| :reasoning off | Disable reasoning mode |
| :reasoning router on | Enable reasoning for intent router |
| :reasoning router off | Disable reasoning for intent router |

### Environment Variables
GOGITOR_REASONING=true
GOGITOR_REASONING_EFFORT=low|medium|high
GOGITOR_REASONING_BUDGET=<tokens>

### Help
:reasoning help — show this help
:help reasoning — same as above`,
		Ru: `## :reasoning — Режим размышления

### Синтаксис
:reasoning [on|off|router [on|off]]

### Описание
Управление режимом размышления для моделей, которые его поддерживают
(DeepSeek-R1, QwQ, Qwen3, OpenAI o1/o3/o4-mini и др.)

### Подкоманды
| Команда | Описание |
|---------|----------|
| :reasoning | Показать текущее состояние |
| :reasoning on | Включить режим размышления |
| :reasoning off | Выключить режим размышления |
| :reasoning router on | Включить размышления для роутера |
| :reasoning router off | Выключить размышления для роутера |

### Переменные окружения
GOGITOR_REASONING=true
GOGITOR_REASONING_EFFORT=low|medium|high
GOGITOR_REASONING_BUDGET=<токены>

### Помощь
:reasoning help — показать эту помощь
:help reasoning — то же самое`,
	},
	{
		Name:    "test",
		Aliases: []string{":test", "test"},
		En: `## :test — Testing

### Syntax
:test
:test lint

### Description
Run Go tests or linting in a sandbox.

### Subcommands
| Command | Description |
|---------|-------------|
| :test | Run go test -v -cover ./... |
| :test lint | Run golangci-lint and auto-fix issues |

### Related Commands
| Command | Description |
|---------|-------------|
| :vet | Run go vet (fast, no LLM) |
| :mutate [limit] | Mutation testing |
| :autogen-tests [n] | Auto-generate unit tests |

### Help
:test help — show this help
:help test — same as above`,
		Ru: `## :test — Тестирование

### Синтаксис
:test
:test lint

### Описание
Запуск Go-тестов или линтинга в песочнице.

### Подкоманды
| Команда | Описание |
|---------|----------|
| :test | Запуск go test -v -cover ./... |
| :test lint | Запуск golangci-lint и автоисправление |

### Связанные команды
| Команда | Описание |
|---------|----------|
| :vet | Запуск go vet (быстро, без LLM) |
| :mutate [лимит] | Мутационное тестирование |
| :autogen-tests [n] | Автогенерация юнит-тестов |

### Помощь
:test help — показать эту помощь
:help test — то же самое`,
	},
}

// HelpForCommand возвращает точечную помощь по команде.
// Если команда не найдена, возвращает список доступных разделов.
func HelpForCommand(cmd string) domain.Result {
	cmd = strings.ToLower(strings.TrimSpace(cmd))
	cmd = strings.TrimPrefix(cmd, ":")

	for _, topic := range helpTopics {
		if topic.Name == cmd {
			text := topic.En
			if i18n.Current() == i18n.RU {
				text = topic.Ru
			}
			return domain.Result{
				Success:  true,
				Mode:     "help",
				Response: text,
			}
		}
		for _, alias := range topic.Aliases {
			if strings.TrimPrefix(alias, ":") == cmd {
				text := topic.En
				if i18n.Current() == i18n.RU {
					text = topic.Ru
				}
				return domain.Result{
					Success:  true,
					Mode:     "help",
					Response: text,
				}
			}
		}
	}

	// Команда не найдена — показываем список доступных разделов.
	var available []string
	for _, topic := range helpTopics {
		available = append(available, "  :help "+topic.Name)
	}

	msg := "Unknown help topic: " + cmd + "\n\nAvailable topics:\n" +
		strings.Join(available, "\n") +
		"\n\nUse :help for full help."

	if i18n.Current() == i18n.RU {
		msg = "Неизвестный раздел помощи: " + cmd + "\n\nДоступные разделы:\n" +
			strings.Join(available, "\n") +
			"\n\nИспользуйте :help для полной справки."
	}

	return domain.Result{
		Success:  true,
		Mode:     "help",
		Response: msg,
	}
}

// HelpTopicNames возвращает имена всех разделов помощи (для автодополнения).
func HelpTopicNames() []string {
	names := make([]string, 0, len(helpTopics))
	for _, topic := range helpTopics {
		names = append(names, topic.Name)
	}
	return names
}
