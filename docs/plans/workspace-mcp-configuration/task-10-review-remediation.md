---
id: "10-review-remediation"
title: "Preserve MCP write and snapshot authority"
status: done
wave: 7
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-001
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-004
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-007
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-007.5
system_design:
  - ../../specs/agents/system-design/workspace-mcp-resolution-delivery.md
---

# Task 10: Preserve MCP write and snapshot authority

## Summary

Repair independent-review findings 1, 2, 4 and 5 without expanding the product
scope. Confirmed deletion, coherent resolution, legacy secret safety and editor
context ownership are existing requirements.

## In scope

PostgreSQL delete parameter rebinding and consistent resolution transactions;
legacy URL projection with retained fallback; stale save cleanup guarded by its
original scope context. Preserve the main-merge fixes and other user changes.

## Out of scope

New management features, runtime probes, migrations, or changes to selection
composition. Tasks 11/12 own setup controls and attachment evidence.

## Acceptance

1. PostgreSQL confirmed deletion removes all four association types atomically;
   a stale revision preserves them. Snapshot resolution cannot acknowledge a
   newer selection revision while delivering an older set.
2. Legacy URL userinfo/query/fragment credentials never enter the public catalog;
   unsafe imports remain pending and idempotent, with original runtime fallback.
3. A late old save's success or rejection cannot invalidate the replacement
   context's pending GET or clear/replace its state. Existing save behavior stays.

## Verification

From repository root; set `KANDEV_TEST_POSTGRES_DSN` to the owned disposable
PostgreSQL service before the first command. A skipped PostgreSQL test is not
evidence for these findings.

```bash
(cd apps/backend && go test ./internal/agent/mcpconfig ./internal/agent/settings/store -run 'Test(LegacyImportDoesNotPublishURLCredentials|LegacyEndpointRetainsPublicURLAndSanitizesParseFailure|PostgresConfirmedMCPDelete|PostgresMCPResolutionSnapshot)$' -count=1)
(cd apps/web && corepack pnpm@9.15.9 exec vitest run hooks/domains/workspace/use-mcp-selection-editor-context.test.ts hooks/domains/workspace/use-mcp-selection-editor.test.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check origin/main
```

## Files likely touched

Store `mcp_selections.go`, `mcp_resolution_inputs.go`, and the new
`postgres_mcp_consistency_test.go`; domain `legacy_import.go` and
`legacy_import_url_test.go`; `use-mcp-selection-editor.ts` and its existing/new
context tests. The implementation uses the same driver-aware transaction pattern
as profile-order snapshot reads.

## Dependencies

Existing implemented feature; user's explicit review/fixup request authorizes
this repair of accepted requirements.

## Risks

Stripping URL components cannot be called successful credential migration.
ReadOnly alone does not create a PostgreSQL transaction snapshot. Avoid a
timing-only race test: the regression observes a blocked catalog read through
PostgreSQL lock state while another transaction commits.

## Parallelism

`sequential`

## Inputs

Independent review findings 1/2/4/5, recorded in the user's separate isolated
review session; owning design Store/migration and Frontend request ownership.

## Results

RED reproduced SQLSTATE 42601, mixed revision=2/old selection, catalog URL
credentials, and cancelled replacement-context load. Expanded rollback, fallback and rejection controls passed
with a real PostgreSQL 17 service. The final full store/domain suites and all
changed frontend test files passed, including stale-context success/rejection
regressions. Current Go lint, frontend lint/typecheck and documentation checks
passed. Publication and final PR gates remain owned by the parent plan.
