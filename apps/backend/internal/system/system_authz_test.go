package system

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/auth/httpmw"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/system/backups"
	"github.com/kandev/kandev/internal/system/dataretention"
	"github.com/kandev/kandev/internal/system/jobs"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	"github.com/kandev/kandev/internal/system/storage"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	sqlite3driver "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

// The backups snapshot planted on disk by newBackupsAuthzFixture. Its bytes
// double as the leak canary for the download route.
const (
	authzSnapshotName    = "manual-20260101-000000.db"
	authzSnapshotContent = "every-users-private-rows"
	authzBackupsPath     = "/api/v1/system/backups"
	authzSnapshotPath    = authzBackupsPath + "/" + authzSnapshotName
	authzStoragePath     = "/api/v1/system/storage"
	authzJSONContentType = "application/json"
)

// authzJobTimeout bounds the wait for an async backup job. It only has to
// cover a VACUUM INTO of a one-table database, so a generous value costs
// nothing on the happy path and still fails loudly rather than hanging.
const authzJobTimeout = 30 * time.Second

// systemRouterForSyntheticIdentity mirrors the auth-disabled request path:
// httpmw injects the synthetic single-user admin on every request.
func systemRouterForSyntheticIdentity() *gin.Engine {
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		authn.SetOnGin(ctx, httpmw.SyntheticIdentity())
		ctx.Next()
	})
	return router
}

type backupsAuthzFixture struct {
	service    *backups.Service
	tracker    *jobs.Tracker
	backupsDir string

	// POST /backups returns a job id and does its filesystem work in a
	// tracker goroutine, so a test that returns without waiting races
	// t.TempDir()'s RemoveAll against a VACUUM INTO still writing into
	// backupsDir. The tracker publishes every transition on the event bus,
	// so awaiting the terminal one is a real barrier rather than a sleep:
	// jobs.Tracker.run publishes it only after the job function has
	// returned, which is after the last write.
	mu   sync.Mutex
	done map[string]chan struct{}
}

// jobDone returns the channel closed when jobID reaches a terminal state,
// creating it if neither side has seen the job yet. Both the subscriber and
// the waiter go through here, so a job that finishes before anyone waits is
// still observed.
func (f *backupsAuthzFixture) jobDone(jobID string) chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	channel, ok := f.done[jobID]
	if !ok {
		channel = make(chan struct{})
		f.done[jobID] = channel
	}
	return channel
}

// awaitJob blocks until the job finishes. It is the barrier that keeps the
// tracker goroutine from outliving the test and its temporary directory.
func (f *backupsAuthzFixture) awaitJob(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var body struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.JobID == "" {
		t.Fatalf("create response carried no job id: %s", response.Body.String())
	}
	select {
	case <-f.jobDone(body.JobID):
	case <-time.After(authzJobTimeout):
		t.Fatalf("backup job %s did not finish in %s; state = %+v",
			body.JobID, authzJobTimeout, f.tracker.Get(body.JobID))
	}
}

