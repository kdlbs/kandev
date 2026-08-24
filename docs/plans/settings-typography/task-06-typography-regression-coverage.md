---
id: "06-typography-regression-coverage"
title: "Typography regression coverage"
status: in-progress
wave: 3
depends_on: ["02-form-controls-and-profile-migration", "03-workspace-provider-migration", "04-system-account-migration", "05-technical-surfaces-migration"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-typography.md"
---

# Task 06: Typography regression coverage

## Acceptance

- Desktop and mobile tests assert semantic settings roles across representative
  preference, agent/executor, workspace/provider, system/account, SSH, and
  Terminal Editors routes.
- Notifications tests cover event titles, group headings, descriptions,
  provider rows, and table rows using the shared role contract.
- Mobile tests cover Pixel 5 and 640–767px composition, 44px navigation/action
  hitboxes, long translated content, and absence of document-level horizontal
  overflow. The audit and implementation plan statuses reflect the completed
  migration.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --no-build --project chromium e2e/tests/settings/settings-typography.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/settings/mobile-settings-typography.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project chromium e2e/tests/settings/notifications-type-scale.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/settings/mobile-notifications-type-scale.spec.ts)
```

## Files likely touched

- `apps/web/e2e/tests/settings/settings-typography.spec.ts`
- `apps/web/e2e/tests/settings/mobile-settings-typography.spec.ts`
- `apps/web/e2e/tests/settings/notifications-type-scale.spec.ts`
- `apps/web/e2e/tests/settings/mobile-notifications-type-scale.spec.ts`
- `apps/web/components/settings/settings-typography.test.ts`
- `apps/web/components/settings/settings-field.test.tsx`
- `apps/web/components/settings/settings-card-header.test.tsx`
- `docs/audits/settings-typography/README.md`
- `docs/audits/settings-typography/001-define-settings-typography-contract.md`
- `docs/audits/settings-typography/012-expand-settings-typography-verification.md`
- `docs/audits/settings-typography/013-normalize-settings-copy-punctuation.md`

## Dependencies

Tasks 02, 03, 04, and 05.

## Parallelism

Sequential. Coverage must follow the final shared markup, roles, and responsive
variants.

## Inputs

- Spec: `docs/specs/ui/requirements/settings-typography.md`, all Scenarios.
- Plan: `plan.md`, Tests, E2E Tests, and Mobile design contract sections.
- Audit: `docs/audits/settings-typography/012-expand-settings-typography-verification.md`
  and `010-align-mobile-settings-type-scale.md`.

## Output contract

Report every exact command and result, test counts, viewport coverage, changed
fixtures, and any remaining risk. Mark this task and all linked plan checklist
items complete only after the listed checks pass.

## Results

In progress. Added pure role-contract, field-composition, and semantic
card-header rendering coverage. The desktop settings typography E2E passes for
Appearance and Terminal Editors (2 tests), and the mobile suite passes for the
Pixel 5 and 700px narrow-tablet checks (2 tests). Existing desktop and mobile
notification type-scale E2E tests also pass. Dedicated representative
system/account/SSH E2E coverage and broader long-content route checks remain
before this task can be completed.
