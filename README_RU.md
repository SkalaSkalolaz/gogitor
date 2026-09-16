# Gogitor 2.1.13

AI-ассистент разработчика в терминале для проектов на Go.

Gogitor — **TUI-only приложение** с единственным фиксированным интерфейсом **Zen**. Создание и изменение кода выполняются через **Agent pipeline**, а анализ, тестирование, Git/GitHub, web research, управление компьютером, автономный мониторинг и диагностика используют общий service/event pipeline.

[English README](README.md)

## Обзор

Gogitor предназначен для непосредственной работы внутри существующего Go-проекта.

Основные возможности:

* создание и изменение Go-кода;
* анализ проекта без изменения файлов;
* точечное применение DIFF/PATCH к существующему коду;
* запуск сборки, тестов, coverage, `go vet` и `golangci-lint`;
* автоматическое восстановление внешних Go-зависимостей;
* автоматический web research для задач, где нужны актуальные внешние технические сведения;
* планирование, реализация, ревью и верификация крупных изменений через Agent;
* интеграция с Git и GitHub;
* генерация тестов и мутационное тестирование;
* автономный мониторинг проекта;
* опциональное выполнение контролируемых системных команд;
* хранение Agent-сессий, решений, диагностики и результатов quality gates в `.gogitor/`.

В настоящее время зарегистрирован только один язык и соответствующий toolchain:

```text
Go → .go
```

## Архитектура

```text
cmd/gogitor
      │
      ▼
   ui/tui
      │
      ▼
    app
      │
      ├── domain       события, результаты, контракты
      ├── language     реестр языков
      ├── workspace    DIFF/PATCH и состояние рабочей области
      ├── agent        Agent dispatcher
      ├── runner       Go build/test/run/lint/vet
      ├── index        AST и релевантность файлов
      ├── git/github   Git и GitHub
      ├── search       web search
      ├── computer     контролируемое выполнение команд
      └── config/llm   конфигурация и LLM-инфраструктура
```

TUI отвечает за ввод и отображение событий и не содержит логики непосредственного выполнения задач. Команды передаются в application/service layer.

Языковая поддержка изолирована в `internal/language`. Сейчас зарегистрирован:

```text
Go → .go
```

Добавление следующего языка предусматривает отдельную регистрацию языка и toolchain, а не переписывание TUI и командного роутера.

## Agent-first выполнение кода

Для создания и изменения кода Gogitor использует Agent как основной pipeline.

```text
Planner → Coder → Reviewer → Verifier
```

`:code` является точкой входа в Agent, а не отдельным быстрым режимом выполнения.

### Глубина Agent

По умолчанию используется адаптивный режим:

```text
auto → normal или deep
```

Выбор зависит от сложности задачи и профиля модели.

Обычный запуск:

```text
:code <задача>
:agent <задача>
```

Усиленный профиль:

```text
:agent deep <задача>
```

Для обратной совместимости также принимается:

```text
:agent enhanced <задача>
```

### Этапы Agent

| Этап | Роль     | Назначение                                                                |
| ---- | -------- | ------------------------------------------------------------------------- |
| 1    | Planner  | Разбивка задачи на подзадачи с критериями приемки                         |
| 2    | Coder    | Реализация подзадач                                                       |
| 3    | Reviewer | Проверка компиляции, регрессий и важных проблем корректности/безопасности |
| 4    | Verifier | Проверка достижения исходной цели                                         |

Для сложных задач Agent может выполнять несколько подзадач с сохранением состояния всей сессии.

### Артефакты Agent-сессии

Сессии сохраняются в:

```text
.gogitor/agent/<timestamp>/
```

Внутри могут находиться:

```text
inbox.md
research.md
plan.md
plan.json
process.md
result.json
state.json
gate-final.json
gate-task-XX.json
```

Благодаря этому после выполнения доступны план, ход работы, состояние, результаты и сведения о quality gates.

### Команды Agent

```text
:agent <задача>
:agent deep <задача>
:agent interview <задача>
:agent reflect
:agent report
:agent resume
:agent undo
```

`interview` формирует уточняющие вопросы перед выполнением.

`reflect` анализирует последнюю Agent-сессию и извлекает уроки.

`report` показывает отчёт последней Agent-сессии.

`resume` продолжает последнюю доступную для продолжения сессию.

`undo` безопасно отменяет последний завершённый Agent commit.

## DIFF/PATCH safety

