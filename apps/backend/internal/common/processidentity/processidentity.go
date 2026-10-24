// Package processidentity records and verifies host process ownership without
// treating a PID or port alone as proof of identity.
package processidentity

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"
)

var ErrUnverifiableIdentity = errors.New("process identity is incomplete or unsupported")

// WindowsJobNamePrefix identifies launcher-created containment objects.
const WindowsJobNamePrefix = `Local\kandev-agentctl-`

// WindowsJobNameEnv carries the launcher-owned job to its suspended child.
// The child takes it with TakeWindowsJobNameFromEnv at startup, which removes
// it from the environment so the child's descendants never inherit it.
const WindowsJobNameEnv = "KANDEV_INTERNAL_WINDOWS_JOB_NAME"

func validWindowsJobName(name string) bool {
	if !strings.HasPrefix(name, WindowsJobNamePrefix) {
		return false
	}
	_, err := uuid.Parse(strings.TrimPrefix(name, WindowsJobNamePrefix))
	return err == nil
}

var ownWindowsJobName struct {
	sync.Mutex
	name  string
	taken bool
}

// TakeWindowsJobNameFromEnv reads the job name the launcher passed to this
// process, removes it from the process environment, and keeps it for Capture
// of this process. agentctl calls it first at startup, before it starts any
// process, so agents, scripts and nested agentctl processes do not inherit the
// outer job name and cannot record the outer job as their own. Only the first
// call that finds the variable is kept.
func TakeWindowsJobNameFromEnv() {
	name, ok := os.LookupEnv(WindowsJobNameEnv)
	if !ok {
		return
	}
	_ = os.Unsetenv(WindowsJobNameEnv)
	ownWindowsJobName.Lock()
	defer ownWindowsJobName.Unlock()
	if !ownWindowsJobName.taken {
		ownWindowsJobName.name = name
		ownWindowsJobName.taken = true
	}
}

// takenWindowsJobName returns the job name taken at startup, or empty.
func takenWindowsJobName() string {
	ownWindowsJobName.Lock()
	defer ownWindowsJobName.Unlock()
	return ownWindowsJobName.name
}

type State string

const (
	StateAlive   State = "alive"
	StateExited  State = "exited"
	StateReused  State = "reused"
	StateUnknown State = "unknown"
)

// Identity binds a process ID and its containment group to an OS-specific
// birth token. Empty birth tokens represent legacy or unsupported evidence.
type Identity struct {
	PID            int    `json:"pid"`
	GroupID        int    `json:"group_id,omitempty"`
	SessionID      int    `json:"session_id,omitempty"`
	BirthToken     string `json:"birth_token,omitempty"`
	WindowsJobName string `json:"windows_job_name,omitempty"`
}

func (identity Identity) Validate() error {
	if identity.PID <= 0 || identity.BirthToken == "" {
		return ErrUnverifiableIdentity
	}
	return nil
}

func (identity Identity) String() string {
	if identity.PID <= 0 {
		return "unknown"
	}
	return fmt.Sprintf("pid=%d group=%d session=%d", identity.PID, identity.GroupID, identity.SessionID)
}
