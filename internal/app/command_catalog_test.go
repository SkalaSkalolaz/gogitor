package app

import "testing"

func TestCommandCatalogContainsCoreTUICommands(t *testing.T) {
	wanted := map[string]bool{
		":code":       false,
		":agent":      false,
		":fix":        false,
		":task-diff":  false,
		":diff-trace": false,
		":git":        false,
		":quit":       false,
	}

	foundFast := false

	for _, name := range CommandNames() {
		if _, ok := wanted[name]; ok {
			wanted[name] = true
		}

		if name == ":fast" {
			foundFast = true
		}
	}

	for name, found := range wanted {
		if !found {
			t.Errorf(
				"command catalog missing %s",
				name,
			)
		}
	}

	if foundFast {
		t.Fatal(":fast must not be present in command catalog")
	}
}
