---
spec: docs/specs/agents/requirements/utility-agent-profiles.md
created: 2026-08-12
status: complete
---

# Implementation Plan: Repair Unavailable Utility Actions

## Overview

The live database contains all eight built-in utility actions as empty `unconfigured` rows. A valid
default profile exists. The settings picker translates its Default option to an empty callback
value. The draft code treats only `__USE_DEFAULT__` as inherited and submits an invalid empty
explicit binding. The repair first normalizes legacy empty built-ins to `inherit`. Then it fixes the
shared desktop and mobile save path and proves persistence through the real backend.

---

## Backend

### Legacy binding normalization

- `apps/backend/internal/utility/service/service.go`: update `MigrateLegacyBindings` so a built-in
  with `profile_binding_state = 'unconfigured'` and an empty `agent_profile_id` is persisted as
  `inherit` through a conditional repository update. Preserve non-empty stale IDs and every custom
  `unconfigured` row, including when a concurrent settings save changes the stale row first.
- Keep the operation idempotent and preserve the existing `enabled` value. This repair changes only
  binding ownership. It does not silently enable an action or copy a concrete profile ID.

## Frontend

### Default action selection

- `apps/web/components/settings/utility-agents-section.tsx`: extract the built-in profile draft
  transition into a small pure helper. Treat the profile picker's empty fallback callback as Default,
  producing an empty profile ID, `inherit`, and `enabled: true`. Keep concrete values explicit.
- Do not change `UtilityAgentProfilePicker`'s global callback contract because the default-profile
  card intentionally uses the empty value to mean no saved default.

### Mobile design contract

The existing stacked action row remains the desktop and phone entry point. The picker, save bar,
single page scroll owner, touch targets, and responsive composition do not change. The state
transition is shared across viewports. The existing mobile action-row spec is the nearest shipped
mobile surface and will be extended to prove that choosing Default by touch submits an inherited
binding.

## Tests

- **What:** Empty unavailable built-ins normalize to inherited state, while concrete stale and custom
  rows remain unconfigured.
  **File:** `apps/backend/internal/utility/service/service_test.go`.
  **How:** Replace the preservation regression with table-driven service migration tests that assert
  repository updates and idempotent second-run behavior.
- **What:** The fallback value from the shared picker creates a valid inherited draft, and a concrete
  profile creates an explicit draft.
  **File:** `apps/web/components/settings/utility-agents-section.test.ts`.
  **How:** Pure helper tests, including the exact empty-string callback that caused the failure.
- **What:** Selecting Default traverses the handler, service, and SQLite repository without an empty
  explicit binding.
  **File:** `apps/web/e2e/tests/settings/utility-agents.spec.ts`.
  **How:** Serve one unavailable GET state, continue the PATCH to the real isolated backend, wait for
  Saved, reload from the real backend, and assert the inherited default remains selected.

## E2E Tests

- **Scenario:** GIVEN an empty unavailable built-in action and a valid global default, WHEN the user
  selects Default and saves, THEN the real API save succeeds and reload shows the inherited default.
  **File:** `apps/web/e2e/tests/settings/utility-agents.spec.ts`.
- **Scenario:** GIVEN the same action on a Pixel 5 viewport, WHEN the user selects Default with the
  touch picker and saves, THEN the outgoing binding is `inherit` and all controls remain reachable.
  **File:** `apps/web/e2e/tests/settings/mobile-utility-agents.spec.ts`.

## Verification Results

- Backend utility and backend-app suites: `cd apps/backend && go test ./internal/utility/... ./internal/backendapp/...`
  passed 378 tests; `go vet ./internal/utility/...` passed. The SQLite stale-predicate regression is
  covered by the utility store migration suite.
- Frontend utility suites: `cd apps/web && pnpm exec vitest run
  components/settings/utility-agents-section.test.ts
  components/settings/utility-sections.test.ts` passed 8 tests.
- Frontend type check: `cd apps/web && pnpm run typecheck` passed.
- Internationalization checks: `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet`
  passed. Existing translation parity notices remained advisory.
- Desktop E2E: the unavailable action saved through the real backend, reloaded as `inherit`, and
  restored its isolated fixture baseline.
- Mobile E2E: both Utility Agents scenarios passed. The touch flow sent the inherited binding and
  the actions card had no horizontal overflow.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-normalize-empty-bindings](task-01-normalize-empty-bindings.md)

Wave 2:

- [x] [task-02-save-default-selection](task-02-save-default-selection.md)

The tasks are sequential because the UI persistence scenario verifies the backend binding contract.
No subagent delegation is authorized by this plan.
