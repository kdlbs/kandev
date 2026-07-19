---
id: "04-workbench-default-integration"
title: "Workbench default integration"
status: done
wave: 2
depends_on: ["02-layout-profile-domain"]
plan: "plan.md"
spec: "../../specs/ui/task-layout-profiles.md"
---

# Task 04: Workbench Default Integration

## Acceptance

- A valid custom default supplies the fresh-task and Reset Layout state, while an invalid legacy default falls back to built-in Default.
- Existing environment layouts retain precedence over user defaults.
- Omitting Terminal from a fresh default prevents the default-shell migration path from creating a user shell.

## Files likely touched

- `apps/web/components/task/dockview-desktop-layout.tsx`
- `apps/web/components/task/layout-preset-selector.tsx`
- `apps/web/hooks/domains/session/use-ensure-default-terminal-ordinary.ts`
- `apps/web/hooks/domains/session/use-ensure-default-terminal-ordinary.test.ts`
- `apps/web/lib/layout/layout-profiles.test.ts`

## Dependencies

- Task 02 supplies effective-default validation and fallback behavior.

## Inputs

- Spec task-isolation, Reset Layout, terminal, and invalid-default scenarios.
- Existing `useSyncUserDefaultLayout`, `performBuildDefault`, `restoreEnvLayout`, and `useEnsureDefaultTerminalOrdinary` behavior.

## Verification

```bash
pnpm --filter @kandev/web test -- --run lib/layout/layout-profiles.test.ts hooks/domains/session/use-ensure-default-terminal-ordinary.test.ts
pnpm --filter @kandev/web typecheck
```

Run from `apps`.

## Output contract

Report precedence/fallback behavior, tests run, files changed, blockers, and risks; mark this task and its plan item done.
