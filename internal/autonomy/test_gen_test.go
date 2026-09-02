package autonomy

import (
	"testing"
)

func TestCleanTestCode(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"plain", "func TestFoo(t *testing.T) {}", "func TestFoo(t *testing.T) {}"},
		{"markdown", "```go\nfunc TestFoo(t *testing.T) {}\n```", "func TestFoo(t *testing.T) {}"},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanTestCode(tc.in); got != tc.want {
				t.Errorf("cleanTestCode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}