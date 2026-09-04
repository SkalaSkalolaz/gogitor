package domain

// DiffMatchingConfig содержит настраиваемые параметры поиска
// SEARCH-блока в существующем коде.
//
// Все значения относятся только к DIFF matching.
// Они не меняют саму стратегию patch policy.
type DiffMatchingConfig struct {
	// ASTWeight — вес структурного AST/token similarity
	// в итоговом confidence AST-aware fuzzy.
	ASTWeight float64 `json:"ast_weight"`

	// LineWeight — вес буквального line similarity
	// в итоговом confidence AST-aware fuzzy.
	LineWeight float64 `json:"line_weight"`

	// ASTMinStructure — минимальная структурная похожесть,
	// при которой AST-aware кандидат вообще рассматривается.
	ASTMinStructure float64 `json:"ast_min_structure"`

	// FuzzyBaseThreshold — минимальный порог legacy fuzzy
	// перед применением policy threshold.
	FuzzyBaseThreshold float64 `json:"fuzzy_base_threshold"`

	// BalancedThreshold — финальный confidence threshold
	// для PatchPolicyBalanced.
	BalancedThreshold float64 `json:"balanced_threshold"`

	// BalancedMargin — минимальная разница между лучшим
	// и вторым кандидатом для Balanced.
	BalancedMargin float64 `json:"balanced_margin"`

	// AdvancedThreshold — финальный confidence threshold
	// для PatchPolicyAdvanced.
	AdvancedThreshold float64 `json:"advanced_threshold"`

	// AdvancedMargin — минимальная разница между лучшим
	// и вторым кандидатом для Advanced.
	AdvancedMargin float64 `json:"advanced_margin"`
}

// DefaultDiffMatchingConfig возвращает безопасные исходные значения.
func DefaultDiffMatchingConfig() DiffMatchingConfig {
	return DiffMatchingConfig{
		ASTWeight:           0.85,
		LineWeight:          0.15,
		ASTMinStructure:     0.82,
		FuzzyBaseThreshold:  0.60,
		BalancedThreshold:   0.82,
		BalancedMargin:      0.08,
		AdvancedThreshold:   0.85,
		AdvancedMargin:      0.05,
	}
}

// Normalized возвращает безопасную конфигурацию.
//
// Веса приводятся к диапазону [0..1] и нормализуются так,
// чтобы ASTWeight + LineWeight = 1.
//
// Остальные параметры также ограничиваются диапазоном [0..1].
// Некорректные нулевые/отрицательные веса заменяются
// значениями по умолчанию.
func (c DiffMatchingConfig) Normalized() DiffMatchingConfig {
	defaults := DefaultDiffMatchingConfig()

	if c.ASTWeight < 0 || c.ASTWeight > 1 {
		c.ASTWeight = defaults.ASTWeight
	}

	if c.LineWeight < 0 || c.LineWeight > 1 {
		c.LineWeight = defaults.LineWeight
	}

	sum := c.ASTWeight + c.LineWeight

	if sum <= 0 {
		c.ASTWeight = defaults.ASTWeight
		c.LineWeight = defaults.LineWeight
		sum = c.ASTWeight + c.LineWeight
	}

	c.ASTWeight /= sum
	c.LineWeight /= sum

	if c.ASTMinStructure < 0 ||
		c.ASTMinStructure > 1 {
		c.ASTMinStructure =
			defaults.ASTMinStructure
	}

	if c.FuzzyBaseThreshold < 0 ||
		c.FuzzyBaseThreshold > 1 {
		c.FuzzyBaseThreshold =
			defaults.FuzzyBaseThreshold
	}

	if c.BalancedThreshold < 0 ||
		c.BalancedThreshold > 1 {
		c.BalancedThreshold =
			defaults.BalancedThreshold
	}

	if c.BalancedMargin < 0 ||
		c.BalancedMargin > 1 {
		c.BalancedMargin =
			defaults.BalancedMargin
	}

	if c.AdvancedThreshold < 0 ||
		c.AdvancedThreshold > 1 {
		c.AdvancedThreshold =
			defaults.AdvancedThreshold
	}

	if c.AdvancedMargin < 0 ||
		c.AdvancedMargin > 1 {
		c.AdvancedMargin =
			defaults.AdvancedMargin
	}

	return c
}