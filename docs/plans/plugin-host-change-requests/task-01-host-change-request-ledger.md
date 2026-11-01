---
id: "01-host-change-request-ledger"
title: "Host-owned task change-request ledger"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PLUGINS-001
acceptance_criteria:
  - AC-PLUGINS-PLUGINS-001.7
  - AC-PLUGINS-PLUGINS-001.9
  - AC-PLUGINS-PLUGINS-001.10
  - AC-PLUGINS-PLUGINS-001.11
  - AC-PLUGINS-PLUGINS-001.12
system_design:
  - ../../specs/plugins/system-design/plugins-01.md
---

# Task 01: Host-owned task change-request ledger

## Summary

Add a Host-owned, plugin-reported task change-request ledger. A code-host
plugin can report the change request it observes, keep its lifecycle current,
remove the association, and read its own exact evidence with the real provider
id.

## In scope

- Exact `ReportTaskChangeRequestExact` and `RemoveTaskChangeRequestExact` Host
  commands behind the existing manifest and workspace approval boundary.
- Installation, workspace, provider, immutable repository, and number identity.
- A first report fixes the task association; merged and closed are terminal.
- `UNCERTAIN` when a ledger write succeeds but the durable receipt cannot be
  completed; the same idempotency key reconciles.
- Disable fences new writes; uninstall deletes the installation's rows;
  workspace deletion removes every row for that workspace.
- `ListChangeRequestEvidenceExact` returns the caller's plugin rows with the real
  provider id and keeps GitHub failure and unsupported states visible.
- Required-store wiring, conformance, SDK transport, and the frozen contract
  documentation.

## Out of scope

- Forgejo or Gitea code in the core host.
- PR statistics, `Tasks.PullRequests`, parent or MCP summaries, a provider-neutral
  merged trigger, overview UI, or initial consumer opt-in.
- Implicit automation, merge, comment, or CI-rerun authority for plugin-reported
  records.
- A new provider-specific poller or per-task provider fan-out.

## Verification

- `go test ./internal/plugins -run "TestExactChangeRequest|TestExactEvidenceAndUsage" -count=1`
- `go test ./internal/plugins/state -run TestChangeRequest -count=1`
- `go test ./internal/persistence/storeconformance -run "TestStoreConformance/sqlite3/plugin-task-change-requests|TestStoreCatalogCompleteness|TestAdapterIDsMatchExpectedCatalogSet|TestOwnerBehaviorCoverage" -count=1`
- `go test ./... -count=1` and `go vet ./...` in `pluginsdk/`
- Changed-file Go lint and affected package compile checks.
- Formal upper-tier review: APPROVE.
- PostgreSQL conformance: skipped because the test DSN is not configured.
