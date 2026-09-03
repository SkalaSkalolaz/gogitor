# Gogitor — AI-ассистент программиста для Go

[![License](https://img.shields.io/badge/License-BSD--3--Clause-blue.svg)](LICENSE.txt)
[![Go](https://img.shields.io/badge/Go-1.25.1-00ADD8.svg)](https://go.dev/)
[![Version](https://img.shields.io/badge/version-1.1.3-blue.svg)](https://github.com/SkalaSkalolaz/gogitor)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20WSL-lightgrey.svg)](#требования)

[English version](README.md)

**Gogitor** — инженерный AI-ассистент для разработки на Go, работающий в терминале.

Он объединяет:

* CLI;
* интерактивный TUI;
* генерацию и изменение кода с учётом существующего проекта;
* AST-индексирование проекта;
* минимальные SEARCH/REPLACE-патчи;
* проверку изменений во временной рабочей области;
* автоматический выбор стратегии выполнения;
* multi-agent конвейер;
* Git и GitHub;
* веб-поиск;
* генерацию статей и документации;
* анализ состояния проекта;
* reasoning;
* анализ изображений;
* computer mode;
* автономную инженерную помощь;
* мутационное тестирование;
* автогенерацию тестов;
* TODO/FIXME Scanner;
* Go Vet;
* журнал решений и историю выполнения задач.

Gogitor ориентирован прежде всего на проекты на **Go** и поддерживает локальные LLM через **Ollama**, а также удалённые модели через **OpenAI-совместимые API**.

> **Текущая версия исходного кода:** `1.1.3`
>
> Основная область применения Gogitor — разработка на Go. Интерфейс доступен на русском и английском языках.

---

## Содержание

* [Что делает Gogitor](#что-делает-gogitor)
* [Архитектура и процесс выполнения](#архитектура-и-процесс-выполнения)
* [Возможности](#возможности)

  * [Интерактивный TUI](#интерактивный-tui)
  * [Генерация кода](#генерация-кода)
  * [Patch Engine](#patch-engine)
  * [Проверка и исправление ошибок](#проверка-и-исправление-ошибок)
  * [Автоматический выбор стратегии выполнения](#автоматический-выбор-стратегии-выполнения)
  * [Multi-Agent разработка](#multi-agent-разработка)
  * [Интеллектуальный анализ проекта](#интеллектуальный-анализ-проекта)
  * [Сравнение подходов](#сравнение-подходов)
  * [Git и GitHub](#git-и-github)
  * [Веб-поиск](#веб-поиск)
  * [Статьи и документация](#статьи-и-документация)
  * [Анализ состояния проекта](#анализ-состояния-проекта)
  * [Режим размышления](#режим-размышления)
  * [Анализ изображений](#анализ-изображений)
  * [Режим управления компьютером](#режим-управления-компьютером)
  * [Автономный режим](#автономный-режим)
  * [Мутационное тестирование](#мутационное-тестирование)
  * [Автогенерация тестов](#автогенерация-тестов)
  * [TODO/FIXME Scanner](#todofixme-scanner)
  * [Go Vet](#go-vet)
  * [Журнал инженерных решений](#журнал-инженерных-решений)
  * [История выполнения задач](#история-выполнения-задач)
* [Требования](#требования)
* [Установка](#установка)
* [Быстрый старт](#быстрый-старт)
* [LLM-провайдеры](#llm-провайдеры)
* [Команды CLI](#команды-cli)
* [Параметры CLI](#параметры-cli)
* [Команды TUI](#команды-tui)
* [Режимы выполнения](#режимы-выполнения)
* [Процесс генерации кода](#процесс-генерации-кода)
* [Индексирование проекта](#индексирование-проекта)
* [Конфигурация](#конфигурация)
* [Переменные окружения](#переменные-окружения)
* [Примеры](#примеры)
* [Диагностика](#диагностика)
* [Структура проекта](#структура-проекта)
* [Безопасность](#безопасность)
* [Устранение проблем](#устранение-проблем)
* [Разработка](#разработка)
* [Лицензия](#лицензия)
* [Участие в разработке](#участие-в-разработке)

---

# Что делает Gogitor

Gogitor задуман как инженерный слой между разработчиком и LLM.

Вместо простого отправления промпта модели и копирования результата обратно в проект Gogitor организует контролируемый процесс:

```text
Задача пользователя
       │
       ▼
Определение намерения
       │
       ├── Chat
       ├── Анализ
       ├── Web Search
       ├── Генерация кода
       ├── Исправление
       ├── Запуск
       ├── Тестирование
       ├── Git
       ├── Article
       └── Computer
              │
              ▼
        Контекст проекта
              │
              ▼
          AST-индекс
              │
              ▼
             LLM
              │
              ▼
     Файлы / SEARCH-REPLACE
              │
              ▼
    Временная рабочая область
              │
              ├── go mod init
              ├── go mod tidy
              ├── gofmt
              ├── go build
              ├── go test
              ├── go vet
              └── golangci-lint
              │
              ▼
       Проверенный результат
              │
              ├── Применение изменений
              └── Git commit
```

Для сложных задач Gogitor может переключаться с однопроходной стратегии на полноценный multi-agent конвейер.

---

# Архитектура и процесс выполнения

Код проекта разделён на подсистемы с отдельными зонами ответственности.

Общая схема:

```text
CLI / TUI
   │
   ▼
Application Service
   │
   ├── Intent Router
   ├── Выбор стратегии
   ├── Agent Orchestration
   ├── Контекст проекта
   ├── Workspace
   ├── Runner
   └── Git / GitHub
          │
          ▼
        LLM
```

Главный принцип архитектуры: сгенерированный код считается кандидатом на изменение и проверяется до того, как будет скопирован обратно в реальный проект.

---

# Возможности

## Интерактивный TUI

TUI построен на Bubble Tea.

Он поддерживает:

* отображение Markdown;
* историю диалога;
* автодополнение команд;
* потоковую выдачу LLM;
* переключение фокуса;
* режим выделения текста мышью;
* отображение прогресса;
* отображение плана multi-agent задач;
* статусы этапов;
* визуализацию diff;
* информацию о текущем агенте;
* диагностику проекта и LLM;
* русский и английский интерфейс;
* работу с изображениями для vision-capable моделей.

Запуск:

```bash
./gogitor
```

или:

```bash
./gogitor tui
```

---

## Генерация кода

Gogitor умеет создавать новый Go-код и изменять существующий проект.

Поддерживаются:

* создание новых файлов;
* изменение существующих файлов;
* рефакторинг;
* разделение кода между файлами;
* выделение функций и компонентов;
* реализация новых возможностей;
* исправление ошибок компиляции;
* исправление неудачных тестов;
* выполнение задач из файлов;
* минимальные патчи;
* переход к полному содержимому файла, когда безопасный patch невозможен.

Пример:

```bash
./gogitor code \
  "создай REST API с endpoint /health и /version"
```

Для существующего проекта Gogitor предпочитает минимальные изменения вместо произвольной перезаписи файлов.

---

## Patch Engine

Для существующих файлов Gogitor может использовать минимальные `SEARCH/REPLACE`-патчи:

```text
--- Patch: internal/server/server.go ---
<<<<<<< SEARCH
точно существующий код
=======
заменяющий код
>>>>>>> REPLACE
```

Блок `SEARCH` должен происходить из существующего исходного кода.

Patch Engine поддерживает:

* точное совпадение;
* нормализованное совпадение;
* fuzzy matching там, где это разрешено политикой;
* Symbol anchors;
* strict, balanced и advanced политики;
* проверку confidence;
* AST-поиск символов;
* переход к полной замене файла, если patch нельзя безопасно применить.

### Политики patch

```text
strict
balanced
advanced
```

### Strict

Самая строгая политика.

Она:

* не использует автоматический fuzzy matching;
* ограничивает большие SEARCH-блоки;
* требует Symbol anchor для крупных изменений функций/методов;
* проверяет целевой символ через Go AST.

SEARCH-блок более чем из 10 строк в strict mode отклоняется.

### Balanced

Предназначен для средних и крупных coding-моделей.

Базовые пороги fuzzy matching:

```text
confidence >= 0.82
margin >= 0.08
```

### Advanced

Более гибкая стратегия сопоставления.

Базовые значения:

```text
confidence >= 0.85
margin >= 0.05
```

### Symbol anchors

Patch может содержать:

```text
--- Symbol: ParseConfig ---
```

или:

```text
--- Symbol: Handler.ServeHTTP ---
```

Символ определяется через Go AST.

Это особенно полезно, когда похожие фрагменты присутствуют в одном файле несколько раз.

### Политика, зависящая от модели

Gogitor умеет выбирать patch policy в зависимости от модели и провайдера.

Основные встроенные значения:

| Модель / endpoint                            | Политика |
| -------------------------------------------- | -------- |
| `gemma3:4b`                                  | strict   |
| `gemma4:12b`                                 | strict   |
| `ornith-1.5:9b`                              | strict   |
| `gpt-oss:20b`                                | strict   |
| `qwen3.8:27b`                                | balanced |
| `gemma4:26b`                                 | balanced |
| `llama3`                                     | balanced |
| `gemma4:31b-cloud`                           | advanced |
| `openai-compatible+http://localhost:8000/v1` | advanced |

Эти значения можно переопределить в конфигурации.

---

## Проверка и исправление ошибок

Изменения сначала проверяются во временной рабочей области.

В зависимости от операции могут выполняться:

```text
go mod init
go mod tidy
gofmt
go build ./...
go test -v -cover ./...
go vet ./...
golangci-lint run ./...
```

Gogitor умеет извлекать из результатов тестов:

* пройденные тесты;
* упавшие тесты;
* имена тестов;
* связанные функции;
* файлы;
* строки;
* сообщения об ошибках;
* информацию о покрытии.

Полученные данные могут повторно передаваться LLM для точечного исправления.

### Режим fix

`fix` предназначен для:

* ошибок компиляции;
* panic;
* stack trace;
* runtime error;
* ошибок тестов.

Пример:

```bash
./gogitor fix \
  "panic: runtime error: index out of range"
```

Intent router также умеет распознавать характерные признаки Go-ошибок:

```text
panic:
runtime error
goroutine
.go:123
--- FAIL
```

---

## Автоматический выбор стратегии выполнения

В текущем коде используются три активных режима:

```text
auto
fast
agent
```

Отдельного режима `workflow` в текущем execution engine больше нет.

### `auto`

Режим по умолчанию.

Gogitor учитывает:

* сложность задачи;
* локальный или удалённый провайдер;
* профиль модели;
* возможности модели;
* размер задачи;
* риск;
* явно заданные пользователем параметры.

Типичная схема:

```text
Простая задача
      ↓
fast / simple

Средняя задача
      ↓
agent

Большая/сложная задача
      ↓
agent deep
```

Для локальных моделей сложные задачи могут сразу направляться в более глубокий agent pipeline.

Для подходящих удалённых провайдеров выбор стратегии может дополнительно выполняться с помощью LLM.

### `fast`

Однопроходное выполнение:

```bash
./gogitor code \
  "переименуй эту функцию" \
  --mode fast
```

В TUI:

```text
:fast переименуй эту функцию
```

### `agent`

Полный multi-agent конвейер:

```bash
./gogitor code \
  "отрефакторь модуль авторизации" \
  --mode agent
```

В TUI:

```text
:agent отрефакторить модуль авторизации
```

### Deep Agent

Используется через:

```text
:agent deep <задача>
```

или:

```bash
./gogitor code \
  "перестрой архитектуру слоя хранения" \
  --agent \
  --deep
```

Deep-режим применяет более строгую обработку patch и более жёсткие quality gates.

### Удалённый Workflow

Следующее больше не является поддерживаемым режимом:

```text
workflow
```

Не следует использовать:

```bash
./gogitor workflow "задача"
```

или:

```bash
./gogitor code "задача" --mode workflow
```

Вместо этого используются:

```bash
./gogitor code "задача" --mode agent
```

или:

```text
:agent deep задача
```

---

## Multi-Agent разработка

Полный конвейер состоит из четырёх ролей:

```text
┌──────────┐
│ Planner  │
└────┬─────┘
     │
     ▼
┌──────────┐
│  Coder   │
└────┬─────┘
     │
     ▼
┌──────────┐
│ Reviewer │
└────┬─────┘
     │
     ▼
┌──────────┐
│ Verifier │
└──────────┘
```

### Planner

Planner:

* разбивает задачу на подзадачи;
* формирует критерии приёмки;
* старается сделать подзадачи независимо проверяемыми.

### Coder

Coder:

* реализует подзадачи;
* работает с рабочей областью проекта;
* создаёт и применяет изменения;
* выполняет промежуточную проверку.

### Reviewer

Reviewer проверяет:

* ошибки компиляции;
* корректность реализации;
* возможные nil dereference;
* проблемы безопасности;
* регрессии;
* соответствие исходной задаче.

### Verifier

Verifier определяет, действительно ли исходная задача выполнена.

Multi-Agent подсистема также поддерживает:

* очереди запросов к LLM;
* приоритеты;
* бюджеты ролей;
* retry;
* exponential backoff;
* статистику использования;
* статистику времени выполнения;
* progress и ETA;
* persistent agent memory;
* checkpoints;
* rollback.

### Команды агента

```text
:agent <задача>
:agent deep <задача>
:agent interview <задача>
:agent reflect
:agent report
:agent resume
:agent undo
```

Примеры:

```text
:agent отрефакторить authentication в отдельный пакет
```

```text
:agent deep создать REST API с middleware и тестами
```

```text
:agent interview добавить кэширование в API layer
```

Артефакты последней agent-сессии сохраняются в:

```text
.gogitor/agent/<timestamp>/
```

Deep-сессии могут содержать состояние, план, результат и checkpoint-информацию.

`:agent undo` безопасно отменяет последний commit агента. Если этот commit уже не является текущим `HEAD`, Gogitor не выполняет небезопасный rollback и рекомендует обычный `git revert`.

---

## Интеллектуальный анализ проекта

Gogitor не отправляет в LLM весь проект без разбора.

Используется AST-индекс Go и система оценки релевантности.

Индекс учитывает:

* Go-файлы;
* пакеты;
* импорты;
* функции;
* методы;
* связи вызовов;
* структуру проекта;
* важность файлов;
* текстовую релевантность.

Для ранжирования используются:

* граф импортов;
* call graph;
* PageRank-подобная оценка важности;
* BM25-подобная текстовая релевантность;
* расширение русских и английских синонимов.

При формировании контекста сначала учитываются явно названные файлы, затем наиболее релевантные индексированные файлы.

Индекс кешируется и обновляется при изменении исходников.

---

## Сравнение подходов

Для достаточно сложных задач Gogitor может перед реализацией предложить несколько вариантов архитектуры или реализации.

Сравнение может учитывать:

* сложность;
* производительность;
* читаемость;
* зависимости;
* тестируемость;
* компромиссы.

Отключение:

```bash
./gogitor code \
  "создай HTTP сервер" \
  --no-compare
```

или:

```bash
export GOGITOR_COMPARE_APPROACHES=false
```

---

## Git и GitHub

Поддерживаются:

```text
status
diff
diff-task
commit
init
log
checkout
branch
merge
revert
reset
push
pull
fetch
clone
remote
create
pr
issue
changelog
pr-comment
```

Примеры:

```bash
./gogitor git status
```

```bash
./gogitor git diff
```

```bash
./gogitor git commit
```

Сообщение коммита может автоматически генерироваться по реальному diff в формате Conventional Commits.

Примеры:

```text
feat(auth): add JWT token validation
fix(runner): handle empty test output
refactor(workspace): extract patch application
test(index): add BM25 ranking coverage
```

### GitHub API

Создание репозитория:

```bash
./gogitor git create myproject \
  --private \
  --desc "My project"
```

Клонирование:

```bash
./gogitor git clone \
  https://github.com/user/repository \
  --key-github ghp_xxx
```

Push:

```bash
./gogitor git push \
  --github https://github.com/user/repository \
  --key-github ghp_xxx
```

Распознаваемые префиксы токенов включают:

```text
ghp_...
github_pat_...
```

Токены не следует хранить в Git или помещать в исходный код.

---

## Веб-поиск

Gogitor умеет выполнять веб-поиск и передавать найденную информацию LLM.

Пример:

```bash
./gogitor search \
  "последняя версия Go"
```

Поисковая подсистема включает:

* ограничение частоты запросов;
* контроль доменов;
* SSRF-защиту;
* обнаружение секретов;
* санитизацию содержимого;
* защиту от prompt injection;
* явное отношение к полученным страницам как к недоверенному содержимому.

Автоматический поиск можно включить:

```bash
./gogitor code \
  "исследуй современные подходы к HTTP routing в Go и реализуй лучший вариант" \
  --auto-search
```

### Конфиденциальность

При `--auto-search` и удалённом LLM-провайдере код проекта и связанная с поиском информация могут передаваться внешним сервисам.

Для чувствительных проектов лучше использовать локальный Ollama endpoint.

---

## Статьи и документация

Gogitor может создавать:

* технические статьи;
* tutorial;
* how-to;
* обзоры;
* рассказы;
* новостные тексты;
* описания кода;
* другие длинные тексты.

Простой режим:

```bash
./gogitor article \
  "как работает garbage collector в Go"
```

Расширенный режим:

```bash
./gogitor article \
  "подробный разбор middleware pattern" \
  --full
```

Типы:

```text
technical
news
story
review
howto
code_desc
free
```

В зависимости от задачи могут использоваться:

* контекст проекта;
* веб-поиск;
* план статьи;
* последовательная генерация разделов;
* контекст предыдущих разделов.

---

## Анализ состояния проекта

Команда:

```bash
./gogitor suggest
```

проводит целенаправленный анализ проекта.

Результаты группируются по категориям:

* критические проблемы;
* технический долг;
* отсутствующие тесты;
* code smells;
* улучшения.

Рекомендации стараются ссылаться на конкретные файлы и функции.

---

## Режим размышления

Gogitor поддерживает reasoning/thinking для моделей и провайдеров, которые предоставляют такую возможность.

Флаги:

```text
--reasoning
--reasoning-effort <low|medium|high>
--reasoning-budget <n>
--reasoning-show
--reasoning-router
```

Пример:

```bash
./gogitor code \
  "спроектируй конкурентный worker pool" \
  --reasoning \
  --reasoning-effort high \
  --reasoning-budget 8192
```

TUI:

```text
:reasoning
:reasoning on
:reasoning off
:reasoning router on
:reasoning router off
```

Механизм зависит от провайдера.

Например:

* Ollama может использовать `think`;
* OpenAI-compatible API может использовать `reasoning_effort`.

Если модель reasoning не поддерживает, параметр может не дать эффекта или запрос может быть повторён без него.

---

## Анализ изображений

Команды `ask` и `analyze` могут принимать изображение:

```bash
./gogitor ask \
  "что изображено на этом скриншоте?" \
  --image screenshot.png
```

```bash
./gogitor analyze \
  "найди ошибку на этом скриншоте" \
  --image error.png
```

Поддерживаются:

```text
.png
.jpg
.jpeg
.gif
.webp
.bmp
```

Требуется vision-capable модель.

Полезно для:

* скриншотов;
* сообщений об ошибках;
* UI;
* архитектурных схем;
* скриншотов кода;
* технических изображений.

---

## Режим управления компьютером

Computer mode позволяет планировать и выполнять реальные команды операционной системы.

По умолчанию он **отключён**.

Пример:

```bash
./gogitor computer \
  "покажи использование диска" \
  --computer \
  --dry-run
```

Включение через переменную:

```bash
export GOGITOR_COMPUTER_ENABLED=true
```

или `.gogitor.json`:

```json
{
  "computer_enabled": true
}
```

Флаги:

```text
--computer
--dry-run
--allow-sudo
```

Механизмы защиты включают:

* блокировку запрещённых команд;
* классификацию риска;
* подтверждение опасных команд;
* ограничения command substitution;
* аудит;
* отдельное разрешение sudo;
* dry-run;
* проверку результата после выполнения.

Журнал:

```text
.gogitor/computer_audit.json
```

`--allow-sudo` следует включать только при необходимости.

---

## Автономный режим

Autonomy — механизм инженерного мониторинга, который обнаруживает исправимые проблемы и помещает конкретные задачи в очередь.

TUI:

```text
:autonomy
:autonomy on
:autonomy off
:autonomy status
:autonomy run
:autonomy clear
```

CLI:

```bash
./gogitor autonomy status
```

```bash
./gogitor autonomy on
```

```bash
./gogitor autonomy run
```

```bash
./gogitor autonomy clear
```

Общий принцип:

```text
Обнаружение проблемы
        ↓
Добавление задачи в очередь
        ↓
Проверка очереди пользователем
        ↓
autonomy run
        ↓
Конкретное исправление
        ↓
Проверка результата
```

По умолчанию автономный режим работает осторожно и передаёт модели конкретную обнаруженную проблему, а не свободную команду вроде «улучши проект».

---

## Мутационное тестирование

Мутационное тестирование детерминировано и не требует LLM.

Запуск:

```bash
./gogitor mutate
```

Лимит:

```bash
./gogitor mutate 10
```

Поддерживаются замены:

```text
>= → >
<= → <
&& → ||
|| → &&
== → !=
!= → ==
```

Результат содержит:

* сгенерированные мутации;
* убитые мутации;
* выжившие мутации;
* ошибки;
* Mutation Score.

Убитая мутация означает, что тесты обнаружили внесённое изменение.

Выжившая мутация означает, что тесты его не обнаружили.

---

## Автогенерация тестов

Gogitor может найти через AST экспортируемые функции без соответствующих тестов.

Запуск:

```bash
./gogitor autogen-tests
```

Ограничение:

```bash
./gogitor autogen-tests 3
```

Процесс:

```text
AST-анализ
    ↓
Экспортируемые функции без тестов
    ↓
Генерация теста
    ↓
Создание test-файла
    ↓
Запуск тестов
    ↓
Сохранение только успешного результата
```

Сгенерированный test-файл сохраняется только при успешной проверке.

---

## TODO/FIXME Scanner

Проверка без LLM:

```bash
./gogitor todo
```

Ищутся:

```text
TODO
FIXME
HACK
XXX
BUG
```

TUI также выполняет лёгкое сканирование TODO при запуске и предлагает использовать `:todo`, если маркеры обнаружены.

---

## Go Vet

Запуск:

```bash
./gogitor vet
```

Эквивалент:

```bash
go vet ./...
```

LLM для `vet` не требуется.

Проверка выполняется во временной рабочей области.

---

## Журнал инженерных решений

В multi-agent работе Gogitor может сохранять инженерные решения.

Журнал может содержать:

* выбранные решения;
* отклонённые альтернативы;
* ограничения;
* причины выбора;
* неудачные подходы.

Просмотр:

```bash
./gogitor decisions
```

В TUI:

```text
:decisions
:journal
```

Система также может обнаруживать **decision debt** — временные решения, ограничения которых могли перестать быть актуальными.

---

## История выполнения задач

История хранится в:

```text
.gogitor/task_history.json
```

Записи содержат:

* статус;
* ID задачи;
* время;
* запрос;
* режим выполнения;
* число затронутых файлов;
* количество добавленных/удалённых строк;
* hash commit.

Хранится до 100 записей.

В TUI:

```text
:history
```

Показываются до 20 последних записей.

Для просмотра совокупного diff последней завершённой задачи:

```text
:task-diff
```

Также поддерживается:

```text
task-diff
```

---

# Требования

Gogitor прежде всего предназначен для Unix-подобных сред.

## Обязательно

* **Go 1.25.1** или совместимый Go toolchain;
* **Ollama** или **OpenAI-compatible API**;
* сетевой доступ при загрузке зависимостей и работе с удалёнными сервисами.

## Рекомендуется

* Git;
* достаточно мощная coding-модель;
* большой context window для крупных проектов.

## Поддерживаемые среды

* Linux;
* macOS;
* Windows через WSL.

## Дополнительно

Для lint:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

---

# Установка

Клонирование:

```bash
git clone https://github.com/SkalaSkalolaz/gogitor.git
cd gogitor
```

Загрузка зависимостей:

```bash
go mod tidy
```

Сборка:

```bash
go build -o gogitor .
```

Проверка:

```bash
./gogitor --help
```

Версия:

```bash
./gogitor version
```

Ожидаемый текущий результат:

```text
gogitor 1.1.3
```

Установка глобально:

```bash
sudo mv gogitor /usr/local/bin/
```

---

# Быстрый старт

## 1. Запустите Ollama

```bash
ollama serve
```

## 2. Запустите Gogitor

```bash
./gogitor
```

или:

```bash
./gogitor tui
```

## 3. Выберите модель

Например:

```bash
./gogitor tui \
  --provider ollama \
  --model gemma3:4b
```

## 4. Создайте код

```bash
./gogitor code \
  "создай CLI-калькулятор на Go"
```

## 5. Проанализируйте проект

```bash
./gogitor analyze \
  "найди потенциальные ошибки и предложи улучшения"
```

## 6. Запустите тесты

```bash
./gogitor test
```

## 7. Проверьте окружение

```bash
./gogitor doctor
```

---

# LLM-провайдеры

Gogitor поддерживает Ollama-compatible endpoints и OpenAI-compatible API.

## Ollama

Локальный Ollama:

```bash
./gogitor tui \
  --provider ollama \
  --model gemma3:4b
```

Удалённый Ollama-compatible endpoint:

```bash
./gogitor tui \
  --provider http://192.168.1.10:11434 \
  --model gemma3:4b
```

Поддерживаются:

```text
ollama
http://host:11434
https://host:11434
```

## OpenAI-compatible API

OpenAI-style endpoint:

```bash
./gogitor ask \
  "объясни generics в Go" \
  --provider openai+https://api.example.com/v1 \
  --model gpt-4o-mini \
  --key sk-...
```

Обычный OpenAI-compatible endpoint:

```bash
./gogitor code \
  "создай main.go" \
  --provider openai-compatible+http://localhost:8000/v1 \
  --model local-model
```

| Значение                           | Назначение                    |
| ---------------------------------- | ----------------------------- |
| `ollama`                           | Локальный Ollama              |
| `http://host:11434`                | Ollama-compatible HTTP        |
| `https://host:11434`               | Ollama-compatible HTTPS       |
| `openai+https://host/v1`           | OpenAI-style API              |
| `openai-compatible+http://host/v1` | Generic OpenAI-compatible API |

---

# Команды CLI

```text
gogitor
gogitor tui [flags]

gogitor code <task> [flags]
gogitor fix <error / stack trace> [flags]

gogitor task <path/to/file.txt|file.md> [flags]
gogitor file <path/to/file.txt|file.md> [flags]

gogitor ask <question> [flags]
gogitor analyze <question> [flags]
gogitor search <query> [flags]

gogitor run [file] [flags]

gogitor test [flags]
gogitor test lint [flags]
gogitor vet [flags]
gogitor todo [flags]
gogitor suggest [flags]

gogitor article <topic> [--full] [flags]

gogitor computer <task> [flags]
gogitor autonomy [on|off|status|run|clear] [flags]
gogitor mutate [limit] [flags]
gogitor autogen-tests [count] [flags]

gogitor decisions [flags]
gogitor journal [flags]

gogitor git <subcommand> [flags]

gogitor doctor [flags]
gogitor help
```

Активной команды `gogitor workflow` нет.

---

# Параметры CLI

## Общие параметры

| Параметр                                 | Короткий | Назначение                        |
| ---------------------------------------- | :------: | --------------------------------- |
| `--provider <name>`                      |   `-p`   | LLM-провайдер                     |
| `--model <model>`                        |   `-m`   | Модель                            |
| `--key <key>`                            |   `-k`   | API-ключ LLM                      |
| `--repo <path>`                          |   `-r`   | Корень проекта                    |
| `--github <url>`                         |          | URL GitHub-репозитория            |
| `--key-github <token>`                   |          | GitHub token                      |
| `--image <path>`                         |          | Изображение для `ask` / `analyze` |
| `--max-context <n>`                      |          | Максимальный контекст             |
| `--output <file>`                        |   `-o`   | Сохранить результат               |
| `--debug`                                |          | Подробное логирование             |
| `--raw`                                  |          | Raw output                        |
| `--pretty`                               |          | Человекочитаемый вывод            |
| `--auto-search`                          |          | Автоматический веб-поиск          |
| `--reasoning`                            |          | Включить reasoning                |
| `--reasoning-effort <low\|medium\|high>` |          | Глубина reasoning                 |
| `--reasoning-budget <n>`                 |          | Бюджет reasoning                  |
| `--reasoning-show`                       |          | Показывать reasoning              |
| `--reasoning-router`                     |          | Reasoning для intent router       |
| `--computer`                             |          | Включить computer mode            |
| `--help`                                 |   `-h`   | Справка                           |

## Параметры генерации кода

```text
--mode <auto|fast|agent>
--agent
--deep
--dry-run
--no-commit
--no-tests
--no-compare
--json
```

`--agent` принудительно включает agent mode.

`--deep` запрашивает глубокий agent pipeline.

## Параметры task-файлов

```text
--code
--json
```

`--code` принудительно выбирает code mode вместо automatic intent detection.

Task-файлы должны иметь расширение:

```text
.txt
.md
```

Файл должен быть непустым и иметь допустимый размер.

## Computer mode

```text
--computer
--dry-run
--allow-sudo
```

---

# Команды TUI

| Команда                   | Назначение                           |
| ------------------------- | ------------------------------------ |
| `:help`                   | Справка                              |
| `:clear`                  | Очистить контекст диалога            |
| `:cls`                    | Очистить экран                       |
| `:code <task>`            | Создать или изменить код             |
| `:fast <task>`            | Принудительный fast mode             |
| `:agent <task>`           | Multi-agent pipeline                 |
| `:agent deep <task>`      | Deep agent pipeline                  |
| `:agent interview <task>` | Уточняющие вопросы перед выполнением |
| `:agent reflect`          | Анализ последней agent-сессии        |
| `:agent report`           | Отчёт последней agent-сессии         |
| `:agent resume`           | Продолжить неудачную agent-сессию    |
| `:agent undo`             | Отменить последний commit агента     |
| `:fix <error>`            | Исправить ошибку                     |
| `:ask <question>`         | Обычный чат                          |
| `:analyze <task>`         | Анализ проекта без изменения файлов  |
| `:search <query>`         | Веб-поиск                            |
| `:load <file>`            | Загрузить `.txt`/`.md` задачу        |
| `:run [file]`             | Запустить проект или Go-файл         |
| `:test`                   | Тестирование                         |
| `:test lint`              | Lint и обработка исправлений         |
| `:vet`                    | `go vet`                             |
| `:todo`                   | TODO/FIXME/HACK/XXX/BUG              |
| `:suggest`                | Анализ состояния проекта             |
| `:article <topic>`        | Генерация статьи                     |
| `:git <subcommand>`       | Git                                  |
| `:decisions`              | Журнал решений                       |
| `:journal`                | Алиас `:decisions`                   |
| `:history`                | История задач                        |
| `:task-diff`              | Совокупный diff последней задачи     |
| `:reasoning`              | Состояние reasoning                  |
| `:reasoning on`           | Включить reasoning                   |
| `:reasoning off`          | Выключить reasoning                  |
| `:reasoning router on`    | Reasoning для intent router          |
| `:reasoning router off`   | Выключить reasoning router           |
| `:computer <task>`        | Системная задача                     |
| `:autonomy`               | Статус autonomy                      |
| `:autonomy on`            | Включить autonomy                    |
| `:autonomy off`           | Выключить autonomy                   |
| `:autonomy status`        | Статус autonomy                      |
| `:autonomy run`           | Выполнить очередь                    |
| `:autonomy clear`         | Очистить очередь                     |
| `:mutate [limit]`         | Mutation testing                     |
| `:autogen-tests [n]`      | Автогенерация тестов                 |

## Горячие клавиши

| Клавиша     | Действие                                |
| ----------- | --------------------------------------- |
| `Enter`     | Отправить ввод                          |
| `Alt+Enter` | Новая строка                            |
| `Up/Down`   | Перемещение между строками / история    |
| `Tab`       | Переключить ввод/вывод                  |
| `F2`        | Режим выделения текста мышью            |
| `Ctrl+A`    | Копировать весь вывод                   |
| `PgUp/PgDn` | Просмотр истории команд                 |
| `Ctrl+C`    | Отмена выполняющейся операции или выход |

---

# Режимы выполнения

## `auto`

Режим по умолчанию:

```bash
./gogitor code "реализуй новую возможность"
```

Gogitor автоматически выбирает между простым и agent-выполнением.

## `fast`

Однопроходная генерация:

```bash
./gogitor code \
  "переименуй эту функцию" \
  --mode fast
```

## `agent`

Полный Planner → Coder → Reviewer → Verifier:

```bash
./gogitor code \
  "отрефакторь модуль авторизации" \
  --mode agent
```

## Deep Agent

```text
:agent deep <задача>
```

или:

```bash
./gogitor code \
  "перестрой архитектуру хранения" \
  --agent \
  --deep
```

Deep mode использует более строгие patch policy и quality gates.

## Что больше не поддерживается

Старый режим:

```text
workflow
```

удалён.

Не использовать:

```bash
./gogitor workflow "задача"
```

или:

```bash
./gogitor code "задача" --mode workflow
```

Используйте `agent` или `agent deep`.

---

# Процесс генерации кода

Типичная операция изменения проекта проходит через следующие стадии.

## 1. Определение намерения

Запрос классифицируется как:

```text
code
fix
analyze
search
run
test
git
article
computer
chat
```

## 2. Формирование контекста

Из проекта выбираются наиболее релевантные файлы.

## 3. Выбор стратегии

Выбирается:

```text
simple
```

или:

```text
agent
```

Для сложных задач возможен deep agent.

## 4. Генерация

Для существующих файлов предпочтителен минимальный patch.

Для новых файлов могут возвращаться полные файлы.

## 5. Применение patch

Patch Engine:

* проверяет SEARCH;
* проверяет Symbol;
* оценивает confidence при разрешённом fuzzy matching;
* отклоняет небезопасные patches;
* при необходимости использует полное содержимое файла.

## 6. Временная проверка

Изменения помещаются во временную рабочую область.

## 7. Validation loop

Могут выполняться:

```text
gofmt
go build
go test
go vet
golangci-lint
```

## 8. Исправления

Если проверка завершается ошибкой, диагностическая информация может быть отправлена LLM для точечного исправления.

## 9. Применение

После успешной проверки изменения переносятся в реальный проект.

## 10. Git

При включённом automatic commit Gogitor может создать Git commit.

---

# Индексирование проекта

Индекс предназначен для уменьшения нерелевантного LLM-контекста.

Используется Go AST.

Основные связи:

```text
File
 ├── Package
 ├── Imports
 ├── Functions
 ├── Methods
 └── Call relationships
```

Для ранжирования используются структурные и текстовые признаки.

При формировании контекста приоритет получают:

1. явно указанные файлы;
2. наиболее релевантные индексированные файлы;
3. дополнительные Go-файлы для заполнения бюджета контекста.

Индекс обновляется при изменении workspace.

---

# Конфигурация

Конфигурация загружается в порядке:

1. значения по умолчанию;
2. глобальная конфигурация;
3. переменные окружения;
4. `.gogitor.json`;
5. CLI-параметры.

## Глобальная конфигурация

```text
~/.gogitor/config.json
```

## Конфигурация проекта

```text
.gogitor.json
```

в корне проекта.

## Логи

```text
~/.gogitor/logs/
```

## Пример `.gogitor.json`

```json
{
  "provider": "ollama",
  "model": "gemma3:4b",
  "auto_git_commit": false,
  "git_auto_init": true,
  "dry_run": false,
  "compare_approaches": true,
  "auto_search": false,
  "raw_output": false,
  "multi_agent": true,
  "max_context_tokens": 0,
  "agent_model_profile": "auto",
  "agent_deep_complexity_threshold": 6,
  "deps_mode": "auto",
  "confirm_apply": false,
  "fuzzy_min_confidence": 0,
  "reasoning_enabled": false,
  "reasoning_effort": "medium",
  "reasoning_budget": 0,
  "reasoning_show": false,
  "reasoning_router": false,
  "computer_enabled": false,
  "computer_allow_sudo": false,
  "computer_confirm_high": true,
  "computer_command_timeout": 120,
  "computer_max_output": 100000,
  "autonomy_enabled": false,
  "autonomy_mode": "suggest",
  "autonomy_interval_sec": 60,
  "autonomy_mutation_limit": 20
}
```

Нет необходимости указывать все параметры.

### Основные значения по умолчанию

```text
provider                  = ollama
model                     = gemma3:4b
ollama_url                = http://localhost:11434
log_level                 = info
llm_timeout               = 3000
max_iterations            = 5
auto_git_commit           = true
git_auto_init             = true
multi_agent               = true
max_context_tokens        = 0
compare_approaches        = true
auto_search               = false
agent_model_profile       = auto
agent_deep_complexity_threshold = 6
deps_mode                 = auto
confirm_apply             = false
fuzzy_min_confidence      = 0
computer_enabled          = false
computer_allow_sudo       = false
computer_confirm_high     = true
computer_command_timeout  = 120
computer_max_output       = 100000
reasoning_enabled         = false
reasoning_effort          = medium
reasoning_budget          = 0
reasoning_show             = false
reasoning_router          = false
autonomy_enabled          = false
autonomy_mode             = suggest
autonomy_interval_sec     = 60
autonomy_mutation_limit   = 20
```

### Размер контекста

Можно задать явно:

```bash
./gogitor code \
  "рефакторинг всего проекта" \
  --max-context 262144
```

или:

```json
{
  "max_context_tokens": 262144
}
```

При `0` используется автоматический режим.

Фактически доступный контекст зависит от выбранной модели и провайдера.

---

# Переменные окружения

| Переменная                        | Назначение                           |
| --------------------------------- | ------------------------------------ |
| `GOGITOR_PROVIDER`                | Провайдер LLM                        |
| `GOGITOR_MODEL`                   | Модель                               |
| `GOGITOR_API_KEY`                 | API-ключ LLM                         |
| `OPENAI_API_KEY`                  | Резервный ключ для OpenAI-compatible |
| `GOGITOR_OLLAMA_URL`              | URL Ollama                           |
| `GOGITOR_LOG_LEVEL`               | Уровень логирования                  |
| `GOGITOR_DEBUG`                   | Debug-логирование                    |
| `GOGITOR_DRY_RUN`                 | Dry-run                              |
| `GOGITOR_RAW`                     | Raw output                           |
| `GOGITOR_LLM_TIMEOUT`             | Таймаут LLM                          |
| `GOGITOR_MAX_ITERATIONS`          | Максимум итераций исправления        |
| `GOGITOR_AUTO_GIT_COMMIT`         | Автоматический commit                |
| `GOGITOR_GIT_AUTO_INIT`           | Автоинициализация Git                |
| `GOGITOR_MULTI_AGENT`             | Multi-agent                          |
| `GOGITOR_COMPARE_APPROACHES`      | Сравнение подходов                   |
| `GOGITOR_MAX_CONTEXT_TOKENS`      | Максимальный контекст                |
| `GOGITOR_GITHUB_URL`              | GitHub URL                           |
| `GOGITOR_GITHUB_TOKEN`            | GitHub token                         |
| `GITHUB_TOKEN`                    | Резервный GitHub token               |
| `GOGITOR_AUTO_SEARCH`             | Автоматический поиск                 |
| `GOGITOR_DEPS_MODE`               | Режим разрешения зависимостей        |
| `GOGITOR_CONFIRM_APPLY`           | Настройка подтверждения применения   |
| `GOGITOR_COMPUTER_ENABLED`        | Computer mode                        |
| `GOGITOR_COMPUTER_ALLOW_SUDO`     | Разрешить sudo                       |
| `GOGITOR_REASONING`               | Reasoning                            |
| `GOGITOR_REASONING_EFFORT`        | `low`, `medium`, `high`              |
| `GOGITOR_REASONING_BUDGET`        | Бюджет reasoning                     |
| `GOGITOR_REASONING_ROUTER`        | Reasoning для router                 |
| `GOGITOR_AUTONOMY`                | Autonomy                             |
| `GOGITOR_AUTONOMY_MODE`           | Режим autonomy                       |
| `GOGITOR_AUTONOMY_INTERVAL`       | Интервал autonomy                    |
| `GOGITOR_AUTONOMY_MUTATION_LIMIT` | Лимит mutation testing               |

---

# Примеры

## Ollama

```bash
./gogitor tui \
  --provider ollama \
  --model gemma3:4b
```

## Удалённый Ollama-compatible endpoint

```bash
./gogitor tui \
  --provider http://192.168.1.10:11434 \
  --model gemma3:4b
```

## OpenAI-compatible API

```bash
./gogitor ask \
  "объясни context.Context" \
  --provider openai+https://api.example.com/v1 \
  --model gpt-4o-mini \
  --key sk-...
```

## Генерация кода

```bash
./gogitor code \
  "создай REST API с endpoint /health и /version"
```

## Анализ проекта

```bash
./gogitor analyze \
  "найди потенциальные ошибки и архитектурные проблемы"
```

## Анализ изображения

```bash
./gogitor analyze \
  "найди ошибку на этом скриншоте" \
  --image screenshot.png
```

## Fast mode

```bash
./gogitor code \
  "переименуй ParseConfig в LoadConfig" \
  --mode fast
```

## Agent mode

```bash
./gogitor code \
  "отрефакторь слой авторизации" \
  --mode agent
```

## Deep agent

```bash
./gogitor code \
  "перестрой архитектуру хранения и добавь тесты" \
  --agent \
  --deep
```

## Dry-run

```bash
./gogitor code \
  "рефакторинг main.go" \
  --dry-run
```

## Без автоматического commit

```bash
./gogitor code \
  "раздели код на пакеты" \
  --no-commit
```

## Без тестов

```bash
./gogitor code \
  "добавь логирование" \
  --no-tests
```

Пропуск тестов следует рассматривать только как временную опцию разработки.

## Без сравнения подходов

```bash
./gogitor code \
  "создай HTTP сервер" \
  --no-compare
```

## Task-файл

```bash
./gogitor task ./tasks/feature.txt
```

## Task-файл в code mode

```bash
./gogitor file ./tasks/refactor.md --code
```

## JSON

```bash
./gogitor test --json
```

## Raw output

```bash
./gogitor ask \
  "объясни context.Context" \
  --raw
```

## Сохранение результата

```bash
./gogitor ask \
  "объясни context.Context" \
  --output answer.md
```

```bash
./gogitor code \
  "создай hello world" \
  --output main.go
```

## Большой контекст

```bash
./gogitor code \
  "рефакторинг всего проекта" \
  --max-context 262144
```

## Исправление panic

```bash
./gogitor fix \
  "panic: runtime error: index out of range [3] with length 2"
```

## Анализ состояния проекта

```bash
./gogitor suggest
```

## TODO

```bash
./gogitor todo
```

## Журнал решений

```bash
./gogitor decisions
```

## Mutation testing

```bash
./gogitor mutate 10
```

## Автогенерация тестов

```bash
./gogitor autogen-tests 3
```

## Reasoning

```bash
./gogitor code \
  "спроектируй конкурентный worker pool" \
  --reasoning \
  --reasoning-effort high
```

## Computer mode

```bash
./gogitor computer \
  "покажи использование диска" \
  --computer \
  --dry-run
```

## Autonomy

```bash
./gogitor autonomy status
```

## Git

```bash
./gogitor git status
```

```bash
./gogitor git diff
```

## Диагностика

```bash
./gogitor doctor
```

---

# Диагностика

Используйте:

```bash
./gogitor doctor
```

Диагностика может показать:

* активного провайдера;
* активную модель;
* эффективный размер контекста;
* Ollama URL;
* рабочую директорию;
* путь конфигурации;
* путь логов;
* таймаут;
* максимум итераций;
* настройки Git;
* multi-agent настройки;
* auto-search;
* dry-run;
* reasoning;
* computer/autonomy.

Для подробного логирования:

```bash
./gogitor --debug
```

Логи:

```text
~/.gogitor/logs/
```

---

# Структура проекта

Основные подсистемы:

```text
.
├── main.go
├── internal/
│   ├── app/
│   │   Оркестрация приложения
│   │
│   ├── agent/
│   │   LLM dispatcher, очереди, бюджеты, retry и agent memory
│   │
│   ├── autonomy/
│   │   Автономная инженерия, mutation testing и генерация тестов
│   │
│   ├── codegen/
│   │   Парсинг и применение сгенерированных файлов и patch
│   │
│   ├── computer/
│   │   Планирование, выполнение и аудит системных команд
│   │
│   ├── config/
│   │   Загрузка и проверка конфигурации
│   │
│   ├── domain/
│   │   Общие типы приложения
│   │
│   ├── git/
│   │   Git-операции
│   │
│   ├── github/
│   │   GitHub API
│   │
│   ├── i18n/
│   │   Локализация
│   │
│   ├── index/
│   │   AST-индекс и ранжирование
│   │
│   ├── llm/
│   │   LLM-клиенты и провайдеры
│   │
│   ├── prompts/
│   │   Построение промптов и стратегии выполнения
│   │
│   ├── runner/
│   │   Build / test / run / vet / lint
│   │
│   ├── search/
│   │   Веб-поиск и обработка результатов
│   │
│   ├── security/
│   │   Проверки безопасности и путей
│   │
│   ├── workspace/
│   │   Проектные файлы и временная рабочая область
│   │
│   └── ui/
│       ├── cli/
│       │   CLI
│       │
│       └── tui/
│           Bubble Tea TUI
│
├── LICENSE.txt
├── README.md
└── README_RU.md
```

---

# Безопасность

Gogitor способен:

* создавать код;
* изменять файлы;
* выполнять сгенерированные программы;
* запускать build и test;
* работать с Git;
* выполнять веб-поиск;
* в computer mode выполнять реальные команды ОС.

LLM-сгенерированный код следует считать **недоверенным до проверки и ревью**.

## Рекомендуемые правила

* использовать Git;
* держать важные проекты под контролем версий;
* использовать `--dry-run` для незнакомых операций;
* проверять изменения перед commit;
* использовать доверенные LLM endpoints;
* не отправлять секреты внешним моделям;
* проверять task-файлы перед выполнением;
* не считать успешную компиляцию доказательством корректности.

## Sandbox

Обычная генерация и проверка кода выполняются во временной рабочей области до применения изменений к реальному проекту.

Это уменьшает вероятность немедленного попадания сломанного кода в рабочее дерево.

Однако sandbox **не является полноценным security container, VM или изолированной средой**.

Сгенерированная программа всё ещё может выполнять операции, разрешённые ОС и текущему пользователю.

## Защита путей

Перед применением изменений пути проверяются, чтобы не допустить path traversal за пределы корня проекта.

## Computer mode

Computer mode значительно мощнее, поскольку выполняет реальные системные команды.

Используйте его только при необходимости и проверяйте план перед выполнением.

---

# Устранение проблем

## `unsupported provider`

Используйте поддерживаемый формат:

```bash
--provider ollama
```

или:

```bash
--provider http://localhost:11434
```

или:

```bash
--provider openai+https://api.example.com/v1
```

или:

```bash
--provider openai-compatible+http://localhost:8000/v1
```

## Ollama недоступен

Запустите:

```bash
ollama serve
```

Проверка:

```bash
./gogitor tui \
  --provider http://127.0.0.1:11434 \
  --model gemma3:4b
```

## Не проходит build

Проверьте проект отдельно:

```bash
go build ./...
```

Для ошибки, появившейся из-за сгенерированного кода:

```bash
./gogitor fix \
  "вставьте сюда ошибку сборки"
```

## Не проходят тесты

Запустите:

```bash
./gogitor test --json
```

Временный пропуск тестов:

```bash
./gogitor code "task" --no-tests
```

Это не должно использоваться вместо полноценной проверки.

## Не установлен `golangci-lint`

Установите:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

Затем:

```bash
./gogitor test lint
```

## Недостаточно контекста

Увеличьте его:

```bash
./gogitor code \
  "рефакторинг всего проекта" \
  --max-context 262144
```

или:

```json
{
  "max_context_tokens": 262144
}
```

Фактический размер доступного контекста зависит от модели и провайдера.

## Проверить текущую конфигурацию

```bash
./gogitor doctor
```

Подробные логи:

```bash
./gogitor --debug
```

---

# Разработка

Сборка:

```bash
go build ./...
```

Тесты:

```bash
go test ./...
```

Статический анализ:

```bash
go vet ./...
```

Форматирование:

```bash
gofmt -w .
```

Lint:

```bash
golangci-lint run ./...
```

Для изменений, затрагивающих качество генерации кода, рекомендуется полный цикл:

```bash
go build ./...
go test ./...
go vet ./...
golangci-lint run ./...
```

---

# Лицензия

Gogitor распространяется под лицензией **BSD 3-Clause**.

Полный текст:

[LICENSE.txt](LICENSE.txt)

---

# Участие в разработке

Приветствуются:

* Issues;
* bug reports;
* предложения функций;
* Pull Requests.

При описании проблемы желательно указать:

* версию Gogitor;
* провайдера и модель;
* команду или TUI-операцию;
* сообщение об ошибке;
* вывод `doctor`;
* шаги воспроизведения;
* затронутую часть проекта.

Не публикуйте в Issues и Pull Requests API-ключи, GitHub tokens, пароли и другие секреты.
