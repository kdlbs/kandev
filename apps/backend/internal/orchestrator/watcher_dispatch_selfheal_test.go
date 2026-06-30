package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger() *logger.Logger {
	l, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	return l
}

// fakeProfileLookup answers a fixed verdict for any profileID.
type fakeProfileLookup struct {
	deleted bool
	name    string
	err     error
	calls   int
	lastID  string
}

func (f *fakeProfileLookup) LookupProfile(_ context.Context, profileID string) (deleted bool, name string, err error) {
	f.calls++
	f.lastID = profileID
	return f.deleted, f.name, f.err
}

// stubWatcherSource is a minimal WatcherSource that records each pipeline
// hook the coordinator invokes. Lets tests assert which hooks ran (or
// didn't) without dragging in a real integration.
type stubWatcherSource struct {
	name           string
	agentProfileID string
	reserveCalls   int
	selfHealCalls  int
	selfHealCause  string
	buildCalls     int
	releaseCalls   int
	// repositories, when set, is returned in the built IssueTaskRequest so the
	// repository pre-flight can be exercised.
	repositories []IssueTaskRepository
	workspaceID  string
}

func (s *stubWatcherSource) Name() string { return s.name }

func (s *stubWatcherSource) AgentProfileID(_ any) string { return s.agentProfileID }

func (s *stubWatcherSource) Reserve(_ context.Context, _ any) (bool, error) {
	s.reserveCalls++
	return true, nil
}

func (s *stubWatcherSource) Release(_ context.Context, _ any) { s.releaseCalls++ }

func (s *stubWatcherSource) BuildTaskRequest(_ any) (*IssueTaskRequest, error) {
	s.buildCalls++
	return &IssueTaskRequest{WorkspaceID: s.workspaceID, Repositories: s.repositories}, nil
}

func (s *stubWatcherSource) AttachTaskID(_ context.Context, _ any, _ string) error { return nil }

func (s *stubWatcherSource) AutoStartParams(_ any) AutoStartParams { return AutoStartParams{} }

func (s *stubWatcherSource) WatchID(_ any) string { return "" }

func (s *stubWatcherSource) MaxInflightTasks(_ any) *int { return nil }

func (s *stubWatcherSource) WatchMetadataKey() string { return "" }

func (s *stubWatcherSource) SelfHeal(_ context.Context, _ any, cause string) error {
	s.selfHealCalls++
	s.selfHealCause = cause
	return nil
}

// TestWatcherDispatchCoordinator_SkipsAndSelfHealsOnDeletedProfile is the
// regression guard for the production bug (BLA-1598): when the watcher's
// bound agent profile has been soft-deleted, the coordinator MUST short-
// circuit before creating a task and MUST invoke SelfHeal so the watcher
// disables itself and stops re-firing every poll.
func TestWatcherDispatchCoordinator_SkipsAndSelfHealsOnDeletedProfile(t *testing.T) {
	src := &stubWatcherSource{name: "stub", agentProfileID: "deleted-profile"}
	lookup := &fakeProfileLookup{deleted: true, name: "Removed Kilo"}

	c := &WatcherDispatchCoordinator{
		profileLookup: lookup,
		logger:        nil,
	}

	c.Dispatch(context.Background(), src, struct{}{})

	if lookup.calls != 1 {
		t.Errorf("expected 1 profile lookup, got %d", lookup.calls)
	}
	if src.reserveCalls != 0 {
		t.Errorf("Reserve must not run when profile is deleted, got %d calls", src.reserveCalls)
	}
	if src.buildCalls != 0 {
		t.Errorf("BuildTaskRequest must not run when profile is deleted, got %d calls", src.buildCalls)
	}
	if src.selfHealCalls != 1 {
		t.Fatalf("expected 1 SelfHeal call, got %d", src.selfHealCalls)
	}
	// Pin the invariant the settings UI relies on: the cause must carry
	// both the human-readable profile name and the profile id so an
	// operator can locate the removed row. A refactor that emits a bare
	// "profile not found" would still pass a `!= ""` check but break the
	// UI contract — assert the actual content.
	if !strings.Contains(src.selfHealCause, "Removed Kilo") ||
		!strings.Contains(src.selfHealCause, "deleted-profile") {
		t.Errorf("SelfHeal cause missing profile name or id: %q", src.selfHealCause)
	}
}

