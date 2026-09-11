package canvas

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	plugininstances "github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/webapp"
)

func TestPublishPackageFirstReleaseRequiresMatchingGrants(t *testing.T) {
	tests := []struct {
		name          string
		reads         []string
		wantActivated bool
		wantStatus    string
	}{
		{
			name:          "zero permissions activate",
			wantActivated: true,
			wantStatus:    plugininstances.ValidationValid,
		},
		{
			name:          "declared permissions require grants",
			reads:         []string{"tasks"},
			wantActivated: false,
			wantStatus:    plugininstances.ValidationPendingPermission,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, instanceStore, _ := newCanvasService(t)
			canvas := createCanvas(t, service, CreateCanvasRequest{
				WorkspaceID: "workspace-1",
				TaskID:      "task-1",
				Title:       "First release",
			})

			result := publishTestPackage(t, service, canvas.ID, "first-release", tt.reads)
			if result.Activated != tt.wantActivated {
				t.Fatalf("activated = %t, want %t", result.Activated, tt.wantActivated)
			}
			if result.Release.ValidationStatus != tt.wantStatus {
				t.Fatalf("release status = %q, want %q", result.Release.ValidationStatus, tt.wantStatus)
			}

			instance, err := instanceStore.Get(context.Background(), canvas.PluginInstanceID)
			if err != nil {
				t.Fatalf("get instance: %v", err)
			}
			if tt.wantActivated {
				if instance.ActiveReleaseID != result.Release.ID {
					t.Fatalf("active release = %q, want %q", instance.ActiveReleaseID, result.Release.ID)
				}
			} else if instance.ActiveReleaseID != "" {
				t.Fatalf("active release = %q, want no active release", instance.ActiveReleaseID)
			}
		})
	}
}

func TestCanvasCreationAuthorityFirstPublish(t *testing.T) {
	service, instanceStore, _ := newCanvasService(t)
	created := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID:        "workspace-1",
		TaskID:             "task-1",
		Title:              "Owner-authorized canvas",
		CreatedBySessionID: "session-1",
		OwnerUserID:        "owner-1",
	})

	authority, err := service.repo.GetCreationAuthority(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get creation authority: %v", err)
	}
	instance, err := instanceStore.Get(context.Background(), created.PluginInstanceID)
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	pkg := testCanvasPackage("owner-first", []string{"tasks"})
	pkg.Manifest.Capabilities.APIWrite = []string{"messages"}
	pkg.Manifest.Capabilities.Events = []string{"task.updated"}
	pkg.Manifest.Capabilities.State = true
	pkg.Manifest.UI.WebApps[0].NetworkOrigins = []string{"https://api.example.test"}
	result, err := service.PublishPackage(context.Background(), PublishRequest{
		CanvasID:          created.ID,
		Package:           pkg,
		Artifact:          webapp.Artifact{Digest: "owner-first", RelativePath: "releases/owner-first", Bytes: 1},
		ExpectedAuthority: instance.PublishAuthority(),
		SourceActorKind:   "agent",
		SourceUserID:      "owner-1",
		SourceTaskID:      "task-1",
		SourceSessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("publish owner-authorized canvas: %v", err)
	}
	if !result.Activated || result.PermissionRequired {
		t.Fatalf("publish result = %+v, want activated without review", result)
	}

	updated, err := instanceStore.Get(context.Background(), created.PluginInstanceID)
	if err != nil {
		t.Fatalf("get updated instance: %v", err)
	}
	if updated.ActiveReleaseID != result.Release.ID || updated.PluginID != result.Release.PluginID {
		t.Fatalf("updated instance = %+v, want active release and package identity", updated)
	}
	grants, err := instanceStore.ListGrants(context.Background(), created.PluginInstanceID)
	if err != nil {
		t.Fatalf("list initial grants: %v", err)
	}
	if len(grants) != 5 {
		t.Fatalf("initial grants = %+v, want five declared grants", grants)
	}
	wantGrants := map[string]bool{
		"api_read:tasks":                   false,
		"api_write:messages":               false,
		"events:task.updated":              false,
		"state:":                           false,
		"network:https://api.example.test": false,
	}
	for _, grant := range grants {
		key := grant.PermissionKind + ":"
		if grant.PermissionKind == "api_read" || grant.PermissionKind == "api_write" || grant.PermissionKind == "events" {
			key += grant.Resource
		}
		if grant.PermissionKind == "network" {
			key += grant.NetworkOrigin
		}
		if _, ok := wantGrants[key]; !ok || grant.ScopeCeiling != ScopeTask || grant.ApprovedBy != "owner-1" {
			t.Fatalf("initial grant = %+v, want task-scoped owner grant", grant)
		}
		wantGrants[key] = true
	}
	for key, found := range wantGrants {
		if !found {
			t.Fatalf("initial grant %q missing from %+v", key, grants)
		}
	}

	consumed, err := service.repo.GetCreationAuthority(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get consumed authority: %v", err)
	}
	if consumed.PolicyVersion != authority.PolicyVersion || consumed.ConsumedAt.IsZero() {
		t.Fatalf("consumed authority = %+v, want policy %d and consumed timestamp", consumed, authority.PolicyVersion)
	}
}

