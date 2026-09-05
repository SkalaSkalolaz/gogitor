package workspace

import (
	"testing"

	"gogitor/internal/domain"
)

func testGoModContext() goModImportContext {
	ctx := goModImportContext{
		HasModule: true,
		ModulePaths: map[string]bool{
			"example.com/m/v2": true,
		},
		RequiredModule: map[string]bool{
			"github.com/PuerkitoBio/goquery": true,
		},
	}

	return ctx
}

func TestModuleImportGuardRejectsWrongProjectImport(
	t *testing.T,
) {
	before := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`

	after := `package main

import (
	"fmt"
	"example.com/project/internal/service"
)

func main() {
	fmt.Println(service.Version)
}
`

	patches := []domain.Patch{{
		Symbol: "main",
		Search: `func main() {
	fmt.Println("hello")
}`,
		Replace: `func main() {
	fmt.Println(service.Version)
}`,
	}}

	err := validateModuleImportGuard(
		before,
		after,
		patches,
		"main.go",
		testGoModContext(),
	)

	if err == nil {
		t.Fatal(
			"expected module import mismatch to be rejected",
		)
	}

	if code :=
		domain.PatchErrorCodeFromError(err);
		code != domain.PatchErrorModuleImportMismatch {

		t.Fatalf(
			"error code = %q, want %q",
			code,
			domain.PatchErrorModuleImportMismatch,
		)
	}
}

func TestModuleImportGuardAcceptsLocalModuleImport(
	t *testing.T,
) {
	before := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`

	after := `package main

import (
	"fmt"
	"example.com/m/v2/internal/service"
)

func main() {
	fmt.Println(service.Version)
}
`

	patches := []domain.Patch{{
		Symbol: "main",
		Search: `import "fmt"`,
		Replace: `import (
	"fmt"
	"example.com/m/v2/internal/service"
)`,
	}}

	err := validateModuleImportGuard(
		before,
		after,
		patches,
		"main.go",
		testGoModContext(),
	)

	if err != nil {
		t.Fatalf(
			"unexpected module import error: %v",
			err,
		)
	}
}

func TestModuleImportGuardAcceptsDeclaredDependency(
	t *testing.T,
) {
	before := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`

	after := `package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
)

func main() {
	_ = goquery.Version
	fmt.Println("hello")
}
`

	patches := []domain.Patch{{
		Symbol: "main",
		Search: `import "fmt"`,
		Replace: `import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
)`,
	}}

	err := validateModuleImportGuard(
		before,
		after,
		patches,
		"main.go",
		testGoModContext(),
	)

	if err != nil {
		t.Fatalf(
			"unexpected dependency import error: %v",
			err,
		)
	}
}

func TestModuleImportGuardAcceptsStdlibImport(
	t *testing.T,
) {
	before := `package main

func main() {}
`

	after := `package main

import "net/http"

func main() {
	_ = http.MethodGet
}
`

	patches := []domain.Patch{{
		Symbol: "main",
		Search: `func main() {}`,
		Replace: `func main() {
	_ = http.MethodGet
}`,
	}}

	// Для этого теста import добавляется одновременно,
	// поэтому Symbol patch сам по себе не изменяет import.
	// Проверяем guard непосредственно по before/after.
	err := validateModuleImportGuard(
		before,
		after,
		patches,
		"main.go",
		testGoModContext(),
	)

	if err != nil {
		t.Fatalf(
			"stdlib import must be accepted: %v",
			err,
		)
	}
}

func TestAddedGoImportsReturnsOnlyNewImports(
	t *testing.T,
) {
	before := `package main

import "fmt"

func main() {}
`

	after := `package main

import (
	"fmt"
	"log"
)

func main() {}
`

	added, err := addedGoImports(
		before,
		after,
	)

	if err != nil {
		t.Fatal(err)
	}

	if len(added) != 1 ||
		added[0] != "log" {

		t.Fatalf(
			"added imports = %#v, want [log]",
			added,
		)
	}
}