---
id: "03-runtime-preparation"
title: "Prepare and recover native MCP connections"
status: complete
wave: 2
depends_on: ["01-profile-contracts"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-PREP-001
  - REQ-AGENTS-MCP-PREP-002
  - REQ-AGENTS-MCP-PREP-003
acceptance_criteria:
  - AC-AGENTS-MCP-PREP-001.2
  - AC-AGENTS-MCP-PREP-001.3
  - AC-AGENTS-MCP-PREP-001.4
  - AC-AGENTS-MCP-PREP-002.1
  - AC-AGENTS-MCP-PREP-002.2
  - AC-AGENTS-MCP-PREP-002.3
  - AC-AGENTS-MCP-PREP-002.4
  - AC-AGENTS-MCP-PREP-002.5
  - AC-AGENTS-MCP-PREP-003.1
  - AC-AGENTS-MCP-PREP-003.2
  - AC-AGENTS-MCP-PREP-003.3
system_design:
  - ../../specs/agents/system-design/agent-mcp-preparation.md
---

# Task 03: Prepare and recover native MCP connections

## Summary

Prepare and recover native MCP connections according to the linked requirements and design.

## In scope

Own new native command adapter/discovery implementations (not credential bridge), agent discovery handler/controller, lifecycle selection/materialization/preparation events, typed session authentication/retry backend routes and tests. Exact discovery route/schema is frozen in design. Before editing agree recovery response with UI worker. Aggregate environment and MCP preparation before completion across launch/resume/promotion; expose bounded sanitized per-server metadata. Validate final ownership/source disables and current generation immediately before native approval. Recovery accepts server ID, resolves trusted task context, opens/deduplicates native task terminal, supports same-session recheck without replay. Do not edit profile storage/resolver/types assigned Task01 except coordinated additions after handoff. No web edits.

### Coordinated ownership

The runtime worker owns native command adapter, lifecycle selection/preparation,
Manager recovery validation/reconnect operations and progress serialization.
After Task 02, the contracts worker owns sanitized discovery service/settings
HTTP route and typed recovery transport handlers, consuming the agreed Manager
interface. They coordinate interface changes before edits and do not edit each
other's files. UI remains with Task 04.

## Out of scope

Other work-order ownership, commits, personal credential changes and unrelated refactors.

## Acceptance

- Satisfy every referenced acceptance criterion within this boundary.
- Add failing behavior tests first, implement, then pass targeted checks.
- Preserve existing dirty work and report exact validation evidence.

## Verification

```bash
(cd apps/backend && GOCACHE=/tmp/kandev-cursor-review-go-cache go test ./internal/agent/mcpconfig ./internal/agent/runtime/lifecycle ./internal/agent/settings/... ./internal/task/... -count=1)
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/cursor_plugin_mcp.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_project_mcp.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/settings/handlers/handlers.go`
- `apps/backend/internal/agent/handlers/shell_handlers.go`

## Dependencies

01-profile-contracts

## Risks

See plan compatibility and concurrency risks; escalate contract changes to coordinator.

## Parallelism

parallel-safe after Task 01 freezes shared contracts; disjoint implementation ownership.

## Inputs

- [Requirements](../../specs/agents/requirements/agent-mcp-preparation.md).
- [System design](../../specs/agents/system-design/agent-mcp-preparation.md).
- Existing scoped AGENTS.md, TDD skill and neighboring test patterns.

## Results

Implementation is complete. Scoped acceptance tests cover profile discovery,
selection/disable/ownership fences, native process bounds, preparation ordering,
credential reuse outcomes, exact-identity recovery, and one-shot login terminals.
The native command adapter uses observed Cursor output and preserves tool
permissions. ACP recovery restarts the child and loads the exact saved conversation;
TUI live reload and Windows login-terminal limits are explicit.

Passing checks on 2026-09-28:

- MCP config and settings package tests and race tests.
- Terminal repository/service and gateway package tests; focused recovery and
  one-shot terminal/process race tests, including real HTTP/PTY/reconnect behavior.
- Lifecycle acceptance/race tests, including mixed-case native IDs, failed saved
  session load, busy rejection and successor-generation barriers.
- Orchestrator preparation ordering/persistence race tests.
- Scoped lint across changed settings, MCP config, lifecycle, terminal, gateway,
  task/agent handlers, process and backendapp packages.
- Windows cross-compilation of MCP config, SQLguard and store-conformance race tests.
- Both opt-in installed Cursor native smoke tests; see
  [compatibility evidence](compatibility-evidence.md).

Broader package runs are not wholly green. Unchanged repository branch-policy
and backend startup tests fail on macOS temporary-path canonicalization; separate
Git-push and managed-runtime-cache tests also fail independently. No clean-baseline
run proves these failures predate the branch. See
[review findings](review-findings.md) for exact names and verification limits.
The full lifecycle rerun also failed in 12 non-MCP test cases involving
retained/worktree path and cleanup expectations and Unix socket paths. Its
138.704s result and exact failing test names are recorded in the review report.
Final path-level regressions also passed: `TestLaunch_PromotionPublishesPrepareCompletedAfterCursorMCPPreparation`
and `TestCursorPassthroughStartAndResumePublishAfterMCPPreparation` (both start
and resume). They block native discovery, assert that no aggregate completion
has fired, then release discovery and verify the final MCP step is present.
Lifecycle lint passed again after these test-only additions.


PR #4025 remediation on 2026-09-30 preserves the executor's authoritative
preparation outcome instead of treating every failed setup-script row as fatal.
The new regression covers both optional failure with overall success and fatal
environment failure with overall failure. The workspace-promotion merge also
preserves main's task scope and provider-restored startup/projection policies;
the promotion regression asserts those policies.

From `apps/backend`, the following focused command passed with and without
`-race`:

```bash
go test ./internal/agent/runtime/lifecycle -run 'TestPreparationAttempt|TestExecutionPrepareCompletion|TestLaunch_.*(Prepare|Promot)|TestPromotion|TestCursorPassthroughStartAndResumePublishAfterMCPPreparation' -count=1
```

`golangci-lint run ./internal/agent/runtime/lifecycle/... --new-from-rev=origin/main --timeout=5m`
also passed. Public setup-script documentation already promises nonfatal
failures, so this remediation restores that contract without a public-copy
change. Fresh pushed-head CI remains an external verification step.


Latest-main integration on 2026-09-30 retained its extracted profile HTTP
handler owner. MCP-selection validation errors are forwarded by
`profile_handlers.go`; profile CRUD and model-capability discovery remain
registered together. `go test -race ./internal/agent/settings/...` and the
focused lifecycle race command above both passed after this integration.


After the latest-main integration,
`golangci-lint run ./internal/agent/settings/... ./internal/agent/runtime/lifecycle/... --new-from-rev=origin/main --timeout=5m`
also passed from `apps/backend`.


The merge hook caught the combined create-profile handler above the cyclomatic
complexity limit. Its existing required-field/MCP-validation message selection
is now shared by create and update handlers through `profileValidationMessage`.
The HTTP regressions retain blank-name, invalid-mode and duplicate-selection
error assertions. The normal changed-package lint hook remains required.