func TestCanvasCreationAuthorityRejectsMismatchedSource(t *testing.T) {
	service, instanceStore, _ := newCanvasService(t)
	created := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID:        "workspace-1",
		TaskID:             "task-1",
		Title:              "Mismatched owner",
		CreatedBySessionID: "session-1",
		OwnerUserID:        "owner-1",
	})
	instance, err := instanceStore.Get(context.Background(), created.PluginInstanceID)
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	result, err := service.PublishPackage(context.Background(), PublishRequest{
		CanvasID:          created.ID,
		Package:           testCanvasPackage("mismatched-owner", []string{"tasks"}),
		Artifact:          webapp.Artifact{Digest: "mismatched-owner", RelativePath: "releases/mismatched-owner", Bytes: 1},
		ExpectedAuthority: instance.PublishAuthority(),
		SourceActorKind:   "agent",
		SourceUserID:      "foreign-owner",
		SourceTaskID:      "task-1",
		SourceSessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("publish mismatched owner: %v", err)
	}
	if result.Activated || !result.PermissionRequired {
		t.Fatalf("mismatched owner result = %+v, want manual review", result)
	}
	grants, err := instanceStore.ListGrants(context.Background(), created.PluginInstanceID)
	if err != nil {
		t.Fatalf("list mismatched-owner grants: %v", err)
	}
	if len(grants) != 0 {
		t.Fatalf("mismatched-owner grants = %+v, want none", grants)
	}
	authority, err := service.repo.GetCreationAuthority(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get unconsumed authority: %v", err)
	}
	if !authority.ConsumedAt.IsZero() {
		t.Fatalf("mismatched owner consumed authority at %s", authority.ConsumedAt)
	}
}

