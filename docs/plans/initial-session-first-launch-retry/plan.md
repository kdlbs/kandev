---
created: 2026-11-27
status: implemented
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
system_design:
  - ../../specs/tasks/system-design/initial-target-first-launch-retry.md
  - ../../specs/agents/system-design/recovery-workspace-availability.md
legacy_specs: []
---

# Implementation Plan: Initial-session retry after a first-launch worktree failure

## Overview

Fixes [#5555](https://github.com/kdlbs/kandev/issues/5555). Suppose a task
starts on an auto-start step with `session_target: {"kind":"initial"}` and its
first worktree preparation fails. The task then cannot recover. The automatic
retry and **Start fresh session** are both refused before a second checkout
starts. The recovery card also offers a read-only restore that cannot succeed,
and it labels the fresh-start refusal **Failed to resume session**.

Two sequential work orders. Task 01 (tasks system) corrects recovery admission.
Task 02 (agents system) corrects the recovery card.

## Evidence and root cause

Checkout `5fbc6dbeeb50`.

1. Task creation prepares the first session through `launchPrepare` →
   `PrepareTaskSession` with the workspace. This path writes no
   `workflow_session_route`. Routes are written by `startTask`'s explicit-route
   path, which only runs when the agent starts. When the worktree fetch times
   out during preparation, the task keeps a failed session, a pending
   `initial_prompt_submission`, and its `workflow_initial_session` snapshot,
   but no route.
2. `runtime_retry` without a resume token is normalized to `fresh_start`
   (`normalizeSessionRecoveryAction`). Both actions reach
   `RecoverSessionWithOptions` → `prepareInitialRecoveryPrompt` →
   `initialRecoveryWorkflowBinding`. That function derives a binding without
   a route and clears `DestinationSessionID`, because a failed session counts
   as terminal for destination checks.
3. `validateClaimedCeilingBinding` (kind `resume`) → `validateCeilingEntry` →
   `validateUnroutedWorkflowEntry` (`ceiling_entry.go:925`). That function
   returns `PermanentlyUnavailable: "workflow entry route is unavailable"`
   for any step with a `SessionTarget`. Recovery fails with
   `ceiling deferred workflow entry was superseded: workflow entry route is unavailable`
   before any launch.

Reproduction, run during diagnosis and then removed: use
`newFreshStartWorkflowFixture` with a receipt, set the step's
`SessionTarget` to `initial`, and call `recover("fresh_start")`. The result was
the exact error with `launchCount == 0`. A temporary prototype accepted an
initial target without a route when the snapshot named the session. With it,
the same test passed: one launch, and the prompt kept the step instructions.
That prototype was reverted.

UI root causes (`apps/web`):

- `appendReadOnlyRestoreChoice` (`session-recovery-model.ts:256`) decides from
  `recoveryError` / bootstrap phase only. Workspace existence is never part of
  the decision. The backend then rejects restore with
  `task environment is not attachable` (`manager_workspace_admission.go:153`).
- `use-session-recovery-actions.ts:300` records every failed `session.recover`
  as `operation: "resume"`, so `recoveryFailureCopy` and
  `stoppedRecoveryCause` show **Failed to resume session** for a fresh-start
  refusal.

## Requirement conformance

| Defect | Classification | Criteria |
| --- | --- | --- |
| Initial-target retry refused before route commit | Missing requirement: added the smallest AC | `AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.13` (new) |
| Restore offered with no checkout | Violates an active AC | `AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.3`, `006.12`, `006.18` |
| Fresh-start failure titled as resume | Violates an active AC | `AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.2`, `006.12`, `006.28` |

System-design updates in this package:

- New `tasks/system-design/initial-target-first-launch-retry.md`: the rule for
  an initial target without a route. It extends the workflow profile session
  lifecycle design, which is at its size limit.
- New `agents/system-design/recovery-workspace-availability.md`: the
  `workspace_restorable` and operation-label rules. It extends session recovery
  failures, which is at its size limit.

## Scope

### In scope

- Accepting an initial-target entry without a route, fenced by the snapshot
  session, workflow, step, entry identity, and operation ID.
- A fresh-start check that the recovering session is the snapshot session.
- The `workspace_restorable` failure metadata, and hiding restore with an
  explanation when it is `false`.
- Operation-specific titles for failures of `fresh_start` and `runtime_retry`.

### Out of scope

- Writing a route during `launchPrepare`. That would change route ownership
  for every prepared task. This repair only needs the snapshot fence.
- Earlier-step (`kind: step`) targets. They keep requiring a committed route.
- Fetch timeout values or **Pull before worktree** behavior.
- Backfilling or repairing existing stuck tasks. They recover once the fix
  ships, because their state already matches the accepted shape.
- New recovery actions (memory: simple recovery UX).

## Technical approach

### Backend (Task 01)

In `validateUnroutedWorkflowEntry`, replace the unconditional
`step.SessionTarget != nil` refusal:

- `kind != initial` → unchanged refusal.
- `kind == initial` → load `models.LoadWorkflowInitialSessionSnapshot(task.Metadata)`.
  Refuse with the same detail if it is missing or has an empty `SessionID`. Take
  the destination as `binding.DestinationSessionID`, falling back to
  `sessionIDFromCeilingPayload(deferral)`. If that is non-empty and differs
  from the snapshot session, return `ceilingEntrySuperseded`
  ("workflow destination session changed"). Otherwise continue to the existing
  workflow, step, entry-identity, operation-ID, and destination checks.

In `initialRecoveryWorkflowBinding`, add a check that runs only for an
initial-target step with no route present. If the snapshot session is not
`sessionID`, return `ErrCeilingLaunchSuperseded` ("workflow destination session
changed"). This keeps the `DestinationSessionID` clear (needed because the
failed session is terminal) without giving up the session fence.

Extract a small helper such as `unroutedInitialTargetDisposition` to stay
within the cyclomatic limit.

### Frontend and metadata (Task 02)

- Backend: in `event_handlers_agent.go`, next to `"recovery_actions": true`,
  set `meta["workspace_restorable"]` when the session's task environment can
  be read. The value uses the same rule as workspace admission: the status is
  `""`, `ready`, or `stopped`, and `WorkspacePath != ""`. A read error leaves
  the key out.
- Web: `appendReadOnlyRestoreChoice` returns early when
  `model.metadata?.workspace_restorable === false`. The card adds a localized
  prerequisite line, `task:workspaceRestoreUnavailable`.
- Web: `use-session-recovery-actions.ts` records `operation: "fresh_start"`
  when the failed action is `fresh_start` or `runtime_retry`. Add
  `task:failedToStartFreshSession` and use it in `recoveryFailureCopy`,
  `stoppedRecoveryCause`, and `action-message-recovery.tsx`. The sanitized
  backend message stays in the details disclosure.

## UI preview

Before (desktop and phone):

```text
┌ Agent startup failed ────────────────────────────────────┐
│ Worktree setup failed                                    │
│ [Start fresh session] [Restore read-only workspace]      │
│ Failed to resume session                                 │
│ ▸ Details                                                │
└──────────────────────────────────────────────────────────┘
```

After, when the retry is refused for some other reason:

```text
┌ Agent startup failed ────────────────────────────────────┐
│ Worktree setup failed                                    │
│ Workspace restore is unavailable right now.              │
│ [Start fresh session]                                    │
│ Failed to start fresh session                            │
│ ▸ Details  (sanitized backend cause)                     │
└──────────────────────────────────────────────────────────┘
```

On phone the content is the same, with stacked full-width buttons at least
44 px tall. With Task 01 in place, **Start fresh session** normally succeeds
and the card retires.

## Tests

| Test | Level | Evidence | Criteria |
| --- | --- | --- | --- |
| `TestFreshStartInitialTargetWithoutRoute` (new file `fresh_start_initial_target_test.go`) | Go service, SQLite | Table: `fresh_start` and `runtime_retry` → one launch, receipt content and step prompt delivered; snapshot names another session → superseded, 0 launches; `kind: step` target without route → still refused | 002.13 |
| `TestValidateUnroutedInitialTarget` (same file) | Go unit | Changed step or entry identity → superseded; missing snapshot → permanently unavailable | 002.13 |
| `TestSessionFailureMetadataWorkspaceRestorable` (new `event_handlers_agent_recovery_workspace_test.go`) | Go, SQLite | Empty, ready, and stopped statuses with a path → `true`; creating, failed, or missing path → `false`; read error → key absent | 005.3 |
| `session-recovery-model.test.ts` (extend) | Vitest | `workspace_restorable: false` → no restore choice and localized prerequisite; absent → unchanged; failed fresh start uses its own label | 006.2, 006.12, 006.18, 006.28 |
| `use-session-recovery-actions.test.ts` (extend) | Vitest | Failed `fresh_start` and `runtime_retry` record operation `fresh_start` | 006.2, 006.28 |
| `action-message-recovery.test.tsx` and `session-stopped-banner.test.tsx` (extend) | Vitest | Failed fresh start uses the operation-specific label on both recovery surfaces | 006.2, 006.28 |
| `mobile-workspace-unavailable-recovery.spec.ts` (new) | Mobile Playwright E2E | The phone card keeps the prerequisite after reload, hides restore, keeps fresh start touch-sized, and has no horizontal overflow | 005.3, 006.18 |

Write the initial metadata, recovery-model, and hook unit tests first, then
confirm their expected failures before changing production code. The mobile
E2E seeds persisted failure metadata after implementation; it does not depend
on a fetch timeout.

## Work orders

Run sequentially: Task 02's fresh-start label is only observable once Task 01
stops the generic refusal.

- [x] [Task 01: Admit initial-target retry before route commit](task-01-initial-target-unrouted-retry.md)
- [x] [Task 02: Recovery card workspace and operation causes](task-02-recovery-card-causes.md)

## Commands

```bash
cd apps/backend
go test -trimpath ./internal/orchestrator -run 'TestFreshStart|TestValidateUnrouted|TestCeiling|TestSessionFailureMetadata|TestRecover' -count=1
go test -trimpath -race ./internal/orchestrator -run 'TestFreshStartInitialTargetWithoutRoute' -count=1
cd ../.. && bash scripts/lint-go-changed \
  apps/backend/internal/orchestrator/ceiling_entry.go \
  apps/backend/internal/orchestrator/initial_recovery_prompt.go \
  apps/backend/internal/orchestrator/fresh_start_initial_target_test.go \
  apps/backend/internal/orchestrator/event_handlers_agent.go \
  apps/backend/internal/orchestrator/event_handlers_agent_recovery_workspace_test.go
cd apps && pnpm --filter @kandev/web test -- session-recovery use-session-recovery-actions action-message-recovery session-stopped-banner
pnpm --filter @kandev/web typecheck && pnpm --filter @kandev/web lint
cd web && pnpm run i18n:check && pnpm run i18n:pseudo
pnpm e2e:run --project mobile-chrome e2e/tests/session/mobile-workspace-unavailable-recovery.spec.ts
cd ../.. && python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

## Risks

- Loosening validation for every explicit target. Keep the change limited to
  `kind == initial` and fence it with the snapshot session.
- A stale ceiling deferral for an initial target replaying into a non-original
  session. The payload-session comparison refuses it.
- `workspace_restorable` drifting from the admission rule. Share the status
  set, or name it in one place.
- Locale coverage: add both keys to every locale under `apps/web/src/locales`,
  regenerate `pseudo`, and use no em dashes.

## Documentation impact

No public docs change. The recovery actions and workflow settings are unchanged.
