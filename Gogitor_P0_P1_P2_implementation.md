# Gogitor — реализация полного P0 + P1 + P2 для надёжности DIFF/PATCH

Документ описывает изменения относительно исходного дерева проекта из `gogitor(20260903-133507).txt`.

## 1. Концептуальная схема

Существующий подход не меняется:

```text
LLM
 ↓
Patch Proposal
 ↓
Snapshot / stale check
 ↓
Deterministic Preflight
 ↓
Symbol / scope / size guards
 ↓
Rebase → AST-aware fuzzy → legacy fuzzy
 ↓
Patch Auditor (для выбранных моделей/режимов)
 ↓
Temporary Workspace
 ↓
gofmt
 ↓
affected package tests
 ↓
full build / tests / vet / lint
 ↓
CopyToRootSafe
 ↓
Git diff / existing validation / commit
```

Ключевой принцип: LLM по-прежнему предлагает изменение, но Gogitor сам определяет, когда и где это изменение имеет право попасть в исходник.

## 2. Изменённые файлы

```text
internal/domain/domain.go
internal/workspace/patch_engine.go
internal/workspace/workspace.go
internal/workspace/patch_safety.go          NEW
internal/workspace/patch_safety_test.go     NEW
internal/codegen/codegen.go
internal/codegen/codegen_test.go
internal/config/config.go
internal/prompts/prompts.go
internal/runner/runner.go
internal/app/app.go
internal/app/patch_safety.go                 NEW
```

Новых внешних Go-зависимостей не добавляется.

## 3. P0

### P0.1 File snapshot / stale patch

В `internal/workspace/patch_safety.go` добавлены:

```go
func (w *Workspace) CaptureProjectSnapshot() (map[string]string, error)
func (w *Workspace) BindSourceSnapshots(...)
```

Snapshot создаётся до запроса к LLM. В нём хранится SHA-256 каждого файла, который Gogitor потенциально может изменить.

Это важно: snapshot делается для всего рабочего дерева, а не только для `ExistingTargets`. Иначе модель могла бы сгенерировать патч другого файла, а его версия была бы проверена только в момент парсинга.

В `domain.FileChange` добавляются:

```go
SourceHash      string `json:"-"`
ExpectedPresent bool   `json:"-"`
ExpectedAbsent  bool   `json:"-"`
```

В `domain.Patch`:

```go
ReplaceOnly               bool   `json:"replace_only,omitempty"`
ExpectedSourceHash        string `json:"expected_source_hash,omitempty"`
ExpectedSymbolFingerprint string `json:"expected_symbol_fingerprint,omitempty"`
```

`SourceHash`, `ExpectedPresent`, `ExpectedAbsent` никогда не должны приходить от LLM.

### P0.2 Exact/relaxed/normalized uniqueness

Существующая логика `applyPatchText` уже отклоняет неоднозначный exact/relaxed/normalized match. Она сохраняется.

Главное изменение — не переходить сразу к fuzzy:

```text
exact
 ↓
relaxed
 ↓
normalized
 ↓
REBASE
 ↓
AST-AWARE FUZZY
 ↓
legacy FUZZY
```

### P0.3 Более строгий Symbol identity

В `internal/workspace/patch_engine.go` полностью заменить существующий `findSymbolRange` на версию из patch-файла.

Теперь поддерживаются:

```text
Function
Receiver.Method
package.Function
package.Receiver.Method
TypeName
package.TypeName
```

Для Function/Method дополнительно проверяется сигнатура AST. Это закрывает ситуацию, когда одинаковое имя встречается в разных контекстах.

### P0.4 Symbol fingerprint

Перед генерацией патча:

```go
fp, err := SymbolFingerprint(string(data), p.Symbol)
```

Fingerprint строится по нормализованному AST-исходнику символа.

Перед применением проверяется:

```text
current symbol fingerprint == expected fingerprint
```

Если нет — патч считается устаревшим и не применяется.

### P0.5 Patch preflight

Новый публичный вход:

```go
func (w *Workspace) PreflightChanges(
    dir string,
    changes []domain.FileChange,
    policy PatchPolicy,
    minConfidenceOverride float64,
) (*PatchPreflightReport, error)
```

Он не изменяет файлов.

Он проверяет:

```text
path
snapshot
expected-present/expected-absent
number of patch blocks
SEARCH/REPLACE
Symbol
Symbol fingerprint
actual patch application
changed-line limits
aggregate changed-line limits
semantic scope
imports
Go module requirements
public API
```