func newBackupsAuthzFixture(t *testing.T) *backupsAuthzFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dataDir := t.TempDir()
	databasePath := filepath.Join(dataDir, "kandev.db")
	connection, err := sqlx.Open("sqlite3", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	if _, err := connection.Exec(`CREATE TABLE things (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	backupsDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(backupsDir, authzSnapshotName)
	if err := os.WriteFile(snapshot, []byte(authzSnapshotContent), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := &backupsAuthzFixture{backupsDir: backupsDir, done: map[string]chan struct{}{}}
	// A real logger: MemoryEventBus.Subscribe logs unconditionally and nil-
	// derefs on a nil one, even though Publish tolerates it.
	eventBus := bus.NewMemoryEventBus(testLoggerForAuthz(t))
	subscription, err := eventBus.Subscribe(events.SystemJobUpdate, func(_ context.Context, event *bus.Event) error {
		job, ok := event.Data.(*jobs.Job)
		if !ok || (job.State != jobs.StateSucceeded && job.State != jobs.StateFailed) {
			return nil
		}
		close(fixture.jobDone(job.ID))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subscription.Unsubscribe() })
	fixture.tracker = jobs.NewTracker(eventBus, testLoggerForAuthz(t))
	fixture.service = backups.NewService(
		databasePath, db.NewPool(connection, connection), fixture.tracker, testLoggerForAuthz(t),
	)
	return fixture
}

func (f *backupsAuthzFixture) router(t *testing.T, router *gin.Engine) *gin.Engine {
	t.Helper()
	(&Service{Backups: f.service}).RegisterRoutes(router, testLoggerForAuthz(t))
	return router
}

func (f *backupsAuthzFixture) snapshotExists() bool {
	_, err := os.Stat(filepath.Join(f.backupsDir, authzSnapshotName))
	return err == nil
}

func testLoggerForAuthz(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	return log
}

// validStorageSettingsBody is a complete, in-range settings payload, so a
// non-403 response can only mean the request reached the handler.
func validStorageSettingsBody(t *testing.T) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"settings": storage.DefaultSettings()})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func newRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", authzJSONContentType)
	return request
}

func serve(router *gin.Engine, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// TestBackupsMutatingRoutesDenyMembers locks the authorization fix: with auth
// enabled a member may not export, create, restore, or delete an install-wide
// snapshot. Each case also asserts the service never ran.
func TestBackupsMutatingRoutesDenyMembers(t *testing.T) {
	fixture := newBackupsAuthzFixture(t)
	router := fixture.router(t, systemRouterForRole(authn.RoleMember))

	download := serve(router, newRequest(http.MethodGet, authzSnapshotPath+"/download", ""))
	if download.Code != http.StatusForbidden {
		t.Fatalf("member download status = %d, want 403; body=%s", download.Code, download.Body.String())
	}
	if strings.Contains(download.Body.String(), authzSnapshotContent) {
		t.Fatalf("member download leaked snapshot bytes: %s", download.Body.String())
	}

	create := serve(router, newRequest(http.MethodPost, authzBackupsPath, "{}"))
	if create.Code != http.StatusForbidden {
		t.Fatalf("member create status = %d, want 403; body=%s", create.Code, create.Body.String())
	}
	if jobsStarted := len(fixture.tracker.List()); jobsStarted != 0 {
		t.Fatalf("member create started %d jobs, want 0", jobsStarted)
	}

	restore := serve(router, newRequest(http.MethodPost, authzSnapshotPath+"/restore", `{"confirm":"RESTORE"}`))
	if restore.Code != http.StatusForbidden {
		t.Fatalf("member restore status = %d, want 403; body=%s", restore.Code, restore.Body.String())
	}
	if jobsStarted := len(fixture.tracker.List()); jobsStarted != 0 {
		t.Fatalf("member restore started %d jobs, want 0", jobsStarted)
	}

	remove := serve(router, newRequest(http.MethodDelete, authzSnapshotPath, ""))
	if remove.Code != http.StatusForbidden {
		t.Fatalf("member delete status = %d, want 403; body=%s", remove.Code, remove.Body.String())
	}
	if !fixture.snapshotExists() {
		t.Fatal("member delete removed the snapshot despite the 403")
	}
}

// TestBackupsListStaysReadableForMembers keeps the metadata-only listing on
// the non-admin group so the page still renders for members.
func TestBackupsListStaysReadableForMembers(t *testing.T) {
	fixture := newBackupsAuthzFixture(t)
	router := fixture.router(t, systemRouterForRole(authn.RoleMember))

	list := serve(router, newRequest(http.MethodGet, authzBackupsPath, ""))
	if list.Code != http.StatusOK {
		t.Fatalf("member list status = %d, want 200; body=%s", list.Code, list.Body.String())
	}
	if !strings.Contains(list.Body.String(), authzSnapshotName) {
		t.Fatalf("member list omitted the snapshot: %s", list.Body.String())
	}
}

// TestBackupsRoutesUnchangedForAdminAndSyntheticIdentity proves the guard is a
// no-op for admins and for the auth-disabled synthetic single user.
func TestBackupsRoutesUnchangedForAdminAndSyntheticIdentity(t *testing.T) {
	for name, build := range map[string]func() *gin.Engine{
		"admin":     func() *gin.Engine { return systemRouterForRole(authn.RoleAdmin) },
		"synthetic": systemRouterForSyntheticIdentity,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newBackupsAuthzFixture(t)
			router := fixture.router(t, build())

			download := serve(router, newRequest(http.MethodGet, authzSnapshotPath+"/download", ""))
			if download.Code != http.StatusOK || download.Body.String() != authzSnapshotContent {
				t.Fatalf("download status = %d body = %q", download.Code, download.Body.String())
			}

			create := serve(router, newRequest(http.MethodPost, authzBackupsPath, "{}"))
			if create.Code != http.StatusAccepted {
				t.Fatalf("create status = %d, want 202; body=%s", create.Code, create.Body.String())
			}
			// Wait here, not at the end: the job writes into the same
			// directory the assertions below read, so letting it run loose
			// would race the delete assertion as well as the cleanup.
			fixture.awaitJob(t, create)

			restore := serve(router, newRequest(http.MethodPost, authzSnapshotPath+"/restore", `{"confirm":"NOPE"}`))
			if restore.Code != http.StatusBadRequest {
				t.Fatalf("restore status = %d, want 400 (confirmation rejected, not authorization)", restore.Code)
			}

			remove := serve(router, newRequest(http.MethodDelete, authzSnapshotPath, ""))
			if remove.Code != http.StatusNoContent {
				t.Fatalf("delete status = %d, want 204; body=%s", remove.Code, remove.Body.String())
			}
			if fixture.snapshotExists() {
				t.Fatal("delete returned 204 but left the snapshot on disk")
			}
			if started := len(fixture.tracker.List()); started == 0 {
				t.Fatal("create never reached the service")
			}
		})
	}
}

type storageAuthzCase struct {
	name   string
	method string
	path   string
	body   string
}

// storageMutatingRoutes is every storage endpoint that changes install-wide
// state. Bodies carry valid confirmations so a missing guard produces a
// non-403 status rather than an incidental 400.
func storageMutatingRoutes() []storageAuthzCase {
	return []storageAuthzCase{
		{name: "adopt go cache", method: http.MethodPost, path: authzStoragePath + "/go-cache/adopt", body: `{"path":"/tmp/cache","confirm":"ADOPT"}`},
		{name: "analyze", method: http.MethodPost, path: authzStoragePath + "/analyze", body: "{}"},
		{name: "run cleanup", method: http.MethodPost, path: authzStoragePath + "/run", body: "{}"},
		{name: "restore quarantine entry", method: http.MethodPost, path: authzStoragePath + "/quarantine/entry-1/restore", body: "{}"},
		{name: "delete quarantine entry", method: http.MethodDelete, path: authzStoragePath + "/quarantine/entry-1", body: `{"confirm":"DELETE"}`},
		{name: "purge quarantine", method: http.MethodDelete, path: authzStoragePath + "/quarantine", body: `{"scope":"eligible","confirm":"DELETE ELIGIBLE"}`},
	}
}

func storageReadRoutes() []string {
	return []string{
		authzStoragePath,
		authzStoragePath + "/disk",
		authzStoragePath + "/settings",
		authzStoragePath + "/runs",
		authzStoragePath + "/quarantine",
	}
}

// recordingStorageMutations records which destructive operation ran so a
// blocked request can be distinguished from one that merely returned an error.
type recordingStorageMutations struct {
	calls []string
}

func (r *recordingStorageMutations) AdoptGoCache(context.Context, string, string) (storage.StorageMaintenanceSettings, storage.Capabilities, error) {
	r.calls = append(r.calls, "adopt")
	return storage.DefaultSettings(), storage.Capabilities{}, nil
}

func (r *recordingStorageMutations) Analyze(context.Context) (string, error) {
	r.calls = append(r.calls, "analyze")
	return "analysis-job", nil
}

func (r *recordingStorageMutations) RunNow(context.Context, []string, bool) (string, error) {
	r.calls = append(r.calls, "run")
	return "cleanup-job", nil
}

func (r *recordingStorageMutations) RestoreQuarantine(context.Context, string) (storage.QuarantineEntry, error) {
	r.calls = append(r.calls, "restore")
	return storage.QuarantineEntry{}, nil
}

func (r *recordingStorageMutations) DeleteQuarantine(context.Context, string, string) (string, error) {
	r.calls = append(r.calls, "delete")
	return "delete-job", nil
}

func (r *recordingStorageMutations) PurgeQuarantine(context.Context, storage.QuarantinePurgeScope, string) (string, error) {
	r.calls = append(r.calls, "purge")
	return "purge-job", nil
}

func newStorageAuthzRouter(t *testing.T, router *gin.Engine, mutations storage.Mutations) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	connection, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	pool := db.NewPool(connection, connection)
	rawSettings, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	storageStore, err := storage.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{Storage: storage.NewHandler(storage.HandlerConfig{
		Settings: storage.NewSettingsStore(rawSettings), Runs: storageStore,
		Quarantine: storageStore, Overview: emptyStorageOverview{}, Mutations: mutations,
	})}
	service.RegisterRoutes(router, testLoggerForAuthz(t))
	return router
}

// TestStorageMutatingRoutesDenyMembers locks the second half of the fix:
// members cannot run install-wide maintenance, and the operation never fires.
func TestStorageMutatingRoutesDenyMembers(t *testing.T) {
	for _, testCase := range storageMutatingRoutes() {
		t.Run(testCase.name, func(t *testing.T) {
			mutations := &recordingStorageMutations{}
			router := newStorageAuthzRouter(t, systemRouterForRole(authn.RoleMember), mutations)

			response := serve(router, newRequest(testCase.method, testCase.path, testCase.body))
			if response.Code != http.StatusForbidden {
				t.Fatalf("member status = %d, want 403; body=%s", response.Code, response.Body.String())
			}
			if len(mutations.calls) != 0 {
				t.Fatalf("member request invoked %v despite the 403", mutations.calls)
			}
		})
	}
}

// TestStorageSettingsPatchDeniesMembers covers the settings write, which goes
// through the settings store rather than Mutations.
func TestStorageSettingsPatchDeniesMembers(t *testing.T) {
	router := newStorageAuthzRouter(t, systemRouterForRole(authn.RoleMember), &recordingStorageMutations{})

	response := serve(router, newRequest(http.MethodPatch, authzStoragePath+"/settings", validStorageSettingsBody(t)))
	if response.Code != http.StatusForbidden {
		t.Fatalf("member PATCH status = %d, want 403; body=%s", response.Code, response.Body.String())
	}
}

// TestStorageReadRoutesStayOpenToMembers keeps the read-only maintenance view
// on the non-admin group.
func TestStorageReadRoutesStayOpenToMembers(t *testing.T) {
	router := newStorageAuthzRouter(t, systemRouterForRole(authn.RoleMember), &recordingStorageMutations{})
	for _, path := range storageReadRoutes() {
		t.Run(path, func(t *testing.T) {
			response := serve(router, newRequest(http.MethodGet, path, ""))
			if response.Code != http.StatusOK {
				t.Fatalf("member GET status = %d, want 200; body=%s", response.Code, response.Body.String())
			}
		})
	}
}

// TestStorageMutatingRoutesUnchangedForAdminAndSyntheticIdentity proves the
// guard changes nothing for admins or for the auth-disabled single user.
func TestStorageMutatingRoutesUnchangedForAdminAndSyntheticIdentity(t *testing.T) {
	for name, build := range map[string]func() *gin.Engine{
		"admin":     func() *gin.Engine { return systemRouterForRole(authn.RoleAdmin) },
		"synthetic": systemRouterForSyntheticIdentity,
	} {
		t.Run(name, func(t *testing.T) {
			for _, testCase := range storageMutatingRoutes() {
				t.Run(testCase.name, func(t *testing.T) {
					mutations := &recordingStorageMutations{}
					router := newStorageAuthzRouter(t, build(), mutations)

					response := serve(router, newRequest(testCase.method, testCase.path, testCase.body))
					if response.Code == http.StatusForbidden || response.Code == http.StatusUnauthorized {
						t.Fatalf("status = %d, want the route to run; body=%s", response.Code, response.Body.String())
					}
					if len(mutations.calls) != 1 {
						t.Fatalf("calls = %v, want exactly one", mutations.calls)
					}
				})
			}
			patch := serve(
				newStorageAuthzRouter(t, build(), &recordingStorageMutations{}),
				newRequest(http.MethodPatch, authzStoragePath+"/settings", validStorageSettingsBody(t)),
			)
			if patch.Code == http.StatusForbidden || patch.Code == http.StatusUnauthorized {
				t.Fatalf("PATCH settings status = %d, want the route to run; body=%s", patch.Code, patch.Body.String())
			}
		})
	}
}

// TestBackupsListLeaksOnlyMetadata keeps GET /backups on the non-admin group
// honest. The listing is the one backups route a member may call, so it must
// carry snapshot metadata and nothing else. The absolute on-disk path is not
// metadata: it discloses the install's data directory (usually including the
// operating account's home directory) to every authenticated user, and the UI
// deliberately shows only a "<data-dir>/backups/" placeholder instead.
func TestBackupsListLeaksOnlyMetadata(t *testing.T) {
	fixture := newBackupsAuthzFixture(t)
	router := fixture.router(t, systemRouterForRole(authn.RoleMember))

	list := serve(router, newRequest(http.MethodGet, authzBackupsPath, ""))
	if list.Code != http.StatusOK {
		t.Fatalf("member list status = %d, want 200; body=%s", list.Code, list.Body.String())
	}
	var payload struct {
		Snapshots []map[string]any `json:"snapshots"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Snapshots) != 1 {
		t.Fatalf("snapshots = %v, want exactly the seeded one", payload.Snapshots)
	}
	fields := make([]string, 0, len(payload.Snapshots[0]))
	for field := range payload.Snapshots[0] {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	want := []string{"kind", "mtime", "name", "size_bytes"}
	if !slices.Equal(fields, want) {
		t.Fatalf("snapshot fields = %v, want %v", fields, want)
	}
	if strings.Contains(list.Body.String(), fixture.backupsDir) {
		t.Fatalf("listing disclosed the backups directory: %s", list.Body.String())
	}
}

// TestArchivedDataAnalyzeRejectsWhilePreparationPending locks the O3 fix: an
// analysis must not start while a preparation owns the operation slot, or its
// caller-supplied operation id is overwritten by the pending preparation. The
// approval gate must also keep refusing a cleanup pass.
func TestArchivedDataAnalyzeRejectsWhilePreparationPending(t *testing.T) {
	gin.SetMode(gin.TestMode)
	connection, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	// The settings row and the three scan cursors startOperation reads, so a
	// permitted analysis can actually start and the busy refusal is the only
	// reason it is stopped.
	for _, ddl := range []string{
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE tasks (id TEXT PRIMARY KEY, state TEXT NOT NULL DEFAULT '', archived_at TEXT)`, messagesDDL,
		`CREATE TABLE task_resource_cleanup_jobs (id TEXT PRIMARY KEY, state TEXT NOT NULL DEFAULT '', completed_at TEXT)`,
	} {
		if _, err := connection.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	service := dataretention.New(db.NewPool(connection, connection))
	ctx := context.Background()

	// Enabling with a backup choice arms a pending preparation and leaves the
	// policy unapproved.
	saved, err := service.Save(ctx, dataretention.Update{
		Enabled:      true,
		ArchivedAge:  dataretention.Age{Days: 14},
		CleanupAge:   dataretention.Age{Days: 7},
		BackupChoice: "backup",
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Preparation.State != "pending" {
		t.Fatalf("preparation state = %q, want pending", saved.Preparation.State)
	}

	// An analysis is refused while the preparation is pending, and starts no
	// operation that could displace it. The exact API classification (respond
	// maps err.Error() to a code), not just any non-nil error, so an unrelated
	// failure cannot pass this case.
	if _, err := service.Analyze(ctx, dataretention.Update{
		ArchivedAge: dataretention.Age{Days: 14},
		CleanupAge:  dataretention.Age{Days: 7},
	}); err == nil || err.Error() != "busy" {
		t.Fatalf("analyze error = %v, want busy", err)
	}
	after, err := service.Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after.Preparation.State != "pending" || after.Operation != nil {
		t.Fatalf("state = %q operation = %#v; want a preserved pending preparation and no operation",
			after.Preparation.State, after.Operation)
	}

	// The approval gate is untouched: the policy still cannot run a pass, and it
	// fails with the API's preparation_required classification rather than any
	// unrelated error.
	if _, err := service.Run(ctx, after.Policy.Revision); err == nil || err.Error() != "preparation_required" {
		t.Fatalf("run error = %v, want preparation_required", err)
	}
}

// messagesDDL is the message table startOperation reads for its pass cursor.
const messagesDDL = `CREATE TABLE task_session_messages (id TEXT PRIMARY KEY, task_session_id TEXT, metadata TEXT)`

// newRetentionSafetyService builds a dataretention.Service over a temp SQLite
// pool with the minimal schema needed to drive a preparation and a cleanup-job
// pass, together with the fake backup callbacks a case supplies.
func newRetentionSafetyService(t *testing.T, opts dataretention.Options) (*dataretention.Service, *sqlx.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	connection, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	for _, ddl := range []string{
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE tasks (id TEXT PRIMARY KEY, state TEXT NOT NULL DEFAULT '', archived_at TEXT, queued_for_step_id TEXT, queued_at TEXT, created_at TEXT, updated_at TEXT)`, messagesDDL,
		`CREATE TABLE task_resource_cleanup_jobs (id TEXT PRIMARY KEY, task_id TEXT NOT NULL DEFAULT '', state TEXT NOT NULL DEFAULT '', completed_at TEXT, resource_snapshot TEXT, workspace_protection TEXT NOT NULL DEFAULT 'unclassified')`,
	} {
		if _, err := connection.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	return dataretention.New(db.NewPool(connection, connection), opts), connection
}

func waitRetentionState(t *testing.T, service *dataretention.Service, want func(dataretention.Status) bool) dataretention.Status {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		status, err := service.Get(context.Background())
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if want(status) {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("state did not settle in time: %#v", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func saveBackupPolicy(t *testing.T, service *dataretention.Service) {
	t.Helper()
	if _, err := service.Save(context.Background(), dataretention.Update{
		Enabled:      true,
		ArchivedAge:  dataretention.Age{Days: 14},
		CleanupAge:   dataretention.Age{Days: 7},
		BackupChoice: "backup",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
}

// TestArchivedRetentionBackupFailureKeepsPolicyUnapproved verifies the deletion
// protection the safety threads require: a failed backup must leave the policy
// unapproved (Run refuses with preparation_required) and preserve the row.
func TestArchivedRetentionBackupFailureKeepsPolicyUnapproved(t *testing.T) {
	service, connection := newRetentionSafetyService(t, dataretention.Options{
		CreateBackup: func(context.Context) (string, error) { return "", errors.New("backup boom") },
		VerifyBackup: func(context.Context, string) error { return nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	service.Start(ctx)
	saveBackupPolicy(t, service)

	status := waitRetentionState(t, service, func(s dataretention.Status) bool { return s.Preparation.State == "failed" })
	if status.Policy.Enabled {
		t.Fatal("a failed backup left the policy enabled")
	}
	if status.Policy.ArchivedAge.Days != 14 || status.Policy.CleanupAge.Days != 7 {
		t.Fatalf("policy changed by a failed backup: %#v", status.Policy)
	}
	if _, err := service.Run(ctx, status.Policy.Revision); err == nil || err.Error() != "preparation_required" {
		t.Fatalf("run error = %v, want preparation_required", err)
	}
	var raw string
	if err := connection.QueryRow(`SELECT value FROM settings WHERE key = 'archived_data_retention'`).Scan(&raw); err != nil {
		t.Fatalf("settings row missing after a failed backup: %v", err)
	}
	if raw == "" {
		t.Fatal("settings row emptied by a failed backup")
	}
}

// TestArchivedRetentionCancelDuringBackupKeepsPolicyUnapproved verifies that
// cancelling an in-flight backup leaves no approval, no progress, and does not
// replace the operation identity.
func TestArchivedRetentionCancelDuringBackupKeepsPolicyUnapproved(t *testing.T) {
	release := make(chan struct{})
	var closeOnce sync.Once
	service, _ := newRetentionSafetyService(t, dataretention.Options{
		CreateBackup: func(ctx context.Context) (string, error) {
			select {
			case <-release:
				return "receipt", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
		VerifyBackup: func(context.Context, string) error { return nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() { closeOnce.Do(func() { close(release) }) })
	service.Start(ctx)
	saveBackupPolicy(t, service)

	running := waitRetentionState(t, service, func(s dataretention.Status) bool {
		return s.Preparation.State == "running" && s.Operation != nil
	})
	opID := running.Operation.ID
	if _, err := service.Cancel(ctx, opID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	after := waitRetentionState(t, service, func(s dataretention.Status) bool { return s.Preparation.State != "running" })
	if after.Policy.Enabled {
		t.Fatal("cancel left the policy enabled")
	}
	if after.Operation == nil {
		t.Fatal("the cancelled operation was dropped, so its identity is unobservable")
	}
	if after.Operation.ID != opID {
		t.Fatalf("operation id was replaced: %s -> %s", opID, after.Operation.ID)
	}
}

// TestArchivedRetentionCleanupPreservesEvidenceAndProtectedRows verifies that a
// cleanup pass keeps the archive source evidence byte-for-byte, drops the
// operational field, writes the marker, and leaves a protected job untouched.
func TestArchivedRetentionCleanupPreservesEvidenceAndProtectedRows(t *testing.T) {
	service, connection := newRetentionSafetyService(t, dataretention.Options{
		CreateBackup: func(context.Context) (string, error) { return "receipt", nil },
		VerifyBackup: func(context.Context, string) error { return nil },
	})
	const old = "2000-01-01T00:00:00Z"
	operational := strings.Repeat("x", 4096)
	eligible := `{"archive_source_manifest":[{"manifest":1}],"workspace_id":"ws-1","worktrees":[{"id":"w1"}],"progress":"` + operational + `"}`
	protected := `{"archive_source_manifest":[{"manifest":2}],"workspace_id":"ws-2","worktrees":[{"id":"w2"}],"progress":"running"}`
	if _, err := connection.Exec(
		`INSERT INTO task_resource_cleanup_jobs(id,task_id,state,completed_at,resource_snapshot,workspace_protection) VALUES('job-eligible','task-1','succeeded',?,?,'')`, old, eligible,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(
		`INSERT INTO task_resource_cleanup_jobs(id,task_id,state,completed_at,resource_snapshot,workspace_protection) VALUES('job-protected','task-2','running',?,?,'')`, old, protected,
	); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	service.Start(ctx)
	saveBackupPolicy(t, service)
	waitRetentionState(t, service, func(s dataretention.Status) bool { return s.LastRun != nil && s.LastRun.State != "running" })

	var reduced string
	if err := connection.QueryRow(`SELECT resource_snapshot FROM task_resource_cleanup_jobs WHERE id = 'job-eligible'`).Scan(&reduced); err != nil {
		t.Fatal(err)
	}
	var reducedFields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(reduced), &reducedFields); err != nil {
		t.Fatalf("reduced snapshot is not JSON: %v (%s)", err, reduced)
	}
	for key, want := range map[string]string{
		"archive_source_manifest": `[{"manifest":1}]`,
		"workspace_id":            `"ws-1"`,
		"worktrees":               `[{"id":"w1"}]`,
	} {
		if got := string(reducedFields[key]); got != want {
			t.Fatalf("evidence field %s = %s, want %s byte-for-byte", key, got, want)
		}
	}
	if _, ok := reducedFields["progress"]; ok {
		t.Fatalf("operational field was not dropped: %s", reduced)
	}
	marker, ok := reducedFields["snapshot_reduced"]
	if !ok {
		t.Fatalf("reduction marker missing: %s", reduced)
	}
	if !strings.Contains(string(marker), `"removed_fields":["progress"]`) {
		t.Fatalf("dropped field not recorded in the marker: %s", marker)
	}
	var kept string
	if err := connection.QueryRow(`SELECT resource_snapshot FROM task_resource_cleanup_jobs WHERE id = 'job-protected'`).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != protected {
		t.Fatalf("protected job changed: %s", kept)
	}
}

// TestArchivedRetentionBatchFailureRollsBack verifies that a failed reduction
// write rolls back. A Go function registered on the test connection is called by
// the rejecting trigger, so the case proves the write was actually attempted
// rather than merely that it never ran.
func TestArchivedRetentionBatchFailureRollsBack(t *testing.T) {
	service, connection := newRetentionSafetyService(t, dataretention.Options{
		CreateBackup: func(context.Context) (string, error) { return "receipt", nil },
		VerifyBackup: func(context.Context, string) error { return nil },
	})
	const old = "2000-01-01T00:00:00Z"
	original := `{"archive_source_manifest":[{"manifest":1}],"workspace_id":"ws-1","worktrees":[{"id":"w1"}],"progress":"` + strings.Repeat("x", 4096) + `"}`
	if _, err := connection.Exec(
		`INSERT INTO task_resource_cleanup_jobs(id,task_id,state,completed_at,resource_snapshot,workspace_protection) VALUES('job-eligible','task-1','succeeded',?,?,'')`, old, original,
	); err != nil {
		t.Fatal(err)
	}
	// Register a driver function the trigger calls, so the case observes the
	// attempt. This is test-connection only; no production instrumentation.
	attempted := make(chan struct{}, 8)
	reserved, err := connection.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := reserved.Raw(func(driverConn any) error {
		sqliteConn, ok := driverConn.(*sqlite3driver.SQLiteConn)
		if !ok {
			return errors.New("unexpected driver connection")
		}
		return sqliteConn.RegisterFunc("retention_attempted", func() int {
			select {
			case attempted <- struct{}{}:
			default:
			}
			return 1
		}, false)
	}); err != nil {
		t.Fatal(err)
	}
	if err := reserved.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(
		`CREATE TRIGGER reject_reduction BEFORE UPDATE ON task_resource_cleanup_jobs BEGIN SELECT retention_attempted(); SELECT RAISE(ABORT, 'no'); END`,
	); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	service.Start(ctx)
	saveBackupPolicy(t, service)
	// Wait for the write attempt itself, not a fixed sleep.
	select {
	case <-attempted:
	case <-time.After(20 * time.Second):
		t.Fatal("the reducing write was never attempted")
	}
	var kept string
	if err := connection.QueryRow(`SELECT resource_snapshot FROM task_resource_cleanup_jobs WHERE id = 'job-eligible'`).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != original {
		t.Fatalf("failed batch did not roll back: %s", kept)
	}
}

// newRetentionMessageService builds a dataretention.Service over the real task
// schema (the same bootstrap internal/task/repository/sqlite's integration
// fixture uses), so the message-metadata eligibility path is reachable.
func newRetentionMessageService(t *testing.T, opts dataretention.Options) (*dataretention.Service, *sqlx.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "retention-message.db"))
	if err != nil {
		t.Fatal(err)
	}
	d := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = d.Close() })
	if _, err := tasksqlite.NewWithDB(d, d, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := officesqlite.NewWithDB(d, d, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := messagequeue.NewSQLiteRepository(d, d); err != nil {
		t.Fatal(err)
	}
	pool := db.NewPool(d, d)
	if _, err := systemsettings.NewStore(pool); err != nil {
		t.Fatal(err)
	}
	return dataretention.New(pool, opts), d
}

// TestArchivedRetentionMessageTargetProtectsLiveAndUnknown verifies the message
// target against the real schema. The eligible finished session holds two ordered
// messages whose metadata is reduced, while a task with a live session (finished
// but not quiescent) and a task with an unsupported message type are retained.
// Message identity, type, content, timestamps, and ordering (rowid order and the
// sorted id list) are compared before and after the pass, because only metadata
// may shrink.
func TestArchivedRetentionMessageTargetProtectsLiveAndUnknown(t *testing.T) {
	service, d := newRetentionMessageService(t, dataretention.Options{
		CreateBackup: func(context.Context) (string, error) { return "receipt", nil },
		VerifyBackup: func(context.Context, string) error { return nil },
	})
	reducible := `{"tool_call_id":"id","normalized":{"kind":"shell_exec","shell_exec":{"output":{"stdout":"` + strings.Repeat("payload", 400) + `"}}},"result":"` + strings.Repeat("secret", 400) + `"}`
	seed := `INSERT INTO tasks(id,workspace_id,title,state,created_at,updated_at) VALUES('agg','','r','COMPLETED','2020-01-01','2020-01-01'),('live','','r','COMPLETED','2020-01-01','2020-01-01'),('unknown','','r','COMPLETED','2020-01-01','2020-01-01'); INSERT INTO task_sessions(id,task_id,state,started_at,completed_at,updated_at) VALUES('s-agg','agg','COMPLETED','2020-01-01','2020-01-01','2020-01-01'),('s-live','live','RUNNING','2020-01-01',NULL,'2020-01-01'),('s-unknown','unknown','COMPLETED','2020-01-01','2020-01-01','2020-01-01'); INSERT INTO task_session_turns(id,task_id,task_session_id,started_at,completed_at,created_at,updated_at) VALUES('t-agg','agg','s-agg','2020-01-01','2020-01-01','2020-01-01','2020-01-01'),('t-live','live','s-live','2020-01-01','2020-01-01','2020-01-01','2020-01-01'),('t-unknown','unknown','s-unknown','2020-01-01','2020-01-01','2020-01-01','2020-01-01');`
	if _, err := d.Exec(seed); err != nil {
		t.Fatal(err)
	}
	for _, m := range []struct{ id, sess, task, turn, typ, meta, made string }{
		{"m-agg-1", "s-agg", "agg", "t-agg", "tool_execute", reducible, "2020-01-01"},
		{"m-agg-2", "s-agg", "agg", "t-agg", "tool_execute", reducible, "2020-01-02"},
		{"m-live", "s-live", "live", "t-live", "tool_execute", reducible, "2020-01-01"},
		{"m-unknown", "s-unknown", "unknown", "t-unknown", "tool_unknown", "{}", "2020-01-01"},
	} {
		if _, err := d.Exec(`INSERT INTO task_session_messages(id,task_session_id,task_id,turn_id,author_type,content,type,metadata,created_at,updated_at) VALUES(?,?,?,?,'agent','',?,?,?,?)`, m.id, m.sess, m.task, m.turn, m.typ, m.meta, m.made, m.made); err != nil {
			t.Fatal(err)
		}
	}

	readOrder := func() []string {
		t.Helper()
		var rows []string
		if err := d.Select(&rows, `SELECT quote(rowid)||','||quote(id)||','||quote(task_id)||','||quote(task_session_id)||','||quote(turn_id)||','||quote(type)||','||quote(content)||','||quote(created_at)||','||quote(updated_at) FROM task_session_messages WHERE task_session_id = 's-agg' ORDER BY rowid`); err != nil {
			t.Fatal(err)
		}
		return rows
	}
	before := readOrder()
	if len(before) != 2 {
		t.Fatalf("seed produced %d eligible messages, want 2", len(before))
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	service.Start(ctx)
	saveBackupPolicy(t, service)
	waitRetentionState(t, service, func(s dataretention.Status) bool { return s.LastRun != nil && s.LastRun.State != "running" })

	// Only metadata may shrink: rows, identity, type, content, timestamps, and
	// rowid ordering (the sorted id list) must be identical before and after.
	after := readOrder()
	if len(after) != len(before) {
		t.Fatalf("message rows changed: %d -> %d", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("message identity/order changed at %d: %q -> %q", i, before[i], after[i])
		}
	}

	metadata := func(id string) string {
		t.Helper()
		var raw string
		if err := d.QueryRow(`SELECT COALESCE(metadata,'') FROM task_session_messages WHERE id = ?`, id).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	first, second := metadata("m-agg-1"), metadata("m-agg-2")
	var marker map[string]any
	if first != second || first == reducible || json.Unmarshal([]byte(first), &marker) != nil || strings.Contains(first, strings.Repeat("secret", 400)) {
		t.Fatalf("eligible messages were not reduced consistently: %q / %q", first, second)
	}
	if _, ok := marker["payload_retention"]; !ok {
		t.Fatalf("reduced metadata %s lacks the removal marker", first)
	}
	if got := metadata("m-live"); got != reducible {
		t.Fatalf("live task message changed: %s", got)
	}
	if got := metadata("m-unknown"); got != `{}` {
		t.Fatalf("unsupported-type message changed: %s", got)
	}
}

// TestArchivedRetentionSecondServiceSeesSavedState verifies that a second Service
// over the same pool reads the saved policy and preparation rather than its own
// defaults, which is the precondition for restart recovery. This case asserts only
// that precondition; the actual recovery tick was observed separately with a
// one-shot test that waited the real first one-minute idle tick (recovery after
// about one minute), so no slow wait is kept here.
func TestArchivedRetentionSecondServiceSeesSavedState(t *testing.T) {
	first, connection := newRetentionSafetyService(t, dataretention.Options{
		CreateBackup: func(context.Context) (string, error) { return "receipt", nil },
		VerifyBackup: func(context.Context, string) error { return nil },
	})
	if _, err := first.Save(context.Background(), dataretention.Update{
		Enabled:      true,
		ArchivedAge:  dataretention.Age{Days: 21},
		CleanupAge:   dataretention.Age{Days: 9},
		BackupChoice: "backup",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	second := dataretention.New(db.NewPool(connection, connection), dataretention.Options{})
	status, err := second.Get(context.Background())
	if err != nil {
		t.Fatalf("second get: %v", err)
	}
	if status.Policy.ArchivedAge.Days != 21 || status.Policy.CleanupAge.Days != 9 {
		t.Fatalf("second service did not read the saved policy: %#v", status.Policy)
	}
	if status.Preparation.State != "pending" {
		t.Fatalf("second service did not read the saved preparation: %q", status.Preparation.State)
	}
}
