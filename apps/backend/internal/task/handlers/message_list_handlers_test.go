package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// messageListRepo owns session "sess-b" as "user-b" and records the options
// each list/search request reaches the repository with.
type messageListRepo struct {
	foreignSessionRepo

	listErr      error
	paginatedErr error
	searchErr    error

	listCalls        int
	paginatedOptions []models.ListMessagesOptions
	searchOptions    []models.SearchMessagesOptions
	hasMore          bool
}

func (r *messageListRepo) ListMessages(context.Context, string) ([]*models.Message, error) {
	r.listCalls++
	if r.listErr != nil {
		return nil, r.listErr
	}
	return []*models.Message{
		{ID: "msg-1", TaskSessionID: "sess-b", Content: "hello world"},
		{ID: "msg-2", TaskSessionID: "sess-b", Content: "goodbye"},
	}, nil
}

func (r *messageListRepo) ListMessagesPaginated(
	_ context.Context, _ string, opts models.ListMessagesOptions,
) ([]*models.Message, bool, error) {
	r.paginatedOptions = append(r.paginatedOptions, opts)
	if r.paginatedErr != nil {
		return nil, false, r.paginatedErr
	}
	return []*models.Message{
		{ID: "msg-1", TaskSessionID: "sess-b", Content: "hello world"},
	}, r.hasMore, nil
}

func (r *messageListRepo) SearchMessages(
	_ context.Context, _ string, opts models.SearchMessagesOptions,
) ([]*models.Message, error) {
	r.searchOptions = append(r.searchOptions, opts)
	if r.searchErr != nil {
		return nil, r.searchErr
	}
	return []*models.Message{
		{ID: "msg-1", TaskSessionID: "sess-b", Content: "hello world", TurnID: "turn-1"},
	}, nil
}

func newMessageListHandlers(t *testing.T, repo *messageListRepo) *MessageHandlers {
	t.Helper()
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return &MessageHandlers{service: svc, logger: log}
}

func messageRequestAs(t *testing.T, userID, target, sessionID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	c, rec := newWorkflowRequest(t, http.MethodGet, target, "")
	if userID != "" {
		c.Request = c.Request.WithContext(authn.WithIdentity(
			c.Request.Context(), authn.Identity{UserID: userID, Role: authn.RoleMember}))
	}
	c.Params = gin.Params{{Key: "id", Value: sessionID}}
	return c, rec
}

