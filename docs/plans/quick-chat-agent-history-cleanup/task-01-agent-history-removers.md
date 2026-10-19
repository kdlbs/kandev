---
id: "01-agent-history-removers"
title: "Agent history removers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUICK-CHAT-AGENT-HISTORY-001
  - REQ-TASKS-QUICK-CHAT-AGENT-HISTORY-002
acceptance_criteria:
  - AC-TASKS-QUICK-CHAT-AGENT-HISTORY-001.1
  - AC-TASKS-QUICK-CHAT-AGENT-HISTORY-001.3
  - AC-TASKS-QUICK-CHAT-AGENT-HISTORY-001.4
  - AC-TASKS-QUICK-CHAT-AGENT-HISTORY-002.2
  - AC-TASKS-QUICK-CHAT-AGENT-HISTORY-002.3
system_design:
  - ../../specs/tasks/system-design/quick-chat-agent-history-cleanup.md
---

# Task 01: Agent history removers

## Summary

Create the `agenthistory` package: given a session's history identity, derive and remove the
Claude Code transcript (plus sidecar and empty project dir) or the matching Codex rollout.

## In scope

- `Identity` type and `Remove(ctx, Identity) error` dispatcher.
- Claude remover: root resolution, cwd encoding, exact file/dir removal, empty project-dir
  removal, >200-char skip.
- Codex remover: root resolution, literal traversal of three date-directory levels,
  first-line `session_meta` id and cwd check.
- Path containment and symlink guards; token validation.

## Out of scope

- Task service wiring and eligibility (WO-02).
- Any edit of Claude or Codex shared indexes.

## Acceptance

- Table tests in a temp root: removing session A's Claude transcript leaves session B's
  transcript in the same project dir, another project dir, and a dir whose name contains
  `quick-chat` untouched; the project dir is removed only when empty.
- Codex: a matching rollout is removed; rollouts with a different id, a mismatched cwd, or a
  malformed first line are kept.
- Missing root, missing files, empty or unsafe token, and unknown agent return nil and remove
  nothing; a symlinked transcript or project dir is not followed.

## Verification

From `apps/backend`:

```bash
go test -trimpath ./internal/task/service/agenthistory/... -count=1
```

Manual check against the installed Codex (record the result here): create a throwaway thread in
a temp cwd, delete its rollout, run `codex resume` and confirm the picker and CLI do not fail.

## Files likely touched

- `apps/backend/internal/task/service/agenthistory/agenthistory.go`
- `apps/backend/internal/task/service/agenthistory/claude.go`
- `apps/backend/internal/task/service/agenthistory/codex.go`
- `apps/backend/internal/task/service/agenthistory/*_test.go`

## Dependencies

None.

## Risks

Claude's cwd encoding is observed, not documented. Tests pin the observed format.

## Inputs

- [Requirements](../../specs/tasks/requirements/quick-chat-agent-history-cleanup.md)
- [System design](../../specs/tasks/system-design/quick-chat-agent-history-cleanup.md)
- `apps/backend/AGENTS.md`

## Results

- PR remediation: the original stdlib package passed before new regressions were added.
  The added cases failed on year/month/day symlinks, a bracketed home, a replaced Claude
  project, and an unowned empty project with a colliding encoded cwd. Pinned directory
  handles, literal traversal, and ownership-gated project pruning make all those cases
  and the existing package tests pass in the constrained non-root sandbox. Service and
  full backend verification are hosted separately.
- Further exact-source regressions demonstrated that trimming a config path selected a
  different store, and a relative override selected the default store. Literal absolute
  overrides and skipping unresolved relative overrides make both assertions and the full
  stdlib package pass. A pure extraction addresses the hosted function-complexity failure;
  hosted source-bound proofs 38613208219 and 38620076258 execute the complete
  filesystem package successfully. Repository lint acceptance is recorded in the PR checks.

- `go test -trimpath ./internal/task/service/agenthistory/... -count=1`: pass (tests fail when
  the removers are disabled).
- Contributor-recorded Codex check, codex-cli 0.162.1, isolated `CODEX_HOME` with synthetic rollouts (no login, so
  the interactive `codex resume` picker itself could not be opened): after deleting one
  thread's rollout, app-server `thread/list` (default and all source kinds) returned only the
  remaining thread; `thread/resume` and `codex exec resume <id>` failed with "failed to resolve
  rollout path ... file does not exist"; `thread/read` by explicit id still returned the stale
  metadata and preview from `state_5.sqlite`. No Codex database or index was edited.
