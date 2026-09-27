package process

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// agentPgidFileName is the file name written under sessionDir(). Reap-on-redial
// (tasks 05/06) reads this exact name.
const agentPgidFileName = "agent.pgid"

// agentPgidRecord is the on-disk shape of agent.pgid: the process group ID
// (Manager.AgentPID(), because Setpgid makes the group ID equal the PID) and
// the time the process started, per system design part 2 "Orphaned agent
// after agentctl loss" (AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.7). A
// later redial reads this file to tell a live agent from a reused PID before
// reaping it.
type agentPgidRecord struct {
	PGID      int       `json:"pgid"`
	StartedAt time.Time `json:"started_at"`
}

// sessionDir returns this instance's session directory, where per-session
// runtime data (agent.pgid, eventually logs) lives. It must land on the same
// path the SSH lifecycle executor's ensureRemoteSessionDir creates for this
// session: WorkDir is that executor's taskDir (passed to agentctl as
// --workdir), so <WorkDir>/.kandev/sessions/<SessionID> matches it exactly.
func (m *Manager) sessionDir() string {
	return filepath.Join(m.cfg.WorkDir, ".kandev", "sessions", m.cfg.SessionID)
}

// writeAgentPgidFile records this instance's agent process as reapable: the
// process group ID (equal to the PID, since the process is started with
// Setpgid) and its start time, at sessionDir()/agent.pgid. Best-effort, like
// the sibling per-instance VS Code settings write: a failure here must not
// fail agent Start, and is logged instead. Skipped when cfg is nil or
// SessionID is empty (no session directory to land in), which includes tests
// that construct a bare Manager to exercise a single method in isolation.
func (m *Manager) writeAgentPgidFile(pgid int, startedAt time.Time) {
	if m.cfg == nil || m.cfg.SessionID == "" {
		return
	}
	dir := m.sessionDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		m.logger.Warn("failed to create session directory for agent.pgid", zap.String("dir", dir), zap.Error(err))
		return
	}
	data, err := json.Marshal(agentPgidRecord{PGID: pgid, StartedAt: startedAt})
	if err != nil {
		m.logger.Warn("failed to marshal agent.pgid record", zap.Error(err))
		return
	}
	path := filepath.Join(dir, agentPgidFileName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		m.logger.Warn("failed to write agent.pgid", zap.String("path", path), zap.Error(err))
	}
}

// removeAgentPgidFile deletes the agent.pgid record written by
// writeAgentPgidFile. Idempotent: a missing file (never written, or already
// removed by a prior stop) is not an error. Best-effort, like the write side.
func (m *Manager) removeAgentPgidFile() {
	if m.cfg == nil || m.cfg.SessionID == "" {
		return
	}
	path := filepath.Join(m.sessionDir(), agentPgidFileName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		m.logger.Warn("failed to remove agent.pgid", zap.String("path", path), zap.Error(err))
	}
}
