---
spec: docs/specs/agents/requirements/utility-agent-profiles.md
created: 2026-08-12
status: complete
---

# Implementation Plan: Default Utility Action Profiles

## Overview

Repair profile-backed utility actions so an inherited built-in action without a concrete override
uses the selected global default. The backend will normalize legacy migration-state rows, preserve
ambiguous empty `unconfigured` rows for fail-closed repair, and preserve stale explicit IDs. The
frontend will render inherited actions using the default label, with focused unit and desktop/mobile
E2E coverage.

The behavioral amendment is recorded in
[ADR-2026-08-12-built-in-utility-default-inheritance](../../decisions/2026-08-12-built-in-utility-default-inheritance.md).

---

## Backend

### Binding normalization and dependency cleanup

- `apps/backend/internal/utility/models/models.go`: add the shared predicate for a built-in action
  that has no concrete profile and therefore uses the default.
- `apps/backend/internal/utility/service/service.go`: normalize legacy migration-state built-in rows
  with no concrete profile to `inherit`, preserve empty `unconfigured` rows, resolve persisted
  inherited bindings through the global default, and retain stale explicit profile IDs when a profile
  is deleted.
- `apps/backend/internal/backendapp/orchestrator.go`: keep inherited built-in rows inherited when
  the deleted profile was the global default, and include effective inherited rows in dependency
  lookup.
- `apps/backend/internal/backendapp/services.go`: use the same effective-default predicate for
  plugin utility-agent reads.

### Backend tests

- `apps/backend/internal/utility/service/service_test.go`: cover ambiguous and unmatched built-in
  migration, preservation of empty unconfigured rows, inherited execution, missing default failure,
  custom repair state, and stale explicit ID preservation.
- Add or update the nearest backend adapter tests if the orchestrator/plugin dependency behavior is
  not covered by service tests.

---

## Frontend

### Utility Agents settings page

- `apps/web/components/settings/utility-sections.tsx`: display the selected default only for an
  inherited built-in, and keep both empty and concrete unconfigured bindings in the unavailable
  repair state.
- Add a small pure selection helper if needed so the empty-binding behavior is tested without a
  React component test.

### Tests and responsive behavior

- `apps/web/components/settings/utility-sections.test.tsx` or the nearest existing utility settings
  test: prove empty built-in bindings select the default and concrete stale bindings remain
  unavailable.
- `apps/web/e2e/tests/settings/utility-agents.spec.ts`: add a regression scenario for an inherited
  built-in with an empty profile ID and a valid default, asserting the default label is rendered and
  the repair copy is absent.
- `apps/web/e2e/tests/settings/mobile-utility-agents.spec.ts`: retain the existing narrow-viewport
  reachability coverage and assert the inherited default remains visible in the same phone layout.
  No new mobile composition is needed because this is state normalization inside the existing
  responsive surface.

The mobile design contract is unchanged: the existing stacked action row is the phone composition,
the profile picker remains the primary control, the card remains the single page scroll owner, and
the picker keeps its existing viewport-contained Radix surface and touch target.

---

## Tests

- **What:** Built-in legacy migration-state rows with zero or multiple eligible profile matches
  inherit the global default. Empty unconfigured rows remain repair-gated, as do custom rows.
  **File:** `apps/backend/internal/utility/service/service_test.go`.
  **How:** table-driven service tests with a fake profile resolver and repository.
- **What:** Inherited built-ins resolve the selected default, while no default and stale explicit
  bindings fail closed.
  **File:** `apps/backend/internal/utility/service/service_test.go`.
  **How:** focused `PreparePromptRequest` tests asserting profile ID and error behavior.
- **What:** Inherited built-in UI bindings render the default; empty and concrete unconfigured
  bindings render repair state.
  **File:** `apps/web/components/settings/utility-sections.test.tsx`.
  **How:** pure helper tests for the selection value and unavailable value.
- **What:** The settings page preserves the default profile on desktop and phone viewports.
  **Files:** `apps/web/e2e/tests/settings/utility-agents.spec.ts` and
  `apps/web/e2e/tests/settings/mobile-utility-agents.spec.ts`.
  **How:** Playwright fixtures with mocked utility-agent responses and the seeded eligible profile.

---

## E2E Tests

- **Scenario:** GIVEN a valid default profile and a built-in action with an `inherit` binding and an
  empty profile ID, WHEN the Utility Agents page loads, THEN the action selector shows the default
  profile and does not show unavailable repair copy.
- **Scenario:** GIVEN a built-in action with an empty `unconfigured` binding from an older release,
  WHEN the page loads, THEN the action remains unavailable until the user repairs it.
  **Files:** desktop and mobile utility-agent settings specs.
- **Scenario:** GIVEN a built-in action with a stale concrete profile ID, WHEN the page loads, THEN
  the stale action remains visible as unavailable and does not silently switch profiles.
  **File:** desktop utility-agent settings spec.

---

## Verification Results

Implementation and verification are complete.

- Backend migration, execution, and stale-binding checks passed: 17 utility/profilebinding tests,
  including 13 focused migration/request/clear tests and 3 plugin adapter tests.
- Frontend unit coverage passed: 2 files and 6 tests.
- Web typecheck, i18n check, and i18n ratchet passed. The i18n check reported existing advisory
  real-locale catalog parity warnings.
- Desktop managed E2E passed: 9 tests in `utility-agents.spec.ts`.
- Mobile managed E2E passed: 1 test in `mobile-utility-agents.spec.ts`.

---

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-backend-default-inheritance](task-01-backend-default-inheritance.md)

Wave 2:

- [x] [task-02-settings-default-inheritance](task-02-settings-default-inheritance.md)

The tasks are sequential because the frontend regression contract depends on the backend binding
semantics. No subagent delegation is authorized by this plan.

## Open Questions

None.
