package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/events/bus"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// recordingBundleLister is a ClarificationBundleLister test double that
// records the options it was called with, for tests that only need to
// inspect request parsing/visibility resolution rather than exercise a real
// database.
type recordingBundleLister struct {
	gotOpts models.ListClarificationBundlesOptions
	page    *models.ClarificationBundlePage
	pages   []*models.ClarificationBundlePage
	calls   int
	msgs    map[string][]*models.Message
	listErr error
}

func (r *recordingBundleLister) ListUnresolvedClarificationBundles(_ context.Context, opts models.ListClarificationBundlesOptions) (*models.ClarificationBundlePage, error) {
	r.gotOpts = opts
	r.calls++
	if r.listErr != nil {
		return nil, r.listErr
	}
	if len(r.pages) > 0 {
		page := r.pages[r.calls-1]
		if page == nil {
			return &models.ClarificationBundlePage{}, nil
		}
		return page, nil
	}
	if r.page == nil {
		return &models.ClarificationBundlePage{}, nil
	}
	return r.page, nil
}

func (r *recordingBundleLister) FindMessagesByPendingID(_ context.Context, pendingID string) ([]*models.Message, error) {
	return r.msgs[pendingID], nil
}

func questionMessage(pendingID, questionID, status string, index int, question map[string]any) *models.Message {
	meta := map[string]any{
		"pending_id":     pendingID,
		"question_id":    questionID,
		"question_index": index,
		"status":         status,
		"context":        "shared context",
	}
	if question != nil {
		meta["question"] = question
	}
	return &models.Message{ID: pendingID + "-" + questionID, Metadata: meta}
}

// --- list_pending_questions_kandev: request parsing / visibility ---

func TestHandleListPendingQuestions_UnscopedCallerSetsUnscopedTrue(t *testing.T) {
	svc, _ := newTestTaskService(t)
	lister := &recordingBundleLister{}
	h := &Handlers{taskSvc: svc, clarificationBundles: lister, logger: testLogger(t)}

	msg := makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{})
	resp, err := h.handleListPendingQuestions(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	require.True(t, lister.gotOpts.Unscoped)
	require.Empty(t, lister.gotOpts.VisibleWorkspaceIDs)
}

func TestHandleListPendingQuestions_ScopedCallerPassesVisibleWorkspaceIDs(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-owned", Name: "Mine", OwnerID: "user-1"}))
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-other", Name: "Theirs", OwnerID: "user-2"}))

	lister := &recordingBundleLister{}
	h := &Handlers{taskSvc: svc, clarificationBundles: lister, logger: testLogger(t)}

	scopedCtx := authn.WithIdentity(ctx, authn.Identity{UserID: "user-1"})
	msg := makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{})
	resp, err := h.handleListPendingQuestions(scopedCtx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	require.False(t, lister.gotOpts.Unscoped)
	// The fixture DB also seeds an unowned default workspace, which stays
	// visible to every caller (L1c) until claimed - assert on membership
	// rather than exact equality so that seed doesn't make this test brittle.
	require.Contains(t, lister.gotOpts.VisibleWorkspaceIDs, "ws-owned")
	require.NotContains(t, lister.gotOpts.VisibleWorkspaceIDs, "ws-other")
}

func TestHandleListPendingQuestions_ClampsLimit(t *testing.T) {
	cases := []struct {
		name  string
		input int
		want  int
	}{
		{"below one defaults to 50", 0, defaultPendingQuestionsLimit},
		{"negative defaults to 50", -5, defaultPendingQuestionsLimit},
		{"above cap clamps to 200", 500, maxPendingQuestionsLimit},
		{"in range passes through", 10, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newTestTaskService(t)
			lister := &recordingBundleLister{}
			h := &Handlers{taskSvc: svc, clarificationBundles: lister, logger: testLogger(t)}

			msg := makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{"limit": tc.input})
			_, err := h.handleListPendingQuestions(context.Background(), msg)
			require.NoError(t, err)
			require.Equal(t, tc.want, lister.gotOpts.Limit)
		})
	}
}

func TestHandleListPendingQuestions_UnparseableCreatedSince_ValidationError(t *testing.T) {
	svc, _ := newTestTaskService(t)
	h := &Handlers{taskSvc: svc, clarificationBundles: &recordingBundleLister{}, logger: testLogger(t)}

	msg := makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{"created_since": "not-a-date"})
	resp, err := h.handleListPendingQuestions(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)

	var ep ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &ep))
	require.Contains(t, ep.Message, "created_since")
}

