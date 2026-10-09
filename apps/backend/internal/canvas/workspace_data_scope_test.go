package canvas

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/webapp"
)

func TestFirstPublicationRecordsWorkspaceDataTransition(t *testing.T) {
	service, instanceStore, _ := newCanvasService(t)
	created := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID: "workspace-1", TaskID: "task-1", Title: "Owner-authorized canvas",
		CreatedBySessionID: "session-1", OwnerUserID: "owner-1",
	})
	instance, err := instanceStore.Get(context.Background(), created.PluginInstanceID)
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	metric := "transition=first_publication;result=data_scope_enabled"
	before := canvasScopeMetricCount(t, metric)
	result, err := service.PublishPackage(context.Background(), PublishRequest{
		CanvasID:          created.ID,
		Package:           testCanvasPackage("workspace-metric", []string{"tasks"}),
		Artifact:          webapp.Artifact{Digest: "workspace-metric", RelativePath: "releases/workspace-metric", Bytes: 1},
		ExpectedAuthority: instance.PublishAuthority(),
		SourceActorKind:   "agent",
		SourceUserID:      "owner-1",
		SourceTaskID:      "task-1",
		SourceSessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("publish owner-authorized canvas: %v", err)
	}
	if !result.Activated {
		t.Fatalf("publish result = %+v, want activated first release", result)
	}
	if after := canvasScopeMetricCount(t, metric); after-before != 1 {
		t.Fatalf("first-publication scope metric delta = %d, want 1", after-before)
	}
}

func TestWorkspaceDataUpgradePreservesPlacementAndRequiresFreshOwnerReview(t *testing.T) {
	service, instanceStore, _ := newCanvasService(t)
	canvas := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID: "workspace-1", TaskID: "task-1", Title: "Legacy canvas",
	})
	published := publishTestPackage(t, service, canvas.ID, "legacy-workspace-review", []string{"tasks"})
	if _, err := service.ApproveRelease(context.Background(), canvas.ID, published.Release.ID, "owner-1"); err != nil {
		t.Fatalf("approve legacy task release: %v", err)
	}

	preview, err := service.WorkspaceDataPreview(context.Background(), canvas.ID)
	if err != nil {
		t.Fatalf("workspace data preview: %v", err)
	}
	if preview.CurrentDataScopeKind != instances.ScopeTask || preview.TargetDataScopeKind != instances.ScopeWorkspace ||
		preview.ActiveReleaseID != published.Release.ID || preview.Permissions.Reads[0] != "tasks" {
		t.Fatalf("workspace data preview = %+v, want task-to-workspace review of the active task read", preview)
	}
	if err := instanceStore.AddGrant(context.Background(), instances.Grant{
		InstanceID: canvas.PluginInstanceID, PermissionKind: "api_read", Resource: "tasks",
		ScopeCeiling: instances.ScopeTask, ApprovedBy: "owner-1",
	}); err != nil {
		t.Fatalf("advance grant generation: %v", err)
	}
	if err := instanceStore.AddGrant(context.Background(), instances.Grant{
		InstanceID: canvas.PluginInstanceID, PermissionKind: "api_write", Resource: "messages",
		ScopeCeiling: instances.ScopeTask, ApprovedBy: "owner-1",
	}); err != nil {
		t.Fatalf("seed stale task-ceiling grant: %v", err)
	}
	if _, err := service.EnableWorkspaceDataReviewed(context.Background(), canvas.ID, "owner-1", preview.ActiveReleaseID, preview.PermissionDigest, preview.GrantGeneration); !errors.Is(err, instances.ErrStaleWorkspaceDataReview) {
		t.Fatalf("stale workspace data confirmation = %v, want ErrStaleWorkspaceDataReview", err)
	}
	unchanged, err := instanceStore.Get(context.Background(), canvas.PluginInstanceID)
	if err != nil {
		t.Fatalf("get unchanged instance: %v", err)
	}
	if unchanged.ScopeKind != ScopeTask || unchanged.EffectiveDataScopeKind() != instances.ScopeTask {
		t.Fatalf("stale review changed scopes to placement %q/data %q", unchanged.ScopeKind, unchanged.EffectiveDataScopeKind())
	}

	preview, err = service.WorkspaceDataPreview(context.Background(), canvas.ID)
	if err != nil {
		t.Fatalf("refresh workspace data preview: %v", err)
	}
	if _, err := service.EnableWorkspaceDataReviewed(context.Background(), canvas.ID, "owner-2", preview.ActiveReleaseID, preview.PermissionDigest, preview.GrantGeneration); !errors.Is(err, instances.ErrWorkspaceOwnerRequired) {
		t.Fatalf("foreign owner confirmation = %v, want ErrWorkspaceOwnerRequired", err)
	}
	metric := "transition=reviewed_upgrade;result=data_scope_enabled"
	before := canvasScopeMetricCount(t, metric)
	upgraded, err := service.EnableWorkspaceDataReviewed(context.Background(), canvas.ID, "owner-1", preview.ActiveReleaseID, preview.PermissionDigest, preview.GrantGeneration)
	if err != nil {
		t.Fatalf("owner enables workspace data: %v", err)
	}
	if after := canvasScopeMetricCount(t, metric); after-before != 1 {
		t.Fatalf("reviewed-upgrade scope metric delta = %d, want 1", after-before)
	}
	if upgraded.ScopeKind != ScopeTask || upgraded.DataScopeKind != instances.ScopeWorkspace ||
		upgraded.TaskID != "task-1" || upgraded.ActiveReleaseID != published.Release.ID || upgraded.PromotedAt != nil {
		t.Fatalf("upgraded canvas = %+v, want unchanged task placement and active release", upgraded)
	}
	grants, err := instanceStore.ListGrants(context.Background(), canvas.PluginInstanceID)
	if err != nil {
		t.Fatalf("list upgraded grants: %v", err)
	}
	if len(grants) != 1 || grants[0].PermissionKind != "api_read" || grants[0].Resource != "tasks" || grants[0].ScopeCeiling != instances.ScopeWorkspace || grants[0].ApprovedBy != "owner-1" {
		t.Fatalf("upgraded grants = %+v, want one owner-approved workspace task grant", grants)
	}
}

func TestPromotionRecordsExpandedDataScope(t *testing.T) {
	service, _, _ := newCanvasService(t)
	created := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID: "workspace-1", TaskID: "task-1", Title: "Promoted canvas",
	})
	published := publishTestPackage(t, service, created.ID, "promotion-scope-metric", []string{"tasks"})
	if _, err := service.ApproveRelease(context.Background(), created.ID, published.Release.ID, "owner-1"); err != nil {
		t.Fatalf("approve task-scoped release: %v", err)
	}
	preview, err := service.PromotionPreview(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("promotion preview: %v", err)
	}
	metric := "transition=promotion;result=data_scope_expanded"
	before := canvasScopeMetricCount(t, metric)
	promoted, err := service.PromoteCanvasReviewed(
		context.Background(), created.ID, "owner-1", preview.ActiveReleaseID,
		preview.PermissionDigest, preview.GrantGeneration,
	)
	if err != nil {
		t.Fatalf("promote task-scoped canvas: %v", err)
	}
	if promoted.ScopeKind != ScopeWorkspace || promoted.DataScopeKind != instances.ScopeWorkspace {
		t.Fatalf("promoted scopes = placement %q, data %q, want workspace/workspace", promoted.ScopeKind, promoted.DataScopeKind)
	}
	if after := canvasScopeMetricCount(t, metric); after-before != 1 {
		t.Fatalf("promotion scope metric delta = %d, want 1", after-before)
	}
}
