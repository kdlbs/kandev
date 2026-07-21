---
spec: docs/specs/ui/settings-manual-save.md
decision: docs/decisions/0046-settings-route-save-coordinator.md
created: 2026-07-14
status: implemented
---

# Implementation Plan: Settings Manual Save

## Overview

Introduce a route-scoped save coordinator and navigation guard in the settings shell, then migrate workflow, general/system, resource-editor, and integration settings to register local drafts. The migrations reuse existing APIs; explicit commands and dialog submissions remain immediate. Desktop/mobile E2E and a final write-through audit close the work.

## Audit

Confirmed write-through settings are:

- workflows: metadata, workflow/step ordering, step fields, and step membership in `workspace-workflows-client.tsx` and `workflow-card-actions.ts`;
- General: theme, changes-panel layout, resource metrics, submit key, keyboard shortcuts, terminal font/size/link behavior, voice mode, and changelog notification;
- operational: utility-agent enable/default/config-chat selection, runtime flags, and SSH profile login shell;
- integrations: GitHub/Jira/Linear/Sentry watcher enable toggles and existing Sentry instance
  editing;
- list and connection editors: automation enabled toggles and existing SSH executor editing.

Existing manual-save surfaces use header, section, card, or footer actions: workspace/repository, agent/profile, executor/profile, automation, editors, notifications, prompts, secrets, shell, GitHub scope/presets/queries, GitLab credentials, and Jira/Linear/Slack configuration. These move to the shared action when the form is route-level. Dialog/sheet submissions remain local to their overlay.

## Backend

No backend, schema, or API changes are planned. Existing partial-update and resource endpoints remain the persistence boundary. Multi-contributor and multi-step workflow saves are intentionally non-atomic.

## Frontend

### Save coordinator and navigation

- Add `apps/web/components/settings/settings-save-provider.tsx` with a keyed contributor registry (`isDirty`, `canSave`, `invalidReason`, `save`, `discard`) and revision-safe save-all behavior.
- Add `apps/web/components/settings/settings-floating-save.tsx` for the fixed status/action surface and unsaved-navigation dialog.
- Update `apps/web/components/settings/settings-layout-client.tsx` to own the provider, reserve scroll padding, and position the action using mobile safe-area insets.
- Add a navigation-blocking boundary in `apps/web/lib/routing/navigation-guard.ts`; integrate it with `client-router.ts`, `app-link.tsx`, browser history, and `beforeunload`.

### Workflow drafts

- Replace mutation-on-control-change in `apps/web/app/settings/workspace/workspace-workflows-client.tsx` and `apps/web/components/settings/workflow-card-actions.ts` with saved snapshots and local workflow/step drafts.
- Update `use-workflow-creation.ts`, `workflow-card.tsx`, and the pipeline editor files so new workflows/steps use client identities and Save creates/remaps them.
- Preserve immediate confirmed deletion/migration/import/export. Newly added unsaved steps are removed locally.

### General, system, and utility settings

- Compose backend user-settings fields per route so one contributor issues one patch and controls do not write independently.
- Give theme selection a preview-only draft path; commit local storage on Save and restore the saved theme on discard.
- Stage global metrics/runtime flags and only show restart affordances after successful Save when required.

### Existing manual-save pages

- Convert `SettingsPageTemplate`, `UnsavedSaveButton`, profile chrome, and route-level edit cards to registrations; remove duplicate embedded Save actions. New-resource routes and overlay forms retain explicit Create actions.
- Fold SSH login-shell selection into the executor-profile draft instead of invoking profile Save out of band.
- Preserve create/save/delete controls inside dialogs and sheets.
- Pages with several cards, such as repositories, register stable per-resource contributors so the floating action saves all dirty cards.

### Integrations

- Register page-level GitHub scope, action-preset, default-query, GitLab credential, and Jira/Linear/Slack configuration drafts.
- Stage watcher enabled changes on GitHub, Jira, Linear, and Sentry; watcher Create/Edit dialogs, run/reset commands, and confirmed deletion stay immediate.
- Resetting default queries or action presets updates the draft and waits for Save.

## Tests

- Coordinator component tests cover registration, stable ordering, validation, partial failure, retry, duplicate submission, unregister, discard, and edits during an in-flight save.
- Routing tests cover Link/router/history blocking, Save and leave, Discard and leave, failed Save and leave, and `beforeunload` registration.
- Workflow unit tests prove toggles/reorders/additions do not call APIs before Save, temporary IDs remap, partial creation retries without duplication, and confirmed deletes remain immediate.
- Focused settings tests prove each former write-through surface stays local until Save and reset-to-default remains draftable.
- Existing manual-form tests are updated to target the shared action without weakening dirty/validation assertions.

## E2E Tests

- Update `apps/web/e2e/pages/workflow-settings-page.ts` and workflow desktop/mobile specs to prove no persistence before Save, one Save covers metadata and steps, new workflow creation waits for Save, and destructive commands remain explicit.
- Add `apps/web/e2e/tests/settings/settings-manual-save.spec.ts` for Appearance plus a second settings surface, reload persistence, partial failure/status where mockable, and dirty navigation.
- Update `mobile-general-settings.spec.ts` to verify the 390px floating action, safe-area clearance, touch target, last-control reachability, and no horizontal overflow.
- Run affected existing agent, executor, repository, automation, terminal, and integration E2E specs after their Save selectors change.

## Risks

- Existing endpoints are not transactional; partial success must remain visible and retryable.
- Several sections patch `userSettings`; each route must compose a single owner or serialize patches to avoid lost fields.
- Workflow temporary-ID remapping affects transitions and pull-from references and needs focused regression coverage.
- Browser back/forward blocking can corrupt history if the blocked destination is not restored carefully.
- Theme preview and websocket/store refreshes must not overwrite dirty drafts or accidentally persist previews.

## Out of Scope

- Backend transactions, record versions, multi-user merge/conflict UI, and durable client drafts.
- Replacing named operational commands or overlay-local Create/Save/Delete actions.

## Implementation Waves

Wave 1:

- [x] [Task 01: Save coordinator and navigation guard](task-01-save-coordinator.md) (done)

Wave 2 (parallel after Task 01; file ownership is disjoint):

- [x] [Task 02: Workflow drafts](task-02-workflow-drafts.md) (done)
- [x] [Task 03: General and operational drafts](task-03-general-operational-drafts.md) (done)
- [x] [Task 04: Existing manual editor migration](task-04-manual-editor-migration.md) (done)
- [x] [Task 05: Integration settings migration](task-05-integration-settings.md) (done)

Wave 3:

- [x] [Task 06: E2E, mobile QA, and final audit](task-06-e2e-mobile-qa.md) (done)

Task 06 completed the static settings audit, rendered desktop and mobile Playwright coverage,
Docker-backed SSH persistence coverage, and the full repository verification pipeline.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/settings app/settings lib/routing
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm e2e:run tests/workflow/workflow-settings.spec.ts tests/settings/settings-manual-save.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/workflow/mobile-workflow-settings.spec.ts tests/settings/mobile-general-settings.spec.ts
GOCACHE=/tmp/kandev-go-cache make fmt
GOCACHE=/tmp/kandev-go-cache make typecheck
GOCACHE=/tmp/kandev-go-cache make test
GOCACHE=/tmp/kandev-go-cache GOLANGCI_LINT_CACHE=/tmp/kandev-golangci-cache make lint
```
