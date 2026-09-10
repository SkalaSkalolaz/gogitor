# Gogitor 2.0

AI-ассистент разработчика в терминале для проектов на Go. Gogitor теперь является **TUI-only приложением**: отдельного CLI-режима и CLI-пакета нет.

[English README](README.md)

## Что изменено в 2.0

- единая TUI-точка входа без CLI-ветки;
- единый каталог TUI-команд, который используется автодополнением;
- исправлена обработка `:task-diff`;
- `:agent enhanced <task>` — понятное имя усиленного профиля; `deep` сохранён как обратная совместимость;
- `:quit`, `:exit` и `:q` завершают приложение одинаково;
- `:cls` работает как псевдоним `:clear`;
- стартовая диагностика TODO/FIXME/HACK/BUG выполняется через Bubble Tea message loop, без гонки данных;
- добавлен реестр языков. Сейчас зарегистрирован только Go, но языковой слой больше не привязан к TUI и командному роутеру.

При этом сохранены основные подсистемы рабочей версии: генерация и изменение кода, DIFF/PATCH safety, AST-индекс, agent pipeline, проверки, Git/GitHub, web search, reasoning, computer mode, autonomy, mutation testing, генерация тестов, TODO scanner и журналирование.

## Параметры запуска

Gogitor остаётся TUI-приложением, но все основные параметры можно задать непосредственно при запуске. Порядок приоритета: **значения по умолчанию → `~/.gogitor/config.json` → `.gogitor.json` → переменные окружения → флаги запуска**.

Примеры:

```bash
./gogitor --provider ollama --model gpt-oss:20b --repo ~/Code/myapp
./gogitor --provider 'openai-compatible+http://localhost:8000/v1' --model my-model --key '...'
```

Основные параметры: `--provider`, `--model`, `--key`/`--api-key`, `--ollama-url`, `--repo`/`--workdir`, `--github`, `--key-github`, `--max-context`, `--llm-timeout`, `--runner-timeout`, `--max-iterations`, `--reasoning*`, `--auto-search`, `--computer*`, `--autonomy*`, `--patch-*`, `--agent-*`, `--auto-commit`, `--git-auto-init`, `--compare`, `--debug` и `--log-level`. Полный список: `./gogitor --help`.

Не передавай секреты в командной строке без необходимости: для API-ключей предпочтительнее `GOGITOR_API_KEY`, `OPENAI_API_KEY` и `GOGITOR_GITHUB_TOKEN`.

## Zen TUI

Gogitor использует один фиксированный интерфейс: **Zen**. Параметров `--tui` и `--ui`, выбора профиля, стартового меню и переключения TUI во время работы нет.

Zen намеренно не перегружает экран: максимальная площадь отдана выводу, редактор ввода находится непосредственно под ним, а компактная строка состояния остаётся видимой. Все возможности приложения используют один и тот же service/event pipeline.

## Архитектура

```text
cmd/gogitor
      │
      ▼
  ui/tui                 презентационный слой
      │
      ▼
  app                    application/service layer
      │
      ├── domain         события, результаты, контракты
      ├── language       реестр языков
      ├── workspace      DIFF/PATCH и проектная рабочая область
      ├── agent          multi-agent dispatcher
      ├── runner         Go build/test/run/lint/vet
      ├── index          AST и релевантность файлов
      ├── git/github     Git и GitHub API
      ├── search         безопасный web search
      ├── computer       контролируемое выполнение команд
      └── config/llm/... инфраструктура
```

### Главный принцип

TUI не знает, **как** выполняется задача. Он передаёт команду сервисному слою. Поэтому добавление нового функционала не требует переписывать обработку клавиш и разметку экрана.

Языковая поддержка вынесена в `internal/language`. Сейчас реестр содержит только Go:

```text
Go → .go
```

Следующий язык добавляется отдельным определением и подключением toolchain, а не изменением всего TUI.

## Возможности TUI

### Код и анализ

