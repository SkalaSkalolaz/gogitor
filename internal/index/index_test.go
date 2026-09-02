package index

import (
	"testing"
)

func TestNormalizeTokensV2(t *testing.T) {
	for _, tc := range []struct{ in string; want []string }{
		{"HelloWorld", []string{"hello", "world"}},
		{"parse_file", []string{"parse", "file"}},
		{"main.go", []string{"main", "go"}},
		{"", nil},
		{"a", nil},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got := normalizeTokensV2(tc.in)
			if len(got) != len(tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
				return
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestSplitCamelV2(t *testing.T) {
	for _, tc := range []struct{ in string; want []string }{
		{"HelloWorld", []string{"hello", "world"}},
		{"HTTPServer", []string{"http", "server"}},
		{"parseFile", []string{"parse", "file"}},
		{"simple", []string{"simple"}},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got := splitCamelV2(tc.in)
			if len(got) != len(tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
				return
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestExtractKeywordsV2(t *testing.T) {
	kw := extractKeywordsV2("create HTTP server with authentication")
	if len(kw) == 0 {
		t.Fatal("empty")
	}
	found := false
	for _, k := range kw {
		if k == "http" {
			found = true
		}
	}
	if !found {
		t.Error("missing 'http'")
	}
}

func TestExtractKeywordsV2_Russian(t *testing.T) {
	if kw := extractKeywordsV2("создай сервер с авторизацией"); len(kw) == 0 {
		t.Error("empty for Russian")
	}
}

func TestDetectTestIntentV2(t *testing.T) {
	for _, tc := range []struct{ task string; want bool }{
		{"write tests for the function", true},
		{"напиши тесты для функции", true},
		{"test coverage", true},
		{"create a web server", false},
		{"", false},
	} {
		if got := detectTestIntentV2(tc.task); got != tc.want {
			t.Errorf("detectTestIntentV2(%q) = %v, want %v", tc.task, got, tc.want)
		}
	}
}

func TestStopWordV2(t *testing.T) {
	for _, w := range []string{"и", "в", "на", "the", "a", "is", "код", "файл", "write", "create"} {
		if !stopWordV2(w) {
			t.Errorf("stopWordV2(%q) = false, want true", w)
		}
	}
	for _, w := range []string{"server", "http", "сервер", "авторизация"} {
		if stopWordV2(w) {
			t.Errorf("stopWordV2(%q) = true, want false", w)
		}
	}
}

func TestStemVariantsV2(t *testing.T) {
	if v := stemVariantsV2("authentication"); len(v) == 0 {
		t.Error("empty variants")
	}
}

func TestExpandSynonymsV2(t *testing.T) {
	seen := map[string]bool{"http": true}
	expanded := expandSynonymsV2([]string{"http"}, seen)
	if len(expanded) <= 1 {
		t.Error("expected synonyms")
	}
}