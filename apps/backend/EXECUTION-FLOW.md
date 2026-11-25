## Execution Flow

```text
Client (WS) → Orchestrator → Lifecycle Manager → ExecutorBackend (container/process) → agentctl
                                                                                          ↓
Client (WS) ← Orchestrator ← Lifecycle Manager ←──── stream updates (WS) ──────── agent subprocess
```

1. Orchestrator receives `session.launch` via WS
2. Lifecycle Manager creates executor instance (container or process)
3. agentctl starts inside the instance, agent subprocess is configured and started
4. Agent events stream back via WS through the chain

**Session Resume:** `TaskSession.ACPSessionID` stored for resume; `ExecutorRunning` tracks active state; on restart `RecoverInstances()` reconnects.

**Provider Pattern:** Packages expose `Provide(cfg, log) (*impl, cleanup, error)` for DI. Returns implementation, cleanup function, and error. Cleanup called during graceful shutdown.

**Worktrees:** `internal/worktree/Manager` provides workspace isolation. Each session can have its own worktree (branch) to prevent conflicts between concurrent agents.

**Worktree file materialization:** `copy_files` (`Repository.CopyFiles`) is a comma-separated spec of repository-relative gitignored paths/doublestar globs seeded into each new worktree. Copy is the default; the exact terminal `:symlink` suffix (for example `.env.local:symlink`) creates a relative host-worktree link so source changes propagate live. Other colons stay literal (`config:dev`, `.env:`); use `::symlink` to copy a literal path ending in `:symlink`. Malformed reserved syntax is rejected at save time. Parsing and materialization live in `internal/worktree/copyfiles/` (`ParseSpecs`, `ValidateSpec`, `Copy`); duplicates and overlapping matches are first-entry-wins. `Manager.copyConfiguredFiles` runs before setup during worktree creation. Source and destination containment checks reject traversal and symlinked destination parents. Failures are non-fatal warnings. Windows link creation is best-effort. Remote executors cannot link to the host, so `Parse`/`Plan` preserve literal colon paths but turn symlink-mode entries into copied bytes delivered through `WriteEntries`.

**Executor default scripts:** Default prepare scripts are in `internal/agent/runtime/lifecycle/default_scripts.go`; `internal/scriptengine/` handles placeholder resolution.

- **Launcher quoting:** Quote interpolated environment values for POSIX/Git Bash
  versus native Windows; when launch recipes change, extend
  `scripts/check-make-shells` with default, spaced, and quote-containing values.
