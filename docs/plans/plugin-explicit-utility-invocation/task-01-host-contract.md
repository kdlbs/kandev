---
id: "01-host-contract"
title: "Explicit host invocation contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-EXPLICIT-UTILITY-001
acceptance_criteria:
  - AC-PLUGINS-EXPLICIT-UTILITY-001.1
  - AC-PLUGINS-EXPLICIT-UTILITY-001.2
  - AC-PLUGINS-EXPLICIT-UTILITY-001.3
  - AC-PLUGINS-EXPLICIT-UTILITY-001.4
  - AC-PLUGINS-EXPLICIT-UTILITY-001.5
  - AC-PLUGINS-EXPLICIT-UTILITY-001.6
  - AC-PLUGINS-EXPLICIT-UTILITY-001.7
  - AC-PLUGINS-EXPLICIT-UTILITY-001.8
  - AC-PLUGINS-EXPLICIT-UTILITY-001.10
  - AC-PLUGINS-EXPLICIT-UTILITY-001.11
  - AC-PLUGINS-EXPLICIT-UTILITY-001.12
system_design:
  - ../../specs/plugins/system-design/plugin-explicit-utility-invocation.md
---

# Task 01: Explicit host invocation contract

## Summary

Implement the options API and safe wire method through the real SDK, host, and backend adapters.
Remove implicit configuration routing and its obsolete transition machinery in the same compilable slice.

## In scope

- Add UtilityAgentOptions, update Host implementations and test doubles, and regenerate proto stubs with make proto.
- Preserve old prompt-only wire calls as default requests. Never downgrade the revised SDK to the old RPC.
- Wire the existing default getter through a narrow interface; retain profile eligibility, late wiring, and final runner checks.
- Remove invocation-only utility lookup, legacy markers, install stamping, and hidden configuration preservation. Keep old files loadable.
- Add transport, selection, permissions, storage-error, cancellation, default-change, and reload regressions using TDD.

## Out of scope

- General agent execution, cwd, session APIs, profile eligibility changes, and unrelated UI redesign.
- Work owned by other tasks. Do not revert their changes or the prior hydration and Windows CI fixes.

## Acceptance

- Every default/override and legacy-wire branch selects the prescribed profile without reading plugin configuration.
- Unsupported hosts and invalid selections cannot execute a fallback profile.
- The backend compiles, targeted tests pass, and old serialized records load without retaining routing authority.

## Verification

Use TDD for changed logic. Commands below run from the host repository root unless stated otherwise.

```bash
make -C apps/backend proto
(cd apps/backend && go test ./pkg/pluginsdk ./internal/plugins/... ./internal/backendapp ./internal/agent/hostutility ./internal/utility/profilebinding)
git diff --check
```

## Files likely touched

- `apps/backend/pkg/pluginsdk/host.go`
- `apps/backend/pkg/pluginsdk/host_data_test.go`
- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- `apps/backend/proto/kandev/plugin/v1/plugin.pb.go`
- `apps/backend/proto/kandev/plugin/v1/plugin_grpc.pb.go`
- `apps/backend/internal/plugins/host_utility.go`
- `apps/backend/internal/plugins/host_utility_test.go`
- `apps/backend/internal/plugins/host_data_wire_test.go`
- `apps/backend/internal/plugins/host.go`
- `apps/backend/internal/plugins/service.go`
- `apps/backend/internal/plugins/service_install.go`
- `apps/backend/internal/plugins/service_install_test.go`
- `apps/backend/internal/plugins/service_config.go`
- `apps/backend/internal/plugins/config_test.go`
- `apps/backend/internal/plugins/store/store.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/services_plugin_utility_test.go`

## Dependencies

None.

## Risks

- Changing SDK interfaces and wire semantics together requires all in-tree implementations to be migrated.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/plugins/requirements/plugin-explicit-utility-invocation.md) and [design](../../specs/plugins/system-design/plugin-explicit-utility-invocation.md).
- [Plan](plan.md), including compatibility rules, evidence matrix, and delivery instructions.
- Existing SDK wire tests, host utility tests, and plugin-fixture patterns. Read changed source and local AGENTS.md before editing.

## Results

Implemented. `make -C apps/backend proto` passed. The focused Go command passed
for `pkg/pluginsdk`, `internal/plugins/...`, `internal/backendapp`,
`internal/agent/hostutility`, and `internal/utility/profilebinding`. The
`cmd/mock-agent` suite also passed, including
`TestPromptUsesCurrentSessionModel`, which proves that ACP session model
selection reaches prompt execution. `git diff --check` passed.

Host commit: `eb9cfceda75cdc51c40efa289018e1a6f008d04a`, pushed to PR #2870.
