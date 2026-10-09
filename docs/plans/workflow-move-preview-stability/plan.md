---
created: 2026-09-17
status: complete
requirements:
  - REQ-TASKS-WORKFLOW-MOVE-PREVIEW-001
  - REQ-TASKS-WORKFLOW-MOVE-PREVIEW-002
system_design:
  - ../../specs/tasks/system-design/workflow-move-preview.md
legacy_specs: []
---

# Implementation Plan: Stable workflow move previews

## Overview

Keep an open move preview stable when background updates cannot change its
prediction. Deliver one sequential work order covering semantic invalidation
and its desktop and touch regression evidence.

The tasks system owns this repair because it owns move prediction and recipient
selection. The existing requirement/design pair remains authoritative. This
package adds the missing stability criterion, AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.8,
and refines the design without changing routing or execution policy.

Implementation followed the approved package in this work order. Existing .001.7
and .002.3 require real invalidation and prohibit stale predictions appearing
current. The correction retains results only when prediction inputs remain equal.

## Evidence and reproduction

`getWorkflowMovePreviewRevision` currently serializes complete task, session,
workflow, step, model, and profile objects. Any serialized metadata change
changes the request key. `useWorkflowMovePreview` then clears success, sets
loading, and requests again after 150 ms. This is an update-driven cycle;
the hook has no periodic polling timer.

The existing test `clears a successful result and fetches again when the
invalidation key changes` proves the reset path. The two focused hook suites
passed all eight tests on 2026-09-17. They lack a harmless-update regression.
No live trace established which event triggered the user's particular hover.

Smallest deterministic regression: resolve a preview, change only the task
description or session read cursor in the real revision input, and rerender.
Advance fake timers beyond 150 ms. Require unchanged success and one request.
Current code changes the revision, clears success, and issues a second request.

## Scope

### In scope

- Explicit semantic projections in the shared revision hook.
- Harmless-update stability in active and background task/session projections.
- Positive invalidation for routing, model, options, connection, and context changes.
- Session candidate-order semantics, mixed eligible/terminal inventories, and ties.
- Existing hover, keyboard, touch drawer, and next-step footer consumers.

### Out of scope

- Backend routing changes, new APIs, persistence, or TTL caches.
- Keeping known-stale results visible as current during a genuine refresh.
- Layout redesign, new copy, provider probes, and new phone navigation.
- Diagnosing unrelated timers or changing task/session store ownership.

## Technical approach

