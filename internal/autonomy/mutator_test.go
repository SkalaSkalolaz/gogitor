package autonomy

import (
	"testing"
)

func TestIsFalsePositive(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
		idx  int
		op   string
		want bool
	}{
		{"normal >=", "if x >= 10 {", 5, ">=", false},       // '>' находится на индексе 5
		{"part of !=", "if x != 10 {", 6, "==", true},       // '=' находится на индексе 6 (проверяем line[5] == '!')
		{"triple =", "if x === 10 {", 5, "==", true},        // первое '=' на индексе 5 (проверяем line[7] == '=')
		{"normal &&", "if a && b {", 5, "&&", false},        // '&' находится на индексе 5
		{"normal ==", "if a == b {", 5, "==", false},        // '=' находится на индексе 5
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isFalsePositive(tc.line, tc.idx, tc.op); got != tc.want {
				t.Errorf("isFalsePositive(%q, %d, %q) = %v, want %v", tc.line, tc.idx, tc.op, got, tc.want)
			}
		})
	}
}

func TestMutationRulesOrder(t *testing.T) {
	foundGE := false
	for _, rule := range mutationRules {
		if rule.From == ">=" {
			foundGE = true
		}
		if rule.From == ">" && !foundGE {
			t.Error("> should come after >=")
		}
	}
	if !foundGE {
		t.Error(">= rule not found")
	}
}

func TestMutationReport_Format(t *testing.T) {
	r := &MutationReport{
		Mutations: []Mutation{
			{File: "main.go", Line: 10, Type: MutRelational, Killed: true},
			{File: "util.go", Line: 20, Type: MutLogical, Killed: false},
			{File: "err.go", Line: 30, Error: "sandbox error"},
		},
		Killed: 1, Survived: 1, Errors: 1,
	}
	if r.Format() == "" {
		t.Error("empty format")
	}
	if (&MutationReport{}).Format() == "" {
		t.Error("empty report should format")
	}
}