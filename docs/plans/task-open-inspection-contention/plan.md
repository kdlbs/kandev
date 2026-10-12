---
created: 2026-10-08
status: complete
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-003
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
legacy_specs: []
---

# Implementation plan: Task-opening inspection contention

## Overview

Resume waits briefly for a competing workspace inspection and retains the task
workflow state. Exhausted contention becomes a same-session retry without durable
failure bookkeeping or workspace-only fallback.

Implement backend coordination first. Then integrate the typed browser outcome
and desktop/phone evidence. Review remediation is complete across both work
orders.

The task system owns this fix because it owns environment admission and durable
task/session lifecycle. Agents retain provider recovery semantics. Workspace and
UI specifications remain dependencies, not duplicate owners.

## Evidence and root cause

For task `049df54f-4952-4908-a5df-617e212f0b0e` on October 8, logs show:

- At 11:32:35.507 Lisbon time, Git loading began workspace-only execution creation.
- At 11:32:35.618, automatic resume entered a separate outer admission.
- At 11:32:35.756, inspection contention returned before lifecycle singleflight.
- The session changed from WAITING_FOR_INPUT to FAILED. The task changed from
  REVIEW to FAILED through generic resume failure bookkeeping.
- At 11:32:36.599, the workspace became ready. Browser fallback reported workspace
  availability with the agent stopped.
- A later resume reached WAITING_FOR_INPUT at 11:34:46. The task retained FAILED.

The running binary was `1769b9abafea5c45c0798409e33dbaa6236d0f3b`.
The affected source paths match the inspected checkout at
`7c6fcf1b9d0e9f2e91e524c44dc710a319fd24c2`.

## Scope

### In scope

- One bounded inspection budget across outer admissions in a resume request.
- Safe retryable contention before agent startup, including guarded rollback.
- Existing typed conflict mapping and localized same-session recovery feedback.
- Deterministic concurrency, failure-isolation, desktop, and phone regressions.

### Out of scope

- Repair of historical FAILED task rows, including the observed task.
- Changes to workflow state on ordinary successful resume.
- New provider conversations, automatic fresh starts, branch replacement, or dirty relocation permission.
- New public endpoints, database migrations, feature flags, or global locks.
- Publication, implementation workers, and changes to the user's running instance.

## Technical approach

### Backend admission

Extend the wait policy in `internal/worktree/recovery_admission.go` without changing
default nonblocking callers. Carry a single absolute deadline through the outer
manual preflight and executor `resumePreflight`. Retain it across request assembly
and `admitResumeSelectionAfterRequest`. Nested lifecycle creation never waits.

Reuse selection snapshots, existing claims, cancellation, and singleflight.
Keep the current 15-second maximum and 30-second client timeout. Rename the
manual-only budget identifier or document its broader outer-admission scope.
Update its callers and tests together. The budget conveys no mutation authority.

### Failure isolation

Return a retryable typed conflict only after safe rollback of any owned STARTING
projection and credential snapshot. `resumeTaskSessionWithContinuation`
must bypass `handleSessionLaunchFailure` only for that verified outcome.
Cancellation, superseded attempts, and rollback failure retain their own behavior.

Use `recoveryInspectionConflictResponse` and the existing structured kind.
Retain generic error bookkeeping for real metadata, credential, branch, and
provider failures. Do not change task state on successful silent resume.

### Browser behavior

Recognize `recovery_inspection_busy` in `session-recovery-service.ts`.
Handle it before workspace fallback in `finishSilentResume`. Keep request-local
feedback in the existing recovery owner. Reuse the same-session Resume action.
Apply the same localized mapping to manual recovery failure feedback.

Add `task:workspaceRecoveryInspectionBusy` in all seven shipped catalogs.
Proposed copy: "This workspace is still being checked. Try resuming the session again."
Generate Traditional Chinese and pseudo through repository tooling.
No new recovery operation or persisted error category is needed.

### Compatibility

| Boundary | Behavior | Evidence |
| --- | --- | --- |
| Host Worktree, healthy selected inventory | Bounded outer wait, same-session resume | Real Git/mutex and executor regressions |
| Host Worktree, invalid or changed slot | Existing safe refusal, no startup | Mixed-inventory and ownership tests |
| Lifecycle/workspace-only creation | Immediate contention refusal, shared runtime | Nonblocking and singleflight regressions |
| Explicit manual recovery | Same bounded budget, unchanged relocation authority | Existing manual-admission tests |
| Local, Docker, SSH, Sprites, Kubernetes, repository-free task | No host-inspection change | Existing scope tests and recording assertions |
| Provider protocols | Existing retained conversation and startup path | Identity/start-count assertions with mock provider |

