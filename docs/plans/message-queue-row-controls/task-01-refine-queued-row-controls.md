---
id: "01-refine-queued-row-controls"
title: "Refine queued row controls"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-MANAGEMENT-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.9
  - AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.10
system_design:
  - ../../specs/ui/system-design/message-queue-row-controls.md
---

# Task 01: Refine Queued Row Controls

## Summary

Make queued-message disclosure depend on rendered overflow and remain responsive to live layout changes. Align the disclosure and Remove controls with the shared action-row order, iconography, destructive hover cue, and phone touch behavior.

## In scope

- Add focused overflow tests. Include a boundary sequence that first establishes real overflow/disclosure, then changes geometry so only a layout-participating disclosure causes overflow. After a signal, disclosure disappears and stays absent. Assert the probe sets inline `display: none` before geometry reads and restores the exact prior value in `finally`, including a throwing getter. Retain exact opposing, signal, fallback, guard, cleanup-order, and unmount coverage.
- Add desktop exact-title E2E. Use `queue-entry-actions`; when disclosure appears, evaluate direct children and require its `nextElementSibling` to be `queue-entry-remove`, with Remove the final actionable child. Retain authoritative setup, fit/overflow/fit, opacity, role/name, title, color, and removal assertions.
- Add Pixel 5 exact-title E2E with exact panel, scroll-region, fixture-filtered row, preview, actions, expand, and Remove test-ID locators. Require document width equality and scroll-region `overflowY === "auto"`. Evaluate panel plus all descendants, filter computed overflow-Y `auto|scroll`, map `data-testid`, and require exactly `["queue-scroll-region"]`. With `EPSILON = 1`, use explicit left/right inequalities for panel within viewport, scroll region/row/preview/actions within their named parent, and both Expand and Remove within actions. Require preview `scrollWidth <= clientWidth + EPSILON` and effective expand/Remove width and height each at least 44. Retain media, adjacency, icon, accessibility, color, capture, and removal.
- Extract measurement to `use-queued-message-overflow.ts`. Return preview and disclosure-button refs plus `canExpand`; keep `DisplayView` state/rendering ownership. Pass the button ref through row actions and expose `data-testid="queue-entry-actions"`.
- Probe at no-disclosure width by saving inline `display`, setting it to `none`, reading geometry synchronously, and restoring exactly in `finally` before state updates. Guard callbacks/lifecycles as specified; use callable observer detection with additive viewport, descendant-load, and font-completion signals.
- Reorder disclosure as Remove's direct DOM predecessor; replace close with `IconTrash`; use `variant={null}`, local ghost background, `text-muted-foreground`, and `[@media(pointer:fine)]:hover:text-destructive`.
- Run the focused Vitest, desktop Playwright, and mobile Playwright tests, then the user-required repository checks.

## Out of scope

- Production backend, WebSocket, store, queue mutation, permission, merge, ordering, capacity, or Auto-run behavior changes. Tests may invoke the existing Auto-run operation through the typed E2E API-client wrapper.
- Queue-panel restructuring, new responsive breakpoints, drawers, menus, or scroll owners.
- Translation catalog or public-documentation changes.
- Generalizing or refactoring unrelated expandable content.

## Acceptance

- Component tests prove exact opposing geometry, self-induced-overflow escape, exact inline-style restoration including exceptions, signals, fallback, and every lifecycle guard.
- Both E2Es assert direct disclosure/Remove adjacency and terminal Remove. Desktop asserts fine-pointer media, muted idle color, then destructive hover color. Pixel 5 asserts coarse-pointer media, rejects fine-pointer media, and requires muted Remove color before tap. Accessibility, opacity, icon, geometry, and touch contracts remain.
- Pixel 5 uses the exact locator chain and `EPSILON = 1`. It proves exact document width, sole `auto|scroll` overflow-Y descendant ownership by `queue-scroll-region`, explicit nested left/right bounding-box containment through panel/scroll-region/row/preview/actions and for both Expand and Remove within actions, preview content width, and effective 44-by-44 minimums for Expand and Remove before retained screenshot and last-step removal.

## Verification

Bootstrap once if `apps/node_modules` is absent:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

RED, before editing production files:

```bash
(cd apps && pnpm --filter @kandev/web test -- components/task/chat/queued-ghost-message-overflow.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/message-queue.spec.ts -- --grep "adapts queued row controls to rendered overflow")
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-message-queue-management.spec.ts -- --grep "keeps queued row controls ordered and touchable")
```

All three commands must discover their intended tests and fail on the new behavioral assertions rather than setup, build, import, or timeout errors.

GREEN, after the minimum production change, rerun all three commands:

```bash
(cd apps && pnpm --filter @kandev/web test -- components/task/chat/queued-ghost-message-overflow.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/message-queue.spec.ts -- --grep "adapts queued row controls to rendered overflow")
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-message-queue-management.spec.ts -- --grep "keeps queued row controls ordered and touchable")
```

After the normal Pixel 5 command is GREEN, generate retained visual evidence with `(cd apps/web && CAPTURE_PR_ASSETS=true pnpm e2e:run --project mobile-chrome tests/chat/mobile-message-queue-management.spec.ts -- --grep "keeps queued row controls ordered and touchable")`. The test must destructure `prCapture` and invoke `prCapture.screenshot("queued-row-controls-pixel-5", { caption: "Pixel 5 queued row with adaptive disclosure immediately before the touch-sized Remove action" })` before removal. Require both `apps/web/.pr-assets/mobile-message-queue-management--queued-row-controls-pixel-5.png` and its entry in `apps/web/.pr-assets/manifest.json`; inspect the PNG and record that path plus concrete hierarchy, clipping, overlap, spacing, and phone-composition observations under Verification results. Missing files or manifest entry fail verification.

