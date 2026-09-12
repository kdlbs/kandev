package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type fakePassthroughSessionProvider struct {
	session *models.TaskSession
	err     error
}

func (f *fakePassthroughSessionProvider) GetTaskSession(_ context.Context, _ string) (*models.TaskSession, error) {
	return f.session, f.err
}

// TestPassthroughLookupFromSessionProviderReturnsSnapshotOnSuccess pins
// Review round 3, finding 6: the closure wired into
// Manager.SetPassthroughLookup must answer with the durable
// TaskSession.IsPassthrough snapshot, not the zero value, when the read
// succeeds.
func TestPassthroughLookupFromSessionProviderReturnsSnapshotOnSuccess(t *testing.T) {
	provider := &fakePassthroughSessionProvider{session: &models.TaskSession{IsPassthrough: true}}
	lookup := passthroughLookupFromSessionProvider(provider)

	isPassthrough, ok := lookup(context.Background(), "session-1")

	if !ok {
		t.Fatal("ok = false, want true on a successful read")
	}
	if !isPassthrough {
		t.Fatal("isPassthrough = false, want true")
	}
}

func TestPassthroughLookupFromSessionProviderReturnsFalseSnapshotOnSuccess(t *testing.T) {
	provider := &fakePassthroughSessionProvider{session: &models.TaskSession{IsPassthrough: false}}
	lookup := passthroughLookupFromSessionProvider(provider)

	isPassthrough, ok := lookup(context.Background(), "session-1")

	if !ok {
		t.Fatal("ok = false, want true on a successful read")
	}
	if isPassthrough {
		t.Fatal("isPassthrough = true, want false")
	}
}

// TestPassthroughLookupFromSessionProviderFailsOpenToUnknownOnError pins the
// safe-default half of finding 6: a failed durable read must answer
// ok=false (unknown), never fabricate isPassthrough=true/false, so the
// recovery-guard caller falls back to guarding the session.
func TestPassthroughLookupFromSessionProviderFailsOpenToUnknownOnError(t *testing.T) {
	provider := &fakePassthroughSessionProvider{err: errors.New("db unavailable")}
	lookup := passthroughLookupFromSessionProvider(provider)

	isPassthrough, ok := lookup(context.Background(), "session-1")

	if ok {
		t.Fatal("ok = true, want false when the durable read fails")
	}
	if isPassthrough {
		t.Fatal("isPassthrough = true, want false alongside ok=false")
	}
}

// TestPassthroughLookupFromSessionProviderFailsOpenToUnknownOnNilSession
// covers a provider answering successfully with a nil session (e.g. scoping
// denial folded into a nil, no-error return by some future implementation).
func TestPassthroughLookupFromSessionProviderFailsOpenToUnknownOnNilSession(t *testing.T) {
	provider := &fakePassthroughSessionProvider{session: nil, err: nil}
	lookup := passthroughLookupFromSessionProvider(provider)

	isPassthrough, ok := lookup(context.Background(), "session-1")

	if ok {
		t.Fatal("ok = true, want false when the provider answers with a nil session")
	}
	if isPassthrough {
		t.Fatal("isPassthrough = true, want false alongside ok=false")
	}
}
