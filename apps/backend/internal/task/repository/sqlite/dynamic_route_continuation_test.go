package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	d "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
	"github.com/kandev/kandev/internal/task/models"
)

type continuationHarness struct {
	repo      *Repository
	engine    *d.Engine
	conductor *d.Conductor
	profile   d.Profile
	decision  d.RouteDecision
	now       time.Time
}

func newContinuationHarness(t *testing.T) *continuationHarness {
	t.Helper()
	return newContinuationHarnessWith(t, nil)
}

// newContinuationHarnessWith lets a test wrap the engine's route persistence;
// continuation saves and state loads still go straight to the repository.
func newContinuationHarnessWith(t *testing.T, wrap func(*Repository) d.Persistence) *continuationHarness {
	t.Helper()
	r := newRepoForSessionTests(t)
	ctx := context.Background()
	var workspaceID string
	if err := r.db.QueryRow(`SELECT id FROM workspaces ORDER BY id LIMIT 1`).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateTask(ctx, &models.Task{ID: "handoff-task", WorkspaceID: workspaceID, Title: "original task"}); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateTaskSession(ctx, &models.TaskSession{ID: "handoff-session", TaskID: "handoff-task", State: models.TaskSessionStateStarting}); err != nil {
		t.Fatal(err)
	}
	h := &continuationHarness{repo: r, now: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
	var persistence d.Persistence = r
	if wrap != nil {
		persistence = wrap(r)
	}
	h.engine = d.NewEngine(d.WithPersistence(persistence), d.WithStateLoader(r), d.WithClock(func() time.Time { return h.now }))
	h.conductor = d.NewConductor(h.engine, nil, nil, d.WithContinuationPersistence(r))
	doc := routingpolicy.DefaultDocument()
	doc.Transient.Retry = routingpolicy.RetryPolicy{Enabled: true, MaxRetries: 1, InitialIntervalSeconds: 5}
	h.profile = d.Profile{ID: "original-profile", Version: 1, Candidates: []d.Candidate{{ID: "A", Enabled: true, BindingKey: "A", Policies: doc}}}
	var err error
	h.decision, err = h.engine.SelectContext(ctx, "handoff-session", h.profile, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *continuationHarness) persist(t *testing.T, conversation string) {
	t.Helper()
	if err := h.conductor.PersistContinuation(context.Background(), h.decision, d.Continuation{Conversation: conversation}); err != nil {
		t.Fatal(err)
	}
}

func (h *continuationHarness) want(t *testing.T, conversation, status string) {
	t.Helper()
	state, err := h.repo.LoadRouteState(context.Background(), h.decision.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	var got d.Continuation
	if err := json.Unmarshal([]byte(state.ContinuationJSON), &got); err != nil {
		t.Fatal(err)
	}
	if got.Conversation != conversation || state.Status != status || state.Generation != h.decision.Generation {
		t.Fatalf("stored handoff/state mismatch: continuation=%q status=%s generation=%d", got.Conversation, state.Status, state.Generation)
	}
}

func (h *continuationHarness) wait(t *testing.T) {
	t.Helper()
	pending, err := h.engine.ApplyFailureContext(context.Background(), h.decision.SessionID, h.profile, h.decision.Generation, "A", rateLimited())
	if !errors.Is(err, d.ErrRecoveryPending) || pending.Status != "retry_wait" {
		t.Fatalf("retry wait: %#v %v", pending, err)
	}
}

func TestContinuationConsistencyStarting(t *testing.T) {
	h := newContinuationHarness(t)
	h.persist(t, "starting handoff")
	if err := h.engine.MarkActive(context.Background(), h.decision.SessionID, h.decision.Generation); err != nil {
		t.Fatal(err)
	}
	h.want(t, "starting handoff", "active")
}

func TestContinuationConsistencyRetrying(t *testing.T) {
	h := newContinuationHarness(t)
	h.wait(t)
	h.now = h.now.Add(10 * time.Second)
	decision, err := h.engine.ResumePending(context.Background(), h.decision.SessionID, h.decision.Generation)
	if err != nil || decision.Status != "retrying" {
		t.Fatalf("resume: %#v %v", decision, err)
	}
	h.persist(t, "retrying handoff")
	if err := h.engine.MarkActive(context.Background(), h.decision.SessionID, h.decision.Generation); err != nil {
		t.Fatal(err)
	}
	h.want(t, "retrying handoff", "active")
}

func TestContinuationConsistencyCancelled(t *testing.T) {
	h := newContinuationHarness(t)
	h.persist(t, "cancelled handoff")
	h.wait(t)
	if _, err := h.engine.CancelPending(context.Background(), h.decision.SessionID, h.decision.Generation, "existing stop"); err != nil {
		t.Fatal(err)
	}
	h.now = h.now.Add(time.Minute)
	if _, err := h.engine.ResumePending(context.Background(), h.decision.SessionID, h.decision.Generation); !errors.Is(err, d.ErrRecoveryPending) {
		t.Fatalf("cancelled route resumed: %v", err)
	}
	h.want(t, "cancelled handoff", "action_required")
}

func TestContinuationConsistencyStaleGeneration(t *testing.T) {
	h := newContinuationHarness(t)
	h.persist(t, "latest handoff")
	stale := h.decision
	stale.Generation--
	err := h.conductor.PersistContinuation(context.Background(), stale, d.Continuation{Conversation: "stale overwrite"})
	if !errors.Is(err, d.ErrStaleGeneration) {
		t.Fatalf("stale save: %v", err)
	}
	if err := h.engine.MarkActive(context.Background(), h.decision.SessionID, h.decision.Generation); err != nil {
		t.Fatal(err)
	}
	h.want(t, "latest handoff", "active")
}

type failedContinuationStore struct{ err error }

func (s failedContinuationStore) SaveRouteContinuation(context.Context, d.ContinuationRecord) error {
	return s.err
}

func TestContinuationConsistencyFailedSave(t *testing.T) {
	h := newContinuationHarness(t)
	h.persist(t, "committed handoff")
	wantErr := errors.New("isolated continuation save failure")
	c := d.NewConductor(h.engine, nil, nil, d.WithContinuationPersistence(failedContinuationStore{err: wantErr}))
	if err := c.PersistContinuation(context.Background(), h.decision, d.Continuation{Conversation: "uncommitted handoff"}); !errors.Is(err, wantErr) {
		t.Fatalf("save error: %v", err)
	}
	if err := h.engine.MarkActive(context.Background(), h.decision.SessionID, h.decision.Generation); err != nil {
		t.Fatal(err)
	}
	h.want(t, "committed handoff", "active")
}
