---
id: "03-launch-proof"
title: "Integrate native discovery and prove both launch modes"
status: done
wave: 3
depends_on: ['02-native-inventory']
plan: "plan.md"
requirements:
  - REQ-AGENTS-CURSOR-AUTH-001
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-001
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-003
acceptance_criteria:
  - AC-AGENTS-CURSOR-AUTH-001.3
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.10
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.12
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.1
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.3
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.5
system_design:
  - ../../specs/agents/system-design/cursor-mcp-oauth-bridge.md
  - ../../specs/agents/system-design/cursor-plugin-mcp-import.md
---

# Integrate native discovery and prove both launch modes

## Scope and files

Own lifecycle/cursor_plugin_mcp.go and its tests, manager_project_mcp.go,
manager_passthrough.go and nearest launch tests; use manager_launch.go trusted
RepositoryPath metadata and executor_backend.go MetadataKeyRepositoryPath.
Keep policy/profile reservations, native identity and owned-entry reconciliation.
Add a small native smoke harness under scripts/ only if needed for reproducibility.

Resolve the source repository from fresh and resumed launch context, including
promotion. Cover primary-repository semantics for multi-repository tasks and
repository-free tasks. Do not infer source from credential origins or task cwd.

## Acceptance

1. ACP and terminal preparation import exact enabled native identities, share
   the selected complete credential object, and omit source/task-disabled
   servers while preserving explicit/user-edited entries.
2. Remote, different-HOME, disabled preference, no-repository and missing source
   context paths have explicit tested outcomes. All four auth/import flag
   combinations remain independent.
3. A versioned disposable native smoke demonstrates registration and one
   harmless local fixture tool call. Authentication or approval failures are
   reported accurately; fixture-only output does not count as native success.

## TDD and validation

RED: TestCursorMCPLaunchUsesNativeInventoryAndSourceDisables for both entry
points, with older authenticated/newer registration-only sources and a synthetic
enabled service response. Include disabled harness Figma only in the selected
source, and the negative unrelated-workspace case. Cover fresh/resume/promotion,
stale owned bare entries, service failure and request-order races with barriers.
Run service calls outside the global file lock but fence stale per-workspace
results. Reuse existing lifetime/locking mechanisms where available.

From apps/backend:

- go test -race ./internal/agent/mcpconfig -count=1
- go test -race ./internal/agent/runtime/lifecycle -run 'Cursor|ProjectMCP' -count=1
- golangci-lint run ./internal/agent/mcpconfig/... ./internal/agent/runtime/lifecycle/...

Use acp-debug for the native ACP smoke and its documented sanitized capture
workflow. Use an isolated HOME/workspace and local MCP fixture for terminal
registration. Never operate on the reported live task. Verify the exact emitted
identifier consumes synthetic shared auth. Record CLI version, commands,
registration, tool result and cleanup in compatibility-evidence.md.

At implementation time update docs/public/agents-and-profiles.md and remove
superseded filesystem-only limitations. Run from repo root:

- python3 scripts/list-docs.py validate
- python3 scripts/lint-spec-files.py --all
- node --test scripts/validate-public-docs.test.mjs
- node scripts/validate-public-docs.mjs
- git diff --check

## Dependencies and exclusions

Tasks 01 and 02. No browser changes, blanket approval, personal MCP credentials
in tests, production Jira calls, PR push or live-task restart.

## Results

Implemented shared native inventory and source/task disable preparation for ACP
and terminal. Trusted primary repository metadata is projected at fresh launch
and workspace promotion; terminal resume uses persisted source context. Ordinary
attached folders do not supply repository disables. Per-workspace generations
fence stale inventory responses, including symlink aliases, while service I/O
stays outside the project-file lock. Cancellation does not publish results.

Regression coverage includes exact native cache revisions, complete synthetic
OAuth objects, source/task disables, explicit configuration preservation, all
four auth/import preference combinations, remote/custom-HOME gates, unavailable
source/disable state, legacy resumes, promotion and owned-entry reconciliation.
The promotion test reproduced importing a disabled plugin before trusted launch
context was projected. Barrier tests reproduced the symlink-alias stale-write
case before canonical workspace fencing. A deferred-inventory path-swap test
failed by writing outside the workspace when the final containment recheck was
removed, and passed with the recheck restored.

Validation from `apps/backend`:

- `go test -race ./internal/agent/mcpconfig -count=1`: passed (Task 02).
- `go test -race ./internal/agent/runtime/lifecycle -run 'Cursor|ProjectMCP' -count=1`: passed.
- Trusted launch metadata and promotion metadata regression tests: passed separately.
- `golangci-lint run ./internal/agent/mcpconfig/... ./internal/agent/runtime/lifecycle/...`: passed, 0 issues.

Native terminal and ACP calls both passed with synthetic provider credentials
and fixture-only permissions on Cursor Agent `2026.09.26-dd393fe`; see
[compatibility evidence](../cursor-plugin-mcp-import/compatibility-evidence.md).
No live Jira call or reported-task mutation was performed.

Public docs updated: `docs/public/agents-and-profiles.md` (reference/explanation).
Specification catalog/lint and public-doc validation passed; public-doc tests
passed all 62 cases. Changes remain uncommitted and unpushed.