Follow [Semantic revision inputs](../../specs/tasks/system-design/workflow-move-preview.md#semantic-revision-inputs).
Replace whole-object serialization in
`apps/web/hooks/domains/kanban/use-workflow-move-preview-revision.ts` with typed
projections and deterministic serialization. Reuse existing configuration
helpers where available; do not create a generic cache framework.

Read `workflow_move_preview.go`, `workflow_move_preview_settings.go`, and the
session target helpers to verify each selected input. Session `updated_at`
cannot simply be dropped: it ranks reusable candidates. Represent meaningful
ordering without key churn when the winner/order remains unchanged. Preserve
backend tie behavior and original-session provenance.

Keep the 150 ms debounce, two-request limit, in-flight sharing, abort/cleanup,
retry, close/reopen, and late-generation guards in `use-workflow-move-preview.ts`.
Change that hook only if the composed regression identifies an additional
lifecycle defect. No stale-while-refreshing state or renderer rewrite is needed.

All existing profile records may remain candidates for projection to avoid
missing dynamic dependencies, but project only fields used by prediction.
Do not include `agentProfiles.version` as an independent refresh trigger.
Track source, destination, and an explicit referenced target step when available.

## ASCII UI preview

UI-01: Desktop hover/focus disclosure, resolved result plus harmless update.
AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.8 and -002.1, .3.

```text
Current cycle: result -> Checking session... -> result
After:         result -----------------------> same result

+-----------------------------+
|          Move here          |
|           Options           |
|     [step capabilities]     |
|-----------------------------|
|    Reuse current session    |
|         Astra (i)           |
+-----------------------------+
```

UI-02: Coarse-pointer drawer, opened by tapping the existing step trigger.
AC-TASKS-WORKFLOW-MOVE-PREVIEW-001.8 and -002.2, .3.

```text
+--------------------------------+
| Move to                     x  | fixed header
|--------------------------------|
| Implement                      |
| [Options]          [Move here]  |
| Reuse current session          | existing row alignment
| Astra                          | stable during harmless updates
| [expanded details if selected] |
|________________________________| safe-area clearance
```

Existing compact rows keep their shipped alignment; this repair does not apply
the centered desktop footer layout to drawer rows. Drawer content retains one
scroll owner and 44px touch actions. Nearest shipped surface is
`CompactWorkflowStepDisclosure`; the curated MobilePickerSheet pattern supplies
the temporary-choice drawer baseline. Full phone task routes keep their
existing composition; no desktop stepper is added to `SessionMobileLayout`.

UI-03: First open or meaningful input change still shows `Checking session...`.
A failed refresh shows `Preview unavailable` with `Retry`. Neither state alone
disables Move. Closing clears the result; reopening requests a fresh one.
These states also apply to the next-step options footer and touch rows.

Control order and stable feedback are structural requirements; names and
spacing are illustrative. Existing localization and accessibility remain intact.

## Tests

Add these named regressions to the existing suites; use deferred responses and
fake timers to assert both request count and status transitions.

| Criteria | File and planned tests |
| --- | --- |
| 001.8 | `use-workflow-move-preview-revision.test.ts`: `ignores non-predictive task and session updates`; `normalizes equivalent map key order`; cover each store source and missing initial-target profile provenance |
| 001.7, .8 | Same file: `tracks relevant configuration and routing changes`; `tracks candidate order without timestamp churn`; include ties, terminal and completion-follow-up candidates, primary fallback, and original provenance |
| 001.7, .8; 002.3 | `use-workflow-move-preview.test.ts`: `keeps success and request count stable across harmless store updates`; compose the real revision function with the request hook |
| 001.7; 002.3 | Same file: `refreshes after a predictive change amid harmless updates`; delayed stale response, reconnect, close/reopen, retry, two-request queue |
| 002.1-.4 | Existing renderer, stepper, and proceed-button component suites; retain loading/error/details and shared footer behavior |

Negative fixtures change task descriptions/titles, status-summary timestamps,
command counts, read cursors, unrelated metadata, and global profile version.
Positive fixtures change model/options, session name/profile/state/membership,
source end policy, destination start policy/target/actions, workflow default
profile, task profile fallback, reconnect, and workspace generation. Pair every
removed broad input with evidence that its meaningful fields remain observed.

## E2E tests

Extend `apps/web/e2e/tests/workflow/workflow-move-preview.spec.ts` in `chromium`:
hold one hover open after success, apply repeated harmless updates through the
existing `__KANDEV_E2E_STORE__` bridge, and assert no new POST or loading flash.
Observe the result during the updates, not only after it settles. Use the shared
negative-assertion dwell observer for the no-refresh window. Count requests by
task AND destination. Then change a real model with `apiClient.setSessionModel`,
hold the preview response, assert loading, and release it to verify the new model.
Do not mock the revision hook or add a production diagnostic endpoint.

Extend `mobile-workflow-move-preview.spec.ts` in `mobile-chrome` with the same
stability assertion in its existing `tabletTestPage` drawer. This is the shipped
coarse-pointer surface; the full Pixel 5 task route has no desktop stepper.
Keep Options/details open during updates and preserve the existing focus-return,
44px target, and overflow checks. Restore shared user settings in cleanup.
The shared hook regression covers the phone next-step footer without introducing
a new phone navigation surface. Preserve existing move-execution scenarios.

## Work orders

- [x] [Task 01: Stabilize preview invalidation](task-01-stabilize-invalidation.md)

One work order, sequential. No implementation subagents are authorized.
The [original delivery plan](../workflow-move-preview/plan.md) retains its
historical completion results; this repair owns its new tests and results.

## Verification results

Implementation completed on 2026-09-18. The focused unit suite passed 74 tests
across five files, including 9 revision tests covering the review remediation.
Typecheck and changed-file ESLint passed. The final desktop
Chromium run passed 3 tests, and the final mobile-chrome run passed 1 test.
Both browser flows counted one initial preview request, kept the resolved result
and selected details visible across three harmless updates without a loading
state, then observed request two and loading for a genuine model change before
the refreshed model appeared. The browser checks also passed existing move,
focus-return, 44px target, row-alignment, and overflow assertions.

`pnpm run build:e2e` passed after the final source changes. Package validation
passed on 2026-09-18:

- `python3 scripts/list-docs.py validate`: 286 decisions and 998 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Package links, acceptance references, and frontend command paths: passed.
- `git diff --check`: passed; scoped status confirms both new package files and
  the updated requirements, design, and delivery-plan documents.

No backend policy, API, cache, or layout changes were made.

## Risks

- Over-narrow projection can hide a model, metadata, or recipient change.
- Raw timestamps cause churn; dropping selection order can leave stale recipients.
- Reordered rule arrays are meaningful even when object key order is not.
- Active/background stores can update separately; retain both invalidation paths.
- Live updates may legitimately cause checks. The fix promises stability only
  when prediction inputs remain equivalent.
