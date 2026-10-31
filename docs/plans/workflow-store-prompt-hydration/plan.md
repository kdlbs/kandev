---
created: 2026-10-26
status: in_progress
requirements:
  - REQ-UI-SETTINGS-MANUAL-SAVE-001
system_design:
  - ../../specs/ui/system-design/settings-manual-save.md
legacy_specs: []
---

# Implementation Plan: Workflow Prompt Hydration

## Overview

Saving any workflow step can erase the workflow-level prompt after the user
reaches workflow settings from the kanban board ([#2761](https://github.com/kdlbs/kandev/issues/2761)).
One work order makes every route hydrate the workflow store with the full
editable workflow projection and adds unit and browser regressions.

## Confirmed root cause and evidence

- `src/kanban-route.tsx` and `src/spa-routes.tsx` hydrate `workflows.items`
  through local `mapWorkflowItem` copies that omit `prompt`;
  `lib/ssr/session-page-state.ts` and the mobile task switcher also omit
  `description`. The Go boot payload (`mapWorkflowItemState`) omits `prompt`
  too, and an already hydrated board skips the client route bootstrap, so a
  failed sidebar refresh keeps those entries (found in review).
- `useWorkflowSettings` merges store entries into clean drafts and baselines.
  A missing prompt is merged as `undefined`, so the field renders empty and is
  not dirty.
- `persistWorkflowDraft` sends `prompt: workflow.prompt ?? ""` with every
  workflow save, including a save that only changed a step.
- Observed on a v0.96.0 desktop instance (Windows): the API returned the prompt
  while the settings field rendered empty; one step-profile save logged
  `workflow updated` and `updated workflow step` in the same second and the
  stored prompt became empty.
- Direct loads of the settings route do not go through the kanban hydration,
  which explains why #2761 did not reproduce there.
- A new `useWorkflowSettings` regression that hydrates the store through the
  shared mapper failed with the old projection and passes with the fix.

## Scope

### In scope

- One shared `mapWorkflowItem` next to `mapWorkspaceItem`, used by the kanban,
  SPA, session-page and mobile task-switcher hydration paths.
- `prompt` in the Go boot payload's `mapWorkflowItemState`.
- A test id on settings search results so the E2E can select the Workflows
  result without depending on its `href`.
- Unit regressions for the mapper and the settings hook, and one chromium E2E
  for board navigation followed by a step save.
- AC-UI-SETTINGS-MANUAL-SAVE-001.9 and the owning design section.

### Out of scope

- API payloads, the save coordinator, the settings merge logic and
  `useWorkflows` (it already maps the full projection).

## Technical approach

Follow [Workflow store projections](../../specs/ui/system-design/settings-manual-save.md#workflow-store-projections).
The shared mapper keeps the kanban mapper's existing field handling and adds
`prompt: workflow.prompt ?? ""`, because the list API omits an empty prompt.
The two thinner paths gain `description`, `prompt` and `sortOrder`.

## Tests

| Test | Covers |
| --- | --- |
| `route-bootstrap.test.ts` › mapWorkflowItem keeps editable fields / maps omitted prompt | .9 |
| `use-workflow-settings.test.ts` › keeps the saved prompt and description when a route hydrated the store | .9 |
| `boot_state_routes_test.go` › TestMapWorkflowItemStateIncludesPrompt | .9 |
| `workflow-settings.spec.ts` › keeps the workflow prompt when a step is saved after opening settings from the board | .9 |

## Work orders

- [ ] [Task 01: Full workflow store projection](task-01-full-workflow-store-projection.md)

## Documentation impact

Public docs describe the workflow prompt but not its persistence internals;
no public page changes.

## Risks

- A future hydration path could add another partial projection. The design
  section and the shared mapper make the full projection the default.
