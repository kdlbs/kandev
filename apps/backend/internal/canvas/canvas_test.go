package canvas

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	plugininstances "github.com/kandev/kandev/internal/plugins/instances"
)

func openCanvasPool(t *testing.T, path string) *db.Pool {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	pool := db.NewPool(sqlx.NewDb(conn, "sqlite3"), sqlx.NewDb(conn, "sqlite3"))
	t.Cleanup(func() { _ = pool.Close() })
	return pool
}

func newCanvasService(t *testing.T) (*Service, *plugininstances.Store, *db.Pool) {
	t.Helper()
	pool := openCanvasPool(t, filepath.Join(t.TempDir(), "canvas.db"))
	instanceStore, err := plugininstances.NewStore(pool)
	if err != nil {
		t.Fatalf("new instance store: %v", err)
	}
	repo, err := NewRepository(pool)
	if err != nil {
		t.Fatalf("new canvas repository: %v", err)
	}
	return NewService(repo, instanceStore), instanceStore, pool
}

func createCanvas(t *testing.T, service *Service, request CreateCanvasRequest) Canvas {
	t.Helper()
	created, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("create canvas: %v", err)
	}
	return *created
}

func TestGetExposesPendingFirstRelease(t *testing.T) {
	service, store, _ := newCanvasService(t)
	created := createCanvas(t, service, CreateCanvasRequest{WorkspaceID: "workspace-1", TaskID: "task-1", Title: "Pending"})
	release := plugininstances.Release{
		ID: "release-pending", PluginID: "canvas-package", InstanceID: created.PluginInstanceID,
		PackageDigest: "digest", SourceKind: plugininstances.SourceLocalCanvas,
		ArtifactPath: "releases/digest", ValidationStatus: plugininstances.ValidationPendingPermission,
		ValidationError: "permission_review_required", ProtocolVersion: 1,
	}
	if err := store.CreateRelease(context.Background(), release); err != nil {
		t.Fatalf("create pending release: %v", err)
	}
	got, err := service.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get canvas: %v", err)
	}
	if got.PendingRelease == nil || got.PendingRelease.ID != release.ID {
		t.Fatalf("pending release = %+v, want %s", got.PendingRelease, release.ID)
	}
	if got.ActiveReleaseID != "" {
		t.Fatalf("active release ID = %q, want empty", got.ActiveReleaseID)
	}
}

func TestPendingReleaseLeavesRuntimeIdentityAndGenerationUntouched(t *testing.T) {
	service, store, _ := newCanvasService(t)
	created := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID: "workspace-1", TaskID: "task-1", Title: "Pending permissions",
	})
	before, err := store.Get(context.Background(), created.PluginInstanceID)
	if err != nil {
		t.Fatalf("get instance before publish: %v", err)
	}
	result := publishTestPackage(t, service, created.ID, "pending-release", []string{"tasks"})
	if result.Activated {
		t.Fatal("permission-requiring release activated without a grant")
	}
	after, err := store.Get(context.Background(), created.PluginInstanceID)
	if err != nil {
		t.Fatalf("get instance after publish: %v", err)
	}
	if after.PluginID != before.PluginID {
		t.Fatalf("plugin identity changed for pending release: %q -> %q", before.PluginID, after.PluginID)
	}
	if after.GrantGeneration != before.GrantGeneration {
		t.Fatalf("grant generation changed for pending release: %d -> %d", before.GrantGeneration, after.GrantGeneration)
	}
	if after.ActiveReleaseID != before.ActiveReleaseID {
		t.Fatalf("active release changed for pending release: %q -> %q", before.ActiveReleaseID, after.ActiveReleaseID)
	}
	if result.Release.ValidationStatus != plugininstances.ValidationPendingPermission {
		t.Fatalf("pending release status = %q", result.Release.ValidationStatus)
	}
}

