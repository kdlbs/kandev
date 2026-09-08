package skills_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/skills"
)

// newTestSkillRouter mounts the skills routes over a fresh service.
func newTestSkillRouter(t *testing.T) (*gin.Engine, *skills.SkillService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := newTestSkillService(t)
	router := gin.New()
	skills.NewHandler(svc).RegisterRoutes(router.Group("/api/v1"))
	return router, svc
}

func doSkillRequest(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestUpdateSkillHandler_RejectsEmptySlug(t *testing.T) {
	router, svc := newTestSkillRouter(t)
	ctx := context.Background()

	skill := &models.Skill{WorkspaceID: "ws-1", Name: "Existing", Slug: "kandev-existing", SourceType: "inline"}
	if err := svc.ValidateAndPrepareSkill(ctx, skill); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}

	rec := doSkillRequest(t, router, http.MethodPatch, "/api/v1/skills/"+skill.ID, `{"slug":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	reloaded, err := svc.GetSkillFromConfig(ctx, skill.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Slug != "kandev-existing" {
		t.Errorf("stored slug = %q, want unchanged %q", reloaded.Slug, "kandev-existing")
	}
}

func TestUpdateSkillHandler_RejectsNotWellFormedSlug(t *testing.T) {
	router, svc := newTestSkillRouter(t)
	ctx := context.Background()

	skill := &models.Skill{WorkspaceID: "ws-1", Name: "Existing", Slug: "kandev-existing", SourceType: "inline"}
	if err := svc.ValidateAndPrepareSkill(ctx, skill); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}

	rec := doSkillRequest(t, router, http.MethodPatch, "/api/v1/skills/"+skill.ID, `{"slug":"not a valid slug!"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	reloaded, err := svc.GetSkillFromConfig(ctx, skill.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Slug != "kandev-existing" {
		t.Errorf("stored slug = %q, want unchanged %q", reloaded.Slug, "kandev-existing")
	}
}

func TestUpdateSkillHandler_OmittedSlugLeavesItUnchanged(t *testing.T) {
	router, svc := newTestSkillRouter(t)
	ctx := context.Background()

	skill := &models.Skill{WorkspaceID: "ws-1", Name: "Existing", Slug: "kandev-existing", SourceType: "inline"}
	if err := svc.ValidateAndPrepareSkill(ctx, skill); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}

	rec := doSkillRequest(t, router, http.MethodPatch, "/api/v1/skills/"+skill.ID, `{"content":"new content"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp skills.SkillResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if resp.Skill.Slug != "kandev-existing" {
		t.Errorf("response slug = %q, want unchanged %q", resp.Skill.Slug, "kandev-existing")
	}

	reloaded, err := svc.GetSkillFromConfig(ctx, skill.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Slug != "kandev-existing" {
		t.Errorf("stored slug = %q, want unchanged %q", reloaded.Slug, "kandev-existing")
	}
}
