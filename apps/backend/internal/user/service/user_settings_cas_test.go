package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/store"
	"go.uber.org/zap"
)

// casRead records the revision and editor observed by a single read.
type casRead struct {
	revision        int64
	defaultEditorID string
}

// casUpsert records one revision-conditional write attempt.
type casUpsert struct {
	expectedRevision int64
	defaultEditorID  string
	committed        bool
}

// casFakeRepo is a synchronized store.Repository fake for concurrent CAS
// service tests. It clones the current row on every read and commits an
// upsert only when the expected revision matches the current row revision
// (otherwise it returns store.ErrUserSettingsRevisionConflict), modeling the
// repository's revision-conditional write without a database.
type casFakeRepo struct {
	mu  sync.Mutex
	row *models.UserSettings

	reads   []casRead
	upserts []casUpsert

	// forceConflictNext makes the next upsert return a conflict without
	// committing, then clears itself (used to force a retry deterministically).
	forceConflictNext bool

	// forceConflictCount makes the next N upserts return a conflict without
	// committing (used to force retry exhaustion deterministically).
	forceConflictCount int

	// blockNextUpsert, when true, blocks the next upsert until releaseUpsert
	// is closed, then clears itself. Used to force a stale-CAS interleaving.
	blockNextUpsert bool
	releaseUpsert   chan struct{}

	// blockUpsert, when non-nil, blocks every upsert until the channel is
	// closed. Used to force both concurrent readers to observe the same
	// revision before either write commits.
	blockUpsert chan struct{}

	// upsertStarted and readDone are buffered phase-observation signals.
	upsertStarted chan struct{}
	readDone      chan struct{}
}

// newCASFakeRepo builds a casFakeRepo seeded with a clone of row.
func newCASFakeRepo(row *models.UserSettings) *casFakeRepo {
	return &casFakeRepo{row: cloneSettings(row)}
}

// snapshot returns a deep clone of the current row.
func (f *casFakeRepo) snapshot() *models.UserSettings {
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneSettings(f.row)
}

// readLog returns a copy of the recorded reads.
func (f *casFakeRepo) readLog() []casRead {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]casRead(nil), f.reads...)
}

// upsertLog returns a copy of the recorded upsert attempts.
func (f *casFakeRepo) upsertLog() []casUpsert {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]casUpsert(nil), f.upserts...)
}

// GetUser returns an unexpected-call error.
func (f *casFakeRepo) GetUser(context.Context, string) (*models.User, error) {
	return nil, errors.New("unexpected GetUser call")
}

// GetDefaultUser returns an unexpected-call error.
func (f *casFakeRepo) GetDefaultUser(context.Context) (*models.User, error) {
	return nil, errors.New("unexpected GetDefaultUser call")
}

// GetUserSettings returns a clone of the row, records the read, and signals phase observers.
func (f *casFakeRepo) GetUserSettings(_ context.Context, _ string) (*models.UserSettings, error) {
	f.mu.Lock()
	row := cloneSettings(f.row)
	f.reads = append(f.reads, casRead{revision: row.Revision, defaultEditorID: row.DefaultEditorID})
	sig := f.readDone
	f.mu.Unlock()
	if sig != nil {
		select {
		case sig <- struct{}{}:
		default:
		}
	}
	return row, nil
}

