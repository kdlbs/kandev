---
spec: docs/specs/feature-toggles/spec.md
created: 2026-08-01
status: completed
---

# Implementation Plan: Release Feature Toggles

## Overview

This plan simplifies the existing startup runtime-flag plumbing without
changing its API, persistence, precedence, restart, or settings-page behavior.
Backend config bindings move into the runtime-flag registry, frontend names and
all-false defaults gain one declaration, and generic tests prevent incomplete
flag wiring. Public and contributor documentation then defines the fail-closed
release-toggle lifecycle that follow-on feature PRs, beginning with #2104, use.

---

## Backend

### Definition-owned config bindings

- In `apps/backend/internal/runtimeflags/registry.go`, replace the metadata-only
  `definitions` slice with an internal registration type containing:
  - the public `RuntimeFlagDefinition` metadata;
  - a typed `read(*config.Config) bool` function;
  - a typed `apply(*config.Config, bool)` function.
- Keep `RuntimeFlagDefinition`, `RuntimeFlagState`, and the HTTP response shape
  unchanged. `Definitions()` and `DefinitionByKey()` continue to expose only
  metadata.
- Inline each feature's registry key, environment variable, config getter, and
  config setter in one registration. Keep Debug mode's subordinate environment
  behavior in a named helper because it intentionally updates
  `KANDEV_DEBUG_PPROF_ENABLED` and `KANDEV_DEBUG_AGENT_MESSAGES` together.
- In `apps/backend/internal/runtimeflags/config.go`, make
  `OptionsFromConfig`, `ValuesFromConfig`, and `ApplyStatesToConfig` iterate the
  registrations. Delete the per-feature key/env constants, environment map,
  value map, and apply switch. Retain only constants needed for Debug mode's
  implied environment variables.

### Completeness invariants

- Add a table/reflective test in
  `apps/backend/internal/runtimeflags/registry_test.go` that compares every
  boolean field in `config.FeaturesConfig` with:
  - one `features.<json-tag>` runtime registration;
  - its `KANDEV_FEATURES_<MAPSTRUCTURE_TAG>` profile entry;
  - a getter/setter round trip that changes only that field.
- Require exact equality in both directions, validate every registration's
  metadata and binding isolation, and reject active key/environment collisions
  with the append-only retired-identity set.
- Replace the hardcoded serialized object equality in
  `apps/backend/internal/common/config/config_test.go` with a generic assertion
  that every `FeaturesConfig` field is boolean and has non-empty, non-Go-name
  `json` and `mapstructure` tags. Feature-specific behavior tests remain where
  their semantics matter.
- Preserve unknown stored overrides: `Service.ListStates` continues to iterate
  registrations rather than store rows, so removed keys remain inert and
  hidden.

No database schema, HTTP route, JSON response, restart protocol, or precedence
change is required.

---

## Frontend

### Single feature declaration

- In `apps/web/lib/state/slices/features/types.ts`, declare the all-false
  feature object once and derive `FeatureName` and `FeatureFlags` from its keys.
- In `apps/web/lib/state/slices/features/features-slice.ts`, initialize the
  Zustand slice from that declaration instead of repeating every key.
- Keep `getFeatureFlagsAction()` type-driven normalization and `useFeature()`
  behavior unchanged: malformed, missing, or unreachable values remain false.
- Update `features-slice.test.ts` and `app/actions/features.test.ts` so tests
  prove all declared defaults are false and missing/non-boolean backend values
  fail closed without repeating the complete feature list in test fixtures.
- Add a repository contract test that extracts the backend `FeaturesConfig`
  JSON tags and requires exact equality with `defaultFeatureFlags` keys.

This changes state/type normalization only. It does not change layout,
navigation, touch behavior, or the existing Feature Toggles cards. The closest
mobile surface remains the shipped responsive System settings page; no new
mobile composition or Playwright scenario is required for this task.

---

## Tests

- **What:** every typed backend feature has exactly one registry/profile
  binding and its getter/setter round-trips.
  - **File:** `apps/backend/internal/runtimeflags/registry_test.go`
  - **How:** reflect over `config.FeaturesConfig`, compare tags/default keys,
    apply true/false on a fresh config, and assert unrelated fields do not
    change.
- **What:** generic runtime resolution preserves environment lock, persisted
  override, profile default, pending-restart, and Debug implied-env behavior.
  - **Files:** `apps/backend/internal/runtimeflags/config_test.go`,
    `service_test.go`, and existing handler/store tests.
  - **How:** run the existing focused package tests after making them consume
    registrations.
- **What:** the public feature JSON contract remains tagged and boolean.
  - **File:** `apps/backend/internal/common/config/config_test.go`
  - **How:** reflective tag validation plus JSON marshal/unmarshal coverage.
