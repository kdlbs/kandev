---
id: "02-form-controls-and-profile-migration"
title: "Form controls and profile migration"
status: in-progress
wave: 2
depends_on: ["01-typography-primitives-and-shells"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-typography.md"
---

# Task 02: Form controls and profile migration

## Acceptance

- Profile, agent, executor, MCP, environment-variable, model, mode, and CLI
  flag fields use shared label, helper, error, and control roles.
- Editable controls preserve the coarse-pointer 16px anti-zoom rule, while
  selectors and actions use named mobile/desktop variants with 44px mobile
  hitboxes and no 640–767px squeezing.
- Long profile names, installation paths, labels, and descriptions wrap or
  truncate within their headers and cards without document-level overflow.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test -- --run components/settings/settings-field.test.tsx components/settings/settings-typography.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
```

## Files likely touched

- `apps/web/components/settings/settings-field.tsx`
- `apps/web/components/settings/settings-control.tsx`
- `apps/web/components/settings/profile-form-fields.tsx`
- `apps/web/components/settings/profile-model-fields.tsx`
- `apps/web/components/settings/model-combobox.tsx`
- `apps/web/components/settings/mode-combobox.tsx`
- `apps/web/components/model-config-selector.tsx`
- `apps/web/components/settings/profile-edit/inline-secret-select.tsx`
- `apps/web/components/settings/profile-edit/env-vars-card.tsx`
- `apps/web/components/settings/profile-edit/mcp-policy-card.tsx`
- `apps/web/components/settings/profile-edit/profile-details-card.tsx`
- `apps/web/components/settings/profile-edit/git-identity-fields.tsx`
- `apps/web/components/settings/profile-edit/profile-edit-page-chrome.tsx`
- `apps/web/components/settings/agent-profile-page.tsx`
- `apps/web/components/settings/cli-flags-field.tsx`
- `apps/web/app/settings/agents/[agentId]/agent-setup-parts.tsx`
- `apps/web/app/settings/executors/new/[type]/page.tsx`
- `apps/web/app/settings/executors/new/[type]/ssh-create-page.tsx`
- `apps/web/app/settings/executors/ssh/[executorId]/page.tsx`
- `apps/web/app/settings/executor/[id]/page.tsx`
- `apps/web/app/settings/executor/[id]/profile/[profileId]/page.tsx`
- `apps/web/components/settings/settings-field.test.tsx`

## Dependencies

Task 01.

## Parallelism

Parallel-safe with Tasks 03 and 04 after Task 01; owned files are disjoint.

## Inputs

- Spec: `docs/specs/ui/requirements/settings-typography.md`, What items 5–8 and the
  mobile/long-string scenarios.
- Plan: `plan.md`, Forms and controls section.
- Audit: `docs/audits/settings-typography/004-normalize-settings-field-labels.md`,
  `005-normalize-settings-helper-text.md`,
  `006-normalize-settings-control-typography.md`, and
  `007-remove-agent-profile-compact-drift.md`.

## Output contract

Report changed files, responsive control decisions, exact focused test and
static-check results, and any intentionally compact profile roles. Update this
task and the parent plan only after all listed checks pass.

## Results

In progress. Shared field label/error helpers and responsive control/action
variants are implemented. Model/mode selectors, model configuration, env vars,
MCP policy, profile details, CLI flags, agent setup, and executor profile
surfaces now consume the contract. The shared field, role, card-header,
integration, and notification focused tests pass (20 tests across 5 files), and
web typecheck/lint/i18n checks pass. Remaining profile-form and legacy field
callsites need migration and dedicated component coverage.
