---
created: 2026-10-27
status: complete
requirements:
  - REQ-TASKS-CREATE-RECOVERY-001
system_design:
  - ../../specs/tasks/system-design/task-create-recovery.md
legacy_specs: []
---

# Implementation plan: Task creation retry protection

## Overview

Keep one creation identity through an uncertain native task submission.
Recover its task without another allocation or agent launch.
First add the recovery contract and receipt. Then connect the dialog and prove both viewports.
All three work orders are complete. The implementation and runtime checks are recorded below.

Tasks owns creation identity and settlement. Its requirement and design also own the recovery UI.

## Evidence and classification

The October 27 incident created three tasks with identical 20,211-character briefs:

| Created, UTC | Task ID | Observed session outcome |
| --- | --- | --- |
| 19:11:35.155 | `761457ee-6992-4e32-b2d2-8f3010568289` | Cancellation recorded, WAITING_FOR_INPUT. |
| 19:11:39.842 | `608155f2-4945-4cba-b59f-e22f294ccdfa` | Retained task, RUNNING during inspection. |
| 19:11:46.367 | `fe9d3729-0bc7-4c16-bf81-706cc6667544` | Cancellation recorded, WAITING_FOR_INPUT. |

All three `POST /api/v1/tasks` requests logged HTTP 200.
Their durations were 5,528ms, 1,038ms, and 411ms.
The browser disconnected at 19:11:39.605 and logged concurrent `Failed to fetch` errors.
The exact dialog error and the connection interruption's underlying cause remain unknown.

The running build was `b6263ea4c3165c86ec60e29e798475ad552ad6ce`.
The planning checkout was `a6144c3cb8e`.
Both native dialog paths omit `external_id` and do not interpret `creation_complete`.
The current handler already supports those fields, found outcomes, and external-ID lookup.
Its found task DTO can omit session information, so recovery must hydrate that separately.

The confirmed defect is an unkeyed repeated create after an ambiguous result.
Button disabling alone cannot protect a retry after the first request fails in the browser.
The requirement gap is normal-dialog recovery. This package adds its smallest vertical requirement/design pair.
The shipped REST identity contract remains a dependency. Its separate draft aggregate redesign is excluded.

The retained diagnostic ZIP is local investigation evidence, not a repository dependency:
`.kandev/diagnostics/64f583ca5ee148397946478c15ae6586.zip`.
Its manifest is partial because of the archive byte limit. The relevant incident window was present.
Implementation and regression tests must work without this archive or the user's live instance.

## Scope

### In scope

- Native Start task, plan-mode, and create-only submissions, including native prefilled contexts.
- One captured receipt, shared submission guard, and same-identity explicit retry.
- Read-only lookup, incomplete creation visibility, and known-task navigation.
- An additive pre-allocation rejection marker with allocation-boundary tests.
- Draft ownership, reload restoration, attachment descriptors, and presentation-error recovery.
- Existing branch consent, workflow choices, queued launches, and completion callbacks.
- Accessible desktop/phone status and recovery actions in all seven locales.
- Focused unit, handler, and browser regressions plus public task recovery guidance.

### Out of scope

- Changes outside the scoped production and test files named in the work orders.
- Live task cleanup, cancellation changes, transport diagnosis, and timeout configuration.
- Database migration, the draft aggregate creation coordinator, or a new server worker.
- Caller repair, automatic release, deletion, task replacement, or prompt replay.
- Idempotency changes to custom plugin transports, WS creation, Quick Chat, edit, or additional sessions.
- Cross-tab/device synchronization and tombstones after physical deletion or operator release.
- Feature flags, broad verification, commits, PRs, and delegated implementation.

## Technical approach

### Recovery contract and receipt

Add `external_id` to the native `createTask` client input.
Add a typed lookup client for the existing by-external-id route.
Decode native creation metadata without changing every general task representation or custom transport contract.

