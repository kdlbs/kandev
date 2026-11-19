package backendapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/repoclone"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func TestQuiesceForRestoreStopsAllWritersAfterFailures(t *testing.T) {
	firstErr := errors.New("scheduler failed")
	secondErr := errors.New("worker failed")
	var calls []string
	mark := func(name string) func() error {
		return func() error {
			calls = append(calls, name)
			if name == "scheduling" {
				return firstErr
			}
			if name == "worker" {
				return secondErr
			}
			return nil
		}
	}

	err := quiesceForRestore(
		func() { calls = append(calls, "cancel") },
		mark("scheduling"),
		mark("orchestrator"),
		mark("agents"),
		[]func() error{mark("worker")},
	)

	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("error = %v, want both stop errors", err)
	}
	wantCalls := []string{"cancel", "scheduling", "orchestrator", "agents", "worker"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", calls, wantCalls)
	}
}

func TestRestoreCleanupRegistrationDrainsGatedWorker(t *testing.T) {
	var runtimeCleanups []func() error
	var restoreCleanups []func() error
	addRuntimeCleanup := func(cleanup func() error) { runtimeCleanups = append(runtimeCleanups, cleanup) }
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	workerStarted := make(chan struct{})
	workerDone := make(chan struct{})
	go func() {
		close(workerStarted)
		<-workerCtx.Done()
		close(workerDone)
	}()
	select {
	case <-workerStarted:
	case <-time.After(time.Second):
		t.Fatal("gated worker did not start")
	}

	var stopCalls atomic.Int32
	restoreWorkerCleanupRegistrar(addRuntimeCleanup, &restoreCleanups)(func() error {
		stopCalls.Add(1)
		cancelWorker()
		<-workerDone
		return nil
	})
	if len(runtimeCleanups) != 1 || len(restoreCleanups) != 1 {
		t.Fatalf("registered cleanups = runtime %d, restore %d; want one of each", len(runtimeCleanups), len(restoreCleanups))
	}
	if err := quiesceForRestore(nil, nil, nil, nil, restoreCleanups); err != nil {
		t.Fatalf("restore quiesce: %v", err)
	}
	select {
	case <-workerDone:
	default:
		t.Fatal("restore quiesce returned before the gated worker stopped")
	}
	for _, cleanup := range runtimeCleanups {
		if err := cleanup(); err != nil {
			t.Fatalf("runtime cleanup: %v", err)
		}
	}
	if got := stopCalls.Load(); got != 1 {
		t.Fatalf("worker stop calls = %d, want one across restore and shutdown", got)
	}
}

func TestRegisterImproveKandevRoutesUsesRestoreCleanup(t *testing.T) {
	harness := newBootStateTestHarness(t)
	root := t.TempDir()
	startedPath := installRestoreGatedGit(t, root)
	cloner := repoclone.NewCloner(repoclone.Config{BasePath: filepath.Join(root, "managed")}, repoclone.ProtocolHTTPS, "", logger.Default())
	cloner.SetGitCredentialProvider(restoreTestGitCredentialProvider{})
	router, runtimeCleanups, restoreCleanups := registerImproveKandevRoutesForRestore(t, harness.taskSvc, cloner)

	if len(runtimeCleanups) != 1 || len(restoreCleanups) != 1 {
		t.Fatalf("Improve Kandev cleanup registration = runtime %d, restore %d; want one of each", len(runtimeCleanups), len(restoreCleanups))
	}
	jobID := requestRestoreBootstrap(t, router)
	waitForRestoreClone(t, startedPath)

	quiesceDone := make(chan error, 1)
	go func() { quiesceDone <- quiesceForRestore(nil, nil, nil, nil, restoreCleanups) }()
	select {
	case err := <-quiesceDone:
		if err != nil {
			t.Fatalf("restore quiesce: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("restore quiesce did not drain the gated bootstrap worker")
	}
	statusRequest := httptest.NewRequest(http.MethodGet, "/api/v1/system/improve-kandev/bootstrap/"+jobID, nil)
	status := httptest.NewRecorder()
	router.ServeHTTP(status, statusRequest)
	if status.Code != http.StatusInternalServerError {
		t.Fatalf("bootstrap status after restore drain = %d, want %d: %s", status.Code, http.StatusInternalServerError, status.Body.String())
	}
}

func installRestoreGatedGit(t *testing.T, root string) string {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	startedPath := filepath.Join(root, "clone-started")
	fakeGit := "#!/bin/sh\n" +
		"if [ \"$1\" = clone ]; then\n" +
		"  for target do :; done\n" +
		"  mkdir -p \"$target/.git\"\n" +
		"  : > \"$CLONE_STARTED_PATH\"\n" +
		"  exec sleep 30\n" +
		"fi\n" +
		"exec \"$REAL_GIT_PATH\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(fakeGit), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("REAL_GIT_PATH", realGit)
	t.Setenv("CLONE_STARTED_PATH", startedPath)
	return startedPath
}

func registerImproveKandevRoutesForRestore(
	t *testing.T,
	taskSvc *taskservice.Service,
	cloner *repoclone.Cloner,
) (*gin.Engine, []func() error, []func() error) {
	t.Helper()
	var runtimeCleanups []func() error
	var restoreCleanups []func() error
	addRuntimeCleanup := func(cleanup func() error) { runtimeCleanups = append(runtimeCleanups, cleanup) }
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "restore-test"})
		c.Next()
	})
	registerImproveKandevRoutes(routeParams{
		router:            router,
		services:          &Services{},
		repoCloner:        cloner,
		taskSvc:           taskSvc,
		log:               logger.Default(),
		addCleanup:        addRuntimeCleanup,
		addRestoreCleanup: restoreWorkerCleanupRegistrar(addRuntimeCleanup, &restoreCleanups),
	})
	t.Cleanup(func() {
		for i := len(runtimeCleanups) - 1; i >= 0; i-- {
			_ = runtimeCleanups[i]()
		}
	})
	return router, runtimeCleanups, restoreCleanups
}

func requestRestoreBootstrap(t *testing.T, router *gin.Engine) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewBufferString(`{"async":true,"create_workspace":true}`))
	request.Header.Set("Content-Type", "application/json")
	admission := httptest.NewRecorder()
	router.ServeHTTP(admission, request)
	if admission.Code != http.StatusAccepted {
		t.Fatalf("bootstrap admission status = %d, want %d: %s", admission.Code, http.StatusAccepted, admission.Body.String())
	}
	var admitted struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(admission.Body.Bytes(), &admitted); err != nil {
		t.Fatalf("decode bootstrap admission: %v", err)
	}
	if admitted.JobID == "" {
		t.Fatal("bootstrap admission returned an empty job ID")
	}
	return admitted.JobID
}

func waitForRestoreClone(t *testing.T, startedPath string) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		if _, err := os.Stat(startedPath); err == nil {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("production bootstrap route did not reach the gated Git clone")
		}
	}
}

type restoreTestGitCredentialProvider struct{}

func (restoreTestGitCredentialProvider) ResolveGitCredential(context.Context, repoclone.GitCredentialRequest) (string, string, error) {
	return "test", "test-token", nil
}
