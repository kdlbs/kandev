---
id: "01-accepted-setup"
title: "Persist and project the accepted MCP setup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.17
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.18
system_design:
  - ../../specs/agents/system-design/workspace-mcp-pending-changes.md
---

# Task 01: Persist and project accepted MCP setup

## Outcome and ownership

Supply authoritative, safe before/after membership for session customization.
Own `apps/backend/internal/agent/mcpconfig/{selections,resolve_effective}.go`
and adjacent tests; `agent/settings/store/{sqlite,mcp_session_state,
mcp_selections}.go` and adjacent migration/store tests;
`agent/settings/handlers/mcp_selection_handlers.go` and tests; lifecycle
`manager_mcp_reconfiguration.go`, resolution/accepted-start integration and
adjacent tests. Integration also owns the exact provider-acceptance callback
in `lifecycle/session.go`, cursor recovery capture, and session-context wiring
in `internal/backendapp/services.go`. Read backend/scoped AGENTS before implementation.

No frontend markup, full configuration storage, endpoint probe, inherited
exclusion, changed provider timing or connection-evidence schema. Snapshot
includes the selected workspace definitions and their revision/enabled state,
not only delivered/connected server rows.

## Acceptance

1. Nullable versioned snapshot survives DB reopen, known empty differs from
   unknown, and initial start/reconfiguration/normal restart capture exactly the
   accepted request. Safe descriptors and bounded comparison satisfy the design.
2. Failed apply, stale operation and overlapping save cannot replace or clear
   newer authority. Projection uses latest consistent saved union; duplicate
   scopes, disabled/deleted definitions and origin-only changes remain truthful.
3. Session GET/PUT adds the optional comparison without changing other scopes
   or weakening workspace/session authorization. A comparison read failure does
   not turn a committed write into a save error; unknown/overflow is explicit.

## Verification

Use TDD with focused tests in each owned package. From apps/backend:

```sh
go test ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/agent/settings/handlers ./internal/agent/runtime/lifecycle -run MCP -count=1
```

Require targeted cases for initial known-empty, old-null migration, DB reopen,
add/remove/update, identity-preserving rename, deleted baseline name, multiple
origins, failed apply, R accepted after R+1 saved, stale result, cross-session
and cross-workspace isolation, sanitized output and bounded overflow. Verify
the exact operation snapshot is reused for delivery rather than read afterward.
Run gofmt on touched Go files and `git diff --check`.

## Risks and dependencies

No dependency. Do not hold a SQL transaction across provider I/O. Keep CAS
and atomic selection/state paths coherent and preserve model/mode behavior.
Inventory all producers before declaring coverage; unsupported legacy shapes
must remain unavailable. Do not backfill from current selection or connection
history. Task 02 depends on this response and lifecycle proof.

## Results

Implemented safe versioned snapshots, transactional resolution inputs and owner-guarded
comparison projection. Initial startup (including known empty), reset/restart,
idle reconfiguration, passthrough and cursor recovery retain the exact captured
request. Accepted state is saved atomically; stale pending, deferred and failure
writes cannot overwrite a newer applied baseline.

RED cases failed before their fixes: overlapping R/R+1 authority, nullable
migration and persistent DB reopen, combined projection overflow, initial
prompted startup, and stale status writers. GREEN on 2026-10-10:

```sh
GOCACHE=/tmp/kandev-pr3293-test-runtime/pending-go-cache go test ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/agent/settings/handlers ./internal/agent/runtime/lifecycle ./internal/backendapp -run MCP -count=1
GOCACHE=/tmp/kandev-pr3293-test-runtime/pending-go-cache go test -race ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/agent/runtime/lifecycle -run MCP -count=1
```

All five packages passed; all three race packages passed. Tests cover denied
owner/workspace access, projection failure after a successful save, deleted
names, duplicate origins, known empty versus unknown, bounded safe wire output,
DB reopen and sibling-session isolation. Private Go cache avoids another
workspace removing shared build artifacts. Changed-package golangci-lint caught
three repeated literals and an unchecked test DB close; constants and checked
cleanup fixed them without changing behavior. The final changed-package lint
reported zero issues; the two affected packages passed again and all required
host artifacts rebuilt successfully. gofmt and whitespace checks passed.