func TestHandleListPendingQuestions_UnparseableCursor_ValidationError(t *testing.T) {
	svc, _ := newTestTaskService(t)
	h := &Handlers{taskSvc: svc, clarificationBundles: &recordingBundleLister{}, logger: testLogger(t)}

	msg := makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{"cursor": "!!!not-base64!!!"})
	resp, err := h.handleListPendingQuestions(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)

	var ep ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &ep))
	require.Contains(t, ep.Message, "cursor")
}

func TestHandleListPendingQuestions_CursorRoundTrip(t *testing.T) {
	svc, _ := newTestTaskService(t)
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cursor := encodeBundleCursor(createdAt, "pending-9")

	lister := &recordingBundleLister{}
	h := &Handlers{taskSvc: svc, clarificationBundles: lister, logger: testLogger(t)}

	msg := makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{"cursor": cursor})
	_, err := h.handleListPendingQuestions(context.Background(), msg)
	require.NoError(t, err)

	require.True(t, lister.gotOpts.CursorCreatedAt.Equal(createdAt))
	require.Equal(t, "pending-9", lister.gotOpts.CursorPendingID)
}

func TestHandleListPendingQuestions_EmptyResult_NoErrorEmptyEnvelope(t *testing.T) {
	svc, _ := newTestTaskService(t)
	h := &Handlers{taskSvc: svc, clarificationBundles: &recordingBundleLister{}, logger: testLogger(t)}

	msg := makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{})
	resp, err := h.handleListPendingQuestions(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var body listPendingQuestionsResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	require.Empty(t, body.Bundles)
	require.Equal(t, 0, body.Count)
	require.Equal(t, "", body.NextCursor)

	// L11: bundles must be an empty array, never a null/omitted key.
	require.Contains(t, string(resp.Payload), `"bundles":[]`)
}

func TestHandleListPendingQuestions_AutomationPaginatesPastSelfBundle(t *testing.T) {
	svc, _ := newTestTaskService(t)
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)
	lister := &recordingBundleLister{
		pages: []*models.ClarificationBundlePage{
			{Bundles: []models.ClarificationBundleSummary{{
				PendingID: "self-pending", TaskID: "automation-task", SessionID: "self-session", CreatedAt: first,
			}}, HasMore: true},
			{Bundles: []models.ClarificationBundleSummary{{
				PendingID: "foreign-pending", TaskID: "foreign-task", SessionID: "foreign-session", CreatedAt: second,
			}}},
		},
		msgs: map[string][]*models.Message{
			"foreign-pending": {questionMessage("foreign-pending", "q1", "pending", 0, nil)},
		},
	}
	h := &Handlers{taskSvc: svc, clarificationBundles: lister, logger: testLogger(t)}
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		AutomationID: "automation-1", WorkspaceID: "ws-1",
		CallerTaskID: "automation-task", CallerSessionID: "automation-session",
		Surface: mcpprofile.SurfaceAutomation,
	})

	resp, err := h.handleListPendingQuestions(ctx, makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{
		"limit": 1,
	}))
	require.NoError(t, err)
	var body listPendingQuestionsResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	require.Equal(t, 1, body.Count)
	require.Equal(t, "foreign-pending", body.Bundles[0].PendingID)
	require.Equal(t, 2, lister.calls)
	require.Equal(t, "self-pending", lister.gotOpts.CursorPendingID)
	require.Empty(t, body.NextCursor)
}

