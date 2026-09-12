package backendapp

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/startup"
	"github.com/kandev/kandev/internal/system/backups"
	"github.com/kandev/kandev/internal/system/jobs"
)

// @covers AC-PLATFORM-STARTUP-LIFECYCLE-001.1, .5
func TestRunBindsBeforeDatabaseFailure(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	cfg := &config.Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = freePort(t)
	cfg.HomeDir = t.TempDir()
	cfg.Database.Path = filepath.Join(cfg.HomeDir, "data", "kandev.db")
	if err := os.MkdirAll(cfg.Database.Path, 0700); err != nil {
		t.Fatal(err)
	} // Directory, not a database.
	var cleanups []func() error
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			if cleanups[i] != nil {
				_ = cleanups[i]()
			}
		}
		cleanups = nil
	}
	defer cleanup()
	if run(cfg, log, &cleanups, cleanup) {
		t.Fatal("database failure reported success")
	}
	if logs.FilterMessage("HTTP listener bound; serving liveness probe while startup continues").Len() != 1 {
		t.Fatal("database initialization ran before the liveness listener bound")
	}
	if _, _, err := getJSON("127.0.0.1", cfg.Server.Port, "/health"); err == nil {
		t.Fatal("failed initializer left healthy listener")
	}
}

func TestBootstrapReportsStartupPhase(t *testing.T) {
	w := httptest.NewRecorder()
	newBootstrapHandler("test").ServeHTTP(w, httptest.NewRequest("GET", "/ready", nil))
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	startupBody, ok := body["startup"].(map[string]any)
	if !ok {
		t.Fatalf("startup body = %#v, want an object", body["startup"])
	}
	if startupBody["phase"] != string(startup.OpeningDatabase) {
		t.Fatalf("startup phase = %v, want %q", startupBody["phase"], startup.OpeningDatabase)
	}
	for _, field := range []string{"elapsed_ms", "phase_elapsed_ms"} {
		value, ok := startupBody[field].(float64)
		if !ok || value < 0 {
			t.Fatalf("startup %s = %v, want a non-negative number", field, startupBody[field])
		}
	}
}

// @covers AC-PLATFORM-STARTUP-LIFECYCLE-001.2, .6
func TestBootstrapCancellationClosesListenerBeforeInitializerReturns(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	done := make(chan bool, 1)
	defer releaseOnce.Do(func() { close(release) })
	go func() {
		done <- runWithBootstrap(ctx, cfg, testLogger(t), func(ctx context.Context) bool {
			close(entered)
			<-release
			return true
		})
	}()
	<-entered
	for _, path := range []string{"/ready", "/api/v1/tasks"} {
		status, _, err := getJSON("127.0.0.1", cfg.Server.Port, path)
		if err != nil || status != http.StatusServiceUnavailable {
			t.Fatalf("%s: %d %v", path, status, err)
		}
	}
	cancel()
	deadline := time.After(2 * time.Second)
	for {
		_, _, err := getJSON("127.0.0.1", cfg.Server.Port, "/health")
		if err != nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("canceled initialization left listener healthy")
		default:
		}
	}
	// The wrapper must join initialization instead of racing resource cleanup.
	select {
	case <-done:
		t.Fatal("returned before initializer drained")
	default:
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("initializer did not drain after release")
	}
}

