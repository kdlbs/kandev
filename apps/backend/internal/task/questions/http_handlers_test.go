package questions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"errors"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/common/imagepreview"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/questions"
	"go.uber.org/zap"
)

type allowAllWorkspaceAuthorizer struct{ denied bool }

func (a allowAllWorkspaceAuthorizer) AuthorizeWorkspaceScope(context.Context, string, authz.Scope) error {
	if a.denied {
		return errors.New("forbidden")
	}
	return nil
}

type workspaceReadOnlyAuthorizer struct{}

func (workspaceReadOnlyAuthorizer) AuthorizeWorkspaceScope(_ context.Context, _ string, scope authz.Scope) error {
	if scope == authz.ScopeTaskWrite {
		return errors.New("task write is forbidden")
	}
	return nil
}

type emptyQuestionDirectory struct{}

func (emptyQuestionDirectory) GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error) {
	return nil, nil
}

func (emptyQuestionDirectory) GetTasksByIDs(context.Context, []string) ([]*taskmodels.Task, error) {
	return nil, nil
}

func (emptyQuestionDirectory) BatchGetSessionsForTasks(context.Context, []string) (map[string][]*taskmodels.TaskSession, error) {
	return nil, nil
}

func newQuestionsRouter(t *testing.T, svc *questions.Service, authorizer questions.WorkspaceAuthorizer) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	questions.RegisterRoutes(router, svc, authorizer, emptyQuestionDirectory{}, zap.NewNop())
	return router
}

