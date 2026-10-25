---
id: "07-shared-tabs-e2e"
title: "Prove shared tabs across viewports"
status: done
wave: 7
depends_on:
  - "04-per-tab-host-shell-sessions"
  - "05-detachable-pty-lifecycle"
  - "06-unified-quick-tabs"
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-terminal.md"
---

# Task 07: Prove shared tabs across viewports

## Acceptance

- Desktop E2E proves first-create versus last-terminal reuse, same-session reconnect after modal
  dismissal, grouped creation-menu behavior with tab-strip switching, conversation context-menu
  rename, two independent PTYs, chat/terminal launcher selection, single-tab stop, fallback, focus
  return, and tooltip absence.
- Host-shell integration E2E proves same-client idempotency, different-client concurrency,
  independent command streams and stops, plus omitted-client compatibility.
- Pixel 5 E2E proves the same user value through touch with contained full-height geometry,
  bottom-sheet menu behavior, 44 px controls/rows, internal scrolling, safe-area clearance, and no
  document horizontal overflow; affected existing Quick Chat flows remain green.

## Verification

From the repository worktree:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run tests/terminal/quick-terminal.spec.ts tests/settings/host-shell-pty.spec.ts tests/chat/quick-chat.spec.ts tests/chat/entity-reference-composer.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/terminal/mobile-quick-terminal.spec.ts tests/chat/mobile-quick-chat-entry.spec.ts)
```

Confirm both managed-runner invocations discover the intended test counts, rebuild the backend and
production Vite assets, and tear down every worker-scoped backend and host-shell process.

## Files likely touched

- `apps/web/e2e/tests/terminal/quick-terminal.spec.ts`
- `apps/web/e2e/tests/terminal/mobile-quick-terminal.spec.ts`
- `apps/web/e2e/tests/settings/host-shell-pty.spec.ts`
- `apps/web/e2e/tests/chat/quick-chat.spec.ts` (only for selectors/assertions affected by the menu)
- `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts` (only for affected touch entry/menu flow)
- `apps/web/e2e/tests/terminal/quick-terminal-helpers.ts` (new only if shared PTY polling/selection
  would otherwise push a spec over limits)
- `docs/plans/quick-terminal/plan.md` (status/results only)
- `docs/plans/quick-terminal/task-07-shared-tabs-e2e.md` (status/results only)

## Dependencies

- Tasks 04–06 must be done so the API, PTY lifecycle, state, launchers, and shared dialog are
  available through the production build.

## Parallelism

Sequential. The managed runner owns the same build artifacts and the scenarios share host-shell and
dialog lifecycle assumptions.

## Inputs

- Every `Scenarios` item in `docs/specs/ui/requirements/quick-terminal.md`.
- Plan: `E2E Tests`, `Mobile design contract`, and all lifecycle/geometry risks.
- Existing patterns:
  `quick-terminal.spec.ts`, `mobile-quick-terminal.spec.ts`,
  `host-shell-pty.spec.ts`, `mobile-quick-chat-entry.spec.ts`, animation-aware geometry guidance
  in `.agents/skills/e2e/SKILL.md`, and mobile checks in `.agents/skills/mobile-parity/SKILL.md`.

## Implementation notes

- Follow E2E TDD: update assertions for the old standalone dialog first and observe them fail before
  implementing or completing the corresponding shared behavior.
- Drive creation, switching, close, and launcher reuse through visible UI. Use the low-level API only
  in `host-shell-pty.spec.ts`.
- Scope all xterm/buffer selectors to the active terminal tab; hidden or detached tabs may coexist.
- Use unique command markers per terminal and assert each marker is absent from its sibling.
- Use `.tap()` in the mobile spec, wait for finite overlay animations before geometry checks, and
  assert both element containment and document `scrollWidth`.
- Do not use fixed sleeps for shell readiness; poll the scoped test buffer or observable state.

## Risks

- A test can falsely pass by reading the first globally mounted xterm. Every terminal assertion must
  be scoped to the active tab/container.
- Closing the modal is no longer session teardown. Teardown evidence must explicitly close terminal
  tabs or stop sessions so the managed runner does not rely only on process exit.

## Output contract

Report discovered test counts, exact commands/outcomes, rendered desktop/phone geometry, failure
artifact paths if any, explicit session/process teardown evidence, blockers, residual risks, and
synchronized task/plan status. Set this task to `in_progress` before test edits and replace
`## Results` before marking it `done`.

## Results

- Updated desktop shared Quick Chat scenarios to cover first-terminal creation, dismissal and
  same-session reattach, grouped creation-menu behavior with tab-strip switching, independent
  PTYs, launcher selection, single-terminal close/fallback, focus return, and the absence of the old
  standalone footer; the cross-device chat scenario also covers context-menu rename.
- Added low-level host-shell coverage for legacy starts, same-client idempotency, distinct-client
  concurrency and streams, independent stop, malformed UUID rejection, and omitted-client
  compatibility.
- Added Pixel 5 coverage for the shared full-height dialog, safe-area classes, 44px launch/menu
  targets, bottom-sheet geometry, internal terminal scrolling, close/reopen behavior, and zero
  document horizontal overflow; existing mobile Quick Chat entry scenarios remain green.
- `cd apps/web && pnpm e2e:run tests/terminal/quick-terminal.spec.ts tests/settings/host-shell-pty.spec.ts tests/chat/quick-chat.spec.ts tests/chat/entity-reference-composer.spec.ts` — managed Docker runner rebuilt the backend and Vite assets; 20 tests passed.
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/terminal/mobile-quick-terminal.spec.ts tests/chat/mobile-quick-chat-entry.spec.ts` — managed Docker runner rebuilt the backend and Vite assets; 5 tests passed.
- Both managed runners completed their worker teardown, including the temporary remote fixture
  repositories and worker-scoped backend/host-shell processes; no failure artifacts remained.
