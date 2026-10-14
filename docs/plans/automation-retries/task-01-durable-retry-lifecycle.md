---
id: "01-durable-retry-lifecycle"
title: "Deliver the durable automation retry lifecycle"
status: done
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
  - AC-OFFICE-AUTOMATION-RETRIES-001.7
  - AC-OFFICE-AUTOMATION-RETRIES-001.8
system_design:
  - ../../specs/office/system-design/automation-retries.md
---

# Task 01: Deliver the Durable Automation Retry Lifecycle

## Summary

Persist retry policy and attempt ownership across failures and restarts, then expose safe controls and complete retry history. The editor also resets retry policy when managed conversation is selected and uses consistent radio-card feedback across supported targets.

## In scope

- Validate retry policy modes and ensure a finite policy is valid on the first save.
- Preserve retry group, operation, and outbox ownership across multiple instances and restart recovery.
- Correct PostgreSQL query context and parameter rebinding, due-retry indexing, and per-automation summary query behavior.
- Keep automation disable and retry cancellation safe when stopping a live run fails.
- Project history before filtering, counting, and group-scoped deletion; include older standalone runs.
- Bound history refresh and cursor traversal, use native UI select components, and keep disabled policy controls consistent.
- Preserve legacy webhook interpolation, dedup-key, and repository-selector paths; terminalize permission-denied runs without a live turn.
- Hide retry settings for managed-conversation targets, reset draft policy to canonical disabled values on selection and hydration, and serialize disabled values for managed-target creates and updates.
- Reuse shared selected/unselected card-state styles for retry-mode choices, preserving radio behavior and 44-pixel phone touch targets.
- Document the managed-target retry reset in the public automation guide; add no UI copy unless required, and localize any new user-facing text.

## Out of scope

- Changing retry semantics for workflow-step automations or third-party integration delivery queues.
- Adding new retry modes, retry error-classification rules, or provider-specific backoff configuration.
- Changing backend generic retry admission or managed-conversation delivery retry semantics; managed-target editor saves use the existing retry-policy contract with disabled values.
- Changing task/session ownership APIs outside the automation boundary.

## ASCII UI preview

See the full [UI-01 preview in the plan](plan.md#ui-01-retry-policy-target-eligibility-and-history-presentation).

**Desktop**

```text
Retry policy (retry-capable target)
  +------------------------------+ selected: primary border + pale primary fill
  | (●) Do not retry             |
  +------------------------------+
  +------------------------------+ neutral border; faint muted fill on hover
  | ( ) Retry a fixed number     |
  +------------------------------+

Managed-conversation target
  [Managed destination selector]
  Context between runs: hidden
  Retry policy: hidden; draft disabled
  Existing enabled policy: dirty until Save
```

**Phone**

```text
Same stacked editor order; managed mode hides both sections above.
Retry cards stay full width and at least 44px high.
Selected border/fill is the touch feedback; no hover state is required.
```

## Acceptance

- A finite policy can be selected and saved with a valid retry count, while disabled mode canonicalizes and disables retry-only history controls.
- Each retry attempt has one durable group/generation identity, exact due time, and safe resumable operation; live leases are not stolen, and stale workers cannot publish.
- A failed stop leaves the active run's durable ownership intact. A disabled automation admits no new retry work, and later reconciliation can settle outstanding work.
- Timeline filters/counts and scoped deletes operate on projected groups; ordinary standalone history is not lost behind the retry page cap.
- Webhook retries retain legacy interpolation, dedup-key, and repository-selector values without pointers, and retain only configured safe values when pointers are supplied.
- For a managed-conversation target, the editor hides retry controls and resets or hydrates the draft with canonical disabled values. A legacy enabled value remains the dirty baseline until explicit save; managed-target create/update payloads contain disabled policy. Switching back never restores discarded values.
- Retry-mode cards share the Context between runs and Run destination selected border/fill and unselected hover treatment; keyboard radio selection remains available and phone selection has visible feedback.

## Verification

In a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)` once before the first frontend command.

```bash
(cd apps/backend && go test -tags fts5 ./internal/automation -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestFailAutomationRunOnPermissionWithoutActiveTurn$')
(cd apps/backend && make lint)
# Optional: requires KANDEV_TEST_POSTGRES_DSN and a PostgreSQL-enabled environment.
(cd apps/backend && go test -tags fts5 ./internal/automation -run '^TestPostgresRetryConcurrencyContracts$')
(cd apps/backend && make build)
(cd apps/web && pnpm run build:e2e)
(cd apps/backend && make e2e-plugin-ui && make e2e-plugin-package)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec vitest run \
  components/automations/runs-section.test.tsx \
  components/automations/automation-history.test.ts \
  components/automations/automation-payload.test.ts \
  hooks/domains/settings/use-automation-runs.test.ts \
  hooks/domains/settings/automation-run-history.test.ts \
  hooks/domains/settings/automation-run-polling.test.ts)
