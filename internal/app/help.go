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
| Rule | Detail |
|------|--------|
| Extensions | Only .txt and .md accepted |
| Max size | 1 MB |
| Path | Relative to project root, absolute, or ~/... |
| Encoding | UTF-8, BOM stripped automatically |
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
| Правило | Детали |
|---------|--------|
| Расширения | Только .txt и .md |
| Макс. размер | 1 МБ |
| Путь | Относительный от корня проекта, абсолютный или ~/... |
| Кодировка | UTF-8, BOM удаляется автоматически |
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
| revert [hash] | Revert a commit (safe, creates new commit) |
| reset [--hard] <hash> | Reset to commit (--hard is destructive) |
| push [branch] | Push to remote |
| pull [branch] | Pull from remote |
| fetch | Fetch from remote |
| clone <url> | Clone repository |
| remote | List remotes |
| remote add <name> <url> | Add remote |
| remote remove <name> | Remove remote |
| create <name> [--private] [--desc <text>] | Create GitHub repo |
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
| revert [хеш] | Отменить коммит (безопасно, создаёт новый) |
| reset [--hard] <хеш> | Откатить к коммиту (--hard удаляет изменения) |
| push [ветка] | Отправить на удалённый репозиторий |
| pull [ветка] | Получить с удалённого репозитория |
| fetch | Загрузить с удалённого репозитория |
| clone <url> | Клонировать репозиторий |
| remote | Список удалённых репозиториев |
| remote add <имя> <url> | Добавить remote |
| remote remove <имя> | Удалить remote |
| create <имя> [--private] [--desc <текст>] | Создать репозиторий на GitHub |
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
### Subcommands
| Command | Description |
|---------|-------------|
| on | Start background monitoring (go build, go vet, TODO scan) |
| off | Stop background monitoring |
| status | Show monitor state and pending task queue |
| run | Execute fixable tasks from the queue via LLM |
| clear | Clear the task queue |
### Monitor Checks
| Check | Priority | Description |
|-------|----------|-------------|
| go build | Critical | Detects compilation errors |
| go vet | High | Detects static analysis issues |
| TODO scan | Low | Finds TODO/FIXME/HACK markers |
### Configuration
| Setting | Location | Default |
|---------|----------|---------|
| autonomy_enabled | .gogitor.json / env | false |
| autonomy_interval_sec | .gogitor.json / env | 60 |
| autonomy_mutation_limit | .gogitor.json / env | 20 |
### Related Commands
| Command | Description |
|---------|-------------|
| :mutate [limit] | Run mutation testing (deterministic, no LLM) |
| :autogen-tests [n] | Auto-generate unit tests |
### Examples
:autonomy on
:autonomy status
:autonomy run
:autonomy clear
### Help
:autonomy help — show this help
:help autonomy — same as above`,
		Ru: `## :autonomy — Автономный режим
