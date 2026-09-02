package app

import (
	"testing"
)

func TestExtractJSONCandidate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		text    string
		wantErr bool
	}{
		{"valid", `{"key": "value"}`, false},
		{"in text", `prefix {"key": "value"} suffix`, false},
		{"no json", "no json", true},
		{"nested", `{"outer": {"inner": 1}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := extractJSONCandidate(tc.text)
			if (err != nil) != tc.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestExtractAllJSONCandidates(t *testing.T) {
	candidates := extractAllJSONCandidates(`{"a": 1} text {"b": 2}`)
	if len(candidates) != 2 {
		t.Errorf("expected 2, got %d", len(candidates))
	}
}

func TestParseAgentJSON(t *testing.T) {
	var out struct {
		Name string `json:"name"`
	}
	if err := parseAgentJSON(`{"name": "test"}`, &out); err != nil {
		t.Fatal(err)
	}
	if out.Name != "test" {
		t.Errorf("name = %q", out.Name)
	}
	var out2 struct{}
	if err := parseAgentJSON("not json", &out2); err == nil {
		t.Error("expected error")
	}
}
