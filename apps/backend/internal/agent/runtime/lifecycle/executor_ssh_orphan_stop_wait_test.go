//go:build !windows

package lifecycle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.16
func TestSSHOrphanStopCommandWaitsForKillDelivery(t *testing.T) {
	for _, shell := range []string{"sh", "bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			if _, err := exec.LookPath(shell); err != nil {
				t.Skipf("%s not available", shell)
			}
			for _, outcome := range []string{"exited", "zombie", "running"} {
				t.Run(outcome, func(t *testing.T) {
					testSSHOrphanKillDelivery(t, shell, outcome)
				})
			}
		})
	}
}

func testSSHOrphanKillDelivery(t *testing.T, shell, outcome string) {
	t.Helper()
	sessionDir := filepath.Join(t.TempDir(), "session")
	if err := os.Mkdir(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "agentctl.pid"), []byte("4242"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The OS boundary reports a live process until the script yields after
	// SIGKILL. kill -0 still succeeds when the terminated process is a zombie.
	boundary := fmt.Sprintf(`
kill_sent=0
kill_delivered=0
kill() {
  case "$1" in
    -9) kill_sent=1 ;;
    -0) [ "$kill_delivered" = 1 ] && [ %s = exited ] && return 1 ;;
  esac
  return 0
}
sleep() {
  [ "$kill_sent" = 1 ] && kill_delivered=1
  return 0
}
ps() {
  case "$1" in
    -p) printf 'agentctl --workdir /remote/tasks/task-1\n' ;;
    -o)
      if [ "$kill_delivered" = 1 ] && [ %s = zombie ]; then
        printf 'Z\n'
      else
        printf 'R\n'
      fi ;;
  esac
}
`, shellQuote(outcome), shellQuote(outcome))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	script := boundary + sshOrphanStopCommand(4242, "/remote/tasks/task-1", sessionDir)
	output, err := exec.CommandContext(ctx, shell, "-c", script).CombinedOutput()
	if outcome == "running" {
		if err == nil || ctx.Err() != nil {
			t.Fatalf("live target must fail within the polling bound: err=%v context=%v\n%s", err, ctx.Err(), output)
		}
		if !strings.Contains(string(output), "remote agentctl pid 4242 is still running") {
			t.Fatalf("missing live-target diagnostic: %s", output)
		}
		if _, err := os.Stat(sessionDir); err != nil {
			t.Fatalf("live target lost its session directory: %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("stop failed before signal delivery completed: %v\n%s", err, output)
	}
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Fatalf("terminated target retained its session directory: %v", err)
	}
}
