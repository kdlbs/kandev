---
created: 2026-09-30
status: complete
requirements:
  - REQ-TASKS-CREATE-WORKFLOW-STEPS-001
system_design:
  - ../../specs/tasks/system-design/task-create-workflow-step-previews.md
legacy_specs: []
---

# Implementation Plan: Task creation workflow picker scrolling

## Overview

Keep every workflow reachable in the task creation picker. Issue [#4073](https://github.com/kdlbs/kandev/issues/4073) reports an unscrollable menu with nine or ten workflows in v0.96.0.

The production correction already landed in [PR #4058](https://github.com/kdlbs/kandev/pull/4058). It merged at 2026-09-30 08:08:35 UTC as `ff917a370ac`. Current main passes the reported desktop inventory and viewport reproduction. This package owns permanent scrolling regression coverage, not another production correction.

One sequential work order adds desktop and phone coverage to the existing preview suites. It preserves the current picker composition and task creation behavior.

Tasks owns this outcome because the picker selects a workflow for task creation. The existing requirement/design pair remains authoritative. AC-TASKS-CREATE-WORKFLOW-STEPS-001.7 makes overflow reachability explicit. The [earlier package](../task-create-workflow-step-previews/plan.md) remains complete for AC-001.1 through AC-001.6.

## Evidence and root cause

The issue attachment shows a menu taller than the viewport. Its first rows extend above the visible page. The user cannot reach them by scrolling.

At tag `v0.96.0`, `WorkflowSelectorRow` renders each workflow directly inside `PopoverContent`. The content has `w-auto min-w-[300px] max-w-none p-1`, without a height bound or an internal scroll region. Radix placement cannot make an oversized, non-scrollable list reachable. Long inline step previews also expand the menu horizontally.

PR #4058 introduced `WorkflowSelectorOptionList`, a viewport-bound popover, wrapping step previews, and an internal scrolling list. It also portals the picker into the task dialog. At implementation start, the remaining coverage gap was physical input: existing phone tests assigned `scrollTop` directly, and desktop coverage seeded only three custom workflows.

A temporary Playwright diagnostic seeded ten custom workflows and entered task creation directly from a task page. At 1682x768, the popover measured x=415.28, y=36.11, width=461.30, height=429.35 CSS pixels. The option list had scrollHeight=700 and clientHeight=410. Wheel input moved it down, then returned scrollTop to zero. Selecting Preview Kanban succeeded. This evidence applies to `ff917a370ac`, not the released Docker image.

The temporary diagnostic was removed after the investigation. Its behavior belongs in the permanent suites through Task 01.

## Assumption check

- Confirmed: investigate the issue, assign a sufficiently certain repair, and create a fix package.
- Verified: the authenticated GitHub account is `carlosflorencio`. Issue #4073 now has that assignee.
- Verified: the released source lacks a scroll owner. Current main contains the correction and passes desktop wheel scrolling.
- Settled repair scope: protect the existing correction with regression tests. No material product choice remains unresolved.
- Limitation: the reported Docker image was not launched. The release diagnosis uses its tagged source and the issue screenshot.

## Scope

### In scope

- Ten custom workflows, each with at least fifteen loaded steps and enough
  content for overflow. Include the long stage names visible in the issue.
- Wheel scrolling in both directions on desktop, including a short viewport.
- Touch scrolling in both directions on a phone.
- Selection of the first and last rendered options after scrolling.
- Keyboard reachability, task draft preservation, focus return, and viewport containment.

### Out of scope

- Production component changes, a new picker shell, search, or virtualization.
- Workflow definitions, launch routing, defaults, APIs, storage, flags, and localization changes.
- Release publication, issue closure, and comments to the reporter.

## Technical approach

Extend `apps/web/e2e/tests/task/task-create-workflow-step-previews.spec.ts` and its phone counterpart. Reuse `seedWorkflowStepPreviewScenario` with `extraWorkflowCount: 7`. This creates ten custom workflows without counting baseline seed workflows.

Use `workflow-selector-popover` for containment and `workflow-selector-option-list` for scroll geometry. The list must actually overflow after previews load. Determine first and last selectable options from rendered order rather than workflow creation order.

Extend every custom workflow with representative long stage names, not only
one long workflow among short options. Include architecture consultation,
backend and frontend development, security, integration, and release stages.
Assert that step groups wrap and the option list has no horizontal overflow.
The picker width remains bounded while each option gains height.

On desktop, hover the option list and send `page.mouse.wheel` in both directions. Poll its scroll position with Playwright assertions. Prove end options are inside the visible list and receive pointer hits before clicking them. Include the issue viewport, 1682x768, and a short desktop viewport, 1280x600.

Include small vertical deltas and diagonal deltas over visible step content.
These exercise the browser wheel-event path used by mouse wheels and trackpads.
Automated input does not establish physical-device behavior on every platform.

On `mobile-chrome`, use a touch-enabled context at 390x640. Send a genuine touch gesture within the list using Chromium CDP `Input.dispatchTouchEvent`, through `page.context().newCDPSession(page)`. Keep that helper Chromium-specific and detach the CDP session in `finally`. Touch coordinates must remain inside the measured list. Direct DOM scroll assignments are not the input regression gate.

The phone case must prove downward and upward movement in the same loaded picker. Before the upward swipe, require a positive starting `scrollTop`; require each claimed direction to send at least one gesture and observe movement. Then verify both end options remain selectable.

Every time a scenario reopens the picker, arm response waits before opening, await all scenario workflow responses, and retry ordered step-content assertions until the full previews have rendered. Complete those checks before keyboard navigation, boundary scrolling, geometry measurement, or hit testing. Do not add fixed sleeps or infer render readiness from HTTP completion alone.

Keep any shared geometry or input helper in `workflow-step-previews-helpers.ts`. Preserve the fixture's cleanup and restore any additional shared settings that the new scenarios change.

The current desktop task dialog and `mobile-create-task-launch-preview.spec.ts` are the nearest shipped exemplars. Keep the click-open contained picker. This temporary choice uses shared selection state and preview data on both viewports. The list owns scrolling, the heading stays fixed, and phone options retain 44px targets. Radix collision sizing tracks the available viewport. The parent task dialog retains its safe-area behavior.

## ASCII UI preview

### UI-01: Desktop picker, task dialog open, overflow state

Historical v0.96.0:

```text
[First workflows above viewport]
+-------------------------------------+
| Workflow 04 ...                      |
| Workflow 05 ...                      |
| ...                                 |
| Workflow 10 ...                      |
+-------------------------------------+
[Task dialog workflow trigger]
```

Current correction retained:

```text
+-------------------------------------+
| Workflow                            | <- fixed heading
|-------------------------------------|
| Workflow 01                         |
|   Start > Implement > Review        |
| Workflow 02                         | <- one vertical list
| ...                            [|]  |    wheel up/down
+-------------------------------------+
[Selected workflow v]  [Launch step]
```

### UI-02: Phone picker, task dialog open, overflow state

```text
+------------------------------+
| Workflow                     | <- fixed heading
|------------------------------|
| Workflow 01                  |
|   Start > Implement >        |
|   Review                     |
| Workflow 02                  | <- touch up/down
| ...                     [|]  |
+------------------------------+
[Selected workflow v]
```

The viewport bound, single scroll owner, wrapped previews, reachable end options, and preserved draft are structural requirements. Spacing and names are illustrative. Product labels remain localized. Both views map to AC-001.6 and AC-001.7, with the full `AC-TASKS-CREATE-WORKFLOW-STEPS` prefix.

## Tests

No new business logic requires a unit test. Existing selector and preview hook suites remain compatibility guards. Browser geometry and native input are the appropriate regression boundary.

| Criterion | Evidence |
| --- | --- |
| AC-001.7 | Desktop case `scrolls ten workflow options in both directions without losing the task draft` |
| AC-001.6, AC-001.7 | Phone case `touch scrolls ten workflow options and selects either end` |
| AC-001.5, AC-001.7 | Each case verifies unchanged draft and selection during scrolling, then the chosen workflow after activation |

## E2E tests

Add the named cases to the existing desktop and phone preview suites. Assert list overflow, native input movement, visible end-option geometry, actual hit targets, selection, and draft preservation. Check popover bounds on all four edges and absence of document horizontal overflow. Keep existing loading, retry, and launch behavior tests intact.

A regression on legacy markup fails the viewport containment and input-scroll assertions. Current main is already corrected, so the new regression can pass immediately. Do not remove the merged correction to manufacture a failing test. If native input reveals a remaining defect, stop and record its reproduction before extending this package's production scope.

## Work orders

- [x] [Task 01: Protect workflow picker scrolling](task-01-scrolling-regressions.md)

## Verification results

Investigation completed on 2026-09-30 at `ff917a370ac`:

- Managed backend and Vite E2E builds passed.
- `(cd apps/web && pnpm e2e:run --host --project chromium tests/task/task-create-workflow-step-previews.spec.ts)`: 1 passed.
- Temporary ten-workflow desktop diagnostic with wheel input: 1 passed. Geometry is recorded above.
- A second desktop diagnostic used ten workflows with at least fifteen steps
  each, including long stage names from the screenshot. It passed at 1682x768.
  The popover measured 480px wide. The list had matching clientWidth and
  scrollWidth of 472px, clientHeight=410, and scrollHeight=1520. A small wheel
  delta moved scrollTop to 60. Diagonal wheel input reached the bottom at 1110
  and returned to zero. The selected workflow and task title stayed unchanged.
- `(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-task-create-workflow-step-previews.spec.ts)`: 1 passed. This proves existing phone containment and programmatic scrolling, not the proposed native-touch regression.
- Temporary ten-workflow phone diagnostic with CDP touch gestures: 1 passed.
  The first swipe moved scrollTop from 0 to 296. The reverse swipe moved it to
  56, with Preview Kanban still selected. The permanent phone case now reaches
  both endpoints and selects each after verifying its rendered hit target.
- Specification catalog validation passed: 333 decisions and 1260 specifications.
- Full specification lint passed. Its test suite passed all 36 tests.
- The repository documentation validator accepted the work-order reference
  chain in an offline structural probe with a simulated runtime trigger.
- `git diff --check -- docs/specs docs/plans` passed.

Permanent regression coverage is complete. The managed desktop and phone suites each passed both existing and new scenarios after rebuilding the backend, Vite assets, and E2E fixture plugin. Targeted preview unit tests passed 15 tests across two files; targeted ESLint passed. The documentation catalog validated 333 decisions and 1260 specifications, full specification lint passed, and `git diff --check` passed.

Review remediation is complete. The phone case performs a down swipe to the bottom, then an up swipe from that nonzero position in the same loaded picker; the shared boundary helper requires a gesture and observed movement for each direction. The test selects both end options and checks the task draft. Every picker reopen arms waits for all scenario workflow responses before opening, then asserts all ordered step names are rendered before input or geometry checks. Ordered-content checks retry until rendering completes.

- Targeted ESLint on both task specs and the shared helper passed.
- The managed desktop task preview suite passed both tests after backend, Vite, and fixture-plugin builds.
- The managed phone task preview suite passed both tests after backend, Vite, and fixture-plugin builds.
- `git diff --check` passed after the remediation.

Review follow-up confirmed the worker fixture's existing workflow precedes the
ten scenario-created workflows. Both tests now include that workflow in preview
response and rendered-content readiness checks, and assert that the selected
endpoint IDs match the actual first and last rendered buttons. The endpoint
assertion failed with the prior scenario-only filter, which chose Preview
Kanban instead of the rendered first row. After correction, targeted ESLint and
both managed suites passed again, with 2 tests passing in each suite.

## Risks

- Direct `scrollTop` writes can pass while modal event handling blocks real user input.
- Browser focus can scroll a list independently of wheel or touch input. Capture the baseline after the picker settles.
- Workflow ordering must come from rendered options, not fixture assumptions.
- The merged correction needs a release containing `ff917a370ac` before the reporter's Docker install receives it.

## Documentation impact

Internal requirements and design clarify scrolling. Public documentation already describes the task picker. Test coverage adds no user flow or command, so no public documentation change is required.
