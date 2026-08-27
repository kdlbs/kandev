---
id: "04-system-account-migration"
title: "System account migration"
status: in-progress
wave: 2
depends_on: ["01-typography-primitives-and-shells"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-typography.md"
---

# Task 04: System account migration

## Acceptance

- Users, Backups, Licenses, Storage Policy, Account token/security, and system
  dialogs use shared card, label, helper, error, and responsive action roles.
- Account/security tables use a deliberate dense-table role: readable primary
  columns and errors remain legible while IPs, timestamps, user agents, tokens,
  and diagnostics remain compact where appropriate.
- System/account card headers and license filters stack correctly on phone and
  narrow-tablet widths without changing existing behavior or overflow.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test -- --run components/settings/settings-field.test.tsx components/settings/settings-card-header.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
```

## Files likely touched

- `apps/web/components/settings/system/users-table.tsx`
- `apps/web/components/settings/system/backups-table.tsx`
- `apps/web/components/settings/system/licenses-list.tsx`
- `apps/web/components/settings/system/storage/storage-policy-fields.tsx`
- `apps/web/components/settings/system/storage/storage-policy-card.tsx`
- `apps/web/components/settings/account/api-tokens.tsx`
- `apps/web/components/settings/account/security-settings.tsx`
- `apps/web/components/settings/system/create-user-dialog.tsx`
- `apps/web/components/settings/system/invite-dialog.tsx`
- `apps/web/components/settings/system-settings-typography.test.tsx`
- `apps/web/components/settings/account-settings-typography.test.tsx`

## Dependencies

Task 01.

## Parallelism

Parallel-safe with Tasks 02, 03, and 05 after Task 01; owned files are
disjoint.

## Inputs

- Spec: `docs/specs/ui/requirements/settings-typography.md`, field, error, technical-value,
  and mobile scenarios.
- Plan: `plan.md`, System and account surfaces section.
- Audit: `docs/audits/settings-typography/003-normalize-card-and-section-headings.md`,
  `005-normalize-settings-helper-text.md`, and
  `010-align-mobile-settings-type-scale.md`.

## Output contract

Report changed files, dense-table/error decisions, exact focused test and
static-check results, and any intentional compact exceptions. Update this task
and the parent plan only after all listed checks pass.

## Results

In progress. Users, Backups, Licenses, Storage Policy, API tokens, security,
and account dialogs now use shared card headers, labels, errors, helper copy,
mobile control sizing, and stacked action rows. Web typecheck, lint, i18n, and
the focused settings test set pass. Dense table roles, dedicated system/account
component coverage, and representative system/account E2E checks remain to be
added.
