---
spec: docs/specs/platform/requirements/provider-error-recovery.md
created: 2026-08-17
status: done
---

# Implementation Plan: Provider Error Policies

## Overview

Replace dynamic candidates' generic provider-error action map with shared,
versioned transient and hard policies. Build a deterministic provider-error
catalogue, persist bounded reset waits and exponential retries, and apply the
same policy in Kanban, utility calls, workflows, and Office without weakening
effect-safety or route-generation fencing.

This package also removes Add profile from the one-profile creation route and
adds complete desktop and mobile policy controls to each candidate. Model-based
error classification, cost routing, and telemetry routing are deferred.

## Product and architecture contract

- `routingerr` owns deterministic evidence normalization, semantic codes,
  classes, timing hints, catalogue versions, and fixture-driven provider
  signatures.
- A shared runtime policy package owns validation and evaluation of reset wait,
  exponential retry, skip, and stop. Workspace modes do not copy its tables.
- Unknown, low-confidence, stale, conflicting, or effect-unsafe failures stop
  automatic recovery.
- Each class policy has optional reset waiting, optional retry, and an explicit
  exhausted outcome. This prevents retry exhaustion from having an implicit
  behavior.
- Legacy dynamic rules normalize to a versioned policy document. The backend
  returns only the normalized form.
- Retry and reset deadlines are backend-owned, durable, restart-safe, and
  generation-fenced.
- The dynamic new-profile route owns one draft and has no Add profile action.
- Desktop and phone expose the same values and validation. Phone uses one
  column, one scroll owner, 44px controls, and no horizontal overflow.

## Backend

### Classification and catalogue

- Add a `Class` and catalogue version to normalized provider errors.
- Exhaustively map recoverable semantic codes to `transient` or `hard`.
  Non-provider and unknown evidence remain outside configurable policy.
- Extend structured and fixture-driven rules for capacity, network, timeout,
  outage, throttling, subscription, quota/reset, credits, auth, credentials,
  model, and provider-configuration failures across supported adapters.
- Validate and sanitize `retry_after` and `reset_at`; timing extraction cannot
  trust arbitrary model output.
- Remove classifier-owned recovery booleans after all consumers move to the
  shared policy contract.

### Policy document and compatibility

- Replace candidate `rules: Record<string,string>` with a versioned typed policy
  containing complete transient and hard sections.
- Validate finite bounds, supported outcomes, complete class coverage, and
  version compatibility in profile CRUD.
- Normalize legacy `try_next`, `stop`, `retry_same`, and per-code maps without
  changing stored candidate order or enabled state.
- Return field-addressable API errors and normalized policy on create/update.

### Durable evaluator and scheduler

- Evaluate the effect-safety gate before policy.
- Prefer a trusted reset deadline only when it is in the future and within the
  configured maximum. Otherwise evaluate the next exponential retry or final
  outcome.
- Persist policy snapshot, class, semantic code, catalogue version, retry
  ordinal, deadline, and pending exhausted outcome before scheduling.
- Add `retry_wait`, `waiting_for_reset`, and `retrying` transitions to the
  generation-fenced route state machine.
- Reconcile scheduled work after restart only when dispatch is proven absent.
  Use deterministic clocks and `testing/synctest`; do not use sleeps.
- Coordinate shared circuits so one probe or retry owner prevents a provider
  stampede.

### Caller convergence

- Route dynamic launch, settled-turn failures, manual actions, and continuation
  through the evaluator.
- Give utility calls unique invocation identities and reject recovery after any
  partial result or ambiguous effect.
- Replace Kanban and Office provider-code allow-lists with the shared class and
  timing contract. Concrete-profile defaults can differ, but classification
  cannot.
- Keep Office scheduler wake reasons and legacy routing rows from overriding
  the shared class contract. The current Office execution-profile catalog
  excludes rich dynamic profiles without a model; concrete Office routes use
  the shared class metadata while task and Kanban dynamic routes use the full
  per-candidate policy document.

## Frontend

### Dynamic candidate editor

- Add typed transient and hard policy state to API normalization and profile
  drafts.
- Render separate, visibly explained class sections for every candidate.
- Let users enable retry, edit max retries and initial interval, enable trusted
  reset waiting, edit max wait, and choose Skip candidate or Stop after
  exhaustion.
- Show the derived exponential schedule and inline validation. Tooltips may add
  detail but cannot contain the only behavior explanation.
- Remove Add profile when the route is creating a new dynamic profile. Keep
  profile creation on the Dynamic agents list.
- Split the current editor into focused components and hooks so component and
  file limits remain within web guidance.

### Recovery presentation

- Show class, safe cause, retry ordinal, deadline, and exhausted outcome in
  task and Office route state.
- Replace generic Retry/Try next actions with generation-fenced Retry now, Skip
  now, Cancel wait, and Stop where valid.
- Keep the dynamic logical identity, continuation, immutable turn attribution,
  and capability replacement behavior unchanged.

## Data migration

- Reuse the existing candidate JSON column for a new document version unless
  repository implementation proves a column rename is required. Do not rewrite
  candidate identity or order.
- Normalize legacy documents transactionally on first profile write or through
  an idempotent migration. Invalid conflicts become actionable configuration
  errors and never authorize work.
- Add route-state columns or a bounded policy-state JSON document for durable
  schedules. Backfill existing rows to no pending schedule.
- Historical attempts keep their original code and gain class/catalogue fields
  only when known. Do not relabel old attempts using a newer catalogue.

## Public documentation

- Update agent/profile documentation with both error classes, retry schedule,
  reset-wait maximum, skip/stop outcomes, safe defaults, and unknown-error
  behavior.