Add proposed `lib/task-creation-receipt.ts` and `components/task-create-dialog-recovery.ts`.
The receipt stores an exact normalized payload and caller UUID before dispatch.
Persist descriptor-only recovery data in sessionStorage, scoped to user, workspace, and draft.
Keep a synchronous guard and sticky uncertainty state across every explicit retry.

Expose optional `creation_rejected: true` only from proven native pre-allocation errors.
Reuse `PreAllocationTaskCreateError` and preserve existing machine-readable error details.
No later rejection can retire a receipt after an earlier uncertain request.

This step adds no rendered UI and does not enable a partially wired product flow.

### Native dialog integration

Use the recovery owner in both native creation branches in `task-create-dialog-submit.tsx`.
All submit variants share its guard. Retrying uses the receipt, not rebuilt form defaults.
The existing fresh-branch re-consent path keeps the same key.
Add status presentation and footer actions without replacing the full task form.

Record accepted task identity and the callback fence before presentation side effects.
Hydrate missing session data through `fetchTask`, without preparing a session.
Keep newer draft edits and prevent late results from navigating a different owner.
Close retains unresolved receipts and does not cancel server work.

### Compatibility matrix

| Surface or outcome | Transport and identity | Intended behavior | Evidence and fallback |
| --- | --- | --- | --- |
| Native start/plan/create-only | REST, browser UUID in `external_id` | One task for one submission. Original start intent remains fixed. | Unit and E2E lost-response regression. |
| Native prefilled task/subtask | Same REST contract, captured parent/context | Preserve adapter callbacks and draft ownership. | Component tests. |
| Queued task | Same key, complete task with no immediate session | Preserve queue notification and deferred start. | Submit regression. |
| Found settled without `session_id` | Existing task ID plus detail GET | Navigate using the existing task/session. | Hook and handler tests. Failed hydration retains Open task. |
| Found incomplete | Existing task, `creation_complete: false` | Show incomplete state. Read or open only. | Hook and handler tests. No reprepare or start. |
| Created identity lost | Complete response with task ID, absent external ID | Retain that task and stop using the released identity. | Receipt test. |
| Custom `createTask` override | Adapter-specific response, no native identity capability | Keep the existing transport contract. | Existing composition tests and explicit exclusion test. |
| Quick Chat/edit/additional session | Their existing transports | Bypass the native create owner. | Existing focused submit tests. |
| Malformed or unsupported native response | Unknown allocation evidence | Retain the receipt and show recovery. | Decoder tests. No new identity. |

### Browser reproduction

The minimum faithful regression forwards the first native POST to the real isolated backend.
After creation succeeds, the test replaces only the browser response with 502 or aborts that response.
The user retries in the same dialog.
The test proves one backend task, one expected session, and one initial prompt admission.
Request counts alone do not prove this invariant.

## ASCII UI preview

### UI-01: Native dialog after an uncertain creation

Entry point: New Task in the sidebar or Kanban.
The observed failure kept the dialog usable for another Start task click.
The proposal replaces that ambiguous action with explicit recovery.
The exact historical error wording is unknown.

```text
Current failure, schematic
+-------------------------------------------------------+
| Create task                                           |
| [The same task draft]                                 |
| Error shown, earlier task can already exist            |
|                                  [Cancel] [Start task]|
+-------------------------------------------------------+

Proposed desktop, unknown result
+-------------------------------------------------------+
| Create task                                           |
| [Editable draft and existing selectors]                |
| Task creation is unconfirmed.                          |
| Retry uses the earlier submission.                     |
| Changes stay in this draft.                            |
|          [Close] [Retry submission] [Check status]      |
+-------------------------------------------------------+

Proposed phone, unknown result
+----------------------------------+
| Create task                      |
| [Draft and selectors]            |
|                                  |
| Internal scrolling form body     |
| Task creation is unconfirmed.    |
| Retry uses the earlier values.   |
+----------------------------------+
| [         Check status         ] |
| [       Retry submission       ] |
| [             Close            ] |
| Bottom safe area                 |
+----------------------------------+
```

