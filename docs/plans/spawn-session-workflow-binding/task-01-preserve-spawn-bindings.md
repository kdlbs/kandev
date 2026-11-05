---
id: "01-preserve-spawn-bindings"
title: "Preserve workflow bindings for spawned sessions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.4
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.8
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 01: Preserve workflow bindings for spawned sessions

## Summary

Carry explicit spawn intent from the MCP handler through launch and deferred
replay. Preserve the source-step binding while the helper starts and while
later workflow entries select their recipient.

## In scope

- Add server-only `LaunchSessionRequest.PreserveWorkflowSourceBinding` with `json:"-"`.
- Set it for every `handleSpawnSession` request, independently of optional attribution.
- Pass it through `launchStart` into `startTaskOptions`.
- Guard the source-binding block in `startTask` while retaining launch configuration fallback.
- Store and restore `preserve_workflow_source_binding` in ceiling start payloads.
- Add focused handler, launch, later-entry, and replay regressions from the plan.
- Clarify spawned-session routing in `docs/public/coordination.md` after the correction.

## Out of scope

- SQL ordering changes, migrations, or repair of historical bindings.
- New session-stop tools or changes to current session-primary behavior.
- UI markup, profile selection, explicit-target route preparation, or Office ownership.

## Acceptance

1. Every new MCP spawn preserves the existing source-binding row or its absence.
   Coverage includes same-task, same-workspace cross-task, and missing or invalid sender attribution.
2. A later explicit source target sends its prompt only to the eligible source
   conversation. A live same-profile helper cannot replace that conversation.
   An absent source binding still uses the documented fresh-session fallback.
3. Newly persisted deferrals retain the policy through JSON decoding and replay.
   Ordinary implicit initial starts and genuine source-step re-entry still record their selected sessions.

## TDD sequence

1. Mark this work order `in_progress` and synchronize the plan.
2. Add `TestLaunchSpawnPreservesWorkflowSourceBinding` before production changes.
   Use real SQL task-state updates during `LaunchSession`, not only the in-memory task mock.
   Verify that the helper replaces the binding within the same entry before the correction.
   Enable preservation in the request. Add only its field declaration if compilation requires it.
   Keep the binding guard absent for behavioral RED.
3. Add the handler propagation, absent-binding, later-entry, ordinary-start, and replay tests.
4. Implement the shared guard and its trusted launch policy.
5. Run the commands below and record their actual results.
6. Update the public explanation, then mark this work order `done` and synchronize the plan.

The confirmed root cause and temporary reproduction are in
[Evidence and requirement conformance](plan.md#evidence-and-requirement-conformance).
The permanent regression must fail on the recipient assertion, not a missing field or fixture error.

## Verification

Run each command from the repository root. These commands are the implementation gates.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/mcp/handlers -run '^TestHandleSpawnSession' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator -run '^(TestLaunchSpawn.*WorkflowSourceBinding|TestSpawnThenEnterSourceTargetUsesBoundSession|TestManualInitialStartRecordsWorkflowSourceBinding|TestSeam1StartPayloadPreservesWorkflowBindingPolicy|TestCeilingReplayPreservesWorkflowSourceBinding|TestStartTaskRecordsDirectWorkflowSourceBindingBeforeLaunch|TestRecordWorkflowSourceBinding.*|TestRevisitedSourceBindingBarrierPreservesLaterTargetSelection|TestWorkflowSessionTargetUsesTaskEffectiveSourceProfile)$' -count=1)
bash scripts/lint-go-changed \
  apps/backend/internal/mcp/handlers/spawn_session.go \
  apps/backend/internal/mcp/handlers/spawn_session_workflow_binding_test.go \
  apps/backend/internal/orchestrator/session_launch.go \
  apps/backend/internal/orchestrator/task_operations.go \
  apps/backend/internal/orchestrator/ceiling_seams.go \
  apps/backend/internal/orchestrator/ceiling_replay.go \
  apps/backend/internal/orchestrator/spawn_session_workflow_binding_test.go \
  apps/backend/internal/orchestrator/spawn_session_workflow_binding_replay_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short -- docs/plans/spawn-session-workflow-binding
