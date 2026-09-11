---
id: "02-repair-popup-positioning"
title: "Repair confirmed popup positioning"
status: done
wave: 2
depends_on: ["01-record-iphone-comparison"]
plan: "plan.md"
requirements:
  - REQ-UI-COMPOSER-OVERLAY-001
acceptance_criteria:
  - AC-UI-COMPOSER-OVERLAY-001.1
  - AC-UI-COMPOSER-OVERLAY-001.2
  - AC-UI-COMPOSER-OVERLAY-001.3
  - AC-UI-COMPOSER-OVERLAY-001.4
  - AC-UI-COMPOSER-OVERLAY-001.5
system_design:
  - ../../specs/ui/system-design/composer-suggestion-overlays.md
---

# Task 02: Repair confirmed popup positioning

## Summary

After the phone comparison confirms a geometry cause and the user explicitly
requests implementation, reconcile the owning system design and repair the
shared popup. If activation is the cause, revise this work order before coding.

## In scope

Browser-coordinate normalization, non-collapsing containment, viewport reflow,
and focused tests. Keep the existing above/below placement contract, portal,
one scrolling list, selection and focus semantics.

## Out of scope

Search changes, input/IME rewrites, new drawers, global overlays, unrelated
clipboard problems, automatic upgrades, or production diagnostic probes.

## Acceptance

1. A regression reproduces the confirmed measurements before the repair and
   passes afterward; an off-screen anchor cannot collapse an otherwise usable
   picker. Ordinary adjacency and below placement remain correct.
2. Both `@` and `#` remain visible/selectable before Done on the reporting
   device; automated tests prove internal scrolling, 44-pixel rows, retained
   focus, no implicit send, and viewport reflow without retyping.
3. The final implementation lives in the shared React primitive, not in a
   proxy, mutation observer, or independent DOM-styling layer.

## ASCII UI preview

