---
spec: docs/specs/ui/requirements/settings-prompt-editor.md
created: 2026-08-20
status: done
---

# Implementation Plan: Settings Prompt Editor

## Overview

Add a prompt-specific Settings editor above the existing Monaco `ScriptEditor`. First, scope completion providers to each Monaco model.

Then migrate all prompt-authoring settings fields to the shared editor.

The change preserves all stored strings and current save APIs. It adds frontend behavior, focused tests, desktop and mobile E2E coverage, and public documentation.

## Audit

The current prompt editors have four different states:

- Workflow step and GitHub review-watch prompts support placeholders and saved-prompt references.
- GitHub, Jira, and Azure DevOps quick actions support placeholders only.
- Several watcher, workflow, automation, and utility prompts use Monaco with only part of the available completion contract.
- GitLab quick actions, GitLab watches, Azure DevOps watches, workflow prompts, and custom prompts use plain textareas.

`ScriptEditor` registers one completion provider per language and slot. The last mounted editor can replace the provider for another mounted editor.

## Backend

No backend, database, API, or interpolation change is required.

Task descriptions already resolve saved-prompt references during workflow prompt assembly. Utility prompts use a separate sessionless template engine and do not resolve these references.

## Frontend

### Shared prompt editor

Create `apps/web/components/settings/settings-prompt-editor.tsx` with a controlled plain-text contract:

- `value` and `onChange`
- `placeholders?: ScriptPlaceholder[]`
- `promptReferences?: boolean`
- `excludedPromptIds?: string[]`
- `language?: "markdown" | "plaintext"`
- `height`, `minLines`, `readOnly`, `ariaLabel`, `testId`, and dirty-state props
- optional supplemental help content

The component reads saved prompts through `useCustomPrompts` when prompt references are enabled. It filters excluded prompts before it passes them to `ScriptEditor`.

The component owns the border, completion hint, loading-safe prompt list, and content-based default height. Consumers keep field labels and context-specific placeholder descriptions.

Add localized completion hints to the `settings` catalogs. Use one hint for placeholders, one for prompt references, and one for both completion types.

### Monaco completion ownership

Update `apps/web/components/settings/profile-edit/script-editor.tsx`. Replace the language-wide singleton registrations with per-instance disposables.

Wrap each completion provider with a model identity check. A provider returns no items when Monaco calls it for a different model.

Dispose both providers when the editor unmounts. Re-register only the changed provider when placeholders or saved prompts change.

Expose stable accessibility and test attributes from `ScriptEditor`. Keep the existing script-editor consumers compatible.

### Quick actions

Use `SettingsPromptEditor` in these files:

- `apps/web/components/github/action-presets-section.tsx`
- `apps/web/components/gitlab/action-presets-section.tsx`
- `apps/web/components/jira/task-presets-section.tsx`
- `apps/web/components/azure-devops/azure-devops-quick-actions.tsx`

Enable saved-prompt references for every quick action. Keep each provider's current `{{...}}` tokens.

Add GitLab `{{url}}` and `{{title}}` definitions. Keep the current quick-action data model and shared save contributor.

### Watch prompts

Use `SettingsPromptEditor` for GitHub, GitLab, Jira, Linear, Sentry, and Azure DevOps watch prompts. Enable saved-prompt references for all watch prompts.

Reuse existing placeholder definitions for GitHub, GitLab, Jira, Linear, and Sentry. Add typed Azure DevOps definitions for work-item and pull-request watch tokens.

Do not migrate provider query fields such as WIQL or JQL. These fields are not agent prompts.

### Other Settings prompts

Use `SettingsPromptEditor` for these settings fields:

- workflow-level prompts
- workflow step prompts
- automation instructions
- custom prompt content
- utility agent prompt templates

Enable saved-prompt references for workflow, automation, and custom prompt fields. Exclude the current custom prompt from its own completion list.

Disable saved-prompt references for utility prompts. Keep utility template variables enabled.

Update existing tests that assume a native textarea. Use the stable editor test ID and controlled-value assertions instead.

### Mobile design contract

- **Desktop outcome and mobile entry:** both viewports use the existing Settings routes and existing quick-action expansion control.
- **Nearest mobile exemplars:** `mobile-github-integration-layout.spec.ts` and `mobile-workflow-settings.spec.ts` define the settings-page composition and overflow rules.
- **Hierarchy:** the field label and completion hint remain above and below the inline editor. The route-level Save changes control remains primary.
- **Presentation:** keep the editor inline. A drawer adds no value because prompt editing is the primary content of the expanded settings row.
- **Scroll owner:** the Settings page remains the page scroll owner. Monaco owns vertical editor scroll after the content-height cap.
- **Touch and geometry:** completion items support touch selection. The editor and suggestion widget must stay inside the phone viewport.
- **Shared logic:** editor value, completion providers, dirty state, and save behavior are identical across viewports.

