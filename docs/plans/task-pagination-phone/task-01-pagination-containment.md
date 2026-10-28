---
id: "01-pagination-containment"
title: "Contain task pagination and prove navigation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-PAGINATION-PRESENTATION-001
acceptance_criteria:
  - AC-UI-TASK-PAGINATION-PRESENTATION-001.1
  - AC-UI-TASK-PAGINATION-PRESENTATION-001.2
  - AC-UI-TASK-PAGINATION-PRESENTATION-001.3
system_design:
  - ../../specs/ui/system-design/task-pagination-presentation.md
---

# Task 01: Contain task pagination and prove navigation

## Summary

Make the task-list pagination row fit phone widths while retaining existing page
selection and desktop behavior. First independently reproduce the real
middle-page containment failure, then make the smallest local correction.

## In scope

- The local `TasksPagination` layout and its real component and browser tests.
- Isolated managed backend test data without agents, and actual scrolled-footer
  screenshots for both phone widths.

## Out of scope

Shared pagination/button primitives, global sizing/breakpoint hooks, fetching,
controller, settings, mobile navigation, adjacent saved-view contracts, new
copy, dependencies, harness changes, and public documentation changes.

## Acceptance

- At 320, 393, and 430px the actual page 4 of 7 footer has contained, nonoverlapping
  44px actions, no unintended horizontal scroll, and working Next, Previous,
  and direct-page actions; the range and current-page status follow navigation.
- At 1280px desktop retain the single row, first/last and direct actions, compact
  sizing, keyboard activation, and correct boundary disabled states. Exercise a
  narrow fine-pointer viewport and widths immediately around the chosen layout
  breakpoint, including coarse-pointer 640px; phone-width controls retain 44px
  hit areas. Measure the complete footer and its ancestors, including the range
  and selector, for containment and horizontal overflow, not only the page row.
- Component and real managed E2E tests are independently authored; a real
  pre-fix browser run fails specifically on the middle-page containment while
  ordinary page-1 controls and navigation pass. The same targeted checks pass
  after the local repair.

## UI composition