// UpsertUserSettingsPreservingTaskCreateLastUsed commits revision-conditionally, merging the task-create patch, or returns a revision conflict.
func (f *casFakeRepo) UpsertUserSettingsPreservingTaskCreateLastUsed(
	_ context.Context,
	settings *models.UserSettings,
	patch *models.TaskCreateLastUsed,
	expectedRevision int64,
) (*models.UserSettings, error) {
	f.mu.Lock()
	block := f.blockNextUpsert
	if block {
		f.blockNextUpsert = false
	}
	rel := f.releaseUpsert
	sig := f.upsertStarted
	gate := f.blockUpsert
	f.mu.Unlock()

	if sig != nil {
		select {
		case sig <- struct{}{}:
		default:
		}
	}
	if block {
		<-rel
	}
	if gate != nil {
		<-gate
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	record := casUpsert{expectedRevision: expectedRevision, defaultEditorID: settings.DefaultEditorID}
	if f.forceConflictNext {
		f.forceConflictNext = false
		f.upserts = append(f.upserts, record)
		return nil, store.ErrUserSettingsRevisionConflict
	}
	if f.forceConflictCount > 0 {
		f.forceConflictCount--
		f.upserts = append(f.upserts, record)
		return nil, store.ErrUserSettingsRevisionConflict
	}
	if f.row.Revision != expectedRevision {
		f.upserts = append(f.upserts, record)
		return nil, store.ErrUserSettingsRevisionConflict
	}
	next := cloneSettings(settings)
	next.TaskCreateLastUsed = applyTaskCreatePatchToFake(next.TaskCreateLastUsed, patch)
	next.Revision = f.row.Revision + 1
	f.row = next
	record.committed = true
	f.upserts = append(f.upserts, record)
	return cloneSettings(next), nil
}

// UpdateTaskCreateLastUsed returns an unexpected-call error.
func (f *casFakeRepo) UpdateTaskCreateLastUsed(context.Context, string, models.TaskCreateLastUsed) (*models.UserSettings, error) {
	return nil, errors.New("unexpected UpdateTaskCreateLastUsed call")
}

// Close is a no-op.
func (f *casFakeRepo) Close() error { return nil }

// casEventBus is a thread-safe recording bus for concurrent service tests.
type casEventBus struct {
	mu       sync.Mutex
	subjects []string
	events   []*bus.Event
}

// Publish records the published subject and event.
func (b *casEventBus) Publish(_ context.Context, subject string, event *bus.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subjects = append(b.subjects, subject)
	b.events = append(b.events, event)
	return nil
}

// count returns the number of published events.
func (b *casEventBus) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

// Subscribe returns an unexpected-call error.
func (b *casEventBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, errors.New("unexpected Subscribe call")
}

// QueueSubscribe returns an unexpected-call error.
func (b *casEventBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, errors.New("unexpected QueueSubscribe call")
}

// Request returns an unexpected-call error.
func (b *casEventBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, errors.New("unexpected Request call")
}

// Close is a no-op.
func (b *casEventBus) Close() {}

// IsConnected reports the fake bus as connected.
func (b *casEventBus) IsConnected() bool { return true }

// newCASService builds a Service wired to the given repository and bus with a nop logger.
func newCASService(repo store.Repository, eventBus bus.EventBus) *Service {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		panic(err)
	}
	return NewService(repo, eventBus, log)
}

// cloneSettings deep-clones settings via JSON so nested maps and slices are never aliased.
func cloneSettings(s *models.UserSettings) *models.UserSettings {
	if s == nil {
		return nil
	}
	// Deep clone via JSON so nested slices/maps (TaskCreateLastUsed workflow
	// maps, KanbanHiddenStepIDs, LspServerConfigs, KeyboardShortcuts, ...) are
	// never aliased with the fake's stored row: a caller mutating a read result
	// or a concurrent task-create patch must not corrupt the shared row.
	raw, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	var clone models.UserSettings
	if err := json.Unmarshal(raw, &clone); err != nil {
		panic(err)
	}
	return &clone
}

// applyTaskCreatePatchToFake mirrors the repository's task_create_last_used
// json merge for the fake: non-empty patch fields override the current value,
// and an empty branch is applied when the repository changed (clearing it).
func applyTaskCreatePatchToFake(current models.TaskCreateLastUsed, patch *models.TaskCreateLastUsed) models.TaskCreateLastUsed {
	if patch == nil {
		return current
	}
	if patch.RepositoryID != "" {
		current.RepositoryID = patch.RepositoryID
	}
	if patch.Branch != "" || patch.RepositoryID != "" {
		current.Branch = patch.Branch
	}
	if patch.AgentProfileID != "" {
		current.AgentProfileID = patch.AgentProfileID
	}
	if patch.ExecutorProfileID != "" {
		current.ExecutorProfileID = patch.ExecutorProfileID
	}
	if patch.WorkflowIDsByWorkspace != nil {
		if current.WorkflowIDsByWorkspace == nil {
			current.WorkflowIDsByWorkspace = map[string]string{}
		}
		for k, v := range patch.WorkflowIDsByWorkspace {
			if v == "" {
				delete(current.WorkflowIDsByWorkspace, k)
			} else {
				current.WorkflowIDsByWorkspace[k] = v
			}
		}
	}
	return current
}