// TestHandleListPendingQuestions_L3L4ResponseShape proves the L11 envelope
// and L3/L4 per-bundle/per-question fields, including L4a's never-null rule
// for a question with no parseable metadata (L15).
func TestHandleListPendingQuestions_L3L4ResponseShape(t *testing.T) {
	svc, _ := newTestTaskService(t)
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	lister := &recordingBundleLister{
		page: &models.ClarificationBundlePage{
			Bundles: []models.ClarificationBundleSummary{
				{PendingID: "p1", TaskID: "t1", SessionID: "s1", CreatedAt: createdAt},
			},
			HasMore: true,
		},
		msgs: map[string][]*models.Message{
			"p1": {
				questionMessage("p1", "q1", "pending", 0, map[string]any{
					"id": "q1", "title": "Color", "prompt": "Pick one",
					"options": []interface{}{
						map[string]interface{}{"option_id": "opt-a", "label": "Red", "description": "Red option"},
					},
				}),
				questionMessage("p1", "q2", "pending", 1, nil), // L15: unparseable question metadata
			},
		},
	}
	h := &Handlers{taskSvc: svc, clarificationBundles: lister, logger: testLogger(t)}

	msg := makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{})
	resp, err := h.handleListPendingQuestions(context.Background(), msg)
	require.NoError(t, err)

	var body listPendingQuestionsResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	require.Equal(t, 1, body.Count)
	require.Len(t, body.Bundles, 1)

	b := body.Bundles[0]
	require.Equal(t, "p1", b.PendingID)
	require.Equal(t, "t1", b.TaskID)
	require.Equal(t, "s1", b.SessionID)
	require.Equal(t, "2026-01-01T00:00:00Z", b.CreatedAt)
	require.GreaterOrEqual(t, b.AgeSeconds, int64(0))
	require.Equal(t, "shared context", b.Context)
	require.NotEmpty(t, body.NextCursor) // page.HasMore

	require.Len(t, b.Questions, 2)
	q1 := b.Questions[0]
	require.Equal(t, "q1", q1.QuestionID)
	require.Equal(t, "Color", q1.Title)
	require.Equal(t, "Pick one", q1.Prompt)
	require.Equal(t, "pending", q1.Status)
	require.Len(t, q1.Options, 1)
	require.Equal(t, "opt-a", q1.Options[0].ID)
	require.Equal(t, "Red", q1.Options[0].Label)
	require.Equal(t, "Red option", q1.Options[0].Description)

	q2 := b.Questions[1]
	require.Equal(t, "q2", q2.QuestionID)
	require.Equal(t, "", q2.Title)
	require.Equal(t, "", q2.Prompt)
	require.NotNil(t, q2.Options)
	require.Empty(t, q2.Options)
}

func TestHandleListPendingQuestions_ListError_InternalError(t *testing.T) {
	svc, _ := newTestTaskService(t)
	lister := &recordingBundleLister{listErr: assertError("boom")}
	h := &Handlers{taskSvc: svc, clarificationBundles: lister, logger: testLogger(t)}

	msg := makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{})
	resp, err := h.handleListPendingQuestions(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
}

