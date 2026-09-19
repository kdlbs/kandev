---
id: "02-quick-actions"
title: "Migrate quick action prompts"
status: done
wave: 2
depends_on: ["01-shared-editor"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-prompt-editor.md"
---

# Task 02: Migrate quick action prompts

## Acceptance

- GitHub, GitLab, Jira, and Azure DevOps quick actions use `SettingsPromptEditor`.
- Each provider offers its exact placeholders and saved-prompt references.
- Draft, reset, dirty-state, validation, and shared Save changes behavior remain unchanged.

## Verification

Follow TDD. Add consumer assertions for completion modes and tokens before each migration.

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run components/github/action-presets-section.test.tsx components/gitlab/action-presets-section.test.tsx components/jira/task-presets-section.test.tsx components/azure-devops/azure-devops-quick-actions.test.tsx && cd web && pnpm run i18n:ratchet && pnpm exec eslint components/github/action-presets-section.tsx components/github/action-presets-section.test.tsx components/gitlab/action-presets-section.tsx components/gitlab/action-presets-section.test.tsx components/jira/task-presets-section.tsx components/jira/task-presets-section.test.tsx components/azure-devops/azure-devops-quick-actions.tsx components/azure-devops/azure-devops-quick-actions.test.tsx
```

## Files likely touched

- `apps/web/components/github/action-presets-section.tsx`
- `apps/web/components/github/action-presets-section.test.tsx`
- `apps/web/components/gitlab/action-presets-section.tsx`
- `apps/web/components/gitlab/action-presets-section.test.tsx`
- `apps/web/components/jira/task-presets-section.tsx`
- `apps/web/components/jira/task-presets-section.test.tsx`
- `apps/web/components/azure-devops/azure-devops-quick-actions.tsx`
- `apps/web/components/azure-devops/azure-devops-quick-actions.test.tsx`
- GitLab locale catalogs only if new provider-specific placeholder descriptions are required

## Dependencies

Task 01.

## Parallelism

Sequential. All four consumers share the new component contract and quick-action E2E surface.

## Inputs

- Spec: `Settings surfaces` and the GitHub quick-action scenarios.
- Plan: `Quick actions` and `Shared prompt editor`.
- Existing patterns: GitHub and Azure DevOps expandable prompt rows, Jira placeholder definitions, and each save contributor.

## Output contract

Report the RED error, provider token lists, save-payload evidence, files changed, exact command results, blockers, risks, and synchronized task and plan status.

## Results

- GitHub, GitLab, Jira, and Azure DevOps quick-action prompt fields use `SettingsPromptEditor` with saved-prompt references and their provider-specific placeholder maps.
- GitLab now exposes localized `{{url}}` and `{{title}}` descriptions. Existing draft, reset, dirty-state, and save-contributor paths remain unchanged.
- Focused consumer tests pass: 7 tests across 4 files.
- `pnpm run i18n:ratchet` passes with no new hardcoded-copy violations.