// TestApplyLastSeenDisplay verifies applyLastSeenDisplay preserves nil, accepts absolute and relative, rejects invalid, and trims whitespace.
func TestApplyLastSeenDisplay(t *testing.T) {
	t.Run("nil leaves settings unchanged", func(t *testing.T) {
		settings := &models.UserSettings{LastSeenDisplay: models.LastSeenDisplayRelative}
		if err := applyLastSeenDisplay(settings, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.LastSeenDisplay != models.LastSeenDisplayRelative {
			t.Fatalf("LastSeenDisplay = %q, want relative", settings.LastSeenDisplay)
		}
	})

	t.Run("accepts absolute", func(t *testing.T) {
		settings := &models.UserSettings{LastSeenDisplay: models.LastSeenDisplayRelative}
		if err := applyLastSeenDisplay(settings, ptr("absolute")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.LastSeenDisplay != models.LastSeenDisplayAbsolute {
			t.Fatalf("LastSeenDisplay = %q, want absolute", settings.LastSeenDisplay)
		}
	})

	t.Run("accepts relative", func(t *testing.T) {
		settings := &models.UserSettings{}
		if err := applyLastSeenDisplay(settings, ptr("relative")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.LastSeenDisplay != models.LastSeenDisplayRelative {
			t.Fatalf("LastSeenDisplay = %q, want relative", settings.LastSeenDisplay)
		}
	})

	t.Run("rejects invalid", func(t *testing.T) {
		if err := applyLastSeenDisplay(&models.UserSettings{}, ptr("garbage")); err == nil {
			t.Fatal("expected error for invalid display, got nil")
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		settings := &models.UserSettings{}
		if err := applyLastSeenDisplay(settings, ptr("  relative  ")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.LastSeenDisplay != models.LastSeenDisplayRelative {
			t.Fatalf("LastSeenDisplay = %q, want relative", settings.LastSeenDisplay)
		}
	})
}

// TestPublishUserSettingsEventIncludesNormalizedLastSeenDisplay verifies the event normalizes an unknown last_seen_display to absolute.
func TestPublishUserSettingsEventIncludesNormalizedLastSeenDisplay(t *testing.T) {
	eventBus := &casEventBus{}
	svc := newCASService(&casFakeRepo{row: &models.UserSettings{}}, eventBus)
	svc.publishUserSettingsEvent(context.Background(), &models.UserSettings{LastSeenDisplay: "future_value"})

	if eventBus.count() != 1 {
		t.Fatalf("events = %d, want 1", eventBus.count())
	}
	eventData, ok := eventBus.events[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected event data map, got %T", eventBus.events[0].Data)
	}
	if got := eventData["last_seen_display"]; got != models.LastSeenDisplayAbsolute {
		t.Fatalf("last_seen_display = %#v, want %q", got, models.LastSeenDisplayAbsolute)
	}
}

// TestUpdateUserSettingsPublishesLastSeenDisplay verifies UpdateUserSettings publishes the applied last_seen_display.
func TestUpdateUserSettingsPublishesLastSeenDisplay(t *testing.T) {
	updated := &models.UserSettings{
		UserID:          store.DefaultUserID,
		LastSeenDisplay: models.LastSeenDisplayRelative,
	}
	repo := &recordingUserRepository{
		getSettings:        &models.UserSettings{UserID: store.DefaultUserID},
		preservingSettings: updated,
	}
	eventBus := &recordingEventBus{}
	svc := newCASService(repo, eventBus)

	if _, err := svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{LastSeenDisplay: ptr(models.LastSeenDisplayRelative)}); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	if len(eventBus.publishedEvents) != 1 {
		t.Fatalf("events = %d, want 1", len(eventBus.publishedEvents))
	}
	eventData, ok := eventBus.publishedEvents[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected event data map, got %T", eventBus.publishedEvents[0].Data)
	}
	if got := eventData["last_seen_display"]; got != models.LastSeenDisplayRelative {
		t.Fatalf("last_seen_display = %#v, want %q", got, models.LastSeenDisplayRelative)
	}
}

// TestUpdateUserSettingsRejectsInvalidLastSeenDisplay verifies an invalid last_seen_display is rejected without an upsert.
func TestUpdateUserSettingsRejectsInvalidLastSeenDisplay(t *testing.T) {
	repo := &recordingUserRepository{getSettings: &models.UserSettings{UserID: store.DefaultUserID}}
	eventBus := &recordingEventBus{}
	svc := newCASService(repo, eventBus)

	if _, err := svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{LastSeenDisplay: ptr("garbage")}); err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if repo.upsertUserSettingsPreservingLastUsedCalls != 0 {
		t.Fatalf("expected no upsert on validation failure, got %d", repo.upsertUserSettingsPreservingLastUsedCalls)
	}
}

// TestUpdateUserSettingsEmptyPatchIsNoop verifies an empty patch performs no write or publish.
func TestUpdateUserSettingsEmptyPatchIsNoop(t *testing.T) {
	base := &models.UserSettings{
		UserID:          store.DefaultUserID,
		LastSeenDisplay: models.LastSeenDisplayAbsolute,
		Revision:        7,
	}
	repo := &recordingUserRepository{getSettings: base}
	eventBus := &recordingEventBus{}
	svc := newCASService(repo, eventBus)

	got, err := svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{})
	if err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	if repo.upsertUserSettingsPreservingLastUsedCalls != 0 {
		t.Fatalf("empty patch upserts = %d, want 0", repo.upsertUserSettingsPreservingLastUsedCalls)
	}
	if len(eventBus.publishedEvents) != 0 {
		t.Fatalf("empty patch events = %d, want 0", len(eventBus.publishedEvents))
	}
	if got == nil || got.Revision != base.Revision {
		t.Fatalf("no-op returned settings = %+v, want current revision %d", got, base.Revision)
	}
}

// TestUpdateUserSettingsSameValuePatchIsNoop verifies a same-value patch performs no write or publish.
func TestUpdateUserSettingsSameValuePatchIsNoop(t *testing.T) {
	base := &models.UserSettings{
		UserID:          store.DefaultUserID,
		LastSeenDisplay: models.LastSeenDisplayRelative,
		Revision:        7,
	}
	repo := &recordingUserRepository{getSettings: base}
	eventBus := &recordingEventBus{}
	svc := newCASService(repo, eventBus)

	relative := models.LastSeenDisplayRelative
	got, err := svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{LastSeenDisplay: &relative})
	if err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	if repo.upsertUserSettingsPreservingLastUsedCalls != 0 {
		t.Fatalf("same-value patch upserts = %d, want 0", repo.upsertUserSettingsPreservingLastUsedCalls)
	}
	if len(eventBus.publishedEvents) != 0 {
		t.Fatalf("same-value patch events = %d, want 0", len(eventBus.publishedEvents))
	}
	if got == nil || got.Revision != base.Revision {
		t.Fatalf("no-op returned settings = %+v, want current revision %d", got, base.Revision)
	}
}

// TestUpdateUserSettingsCASExhaustionWrapsConflict verifies callers can
// distinguish a bounded retry exhaustion from an unexpected storage failure.
func TestUpdateUserSettingsCASExhaustionWrapsConflict(t *testing.T) {
	repo := newCASFakeRepo(&models.UserSettings{UserID: store.DefaultUserID})
	repo.forceConflictCount = maxUserSettingsCASAttempts
	svc := newCASService(repo, &casEventBus{})

	_, err := svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{
		LastSeenDisplay: ptr(models.LastSeenDisplayRelative),
	})
	if err == nil {
		t.Fatal("expected retry exhaustion error, got nil")
	}
	if !errors.Is(err, store.ErrUserSettingsRevisionConflict) {
		t.Fatalf("error = %v, want ErrUserSettingsRevisionConflict", err)
	}
	if !strings.Contains(err.Error(), "exhausted") {
		t.Fatalf("error = %v, want retry exhaustion context", err)
	}
	if got := len(repo.upsertLog()); got != maxUserSettingsCASAttempts {
		t.Fatalf("upsert attempts = %d, want %d", got, maxUserSettingsCASAttempts)
	}
}

