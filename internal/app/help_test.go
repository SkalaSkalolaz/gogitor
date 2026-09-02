package app

import (
	"testing"

	"gogitor/internal/i18n"
)

func TestHelpForCommand_Known(t *testing.T) {
	for _, topic := range []string{
		"git",
		"autonomy",
		"code",
		"computer",
		"article",
		"fix",
		"agent",
		"mutate",
		"suggest",
		"decisions",
		"search",
		"vet",
		"todo",
		"analyze",
		"ask",
		"autogen-tests",
		"reasoning",
		"test",
	} {
		t.Run(topic, func(t *testing.T) {
			res := HelpForCommand(topic)
			if !res.Success || res.Response == "" {
				t.Errorf("HelpForCommand(%q) failed", topic)
			}
		})
	}
}

func TestHelpForCommand_WithColon(t *testing.T) {
	if res := HelpForCommand(":git"); !res.Success {
		t.Error("failed")
	}
}

func TestHelpForCommand_Unknown(t *testing.T) {
	res := HelpForCommand("nonexistent")
	if !res.Success || res.Response == "" {
		t.Error("should list available topics")
	}
}

func TestHelpTopicNames(t *testing.T) {
	names := HelpTopicNames()
	if len(names) == 0 {
		t.Fatal("empty")
	}
	expected := map[string]bool{
		"git":      true,
		"agent":    true,
		"code":     true,
		"autonomy": true,
	}
	for _, n := range names {
		delete(expected, n)
	}
	if len(expected) > 0 {
		t.Errorf("missing: %v", expected)
	}
}

func TestHelpForCommand_Russian(t *testing.T) {
	i18n.SetLang(i18n.RU)
	defer i18n.SetLang(i18n.EN)
	if res := HelpForCommand("git"); !res.Success || res.Response == "" {
		t.Error("Russian help failed")
	}
}
