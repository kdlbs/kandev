package toolretention

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/stretchr/testify/require"
)

const retentionPath = "/api/v1/system/database/tool-payload-retention"

type handlerFake struct {
	calls []string
	err   error
}

func (s *handlerFake) Get(context.Context) (Status, error) {
	s.calls = append(s.calls, "get")
	return defaultRecord().Status, s.err
}
func (s *handlerFake) Save(context.Context, Update) (Status, error) {
	s.calls = append(s.calls, "save")
	return defaultRecord().Status, s.err
}
func (s *handlerFake) Analyze(context.Context, Age) (string, error) {
	s.calls = append(s.calls, "analyze")
	return "operation", s.err
}
func (s *handlerFake) Run(context.Context, int64) (string, error) {
	s.calls = append(s.calls, "run")
	return "operation", s.err
}
func (s *handlerFake) Cancel(context.Context, string) (Status, error) {
	s.calls = append(s.calls, "cancel")
	return defaultRecord().Status, s.err
}
func retentionRouter(s HTTPService, role authn.Role) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) { authn.SetOnGin(c, authn.Identity{UserID: "caller", Role: role}); c.Next() })
	read := r.Group("/api/v1/system")
	admin := read.Group("", authz.RequireOrgScope(authz.ScopeOrgSettingsManage))
	RegisterRoutes(read, admin, s)
	return r
}
func retentionRequest(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, retentionPath+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}
func TestHandlerMembersOnlyReadStatus(t *testing.T) {
	s := &handlerFake{}
	r := retentionRouter(s, authn.RoleMember)
	require.Equal(t, 200, retentionRequest(r, "GET", "", "").Code)
	for _, tc := range []struct{ method, path string }{{"PUT", ""}, {"POST", "/analyze"}, {"POST", "/run"}, {"POST", "/cancel"}} {
		require.Equal(t, 403, retentionRequest(r, tc.method, tc.path, `{}`).Code)
	}
	require.Equal(t, []string{"get"}, s.calls)
}
func TestHandlerErrorCodesAreSanitized(t *testing.T) {
	for _, tc := range []struct {
		code   string
		status int
	}{{"invalid_age", 400}, {"invalid_backup_choice", 400}, {"backup_choice_required", 400}, {"conflict", 409}, {"busy", 409}, {"preparation_required", 409}, {"unsupported", 501}, {"database secret /private/path", 500}} {
		t.Run(tc.code, func(t *testing.T) {
			s := &handlerFake{err: errors.New(tc.code)}
			r := retentionRouter(s, authn.RoleAdmin)
			got := retentionRequest(r, "POST", "/run", `{"revision":0}`)
			require.Equal(t, tc.status, got.Code)
			code := tc.code
			if tc.status == 500 {
				code = "internal_error"
			}
			require.JSONEq(t, `{"code":"`+code+`"}`, got.Body.String())
		})
	}
}
func TestHandlerStrictBodies(t *testing.T) {
	for _, body := range []string{`{"revision":0,"extra":1}`, `{"revision":0} {}`, `null`, `{}`, `{"revision":0}` + strings.Repeat(" ", 1<<20)} {
		s := &handlerFake{}
		r := retentionRouter(s, authn.RoleAdmin)
		got := retentionRequest(r, "POST", "/run", body)
		require.Equal(t, http.StatusBadRequest, got.Code)
		require.Empty(t, s.calls)
	}
}
func TestHandlerMutationResults(t *testing.T) {
	for _, tc := range []struct {
		method, path, body, call string
		status                   int
	}{{"PUT", "", `{"enabled":false,"age":{"value":3,"unit":"months"},"revision":0}`, "save", 200}, {"POST", "/analyze", `{"age":{"value":3,"unit":"months"}}`, "analyze", 202}, {"POST", "/run", `{"revision":0}`, "run", 202}, {"POST", "/cancel", `{"operation_id":"operation"}`, "cancel", 200}} {
		s := &handlerFake{}
		r := retentionRouter(s, authn.RoleAdmin)
		got := retentionRequest(r, tc.method, tc.path, tc.body)
		require.Equal(t, tc.status, got.Code, got.Body.String())
		require.Equal(t, []string{tc.call}, s.calls)
		if tc.status == 202 {
			require.JSONEq(t, `{"operation_id":"operation"}`, got.Body.String())
		}
	}
}
func TestHandlerGetDoesNotStartAnalysisOrWritePolicy(t *testing.T) {
	s := testService(t)
	r := retentionRouter(s, authn.RoleMember)
	require.Equal(t, 200, retentionRequest(r, "GET", "", "").Code)
	var count int
	require.NoError(t, s.pool.Reader().Get(&count, `SELECT count(*) FROM settings WHERE key='tool_payload_retention'`))
	require.Zero(t, count)
	state, err := s.Get(context.Background())
	require.NoError(t, err)
	require.Nil(t, state.Operation)
}