func TestCanvasCreationAuthorityDoesNotApproveLaterIncrease(t *testing.T) {
	service, instanceStore, _ := newCanvasService(t)
	created := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID:        "workspace-1",
		TaskID:             "task-1",
		Title:              "Later review",
		CreatedBySessionID: "session-1",
		OwnerUserID:        "owner-1",
	})
	firstInstance, err := instanceStore.Get(context.Background(), created.PluginInstanceID)
	if err != nil {
		t.Fatalf("get first instance: %v", err)
	}
	first, err := service.PublishPackage(context.Background(), PublishRequest{
		CanvasID:          created.ID,
		Package:           testCanvasPackage("owner-base", []string{"tasks"}),
		Artifact:          webapp.Artifact{Digest: "owner-base", RelativePath: "releases/owner-base", Bytes: 1},
		ExpectedAuthority: firstInstance.PublishAuthority(),
		SourceActorKind:   "agent",
		SourceUserID:      "owner-1",
		SourceTaskID:      "task-1",
		SourceSessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("publish first owner release: %v", err)
	}
	if !first.Activated {
		t.Fatalf("first owner release = %+v, want active", first)
	}

	secondInstance, err := instanceStore.Get(context.Background(), created.PluginInstanceID)
	if err != nil {
		t.Fatalf("get second instance: %v", err)
	}
	secondPackage := testCanvasPackage("owner-increase", []string{"tasks", "workflows"})
	second, err := service.PublishPackage(context.Background(), PublishRequest{
		CanvasID:          created.ID,
		Package:           secondPackage,
		Artifact:          webapp.Artifact{Digest: "owner-increase", RelativePath: "releases/owner-increase", Bytes: 1},
		ExpectedAuthority: secondInstance.PublishAuthority(),
		SourceActorKind:   "agent",
		SourceUserID:      "owner-1",
		SourceTaskID:      "task-1",
		SourceSessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("publish later owner release: %v", err)
	}
	if second.Activated || !second.PermissionRequired {
		t.Fatalf("later owner release = %+v, want pending review", second)
	}
	current, err := instanceStore.Get(context.Background(), created.PluginInstanceID)
	if err != nil {
		t.Fatalf("get current instance: %v", err)
	}
	if current.ActiveReleaseID != first.Release.ID {
		t.Fatalf("active release after later increase = %q, want %q", current.ActiveReleaseID, first.Release.ID)
	}
}

func TestCanvasCreationAuthorityRollsBackOnActivationFailure(t *testing.T) {
	service, instanceStore, _ := newCanvasService(t)
	created := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID:        "workspace-1",
		TaskID:             "task-1",
		Title:              "Rollback authority",
		CreatedBySessionID: "session-1",
		OwnerUserID:        "owner-1",
	})
	instance, err := instanceStore.Get(context.Background(), created.PluginInstanceID)
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	service.instances = &activationFailureStore{Store: instanceStore}
	_, err = service.PublishPackage(context.Background(), PublishRequest{
		CanvasID:          created.ID,
		Package:           testCanvasPackage("rollback-authority", []string{"tasks"}),
		Artifact:          webapp.Artifact{Digest: "rollback-authority", RelativePath: "releases/rollback-authority", Bytes: 1},
		ExpectedAuthority: instance.PublishAuthority(),
		SourceActorKind:   "agent",
		SourceUserID:      "owner-1",
		SourceTaskID:      "task-1",
		SourceSessionID:   "session-1",
	})
	if err == nil {
		t.Fatal("publish with activation failure succeeded, want rollback")
	}
	releases, err := instanceStore.ListReleases(context.Background(), created.PluginInstanceID)
	if err != nil {
		t.Fatalf("list releases after rollback: %v", err)
	}
	if len(releases) != 0 {
		t.Fatalf("releases after rollback = %+v, want none", releases)
	}
	grants, err := instanceStore.ListGrants(context.Background(), created.PluginInstanceID)
	if err != nil {
		t.Fatalf("list grants after rollback: %v", err)
	}
	if len(grants) != 0 {
		t.Fatalf("grants after rollback = %+v, want none", grants)
	}
	current, err := instanceStore.Get(context.Background(), created.PluginInstanceID)
	if err != nil {
		t.Fatalf("get instance after rollback: %v", err)
	}
	if current.ActiveReleaseID != "" || current.PluginID != CanvasPluginID {
		t.Fatalf("instance after rollback = %+v, want pending synthetic instance", current)
	}
	authority, err := service.repo.GetCreationAuthority(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get authority after rollback: %v", err)
	}
	if !authority.ConsumedAt.IsZero() {
		t.Fatalf("authority after rollback consumed at %s", authority.ConsumedAt)
	}
}

