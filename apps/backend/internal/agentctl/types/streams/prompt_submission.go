package streams

import "context"

type promptSubmissionKey struct{}

func WithPromptSubmissionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, promptSubmissionKey{}, id)
}

func PromptSubmissionID(ctx context.Context) string {
	id, _ := ctx.Value(promptSubmissionKey{}).(string)
	return id
}

// ReportedPromptFailure is an accepted turn failure already emitted on the
// adapter's ordered event stream.
type ReportedPromptFailure struct{ Message string }

func (e *ReportedPromptFailure) Error() string                  { return e.Message }
func (*ReportedPromptFailure) DeterministicPromptFailure() bool { return true }
