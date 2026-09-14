package backups

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/system/jobs"
	"github.com/kandev/kandev/internal/system/maintenance"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManualBackupCanceledDoesNotPublish(t *testing.T) {
	s, _ := newTestService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.runCreate(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want canceled, got %v", err)
	}
	snapshots, err := s.List()
	if err != nil || len(snapshots) != 0 {
		t.Fatalf("published canceled backup: %v %v", snapshots, err)
	}
}

func TestManualBackupIsPrivate(t *testing.T) {
	s, dir := newTestService(t)
	result, err := s.runCreate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "backups", result["name"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("snapshot permissions = %o", info.Mode().Perm())
	}
}

func TestRetentionReceiptDetectsMissingChangedAndCorruptSnapshot(t *testing.T) {
	for _, mutation := range []string{"missing", "changed", "corrupt", "symlink"} {
		t.Run(mutation, func(t *testing.T) {
			s, _ := newTestService(t)
			receipt, err := s.CreateForRetention(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if err := s.VerifyRetentionBackup(context.Background(), receipt); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(s.backupsDir(), receipt.Name)
			switch mutation {
			case "missing":
				err = os.Remove(path)
			case "changed":
				var snapshot *sqlx.DB
				snapshot, err = sqlx.Open("sqlite3", path)
				if err == nil {
					_, err = snapshot.Exec("UPDATE things SET name='other'")
					_ = snapshot.Close()
				}
			case "corrupt":
				err = os.WriteFile(path, make([]byte, receipt.SizeBytes), 0600)
			case "symlink":
				err = os.Remove(path)
				if err == nil {
					err = os.Symlink(s.databasePath, path)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := s.VerifyRetentionBackup(context.Background(), receipt); err == nil {
				t.Fatal("accepted invalid snapshot")
			}
		})
	}
}

func TestRestoreCanceledBeforeSideEffects(t *testing.T) {
	s, _ := newTestService(t)
	receipt, err := s.CreateForRetention(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	s.RestoreQuiesce = func() error { called = true; return nil }
	_, err = s.runRestore(ctx, filepath.Join(s.backupsDir(), receipt.Name))
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("err=%v quiesced=%v", err, called)
	}
}

func TestBackupFailureDoesNotPublishOrLeakStaging(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.pool.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateForRetention(context.Background()); err == nil {
		t.Fatal("expected closed database error")
	}
	entries, err := os.ReadDir(s.backupsDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed backup left files: %v", entries)
	}
}

func TestRestoreHoldsMaintenanceAdmissionThroughQuiescence(t *testing.T) {
	s, _ := newTestService(t)
	receipt, err := s.CreateForRetention(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	stopped := errors.New("stop before pool close")
	s.RestoreQuiesce = func() error {
		if release, ok := maintenance.ForPool(s.pool).TryAcquire(); ok {
			release()
			t.Error("restore released admission before quiescence")
		}
		return stopped
	}
	_, err = s.runRestore(context.Background(), filepath.Join(s.backupsDir(), receipt.Name))
	if !errors.Is(err, stopped) {
		t.Fatal(err)
	}
	release, ok := maintenance.ForPool(s.pool).TryAcquire()
	if !ok {
		t.Fatal("failed restore leaked admission")
	}
	release()
}

func TestVerifySnapshotRejectsCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.db")
	if err := os.WriteFile(path, []byte("not a sqlite database"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifySnapshot(context.Background(), path); err == nil {
		t.Fatal("accepted invalid database")
	}
}

func TestBackupSweepsAbandonedPrivateStaging(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.ensureBackupsDir(); err != nil {
		t.Fatal(err)
	}
	stage, err := os.MkdirTemp(s.backupsDir(), ".snapshot-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "snapshot.db"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * staleTmpAge)
	if err := os.Chtimes(stage, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateForRetention(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatalf("abandoned staging remains: %v", err)
	}
}

func TestDeleteWaitsForMaintenanceAdmission(t *testing.T) {
	s, _ := newTestService(t)
	receipt, err := s.CreateForRetention(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release, ok := maintenance.ForPool(s.pool).TryAcquire()
	if !ok {
		t.Fatal("lease unavailable")
	}
	defer release()
	// Cancellation proves that deletion attempts admission before touching the file.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.DeleteContext(ctx, receipt.Name); !errors.Is(err, context.Canceled) {
		t.Fatalf("want canceled, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.backupsDir(), receipt.Name)); err != nil {
		t.Fatal(err)
	}
}

func TestRetentionReceiptVerificationUnderBatchLease(t *testing.T) {
	s, _ := newTestService(t)
	receipt, err := s.CreateForRetention(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release, ok := maintenance.ForPool(s.pool).TryAcquire()
	if !ok {
		t.Fatal("lease unavailable")
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.VerifyRetentionBackupUnderLease(ctx, receipt); err != nil {
		t.Fatal(err)
	}
	if other, ok := maintenance.ForPool(s.pool).TryAcquire(); ok {
		other()
		t.Fatal("verification released caller lease")
	}
}

func TestHTTPBackupSurvivesAcceptedRequestCancellation(t *testing.T) {
	for _, kind := range []string{"create", "restore"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := newTestService(t)
			path := "/api/v1/system/backups"
			if kind == "restore" {
				result, err := s.runCreate(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				path += "/" + result["name"].(string) + "/restore"
			}
			release, ok := maintenance.ForPool(s.pool).TryAcquire()
			if !ok {
				t.Fatal("lease unavailable")
			}
			defer release()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"confirm":"RESTORE"}`)).WithContext(ctx)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			newRouter(s).ServeHTTP(response, request)
			if response.Code != http.StatusAccepted {
				t.Fatalf("response: %d %s", response.Code, response.Body.String())
			}
			var body struct {
				JobID string `json:"job_id"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			cancel() // net/http cancels the request after the accepted response returns.
			release()
			waitForJob(t, s.jobs, body.JobID, jobs.StateSucceeded)
		})
	}
}

func TestAcceptedBackupAPIsSurviveCallerCancellation(t *testing.T) {
	for _, kind := range []string{"create", "restore"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := newTestService(t)
			snapshot, err := s.runCreate(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			release, ok := maintenance.ForPool(s.pool).TryAcquire()
			if !ok {
				t.Fatal("lease unavailable")
			}
			defer release()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var id string
			if kind == "create" {
				id = s.Create(ctx)
			} else {
				id, err = s.Restore(ctx, snapshot["name"].(string), RestoreConfirmToken)
				if err != nil {
					t.Fatal(err)
				}
			}
			cancel()
			release()
			waitForJob(t, s.jobs, id, jobs.StateSucceeded)
		})
	}
}