- **What:** frontend feature names have one all-false declaration and bad or
  absent backend values fail closed, while backend and frontend key sets remain
  exactly equal.
  - **Files:** `apps/web/lib/state/slices/features/features-slice.test.ts`,
    `apps/web/app/actions/features.test.ts`, and
    `apps/web/lib/state/slices/features/features-contract.test.ts`
  - **How:** focused Vitest tests derive fixtures from the declaration and read
    the backend `FeaturesConfig` contract directly.

## E2E Tests

No production UI interaction changes in this foundation refactor. Existing
component tests cover registry-rendered cards and state transitions; #2104's
follow-on flag integration retains its dedicated desktop and mobile task-title
Playwright coverage with the flag explicitly enabled. Mobile parity is
satisfied here by preserving the existing responsive Feature Toggles page and
changing only state/type normalization.

---

## Risks

- Registry config functions must remain internal so the public JSON metadata
  contract does not acquire implementation fields.
- Debug mode has intentional environment side effects beyond its typed config
  value; generic registration must preserve them exactly.
- A reflective completeness test must compare `json` names, `mapstructure`
  names, and `KANDEV_FEATURES_*` names without making production config loading
  reflective.
- PR #2104 cannot claim full flag isolation for code that still refactors the
  shared task-creation path unconditionally.

---

## Documentation

- Amend `docs/decisions/0007-runtime-feature-flags.md` to reference the new
  release-toggle ADR, current `backendapp` paths, definition-owned bindings,
  default-off lifecycle, and actual add/remove steps.
- Update `docs/public/extending-kandev.md` with the authoritative-gate checklist
  and staged rollout/removal procedure.
- Repair the runtime-toggle inventory in `docs/public/configuration.md`, which
  currently contains duplicated and graduated plugin entries.
- Update root `AGENTS.md` so future agents use the streamlined declaration and
  fail-closed entry-path rules.
- Remove the orphaned
  `apps/web/e2e/tests/settings/feature-toggles-helpers.ts`, which still refers to
  the graduated `features.unreadDivider` flag and has no callers.

---

## Follow-on Pull Request #2104

After this foundation lands on `main`, rebase PR #2104 and update its existing
agent-generated-title spec, ADR, plan, and tasks on that branch:

1. Add `features.agentGeneratedTaskTitles` with
   `KANDEV_FEATURES_AGENT_GENERATED_TASK_TITLES=false` in all profiles and a
   high-risk experimental registry definition.
2. Remove the temporary rollout concern from portable user settings: delete the
   `agent_generated_task_titles` user-setting DTO/store/boot/state plumbing and
   its General settings card unless product requirements separately retain a
   permanent user preference.
3. Gate task/subtask creation UI with
   `useFeature("agentGeneratedTaskTitles")`, keeping legacy title-required
   behavior while off.
4. Reject `auto_title` in the task service while the install-wide feature is
   off, before deriving a title or writing pending metadata.
5. Require the flag as well as pending-title state before injecting the system
   prompt or exposing `set_task_title_kandev`, including every launch, resume,
   queued-message, and workflow entry path. Disabling the flag leaves existing
   provisional titles readable and inert.
6. Run backend on/off tests plus the existing desktop and mobile Playwright
   specs with the flag explicitly enabled. Add a legacy/off regression proving
   ordinary title creation remains unchanged.
7. Keep the dialog extraction separate from the rollout claim: either split the
   unconditional refactor or retain explicit regression evidence for the
   feature-off path.

PR #2104 remains a separate dependent PR so the flag-system change can merge
and be released independently.

---

## Verification Results

- Backend-focused tests: passed (91 tests across runtimeflags, common/config,
  and profiles).
- Backend lint: passed with 0 issues.
- Frontend-focused Vitest: passed (2 files, 8 tests).
- Frontend typecheck and lint: passed.
- Public docs validation: passed (58 tests; 41 published pages).
- `git diff --check`: passed.

### Review remediation (2026-08-02)

- Red phase: focused runtimeflags tests failed because the retired-identity set
  did not exist.
- `go test ./internal/runtimeflags ./internal/profiles` from `apps/backend`
  passed (37 tests across 2 packages); the full backend suite passed, and
  backend lint reported 0 issues.
- The new backend/frontend contract test passed; the full web Vitest suite,
  web typecheck, and web lint passed.
- Public docs validation passed (58 tests; 41 published pages), and
  `git diff --check` passed.

---

## Implementation Waves And Parallel Candidates

Wave 1 (parallel candidates — user authorization required):

- [x] [task-01-centralize-backend-bindings](task-01-centralize-backend-bindings.md)
- [x] [task-02-consolidate-frontend-contract](task-02-consolidate-frontend-contract.md)

Wave 2:

- [x] [task-03-document-release-toggle-workflow](task-03-document-release-toggle-workflow.md)

The default execution order is sequential in the primary conversation. These
wave labels do not authorize subagents.
