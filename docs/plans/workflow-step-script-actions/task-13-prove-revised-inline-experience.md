---
id: 13-prove-revised-inline-experience
title: Prove the revised inline experience
status: pending
wave: 10
depends_on:
  - 11-harden-script-occurrence-and-locks
  - 12-restore-inline-workflow-tabs
plan: plan.md
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-002
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-004
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-006
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-008
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-002.2
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-002.3
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-002.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-004.2
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-006.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.1
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.6
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.8
system_design:
  - ../../specs/tasks/system-design/workflow-step-script-actions.md
---

# Task 13: Prove the revised inline experience

## Summary

Replace route-specific editor evidence with desktop/mobile inline workflow-card
coverage and add the missing runtime profile-switch and failure scenarios.

## In scope

- Desktop inline authoring, tab density, action focus, dirty step switching,
  multi-workflow save, validation targeting, and read-only coverage.
- Mobile inline authoring, metadata parity, touch targets, action ordering, and
  document-overflow coverage.
- Runtime E2E for profile reuse/new/park placement, repeated transitions,
  success, non-zero, timeout, block, continue, reload, and recovery.
- Remove superseded dedicated-route and mobile-journey assertions.

## Out of scope

- Unrelated broad browser-suite remediation.

## Acceptance

1. Playwright proves desktop and phone authors use the existing workflow card
   and compact tabs for the same authoring outcomes.
2. Profile-switch evidence asserts source completion/exit output and destination
   entry output in their exact agent sessions.
3. Repeated lifecycle occurrences execute once each, while duplicate delivery
   of one occurrence does not start another process.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/workflow/workflow-settings.spec.ts tests/workflow/workflow-step-scripts.spec.ts tests/workflow/workflow-step-script-profile-switch.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/workflow/mobile-workflow-cycle-guardrails.spec.ts
cd apps/backend && go test -race -tags fts5 -run 'Test.*WorkflowScript|Test.*ProfileSwitch' ./internal/orchestrator
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps && pnpm --filter @kandev/web lint
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/e2e/pages/workflow-settings-page.ts`
- `apps/web/e2e/tests/workflow/workflow-settings.spec.ts`
- `apps/web/e2e/tests/workflow/workflow-step-scripts.spec.ts`
- `apps/web/e2e/tests/workflow/workflow-step-script-profile-switch.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-cycle-guardrails.spec.ts`
- Focused backend orchestration tests.

## Dependencies

- Tasks 11 and 12 provide the final runtime and editor behavior.

## Risks

- Mock-only browser assertions can prove placement labels without proving the
  bound runtime session; retain backend assertions on stored session IDs.
- Timing-based streaming tests need deterministic fixtures and bounded waits.

## Parallelism

`sequential`. This task owns final integration evidence.

## Results

Pending implementation.