При изменении существующего проекта Gogitor предпочитает структурированные изменения вместо безусловной полной перегенерации файлов.

Подсистема поддерживает:

* точный SEARCH/REPLACE;
* `REPLACE_ONLY`;
* symbol anchors;
* несколько политик сопоставления;
* fuzzy matching с порогом уверенности;
* восстановление patch после отказа;
* DIFF trace;
* диагностику области изменений;
* patch audit;
* обнаружение no-op patch;
* применение в sandbox;
* rollback и контроль целостности рабочей области.

Для существующих файлов по умолчанию используется **PATCH**.

Полная перезапись файла возможна при явном указании в задаче.

### Протоколы patch

Поддерживаются:

```text
auto
search_replace
replace_only
```

`REPLACE_ONLY` является наиболее строгим протоколом: при исчерпании безопасного восстановления запрещён переход к полной перезаписи файла.

### Аудит patch

Параметр:

```text
auto
off
always
```

В режиме `auto` аудит используется для более строгих и рискованных сценариев patching.

### Диагностика DIFF

Включить:

```text
:diff-trace on
```

Проверить состояние:

```text
:diff-trace status
```

Выключить:

```text
:diff-trace off
```

Трассировка может показывать этапы:

```text
PARSE
SOURCE
SYMBOL
EXACT
RELAXED
NORMALIZED
REBASE
FUZZY
APPLY
PREFLIGHT
SANDBOX_APPLY
BUILD
TARGETED_TESTS
FULL_TESTS
ROOT_APPLY
```

Статусы этапов:

```text
OK
MISS
REJECT
SKIP
RUN
```

## Проверки и quality gates

Gogitor использует стандартный Go toolchain и интегрирует его проверки в pipeline.

### Сборка

```bash
go build ./...
```

### Тесты

Основной запуск:

```bash
go test -v -cover ./...
```

Gogitor анализирует:

* количество пройденных тестов;
* количество ошибок;
* coverage;
* местоположение тестовых ошибок;
* сообщения об ошибках.

Для Agent также поддерживается запуск тестов изменённых пакетов.

### go vet

Команда:

```text
:vet
```

выполняет:

```bash
go vet ./...
```

### Линтер

Команда:

```text
:test lint
```

использует:

```bash
golangci-lint run ./...
```

При отсутствии `.golangci.yml` Gogitor может создать стандартную конфигурацию перед запуском линтера.

### Deep Agent quality gates

В усиленном профиле используются детерминированные проверки:

```text
gofmt
go build
go test
go vet
golangci-lint
```

Проверки выполняются в sandbox.

При нарушении quality gates текущая подзадача может быть откатана.

Финальный Git commit Agent откладывается до завершения всей сессии и прохождения финальной верификации.

## Работа с Go-зависимостями

При обнаружении внешних импортов Gogitor может автоматически выполнить:

```bash
go mod tidy
```

Режим задаётся параметром:

```text
auto
ask
never
```

В режиме `auto` разрешение зависимостей выполняется при наличии внешних импортов.

Если загрузка зависимости завершилась ошибкой Git/SSH, Gogitor может повторить попытку через:

```text
https://proxy.golang.org
```

Результат обработки зависимости сокращается до диагностически полезной информации и может быть использован в процессе восстановления ошибки.

## Автоматический web research

Флаг:

```text
--auto-search
```

включает автоматический web research для подходящих технических задач.

Классификатор учитывает, в частности:

```text
dependency
library
api
migration
version
toolchain
security
lint
architecture
performance
documentation
os
```

Запрос строится специально для текущей технической задачи.

Механизм используется не только для обычного research, но и для восстановления проблем внешних зависимостей.

### Важное замечание для удалённых LLM

При включённом `--auto-search` и использовании удалённого LLM Gogitor предупреждает, что код проекта и поисковые запросы могут отправляться на внешние серверы.

Для чувствительных проектов целесообразно использовать локальный Ollama, когда требуется не покидать пределы локальной машины.

## Zen TUI

Gogitor использует единственный интерфейс:

```text
Zen
```

Переключения TUI-профиля или смены интерфейса во время работы нет.

Запуск имеет вид:

```bash
gogitor [flags]
```

Позиционные аргументы после startup flags не принимаются.

### Клавиши

