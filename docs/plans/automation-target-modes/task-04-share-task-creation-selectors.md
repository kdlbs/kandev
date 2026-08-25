---
id: "04-share-task-creation-selectors"
title: "Share task-creation selectors"
status: completed
wave: 4
depends_on:
  - "03-editor-and-proof"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-TARGETS-001
  - REQ-OFFICE-AUTOMATION-TARGETS-003
system_design:
  - ../../specs/office/system-design/automation-target-modes.md
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-TARGETS-001.9
  - AC-OFFICE-AUTOMATION-TARGETS-001.10
  - AC-OFFICE-AUTOMATION-TARGETS-001.11
  - AC-OFFICE-AUTOMATION-TARGETS-003.2
  - AC-OFFICE-AUTOMATION-TARGETS-003.3
  - AC-OFFICE-AUTOMATION-TARGETS-003.5
---

# Task 04: Share task-creation selectors

## Summary

Remove the first-workspace-repository fallback, persist exact repository and
base-branch pairs, and compose the automation configuration from the shared
task-creation workflow, repository, agent, and executor selectors.

## In scope

- Add ordered base branches to automation repository persistence and API data.
- Migrate empty workspace-default rows to no repository and selected rows to a
  concrete default branch.
- Dispatch exact saved base branches and include them in continuation checks.
- Remove the automation workflow-step and repository-mode controls.
- Reuse task-creation workflow previews, profile search/logos, and paired
  repository/base-branch chips.
- Add focused backend/frontend tests and desktop/mobile E2E proof.
- Update public docs and localized copy.

## Acceptance

- The automation editor has no workspace-default repository option and no
  workflow-step picker.
- A person can select multiple repository/base-branch pairs or no repository.
- Workflow and profile controls provide the same previews, search, logos, and
  availability behavior as task creation.
- A new visible automation task starts at the workflow's configured start step.
- Desktop and mobile retain one scroll owner, 44-pixel touch controls, and zero
  horizontal overflow.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/automation ./internal/orchestrator
cd apps && pnpm --filter @kandev/web test -- --run components/automations
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run tests/automations-target-modes.spec.ts tests/automations-settings.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/mobile-automations-scroll.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Parallelism

`sequential`

## Results

- Removed the first-workspace-repository and workflow-step controls.
- Reused the store-backed New Task workflow preview, searchable profile
  selectors, and paired repository/base-branch chips.
- Persisted ordered repository/base-branch pairs and used them for dispatch and
  continuation compatibility.
- Kept Worktree valid for an empty repository list through task-owned scratch
  execution.
- Verification: 2,937 focused backend tests, 296 focused frontend tests,
  typecheck, i18n checks, 19 desktop settings tests, two target-mode tests,
  three mobile settings tests, and public documentation validation passed.
