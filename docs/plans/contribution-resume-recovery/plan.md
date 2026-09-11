---
created: 2026-09-11
status: completed
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-002
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
system_design:
  - ../../specs/tasks/system-design/remote-contribution-tasks.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
  - ../../specs/agents/system-design/session-recovery-failures.md
legacy_specs: []
---

# Implementation Plan: Contribution Resume Recovery

## Overview

Allow an existing contribution session to resume after a history-only push
rejection. Restore eligible workspace access after a real startup failure.
Show one cause-specific recovery card with accessible actions and details.

Work sequentially: contribution admission, workspace restoration, durable
failure projection, shared presentation, then rendered regression evidence.
All five work orders are complete, including desktop and mobile browser
regressions.

## Confirmed root cause

The reported [task](https://kandev.cfl.tools/t/80025f8f-68c7-47e4-99c8-fee9211476ab)
uses session `5105d361-3a85-4de7-94ad-840045aa8255`.
Backend build `407ed4f1a5` matches the inspected checkout.

At 2026-09-11 10:49:25 UTC, resume promoted a workspace-only execution.
At 10:49:27 UTC, `StartAgentProcess` failed in contribution push preflight.
The dry run targeted the contributor source branch and returned `fetch first`.
The remote contained history absent from local HEAD. This was not an actual
publication attempt, and does not identify who changed the remote or prove
whether it advanced linearly or was rewritten.

`GitOperator.PushPreflight` returns an unsuccessful result for any rejected
push. `preflightRemoteContributionPushes` turns it into a fatal error before
agent startup. Its empty default-repository routing key becomes the misleading
display label `repository ""`.

The asynchronous startup failure path forwards raw error text to the failure
transition. `handleAgentStartFailed` suppresses a toast for resume but does not
make an ordinary Git rejection a typed launch failure. The transcript and
stopped-session controls therefore render it independently.

Automatic fallback then calls `launchRestoreWorkspace`. This intent explicitly
supports terminal sessions, but `ensureLaunchSessionStillActive` rejects
`FAILED` during execution registration. The log at 10:49:27.894 UTC records
that rejection. It is labeled as shutdown although the recorded reason is
terminal-session admission; do not infer an actual shutdown from that label.

The screenshot confirms three visible surfaces. This is a read-only source/log
trace, not a fresh browser reproduction. The backend diagnostic bundle
`813dcf465916c8d583cf823ce0a5533b` is partial because of its archive byte limit.
The relevant failure sequence is present in `backend-logs.log`; no claim is
made about omitted history. Raw logs and conversation data do not belong in
this package.

## Requirement conformance and ownership

Tasks own contribution identity/admission and durable launch failure records.
Agents own workspace recovery eligibility and session recovery presentation.
These are existing independent contracts, not separate frontend/backend specs.

- Contribution requirement 002 qualifies admission after remote drift, while
  requirement 001 preserves user-controlled version replacement.
- Launch requirement 001.2/.7/.8 already requires safe, valid recovery.
  New .11/.12 extend that contract explicitly to asynchronous startup.
- Agent recovery requirements 005/006 specify terminal workspace admission and
  presentation arbitration. Existing 001/002/003/004 retain provider identity,
  dual causes, explicit branch replacement, and archive/navigation guards.
- [ADR: resume preflight](../../decisions/2026-09-11-contribution-resume-preflight.md)
  records the proposed qualification to the existing credential preflight rule.

## Recent related work

[PR #3437](https://github.com/kdlbs/kandev/pull/3437), commit `3fba828e2f`,
implemented [launch error consolidation](../task-launch-error-consolidation/plan.md).
It handles typed launch records, while this asynchronous path lacks that record.

[PR #3519](https://github.com/kdlbs/kandev/pull/3519), commit `258bfabf18`,
implemented [archived session recovery](../archived-session-recovery/plan.md).
It preserves dual causes but leaves an outer recovery surface independently
mounted. Keep its archive and stale-request scenarios. Completed packages and
their historical test counts remain unchanged; this package owns new evidence.

## Scope

In scope: exact history-only preflight classification on resume; eligible
workspace-only registration; safe correlated bootstrap errors; single recovery
presentation in task detail, preview, and Quick Chat; localization and focused
desktop/phone regressions.

Out of scope: fixing the contributor branch, automatic pull/rebase/reset/push,
new merge strategies, relaxing credential scope, changes to initial contribution
creation, blanket suppression of runtime errors, and unrelated PR sync failures.
No live task mutation, new background poller, runtime flag, or database table.

## Technical approach

### Preflight contract

Add an optional typed preflight reason to the existing Git result and runtime
client DTO. Keep unsuccessful push semantics. Use exact porcelain destination
and reason matching in a controlled locale, never broad stderr substring
matching. Only explicit resume intent can admit `history_update_required`.
Test cold resume and promotion independently; an existing token is not the
only possible resume path. Unknown/legacy agentctl responses remain blocking.

### Workspace admission

Pass an internal execution purpose into both registration checks. Preserve
terminal rejection for agent launches. Workspace-only creation validates live
task/session/environment ownership, archive and cleanup state, without changing
terminal state. Repeat checks after durable registration and on reuse paths.
Retain singleflight, exact-resource rollback, and existing credential policy.

### Durable error and UI contract

Extend existing `last_agent_error`/active-error projections with optional
bootstrap phase and attempt correlation. Use existing stamps and execution
fences rather than a parallel store. Keep safe reason codes separate from
display strings. Persist before publishing a failed state.

Implement the optional phase, execution, attempt, and cause fields defined in
the [task launch design](../../specs/tasks/system-design/task-launch-failure-recovery.md#asynchronous-startup-amendment).
That design owns field names, reason codes, and projection bounds. No schema
migration or raw command output persistence is needed.

Carry the error stamp/attempt in request errors and synthetic error metadata.
The shared view model combines durable failure, current request state, and
existing valid actions. Do not use text equality as ownership proof.
A fallback failure updates the matching attempt only. Initial-session ensure
errors and unrelated historical/runtime failures remain available.

## ASCII UI preview

### UI-01: Task Chat, blocked resume
Entry: selected task/session, automatic or manual resume failed.
The supplied screenshot shows three presentations of one failure:
```text
BEFORE
[Top: Session recovery failed                 Retry]
[Transcript: Agent has encountered an error + Git output]
[Composer: same Git output       Resume | Start fresh]

AFTER / DESKTOP
[Task header and session tabs]
[Transcript history ...]
[Could not resume the session]
[Source repository access could not be verified.]
[Retry resume] [Restore workspace] [More options]
[> Details]
[Composer reflects stopped state, no duplicate warning]
```
The cause above illustrates an access failure; confirmed history-only rejection
instead follows UI-03. Never claim access denial without matching evidence.

### UI-02: Phone Chat, blocked resume
```text
[Back] [Task / session]
[Transcript history ...]
[Could not resume the session]
[Short cause, wrapping]
[ Retry resume            ]
[ Restore workspace       ]
[ More options            ]
[ > Details               ]
[Safe-area clearance]
```
More options retains valid existing fresh-start and branch-loss recovery flows;
fresh start is secondary and keeps its existing confirmation. No new drawer is
needed for the card. Use the existing responsive menu for secondary choices.

### UI-03: Successful resume after remote history update
```text
[Task / session]
[Transcript and active composer]
[Changes: existing provider/local history state and version actions]
```
No agent-error card or recovery banner is produced by the history-only rejection.

### UI-04: Pending, details, and fallback states
```text
PENDING: [Resuming...] [Restore workspace disabled] [> Details]
EXPANDED:
  Resume: <safe cause>
  Workspace restore: <safe cause, only if attempted>
RESTORE SUCCEEDED:
  [Workspace available. Agent remains stopped.]
  [Retry resume] [> Details]
RESUME SUCCEEDED:
  [Normal transcript and composer; active card removed]
```
The card stays inline in the transcript scroll owner. Details wrap without
their own scroller. Existing phone layout owns dynamic viewport and safe-area
spacing. Desktop buttons measure 28px; phone/coarse-pointer targets are at least
44px. Control order, one-owner behavior, scroll ownership, and state changes
are required. Exact wording and spacing are illustrative and must use localized
keys. UI-01/02/04 map to recovery requirement 006; UI-03 maps to contribution
requirement 002.

## Tests

Every work order starts with a behavioral RED regression before production edits.
New test names below are proposed; listed source files exist unless marked new.

| Criteria | Regression evidence |
| --- | --- |
| Contribution 002.1-.5 | New `TestPushPreflightHistoryClassification` in process Git tests; new lifecycle `TestContributionResumePreflight` covers cold/promotion and mixed repositories |
| Recovery 005.1-.4 | New `TestWorkspaceRestoreTerminalAdmission` covers failed/completed/cancelled, reuse, cleanup/archive races, and one runtime |
| Launch 001.11-.12 | Executor and orchestrator startup tests cover safe persistence, event ordering, successor execution, and existing provider routes |
| Recovery 006.1-.5 | Shared view-model/component tests cover correlation, one owner, reversed events, reload data, pending, dual causes, and unrelated history |
| Recovery 006.6 | Desktop/mobile rendered geometry, keyboard/touch, translated labels, and details expansion |

## E2E tests

Extend `session/session-resume-recovery.spec.ts` and its mobile counterpart.
Reuse `helpers/session-resume-recovery.ts` and existing API fixtures. Include
one real local Git fixture with a second clone advancing the source branch;
backend tests own full Git classification. Browser fixtures may seed typed
blocking failures for deterministic presentation tests, but must not claim
those fixtures prove backend admission.

Run existing launch-failure and archived-session specs alongside these changed
specs. Assert one active card, no matching duplicate, valid retry payload,
pending latch, successful resume, workspace-only success, labeled dual failure,
reload/reconnect, and unrelated history. Phone tests use `mobile-chrome`.

## Work orders

- [x] [01: Classify contribution resume preflight](task-01-contribution-preflight.md) (completed)
- [x] [02: Restore terminal-session workspace access](task-02-workspace-admission.md) (completed)
- [x] [03: Persist correlated bootstrap failures](task-03-bootstrap-projection.md) (completed)
- [x] [04: Unify recovery presentation](task-04-recovery-presentation.md) (completed)
- [x] [05: Prove recovery flows in the browser](task-05-rendered-regressions.md) (completed)

## Documentation impact

Updated `docs/public/tasks-and-workflows.md` and
`docs/public/sessions-and-review.md` with verified recovery-card behavior,
read-only workspace restoration semantics, and history-only contribution
resume guidance. The guidance preserves actual Git mutation safeguards.

## Verification results

Implementation validation passed on 2026-09-11. Backend regression commands,
the full lifecycle package, backend builds, focused frontend tests, typecheck,
lint, localization gates, Vite production builds, and the planned desktop and
mobile browser suites all passed. The desktop suite passed 8 tests and the
mobile suite passed 7 tests. Public documentation and specification validators
also passed, including 62 documentation-linter tests, 46 published-page
checks, 36 specification-linter tests, full specification lint, and
`git diff --check`. Exact evidence is recorded in each work order.

## Review remediation results (2026-09-11)

The code-only review findings are resolved. Bootstrap failure persistence now
uses one execution-, state-, and error-stamp-fenced repository write before
publishing `FAILED`; executor registration and failure mutation serialize on
the same session row, and bounded ownership-read retries handle transient
lookups. Deterministic tests cover successor registration for both absent and
unchanged-stamp errors, plus the terminal promotion race. The detail, preview,
and Quick Chat owners now receive the automatic resumption state, preserve
workspace-only success and dual failure causes, share the in-flight busy latch,
and keep manual failures in the same sanitized card disclosure. Phone action
targets remain at least 44 pixels for coarse pointers.

Verification passed:

- `go test ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process ./internal/orchestrator/executor ./internal/orchestrator ./internal/task/repository/sqlite -count=1`
- `go test ./internal/orchestrator/executor ./internal/orchestrator ./internal/task/repository/sqlite ./internal/task/statussummary ./internal/task/service -count=1`
- `make -C apps/backend build` and `make -C apps/backend lint`
- Focused frontend recovery tests: 5 files, 72 tests passed.
- Full web Vitest sweep: 1,956 files, 16,767 tests passed, 4 skipped.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet`
- `pnpm --filter @kandev/web build:vite` and `pnpm run build:e2e`
- Changed desktop recovery spec: 4 passed.
- Changed mobile recovery spec: 3 passed.
- `git diff --check`

## Risks

- Dry-run history rejection does not prove write permission; real push retains
  its checks. A classifier that accepts mixed/unknown errors is unsafe.
- Workspace admission shares cleanup boundaries with agent launch. A blanket
  terminal-state bypass can leak runtimes or revive archived tasks.
- Mis-correlated errors can hide unrelated history or clear a successor failure.
  Identity and reversed-event regressions are required.
- Old persisted raw errors lack structured cause evidence. Render safe generic
  copy and sanitized details; never infer a specific Git diagnosis from them.
- Exact contributor history and the actor who changed it were not established.
  Neither is necessary to reproduce the product defects.
