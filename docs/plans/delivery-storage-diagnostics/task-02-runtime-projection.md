---
id: "02-runtime-projection"
title: "Project owned storage observations and failures"
status: done
wave: 2
depends_on:
  - "01-source-diagnostics"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.2
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.4
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.5
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.6
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.1
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.2
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.3
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.4
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.5
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.6
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.7
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.8
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.9
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.10
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.11
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.12
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.13
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.14
system_design:
  - ../../specs/platform/system-design/delivery-storage-diagnostics.md
---

# Task 02: Project owned storage observations and failures

## Summary

Preserve terminal storage evidence through existing error metadata and history.
Expose current pressure through one bounded lifecycle observer, runtime DTOs, and session-authorized events.

## In scope

- One 30-second warning observer with bounded remote requests and two-second per-request deadlines.
- Read source-cached capacity through each owning executor's advisory route, including provider-verified Sprites without durable delivery.
- Preserve separate source namespaces for local, SSH, Sprites, Docker, and Kubernetes, even when filesystem IDs or profiles match.
- Reuse active executor connections without starting, restoring, or waking inactive remote environments for monitoring.
- A 90% advisory threshold and below-85% clearance, independent of write admission and actual producer pressure.
- Current execution, native generation, and runtime epoch checks before accepting status results.
- A bounded runtime-only snapshot provider for session detail and boot data.
- `task_session.runtime_storage_changed` event registration and authorized gateway routing.
- `AgentErrorCause.storage` normalization, equality, terminal forwarding, history, and summary projection.
- Semantic update deduplication, nonhealthy freshness refresh, release, and restart behavior.
- Observer-independent SQL failure handling and unknown/older-peer behavior.

## Out of scope

Rendered UI, browser source polling, SQL migrations, cleanup, or changes to process liveness and prompt settlement.

## Acceptance

1. Agentctl source samples drive the 90% advisory and below-85% clearance. Twenty local instances can share a source sample, while twenty separate Sprites stay independent.
2. Terminal snapshots survive metadata/history/summary round trips. A newer owner rejects old results, and a healthy observation cannot settle a failure.
3. Observations survive blocked journal and SQL paths. Advisory sampling leaves write checks unchanged, and failed or replaced sources cannot create a false healthy state.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -race -trimpath -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/models ./internal/task/dto ./internal/task/service ./internal/task/statussummary ./internal/gateway/websocket ./internal/events/bus ./internal/backendapp -run 'DeliveryStorage|AgentErrorCause|DeliveryPressure' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go apps/backend/internal/orchestrator/event_handlers_agent.go apps/backend/internal/task/models/launch_errors.go apps/backend/internal/task/dto/dto.go apps/backend/internal/task/service/service_status_summary_rebuild_input.go apps/backend/internal/task/statussummary/projector.go apps/backend/internal/gateway/websocket/hub.go apps/backend/internal/events/types.go apps/backend/internal/backendapp/metrics_provider.go
git diff --check
```

Use `/tdd`. Prefix new observer, projection, routing, boot, and persistence tests with `TestDeliveryStorage`.
Test real event serialization and both memory/NATS-like routing semantics.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go`, `event_types.go`, and `manager_events.go`.
- New lifecycle `delivery_storage_observer.go` and `delivery_storage_observer_test.go`.
- Source sampler and advisory-route contracts from Task 01; the backend consumes their results without native filesystem reads.
- `apps/backend/internal/agent/runtime/lifecycle/streams.go` where terminal evidence crosses callbacks.
- `apps/backend/internal/task/models/launch_errors.go` and cause normalization/equality tests.
- `apps/backend/internal/task/dto/dto.go` and a new runtime snapshot provider beside `cancellation_pending.go`.
- `apps/backend/internal/orchestrator/watcher/watcher.go`, `event_handlers_agent.go`, and `event_handlers_streaming.go`.
- `apps/backend/internal/task/service/service_status_summary_rebuild_input.go` and history builders.
- `apps/backend/internal/events/types.go`, relevant gateway subscribers, and `apps/backend/internal/backendapp/` wiring.

## Dependencies

[Task 01](task-01-source-diagnostics.md).

## Risks

- Polls can race with native generation changes or a replaced shared agentctl.
- Callbacks must not acquire lifecycle locks while an event publisher holds them.
- A full SQL filesystem can prevent historical persistence. Live observations must remain observational and available without claiming durability.
- Other branches can introduce an observer first. Reuse that owner instead of adding a second loop.
- Verify the actual journal filesystem before sample reuse. Display settings can select another filesystem, and remote roots can differ from journal mounts.
- Sprites uses provider-owned proxy identity instead of an agentctl token. Do not reject it solely because token authentication is absent.
- Test 89.9%, 90%, 88% after warning, 85%, and 84.9%, plus unknown samples and the independent write-admission cache.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/delivery-storage-diagnostics.md), requirements 001 and 002.
- [Design](../../specs/platform/system-design/delivery-storage-diagnostics.md), Wire and persistence, Live observations, and Ownership and failure behavior.
- Existing cancellation snapshot provider, lifecycle loop lifetime, and typed error cause normalization patterns.

## Results

The 30-second lifecycle observation, hysteresis projection, session DTO and boot hydration, session-authorized event routing, and typed terminal cause projection are implemented. Targeted race tests and changed-Go lint pass.