func TestHandleListPendingQuestions_InvalidPayload_BadRequest(t *testing.T) {
	svc, _ := newTestTaskService(t)
	h := &Handlers{taskSvc: svc, clarificationBundles: &recordingBundleLister{}, logger: testLogger(t)}
	msg := &ws.Message{ID: "x", Action: ws.ActionMCPListPendingQuestions, Payload: json.RawMessage(`{"limit": "not-a-number"}`)}
	resp, err := h.handleListPendingQuestions(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

// --- Registration gating (S1: external-surface tools only when wired) ---

func TestRegisterHandlers_ClarificationQuestionToolsOnlyWhenBothDepsSet(t *testing.T) {
	svc, repo := newTestTaskService(t)
	store := clarification.NewStore(time.Minute)

	cases := []struct {
		name     string
		resolver *clarification.Resolver
		bundles  ClarificationBundleLister
		want     bool
	}{
		{"neither set", nil, nil, false},
		{"only resolver set", newTestResolver(t, store, repo, svc), nil, false},
		{"only lister set", nil, repo, false},
		{"both set", newTestResolver(t, store, repo, svc), repo, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandlers(svc, nil, store, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
			h.SetClarificationResolver(tc.resolver, tc.bundles)
			d := ws.NewDispatcher()
			h.RegisterHandlers(d)
			require.Equal(t, tc.want, d.HasHandler(ws.ActionMCPListPendingQuestions))
			require.Equal(t, tc.want, d.HasHandler(ws.ActionMCPAnswerQuestion))
		})
	}
}

// --- answer_question_kandev: delegation to ResolveBundle ---

// svcMessageUpdater adapts service.Service to clarification.MessageCreator for
// tests, mirroring backendapp's messageCreatorAdapter. It embeds *service.Service
// to inherit CompleteActiveClarificationBundle, FinalizeClarificationResponseDelivery,
// RestoreActiveClarificationBundle, and PublishClarificationBundleUpdates directly,
// and only overrides UpdateClarificationMessage (name/signature mismatch with
// UpdateClarificationMessageForQuestion, including its COR-001 nil-forwarding fix:
// a nil *clarification.Answer must reach the service as an untyped nil, not boxed
// into its interface{} parameter) and CreateClarificationRequestMessages (unused
// by these tests, which seed bundles directly via repo).
type svcMessageUpdater struct{ *service.Service }

func (s *svcMessageUpdater) UpdateClarificationMessage(ctx context.Context, sessionID, pendingID, questionID, status string, answer *clarification.Answer) error {
	if answer == nil {
		return s.UpdateClarificationMessageForQuestion(ctx, sessionID, pendingID, questionID, status, nil)
	}
	return s.UpdateClarificationMessageForQuestion(ctx, sessionID, pendingID, questionID, status, answer)
}

func (s *svcMessageUpdater) CreateClarificationRequestMessages(context.Context, string, string, string, []clarification.Question, string) ([]string, error) {
	return nil, errors.New("svcMessageUpdater: CreateClarificationRequestMessages not implemented for tests")
}

// stubDetachedResumer is a no-op EventBus + DetachedClarificationResumer test
// double, mirroring clarification package's own stubEventBus (used by
// handlers_authz_test.go). Without it, ResolveBundle's detached-fallback path
// (exercised whenever these tests answer a bundle with no live in-session
// waiter registered) fails closed with "detached clarification resumer
// unavailable", since production wires that interface to
// orchestrator.Service.ResumeDetachedClarification, not available here.
type stubDetachedResumer struct{}

func (stubDetachedResumer) Publish(context.Context, string, *bus.Event) error { return nil }

func (stubDetachedResumer) ResumeDetachedClarification(context.Context, clarification.DetachedClarificationResume) error {
	return nil
}

func newTestResolver(t *testing.T, store *clarification.Store, repo *sqliterepo.Repository, svc *service.Service) *clarification.Resolver {
	t.Helper()
	resumer := stubDetachedResumer{}
	return clarification.NewResolver(store, repo, &svcMessageUpdater{Service: svc}, svc, resumer, resumer, nil, testLogger(t))
}

// seedBundle creates a task/session/turn and a two-question clarification
// bundle's durable messages directly via repo, mirroring the fixture shape
// CreateClarificationRequestMessages produces.
func seedBundle(t *testing.T, ctx context.Context, svc *service.Service, repo *sqliterepo.Repository, pendingID string) (taskID, sessionID string) {
	t.Helper()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-" + pendingID, Name: "WS"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-" + pendingID, WorkspaceID: "ws-" + pendingID, Name: "Board"}))
	taskResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-" + pendingID,
		WorkflowID:  "wf-" + pendingID,
		Title:       "Task",
	})
	require.NoError(t, err)
	task := taskResult.Task

	sess := &models.TaskSession{ID: "sess-" + pendingID, TaskID: task.ID, IsPrimary: true, State: models.TaskSessionStateRunning}
	require.NoError(t, repo.CreateTaskSession(ctx, sess))
	turn := &models.Turn{ID: "turn-" + pendingID, TaskSessionID: sess.ID, TaskID: task.ID}
	require.NoError(t, repo.CreateTurn(ctx, turn))

	questions := []struct {
		id      string
		options []map[string]interface{}
	}{
		{"q1", []map[string]interface{}{{"option_id": "opt-a", "label": "A", "description": "A opt"}, {"option_id": "opt-b", "label": "B", "description": "B opt"}}},
		{"q2", []map[string]interface{}{{"option_id": "opt-c", "label": "C", "description": "C opt"}, {"option_id": "opt-d", "label": "D", "description": "D opt"}}},
	}
	for i, q := range questions {
		opts := make([]interface{}, len(q.options))
		for j, o := range q.options {
			opts[j] = o
		}
		require.NoError(t, repo.CreateMessage(ctx, &models.Message{
			TaskSessionID: sess.ID,
			TaskID:        task.ID,
			TurnID:        turn.ID,
			AuthorType:    "agent",
			Type:          "clarification_request",
			Content:       "Q?",
			Metadata: map[string]interface{}{
				"pending_id":     pendingID,
				"question_id":    q.id,
				"question_index": i,
				"status":         "pending",
				"context":        "why we ask",
				"question": map[string]interface{}{
					"id": q.id, "title": "T", "prompt": "P?", "options": opts,
				},
			},
		}))
	}
	return task.ID, sess.ID
}