// TestConcurrentPatchMergesLastSeenDisplay verifies concurrent patches merge via a single CAS retry.
func TestConcurrentPatchMergesLastSeenDisplay(t *testing.T) {
	repo := newCASFakeRepo(&models.UserSettings{
		UserID:          store.DefaultUserID,
		LastSeenDisplay: models.LastSeenDisplayAbsolute,
	})
	repo.readDone = make(chan struct{}, 4)
	repo.blockUpsert = make(chan struct{})
	eventBus := &casEventBus{}
	svc := newCASService(repo, eventBus)

	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := svc.UpdateUserSettings(ctx, &UpdateUserSettingsRequest{LastSeenDisplay: ptr(models.LastSeenDisplayRelative)}); err != nil {
			t.Errorf("relative patch: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := svc.UpdateUserSettings(ctx, &UpdateUserSettingsRequest{AppStatusBarEnabled: ptr(true)}); err != nil {
			t.Errorf("status-bar patch: %v", err)
		}
	}()

	// Barrier: both initial reads must observe the same revision before either
	// write commits, so one stale upsert is forced to conflict and retry.
	<-repo.readDone
	<-repo.readDone
	close(repo.blockUpsert)
	wg.Wait()

	row := repo.snapshot()
	if row.LastSeenDisplay != models.LastSeenDisplayRelative {
		t.Fatalf("final LastSeenDisplay = %q, want relative", row.LastSeenDisplay)
	}
	if !row.AppStatusBarEnabled {
		t.Fatal("final AppStatusBarEnabled = false, want true")
	}
	if row.Revision != 2 {
		t.Fatalf("final revision = %d, want 2", row.Revision)
	}
	if got := eventBus.count(); got != 2 {
		t.Fatalf("events = %d, want 2", got)
	}
	assertOneStaleConflictAndRetry(t, repo)
}