```

For package coverage, run the local validator below from the repository root.
It validates the planned application paths as well as the new work order.
It makes no GitHub calls and does not stage files.

```bash
node <<'NODE'
const fs = require('node:fs');
const assert = require('node:assert/strict');
const {validateCoverage} = require('./.github/scripts/pr-docs.cjs');
const workOrder = 'docs/plans/spawn-session-workflow-binding/task-01-preserve-spawn-bindings.md';
const paths = [
  workOrder,
  'docs/plans/spawn-session-workflow-binding/plan.md',
  'docs/specs/tasks/requirements/workflow-profile-session-lifecycle.md',
  'docs/specs/tasks/system-design/workflow-profile-session-lifecycle.md',
];
const applicationPaths = [
  'apps/backend/internal/mcp/handlers/spawn_session.go',
  'apps/backend/internal/orchestrator/session_launch.go',
  'apps/backend/internal/orchestrator/task_operations.go',
  'apps/backend/internal/orchestrator/ceiling_seams.go',
  'apps/backend/internal/orchestrator/ceiling_replay.go',
];
const result = validateCoverage({
  changedFiles: [...paths, ...applicationPaths],
  fileContents: Object.fromEntries(paths.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
assert.equal(result.ok, true);
assert.equal(result.status, 'covered');
assert.deepEqual(result.workOrders, [workOrder]);
NODE
```

No browser layout changes or new Playwright dependency are required.
The launch-to-entry integration test supplies the recipient and prompt evidence.

## Files likely touched

- `apps/backend/internal/mcp/handlers/spawn_session.go`
- `apps/backend/internal/mcp/handlers/spawn_session_workflow_binding_test.go` (new)
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/ceiling_seams.go`
- `apps/backend/internal/orchestrator/ceiling_replay.go`
- `apps/backend/internal/orchestrator/spawn_session_workflow_binding_test.go` (new)
- `apps/backend/internal/orchestrator/spawn_session_workflow_binding_replay_test.go` (new)
- `docs/public/coordination.md`
- This work order and `plan.md` for status and results.

## Dependencies

None.

## Risks

- `SpawnOrigin` can be nil for an authorized spawn.
- The timestamp condition requires SQL-backed task-state writes in regression fixtures.
- Missing replay propagation loses the trusted policy.
- Previously incorrect bindings remain unchanged by this repair.

## Parallelism

`sequential`

## Inputs

- [Recipient requirement](../../specs/tasks/requirements/workflow-profile-session-lifecycle.md), requirement 002, criteria 002.4, 002.6, and 002.8.
- [Recipient bindings design](../../specs/tasks/system-design/workflow-profile-session-lifecycle.md#source-step-bindings).
- `orchestrator/event_handlers_workflow_profile_session_policy_test.go:newProfileSwitchFixture`.
- `orchestrator/scheduling_orphan_interruption_test.go:orphanSQLTaskStates`.
- `orchestrator/event_handlers_workflow_same_profile_policy_test.go:TestProcessOnEnter_SameProfileNewPromptRecipient`.
- `orchestrator/ceiling_deferred_settlement_test.go` for payload and actual replay patterns.
- `mcp/handlers/spawn_session_test.go` for handler fixtures and origin cases.
- `.agents/skills/tdd/references/backend-tests.md` for asynchronous dispatch and cleanup.

## Results

Implemented on 2026-11-04.

- `LaunchSessionRequest.PreserveWorkflowSourceBinding` is set by every
  `handleSpawnSession` launch, including spawns without verified attribution.
- `launchStart` carries it into `startTaskOptions`; `startTask` skips only the
  source-binding write and keeps current-step launch configuration intact.
- `seam1StartPayload` stores `preserve_workflow_source_binding` and
  `replayCeilingLaunchStart` restores it; absent values keep the false default.
- Added handler, launch, absent-binding, later-entry, ordinary-start, payload,
  and replay regressions. The launch and later-entry tests failed before the
  guard and pass after it. A shared fixture saves the source binding one hour
  before the task row, asserts a real SQL task write is newer than that binding,
  and asserts the workflow entry identity is unchanged; a positive control with
  the policy off proves the overwrite path remains reachable under that setup.
  No regression relies on a wall-clock sleep.
- Updated the spawn routing explanation in `docs/public/coordination.md`.

Verification passed: the work-order Go test commands with `-tags fts5 -race`,
`bash scripts/lint-go-changed` (`0 issues`), `go build -trimpath -tags fts5 ./...`,
full `internal/orchestrator` and `internal/mcp/handlers` package tests with
`-tags fts5`, the four documentation validators, the local PR-documentation
coverage preflight (`covered`), and `git diff --check`. Exact evidence is in the
[plan verification results](plan.md#verification-results).
