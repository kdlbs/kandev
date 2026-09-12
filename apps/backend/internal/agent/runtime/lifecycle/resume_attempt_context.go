package lifecycle

import "context"

// resumeAttemptContextKey carries the immutable recovery-attempt identity
// through lifecycle startup and event publication. The key lives in lifecycle
// so both the orchestrator and lifecycle manager can preserve the value without
// importing one another's implementation details.
type resumeAttemptContextKey struct{}

// WithResumeAttemptID associates one recovery attempt with a lifecycle call.
// The value is immutable for the lifetime of the derived context.
func WithResumeAttemptID(ctx context.Context, attemptID string) context.Context {
	if ctx == nil || attemptID == "" {
		return ctx
	}
	return context.WithValue(ctx, resumeAttemptContextKey{}, attemptID)
}

// ResumeAttemptIDFromContext returns the recovery attempt carried by ctx.
func ResumeAttemptIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	attemptID, _ := ctx.Value(resumeAttemptContextKey{}).(string)
	return attemptID
}