func TestHandleAnswerQuestion_Answers_ClaimsAndReturnsNormalizedResponse(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	seedBundle(t, ctx, svc, repo, "pending-answer-1")

	store := clarification.NewStore(time.Minute)
	resolver := newTestResolver(t, store, repo, svc)
	h := &Handlers{taskSvc: svc, clarificationResolver: resolver, clarificationBundles: repo, logger: testLogger(t)}

	msg := makeWSMessage(t, ws.ActionMCPAnswerQuestion, map[string]interface{}{
		"pending_id": "pending-answer-1",
		"answers": []map[string]interface{}{
			{"question_id": "q2", "selected_options": []string{"opt-c"}},
			{"question_id": "q1", "selected_options": []string{"opt-b", "opt-a"}, "custom_text": "  trimmed  "},
		},
	})
	resp, err := h.handleAnswerQuestion(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var body answerQuestionResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	require.True(t, body.Claimed)
	require.Equal(t, string(clarification.StatusAnswered), body.Status)

	var respPayload map[string]interface{}
	require.NoError(t, json.Unmarshal(body.Response, &respPayload))
	answers, ok := respPayload["answers"].([]interface{})
	require.True(t, ok)
	require.Len(t, answers, 2)
	// N3a rule 1: ordered by the bundle's own question order (q1 then q2),
	// not the caller's submission order (q2 then q1 above).
	first := answers[0].(map[string]interface{})
	require.Equal(t, "q1", first["question_id"])
	// N3a rule 2: selected_options ordered by option position (opt-a before opt-b).
	require.Equal(t, []interface{}{"opt-a", "opt-b"}, first["selected_options"])
	require.Equal(t, "trimmed", first["custom_text"]) // rule 3: trimmed
}

// TestHandleAnswerQuestion_M6_StrayRejectReasonDiscardedOnAnsweredPath proves
// M6 end-to-end through the handler: a full answers submission
// (rejected:false) that also carries a reject_reason must have that stray
// field discarded from the stored/returned response, not merely rejected by
// validation.
func TestHandleAnswerQuestion_M6_StrayRejectReasonDiscardedOnAnsweredPath(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	seedBundle(t, ctx, svc, repo, "pending-m6-1")

	store := clarification.NewStore(time.Minute)
	resolver := newTestResolver(t, store, repo, svc)
	h := &Handlers{taskSvc: svc, clarificationResolver: resolver, clarificationBundles: repo, logger: testLogger(t)}

	resp, err := h.handleAnswerQuestion(ctx, makeWSMessage(t, ws.ActionMCPAnswerQuestion, map[string]interface{}{
		"pending_id": "pending-m6-1",
		"answers": []map[string]interface{}{
			{"question_id": "q1", "selected_options": []string{"opt-a"}},
			{"question_id": "q2", "selected_options": []string{"opt-c"}},
		},
		"reject_reason": "should never be stored",
	}))
	require.NoError(t, err)

	var body answerQuestionResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	require.True(t, body.Claimed)
	require.Equal(t, string(clarification.StatusAnswered), body.Status)

	var respPayload map[string]interface{}
	require.NoError(t, json.Unmarshal(body.Response, &respPayload))
	require.Equal(t, "", respPayload["reject_reason"])

	// Durable storage never persists reject_reason at all (R2b: reconstruction
	// always returns ""), so the in-memory response check above is the whole
	// contract; there is no separate resolution row to cross-check.
	msgs, err := repo.FindMessagesByPendingID(ctx, "pending-m6-1")
	require.NoError(t, err)
	require.NotEmpty(t, msgs)
	for _, m := range msgs {
		response, ok := m.Metadata["response"].(map[string]interface{})
		require.True(t, ok)
		_, hasRejectReason := response["reject_reason"]
		require.False(t, hasRejectReason)
	}
}

// TestHandleAnswerQuestion_N8b_CustomTextAtCapAccepted proves N8b's positive
// boundary end-to-end through the MCP handler: an answer's custom_text of
// exactly 2000 runes (clarification.answerTextRuneCap, unexported), counted
// over code points with a multi-byte fixture, is accepted rather than
// rejected as over the cap.
func TestHandleAnswerQuestion_N8b_CustomTextAtCapAccepted(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	seedBundle(t, ctx, svc, repo, "pending-n8b-1")

	store := clarification.NewStore(time.Minute)
	resolver := newTestResolver(t, store, repo, svc)
	h := &Handlers{taskSvc: svc, clarificationResolver: resolver, clarificationBundles: repo, logger: testLogger(t)}

	exact := make([]rune, 2000)
	for i := range exact {
		exact[i] = '本' // multi-byte rune: proves the cap counts runes, not bytes
	}

	resp, err := h.handleAnswerQuestion(ctx, makeWSMessage(t, ws.ActionMCPAnswerQuestion, map[string]interface{}{
		"pending_id": "pending-n8b-1",
		"answers": []map[string]interface{}{
			{"question_id": "q1", "selected_options": []string{"opt-a"}, "custom_text": string(exact)},
			{"question_id": "q2", "selected_options": []string{"opt-c"}},
		},
	}))
	require.NoError(t, err)

	var body answerQuestionResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	require.True(t, body.Claimed)
	require.Equal(t, string(clarification.StatusAnswered), body.Status)

	var respPayload map[string]interface{}
	require.NoError(t, json.Unmarshal(body.Response, &respPayload))
	answers, ok := respPayload["answers"].([]interface{})
	require.True(t, ok)
	first := answers[0].(map[string]interface{})
	require.Equal(t, string(exact), first["custom_text"])
}

func TestHandleAnswerQuestion_SecondCaller_ClaimedFalseSameOutcome(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	seedBundle(t, ctx, svc, repo, "pending-answer-2")

	store := clarification.NewStore(time.Minute)
	resolver := newTestResolver(t, store, repo, svc)
	h := &Handlers{taskSvc: svc, clarificationResolver: resolver, clarificationBundles: repo, logger: testLogger(t)}

	answerPayload := map[string]interface{}{
		"pending_id": "pending-answer-2",
		"answers": []map[string]interface{}{
			{"question_id": "q1", "selected_options": []string{"opt-a"}},
			{"question_id": "q2", "selected_options": []string{"opt-c"}},
		},
	}
	first, err := h.handleAnswerQuestion(ctx, makeWSMessage(t, ws.ActionMCPAnswerQuestion, answerPayload))
	require.NoError(t, err)
	var firstBody answerQuestionResponse
	require.NoError(t, json.Unmarshal(first.Payload, &firstBody))
	require.True(t, firstBody.Claimed)

	second, err := h.handleAnswerQuestion(ctx, makeWSMessage(t, ws.ActionMCPAnswerQuestion, map[string]interface{}{
		"pending_id": "pending-answer-2",
		"rejected":   true,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, second.Type)
	var secondBody answerQuestionResponse
	require.NoError(t, json.Unmarshal(second.Payload, &secondBody))
	require.False(t, secondBody.Claimed)
	require.Equal(t, firstBody.Status, secondBody.Status)
	require.JSONEq(t, string(firstBody.Response), string(secondBody.Response))
}

// TestHandleAnswerQuestion_NoWinner_ConflictError proves N4a: R2's second
// branch (the claim was lost and no message was ever answered/rejected —
// here because a later Turn on the same session supersedes the bundle's
// turn, mirroring TestCompleteActiveClarificationBundleRejectsSupersededTurn
// in the sqlite repository package) must be reported as a CONFLICT error,
// distinct from both success and not-found — not the generic INTERNAL_ERROR
// the default case falls through to for every other unclassified error.
func TestHandleAnswerQuestion_NoWinner_ConflictError(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	taskID, sessionID := seedBundle(t, ctx, svc, repo, "pending-nowinner-1")

	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID:            "turn-pending-nowinner-1-newer",
		TaskSessionID: sessionID,
		TaskID:        taskID,
	}))

	store := clarification.NewStore(time.Minute)
	resolver := newTestResolver(t, store, repo, svc)
	h := &Handlers{taskSvc: svc, clarificationResolver: resolver, clarificationBundles: repo, logger: testLogger(t)}

	resp, err := h.handleAnswerQuestion(ctx, makeWSMessage(t, ws.ActionMCPAnswerQuestion, map[string]interface{}{
		"pending_id": "pending-nowinner-1",
		"answers": []map[string]interface{}{
			{"question_id": "q1", "selected_options": []string{"opt-a"}},
			{"question_id": "q2", "selected_options": []string{"opt-c"}},
		},
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeConflict)

	var ep ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &ep))
	require.Equal(t, "clarification request is no longer active", ep.Message)
}

