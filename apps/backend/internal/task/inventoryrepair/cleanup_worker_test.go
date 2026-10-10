package inventoryrepair

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

func TestCleanupWorkerReplaysRepairedSuccessor(t *testing.T) {
	f := cleanupFixture(t)
	repo, err := tasksqlite.NewWithDB(f.db, f.db, nil)
	if err != nil {
		t.Fatal(err)
	}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	svc := taskservice.NewService(taskservice.Repos{Tasks: repo, Sessions: repo, Executors: repo, TaskEnvironments: repo, ResourceCleanups: repo, TaskRepos: repo, Workspaces: repo}, nil, log, taskservice.RepositoryDiscoveryConfig{})
	svc.SetWorktreeCleanup(fixtureManager(t, f))
	p := observedPlan(t, f)
	if err = repairWithIsolatedHost(p, "apply"); err != nil {
		t.Fatal(err)
	}
	if err = svc.ResumeTaskResourceCleanupJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	var state, data string
	if err = f.db.QueryRow(`SELECT state,resource_snapshot FROM task_resource_cleanup_jobs WHERE id<>'job'`).Scan(&state, &data); err != nil {
		t.Fatal(err)
	}
	if state != "succeeded" {
		t.Fatalf("successor state=%s snapshot=%s", state, data)
	}
	var s map[string]json.RawMessage
	if err = json.Unmarshal([]byte(data), &s); err != nil {
		t.Fatal(err)
	}
	if len(s["inventory_repair"]) == 0 || string(s["archive_source_manifest_captured"]) != "true" {
		t.Fatal("worker lost provenance or failed source capture")
	}
	if _, err = os.Stat(p.Repairs[0].Path); !os.IsNotExist(err) {
		t.Fatalf("checkout was not cleaned: %v", err)
	}
	if got := git(t, f.repo, "rev-parse", p.Repairs[0].Branch); got != p.Repairs[0].HeadOID {
		t.Fatal("archive deleted or changed branch")
	}
}
