package database

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/jobs"
)

// httpGet builds a GET request and is shared by stats/maintenance/reset tests.
func httpGet(t *testing.T, path string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	return req
}

// httpPost builds a POST request with an optional JSON body.
func httpPost(t *testing.T, path string, body string) *http.Request {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(http.MethodPost, path, reader)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func serveHTTP(r *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

func TestVacuum_StartsJobAndShrinksDB(t *testing.T) {
	svc, tracker, _, _ := newTestService(t)

	before, err := svc.Stats()
	if err != nil {
		t.Fatalf("stats before: %v", err)
	}

	id := svc.Vacuum(context.Background())
	if id == "" {
		t.Fatal("Vacuum returned empty job id")
	}
	job := waitForState(t, tracker, id, jobs.StateSucceeded)
	if job.Result["reclaimed_bytes"] == nil {
		t.Errorf("expected reclaimed_bytes in result, got %+v", job.Result)
	}

	after, err := svc.Stats()
	if err != nil {
		t.Fatalf("stats after: %v", err)
	}
	// DB file must still exist (Stats reads from it).
	if after.Path != before.Path {
		t.Errorf("path changed: %s -> %s", before.Path, after.Path)
	}
	if after.SizeBytes <= 0 {
		t.Errorf("size after vacuum = %d, want > 0", after.SizeBytes)
	}
	// schema_version preserved across vacuum.
	if after.SchemaVersion != "v0.99.0" {
		t.Errorf("schema_version lost after vacuum: %q", after.SchemaVersion)
	}
}

func TestOptimize_StartsJobAndSucceeds(t *testing.T) {
	svc, tracker, _, _ := newTestService(t)

	id := svc.Optimize(context.Background())
	if id == "" {
		t.Fatal("Optimize returned empty job id")
	}
	job := waitForState(t, tracker, id, jobs.StateSucceeded)
	if job.State != jobs.StateSucceeded {
		t.Errorf("state = %s, want succeeded", job.State)
	}

	// Verify a follow-up query against the DB still works (sanity).
	stats, err := svc.Stats()
	if err != nil {
		t.Fatalf("post-optimize stats: %v", err)
	}
	if stats.SchemaVersion != "v0.99.0" {
		t.Errorf("schema_version lost after optimize: %q", stats.SchemaVersion)
	}
}

func TestMaintenanceOperationsRejectPostgres(t *testing.T) {
	svc := NewService(newFakePostgresStatsPool(t), t.TempDir(), ResetDirs{}, nil, nil)
	shutdownCalled := false
	svc.OrchestratorShutdown = func() { shutdownCalled = true }

	cases := []struct {
		name    string
		run     func() error
		wantErr string
	}{
		{
			name: "vacuum",
			run: func() error {
				_, err := svc.runVacuum(context.Background())
				return err
			},
			wantErr: "vacuum: not supported for postgres driver",
		},
		{
			name: "optimize",
			run: func() error {
				_, err := svc.runOptimize(context.Background())
				return err
			},
			wantErr: "optimize: not supported for postgres driver",
		},
		{
			name: "factory reset",
			run: func() error {
				_, err := svc.runFactoryReset(context.Background())
				return err
			},
			wantErr: "factory reset: not supported for postgres driver",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
	if shutdownCalled {
		t.Fatal("factory reset called orchestrator shutdown before rejecting postgres")
	}
}

func TestMaintenanceOperationsRejectNilWriter(t *testing.T) {
	svc := NewService(db.NewPool(nil, nil), t.TempDir(), ResetDirs{}, nil, nil)

	_, err := svc.runVacuum(context.Background())
	if err == nil || err.Error() != "vacuum: no database pool" {
		t.Fatalf("err = %v, want vacuum: no database pool", err)
	}
}

func TestHandleVacuum_Returns202WithJobID(t *testing.T) {
	svc, tracker, _, _ := newTestService(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/vacuum", HandleVacuum(svc))

	w := serveHTTP(r, httpPost(t, "/vacuum", ""))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), `"job_id"`) {
		t.Errorf("body missing job_id: %s", w.Body.String())
	}
	// Make sure the spawned job finishes before the test exits so the temp dir
	// cleanup does not race with VACUUM still writing the SQLite journal.
	for _, j := range tracker.List() {
		waitForState(t, tracker, j.ID, jobs.StateSucceeded)
	}
}

func TestHandleOptimize_Returns202WithJobID(t *testing.T) {
	svc, tracker, _, _ := newTestService(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/optimize", HandleOptimize(svc))

	w := serveHTTP(r, httpPost(t, "/optimize", ""))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), `"job_id"`) {
		t.Errorf("body missing job_id: %s", w.Body.String())
	}
	// Same cleanup-race guard as TestHandleVacuum_Returns202WithJobID.
	for _, j := range tracker.List() {
		waitForState(t, tracker, j.ID, jobs.StateSucceeded)
	}
}
