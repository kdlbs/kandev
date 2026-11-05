package sandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
)

func TestSandboxAuthorityHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name     string
		identity authn.Identity
		enabled  bool
		want     int
	}{
		{"operator", authn.Identity{UserID: "operator", Role: authn.RoleAdmin, SessionID: "browser", OrgID: "org"}, true, 201},
		{"member", authn.Identity{UserID: "member", Role: authn.RoleMember, SessionID: "browser", OrgID: "org"}, true, 403},
		{"PAT", authn.Identity{UserID: "operator", Role: authn.RoleAdmin, TokenID: "pat", OrgID: "org"}, true, 403},
		{"synthetic", authn.Identity{UserID: "operator", Role: authn.RoleAdmin, Synthetic: true}, true, 404},
		{"disabled", authn.Identity{UserID: "operator", Role: authn.RoleAdmin, SessionID: "browser"}, false, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewService(sqlitePolicyDB(t), tc.enabled, nil)
			if err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			router.Use(func(c *gin.Context) { authn.SetOnGin(c, tc.identity) })
			s.RegisterRoutes(router)
			body, err := json.Marshal(map[string]any{"policy": policyFixture(t), "expected_revision": 0})
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodPost, "/api/v1/sandbox/policies", strings.NewReader(string(body)))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestSandboxPolicyHTTPBindingRoundTrip(t *testing.T) {
	s, _, read := policySelectionFixture(t)
	router := gin.New()
	id, _ := authn.IdentityFromContext(operatorContext(t))
	router.Use(func(c *gin.Context) { authn.SetOnGin(c, id) })
	s.RegisterRoutes(router)
	body := `{"binding":{"scope":"task","target_id":"task","revision_id":"` + read.ID + `"},"expected_version":0}`
	for _, request := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPut, "/api/v1/sandbox/bindings/task/task", body, 200},
		{http.MethodPut, "/api/v1/sandbox/bindings/task/task", body, 409},
		{http.MethodGet, "/api/v1/sandbox/bindings/task/task", "", 200},
		{http.MethodPut, "/api/v1/sandbox/bindings/task/other", body, 400},
		{http.MethodPost, "/api/v1/sandbox/policies", `{"unknown_policy_override":true}`, 400},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(request.method, request.path, strings.NewReader(request.body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		if w.Code != request.status {
			t.Fatalf("%s %s status=%d want=%d", request.method, request.path, w.Code, request.status)
		}
	}
	s.SetTargetResolver(func(context.Context, string, string) (TargetScope, error) { return TargetScope{OrgID: "foreign"}, nil })
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/sandbox/bindings/task/task", nil))
	if w.Code != 404 {
		t.Fatalf("foreign binding status=%d", w.Code)
	}
}
