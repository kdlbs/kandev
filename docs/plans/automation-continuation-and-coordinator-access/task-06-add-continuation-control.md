---
id: "06-add-continuation-control"
title: "Add continuation control"
status: completed
wave: 2
depends_on: ["01-persist-continuation-policy"]
plan: "plan.md"
spec: "../../specs/office/requirements/automation-continuity.md"
---

# Task 06: Add Continuation Control

## Acceptance

- Create and edit forms always show Context between runs and round-trip both choices; new
  automations visibly default to a new task per run.
- Reuse mode locks concurrency at 1 and explains continued conversation, environment, and worktree;
  no MCP capability control is rendered.
- The section, both choices, and the concurrency lock use the exact visible descriptions from the
  spec. Each description has an accessible relationship and does not require hover or focus.
- Desktop and mobile use the same localized stacked radio group with 44 px targets, one scroll
  owner, and no horizontal overflow.

## TDD scenarios

1. RED: Add payload tests for default, both policies, create/edit hydration, and switching.
2. RED: Add component tests for exact visible descriptions, `aria-describedby` relationships,
   visible-on-create choices, and concurrency locking.
3. GREEN: Extend TypeScript contracts, form state, payload builders, and Settings card controls.
4. GREEN: Add English, Portuguese, Simplified Chinese, generated Traditional Chinese, and pseudo
   locale keys.
5. REFACTOR: Keep presentation within the existing Settings card and shared responsive logic.

## Verification

- `cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run components/automations/automation-payload.test.ts components/automations/config-section.test.tsx`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run i18n:zh-hant`
- `cd apps/web && pnpm run i18n:check`
- `cd apps/web && pnpm run i18n:ratchet`
- `git diff --check`

## Files likely touched

- `apps/web/lib/types/automation.ts`
- `apps/web/components/automations/automation-payload.ts`
- `apps/web/components/automations/automation-payload.test.ts`
- `apps/web/components/automations/automation-editor.tsx`
- `apps/web/components/automations/automation-editor-sections.tsx`
- `apps/web/components/automations/config-section.test.tsx`
- `apps/web/src/locales/en/automations.json`
- `apps/web/src/locales/pt-pt/automations.json`
- `apps/web/src/locales/zh-cn/automations.json`
- `apps/web/src/locales/zh-hk/automations.json`
- `apps/web/src/locales/zh-tw/automations.json`
- `apps/web/src/locales/pseudo/automations.json`

## Dependencies

Task 01 defines backend policy values, defaults, and validation.

## Inputs

- Continuation UI rules in the automation settings spec.
- Existing AutomationEditor Settings card and payload builder.

## Parallelism

Parallel-safe with Tasks 02 and 04 after Task 01: this task owns editor/payload/locale files only.

## Output contract

Report payload snapshots, exact rendered descriptions, accessible relationships, create/edit
behavior, mobile/desktop geometry, locales, files changed, and exact tests.

## Risks

- UI-only concurrency handling can drift from backend validation or hide the create-time choice.

## Results

Implemented localized create/edit controls, visible accessible descriptions, reuse concurrency locking, and continuation payloads. Focused frontend verification passed with 307 tests and typecheck passed.