- Explain that a dynamic profile behaves the same in Kanban and Office and that
  provider-message coverage grows through deterministic catalogue updates.
- Keep future model-based classification and telemetry routing clearly marked
  as unavailable.

## Test strategy

| What | File | How |
| --- | --- | --- |
| Every semantic code has one class or is explicitly outside provider policy | `apps/backend/internal/agent/runtime/routingerr/classify_test.go` and new catalogue tests | Table-driven exhaustive enum test |
| Provider signatures, timing hints, false positives, stale evidence, and redaction | `apps/backend/internal/agent/runtime/routingerr/*_test.go` | Sanitized fixture tables per adapter/provider family |
| Legacy actions normalize to complete class policies | `apps/backend/internal/agent/settings/controller/dynamic_profile_test.go` | Table-driven unit tests including conflicting per-code rules |
| Profile create/update persists and returns canonical policy | `apps/backend/internal/agent/settings/store/sqlite_dynamic_profile_test.go` and handler tests | Handler to controller to real SQLite integration test |
| `NextRetryDelay` doubles and caps safely | New shared policy `*_test.go` | Deterministic boundary table for every numeric limit |
| `Evaluate` orders effect safety, one reset wait, retry, skip, and stop | New shared policy `*_test.go` | Table-driven unit tests with injected clock |
| Deadlines and route generations survive restart and races | `apps/backend/internal/task/repository/sqlite/dynamic_route_test.go` and `apps/backend/internal/agent/runtime/dynamic/*_test.go` | Real SQLite plus `testing/synctest` integration tests |
| Kanban concrete defaults consume shared classes | `apps/backend/internal/orchestrator/*transient*_test.go` | Orchestrator integration tests with classified events |
| Utility recovery rejects partial results and isolates invocation IDs | `apps/backend/internal/utility/**/*_test.go` | Service/handler integration tests |
| Office consumes selected dynamic policy without a second allow-list | `apps/backend/internal/office/**/*routing*_test.go` | Scheduler/runtime integration tests |
| Draft normalization, limits, one-profile creation, and policy controls | `apps/web/lib/api/domains/agent-profile-normalize.test.ts` and `apps/web/components/settings/*dynamic*test.tsx` | Vitest component and state tests |
| Route snapshots and actions render current policy state | Task chat and Office recovery component tests | Vitest event/state/component integration tests |

## E2E tests

- **Scenario:** Creating a dynamic profile shows one draft, no Add profile
  action, and complete transient/hard controls; saving and reloading preserves
  them. **File:**
  `apps/web/e2e/tests/settings/dynamic-agent-profile-card.spec.ts`.
  **Verify:** field values, derived schedule, saved API state, and direct edit
  route.
- **Scenario:** The same editor works on Pixel 5. **File:**
  `apps/web/e2e/tests/settings/mobile-dynamic-agent-profile-card.spec.ts`.
  **Verify:** one column, one scroll owner, 44px controls, picker behavior, and
  zero document horizontal overflow.
- Runtime retry, reset-wait, skip, stop, restart reconciliation, and
  generation fencing are covered by the deterministic backend suites. This
  checkout has no `dynamic-routing` or Office dynamic-profile Playwright
  fixtures, so the browser release gate covers the shipped settings surface
  on Chromium and Pixel 5; no unavailable fixture is claimed as passing.

## Verification gate

- Focused Go tests pass under the `fts5` tag for classifier, policy, dynamic
  routing, orchestrator, utility, Office, and SQLite packages.
- Backend lint passes with no new complexity or timer-test violations.
- Focused web tests, typecheck, lint, `i18n:check`, and `i18n:ratchet` pass.
- Desktop and mobile Playwright policy flows pass without retries or flakes.
- Public documentation validation and `git diff --check` pass.

## Implementation waves

Wave 1:

- [x] [Task 01: Shared error catalogue](task-01-shared-error-catalogue.md)

Wave 2:

- [x] [Task 02: Versioned policy document](task-02-versioned-policy-document.md)

Wave 3:

- [x] [Task 03: Durable policy evaluator](task-03-durable-policy-evaluator.md)

Wave 4 (parallel candidates after Task 03):

- [x] [Task 04: Dynamic conductor policy integration](task-04-dynamic-conductor-policy-integration.md)
- [x] [Task 08: Dynamic policy settings UI](task-08-dynamic-policy-settings-ui.md)

Wave 5:

- [x] [Task 05: Kanban recovery convergence](task-05-kanban-recovery-convergence.md)

Wave 6:

- [x] [Task 06: Utility policy integration](task-06-utility-policy-integration.md)

Wave 7:

- [x] [Task 07: Office policy convergence](task-07-office-policy-convergence.md)

Wave 8:

- [x] [Task 09: Recovery presentation](task-09-recovery-presentation.md)

Wave 9:

- [x] [Task 10: Policy end-to-end coverage](task-10-policy-e2e.md)

Wave 10:

- [x] [Task 11: Documentation release gate](task-11-documentation-release-gate.md)

Sequential execution remains the default. Task 04 and Task 08 are the only
declared parallel pair because they own disjoint backend runtime and frontend
settings files after the policy API is fixed.

## Verification Results

Implementation complete. The package now has a versioned provider-error
catalogue, class policies, durable generation-fenced recovery, shared
Kanban/utility/Office classification, dynamic settings controls, task recovery
presentation, and public documentation. Runtime behavior is covered by the
focused backend suites; Chromium and Pixel 5 settings flows pass three repeats
each without flakes. The planned dynamic-routing and Office dynamic-profile
Playwright fixtures are not present in this checkout and are recorded as a
coverage boundary rather than reported as passing.
