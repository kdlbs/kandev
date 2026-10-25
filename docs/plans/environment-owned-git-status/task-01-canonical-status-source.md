---
id: "01-canonical-status-source"
title: "Select canonical Git-status sources"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/additional-session-workspace-reuse.md"
design: "../../specs/tasks/system-design/environment-owned-git-status.md"
---

# Task 01: Select Canonical Git-Status Sources

## Goal

Make session subscription and explicit Git refresh project current status only
from a session and execution bound to the task environment's canonical
workspace.

## Scope

- Add focused source-selection helpers in
  `apps/backend/internal/backendapp/helpers.go` and
  `apps/backend/internal/backendapp/git_status_sources.go`.
- Resolve the requested session's task environment and every session bound to
  that environment, including inherited sessions from another task.
- Validate session and live-execution workspace identity before a live query.
- Batch-load eligible snapshots and select the newest observation per
  repository after live status is unavailable.
- Route the selected result with the requested subscription session ID.
- Add bounded debug logs for source choice and rejection reason.

## Exclusions

- Do not change workspace recovery or resume path selection from PR #3167.
- Do not move snapshots to a task-environment table.
- Do not expose workspace paths in logs or WebSocket payloads.
- Do not prefer dirty or non-empty status.

## Requirements

- `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001`
- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.5`

## RED regression

Add backend tests that seed two sessions with one task-environment ID and two
different workspace paths. The requested sibling points at the non-canonical
path. The canonical sibling has a dirty snapshot.

Before production changes, prove that hydration returns the requested sibling's
clean snapshot. Also cover a mismatched live execution so it cannot bypass the
snapshot authority rule.

Suggested test names:

- `TestAppendLiveGitStatusMessageSelectsCanonicalEnvironmentSnapshot`
- `TestAppendLiveGitStatusMessageRejectsMismatchedLiveExecution`

## Acceptance

- A mismatched sibling cannot supply live or persisted current status for the
  shared environment.
- A canonical sibling can supply status while the outgoing event remains routed
  to the requested subscription session.
- A lookup error or an environment with no eligible source emits no suspect
  status and does not fall back to the mismatched session.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/backendapp -run 'TestAppendLiveGitStatusMessage.*(Canonical|Mismatched)' -count=1
```

## Files likely touched

- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/git_status_sources.go`
- `apps/backend/internal/backendapp/helpers_git_status_test.go`
- `apps/backend/internal/task/repository/sqlite/session.go`

## Dependencies

None. Rebase on PR #3167 before delivery if it merges first.

## Output contract

Report RED and GREEN evidence, the source-selection order, changed files, test
results, and residual compatibility behavior for sessions without an
environment. Update this work order and `plan.md` in the implementation turn.

## Results

- RED evidence: the new canonical snapshot test stored no status, and the
  mismatched-live test selected the wrong live status before the production
  changes.
- GREEN evidence: the targeted canonical and mismatched-source test command
  passed 3 tests.
- Source order: a matching live execution is preferred; when live status is
  unavailable, the newest timestamped snapshot from canonical-bound sessions
  is selected. The requested session ID remains on the published event.
- Environment-owned hydration fails closed when the task environment is
  missing, mismatched, or has no eligible source. Sessions without an
  environment retain the existing session-scoped behavior.
- The fixup adds raw workspace provenance checks, environment-wide source
  enumeration across task boundaries, recovered-execution handling, a single
  live-probe deadline, and per-repository snapshot fallback.
- `go test -tags fts5 ./internal/backendapp -count=1` passed 754 tests, and
  `make -C apps/backend build` passed.