func TestFirstTaskReleaseCanBeReviewedAndApproved(t *testing.T) {
	service, instanceStore, _ := newCanvasService(t)
	canvas := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID: "workspace-1",
		TaskID:      "task-1",
		Title:       "First approval",
	})
	published := publishTestPackage(t, service, canvas.ID, "first-approved", []string{"tasks"})
	if published.Activated || !published.PermissionRequired {
		t.Fatalf("first release result = %+v, want pending permission review", published)
	}
	before, err := service.Get(context.Background(), canvas.ID)
	if err != nil {
		t.Fatalf("get pending canvas: %v", err)
	}
	if before.PendingRelease == nil || before.PendingRelease.Permissions == nil || len(before.PendingRelease.Permissions.Reads) != 1 || len(before.PendingRelease.MissingPermissions) != 1 {
		t.Fatalf("pending release review projection = %+v, want declared and missing tasks permission", before.PendingRelease)
	}
	if before.PendingRelease.Permissions.Reads[0] != "tasks" || before.PendingRelease.MissingPermissions[0] != "api_read:tasks" {
		t.Fatalf("pending permission projection = %+v, want api_read:tasks", before.PendingRelease)
	}

	approved, err := service.ApproveRelease(context.Background(), canvas.ID, published.Release.ID, "user-1")
	if err != nil {
		t.Fatalf("approve first task release: %v", err)
	}
	if approved.ActiveReleaseID != published.Release.ID || approved.ActiveReleaseStatus != plugininstances.ValidationValid {
		t.Fatalf("approved canvas = %+v, want active valid release", approved)
	}
	instance, err := instanceStore.Get(context.Background(), canvas.PluginInstanceID)
	if err != nil {
		t.Fatalf("get approved instance: %v", err)
	}
	if instance.PluginID != published.Release.PluginID {
		t.Fatalf("approved plugin id = %q, want release plugin id %q", instance.PluginID, published.Release.PluginID)
	}
	grants, err := instanceStore.ListGrants(context.Background(), canvas.PluginInstanceID)
	if err != nil {
		t.Fatalf("list approved grants: %v", err)
	}
	if len(grants) != 1 || grants[0].PermissionKind != "api_read" || grants[0].Resource != "tasks" {
		t.Fatalf("approved grants = %+v, want api_read:tasks", grants)
	}
}

func TestPublishPackageRejectsStaleEditBaseRelease(t *testing.T) {
	service, instanceStore, _ := newCanvasService(t)
	canvas := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID: "workspace-1",
		Title:       "Concurrent edits",
	})
	first := publishTestPackage(t, service, canvas.ID, "edit-base-a", nil)
	second := publishTestPackage(t, service, canvas.ID, "edit-base-b", nil)

	_, err := service.PublishPackage(context.Background(), PublishRequest{
		CanvasID:              canvas.ID,
		Package:               testCanvasPackage("edit-stale", nil),
		Artifact:              webapp.Artifact{Digest: "edit-stale", RelativePath: "releases/edit-stale", Bytes: 1},
		SourceActorKind:       "agent",
		ExpectedBaseReleaseID: first.Release.ID,
	})
	if !errors.Is(err, ErrStaleCanvasEdit) {
		t.Fatalf("stale edit publish error = %v, want ErrStaleCanvasEdit", err)
	}
	instance, err := instanceStore.Get(context.Background(), canvas.PluginInstanceID)
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	if instance.ActiveReleaseID != second.Release.ID {
		t.Fatalf("active release = %q, want newer edit %q", instance.ActiveReleaseID, second.Release.ID)
	}
}

func TestPromotionRejectsStaleReleaseReview(t *testing.T) {
	service, instanceStore, _ := newCanvasService(t)
	canvas := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID: "workspace-1",
		TaskID:      "task-1",
		Title:       "Review race",
	})
	first := publishTestPackage(t, service, canvas.ID, "review-a", nil)
	preview, err := service.PromotionPreview(context.Background(), canvas.ID)
	if err != nil {
		t.Fatalf("promotion preview: %v", err)
	}
	if preview.ActiveReleaseID != first.Release.ID {
		t.Fatalf("preview release = %q, want %q", preview.ActiveReleaseID, first.Release.ID)
	}
	second := publishTestPackage(t, service, canvas.ID, "review-b", nil)

	_, err = service.PromoteCanvasReviewed(context.Background(), canvas.ID, "user-1", preview.ActiveReleaseID, preview.PermissionDigest, preview.GrantGeneration)
	if !errors.Is(err, ErrStalePromotionReview) {
		t.Fatalf("stale promotion error = %v, want ErrStalePromotionReview", err)
	}
	instance, err := instanceStore.Get(context.Background(), canvas.PluginInstanceID)
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	if instance.ActiveReleaseID != second.Release.ID || instance.ScopeKind != plugininstances.ScopeTask {
		t.Fatalf("instance after stale promotion = %+v, want task scope and release %q", instance, second.Release.ID)
	}
}

