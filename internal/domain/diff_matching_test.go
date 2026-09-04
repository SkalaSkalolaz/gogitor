package domain

import (
	"encoding/json"
	"testing"
)

func TestDefaultDiffMatchingConfig(t *testing.T) {
	cfg := DefaultDiffMatchingConfig()

	if cfg.ASTWeight != 0.85 {
		t.Fatalf(
			"ASTWeight = %.2f, want 0.85",
			cfg.ASTWeight,
		)
	}

	if cfg.LineWeight != 0.15 {
		t.Fatalf(
			"LineWeight = %.2f, want 0.15",
			cfg.LineWeight,
		)
	}

	if cfg.ASTMinStructure != 0.82 {
		t.Fatalf(
			"ASTMinStructure = %.2f, want 0.82",
			cfg.ASTMinStructure,
		)
	}

	if cfg.FuzzyBaseThreshold != 0.60 {
		t.Fatalf(
			"FuzzyBaseThreshold = %.2f, want 0.60",
			cfg.FuzzyBaseThreshold,
		)
	}

	if cfg.BalancedThreshold != 0.82 {
		t.Fatalf(
			"BalancedThreshold = %.2f, want 0.82",
			cfg.BalancedThreshold,
		)
	}

	if cfg.BalancedMargin != 0.08 {
		t.Fatalf(
			"BalancedMargin = %.2f, want 0.08",
			cfg.BalancedMargin,
		)
	}

	if cfg.AdvancedThreshold != 0.85 {
		t.Fatalf(
			"AdvancedThreshold = %.2f, want 0.85",
			cfg.AdvancedThreshold,
		)
	}

	if cfg.AdvancedMargin != 0.05 {
		t.Fatalf(
			"AdvancedMargin = %.2f, want 0.05",
			cfg.AdvancedMargin,
		)
	}
}

func TestDiffMatchingConfigNormalizesWeights(t *testing.T) {
	cfg := DiffMatchingConfig{
		ASTWeight:  0.20,
		LineWeight: 0.30,
	}

	cfg = cfg.Normalized()

	if cfg.ASTWeight != 0.40 {
		t.Fatalf(
			"ASTWeight = %.3f, want 0.4",
			cfg.ASTWeight,
		)
	}

	if cfg.LineWeight != 0.60 {
		t.Fatalf(
			"LineWeight = %.3f, want 0.6",
			cfg.LineWeight,
		)
	}
}

func TestDiffMatchingConfigNormalizesInvalidValues(
	t *testing.T,
) {
	cfg := DiffMatchingConfig{
		ASTWeight:          -1,
		LineWeight:         -2,
		ASTMinStructure:    2,
		FuzzyBaseThreshold: -1,
		BalancedThreshold:  3,
		BalancedMargin:     -1,
		AdvancedThreshold:  2,
		AdvancedMargin:     -1,
	}

	cfg = cfg.Normalized()
	defaults := DefaultDiffMatchingConfig()

	if cfg.ASTWeight != defaults.ASTWeight {
		t.Fatalf(
			"ASTWeight = %.3f, want %.3f",
			cfg.ASTWeight,
			defaults.ASTWeight,
		)
	}

	if cfg.LineWeight != defaults.LineWeight {
		t.Fatalf(
			"LineWeight = %.3f, want %.3f",
			cfg.LineWeight,
			defaults.LineWeight,
		)
	}

	if cfg.ASTMinStructure != defaults.ASTMinStructure {
		t.Fatalf(
			"ASTMinStructure = %.3f, want %.3f",
			cfg.ASTMinStructure,
			defaults.ASTMinStructure,
		)
	}

	if cfg.FuzzyBaseThreshold !=
		defaults.FuzzyBaseThreshold {
		t.Fatalf(
			"FuzzyBaseThreshold = %.3f, want %.3f",
			cfg.FuzzyBaseThreshold,
			defaults.FuzzyBaseThreshold,
		)
	}
}

func TestDiffMatchingConfigJSON(t *testing.T) {
	cfg := DefaultDiffMatchingConfig()

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var decoded DiffMatchingConfig

	if err := json.Unmarshal(
		data,
		&decoded,
	); err != nil {
		t.Fatal(err)
	}

	decoded = decoded.Normalized()

	if decoded != cfg {
		t.Fatalf(
			"decoded config differs from default:\n got=%+v\nwant=%+v",
			decoded,
			cfg,
		)
	}
}