func TestReconcileRepairsOrphanCanvasRowsAndInstances(t *testing.T) {
	service, store, _ := newCanvasService(t)
	ctx := context.Background()
	orphanInstanceID := "orphan-instance"
	if err := store.Create(ctx, plugininstances.Instance{
		ID: orphanInstanceID, PluginID: CanvasPluginID,
		SourceKind: plugininstances.SourceLocalCanvas, ScopeKind: plugininstances.ScopeWorkspace,
		WorkspaceID: "workspace-1", Status: plugininstances.StatusPending,
	}); err != nil {
		t.Fatalf("create orphan instance: %v", err)
	}
	metadata := CanvasMetadata{
		ID: "orphan-metadata", PluginInstanceID: "missing-instance",
		WorkspaceID: "workspace-1", Title: "Orphan metadata",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := service.repo.Create(ctx, metadata); err != nil {
		t.Fatalf("create orphan metadata: %v", err)
	}
	cleanedState := make(map[string]int)
	service.SetInstanceStateCleanup(func(_ context.Context, instanceID string) error {
		cleanedState[instanceID]++
		return nil
	})
	if err := service.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	instance, err := store.Get(ctx, orphanInstanceID)
	if err != nil {
		t.Fatalf("get reconciled instance: %v", err)
	}
	if instance.Status != plugininstances.StatusRemoved {
		t.Fatalf("orphan instance status = %q, want removed", instance.Status)
	}
	if _, err := service.repo.Get(ctx, metadata.ID); !errors.Is(err, ErrCanvasNotFound) {
		t.Fatalf("orphan metadata = %v, want ErrCanvasNotFound", err)
	}
	if cleanedState[orphanInstanceID] != 1 || cleanedState[metadata.PluginInstanceID] != 1 {
		t.Fatalf("cleaned state = %#v, want both orphan identifiers once", cleanedState)
	}
}

func TestCreateRollsBackInstanceWhenCanvasAdmissionFails(t *testing.T) {
	service, store, _ := newCanvasService(t)
	ctx := context.Background()
	for i := 0; i < MaxWorkspaceCanvases; i++ {
		if err := service.repo.Create(ctx, CanvasMetadata{
			ID: fmt.Sprintf("metadata-%d", i), PluginInstanceID: fmt.Sprintf("instance-%d", i),
			WorkspaceID: "workspace-full", Title: "Existing metadata",
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("seed metadata %d: %v", i, err)
		}
	}
	_, err := service.Create(ctx, CreateCanvasRequest{WorkspaceID: "workspace-full", Title: "Rejected"})
	if !errors.Is(err, ErrWorkspaceCanvasLimit) {
		t.Fatalf("create error = %v, want workspace limit", err)
	}
	instances, err := store.ListBySource(ctx, plugininstances.SourceLocalCanvas, false)
	if err != nil {
		t.Fatalf("list instances: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("persisted instances after rejected create = %d, want 0", len(instances))
	}
}

func TestCleanupTaskClearsPromotedCanvasOrigin(t *testing.T) {
	service, _, _ := newCanvasService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := service.repo.Create(ctx, CanvasMetadata{
		ID: "task-metadata", PluginInstanceID: "missing-task-instance",
		WorkspaceID: "workspace-1", TaskID: "task-1", OriginTaskID: "task-1",
		Title: "Task canvas", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task metadata: %v", err)
	}
	if err := service.repo.Create(ctx, CanvasMetadata{
		ID: "promoted-metadata", PluginInstanceID: "missing-promoted-instance",
		WorkspaceID: "workspace-1", OriginTaskID: "task-1",
		Title: "Promoted canvas", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("create promoted metadata: %v", err)
	}
	if err := service.CleanupTask(ctx, "task-1"); err != nil {
		t.Fatalf("cleanup task: %v", err)
	}
	promoted, err := service.repo.Get(ctx, "promoted-metadata")
	if err != nil {
		t.Fatalf("get promoted metadata: %v", err)
	}
	if promoted.OriginTaskID != "" {
		t.Fatalf("promoted origin task = %q, want empty", promoted.OriginTaskID)
	}
	if _, err := service.repo.Get(ctx, "task-metadata"); !errors.Is(err, ErrCanvasNotFound) {
		t.Fatalf("task metadata = %v, want ErrCanvasNotFound", err)
	}
}

// @covers AC-CANVASES-AGENT-WEB-APPS-005.1
func TestCanvasListsTaskAndWorkspaceScope(t *testing.T) {
	service, _, _ := newCanvasService(t)
	ctx := context.Background()
	createCanvas(t, service, CreateCanvasRequest{WorkspaceID: "workspace-1", TaskID: "task-1", Title: "Task board"})
	workspaceCanvas := createCanvas(t, service, CreateCanvasRequest{WorkspaceID: "workspace-1", Title: "Workspace board"})

	taskCanvases, err := service.ListForTask(ctx, "workspace-1", "task-1", false)
	if err != nil {
		t.Fatalf("list task canvases: %v", err)
	}
	if len(taskCanvases) != 2 {
		t.Fatalf("task canvas count = %d, want 2", len(taskCanvases))
	}
	workspaceCanvases, err := service.ListWorkspaceCanvases(ctx, "workspace-1", false)
	if err != nil {
		t.Fatalf("list workspace canvases: %v", err)
	}
	if len(workspaceCanvases) != 1 || workspaceCanvases[0].ID != workspaceCanvas.ID {
		t.Fatalf("workspace canvases = %+v, want only %s", workspaceCanvases, workspaceCanvas.ID)
	}
	wrongWorkspace, err := service.ListForTask(ctx, "workspace-2", "task-1", false)
	if err != nil {
		t.Fatalf("list foreign task workspace: %v", err)
	}
	if len(wrongWorkspace) != 0 {
		t.Fatalf("foreign workspace returned %d canvases, want 0", len(wrongWorkspace))
	}
}

func TestLifecycleEventsArePublishedAfterCommittedMutations(t *testing.T) {
	service, _, _ := newCanvasService(t)
	var events []LifecycleEvent
	var callbackErr error
	service.SetEventPublisher(func(ctx context.Context, event LifecycleEvent) {
		events = append(events, event)
		if event.Type == EventCreated {
			_, callbackErr = service.Get(ctx, event.CanvasID)
		}
	})
	created := createCanvas(t, service, CreateCanvasRequest{WorkspaceID: "workspace-1", TaskID: "task-1", Title: "Events"})
	if callbackErr != nil {
		t.Fatalf("created event observed before commit: %v", callbackErr)
	}
	if _, err := service.Archive(context.Background(), created.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := service.Restore(context.Background(), created.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if err := service.Remove(context.Background(), created.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	want := []string{EventCreated, EventArchived, EventRestored, EventRemoved}
	if len(events) != len(want) {
		t.Fatalf("event count = %d, want %d (%+v)", len(events), len(want), events)
	}
	for index, event := range events {
		if event.Type != want[index] || event.CanvasID != created.ID || event.PluginInstanceID == "" {
			t.Fatalf("event[%d] = %+v, want type %s for %s", index, event, want[index], created.ID)
		}
	}
}

// @covers AC-CANVASES-AGENT-WEB-APPS-008.1
func TestConcurrentTaskAdmissionDoesNotExceedLimit(t *testing.T) {
	service, _, _ := newCanvasService(t)
	const extraAttempts = 7
	attempts := MaxTaskCanvases + extraAttempts
	start := make(chan struct{})
	var wait sync.WaitGroup
	var mu sync.Mutex
	created := 0
	var unexpected []error
	for i := 0; i < attempts; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, err := service.Create(context.Background(), CreateCanvasRequest{
				WorkspaceID: "workspace-1",
				TaskID:      "task-1",
				Title:       "Task canvas " + strconv.Itoa(index),
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				created++
			} else if !errors.Is(err, ErrTaskCanvasLimit) {
				unexpected = append(unexpected, err)
			}
		}(i)
	}
	close(start)
	wait.Wait()
	if len(unexpected) > 0 {
		t.Fatalf("unexpected admission errors: %v", unexpected)
	}
	if created != MaxTaskCanvases {
		t.Fatalf("created = %d, want %d", created, MaxTaskCanvases)
	}
	canvases, err := service.ListTaskCanvases(context.Background(), "task-1", true)
	if err != nil {
		t.Fatalf("list task canvases: %v", err)
	}
	if len(canvases) != MaxTaskCanvases {
		t.Fatalf("persisted task canvases = %d, want %d", len(canvases), MaxTaskCanvases)
	}
}

// @covers AC-CANVASES-AGENT-WEB-APPS-008.1
func TestConcurrentWorkspaceAdmissionDoesNotExceedLimit(t *testing.T) {
	service, _, _ := newCanvasService(t)
	attempts := MaxWorkspaceCanvases + 5
	start := make(chan struct{})
	var wait sync.WaitGroup
	var mu sync.Mutex
	created := 0
	var unexpected []error
	for i := 0; i < attempts; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, err := service.Create(context.Background(), CreateCanvasRequest{
				WorkspaceID: "workspace-1",
				Title:       fmt.Sprintf("Workspace canvas %d", index),
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				created++
			} else if !errors.Is(err, ErrWorkspaceCanvasLimit) {
				unexpected = append(unexpected, err)
			}
		}(i)
	}
	close(start)
	wait.Wait()
	if len(unexpected) > 0 {
		t.Fatalf("unexpected admission errors: %v", unexpected)
	}
	if created != MaxWorkspaceCanvases {
		t.Fatalf("created = %d, want %d", created, MaxWorkspaceCanvases)
	}
	canvases, err := service.ListWorkspaceCanvases(context.Background(), "workspace-1", false)
	if err != nil {
		t.Fatalf("list workspace canvases: %v", err)
	}
	if len(canvases) != MaxWorkspaceCanvases {
		t.Fatalf("persisted workspace canvases = %d, want %d", len(canvases), MaxWorkspaceCanvases)
	}
}

// @covers AC-CANVASES-AGENT-WEB-APPS-002.1
func TestCanvasScopeAndActiveReleaseSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "canvas.db")
	pool := openCanvasPool(t, path)
	instanceStore, err := plugininstances.NewStore(pool)
	if err != nil {
		t.Fatalf("new instance store: %v", err)
	}
	repo, err := NewRepository(pool)
	if err != nil {
		t.Fatalf("new canvas repository: %v", err)
	}
	service := NewService(repo, instanceStore)
	created := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID: "workspace-1",
		TaskID:      "task-1",
		Title:       "Durable board",
	})
	if err := instanceStore.CreateRelease(context.Background(), plugininstances.Release{
		ID:               "release-1",
		PluginID:         created.PluginID,
		InstanceID:       created.PluginInstanceID,
		PackageDigest:    "digest-1",
		SourceKind:       plugininstances.SourceLocalCanvas,
		SourceActorKind:  "agent",
		ArtifactPath:     "releases/digest-1",
		ValidationStatus: plugininstances.ValidationValid,
	}); err != nil {
		t.Fatalf("create release: %v", err)
	}
	if err := instanceStore.SetActiveRelease(context.Background(), created.PluginInstanceID, "release-1"); err != nil {
		t.Fatalf("activate release: %v", err)
	}
	if err := pool.Close(); err != nil {
		t.Fatalf("close first pool: %v", err)
	}

	restartedPool := openCanvasPool(t, path)
	restartedInstances, err := plugininstances.NewStore(restartedPool)
	if err != nil {
		t.Fatalf("reopen instance store: %v", err)
	}
	restartedRepo, err := NewRepository(restartedPool)
	if err != nil {
		t.Fatalf("reopen canvas repository: %v", err)
	}
	restarted := NewService(restartedRepo, restartedInstances)
	got, err := restarted.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get after restart: %v", err)
	}
	if got.ScopeKind != plugininstances.ScopeTask || got.TaskID != "task-1" {
		t.Fatalf("scope after restart = %q/%q", got.ScopeKind, got.TaskID)
	}
	if got.ActiveReleaseID != "release-1" || got.ActiveReleaseStatus != plugininstances.ValidationValid {
		t.Fatalf("release after restart = %q/%q", got.ActiveReleaseID, got.ActiveReleaseStatus)
	}
}

// @covers AC-CANVASES-AGENT-WEB-APPS-007.3
func TestUnavailableActiveReleaseRemainsVisibleForRecovery(t *testing.T) {
	service, instanceStore, _ := newCanvasService(t)
	created := createCanvas(t, service, CreateCanvasRequest{WorkspaceID: "workspace-1", TaskID: "task-1", Title: "Missing board"})
	if err := instanceStore.CreateRelease(context.Background(), plugininstances.Release{
		ID:               "release-missing",
		PluginID:         created.PluginID,
		InstanceID:       created.PluginInstanceID,
		PackageDigest:    "digest-missing",
		SourceKind:       plugininstances.SourceLocalCanvas,
		SourceActorKind:  "agent",
		ArtifactPath:     "releases/digest-missing",
		ValidationStatus: plugininstances.ValidationValid,
	}); err != nil {
		t.Fatalf("create release: %v", err)
	}
	if err := instanceStore.SetActiveRelease(context.Background(), created.PluginInstanceID, "release-missing"); err != nil {
		t.Fatalf("activate release: %v", err)
	}
	marked, err := instanceStore.ReconcileArtifacts(context.Background(), func(string, string, int64) (plugininstances.ArtifactCheck, error) {
		return plugininstances.ArtifactCheck{Reason: "digest_mismatch"}, nil
	})
	if err != nil || marked != 1 {
		t.Fatalf("reconcile = %d, %v; want one unavailable release", marked, err)
	}

	got, err := service.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get unavailable canvas: %v", err)
	}
	if got.ActiveReleaseStatus != plugininstances.ValidationUnavailable || got.ActiveReleaseError != "digest_mismatch" {
		t.Fatalf("unavailable release = %q/%q", got.ActiveReleaseStatus, got.ActiveReleaseError)
	}
}