| Клавиша     | Действие                                             |
| ----------- | ---------------------------------------------------- |
| `Enter`     | Выполнить ввод                                       |
| `Alt+Enter` | Новая строка                                         |
| `Up/Down`   | Переход между строками ввода                         |
| `Tab`       | Переключение фокуса ввод/вывод                       |
| `PgUp/PgDn` | История команд                                       |
| `F2`        | Режим выделения мышью                                |
| `Ctrl+A`    | Копировать весь вывод                                |
| `Ctrl+C`    | Отменить текущую задачу / выйти в состоянии ожидания |
| `Ctrl+D`    | Выйти в состоянии ожидания                           |
| `Esc`       | Вернуть фокус на ввод                                |

## Команды TUI

### Общие команды

```text
:help
:help <тема>
:clear
:cls
:save <файл>
:reasoning
:reasoning on
:reasoning off
:diff-trace
:diff-trace on
:diff-trace off
:quit
:exit
:q
```

`:clear` и `:cls` очищают хранимый в памяти контекст разговора.

`:save <файл>` сохраняет последний результат. Поддерживаются форматы `.md`, `.txt`, `.go`, `.json`.

### Код и анализ

```text
:code <задача>
:fix <ошибка>
:ask <вопрос>
:analyze <задача>
:search <запрос>
:suggest
:load <файл>
```

Примеры:

```text
:code добавить endpoint /health
:fix panic: runtime error: index out of range
:analyze проверить пакет аутентификации
:search изменения context в Go 1.25
:load ./task.md
```

`:analyze` работает в режиме анализа и не должен изменять файлы проекта.

### Статьи

```text
:article <тема>
:article --full <тема>
```

Обычный режим формирует более короткую статью.

`--full` использует более сложную многочастную генерацию.

### Выполнение и тестирование

```text
:run [файл]
:test
:test lint
:test unusual
:vet
:todo
:check
:task-diff
```

`:test unusual` выполняет дополнительные проверки поведения на необычных входных данных.

`:todo` ищет в проекте маркеры:

```text
TODO
FIXME
HACK
BUG
```

`:task-diff` показывает накопительный Git diff последней завершённой задачи.

## Git и GitHub

Gogitor поддерживает локальные Git-операции и интеграцию с GitHub при наличии соответствующей конфигурации.

### Git-команды

```text
:git status
:git diff
:git diff-task
:git commit
:git commit --split <файл1,файл2>
:git init
:git log
:git checkout <ref>
:git checkout -b <имя>
:git branch
:git branch <имя>
:git branch -d <имя>
:git merge <ветка>
:git revert [hash]
:git reset [--hard] <hash>
:git push [ветка]
:git pull [ветка]
:git fetch
:git clone <url>
:git remote
:git remote add <имя> <url>
:git remote remove <имя>
:git create <имя>
:git pr
:git issue
:git changelog
:git pr-comment <номер> [текст]
```

Примеры:

```text
:git status
:git diff
:git commit
:git commit --split main.go,internal/app/app.go
:git push
:git pr
```

` :git reset --hard` является разрушительной операцией, поскольку изменяет состояние рабочего дерева и истории.

`:git pr` требует GitHub token.

`:git issue` может создать issue на основе информации о неудачных тестах.

`:git changelog` генерирует `CHANGELOG.md` на основе Conventional Commits.

### Автоматический commit

Флаг:

```text
--auto-commit
```

разрешает автоматически создавать Git commit после успешной Agent-реализации.

При этом сам Agent намеренно откладывает итоговый commit до успешного завершения финальной верификации.

### Автоматическая инициализация Git

Флаг:

```text
--git-auto-init
```

разрешает автоматически создать Git repository, когда это требуется workflow.

## Autonomy

Autonomy предоставляет фоновый мониторинг проекта и очередь исправляемых задач.

Включение:

```text
--autonomy
```

или:

```text
:autonomy on
```

Команды:

```text
:autonomy on
:autonomy off
:autonomy status
:autonomy run
:autonomy clear
```

Мониторинг может проверять:

* `go build`;
* `go vet`;
* TODO/FIXME/HACK.

Основные параметры:

```text
autonomy_enabled
autonomy_interval_sec
autonomy_mutation_limit
```

Autonomy по умолчанию выключен.

## Мутационное тестирование

Команда:

```text
:mutate
```

или:

```text
:mutate 50
```

работает детерминированно и **не использует LLM**.

Генерируются мутации исходного кода за счёт замены операторов, после чего проверяется, ловят ли их существующие тесты.

Типичные результаты:

```text
Killed
Survived
Error
```

