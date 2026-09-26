---
id: "05-technical-surfaces-migration"
title: "Technical surfaces migration"
status: in-progress
wave: 2
depends_on: ["01-typography-primitives-and-shells"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-typography.md"
---

# Task 05: Technical surfaces migration

## Acceptance

- SSH cards, plugin metadata, badges, diagnostic actions, and technical values
  use named compact/mono roles without shrinking ordinary labels or helpers.
- PTY and Monaco/script editor surfaces use existing terminal/editor tokens or
  a documented fallback, while editor previews/forms stack actions on mobile.
- SSH technical details remain compact/mono, but SSH card titles, descriptions,
  and actions use the shared settings hierarchy and responsive card header.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test -- --run components/settings/settings-card-header.test.tsx components/settings/notification-events-table.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
```

## Files likely touched

- `apps/web/components/settings/ssh-connection-card.tsx`
- `apps/web/components/settings/ssh-agent-readiness-card.tsx`
- `apps/web/components/settings/ssh-sessions-card.tsx`
- `apps/web/components/settings/profile-status-panels.tsx`
- `apps/web/components/settings/record-badges.tsx`
- `apps/web/components/settings/plugins/plugin-row.tsx`
- `apps/web/components/settings/plugins/plugin-manifest-card.tsx`
- `apps/web/components/settings/pty-terminal-view.tsx`
- `apps/web/components/settings/profile-edit/script-editor.tsx`
- `apps/web/components/settings/editors-settings.tsx`
- `apps/web/components/settings/editor-form.tsx`
- `apps/web/components/settings/technical-settings-typography.test.tsx`

## Dependencies

Task 01.

## Parallelism

Parallel-safe with Tasks 02, 03, and 04 after Task 01; owned files are
disjoint.

## Inputs

- Spec: `docs/specs/ui/requirements/settings-typography.md`, technical-value and mobile
  scenarios.
- Plan: `plan.md`, Technical surfaces section.
- Audit: `docs/audits/settings-typography/003-normalize-card-and-section-headings.md`,
  `008-document-settings-micro-type-exceptions.md`,
  `009-centralize-settings-font-family-roles.md`, and
  `010-align-mobile-settings-type-scale.md`.

## Output contract

Report changed files, technical-role/token decisions, exact focused test and
static-check results, and any intentional compact exceptions. Update this task
and the parent plan only after all listed checks pass.

## Results

In progress. SSH cards/actions, plugin hierarchy and metadata badges, profile
diagnostic copy, editor previews/forms, PTY, and Monaco now use the shared
responsive or named technical roles. Web typecheck, lint, i18n, focused tests,
and the existing desktop/mobile notification E2E contracts pass. Remaining
technical badge callsites and dedicated technical-surface/SSH tests are still
open.