The status belongs above the fixed footer. The existing form body owns vertical scrolling.
Phone actions stack in the same full-height surface, with targets of at least 44px.
Desktop actions retain the normal 28px control size.
The proposed copy is illustrative English. Implementation uses localized keys.

### UI-02: Known task and incomplete creation

```text
Status: This task exists. Creation is still in progress.
Desktop footer: [Close] [Check status] [Open task]
Phone footer:   [Open task]
                [Check status]
                [Close]
```

The primary action opens the known task. It does not start an agent or repair creation.
An accepted task after a presentation error uses the same Open task route.
A known deleted or inaccessible task exposes Check status and Close, without replacement creation.

Structural requirements cover status placement, action meaning, scroll ownership, and touch reachability.
ASCII spacing is illustrative. Existing primitives and tokens own final geometry.
These views map to criteria .5, .8, .9, .11, .12, and .13.

## Tests

All criteria below use the prefix `AC-TASKS-CREATE-RECOVERY-001`.

| Criteria | Focused evidence |
| --- | --- |
| .1, .2, .3, .4 | `task-create-dialog-creation-retry.test.tsx`: lost response, same-tick actions, captured payload, and independent next submission. |
| .5, .6 | `task-create-dialog-recovery.test.tsx`: incomplete/found results, rejection evidence, and sticky uncertainty race. |
| .6 | `task_http_create_rejection_test.go`: pre-allocation marker only. Existing external-ID tests retain found outcomes. |
| .7, .8, .10, .11 | `task-creation-receipt.test.ts`: restoration, scope, late results, descriptor-only storage, unavailable inline data, and deletion fence. |
| .9 | `task-create-dialog-creation-retry.test.tsx`: throwing callback and missing session hydration preserve one accepted task. |
| .12, .13 | Rendered recovery tests and the two Playwright files below. |
| .14 | Existing submit, fresh-branch consent, and custom composition tests. |

The defect-specific RED gate belongs to work order 02:
`retains one native task when creation succeeds but the browser response fails`.
The baseline must create two distinct task IDs on retry or omit the stable key.
A missing selector is not the RED result.
Work order 01 separately proves the additive rejection contract with a failing marker assertion.

## E2E tests

| Scenario | Criteria | File and project |
| --- | --- | --- |
| Real creation followed by lost response and explicit same-key retry | .1, .2, .4, .5, .9, .12 | `task-create-recovery.spec.ts`, `chromium`. |
| Reopen/reload keeps the receipt and performs no create until explicit action | .7, .8, .10 | Same desktop file, `chromium`. |
| Incomplete task stays visible, later lookup settles it, and opening does not launch again | .5, .11, .12 | Same desktop file, `chromium`. |
| Native create-only retry allocates no agent session | .1, .9, .14 | Same desktop file, `chromium`. |
| Phone lost-response recovery, measured targets, internal scroll, and containment | .1, .7, .12, .13 | `mobile-task-create-recovery.spec.ts`, `mobile-chrome`. |
| Creation-pending task route suppresses automatic session ensure | .5, .11, .12 | `task-create-recovery.spec.ts`, `chromium`. |
| Desktop to phone to desktop preserves unresolved receipt and newer edits | .7, .8, .13 | Same desktop file with responsive viewport transitions. |

Use the existing isolated mock-agent fixture and real task creation routes.
Do not create or modify tasks on the user's running instance.
Use causal HTTP/WS waits and backend state polling, without raw sleeps.

## Work orders

- [x] [Task 01: Add the native recovery contract and receipt](task-01-native-recovery-contract.md)
- [x] [Task 02: Integrate recovery into the native dialog](task-02-dialog-recovery.md)
- [x] [Task 03: Prove desktop and phone recovery](task-03-browser-regressions.md)

