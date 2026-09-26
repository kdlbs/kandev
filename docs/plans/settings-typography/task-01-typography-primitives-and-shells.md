---
id: "01-typography-primitives-and-shells"
title: "Typography primitives and shells"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-typography.md"
---

# Task 01: Typography primitives and shells

## Acceptance

- Shared settings primitives define the page, section, card, description,
  helper, error, metadata, badge, and technical-value roles without changing
  global `@kandev/ui` consumers.
- Settings page shells use one page-level header, and card headers expose
  semantic heading elements with stable title/description/action composition.
- Utility Agents, legacy global Integrations, legacy executor profile chrome,
  and Terminal Editors no longer introduce an incorrect or duplicate page
  heading.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test -- --run components/settings/settings-typography.test.ts components/settings/settings-card-header.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
```

## Files likely touched

- `apps/web/components/settings/settings-typography.tsx`
- `apps/web/components/settings/settings-card-header.tsx`
- `apps/web/components/settings/settings-page-template.tsx`
- `apps/web/components/settings/settings-section.tsx`
- `apps/web/components/settings/system/system-page-shell.tsx`
- `apps/web/components/settings/profile-edit/profile-edit-page-chrome.tsx`
- `apps/web/components/settings/workspaces/workspace-section-header.tsx`
- `apps/web/components/settings/utility-agents-section.tsx`
- `apps/web/components/integrations/integrations-index-page.tsx`
- `apps/web/components/settings/terminal-editors-settings.tsx`
- `apps/web/components/settings/editors-settings.tsx`
- `apps/web/components/settings/settings-typography.test.ts`
- `apps/web/components/settings/settings-card-header.test.tsx`

## Dependencies

None.

## Parallelism

Sequential. All later migration tasks depend on these shared role contracts and
shell markup.

## Inputs

- Spec: `docs/specs/ui/requirements/settings-typography.md`, especially What items 1–4 and
  the scenarios for page/card hierarchy and Terminal Editors.
- Plan: `plan.md`, Shared role primitives and Mobile design contract sections.
- Audit: `docs/audits/settings-typography/001-define-settings-typography-contract.md`,
  `002-consolidate-settings-page-shells.md`, and
  `003-normalize-card-and-section-headings.md`.

## Output contract

Report changed files, semantic heading decisions, exact test/typecheck/i18n
commands and results, and any route-level exceptions. Update this task and the
parent plan only after all listed checks pass.

## Results

Implemented. Added the settings role map, semantic responsive card header,
page header, mobile control/action helpers, and migrated the shared page,
section, profile, Utility Agents, Integrations, Terminal Editors, and legacy
executor profile shells. Focused tests, typecheck, lint, and i18n checks pass.
Remaining broad field and card callsites are tracked by Tasks 02–06.