// @covers AC-PLATFORM-STARTUP-LIFECYCLE-001.1, .2, .3, .6
func TestBootstrapDelayedSuccessKeepsLivenessUntilReadiness(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = freePort(t)
	cfg.Launcher.HealthTimeoutMs = 100
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	entered := make(chan struct{})
	release := make(chan struct{})
	published := make(chan struct{})
	done := make(chan bool, 1)
	var releaseOnce sync.Once
	var publishOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })

	go func() {
		done <- runWithBootstrap(ctx, cfg, testLogger(t), func(ctx context.Context) bool {
			runtime := ctx.Value(bootstrapContextKey{}).(*bootstrapRuntime)
			close(entered)
			<-release
			application := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					writeBootstrapJSON(w, http.StatusOK, map[string]any{
						statusKey:       "ok",
						serviceFieldKey: kandevName,
						"mode":          "websocket+http",
						versionFieldKey: Version,
					})
					return
				}
				if r.URL.Path == "/ready" || r.URL.Path == "/api/v1/tasks" {
					writeBootstrapJSON(w, http.StatusOK, map[string]any{statusKey: "ok"})
					return
				}
				writeBootstrapJSON(w, http.StatusNotFound, map[string]any{statusKey: "not_found"})
			})
			publishReadiness(func() {
				ready.Store(true)
				runtime.ready.Store(true)
				publishOnce.Do(func() { close(published) })
			}, func() { runtime.handler.Store(application) })
			<-processContextFromContext(ctx).Done()
			return true
		})
	}()

	<-entered
	// Keep the bootstrap initializer behind the same short health budget the
	// launcher uses for its keep-or-kill decision. Readiness has no matching
	// deadline: it must keep polling while recovery is still running.
	healthDeadline := time.Now().Add(time.Duration(cfg.Launcher.HealthTimeoutMs) * time.Millisecond)
	for time.Now().Before(healthDeadline) {
		status, body, err := getJSON("127.0.0.1", cfg.Server.Port, "/health")
		if err != nil || status != http.StatusOK {
			t.Fatalf("bootstrap /health during delayed initialization = %d, %v; want 200", status, err)
		}
		if body[statusKey] != "ok" || body[serviceFieldKey] != kandevName || body[versionFieldKey] != Version {
			t.Fatalf("bootstrap /health identity = %#v, want status=ok service=%q version=%q", body, kandevName, Version)
		}
		status, _, err = getJSON("127.0.0.1", cfg.Server.Port, "/ready")
		if err != nil || status != http.StatusServiceUnavailable {
			t.Fatalf("bootstrap /ready during delayed initialization = %d, %v; want 503", status, err)
		}
		status, _, err = getJSON("127.0.0.1", cfg.Server.Port, "/api/v1/tasks")
		if err != nil || status != http.StatusServiceUnavailable {
			t.Fatalf("bootstrap application route during delayed initialization = %d, %v; want 503", status, err)
		}
	}
	select {
	case result := <-done:
		t.Fatalf("bootstrap exited before readiness handoff: %v", result)
	default:
	}
	select {
	case <-published:
		t.Fatal("readiness published before initialization gate released")
	default:
	}

	readyDone := make(chan error, 1)
	go func() {
		for {
			status, _, err := getJSON("127.0.0.1", cfg.Server.Port, "/ready")
			if err != nil {
				readyDone <- err
				return
			}
			if status == http.StatusOK {
				readyDone <- nil
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	select {
	case err := <-readyDone:
		t.Fatalf("unbounded readiness wait completed before initialization release: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(release) })
	select {
	case <-published:
	case <-time.After(2 * time.Second):
		t.Fatal("initializer did not publish readiness after release")
	}
	select {
	case err := <-readyDone:
		if err != nil {
			t.Fatalf("readiness wait after release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("readiness wait did not observe router handoff")
	}
	if status, _, err := getJSON("127.0.0.1", cfg.Server.Port, "/api/v1/tasks"); err != nil || status != http.StatusOK {
		t.Fatalf("application route after readiness = %d, %v; want 200", status, err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bootstrap did not stop after explicit process cancellation")
	}
}

// @covers AC-PLATFORM-STARTUP-LIFECYCLE-001.3, .6
func TestBootstrapRestoreKeepsProcessAliveUntilResultAndShutdown(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "kandev.db")
	backupsDir := filepath.Join(root, "backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(databasePath, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupsDir, "manual-restore.db"), []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	restoreBlocked := make(chan struct{})
	restoreRelease := make(chan struct{})
	jobIDReady := make(chan string, 1)
	resultReady := make(chan *jobs.Job, 1)
	bootstrapReady := make(chan struct{})
	done := make(chan bool, 1)
	var restoreBlockedOnce sync.Once
	var bootstrapReadyOnce sync.Once
	var restoreReleaseOnce sync.Once
	defer restoreReleaseOnce.Do(func() { close(restoreRelease) })

	go func() {
		done <- runWithBootstrap(ctx, cfg, testLogger(t), func(ctx context.Context) bool {
			runtime := ctx.Value(bootstrapContextKey{}).(*bootstrapRuntime)
			tracker := jobs.NewTracker(nil, testLogger(t))
			svc := backups.NewService(databasePath, nil, tracker, testLogger(t))
			svc.RestoreQuiesce = func() error {
				return quiesceForRestore(
					workerCancelFromContext(ctx),
					func() error {
						restoreBlockedOnce.Do(func() { close(restoreBlocked) })
						<-restoreRelease
						return nil
					}, nil, nil, nil,
				)
			}
			application := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" || r.URL.Path == "/ready" {
					writeBootstrapJSON(w, http.StatusOK, map[string]any{statusKey: "ok"})
					return
				}
				writeBootstrapJSON(w, http.StatusNotFound, map[string]any{statusKey: "not_found"})
			})
			publishReadiness(func() {
				ready.Store(true)
				runtime.ready.Store(true)
				bootstrapReadyOnce.Do(func() { close(bootstrapReady) })
			}, func() { runtime.handler.Store(application) })

			jobID, err := svc.Restore(context.Background(), "manual-restore.db", backups.RestoreConfirmToken)
			if err != nil {
				return false
			}
			jobIDReady <- jobID
			<-restoreBlocked
			<-restoreRelease
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				if job := tracker.Get(jobID); job != nil && job.State == jobs.StateSucceeded {
					resultReady <- job
					<-processContextFromContext(ctx).Done()
					return true
				}
				time.Sleep(5 * time.Millisecond)
			}
			return false
		})
	}()

	select {
	case <-bootstrapReady:
	case <-time.After(2 * time.Second):
		t.Fatal("bootstrap did not publish readiness")
	}
	select {
	case <-jobIDReady:
	case <-time.After(2 * time.Second):
		t.Fatal("restore job was not submitted")
	}
	select {
	case <-restoreBlocked:
	case <-time.After(2 * time.Second):
		t.Fatal("restore did not reach the blocked quiesce boundary")
	}
	if status, _, err := getJSON("127.0.0.1", cfg.Server.Port, "/health"); err != nil || status != http.StatusOK {
		t.Fatalf("/health while restore quiesce is blocked = %d, %v; want 200", status, err)
	}
	if status, _, err := getJSON("127.0.0.1", cfg.Server.Port, "/ready"); err != nil || status != http.StatusOK {
		t.Fatalf("/ready while restore quiesce is blocked = %d, %v; want 200", status, err)
	}
	select {
	case result := <-done:
		t.Fatalf("backend exited while restore was blocked: %v", result)
	default:
	}
	restoreReleaseOnce.Do(func() { close(restoreRelease) })
	select {
	case job := <-resultReady:
		if got, _ := job.Result["restart_required"].(bool); !got {
			t.Fatalf("restore result = %#v, want restart_required=true", job.Result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("restore result was not delivered")
	}
	select {
	case <-done:
		t.Fatal("backend exited before explicit shutdown after restore result")
	default:
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("backend did not exit after explicit shutdown")
	}
}

// @covers AC-PLATFORM-STARTUP-LIFECYCLE-001.5, .7
func TestRepositoryBackupFailureReportsPhaseAndPreventsMigration(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dataDir, "kandev.db")
	db, err := sqlx.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err = db.Exec("CREATE TABLE sentinel(value TEXT); INSERT INTO sentinel VALUES ('original')"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dataDir, "backups"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{HomeDir: dir}
	cfg.Database.Path = path
	progress := startup.New(nil)
	ctx := startup.WithReporter(context.Background(), progress)
	_, _, _, err = provideRepositories(ctx, cfg, testLogger(t), "upgrade-test")
	if err == nil {
		t.Fatal("backup failure did not fail initialization")
	}
	if got := progress.Snapshot().Phase; got != startup.BackingUpDatabase {
		t.Fatalf("phase=%s, want backup", got)
	}
	var count int
	if err = db.Get(&count, "SELECT count(*) FROM sqlite_master WHERE name='tasks'"); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("migration ran after failed backup")
	}
}

func TestCanceledPersistenceDoesNotStartBackup(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{HomeDir: dir}
	cfg.Database.Path = filepath.Join(dir, "test.db")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, cleanup, err := persistence.ProvideContext(ctx, cfg, nil, "test")
	if cleanup != nil {
		defer func() { _ = cleanup() }()
	}
	if err == nil {
		t.Fatal("canceled initialization opened persistence")
	}
}

func TestRepositoryFailureClosesDatabase(t *testing.T) {
	if _, err := os.Stat("/proc/self/fd"); err != nil {
		t.Skip("requires Linux descriptor inspection")
	}
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(data, "kandev.db")
	db, err := sqlx.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE tasks(id TEXT)"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{HomeDir: dir}
	cfg.Database.Path = path
	_, _, cleanups, err := provideRepositories(context.Background(), cfg, testLogger(t), "")
	for _, cleanup := range cleanups {
		if cleanup != nil {
			_ = cleanup()
		}
	}
	if err == nil {
		t.Fatal("invalid schema unexpectedly initialized")
	}
	fds, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for _, fd := range fds {
		target, _ := os.Readlink(filepath.Join("/proc/self/fd", fd.Name()))
		if target == path {
			t.Fatalf("failed migration leaked database descriptor %s", fd.Name())
		}
	}
}

func TestBootstrapSignalDuringInitialization(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals")
	}
	if os.Getenv("KANDEV_TEST_BOOTSTRAP_SIGNAL") == "1" {
		cfg := &config.Config{}
		cfg.Server.Host = "127.0.0.1"
		cfg.Server.Port = freePort(t)
		ok := runWithBootstrap(context.Background(), cfg, testLogger(t), func(ctx context.Context) bool {
			_, _ = os.Stdout.WriteString("BOUND\n")
			<-ctx.Done()
			return false
		})
		if ok {
			t.Fatal("canceled startup succeeded")
		}
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestBootstrapSignalDuringInitialization$")
	cmd.Env = append(os.Environ(), "KANDEV_TEST_BOOTSTRAP_SIGNAL=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	scan := bufio.NewScanner(stdout)
	bound := false
	for scan.Scan() {
		if scan.Text() == "BOUND" {
			bound = true
			break
		}
	}
	if !bound {
		t.Fatal("child did not bind")
	}
	if err = cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err != nil {
		t.Fatalf("initialization did not drain on SIGTERM: %v", err)
	}
}

// @covers AC-PLATFORM-STARTUP-LIFECYCLE-001.3, .6
func TestBootstrapSignalAfterReadiness(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals")
	}
	if os.Getenv("KANDEV_TEST_BOOTSTRAP_SIGNAL_READY") == "1" {
		cfg := &config.Config{}
		cfg.Server.Host = "127.0.0.1"
		cfg.Server.Port = freePort(t)
		ok := runWithBootstrap(context.Background(), cfg, testLogger(t), func(ctx context.Context) bool {
			runtime := ctx.Value(bootstrapContextKey{}).(*bootstrapRuntime)
			application := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeBootstrapJSON(w, http.StatusOK, map[string]any{statusKey: "ok"})
			})
			publishReadiness(func() {
				ready.Store(true)
				runtime.ready.Store(true)
			}, func() { runtime.handler.Store(application) })
			_, _ = os.Stdout.WriteString("READY\n")
			awaitShutdown(ctx, runtime.server, runtime.listeners, &schedulingRuntime{}, nil, nil, func() {}, testLogger(t))
			return true
		})
		if !ok {
			t.Fatal("ready bootstrap did not complete graceful shutdown")
		}
		return
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestBootstrapSignalAfterReadiness$")
	cmd.Env = append(os.Environ(), "KANDEV_TEST_BOOTSTRAP_SIGNAL_READY=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	scan := bufio.NewScanner(stdout)
	readyOutput := false
	for scan.Scan() {
		if scan.Text() == "READY" {
			readyOutput = true
			break
		}
	}
	if !readyOutput {
		t.Fatal("child did not publish readiness")
	}
	if err = cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err != nil {
		t.Fatalf("ready bootstrap did not shut down on SIGTERM: %v", err)
	}
}
