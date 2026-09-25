---
id: "01-auth-helpers"
title: "Safe Cursor auth helpers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CURSOR-AUTH-001
  - REQ-AGENTS-CURSOR-AUTH-002
  - REQ-AGENTS-CURSOR-AUTH-003
acceptance_criteria:
  - AC-AGENTS-CURSOR-AUTH-001.1
  - AC-AGENTS-CURSOR-AUTH-001.2
  - AC-AGENTS-CURSOR-AUTH-001.4
  - AC-AGENTS-CURSOR-AUTH-001.6
  - AC-AGENTS-CURSOR-AUTH-002.3
  - AC-AGENTS-CURSOR-AUTH-003.1
  - AC-AGENTS-CURSOR-AUTH-003.2
  - AC-AGENTS-CURSOR-AUTH-003.3
  - AC-AGENTS-CURSOR-AUTH-003.4
  - AC-AGENTS-CURSOR-AUTH-003.5
  - AC-AGENTS-CURSOR-AUTH-003.6
system_design:
  - ../../specs/agents/system-design/cursor-mcp-oauth-bridge.md
---

# Safe Cursor auth helpers

## Summary

Create safe, independently tested filesystem helpers with synthetic credentials.

## Scope and owned files

- New `apps/backend/internal/agent/mcpconfig/cursor_auth_bridge.go`.
- New `apps/backend/internal/agent/mcpconfig/cursor_auth_bridge_test.go`.
- A focused private helper file if Go size limits require it.
- Requested public helper signatures, private aggregation result, and bridge-link cleanup.
- Serialization, source selection, canonical workspace resolution, atomic writes, and destination preservation.

Exclude lifecycle hooks, profile persistence, UI, OAuth calls, and real home-directory reads.

## Implementation acceptance

1. All filesystem cases in the design pass, including duplicate timestamps and symlinked source directories.
2. No-source and malformed-source cases cannot create a new link to a stale master. Regular destinations remain unchanged.
3. Parallel bridge calls produce complete snapshots and links. Disabled cleanup removes only a matching bridge link.

## TDD and verification

First write table cases for selection, slug derivation, protected files, and removal.
Then implement the smallest helpers that satisfy them.
Add error cases for malformed JSON, unreadable entries, invalid master type, and failed publication.
Use injected failures where root execution makes permission tests unreliable.
Check exact mode bits on supported filesystems.
Check temporary-file cleanup after failures.

From `apps/backend`:

```bash
go test -v ./internal/agent/mcpconfig/...
go test -race ./internal/agent/mcpconfig/...
```

## Dependencies and risks

No implementation dependency. Use only standard-library filesystem and JSON operations.
The mutex coordinates Kandev callers, not Cursor or hostile external path replacement.
Do not promise token validity or external-process refresh synchronization.

## Results

Implemented the synthetic Cursor auth aggregation, atomic master publication, project symlink, protected destination handling, canonical workspace resolution, and disabled cleanup helpers.

Verification passed from `apps/backend`:

- `go test ./internal/agent/mcpconfig/...`
- `go test -race ./internal/agent/mcpconfig/...`