func TestPromotionRejectsChangedGrantGeneration(t *testing.T) {
	service, instanceStore, _ := newCanvasService(t)
	canvas := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID: "workspace-1",
		TaskID:      "task-1",
		Title:       "Grant review race",
	})
	publishTestPackage(t, service, canvas.ID, "grant-review", nil)
	preview, err := service.PromotionPreview(context.Background(), canvas.ID)
	if err != nil {
		t.Fatalf("promotion preview: %v", err)
	}
	if err := instanceStore.AddGrant(context.Background(), plugininstances.Grant{
		InstanceID:     canvas.PluginInstanceID,
		PermissionKind: "api_read",
		Resource:       "tasks",
		ScopeCeiling:   plugininstances.ScopeTask,
		ApprovedBy:     "user-1",
	}); err != nil {
		t.Fatalf("add grant: %v", err)
	}

	_, err = service.PromoteCanvasReviewed(context.Background(), canvas.ID, "user-1", preview.ActiveReleaseID, preview.PermissionDigest, preview.GrantGeneration)
	if !errors.Is(err, ErrStalePromotionReview) {
		t.Fatalf("changed grant generation error = %v, want ErrStalePromotionReview", err)
	}
}

func TestPermissionsFitHonorsInstanceScope(t *testing.T) {
	permissions := PermissionSummary{Reads: []string{"tasks"}}
	grants := []plugininstances.Grant{{
		PermissionKind: "api_read",
		Resource:       "tasks",
		ScopeCeiling:   plugininstances.ScopeTask,
	}}
	if permissionsFit(permissions, plugininstances.ScopeTask, grants) != true {
		t.Fatal("task grant did not cover task-scoped release")
	}
	if permissionsFit(permissions, plugininstances.ScopeWorkspace, grants) {
		t.Fatal("task grant covered workspace-scoped release")
	}
}

func TestCanvasProjectionIncludesEffectiveGrantedPermissions(t *testing.T) {
	service, instanceStore, _ := newCanvasService(t)
	canvas := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID: "workspace-1",
		TaskID:      "task-1",
		Title:       "Effective grants",
	})
	if err := instanceStore.AddGrant(context.Background(), plugininstances.Grant{
		InstanceID:     canvas.PluginInstanceID,
		PermissionKind: "api_read",
		Resource:       "tasks",
		ScopeCeiling:   plugininstances.ScopeTask,
		ApprovedBy:     "user-1",
	}); err != nil {
		t.Fatalf("add grant: %v", err)
	}
	publishTestPackage(t, service, canvas.ID, "effective-grants", []string{"tasks"})

	got, err := service.Get(context.Background(), canvas.ID)
	if err != nil {
		t.Fatalf("get canvas: %v", err)
	}
	if len(got.EffectiveGrants) != 1 || got.EffectiveGrants[0].PermissionKind != "api_read" || got.EffectiveGrants[0].Resource != "tasks" {
		t.Fatalf("effective grants = %+v, want api_read:tasks", got.EffectiveGrants)
	}
}