Mutation score определяется по доле обнаруженных тестами мутаций.

## Автогенерация тестов

Команда:

```text
:autogen-tests [n]
```

используется для автоматической генерации unit-тестов для подходящих функций, которые ещё не покрыты тестами.

Сгенерированные тесты затем участвуют в обычном Go test workflow.

## Computer mode

Режим управления компьютером по умолчанию выключен.

Включить его можно:

```text
--computer
```

или:

```text
GOGITOR_COMPUTER_ENABLED=true
```

Также поддерживается настройка через `.gogitor.json`.

Команда:

```text
:computer <задача>
```

Примеры:

```text
:computer показать использование диска
:computer найти самые большие файлы в текущем каталоге
:computer установить curl
```

Этот режим выполняет реальные команды операционной системы.

Перед выполнением команда проходит предварительную оценку безопасности. Запрещённые команды блокируются. Подстановка команд через `$()` или backticks запрещена. Команды типа `sudo`/`doas` требуют явного разрешения:

```text
--allow-sudo
```

Аудит выполняемых команд сохраняется в:

```text
.gogitor/computer_audit.json
```

## Reasoning

Режим reasoning управляется startup-параметрами:

```text
--reasoning
--reasoning-effort low|medium|high
--reasoning-budget <n>
--reasoning-show
--reasoning-router
```

Во время работы:

```text
:reasoning
:reasoning on
:reasoning off
:reasoning router
```

Reasoning изменяет поведение модели, но не создаёт отдельный execution pipeline.

## Конфигурация

Источники настроек имеют следующий приоритет:

```text
значения по умолчанию
        ↓
~/.gogitor/config.json
        ↓
.gogitor.json
        ↓
переменные окружения
        ↓
флаги запуска
```

Таким образом, startup flags имеют максимальный приоритет для текущего запуска.

### Примеры запуска

Локальный Ollama:

```bash
./gogitor \
  --provider ollama \
  --model gpt-oss:20b \
  --repo ~/Code/myapp
```

OpenAI-compatible endpoint:

```bash
./gogitor \
  --provider 'openai-compatible+http://localhost:8000/v1' \
  --model my-model
```

Удалённый совместимый endpoint:

```bash
./gogitor \
  --provider 'openai+https://api.example.com/v1' \
  --model my-model \
  --key '...'
```

### Формы provider

Поддерживаются формы:

```text
ollama
openai+URL
openai-compatible+URL
HTTP(S) URL
```

Например:

```text
--provider ollama
--provider openai+https://api.openai.com/v1
--provider openai-compatible+http://localhost:8000/v1
```

### Переменные окружения

Основные переменные:

```text
GOGITOR_PROVIDER
GOGITOR_MODEL
GOGITOR_OLLAMA_URL
GOGITOR_API_KEY
OPENAI_API_KEY
GOGITOR_GITHUB_TOKEN
GITHUB_TOKEN
GOGITOR_COMPUTER_ENABLED
```

Для секретов предпочтительнее использовать переменные окружения или конфигурационный файл, а не помещать токены непосредственно в командную строку.

## Startup-параметры

Полный текущий список:

```bash
./gogitor --help
```

### Основные

| Параметр           | Назначение                |
| ------------------ | ------------------------- |
| `-p`, `--provider` | LLM provider/endpoint     |
| `-m`, `--model`    | Имя модели                |
| `-k`, `--key`      | LLM/API key               |
| `--api-key`        | Псевдоним `--key`         |
| `--ollama-url`     | Base URL Ollama           |
| `-r`, `--repo`     | Каталог проекта/workspace |
| `--workdir`        | Псевдоним `--repo`        |
| `--github`         | URL GitHub repository     |
| `--key-github`     | GitHub token              |

### Выполнение

| Параметр                 | Назначение                                      |
| ------------------------ | ----------------------------------------------- |
| `--max-context`          | Максимальный контекст модели; `0` = auto        |
| `--llm-timeout`          | Таймаут LLM-запроса                             |
| `--runner-timeout`       | Таймаут build/test команд                       |
| `--max-iterations`       | Максимальное число correction iterations        |
| `--agent-profile`        | `auto`, `small`, `medium`, `large`, `enhanced`  |
| `--agent-deep-threshold` | Порог перехода к более глубокой обработке       |
| `--auto-commit`          | Автоматический Git commit после успешного Agent |
| `--git-auto-init`        | Автоинициализация Git                           |
| `--compare`              | Сравнение вариантов реализации                  |
| `--auto-search`          | Автоматический web research                     |