### Синтаксис
:autonomy [on|off|status|run|clear]
### Подкоманды
| Команда | Описание |
|---------|----------|
| on | Запустить фоновый мониторинг (go build, go vet, TODO) |
| off | Остановить фоновый мониторинг |
| status | Показать состояние монитора и очередь задач |
| run | Выполнить исправляемые задачи из очереди через LLM |
| clear | Очистить очередь задач |
### Проверки монитора
| Проверка | Приоритет | Описание |
|----------|-----------|----------|
| go build | Критический | Обнаружение ошибок компиляции |
| go vet | Высокий | Обнаружение проблем статического анализа |
| TODO scan | Низкий | Поиск маркеров TODO/FIXME/HACK |
### Конфигурация
| Параметр | Расположение | По умолчанию |
|----------|--------------|--------------|
| autonomy_enabled | .gogitor.json / env | false |
| autonomy_interval_sec | .gogitor.json / env | 60 |
| autonomy_mutation_limit | .gogitor.json / env | 20 |
### Связанные команды
| Команда | Описание |
|---------|----------|
| :mutate [лимит] | Мутационное тестирование (детерминированно) |
| :autogen-tests [n] | Автогенерация юнит-тестов |
### Примеры
:autonomy on
:autonomy status
:autonomy run
:autonomy clear
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
| Requirement | How to enable |
|-------------|---------------|
| Flag | --computer |
| Env var | GOGITOR_COMPUTER_ENABLED=true |
| Config | "computer_enabled": true in .gogitor.json |
### Safety Levels
| Risk Level | Behavior |
|------------|----------|
| Low | Auto-execute (ls, cat, pwd) |
| Medium | Log + optional confirm (apt install, git clone) |
| High | Mandatory confirmation (rm, chmod, sudo) |
| Forbidden | Immediate block (rm -rf /, mkfs, dd of=/dev/*) |
### CLI Flags
| Flag | Description |
|------|-------------|
| --computer | Enable computer mode |
| --dry-run | Show plan without executing |
| --allow-sudo | Allow sudo commands |
### Audit
All commands are logged to .gogitor/computer_audit.json
### Examples
:computer show disk usage
:computer list largest files in current directory
:computer install curl
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
| Требование | Как включить |
|------------|--------------|
| Флаг | --computer |
| Переменная | GOGITOR_COMPUTER_ENABLED=true |
| Конфиг | "computer_enabled": true в .gogitor.json |
### Уровни безопасности
| Уровень риска | Поведение |
|---------------|-----------|
| Низкий | Автовыполнение (ls, cat, pwd) |
| Средний | Лог + опциональное подтверждение (apt install, git clone) |
| Высокий | Обязательное подтверждение (rm, chmod, sudo) |
| Запрещённый | Немедленная блокировка (rm -rf /, mkfs, dd of=/dev/*) |
### Флаги CLI
| Флаг | Описание |
|------|----------|
| --computer | Включить режим управления компьютером |
| --dry-run | Показать план без выполнения |
| --allow-sudo | Разрешить sudo |
### Аудит
Все команды логируются в .gogitor/computer_audit.json
### Примеры
:computer показать использование дисков
:computer список самых больших файлов в текущей директории
:computer установить curl
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
### Modes
| Mode | Description |
|------|-------------|
| :article <topic> | Simple article (single LLM call, 300-800 words) |
| :article --full <topic> | Complex multi-section article with plan |
### Genres (auto-detected)
| Genre | Trigger keywords |
|-------|-----------------|
| technical | статья, article, техническ, документаци |
| news | новост, news, релиз, release, анонс |
| story | рассказ, история, сказка, story, tale |
| review | сравни, обзор, vs, review, comparison |
| howto | как сделать, инструкци, how to, tutorial |
| code_desc | опиши код, что делает, explain function |
| free | anything else |
### Complex Mode Pipeline
1. Classify genre and parameters
2. Generate article plan (sections + key points)
3. Write each section sequentially
4. Assemble final article
### Examples
:article how garbage collection works in Go
:article --full middleware pattern deep dive
:article расскажи про каналы в Go
### Help
:article help — show this help
:help article — same as above`,
		Ru: `## :article — Генерация статей
### Синтаксис
:article <тема>
:article --full <тема>
### Режимы
| Режим | Описание |
|-------|----------|
| :article <тема> | Простая статья (один вызов LLM, 300-800 слов) |
| :article --full <тема> | Сложная многосекционная статья с планом |
### Жанры (определяются автоматически)
| Жанр | Ключевые слова |
|------|----------------|
| technical | статья, article, техническ, документаци |
| news | новост, news, релиз, release, анонс |
| story | рассказ, история, сказка, story, tale |
| review | сравни, обзор, vs, review, comparison |
| howto | как сделать, инструкци, how to, tutorial |
| code_desc | опиши код, что делает, explain function |
| free | всё остальное |
### Конвейер сложного режима
1. Классификация жанра и параметров
2. Генерация плана статьи (секции + ключевые точки)
3. Последовательное написание каждой секции
4. Сборка финальной статьи
### Примеры
:article как работает сборщик мусора в Go
:article --full подробный разбор паттерна middleware
:article расскажи про каналы в Go
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
### Supported Error Types
| Type | Detection pattern | Example |
|------|-------------------|---------|
| panic | "panic:" | panic: runtime error: index out of range |
| fatal | "fatal error:" | fatal error: concurrent map writes |
| build | ".go:123:" | ./main.go:5:2: undefined: foo |
| test | "--- FAIL:" | --- FAIL: TestAdd (0.00s) |
| runtime | "goroutine" | goroutine 1 [running]: |
### Auto-detection
The assistant automatically detects error traces in regular input.
You can paste an error directly without using :fix explicitly.
### Pipeline
1. Parse error trace → identify error type
2. Extract file paths and line numbers
3. Filter external paths (stdlib, vendor)
4. Build targeted fix prompt
5. Execute code generation with fix context
### Examples
:fix panic: runtime error: index out of range [3] with length 2
:fix build error in internal/app/app.go
:fix --- FAIL: TestAdd (0.00s)
### Help
:fix help — show this help
:help fix — same as above`,
		Ru: `## :fix — Исправление ошибок
### Синтаксис
:fix <вывод ошибки / stack trace>
### Описание
Исправление ошибок из вывода компилятора, падений тестов, panic или stack trace.
Ассистент разбирает ошибку, определяет исходные файлы и генерирует исправления.
### Поддерживаемые типы ошибок
| Тип | Паттерн обнаружения | Пример |
|-----|---------------------|--------|
| panic | "panic:" | panic: runtime error: index out of range |
| fatal | "fatal error:" | fatal error: concurrent map writes |
| build | ".go:123:" | ./main.go:5:2: undefined: foo |
| test | "--- FAIL:" | --- FAIL: TestAdd (0.00s) |
| runtime | "goroutine" | goroutine 1 [running]: |
### Автоопределение
Ассистент автоматически определяет трассировки ошибок в обычном вводе.
Можно вставить ошибку напрямую без явного использования :fix.
### Конвейер
1. Разбор трассировки → определение типа ошибки
2. Извлечение путей к файлам и номеров строк
3. Фильтрация внешних путей (stdlib, vendor)
4. Формирование целевого промпта для исправления
5. Генерация кода с контекстом исправления
### Примеры
:fix panic: runtime error: index out of range [3] with length 2
:fix ошибка сборки в internal/app/app.go
:fix --- FAIL: TestAdd (0.00s)
### Помощь
:fix help — показать эту помощь
:help fix — то же самое`,
	},
	{
		Name:    "agent",
		Aliases: []string{":agent", "agent"},
		En: `## :agent — Multi-Agent Mode
### Syntax
:agent <subcommand> [args]
### Subcommands
| Command | Description |
|---------|-------------|
| :agent <task> | Run full agent pipeline (planner→coder→reviewer→verifier) |
| :agent deep <task> | Run with strict patch policy and quality gates |
| :agent interview <task> | Ask clarifying questions before execution |
| :agent reflect | Analyze latest agent session and extract lessons |
| :agent report | Show latest agent session report |
| :agent resume | Resume latest failed agent session |
| :agent undo | Revert latest completed agent commit |
### Pipeline Stages
| Stage | Role | Description |
|-------|------|-------------|
| 1. Planning | Planner | Breaks task into 2-7 subtasks with acceptance criteria |
| 2. Coding | Coder | Implements each subtask, validates with go build/test |
| 3. Review | Reviewer | Checks for compilation errors, security, regressions |
| 4. Verification | Verifier | Confirms original task goal was achieved |
### Deep Mode Extras
| Feature | Description |
|---------|-------------|
| Strict patch policy | No fuzzy matching, Symbol anchors required |
| Quality gates | gofmt, go build, go test, go vet, golangci-lint |
| Session artifacts | Saved to .gogitor/agent/<timestamp>/ |
| Final verification | All gates must pass before success |
### Related Commands
| Command | Description |
|---------|-------------|
| :code <task> | Auto-selects execution strategy |
| :fast <task> | Single-pass mode (no agent pipeline) |
### Examples
:agent refactor authentication module into separate package
:agent deep create REST API with middleware and tests
:agent interview add caching to the API layer
:agent reflect
:agent undo
### Help
:agent help — show this help
:help agent — same as above`,
		Ru: `## :agent — Мультиагентный режим
### Синтаксис
:agent <подкоманда> [аргументы]
### Подкоманды
| Команда | Описание |
|---------|----------|
| :agent <задача> | Полный конвейер (планировщик→кодер→ревьюер→верификатор) |
| :agent deep <задача> | Строгая политика патчей и quality gates |
| :agent interview <задача> | Уточняющие вопросы перед выполнением |
| :agent reflect | Анализ последней сессии и извлечение уроков |
| :agent report | Отчёт последней сессии |
| :agent resume | Продолжить последнюю неудачную сессию |
| :agent undo | Отменить последний коммит агента |
### Этапы конвейера
| Этап | Роль | Описание |
|------|------|----------|
| 1. Планирование | Планировщик | Разбивает задачу на 2-7 подзадач с критериями |
| 2. Кодирование | Кодер | Реализует подзадачи, проверяет go build/test |
| 3. Ревью | Ревьюер | Проверка ошибок компиляции, безопасности, регрессий |
| 4. Верификация | Верификатор | Подтверждение достижения исходной цели |
### Дополнения Deep-режима
| Возможность | Описание |
|-------------|----------|
| Строгая политика патчей | Без fuzzy, обязательны Symbol-якоря |
| Quality gates | gofmt, go build, go test, go vet, golangci-lint |
| Артефакты сессии | Сохраняются в .gogitor/agent/<timestamp>/ |
| Финальная верификация | Все проверки должны пройти |
### Связанные команды
| Команда | Описание |
|---------|----------|
| :code <задача> | Автоматический выбор стратегии |
| :fast <задача> | Однопроходный режим (без конвейера) |
### Примеры
:agent отрефакторить модуль аутентификации в отдельный пакет
:agent deep создать REST API с middleware и тестами
:agent interview добавить кэширование в API
:agent reflect
:agent undo
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
### Mutation Operators
| Type | Original | Mutated | Example |
|------|----------|---------|---------|
| Relational | >= | > | if x >= 10 → if x > 10 |
| Relational | <= | < | if x <= 5 → if x < 5 |
| Logical | && | \|\| | if a && b → if a \|\| b |
| Logical | \|\| | && | if a \|\| b → if a && b |
| Equality | == | != | if a == b → if a != b |
| Equality | != | == | if a != b → if a == b |
### Report Interpretation
| Status | Meaning |
|--------|---------|
| Killed ✓ | Your test detected the mutation (good) |
| Survived ✗ | Your test missed the mutation (weak test) |
| Error ⚠ | Mutation could not be applied/tested |
| Score | Percentage of killed mutations |
### Examples
:mutate
:mutate 50
:mutate 10
### Help
:mutate help — show this help
:help mutate — same as above`,
		Ru: `## :mutate — Мутационное тестирование
### Синтаксис
:mutate [лимит]
### Описание
Детерминированно генерирует мутации кода и проверяет, ловят ли их ваши тесты.
LLM не используется — мутации генерируются заменой операторов.
### Операторы мутаций
| Тип | Оригинал | Мутация | Пример |
|-----|----------|---------|--------|
| Отношения | >= | > | if x >= 10 → if x > 10 |
| Отношения | <= | < | if x <= 5 → if x < 5 |
| Логика | && | \|\| | if a && b → if a \|\| b |
| Логика | \|\| | && | if a \|\| b → if a && b |
| Равенство | == | != | if a == b → if a != b |
| Равенство | != | == | if a != b → if a == b |
### Интерпретация отчёта
| Статус | Значение |
|--------|----------|
| Убита ✓ | Ваш тест обнаружил мутацию (хорошо) |
| Выжила ✗ | Ваш тест пропустил мутацию (слабый тест) |
| Ошибка ⚠ | Мутация не могла быть применена/протестирована |
| Оценка | Процент убитых мутаций |
### Примеры
:mutate
:mutate 50
:mutate 10
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
suggestions organized into 5 categories.
### Output Sections
| Section | Icon | Description |
|---------|------|-------------|
| Critical | 🔴 | Bugs, security vulnerabilities, data loss risks |
| Tech Debt | 🟡 | Temporary solutions, duplicated code |
| Missing Tests | 🧪 | Exported functions without test coverage |
| Code Smells | 🧹 | Style issues, naming problems, unused code |
| Improvements | 💡 | Performance, architecture improvements |
### Behavior
| Rule | Detail |
|------|--------|
| Specificity | Each item references a specific file and function/line |
| No repeats | Suggestions from previous sessions are not repeated |
| Limit | Maximum 5 items per category |
| Language | Matches project language (ru/en) |
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
предложения по улучшению, организованные в 5 категорий.
### Секции вывода
| Секция | Иконка | Описание |
|--------|--------|----------|
| Критические | 🔴 | Баги, уязвимости, риски потери данных |
| Технический долг | 🟡 | Временные решения, дублирование кода |
| Отсутствующие тесты | 🧪 | Экспортированные функции без покрытия |
| Запахи кода | 🧹 | Проблемы стиля, именования, неиспользуемый код |
| Улучшения | 💡 | Производительность, архитектура |
### Поведение
| Правило | Детали |
|---------|--------|
| Конкретность | Каждый пункт ссылается на конкретный файл и функцию/строку |
| Без повторов | Предложения из предыдущих сессий не повторяются |
| Лимит | Максимум 5 пунктов в каждой категории |
| Язык | Соответствует языку проекта (ru/en) |
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
:journal
### Description
Shows the project decision journal with LLM analysis of "decision debt".
The journal records important engineering decisions made during
multi-agent sessions.
### Journal Contents
| Field | Description |
|-------|-------------|
| Decision | The decision that was made |
| Context | Task context when decision was made |
| Alternatives | Considered and rejected alternatives |
| Constraint | What forced this decision |
| Temporary | Whether the decision is temporary |
| Source | Where the decision came from (planner, user, etc.) |
### Decision Debt Analysis
LLM analyzes the journal to find temporary decisions whose original
constraints may no longer apply, suggesting what to revisit.
### Output Sections
| Section | Description |
|---------|-------------|
| Timeline | Chronological list of decisions |
| Decision Debt | Temporary decisions to revisit |
| Patterns | Observed decision-making patterns |
| Risks | Risks from accumulated debt |
### Examples
:decisions
:journal
### Help
:decisions help — show this help
:help decisions — same as above`,
		Ru: `## :decisions — Журнал решений
### Синтаксис
:decisions
:journal
### Описание
Показывает журнал инженерных решений проекта с LLM-анализом «долга решений».
Журнал записывает важные инженерные решения, принятые во время
мультиагентных сессий.
### Содержимое журнала
| Поле | Описание |
|------|----------|
| Решение | Принятое решение |
| Контекст | Контекст задачи при принятии решения |
| Альтернативы | Рассмотренные и отклонённые альтернативы |
| Ограничение | Что вынудило принять это решение |
| Временное | Является ли решение временным |
| Источник | Откуда пришло решение (планировщик, пользователь и т.д.) |
### Анализ долга решений
LLM анализирует журнал, чтобы найти временные решения, ограничения
которых могли перестать быть актуальными, и предлагает, что пересмотреть.
### Секции вывода
| Секция | Описание |
|--------|----------|
| Хронология | Хронологический список решений |
| Долг решений | Временные решения для пересмотра |
| Паттерны | Наблюдаемые паттерны принятия решений |
| Риски | Риски от накопленного долга |
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
### Pipeline
| Step | Description |
|------|-------------|
| 1 | Rewrite query into optimized search query via LLM |
| 2 | Perform web search (DuckDuckGo) |
| 3 | Sanitize results (prompt injection protection) |
| 4 | LLM summarizes results into coherent answer |
| 5 | Sources listed at end of response |
### Safety
| Protection | Detail |
|------------|--------|
| Rate limiting | Max 3 searches per session |
| Domain filter | Whitelist of allowed domains |
| SSRF protection | Blocks internal/private IPs |
| Secret detection | Redacts potential secrets from queries |
| Injection protection | Sanitizes retrieved content |
### Examples
:search latest Go version features
:search best practices for Go error handling
:search golang context package usage
### Help
:search help — show this help
:help search — same as above`,
		Ru: `## :search — Веб-поиск
### Синтаксис
:search <запрос>
### Описание
Выполняет веб-поиск и резюмирует результаты с помощью LLM.
### Конвейер
| Шаг | Описание |
|-----|----------|
| 1 | Переписывание запроса в оптимизированный поисковый запрос через LLM |
| 2 | Выполнение веб-поиска (DuckDuckGo) |
| 3 | Санитизация результатов (защита от prompt injection) |
| 4 | LLM резюмирует результаты в связный ответ |
| 5 | Источники перечисляются в конце ответа |
### Безопасность
| Защита | Детали |
|--------|--------|
| Ограничение частоты | Макс. 3 поиска за сессию |
| Фильтр доменов | Белый список разрешённых доменов |
| Защита от SSRF | Блокировка внутренних/приватных IP |
| Обнаружение секретов | Удаление потенциальных секретов из запросов |
| Защита от инъекций | Санитизация полученного содержимого |
### Примеры
:search последние возможности Go
:search лучшие практики обработки ошибок в Go
:search golang context package usage
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
### Comparison with :test lint
| Feature | :vet | :test lint |
|---------|------|------------|
| Tool | go vet | golangci-lint |
| Speed | Fast | Slower |
| LLM required | No | Yes (for auto-fix) |
| Auto-fix | No | Yes |
| Config | None needed | .golangci.yml (auto-created) |
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
### Сравнение с :test lint
| Возможность | :vet | :test lint |
|-------------|------|------------|
| Инструмент | go vet | golangci-lint |
| Скорость | Быстро | Медленнее |
| Нужен LLM | Нет | Да (для автоисправления) |
| Автоисправление | Нет | Да |
| Конфиг | Не нужен | .golangci.yml (создаётся автоматически) |
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
| Rule | Detail |
|------|--------|
| Files scanned | All .go files in project |
| Search scope | Comments only (not in code) |
| Max items | 50 |
| Output format | file:line [MARKER] text |
| Directories skipped | .git, .gogitor, node_modules, vendor |
### Markers
| Marker | Typical meaning |
|--------|-----------------|
| TODO | Planned work |
| FIXME | Known bug to fix |
| HACK | Workaround to improve |
| XXX | Dangerous/unclear code |
| BUG | Known bug |
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
| Правило | Детали |
|---------|--------|
| Сканируемые файлы | Все .go файлы проекта |
| Область поиска | Только комментарии (не в коде) |
| Макс. элементов | 50 |
| Формат вывода | файл:строка [МАРКЕР] текст |
| Пропускаемые директории | .git, .gogitor, node_modules, vendor |
### Маркеры
| Маркер | Типичное значение |
|--------|-------------------|
| TODO | Запланированная работа |
| FIXME | Известный баг для исправления |
| HACK | Обходное решение для улучшения |
| XXX | Опасный/неясный код |
| BUG | Известный баг |
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
:analyze <question> --image <path>
### Description
Analyzes the project code without modifying files. Uses the project
index to select the most relevant files for analysis.
### Comparison with other commands
| Command | Modifies files | Project context | Use case |
|---------|---------------|-----------------|----------|
| :analyze | No | Yes (smart selection) | Understand code, find bugs |
| :ask | No | No (general chat) | General questions |
| :code | Yes | Yes (smart selection) | Create/modify code |
### Image Analysis
Use --image flag (CLI) or attach image path in the query (TUI):
gogitor analyze "explain this diagram" --image diagram.png
### Examples
:analyze find potential bugs in this project
:analyze explain the authentication flow
:analyze what does the index package do?
### Help
:analyze help — show this help
:help analyze — same as above`,
		Ru: `## :analyze — Анализ кода
### Синтаксис
:analyze <вопрос>
:analyze <вопрос> --image <путь>
### Описание
Анализирует код проекта без изменения файлов. Использует индекс
проекта для выбора наиболее релевантных файлов для анализа.
### Сравнение с другими командами
| Команда | Изменяет файлы | Контекст проекта | Когда использовать |
|---------|---------------|------------------|-------------------|
| :analyze | Нет | Да (умный выбор) | Понять код, найти баги |
| :ask | Нет | Нет (общий чат) | Общие вопросы |
| :code | Да | Да (умный выбор) | Создать/изменить код |
### Анализ изображений
Используйте флаг --image (CLI) или укажите путь к изображению (TUI):
gogitor analyze "объясни эту диаграмму" --image diagram.png
### Примеры
:analyze найди потенциальные баги в проекте
:analyze объясни поток аутентификации
:analyze что делает пакет index?
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
:ask <question> --image <path>
### Description
General chat mode. Answers questions about Go development,
provides advice, explains concepts. Does NOT modify files.
### Comparison with :analyze
| Feature | :ask | :analyze |
|---------|------|----------|
| Project context | No | Yes |
| Conversation history | Yes | No |
| Modifies files | No | No |
| Best for | General questions | Code-specific analysis |
### Image Analysis
Use --image flag (CLI) or attach image path in the query (TUI):
gogitor ask "what is on this image?" --image screenshot.png
### Examples
:ask explain context.Context
:ask what is the difference between goroutine and thread?
:ask how to handle errors idiomatically in Go?
### Help
:ask help — show this help
:help ask — same as above`,
		Ru: `## :ask — Режим чата
### Синтаксис
:ask <вопрос>
:ask <вопрос> --image <путь>
### Описание
Общий режим чата. Отвечает на вопросы о разработке на Go,
даёт советы, объясняет концепции. НЕ изменяет файлы.
### Сравнение с :analyze
| Возможность | :ask | :analyze |
|-------------|------|----------|
| Контекст проекта | Нет | Да |
| История диалога | Да | Нет |
| Изменяет файлы | Нет | Нет |
| Лучше для | Общие вопросы | Анализ конкретного кода |
### Анализ изображений
Используйте флаг --image (CLI) или укажите путь к изображению (TUI):
gogitor ask "что на этом изображении?" --image screenshot.png
### Примеры
:ask объясни context.Context
:ask в чём разница между горутиной и потоком?
:ask как идиоматично обрабатывать ошибки в Go?
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
### Pipeline
| Step | Description |
|------|-------------|
| 1 | AST scan for exported functions without tests (no LLM) |
| 2 | For each function, send focused prompt to LLM |
| 3 | Write generated test to _test.go file |
| 4 | Run tests immediately |
| 5 | If tests fail → delete file; if pass → keep file |
### Generated Test Format
| Feature | Detail |
|---------|--------|
| Style | Table-driven with t.Run subtests |
| Cases | At least 3: happy path, boundary, error/edge |
| Imports | Only standard library "testing" |
| Naming | Test<FunctionName> |
### Examples
:autogen-tests
:autogen-tests 3
:autogen-tests 10
### Help
:autogen-tests help — show this help
:help autogen-tests — same as above`,
		Ru: `## :autogen-tests — Автогенерация тестов
### Синтаксис
:autogen-tests [количество]
### Описание
Сканирует AST проекта в поисках экспортированных функций без
соответствующего файла _test.go, затем генерирует табличные тесты через LLM.
### Конвейер
| Шаг | Описание |
|-----|----------|
| 1 | Сканирование AST на функции без тестов (без LLM) |
| 2 | Для каждой функции — сфокусированный промпт в LLM |
| 3 | Запись сгенерированного теста в файл _test.go |
| 4 | Немедленный запуск тестов |
| 5 | Если тесты падают → файл удаляется; если проходят → остаётся |
### Формат генерируемых тестов
| Характеристика | Детали |
|----------------|--------|
| Стиль | Табличные с подтестами t.Run |
| Случаи | Минимум 3: нормальный, граничный, ошибка/крайний |
| Импорты | Только стандартная библиотека "testing" |
| Именование | Test<ИмяФункции> |
### Примеры
:autogen-tests
:autogen-tests 3
:autogen-tests 10
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
### Subcommands
| Command | Description |
|---------|-------------|
| :reasoning | Show current reasoning state |
| :reasoning on | Enable reasoning mode |
| :reasoning off | Disable reasoning mode |
| :reasoning router on | Enable reasoning for intent router |
| :reasoning router off | Disable reasoning for intent router |
### Supported Models
| Provider | Mechanism |
|----------|-----------|
| Ollama | "think": true parameter |
| OpenAI-compatible | "reasoning_effort" parameter |
### Environment Variables
| Variable | Description |
|----------|-------------|
| GOGITOR_REASONING=true | Enable reasoning |
| GOGITOR_REASONING_EFFORT=low\|medium\|high | Reasoning depth |
| GOGITOR_REASONING_BUDGET=<tokens> | Max reasoning tokens |
### Examples
:reasoning
:reasoning on
:reasoning off
:reasoning router on
### Help
:reasoning help — show this help
:help reasoning — same as above`,
		Ru: `## :reasoning — Режим размышления
### Синтаксис
:reasoning [on|off|router [on|off]]
### Подкоманды
| Команда | Описание |
|---------|----------|
| :reasoning | Показать текущее состояние |
| :reasoning on | Включить режим размышления |
| :reasoning off | Выключить режим размышления |
| :reasoning router on | Включить размышления для роутера |
| :reasoning router off | Выключить размышления для роутера |
### Поддерживаемые модели
| Провайдер | Механизм |
|-----------|----------|
| Ollama | Параметр "think": true |
| OpenAI-compatible | Параметр "reasoning_effort" |
### Переменные окружения
| Переменная | Описание |
|------------|----------|
| GOGITOR_REASONING=true | Включить размышление |
| GOGITOR_REASONING_EFFORT=low\|medium\|high | Глубина размышления |
| GOGITOR_REASONING_BUDGET=<токены> | Макс. токенов размышления |
### Примеры
:reasoning
:reasoning on
:reasoning off
:reasoning router on
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
### Subcommands
| Command | Description |
|---------|-------------|
| :test | Run go test -v -cover ./... in sandbox |
| :test lint | Run golangci-lint and auto-fix issues via LLM |
### Test Output
| Field | Description |
|-------|-------------|
| Passed | Number of passed tests |
| Failed | Number of failed tests |
| Coverage | Average coverage percentage |
| Failures | Detailed failure info (test, function, file, line) |
### Lint Behavior
| Step | Description |
|------|-------------|
| 1 | Run golangci-lint in sandbox |
| 2 | Count issues from output |
| 3 | If issues found → send to LLM for fixing |
| 4 | Apply fixes and re-validate |
### Related Commands
| Command | Description |
|---------|-------------|
| :vet | Run go vet (fast, no LLM) |
| :mutate [limit] | Mutation testing |
| :autogen-tests [n] | Auto-generate unit tests |
### Examples
:test
:test lint
### Help
:test help — show this help
:help test — same as above`,
		Ru: `## :test — Тестирование
### Синтаксис
:test
:test lint
### Подкоманды
| Команда | Описание |
|---------|----------|
| :test | Запуск go test -v -cover ./... в песочнице |
| :test lint | Запуск golangci-lint и автоисправление через LLM |
### Вывод тестов
| Поле | Описание |
|------|----------|
| Пройдено | Количество пройденных тестов |
| Упало | Количество упавших тестов |
| Покрытие | Средний процент покрытия |
| Падения | Детальная информация (тест, функция, файл, строка) |
### Поведение lint
| Шаг | Описание |
|-----|----------|
| 1 | Запуск golangci-lint в песочнице |
| 2 | Подсчёт проблем из вывода |
| 3 | Если есть проблемы → отправка в LLM для исправления |
| 4 | Применение исправлений и повторная проверка |
### Связанные команды
| Команда | Описание |
|---------|----------|
| :vet | Запуск go vet (быстро, без LLM) |
| :mutate [лимит] | Мутационное тестирование |
| :autogen-tests [n] | Автогенерация юнит-тестов |
### Примеры
:test
:test lint
### Помощь
:test help — показать эту помощь
:help test — то же самое`,
	},
	{
		Name:    "code",
		Aliases: []string{":code", "code"},
		En: `## :code — Code Generation and Modification
### Syntax
:code <task>
### Description
Creates or modifies Go code based on the task description.
Uses the project index for relevant context selection.
Validates changes in a sandbox before applying.
### Execution Modes
| Mode | Trigger | Description |
|------|---------|-------------|
| auto | Default | Gogitor selects the best strategy |
| simple/fast | :fast or --mode fast | Single-pass generation |
| agent | :agent or --mode agent | Full 4-stage pipeline |
### Patch vs Full File
| Condition | Output format |
|-----------|---------------|
| Existing project + modify task | SEARCH/REPLACE patches |
| New file creation | Full file content |
| Patch fails | Fallback to full file |
### Validation Pipeline
| Step | Tool |
|------|------|
| 1 | go mod init / go mod tidy |
| 2 | gofmt |
| 3 | go build |
| 4 | go test -v -cover |
### CLI Flags
| Flag | Description |
|------|-------------|
| --mode <auto\|simple\|agent> | Execution mode |
| --agent | Force agent mode |
| --deep | Force deep agent profile |
| --dry-run | Validate without applying |
| --no-commit | Disable auto git commit |
| --no-tests | Skip tests |
| --no-compare | Skip approach comparison |
| --json | JSON output |
### Examples
:code create a REST API with /health endpoint
:code refactor the authentication module
:code add error logging to the middleware
### Help
:code help — show this help
:help code — same as above`,
		Ru: `## :code — Генерация и изменение кода
### Синтаксис
:code <задача>
### Описание
Создаёт или изменяет Go-код на основе описания задачи.
Использует индекс проекта для выбора релевантного контекста.
Проверяет изменения в песочнице перед применением.
### Режимы выполнения
| Режим | Активация | Описание |
|-------|-----------|----------|
| auto | По умолчанию | Gogitor выбирает лучшую стратегию |
| simple/fast | :fast или --mode fast | Однопроходная генерация |
| agent | :agent или --mode agent | Полный 4-этапный конвейер |
### Патч vs Полный файл
| Условие | Формат вывода |
|---------|---------------|
| Существующий проект + изменение | Патчи SEARCH/REPLACE |
| Создание нового файла | Полное содержимое файла |
| Патч не сработал | Откат к полному файлу |
### Конвейер валидации
| Шаг | Инструмент |
|-----|------------|
| 1 | go mod init / go mod tidy |
| 2 | gofmt |
| 3 | go build |
| 4 | go test -v -cover |
### Флаги CLI
| Флаг | Описание |
|------|----------|
| --mode <auto\|simple\|agent> | Режим выполнения |
| --agent | Принудительный агентный режим |
| --deep | Принудительный глубокий профиль |
| --dry-run | Проверка без применения |
| --no-commit | Отключить автокоммит |
| --no-tests | Пропустить тесты |
| --no-compare | Пропустить сравнение подходов |
| --json | Вывод JSON |
### Примеры
:code создать REST API с эндпоинтом /health
:code отрефакторить модуль аутентификации
:code добавить логирование ошибок в middleware
### Помощь
:code help — показать эту помощь
:help code — то же самое`,
	},
	{
		Name:    "fast",
		Aliases: []string{":fast", "fast"},
		En: `## :fast — Quick Code Generation
### Syntax
:fast <task>
### Description
Forces simple single-pass execution mode regardless of task complexity.
Skips the multi-agent pipeline (planner, reviewer, verifier).
Best for small, well-defined, low-risk changes.
### Comparison with other modes
| Command | Pipeline | Best for |
|---------|----------|----------|
| :code | Auto-selects | Default choice |
| :fast | Single pass only | Quick small fixes |
| :agent | Full 4-stage | Complex multi-file tasks |
### What is skipped
| Skipped | Reason |
|---------|--------|
| Planner | No task decomposition |
| Reviewer | No code review |
| Verifier | No goal verification |
| Approach comparison | No alternative analysis |
### What is preserved
| Preserved | Detail |
|-----------|--------|
| Sandbox validation | go build + go test still run |
| Patch engine | SEARCH/REPLACE still used |
| Git commit | Auto-commit if enabled |
### Examples
:fast rename the function processInput to handleInput
:fast add error logging to the middleware
:fast add a String() method to the Config struct
### Help
:fast help — show this help
:help fast — same as above`,
		Ru: `## :fast — Быстрая генерация кода
### Синтаксис
:fast <задача>
### Описание
Принудительно запускает простой однопроходный режим независимо от сложности.
Пропускает мультиагентный конвейер (планировщик, ревьюер, верификатор).
Лучше всего подходит для небольших, чётко определённых изменений.
### Сравнение с другими режимами
| Команда | Конвейер | Лучше для |
|---------|----------|-----------|
| :code | Автовыбор | Выбор по умолчанию |
| :fast | Однопроходный | Быстрые мелкие правки |
| :agent | Полный 4-этапный | Сложные многофайловые задачи |
### Что пропускается
| Пропускается | Причина |
|--------------|----------|
| Планировщик | Нет декомпозиции задачи |
| Ревьюер | Нет ревью кода |
| Верификатор | Нет проверки цели |
| Сравнение подходов | Нет анализа альтернатив |
### Что сохраняется
| Сохраняется | Детали |
|-------------|--------|
| Валидация в песочнице | go build + go test всё ещё выполняются |
| Патч-движок | SEARCH/REPLACE всё ещё используется |
| Git-коммит | Автокоммит, если включён |
### Примеры
:fast переименуй функцию processInput в handleInput
:fast добавь логирование ошибок в middleware
:fast добавь метод String() к структуре Config
### Помощь
:fast help — показать эту помощь
:help fast — то же самое`,
	},
	{
		Name:    "run",
		Aliases: []string{":run", "run"},
		En: `## :run — Execute Go Program
### Syntax
:run [file]
### Description
Runs the Go project (or a specific file directory) in a sandbox.
Uses 'go run .' on the target directory.
The sandbox is a temporary copy; the real project is not modified.
### Pipeline
| Step | Description |
|------|-------------|
| 1 | Copy project to temporary sandbox |
| 2 | go mod init / go mod tidy if needed |
| 3 | gofmt formatting |
| 4 | Check for 'package main' with func main() |
| 5 | Execute 'go run .' |
### Requirements
| Requirement | Detail |
|-------------|--------|
| package main | Must exist in target directory |
| func main() | Must be present |
| Go files | At least one .go file required |
### Examples
:run
:run main.go
:run cmd/server/main.go
### Help
:run help — show this help
:help run — same as above`,
		Ru: `## :run — Запуск Go-программы
### Синтаксис
:run [файл]
### Описание
Запускает Go-проект (или конкретный файл) в песочнице.
Использует 'go run .' для целевой директории.
Песочница — временная копия; реальный проект не изменяется.
### Конвейер
| Шаг | Описание |
|-----|----------|
| 1 | Копирование проекта во временную песочницу |
| 2 | go mod init / go mod tidy при необходимости |
| 3 | Форматирование gofmt |
| 4 | Проверка наличия 'package main' с func main() |
| 5 | Выполнение 'go run .' |
### Требования
| Требование | Детали |
|------------|--------|
| package main | Должен существовать в целевой директории |
| func main() | Должна присутствовать |
| Go-файлы | Минимум один .go файл |
### Примеры
:run
:run main.go
:run cmd/server/main.go
### Помощь
:run help — показать эту помощь
:help run — то же самое`,
	},
	{
		Name:    "clear",
		Aliases: []string{":clear", "clear"},
		En: `## :clear — Clear Conversation Context
### Syntax
:clear
### Description
Clears the in-memory conversation history used for chat
and analysis context. Does not affect project files,
Git history, or saved artifacts.
### What is cleared
| Cleared | Detail |
|---------|--------|
| Chat history | In-memory conversation for :ask |
| Analysis context | Previous :analyze context |
### What is NOT cleared
| Preserved | Detail |
|-----------|--------|
| Project files | No file changes |
| Git history | No git operations |
| Agent memory | .gogitor/agent_memory.json untouched |
| Decision journal | .gogitor/decisions.json untouched |
### Related Commands
| Command | Description |
|---------|-------------|
| :cls | Clear visual screen (TUI only) |
### Examples
:clear
### Help
:clear help — show this help
:help clear — same as above`,
		Ru: `## :clear — Очистка контекста диалога
### Синтаксис
:clear
### Описание
Очищает историю диалога в памяти, используемую для чата
и анализа. Не влияет на файлы проекта, историю Git
или сохранённые артефакты.
### Что очищается
| Очищается | Детали |
|-----------|--------|
| История чата | Диалог в памяти для :ask |
| Контекст анализа | Предыдущий контекст :analyze |
### Что НЕ очищается
| Сохраняется | Детали |
|-------------|--------|
| Файлы проекта | Никаких изменений файлов |
| История Git | Никаких git-операций |
| Память агента | .gogitor/agent_memory.json не изменяется |
| Журнал решений | .gogitor/decisions.json не изменяется |
### Связанные команды
| Команда | Описание |
|---------|----------|
| :cls | Очистка визуального экрана (только TUI) |
### Примеры
:clear
### Помощь
:clear help — показать эту помощь
:help clear — то же самое`,
	},
	{
		Name:    "history",
		Aliases: []string{":history", "history"},
		En: `## :history — Task Execution History
### Syntax
:history
### Description
Shows the recent task execution history stored in
.gogitor/task_history.json. Displays up to the last 20 entries
with status, mode, file count, added/removed lines, and commit hash.
### Output Fields
| Field | Description |
|-------|-------------|
| Status | ✓ success / ✗ failure |
| ID | Sequential task number |
| Time | Execution timestamp |
| Query | Task description (truncated to 80 chars) |
| Mode | Execution mode (code, agent, git, etc.) |
| Files | Number of files affected |
| Lines | Added/removed line counts |
| Commit | Git commit hash if applicable |
### Storage
| Detail | Value |
|--------|-------|
| Location | .gogitor/task_history.json |
| Max entries | 100 |
| Format | JSON array |
### Examples
:history
### Help
:history help — show this help
:help history — same as above`,
		Ru: `## :history — История выполнения задач
### Синтаксис
:history
### Описание
Показывает историю выполнения задач, хранящуюся в
.gogitor/task_history.json. Выводит до 20 последних записей
со статусом, режимом, числом файлов, строками и хешем коммита.
### Поля вывода
| Поле | Описание |
|------|----------|
| Статус | ✓ успех / ✗ ошибка |
| ID | Порядковый номер задачи |
| Время | Временная метка выполнения |
| Запрос | Описание задачи (обрезано до 80 символов) |
| Режим | Режим выполнения (code, agent, git и т.д.) |
| Файлы | Количество затронутых файлов |
| Строки | Количество добавленных/удалённых строк |
| Коммит | Хеш git-коммита, если применимо |
### Хранение
| Деталь | Значение |
|--------|----------|
| Расположение | .gogitor/task_history.json |
| Макс. записей | 100 |
| Формат | Массив JSON |
### Примеры
:history
### Помощь
:history help — показать эту помощь
:help history — то же самое`,
	},
	{
		Name:    "task-diff",
		Aliases: []string{":task-diff", "task-diff"},
		En: `## :task-diff — Last Task Diff
### Syntax
:task-diff
### Description
Shows the cumulative Git diff produced by the last completed task.
This includes all iterations and subtask changes within that task.
Requires a previous code task to have been executed.
### Behavior
| Condition | Action |
|-----------|--------|
| Pre-task HEAD captured | Shows diff from that HEAD to current |
| No pre-task HEAD | Falls back to 'git diff' (working directory) |
| No diff at all | Falls back to 'git show --stat HEAD' |
| Output too large | Truncated to 100,000 bytes |
### Related Commands
| Command | Description |
|---------|-------------|
| :git diff | Show current working directory diff |
| :git diff-task | Same as :task-diff (alias) |
### Examples
:task-diff
### Help
:task-diff help — show this help
:help task-diff — same as above`,
		Ru: `## :task-diff — Diff последней задачи
### Синтаксис
:task-diff
### Описание
Показывает накопительный Git diff, созданный последней завершённой задачей.
Включает все итерации и изменения подзадач в рамках этой задачи.
Требует, чтобы ранее была выполнена задача генерации кода.
### Поведение
| Условие | Действие |
|---------|----------|
| Зафиксирован HEAD до задачи | Показывает diff от него до текущего |
| Нет HEAD до задачи | Использует 'git diff' (рабочая директория) |
| Нет diff вообще | Использует 'git show --stat HEAD' |
| Вывод слишком большой | Обрезается до 100 000 байт |
### Связанные команды
| Команда | Описание |
|---------|----------|
| :git diff | Показать diff рабочей директории |
| :git diff-task | То же самое, что :task-diff (алиас) |
### Примеры
:task-diff
### Помощь
:task-diff help — показать эту помощь
:help task-diff — то же самое`,
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
