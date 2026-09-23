package backendapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	canvasservice "github.com/kandev/kandev/internal/canvas"
)

func TestCanvasRenameRequiresWorkspaceManageScope(t *testing.T) {
	taskSvc, _, _ := newRunSubscriptionCheckHarness(t)
	workspaceID := seedOwnedWorkspace(t, taskSvc, "canvas-owner")
	owner := authn.WithIdentity(context.Background(), authn.Identity{UserID: "canvas-owner", Role: authn.RoleMember})
	if _, err := taskSvc.UpsertWorkspaceMember(owner, workspaceID, "canvas-viewer", "viewer"); err != nil {
		t.Fatal(err)
	}
	handler := &canvasHTTPHandler{tasks: taskSvc}
	for _, test := range []struct {
		name    string
		user    string
		allowed bool
		status  int
	}{
		{name: "owner", user: "canvas-owner", allowed: true, status: http.StatusOK},
		{name: "viewer", user: "canvas-viewer", allowed: false, status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPatch, "/", nil).WithContext(authn.WithIdentity(context.Background(), authn.Identity{UserID: test.user, Role: authn.RoleMember}))
			if allowed := handler.authorizeWorkspaceManage(ctx, workspaceID); allowed != test.allowed {
				t.Fatalf("allowed = %v, want %v", allowed, test.allowed)
			}
			if !test.allowed && recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
		})
	}
}

func TestCanvasDistributionErrorStatusUsesInternalErrorForUnexpectedFailures(t *testing.T) {
	status, code := canvasDistributionErrorStatus(errors.New("database unavailable"))
	if status != http.StatusInternalServerError || code != "internal_error" {
		t.Fatalf("status = %d %q, want 500 internal_error", status, code)
	}
}

func TestCanvasDistributionErrorStatusMapsIncompatibleInstall(t *testing.T) {
	status, code := canvasDistributionErrorStatus(canvasservice.ErrInstallIncompatible)
	if status != http.StatusConflict || code != "incompatible_install" {
		t.Fatalf("status = %d %q, want 409 incompatible_install", status, code)
	}
}

func TestCanvasReleaseResponseIncludesSafeDistributionFields(t *testing.T) {
	response := releaseResponseFromMetadata(&canvasservice.ReleaseMetadata{
		ID: "release-1", PackageID: "canvas-example", Version: "1.1.0", DisplayName: "Example",
		Description: "Example canvas", Author: "Author", License: "MIT", SourceMode: "static",
		MinKandevVersion: "0.95.0", RepoURL: "https://example.test/canvas",
	})
	if response.PackageID != "canvas-example" || response.Version != "1.1.0" || response.DisplayName != "Example" || response.Description != "Example canvas" || response.Author != "Author" || response.License != "MIT" || response.SourceMode != "static" || response.MinKandevVersion != "0.95.0" || response.RepoURL != "https://example.test/canvas" {
		t.Fatalf("distribution projection = %+v", response)
	}
}