func TestHTTPListMessagesRequiresSessionID(t *testing.T) {
	repo := &messageListRepo{}
	h := newMessageListHandlers(t, repo)
	c, rec := messageRequestAs(t, "", "/api/v1/task-sessions//messages", "")

	h.httpListMessages(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":"task session id is required"}`, rec.Body.String())
	require.Zero(t, repo.listCalls)
}

// TestHTTPListMessagesUsesTheUnpaginatedPathByDefault pins the mode switch:
// with no cursor, sort or limit the handler must take the plain list, whose
// response carries no cursor.
func TestHTTPListMessagesUsesTheUnpaginatedPathByDefault(t *testing.T) {
	repo := &messageListRepo{}
	h := newMessageListHandlers(t, repo)
	c, rec := messageRequestAs(t, "", "/api/v1/task-sessions/sess-b/messages", "sess-b")

	h.httpListMessages(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp dto.ListMessagesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 2, resp.Total)
	require.Equal(t, 1, repo.listCalls)
	require.Empty(t, repo.paginatedOptions, "the paginated query must not run")
	require.Empty(t, resp.Cursor)
	require.False(t, resp.HasMore)
}

// TestHTTPListMessagesSwitchesToPaginationOnAnyCursorParam covers each param
// that flips the mode independently — a regression that only checked `limit`
// would silently ignore before/after/sort.
func TestHTTPListMessagesSwitchesToPaginationOnAnyCursorParam(t *testing.T) {
	for name, query := range map[string]string{
		"limit":  "?limit=25",
		"before": "?before=msg-9",
		"after":  "?after=msg-1",
		"sort":   "?sort=asc",
	} {
		t.Run(name, func(t *testing.T) {
			repo := &messageListRepo{hasMore: true}
			h := newMessageListHandlers(t, repo)
			c, rec := messageRequestAs(t, "", "/api/v1/task-sessions/sess-b/messages"+query, "sess-b")

			h.httpListMessages(c)

			require.Equal(t, http.StatusOK, rec.Code)
			require.Zero(t, repo.listCalls, "the unpaginated query must not run")
			require.Len(t, repo.paginatedOptions, 1)

			var resp dto.ListMessagesResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.True(t, resp.HasMore)
			require.Equal(t, "msg-1", resp.Cursor, "the cursor is the last returned message")
		})
	}
}

func TestHTTPListMessagesRejectsConflictingAndInvalidParams(t *testing.T) {
	repo := &messageListRepo{}
	h := newMessageListHandlers(t, repo)

	for name, tc := range map[string]struct {
		query    string
		wantBody string
	}{
		"before and after together": {
			query:    "?before=msg-9&after=msg-1",
			wantBody: `{"error":"only one of before or after can be set"}`,
		},
		"unknown sort": {
			query:    "?sort=sideways",
			wantBody: `{"error":"sort must be asc or desc"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			c, rec := messageRequestAs(t, "", "/api/v1/task-sessions/sess-b/messages"+tc.query, "sess-b")

			h.httpListMessages(c)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.JSONEq(t, tc.wantBody, rec.Body.String())
		})
	}
	require.Empty(t, repo.paginatedOptions, "a rejected request must not query messages")
	require.Zero(t, repo.listCalls)
}

func TestHTTPListMessagesRejectsInvalidLimits(t *testing.T) {
	for name, query := range map[string]string{
		"empty":     "?limit=",
		"malformed": "?limit=many",
		"zero":      "?limit=0",
		"negative":  "?limit=-1",
	} {
		t.Run(name, func(t *testing.T) {
			repo := &messageListRepo{}
			h := newMessageListHandlers(t, repo)
			c, rec := messageRequestAs(t, "", "/api/v1/task-sessions/sess-b/messages"+query, "sess-b")

			h.httpListMessages(c)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.JSONEq(t, `{"error":"limit must be a positive integer"}`, rec.Body.String())
			require.Empty(t, repo.paginatedOptions)
			require.Zero(t, repo.listCalls)
		})
	}
}

func TestHTTPListMessagesRejectsEmptyAroundAndUsesAroundSortError(t *testing.T) {
	for name, query := range map[string]string{
		"empty around":   "?around=",
		"ascending sort": "?around=message&sort=asc",
	} {
		t.Run(name, func(t *testing.T) {
			repo := &messageListRepo{}
			h := newMessageListHandlers(t, repo)
			c, rec := messageRequestAs(t, "", "/api/v1/task-sessions/sess-b/messages"+query, "sess-b")

			h.httpListMessages(c)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Zero(t, repo.listCalls)
			require.Empty(t, repo.paginatedOptions)
			if name == "empty around" {
				require.JSONEq(t, `{"error":"around must not be empty"}`, rec.Body.String())
			} else {
				require.JSONEq(t, `{"error":"sort must be desc with around"}`, rec.Body.String())
			}
		})
	}
}

func TestHTTPListMessagesRejectsUnsupportedAuthorType(t *testing.T) {
	repo := &messageListRepo{}
	h := newMessageListHandlers(t, repo)
	c, rec := messageRequestAs(t, "", "/api/v1/task-sessions/sess-b/messages?author_type=agent", "sess-b")

	h.httpListMessages(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":"author_type must be \"user\""}`, rec.Body.String())
	require.Empty(t, repo.paginatedOptions)
	require.Zero(t, repo.listCalls)
}

func TestHTTPListMessagesPassesAuthorFilterAndAround(t *testing.T) {
	repo := &messageListRepo{hasMore: true}
	h := newMessageListHandlers(t, repo)

	c, rec := messageRequestAs(t, "", "/api/v1/task-sessions/sess-b/messages?author_type=user&limit=20", "sess-b")
	h.httpListMessages(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "user", repo.paginatedOptions[0].AuthorType)

	c, rec = messageRequestAs(t, "", "/api/v1/task-sessions/sess-b/messages?around=msg-2&limit=20&sort=desc", "sess-b")
	h.httpListMessages(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "msg-2", repo.paginatedOptions[1].Around)
	var aroundResponse dto.ListMessagesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &aroundResponse))
	require.False(t, aroundResponse.HasMore)
}

