---
created: 2026-11-04
status: done
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Spawned-session binding isolation

## Overview

Keep a spawned helper from becoming the recorded recipient of its spawning
workflow step. One sequential work order carries explicit spawn intent through
launch and replay, then verifies later step routing.

Tasks owns this repair because it owns the conversation selected for each
workflow entry. Agent runtime and workspace ownership do not change.

## Evidence and requirement conformance

[Issue #5045](https://github.com/kdlbs/kandev/issues/5045) reports helper sessions
replacing source-step bindings. Investigation used checkout `5d6adcbca18`.

The failure path is:

1. `mcp/handlers.handleSpawnSession` sends `IntentStart` without `WorkflowStepID`.
2. `launchStart` calls `startTask`, which resolves the current step for launch configuration.
3. `startTask` also uses that fallback step for `recordWorkflowSourceBinding`.
4. `workflowEntryIdentity` assigns the helper the current entry's identity.
5. `UpsertWorkflowSessionBinding` accepts the same operation with a newer task timestamp.
6. `resolveBoundSourceWorkflowSessionWithProfile` later returns the helper.

`beginTaskScheduling` advances task state through the production task repository.
That write supplies a newer timestamp without a workflow re-entry. A fixture
that updates only an in-memory task mock can hide the defect.

A temporary Go overlay reproduced the overwrite using SQL-backed scheduling
updates during `LaunchSession`. Both verified-origin and absent-origin cases
changed `session-a` to the helper.
The later explicit target also resolved to the helper. An unchanged-timestamp
control retained `session-a`, which explains why timestamp-blind tests can pass.
No production or permanent test file changed during diagnosis.

This violates existing
[AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.4](../../specs/tasks/requirements/workflow-profile-session-lifecycle.md).
Criteria 002.6 and 002.8 cover absent recipients and persisted binding behavior.
No new requirement or ADR is needed. The existing requirement and design remain
draft because they also describe broader work. This repair does not change their lifecycle status.

## Scope

### In scope

- Every `spawn_session_kandev` launch, with or without verified sender attribution.
- Same-task spawns and authorized same-workspace cross-task spawns.
- Preserving an existing binding, including its operation and timestamp.
- Preserving an absent binding rather than assigning the helper to that step.
- Retaining the policy through newly recorded ceiling deferrals and replay.
- Normal initial-start and workflow re-entry behavior.
- A focused public explanation of spawned-session routing.

### Out of scope

- New stop tools, helper termination, or changes to completion signals.
- Automatic repair of historical bindings or ambiguous legacy deferred records.
- Changing primary-session policy, profile resolution, or session configuration.
- Changing UI-created sessions, explicit-target route preparation, or Office ownership.
- Schema migrations, portable workflow fields, runtime flags, UI markup, or translations.

## Technical approach

### Mark spawn launches independently of attribution

Add `PreserveWorkflowSourceBinding bool` to `LaunchSessionRequest` with
`json:"-"`. `handleSpawnSession` always sets it to true. A missing or rejected
sender session must not disable the policy. Keep authorization and profile
selection unchanged.

`SpawnOrigin` remains optional attribution. Its absence does not mean that the
launch owns a workflow entry. A new public intent or client-controlled field is unnecessary.

### Preserve the binding at the shared start boundary

Pass the value through `launchStart` into `startTaskOptions`. In `startTask`,
guard the source-binding block with this value. Preserve the existing current-step
fallback for profile and session configuration.

Keep ordinary starts at the false default. Initial manual starts can omit
`WorkflowStepID` and still need a binding. Workflow re-entry must still replace
the source binding under the existing entry ordering rules.

Keep `recordWorkflowSourceBinding` and its SQL ordering unchanged. Equal-entry
ordering cannot identify whether a launch belongs to the workflow entry.

### Retain the policy through replay

`ceiling_seams.go:seam1StartPayload` stores
`preserve_workflow_source_binding`. `ceiling_replay.go:replayCeilingLaunchStart`
restores it into `startTaskOptions`. Missing values retain the false default
for ordinary and older launch records. No schema change is required.

This repair applies the explicit policy to newly recorded spawns. Existing
unattributed records cannot be safely classified from their profile or timestamp.

## Tests

All criterion suffixes below belong to
`AC-TASKS-WORKFLOW-PROFILE-SESSIONS`.

| Planned test | Evidence | Criteria |
| --- | --- | --- |
| `TestHandleSpawnSessionPreservesWorkflowSourceBinding` | Handler forwards the policy for same-task, cross-task, absent, unknown, and mismatched sender sessions | 002.4 |
| `TestLaunchSpawnPreservesWorkflowSourceBinding` | Public launch creates the helper while every field of the source binding remains unchanged | 002.4, 002.8 |
| `TestLaunchSpawnDoesNotCreateWorkflowSourceBinding` | No source binding appears, and later explicit targeting creates a distinct fallback conversation | 002.6 |
| `TestSpawnThenEnterSourceTargetUsesBoundSession` | A later entry sends its unique prompt to the original source conversation, with a live same-profile helper present | 002.4 |
| `TestManualInitialStartRecordsWorkflowSourceBinding` | An ordinary initial start without a supplied step still records its recipient | 002.4 |
| `TestSeam1StartPayloadPreservesWorkflowBindingPolicy` | True and default values survive the persisted payload shape | 002.8 |
| `TestCeilingReplayPreservesWorkflowSourceBinding` | JSON round-trip and actual replay create a helper without changing the binding | 002.4, 002.8 |

Add handler coverage in
`apps/backend/internal/mcp/handlers/spawn_session_workflow_binding_test.go`.
Add launch and later-entry coverage in
`apps/backend/internal/orchestrator/spawn_session_workflow_binding_test.go`.
Add deferred coverage in
`apps/backend/internal/orchestrator/spawn_session_workflow_binding_replay_test.go`.

Reuse `newProfileSwitchFixture`, with SQL-backed task-state updates such as
`orphanSQLTaskStates`. The permanent RED test must exercise a newer timestamp
within the same entry. The later-entry test uses `processOnEnter` and a unique
prompt marker. Synchronize asynchronous dispatch with channels rather than sleeps.

Retain the existing direct-start, effective-source-profile, delayed-entry,
and revisited-source binding tests.

## End-to-end evidence

The Go launch-to-entry scenario covers the affected backend behavior through
persisted session selection and runtime prompt dispatch. Handler coverage proves
that the MCP entry point supplies the policy. Browser layout and interactions do
not change, so this package adds no Playwright scenario or UI preview.

## Work orders

- [x] [Task 01: Preserve workflow bindings for spawned sessions](task-01-preserve-spawn-bindings.md)

Execution is sequential in the primary conversation. No delegation is authorized.
The work order contains the exact regression, lint, documentation, and diff commands.

## Related delivery records

[Workflow session targeting](../workflow-session-targeting/plan.md) delivered
explicit source recipients. Its completed evidence remains historical. This
package owns the missing spawn-isolation regression and does not reopen its work orders.

[Same-profile fresh-session repair](../workflow-same-profile-new-session/plan.md),
[queued session ownership](../queued-session-ownership/plan.md),
[manual workflow session focus](../workflow-session-focus/plan.md), and
[dynamic workflow completion](../dynamic-workflow-completion/plan.md) retain
their existing scopes and results. This repair changes no UI scenario matrix.

## Documentation impact

The implementation adds a short statement under **Spawn a session from an
agent** in `docs/public/coordination.md`: spawning preserves the current step's
recipient for later explicit targets. This page is a how-to guide.
Internal flag and table details stay in the system design.

The documentation search also covered `README.md`, `docs/screenshots.md`,
`sessions-and-review.md`, `agent-communication.md`, `automation-and-mcp.md`,
and `workflow-tips.md`. No screenshot, API schema, or public setting changes are required.

## Verification results

Implemented on 2026-11-04.

Production changes:

- `handleSpawnSession` sets the server-only
  `LaunchSessionRequest.PreserveWorkflowSourceBinding` for every spawn.
- `launchStart` carries the policy into `startTaskOptions`, and `startTask`
  skips only its source-binding write while retaining current-step launch
  configuration.
- `seam1StartPayload` persists `preserve_workflow_source_binding`, and
  `replayCeilingLaunchStart` restores it. Missing values keep the false default.
- `docs/public/coordination.md` states the spawned-session routing outcome.

The permanent regressions reproduce the original overwrite before the guard and
pass after it:

- `TestLaunchSpawnPreservesWorkflowSourceBinding` failed with the helper session
  replacing `session-a` under a newer SQL task timestamp, then passed with every
  binding field unchanged. The shared `newSpawnBindingFixture` saves the binding
  at the same entry operation one hour before the task row, then asserts a real
  SQL task write is newer than that binding and that the entry identity did not
  change. A positive control in the same test launches with the policy off and
  proves the overwrite path is reachable under that setup, so an equal-timestamp
  write can no longer make the preservation assertion pass vacuously.
- `TestSpawnThenEnterSourceTargetUsesBoundSession` failed without the guard
  (the later entry attempted the helper), then passed with the prompt delivered
  to the original source conversation. It uses the same deterministic
  precondition before the entry runs.
- `TestSeam1StartPayloadPreservesWorkflowBindingPolicy` and
  `TestCeilingReplayPreservesWorkflowSourceBinding` failed before the payload and
  replay propagation, then passed. The replay case also asserts the same-entry
  newer-timestamp precondition before claiming preservation.

No regression uses a wall-clock sleep to establish the timestamp condition.

Command results:

- `go test -trimpath -tags fts5 -race ./internal/mcp/handlers -run '^TestHandleSpawnSession' -count=1`: passed.
- Targeted orchestrator regressions (work order command, `-tags fts5 -race`): passed.
- `bash scripts/lint-go-changed <changed files>`: `0 issues`.
- `go build -trimpath -tags fts5 ./...`: passed.
- `go test -trimpath -tags fts5 ./internal/orchestrator ./internal/mcp/handlers -count=1`: both packages passed.
- `python3 scripts/list-docs.py validate`: passed (431 decisions and 1,912 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: passed (63 tests).
- `node scripts/validate-public-docs.mjs`: passed (55 pages).
- Local PR-documentation coverage preflight: `covered`, no errors.
- `git diff --check` and plan whitespace checks: passed.

The earlier 2026-11-04 diagnostic command remains historical evidence. It ran
without build tags and is not part of the verification gate; every permanent
backend test and build command in this package uses `-tags fts5`.

```bash
(cd apps/backend && go test -trimpath -overlay /tmp/kandev-5045-repro-m6b_910_/overlay.json ./internal/orchestrator -run '^TestIssue5045SpawnBindingReproduction$' -count=1 -v)
```

Both origin variants failed the binding-preservation assertion after successful
helper creation. Both also resolved the later explicit target to the helper.
The final diagnostic used `orphanSQLTaskStates` for real scheduling writes.
An earlier unchanged-timestamp control passed both variants. An intervening
explicit task-update experiment also reproduced both failures.

The overlay is temporary session evidence. The permanent regressions do not
depend on this temporary path.

Design-turn validation on 2026-11-04:

- `python3 scripts/list-docs.py validate`: passed (431 decisions and 1,912 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local PR-documentation preflight from the work order: `covered`, with no errors.
- Specification and plan whitespace checks: passed.
- `git status --short -- docs/plans/spawn-session-workflow-binding`: the new package is present and untracked.

The package remains unstaged and uncommitted.

## Risks

- A guard based only on `SpawnOrigin` misses callers without verified attribution.
- A guard based only on an empty step ID breaks ordinary initial starts.
- Missing replay propagation reintroduces the overwrite after deferral.
- An in-memory task-state fixture can conceal the SQL timestamp condition.
- Existing incorrect bindings lack enough evidence for automatic repair.
