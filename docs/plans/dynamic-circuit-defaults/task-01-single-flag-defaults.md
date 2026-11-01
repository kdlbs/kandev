---
id: "01-single-flag-defaults"
title: "Retire the secondary flag and establish circuit defaults"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
  - REQ-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.20
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.21
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.24
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.25
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.26
  - AC-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001.1
  - AC-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001.2
  - AC-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001.3
system_design:
  - ../../specs/agents/system-design/dynamic-circuit-backoff.md
  - ../../specs/agents/system-design/dynamic-circuit-health.md
---

# Task 01: Retire the secondary flag and establish circuit defaults

## Summary

Deliver the single-flag policy described by the
[plan](plan.md) and [backoff design](../../specs/agents/system-design/dynamic-circuit-backoff.md).
Remove the subordinate runtime flag and make the existing enabled backoff policy
work through default constructors. Keep the existing dynamic-agent gate and
circuit/run/session ownership guards.

## In scope

- All runtime, config, registry, embedded-profile, frontend-contract, and
  startup-wiring changes required for removing the secondary flag atomically.
- Always-on registry counting, hint bounds, probe failure progression, and
  restoration; product-gated completion resets remain intact.
- Retired key/env reservation, old true/false value tests, retained override
  rows, and existing main-flag defaults/precedence/restart behavior.
- Permanent timing, engine-default, reset, persistence, and gate regression
  coverage. Desktop and phone must consume the real catalog.
- Current specs, public configuration and flag risk copy, and brief historical
  notices on the completed source packages. Preserve their old delivery results.

## Out of scope

New flags or timing options, new durations, dynamic-agent promotion/graduation,
provider classifications, account-hold release UI, readback polling, resource
identity changes, new workers, replay ownership changes, and destructive DB work.
Do not alter unrelated flags or the unclassified-fallback policy.

## Acceptance

1. Default registry and engine paths satisfy routing `.20`/`.21` and health `.3`;
   there is no internal optional escalation mode or stale count-discard branch.
   Existing leases, holds, provider hints, and captured execution guards remain.
2. Routing `.24`/`.25` is enforced at production boundaries: the remaining gate
   still protects disabled execution, the old identity is retired, and stale
   true/false environment/SQL values are inert without deleting stored data.
3. Routing `.26` matches `UI-01` on desktop and phone. Runtime/boot feature
   contracts expose no retired field, and docs describe one flag and default
   policy. Required targeted checks pass with their actual evidence recorded.

## TDD implementation sequence

- First prove that a default constructor currently fails the full ladder and
  retained-count expectations. Remove the policy option and legacy branches,
  then update callers and preserve existing meaningful probe/reset tests.
- Add retirement regressions against the real SQL flag store and true/false
  old environment settings. Remove the active field and registration across
  all layers together, and append the retired identity pair.
- Exercise both values of `features.dynamicAgentRouting`, including completion
  resets. Reuse existing queue/resume and captured-identity tests; do not add a
  replacement boolean in registry construction or a per-profile setting.
- Add live-catalog desktop/phone cases before completing the operator contract.
  Do not stub `/api/v1/runtime-flags` to manufacture the missing card.

## ASCII UI preview