func TestHTTPListMessagesSurfacesRepositoryFailure(t *testing.T) {
	repo := &messageListRepo{listErr: errors.New("database unavailable")}
	h := newMessageListHandlers(t, repo)
	c, rec := messageRequestAs(t, "", "/api/v1/task-sessions/sess-b/messages", "sess-b")

	h.httpListMessages(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"failed to list messages"}`, rec.Body.String())
}

func TestHTTPListMessagesMapsMissingAroundTargetToNotFound(t *testing.T) {
	repo := &messageListRepo{paginatedErr: taskrepo.ErrMessageNotFound}
	h := newMessageListHandlers(t, repo)
	c, rec := messageRequestAs(t, "", "/api/v1/task-sessions/sess-b/messages?around=gone&limit=20&sort=desc", "sess-b")

	h.httpListMessages(c)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":"message not found"}`, rec.Body.String())
}

// TestHTTPListMessagesDeniesForeignSession covers the transcript read, which
// carries the full agent conversation.
//
// The denial itself is sound — AuthorizeSessionAccess refuses before any
// repository call — but it surfaces as 500 rather than the 404 every other
// session route answers, because httpListMessages funnels every fetchMessages
// error into one generic branch without separating the authorization sentinel
// from a storage failure. The status is asserted as-is so the behavior is
// pinned; the inconsistency is reported separately rather than treated here as
// if it were intended.
func TestHTTPListMessagesDeniesForeignSession(t *testing.T) {
	repo := &messageListRepo{}
	h := newMessageListHandlers(t, repo)

	c, rec := messageRequestAs(t, "user-a", "/api/v1/task-sessions/sess-b/messages", "sess-b")
	h.httpListMessages(c)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), "hello world", "the transcript must not leak")
	require.Zero(t, repo.listCalls, "a denied read must not reach the repository")

	ownerCtx, ownerRec := messageRequestAs(t, "user-b", "/api/v1/task-sessions/sess-b/messages", "sess-b")
	h.httpListMessages(ownerCtx)
	require.Equal(t, http.StatusOK, ownerRec.Code)
	require.Contains(t, ownerRec.Body.String(), "hello world")
}

func TestWSListMessagesValidatesRequest(t *testing.T) {
	repo := &messageListRepo{}
	h := newMessageListHandlers(t, repo)

	for name, tc := range map[string]struct {
		payload any
		wantMsg string
	}{
		"missing session": {
			payload: map[string]any{},
			wantMsg: "session_id is required",
		},
		"before and after": {
			payload: map[string]any{"session_id": "sess-b", "before": "m1", "after": "m2"},
			wantMsg: "only one of before or after can be set",
		},
		"unknown sort": {
			payload: map[string]any{"session_id": "sess-b", "sort": "sideways"},
			wantMsg: "sort must be asc or desc",
		},
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := h.wsListMessages(context.Background(),
				wsWorkflowRequest(t, ws.ActionMessageList, tc.payload))

			require.NoError(t, err)
			payload := wsWorkflowError(t, resp)
			require.Equal(t, string(ws.ErrorCodeValidation), payload.Code)
			require.Equal(t, tc.wantMsg, payload.Message)
		})
	}
	require.Empty(t, repo.paginatedOptions)

	malformed, err := h.wsListMessages(context.Background(), wsMalformedRequest(ws.ActionMessageList))
	require.NoError(t, err)
	require.Equal(t, string(ws.ErrorCodeBadRequest), wsWorkflowError(t, malformed).Code)
}