Execute 01, then 02, then 03. All work is sequential.
This order does not authorize subagents.

For a fresh worktree, install `apps/` dependencies once before the first pnpm command.
Each work order lists its exact product checks and owned files.
Run these package gates after specification or plan changes:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/task-create-recovery
```

### Package coverage preflight

Run from the repository root. The two prospective source paths exercise the implementation coverage gate during package authoring.
The validator must accept all three work orders and their referenced contracts.

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const root = 'docs/plans/task-create-recovery';
const files = fs.readdirSync(root).filter(p => p.endsWith('.md')).map(p => `${root}/${p}`);
files.push('docs/specs/tasks/requirements/task-create-recovery.md', 'docs/specs/tasks/system-design/task-create-recovery.md');
const fileContents = Object.fromEntries(files.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = [...files, 'apps/web/components/task-create-dialog-submit.tsx', 'apps/backend/internal/task/handlers/task_http_handlers.go'].map(filename => ({ filename, status: 'modified' }));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ status: result.status, workOrders: result.workOrders.length, errors: result.errors }));
if (!result.ok) process.exitCode = 1;
JS
```

## Verification results

Product implementation and runtime verification: completed on 2026-10-28.
Each work order records its own actual checks below.

Package checks on 2026-10-27:

- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- The coverage preflight above: passed, all three work orders covered.
- `git diff --check -- docs/specs docs/plans/task-create-recovery`: passed.
- Direct whitespace checks for all six untracked Markdown files: passed.

Implementation checks on 2026-10-28:

- `(cd apps/backend && go test ./internal/task/handlers -count=1)`: passed.
- `bash scripts/lint-go-changed` on the three changed Go files: 0 issues.
- Focused Vitest suites for receipt, recovery, creation retry, dialog, API transport, and the creation-pending task-page helper: passed, 11 files and 192 tests.
- Remediation review covered owner-generation fencing before the first await, in-memory retention on storage failure, auto-title-aware live draft comparison, the incomplete-task read-only route gate, stale branch-policy recovery, state-preserving lookup misses, and foreign create-response rejection.
- Fixup review split receipt storage, owner fencing, resolution, handoff, and native recovery into smaller modules so the new functions meet the frontend limits without new complexity exemptions.
- `(cd apps/web && pnpm run typecheck)`: passed.
- Focused ESLint on the changed web modules: passed.
- `(cd apps/web && pnpm run i18n:check)` and `i18n:ratchet`: passed for all seven locales and the pseudo catalog.
- `(cd apps/web && pnpm e2e:run --host --project chromium -- tests/task/task-create-recovery.spec.ts)`: 5 passed.
- `(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- tests/task/mobile-task-create-recovery.spec.ts)`: 1 passed.
- `node --test scripts/validate-public-docs.test.mjs`: 63 passed.
- `node scripts/validate-public-docs.mjs`: 54 pages validated.
- `python3 scripts/list-docs.py validate`: 409 decisions and 1765 specifications validated.
- `python3 scripts/lint-spec-files.test.py` and `python3 scripts/lint-spec-files.py --all`: passed.
- The coverage preflight above: covered, all three work orders.
- `git diff --check` over the specifications, plan, and public page: passed.

The requirement is active. The system design is current. All three work orders are complete.

## Risks

- Native and custom transports share dialog types. Do not require unsupported metadata from a plugin adapter.
- A later pre-allocation rejection cannot prove that an earlier uncertain request stopped.
- Found responses can omit session data. Missing hydration must not trigger a launch.
- An incomplete generic creation can stay incomplete. This package exposes it without adding caller repair authority.
- Browser storage can fail or omit legacy inline contents. Keep identity and disable unsafe replay.
- Physical deletion and operator release free the server identity. No tombstone guarantee is added.
- Existing fresh-branch rollback can fail. A surviving task must remain visible rather than receive a replacement.
- The underlying network disruption remains outside this fix. Recovery must work through another lost response.
