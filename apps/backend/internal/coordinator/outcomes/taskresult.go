package outcomes

// Task results of a task a proposal created.
const (
	ResultOpen    = "open"
	ResultDone    = "done"
	ResultMerged  = "merged"
	ResultFailed  = "failed"
	ResultDropped = "dropped"
)

// TaskFacts are the stored facts a task result derives from.
type TaskFacts struct {
	PRMerged            bool
	StepCompletes       bool
	LatestSessionFailed bool
	Archived            bool
}

// TaskResult derives the result in precedence order: merged, done, failed,
// dropped, else open.
func TaskResult(f TaskFacts) string {
	switch {
	case f.PRMerged:
		return ResultMerged
	case f.StepCompletes:
		return ResultDone
	case f.LatestSessionFailed:
		return ResultFailed
	case f.Archived:
		return ResultDropped
	default:
		return ResultOpen
	}
}

// IsFinalResult reports whether a result ends grading of the task.
func IsFinalResult(result string) bool { return result == ResultMerged || result == ResultDropped }

// Reopened reports whether moving from the stored result to the next one is a
// reopening: a done task that is open or failed again.
func Reopened(stored, next string) bool {
	return stored == ResultDone && (next == ResultOpen || next == ResultFailed)
}
