package prompts

import (
	"fmt"
	"strings"

	"gogitor/internal/textutil"
)

// UnusualTestPrompt генерирует промпт для написания fuzz-теста.
// Пользователь не должен видеть слово «fuzz» в интерфейсе,
// но внутри целевой тест — это стандартный Go fuzz-таргет.
func UnusualTestPrompt(
	functionName string,
	packageName string,
	source string,
) string {
	var b strings.Builder

	b.WriteString("You are a Go test writer. Write ONE fuzz test for the function below.\n")
	b.WriteString("The test checks that the function does not crash on unusual input.\n\n")

	fmt.Fprintf(&b, "PACKAGE: %s\n", packageName)
	fmt.Fprintf(&b, "FUNCTION: %s\n\n", functionName)

	b.WriteString("SOURCE:\n")
	b.WriteString(textutil.TruncateStringBytes(source, 4000))
	b.WriteString("\n\n")

b.WriteString(`RULES:
1. Return ONLY the Go code of one function. No markdown, no explanations.
2. The function MUST start with: func Fuzz<Name>(f *testing.F) {
3. Use f.Add(...) to provide at least 3 seed values that look like real input.
4. Inside f.Fuzz(func(t *testing.T, ...)) call the target function.
5. The test must FAIL only if the target function PANICS.
   Returning an error is fine. Returning zero values is fine.
6. Standard library only. Allowed imports:
   - "testing" (always)
   - "context" if the target takes a context.Context
   - "fmt" or "errors" if needed to construct seed values
7. Do not create helper functions.
8. The generated code MUST compile inside the given package.
9. If the target takes a context.Context, ALWAYS pass context.Background().
   NEVER pass nil as a context — that will panic inside the target.
10. If the target is a method on a struct receiver, and the package
    provides a New<Type>() constructor for that type, call that
    constructor to create the receiver. Do not use a zero-value receiver
    unless the source clearly shows that this is safe.
11. If the target takes a func parameter, pass nil.
12. If the target takes a slice parameter, include nil and an empty slice
    in the seed values plus 1-2 non-empty slices.
`)

	return b.String()
}