(cd apps/web && pnpm e2e:raw --project=chromium tests/automations-webhook-alert-ingest.spec.ts)
(cd apps/web && pnpm e2e:raw --project=chromium tests/automations-settings.spec.ts)
(cd apps/web && pnpm e2e:raw --project=mobile-chrome tests/settings/mobile-automations-settings.spec.ts)
(cd apps/web && pnpm e2e:raw --project=mobile-chrome tests/plugins/mobile-managed-automation.spec.ts)
(cd apps/web && pnpm run i18n:check)
(python3 scripts/list-docs.py validate)
(python3 scripts/lint-spec-files.py --all)
(node --test scripts/validate-public-docs.test.mjs)
(node scripts/validate-public-docs.mjs)
(git diff --check)
```

## Files likely touched

- `apps/backend/internal/automation/`
- `apps/backend/internal/orchestrator/`
- `apps/web/components/automations/automation-editor.tsx`
- `apps/web/components/automations/automation-editor-sections.tsx`
- `apps/web/components/automations/retry-policy-section.tsx`
- `apps/web/components/automations/automation-payload.ts`
- `apps/web/components/automations/automation-card-styles.ts`
- `apps/web/components/automations/automation-payload.test.ts`
- `apps/web/hooks/domains/settings/use-automation-runs.ts`
- `apps/web/e2e/tests/automations-webhook-alert-ingest.spec.ts`
- `apps/web/e2e/tests/automations-settings.spec.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/plugins/managed-automation-helpers.ts`
- `apps/web/e2e/tests/settings/mobile-automations-settings.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-managed-automation.spec.ts`
- `docs/public/automation-and-mcp.md`
- `docs/specs/office/`

## Dependencies

- None; the editor builds on the existing retry policy and managed-conversation target mode.

## Risks

- Existing managed records with non-disabled retry policies stay persisted until the operator saves; current `admitTriggerLocked` can create generic retry groups from that policy before then. Loading must not silently mutate configuration. Blocking managed retry admission immediately requires separate backend scope.
- Saving the reset is intentionally destructive; the editor will not restore the previous retry settings when switching back to a supported target.

## Parallelism

- Sequential; editor state, payload normalization, shared card styling, and E2E behavior form one contract.

## Inputs

- `docs/specs/office/requirements/automation-retries.md`, AC-OFFICE-AUTOMATION-RETRIES-001.7 and .8.
- `docs/specs/office/system-design/automation-retries.md`, Frontend editor behavior.
- Existing editor and payload paths listed under Files likely touched.

## Results

- Passed: backend automation package tests, backend lint, targeted orchestrator permission-terminalization test, frontend typecheck/lint, 58 focused frontend tests, i18n validation, and PR documentation coverage tests (101 assertions).
- Passed: webhook E2E in Chromium (5/5), Vite E2E build, backend build, plugin UI/package E2E artifacts, specification validation, and spec lint.
- Passed for AC-OFFICE-AUTOMATION-RETRIES-001.7 and .8: frontend retry payload tests (19/19), typecheck, focused ESLint, and backend E2E seeder package compile.
- Passed: desktop Chromium managed-target/card-state E2E (1/1) and mobile automation plus managed-policy E2Es (4/4), including touch geometry, no auto-write before explicit save, and persisted reset.
- Passed: public-doc validation tests (62/62) and validation of all 47 published docs pages; specification validation (339 decisions, 1287 specifications) and full spec lint.
- Full retry/orchestrator suites were not rerun. PostgreSQL concurrency integration was not run because `KANDEV_TEST_POSTGRES_DSN` was unavailable. E2E ran under Node 22.22.3; the workspace declares Node 24.