`UI-01: Dynamic-agent controls` is the same shared-composition region as the
[full preview](plan.md#ascii-ui-preview). Entry: `/settings/system/feature-toggles`.

```text
Dynamic agent routing                         [Off]
Routing, shared circuits, default backoff
Restart required. Environment lock when set.
[Use default]
```

The separate backoff card is absent. Other flags keep their existing cards.
Phone navigation, scroll ownership, touch behavior, save/reset, loading/error,
and restart handling remain with `FeatureTogglesSettings` / `FeatureToggleCard`.
The real-catalog mobile case verifies the same effective and locked state after
reload. No phone-specific policy is introduced. Covers routing `.24` to `.26`.

## Verification

Run from the repository root. Install workspace dependencies from `apps/`
first only if using a fresh worktree.
Run lint and Go race/build checks sequentially. Use the repository's bounded
runners; do not overlap full test suites.

```bash
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/agent/runtime/dynamic ./internal/agent/runtime ./internal/runtimeflags ./internal/common/config ./internal/profiles)
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/task/repository/sqlite -run 'Test(CircuitFailureCountSurvivesRealStoreRestore|PostgresCircuitDefaultsRestore)')
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/orchestrator -run '^TestFixedProfile')
bash scripts/lint-go-changed \
  apps/backend/internal/agent/runtime/dynamic/circuit.go \
  apps/backend/internal/agent/runtime/dynamic/circuit_backoff.go \
  apps/backend/internal/agent/runtime/dynamic/circuit_backoff_contract_test.go \
  apps/backend/internal/agent/runtime/dynamic/engine_test.go \
  apps/backend/internal/agent/runtime/dynamic_resolver.go \
  apps/backend/internal/agent/runtime/dynamic_resolver_test.go \
  apps/backend/internal/agent/runtime/fixed_profile_circuit.go \
  apps/backend/internal/agent/runtime/fixed_profile_circuit_test.go \
  apps/backend/internal/backendapp/services.go \
  apps/backend/internal/common/config/config.go \
  apps/backend/internal/runtimeflags/registry.go \
  apps/backend/internal/runtimeflags/dynamic_circuit_backoff_retirement_test.go \
  apps/backend/internal/task/repository/sqlite/dynamic_route_test.go \
  apps/backend/internal/task/repository/sqlite/dynamic_route_postgres_test.go
(cd apps && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts lib/control-ui/routing-circuits.test.ts hooks/domains/coordinator/use-control-circuits.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project chromium e2e/tests/settings/dynamic-agent-single-flag.spec.ts
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --no-build --project mobile-chrome e2e/tests/settings/mobile-dynamic-agent-single-flag.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Both browser files ran and passed: two Chromium scenarios and two mobile-Chrome
scenarios. The mobile run reused the build from the immediately preceding
Chromium run; no production source changed between those runs. The PostgreSQL
variant ran and was skipped because `KANDEV_TEST_POSTGRES_DSN` was not set; the
SQLite restore test passed. The implementation documentation-coverage
preflight used every changed path and all eleven changed documents, and returned
`covered` with no errors.

## Files likely touched

Production:

- `apps/backend/internal/agent/runtime/dynamic/circuit.go`
- `apps/backend/internal/agent/runtime/dynamic/circuit_backoff.go`
- `apps/backend/internal/agent/runtime/dynamic_resolver.go`
- `apps/backend/internal/agent/runtime/fixed_profile_circuit.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/runtimeflags/registry.go`
- `apps/backend/internal/profiles/profiles.yaml` (root `profiles.yaml` symlink)
- `apps/web/lib/state/slices/features/types.ts`

Tests and contracts:

- `apps/backend/internal/agent/runtime/dynamic/circuit_backoff_contract_test.go`
- `apps/backend/internal/agent/runtime/dynamic/circuit_test.go`
- `apps/backend/internal/agent/runtime/dynamic/engine_test.go`
- `apps/backend/internal/agent/runtime/fixed_profile_circuit_test.go`
- `apps/backend/internal/task/repository/sqlite/dynamic_route_test.go`
- `apps/backend/internal/task/repository/sqlite/dynamic_route_postgres_test.go` (new)
- `apps/backend/internal/runtimeflags/dynamic_circuit_backoff_retirement_test.go` (new)
- Existing registry, profile, config, and frontend feature-contract tests
- `apps/web/e2e/tests/settings/dynamic-agent-single-flag.spec.ts` (new)
- `apps/web/e2e/tests/settings/mobile-dynamic-agent-single-flag.spec.ts` (new)
- `apps/web/e2e/tests/settings/dynamic-agent-single-flag-helpers.ts` (new)

Documentation:

- `docs/public/configuration.md`
- The requirement/design files linked by this work order
- `docs/specs/agents/system-design/dynamic-agent-routing-01.md`
- This plan/work order and historical source-package notices

Do not change generic settings rendering solely to hide the retired flag.
Backend catalog removal is authoritative. Include any additional changed Go
test files in the affected-package lint command if the implementation finds them.

## Dependencies

None. Start from the current PR branch and preserve its shared-circuit fixes.
The future dynamic-agent graduation package is explicitly outside this task.

## Risks

- Dynamic-enabled installs receive longer waits by default after upgrade.
- Removing a reset-policy guard can accidentally remove the execution gate.
- Stale false values can reintroduce a hidden fixed-minute mode unless tested.
- Old zero counts cannot reconstruct history; existing deadlines must survive.

## Parallelism

`sequential`

## Inputs

- [Routing requirements](../../specs/agents/requirements/dynamic-agent-routing.md)
- [Circuit-health requirements](../../specs/agents/requirements/dynamic-circuit-health.md)
- [Backoff design](../../specs/agents/system-design/dynamic-circuit-backoff.md)
- [Shared circuit design](../../specs/agents/system-design/dynamic-circuit-health.md)
- Existing retired-flag tests in `runtimeflags/office_session_identity_graduation_test.go`
- Existing real SQL flag store in `runtimeflags/store_test.go`
- Existing settings card and restart specs; their stubbed catalog is not retirement proof

## Results

Implemented on the existing PR #4985 branch. Default circuit construction now
uses the retained ladder; the secondary runtime identity is retired and old
environment and SQL values are inert. The main dynamic-agent flag still gates
execution and clean-completion resets. Desktop and mobile settings read the live
catalog and boot payload, and both real-catalog browser projects pass.

Validation passed:

- Runtime, runtime-flag, config, and profile Go packages passed with `-race`.
- The SQLite restoration test passed with `-race`; the PostgreSQL variant was
  skipped because `KANDEV_TEST_POSTGRES_DSN` is not configured.
- Fixed-profile orchestrator tests passed with `-race`.
- Scoped frontend feature-contract tests, web lint, typecheck, and i18n checks
  passed.
- The two Chromium and two mobile-Chrome single-flag scenarios passed.
- Document catalog validation, full specification lint, public-doc validation,
  and PR documentation-coverage preflight passed.
- `git diff --check`, scoped Go lint, and commit hooks passed.

The first Chromium run caught an incorrect boot-payload assertion path in the
new test helper. The helper now reads `initialState.features` from the Go boot
payload; the rerun passed. Full repository tests and a PostgreSQL-backed run
were not performed. The implementation is committed and pushed to PR #4985;
the requested post-push PR fixup is recorded in the task checklist.
