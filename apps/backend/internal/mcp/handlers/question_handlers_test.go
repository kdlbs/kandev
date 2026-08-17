package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/clarification"
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
	msgs    map[string][]*models.Message
	listErr error
}

func (r *recordingBundleLister) ListUnresolvedClarificationBundles(_ context.Context, opts models.ListClarificationBundlesOptions) (*models.ClarificationBundlePage, error) {
	r.gotOpts = opts
	if r.listErr != nil {
		return nil, r.listErr
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

// svcMessageUpdater adapts service.Service.UpdateClarificationMessageForQuestion
// to clarificationMessageUpdater (unexported in package clarification),
// mirroring backendapp's messageCreatorAdapter for tests.
type svcMessageUpdater struct{ svc *service.Service }

func (s *svcMessageUpdater) UpdateClarificationMessage(ctx context.Context, sessionID, pendingID, questionID, status string, answer *clarification.Answer) error {
	return s.svc.UpdateClarificationMessageForQuestion(ctx, sessionID, pendingID, questionID, status, answer)
}

func newTestResolver(t *testing.T, store *clarification.Store, repo *sqliterepo.Repository, svc *service.Service) *clarification.Resolver {
	t.Helper()
	return clarification.NewResolver(store, repo, repo, &svcMessageUpdater{svc: svc}, svc, nil, testLogger(t))
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
	require.Equal(t, models.ClarificationResolutionStatusAnswered, body.Status)
	require.NotEmpty(t, body.Resume)

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
	require.Equal(t, models.ClarificationResolutionStatusRejected, body.Status)
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