func doRequest(router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAsyncQuestionsInboxMixedPage(t *testing.T) {
	svc, repo := newQuestionsService(t)
	result := createOperatorQuestion(t, svc, repo, "session-http")
	router := newQuestionsRouter(t, svc, allowAllWorkspaceAuthorizer{})

	response := doRequest(router, http.MethodGet, "/api/v1/agent-questions?workspace_id=workspace-session-http", nil)
	require.Equal(t, http.StatusOK, response.Code)
	var page struct {
		Questions []struct {
			QuestionID string `json:"question_id"`
			Status     string `json:"status"`
			Waiting    bool   `json:"waiting"`
		} `json:"questions"`
		Count int `json:"count"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
	require.Equal(t, 1, page.Count)
	require.Equal(t, result.Question.ID, page.Questions[0].QuestionID)
	require.Equal(t, "pending", page.Questions[0].Status)
	require.False(t, page.Questions[0].Waiting)
}

func TestAsyncQuestionsInboxWaitingIsPerDependency(t *testing.T) {
	svc, repo := newQuestionsService(t)
	first := createOperatorQuestion(t, svc, repo, "session-http-waiting")
	second, err := svc.Create(context.Background(), questions.CreateInput{
		TaskID: first.Question.TaskID, WorkspaceID: first.Question.WorkspaceID,
		SessionID: first.Question.SessionID, SessionIncarnationID: first.Question.SessionIncarnationID,
		Recipient: taskmodels.AsyncQuestionRecipientUser,
		Questions: []protocol.Question{{
			Prompt:  "Which value belongs in the other field?",
			Options: []protocol.Option{{ID: "default", Label: "Default"}, {ID: "empty", Label: "Empty"}},
		}},
		RequestID: "second-http-waiting-question",
	})
	require.NoError(t, err)
	_, err = repo.CreateAsyncQuestionWait(context.Background(), &taskmodels.AsyncQuestionWait{
		WorkspaceID: first.Question.WorkspaceID, TaskID: first.Question.TaskID,
		SessionID: first.Question.SessionID, DependencyIDs: []string{first.Question.ID},
		Status: taskmodels.AsyncQuestionWaitParked,
	})
	require.NoError(t, err)
	router := newQuestionsRouter(t, svc, allowAllWorkspaceAuthorizer{})
	response := doRequest(router, http.MethodGet,
		"/api/v1/agent-questions?workspace_id="+first.Question.WorkspaceID, nil)
	require.Equal(t, http.StatusOK, response.Code)
	var page struct {
		Questions []struct {
			QuestionID string `json:"question_id"`
			Waiting    bool   `json:"waiting"`
		} `json:"questions"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
	waitingByID := make(map[string]bool, len(page.Questions))
	for _, question := range page.Questions {
		waitingByID[question.QuestionID] = question.Waiting
	}
	require.True(t, waitingByID[first.Question.ID])
	require.False(t, waitingByID[second.Question.ID])
}

func TestAsyncQuestionsHTTPGetByIDReturnsCurrentState(t *testing.T) {
	svc, repo := newQuestionsService(t)
	created := createOperatorQuestion(t, svc, repo, "session-http-detail")
	router := newQuestionsRouter(t, svc, allowAllWorkspaceAuthorizer{})
	response := doRequest(router, http.MethodGet,
		"/api/v1/agent-questions/"+created.Question.ID, nil)
	require.Equal(t, http.StatusOK, response.Code)
	var view struct {
		QuestionID string `json:"question_id"`
		Status     string `json:"status"`
		Questions  []struct {
			Prompt string `json:"prompt"`
		} `json:"questions"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view))
	require.Equal(t, created.Question.ID, view.QuestionID)
	require.Equal(t, "pending", view.Status)
	require.Equal(t, "Which value belongs in the field?", view.Questions[0].Prompt)
}

func TestAsyncQuestionsInboxBootHiddenCount(t *testing.T) {
	svc, repo := newQuestionsService(t)
	createOperatorQuestion(t, svc, repo, "session-http-boot")
	router := newQuestionsRouter(t, svc, allowAllWorkspaceAuthorizer{})

	count, err := svc.CountInbox(context.Background(), "workspace-session-http-boot")
	require.NoError(t, err)
	require.Equal(t, 1, count)

	response := doRequest(router, http.MethodGet, "/api/v1/agent-questions?workspace_id=workspace-session-http-boot&limit=0", nil)
	require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestAsyncQuestionsHTTPTaskScopeIncludesAllSessions(t *testing.T) {
	svc, repo := newQuestionsService(t)
	first := createOperatorQuestion(t, svc, repo, "session-task-scope-first")
	secondSessionID := "session-task-scope-second"
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(context.Background(), &taskmodels.TaskSession{
		ID: secondSessionID, TaskID: first.Question.TaskID, IsPrimary: false,
		State: taskmodels.TaskSessionStateRunning, QueueIncarnationID: "inc-" + secondSessionID,
		StartedAt: now, UpdatedAt: now,
	}))
	second, err := svc.Create(context.Background(), questions.CreateInput{
		TaskID: first.Question.TaskID, WorkspaceID: first.Question.WorkspaceID, SessionID: secondSessionID,
		SessionIncarnationID: "inc-" + secondSessionID, Recipient: taskmodels.AsyncQuestionRecipientUser,
		Questions: []protocol.Question{{
			Prompt:  "Which value belongs in the second field?",
			Options: []protocol.Option{{ID: "default", Label: "Default"}, {ID: "empty", Label: "Empty"}},
		}},
		RequestID: "task-scope-second-request",
	})
	require.NoError(t, err)
	require.True(t, second.Created)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	directory := staticQuestionDirectory{
		task: &taskmodels.Task{
			ID: first.Question.TaskID, WorkspaceID: first.Question.WorkspaceID, Title: "Task scope",
		},
	}
	questions.RegisterRoutes(router, svc, allowAllWorkspaceAuthorizer{}, directory, zap.NewNop())
	response := doRequest(router, http.MethodGet,
		"/api/v1/agent-questions?workspace_id="+first.Question.WorkspaceID+"&task_id="+first.Question.TaskID, nil)
	require.Equal(t, http.StatusOK, response.Code)
	var page struct {
		Questions []struct {
			QuestionID string `json:"question_id"`
		} `json:"questions"`
		Count int `json:"count"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
	require.Equal(t, 2, page.Count)
	require.ElementsMatch(t, []string{first.Question.ID, second.Question.ID}, []string{
		page.Questions[0].QuestionID, page.Questions[1].QuestionID,
	})
}

func TestAsyncQuestionsRespondAndConcurrentWinner(t *testing.T) {
	svc, repo := newQuestionsService(t)
	result := createOperatorQuestion(t, svc, repo, "session-http-respond")
	router := newQuestionsRouter(t, svc, allowAllWorkspaceAuthorizer{})

	body := map[string]any{
		"answers": []map[string]any{{
			"question_id":      result.Question.Questions[0].ID,
			"selected_options": []string{"default"},
			"custom_text":      "use the documented default",
		}},
		"submission_id": "submission-1",
	}
	response := doRequest(router, http.MethodPost,
		"/api/v1/agent-questions/"+result.Question.ID+"/respond", body)
	require.Equal(t, http.StatusOK, response.Code)
	var first struct {
		Won       bool   `json:"won"`
		Status    string `json:"status"`
		Questions []struct {
			Outcome struct {
				Answers []struct {
					SelectedOptions []string `json:"selected_options"`
				} `json:"answers"`
			} `json:"outcome"`
		} `json:"questions"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &first))
	require.True(t, first.Won)
	require.Equal(t, "answered", first.Status)
	require.Equal(t, []string{"default"}, first.Questions[0].Outcome.Answers[0].SelectedOptions)

	// A second caller observes the winner without another delivery.
	body["answers"] = []map[string]any{{
		"question_id":      result.Question.Questions[0].ID,
		"selected_options": []string{"empty"},
	}}
	body["submission_id"] = "submission-2"
	response = doRequest(router, http.MethodPost,
		"/api/v1/agent-questions/"+result.Question.ID+"/respond", body)
	require.Equal(t, http.StatusOK, response.Code)
	var second struct {
		Won    bool   `json:"won"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &second))
	require.False(t, second.Won)
	require.Equal(t, "answered", second.Status)
}

func TestAsyncQuestionsRespondRetryKeepsWin(t *testing.T) {
	svc, repo := newQuestionsService(t)
	created := createOperatorQuestion(t, svc, repo, "session-http-retry")
	router := newQuestionsRouter(t, svc, allowAllWorkspaceAuthorizer{})
	path := "/api/v1/agent-questions/" + created.Question.ID + "/respond"
	body := map[string]any{
		"answers": []map[string]any{{
			"question_id":      created.Question.Questions[0].ID,
			"selected_options": []string{"default"},
		}},
		"submission_id": "submission-retry-1",
	}
	post := func() bool {
		response := doRequest(router, http.MethodPost, path, body)
		require.Equal(t, http.StatusOK, response.Code)
		var result struct {
			Won bool `json:"won"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		return result.Won
	}

	require.True(t, post())
	require.True(t, post(), "an exact submission retry must report its original win")
	body["submission_id"] = "submission-retry-2"
	require.False(t, post(), "a different submission must observe the prior winner")

	deliveries, err := repo.ListAsyncQuestionDeliveries(context.Background(), time.Now().UTC().Add(time.Minute), 20)
	require.NoError(t, err)
	ownerDeliveries := 0
	for _, delivery := range deliveries {
		if delivery.QuestionID == created.Question.ID && delivery.Kind == taskmodels.AsyncQuestionDeliveryOwner {
			ownerDeliveries++
		}
	}
	require.Equal(t, 1, ownerDeliveries)
}

func TestAsyncQuestionsHTTPRejectsForeignWorkspace(t *testing.T) {
	svc, repo := newQuestionsService(t)
	result := createOperatorQuestion(t, svc, repo, "session-http-authz")
	router := newQuestionsRouter(t, svc, allowAllWorkspaceAuthorizer{denied: true})

	response := doRequest(router, http.MethodGet, "/api/v1/agent-questions?workspace_id=workspace-session-http", nil)
	require.Equal(t, http.StatusForbidden, response.Code)

	response = doRequest(router, http.MethodPost,
		"/api/v1/agent-questions/"+result.Question.ID+"/respond",
		map[string]any{"rejected": true})
	require.Equal(t, http.StatusNotFound, response.Code)
}

func TestAsyncQuestionsRespondHidesForeignRecord(t *testing.T) {
	svc, repo := newQuestionsService(t)
	created := createOperatorQuestion(t, svc, repo, "session-http-hidden")
	router := newQuestionsRouter(t, svc, allowAllWorkspaceAuthorizer{denied: true})
	body := map[string]any{"rejected": true}
	foreign := doRequest(router, http.MethodPost,
		"/api/v1/agent-questions/"+created.Question.ID+"/respond", body)
	missing := doRequest(router, http.MethodPost,
		"/api/v1/agent-questions/missing-question/respond", body)
	require.Equal(t, http.StatusNotFound, foreign.Code)
	require.Equal(t, http.StatusNotFound, missing.Code)
	require.Equal(t, missing.Body.String(), foreign.Body.String())
}

func TestAsyncQuestionsRespondKeepsWriteScopeForWorkspaceReaders(t *testing.T) {
	svc, repo := newQuestionsService(t)
	created := createOperatorQuestion(t, svc, repo, "session-http-read-only")
	router := newQuestionsRouter(t, svc, workspaceReadOnlyAuthorizer{})
	response := doRequest(router, http.MethodPost,
		"/api/v1/agent-questions/"+created.Question.ID+"/respond", map[string]any{"rejected": true})
	require.Equal(t, http.StatusForbidden, response.Code)
}

func TestAsyncQuestionsHTTPRejectsMalformedAnswers(t *testing.T) {
	svc, repo := newQuestionsService(t)
	result := createOperatorQuestion(t, svc, repo, "session-http-malformed")
	router := newQuestionsRouter(t, svc, allowAllWorkspaceAuthorizer{})

	response := doRequest(router, http.MethodPost,
		"/api/v1/agent-questions/"+result.Question.ID+"/respond",
		map[string]any{"answers": []map[string]any{{"question_id": "missing", "selected_options": []string{"x"}}}})
	require.Equal(t, http.StatusBadRequest, response.Code)

	response = doRequest(router, http.MethodPost,
		"/api/v1/agent-questions/does-not-exist/respond",
		map[string]any{"rejected": true})
	require.Equal(t, http.StatusNotFound, response.Code)
}

func TestAsyncQuestionsHTTPWithdrawStubRouteIsAbsent(t *testing.T) {
	svc, repo := newQuestionsService(t)
	result := createOperatorQuestion(t, svc, repo, "session-http-withdraw")
	router := newQuestionsRouter(t, svc, allowAllWorkspaceAuthorizer{})

	response := doRequest(router, http.MethodPost,
		"/api/v1/agent-questions/"+result.Question.ID+"/withdraw",
		map[string]any{"reason": "no longer needed"})
	require.Equal(t, http.StatusNotFound, response.Code)
	question, err := svc.Get(context.Background(), result.Question.ID)
	require.NoError(t, err)
	require.Equal(t, taskmodels.AsyncQuestionStatusPending, question.Status)
}

type staticQuestionDirectory struct {
	task    *taskmodels.Task
	session *taskmodels.TaskSession
}

func (d staticQuestionDirectory) GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error) {
	return d.session, nil
}

func (d staticQuestionDirectory) GetTasksByIDs(context.Context, []string) ([]*taskmodels.Task, error) {
	return []*taskmodels.Task{d.task}, nil
}

func (d staticQuestionDirectory) BatchGetSessionsForTasks(context.Context, []string) (map[string][]*taskmodels.TaskSession, error) {
	return map[string][]*taskmodels.TaskSession{d.task.ID: {d.session}}, nil
}

func TestAsyncQuestionsHTTPRejectsCrossWorkspaceTaskRead(t *testing.T) {
	svc, repo := newQuestionsService(t)
	_ = createOperatorQuestion(t, svc, repo, "session-http-cross-workspace")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	directory := staticQuestionDirectory{
		task:    &taskmodels.Task{ID: "task-session-http-cross-workspace", WorkspaceID: "workspace-other"},
		session: &taskmodels.TaskSession{ID: "session-http-cross-workspace", TaskID: "task-session-http-cross-workspace"},
	}
	questions.RegisterRoutes(router, svc, allowAllWorkspaceAuthorizer{}, directory, zap.NewNop())

	response := doRequest(router, http.MethodGet,
		"/api/v1/agent-questions?workspace_id=workspace-1&task_id=task-session-http-cross-workspace", nil)
	require.Equal(t, http.StatusNotFound, response.Code)
}

type fakeAsyncImageReader struct {
	called bool
}

func (f *fakeAsyncImageReader) ReadQuestionImage(context.Context, string, protocol.QuestionImage) (*imagepreview.Content, error) {
	f.called = true
	return &imagepreview.Content{MIME: "image/png", Bytes: []byte("png")}, nil
}

func TestAsyncQuestionsImageRoute(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	sessionID := "session-http-image"
	workspaceID := seedQuestionBinding(t, repo, sessionID, "")
	result, err := svc.Create(ctx, questions.CreateInput{
		TaskID: "task-" + sessionID, WorkspaceID: workspaceID, SessionID: sessionID,
		SessionIncarnationID: "inc-" + sessionID,
		Recipient:            taskmodels.AsyncQuestionRecipientUser,
		Questions: []protocol.Question{{
			ID: "q1", Prompt: "Which image?", Options: []protocol.Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
			Images: protocol.QuestionImages{{ID: "img1", Path: "docs/a.png", Alt: "A diagram"}},
		}},
		RequestID: "http-image",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	reader := &fakeAsyncImageReader{}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	questions.RegisterRoutes(router, svc, allowAllWorkspaceAuthorizer{}, emptyQuestionDirectory{}, zap.NewNop(), reader)
	response := doRequest(router, http.MethodGet,
		"/api/v1/agent-questions/"+result.Question.ID+"/images/img1?question_id=q1", nil)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "image/png", response.Header().Get("Content-Type"))
	require.True(t, reader.called)

	response = doRequest(router, http.MethodGet,
		"/api/v1/agent-questions/"+result.Question.ID+"/images/missing?question_id=q1", nil)
	require.Equal(t, http.StatusNotFound, response.Code)
}

func TestAsyncQuestionsRespondFailsClosedWithoutWorkspace(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	question := &taskmodels.AsyncQuestion{
		ID: "q-empty-workspace", TaskID: "task-empty-workspace", SessionID: "session-empty-workspace",
		Recipient: taskmodels.AsyncQuestionRecipientUser,
		Questions: []protocol.Question{{ID: "q1", Prompt: "Which?", Options: []protocol.Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}}},
		RetryKey:  "empty-workspace", PayloadDigest: "digest", Status: taskmodels.AsyncQuestionStatusPending, Version: 1,
	}
	if _, _, err := repo.CreateAsyncQuestion(ctx, question, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	router := newQuestionsRouter(t, svc, allowAllWorkspaceAuthorizer{})
	response := doRequest(router, http.MethodPost,
		"/api/v1/agent-questions/q-empty-workspace/respond",
		map[string]any{"answers": []map[string]any{{"question_id": "q1", "selected_options": []string{"a"}}}})
	require.Equal(t, http.StatusNotFound, response.Code)
}