func TestPublishPackagePrunesSupersededPendingReleasesWithoutChangingActive(t *testing.T) {
	service, instanceStore, pool := newCanvasService(t)
	canvas := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID: "workspace-1",
		TaskID:      "task-1",
		Title:       "Pending releases",
	})
	setAuthoringTestClock(service)

	active := publishTestPackage(t, service, canvas.ID, "active-release", nil)
	pendingOne := publishTestPackage(t, service, canvas.ID, "pending-one", []string{"tasks"})
	pendingTwo := publishTestPackage(t, service, canvas.ID, "pending-two", []string{"workflows"})

	instance, err := instanceStore.Get(context.Background(), canvas.PluginInstanceID)
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	if instance.ActiveReleaseID != active.Release.ID {
		t.Fatalf("active release = %q, want %q", instance.ActiveReleaseID, active.Release.ID)
	}

	releases, err := instanceStore.ListReleases(context.Background(), canvas.PluginInstanceID)
	if err != nil {
		t.Fatalf("list releases: %v", err)
	}
	assertReleaseStatus(t, releases, active.Release.ID, plugininstances.ValidationValid, "")
	assertReleaseAbsent(t, releases, pendingOne.Release.ID)
	assertReleaseStatus(t, releases, pendingTwo.Release.ID, plugininstances.ValidationPendingPermission, "permission_review_required")
	if got := countReleaseStatus(releases, plugininstances.ValidationPendingPermission); got != 1 {
		t.Fatalf("pending releases = %d, want 1", got)
	}
	if got := cleanupArtifactPaths(t, pool, canvas.PluginInstanceID); len(got) != 1 || got[0] != "releases/pending-one" {
		t.Fatalf("cleanup paths = %v, want [releases/pending-one]", got)
	}
}

func TestPublishPackageRetainsPriorValidReleaseForRollback(t *testing.T) {
	service, instanceStore, pool := newCanvasService(t)
	canvas := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID: "workspace-1",
		TaskID:      "task-1",
		Title:       "Valid releases",
	})
	setAuthoringTestClock(service)
	active := publishTestPackage(t, service, canvas.ID, "active-release", nil)

	if err := instanceStore.AddGrant(context.Background(), plugininstances.Grant{
		InstanceID:     canvas.PluginInstanceID,
		PermissionKind: "api_read",
		Resource:       "tasks",
		ScopeCeiling:   plugininstances.ScopeTask,
		ApprovedBy:     "user-1",
	}); err != nil {
		t.Fatalf("add grant: %v", err)
	}
	prior := publishTestPackage(t, service, canvas.ID, "prior-release", []string{"tasks"})
	latest := publishTestPackage(t, service, canvas.ID, "latest-release", []string{"tasks"})

	instance, err := instanceStore.Get(context.Background(), canvas.PluginInstanceID)
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	if instance.ActiveReleaseID != latest.Release.ID {
		t.Fatalf("active release = %q, want %q", instance.ActiveReleaseID, latest.Release.ID)
	}

	releases, err := instanceStore.ListReleases(context.Background(), canvas.PluginInstanceID)
	if err != nil {
		t.Fatalf("list releases: %v", err)
	}
	assertReleaseAbsent(t, releases, active.Release.ID)
	assertReleaseStatus(t, releases, prior.Release.ID, plugininstances.ValidationValid, "")
	assertReleaseStatus(t, releases, latest.Release.ID, plugininstances.ValidationValid, "")
	if got := countReleaseStatus(releases, plugininstances.ValidationValid); got != 2 {
		t.Fatalf("valid releases = %d, want active plus one prior", got)
	}
	if got := cleanupArtifactPaths(t, pool, canvas.PluginInstanceID); len(got) != 1 || got[0] != "releases/active-release" {
		t.Fatalf("cleanup paths = %v, want [releases/active-release]", got)
	}
	if err := instanceStore.ActivateRelease(context.Background(), canvas.PluginInstanceID, active.Release.ID); !errors.Is(err, plugininstances.ErrInvalidRelease) {
		t.Fatalf("activate superseded release = %v, want ErrInvalidRelease", err)
	}

	rolledBack, err := service.RollbackRelease(context.Background(), canvas.ID, "")
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if rolledBack.ActiveReleaseID != prior.Release.ID {
		t.Fatalf("rollback active release = %q, want %q", rolledBack.ActiveReleaseID, prior.Release.ID)
	}
	if rolledBack.PluginInstanceID != canvas.PluginInstanceID || rolledBack.ScopeKind != plugininstances.ScopeTask {
		t.Fatalf("rollback changed canvas identity or scope: %+v", rolledBack)
	}
}

