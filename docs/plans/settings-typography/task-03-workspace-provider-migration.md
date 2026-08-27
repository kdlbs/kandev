---
id: "03-workspace-provider-migration"
title: "Workspace provider migration"
status: in-progress
wave: 2
depends_on: ["01-typography-primitives-and-shells"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-typography.md"
---

# Task 03: Workspace provider migration

## Acceptance

- Workspace, repository, integration, and provider forms use the shared label,
  description, helper, error, control, and card roles.
- Equivalent GitHub, GitLab, Jira, Linear, Azure DevOps, and Sentry credential
  fields use one documented font family and size treatment.
- Automation helpers, workflow fields, repository dialogs, and provider forms
  remain readable on phone and narrow-tablet widths without horizontal
  overflow.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test -- --run components/settings/settings-field.test.tsx app/settings/integrations/page.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
```

## Files likely touched

- `apps/web/components/settings/workspace-section-header.tsx`
- `apps/web/components/settings/repository-card.tsx`
- `apps/web/app/settings/workspace/workspace-repositories-dialog.tsx`
- `apps/web/app/settings/workspace/workspace-workflows-dialogs.tsx`
- `apps/web/components/github/github-pat-form.tsx`
- `apps/web/components/github/github-repo-scope-section.tsx`
- `apps/web/components/gitlab/gitlab-settings.tsx`
- `apps/web/components/jira/jira-settings.tsx`
- `apps/web/components/linear/linear-settings.tsx`
- `apps/web/components/azure-devops/azure-devops-settings.tsx`
- `apps/web/components/sentry/sentry-instance-form.tsx`
- `apps/web/components/sentry/sentry-settings.tsx`
- `apps/web/components/automations/config-section.tsx`
- `apps/web/components/automations/automation-repository-rows.tsx`
- `apps/web/components/settings/workflow-card.tsx`
- `apps/web/components/settings/workflow-description-field.tsx`
- `apps/web/components/settings/workflow-pipeline-editor-panels.tsx`
- `apps/web/components/settings/workflow-pipeline-editor-step-actions.tsx`
- `apps/web/components/settings/workspace-settings-typography.test.tsx`

## Dependencies

Task 01.

## Parallelism

Parallel-safe with Tasks 02 and 04 after Task 01; owned files are disjoint.

## Inputs

- Spec: `docs/specs/ui/requirements/settings-typography.md`, provider, technical-value,
  and long-string scenarios.
- Plan: `plan.md`, Workspace, provider, and integration surfaces section.
- Audit: `docs/audits/settings-typography/009-centralize-settings-font-family-roles.md`,
  `011-align-workspace-and-provider-settings.md`, and
  `005-normalize-settings-helper-text.md`.

## Output contract

Report the chosen credential family/size role, changed files, exact focused
test and static-check results, and any domain-specific exceptions. Update this
task and the parent plan only after all listed checks pass.

## Results

In progress. Provider credential inputs now share the mono, mobile-safe
credential class, GitHub scope labels use the shared field label, automation
helpers/errors use the normal helper scale, and workflow composition uses the
canonical md breakpoint where it was squeezing. The focused settings tests,
typecheck, lint, i18n check, and i18n ratchet pass. Repository/dialog and
remaining workflow label callsites still need migration and coverage.
