---
created: 2026-11-01
status: done
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
  - REQ-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001
system_design:
  - ../../specs/agents/system-design/dynamic-circuit-backoff.md
  - ../../specs/agents/system-design/dynamic-circuit-health.md
legacy_specs: []
---

# Implementation plan: One dynamic-agent flag and default circuit policy

## Overview and handoff boundary

Remove the secondary backoff release toggle and retain the existing enabled
policy as the registry default. One vertical work order owns the runtime,
flag contracts, operator catalog, compatibility tests, and documentation.
These changes must land together because registry/profile/frontend flag sets
must agree exactly.

Planning baseline: `73761db02a` on the contributor branch of PR #4985. The user
later authorized implementation of this package on the same branch. The
completed work order records the implementation and its exact validation.

## Confirmed intent and proposed defaults

- Confirmed: retire `features.dynamicCircuitBackoffEscalation`; keep one
  `features.dynamicAgentRouting` release gate for the dynamic-agent capability.
- Verified: the registry currently has two policy branches. The disabled branch
  ignores saved counts. The product resolver already has the main execution gate.
- Proposed defaults: retain the existing enabled ladder and reset-hint policy,
  rather than choosing new durations. Retain positive circuit history even while
  the product feature is disabled. Passive restoration must not dispatch work.
- Owner: Agents owns shared resource health; the settings page and task/Office
  consumers use that contract. No new UI-owned requirement or ADR is needed.

## Scope

### In scope

- Remove the secondary flag end to end and reserve its retired key/env pair.
- Make default constructors use the existing ladder, incident/probe semantics,
  and clean-completion reset without a policy boolean.
- Preserve counts, deadlines, holds, and inert old override rows across restart.
- Keep the main flag off in all shipped profiles, with its current precedence
  and restart requirement. Preserve disabled execution behavior.
- Remove the separate settings card through the backend catalog and frontend
  feature contract. Document the bundled defaults and verify desktop/phone.

### Out of scope

- Promoting or removing `features.dynamicAgentRouting` itself.
- New flags, timing knobs, per-profile overrides, or a different ladder.
- Provider classification, resource-sharing scope, model/candidate order,
  fallback permissions, probe leases, retry budgets, and queue ownership changes.
- Account-hold release UI, readback polling, permissions, and new metrics.
- Schema rewrites, deletion of stored overrides/history, or changes to other flags.

## Technical approach

The primary [backoff design](../../specs/agents/system-design/dynamic-circuit-backoff.md)
owns the policy, retirement, persistence, and surface contract.

1. In `dynamic/circuit.go` and `circuit_backoff.go`, remove `escalation`,
   `WithCircuitBackoffEscalation`, `EscalationEnabled`, and the fixed-minute
   alternative. Preserve explicit minimum probe backoff and all ownership guards.
2. Remove the startup option in `backendapp/services.go`. Remove secondary reset
   guards in `dynamic_resolver.go` and `fixed_profile_circuit.go`; keep the main
   `Enabled()` gate on both completion-reset paths.
3. Remove the typed config field, active registration, embedded profile entry,
   and frontend default. Append the exact retired pair in `registry.go`; its
   generic config binding requires no new map or switch.
4. Replace opt-in test fixtures with default constructors. Update expectations
   that previously discarded counts or allowed unbounded inferred hints.
   Add retirement and feature-off/on evidence at their owning boundaries.
5. Update public configuration guidance and remaining main-flag risk metadata.
   The generic settings list removes the old card without a client-only filter.

No persistence migration is required. Existing zero counts remain zero; saved
positive counts and existing deadlines are not recalculated at startup.

### Consumer compatibility

| Consumer | Identity / boundary | Intended behavior | Evidence and fallback |
| --- | --- | --- | --- |
| Dynamic task, utility, and Office routes | Existing concrete candidate and route generation | Enabled routes use one default circuit policy | Engine/probe tests; existing invalid/stale ownership remains rejected |
| Fixed profiles | Captured execution resource; current-profile launch resource | Main flag on checks/charges shared circuits; off keeps legacy dispatch | Existing fixed-profile gate and queue tests; unknown identity keeps existing admission behavior |
| Restart and passive readback | Durable resource key and UTC snapshot | Restore history without launching or probing | Real-store count continuation tests; zero remains zero |
| Operator settings and boot features | Active typed runtime catalog | One remaining dynamic-agent flag; retired values have no effect | Real catalog browser cases plus config/registry/frontend contracts |

## ASCII UI preview

`UI-01: Dynamic-agent controls`, entry point `/settings/system/feature-toggles`.
This is the affected region only; unrelated feature cards remain in the list.
The composition is shared on desktop and phone.

```text
Feature Toggles
+--------------------------------------------------+
| Dynamic agent routing                      [Off] |
| Routing, shared circuits, default backoff         |
| Restart required. Environment lock when set.      |
| [Use default]                                    |
+--------------------------------------------------+
```

There is no separate backoff card. `[Off]` illustrates the shipped default; the
existing on, locked, saving, and restart-pending states still apply. Metadata
copy is illustrative and must describe the accepted defaults accurately.

