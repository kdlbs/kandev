package review

// PromptBudgetBytes bounds how much diff text one reviewer prompt carries.
// Chosen well below the smallest context window kandev routinely targets so the
// surrounding instructions and the model's own reply still fit. A review is
// split across several prompts rather than truncated, because a truncated diff
// produces findings anchored to lines the reviewer never saw.
const PromptBudgetBytes = 120_000

// BatchPlan is the result of splitting a change set into reviewer prompts.
type BatchPlan struct {
	// Batches each fit within the byte budget. Files are never split.
	Batches [][]ChangedFile
	// Skipped holds files whose own diff exceeds the budget on its own. They
	// cannot be reviewed and are named in the run summary rather than silently
	// dropped.
	Skipped []ChangedFile
}

// FileCount returns how many files the plan will actually submit.
func (p BatchPlan) FileCount() int {
	n := 0
	for _, batch := range p.Batches {
		n += len(batch)
	}
	return n
}

// PlanBatches groups files into prompt-sized batches, preserving the incoming
// order so findings arrive in the same order the Review panel lists files.
//
// budgetBytes <= 0 falls back to PromptBudgetBytes.
func PlanBatches(files []ChangedFile, budgetBytes int) BatchPlan {
	if budgetBytes <= 0 {
		budgetBytes = PromptBudgetBytes
	}
	plan := BatchPlan{}
	var current []ChangedFile
	currentSize := 0

	flush := func() {
		if len(current) > 0 {
			plan.Batches = append(plan.Batches, current)
			current = nil
			currentSize = 0
		}
	}

	for _, file := range files {
		size := len(file.Diff)
		if size > budgetBytes {
			plan.Skipped = append(plan.Skipped, file)
			continue
		}
		if currentSize+size > budgetBytes {
			flush()
		}
		current = append(current, file)
		currentSize += size
	}
	flush()
	return plan
}
