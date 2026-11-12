---
id: "02-prove-render-continuity"
title: "Prove desktop and phone input continuity"
status: done
wave: 2
depends_on:
  - "01-preserve-terminal-lifecycle"
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-TERMINAL-001
acceptance_criteria:
  - AC-UI-QUICK-TERMINAL-001.8
  - AC-UI-QUICK-TERMINAL-001.13
  - AC-UI-QUICK-TERMINAL-001.21
  - AC-UI-QUICK-TERMINAL-001.22
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
  - ../../specs/ui/system-design/quick-terminal-render-lifecycle.md
---

# Task 02: Prove desktop and phone input continuity

## Summary

Prove that typing and output survive unrelated Quick Chat renders in real browser sessions.
Use the current desktop and phone entry points, with transport counts and existing terminal lifecycle scenarios as controls.

## In scope

- Add a desktop continuity spec and a `mobile-` companion using the existing fixtures.
- Extract shared scenario code into a sibling helper used by both specs.
- Arm host-shell start, descriptor-write, and terminal-stream counters before launching.
- Publish five separate immutable `taskSessions.items` updates through the existing E2E store bridge.
- Split command entry across these updates and assert assembled output, focus, session identity, and original xterm-node continuity.
- Keep the selected terminal's output visible and require one initial start and stream for the uninterrupted mounted view.
- Cover phone entry with `.tap()`, retained shortcut controls, viewport containment, and a screenshot.
- Rerun existing desktop and phone Quick Terminal scenarios without replacing their action, detach, reload, or close coverage.

## Out of scope

- New production instrumentation or store bridges.
- Requiring a real agent, Safari automation, proxy keepalive, or changing stream retry behavior.
- Adding a separate terminal surface or changing touch controls.
- Reopening unrelated historical plan tasks or broad local verification.

## Acceptance

1. Desktop and phone tests retain the original xterm node, focused input, session identity, and complete command output through all five updates.
2. The uninterrupted view creates one host-shell start and stream, with no continued descriptor-write or connection growth from these updates.
3. Existing desktop/phone lifecycle and shortcut scenarios pass, all test-owned terminals close, and the final package records exact results.

## Scenario details

Use `testPage`, `apiClient`, and `seedData` from the existing fixtures.
The fixture already exposes the E2E store. Enable its existing init flag before navigation if required.
Do not treat a missing store bridge as a passing skipped stimulus.

Open the terminal through the visible desktop launcher or phone app menu.
Wait for backend shell readiness through output or current lifecycle state.
Capture the selected terminal's session ID and actual `.xterm` node.
The stable outer React container alone does not prove xterm continuity.

Type the first portion of a `printf` command.
Between each separate immutable store update, allow React to commit a browser frame.
Then finish the command and require a complete marker absent from the echoed command text.
Keep terminal-stream counters distinct from the application's main WebSocket.
Compare descriptor-write activity with the settled startup baseline, rather than requiring one write.
The existing startup path can report state twice.

Use causal waits for known transitions.
For the absence of repeat requests, use an armed counter and the documented negative-assertion `dwell` helper.
Capture a phone screenshot and inspect output, shortcuts, and keyboard clearance against UI-02.
Existing phone tests also cover simulated visual-viewport occlusion and modifier reset.

Use `closeQuickTerminalTab` in `finally` to remove every created descriptor and stop its PTY.
Restore any user setting changed for launcher setup.

## ASCII UI preview