```text
:code <задача>          автоматический выбор стратегии
:fast <задача>          однопроходная генерация
:agent <задача>         адаптивный multi-agent
:agent enhanced <задача> усиленный профиль
:agent interview <задача> уточняющие вопросы
:agent reflect          анализ последней сессии
:agent report           отчёт последней сессии
:agent resume           продолжение незавершённой сессии
:agent undo             безопасный откат последнего agent commit
:fix <ошибка>           исправление ошибки
:ask <вопрос>           общий чат
:analyze <задача>       анализ без изменения файлов
:search <запрос>        web search
:load <файл>            загрузка задачи из .txt/.md
:article <тема>         статья
```

### Проект, тесты и диагностика

```text
:run [файл]
:test
:test lint
:vet
:todo
:suggest
:decisions
:task-diff
:diff-trace [on|off|status]
:reasoning [on|off|router]
```

### Git/GitHub

```text
:git status
:git diff
:git diff-task
:git commit
:git init
:git log
:git checkout ...
:git branch ...
:git merge ...
:git revert ...
:git reset ...
:git push ...
:git pull ...
:git fetch
:git clone ...
:git remote ...
:git create ...
:git pr
:git issue
:git changelog
:git pr-comment ...
```

### Дополнительные режимы

```text
:autonomy [on|off|status|run|clear]
:mutate [limit]
:autogen-tests [n]
:computer <задача>
```

`computer` по умолчанию выключен и требует явного разрешения в конфигурации.

## DIFF/PATCH

Для существующего кода Gogitor использует структурированные изменения вместо безусловной перегенерации файлов. В рабочей версии сохранены:

- точное SEARCH/REPLACE;
- `REPLACE_ONLY`;
- symbol anchors;
- строгая/сбалансированная/расширенная политика совпадения;
- fuzzy matching с порогом и margin;
- patch trace;
- scope diagnostics;
- patch audit для рискованных сценариев;
- проверки no-op patch;
- откат и контроль целостности рабочего дерева.

Эта часть намеренно не переписывалась без необходимости: она критична для надёжности изменений в существующем проекте.

## Agent

Полный конвейер:

```text
Planner → Coder → Reviewer → Verifier
```

Для сложных задач усиленный профиль автоматически включает более строгие проверки. Сессии сохраняются в `.gogitor/agent/<timestamp>/`.

## Установка

Требуется Go **1.25+** и установленный toolchain Go.

```bash
git clone https://github.com/SkalaSkalolaz/gogitor.git
cd gogitor
go mod tidy
go build -o gogitor ./cmd/gogitor/
```

Запуск:

```bash
./gogitor
```

Проверки:

```bash
make fmt
make test
make vet
make build
```

## LLM

Поддерживаются локальный Ollama и OpenAI-compatible API.

Основные параметры находятся в `~/.gogitor/config.json` и могут переопределяться переменными окружения. Текущие поля конфигурации сохраняются совместимыми с рабочей версией проекта.

Примеры:

```bash
export GOGITOR_PROVIDER=ollama
export GOGITOR_MODEL=gpt-oss:20b
export GOGITOR_OLLAMA_URL=http://localhost:11434
```

Для OpenAI-compatible API:

```bash
export GOGITOR_PROVIDER='openai-compatible+http://localhost:8000/v1'
export GOGITOR_MODEL='my-model'
export GOGITOR_API_KEY='...'
```

GitHub token можно задать через `GOGITOR_GITHUB_TOKEN` или `GITHUB_TOKEN`.

## Клавиши

| Клавиша | Действие |
|---|---|
| `Enter` | выполнить ввод |
| `Alt+Enter` | новая строка во вводе |
| `Up/Down` | переход по строкам ввода |
| `Tab` | переключение ввод/вывод |
| `PgUp/PgDn` | история команд |
| `F2` | режим выделения мышью |
| `Ctrl+A` | копировать весь вывод |
| `Ctrl+C` | отменить задачу / выйти |
| `Ctrl+D` | выйти, если задача не выполняется |
| `Esc` | вернуть фокус на ввод |

## Безопасность

Токены не должны попадать в Git-репозиторий. Для удалённых LLM и автоматического поиска учитывайте, что исходники проекта могут покидать локальную машину.

## Лицензия

BSD 3-Clause. См. `LICENSE.txt`.
