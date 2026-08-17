package clarification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// stubResolutionStore is an in-memory resolutionStore double: InsertClarificationResolution
// claims exactly once per pending_id (M8), returning (false, the winning row, nil) to every
// loser (R2); a configurable authorizeErr/insertErr let tests exercise M8a/M9.
type stubResolutionStore struct {
	rows          map[string]*taskmodels.ClarificationResolution
	insertErr     error
	resumeUpdates []struct{ pendingID, resume string }
	resumeErr     error
}

func newStubResolutionStore() *stubResolutionStore {
	return &stubResolutionStore{rows: map[string]*taskmodels.ClarificationResolution{}}
}

func (s *stubResolutionStore) InsertClarificationResolution(_ context.Context, res *taskmodels.ClarificationResolution) (bool, *taskmodels.ClarificationResolution, error) {
	if s.insertErr != nil {
		return false, nil, s.insertErr
	}
	if existing, ok := s.rows[res.PendingID]; ok {
		return false, existing, nil
	}
	s.rows[res.PendingID] = res
	return true, res, nil
}

func (s *stubResolutionStore) UpdateClarificationResolutionResume(_ context.Context, pendingID, resume string) error {
	s.resumeUpdates = append(s.resumeUpdates, struct{ pendingID, resume string }{pendingID, resume})
	if s.resumeErr != nil {
		return s.resumeErr
	}
	if row, ok := s.rows[pendingID]; ok {
		row.Resume = resume
	}
	return nil
}

// stubAuthorizer is a taskAccessAuthorizer double; a non-nil err denies every taskID (A3).
type stubAuthorizer struct {
	err error
}

func (a *stubAuthorizer) AuthorizeTaskAccess(context.Context, string) error { return a.err }

// stubResolverMessageUpdater is a clarificationMessageUpdater double that records every
// call in order (for D2 ordering assertions) and can be configured to fail on a specific
// question_id (R5a).
type stubResolverMessageUpdater struct {
	calls   []resolverUpdateCall
	failOnQ string
	failErr error
}

type resolverUpdateCall struct {
	sessionID, pendingID, questionID, status string
	answer                                   *Answer
}

func (u *stubResolverMessageUpdater) UpdateClarificationMessage(_ context.Context, sessionID, pendingID, questionID, status string, answer *Answer) error {
	u.calls = append(u.calls, resolverUpdateCall{sessionID, pendingID, questionID, status, answer})
	if u.failOnQ != "" && questionID == u.failOnQ {
		if u.failErr != nil {
			return u.failErr
		}
		return errors.New("simulated message update failure")
	}
	return nil
}

func questionMessage(id, sessionID, pendingID, questionID string, index int) *taskmodels.Message {
	return &taskmodels.Message{
		ID:            id,
		TaskSessionID: sessionID,
		TaskID:        "task-1",
		CreatedAt:     time.Date(2026, 1, 1, 0, 0, index, 0, time.UTC),
		Metadata: map[string]any{
			"pending_id":     pendingID,
			"question_id":    questionID,
			"question_index": index,
			"status":         "pending",
			"question":       map[string]any{"id": questionID, "prompt": "prompt " + questionID},
		},
	}
}

type resolverFixture struct {
	resolver    *Resolver
	store       *Store
	resolutions *stubResolutionStore
	repo        *stubMessageStore
	messages    *stubResolverMessageUpdater
	authorizer  *stubAuthorizer
	eventBus    *stubEventBus
}

func newResolverFixture(t *testing.T, msgs map[string][]*taskmodels.Message) *resolverFixture {
	t.Helper()
	f := &resolverFixture{
		store:       NewStore(time.Minute),
		resolutions: newStubResolutionStore(),
		repo:        &stubMessageStore{messages: msgs},
		messages:    &stubResolverMessageUpdater{},
		authorizer:  &stubAuthorizer{},
		eventBus:    &stubEventBus{},
	}
	f.resolver = NewResolver(f.store, f.resolutions, f.repo, f.messages, f.authorizer, f.eventBus, logger.Default())
	return f
}

