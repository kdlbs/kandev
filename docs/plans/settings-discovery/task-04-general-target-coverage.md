---
id: "04-general-target-coverage"
title: "General and standalone target coverage"
status: completed
wave: 4
depends_on: ["02-tree-and-target-navigation"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-discovery.md"
---

# Task 04: General and standalone target coverage

## Acceptance

- General, terminal, appearance, layouts, notifications, editors, shortcuts, task actions, prompts,
  voice, utilities, secrets, sprites, and external MCP expose stable inline targets.
- Catalog labels and aliases resolve at render time; pseudo locale remains complete.
- Dialog-only, destructive, generated, and value-bearing fields remain explicitly excluded.

## Verification

`cd apps && pnpm --filter @kandev/web test -- --run lib/settings-discovery components/settings`

## Files likely touched

- General and standalone files named in `plan.md`
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/pseudo/settings.json`

## Dependencies

Task 02.

## Parallelism

Sequential because catalog files are shared.

## Results

- `cd apps && pnpm --filter @kandev/web test -- --run lib/settings-discovery components/settings`
  — passed.
- `cd apps/web && pnpm run typecheck` — passed after all Task 04 targets were wired.