No real external-provider coverage is claimed. Unsupported or unsafe inventory
retains the existing refusal rather than a permissive fallback.

## ASCII UI preview

UI-01: Task Chat during inspection. The current inline resuming status remains.

```text
Transcript
Draft retained in the existing session draft store
[spinner] Resuming session
```

UI-02: Inspection wait exhausted. One existing inline recovery region owns the
request-local notice. No failure-history entry or read-only fallback appears.

Desktop:

```text
Transcript
+------------------------------------------------------------+
| This workspace is still being checked.                      |
| Try resuming the session again.                             |
| [Resume session]                                           |
+------------------------------------------------------------+
```

Phone:

```text
Transcript
+----------------------------------+
| This workspace is still being    |
| checked. Try resuming the        |
| session again.                   |
| [        Resume session        ] |
+----------------------------------+
```

UI-03: Matching retry succeeds. The existing composer returns with its draft and
attachments. The task retains its workflow position and historical messages.

Reuse `SessionRecoveryCard` and `RecoveryActions` as the shipped phone exemplar.
The card stays inline in Chat. The phone action has a minimum 44-pixel hit area.
Preserve the recovery-region scroll owner, safe-area clearance, and focus policy.
Do not add a drawer, overlay, or second mobile recovery owner.

## Tests

| Criteria | Regression evidence |
| --- | --- |
| 003.6 | `TestSessionOpenResumeWaitsForWorkspaceInspection`, `TestResumeInspectionBudgetSharedAcrossAdmissions` |
| 003.7 | `TestResumeInspectionContentionPreservesTaskState`, late-admission guarded rollback matrix |
| 003.8 | Typed browser recognition, no-fallback unit test, UI-01 through UI-03 browser flows |
| 003.9 | Cancellation/archive/selection-drift barriers, real-error control, mixed inventory |

Test names are proposed. New files are listed in the work orders.
Record behavioral RED before production edits. The first backend reproduction
uses the real lock and persisted selected inventory, not only a recording callback.

## E2E tests

- `apps/web/e2e/tests/session/session-open-inspection-contention.spec.ts`, chromium:
  delayed pending response, typed exhaustion without restore fallback, and retry
  into the same conversation with the draft intact.
- `apps/web/e2e/tests/session/mobile-session-open-inspection-contention.spec.ts`,
  mobile-chrome: the same outcomes through phone Chat and its existing recovery owner.

Browser conflict fixtures intercept the selected resume request before forwarding
it. They do not allow a real upstream resume to proceed behind a fabricated refusal.
Real mutex coordination remains a backend integration assertion.

## Work orders

- [x] [Task 01: Coordinate outer resume inspection](task-01-resume-admission.md)
- [x] [Task 02: Present retryable inspection contention](task-02-browser-retry.md)

## Verification results

Design-package checks passed on 2026-10-08:

- Document catalog validation and all specification-file lint checks.
- Specification-linter tests: 36 passed.
- Work-order coverage preflight for the planned backend and browser changes.
- `git diff --check` and the modified/untracked document inventory.

Task 01 backend regressions and race checks passed. Task 02 browser behavior,
translations, and desktop/phone screenshots passed. The final Task 02 verification
results are recorded in its work order.

Review corrections also passed full tests for worktree, executor, orchestrator,
handlers, and lifecycle packages; focused backend race regressions; backend and
production Vite builds; 34 current browser recovery tests across four files;
web typecheck and focused ESLint; and `git diff --check`. The corrected lifecycle
failure cases verify that only successful attempt-fenced rollback deferrals skip
task failure bookkeeping. The caller-deadline regression exercises the ordinary
session-open path while a real workspace inspection lock is held.

## Risks

- Blocking inside lifecycle singleflight can deadlock an outer admission owner.
- A fresh wait budget at each admission can exceed the browser request deadline.
- A broad error exemption can conceal failed rollback or a real startup failure.
- Browser-only response simulation cannot prove mutex behavior.
- Old FAILED task rows lack sufficient provenance for automatic repair.
