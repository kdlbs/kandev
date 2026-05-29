package updates

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/system/jobs"
)

func TestService_ApplyQueuesSelfUpdateJobAndWritesIntent(t *testing.T) {
	homeDir := t.TempDir()
	metadataPath, _ := writeServiceInstallForTest(t, homeDir, serviceInstallMetadata{
		Manager:     "systemd",
		Mode:        "user",
		Kind:        "npm",
		HomeDir:     homeDir,
		LogDir:      filepath.Join(homeDir, "logs"),
		ServicePath: filepath.Join(homeDir, "kandev.service"),
		NodePath:    "/usr/bin/node",
		CLIEntry:    "/usr/lib/node_modules/kandev/bin/cli.js",
		Port:        38429,
	})
	t.Setenv(envRunningAsService, "true")
	t.Setenv(envServiceMode, "user")
	t.Setenv(envServiceManager, "systemd")
	t.Setenv(envInstallKind, "npm")
	t.Setenv(envServiceMetadata, metadataPath)

	pool := newTestPool(t)
	if err := persistence.WriteLatestVersion(pool.Writer(), "v1.0.1", "https://example/v1.0.1", time.Now().UTC()); err != nil {
		t.Fatalf("write latest: %v", err)
	}
	tracker := jobs.NewTracker(nil, logger.Default())
	var gotReq applyRequest
	svc := NewService(
		pool,
		"v1.0.0",
		nil,
		logger.Default(),
		WithHomeDir(homeDir),
		WithJobs(tracker),
		WithApplyRunner(func(_ context.Context, req applyRequest) (map[string]interface{}, error) {
			gotReq = req
			return map[string]interface{}{"status": "started"}, nil
		}),
	)

	jobID, err := svc.Apply(context.Background(), "UPDATE")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	waitForJobState(t, tracker, jobID, jobs.StateSucceeded)
	if gotReq.IntentPath == "" {
		t.Fatalf("runner did not receive intent path")
	}
	data, err := os.ReadFile(gotReq.IntentPath)
	if err != nil {
		t.Fatalf("read intent: %v", err)
	}
	var intent updateIntent
	if err := json.Unmarshal(data, &intent); err != nil {
		t.Fatalf("unmarshal intent: %v", err)
	}
	if intent.TargetVersion != "1.0.1" {
		t.Fatalf("TargetVersion=%q want 1.0.1", intent.TargetVersion)
	}
	if intent.Install.Port != 38429 {
		t.Fatalf("Port=%d want 38429", intent.Install.Port)
	}
}

func TestService_ApplyRejectsUnsupportedInstall(t *testing.T) {
	pool := newTestPool(t)
	if err := persistence.WriteLatestVersion(pool.Writer(), "v1.0.1", "https://example/v1.0.1", time.Now().UTC()); err != nil {
		t.Fatalf("write latest: %v", err)
	}
	svc := NewService(pool, "v1.0.0", nil, logger.Default(), WithJobs(jobs.NewTracker(nil, logger.Default())))

	_, err := svc.Apply(context.Background(), "UPDATE")
	if !errors.Is(err, ErrApplyUnsupported) {
		t.Fatalf("err=%v want ErrApplyUnsupported", err)
	}
}

func TestService_ApplyRejectsWrongConfirm(t *testing.T) {
	svc := NewService(newTestPool(t), "v1.0.0", nil, logger.Default())
	_, err := svc.Apply(context.Background(), "NOPE")
	if !errors.Is(err, ErrApplyConfirm) {
		t.Fatalf("err=%v want ErrApplyConfirm", err)
	}
}

func waitForJobState(t *testing.T, tracker *jobs.Tracker, id string, want jobs.State) *jobs.Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job := tracker.Get(id)
		if job != nil && job.State == want {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach %s", id, want)
	return nil
}