### Reasoning

| Параметр             | Назначение                  |
| -------------------- | --------------------------- |
| `--reasoning`        | Включить reasoning          |
| `--reasoning-effort` | `low`, `medium`, `high`     |
| `--reasoning-budget` | Бюджет reasoning tokens     |
| `--reasoning-show`   | Показывать thinking output  |
| `--reasoning-router` | Reasoning для intent router |

### Patch и зависимости

| Параметр                 | Назначение                               |
| ------------------------ | ---------------------------------------- |
| `--deps-mode`            | `auto`, `ask`, `never`                   |
| `--confirm-apply`        | Подтверждение перед применением patch    |
| `--fuzzy-min-confidence` | Переопределение порога fuzzy matching    |
| `--patch-protocol`       | `auto`, `search_replace`, `replace_only` |
| `--patch-auditor`        | `auto`, `off`, `always`                  |
| `--diff-trace`           | Подробная диагностика patch              |

### Computer и Autonomy

| Параметр                | Назначение                                |
| ----------------------- | ----------------------------------------- |
| `--computer`            | Включить Computer mode                    |
| `--allow-sudo`          | Разрешить sudo-подобные команды           |
| `--computer-confirm`    | Политика подтверждения рискованных команд |
| `--computer-timeout`    | Таймаут системной команды                 |
| `--computer-max-output` | Максимальный объём вывода                 |
| `--autonomy`            | Включить автономный мониторинг            |
| `--autonomy-mode`       | `suggest` или `auto`                      |
| `--autonomy-interval`   | Интервал мониторинга                      |
| `--autonomy-limit`      | Максимум автономных мутаций               |

### Прочие

| Параметр        | Назначение                                                                        |
| --------------- | --------------------------------------------------------------------------------- |
| `--output`      | Автоматически сохранить последний результат                                       |
| `--raw`         | Уменьшить presentation formatting там, где поддерживается                         |
| `--dry-run`     | Показать/проверить предполагаемые действия без применения там, где поддерживается |
| `--debug`       | Debug logging                                                                     |
| `--log-level`   | `debug`, `info`, `warn`, `error`                                                  |
| `--save-config` | Сохранить итоговую конфигурацию в `~/.gogitor/config.json`                        |
| `--version`     | Показать версию                                                                   |
| `-v`            | Псевдоним `--version`                                                             |
| `--help`        | Показать справку                                                                  |

Параметров `--tui` и `--ui` нет: Zen является единственным интерфейсом.

## Сборка и разработка

Требования:

* Go **1.25+**;
* установленный Go toolchain;
* настроенный LLM provider для AI-команд.

Сборка:

```bash
go build -o gogitor ./cmd/gogitor/
```

Форматирование:

```bash
gofmt -w \
  internal/app/*.go \
  internal/config/*.go \
  internal/prompts/*.go \
  internal/ui/tui/*.go
```

Тесты:

```bash
go test ./...
```

Статический анализ:

```bash
go vet ./...
```

Запуск:

```bash
./gogitor
```

Проверка startup contract:

```bash
./gogitor --help
./gogitor --version
```

## Локальное состояние проекта

Проектное состояние Gogitor хранится в:

```text
.gogitor/
```

В зависимости от активных возможностей там могут находиться:

```text
agent/
computer_audit.json
logs/
```

Agent-сессии находятся в:

```text
.gogitor/agent/
```

Секреты и токены не следует хранить в Git-репозитории.

## Безопасность и приватность

Gogitor поддерживает локальные и удалённые LLM.

При использовании локального Ollama запросы к модели могут оставаться внутри локальной машины.

При использовании удалённого LLM проектный контекст, отправленный модели, обрабатывается внешним сервисом, указанным в конфигурации.

При включённом `--auto-search` поисковые запросы и связанная с research информация также могут выходить за пределы локальной машины.

Удалённый LLM и web research следует рассматривать как внешние границы обработки данных.

Для секретов предпочтительнее:

```text
GOGITOR_API_KEY
OPENAI_API_KEY
GOGITOR_GITHUB_TOKEN
GITHUB_TOKEN
```

а не хранение токенов непосредственно в исходниках или Git history.

Computer mode требует отдельного внимания, поскольку способен выполнять реальные команды операционной системы.

## Лицензия

BSD 3-Clause License.

См. [LICENSE.txt](LICENSE.txt).
