package lifecycle

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/common/logger"
)

type recordingWorker struct {
	commands []string
	failOn   string
}

func (w *recordingWorker) Run(_ context.Context, command string) (string, string, error) {
	w.commands = append(w.commands, command)
	if w.failOn != "" && strings.Contains(command, w.failOn) {
		return "", "", errors.New("worker command failed")
	}
	return "", "", nil
}
func (*recordingWorker) RunStdin(context.Context, string, io.Reader) (string, string, error) {
	return "", "", nil
}
func (*recordingWorker) Upload(context.Context, string, []byte, os.FileMode) error { return nil }
func (*recordingWorker) Forward(int, *logger.Logger) (nomadPortForward, error) {
	return nil, errors.New("unused")
}
func (*recordingWorker) Close() error { return nil }

func poolCleanupForFinish(t *testing.T, w *recordingWorker, releases *int) *nomadCleanup {
	t.Helper()
	root := t.TempDir()
	workspaceRoot := filepath.Join(root, "workspaces")
	workspace, err := nomad.EnvironmentWorkspacePath(workspaceRoot, "env-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	state := nomad.EnvironmentState{
		PlacementMode: nomad.ModePool, EnvironmentID: "env-1", Generation: 1, JobID: "kandev-job-1",
		NodeID: "node-1", AllocationID: "alloc-1", StateRoot: root, WorkspaceRoot: workspaceRoot,
		WorkspacePath: workspace, AllocationRoot: filepath.Join(root, "pool", "allocations"),
	}
	req := &ExecutorCreateRequest{ReleaseRuntimeInventory: func(context.Context) error { *releases++; return nil }}
	return &nomadCleanup{
		exec: &NomadExecutor{logger: newTestLogger(), sessions: map[string]*nomadAgentctlConnection{}}, req: req,
		profile: nomad.ProfileConfig{NodeID: "node-1", AllocationRoot: state.AllocationRoot}, state: state, worker: w,
	}
}

func TestNomadPoolCleanupRemovesAllocationDirBeforeFenceAndInventory(t *testing.T) {
	w, releases := &recordingWorker{}, 0
	c := poolCleanupForFinish(t, w, &releases)
	if err := c.finish(context.Background()); err != nil {
		t.Fatalf("finish: %v", err)
	}
	var alloc, fence = -1, -1
	for i, command := range w.commands {
		if strings.Contains(command, filepath.Join(c.state.AllocationRoot, c.state.JobID)) && strings.Contains(command, "rm -rf") {
			alloc = i
		}
		if strings.Contains(command, `/owner"`) {
			fence = i
		}
	}
	if alloc < 0 || fence < 0 || alloc > fence || releases != 1 {
		t.Fatalf("alloc dir removal at %d, fence at %d, releases %d: %v", alloc, fence, releases, w.commands)
	}
}

func TestNomadPoolCleanupRetainsWhenAllocationDirRemovalFails(t *testing.T) {
	w, releases := &recordingWorker{failOn: "kandev-job-1"}, 0
	c := poolCleanupForFinish(t, w, &releases)
	if err := c.finish(context.Background()); !errors.Is(err, ErrNomadCleanupRetained) {
		t.Fatalf("finish = %v, want retained", err)
	}
	if releases != 0 {
		t.Fatal("inventory released with the allocation directory still present")
	}
}