func TestHandleAnswerQuestion_Rejected(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	seedBundle(t, ctx, svc, repo, "pending-reject-1")

	store := clarification.NewStore(time.Minute)
	resolver := newTestResolver(t, store, repo, svc)
	h := &Handlers{taskSvc: svc, clarificationResolver: resolver, clarificationBundles: repo, logger: testLogger(t)}

	resp, err := h.handleAnswerQuestion(ctx, makeWSMessage(t, ws.ActionMCPAnswerQuestion, map[string]interface{}{
		"pending_id":    "pending-reject-1",
		"rejected":      true,
		"reject_reason": "not needed",
	}))
	require.NoError(t, err)
	var body answerQuestionResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	require.True(t, body.Claimed)
	require.Equal(t, string(clarification.StatusRejected), body.Status)

	// COR-001 regression: resolver.go's applyMessageUpdates leaves a nil
	// *clarification.Answer for a rejected outcome, forwarded unchanged
	// through svcMessageUpdater (which mirrors backendapp's production
	// messageCreatorAdapter one-for-one) into the real, unmodified
	// Service.UpdateClarificationMessageForQuestion. A nil *clarification.Answer
	// boxed into that method's interface{} parameter must not become a
	// non-nil interface value that gets written into metadata as
	// "response": null.
	msgs, err := repo.FindMessagesByPendingID(ctx, "pending-reject-1")
	require.NoError(t, err)
	require.NotEmpty(t, msgs)
	for _, msg := range msgs {
		_, hasResponse := msg.Metadata["response"]
		require.Falsef(t, hasResponse, "message %s metadata got a response key from a rejected answer: %v", msg.ID, msg.Metadata["response"])
	}
}