Relevant [UI-01 and UI-02](plan.md#ascii-ui-preview), covering .21 and .22 with .8 and .13 controls:

```text
UI-01 desktop, after unrelated updates
[ Quick Chat                         Close ]
[ + ] [ Terminal 1* ]
[ existing output                          ]
[ $ complete command executes once         ]

UI-02 phone, after unrelated updates
[ Quick Chat                 Close ]
[ + | Terminal 1* | tabs ->         ]
[ existing output                  ]
[ $ complete command executes once ]
[ Ctrl Shift ^C ^D Esc Tab ... ->   ]
[ on-screen keyboard               ]
```

The tests require the retained xterm and focused input inside the existing surfaces.
Phone tabs and shortcuts keep independent horizontal overflow.
The full-height surface keeps terminal output as the vertical scroll owner.
Spacing and command text are illustrative.

## Verification

Run from the repository root after Task 01 passes.
In a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)` once.
Run desktop and phone commands sequentially. Each managed run builds fresh assets and the backend.

```bash
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium e2e/tests/terminal/quick-terminal-render-continuity.spec.ts e2e/tests/terminal/quick-terminal.spec.ts --list)
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=mobile-chrome e2e/tests/terminal/mobile-quick-terminal-render-continuity.spec.ts e2e/tests/terminal/mobile-quick-terminal.spec.ts --list)
(cd apps/web && pnpm e2e:run --host --project chromium e2e/tests/terminal/quick-terminal-render-continuity.spec.ts e2e/tests/terminal/quick-terminal.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome e2e/tests/terminal/mobile-quick-terminal-render-continuity.spec.ts e2e/tests/terminal/mobile-quick-terminal.spec.ts -- --retries=0)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings e2e/tests/terminal/quick-terminal-render-continuity.spec.ts e2e/tests/terminal/mobile-quick-terminal-render-continuity.spec.ts e2e/tests/terminal/quick-terminal-render-continuity-helpers.ts)
(cd apps/web && pnpm exec oxlint -c oxlint.e2e-sleeps.config.mjs --deny-warnings e2e/tests/terminal/quick-terminal-render-continuity.spec.ts e2e/tests/terminal/mobile-quick-terminal-render-continuity.spec.ts e2e/tests/terminal/quick-terminal-render-continuity-helpers.ts)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- apps/web/e2e/tests/terminal docs/specs/ui docs/plans/quick-terminal-render-continuity
git status --short -- apps/web/e2e/tests/terminal docs/specs/ui docs/plans/quick-terminal-render-continuity
```

Run the documentation-coverage block in [the manifest](plan.md#verification-results) from the repository root.
Confirm both discovery outputs contain the new regression and existing terminal tests.
Record actual test counts, screenshot paths, inspection results, and fixture teardown.

Task 01 owns deterministic behavioral RED for the original callback and stale-event defects.
These browser tests add complete input and transport evidence after that minimum correction.
If production code changes during this task, rerun Task 01's relevant checks before completion.

## Files likely touched

- `apps/web/e2e/tests/terminal/quick-terminal-render-continuity.spec.ts` (new)
- `apps/web/e2e/tests/terminal/mobile-quick-terminal-render-continuity.spec.ts` (new)
- `apps/web/e2e/tests/terminal/quick-terminal-render-continuity-helpers.ts` (new)
- This work order and `plan.md` for status and results.

Existing `terminal-test-helpers.ts`, desktop/mobile Quick Terminal specs, and causal-wait helpers are reusable inputs.
Their existing scenarios remain part of the final runs.

## Dependencies

Task `01-preserve-terminal-lifecycle` must pass first.

## Risks

- A batched store update can miss the repeated-render condition. Commit each update separately.
- An output assertion can match the echoed command. Assemble the expected marker with `printf` substitution.
- A stable outer terminal container can conceal xterm replacement. Retain the inner `.xterm` element handle.
- Negative-request checks need an armed observer and a bounded observation window.
- Desktop and mobile production builds must use the current checkout, rather than stale prebuilt assets.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/quick-terminal.md), especially .8, .13, .14, .21, and .22.
- [Shared-tab design](../../specs/ui/system-design/quick-terminal.md).
- [Render lifecycle design](../../specs/ui/system-design/quick-terminal-render-lifecycle.md).
- Existing `quick-terminal.spec.ts`, `mobile-quick-terminal.spec.ts`, and `terminal-test-helpers.ts`.
- `components/state-provider.tsx` for the existing E2E store bridge.
- `e2e/fixtures/test-base.ts`, `e2e/helpers/causal-waits.ts`, and terminal/cleanup guidance in `/e2e`.
- `apps/web/AGENTS.md`, `/mobile-parity`, and `/e2e` resource-safety guidance.

## Results

Desktop and phone test discovery each found all three targeted Quick Terminal tests: the new render-continuity regression and the two existing lifecycle/controls scenarios.

Both managed runs built the backend fixture and fresh pseudo-locale Vite assets before execution. Desktop Chromium passed 3/3 tests, including detach/reuse/switch/close and tablet hit-target controls. Mobile Chrome passed 3/3 tests, including shortcut interruption, keyboard focus, safe-height layout, touch targets, and terminal-scroll containment. All fixture-owned tabs and descriptors were closed during teardown.

The new continuity cases each retained the original `.xterm` node and focused input through five separate immutable store updates, assembled and executed the split command, preserved the selected session identity, and observed no render-driven start/stream growth. The phone case entered through the menu using touch and retained its shortcut row and viewport-contained surface.

Review follow-up protects desktop E2E sidebar settings: it reads the existing values before mutation, places settings mutation and navigation inside the protected scope, and restores the prior values in an inner `finally` around terminal cleanup. The restore therefore runs if setup fails after mutation or if cleanup throws. Static path inspection and focused lint passed; no browser rerun was needed for this cleanup-only change.

PR screenshot capture then ran both continuity projects together in the managed runner and passed 2/2 tests. It generated exactly one desktop and one phone entry in the fresh `.pr-assets/manifest.json`; both files were inspected, their prompt paths were sanitized to `/tmp`, and PNG compression completed before upload.

Phone screenshot captured and inspected: `apps/web/e2e/test-results/terminal-mobile-quick-term-714ab-s-through-unrelated-renders-mobile-chrome/quick-terminal-render-continuity.png`. It shows the assembled `KQT_RENDER_CONTINUITY` output and phone shortcut controls without an update-toast overlay.

Verification passed:

- Desktop discovery: 3 tests; fresh-build desktop managed run: 3/3 passed.
- Phone discovery: 3 tests; fresh-build phone managed run: 3/3 passed.
- Focused Oxlint and E2E sleep-lint for the new specs and shared helper.
- Web typecheck, catalog validation, all 36 specification-linter tests, and full specification lint.
- Documentation-coverage validation and `git diff --check`.

PR fixup after the animation-timing correction:

- Desktop render-continuity E2E: fresh managed build, 1/1 passed.
- Phone render-continuity E2E: fresh managed build, 1/1 passed.
