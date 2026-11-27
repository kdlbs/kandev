---
id: "01-shared-inspection"
title: "Shared image inspection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SHARED-IMAGE-INSPECTION-001
acceptance_criteria:
  - AC-AGENTS-SHARED-IMAGE-INSPECTION-001.1
  - AC-AGENTS-SHARED-IMAGE-INSPECTION-001.2
  - AC-AGENTS-SHARED-IMAGE-INSPECTION-001.3
  - AC-AGENTS-SHARED-IMAGE-INSPECTION-001.4
  - AC-AGENTS-SHARED-IMAGE-INSPECTION-001.5
system_design:
  - ../../specs/agents/system-design/shared-image-inspection.md
---

# Task 01: Shared image inspection

## Summary and scope

Implement the shared viewer's bounded fit/actual interaction and composer
integration, preserving all existing image source and lifecycle boundaries.
Update the existing public viewer explanation in
`docs/public/automation-and-mcp.md` and the attachment explanation in
`docs/public/tasks-and-workflows.md` with concise user-facing guidance.
The former is reference/explanation; the latter is a workflow guide. Exclude
question-viewer behavior changes, pinch/drag zoom, new preferences and network
or editor changes. Use `/tdd`, `/e2e`, and `/mobile-parity` on execution.

## Acceptance

1. All shared entry points and the direct composer caller start in Fit, switch
   to intrinsic size and back, and reset on reopen and source change without
   changing source, file reads, delivery, or disclosure.
2. Desktop keyboard and phone touch can reach controls, scroll a large image in
   both axes, dismiss, and return focus; modal and touch targets remain contained.
3. Noninteractive images and file-card replacement remain correct, while
   clarification behavior remains unchanged. Public documentation and exact
   targeted checks record their outcomes.

## UI interaction preview

