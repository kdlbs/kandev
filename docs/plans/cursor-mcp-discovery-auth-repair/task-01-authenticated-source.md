---
id: "01-authenticated-source"
title: "Prefer whole credential-bearing source objects"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CURSOR-AUTH-001
  - REQ-AGENTS-CURSOR-AUTH-003
acceptance_criteria:
  - AC-AGENTS-CURSOR-AUTH-001.2
  - AC-AGENTS-CURSOR-AUTH-001.6
  - AC-AGENTS-CURSOR-AUTH-003.1
  - AC-AGENTS-CURSOR-AUTH-003.5
system_design:
  - ../../specs/agents/system-design/cursor-mcp-oauth-bridge.md
---

# Prefer whole credential-bearing source objects

## Scope and files

Own cursor_auth_bridge.go and cursor_auth_bridge_test.go under
apps/backend/internal/agent/mcpconfig. Retain newest-file/path ordering within
credential-bearing and registration-only classes. Select a whole raw object.
Update the public auth paragraph at implementation time, not during planning.

## Acceptance

1. An older object with a non-empty access or refresh token wins over newer
   registration-only/null/empty/malformed token data; its matching clientInfo and
   unknown fields remain intact.
2. Newest authenticated source wins among peers, including equal-mtime path
   tie-breaks; if none has tokens, the newest registration-only object wins.
3. Current source/task/symlink exclusions, source removal, protected destinations,
   opt-out and no-valid-source behavior remain unchanged. No old master fallback.

## TDD and validation

RED: TestCursorMCPAuthPrefersCredentialBearingSource, using the real aggregation
and publication path with conflicting clientInfo identities and an independent
server. Add access-only, refresh-only, whitespace, malformed values, all-empty,
two authenticated sources, task exclusion and removal cases. GREEN only after
the original implementation fails the intended assertion.

From apps/backend:

- go test ./internal/agent/mcpconfig -run 'CursorMCPAuth|AggregateCursorMCPAuth|LinkCursorMCPAuth' -count=1
- go test -race ./internal/agent/mcpconfig -count=1
- golangci-lint run ./internal/agent/mcpconfig/...

No provider calls or real credential reads in tests. No token expiry ranking,
deep merging, refresh or source file mutation.

## Results

Implemented credential-bearing-first selection for each exact server identity.
Non-empty string access or refresh tokens qualify. Newest-file/path ordering is
retained within credential-bearing and registration-only classes, and the
selected whole object is preserved.

RED/GREEN evidence: `TestCursorMCPAuthPrefersCredentialBearingSource` first
failed because the newer registration-only object won; it passed after the
selection change. The ranking matrix covers access-only, refresh-only,
whitespace/malformed and empty values, newest authenticated peers, and equal-time
path ordering. Existing task-root exclusion and source-removal tests remain
covered by the package suite.

Validation from `apps/backend`:

- `go test ./internal/agent/mcpconfig -run 'CursorMCPAuth|AggregateCursorMCPAuth|LinkCursorMCPAuth' -count=1`: passed.
- `go test -race ./internal/agent/mcpconfig -count=1`: passed.
- `golangci-lint run ./internal/agent/mcpconfig/...`: passed with 0 issues.