[UI-01 and UI-02 in the plan](plan.md#ui-composition) define the contract.
Phone: range and rows-per-page precede the same ordered action set, which wraps
in the list's vertical scroll region, with local bottom clearance for the
existing floating add button and its safe-area inset; no horizontal pan or new
overlay. Desktop:
existing compact single row including first/last. Verify actual footer geometry
and actions for AC .1-.3, not class strings or an unscrolled screenshot.

## TDD sequence

1. Inspect fixture APIs, the current component, E2E resource guidance, and the
   managed runner. Seed 70 rows via existing `apiClient.createTask` in the
   isolated backend, without agent starts. Author the tests independently;
   never read or reuse qualification fixture source. Use stable locators scoped
   to the footer and causal waits for the resulting page/range.
2. Run the mobile native test against unmodified production with page 1 control
   assertions first. Select 10 rows, tap Next through page 4, scroll vertically
   to the footer after each navigation, and assert full containment. Record a
   causal RED at both widths with actual exit and ordinary control results;
   a build, setup, discovery, or transport failure is not a product RED.
3. Apply the minimal local responsive wrapping and shrinkability repair. Choose
   the responsive classes using rendered geometry, including coarse 640px where
   the range and selector share space with the page row. Keep
   every displayed number, label, callback, boundary rule, and page-size behavior.
   Confirm the responsive CSS does not change the desktop row or shared controls.
4. Run the component, mobile, and desktop checks below. Capture actual visible
   footer screenshots at both phone widths after vertical-only scrolling and
   inspect them for clipping and hierarchy. Test Next 4 to 5, Previous, and direct
   page selection with native taps; verify range/current page after each. For all
   displayed controls compare their bounding boxes to both viewport and list
   surface, test 44px width and height and nonoverlap, and check document and
   list scroll widths. Check keyboard activation on desktop.
5. Record actual exits, RED/GREEN, screenshot paths and checksums, exact input and
   built asset identity for any `--no-build` reuse, and remaining risks in this
   work order and plan. Mark both complete only after checks succeed.

## Verification

After a later explicit implementation and global heavy-resource release, use
pnpm 9.15.9 directory ahead of Node 24.21.0/bin and Go 1.26.0/bin on `PATH`.
From the repository root, bootstrap this worktree once with
`(cd apps && pnpm install --frozen-lockfile)`. Run the commands below from the
repository root.
Use a unique task-owned `KANDEV_RUN_QUIET_DIR` for every invocation; the paths
below are distinct examples under the owned evidence directory. Preserve all
native command handles and actual joins. Run serially; do not repeat passing
suites without a new reason. The first E2E run builds; after code changes use a
fresh build for GREEN. `--no-build` is permitted only when exact source, tool,
backend and dist identity is recorded before and after reuse. Managed clears can
remove captures, so preserve only owned screenshots and their manifest/checksums
outside the cleared locations before the next run.

```bash
KANDEV_RUN_QUIET_DIR=/tmp/kandev-child031-red-mobile scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-task-pagination.spec.ts -- --workers=1 --retries=0
KANDEV_RUN_QUIET_DIR=/tmp/kandev-child031-component scripts/run-quiet frontend-test -- pnpm --dir apps/web exec vitest run --project browser-locales --maxWorkers=1 --no-file-parallelism app/tasks/tasks-pagination.test.tsx
KANDEV_RUN_QUIET_DIR=/tmp/kandev-child031-green-mobile scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-task-pagination.spec.ts -- --workers=1 --retries=0
KANDEV_RUN_QUIET_DIR=/tmp/kandev-child031-green-desktop scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/task-pagination.spec.ts -- --workers=1 --retries=0
KANDEV_RUN_QUIET_DIR=/tmp/kandev-child031-lint scripts/run-quiet frontend-lint -- pnpm --dir apps/web exec eslint --max-warnings=0 app/tasks/tasks-pagination.tsx app/tasks/tasks-pagination.test.tsx e2e/tests/task/mobile-task-pagination.spec.ts e2e/tests/task/task-pagination.spec.ts e2e/helpers/task-pagination.ts
KANDEV_RUN_QUIET_DIR=/tmp/kandev-child031-typecheck NODE_OPTIONS=--max-old-space-size=6144 scripts/run-quiet frontend-typecheck -- pnpm --dir apps/web run typecheck
```

If the shared E2E helper is unnecessary, omit its lint argument. Inspect any
`describe` retry override and ensure zero retries. Select each project explicitly
and verify discovery; retain the actual native handle and join for every run.
Do not run these commands before release.

## Files likely touched

- `apps/web/app/tasks/tasks-pagination.tsx`
- `apps/web/app/tasks/tasks-pagination.test.tsx`
- `apps/web/e2e/tests/task/mobile-task-pagination.spec.ts`
- `apps/web/e2e/tests/task/task-pagination.spec.ts`
- `apps/web/e2e/helpers/task-pagination.ts` only if shared helper is needed
- This work order and `plan.md` for status/results

## Dependencies

None within this package. Implementation awaits full review of all four
artifacts and a later explicit release, including the global heavy lease.

## Public documentation assessment

The existing task workflow documentation and screenshots contain no description
of this pagination presentation. This local containment repair adds no label,
setting, workflow, or public contract; no public documentation update is needed.
Reassess if implementation expands the user-visible behavior.

## Results

- The real managed mobile RED joined with exit 1: page 1 and real navigation to
  page 4 passed, then containment failed at 320px (right edge 356 > 321) and
  393px (402 > 394). No production edit preceded the run.
- The component suite passed 2 tests; the fresh managed mobile run passed 2
  cases and the fresh desktop run passed 1 case. The later mobile-only matrix
  passed both cases with 1023/1024 included, using `--no-build` after verifying
  identical source, tool versions, backend/fixture artifacts and dist before
  and after. Native taps cover Next 4 to 5, Previous, and direct page 3;
  desktop covers first/last and keyboard selection. All visible enabled controls
  are hit-tested, with 44px phone targets and containment checks.
- Scoped ESLint, subsequent changed-spec ESLint, and web typecheck with a 6 GiB
  heap passed. Phone screenshots were captured after scrolling to the actual
  footer and inspected at both widths. Public documentation search found no
  pagination presentation needing revision; no user-facing copy was added.
- Additional 430px native RED: after page 1 controls and taps through page 4,
  the actual footer screenshot shows the floating add button covering Next; the
  enabled-control center hit test failed (handle 29538, exit 1). The same
  no-build run had matching source, artifact, and tool fingerprints before and
  after. Reserve a local phone-only bottom inset of 6rem plus the bottom safe
  area, restoring existing desktop padding at 768px. A fresh build and mobile
  matrix then passed 3 cases, including the 430px hit test and actual Next tap;
  the affected desktop/narrow-fine E2E passed 1 case with verified identical
  no-build inputs and artifacts. The component suite passed 2 tests and scoped
  lint passed after the clearance change. Screenshots of all three actual phone
  footers were retained with checksums.

## Required-CI remediation

The current PR head's E2E shard 18 failed the pseudo-locale update-notification
test, independently of the pagination change. The actual job log and its
`test-results-18` artifact show a strict locator matching two update toasts:
the live v0.98.0 notice and the synthetic v1.2.3 notice share the localized
title. All four attempts failed at the same assertion. The existing title-only
locator cannot identify the synthetic event when another notice is visible.

After separate review and release, applied only the already-landed correction
from commit `840de1aeb7e32cea17545106378e824fd57dce55` to
`apps/web/e2e/tests/i18n/pseudo-coverage.spec.ts`: explicitly emit a second
distinct update, assert both coexist, select the v1.2.3 toast by localized title
and version, and assert the full translated body. This preserves the locale
test and makes the overlapping-notification case deterministic. Do not rebase
or alter pagination production code, unrelated tests, or the fixture runtime.

The hosted shard's actual failure was the causal RED. The named real managed
Chromium case passed with one worker and no retries; scoped lint also passed.
The no-build run used identical source, tool versions, backend/fixture artifacts
and dist before and after. The corrected test blob exactly matches the cited
main commit. A new head receives normal CI; no manual CI job retry was used.

```bash
KANDEV_RUN_QUIET_DIR=/tmp/kandev-child031-pseudo-green scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project chromium tests/i18n/pseudo-coverage.spec.ts -- --workers=1 --retries=0 --grep 'localizes a Kandev update notification received from the backend'
KANDEV_RUN_QUIET_DIR=/tmp/kandev-child031-pseudo-lint scripts/run-quiet frontend-lint -- pnpm --dir apps/web exec eslint --max-warnings=0 e2e/tests/i18n/pseudo-coverage.spec.ts
```
