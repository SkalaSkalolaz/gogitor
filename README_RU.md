# Gogitor — AI-ассистент программиста для Go

[![License](https://img.shields.io/badge/License-BSD--3--Clause-blue.svg)](LICENSE.txt)
[![Go](https://img.shields.io/badge/Go-1.25.1-00ADD8.svg)](https://go.dev/)
[![Version](https://img.shields.io/badge/version-1.0-blue.svg)](https://github.com/SkalaSkalolaz/gogitor)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20WSL-lightgrey.svg)](#требования)

[English version](README.md)

**Gogitor** — интеллектуальный терминальный ассистент программиста для разработки на Go. Он объединяет классический CLI, интерактивный TUI, генерацию и анализ кода с учётом проекта, автоматическую валидацию, multi-agent разработку, Workflow, интеграцию с Git/GitHub, веб-поиск, индексирование проекта, reasoning, анализ изображений, автономные инженерные инструменты и управление компьютером.

Gogitor ориентирован прежде всего на проекты на **Go** и умеет работать как с локальными LLM через **Ollama**, так и с удалёнными моделями через **OpenAI-совместимые API**.

Программа может определить намерение пользователя, изучить существующий проект, выбрать релевантный контекст, сгенерировать полные файлы или минимальные патчи, проверить изменения во временной рабочей области, запустить сборку и тесты и только после успешной проверки применить изменения к реальному проекту.

> **Текущая версия исходного кода:** `1.0`
>
> Gogitor прежде всего предназначен для разработки на Go. Интерфейс программы доступен на русском и английском языках.

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
  * [Workflow Mode](#workflow-mode)
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
* [Требования](#требования)
* [Установка](#установка)
* [Быстрый старт](#быстрый-старт)
* [LLM-провайдеры](#llm-провайдеры)
* [Команды CLI](#команды-cli)
* [Параметры CLI](#параметры-cli)
* [Команды TUI](#команды-tui)
* [Режимы выполнения](#режимы-выполнения)
* [Процесс генерации кода](#процесс-генерации-кода)
* [Workflow Mode подробно](#workflow-mode-подробно)
* [Индексирование проекта](#индексирование-проекта)
* [Git и GitHub](#git-и-github-1)
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
        ├── Computer
        └── Анализ изображения
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

Для сложных задач процесс может расширяться до нескольких специализированных агентов или полноценного Workflow с сохраняемыми артефактами.

---

# Архитектура и процесс выполнения

Основные уровни Gogitor:

```text
CLI / TUI
   │
   ▼
Application Service
   │
   ├── Intent Router
   ├── Execution Strategy
   ├── Agent Orchestration
   ├── Workflow Engine
   ├── Autonomy Controller
   └── Computer Controller
   │
   ▼
Контекст проекта / AST-индекс
   │
   ▼
LLM Dispatcher
   │
   ├── Ollama
   └── OpenAI-compatible API
   │
   ▼
Генерация кода / Patch Engine
   │
   ▼
Workspace / Sandbox
   │
   ▼
Runner
   ├── build
   ├── test
   ├── vet
   ├── lint
   └── run
   │
   ▼
Git / GitHub
```

Архитектура программы ориентирована прежде всего на проверку результата, а не на безусловное доверие к сгенерированному коду.

---

# Возможности

## Интерактивный TUI

Gogitor предоставляет интерактивный терминальный интерфейс на базе Bubble Tea.

TUI поддерживает:

* Отображение Markdown
* Историю диалога
* Автодополнение команд
* Потоковую выдачу ответов LLM
* Переключение фокуса
* Выделение текста мышью
* Отображение процесса выполнения
* План выполнения multi-agent задач
* Статусы этапов
* Визуализацию diff
* Информацию о текущем агенте
* Диагностику проекта и LLM
* Русский и английский интерфейс
* Работа с изображениями для vision-capable моделей

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

Gogitor умеет создавать новый Go-код и изменять существующий проект с учётом его текущего состояния.

Поддерживаются:

* Создание новых файлов
* Изменение существующих файлов
* Рефакторинг
* Разделение кода между файлами
* Выделение функций и компонентов
* Реализация новых возможностей
* Исправление ошибок компиляции
* Исправление неудачных тестов
* Выполнение задач из файлов
* Минимальные патчи
* Генерация полного файла, если патч применять небезопасно

Пример:

```bash
./gogitor code "создай REST API с endpoint /health и /version"
```

---

## Patch Engine

Для существующих файлов Gogitor может использовать минимальные `SEARCH/REPLACE`-патчи:

```text
--- Patch: internal/server/server.go ---
<<<<<<< SEARCH
точный существующий код
=======
новый код
>>>>>>> REPLACE
```

Блок `SEARCH` должен соответствовать исходному коду, включая отступы и контекст.

Patch Engine поддерживает:

* Точное совпадение
* Symbol anchors
* Различные политики применения patch
* Strict / Balanced / Advanced режимы
* Проверку confidence для fuzzy matching там, где это разрешено
* Автоматический переход к полному файлу, если patch нельзя безопасно применить

### Политики patch

```text
strict
balanced
advanced
```

Политика может определяться по провайдеру и профилю модели.

Общая логика:

* Для небольших локальных моделей — более строгая политика.
* Для средних моделей — balanced.
* Для сильных облачных и крупных моделей — advanced.

В strict mode автоматический fuzzy matching не используется.

---

## Проверка и исправление ошибок

Изменения не должны сразу попадать в рабочий проект.

Сначала Gogitor работает с временной копией и выполняет необходимые проверки:

```text
go mod init
go mod tidy
gofmt
go build
go test -v -cover
go vet
golangci-lint
```

Результаты тестирования анализируются автоматически.

Gogitor может определить:

* Пройденные тесты
* Неудачные тесты
* Имена тестов
* Связанные функции
* Исходные файлы
* Номера строк
* Сообщения об ошибках
* Информацию о покрытии

Полученные сведения могут передаваться обратно LLM для точечного исправления.

### Исправление ошибок

Команда `fix` предназначена для:

* Ошибок компиляции
* Panic
* Stack trace
* Runtime error
* Ошибок тестов

Например:

```bash
./gogitor fix "panic: runtime error: index out of range"
```

Gogitor также умеет распознавать характерные признаки Go-ошибок:

```text
panic:
runtime error
goroutine
.go:123
--- FAIL
```

---

## Автоматический выбор стратегии выполнения

Для задач генерации кода доступны режимы:

```text
auto
fast
agent
workflow
```

В автоматическом режиме учитываются:

* Сложность задачи
* Локальный или удалённый провайдер
* Профиль модели
* Размер задачи
* Уровень риска
* Явно выбранный пользователем режим

Типичная схема:

```text
Простая задача
      ↓
fast

Средняя задача
      ↓
agent

Большая/сложная задача
      ↓
workflow
```

Для локальных небольших моделей сложные задачи могут направляться сразу в Workflow, чтобы уменьшить риск потери контекста.

Для внешних провайдеров при достаточно сложной задаче Gogitor может дополнительно использовать LLM для выбора стратегии.

Явный выбор режима:

```bash
./gogitor code "исправь модуль авторизации" --mode fast
```

```bash
./gogitor code "отрефакторь слой хранения" --mode agent
```

```bash
./gogitor code "перестрой архитектуру проекта" --mode workflow
```

В TUI:

```text
:fast <задача>
:agent <задача>
:workflow <задача>
```

---

## Multi-Agent разработка

Для сложных задач Gogitor может использовать несколько специализированных агентов:

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

Разбивает задачу на подзадачи и формирует критерии их принятия.

### Coder

Реализует подзадачи и работает с проектной рабочей областью.

### Reviewer

Проверяет:

* Ошибки компиляции
* Некорректную реализацию
* Потенциальные nil dereference
* Проблемы безопасности
* Регрессии
* Несоответствие исходной задаче

### Verifier

Проверяет, действительно ли исходная задача выполнена.

При необходимости может сформировать дополнительную задачу на исправление.

Multi-Agent подсистема также поддерживает:

* Очередь запросов к LLM
* Приоритеты
* Бюджеты для ролей
* Retry
* Exponential backoff
* Статистику использования
* Статистику времени выполнения
* Прогресс и ETA
* Persistent agent memory
* Checkpoint
* Rollback

---

## Workflow Mode

Workflow Mode предназначен для крупных инженерных задач, где важны проверяемость, последовательность и трассируемость.

Запуск:

```bash
./gogitor workflow "создать REST API с авторизацией и тестами"
```

Каждая сессия сохраняется в:

```text
.gogitor/workflow/<timestamp>/
```

Типичные артефакты:

```text
inbox.md
research.md
plan.md
prd.json
process.md
gate-report-task-01.json
gate-report-task-02.json
...
reflection.md
```

Workflow:

1. Уточняет исходную цель.
2. Разбивает задачу на конкретные подзадачи.
3. Выполняет подзадачи через coding agent.
4. Запускает Quality Gates.
5. Сохраняет информацию о выполнении.
6. При включённом автоматическом commit создаёт commit для задачи.
7. Останавливается при провале обязательной проверки.

---

## Интеллектуальный анализ проекта

Gogitor не отправляет в LLM все файлы проекта без разбора.

Он поддерживает AST-based индекс, который помогает выбирать наиболее релевантный контекст.

Индекс учитывает:

* Go-файлы
* Пакеты
* Импорты
* Функции
* Методы
* Связи вызовов
* Структуру проекта
* Важность файлов
* Текстовую релевантность

Для ранжирования используются:

* Граф импортов
* Call Graph
* PageRank
* BM25
* Расширение английских и русских синонимов

Индекс кешируется и обновляется при изменении исходных файлов.

---

## Сравнение подходов

Для достаточно сложной задачи Gogitor может перед началом реализации предложить несколько принципиально разных подходов.

При сравнении учитываются:

* Сложность
* Производительность
* Читаемость
* Зависимости
* Тестируемость
* Компромиссы

Пример:

```text
## Сравнение подходов

| # | Подход          | Сложность | Производительность | Читаемость | Зависимости |
|---|-----------------|-----------|--------------------|------------|-------------|
| 1 | stdlib HTTP mux | низкая    | хорошая             | отличная   | только stdlib |
| 2 | chi router      | средняя   | отличная            | хорошая    | 1 внешняя   |
| 3 | gRPC            | высокая   | отличная            | средняя    | 3 внешние   |
```

Пользователь может:

* Выбрать вариант по номеру
* Принять рекомендацию
* Изменить рекомендованный вариант
* Выбрать другой путь

Отключение:

```bash
./gogitor code "создай HTTP сервер" --no-compare
```

или:

```bash
export GOGITOR_COMPARE_APPROACHES=false
```

---

## Git и GitHub

Поддерживаются основные операции Git:

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

Gogitor может автоматически генерировать commit message на основании реального diff в формате Conventional Commits.

Примеры:

```text
feat(auth): add JWT token validation
fix(runner): handle empty test output
refactor(workspace): extract patch application
test(index): add BM25 ranking coverage
```

### GitHub API

Gogitor умеет взаимодействовать с GitHub через API и токен.

Создание репозитория:

```bash
./gogitor git create myproject \
  --private \
  --desc "My project"
```

Клонирование:

```bash
./gogitor git clone https://github.com/user/repository \
  --key-github ghp_xxx
```

Push:

```bash
./gogitor git push \
  --github https://github.com/user/repository \
  --key-github ghp_xxx
```

Поддерживаются токены форматов:

```text
ghp_...
github_pat_...
```

---

## Веб-поиск

Gogitor умеет выполнять веб-поиск и передавать найденную информацию LLM.

Пример:

```bash
./gogitor search "последняя версия Go"
```

Поисковый механизм включает:

* Ограничение частоты запросов
* Контроль доменов
* SSRF-защиту
* Обнаружение секретов в поисковых запросах
* Санитизацию полученного содержимого
* Защиту от prompt injection
* Явное обозначение полученных страниц как недоверенного содержимого

Автоматический поиск для сложных multi-agent задач:

```bash
./gogitor code \
  "исследуй современные подходы к HTTP routing в Go и реализуй лучший вариант" \
  --auto-search
```

### Важное замечание о приватности

При включённом `--auto-search` и использовании удалённой LLM-провайдера код проекта и связанная с поиском информация могут передаваться внешним сервисам.

Для чувствительных проектов рекомендуется локальный Ollama endpoint.

---

## Статьи и документация

Gogitor умеет создавать:

* Технические статьи
* Инструкции
* Tutorial
* Обзоры
* Истории
* Описания кода
* Другие длинные тексты

Простой режим:

```bash
./gogitor article "как работает garbage collector в Go"
```

Расширенный режим:

```bash
./gogitor article "подробный разбор middleware pattern" --full
```

Поддерживаемые жанры:

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

* Контекст проекта
* Веб-поиск
* План статьи
* Последовательная генерация разделов
* Контекст предыдущих разделов

---

## Анализ состояния проекта

Команда:

```bash
./gogitor suggest
```

проводит целенаправленный анализ состояния проекта.

Рекомендации группируются по категориям:

* Критические проблемы
* Технический долг
* Отсутствующие тесты
* Code smells
* Улучшения

Gogitor стремится указывать конкретные файлы и функции, а не выдавать только общие советы.

---

## Режим размышления

Gogitor поддерживает reasoning/thinking для моделей, которые это умеют.

Поддерживаются соответствующие модели семейств вроде DeepSeek-R1, QwQ, Qwen3 и совместимых OpenAI-style reasoning моделей.

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

Команды TUI:

```text
:reasoning
:reasoning on
:reasoning off
:reasoning router on
:reasoning router off
```

Провайдеры используют соответствующие им механизмы:

* Ollama — параметр `think`
* OpenAI-compatible — `reasoning_effort`

Если модель не поддерживает reasoning, параметр может быть проигнорирован или запрос может быть повторён без него.

---

## Анализ изображений

Команды `ask` и `analyze` могут принимать изображение:

```bash
./gogitor ask "что изображено на этом скриншоте?" \
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

Изображение передаётся vision-capable модели.

Подходит для:

* Скриншотов
* Сообщений об ошибках
* UI
* Архитектурных схем
* Скриншотов кода
* Технических изображений

Требуется совместимая vision-модель.

---

## Режим управления компьютером

Computer mode позволяет Gogitor планировать и выполнять реальные системные команды.

Пример:

```bash
./gogitor computer "покажи использование диска"
```

В TUI:

```text
:computer показать использование диска
```

Режим **отключён по умолчанию**.

Включить его можно через:

```bash
./gogitor computer "покажи самые большие файлы" --computer
```

или:

```bash
export GOGITOR_COMPUTER_ENABLED=true
```

или:

```json
{
  "computer_enabled": true
}
```

Поддерживаются защитные механизмы:

* Блокировка запрещённых команд
* Подтверждение опасных команд
* Запрет command substitution
* Аудит команд
* Возможность разрешения sudo
* Dry-run
* Проверка результата после выполнения

Журнал:

```text
.gogitor/computer_audit.json
```

Флаги:

```text
--computer
--dry-run
--allow-sudo
```

`--allow-sudo` следует включать только при необходимости.

---

## Автономный режим

Autonomy — механизм инженерного мониторинга, который может обнаруживать исправимые проблемы и помещать их в очередь задач.

В TUI:

```text
:autonomy
:autonomy on
:autonomy off
:autonomy status
:autonomy run
:autonomy clear
```

В CLI:

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

По умолчанию автономный режим работает осторожно. Вместо свободной команды вроде «улучши проект» отдельной LLM-задачей передаётся конкретная обнаруженная проблема.

---

## Мутационное тестирование

Мутационное тестирование:

```bash
./gogitor mutate
```

Можно задать лимит:

```bash
./gogitor mutate 10
```

Результат включает:

* Сгенерированные мутации
* Убитые мутации
* Выжившие мутации
* Ошибки
* Mutation Score

Это позволяет оценить, насколько существующие тесты способны обнаруживать реальные изменения в коде.

---

## Автогенерация тестов

Gogitor может найти экспортируемые функции без тестов через AST и сгенерировать для них тесты:

```bash
./gogitor autogen-tests
```

Можно ограничить количество функций:

```bash
./gogitor autogen-tests 3
```

Процесс:

```text
AST-анализ
    ↓
Функции без тестов
    ↓
Генерация теста
    ↓
Создание test-файла
    ↓
Запуск тестов
    ↓
Сохранение только при успешной проверке
```

---

## TODO/FIXME Scanner

Проверка без LLM:

```bash
./gogitor todo
```

Ищутся маркеры:

```text
TODO
FIXME
HACK
XXX
BUG
```

---

## Go Vet

Запуск стандартной проверки:

```bash
./gogitor vet
```

Используется:

```bash
go vet ./...
```

LLM для этой операции не требуется.

---

## Журнал инженерных решений

В multi-agent работе Gogitor может сохранять важные инженерные решения.

Журнал может содержать:

* Принятые решения
* Рассмотренные альтернативы
* Ограничения
* Причины выбора
* Неудачные подходы

Просмотр:

```bash
./gogitor decisions
```

Gogitor также может искать **decision debt** — временные решения, первоначальные ограничения которых могли перестать быть актуальными.

---

# Требования

Gogitor предназначен прежде всего для Unix-подобных сред.

## Обязательно

* **Go 1.25.1** или совместимый Go toolchain
* **Ollama** или **OpenAI-compatible API**
* Доступ к сети при загрузке зависимостей и работе с удалёнными сервисами

## Рекомендуется

* **Git**
* Достаточно мощная coding-модель
* Большой context window для крупных задач

## Поддерживаемые среды

* Linux
* macOS
* Windows через WSL

## Дополнительно

Для lint-проверок требуется:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

---

# Установка

Клонируйте репозиторий:

```bash
git clone https://github.com/SkalaSkalolaz/gogitor.git
cd gogitor
```

Загрузите зависимости:

```bash
go mod tidy
```

Соберите программу:

```bash
go build -o gogitor .
```

Проверьте:

```bash
./gogitor --help
```

```bash
./gogitor version
```

Фактическая версия, заданная текущим исходником:

```text
1.0
```

При необходимости установите бинарник глобально:

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

## 3. Выберите модель

Например:

```bash
./gogitor tui \
  --provider ollama \
  --model gemma3:4b
```

## 4. Создайте код

```bash
./gogitor code "создай CLI-калькулятор на Go"
```

## 5. Проанализируйте проект

```bash
./gogitor analyze "найди потенциальные ошибки и предложи улучшения"
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

Gogitor поддерживает локальные и удалённые endpoint.

## Ollama

Локальный:

```bash
./gogitor tui \
  --provider ollama \
  --model gemma3:4b
```

Удалённый:

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

## OpenAI-совместимые API

HTTPS:

```bash
./gogitor ask "объясни generics в Go" \
  --provider openai+https://api.example.com/v1 \
  --model gpt-4o-mini \
  --key sk-...
```

Обычный OpenAI-compatible endpoint:

```bash
./gogitor code "создай main.go" \
  --provider openai-compatible+http://localhost:8000/v1 \
  --model local-model
```

| Значение                           | Назначение              |
| ---------------------------------- | ----------------------- |
| `ollama`                           | Локальный Ollama        |
| `http://host:11434`                | Ollama-compatible HTTP  |
| `https://host:11434`               | Ollama-compatible HTTPS |
| `openai+https://host/v1`           | OpenAI-compatible API   |
| `openai-compatible+http://host/v1` | OpenAI-compatible API   |

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
gogitor decisions [flags]

gogitor article <topic> [--full] [flags]

gogitor workflow <task> [flags]
gogitor workflow interview <task> [flags]
gogitor workflow reflect [flags]
gogitor workflow pr [flags]

gogitor computer <task> [flags]
gogitor autonomy [on|off|status|run|clear] [flags]
gogitor mutate [limit] [flags]
gogitor autogen-tests [count] [flags]

gogitor git <subcommand> [flags]

gogitor doctor [flags]
gogitor help
```

---

# Параметры CLI

## Общие параметры

| Параметр               | Короткий | Описание                          |
| ---------------------- | :------: | --------------------------------- |
| `--provider <name>`    |   `-p`   | LLM-провайдер                     |
| `--model <model>`      |   `-m`   | Название модели                   |
| `--key <key>`          |   `-k`   | API-ключ LLM                      |
| `--repo <path>`        |   `-r`   | Корень проекта                    |
| `--image <path>`       |          | Изображение для `ask` / `analyze` |
| `--github <url>`       |          | URL GitHub-репозитория            |
| `--key-github <token>` |          | GitHub token                      |
| `--max-context <n>`    |          | Максимальный размер контекста     |
| `--auto-search`        |          | Автопоиск в multi-agent режиме    |
| `--output <file>`      |   `-o`   | Сохранить результат               |
| `--debug`              |          | Подробное логирование             |
| `--raw`                |          | Только содержимое результата      |
| `--pretty`             |          | Человекочитаемый вывод            |
| `--help`               |   `-h`   | Показать справку                  |

## Reasoning

| Параметр                                 | Описание                             |
| ---------------------------------------- | ------------------------------------ |
| `--reasoning`                            | Включить reasoning                   |
| `--reasoning-effort <low\|medium\|high>` | Глубина размышления                  |
| `--reasoning-budget <n>`                 | Максимум токенов reasoning           |
| `--reasoning-show`                       | Показывать reasoning                 |
| `--reasoning-router`                     | Включить reasoning для intent router |

## Computer mode

| Параметр       | Описание                               |
| -------------- | -------------------------------------- |
| `--computer`   | Включить computer mode                 |
| `--dry-run`    | Показать/проверить план без выполнения |
| `--allow-sudo` | Разрешить sudo                         |

## Генерация кода

| Параметр                               | Описание                           |
| -------------------------------------- | ---------------------------------- |
| `--mode <auto\|fast\|agent\|workflow>` | Явный режим выполнения             |
| `--dry-run`                            | Проверить изменения без применения |
| `--no-commit`                          | Отключить автоматический commit    |
| `--no-tests`                           | Пропустить тесты                   |
| `--no-compare`                         | Отключить сравнение подходов       |
| `--json`                               | Вывод JSON                         |

## Task-файлы

| Параметр | Описание                        |
| -------- | ------------------------------- |
| `--code` | Принудительно выбрать code mode |
| `--json` | Вывод JSON                      |

Параметры можно размещать до или после текста задачи.

Если задача сама начинается с `-`, используйте `--`:

```bash
./gogitor code -- --fix something
```

---

# Команды TUI

| Команда                      | Описание                           |
| ---------------------------- | ---------------------------------- |
| `:help`                      | Показать справку                   |
| `:clear`                     | Очистить контекст текущего диалога |
| `:cls`                       | Очистить экран                     |
| `:code <task>`               | Создать или изменить код           |
| `:fast <task>`               | Принудительный fast mode           |
| `:agent <task>`              | Принудительный agent mode          |
| `:fix <error>`               | Исправить ошибку                   |
| `:ask <question>`            | Chat                               |
| `:analyze <task>`            | Анализ без изменения файлов        |
| `:search <query>`            | Веб-поиск                          |
| `:run [file]`                | Запустить проект                   |
| `:test`                      | Запустить тесты                    |
| `:test lint`                 | Запустить lint                     |
| `:vet`                       | Запустить go vet                   |
| `:todo`                      | Найти TODO/FIXME/HACK              |
| `:git <subcommand>`          | Git-операция                       |
| `:save <file>`               | Сохранить результат                |
| `:article <topic>`           | Создать статью                     |
| `:suggest`                   | Анализ состояния проекта           |
| `:decisions`                 | Журнал решений                     |
| `:reasoning`                 | Состояние reasoning                |
| `:reasoning on`              | Включить reasoning                 |
| `:reasoning off`             | Выключить reasoning                |
| `:reasoning router on`       | Включить reasoning router          |
| `:reasoning router off`      | Выключить reasoning router         |
| `:computer <task>`           | Выполнить системную задачу         |
| `:autonomy`                  | Статус автономного режима          |
| `:autonomy on`               | Запустить монитор                  |
| `:autonomy off`              | Остановить монитор                 |
| `:autonomy run`              | Выполнить очередь                  |
| `:autonomy clear`            | Очистить очередь                   |
| `:mutate [limit]`            | Mutation testing                   |
| `:autogen-tests [n]`         | Автогенерация тестов               |
| `:workflow <task>`           | Запустить Workflow                 |
| `:workflow interview <task>` | Интервью перед Workflow            |
| `:workflow reflect`          | Ретроспектива                      |
| `:workflow pr`               | Pull Request из Workflow           |
| `:quit` / `:q`               | Выход                              |

## Горячие клавиши

| Клавиша         | Действие                                   |
| --------------- | ------------------------------------------ |
| `Enter`         | Отправить ввод                             |
| `Alt+Enter`     | Новая строка                               |
| `Tab`           | Переключить фокус / принять автодополнение |
| `Esc`           | Вернуться к вводу                          |
| `PgUp` / `PgDn` | Прокрутка                                  |
| `F2`            | Режим выделения мышью                      |
| `Ctrl+C`        | Отменить операцию или выйти                |

---

# Режимы выполнения

## `auto`

Режим по умолчанию.

Gogitor самостоятельно выбирает стратегию.

## `fast`

Для простых небольших задач:

```bash
./gogitor code "переименуй эту функцию" --mode fast
```

## `agent`

Для многошаговых изменений:

```bash
./gogitor code "отрефакторь модуль авторизации" --mode agent
```

## `workflow`

Для крупных и архитектурных задач:

```bash
./gogitor code "перестрой архитектуру хранения данных" --mode workflow
```

---

# Процесс генерации кода

## 1. Анализ задачи

Gogitor определяет тип запроса и выбирает режим выполнения.

## 2. Формирование контекста

Индекс проекта выбирает наиболее релевантные исходные файлы.

## 3. Генерация изменений

LLM может вернуть полные файлы:

```text
--- File: main.go ---
package main
...
```

или минимальный patch:

```text
--- Patch: internal/server/server.go ---
<<<<<<< SEARCH
старый код
=======
новый код
>>>>>>> REPLACE
```

## 4. Временная рабочая область

Текущий проект копируется во временную директорию.

## 5. Применение изменений

Изменения применяются сначала к копии.

## 6. Проверка

В зависимости от задачи выполняются:

```text
go mod init
go mod tidy
gofmt
go build
go test -v -cover
go vet
golangci-lint
```

## 7. Применение результата

После успешной проверки изменения переносятся в настоящий проект.

## 8. Git

При включённом автоматическом commit Gogitor создаёт сообщение по фактическому diff и выполняет commit.

---

# Workflow Mode подробно

Workflow создаёт отдельный набор инженерных артефактов:

```text
.gogitor/workflow/<timestamp>/
├── inbox.md
├── research.md
├── plan.md
├── prd.json
├── process.md
├── gate-report-task-01.json
├── gate-report-task-02.json
└── reflection.md
```

## Workflow Interview

Запуск:

```bash
./gogitor workflow interview \
  "добавить кэширование в API layer"
```

Planner задаёт уточняющие вопросы.

Пример ответов:

```text
1: использовать in-memory cache
2: кэшировать только GET
3: очищать кэш после записи
```

Можно использовать:

```text
skip
```

или:

```text
go
```

для принятия предложенных значений по умолчанию.

## Workflow Reflection

После завершения:

```bash
./gogitor workflow reflect
```

Рефлексия анализирует артефакты последней сессии.

Результат сохраняется в:

```text
reflection.md
```

## Workflow Pull Request

Создание PR:

```bash
./gogitor workflow pr
```

Для этого требуются:

* Git-репозиторий
* remote `origin`
* GitHub token

В PR могут включаться:

* Исходная цель
* План
* Список задач
* Commit
* Quality Gates
* Журнал выполнения
* Reflection

---

# Индексирование проекта

Индекс строит связи:

```text
Файлы
 │
 ├── Пакеты
 ├── Импорты
 ├── Функции
 ├── Методы
 └── Вызовы
```

## Граф импортов

Показывает зависимости между пакетами и файлами.

## Call Graph

Помогает определять связи между функциями и методами.

## PageRank

Используется для поиска структурно важных файлов.

## BM25

Ранжирует файлы по текстовой релевантности задаче.

## Русский и английский

Для поиска используется расширение синонимов на русском и английском языках.

Индекс кешируется и обновляется при изменении исходных файлов.

---

# Git и GitHub

Основные операции:

```bash
./gogitor git status
./gogitor git diff
./gogitor git diff-task
./gogitor git commit
./gogitor git init
./gogitor git log
./gogitor git checkout
./gogitor git branch
./gogitor git merge
./gogitor git push
./gogitor git pull
./gogitor git fetch
./gogitor git clone <url>
./gogitor git remote
```

GitHub поддерживает операции создания репозитория, Pull Request и связанные операции, предусмотренные текущим CLI.

Не храните токены в исходном коде или в файлах, попадающих в Git.

---

# Конфигурация

Конфигурация загружается в следующем порядке:

1. Значения по умолчанию
2. Глобальная конфигурация
3. Переменные окружения
4. `.gogitor.json`
5. Параметры CLI

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
  "dry_run": false,
  "compare_approaches": true,
  "auto_search": false,
  "raw_output": false,
  "reasoning_enabled": false,
  "reasoning_effort": "medium",
  "reasoning_budget": 0,
  "reasoning_show": false,
  "reasoning_router": false,
  "workflow_mode": "auto",
  "workflow_model_profile": "auto",
  "workflow_local_complex_threshold": 6,
  "workflow_ask_user": true,
  "deps_mode": "auto",
  "confirm_apply": false,
  "fuzzy_min_confidence": 0,
  "computer_enabled": false,
  "computer_allow_sudo": false,
  "computer_confirm_high": true,
  "computer_command_timeout": 120,
  "autonomy_enabled": false,
  "autonomy_mode": "suggest",
  "autonomy_interval_sec": 60,
  "autonomy_mutation_limit": 20
}
```

Указывать все параметры необязательно — отсутствующие значения берутся из конфигурации по умолчанию.

## Контекст модели

Максимальный контекст можно задать так:

```bash
./gogitor code "рефакторинг всего проекта" \
  --max-context 262144
```

или:

```json
{
  "max_context_tokens": 262144
}
```

При значении `0` используется автоматический режим.

Фактический объём доступного контекста зависит от выбранной модели и провайдера.

---

# Переменные окружения

| Переменная                        | Назначение                              |
| --------------------------------- | --------------------------------------- |
| `GOGITOR_PROVIDER`                | Провайдер LLM                           |
| `GOGITOR_MODEL`                   | Модель                                  |
| `GOGITOR_API_KEY`                 | API-ключ                                |
| `OPENAI_API_KEY`                  | Резервный ключ OpenAI-compatible        |
| `GOGITOR_OLLAMA_URL`              | URL Ollama                              |
| `GOGITOR_LOG_LEVEL`               | Уровень логирования                     |
| `GOGITOR_DEBUG`                   | Debug-режим                             |
| `GOGITOR_DRY_RUN`                 | Dry-run                                 |
| `GOGITOR_RAW`                     | Raw output                              |
| `GOGITOR_LLM_TIMEOUT`             | Таймаут LLM                             |
| `GOGITOR_MAX_ITERATIONS`          | Максимальное число итераций исправления |
| `GOGITOR_AUTO_GIT_COMMIT`         | Автоматические commit                   |
| `GOGITOR_GIT_AUTO_INIT`           | Автоинициализация Git                   |
| `GOGITOR_MULTI_AGENT`             | Multi-agent                             |
| `GOGITOR_COMPARE_APPROACHES`      | Сравнение подходов                      |
| `GOGITOR_MAX_CONTEXT_TOKENS`      | Максимальный context                    |
| `GOGITOR_GITHUB_URL`              | GitHub URL                              |
| `GOGITOR_GITHUB_TOKEN`            | GitHub token                            |
| `GITHUB_TOKEN`                    | Резервный GitHub token                  |
| `GOGITOR_AUTO_SEARCH`             | Автоматический поиск                    |
| `GOGITOR_DEPS_MODE`               | Режим разрешения зависимостей           |
| `GOGITOR_CONFIRM_APPLY`           | Подтверждение применения                |
| `GOGITOR_COMPUTER_ENABLED`        | Computer mode                           |
| `GOGITOR_COMPUTER_ALLOW_SUDO`     | Разрешение sudo                         |
| `GOGITOR_REASONING`               | Reasoning                               |
| `GOGITOR_REASONING_EFFORT`        | `low`, `medium`, `high`                 |
| `GOGITOR_REASONING_BUDGET`        | Бюджет reasoning                        |
| `GOGITOR_REASONING_ROUTER`        | Reasoning для router                    |
| `GOGITOR_AUTONOMY`                | Autonomy                                |
| `GOGITOR_AUTONOMY_MODE`           | Режим autonomy                          |
| `GOGITOR_AUTONOMY_INTERVAL`       | Интервал мониторинга                    |
| `GOGITOR_AUTONOMY_MUTATION_LIMIT` | Лимит mutation testing                  |

---

# Примеры

## Ollama

```bash
./gogitor tui \
  --provider ollama \
  --model gemma3:4b
```

## Удалённый Ollama

```bash
./gogitor tui \
  --provider http://192.168.1.10:11434 \
  --model gemma3:4b
```

## OpenAI-compatible API

```bash
./gogitor ask "объясни generics в Go" \
  --provider openai+https://api.example.com/v1 \
  --model gpt-4o-mini \
  --key sk-...
```

## Генерация кода

```bash
./gogitor code "создай REST API с endpoint /health и /version"
```

## Анализ проекта

```bash
./gogitor analyze "найди потенциальные ошибки и предложи улучшения"
```

## Анализ изображения

```bash
./gogitor analyze \
  "найди ошибку на скриншоте" \
  --image screenshot.png
```

## Принудительный режим

```bash
./gogitor code "исправь parser" --mode fast
```

```bash
./gogitor code "отрефакторь parser" --mode agent
```

```bash
./gogitor code "перестрой архитектуру parser" --mode workflow
```

## Dry-run

```bash
./gogitor code "рефакторинг main.go" --dry-run
```

## Без автоматического commit

```bash
./gogitor code "раздели код на пакеты" --no-commit
```

## Без тестов

```bash
./gogitor code "добавь логирование" --no-tests
```

## Без сравнения подходов

```bash
./gogitor code "создай HTTP сервер" --no-compare
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
echo "напиши hello world на Go" | ./gogitor code --raw > main.go
```

```bash
./gogitor ask "объясни context.Context" --raw
```

## Сохранение результата

```bash
./gogitor ask "объясни context.Context" --output answer.md
```

```bash
./gogitor code "создай hello world" --output main.go
```

```bash
./gogitor test --output report.json
```

## Большой контекст

```bash
./gogitor code "рефакторинг всего проекта" \
  --provider ollama \
  --model llama3.3:70b \
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

## Решения

```bash
./gogitor decisions
```

## Mutation testing

```bash
./gogitor mutate 10
```

## Генерация тестов

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

## Workflow

```bash
./gogitor workflow \
  "создать REST API с авторизацией и тестами"
```

## Workflow Interview

```bash
./gogitor workflow interview \
  "добавить кэширование в API layer"
```

## Workflow Reflection

```bash
./gogitor workflow reflect
```

## Workflow Pull Request

```bash
./gogitor workflow pr \
  --key-github ghp_xxx
```

## Диагностика

```bash
./gogitor doctor
```

---

# Диагностика

Если Gogitor работает неожиданно:

```bash
./gogitor doctor
```

Диагностика может показывать:

* Активный провайдер
* Активную модель
* Эффективный размер контекста
* Рабочую директорию
* Конфигурационные пути
* Расположение логов
* Таймауты
* Включённые возможности
* Параметры reasoning
* Состояние computer/autonomy

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
│
├── internal/
│   ├── app/
│   │   Оркестрация приложения
│   │
│   ├── agent/
│   │   LLM dispatcher, очереди, бюджеты, retry и agent memory
│   │
│   ├── autonomy/
│   │   Автономная инженерия, mutation testing, генерация тестов
│   │
│   ├── codegen/
│   │   Парсинг и применение файлов и patch LLM
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

* Создавать код
* Изменять файлы
* Выполнять сгенерированные программы
* Запускать сборку и тесты
* Работать с Git
* Выполнять веб-поиск
* В computer mode выполнять реальные команды операционной системы

Поэтому сгенерированный LLM код следует считать **недоверенным до тех пор, пока он не проверен и не просмотрен человеком**.

## Рекомендуемые меры

* Используйте Git.
* Важные проекты храните под контролем версий.
* Для незнакомых задач используйте `--dry-run`.
* Просматривайте изменения перед commit.
* Используйте доверенные LLM endpoint.
* Не передавайте секреты внешним моделям.
* Проверяйте task-файлы перед запуском.
* Не считайте успешную компиляцию доказательством корректности решения.

## Sandbox

Генерация и проверка выполняются во временной рабочей области.

Изменения сначала применяются к копии проекта и проверяются там.

Только после успешной валидации они могут быть перенесены в настоящий проект.

При этом sandbox **не является полноценным контейнером безопасности, виртуальной машиной или абсолютной границей изоляции**.

Сгенерированная программа всё ещё может выполнять операции, разрешённые текущей операционной системой и учётной записью.

## Защита путей

Перед применением изменений Gogitor проверяет пути файлов и блокирует path traversal за пределы корня проекта.

## Computer mode

Computer mode обладает существенно большими полномочиями, поскольку выполняет реальные команды системы.

Включайте его только при необходимости и проверяйте план перед выполнением.

---

# Устранение проблем

## `unsupported provider`

Используйте:

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

Затем:

```bash
./gogitor tui \
  --provider http://127.0.0.1:11434 \
  --model gemma3:4b
```

## Ошибка сборки

Проверьте проект:

```bash
go build ./...
```

Затем повторите операцию в Gogitor.

Для исправления ошибки:

```bash
./gogitor fix "вставьте сюда ошибку сборки"
```

## Не проходят тесты

Запустите:

```bash
./gogitor test --json
```

Для временного пропуска:

```bash
./gogitor code "задача" --no-tests
```

Пропуск тестов следует использовать только как временную настройку разработки.

## `golangci-lint` не установлен

Установите:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

Затем:

```bash
./gogitor test lint
```

## Недостаточный контекст

Увеличьте его:

```bash
./gogitor code "рефакторинг всего проекта" \
  --max-context 262144
```

или:

```json
{
  "max_context_tokens": 262144
}
```

Фактический объём доступного контекста зависит от модели и провайдера.

## Проверка конфигурации

```bash
./gogitor doctor
```

Для подробных логов:

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

Перед отправкой изменений, особенно затрагивающих генерацию кода, рекомендуется выполнить:

```bash
go build ./...
go test ./...
go vet ./...
golangci-lint run ./...
```

---

# Лицензия

Gogitor распространяется под лицензией **BSD 3-Clause License**.

Полный текст находится в:

```text
LICENSE.txt
```

---

# Участие в разработке

Приветствуются:

* Issue
* Сообщения об ошибках
* Предложения новых функций
* Pull Requests

При создании issue желательно указывать:

* Версию Gogitor
* Версию Go
* Операционную систему
* LLM-провайдера
* Название модели
* Команду, которая выполнялась
* Текст ошибки
* Использовался ли TUI или CLI

Перед созданием Pull Request проверьте:

```bash
go build ./...
go test ./...
go vet ./...
golangci-lint run ./...
```
