---
id: "04-core-settings-prompts"
title: "Migrate core settings prompts"
status: done
wave: 3
depends_on: ["01-shared-editor"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-prompt-editor.md"
---

# Task 04: Migrate core settings prompts

## Acceptance

- Workflow, automation, custom-prompt, and utility-agent fields use `SettingsPromptEditor`.
- Workflow, automation, and custom prompts offer saved-prompt references. The open custom prompt is excluded.
- Utility prompts offer only utility variables. Existing dirty, save, reset, read-only, and cross-tab behavior remains unchanged.

## Verification

Follow TDD. Update native-textarea assumptions only after the shared-editor assertions fail.

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run components/settings/workflow-prompt-section.test.tsx components/settings/workflow-step-prompt-section.test.tsx components/settings/prompts-settings.test.tsx components/settings/utility-agent-dialog.test.tsx components/automations/prompt-section.test.tsx && cd web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet && pnpm exec eslint components/settings/workflow-prompt-section.tsx components/settings/workflow-prompt-section.test.tsx components/settings/workflow-step-prompt-section.tsx components/settings/workflow-step-prompt-section.test.tsx components/settings/prompts-settings.tsx components/settings/prompts-settings.test.tsx components/settings/utility-agent-dialog.tsx components/settings/utility-agent-dialog.test.tsx components/automations/prompt-section.tsx components/automations/prompt-section.test.tsx
```

## Files likely touched

- `apps/web/components/settings/workflow-prompt-section.tsx`
- `apps/web/components/settings/workflow-prompt-section.test.tsx`
- `apps/web/components/settings/workflow-step-prompt-section.tsx`
- `apps/web/components/settings/workflow-step-prompt-section.test.tsx`
- `apps/web/components/settings/prompts-settings.tsx`
- `apps/web/components/settings/prompts-settings.test.tsx`
- `apps/web/components/settings/utility-agent-dialog.tsx`
- `apps/web/components/settings/utility-agent-dialog.test.tsx`
- `apps/web/components/automations/prompt-section.tsx`
- `apps/web/components/automations/prompt-section.test.tsx`
- Relevant locale catalogs when existing help text changes

## Dependencies

Task 01.

## Parallelism

Parallel-safe with Task 03 after Task 01. The tasks use disjoint production and test files.

## Inputs

- Spec: `Settings surfaces`, `Failure modes`, and the custom-prompt and utility scenarios.
- Plan: `Other Settings prompts` and `Shared prompt editor`.
- Existing patterns: workflow step prompt references, prompt save coordination, automation placeholders, and utility variables.

## Output contract

Report the RED error, enabled completion modes, textarea-test migrations, files changed, command results, blockers, risks, and synchronized task and plan status.

## Results

- Workflow, workflow-step, automation, custom-prompt, and utility-agent fields use `SettingsPromptEditor`.
- Workflow, automation, and custom prompts enable saved-prompt references; the open custom prompt is excluded. Utility prompts keep only their template-variable completion mode.
- Focused core tests pass: 10 tests across 5 files.
- Existing save, reset, read-only, dirty-state, and controlled-value behavior remains owned by each consumer.
