---
created: 2026-09-14
status: active
requirements:
  - REQ-PLUGINS-EXPLICIT-UTILITY-001
system_design:
  - ../../specs/plugins/system-design/plugin-explicit-utility-invocation.md
legacy_specs: []
---

# Implementation Plan: Explicit Plugin Utility Invocation

## Overview

Replace implicit plugin-configuration routing in PR #2870 with the user-approved default-plus-override contract.
Implement the host contract first. Then add a real plugin caller fixture, public documentation, and end-to-end evidence.
Finally migrate Notes in its dedicated repository. The companion task has a separate repository and delivery boundary.

The plugins system owns the Host contract. Agent settings continue to own profile records and the platform default.
The user explicitly confirmed default selection when no override is supplied and forbade host reads of plugin configuration for invocation.
The options struct and separate wire RPC are design choices that implement that direction without silent downgrade on older hosts.

## Starting point and handoff

- Repository: kdlbs/kandev; contributor PR: https://github.com/kdlbs/kandev/pull/2870.
- Reviewed branch: feature/implement-host-agent-oav in yattdev/kandev.
- Last verified head: 4af514930a417809eddd7ab2df76e8ec146ba773. Check the live head before editing or pushing.
- Prior fixes 5d620928 and 4af514930 preserve profile hydration correctness and Windows-safe Git test paths. Keep them.
- Last recorded CI: 56 passed. Those checks cover the old API, not this package.
- Companion Notes reference: yattdev/kandev-plugin-notes PR #7, source f8548f9d7a1eaabf19d27bb0b0af8f75d8eaeec4. Recheck its live state.
- These files are intentionally uncommitted. This turn creates no production or permanent test changes and no Kandev task/session objects.

## Scope

### In scope

- Public SDK options, safe wire evolution, platform default resolution, profile validation, and permission/error behavior.
- Removal of invocation-specific implicit configuration routing and legacy transition machinery.
- Explicit plugin-owned preference loading, fixture coverage, authoring documentation, and Notes caller migration.

### Out of scope

- A general InvokeAgentProfile API, cwd, session/worktree creation, streaming, and changes to profile eligibility.
- New settings UI, new storage, changes to ordinary utility actions, or automatic utility-record-to-profile migration.
- Merging either PR, publishing a release, or changing unrelated CI infrastructure.

## Technical approach

Follow the [design](../../specs/plugins/system-design/plugin-explicit-utility-invocation.md) and [requirements](../../specs/plugins/requirements/plugin-explicit-utility-invocation.md).
Task 01 changes SDK, proto adapters, host dispatch, backend composition, and obsolete legacy routing as one compilable contract slice.
Task 02 adds an explicit caller to the in-tree fixture and documents default/override usage. It proves settings and invocation cross real transport.
Task 03 migrates Notes in its own checkout; do not vendor the plugin into this repository.
No rendered UI change is planned. The existing optional profile picker, unavailable selection state, Save/Discard, and hydration remain in use.

## Tests

These are required new or extended tests, not claims that named cases already exist.

| Criteria | Required evidence |
| --- | --- |
| .1, .2, .3, .8 | host_utility_test.go: TestInvokeUtilityAgent_DefaultAndOverride, TestInvokeUtilityAgent_IgnoresPluginConfig, TestInvokeUtilityAgent_DefaultChangesBetweenCalls |
| .4, .5, .6, .7, .12 | host_utility_test.go: TestInvokeUtilityAgent_SelectionErrors and capability, cancellation, and runner tests; assert runner calls and absence of fallback |
| .1, .5, .8 | services_plugin_utility_test.go: TestPluginDefaultUtilityProfileSource; read errors, latest default, selected ID |
| .10, .11 | host_data_test.go and host_data_wire_test.go: TestInvokeUtilityAgent_WireOptions, TestInvokeUtilityAgent_OldHostRejected, TestInvokeUtilityAgent_LegacyWireUsesDefault |
| .3 | service_install_test.go and config_test.go: reload old marker/config without execution authority |
| .9 | cmd/plugin-fixture tests and Notes server/plugin_test.go: GetConfig preference explicitly reaches options; read/type error stops invocation |

