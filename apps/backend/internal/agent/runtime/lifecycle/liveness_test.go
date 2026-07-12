package lifecycle

import (
	"os"
	"os/exec"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// TestRowProcessLivenessLocalAliveVsDead proves a live local row is
// distinguishable from a dead one from the row alone (#1597).
func TestRowProcessLivenessLocalAliveVsDead(t *testing.T) {
	// This process is, by definition, alive.
	alive := &models.ExecutorRunning{
		SessionID: "s-alive",
		Runtime:   agentruntime.RuntimeStandalone,
		LocalPID:  os.Getpid(),
	}
	if got := RowProcessLiveness(alive); got != models.ProcessLivenessAlive {
		t.Fatalf("live standalone row: got %v, want models.ProcessLivenessAlive", got)
	}

	dead := &models.ExecutorRunning{
		SessionID: "s-dead",
		Runtime:   agentruntime.RuntimeStandalone,
		LocalPID:  spawnAndReapPID(t),
	}
	if got := RowProcessLiveness(dead); got != models.ProcessLivenessDead {
		t.Fatalf("dead standalone row: got %v, want models.ProcessLivenessDead", got)
	}
}

// TestRowProcessLivenessNeverProbesSSH is the no-regression guard: an SSH row's
// pid is a REMOTE pid, so the local predicate must never judge it — even when
// that pid value happens to match a live LOCAL process. It returns Unknown and
// defers to the SSH executor's remote kill -0 path (#1597 runtime-aware liveness).
func TestRowProcessLivenessNeverProbesSSH(t *testing.T) {
	sshRow := &models.ExecutorRunning{
		SessionID: "s-ssh",
		Runtime:   agentruntime.RuntimeSSH,
		// Deliberately set PID to THIS live local process. A local check would
		// wrongly report Alive; a runtime-aware one must report Unknown.
		PID:      os.Getpid(),
		LocalPID: 0,
	}
	if got := RowProcessLiveness(sshRow); got != models.ProcessLivenessUnknown {
		t.Fatalf("ssh row must not be judged by a local check: got %v, want models.ProcessLivenessUnknown", got)
	}
}

// TestRowProcessLivenessUnknownCases covers rows that cannot be judged locally:
// docker/remote runtimes, an unpopulated local handle, and a nil row.
func TestRowProcessLivenessUnknownCases(t *testing.T) {
	cases := []struct {
		name string
		row  *models.ExecutorRunning
	}{
		{"nil", nil},
		{"docker", &models.ExecutorRunning{Runtime: agentruntime.RuntimeDocker, LocalPID: os.Getpid()}},
		{"empty-runtime", &models.ExecutorRunning{Runtime: "", LocalPID: os.Getpid()}},
		{"local-no-handle", &models.ExecutorRunning{Runtime: agentruntime.RuntimeStandalone, LocalPID: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RowProcessLiveness(tc.row); got != models.ProcessLivenessUnknown {
				t.Fatalf("got %v, want models.ProcessLivenessUnknown", got)
			}
		})
	}
}

func TestIsLocalPIDAlive(t *testing.T) {
	if !isLocalPIDAlive(os.Getpid()) {
		t.Fatal("current process should be reported alive")
	}
	if isLocalPIDAlive(0) || isLocalPIDAlive(-1) {
		t.Fatal("non-positive pids must be reported dead")
	}
	if isLocalPIDAlive(spawnAndReapPID(t)) {
		t.Fatal("a reaped pid should be reported dead")
	}
}

// spawnAndReapPID starts a trivial child, waits for it to fully exit and be
// reaped, and returns its (now-dead) pid. Reaping avoids leaving a zombie that a
// signal-0 probe would still see as alive.
func spawnAndReapPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		// Fall back to a very high pid unlikely to be in use.
		return 0x7FFFFFF0
	}
	pid := cmd.Process.Pid
	_ = cmd.Wait()
	return pid
}
