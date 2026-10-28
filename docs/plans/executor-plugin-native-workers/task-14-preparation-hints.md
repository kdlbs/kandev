---
id: "14-preparation-hints"
title: "Extensible workspace preparation (M6)"
status: pending
wave: 8
depends_on:
  - "13"
  - "07"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-PREPARATION-001
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-PREPARATION-001.1
  - AC-EXECUTORS-PLUGIN-PREPARATION-001.2
  - AC-EXECUTORS-PLUGIN-PREPARATION-001.3
  - AC-EXECUTORS-PLUGIN-PREPARATION-001.4
  - AC-EXECUTORS-PLUGIN-PREPARATION-001.5
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-operations-migration.md
---

# Task 14: Extensible workspace preparation (M6)

## Summary

Extend repository intent with checkout kind, sparse paths, and uninterpreted hints, and verify the returned inventory generically.

## Size

M: ~250 source, ~450 test lines.

## In scope

- Intent fields, input digest, generic verification, strategy label recording.

## Out of scope

- Any concrete strategy (mirror, sparse, pull request) in core.

## Acceptance

- A new strategy label or hint key needs no core change.

## Failing-first tests

- Two unseen strategy labels pass through the fixture with no host change.
- Returned set differs from the intent: `repository_inventory_mismatch` cleanup.
- Hint rule table: keys, values, counts, secrets.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/plugins/... ./cmd/plugin-fixture -count=1)
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1)
```

## Likely files and risks

- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin.go`
- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- Risk: concurrency on both dialects; add the PostgreSQL race run (`KANDEV_TEST_POSTGRES_DSN`) before marking done.
