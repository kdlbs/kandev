package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/editors/controller"
	editormodels "github.com/kandev/kandev/internal/editors/models"
	"github.com/kandev/kandev/internal/editors/service"
	"github.com/kandev/kandev/internal/editors/store"
	"github.com/kandev/kandev/internal/user/models"
)

type editorManagementRepository struct {
	store.Repository
	editors map[string]*editormodels.Editor
	writes  int
}

func newEditorManagementRepository() *editorManagementRepository {
	existing := &editormodels.Editor{
		ID:      "existing-editor",
		Type:    "custom:existing-editor",
		Name:    "Existing editor",
		Kind:    "custom_remote_ssh",
		Config:  json.RawMessage(`{"host":"example.com"}`),
		Enabled: true,
	}
	return &editorManagementRepository{editors: map[string]*editormodels.Editor{existing.ID: existing}}
}

func (r *editorManagementRepository) ListEditors(context.Context) ([]*editormodels.Editor, error) {
	editors := make([]*editormodels.Editor, 0, len(r.editors))
	for _, editor := range r.editors {
		editors = append(editors, editor)
	}
	return editors, nil
}

func (r *editorManagementRepository) GetEditorByID(_ context.Context, id string) (*editormodels.Editor, error) {
	return r.editors[id], nil
}

func (r *editorManagementRepository) CreateEditor(_ context.Context, editor *editormodels.Editor) error {
	r.writes++
	r.editors[editor.ID] = editor
	return nil
}

func (r *editorManagementRepository) UpdateEditor(_ context.Context, editor *editormodels.Editor) error {
	r.writes++
	r.editors[editor.ID] = editor
	return nil
}

func (r *editorManagementRepository) DeleteEditor(_ context.Context, id string) error {
	r.writes++
	delete(r.editors, id)
	return nil
}

type editorManagementUserSettings struct{}

func (editorManagementUserSettings) GetUserSettings(context.Context) (*models.UserSettings, error) {
	return &models.UserSettings{}, nil
}

func (editorManagementUserSettings) ClearDefaultEditorID(context.Context, string) error { return nil }

func newEditorManagementRouter(t *testing.T, identity authn.Identity) (*gin.Engine, *editorManagementRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := newEditorManagementRepository()
	svc := service.NewService(repo, nil, editorManagementUserSettings{})
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, identity)
		c.Next()
	})
	RegisterRoutes(router, controller.NewController(svc), log)
	return router, repo
}

func sendEditorManagementRequest(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &payload)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func TestEditorManagementRequiresOrgConfigScope(t *testing.T) {
	mutations := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/api/v1/editors", map[string]any{
			"name": "Remote editor", "kind": "custom_remote_ssh", "config": map[string]string{"host": "example.com"},
		}},
		{http.MethodPatch, "/api/v1/editors/existing-editor", map[string]string{"name": "Updated editor"}},
		{http.MethodDelete, "/api/v1/editors/existing-editor", nil},
	}

	for _, identity := range []authn.Identity{
		{UserID: "member", Role: authn.RoleMember, OrgID: "org-1"},
		{UserID: "guest", Role: authn.RoleGuest, OrgID: "org-1"},
	} {
		t.Run(string(identity.Role), func(t *testing.T) {
			router, repo := newEditorManagementRouter(t, identity)
			for _, mutation := range mutations {
				resp := sendEditorManagementRequest(t, router, mutation.method, mutation.path, mutation.body)
				if resp.Code != http.StatusForbidden {
					t.Errorf("%s %s status = %d, want 403; body: %s", mutation.method, mutation.path, resp.Code, resp.Body.String())
					continue
				}
				var result map[string]string
				if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
					t.Fatalf("decode denial: %v", err)
				}
				if result["scope"] != "org.config.manage" {
					t.Errorf("denial scope = %q, want org.config.manage", result["scope"])
				}
			}
			if repo.writes != 0 {
				t.Errorf("repository writes = %d, want none", repo.writes)
			}
			if resp := sendEditorManagementRequest(t, router, http.MethodGet, "/api/v1/editors", nil); resp.Code != http.StatusOK {
				t.Errorf("GET /editors status = %d, want 200", resp.Code)
			}
		})
	}
}

func TestEditorManagementAllowsAdminAndSyntheticIdentity(t *testing.T) {
	for _, identity := range []authn.Identity{
		{UserID: "admin", Role: authn.RoleAdmin, OrgID: "org-1"},
		{UserID: "local", Role: authn.RoleAdmin, Synthetic: true},
	} {
		t.Run(identity.UserID, func(t *testing.T) {
			router, repo := newEditorManagementRouter(t, identity)
			if resp := sendEditorManagementRequest(t, router, http.MethodGet, "/api/v1/editors", nil); resp.Code != http.StatusOK {
				t.Fatalf("GET /editors status = %d, want 200", resp.Code)
			}
			for _, mutation := range []struct {
				method string
				path   string
				body   any
			}{
				{http.MethodPost, "/api/v1/editors", map[string]any{
					"name": "Remote editor", "kind": "custom_remote_ssh", "config": map[string]string{"host": "example.com"},
				}},
				{http.MethodPatch, "/api/v1/editors/existing-editor", map[string]string{"name": "Updated editor"}},
				{http.MethodDelete, "/api/v1/editors/existing-editor", nil},
			} {
				resp := sendEditorManagementRequest(t, router, mutation.method, mutation.path, mutation.body)
				if resp.Code != http.StatusOK {
					t.Errorf("%s %s status = %d, want 200; body: %s", mutation.method, mutation.path, resp.Code, resp.Body.String())
				}
			}
			if repo.writes != 3 {
				t.Errorf("repository writes = %d, want 3", repo.writes)
			}
		})
	}
}
