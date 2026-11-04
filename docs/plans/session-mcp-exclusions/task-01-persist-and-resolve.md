---
id: "01-persist-and-resolve"
title: "Persist and resolve session exclusions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-MCP-EXCLUSIONS-001
  - REQ-AGENTS-SESSION-MCP-EXCLUSIONS-002
acceptance_criteria:
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.1
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.2
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.3
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.4
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.5
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.6
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.7
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.5
  - AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.6
system_design:
  - ../../specs/agents/system-design/session-mcp-exclusions.md
---

# Task 01: Persist and resolve session exclusions

## Summary

Deliver the authenticated API-to-resolver path with atomic session state.
Every runtime consumer receives a post-exclusion setup through the existing resolver.
This is a backend verification boundary before controls become available.

## In scope

- Relation, migration replay, SQLite/PostgreSQL consistency, cleanup, and reopen tests.
- Extend assignment-owned session GET/PUT CAS with presence-aware exclusions and
  compatible old writes. Reuse its lock, response, precondition, and retry rules.
- Same-workspace and inherited-target validation, disabled retention, dormant state.
- Consistent resolver input, pre-secret suppression, safe decision, and comparison identity.

## Out of scope

Rendered controls, built-in policy, assignment, usage navigation, and new provider behavior.

## Acceptance

1. One accepted write changes only the target session. Stale/invalid writes change
   neither set, and omission cannot clear exclusions.
2. Resolution subtracts by ID without losing origins or triggering secret/package
   preparation. Legacy fallback cannot restore the same selected catalog alias.
3. Replay/reopen, deletion, two-store PostgreSQL writers, and exact setup comparison
   pass their tests with retained accepted state.
4. Exclusion-only edits advance desired revision; same-set Retry remains effective.
   Stale revision rejects even matching sets. Addition-only callers preserve E,
   explicit empty E clears it, null rejects, and rejected mixed writes change neither set.

## Verification

From repository root, after red/green tests:

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/agent/settings/handlers -run 'MCP|Selection|Effective' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/backendapp -run 'MCPDeletion|MCPSession' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
git diff --check
```

Repeat the focused store command against a disposable database with
`KANDEV_TEST_POSTGRES_DSN` set. Record the exact command and whether PostgreSQL
actually ran. Do not mark it passed when skipped. Run `bash scripts/lint-go-changed`
with the actual changed Go paths after implementation. No full-backend lint.

## Files likely touched

- `apps/backend/internal/agent/mcpconfig/{selections.go,resolution_inputs.go,resolve_effective.go,setup_snapshot.go,setup_comparison.go,session_setup_comparison.go}`
- `apps/backend/internal/agent/mcpconfig/session_exclusions_test.go` (new) and neighboring selection/resolver/comparison tests.
- `apps/backend/internal/agent/settings/store/{sqlite.go,mcp_selections.go,mcp_resolution_inputs.go,mcp_session_state.go}` and new `mcp_session_exclusions.go`/tests.
- `apps/backend/internal/agent/settings/handlers/mcp_selection_handlers.go` and new exclusion handler tests.
- `apps/backend/internal/backendapp/agents.go`, `mcp_deletion_cleanup_test.go`, and persistence conformance fixtures/manifest if required.

## Dependencies

Registration assignment task `5a8cb5cd-22ee-42e5-bd6d-e2179e899c8c`, work order
`docs/plans/mcp-assignment-after-registration/task-01-guard-selection-replacement.md`.
Integrate its committed conditional-selection primitive before this work order.
Its owner delivers generic locks, session revision admission, consistent GET and
exact committed responses, and retry/conflict semantics. Extend that operation
for E; do not reproduce it. Assignment work orders 02/03 are not prerequisites.
Use `expected_revision` against `selection_revision` (existing desired revision).
The crossed session `expected_definition_ids` / `expected_excluded_definition_ids`
alternative and whichever-lands-first ordering are superseded. Reuse assignment's
rejection of invalid revisions and wrong-scope preconditions.
The empty frontmatter dependency list means no local work-order predecessor;
this external prerequisite is still required. Reverify parent tip and guidance.

## Risks

Database locks must precede read-modify-write. Never hold a transaction across
provider I/O. Task sessions live outside the settings database. Use the existing
`events.SessionRemoved` for individual cleanup and test the registered subscriber.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/agents/system-design/session-mcp-exclusions.md): composition, persistence, API, resolver, snapshot.
- Existing `postgres_mcp_consistency_test.go`, `mcp_setup_projection_test.go`, and `mcp_setup_comparison_test.go` patterns.
- `/tdd` and its backend test reference at implementation time.

## Results

Implementation authorized 2026-10-23. Integrated assignment work order 01 at
`f9137074309bebe99e2c7576baefe936bc20358f`. Reuse `SelectionCondition`,
`SelectionSnapshot`, and `ReplaceMCPSelectionsConditional`; no separate writer.
Implemented the exclusion relation, atomic conditional writes, trusted inherited
validation, resolver subtraction before secrets, comparison identity, and cleanup.

RED evidence: round-trip omitted E, invalid exclusion writes returned 200,
resolver still delivered the excluded server, and deletion left orphan exclusions.
GREEN evidence: focused selection/setup/exclusion Go suites passed under race;
actual PostgreSQL mixed writers admitted one revision and rejected the stale
writer. SQLguard and settings-store conformance passed. Changed-Go lint passed.
Disk reopen and rollback checks are recorded with final verification in the plan.

The broad `MCP|Selection|Effective` command initially failed in
`TestExecNativeMCPCommandRunnerBoundsInheritedPipeAfterCancellation`. Its fixture
hardcoded `/usr/bin:/bin`, which lacks `sleep` on this Nix host, so the shell
exited before cancellation. Preserving the host PATH in that test's explicit
environment fixed the fixture. The exact broad command then passed under race
for all three packages. No production command-runner behavior changed.
No live-provider connection or native-file revocation claim follows from these checks.
