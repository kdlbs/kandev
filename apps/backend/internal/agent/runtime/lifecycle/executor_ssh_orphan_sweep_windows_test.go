package lifecycle

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

const windowsOrphanWorkdirRoot = `C:\Users\neo\kandev-workspaces`

// windowsOrphanSweepHandler answers a Windows host whose OpenSSH default shell
// is Git Bash. Its uname reports a MINGW kernel and its $HOME is a /c/ path, so
// only a platform-aware root resolution can find the C:\ task directories. The
// PowerShell scripts travel on stdin, so the rules match on stdin.
func windowsOrphanSweepHandler(t *testing.T, stopped *atomic.Bool) sshExecHandler {
	return func(command, stdin string) sshExecResult {
		switch {
		case command == "uname -a":
			return sshOK
		case command == "uname -m":
			return sshOut("x86_64")
		case command == "uname -s":
			return sshOut("MINGW64_NT-10.0-19045")
		case command == "git --version":
			return sshFail("git: command not found")
		case strings.Contains(stdin, "$targetPid = 4242"):
			stopped.Store(true)
			return sshOK
		case strings.Contains(stdin, "USERPROFILE"):
			return sshOut(`C:\Users\neo`)
		case strings.Contains(stdin, "$root = ") && strings.Contains(stdin, "kandev-workspaces"):
			return sshOut("PROC\t4242\t1\tagentctl.exe --workdir \"" + windowsOrphanWorkdirRoot + `\tasks\task-1"` + "\n")
		case strings.Contains(stdin, "$root = "):
			return sshOK
		}
		t.Errorf("fake ssh server: unexpected command %q", command)
		return sshFail("no rule matched")
	}
}

// TestSweepSSHExecutorOrphansResolvesWindowsRootsByPlatform proves that the
// sweep resolves each workdir root for the detected platform. A POSIX $HOME
// probe would return /c/Users/neo here and the orphan would never be found.
func TestSweepSSHExecutorOrphansResolvesWindowsRootsByPlatform(t *testing.T) {
	archivedAt := time.Now().Add(-time.Hour)
	var stopped atomic.Bool
	server := newFakeSSHServer(t, windowsOrphanSweepHandler(t, &stopped))
	client := server.dial(t)

	store := &orphanSweepRootTestStore{
		profiles: []*models.ExecutorProfile{
			{ID: "bc154c2b", Config: map[string]string{"ssh_workdir_root": windowsOrphanWorkdirRoot}},
		},
		task: &models.Task{ID: "1", ArchivedAt: &archivedAt},
	}

	report, err := sweepSSHExecutorOrphans(context.Background(), client, store, "executor-1", map[string]string{}, logger.Default(), nil)
	if err != nil {
		t.Fatalf("sweepSSHExecutorOrphans: %v", err)
	}
	if report.Found != 1 || report.Stopped != 1 || !stopped.Load() {
		t.Fatalf("report = %+v, stop script sent = %t; want the Windows orphan found and stopped", report, stopped.Load())
	}
}
