---
id: TASK-SHARED-PROMPT-WRITES-01
title: Guarded shared prompt writes
status: completed
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-SHARED-PROMPT-WRITES-001
acceptance_criteria:
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.1
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.2
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.3
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.4
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.6
  - AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.8
system_design:
  - ../../specs/integrations/system-design/shared-prompt-writes.md
---

# Guarded shared prompt writes

## Scope and files

Prompt model/store/service/DTO/controller; MCP handlers, tools, action constants, config context; generic settings adapter; persistence upgrade evidence.

## Acceptance

- Implement the referenced criteria with targeted Red-Green-Refactor evidence.
- Preserve existing human editing, prompt expansion, and authentication behavior.
- Record all required check results after the final changes.

## Exclusions

MCP deletion and unrelated settings or workflow redesign.

## Dependencies and parallelism

None. Sequential; no subagents.

## Risks

Permission bypass, concurrent revocation, migration data loss, and stale readback.


## Verification

```bash
(cd apps/backend && go test -race ./internal/prompts/... ./internal/mcp/handlers ./internal/mcp/server ./internal/gateway/websocket ./config/prompts -count=1)
(cd apps/backend && go test -race ./internal/backendapp -run 'Test.*(Prompt|Settings)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
```

## Results

Passed prompt, MCP handler/catalog, and generic-settings tests with the race
detector. SQLite and PostgreSQL conditional-write tests reject revocation,
renaming, deletion, built-in mutation, and stale content. Previous-stable upgrade
and replay conformance passed on both databases, including permission-off
sentinels without changing the historical fixture SQL. SQL guard and affected
package Go lint passed.

Red evidence: missing capability/permission, catalog registration, migration, and
change-notification tests failed before implementation; these pass afterward.
Dedicated agent writes and the generic settings adapter share the same guarded
service. Duplicate PostgreSQL name conflicts return the domain error.


Review remediation: operator database updates compare the observed timestamp,
preventing a content-only PATCH from overwriting a concurrent revocation. HTTP
and MCP report stale writes as conflicts. Regressions cover this guard on both
databases, error projection, and generic-settings read permission metadata. The
CI gateway subscription-count expectation now includes prompt invalidation, and
a focused broadcast test verifies its content-free payload. CI also exposed an
unchanged SSH timeout test; ten local race-enabled repetitions passed, and the
next PR head must still pass remote CI before delivery.