Use the existing `FeatureToggleCard` and `FeatureTogglesSettings` as the nearest
settings/mobile exemplar. Keep the current settings scroll owner and phone
navigation. No new panel, drawer, breakpoint branch, or touch interaction is
needed. The phone check proves the remaining card is visible, its state and
lock match the live catalog, reload retains it, and the retired card is absent.
ASCII spacing is not a pixel requirement. Covers `.24` through `.26`.

## Tests and traceability

| Criteria | Primary evidence |
| --- | --- |
| `AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.20` and health `.3` | Extend `dynamic/circuit_backoff_contract_test.go`: `TestCircuitFailureLadderAndSameIncident`, `TestCircuitResetHintDefaults`, and owned-probe coverage; use `NewCircuitRegistry()` without an opt-in option |
| Routing `.21` | Update `sqlite/dynamic_route_test.go`: `TestCircuitFailureCountSurvivesRealStoreRestore`; add a PostgreSQL variant using the existing isolated DB fixture; verify closed-count and zero-count restoration too |
| Routing `.24` and health `.1` through `.3` | Extend `runtime/fixed_profile_circuit_test.go`: `TestFixedProfileCircuitUsesDynamicRoutingFlag`; cover both reset gates and verify a default `NewEngine()` uses the ladder |
| Routing `.25` | New `runtimeflags/dynamic_circuit_backoff_retirement_test.go`: `TestDynamicCircuitBackoffFlagIsRetiredAndStaleValuesAreInert`; reuse `newTestStore` to preserve old true/false SQL overrides while excluding them from active states |
| Routing `.24` through `.26` | Existing registry/profile/config and `features-contract.test.ts` equality checks; main flag defaults and source precedence remain unchanged |

Keep meaningful probe, hold, attribution, completion, queue-resume, and
provider-classification coverage. Remove assertions for the former optional
mode, not the behaviors they protected. No new test-only production seam is
needed.

## E2E tests

The implementation adds `settings/dynamic-agent-single-flag.spec.ts`
(`chromium`) and `settings/mobile-dynamic-agent-single-flag.spec.ts`
(`mobile-chrome`). They share only the catalog scenario helper.

Use the real runtime-flags GET and boot feature payload. The existing
`feature-toggles-restart` specs stub their catalog and cannot prove retirement.
Exercise main flag off + old env true, and main flag on + old env false with
`backend.useEnv`. Verify exactly one remaining dynamic-agent card, no retired
card, the expected effective state/environment lock, and persistence on reload.
Use `waitForHttp` for actual client reads; initial Go boot hydration may avoid
an HTTP request. Backend tests own timing and SQL history; no live-provider
failure or fake-clock browser endpoint is required. Covers `.24` through `.26`.

## Work orders

- [x] [Task 01: Retire the secondary flag and establish defaults](task-01-single-flag-defaults.md)

Wave 1 contains one sequential work order with no prerequisite work orders.
The completed source packages for
[backoff escalation](../dynamic-circuit-backoff-escalation/plan.md) and
[shared admission/readback](../dynamic-circuit-health/plan.md) are history at the
baseline, not pending dependencies. Their recorded tests do not prove this change.

## Planning-package validation

These checks validate documentation only. They do not implement the work order
or count as product-test evidence.

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans
git status --short -- docs/plans/dynamic-circuit-defaults
```

At planning time, the PR documentation preflight used the proposed runtime
change and complete linked document set:

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const documents = [
  'docs/plans/dynamic-circuit-defaults/plan.md',
  'docs/plans/dynamic-circuit-defaults/task-01-single-flag-defaults.md',
  'docs/specs/agents/requirements/dynamic-agent-routing.md',
  'docs/specs/agents/requirements/dynamic-circuit-health.md',
  'docs/specs/agents/system-design/dynamic-circuit-backoff.md',
  'docs/specs/agents/system-design/dynamic-circuit-health.md',
];
const fileContents = Object.fromEntries(documents.map(path => [path, fs.readFileSync(path, 'utf8')]));
const changedFiles = [...documents, 'apps/backend/internal/agent/runtime/dynamic/circuit_backoff.go'];
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ status: result.status, errors: result.errors }));
if (!result.ok) process.exit(1);
JS
```

This records the planning baseline only. The implementation preflight used the
complete changed-file set and is recorded in the work order.

## Verification results

Planning-package checks passed before implementation: document catalog
validation, 36 specification-linter tests, full specification lint,
documentation-coverage preflight (`covered`, no errors), and the documentation
diff check.

Implementation is complete on PR #4985. The full verification record, including
the PostgreSQL skip, first-run E2E correction, and final desktop/mobile results,
is in [Task 01](task-01-single-flag-defaults.md). Product changes passed scoped
Go race tests and lint, frontend tests/lint/typecheck/i18n, both browser
projects, public documentation checks, spec checks, and the complete changed-file
documentation-coverage preflight.

## Risks

- Installations with dynamic agents already enabled move from fixed-minute
  retrying to the ladder without a second opt-in. The remaining flag can disable
  execution after restart; it is not a circuit-history reset.
- Explicit provider reset times can exceed four hours. Holds still require
  owner release. Removing a flag does not add an operator override for either.
- Removing an escalation guard must not remove the product or ownership guard.
- Old disabled-policy writes may already have stored zero. Do not infer lost
  failure history or shorten an existing deadline to compensate.
- A stale environment value or saved override must not recreate a second mode.
