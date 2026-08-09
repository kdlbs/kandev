---
task: 04
title: "Agentctl probe — real process-tree walk"
wave: 1
status: done
depends_on: [01, 02]
spec_acs: AC-70, AC-70a, AC-71, AC-72, AC-80, AC-81
---

# Task 04 — Agentctl probe (real process-tree walk)

Replace the Wave-0 stub (`handleWSBackgroundProbe` returning `unknown`) with the real
implementation: walk the agent process-tree, apply the start-time predicate, and return
`live | settled | unknown`.

## Key constraints from the spec

- **`AgentPID` accessor on `process.Manager`** — the component that spawns the agent as
  its own `m.cmd` and already exposes `agentPID()`. The accessor takes the **ACP** session
  id (not the Kandev session id) and returns a pid only. Never use `ProcessRunner` (which
  manages workspace processes) or the process group.
- **Start-time predicate**: a descendant is "live" if it was created at or after
  `turn_marker_time − 2s` (a constant slack for process-fork delay). Descendants born
  before the turn's reference time are considered pre-existing infrastructure, not
  background work spawned by the current turn.
- **Three return values**: `live` (≥1 qualifying descendant alive), `settled` (no
  qualifying descendant found but process tree reachable), `unknown` (pid not found, tree
  unreadable, or any error).
- **Every error maps to `unknown`** — the caller (lifecycle.Manager.ProbeBackgroundWorkloads)
  already maps errors to `"unknown"`, but the agentctl handler must also not panic or return
  partial data on error.
- **Probe budget**: honour the per-probe timeout (from `KANDEV_BACKGROUND_PROBE_BUDGET`,
  default 5s) as a `context.WithTimeout` wrapping the walk.

## Files to change

### 1. `internal/agentctl/server/process/manager.go`

Expose `AgentPID(acpSessionID string) (pid int, ok bool)`:

```go
// AgentPID returns the PID of the agent subprocess for the given ACP session.
// Returns ok=false when no subprocess is registered for that session.
func (m *Manager) AgentPID(acpSessionID string) (int, bool) {
    // m.cmd is the agent *exec.Cmd — read m.cmd.Process.Pid
    // Guard: only return ok=true when the process is still registered
    // and not yet reaped. Use the existing agentPID() helper if present
    // or read m.cmd.Process directly under the manager mutex.
    ...
}
```

The accessor returns a pid only, not a handle, specifically so the forbidden process-group
predicate is not reachable through it.

### 2. `internal/agentctl/server/api/agent.go` — replace the stub

Update `handleWSBackgroundProbe` to call the real walker:

```go
func (s *Server) handleWSBackgroundProbe(ctx context.Context, msg *ws.Message) *ws.Message {
    var req struct {
        SessionID string `json:"session_id"` // ACP session id
    }
    if err := msg.ParsePayload(&req); err != nil {
        resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{"result": "unknown"})
        return resp
    }

    pid, ok := s.procMgr.AgentPID(req.SessionID)
    if !ok {
        resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{"result": "unknown"})
        return resp
    }

    // Apply budget from config (KANDEV_BACKGROUND_PROBE_BUDGET).
    probeCtx, cancel := context.WithTimeout(ctx, s.cfg.BackgroundProbeBudget)
    defer cancel()

    result := walkProcessTree(probeCtx, pid, s.cfg.TurnStartTime(req.SessionID))
    resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{"result": result})
    return resp
}
```

### 3. New `internal/agentctl/server/process/probe.go` (or `probe_*.go` per platform)

Implement `walkProcessTree(ctx context.Context, rootPID int, turnRefTime time.Time) string`.

The walker:
1. Enumerates all descendants of `rootPID` (children, grandchildren, etc.).
2. For each descendant, reads its process start time.
3. If any descendant started at or after `turnRefTime − 2s` and is still alive → return
   `"live"` immediately (short-circuit).
4. If the walk completes with no qualifying descendant → return `"settled"`.
5. Any error reading the tree (permission denied, proc not found) → return `"unknown"`.

**Platform sources for process start time** (spec §L: these were verified by measurement):

- **Linux**: `/proc/<pid>/stat` field 22 (`starttime`), clock ticks since boot.
  Convert via `sysconf(_SC_CLK_TCK)` and `sysinfo.uptime` to wall-clock time.
- **Darwin**: `sysctl kern.proc.pid.<pid>` → `kinfo_proc.kp_proc.p_starttime` (a
  `timeval`). No `/proc` filesystem on Darwin; test must be gated.
- **Windows**: `GetProcessTimes(OpenProcess(...))` → `FILETIME lpCreationTime`.

Use `//go:build linux`, `//go:build darwin`, `//go:build windows` build tags for the
platform-specific implementations. A fallback file returns `"unknown"` for unsupported
platforms.

For enumerating descendants:
- **Linux**: read `/proc/<pid>/task/<tid>/children` or iterate `/proc` and match `ppid`.
- **Darwin**: `sysctl kern.proc.all` filtered by parent pid.
- **Windows**: `CreateToolhelp32Snapshot` + `Process32Next`.

### 4. Turn reference time

The probe needs to know when the current turn started so it can apply the start-time
predicate. The `process.Manager` (or a session-scoped struct inside agentctl) must track
the most recent `beginPromptTurn` timestamp per ACP session.

Add `RecordTurnStart(acpSessionID string, t time.Time)` to `process.Manager` (or the
session model), called from the `handleWSBackgroundProbe`'s request path — but note that
`turn_started` is what triggers the stamp in the adapter. The agentctl side can maintain
a simple `map[acpSessionID]time.Time` updated on `agent.prompt` arrival (the prompt is
serialized, so no race).

Alternatively, have the probe client send the turn reference time in the request payload
— simpler and avoids agentctl state. Follow whichever approach is cleaner given the
existing `process.Manager` structure.

## Tests to write

**AC-70, AC-70a, AC-71, AC-72, AC-80 must exercise a REAL process tree**, not a stub:

- **AC-70**: spawn a long-lived subprocess after the turn reference time. Probe returns
  `live`. (Unix only — gate with build tags or `t.Skipf`.)
- **AC-70a**: pre-existing long-lived process born before the reference time is not
  counted. Probe returns `settled`.
- **AC-71**: subprocess exits before probe. Probe returns `settled`.
- **AC-72**: no process tree (pid==0 or already reaped). Probe returns `unknown`.
- **AC-80 (Darwin gate)**: on Darwin, start-time is read from `sysctl kern.proc.pid` not
  from `/proc`. Test skips on Linux/Windows.

Use `testing/synctest` where feasible; use real `os/exec` subprocesses for the tree walk.

## Acceptance criteria closed

- **AC-70**: live descendant born after reference time → `"live"`.
- **AC-70a**: pre-existing process not counted.
- **AC-71**: exited descendant → `"settled"`.
- **AC-72**: unreachable tree → `"unknown"`.
- **AC-80**: Darwin start-time source verified.
- **AC-81**: non-positive probe budget rejected (config side in task-02).
