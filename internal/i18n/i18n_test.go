package i18n

import (
	"testing"
)

func TestT_English(t *testing.T) {
	SetLang(EN)
	if got := T("SUCCESS"); got != "SUCCESS" {
		t.Errorf("got %q", got)
	}
}

func TestT_Russian(t *testing.T) {
	SetLang(RU)
	defer SetLang(EN)
	if got := T("SUCCESS"); got != "УСПЕХ" {
		t.Errorf("got %q", got)
	}
}

func TestT_WithArgs(t *testing.T) {
	SetLang(EN)
	if got := T("Mode: %s", "code"); got != "Mode: code" {
		t.Errorf("got %q", got)
	}
}

func TestT_RussianWithArgs(t *testing.T) {
	SetLang(RU)
	defer SetLang(EN)
	if got := T("Tests: passed=%d failed=%d", 5, 2); got != "Тесты: пройдено=5, упало=2" {
		t.Errorf("got %q", got)
	}
}

func TestT_UnknownKey(t *testing.T) {
	SetLang(RU)
	defer SetLang(EN)
	key := "Unknown key without translation"
	if got := T(key); got != key {
		t.Errorf("unknown key should return itself, got %q", got)
	}
}

func TestLocalize(t *testing.T) {
	SetLang(RU)
	defer SetLang(EN)
	if got := Localize("SUCCESS"); got != "УСПЕХ" {
		t.Errorf("got %q", got)
	}
	if got := Localize(""); got != "" {
		t.Errorf("empty = %q", got)
	}
}

func TestLocalize_English(t *testing.T) {
	SetLang(EN)
	if got := Localize("SUCCESS"); got != "SUCCESS" {
		t.Errorf("got %q", got)
	}
}

func TestCurrent(t *testing.T) {
	SetLang(EN)
	if Current() != EN {
		t.Error("expected EN")
	}
	SetLang(RU)
	if Current() != RU {
		t.Error("expected RU")
	}
	SetLang(EN)
}