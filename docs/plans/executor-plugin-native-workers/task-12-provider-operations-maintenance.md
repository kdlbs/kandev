---
id: "12-provider-operations-maintenance"
title: "Provider status, inventory, events, logs, and maintenance (M4)"
status: pending
wave: 8
depends_on:
  - "11"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-OPS-001
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-OPS-001.1
  - AC-EXECUTORS-PLUGIN-OPS-001.2
  - AC-EXECUTORS-PLUGIN-OPS-001.3
  - AC-EXECUTORS-PLUGIN-OPS-001.4
  - AC-EXECUTORS-PLUGIN-OPS-001.5
  - AC-EXECUTORS-PLUGIN-OPS-001.6
  - AC-EXECUTORS-PLUGIN-OPS-001.7
  - AC-EXECUTORS-PLUGIN-OPS-001.8
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-operations-migration.md
---

# Task 12: Provider status, inventory, events, logs, and maintenance (M4)

## Summary

Add the bounded status, inventory, and log reads, the provider event callback into notifications, and the typed maintenance route with audit and idempotency. Absorbs the former deferred maintenance item.

## Size

L: ~650 source, ~1000 test lines.

## In scope

- Idempotency map entry holds digest and outcome for 24 hours (same body returns the recorded outcome, different body `conflict`, forgotten after restart); audit is a structured log line before and after; events go to instance administrators.
- Three read RPCs and admin routes with host-enforced size caps and log redaction.
- `PublishProviderEvent` with schema, token bucket, dedupe, and notification delivery.
- `ApplyProviderMaintenance` with closed actions, audit lines, and an idempotency digest.

## Out of scope

- Fleet-management UI beyond the status read; a persisted audit trail (open question).

## Acceptance

- Reads are bounded and read-only; events are rate-limited and secret-free; maintenance is admin-only and audited.
- An unavailable provider yields `unavailable` and launches continue.

## Failing-first tests

- Oversize status and log responses are rejected, not truncated.
- Event flood is limited and deduped (fake clock); free text in an event is rejected by schema.
- Same idempotency key with a different body is rejected; a non-admin is refused.
- Provider restart mid-read: `unavailable`; launches unaffected (fixture).

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/plugins/... ./cmd/plugin-fixture ./internal/notifications/... -count=1)
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1)
```

## Likely files and risks

- `apps/backend/internal/plugins`
- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin_callbacks.go`
- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- Risk: concurrency on both dialects; add the PostgreSQL race run (`KANDEV_TEST_POSTGRES_DSN`) before marking done.