## Tests

- **What:** two mounted editors keep separate placeholder and saved-prompt providers.
  **Files:** `apps/web/components/settings/profile-edit/script-editor.test.tsx` and `script-editor-completions.test.ts`.
  **How:** use Monaco stubs with distinct models. Make sure that each provider rejects the other model and disposes on unmount.
- **What:** the shared editor selects the correct completion modes and excludes the open custom prompt.
  **File:** `apps/web/components/settings/settings-prompt-editor.test.tsx`.
  **How:** mock `ScriptEditor` and `useCustomPrompts`. Assert props, dynamic prompt updates, dirty state, help text, and no prompt fetch error effect.
- **What:** quick actions preserve drafts, placeholders, reset behavior, and save coordination while enabling prompt references.
  **Files:** focused tests beside the four quick-action sections.
  **How:** mock the shared editor and assert provider tokens, prompt-reference mode, and unchanged save payloads.
- **What:** watcher and core Settings prompts use the shared editor with the correct completion contract.
  **Files:** existing watcher, workflow, and custom-prompt component tests. Add focused tests where no consumer test exists.
  **How:** assert component props and controlled updates. Keep persistence tests on the existing save coordinator.

## E2E Tests

- **Scenario:** **GIVEN** a saved prompt and GitHub quick action, **WHEN** the user selects `@name` and `{{title}}`, **THEN** both references persist after reload.
  **File:** `apps/web/e2e/tests/integrations/github-quick-action-prompt-autocomplete.spec.ts`.
  **What to verify:** both Monaco menus, keyboard or pointer selection, route save, reload persistence, and no automatic save on selection.
- **Scenario:** **GIVEN** GitHub settings on a phone, **WHEN** the user selects both completion types, **THEN** the editor remains usable without page overflow.
  **File:** `apps/web/e2e/tests/integrations/mobile-github-quick-action-prompt-autocomplete.spec.ts`.
  **What to verify:** touch entry, suggestion selection, viewport containment, internal editor scrolling, and no document horizontal overflow.
- **Scenario:** **GIVEN** a custom prompt editor, **WHEN** the user types `@`, **THEN** other prompts appear and the current prompt is absent.
  **File:** `apps/web/e2e/tests/settings/prompts-settings.spec.ts`.
  **What to verify:** create and edit paths, selection, save, and reload persistence.
- **Scenario:** existing workflow step completion behavior remains available through the shared editor.
  **File:** `apps/web/e2e/tests/workflow/workflow-step-autocomplete.spec.ts`.
  **What to verify:** existing placeholder and saved-prompt cases remain green.

## Public documentation

Update these user guides:

- `docs/public/integrations.md` for quick-action and watch prompt completion.
- `docs/public/workflow-tips.md` for workflow-level and step prompt completion.
- `docs/public/developer-tools.md` for nested saved prompts and the utility prompt exception.

These pages remain how-to or explanation pages. No new navigation entry is required.

## Verification Results

- Focused unit/component suites pass for the shared editor, Monaco ownership, quick actions, watch prompts, and core Settings prompts.
- `pnpm run typecheck` and `pnpm exec tsc --noEmit --incremental false` pass.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` pass.
- `node --test scripts/validate-public-docs.test.mjs` passes (61 tests), and `node scripts/validate-public-docs.mjs` validates 41 published pages.
- Desktop and mobile managed Playwright runs pass for the new completion flows, custom-prompt exclusion, and existing workflow autocomplete.
- `git diff --check` passes.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-shared-editor](task-01-shared-editor.md)

Wave 2:

- [x] [task-02-quick-actions](task-02-quick-actions.md)

Wave 3:

- [x] [task-03-watch-prompts](task-03-watch-prompts.md)
- [x] [task-04-core-settings-prompts](task-04-core-settings-prompts.md)

Wave 4:

- [x] [task-05-browser-coverage](task-05-browser-coverage.md)
- [x] [task-06-public-documentation](task-06-public-documentation.md)

Tasks 03 and 04 are parallel-safe after Task 01 because their production files are disjoint.

Tasks 05 and 06 are parallel-safe after Tasks 02 through 04.

The primary conversation runs tasks sequentially by default. These labels do not authorize subagents.

## Risks

- Monaco completion providers are language-global. Model scoping must prevent cross-editor suggestions without breaking existing script completions.
- Migrated native textareas change E2E input mechanics. Tests must focus Monaco's `native-edit-context` before they type.
- GitLab and Azure DevOps watcher placeholders exist in backend interpolators. Frontend lists must match those exact tokens.
- A saved-prompt suggestion is valid only when the runtime expands references. Utility prompts must not enable that completion mode.
- Several settings routes can mount more than one editor. The component must avoid repeated prompt requests and provider churn.
