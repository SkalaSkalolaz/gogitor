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
6. Use only the standard library. Only "testing" is imported.
7. Do not create helper functions.
8. The generated code MUST compile inside the given package.
`)

	return b.String()
}