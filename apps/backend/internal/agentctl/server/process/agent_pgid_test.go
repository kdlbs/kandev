package process

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/pkg/agent"
)

// TestAgentPgidRecord pins the "Record" step of system design part 2
// "Orphaned agent after agentctl loss" (AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.7):
// on agent start, agentctl writes the process group ID and process start time
// to agent.pgid in the session directory, and removes it on stop.
func TestAgentPgidRecord(t *testing.T) {
	workDir := t.TempDir()
	mgr := NewManager(&config.InstanceConfig{
		AgentArgs: fixtureArgs(),
		AgentEnv:  fixtureEnvSlice("sleep 60"),
		WorkDir:   workDir,
		SessionID: "session-1",
		Protocol:  agent.ProtocolACP,
	}, newTestLogger(t))

	before := time.Now()
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })

	pgidPath := filepath.Join(workDir, ".kandev", "sessions", "session-1", "agent.pgid")
	data, err := os.ReadFile(pgidPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v, want agent.pgid written on Start", pgidPath, err)
	}

	var record agentPgidRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("Unmarshal(agent.pgid) error = %v", err)
	}
	if record.PGID != mgr.AgentPID() {
		t.Fatalf("record.PGID = %d, want %d", record.PGID, mgr.AgentPID())
	}
	if record.StartedAt.Before(before) || record.StartedAt.After(time.Now()) {
		t.Fatalf("record.StartedAt = %v, want between %v and now", record.StartedAt, before)
	}

	killAgentProcess(t, mgr)
	if err := mgr.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if _, statErr := os.Stat(pgidPath); !os.IsNotExist(statErr) {
		t.Fatalf("Stat(%s) error = %v, want file removed on Stop", pgidPath, statErr)
	}
}
