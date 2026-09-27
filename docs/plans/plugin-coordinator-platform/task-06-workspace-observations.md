---
id: "06-workspace-observations"
title: "Canonical workspace and evidence queries"
status: complete
wave: 6
depends_on: ["01-exact-host-foundation"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-COORDINATION-004
acceptance_criteria:
  - AC-PLUGINS-COORDINATION-004.1
  - AC-PLUGINS-COORDINATION-004.2
  - AC-PLUGINS-COORDINATION-004.3
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
---

# Task 06: Canonical workspace and evidence queries

## Summary

Expose bounded exact queries for workspace coordination and outcome reporting. Keep provider evidence and unknown usage explicit.

## In scope

- Add snapshot-bound exact catalog/task/session/interaction/message/directive/relation/pending-transition reads using existing services.
- Project canonical task status, blocking reasons, semantic activity, PR head/check/review evidence, and available usage with units and freshness. Reuse GetTaskUsageTotals/GetTaskSessionUsageTotals and preserve ledger completeness, unpriced counts, integer cost units, and no-event semantics.
- Test dropped events followed by full snapshot reconciliation, cursor scope leakage, stale heads, unavailable providers, and unknown costs.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- An authorized consumer can reconstruct current task state after dropped events without deriving status from private data.
- Pagination is stable and scoped, and provider evidence is tied to its subject version.
- Usage reports distinguish measured, estimated, and unknown values.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd apps/backend && go test -race ./internal/plugins -run 'TestExactWorkspaceSnapshot|TestExactEvidenceAndUsage' -count=1)
(cd apps/backend && go test ./internal/task/statussummary/... ./pkg/pluginsdk/...)
```

Evidence to create:

- `apps/backend/internal/plugins/host_exact_queries_test.go`: `TestExactWorkspaceSnapshot`.
- `apps/backend/internal/plugins/host_exact_evidence_test.go`: `TestExactEvidenceAndUsage`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `apps/backend/internal/plugins/host_data_queries.go`
- `apps/backend/internal/plugins/host_data_mappers.go`
- `apps/backend/internal/task/statussummary/`
- `apps/backend/internal/task/service/service_usage.go`
- `apps/backend/internal/task/models/usage_totals.go`
- `apps/backend/internal/plugins/host_interactions.go`
- `apps/backend/pkg/pluginsdk/`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 01](task-01-exact-host-foundation.md)

## Risks

Existing event delivery is best effort. Query contracts must remain sufficient when the event backlog has been lost.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/managed-coordination.md) and [design](../../specs/plugins/system-design/managed-coordination.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

Complete. Added snapshot-bound exact workspace, workflow, step, task, session,
interaction, message, relation, pending-transition, change-request evidence, and
usage reads. Queries enforce installation approval, workspace grants, scoped
cursors, stable snapshots, and bounded pages. Tests cover lost-event reconciliation,
workspace/cursor leakage, stale approval revisions, pending transitions, sanitized
messages, canonical status and blockers, stale PR heads, unavailable providers,
and unknown or incomplete usage.

Task directive reads remain explicitly unsupported and are reported as such by
capability discovery because no durable directive source exists yet. AC-PLUGINS-
COORDINATION-004 does not require directives; WO07 owns durable directive commands
and should connect their records to these already-declared exact read methods.

Verification passed:

- `TMPDIR=/root/.cache/kandev-test-tmp GOTMPDIR=/root/.cache/kandev-test-tmp go test -race ./internal/plugins -run 'TestExact(WorkspaceSnapshot|TaskInboxAndPendingTransitions|TaskRelationsAreVersionedAndWorkspaceScoped|TaskProjectionIncludesSummaryActivityAndBlockers|SanitizedMessageSnapshot|EvidenceAndUsage|UsageNilTotalsFailClosed)$' -count=1`
- `TMPDIR=/root/.cache/kandev-test-tmp GOTMPDIR=/root/.cache/kandev-test-tmp go test ./internal/task/statussummary/... ./pkg/pluginsdk/...`
- `node --test scripts/validate-public-docs.test.mjs` (62 tests) and `node scripts/validate-public-docs.mjs` (47 pages)
- `git diff --check`