// TestCASFakeCloneIsDeep pins the fake's clone contract: mutating a read
// result's nested maps must not corrupt the fake's stored row.
func TestCASFakeCloneIsDeep(t *testing.T) {
	repo := newCASFakeRepo(&models.UserSettings{
		UserID: store.DefaultUserID,
		TaskCreateLastUsed: models.TaskCreateLastUsed{
			WorkflowIDsByWorkspace: map[string]string{"w1": "f1"},
		},
		KeyboardShortcuts: map[string]interface{}{},
	})

	got, err := repo.GetUserSettings(context.Background(), store.DefaultUserID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got.TaskCreateLastUsed.WorkflowIDsByWorkspace["w1"] = "mutated"
	got.KeyboardShortcuts["x"] = map[string]interface{}{"key": "y"}

	row := repo.snapshot()
	if row.TaskCreateLastUsed.WorkflowIDsByWorkspace["w1"] != "f1" {
		t.Fatalf("read clone aliased the workflow map: %+v", row.TaskCreateLastUsed.WorkflowIDsByWorkspace)
	}
	if _, ok := row.KeyboardShortcuts["x"]; ok {
		t.Fatal("read clone aliased the keyboard shortcuts map")
	}
}

// TestConcurrentTaskCreateMapPatchesMerge proves two concurrent task-create
// patches with workflow maps survive the CAS retry and merge without
// corrupting the fake's stored row (deep clone requirement).
func TestConcurrentTaskCreateMapPatchesMerge(t *testing.T) {
	repo := newCASFakeRepo(&models.UserSettings{
		UserID: store.DefaultUserID,
		TaskCreateLastUsed: models.TaskCreateLastUsed{
			WorkflowIDsByWorkspace: map[string]string{"w0": "f0"},
		},
	})
	repo.readDone = make(chan struct{}, 4)
	repo.blockUpsert = make(chan struct{})
	eventBus := &casEventBus{}
	svc := newCASService(repo, eventBus)

	ctx := context.Background()
	patchA := models.TaskCreateLastUsed{WorkflowIDsByWorkspace: map[string]string{"wA": "fA"}}
	patchB := models.TaskCreateLastUsed{WorkflowIDsByWorkspace: map[string]string{"wB": "fB"}}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := svc.UpdateUserSettings(ctx, &UpdateUserSettingsRequest{TaskCreateLastUsed: &patchA}); err != nil {
			t.Errorf("patch A: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := svc.UpdateUserSettings(ctx, &UpdateUserSettingsRequest{TaskCreateLastUsed: &patchB}); err != nil {
			t.Errorf("patch B: %v", err)
		}
	}()

	<-repo.readDone
	<-repo.readDone
	close(repo.blockUpsert)
	wg.Wait()

	row := repo.snapshot()
	workflows := row.TaskCreateLastUsed.WorkflowIDsByWorkspace
	if workflows["w0"] != "f0" || workflows["wA"] != "fA" || workflows["wB"] != "fB" {
		t.Fatalf("workflow maps did not merge: %+v", workflows)
	}
	assertOneStaleConflictAndRetry(t, repo)
}

// assertOneStaleConflictAndRetry pins the CAS retry interleaving: one write
// commits at the shared initial revision, the other conflicts once and
// re-reads/re-applies, so the repo sees two commits, exactly one stale
// conflict, and a fresh retry re-read.
func assertOneStaleConflictAndRetry(t *testing.T, repo *casFakeRepo) {
	t.Helper()
	var commits, conflicts int
	for _, u := range repo.upsertLog() {
		if u.committed {
			commits++
		} else {
			conflicts++
		}
	}
	if commits != 2 {
		t.Fatalf("commits = %d, want 2", commits)
	}
	if conflicts != 1 {
		t.Fatalf("stale conflicts = %d, want exactly 1", conflicts)
	}
	if reads := len(repo.readLog()); reads != 3 {
		t.Fatalf("reads = %d, want 3 (two initial + one retry re-read)", reads)
	}
}

// TestConcurrentClearEditorAndPatchMerge verifies a concurrent Clear and patch both commit via a single CAS retry.
func TestConcurrentClearEditorAndPatchMerge(t *testing.T) {
	repo := newCASFakeRepo(&models.UserSettings{
		UserID:          store.DefaultUserID,
		DefaultEditorID: "editor-1",
		LastSeenDisplay: models.LastSeenDisplayAbsolute,
	})
	repo.readDone = make(chan struct{}, 4)
	repo.blockUpsert = make(chan struct{})
	eventBus := &casEventBus{}
	svc := newCASService(repo, eventBus)

	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := svc.ClearDefaultEditorID(ctx, "editor-1"); err != nil {
			t.Errorf("clear editor: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := svc.UpdateUserSettings(ctx, &UpdateUserSettingsRequest{LastSeenDisplay: ptr(models.LastSeenDisplayRelative)}); err != nil {
			t.Errorf("relative patch: %v", err)
		}
	}()

	// Barrier: both initial reads must observe the same revision before either
	// write commits, so one stale upsert is forced to conflict and retry.
	<-repo.readDone
	<-repo.readDone
	close(repo.blockUpsert)
	wg.Wait()

	row := repo.snapshot()
	if row.DefaultEditorID != "" {
		t.Fatalf("final DefaultEditorID = %q, want empty (cleared)", row.DefaultEditorID)
	}
	if row.LastSeenDisplay != models.LastSeenDisplayRelative {
		t.Fatalf("final LastSeenDisplay = %q, want relative", row.LastSeenDisplay)
	}
	if row.Revision != 2 {
		t.Fatalf("final revision = %d, want 2", row.Revision)
	}
	if got := eventBus.count(); got != 2 {
		t.Fatalf("events = %d, want 2", got)
	}
	assertOneStaleConflictAndRetry(t, repo)
}

// TestClearDefaultEditorIDConflictNoop forces a real CAS conflict for a Clear
// whose stale read saw the old editor: the callback re-evaluates against the
// retry's fresh read and returns applied=false, so the Clear writes and
// publishes nothing while the competing PATCH's effect survives.
func TestClearDefaultEditorIDConflictNoop(t *testing.T) {
	repo := newCASFakeRepo(&models.UserSettings{
		UserID:          store.DefaultUserID,
		DefaultEditorID: "old-editor",
	})
	repo.blockNextUpsert = true
	repo.releaseUpsert = make(chan struct{})
	repo.upsertStarted = make(chan struct{}, 4)
	repo.readDone = make(chan struct{}, 4)
	eventBus := &casEventBus{}
	svc := newCASService(repo, eventBus)

	ctx := context.Background()
	clearDone := make(chan error, 1)
	go func() {
		clearDone <- svc.ClearDefaultEditorID(ctx, "old-editor")
	}()

	// PHASE 1: Clear reads the old editor, then blocks at its upsert.
	<-repo.readDone
	<-repo.upsertStarted

	// PHASE 2: a competing PATCH commits a new editor.
	if _, err := svc.UpdateUserSettings(ctx, &UpdateUserSettingsRequest{DefaultEditorID: ptr("new-editor")}); err != nil {
		t.Fatalf("competing patch: %v", err)
	}

	// PHASE 3: release the stale Clear upsert; it conflicts and re-evaluates.
	close(repo.releaseUpsert)
	if err := <-clearDone; err != nil {
		t.Fatalf("ClearDefaultEditorID: %v", err)
	}

	row := repo.snapshot()
	if row.DefaultEditorID != "new-editor" {
		t.Fatalf("final DefaultEditorID = %q, want new-editor", row.DefaultEditorID)
	}
	if row.Revision != 1 {
		t.Fatalf("final revision = %d, want 1", row.Revision)
	}
	if got := eventBus.count(); got != 1 {
		t.Fatalf("events = %d, want exactly 1 (the PATCH)", got)
	}

	// The Clear contributed exactly one upsert attempt (the forced stale CAS)
	// and no successful write.
	var clearAttempts, clearCommits int
	for _, u := range repo.upsertLog() {
		if u.defaultEditorID == "" {
			clearAttempts++
			if u.committed {
				clearCommits++
			}
		}
	}
	if clearAttempts != 1 {
		t.Fatalf("clear upsert attempts = %d, want 1", clearAttempts)
	}
	if clearCommits != 0 {
		t.Fatalf("clear successful upserts = %d, want 0", clearCommits)
	}
	if len(repo.readLog()) < 3 {
		t.Fatalf("reads = %d, want at least 3 (clear read, patch read, clear retry re-read)", len(repo.readLog()))
	}
}

// TestClearDefaultEditorIDFreshReadNoop verifies Clear after the competing patch committed is an immediate no-op.
func TestClearDefaultEditorIDFreshReadNoop(t *testing.T) {
	repo := newCASFakeRepo(&models.UserSettings{
		UserID:          store.DefaultUserID,
		DefaultEditorID: "old-editor",
	})
	eventBus := &casEventBus{}
	svc := newCASService(repo, eventBus)

	ctx := context.Background()
	if _, err := svc.UpdateUserSettings(ctx, &UpdateUserSettingsRequest{DefaultEditorID: ptr("new-editor")}); err != nil {
		t.Fatalf("competing patch: %v", err)
	}

	// Clear starts after the PATCH committed: immediate no-op, no conflict.
	if err := svc.ClearDefaultEditorID(ctx, "old-editor"); err != nil {
		t.Fatalf("ClearDefaultEditorID: %v", err)
	}

	row := repo.snapshot()
	if row.DefaultEditorID != "new-editor" {
		t.Fatalf("final DefaultEditorID = %q, want new-editor", row.DefaultEditorID)
	}
	if got := eventBus.count(); got != 1 {
		t.Fatalf("events = %d, want exactly 1 (the PATCH)", got)
	}
	if len(repo.upsertLog()) != 1 {
		t.Fatalf("upserts = %d, want exactly 1 (the PATCH)", len(repo.upsertLog()))
	}
}

// TestUpdateUserSettingsTaskCreateOnlyConflictRetry pins the task-create-only
// PATCH under the CAS rewrite: a forced conflict survives the retry, merges
// against the winner's row, and increments the revision once on its own
// successful retry.
func TestUpdateUserSettingsTaskCreateOnlyConflictRetry(t *testing.T) {
	repo := newCASFakeRepo(&models.UserSettings{
		UserID: store.DefaultUserID,
		TaskCreateLastUsed: models.TaskCreateLastUsed{
			RepositoryID: "repo-1",
		},
	})
	eventBus := &casEventBus{}
	svc := newCASService(repo, eventBus)

	ctx := context.Background()

	// Winner commits first, bumping R to R+1.
	if _, err := svc.UpdateUserSettings(ctx, &UpdateUserSettingsRequest{AppStatusBarEnabled: ptr(true)}); err != nil {
		t.Fatalf("winner patch: %v", err)
	}

	// Force the task-create-only PATCH's first upsert into a conflict.
	repo.mu.Lock()
	repo.forceConflictNext = true
	repo.mu.Unlock()

	patch := models.TaskCreateLastUsed{RepositoryID: "repo-2"}
	if _, err := svc.UpdateUserSettings(ctx, &UpdateUserSettingsRequest{TaskCreateLastUsed: &patch}); err != nil {
		t.Fatalf("task-create patch: %v", err)
	}

	row := repo.snapshot()
	if !row.AppStatusBarEnabled {
		t.Fatal("winner's AppStatusBarEnabled was reverted by the retry")
	}
	if row.TaskCreateLastUsed.RepositoryID != "repo-2" {
		t.Fatalf("final task-create repository = %q, want repo-2", row.TaskCreateLastUsed.RepositoryID)
	}
	if row.Revision != 2 {
		t.Fatalf("final revision = %d, want 2", row.Revision)
	}
	if got := eventBus.count(); got != 2 {
		t.Fatalf("events = %d, want 2", got)
	}
}
