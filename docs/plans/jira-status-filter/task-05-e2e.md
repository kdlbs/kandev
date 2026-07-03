---
id: "05-e2e"
title: "E2E: Jira status filter"
status: done
wave: 4
depends_on: ["01-backend-status-endpoint", "04-frontend-ui-wiring"]
plan: "plan.md"
spec: "../../specs/jira-status-filter/spec.md"
---

# Task 05: E2E coverage for the status filter

Verify the user-visible flow against the mock Jira client.

## Acceptance
- Spec scenario 1: with no project selected, the Status pill is disabled and shows the pick-a-project hint.
- Spec scenario 2: after selecting a seeded project, the Status pill lists real status names (`In Development`, `Ready for review`) and not the old categories.
- Spec scenario 3: checking `Ready for review` narrows the ticket list to matching seeded tickets.
- Uses the mock control routes to seed projects, project statuses, and tickets.

## Verification
- `cd apps/web && pnpm e2e tests/integrations/jira-status-filter.spec.ts`

## Files likely touched
- `apps/web/e2e/tests/integrations/jira-status-filter.spec.ts`
- possibly a shared Jira e2e fixture/helper under `apps/web/e2e/`

## Inputs
- Spec: Scenarios 1-3.
- Plan: E2E Tests.
- Patterns: existing Jira e2e specs and mock seeding via `/api/v1/jira/mock/...` (see `apps/web/e2e/README.md`).

## Dependencies
Tasks 01 and 04.

## Output contract
Report: summary, files changed, e2e run result, blockers, risks; set `status: done` + tick plan checkbox.
