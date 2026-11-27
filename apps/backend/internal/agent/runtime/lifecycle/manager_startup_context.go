package lifecycle

import "context"

// agentStartupContext keeps tagged resume startup cancellable by its owner
// while ordinary callers continue to share caller-independent work.
func agentStartupContext(owner, shared context.Context) (context.Context, func()) {
	if ResumeAttemptIDFromContext(owner) == "" {
		return shared, func() {}
	}
	startup, cancel := context.WithCancel(shared)
	stopOwner := context.AfterFunc(owner, cancel)
	if owner.Err() != nil {
		cancel()
	}
	return startup, func() {
		stopOwner()
		cancel()
	}
}
