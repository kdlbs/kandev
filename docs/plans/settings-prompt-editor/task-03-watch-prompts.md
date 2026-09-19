---
id: "03-watch-prompts"
title: "Migrate watch prompts"
status: done
wave: 3
depends_on: ["01-shared-editor"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-prompt-editor.md"
---

# Task 03: Migrate watch prompts

## Acceptance

- GitHub, GitLab, Jira, Linear, Sentry, and Azure DevOps watch prompts use `SettingsPromptEditor`.
- Each watch offers backend-supported placeholders and saved-prompt references.
- Query fields, watcher validation, responsive dialog composition, and persistence remain unchanged.

## Verification

Follow TDD. Add focused assertions for GitLab and Azure DevOps token lists before the textarea migrations.

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run components/github/review-watch-dialog.test.tsx components/gitlab/review-watch-dialog.test.tsx components/gitlab/issue-watch-dialog.test.tsx components/azure-devops/azure-devops-watch-settings.test.tsx components/sentry/sentry-issue-watch-dialog.test.tsx && cd web && pnpm run i18n:ratchet && pnpm exec eslint components/github/issue-watch-dialog.tsx components/github/review-watch-prompt-field.tsx components/gitlab/watch-dialog.tsx components/gitlab/review-watch-placeholders.ts components/gitlab/issue-watch-placeholders.ts components/jira/jira-issue-watch-dialog.tsx components/linear/linear-issue-watch-dialog.tsx components/sentry/sentry-issue-watch-dialog.tsx components/azure-devops/azure-devops-watch-settings.tsx
```

## Files likely touched

- `apps/web/components/github/issue-watch-dialog.tsx`
- `apps/web/components/github/review-watch-prompt-field.tsx`
- `apps/web/components/github/review-watch-dialog.test.tsx`
- `apps/web/components/gitlab/watch-dialog.tsx`
- `apps/web/components/gitlab/review-watch-placeholders.ts`
- `apps/web/components/gitlab/issue-watch-placeholders.ts`
- `apps/web/components/gitlab/review-watch-dialog.test.tsx`
- `apps/web/components/gitlab/issue-watch-dialog.test.tsx`
- `apps/web/components/jira/jira-issue-watch-dialog.tsx`
- `apps/web/components/linear/linear-issue-watch-dialog.tsx`
- `apps/web/components/sentry/sentry-issue-watch-dialog.tsx`
- `apps/web/components/sentry/sentry-issue-watch-dialog.test.tsx`
- `apps/web/components/azure-devops/azure-devops-watch-settings.tsx`
- `apps/web/components/azure-devops/azure-devops-watch-placeholders.ts`
- `apps/web/components/azure-devops/azure-devops-watch-settings.test.tsx`
- Provider locale catalogs for new placeholder descriptions and completion help

## Dependencies

Task 01.

## Parallelism

Parallel-safe with Task 04 after Task 01. The tasks use disjoint production and test files.

## Inputs

- Spec: the watch-prompt row in `Settings surfaces` and the saved-prompt failure scenario.
- Plan: `Watch prompts` and `Mobile design contract`.
- Backend token authority: `source_gitlab.go`, `source_azuredevops.go`, and existing provider placeholder modules.

## Output contract

Report the RED error, exact token maps, responsive behavior, files changed, command results, blockers, risks, and synchronized task and plan status.

## Results

- GitHub, GitLab, Jira, Linear, Sentry, and Azure DevOps watch prompt fields use `SettingsPromptEditor`; query fields such as WIQL and JQL remain native query inputs.
- GitLab placeholder descriptions are localized and Azure DevOps work-item and pull-request token maps are typed against the backend-supported fields.
- Focused watch tests pass: 13 tests across 7 files, plus 5 Azure DevOps watch-settings tests.
- No watcher save, validation, dialog, or persistence contract changed.
