---
created: 2026-09-28
status: complete
requirements:
  - REQ-TASKS-COMPLETION-004
system_design:
  - ../../specs/tasks/system-design/coordination-controls.md
legacy_specs: []
---

# Implementation plan: Remove native coordination controls

## Overview

Remove the Manager and Completion requirements controls introduced by
[PR #3994](https://github.com/kdlbs/kandev/pull/3994). Restore the desktop and
phone task layout while preserving the plugin contracts and task data.

The user requested a removal plan and an audit of other UI changes. This package
covers the task-detail removal. The [PR audit](pr-3994-ui-audit.md) identifies
other changes and recommends their disposition; it does not authorize their removal.
The user later requested implementation of this package.

The task system owns this package because it owns the task-detail outcome and
its management and completion contracts. The existing task requirement and design
are amended in place. No separate UI specification or new ADR is necessary:
the amendment records the local presentation boundary and its tradeoffs.

## Scope

### In scope

- Remove both native rows on desktop, the shared phone toolbar, and their private inspection/editing surfaces.
- Restore phone header spacing, shared-error layout, composer clearance, and navigation.
- Remove dead component helpers and unused translations only after a reference check.
- Preserve domain enforcement, APIs, SDK exports, database records, history, and normal workflow error feedback.
- Replace tests of removed UI with desktop/phone regressions and retain recovery coverage at the API boundary.
- Correct public claims about native task controls during implementation.

### Out of scope

- Reverting all of #3994, removing its backend functionality, or changing database schemas.
- Removing automation UI, plugin capability settings, retained transcripts, or reusable Host UI exports. These are audited separately.
- Adding a replacement native menu, settings page, hidden feature flag, or conditional empty-state variant.
- Building or publishing a coordinator plugin UI. No existing plugin recovery UI is assumed.
- Changing human consent, authorization, approval receipts, or completion policy.

## Technical approach

### Task layout and component removal

Remove `TaskCoordinationControls` and its invocation from
`apps/web/components/task/task-page-inner.tsx`. Remove the three
`task-management-claim-*` files, five `task-completion-gate-*` files, and
`use-task-completion-gate-actions.ts` after confirming their private import tree.
Keep `lib/api/domains/task-management-claims-api.ts`,
`task-completion-gates-api.ts`, their exports and tests, and the workflow move
API. Keep completion flags in workflow snapshots and status DTOs.

Restore the top-offset behavior across `task-layout.tsx` and
`mobile/session-mobile-layout.tsx`. The parent of commit `f24a785cd30` supplies
the reference: `3.5rem` without a shared task error, zero when the error surface
already provides the offset. Preserve later changes, including #3986; do not
restore entire files. Retain safe-area padding and bottom navigation clearance.

Delete unused `managementClaim*` and `completionGate*` locale keys only when no
remaining source references them. Search all catalogs and `_verbatim.json` files.
Do not remove `plugins:managedChat*` strings used by public Host UI components.
Do not shrink the i18n guard allowlist.

### Data and human recovery

The removal does not alter stored claims or completion criteria. A task with
unmet criteria remains blocked, and the existing workflow error surface reports
failed moves. Human claim release/transfer, evidence updates, and one-move override
remain authorized API operations. Plugins gain no new human authority.

This deliberately removes built-in browser shortcuts for human recovery. The
current Host UI exports do not expose these task dialogs. Moving them into a
plugin is separate work and must respect the existing public API and consent
boundary. Document this limitation instead of implying that a replacement exists.

### Test migration

Replace the four browser specs that depend on the removed Manager/Inspect
buttons. Retain claim conflict and unavailable-owner recovery in handler tests.
Retain stale evidence, weakening confirmation, and reasoned one-move override
in service tests. Add HTTP regressions if transport-level recovery assertions
would otherwise disappear. Do not replace meaningful domain coverage with
absence assertions or skip the old suites merely to make the run pass.

### Documentation

Update `docs/public/plugins-authoring.md` at the claim and completion sections
that currently promise native task-detail controls. Preserve its SDK examples.
Keep historical #3994 results as dated evidence; mark its UI-03 preview and
work orders 09/10 as superseded for presentation only. Domain results remain valid.

## ASCII UI preview

### UI-01: Desktop task detail, ordinary or managed task

```text
Before (#3994)
+---------------------------------------------------+
| Existing task title and workflow controls          |
| Manager: None                            [Manage] |
| Completion requirements: 0 of 0          [Inspect]|
| Session / chat                    | Files / changes|
+---------------------------------------------------+
After
+---------------------------------------------------+
| Existing task title and workflow controls          |
| Existing error feedback, only when applicable      |
| Session / chat                    | Files / changes|
+---------------------------------------------------+
```

### UI-02: Phone task detail, ordinary or managed task

```text
Before (#3994)             After
+----------------------+  +----------------------+
| Fixed task header    |  | Fixed task header    |
| Manager | Completion |  | Existing error, if any|
| Active panel         |  | Active panel         |
| Composer             |  | Composer             |
| Bottom navigation    |  | Bottom navigation    |
+----------------------+  +----------------------+
```

There is no replacement toolbar, blank strip, manager-loading row, or gate-loading
row. Existing errors remain conditional, including shared launch errors and
failed completion moves. The phone keeps one active panel and internal scroll
owner, dynamic viewport sizing, safe-area clearance, and existing touch controls.
These structural choices satisfy AC-004.1, AC-004.2, and AC-004.4. Spacing in the
sketch is illustrative. The current task layout is the mobile exemplar.

## Tests

| Criteria | Evidence |
| --- | --- |
| AC-TASKS-COMPLETION-004.1 | New `task-coordination-ui-removal.spec.ts`: ordinary and configured tasks have no native controls or claim/gate detail fetches on open/reload |
| AC-TASKS-COMPLETION-004.2 | New `mobile-task-coordination-ui-removal.spec.ts`; `session-mobile-layout.test.tsx` offset cases; existing mobile launch-error and clarification flows |
| AC-TASKS-COMPLETION-004.3 | `TestTaskManagementClaimFencing`, `TestHTTPReleaseTaskManagementClaim`, `TestHTTPTransferTaskManagementClaimBindsCurrentHumanActor`, `TestCompletionGateEntryPoints`; API client and Host API tests |
| AC-TASKS-COMPLETION-004.4 | Desktop E2E rejects a blocked completion through the normal workflow stepper and shows the existing error banner; handler/service tests retain authorization and gate coverage |

## E2E tests

Use `apps/web/e2e/tests/task/task-coordination-ui-removal.spec.ts` with `chromium`
and `mobile-task-coordination-ui-removal.spec.ts` with `mobile-chrome`.
Cover ordinary and configured tasks, no detail requests, a blocked move through
the task stepper, and normal phone chat. Assert active UI readiness before
negative DOM/request assertions. Use causal waits instead of arbitrary sleeps.

Phone checks cover the fixed-header gap, composer clearance, bottom navigation,
and document width. The existing phone launch-failure and clarification suites
protect shared-error and message paths. Managed E2E commands in work order 01
rebuild the application before browser tests.

## Work orders

- [x] [Task 01: Remove native controls and preserve layout/contracts](task-01-remove-task-controls.md)
- [x] [Task 02: Update public guidance and delivery records](task-02-update-guidance.md)

Run tasks 01 and 02 in sequence. Both work orders record their results.

## Verification results

Planning validation on September 28, 2026:

- `python3 scripts/list-docs.py validate`: passed, 321 decisions and 1,220 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans`: passed.
- Local `.github/scripts/pr-docs.cjs` coverage preflight: the current documentation-only diff is exempt. A simulated planned source change validates both new work orders and their linked plan, design, requirement, and acceptance IDs.
- `git status --short`: only the design package and related specification/delivery annotations changed; all files remain unstaged.

The Python checks used `TMPDIR=/root/.cache/kandev-coordination-plan-tmp`
because `/tmp` was full. No shared files were deleted.
Implementation validation on September 28, 2026:

- Focused frontend Vitest: 108 tests passed across 6 files. Focused Go tests passed in `internal/task/handlers` and `internal/task/service`.
- Web typecheck and i18n checks passed. Desktop E2E passed 1 test, including a blocked move and error banner. Mobile E2E passed 14 tests across layout, launch recovery, and clarification.
- The desktop and phone screenshots were inspected. Phone checks confirmed the composer stays above bottom navigation and the page has no horizontal overflow.
- Public documentation tests passed, 62/62. The public docs validator accepted all 47 pages.
- The specification catalog accepted 321 decisions and 1,220 specifications. All specification files passed lint.
- The local PR documentation-coverage preflight accepted both changed work orders for the simulated source diff.
- `git diff --check` passed. Working tree changes remain unstaged.

Review follow-up on September 28, 2026 fixed phone feedback that could begin
under the fixed task header after the toolbar was removed. Page-level move,
ensure-session, and recovery feedback now reserve the header and safe-area space
at their shared parent; the mobile panel and shared task-error surface suppress
their own offset while that parent owns it. Ordinary chat and shared-task-error
layouts retain one clearance owner.

- Focused frontend Vitest passed 74 tests across the mobile-layout and task-page
  helper files. The page-feedback cases cover ensure failure without a
  task-summary error, page-level recovery feedback, ordinary pages, and desktop
  behavior.
- The managed mobile E2E run rebuilt the backend and web app, then passed all 15
  tests across coordination removal, mobile launch recovery, and clarification.
  It measured feedback and retry controls against the fixed header, tapped the
  status-unavailable retry, confirmed no shared task error, and checked that
  ordinary chat does not overlap the header.
- The desktop coordination-removal E2E passed 1 test. Web typecheck passed.
- The shared task-error mobile recovery case was rerun with an added lower-bound
  assertion against the measured fixed header; it passed.
- `git diff --check` passed. Changes remain unstaged.

## Risks

- Removing the toolbar without restoring phone offsets puts content under the fixed header.
- Browser takeover/override shortcuts disappear. APIs remain; no replacement plugin UI is included.
- A broad revert removes confirmation security or plugin capabilities and can overwrite later changes.
- Removing old E2E files without migrating their domain assertions loses recovery coverage.
- `/tmp` was full during planning. The repository had free space. Before implementation checks, use an approved writable temporary directory or resolve capacity; do not delete shared files blindly.