// TestResolveBundle_NotFound_NoMessages proves an unknown pending_id is
// ErrBundleNotFound.
func TestResolveBundle_NotFound_NoMessages(t *testing.T) {
	f := newResolverFixture(t, map[string][]*taskmodels.Message{})
	_, _, err := f.resolver.ResolveBundle(context.Background(), "missing", Outcome{Rejected: true})
	if !errors.Is(err, ErrBundleNotFound) {
		t.Fatalf("expected ErrBundleNotFound, got %v", err)
	}
}

// TestResolveBundle_NotFound_AuthorizationDenied proves A3: a denied
// AuthorizeTaskAccess yields the same ErrBundleNotFound as a nonexistent
// bundle, never a distinguishable error.
func TestResolveBundle_NotFound_AuthorizationDenied(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{"p1": {questionMessage("m1", "s1", "p1", "q1", 0)}}
	f := newResolverFixture(t, msgs)
	f.authorizer.err = errors.New("denied")

	_, _, err := f.resolver.ResolveBundle(context.Background(), "p1", Outcome{Rejected: true})
	if !errors.Is(err, ErrBundleNotFound) {
		t.Fatalf("expected ErrBundleNotFound, got %v", err)
	}
}

// TestResolveBundle_NotFound_UnresolvableTaskID proves M5a: when every
// message's task_id is empty and the session row itself cannot be found,
// the bundle is unresolvable and ResolveBundle returns ErrBundleNotFound.
func TestResolveBundle_NotFound_UnresolvableTaskID(t *testing.T) {
	msg := questionMessage("m1", "s1", "p1", "q1", 0)
	msg.TaskID = ""
	msgs := map[string][]*taskmodels.Message{"p1": {msg}}
	f := newResolverFixture(t, msgs) // stubMessageStore.GetTaskSession always errors "not implemented"

	_, _, err := f.resolver.ResolveBundle(context.Background(), "p1", Outcome{Rejected: true})
	if !errors.Is(err, ErrBundleNotFound) {
		t.Fatalf("expected ErrBundleNotFound, got %v", err)
	}
}

// TestResolveBundle_ValidationError_PropagatesBeforeClaim proves N8c: an
// invalid outcome fails validation and never reaches the claim insert.
func TestResolveBundle_ValidationError_PropagatesBeforeClaim(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{"p1": {questionMessage("m1", "s1", "p1", "q1", 0)}}
	f := newResolverFixture(t, msgs)

	_, claimed, err := f.resolver.ResolveBundle(context.Background(), "p1", Outcome{
		Answers: []Answer{{QuestionID: "q1"}, {QuestionID: "extra"}},
	})
	if err == nil || !IsValidationError(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}
	if claimed {
		t.Fatalf("expected claimed=false on validation failure")
	}
	if len(f.resolutions.rows) != 0 {
		t.Fatalf("expected no claim row to be inserted, got %d", len(f.resolutions.rows))
	}
}