Then run the required repository checks in this order from the repository root:

```bash
make fmt
make typecheck test lint
```

Confirm final whitespace separately:

```bash
git diff --check
```

## Files likely touched

- `apps/web/components/task/chat/queued-ghost-message.tsx`
- `apps/web/components/task/chat/use-queued-message-overflow.ts`
- `apps/web/components/task/chat/queued-ghost-row-actions.tsx`
- `apps/web/components/task/chat/queued-ghost-message-overflow.test.tsx`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/chat/message-queue.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`
- `docs/plans/message-queue-row-controls/plan.md`
- `docs/plans/message-queue-row-controls/task-01-refine-queued-row-controls.md`

## Dependencies

None.

## Risks

- A pending row can drain before geometry assertions. Both Playwright scenarios must use a suite-level `test.describe.configure({ timeout: 120_000 })` so the deadline covers fixture setup, establish a 60-second running turn, await `waitForActiveSessionForegroundActivity(..., "generating")`, set Auto-run false through `ApiClient.setQueueAutoRun` before target admission, assert its authoritative response, wait for composer queue mode, then queue the fixture, poll queue status for false plus the expected pending count, and confirm its row. Desktop may use `seedRunningGeneratingSession`; Pixel 5 must submit `/sleep 60` with `sendMessageViaButton` because keyboard submission is unsupported there. Exact fixture contents and geometry preconditions separate setup failure from behavioral RED. Causal/backend waits, rather than fixed delays, preserve the invariant through every pre-removal assertion.
- Stale-callback tests cover foreign target, same-element generation, replaced-element identity, and unmount. Invoke all captured callback types after unmount. Separately, make mocked listener removal and observer disconnect synchronously invoke captured callbacks during cleanup. Every path asserts no geometry getter, `setExpanded`, or render/state output, behaviorally proving invalidation happens before teardown calls.
- Expanded measurement suppresses disclosure and compares natural height against the stored collapsed no-control cap. Manual and automatic collapse refresh final geometry with no transition.
- The optional button can create a flex-width fixed point. Boundary tests make it the sole overflow cause and require removal. The probe must restore inline display in `finally`; no permanent blank slot or visible two-pass state is allowed.
- Signals are asynchronous and additive. Observer-present tests prove each. Capture-phase descendant loads and document font completion remeasure rendered content that changes without a preview-box resize. Fallback stubs `ResizeObserver` undefined, asserts no constructor or observe call, and proves initial/window/visual-viewport behavior; implementation uses callable-value detection. Optional visual viewport, cleanup, absent previews, and stale callbacks remain covered.
- Both the existing component and test files are near `max-lines`. Keep all new measurement logic in `use-queued-message-overflow.ts` and all new component coverage in `queued-ghost-message-overflow.test.tsx`; do not grow the near-limit files.
- Remove cannot retain ghost variant because it injects `hover:text-foreground`. Use `variant={null}`, local ghost backgrounds, and `[@media(pointer:fine)]:hover:text-destructive`. Tests prove destructive hover only with a fine primary pointer and muted color under Pixel 5 coarse-pointer media.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/message-queue-management.md`, especially `AC-UI-MESSAGE-QUEUE-MANAGEMENT-001.9` and `.10`.
- `docs/specs/ui/system-design/message-queue-row-controls.md`.
- `apps/web/components/task/chat/anchored-last-prompt-bar.tsx` as the rendered-overflow observation exemplar.
- Existing row-action and mobile queue patterns in `queued-ghost-row-actions.tsx`, `queued-ghost-message.test.tsx`, and `mobile-message-queue-management.spec.ts`.

## Results

- RED: the focused Vitest command discovered all 14 intended tests; 13 new
  behavioral assertions failed before production changes. The desktop and
  Pixel 5 exact-title tests each reached their geometry preconditions and
  failed on the missing adaptive disclosure/action-container contracts.
- GREEN: the focused Vitest command passed all 14 tests. The exact desktop
  Chromium test and exact mobile-chrome Pixel 5 test each passed once.
- Extracted rendered-overflow measurement into
  `use-queued-message-overflow.ts`, including no-control-width probing, exact
  inline-style restoration, additive layout signals, collapsed-cap behavior,
  and stale-callback guards.
- Review remediation: descendant resource loads and font completion now remeasure a preview whose box remains capped while rendered content grows; focused Vitest coverage passed all 16 cases.
- Reordered disclosure immediately before terminal Remove, switched Remove to
  the trash icon, preserved muted idle presentation, and limited destructive
  hover color to fine primary pointers.
- Retained Pixel 5 evidence at
  `apps/web/.pr-assets/mobile-message-queue-management--queued-row-controls-pixel-5.png`
  with a matching manifest entry. Inspection confirmed the preview/action
  hierarchy, no viewport clipping or action overlap, usable control spacing,
  and balanced phone composition.
- `make fmt`, `make typecheck`, and `make lint` passed. The combined
  `make typecheck test lint` reached unrelated environment-sensitive failures:
  the web suite passed 15,279 tests but reported 28 Docker-gateway/time-limit
  failures across 7 unrelated files under Node 22; script tests require an
  unavailable `unzip`; backend discovery/cache tests depended on the host
  login profile and installed-tool path. Focused reruns passed after isolating
  the relevant backend tool paths.
- `python3 scripts/lint-spec-files.py --all` and `git diff --check` passed.
