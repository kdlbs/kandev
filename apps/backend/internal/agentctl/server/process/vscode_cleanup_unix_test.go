//go:build !windows

package process

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetParentPID_CurrentProcess(t *testing.T) {
	ppid := getParentPID(os.Getpid())
	assert.Equal(t, os.Getppid(), ppid)
}

func TestGetParentPID_InvalidPID(t *testing.T) {
	// PID that almost certainly doesn't exist
	ppid := getParentPID(999999999)
	assert.Equal(t, -1, ppid)
}

func TestCleanupOrphanedCodeServers_NoMatch(t *testing.T) {
	log := newTestLogger(t)

	// Should not panic or error when no code-server processes exist.
	// pgrep returns exit 1 on no match, which the function handles gracefully.
	CleanupOrphanedCodeServers(log)
}

func TestLogChildProcesses_InvalidPGID(t *testing.T) {
	log := newTestLogger(t)

	// Should not panic with a non-existent process group.
	logChildProcesses(log, 999999999)
}