UI-01 ([plan](plan.md#ui-interaction-preview)): a fitted image opens in a
bounded modal with visible Fit, Actual size, and Close actions. Actual size
scrolls only the image region while actions stay reachable. Phone uses the
same focused task with safe-area clearance and touch-sized actions. This
structure maps to AC .1-.4; visual spacing is illustrative.

## TDD and rendered evidence

Before production edits, extend `image-preview-dialog.test.tsx` with a real
dialog open and accessible mode action, pressed state, unchanged `src`, return
to Fit, reopen, and source replacement. Add a direct `ImageItem` component
test using a real context chip and controlled dialog, proving actual mode,
close/reopen and source replacement, focus return, and no delivery/remove
callback. Extend `rich-output-renderer-file-targets.test.tsx` for exact source,
read count, mode switching, and removal/replacement. Preserve its existing
v1/v2 and disclosure tests. The first RED must show the missing user-visible
actual-size capability; record the absent control as the expected product
failure rather than claiming it proves rendered geometry. In the desktop
browser baseline, measure the current fitted image as smaller than its natural
size, and record that actual-size action is unavailable. After the minimal
change, exercise the action and assert natural rendered dimensions and positive
horizontal and vertical scroll ranges; scroll to both far edges and verify
the region's scroll positions change. This establishes meaningful behavioral
GREEN rather than treating a selector-only failure as geometry proof. The
initial focused desktop RED run may establish both the missing action and the
fitted geometry; do not repeat a separate native RED run for the same evidence.

Add focused `e2e/tests/chat/image-inspection.spec.ts` and
`e2e/tests/chat/mobile-image-inspection.spec.ts`, reusing the existing fixture
and image seeding patterns. Desktop: open a real rich-output image by keyboard,
measure fit and actual geometry, scroll with keyboard, return to Fit, Escape,
focus and reopen; assert workspace read count unchanged after opening/switching
and replacement dismisses. Exercise composer preview with a locally generated
image, its controlled-dialog focus return and reopened Fit. Phone: tap actual
and Fit, use touch input to scroll a large image, assert both axes and target
boxes (at least 44px and inside the dialog), safe-area clearance, background
containment, document overflow zero, close/reopen, and composer access. Include
a narrow fine-pointer viewport and small image; verify no unnecessary overflow.
Capture and inspect desktop and phone screenshots from the fresh production
build. Before another capture-enabled managed run clears `.pr-assets`, copy
only this task's screenshots and manifest to its external evidence directory
and record their hashes. Preserve those originals; do not recapture a passing
UI solely because tests changed. Keep cleanup confined to test-owned files.

## Verification

Only after the separate implementation and global HEAVY lease grant, install
once in this fresh worktree. Run commands sequentially from the repository root
with pinned PATH (pnpm 9.15.9 installation directory without `/bin`, Node
24.21.0 `bin`, Go 1.26.0 `bin`), `GOTOOLCHAIN=local`, `GOMAXPROCS=2`, and
`GOFLAGS=-p=2`. For every wrapped command set a distinct external
`KANDEV_RUN_QUIET_DIR`; retain and join every native process handle. Record RED
and GREEN separately. Do not run full suites or global cleanup. Inspect the new
specs and any reused helper for `describe.configure` overrides before running;
remove any parallel/retry override in the focused scenarios.

```sh
pnpm --dir apps install --frozen-lockfile
scripts/run-quiet vitest -- pnpm --dir apps/web exec vitest run --maxWorkers=1 --no-file-parallelism components/task/chat/image-preview-dialog.test.tsx components/task/chat/context-items/image-item.test.tsx components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx
scripts/run-quiet web-lint -- pnpm --dir apps/web exec eslint components/task/chat/image-preview-dialog.tsx components/task/chat/image-preview-dialog.test.tsx components/task/chat/context-items/image-item.tsx components/task/chat/context-items/image-item.test.tsx components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx e2e/tests/chat/image-inspection.spec.ts e2e/tests/chat/mobile-image-inspection.spec.ts
scripts/run-quiet i18n -- pnpm --dir apps/web run i18n:check
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/chat/image-inspection.spec.ts -- --workers=1 --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/chat/mobile-image-inspection.spec.ts -- --workers=1 --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Use distinct run-quiet directories even for RED versus GREEN. The first browser
command builds the backend, web E2E assets, and fixture plugin. Record a bounded
receipt immediately after the build: base commit, build-relevant tracked and
untracked changes and hashes of the task's changed build inputs, relevant build
configuration and lockfiles, pinned tool versions and build-affecting
environment, and hashes of the produced backend, delivery fixture, web entry
and referenced assets, and plugin package. Before phone `--no-build`, compare
that receipt to the current inputs, configuration, tools, environment and
artifacts; if any differ or cannot be established, omit `--no-build` and
rebuild. Do not hash the entire repository or sweep global state. Test-only
changes do not require recapture when the relevant build receipt still matches.
Add any new shared E2E helper to the focused lint command. Preserve actual
command output and failure context; no claimed pass without a joined exit.

Reuse the existing translated question-image labels. `i18n:check` includes
pseudo-catalog completeness. If any new copy is necessary, update all seven
real locale catalogs, generate Traditional Chinese via `i18n:zh-hant` and
generate the pseudo catalog before checking. Broad typecheck is not a planned
gate; run it only if an actual unresolved type concern warrants it.

## Likely files and dependencies

- `apps/web/components/task/chat/image-preview-dialog.tsx` and its test.
- `apps/web/components/task/chat/context-items/image-item.tsx` and new test.
- `apps/web/components/task/chat/context-items/context-chip.tsx` and its test,
  plus `apps/web/components/task/chat/messages/chat-message.test.tsx` for the
  existing attachment preview contract.
- `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx`.
- The two focused desktop/phone E2E specs and a shared helper only if needed.
- `docs/public/automation-and-mcp.md`, `docs/public/tasks-and-workflows.md`,
  paired specs and this plan/work order for lifecycle/results.

No dependencies or parallel work orders. Preserve the source-specific dialog
lifetimes, avoid unrelated clarification refactors, and check exact focus
restoration rather than assuming Radix handles the controlled composer.

## Results

The focused Vitest RED failed on the absent mode controls in the shared,
composer, and v1/v2 rich-output paths. Desktop browser RED first measured a
fitted large image and then found Fit unavailable; that failure did not itself
prove actual-size geometry. After implementation, 31 focused component tests
passed. The final desktop browser run passed two scenarios, including keyboard
scrolling, composer focus/reset, and a small image at a narrow fine-pointer
viewport. The phone browser run passed with actual touch gestures on both axes,
measured control targets and containment, and composer focus/reset. The first
phone run exposed a narrow Fit target; the final run passed after correction.
The next phone run showed a tap before touch momentum settled; a gesture-settle
wait corrected the test, and the same build passed without recapturing.
An additional regression exposed image element replacement during a mode
switch; the final implementation preserves the element to avoid a needless
reload. The focused component suite and both browser projects passed again.

Scoped ESLint and i18n checks passed, including pseudo-catalog validation.
A test-only duplicate-string warning was corrected after the normal hook
rejected warnings; the affected test passed again. Desktop and phone screenshots and
manifests were preserved with hashes in the task evidence directory. The phone
`--no-build` run followed a successful bounded receipt comparison. The public
viewer and attachment guidance was updated. Specification catalog and lint,
and both public-document validators passed. Final diff checks and hook results
are recorded in the task handoff.

Hosted CI subsequently exposed a stale attachment fit-class assertion and the
composer image wrapper's violation of the Quick Chat context-row direct-child
contract. The fix passes the main-button ref through `ContextChip`, removes the
wrapper, and checks the attachment's mode behavior instead of the old class.
The affected five-file Vitest suite passed, including both ref branches and
focus restoration. Fresh managed desktop runs passed the Quick Chat case and
two image-inspection cases; receipt-matched phone runs passed Quick Chat and
image inspection. Scoped ESLint passed with zero warnings. Existing Quick Chat
assertions and the prior UI captures were preserved.
