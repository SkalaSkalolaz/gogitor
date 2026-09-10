package language

import "testing"

func TestRegistryDetectGo(t *testing.T) {
	r := NewRegistry()
	def, ok := r.Detect("internal/app/service.go")
	if !ok {
		t.Fatal("expected Go to be detected")
	}
	if def.ID != Go || def.Name != "Go" {
		t.Fatalf("unexpected definition: %+v", def)
	}
}

func TestRegistryRegisterExtension(t *testing.T) {
	r := NewRegistry()
	r.Register(Definition{ID: "rust", Name: "Rust", Extensions: []string{"rs"}})
	def, ok := r.Detect("src/main.rs")
	if !ok || def.ID != "rust" {
		t.Fatalf("expected rust definition, got %+v, %v", def, ok)
	}
}