func TestRejectReleasePrunesRejectedArtifact(t *testing.T) {
	service, instanceStore, pool := newCanvasService(t)
	canvas := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID: "workspace-1",
		TaskID:      "task-1",
		Title:       "Reject cleanup",
	})
	active := publishTestPackage(t, service, canvas.ID, "reject-active", nil)
	pending := publishTestPackage(t, service, canvas.ID, "reject-pending", []string{"tasks"})

	if _, err := service.RejectRelease(context.Background(), canvas.ID, pending.Release.ID); err != nil {
		t.Fatalf("reject release: %v", err)
	}
	releases, err := instanceStore.ListReleases(context.Background(), canvas.PluginInstanceID)
	if err != nil {
		t.Fatalf("list releases: %v", err)
	}
	assertReleaseStatus(t, releases, active.Release.ID, plugininstances.ValidationValid, "")
	assertReleaseAbsent(t, releases, pending.Release.ID)
	if got := cleanupArtifactPaths(t, pool, canvas.PluginInstanceID); len(got) != 1 || got[0] != "releases/reject-pending" {
		t.Fatalf("cleanup paths = %v, want [releases/reject-pending]", got)
	}
}

func TestReleaseMutationsRequireRestoreAfterArchive(t *testing.T) {
	service, _, _ := newCanvasService(t)
	canvas := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID: "workspace-1", TaskID: "task-1", Title: "Archived release mutations",
	})
	prior := publishTestPackage(t, service, canvas.ID, "archived-prior", nil)
	publishTestPackage(t, service, canvas.ID, "archived-active", nil)
	pending := publishTestPackage(t, service, canvas.ID, "archived-pending", []string{"tasks"})
	if _, err := service.ArchiveCanvas(context.Background(), canvas.ID); err != nil {
		t.Fatalf("archive canvas: %v", err)
	}

	if _, err := service.ApproveRelease(context.Background(), canvas.ID, pending.Release.ID, "user-1"); !errors.Is(err, ErrInvalidCanvasState) {
		t.Fatalf("approve archived release = %v, want ErrInvalidCanvasState", err)
	}
	if _, err := service.RollbackRelease(context.Background(), canvas.ID, prior.Release.ID); !errors.Is(err, ErrInvalidCanvasState) {
		t.Fatalf("rollback archived release = %v, want ErrInvalidCanvasState", err)
	}
	if _, err := service.PromoteCanvas(context.Background(), canvas.ID, "user-1"); !errors.Is(err, ErrInvalidLifecycleState) {
		t.Fatalf("promote archived canvas = %v, want ErrInvalidLifecycleState", err)
	}
}

func TestPublishPackageRejectsAuthorityChangedAfterAuthorization(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Service, *plugininstances.Store, string, string) error
	}{
		{
			name: "archive",
			mutate: func(_ *Service, store *plugininstances.Store, _ string, instanceID string) error {
				return store.Archive(context.Background(), instanceID)
			},
		},
		{
			name: "promote",
			mutate: func(service *Service, _ *plugininstances.Store, canvasID, _ string) error {
				_, err := service.PromoteCanvas(context.Background(), canvasID, "user-1")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, instanceStore, _ := newCanvasService(t)
			canvas := createCanvas(t, service, CreateCanvasRequest{
				WorkspaceID: "workspace-1", TaskID: "task-1", Title: "Publish authority race",
			})
			if tt.name == "promote" {
				publishTestPackage(t, service, canvas.ID, "authority-base", nil)
			}
			captured, err := instanceStore.Get(context.Background(), canvas.PluginInstanceID)
			if err != nil {
				t.Fatalf("capture authority: %v", err)
			}
			barrier := &publishTransactionBarrier{
				Store:         instanceStore,
				entered:       make(chan struct{}),
				continueFirst: make(chan struct{}),
			}
			service.instances = barrier
			resultCh := make(chan error, 1)
			go func() {
				_, publishErr := service.PublishPackage(context.Background(), PublishRequest{
					CanvasID: canvas.ID, Package: testCanvasPackage("authority-race", nil),
					Artifact:          webapp.Artifact{Digest: "authority-race", RelativePath: "releases/authority-race", Bytes: 1},
					ExpectedAuthority: captured.PublishAuthority(), SourceActorKind: "agent",
				})
				resultCh <- publishErr
			}()
			<-barrier.entered

			if err := tt.mutate(service, instanceStore, canvas.ID, canvas.PluginInstanceID); err != nil {
				t.Fatalf("%s authority mutation: %v", tt.name, err)
			}
			close(barrier.continueFirst)
			if err := <-resultCh; !errors.Is(err, ErrStaleCanvasPublish) {
				t.Fatalf("publish after %s = %v, want ErrStaleCanvasPublish", tt.name, err)
			}
		})
	}
}

