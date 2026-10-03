---
id: "01-durable-retry-lifecycle"
title: "Deliver the durable automation retry lifecycle"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-RETRIES-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-RETRIES-001.1
  - AC-OFFICE-AUTOMATION-RETRIES-001.2
  - AC-OFFICE-AUTOMATION-RETRIES-001.3
  - AC-OFFICE-AUTOMATION-RETRIES-001.4
  - AC-OFFICE-AUTOMATION-RETRIES-001.5
  - AC-OFFICE-AUTOMATION-RETRIES-001.6
system_design:
  - ../../specs/office/system-design/automation-retries.md
---

# Task 01: Deliver the Durable Automation Retry Lifecycle

## Summary

Persist retry policy and attempt ownership across failures and restarts, then expose safe controls and complete retry history through the automation editor and run-history views.

## In scope

- Validate retry policy modes and ensure a finite policy is valid on the first save.
- Preserve retry group, operation, and outbox ownership across multiple instances and restart recovery.
- Correct PostgreSQL query context and parameter rebinding, due-retry indexing, and per-automation summary query behavior.
- Keep automation disable and retry cancellation safe when stopping a live run fails.
- Project history before filtering, counting, and group-scoped deletion; include older standalone runs.
- Bound history refresh and cursor traversal, use native UI select components, and keep disabled policy controls consistent.
- Preserve legacy webhook interpolation, dedup-key, and repository-selector paths; terminalize permission-denied runs without a live turn.
- Update translated UI and public documentation when observable behavior changes.

## Out of scope

- Changing retry semantics for workflow-step automations or third-party integration delivery queues.
- Adding new retry modes, retry error-classification rules, or provider-specific backoff configuration.
- Changing task/session ownership APIs outside the automation boundary.

## ASCII UI preview

See [UI-01 in the plan](plan.md#ui-01-retry-policy-and-history-presentation). The implemented editor remains a stacked mobile layout; only the retry controls and group history actions change.

## Acceptance

- A finite policy can be selected and saved with a valid retry count, while disabled mode canonicalizes and disables retry-only history controls.
- Each retry attempt has one durable group/generation identity, exact due time, and safe resumable operation; live leases are not stolen, and stale workers cannot publish.
- A failed stop leaves the active run's durable ownership intact. A disabled automation admits no new retry work, and later reconciliation can settle outstanding work.
- Timeline filters/counts and scoped deletes operate on projected groups; ordinary standalone history is not lost behind the retry page cap.
- Webhook retries retain legacy interpolation, dedup-key, and repository-selector values without pointers, and retain only configured safe values when pointers are supplied.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/automation -count=1
go test -tags fts5 ./internal/orchestrator -run '^TestFailAutomationRunOnPermissionWithoutActiveTurn$'
make lint
# Run with KANDEV_TEST_POSTGRES_DSN set in a PostgreSQL-enabled environment.
go test -tags fts5 ./internal/automation -run '^TestPostgresRetryConcurrencyContracts$'
make build
cd ../../apps/web
pnpm run build:e2e
cd ../../apps/backend && make e2e-plugin-ui && make e2e-plugin-package
cd ../web
pnpm run lint
pnpm run typecheck
pnpm exec vitest run \
  components/automations/runs-section.test.tsx \
  components/automations/automation-history.test.ts \
  components/automations/automation-payload.test.ts \
  hooks/domains/settings/use-automation-runs.test.ts \
  hooks/domains/settings/automation-run-history.test.ts \
  hooks/domains/settings/automation-run-polling.test.ts
pnpm e2e:raw --project=chromium tests/automations-webhook-alert-ingest.spec.ts
pnpm run i18n:check
cd ../..
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/backend/internal/automation/`
- `apps/backend/internal/orchestrator/`
- `apps/web/components/automations/`
- `apps/web/hooks/domains/settings/use-automation-runs.ts`
- `apps/web/e2e/tests/automations-webhook-alert-ingest.spec.ts`
- `docs/public/automation-and-mcp.md`
- `docs/specs/office/`

## Results

- Passed: backend automation package tests, backend lint, targeted orchestrator permission-terminalization test, frontend typecheck/lint, 58 focused frontend tests, i18n validation, and PR documentation coverage tests (101 assertions).
- Passed: webhook E2E in Chromium (5/5), Vite E2E build, backend build, plugin UI/package E2E artifacts, specification validation, and spec lint.
- Full orchestrator and full frontend test runs exceeded local time/resource limits. PostgreSQL concurrency integration was not run because `KANDEV_TEST_POSTGRES_DSN` was unavailable.