### P0.6 Rebase перед fuzzy

`findRebasedBlock()` использует уникальный строковый anchor, но перед применением дополнительно проверяет:

```text
same declaration identity
normalized line similarity >= 0.72
Go token similarity
```

Таким образом, например:

```go
func nonexistent() { ... }
```

не сможет «переехать» в:

```go
func main() { ... }
```

### P0.7 Atomic / transactional apply

`Workspace` получает:

```go
applyMu sync.Mutex
```

`ApplyChangesSmartWithPolicy` теперь сначала вызывает `prepareChanges()` для всех файлов, полностью строит итоговые версии в памяти, и только после успешного прохождения всех проверок начинает запись.

Для записи используются временные файлы + `os.Rename`, а при ошибке выполняется rollback.

`CopyToRootSafe` тоже проверяет snapshot непосредственно перед изменением настоящего проекта.

Важно: это транзакционность с rollback, а не физическая filesystem transaction нескольких файлов.

## 4. P1

### P1.1 Semantic diff guard

Новый:

```go
func ValidateSemanticDiff(before, after string, patches []domain.Patch, path string) error
```

Он сравнивает AST fingerprints top-level Go symbols до/после patch.

Если задача меняет:

```text
Handler.ServeHTTP
```

а одновременно скрыто изменился:

```text
Config.Load
```

patch отклоняется.

### P1.2 Import/dependency guard

Новый:

```go
func ValidateImportGuard(...)
func ValidateGoModGuard(...)
```

Для `.go` отслеживаются новые внешние imports.

Для `go.mod` отслеживаются новые `require` entries; новая зависимость должна реально присутствовать внутри предложенного изменения.

Окончательная проверка всё равно остаётся за `go mod tidy` + `go build`.

### P1.3 Affected package tests

В `internal/runner/runner.go` добавлен:

```go
func (r *Runner) TestPackageDirs(
    ctx context.Context,
    dir string,
    packageDirs []string,
) (domain.TestsStatus, error)
```

После patch сначала тестируются затронутые пакеты:

```text
patch
 ↓
affected package tests
 ↓ success
full test suite
```

### P1.4 Public API guard

Новый:

```go
func ValidatePublicAPIGuard(before, after string, patches []domain.Patch, path string) error
```

Он следит за:

```text
exported functions
exported methods
exported types
exported vars/consts
```

Непредусмотренное изменение публичного API отклоняется ещё до build.

### P1.5 AST-aware fuzzy

Добавлен `findASTAwareBlock()`.

Он сравнивает AST nodes через token multisets, а не только положения строк.

При этом полноценные `func`/`type` declarations не разрешается fuzzy-подставлять вместо других declarations — для них применяется более строгая identity-проверка.

## 5. P2

### P2.1 REPLACE_ONLY

LLM для маленьких локальных моделей получает новый протокол:

```text
--- Patch: main.go ---
--- Symbol: main ---
<<<<<<< REPLACE_ONLY
func main() {
    println("world")
}
>>>>>>> REPLACE_ONLY
```

Важнейшее отличие:

```text
LLM НЕ ПОВТОРЯЕТ SEARCH
```

### P2.2 Gogitor сам создаёт SEARCH

`PreparePatchForContent()` делает:

```text
Symbol
 ↓
AST
 ↓
current declaration
 ↓
exact SEARCH
 ↓
обычный apply engine
```

Это удаляет целый класс ошибок, когда 4B/8B/12B модель неверно копирует старый код в SEARCH.

### P2.3 Patch Auditor

Добавлен `internal/app/patch_safety.go`.

Auditor использует существующую инфраструктуру `RoleReviewer` и `sendAgentJSON`, поэтому новый агентный механизм не вводится. Существующий проект уже имеет reviewer role и JSON repair pipeline.

Auditor отвечает строго за:

```json
{
  "approved": true,
  "scope_ok": true,
  "symbol_ok": true,
  "unrelated_changes": false,
  "reason": ""
}
```

Он не занимается redesign кода.

### P2.4 Model-specific patch protocol

Добавлено:

```go
PatchProtocolForModel(provider, model, override)
```

Текущая стратегия:

```text
local <= 12B        → REPLACE_ONLY
20B+                → SEARCH/REPLACE
cloud/OpenAI        → SEARCH/REPLACE
explicit override   → override wins
```

Конфигурация:

```json
{
  "patch_protocol_mode": "auto",
  "patch_auditor_mode": "auto"
}
```

Также доступны:

