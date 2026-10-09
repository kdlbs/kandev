---
created: 2026-09-27
status: implemented
requirements:
  - REQ-EXECUTORS-PLUGIN-001
  - REQ-EXECUTORS-PLUGIN-004
  - REQ-EXECUTORS-PLUGIN-005
  - REQ-EXECUTORS-PLUGIN-007
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
legacy_specs: []
---

# Implementation plan: Graduate remote executor plugin flag

## Overview

Make remote executor provider support available whenever an eligible plugin is installed. Retire the
temporary release toggle from runtime configuration and preserve the provider, profile, resource, and
plugin lifecycle checks already present. First remove the flag and prove backend admission; then update
browser coverage and operator documentation for the permanent behavior.

PR [#3985](https://github.com/kdlbs/kandev/pull/3985) introduced the feature with the toggle disabled in
all shipped profiles. Its implemented [plan](../remote-executor-plugins/plan.md) remains the historical
delivery record. This package changes the same executor requirement and design pair; it does not
replace the original provider contract.

## Scope

### In scope

- Remove `features.remoteExecutorPlugins` and `KANDEV_FEATURES_REMOTE_EXECUTOR_PLUGINS` from live
  backend, profile, and frontend configuration; retire both identities in the append-only registry.
- Make manifest-backed discovery, profile administration, dispatch, recovery, and cleanup independent
  of the release toggle while keeping their existing provider and resource safety checks.
- Keep installed-but-disabled or unavailable providers visible for saved selections without admitting
  a new launch. Keep built-in executors unchanged when there is no provider plugin.
- Remove flag setup and disabled-flag assertions from tests; test ordinary no-plugin and unavailable-
  plugin behavior. Update the three public executor/plugin pages and the rollout wording in the ADR.

### Out of scope

- A production cloud provider, automatic plugin installation, or changes to built-in executor types.
- Removing plugin lifecycle guards, retained inventory, provider authentication, or exact-resource
  recovery checks.
- Deleting old `runtime_flag_overrides` rows or reusing either retired identity.

## Technical approach

`internal/plugins/executor_providers.go` already enumerates `Service.List()` plugin declarations and
requires an active plugin, declared provider, compatible contract, and running process for dispatch.
`internal/task/service/executor_provider_catalog.go` creates only provider-backed executor rows and
retains unavailable saved entries. Remove only the global enabled boolean and its `feature_disabled`
branch; preserve these admission and inventory boundaries. The service constructor will no longer
copy a runtime flag into the plugin service.

Remove the field from `common/config/config.go`, both copies of `profiles.yaml`, and the active
registration in `runtimeflags/registry.go`. Append the exact key and environment variable to
`retiredRuntimeFlagIdentities`; the existing collision and completeness tests must keep passing.
Remove the `remoteExecutorPlugins` frontend default and its flag-specific contract test. Audit
HTTP, WebSocket, MCP, startup, and background paths for any remaining flag reference, including tests.

| Provider or path | Transport and identity | Expected result | Evidence / fallback |
| --- | --- | --- | --- |
| No installed provider plugin | Built-in executor paths only | No plugin remote card or new profile; built-ins work | Backend catalog and browser empty-state checks |
| Installed active fixture provider | Managed plugin gRPC; manifest `plugin:<id>:<key>` identity | Profile, launch, attachment, and cleanup work without flag setup | Fixture Go tests and desktop/phone Playwright flows |
| Installed disabled, incompatible, or stopped provider | Existing manifest identity and saved profile | Saved selection visible; new launch rejected without executor fallback | Admission and plugin lifecycle tests |
| Retained resource with unavailable provider | Durable `executors_running` identity | Inventory and uninstall guard remain; no replacement allocation | Recovery and lifecycle tests |

## Tests

- `AC-EXECUTORS-PLUGIN-001.1`, `.4`, and `.8`: provider catalog tests cover no plugin, eligible plugin,
  unavailable plugin, and unchanged built-in executors.
- `AC-EXECUTORS-PLUGIN-004.1` and `AC-EXECUTORS-PLUGIN-005.1` through `.4`: existing recovery,
  inventory, and lifecycle tests remain and lose only flag-off assumptions.
- Runtime flag contract tests verify the retired identity and exact equality of live profile, Go, and
  web feature keys. Remove the obsolete disabled-default assertion.

## E2E tests

- `AC-EXECUTORS-PLUGIN-001.1`, `.8`, and `AC-EXECUTORS-PLUGIN-007.1`: update the desktop and phone
  provider-profile specs to run with shipped defaults; prove no provider card before fixture install.
- `AC-EXECUTORS-PLUGIN-001.3`, `AC-EXECUTORS-PLUGIN-004.1`, and
  `AC-EXECUTORS-PLUGIN-007.2`: retain task status and unavailable-provider coverage without flag setup.
- Replace the flag-off scenario in `remote-executor-plugin.spec.ts` with a no-plugin or disabled-plugin
  admission scenario. Existing phone composition and controls stay unchanged, so no new layout preview
  is required.

## Work orders

- [x] [Task 01: Retire the flag and keep provider admission](task-01-retire-flag.md)
- [x] [Task 02: Prove default behavior and update guidance](task-02-default-behavior.md)

Task 02 depends on Task 01. Both run sequentially in the primary session.

## Verification results

Implementation and verification completed:

- Targeted Go tests passed for runtime flags, profiles, plugins, task service, agent lifecycle, and
  common config. The common-config package was rerun with the session's pinned internal config
  environment variables unset.
- `make -C apps/backend lint` and `make -C apps/backend build` passed.
- The web feature-contract test, typecheck, lint, and `pnpm run i18n:check` passed.
- Desktop E2E passed all 4 tests; mobile E2E passed both tests. The profile-selection assertions use
  the created profile ID because display names can repeat.
- Public docs validation passed (62 tests and 47 pages); spec validation and lint passed (315
  decisions, 1,201 specs, and 36 lint tests); `git diff --check` passed.

## Risks

- An old SQLite override or explicit environment setting must become inert after registration removal.
- Removing the global gate must not relax provider absence, plugin status, contract version, or
  resource-ownership checks.
- E2E fixtures previously toggled the flag per test; dropping setup may reveal hidden order dependence.