All criteria use prefix AC-PLUGINS-EXPLICIT-UTILITY-001. Test names can be refined during implementation if the work orders record the final names.
SDK tests must cover zero, one-empty, one-nonempty, and multiple options values, plus no downgrade after Unimplemented.

## E2E tests

Add e2e/tests/plugins/plugin-utility-invocation.spec.ts to the chromium project using the real plugin-fixture subprocess.
Map .1, .2, .3, .4, .8, and .9 through these flows:

1. Configure default A, save plugin preference B, invoke the fixture's default action, and prove profile A executed.
2. Invoke its preference action, which reads GetConfig and sends B; prove profile B executed across gRPC and the runner.
3. Clear preference and reload; the preference action now executes A. Change the default to C and invoke again; prove C executed.
4. Restore B, then disable/delete B. The explicit call fails and A/C never executes. An unset default does not prevent valid B execution.

Use isolated mock inference profiles with distinct execution evidence. Observing the plugin's outgoing option alone is insufficient.
Do not call real paid agents. Reuse existing package installation and API helpers. Keep fixture probes separate from production APIs.
This exercises existing settings controls; no new UI structure or mobile-only behavior is introduced.

## Work orders

- [x] [Task 01: Explicit host invocation contract](task-01-host-contract.md)
- [x] [Task 02: Plugin caller integration and public contract](task-02-caller-integration.md)
- [ ] [Task 03: Notes companion migration](task-03-notes-companion.md) (implemented locally; delivery blocked by repository permissions)

Execute 01 -> 02 -> 03. This ordering does not authorize subagents.
Task 03 belongs to the separate Notes repository. Host readiness and companion readiness must be reported separately.

## Verification results

Implementation checks passed. Host and companion results are recorded in the
work orders. Planning checks also passed: catalog validation (268 decisions,
899 specifications), all 36 specification-linter tests, full specification
lint, and diff whitespace checks.

Host checks passed:

- `make -C apps/backend proto`
- `go test ./pkg/pluginsdk ./internal/plugins/... ./internal/backendapp ./internal/agent/hostutility ./internal/utility/profilebinding`
- `go test ./cmd/mock-agent`
- `go test ./cmd/plugin-fixture ./pkg/pluginsdk ./internal/plugins`
- `make -C apps/backend e2e-plugin-package`
- Chromium E2E: one test passed in `e2e/tests/plugins/plugin-utility-invocation.spec.ts`
- Public docs, document catalog, specification tests, full specification lint, and `git diff --check`

Host delivery commit: `eb9cfceda75cdc51c40efa289018e1a6f008d04a`, pushed to PR
#2870 branch `feature/implement-host-agent-oav`. Notes is a separate repository
and PR; its local implementation and delivery blocker are recorded in Task 03.

## Delivery instructions for the implementation agent

Read this package and local AGENTS.md before implementation. Follow TDD for new logic and run normal commit hooks.
Recheck PR head, draft state, conflicts, review threads, and merge queue before delivery. Preserve user edits in this checkout.
When host tasks pass, commit the host implementation and its documentation under Conventional Commits and push over SSH to the contributor PR branch.
Use the existing pr-head remote only after confirming its URL. Never force-push or overwrite contributor commits.
Post the authorized concise explanation of the new default/override contract and resolve only review threads actually addressed.
Wait for CI with scripts/pr-await, fix attributable failures, and record final head/check results. Do not merge the PR.
The user requested no commits in the planning turn; the later implementation turn owns commits and pushes.
For Notes, inspect its own repository instructions and current PR before any delivery. Do not push Notes changes onto host PR #2870.
If companion access or release coordination blocks delivery, record that in Task 03; do not mark the whole package complete.

## Risks

- Old prompt-only clients intentionally change from plugin configuration to the platform default on the new host.
- New clients must receive Unimplemented from old hosts; a fallback would silently change execution identity.
- Changing a Go interface requires updating all implementations, embedded defaults, wire adapters, and test doubles together.
- Existing global default identity rules must be reused; do not bind invocation to an unrelated browser user.
- Removing legacy helper code must not break ordinary utility-agent pickers or loading old serialized plugin records.
- The companion plugin uses a separate SDK dependency and needs coordinated packaging after the host change exists.