UI-01 excerpt from the [full preview](plan.md#ascii-ui-preview):

```text
Suggestions (heading + scrolling rows)
Composer: @mobile  [focus stays here]
iOS keyboard      [remains open]
```

Desktop retains ordinary caret adjacency without the keyboard. Maps to all
five criteria above; the phone capture verifies the order and reachability.

## Verification

```bash
(cd apps/web && pnpm test -- components/task/chat/popup-menu.test.tsx)
(cd apps && pnpm --filter @kandev/web build:e2e)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome \
  tests/chat/mobile-prompt-mention-composer.spec.ts \
  tests/chat/mobile-entity-reference-composer.spec.ts \
  tests/chat/mobile-slash-command-composer.spec.ts \
  tests/task/mobile-task-create-escape.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project chromium \
  tests/chat/entity-reference-composer.spec.ts \
  tests/chat/slash-command-composer.spec.ts \
  --grep 'task chat restores a keyboard-selected draft|selecting a slash command keeps it as an editable draft|keeps Kandev task suggestions under @')
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/lint-spec-files.py --all
git diff --check
```

Define an exact targeted WebKit command in this work order when its configured
project exists; do not list a nonexistent project as runnable verification.
The user confirmed the Task 01 physical-device candidate, then requested
shutdown and production integration. Report that confirmation separately from
automated integrated-patch results; do not claim a second physical-device run.

## Files likely touched

- `apps/web/components/task/chat/popup-menu.tsx` and `popup-menu.test.tsx`.
- The existing mobile `@` and `#` E2E files, and Playwright configuration if a
  runnable focused WebKit lane is added.
- `apps/web/package.json` and `apps/pnpm-lock.yaml` only if declaring the
  existing positioning library as a direct dependency.
- The owning system design, this work order, and `plan.md`.

## Dependencies

Task 01 and a later explicit implementation request.

## Risks

Browser emulation cannot certify an actual iOS keyboard. Late positioning
callbacks must not update closed menus. Preserve short-result sizing and the
plan editor's below placement; do not apply the above-menu experiment blindly
to every shared consumer.

## Parallelism

sequential

## Inputs

Task 01 evidence, active overlay requirements, current system design, and
the earlier mobile-composer-suggestion-viewport package.

## Results

The shared popup now delegates client-to-fixed coordinate normalization to
Floating UI's real DOM platform. Size-before-shift preserves short and long
list adjacency where usable; an occluded above anchor can use the padded
viewport without collapsing. The heading and list share a constrained flex
column. React owns all subscriptions and disposal; both stale promises and
already-queued callbacks are ignored after cleanup. Search, suggestion plugins,
selection handlers, and draft serialization are unchanged.

Red evidence: the original helper returned zero height for the recorded
Safari-like client/viewport inputs, and the new offset-viewport mobile prompt
test failed its reachability assertion against the old build. Additional
focused red tests exposed missing-to-ready anchor initialization and a queued
callback after disposal; both are covered in the integrated unit suite.

The first integrated mobile run passed all geometry and touch-selection
assertions but one transcript assertion compared `innerText` with
`textContent`. It was corrected to check persisted message count through the
API, as the sibling external-reference test does. Final results follow below.

- Unit suite: 16 passed.
- Final fresh-build mobile E2E: 7 passed (38.5 seconds). The final run used
  `CAPTURE_PR_ASSETS=true` and `--no-build` after a successful `build:e2e` of
  the final production changes.
- Selected desktop E2E: 3 passed (27.7 seconds), covering `@`, `#`, and `/`
  keyboard selection, draft restoration, and explicit submission.
- Full lint and final changed-file lint, typecheck, i18n ratchet, specification
  lint, and whitespace check: passed.
- Inspected `apps/web/.pr-assets/mobile-prompt-mention-composer--mobile-composer-prompt-menu.png`:
  contained, caret-adjacent heading/list above the focused phone composer.
- Original phone candidate: user-confirmed. Integrated physical-device run:
  not repeated after the user's requested shutdown. WebKit host launch remains
  unavailable because GTK/GStreamer dependencies are missing; no host package
  installation was performed.
- Test instance stopped and temporary evaluation data/diagnostic ZIPs deleted.
  Final listener check shows only the unchanged main PID 3960526 on port 9998;
  evaluation ports 48761, 48762 and 50761 are closed.

Ready for Open PR. No commit, push, or PR was created during this phase.

## PR review follow-up

Rendering and middleware now share `POPUP_MENU_SIZE`; the geometry and visible
appearance remain unchanged. The signed-overflow invariant and E2E minimum
height are documented. A stable mention-surface test ID replaces wrapper
traversal, geometry tolerances are symmetric, missing-anchor assertions include
hidden DOM, and mocked task references use valid synthetic UUIDs instead of
creating unused backend rows. The phone comparison explicitly marks original
selection acceptance criterion 3 unverified and its instructions historical.

Two review-driven contract tests passed before the constant refactor: initially
narrow menus still recover the padded viewport width, and same-size results
refresh a stable virtual-caret callback. A disposable check of the proposed
`children`-dependency removal failed with menu bottom 232 instead of 192;
restoring the dependency passed. Keep that required update path.

Post-review local checks:

- `pnpm test -- components/task/chat/popup-menu.test.tsx`: 18 passed.
- Fresh-build mobile command above: 7 passed. After final test-selector and
  fixture refinements, reran the two changed mobile `@`/`#` specs with
  `pnpm e2e:run --host --no-build --project mobile-chrome` after `build:e2e`:
  4 passed.
- The selected desktop command above: 3 passed.
- Web typecheck, changed-file ESLint, specification lint, and whitespace check:
  passed. Normal commit hooks remain required.

No public contract or rendered-UI behavior changed, so the owning design and
published screenshots remain representative. Remote checks and review
dispositions remain pending until the refinement commit's CI completes.

## CI follow-up

The full frontend job exposed two existing entity-reference menu assertions
that queried accessible options synchronously, before asynchronous popup
positioning made them visible. Both failures reproduced on the CI-equivalent
merge tree. The assertions now await accessible options with `findByRole`;
they still require visible options and successful selection, without a
positioning mock, hidden-element query, or increased timeout.

- `CI=true pnpm exec vitest run components/task/chat/entity-reference-menu.test.tsx components/task/chat/popup-menu.test.tsx`:
  23 passed after the test-only correction.
- Changed-file ESLint, web typecheck, and whitespace check: passed.

This frontend correction changes no production behavior or public contract.

The container-shard failure exposed a separate SSH recovery defect. Replaying
the exact 23-test CI shard without retries reproduced its teardown timeout.
A disposable probe forced recovery after a second backend restart and captured
the same failure: authenticated health requests used the intermediate local
execution ID, while the remote controller still expected its original launch
ID. Readiness retries held a lifecycle read lock long enough to block task
deletion past its cleanup deadline.

SSH now persists the remote controller's launch identity independently of
replacement local execution IDs. Same-session recovery retains it; sibling
sessions and replacement controllers do not inherit it. Legacy rows keep the
existing previous-execution fallback and record that identity for subsequent
recovery. The existing browser test now verifies two successive reconnects.
The [SSH recovery design](../../specs/executors/system-design/ssh-executor.md#recovery-after-backend-restart)
records this internal contract. Public executor instructions remain unchanged:
this restores their documented live-controller reuse, with no new setting or
operator action.

- New Go regression failed on recovery 2 before the fix, for both client
  construction paths. Fresh-controller identity and metadata lifecycle tests
  also failed before the fix.
- `go test ./internal/agent/runtime/lifecycle -run 'Test(SSH|ResumedSSH|ShouldPersist|FilterPersistent)' -count=1`:
  passed after the fix.
- `go test ./internal/agent/runtime/lifecycle -count=1`: passed (53.481 seconds).
- `golangci-lint run ./internal/agent/runtime/lifecycle/... --new-from-rev=origin/main --timeout=5m`:
  passed with zero issues.
- Web typecheck, changed-file ESLint, specification lint, and whitespace check:
  passed.

- The disposable forced-order SSH probe passed after the repair (27.9 seconds),
  then its fixture instrumentation and temporary ordering code were removed.
- `CI=true KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --no-build --project containers tests/ssh/add-workspace-sources.spec.ts -- --retries=0 --trace=retain-on-failure`:
  the permanent two-restart test passed (26.9 seconds), using CI's Node 24.20.0
  and freshly rebuilt backend/helper/plugin artifacts on the CI-equivalent
  merge tree with the repair applied.

The final remote CI/review gate and current-base integration validation remain
pending until the SSH correction is delivered and checked.