// TestResolveBundle_Cancel_SkipsValidation proves X5: a cancel outcome
// bypasses step-3 validation entirely, even against a garbage answers array
// that would otherwise fail N6.
func TestResolveBundle_Cancel_SkipsValidation(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{"p1": {questionMessage("m1", "s1", "p1", "q1", 0)}}
	f := newResolverFixture(t, msgs)

	res, claimed, err := f.resolver.ResolveBundle(context.Background(), "p1", Outcome{
		Cancel:  true,
		Answers: []Answer{{QuestionID: "not-a-real-question"}, {QuestionID: "another"}},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !claimed {
		t.Fatalf("expected claimed=true")
	}
	if res.Status != taskmodels.ClarificationResolutionStatusCancelled {
		t.Fatalf("expected status=cancelled, got %q", res.Status)
	}
}

// TestResolveBundle_WinningAnswer_DeliversToLiveWaiter proves R7: when a
// live in-memory waiter exists, delivery through Store.Respond wins,
// resume=published, and every message is updated in D2 order with its
// matching normalized answer.
func TestResolveBundle_WinningAnswer_DeliversToLiveWaiter(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"p1": {
			questionMessage("m2", "s1", "p1", "q2", 1),
			questionMessage("m1", "s1", "p1", "q1", 0),
		},
	}
	f := newResolverFixture(t, msgs)
	pendingID, _ := f.store.CreateRequest(&Request{
		SessionID: "s1",
		Questions: []Question{{ID: "q1"}, {ID: "q2"}},
	})
	// Re-key msgs under the real pendingID minted by CreateRequest.
	msgs[pendingID] = msgs["p1"]
	delete(msgs, "p1")
	for _, m := range msgs[pendingID] {
		m.Metadata["pending_id"] = pendingID
	}

	waiterDone := make(chan *Response, 1)
	go func() {
		resp, _ := f.store.WaitForResponse(context.Background(), pendingID)
		waiterDone <- resp
	}()

	res, claimed, err := f.resolver.ResolveBundle(context.Background(), pendingID, Outcome{
		Answers: []Answer{{QuestionID: "q1", CustomText: "one"}, {QuestionID: "q2", CustomText: "two"}},
		Source:  "mcp",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !claimed {
		t.Fatalf("expected claimed=true")
	}
	if res.Resume != taskmodels.ClarificationResolutionResumePublished {
		t.Fatalf("expected resume=published, got %q", res.Resume)
	}

	select {
	case delivered := <-waiterDone:
		if delivered == nil || len(delivered.Answers) != 2 {
			t.Fatalf("expected the waiter to receive the answers, got %+v", delivered)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter never received a response")
	}

	if len(f.messages.calls) != 2 {
		t.Fatalf("expected 2 message updates, got %d", len(f.messages.calls))
	}
	if f.messages.calls[0].questionID != "q1" || f.messages.calls[1].questionID != "q2" {
		t.Fatalf("expected D2 order q1, q2; got %q, %q", f.messages.calls[0].questionID, f.messages.calls[1].questionID)
	}
	if f.messages.calls[0].answer == nil || f.messages.calls[0].answer.CustomText != "one" {
		t.Fatalf("expected q1's own answer, got %+v", f.messages.calls[0].answer)
	}
}

// TestResolveBundle_WinningAnswer_NoWaiter_PublishesAnsweredEvent proves R8:
// with no live waiter, a successful clarification.answered publish yields
// resume=published.
func TestResolveBundle_WinningAnswer_NoWaiter_PublishesAnsweredEvent(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{"p1": {questionMessage("m1", "s1", "p1", "q1", 0)}}
	f := newResolverFixture(t, msgs)

	res, claimed, err := f.resolver.ResolveBundle(context.Background(), "p1", Outcome{
		Answers: []Answer{{QuestionID: "q1"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !claimed {
		t.Fatalf("expected claimed=true")
	}
	if res.Resume != taskmodels.ClarificationResolutionResumePublished {
		t.Fatalf("expected resume=published, got %q", res.Resume)
	}
	if len(f.eventBus.events) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(f.eventBus.events))
	}
}

// TestResolveBundle_Rejected_NoWaiter_AlwaysNotApplicable proves R9: a
// rejected bundle with no live waiter always resumes not_applicable, even
// though the stale_dismissed publish is best-effort.
func TestResolveBundle_Rejected_NoWaiter_AlwaysNotApplicable(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{"p1": {questionMessage("m1", "s1", "p1", "q1", 0)}}
	f := newResolverFixture(t, msgs)

	res, claimed, err := f.resolver.ResolveBundle(context.Background(), "p1", Outcome{Rejected: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !claimed {
		t.Fatalf("expected claimed=true")
	}
	if res.Resume != taskmodels.ClarificationResolutionResumeNotApplicable {
		t.Fatalf("expected resume=not_applicable, got %q", res.Resume)
	}
}

// TestResolveBundle_Cancelled_ResumeNotApplicable proves X4: a winning
// cancel resumes not_applicable.
func TestResolveBundle_Cancelled_ResumeNotApplicable(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{"p1": {questionMessage("m1", "s1", "p1", "q1", 0)}}
	f := newResolverFixture(t, msgs)

	res, claimed, err := f.resolver.ResolveBundle(context.Background(), "p1", Outcome{Cancel: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !claimed {
		t.Fatalf("expected claimed=true")
	}
	if res.Resume != taskmodels.ClarificationResolutionResumeNotApplicable {
		t.Fatalf("expected resume=not_applicable, got %q", res.Resume)
	}
}

// TestResolveBundle_LosingClaim_ReturnsWinnersStoredResponse proves R2: the
// second caller to resolve the same pending_id loses the claim and gets the
// winner's stored status/response back, without re-applying anything.
func TestResolveBundle_LosingClaim_ReturnsWinnersStoredResponse(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{"p1": {questionMessage("m1", "s1", "p1", "q1", 0)}}
	f := newResolverFixture(t, msgs)

	winner, claimed, err := f.resolver.ResolveBundle(context.Background(), "p1", Outcome{
		Answers: []Answer{{QuestionID: "q1", CustomText: "winner"}},
	})
	if err != nil || !claimed {
		t.Fatalf("expected the first call to win, got claimed=%v err=%v", claimed, err)
	}

	loser, claimed, err := f.resolver.ResolveBundle(context.Background(), "p1", Outcome{Rejected: true})
	if err != nil {
		t.Fatalf("unexpected error on losing call: %v", err)
	}
	if claimed {
		t.Fatalf("expected claimed=false for the losing call")
	}
	if loser.Status != winner.Status || loser.Response.Answers[0].CustomText != "winner" {
		t.Fatalf("expected the loser to receive the winner's stored response, got %+v", loser)
	}
	if len(f.messages.calls) != 1 {
		t.Fatalf("expected only the winner's message update, got %d calls", len(f.messages.calls))
	}
}

// TestResolveBundle_PartialApplicationFailure_ReturnsPartialApplicationError
// proves R5/R5a: when a message update fails partway through, ResolveBundle
// stops at the first failure, records resume=failed, and returns a
// partialApplicationError the caller must map to 500 rather than a success.
func TestResolveBundle_PartialApplicationFailure_ReturnsPartialApplicationError(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{
		"p1": {
			questionMessage("m1", "s1", "p1", "q1", 0),
			questionMessage("m2", "s1", "p1", "q2", 1),
		},
	}
	f := newResolverFixture(t, msgs)
	f.messages.failOnQ = "q2"

	res, claimed, err := f.resolver.ResolveBundle(context.Background(), "p1", Outcome{
		Answers: []Answer{{QuestionID: "q1"}, {QuestionID: "q2"}},
	})
	if err == nil || !IsPartialApplicationError(err) {
		t.Fatalf("expected a partial application error, got %v", err)
	}
	if !claimed {
		t.Fatalf("expected claimed=true: the row was inserted even though application failed")
	}
	if res != nil {
		t.Fatalf("expected a nil Resolution on failure, got %+v", res)
	}
	if len(f.messages.calls) != 2 {
		t.Fatalf("expected q1 to be applied before q2 failed, got %d calls", len(f.messages.calls))
	}
	if len(f.resolutions.resumeUpdates) != 1 || f.resolutions.resumeUpdates[0].resume != taskmodels.ClarificationResolutionResumeFailed {
		t.Fatalf("expected resume=failed to be persisted, got %+v", f.resolutions.resumeUpdates)
	}
}

// TestResolveBundle_M7a_ResumeReportedEvenWhenPersistFails proves M7a: the
// returned resume value comes from local computation, never a re-read of
// the resolutions row, so a failure to persist it does not turn a computed
// "published" into a reported "pending".
func TestResolveBundle_M7a_ResumeReportedEvenWhenPersistFails(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{"p1": {questionMessage("m1", "s1", "p1", "q1", 0)}}
	f := newResolverFixture(t, msgs)
	f.resolutions.resumeErr = errors.New("db unavailable")

	res, claimed, err := f.resolver.ResolveBundle(context.Background(), "p1", Outcome{
		Answers: []Answer{{QuestionID: "q1"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !claimed {
		t.Fatalf("expected claimed=true")
	}
	if res.Resume != taskmodels.ClarificationResolutionResumePublished {
		t.Fatalf("expected reported resume=published despite the persist failure, got %q", res.Resume)
	}
	if f.resolutions.rows["p1"].Resume != taskmodels.ClarificationResolutionResumePending {
		t.Fatalf("expected the row itself to remain stuck at pending, got %q", f.resolutions.rows["p1"].Resume)
	}
}

// TestResolveBundle_Cancel_ClosesCancelChOnLoss proves X3/X3a: a losing
// cancel still closes the in-memory CancelCh, even though it performs no
// other step-5 work.
func TestResolveBundle_Cancel_ClosesCancelChOnLoss(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{"p1": {questionMessage("m1", "s1", "p1", "q1", 0)}}
	f := newResolverFixture(t, msgs)
	pendingID, _ := f.store.CreateRequest(&Request{SessionID: "s1", Questions: []Question{{ID: "q1"}}})
	msgs[pendingID] = msgs["p1"]
	delete(msgs, "p1")

	// First call wins the claim by answering.
	if _, claimed, err := f.resolver.ResolveBundle(context.Background(), pendingID, Outcome{
		Answers: []Answer{{QuestionID: "q1"}},
	}); err != nil || !claimed {
		t.Fatalf("expected the answer to win the claim, got claimed=%v err=%v", claimed, err)
	}

	pending, ok := f.store.pending[pendingID]
	if !ok {
		t.Fatalf("expected the in-memory entry to still exist after a winning answer")
	}

	_, claimed, err := f.resolver.ResolveBundle(context.Background(), pendingID, Outcome{Cancel: true})
	if err != nil {
		t.Fatalf("unexpected error on losing cancel: %v", err)
	}
	if claimed {
		t.Fatalf("expected the cancel to lose the claim")
	}
	select {
	case <-pending.CancelCh:
	default:
		t.Fatalf("expected a losing cancel to still close CancelCh")
	}
}

// TestResolveBundle_ClaimInsertSessionMissing_ReturnsNotFound proves M8a:
// an insert failing its session foreign key is reported as ErrBundleNotFound.
func TestResolveBundle_ClaimInsertSessionMissing_ReturnsNotFound(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{"p1": {questionMessage("m1", "s1", "p1", "q1", 0)}}
	f := newResolverFixture(t, msgs)
	f.resolutions.insertErr = sqliterepo.ErrClarificationSessionMissing

	_, _, err := f.resolver.ResolveBundle(context.Background(), "p1", Outcome{Rejected: true})
	if !errors.Is(err, ErrBundleNotFound) {
		t.Fatalf("expected ErrBundleNotFound, got %v", err)
	}
}

// TestResolveBundle_ClaimInsertResolutionVanished_ReturnsNotFound proves M9:
// a conflicting row that vanishes between the conflict and the post-conflict
// read is also reported as ErrBundleNotFound.
func TestResolveBundle_ClaimInsertResolutionVanished_ReturnsNotFound(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{"p1": {questionMessage("m1", "s1", "p1", "q1", 0)}}
	f := newResolverFixture(t, msgs)
	f.resolutions.insertErr = sqliterepo.ErrClarificationResolutionNotFound

	_, _, err := f.resolver.ResolveBundle(context.Background(), "p1", Outcome{Rejected: true})
	if !errors.Is(err, ErrBundleNotFound) {
		t.Fatalf("expected ErrBundleNotFound, got %v", err)
	}
}