func TestWSListMessagesReturnsPageAndCursor(t *testing.T) {
	repo := &messageListRepo{hasMore: true}
	h := newMessageListHandlers(t, repo)

	resp, err := h.wsListMessages(context.Background(),
		wsWorkflowRequest(t, ws.ActionMessageList, map[string]any{
			"session_id": "sess-b", "limit": 10, "sort": "asc",
		}))

	require.NoError(t, err)
	var page dto.ListMessagesResponse
	wsWorkflowResponse(t, resp, &page)
	require.Equal(t, 1, page.Total)
	require.True(t, page.HasMore)
	require.Equal(t, "msg-1", page.Cursor)
	require.Len(t, repo.paginatedOptions, 1)
	require.Equal(t, 10, repo.paginatedOptions[0].Limit)
	require.Equal(t, "asc", repo.paginatedOptions[0].Sort)
}

func TestWSListMessagesSurfacesRepositoryFailure(t *testing.T) {
	repo := &messageListRepo{paginatedErr: errors.New("database unavailable")}
	h := newMessageListHandlers(t, repo)

	resp, err := h.wsListMessages(context.Background(),
		wsWorkflowRequest(t, ws.ActionMessageList, map[string]any{"session_id": "sess-b"}))

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeInternalError), payload.Code)
	require.Equal(t, "Failed to list messages", payload.Message)
}

// TestWSSearchMessagesShortCircuitsOnBlankQuery pins the empty-result contract:
// a whitespace-only query must not reach the repository, and must serialize as
// an empty array rather than null.
func TestWSSearchMessagesShortCircuitsOnBlankQuery(t *testing.T) {
	repo := &messageListRepo{}
	h := newMessageListHandlers(t, repo)

	resp, err := h.wsSearchMessages(context.Background(),
		wsWorkflowRequest(t, ws.ActionMessageSearch, map[string]any{
			"session_id": "sess-b", "query": "   ",
		}))

	require.NoError(t, err)
	var search dto.SearchMessagesResponse
	wsWorkflowResponse(t, resp, &search)
	require.Equal(t, 0, search.Total)
	require.NotNil(t, search.Hits)
	require.Empty(t, repo.searchOptions, "a blank query must not hit the index")
}

func TestWSSearchMessagesReturnsHitsWithSnippets(t *testing.T) {
	repo := &messageListRepo{}
	h := newMessageListHandlers(t, repo)

	resp, err := h.wsSearchMessages(context.Background(),
		wsWorkflowRequest(t, ws.ActionMessageSearch, map[string]any{
			"session_id": "sess-b", "query": "world", "limit": 5,
		}))

	require.NoError(t, err)
	var search dto.SearchMessagesResponse
	wsWorkflowResponse(t, resp, &search)
	require.Equal(t, 1, search.Total)
	require.Equal(t, "msg-1", search.Hits[0].ID)
	require.Equal(t, "turn-1", search.Hits[0].TurnID)
	require.Contains(t, search.Hits[0].Snippet, "world")
	require.Len(t, repo.searchOptions, 1)
	require.Equal(t, "world", repo.searchOptions[0].Query)
	require.Equal(t, 5, repo.searchOptions[0].Limit)
}

func TestWSSearchMessagesValidatesAndSurfacesFailures(t *testing.T) {
	repo := &messageListRepo{}
	h := newMessageListHandlers(t, repo)

	empty, err := h.wsSearchMessages(context.Background(),
		wsWorkflowRequest(t, ws.ActionMessageSearch, map[string]any{"query": "x"}))
	require.NoError(t, err)
	require.Equal(t, "session_id is required", wsWorkflowError(t, empty).Message)

	malformed, err := h.wsSearchMessages(context.Background(), wsMalformedRequest(ws.ActionMessageSearch))
	require.NoError(t, err)
	require.Equal(t, string(ws.ErrorCodeBadRequest), wsWorkflowError(t, malformed).Code)

	failing := newMessageListHandlers(t, &messageListRepo{searchErr: errors.New("index unavailable")})
	resp, err := failing.wsSearchMessages(context.Background(),
		wsWorkflowRequest(t, ws.ActionMessageSearch, map[string]any{"session_id": "sess-b", "query": "x"}))
	require.NoError(t, err)
	require.Equal(t, "Failed to search messages", wsWorkflowError(t, resp).Message)
	require.Empty(t, repo.searchOptions)
}
