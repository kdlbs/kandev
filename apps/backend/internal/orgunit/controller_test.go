package orgunit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
)

func newTestRouter(t *testing.T, role authn.Role) (*gin.Engine, *Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := newTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(authn.WithIdentity(c.Request.Context(),
			authn.Identity{UserID: "user-ana", Role: role}))
		c.Set(authn.GinContextKey, authn.Identity{UserID: "user-ana", Role: role})
		c.Next()
	})
	NewController(svc, log).RegisterRoutes(router)
	return router, svc
}

func do(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// Deciding the shape of an organization is what unit.manage governs. A member
// who does not hold it can look at the tree but must not rearrange it.
func TestUnitWritesRequireUnitManage(t *testing.T) {
	router, svc := newTestRouter(t, authn.RoleMember)
	root, err := svc.EnsureRoot(t.Context(), "", "Acme")
	if err != nil {
		t.Fatalf("ensure root: %v", err)
	}

	if w := do(t, router, http.MethodGet, "/api/v1/units", ""); w.Code != http.StatusOK {
		t.Fatalf("reading the tree = %d, want 200: a member must see why they reach what they reach", w.Code)
	}
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/units", `{"parent_id":"` + root.ID + `","name":"Platform"}`},
		{http.MethodPatch, "/api/v1/units/" + root.ID, `{"name":"Renamed"}`},
		{http.MethodDelete, "/api/v1/units/" + root.ID, ""},
	} {
		if w := do(t, router, tc.method, tc.path, tc.body); w.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", tc.method, tc.path, w.Code)
		}
	}
}

// Each refusal names what is in the way, so a caller can say why rather than
// reporting that something went wrong.
func TestUnitRefusalsAreSpecific(t *testing.T) {
	router, svc := newTestRouter(t, authn.RoleAdmin)
	ctx := t.Context()
	root, err := svc.EnsureRoot(ctx, "", "Acme")
	if err != nil {
		t.Fatalf("ensure root: %v", err)
	}
	dept, err := svc.Create(ctx, root.ID, "Platform")
	if err != nil {
		t.Fatalf("create dept: %v", err)
	}
	if _, err := svc.Create(ctx, dept.ID, "Runtime"); err != nil {
		t.Fatalf("create team: %v", err)
	}
	personal, err := svc.EnsurePersonal(ctx, "", "user-ana", "Ana")
	if err != nil {
		t.Fatalf("ensure personal: %v", err)
	}

	cases := []struct {
		name, method, path, body, wantText string
		wantCode                           int
	}{
		{"a unit with children", http.MethodDelete, "/api/v1/units/" + dept.ID, "", "still holds", http.StatusBadRequest},
		{"the root itself", http.MethodDelete, "/api/v1/units/" + root.ID, "", "cannot be moved or deleted", http.StatusBadRequest},
		{"a member on a personal unit", http.MethodPut, "/api/v1/units/" + personal.ID + "/members/user-bruno", `{"role":"viewer"}`, "takes no members", http.StatusBadRequest},
		{"an unknown role", http.MethodPut, "/api/v1/units/" + dept.ID + "/members/user-bruno", `{"role":"wizard"}`, "must be owner", http.StatusBadRequest},
		{"a unit that does not exist", http.MethodDelete, "/api/v1/units/nope", "", "not found", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, router, tc.method, tc.path, tc.body)
			if w.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d (%s)", w.Code, tc.wantCode, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.wantText) {
				t.Fatalf("body = %s, want it to mention %q", w.Body.String(), tc.wantText)
			}
		})
	}
}

// The happy path: build a department, put someone in it, read it back.
func TestUnitCreateAndMember(t *testing.T) {
	router, svc := newTestRouter(t, authn.RoleAdmin)
	root, err := svc.EnsureRoot(t.Context(), "", "Acme")
	if err != nil {
		t.Fatalf("ensure root: %v", err)
	}

	w := do(t, router, http.MethodPost, "/api/v1/units", `{"parent_id":"`+root.ID+`","name":"Platform"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d body = %s", w.Code, w.Body.String())
	}
	var created Unit
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if w := do(t, router, http.MethodPut,
		"/api/v1/units/"+created.ID+"/members/user-bruno", `{"role":"collaborator"}`); w.Code != http.StatusOK {
		t.Fatalf("add member = %d body = %s", w.Code, w.Body.String())
	}

	w = do(t, router, http.MethodGet, "/api/v1/units/"+created.ID+"/members", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "user-bruno") {
		t.Fatalf("members = %d body = %s", w.Code, w.Body.String())
	}
}

// Reads leak as readily as writes. Knowing a unit id from another tenant must
// not be enough to enumerate who is in it, so the read handler carries the
// same tenant check the write handlers do.
func TestListMembersRefusesAnotherTenantsUnit(t *testing.T) {
	router, svc := newTestRouter(t, authn.RoleAdmin)
	ctx := t.Context()

	// The caller's identity carries no org in this harness, so a unit that
	// does carry one belongs to somebody else.
	foreign, err := svc.Store().Insert(ctx, &Unit{OrgID: "org-other", Kind: KindRoot, Name: "Globex"})
	if err != nil {
		t.Fatalf("seed foreign unit: %v", err)
	}
	if err := svc.SetMember(ctx, foreign.ID, "user-elsewhere", "collaborator", "someone"); err != nil {
		t.Fatalf("seed foreign member: %v", err)
	}

	w := do(t, router, http.MethodGet, "/api/v1/units/"+foreign.ID+"/members", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s, want 404", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "user-elsewhere") {
		t.Fatal("another tenant's membership leaked in the response body")
	}
}
