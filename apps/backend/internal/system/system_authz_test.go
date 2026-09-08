package system

import (
	"context"
	"encoding/json"
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
	"github.com/kandev/kandev/internal/system/backups"
	"github.com/kandev/kandev/internal/system/jobs"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	"github.com/kandev/kandev/internal/system/storage"
	_ "github.com/mattn/go-sqlite3"
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
