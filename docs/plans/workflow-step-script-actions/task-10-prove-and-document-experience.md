---
id: 10-prove-and-document-experience
title: Prove the workflow experience
status: done
wave: 8
depends_on:
  - 08-build-mobile-workflow-editing
  - 09-render-workflow-scripts-in-chat
plan: plan.md
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-002
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-004
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-005
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-006
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-007
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-008
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-002.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-004.2
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-005.3
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-006.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-007.1
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-007.2
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-007.3
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-007.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.5
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.8
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.10
system_design:
  - ../../specs/tasks/system-design/workflow-step-script-actions.md
---

# Task 10: Prove the workflow experience

## Summary

Exercise the complete editor/runtime/chat contract across viewports, executor
state, profile switches, failures, reloads, and portable workflow paths, then
publish the user-facing documentation.

## In scope

- Desktop and mobile E2E for editor navigation, manual save, validation issue
  targeting, script execution, output, and read-only workflows.
- Source/destination profile session placement for reuse/new/park transitions.
- Success, non-zero, timeout, block, continue, duplicate, reload, and
  interrupted recovery scenarios.
- Logs/metrics checks, public workflow/import/sync documentation, and complete
  targeted verification.

## Out of scope

- New behavior discovered beyond the approved requirements; record follow-up
  work instead of expanding this task.

## Acceptance

1. Automated evidence proves the same authoring and inspection outcomes on
   desktop and mobile, including manual save and actionable diagnostics.
2. Runtime evidence proves session placement, lifecycle ordering, at-most-once
   recovery, every terminal state, and bounded observability labels.
3. Public docs explain the editor, config, timing, failure/recovery, supported
   executors, and executable-code/persisted-output trust boundary, and the full
   audit passes.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/workflow/workflow-editor.spec.ts tests/workflow/workflow-step-scripts.spec.ts tests/workflow/workflow-step-script-profile-switch.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/workflow/mobile-workflow-editor.spec.ts
cd apps/backend && go test ./internal/workflow/models ./internal/workflow/engine ./internal/workflow/service ./internal/workflow/handlers ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle
cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite ./internal/orchestrator
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps && pnpm --filter @kandev/web lint
node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/e2e/pages/workflow-settings-page.ts`
- `apps/web/e2e/tests/workflow/workflow-editor.spec.ts`
- `apps/web/e2e/tests/workflow/workflow-step-scripts.spec.ts`
- `apps/web/e2e/tests/workflow/workflow-step-script-profile-switch.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-editor.spec.ts`
- `docs/public/tasks-and-workflows.md`
- `docs/public/workflow-import-export.md`
- `docs/public/workflow-sync.md`

## Dependencies

- Tasks 01 through 09 supply the complete behavior and surfaces.

## Risks

- Mock process fixtures cannot alone prove executor placement; retain a real
  backend agentctl assertion.
- Profile assertions must bind to stable session IDs, not labels alone.
- Timeout tests need deterministic short budgets and process cleanup.

## Parallelism

`sequential`. This is the final integration and documentation audit.

## Inputs

- All prior work-order results and existing workflow/profile/mobile E2E
  exemplars.
- Public documentation and observability validation guidance.

## Results

Implemented focused desktop and mobile Playwright coverage, localized all new
editor and chat copy, and updated workflow task, import/export, and sync
documentation with executor-permission and persisted-output warnings. The
focused E2E suites, i18n gates, specification lint, and public-doc validator
pass; broad backend verification retains only the documented host-home config
discovery failures.