// @covers AC-CANVASES-AGENT-WEB-APPS-008.3
// @covers AC-CANVASES-AGENT-WEB-APPS-008.4
func TestArchivedCanvasCountsUntilRemovedAndCanThenBeRestored(t *testing.T) {
	service, _, _ := newCanvasService(t)
	ctx := context.Background()
	canvases := make([]Canvas, 0, MaxTaskCanvases)
	for i := 0; i < MaxTaskCanvases; i++ {
		canvases = append(canvases, createCanvas(t, service, CreateCanvasRequest{
			WorkspaceID: "workspace-1",
			TaskID:      "task-1",
			Title:       "Canvas " + strconv.Itoa(i),
		}))
	}
	if _, err := service.Archive(ctx, canvases[0].ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := service.Create(ctx, CreateCanvasRequest{WorkspaceID: "workspace-1", TaskID: "task-1", Title: "over limit"}); !errors.Is(err, ErrTaskCanvasLimit) {
		t.Fatalf("create after archive = %v, want task limit", err)
	}
	if err := service.Remove(ctx, canvases[1].ID); err != nil {
		t.Fatalf("remove active canvas: %v", err)
	}
	if _, err := service.Restore(ctx, canvases[0].ID); err != nil {
		t.Fatalf("restore after removal: %v", err)
	}
	got, err := service.Get(ctx, canvases[0].ID)
	if err != nil {
		t.Fatalf("get restored canvas: %v", err)
	}
	if got.Status != plugininstances.StatusActive {
		t.Fatalf("restored status = %q, want active", got.Status)
	}
}

// @covers AC-CANVASES-AGENT-WEB-APPS-002.5
// @covers AC-CANVASES-AGENT-WEB-APPS-002.6
func TestCleanupRemovesTaskCanvasesPreservesWorkspaceCanvasesAndQueuesArtifacts(t *testing.T) {
	service, instanceStore, pool := newCanvasService(t)
	ctx := context.Background()
	taskCanvas := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID: "workspace-1",
		TaskID:      "task-1",
		Title:       "Task board",
	})
	workspaceCanvas := createCanvas(t, service, CreateCanvasRequest{
		WorkspaceID:  "workspace-1",
		OriginTaskID: "task-1",
		Title:        "Promoted board",
	})
	createRelease(t, instanceStore, taskCanvas, "release-task", "releases/task")
	createRelease(t, instanceStore, workspaceCanvas, "release-workspace", "releases/workspace")

	if err := service.CleanupTask(ctx, "task-1"); err != nil {
		t.Fatalf("cleanup task: %v", err)
	}
	if _, err := service.Get(ctx, taskCanvas.ID); !errors.Is(err, ErrCanvasNotFound) {
		t.Fatalf("task canvas after cleanup = %v, want not found", err)
	}
	if _, err := service.Get(ctx, workspaceCanvas.ID); err != nil {
		t.Fatalf("promoted canvas after task cleanup: %v", err)
	}
	if got := cleanupJobCount(t, pool, taskCanvas.PluginInstanceID); got != 1 {
		t.Fatalf("task cleanup jobs = %d, want 1", got)
	}

	if err := service.CleanupWorkspace(ctx, "workspace-1"); err != nil {
		t.Fatalf("cleanup workspace: %v", err)
	}
	if _, err := service.Get(ctx, workspaceCanvas.ID); !errors.Is(err, ErrCanvasNotFound) {
		t.Fatalf("workspace canvas after cleanup = %v, want not found", err)
	}
	if got := cleanupJobCount(t, pool, workspaceCanvas.PluginInstanceID); got != 1 {
		t.Fatalf("workspace cleanup jobs = %d, want 1", got)
	}
	if got := metadataCount(t, pool, "workspace-1"); got != 0 {
		t.Fatalf("metadata rows after workspace cleanup = %d, want 0", got)
	}
}

