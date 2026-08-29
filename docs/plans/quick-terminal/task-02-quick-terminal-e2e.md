---
id: "02-quick-terminal-e2e"
title: "Prove Quick Terminal across viewports"
status: done
wave: 2
depends_on: ["01-quick-terminal-ui"]
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-terminal.md"
---

# Task 02: Prove Quick Terminal across viewports

## Acceptance

- Desktop and tablet UI tests prove launcher order, usable hit targets, larger floating-dialog
  geometry, viewport containment, PTY mount, dismissal, and focus return.
- Pixel 5 coverage proves touch access to a full-height terminal with safe contained geometry, no
  document horizontal overflow, dismissal, and focus return.
- The existing host-shell integration scenarios still prove command I/O, idempotent start, and clean
  stop after the shared dialog change.

## Verification

From the repository worktree:

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/terminal/quick-terminal.spec.ts tests/settings/host-shell-pty.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/terminal/mobile-quick-terminal.spec.ts
```

Confirm each managed-runner invocation discovers the intended tests, rebuilds the production Vite
assets, and tears down its worker-scoped backend/session state.

## Files likely touched

- `apps/web/e2e/tests/terminal/quick-terminal.spec.ts`
- `apps/web/e2e/tests/terminal/mobile-quick-terminal.spec.ts`
- `apps/web/e2e/tests/settings/host-shell-pty.spec.ts` (only if selector/lifecycle coverage needs a
  focused update)
- `docs/plans/quick-terminal/plan.md` (status/results only)
- `docs/plans/quick-terminal/task-02-quick-terminal-e2e.md` (status/results only)

## Dependencies

- Task 01 must be done so the provider, launchers, responsive presentation, and stable test IDs exist.

## Parallelism

Sequential. These scenarios share the Quick Terminal surface and host-shell lifecycle, and the
managed E2E runner owns the same build/test artifact pipeline.

## Inputs

- Spec: every `Scenarios` item in `docs/specs/ui/requirements/quick-terminal.md`.
- Plan: `E2E Tests`, `Mobile design contract`, and `Risks` in `plan.md`.
- Existing patterns: `apps/web/e2e/tests/settings/host-shell-pty.spec.ts`,
  `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts`, and the animation-aware geometry guidance
  in `.agents/skills/e2e/SKILL.md`.

## Implementation notes

- Follow E2E TDD: add each focused Playwright scenario and observe its launcher/geometry assertion
  fail before completing the corresponding production behavior.
- Exercise the feature through visible UI controls. API calls may seed workspace preconditions but
  must not replace launcher, dialog, dismissal, or focus assertions.
- Wait for finite dialog animations before measuring. Use bounding boxes and `elementFromPoint()`
  for containment/hit-target proof rather than fixed sleeps.
- Use `.tap()` in the mobile spec and let the `mobile-chrome` project supply its Pixel 5 device.
- Scope terminal locators to the Quick Terminal dialog because other terminal/xterm instances may be
  mounted elsewhere in the app.

## Output contract

Report discovered test counts, exact commands and outcomes, rendered desktop/tablet/phone geometry,
artifact paths on failure, teardown evidence, blockers, residual risks, and the task/plan status
update in this conversation. Set this task to `in_progress` before test edits, then replace
`## Results` and mark it `done` only after every acceptance condition and targeted command passes.

## Results

- `cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium tests/terminal/quick-terminal.spec.ts` — passed; 2 desktop/tablet tests.
- `cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=mobile-chrome tests/terminal/mobile-quick-terminal.spec.ts` — passed; 1 Pixel 5 test.
- `cd apps/web && pnpm e2e:run tests/terminal/quick-terminal.spec.ts tests/settings/host-shell-pty.spec.ts` — passed; managed Docker runner rebuilt backend/Vite/plugin assets, ran 4 tests (2 host-shell lifecycle + 2 desktop/tablet UI), and tore down the worker backend.
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/terminal/mobile-quick-terminal.spec.ts` — passed; managed Docker runner rebuilt assets, ran 1 Pixel 5 test, and tore down the worker backend.
- Rendered checks covered the 1280px desktop floating surface, 700px tablet floating surface, and Pixel 5 full-height surface; all dialog/terminal bounds remained within the viewport, with zero mobile document horizontal overflow.
- No failure artifacts or residual host-shell sessions remained after the managed runs.
