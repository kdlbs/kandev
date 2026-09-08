---
id: "02-implement-adaptive-terminal-themes"
title: "Implement adaptive terminal themes"
status: done
wave: 2
depends_on:
  - "01-add-terminal-theme-regressions"
plan: "plan.md"
requirements:
  - REQ-UI-TERMINAL-RENDERING-001
acceptance_criteria:
  - AC-UI-TERMINAL-RENDERING-001.1
  - AC-UI-TERMINAL-RENDERING-001.2
  - AC-UI-TERMINAL-RENDERING-001.3
  - AC-UI-TERMINAL-RENDERING-001.4
  - AC-UI-TERMINAL-RENDERING-001.5
  - AC-UI-TERMINAL-RENDERING-001.6
system_design:
  - ../../specs/ui/system-design/terminal-rendering.md
---

# Task 02: Implement Adaptive Terminal Themes

## Summary

Implement the shared light, dark, and fixed-dark theme contract. Update open adaptive terminals without replacing their xterm instance or connection.

## In scope

- Add separate light and dark ANSI palettes.
- Add the shared xterm minimum contrast value.
- Apply the resolved theme during every xterm construction path.
- Synchronize open task and agent terminals after a resolved theme change.
- Use the shared fixed-dark theme in `PtyTerminalView`.
- Turn every Task 01 regression green.

## Out of scope

- Change PTY or WebSocket ownership.
- Clear or recreate a terminal during a theme change.
- Add a theme preference or configurable palette.
- Change desktop, tablet, or phone terminal composition.

## Acceptance

- Every xterm constructor uses the shared palette and minimum contrast contract.
- An open adaptive terminal updates all theme colors without losing buffer content, focus, scroll position, a running command, or new shell output.
- Desktop, mobile, and fixed-dark terminal checks pass without changing their lifecycle behavior.

## Verification

```bash
cd apps/web && pnpm test -- lib/theme/terminal-theme.test.ts
```

```bash
cd apps/web && pnpm test -- components/task/use-terminal-theme.test.ts components/theme/app-theme.test.tsx
```

```bash
cd apps/web && pnpm run typecheck
```

```bash
cd apps/web && pnpm exec eslint lib/theme/terminal-theme.ts lib/theme/terminal-theme.test.ts components/task/use-terminal-theme.ts components/task/use-passthrough-terminal.ts components/task/passthrough-terminal.tsx components/task/shell-terminal.tsx components/task/terminal-buffer-reader.ts components/settings/pty-terminal-view.tsx e2e/tests/terminal/terminal-test-helpers.ts e2e/tests/terminal/terminal-theme.spec.ts e2e/tests/terminal/mobile-terminal-theme.spec.ts
```

```bash
cd apps/web && pnpm e2e:run tests/terminal/terminal-theme.spec.ts
```

```bash
cd apps/web && pnpm e2e:run --project mobile-chrome tests/terminal/mobile-terminal-theme.spec.ts
```

## Files likely touched

- `apps/web/lib/theme/terminal-theme.ts`
- `apps/web/components/task/use-terminal-theme.ts`
- `apps/web/components/task/use-terminal-theme.test.ts`
- `apps/web/components/theme/app-theme.test.tsx`
- `apps/web/components/task/use-passthrough-terminal.ts`
- `apps/web/components/task/passthrough-terminal.tsx`
- `apps/web/components/task/shell-terminal.tsx`
- `apps/web/components/settings/pty-terminal-view.tsx`
- Files from Task 01 when selectors or helper types need final adjustment.

## Dependencies

- Task 01 must record the expected red tests.

## Risks

- Reading computed CSS colors too early can apply the previous theme.
- Recreating xterm can interrupt active commands and erase the scrollback buffer.
- A palette correction can reduce the distinction between ANSI hues after contrast adjustment.

## Parallelism

`sequential`

## Inputs

- Task 01 failure evidence.
- `REQ-UI-TERMINAL-RENDERING-001`
- `docs/specs/ui/system-design/terminal-rendering.md`
- All three current xterm constructor paths.

## Results

Implemented the shared light and dark palettes, xterm minimum contrast
setting, adaptive live-theme synchronization, and fixed-dark PTY theme. The
root theme now applies before descendant terminal construction effects, and
the light bright variants remain visually distinct. The existing terminal
instance, buffer, WebSocket, focus, scroll position, running command, and
mobile composition remain unchanged. Unit, typecheck, targeted lint, desktop
production E2E, and Pixel 5 production E2E verification all pass.