func createRelease(t *testing.T, store *plugininstances.Store, canvas Canvas, id, path string) {
	t.Helper()
	if err := store.CreateRelease(context.Background(), plugininstances.Release{
		ID:               id,
		PluginID:         canvas.PluginID,
		InstanceID:       canvas.PluginInstanceID,
		PackageDigest:    id + "-digest",
		SourceKind:       plugininstances.SourceLocalCanvas,
		SourceActorKind:  "agent",
		ArtifactPath:     path,
		ValidationStatus: plugininstances.ValidationValid,
	}); err != nil {
		t.Fatalf("create release %s: %v", id, err)
	}
}

func cleanupJobCount(t *testing.T, pool *db.Pool, instanceID string) int {
	t.Helper()
	var count int
	err := pool.Reader().Get(&count, pool.Reader().Rebind(
		"SELECT COUNT(*) FROM plugin_artifact_cleanup_jobs WHERE instance_id = ?",
	), instanceID)
	if err != nil {
		t.Fatalf("count cleanup jobs: %v", err)
	}
	return count
}

func metadataCount(t *testing.T, pool *db.Pool, workspaceID string) int {
	t.Helper()
	var count int
	err := pool.Reader().Get(&count, pool.Reader().Rebind(
		"SELECT COUNT(*) FROM canvas_lifecycle_metadata WHERE workspace_id = ?",
	), workspaceID)
	if err != nil {
		t.Fatalf("count canvas metadata: %v", err)
	}
	return count
}