// TestFormatDeletedProfileCause_RendersBothBranches pins the two shapes the
// settings UI banner can render: a fully-named cause and the name-less
// fallback used when the row's name was cleared before deletion.
func TestFormatDeletedProfileCause_RendersBothBranches(t *testing.T) {
	withName := formatDeletedProfileCause("abc-123", "Kilo Profile")
	if !strings.Contains(withName, "Kilo Profile") || !strings.Contains(withName, "abc-123") {
		t.Errorf("with-name branch missing fields: %q", withName)
	}
	noName := formatDeletedProfileCause("abc-123", "")
	if strings.Contains(noName, "\"\"") {
		t.Errorf("empty-name branch must not emit empty quotes: %q", noName)
	}
	if !strings.Contains(noName, "abc-123") {
		t.Errorf("empty-name branch missing profile id: %q", noName)
	}
}

// TestFormatDeletedProfileCause_TruncatesLongName guards convention 6: a
// profile name is bounded at the producer, not by the consumer. An
// unbounded name would pollute last_error and the settings banner.
func TestFormatDeletedProfileCause_TruncatesLongName(t *testing.T) {
	long := strings.Repeat("x", 500)
	got := formatDeletedProfileCause("abc-123", long)
	if len([]rune(got)) > profileNameCauseMaxLen+64 {
		t.Errorf("cause too long (%d runes): %q", len([]rune(got)), got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("truncated cause must end the name with the ellipsis marker: %q", got)
	}
}

// TestWatcherDispatchCoordinator_HealthyProfilePassesPreflight pins the
// negative side: a non-deleted profile lookup does NOT short-circuit and
// the existing pipeline still runs.
func TestWatcherDispatchCoordinator_HealthyProfilePassesPreflight(t *testing.T) {
	src := &stubWatcherSource{name: "stub", agentProfileID: "live-profile"}
	lookup := &fakeProfileLookup{deleted: false}

	c := &WatcherDispatchCoordinator{
		profileLookup: lookup,
		shouldAutoStart: func(_ context.Context, _ string) bool {
			return false
		},
		logger: newTestLogger(),
	}
	c.SetTaskCreator(&countingIssueTaskCreator{taskID: "task-1"})

	c.Dispatch(context.Background(), src, struct{}{})

	if src.selfHealCalls != 0 {
		t.Errorf("SelfHeal must NOT run for a live profile, got %d calls", src.selfHealCalls)
	}
	if src.reserveCalls != 1 {
		t.Errorf("expected 1 Reserve call on the happy path, got %d", src.reserveCalls)
	}
	if src.buildCalls != 1 {
		t.Errorf("expected 1 BuildTaskRequest call on the happy path, got %d", src.buildCalls)
	}
}

// TestWatcherDispatchCoordinator_LookupErrorDoesNotBlockDispatch guards
// against turning a transient lookup failure (DB hiccup) into a watcher
// outage. When the lookup returns an error, the coordinator must
// fail-open and let the existing pipeline run; the legacy StartTask
// path still surfaces any genuine problem.
func TestWatcherDispatchCoordinator_LookupErrorDoesNotBlockDispatch(t *testing.T) {
	src := &stubWatcherSource{name: "stub", agentProfileID: "live-profile"}
	lookup := &fakeProfileLookup{err: errors.New("db unavailable")}

	c := &WatcherDispatchCoordinator{
		profileLookup: lookup,
		shouldAutoStart: func(_ context.Context, _ string) bool {
			return false
		},
		logger: newTestLogger(),
	}
	c.SetTaskCreator(&countingIssueTaskCreator{taskID: "task-1"})

	c.Dispatch(context.Background(), src, struct{}{})

	if src.selfHealCalls != 0 {
		t.Errorf("SelfHeal must NOT run on lookup error, got %d calls", src.selfHealCalls)
	}
	if src.reserveCalls != 1 {
		t.Errorf("expected pipeline to fall through on lookup error, got Reserve calls %d", src.reserveCalls)
	}
}

// TestWatcherDispatchCoordinator_EmptyProfileIDSkipsLookup keeps the
// legacy-zero-profile rows working: watchers created before the
// agent_profile_id column existed have an empty value, and the
// pre-flight check must not look them up (the lookup would return
// "not found" which is a different error path).
func TestWatcherDispatchCoordinator_EmptyProfileIDSkipsLookup(t *testing.T) {
	src := &stubWatcherSource{name: "stub", agentProfileID: ""}
	lookup := &fakeProfileLookup{}

	c := &WatcherDispatchCoordinator{
		profileLookup: lookup,
		shouldAutoStart: func(_ context.Context, _ string) bool {
			return false
		},
		logger: newTestLogger(),
	}
	c.SetTaskCreator(&countingIssueTaskCreator{taskID: "task-1"})

	c.Dispatch(context.Background(), src, struct{}{})

	if lookup.calls != 0 {
		t.Errorf("lookup must be skipped for empty profile id, got %d calls", lookup.calls)
	}
	if src.reserveCalls != 1 {
		t.Errorf("pipeline must still run, got Reserve calls %d", src.reserveCalls)
	}
}
