---
id: "07-remove-host-write-api"
title: "Remove the unused Host write API"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-002
  - REQ-COSTS-USAGE-HISTORY-004
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-002.5
  - AC-COSTS-USAGE-HISTORY-004.1
system_design:
  - ../../specs/costs/system-design/usage-history.md
  - ../../specs/costs/system-design/token-usage.md
---

# Task 07: Remove the unused Host write API

## Summary

Remove the second write contract before the SDK publishes it, so `Report` is the only plugin write path. Make every remaining usage Host error go through `usagePublicError`. This closes a leak, found by reading the code, where raw errors reveal whether a task exists.

## In scope

- Remove `UsageReader.UpsertBatch`, `UsageReader.List`, the `UpsertSessionUsage` RPC, `sessionUsageSource.UpsertPluginSessionUsage`, and `ListPluginSessionUsageMeasurements`.
- Keep `ListCanonical` and its `ListSessionUsage` wire method.
- Regenerate protobuf code, and update `docs/public/plugins-authoring.md` and `docs/plans/plugins/GRPC-CONTRACT.md`.

## Out of scope

- The repository path, which Task 08 owns.

## Acceptance

- `rg 'UpsertBatch|UpsertSessionUsage' pluginsdk apps/backend/internal/plugins` finds no production match.
- A test covers task-not-found, foreign-task, and storage failures through the remaining usage Host paths, and asserts only public messages.
- The plugin builds and passes against the changed SDK.

## Verification

```bash
(cd pluginsdk && go test ./... -count=1)
(cd apps/backend && go test ./internal/plugins/... ./internal/analytics/service/... -count=1)
(cd "$PLUGIN_ROOT" && go test ./server/... -count=1)
bash scripts/lint-go-changed apps/backend/internal/plugins/host_data.go apps/backend/internal/plugins/host_usage_history.go
```

## Files likely touched

- `pluginsdk/usage.go`
- `pluginsdk/host.go`
- `pluginsdk/host_usage.go`
- `pluginsdk/host_call_classification.go`
- `pluginsdk/proto/kandev/plugin/v1/plugin.proto`
- `pluginsdk/proto/kandev/plugin/v1/plugin.pb.go`
- `pluginsdk/proto/kandev/plugin/v1/plugin_grpc.pb.go`
- `pluginsdk/host_data_test.go`
- `pluginsdk/serve_test.go`
- `apps/backend/internal/plugins/host_data.go`
- `apps/backend/internal/plugins/host_usage_history_test.go`
- `apps/backend/internal/plugins/host_usage_metric_applicability_test.go`
- `apps/backend/internal/analytics/service/service.go`
- `docs/public/plugins-authoring.md`
- `docs/plans/plugins/GRPC-CONTRACT.md`

## Dependencies

None.

## Risks

- This changes the SDK tree, so the plugin's `.kandev-sdk-ref` tree assertion must move. Task 13 sets the final pin. Until then the plugin's CI needs a pin that includes this commit.
- Confirm `pluginsdk` has not been released with this surface before removing it. The base commit `aebac5d` does not contain it.

## Parallelism

`parallel-safe`

## Inputs

- `apps/backend/internal/plugins/host_usage_history.go`: `usagePublicError`.
- [Token Usage design](../../specs/costs/system-design/token-usage.md), Host surface.

## Results

Removed `UsageReader.UpsertBatch`, `UsageReader.List`, the `UpsertSessionUsage` RPC and messages, the backend Host write/list adapters, and the now-unused public write result type.
Regenerated the pluginsdk protobuf tree.
Added `TestUsageHostErrorsArePublicOnly` across canonical reads, target discovery, and reports.
The plugin server tests compile and pass against the new SDK tree.

Validation: `(cd pluginsdk && go test ./...)`, `go test ./internal/plugins/... ./internal/analytics/service/...`, `go test ./server/...` in the plugin checkout, and `bash scripts/lint-go-changed` passed. `rg 'UpsertBatch|UpsertSessionUsage' pluginsdk apps/backend/internal/plugins --glob '!*_test.go'` has no matches.
