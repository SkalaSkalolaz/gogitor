package textutil

import (
	"testing"
)

func TestLimitRunes(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		maxRunes int
		suffix   string
		want     string
	}{
		{"empty string", "", 5, "...", ""},
		{"no truncation", "hello", 10, "...", "hello"},
		{"exact length", "hello", 5, "...", "hello"},
		{"truncation", "hello world", 5, "...", "hello..."},
		{"negative max", "hello", -1, "...", "..."},
		{"zero max", "hello", 0, "...", "..."},
		{"unicode cyrillic", "привет мир", 6, "…", "привет…"},
		{"emoji", "👋🌍🎉", 2, "...", "👋🌍..."},
		{"mixed unicode", "Hello мир", 7, "...", "Hello м..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LimitRunes(tt.s, tt.maxRunes, tt.suffix)
			if got != tt.want {
				t.Errorf("LimitRunes(%q, %d, %q) = %q, want %q",
					tt.s, tt.maxRunes, tt.suffix, got, tt.want)
			}
		})
	}
}

func TestTruncateStringBytes(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		maxBytes int
		want     string
	}{
		{"empty", "", 10, ""},
		{"no truncation", "hello", 10, "hello"},
		{"exact bytes", "hello", 5, "hello"},
		{"truncate ascii", "hello world", 5, "hello"},
		{"zero max", "hello", 0, ""},
		{"negative max", "hello", -1, ""},
		{"multibyte no split", "привет", 7, "при"},
		{"multibyte boundary", "привет", 6, "при"},
		{"emoji", "👋🌍", 4, "👋"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateStringBytes(tt.s, tt.maxBytes)
			if got != tt.want {
				t.Errorf("TruncateStringBytes(%q, %d) = %q, want %q",
					tt.s, tt.maxBytes, got, tt.want)
			}
		})
	}
}

func TestTruncateBytes(t *testing.T) {
	tests := []struct {
		name     string
		b        []byte
		maxBytes int
		want     string
	}{
		{"nil input", nil, 10, ""},
		{"empty", []byte{}, 10, ""},
		{"no truncation", []byte("hello"), 10, "hello"},
		{"truncate", []byte("hello world"), 5, "hello"},
		{"zero max", []byte("hello"), 0, ""},
		{"negative max", []byte("hello"), -1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateBytes(tt.b, tt.maxBytes)
			if string(got) != tt.want {
				t.Errorf("TruncateBytes(%q, %d) = %q, want %q",
					tt.b, tt.maxBytes, string(got), tt.want)
			}
		})
	}
}