---
id: "05-detachable-pty-lifecycle"
title: "Extract detachable PTY lifecycle"
status: done
wave: 5
depends_on: ["04-per-tab-host-shell-sessions"]
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-terminal.md"
---

# Task 05: Extract detachable PTY lifecycle

## Acceptance

- The shared xterm view can start with a stable host-shell client ID or attach to an existing session
  ID, report lifecycle changes, and detach without stopping the PTY when used by a Quick Chat tab.
- Standard Agents/login/auth dialogs preserve stop-on-unmount behavior, while explicit terminal-tab
  close owns stop; pending starts, StrictMode replay, exit, missing-session reattach, and stop `404`
  cannot leak or terminate a sibling session.
- Focused frontend tests cover start, attach, detach, reattach, explicit stop, late start after tab
  removal, and unchanged standard-dialog cleanup.

## Verification

From the repository worktree:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web exec vitest run components/settings/pty-terminal-view.test.tsx components/quick-chat/quick-terminal-tab-view.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web exec eslint components/settings/pty-terminal-view.tsx components/settings/pty-terminal-view.test.tsx components/settings/pty-terminal-dialog.tsx components/quick-chat/quick-terminal-tab-view.tsx components/quick-chat/quick-terminal-tab-view.test.tsx lib/api/domains/settings-api.ts)
```

## Files likely touched

- `apps/web/lib/api/domains/settings-api.ts`
- `apps/web/components/settings/pty-terminal-view.tsx` (new)
- `apps/web/components/settings/pty-terminal-view.test.tsx` (new)
- `apps/web/components/settings/pty-terminal-dialog.tsx`
- `apps/web/components/settings/host-shell-dialog.tsx` (only if wrapper props need forwarding)
- `apps/web/components/quick-chat/quick-terminal-tab-view.tsx` (new)
- `apps/web/components/quick-chat/quick-terminal-tab-view.test.tsx` (new)
- `docs/plans/quick-terminal/plan.md` (status/results only)
- `docs/plans/quick-terminal/task-05-detachable-pty-lifecycle.md` (status/results only)

## Dependencies

- Task 04 must be done so the frontend can rely on stable per-tab `client_id` idempotency.

## Parallelism

Sequential. Task 06 uses the extracted view and its callbacks to own terminal state in the shared
dialog.

## Inputs

- Spec: terminal portions of `What`, `API surface`, `State machine`, `Failure modes`, and
  `Persistence guarantees`.
- Plan: `Host-shell client and reusable PTY view` and PTY lifecycle tests.
- Existing patterns:
  `PtyTerminalDialog`, `HostShellDialog`, `agentLoginStreamUrl`,
  `resizeAgentLogin`, `stopAgentLogin`, and Task 03's mount-generation guard.

## Implementation notes

- Follow TDD with mocked `@xterm/xterm`, `WebSocket`, and `ResizeObserver`; keep lifecycle
  decisions in testable helpers rather than a single oversized effect.
- Default the extracted view to today's destructive unmount semantics. Quick-tab detach behavior
  must be explicit so Agents/login callers cannot silently begin leaking PTYs.
- Close only the active WebSocket/xterm on detach. Do not send stop unless the owner says the tab was
  explicitly removed or standard dialog cleanup applies.
- Reattachment must not resend `initialInput`; that input belongs only to the first successful
  connection.

## Risks

- React effect cancellation cannot equate to user intent after terminal tabs exist; tab switch,
  dialog close, StrictMode replay, and explicit close have different ownership semantics.
- Replayed buffered output may arrive before resize settles. Tests should allow the stream to attach
  before asserting fit/resize without introducing fixed sleeps.

## Output contract

Report the lifecycle API, files changed, exact command outcomes, cleanup/stop assertions, blockers,
residual risks, and synchronized task/plan status. Set this task to `in_progress` before changes and
replace `## Results` before marking it `done`.

## Results

- Added `HostShellStartOptions.clientId` serialization and status reattachment support.
- Extracted the xterm, resize, stream, mount-generation, and pending-start lifecycle into the
  reusable `PtyTerminalView`; standard dialogs retain stop-on-unmount and Quick Chat terminals
  explicitly detach.
- Added the Quick Chat terminal wrapper and lightweight pending-start cancellation contract.
- `cd apps && pnpm --filter @kandev/web exec vitest run components/settings/pty-terminal-view.test.tsx components/quick-chat/quick-terminal-tab-view.test.tsx lib/api/domains/settings-api.test.ts --reporter=verbose` — 3 files, 12 tests passed.
- `cd apps/web && pnpm run typecheck` — passed.
- Focused PTY/API ESLint — passed with no warnings.
