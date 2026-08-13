package app

// ContextAllocation распределяет доступный контекст модели между компонентами промпта.
// Возвращает лимиты в байтах для каждого компонента.
type ContextAllocation struct {
	TaskBytes           int
	HistoryBytes        int
	ProjectSummaryBytes int
	PrimaryFilesBytes   int
	RelatedFilesBytes   int
	TestFilesBytes      int
	GitDiffBytes        int
	ReserveBytes        int
	TotalBytes          int
}

// AllocateContextBudget распределяет бюджет контекста.
// totalBytes — общий доступный бюджет (из cfg.ContextBytes()).
func AllocateContextBudget(totalBytes int) ContextAllocation {
	if totalBytes <= 0 {
		totalBytes = 120000
	}
	a := ContextAllocation{TotalBytes: totalBytes}
	a.TaskBytes = totalBytes * 5 / 100           // 5%
	a.HistoryBytes = totalBytes * 8 / 100        // 8%
	a.ProjectSummaryBytes = totalBytes * 3 / 100 // 3%
	a.PrimaryFilesBytes = totalBytes * 30 / 100  // 30%
	a.RelatedFilesBytes = totalBytes * 22 / 100  // 22%
	a.TestFilesBytes = totalBytes * 12 / 100     // 12%
	a.GitDiffBytes = totalBytes * 10 / 100       // 10%
	a.ReserveBytes = totalBytes * 10 / 100       // 10%
	return a
}