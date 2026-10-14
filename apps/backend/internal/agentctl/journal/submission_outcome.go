package journal

// HasUnprovenOutcome identifies history that native restoration can acknowledge
// without changing its recorded outcome. It is not permission to retire live work.
func HasUnprovenOutcome(submission Submission) bool {
	if submission.State == SubmissionInterruptedUnknown {
		return true
	}
	if submission.TerminalEventRetained || submission.TerminalSequence != 0 {
		return false
	}
	switch submission.State {
	case SubmissionFailed, SubmissionCompleted, SubmissionCancelled:
		return true
	default:
		return false
	}
}