// TestHandleAnswerQuestion_SecondCaller_RejectedWinner_ClaimedFalseSameOutcome
// covers R2b's rejected branch of reconstructWinnerResolution, which was
// otherwise untested: TestHandleAnswerQuestion_Rejected only exercises the
// winner's own path, and TestHandleAnswerQuestion_SecondCaller_ClaimedFalseSameOutcome
// only exercises replay of an answered winner. When the winner rejected the
// bundle, a loser arriving after the claim was already lost -- even one that
// submitted real answers of its own -- must be handed the winner's
// reconstructed rejected outcome (Rejected: true, no answers), not its own
// submission. Per reconstructWinnerResolution's own doc comment, reject_reason
// does not survive replay (upstream stores no reject reason on the message
// rows), so the loser's reconstructed reason is always "" even though the
// winner's own claim response carried the real one -- this asymmetry is part
// of R2b's contract, not a bug, so it is asserted explicitly rather than via
// a blanket JSONEq of the two responses.
func TestHandleAnswerQuestion_SecondCaller_RejectedWinner_ClaimedFalseSameOutcome(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	seedBundle(t, ctx, svc, repo, "pending-reject-2")

	store := clarification.NewStore(time.Minute)
	resolver := newTestResolver(t, store, repo, svc)
	h := &Handlers{taskSvc: svc, clarificationResolver: resolver, clarificationBundles: repo, logger: testLogger(t)}

	first, err := h.handleAnswerQuestion(ctx, makeWSMessage(t, ws.ActionMCPAnswerQuestion, map[string]interface{}{
		"pending_id":    "pending-reject-2",
		"rejected":      true,
		"reject_reason": "not needed",
	}))
	require.NoError(t, err)
	var firstBody answerQuestionResponse
	require.NoError(t, json.Unmarshal(first.Payload, &firstBody))
	require.True(t, firstBody.Claimed)
	require.Equal(t, string(clarification.StatusRejected), firstBody.Status)

	second, err := h.handleAnswerQuestion(ctx, makeWSMessage(t, ws.ActionMCPAnswerQuestion, map[string]interface{}{
		"pending_id": "pending-reject-2",
		"answers": []map[string]interface{}{
			{"question_id": "q1", "selected_options": []string{"opt-a"}},
			{"question_id": "q2", "selected_options": []string{"opt-c"}},
		},
	}))
	require.NoError(t, err)
	var secondBody answerQuestionResponse
	require.NoError(t, json.Unmarshal(second.Payload, &secondBody))
	require.False(t, secondBody.Claimed)
	require.Equal(t, firstBody.Status, secondBody.Status)

	var parsed struct {
		Rejected     bool             `json:"rejected"`
		RejectReason string           `json:"reject_reason"`
		Answers      []map[string]any `json:"answers"`
	}
	require.NoError(t, json.Unmarshal(secondBody.Response, &parsed))
	require.True(t, parsed.Rejected)
	require.Empty(t, parsed.Answers)
	require.Empty(t, parsed.RejectReason)
}

