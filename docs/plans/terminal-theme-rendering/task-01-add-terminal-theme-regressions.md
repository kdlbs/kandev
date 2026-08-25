---
id: "01-add-terminal-theme-regressions"
title: "Add terminal theme regressions"
status: done
wave: 1
depends_on: []
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

# Task 01: Add Terminal Theme Regressions

## Summary

Add unit and browser regressions before the correction. Record the expected failures for the light ANSI palette and stale live theme.

## In scope

- Add unit coverage for light, dark, and fixed-dark terminal themes.
- Extend the read-only xterm browser bridge with theme and minimum-contrast snapshots.
- Add a desktop test that changes theme while one terminal stays open.
- Add a Pixel 5 test for the existing mobile terminal in light mode.
- Document the shared `PassthroughTerminal` rendering path used by tablet
  terminals instead of duplicating the browser regression for a third viewport.
- Add `@covers` comments where the acceptance mapping is not clear from the test name.

## Out of scope

- Change palette values or terminal constructors.
- Add live theme synchronization.
- Change terminal transport, layout, or mobile interaction.

## Acceptance

- The unit test fails because the current light theme uses the dark ANSI palette.
- The desktop test fails because the open xterm instance keeps stale theme values.
- The mobile test fails because the live light terminal does not meet the shared contrast contract.

## Verification

These commands must fail for the documented reasons before Task 02 starts.

```bash
cd apps/web && pnpm test -- lib/theme/terminal-theme.test.ts
```

```bash
cd apps/web && pnpm e2e:run tests/terminal/terminal-theme.spec.ts
```

```bash
cd apps/web && pnpm e2e:run --project mobile-chrome tests/terminal/mobile-terminal-theme.spec.ts
```

## Files likely touched

- `apps/web/lib/theme/terminal-theme.test.ts`
- `apps/web/components/task/terminal-buffer-reader.ts`
- `apps/web/e2e/tests/terminal/terminal-test-helpers.ts`
- `apps/web/e2e/tests/terminal/terminal-theme.spec.ts`
- `apps/web/e2e/tests/terminal/mobile-terminal-theme.spec.ts`

## Dependencies

None.

## Risks

- The test must read the active terminal panel because Dockview can mount hidden siblings.
- The test bridge can create false evidence if cleanup leaves a stale snapshot function.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-TERMINAL-RENDERING-001`
- `docs/specs/ui/system-design/terminal-rendering.md`
- Existing terminal keyboard, settings, and mobile helper patterns.

## Results

The unit, desktop, and Pixel 5 regressions were added and run before the
implementation. Each failed for the expected missing palette or live-theme
snapshot behavior, with no fixture or compilation failure. The read-only
xterm bridge now removes the theme snapshot during cleanup.
