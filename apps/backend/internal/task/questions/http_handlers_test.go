package questions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestAsyncQuestionsHTTPRejectsForeignWorkspace(t *testing.T) {
	svc, repo := newQuestionsService(t)
	result := createOperatorQuestion(t, svc, repo, "session-http-authz")
	router := newQuestionsRouter(t, svc, allowAllWorkspaceAuthorizer{denied: true})

	response := doRequest(router, http.MethodGet, "/api/v1/agent-questions?workspace_id=workspace-session-http", nil)
	require.Equal(t, http.StatusForbidden, response.Code)

	response = doRequest(router, http.MethodPost,
		"/api/v1/agent-questions/"+result.Question.ID+"/respond",
		map[string]any{"rejected": true})
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

func TestAsyncQuestionsHTTPWithdrawRouteIsAuthorOnly(t *testing.T) {
	svc, repo := newQuestionsService(t)
	result := createOperatorQuestion(t, svc, repo, "session-http-withdraw")
	router := newQuestionsRouter(t, svc, allowAllWorkspaceAuthorizer{})

	response := doRequest(router, http.MethodPost,
		"/api/v1/agent-questions/"+result.Question.ID+"/withdraw",
		map[string]any{"reason": "no longer needed"})
	require.Equal(t, http.StatusForbidden, response.Code)
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