func TestHandleAnswerQuestion_UnknownPendingID_NotFound(t *testing.T) {
	svc, repo := newTestTaskService(t)
	store := clarification.NewStore(time.Minute)
	resolver := newTestResolver(t, store, repo, svc)
	h := &Handlers{taskSvc: svc, clarificationResolver: resolver, clarificationBundles: repo, logger: testLogger(t)}

	resp, err := h.handleAnswerQuestion(context.Background(), makeWSMessage(t, ws.ActionMCPAnswerQuestion, map[string]interface{}{
		"pending_id": "does-not-exist",
		"rejected":   true,
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)

	var ep ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &ep))
	require.Equal(t, "clarification request not found", ep.Message)
}

func TestHandleAnswerQuestion_MissingPendingID_ValidationError(t *testing.T) {
	h := &Handlers{logger: testLogger(t)}
	resp, err := h.handleAnswerQuestion(context.Background(), makeWSMessage(t, ws.ActionMCPAnswerQuestion, map[string]interface{}{
		"rejected": true,
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleAnswerQuestion_UnknownOptionID_ValidationError(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	seedBundle(t, ctx, svc, repo, "pending-badopt-1")

	store := clarification.NewStore(time.Minute)
	resolver := newTestResolver(t, store, repo, svc)
	h := &Handlers{taskSvc: svc, clarificationResolver: resolver, clarificationBundles: repo, logger: testLogger(t)}

	resp, err := h.handleAnswerQuestion(ctx, makeWSMessage(t, ws.ActionMCPAnswerQuestion, map[string]interface{}{
		"pending_id": "pending-badopt-1",
		"answers": []map[string]interface{}{
			{"question_id": "q1", "selected_options": []string{"opt-fabricated"}},
			{"question_id": "q2", "selected_options": []string{"opt-c"}},
		},
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleAnswerQuestion_InvalidPayload_BadRequest(t *testing.T) {
	h := &Handlers{logger: testLogger(t)}
	msg := &ws.Message{ID: "x", Action: ws.ActionMCPAnswerQuestion, Payload: json.RawMessage(`not-json`)}
	resp, err := h.handleAnswerQuestion(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

// --- End-to-end: an answered bundle drops out of list_pending_questions_kandev ---

func TestHandleListPendingQuestions_ExcludesResolvedBundle_EndToEnd(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	seedBundle(t, ctx, svc, repo, "pending-e2e-1")

	store := clarification.NewStore(time.Minute)
	resolver := newTestResolver(t, store, repo, svc)
	h := &Handlers{taskSvc: svc, clarificationResolver: resolver, clarificationBundles: repo, logger: testLogger(t)}

	listBefore, err := h.handleListPendingQuestions(ctx, makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{}))
	require.NoError(t, err)
	var bodyBefore listPendingQuestionsResponse
	require.NoError(t, json.Unmarshal(listBefore.Payload, &bodyBefore))
	require.Equal(t, 1, bodyBefore.Count)
	require.Equal(t, "pending-e2e-1", bodyBefore.Bundles[0].PendingID)

	_, err = h.handleAnswerQuestion(ctx, makeWSMessage(t, ws.ActionMCPAnswerQuestion, map[string]interface{}{
		"pending_id": "pending-e2e-1",
		"rejected":   true,
	}))
	require.NoError(t, err)

	listAfter, err := h.handleListPendingQuestions(ctx, makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{}))
	require.NoError(t, err)
	var bodyAfter listPendingQuestionsResponse
	require.NoError(t, json.Unmarshal(listAfter.Payload, &bodyAfter))
	require.Equal(t, 0, bodyAfter.Count)
}

type assertError string

func (e assertError) Error() string { return string(e) }