```text
patch_protocol_mode = auto | search_replace | replace_only
patch_auditor_mode  = off | auto | always
```

Переменные окружения:

```text
GOGITOR_PATCH_PROTOCOL
GOGITOR_PATCH_AUDITOR
```

## 6. Критически важное изменение поведения fallback

Для `REPLACE_ONLY` автоматический переход к полному переписыванию существующего файла запрещён.

То есть:

```text
REPLACE_ONLY
 ↓
repair attempts
 ↓
если не получилось
 ↓
STOP / report failure
```

а не:

```text
REPLACE_ONLY
 ↓
repair failed
 ↓
full-file rewrite
```

Именно это не позволяет новой safety-механике снова попасть в первоначальную проблему Gogitor.

Для legacy `SEARCH/REPLACE` существующая возможность полного fallback сохраняется.

## 7. Точные места интеграции в app.go

В `executeSimple`:

1. Сразу после `buildCodeContext()` выбрать protocol:

```go
patchProtocol := workspace.PatchProtocolForModel(
    s.Cfg.Provider,
    s.Cfg.Model,
    s.Cfg.PatchProtocolMode,
)
```

2. До первого LLM request:

```go
patchSnapshot, snapshotErr := s.WS.CaptureProjectSnapshot()
```

3. Для первого patch prompt использовать:

```go
prompts.CodeModifyDiffForModelWithProtocol(
    query,
    originalContext,
    patchPolicy.String(),
    patchProtocol.String(),
)
```

4. После `codegen.Validate()`:

```go
changes, err = s.WS.BindSourceSnapshots(changes, patchSnapshot)
```

5. Перед apply:

```go
report, err := s.WS.PreflightChanges(
    sandbox,
    changes,
    patchPolicy,
    s.Cfg.FuzzyMinConfidence,
)
```

6. После deterministic preflight при необходимости:

```go
audit, err := s.auditPatch(...)
```

7. После успешного patch apply:

```go
affectedDirs := workspace.AffectedPackageDirs(sandbox, changes)
targeted, err := s.Runner.TestPackageDirs(ctx, sandbox, affectedDirs)
```

8. Только затем остаётся полный `Runner.Test()`.

## 8. Изменения prompts.go

Старые API сохраняются как compatibility wrappers:

```go
CodeModifyDiffForModel(...)
CodeFixPatch(...)
```

Новые API:

```go
CodeModifyDiffForModelWithProtocol(...)
CodeFixPatchWithProtocol(...)
PatchAudit(...)
```

Это важно для сохранения существующих вызовов и unit-тестов.

## 9. Parser

В `internal/codegen/codegen.go` добавляются маркеры:

```text
<<<<<<< REPLACE_ONLY
>>>>>>> REPLACE_ONLY
```

Результат сохраняется в:

```go
Patch.ReplaceOnly
```

При этом старый `SEARCH/REPLACE` формат полностью сохраняется.

## 10. Unit tests

Добавлены проверки:

```text
REPLACE_ONLY parsing
REPLACE_ONLY → exact SEARCH
model-specific protocol
project snapshot
stale file
deleted expected file
semantic scope
import scope
public API scope
affected package list
transactional preflight
```

## 11. Проверка результата

На подготовленном дереве были успешно выполнены:

```bash
gofmt -l <изменённые файлы>

go test ./internal/workspace \
       ./internal/codegen \
       ./internal/domain \
       ./internal/prompts \
       ./internal/config \
       ./internal/runner -count=1
```

Также unified diff прошёл:

```bash
patch -p1 --dry-run < gogitor_p0_p1_p2.patch
```

В результате все 12 затронутых файлов корректно проходят dry-run.

## 12. Что ещё требуется проверить уже на машине проекта

Полный `go test ./...` и `go build ./...` следует выполнить в реальном репозитории, где есть исходный `go.mod` и все зависимости. В предоставленном dump `go.mod` отсутствует, а среда проверки не могла загрузить недостающие внешние модули.

После применения патча выполнить:

```bash
gofmt -w \
  internal/domain/domain.go \
  internal/workspace/patch_engine.go \
  internal/workspace/workspace.go \
  internal/workspace/patch_safety.go \
  internal/workspace/patch_safety_test.go \
  internal/codegen/codegen.go \
  internal/codegen/codegen_test.go \
  internal/config/config.go \
  internal/prompts/prompts.go \
  internal/runner/runner.go \
  internal/app/app.go \
  internal/app/patch_safety.go

go test ./...
go build -o gogitor ./cmd/gogitor/
```