type publishTransactionBarrier struct {
	*plugininstances.Store
	entered       chan struct{}
	continueFirst chan struct{}
	once          sync.Once
}

type activationFailureStore struct {
	*plugininstances.Store
}

func (s *activationFailureStore) ActivateReleaseTx(context.Context, *sqlx.Tx, string, string) error {
	return errors.New("injected activation failure")
}

func (s *publishTransactionBarrier) WithTransaction(ctx context.Context, fn func(*sqlx.Tx) error) error {
	first := false
	s.once.Do(func() {
		first = true
		close(s.entered)
	})
	if first {
		<-s.continueFirst
	}
	return s.Store.WithTransaction(ctx, fn)
}

func publishTestPackage(t *testing.T, service *Service, canvasID, digest string, reads []string) *PublishResult {
	t.Helper()
	pkg := testCanvasPackage(digest, reads)
	result, err := service.PublishPackage(context.Background(), PublishRequest{
		CanvasID:        canvasID,
		Package:         pkg,
		Artifact:        webapp.Artifact{Digest: digest, RelativePath: "releases/" + digest, Bytes: 1},
		SourceActorKind: "agent",
	})
	if err != nil {
		t.Fatalf("publish %s: %v", digest, err)
	}
	return result
}

func testCanvasPackage(digest string, reads []string) *webapp.Package {
	return &webapp.Package{
		Manifest: &manifest.Manifest{
			ID:         "canvas-board",
			APIVersion: manifest.CurrentAPIVersion,
			Version:    digest,
			UI: manifest.UISection{WebApps: []manifest.WebApp{{
				Key:        "main",
				Title:      "Board",
				Entry:      "index.html",
				Placements: []string{manifest.WebAppPlacementTask},
			}}},
			Capabilities: manifest.Capabilities{APIRead: reads},
		},
		Digest: digest,
	}
}

func setAuthoringTestClock(service *Service) {
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	tick := 0
	service.clock = func() time.Time {
		tick++
		return base.Add(time.Duration(tick) * time.Second)
	}
}

func assertReleaseStatus(t *testing.T, releases []plugininstances.Release, id, status, validationError string) {
	t.Helper()
	for _, release := range releases {
		if release.ID != id {
			continue
		}
		if release.ValidationStatus != status || release.ValidationError != validationError {
			t.Fatalf("release %q = status %q, error %q; want status %q, error %q", id, release.ValidationStatus, release.ValidationError, status, validationError)
		}
		return
	}
	t.Fatalf("release %q not found in %+v", id, releases)
}

func assertReleaseAbsent(t *testing.T, releases []plugininstances.Release, id string) {
	t.Helper()
	for _, release := range releases {
		if release.ID == id {
			t.Fatalf("release %q is still retained: %+v", id, release)
		}
	}
}

func countReleaseStatus(releases []plugininstances.Release, status string) int {
	count := 0
	for _, release := range releases {
		if release.ValidationStatus == status {
			count++
		}
	}
	return count
}

func cleanupArtifactPaths(t *testing.T, pool *db.Pool, instanceID string) []string {
	t.Helper()
	rows, err := pool.Reader().Queryx(pool.Reader().Rebind(
		"SELECT artifact_path FROM plugin_artifact_cleanup_jobs WHERE instance_id = ? ORDER BY artifact_path",
	), instanceID)
	if err != nil {
		t.Fatalf("list cleanup jobs: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			t.Fatalf("scan cleanup job: %v", err)
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate cleanup jobs: %v", err)
	}
	return paths
}
