package workspace

import "testing"

func TestEvaluateTaskEffectiveness_ExtractOK(t *testing.T) {
	before := `package main

func main() {
	http.HandleFunc("/", handler)
}

func handler(w http.ResponseWriter, r *http.Request) {}
`

	after := `package main

func main() {
	registerRoutes()
}

func registerRoutes() {
	http.HandleFunc("/", handler)
}

func handler(w http.ResponseWriter, r *http.Request) {}
`

	task :=
		"Extract HTTP route registration from main() into a separate registerRoutes() function."

	result := evaluateTaskEffectiveness(
		task,
		before,
		after,
	)

	if result.Decision != taskEffectivenessOK {
		t.Fatalf(
			"decision = %q, want OK; reason=%s",
			result.Decision,
			result.Reason,
		)
	}
}

func TestEvaluateTaskEffectiveness_AlreadySatisfied(
	t *testing.T,
) {
	before := `package main

func registerRoutes() {
	http.HandleFunc("/", handler)
}

func main() {
	registerRoutes()
}

func handler(w http.ResponseWriter, r *http.Request) {}
`

	after := `package main

func registerRoutes() {
	http.HandleFunc("/", handler)
}

func main() {
	registerRoutes()
}

func handler(w http.ResponseWriter, r *http.Request) {}
`

	task :=
		"Вынеси регистрацию HTTP-маршрутов из main() в отдельную функцию registerRoutes()."

	result := evaluateTaskEffectiveness(
		task,
		before,
		after,
	)

	if result.Decision !=
		taskEffectivenessAlreadySatisfied {

		t.Fatalf(
			"decision = %q, want ALREADY_SATISFIED; reason=%s",
			result.Decision,
			result.Reason,
		)
	}
}

func TestEvaluateTaskEffectiveness_RejectsCosmeticChange(
	t *testing.T,
) {
	before := `package main

func registerRoutes() {
	http.HandleFunc("/", handler)
}

func main() {
	registerRoutes()
}

func handler(w http.ResponseWriter, r *http.Request) {}
`

	after := `package main

func registerRoutes() {
	http.HandleFunc("/", handler)
}

func main() {
	registerRoutes()

}

func handler(w http.ResponseWriter, r *http.Request) {}
`

	task :=
		"Вынеси регистрацию HTTP-маршрутов из main() в отдельную функцию registerRoutes()."

	// В этом случае до patch задача уже была выполнена,
	// поэтому ожидаем ALREADY_SATISFIED, а не успех
	// косметического изменения.
	result := evaluateTaskEffectiveness(
		task,
		before,
		after,
	)

	if result.Decision !=
		taskEffectivenessAlreadySatisfied {

		t.Fatalf(
			"decision = %q, want ALREADY_SATISFIED; reason=%s",
			result.Decision,
			result.Reason,
		)
	}
}

func TestEvaluateTaskEffectiveness_RejectsMissingTarget(
	t *testing.T,
) {
	before := `package main

func main() {
	http.HandleFunc("/", handler)
}

func handler(w http.ResponseWriter, r *http.Request) {}
`

	after := `package main

func main() {
	http.HandleFunc("/", handler)
}

func handler(w http.ResponseWriter, r *http.Request) {}
`

	task :=
		"Extract HTTP route registration from main() into a separate registerRoutes() function."

	result := evaluateTaskEffectiveness(
		task,
		before,
		after,
	)

	if result.Decision !=
		taskEffectivenessReject {

		t.Fatalf(
			"decision = %q, want REJECT; reason=%s",
			result.Decision,
			result.Reason,
		)
	}
}

func TestEvaluateTaskEffectiveness_SkipUnknownTask(
	t *testing.T,
) {
	before := `package main

func main() {}
`

	after := `package main

func main() {
	println("hello")
}
`

	result := evaluateTaskEffectiveness(
		"Improve the code.",
		before,
		after,
	)

	if result.Decision !=
		taskEffectivenessSkip {

		t.Fatalf(
			"decision = %q, want SKIP",
			result.Decision,
		)
	}
}

func TestExtractTaskEffectivenessContractEnglish(
	t *testing.T,
) {
	task :=
		"Extract HTTP route registration from main() into a separate registerRoutes() function."

	contract, ok :=
		extractTaskEffectivenessContract(task)

	if !ok {
		t.Fatal("expected task contract")
	}

	if contract.kind != "extract" {
		t.Fatalf(
			"kind = %q, want extract",
			contract.kind,
		)
	}

	if contract.source != "main" {
		t.Fatalf(
			"source = %q, want main",
			contract.source,
		)
	}

	if contract.target != "registerRoutes" {
		t.Fatalf(
			"target = %q, want registerRoutes",
			contract.target,
		)
	}
}

func TestExtractTaskEffectivenessContractRussianShort(
	t *testing.T,
) {
	task :=
		"Вынеси маршруты из main() в registerRoutes()."

	contract, ok :=
		extractTaskEffectivenessContract(task)

	if !ok {
		t.Fatal("expected task contract")
	}

	if contract.kind != "extract" {
		t.Fatalf(
			"kind = %q, want extract",
			contract.kind,
		)
	}

	if contract.source != "main" {
		t.Fatalf(
			"source = %q, want main",
			contract.source,
		)
	}

	if contract.target != "registerRoutes" {
		t.Fatalf(
			"target = %q, want registerRoutes",
			contract.target,
		)
	}
}

func TestExtractTaskEffectivenessContractRussian(
	t *testing.T,
) {
	task :=
		"Вынеси регистрацию HTTP-маршрутов из main() в отдельную функцию registerRoutes()."

	contract, ok :=
		extractTaskEffectivenessContract(task)

	if !ok {
		t.Fatal("expected task contract")
	}

	if contract.kind != "extract" {
		t.Fatalf(
			"kind = %q, want extract",
			contract.kind,
		)
	}

	if contract.source != "main" {
		t.Fatalf(
			"source = %q, want main",
			contract.source,
		)
	}

	if contract.target != "registerRoutes" {
		t.Fatalf(
			"target = %q, want registerRoutes",
			contract.target,
		)
	}
}

func TestEvaluateTaskEffectiveness_RejectsWrongRefactor(
	t *testing.T,
) {
	before := `package main

func main() {
	http.HandleFunc("/", handler)
}

func handler(w http.ResponseWriter, r *http.Request) {}
`

	after := `package main

func main() {
	println("changed")
	http.HandleFunc("/", handler)
}

func registerRoutes() {
	println("unused")
}

func handler(w http.ResponseWriter, r *http.Request) {}
`

	task :=
		"Extract HTTP route registration from main() into a separate registerRoutes() function."

	result := evaluateTaskEffectiveness(
		task,
		before,
		after,
	)

	if result.Decision !=
		taskEffectivenessReject {

		t.Fatalf(
			"decision = %q, want REJECT; reason=%s",
			result.Decision,
			result.Reason,
		)
	}
}
