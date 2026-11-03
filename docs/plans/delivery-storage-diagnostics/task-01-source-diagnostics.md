---
id: "01-source-diagnostics"
title: "Classify source storage diagnostics"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.1
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.2
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.3
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.4
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.6
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.11
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.12
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.13
system_design:
  - ../../specs/platform/system-design/delivery-storage-diagnostics.md
---

# Task 01: Classify source storage diagnostics

## Summary

Create source-owned storage reasons and bounded evidence without changing admission behavior.
Carry them through authenticated HTTP/status and host-generated terminal events.

## In scope

- Typed journal errors for physical space, quota, logical limits, and oversized output.
- Existing sentinel compatibility through `errors.Is` and unchanged HTTP status/code handling.
- A validated optional `DeliveryStorageDiagnostic` wire type and failure-time measurements.
- Host-owned attachment after adapter normalization, with no classification of provider prose.
- `DeliveryHTTPError` decoding and retained total/available capacity fields in the backend status client.
- Optional source-scoped filesystem identity and sample time for later verified sample sharing.
- Independent `RuntimeStorageSnapshot` version 1, source-side shared sampler, and a read-only `/api/v1/runtime/storage` route.
- Executor-local journal/workspace selection and a no-journal Sprites fixture, using existing instance authorization or provider-verified transport.
- Tests against fake capacity readers and actual serialization boundaries.

## Out of scope

Backend observation scheduling, session persistence, UI, or changed reserves and quotas.

## Acceptance

1. Physical, logical, quota, and oversized cases produce distinct diagnostics through real source/API/client paths.
2. Unknown values and old peers stay unknown. Provider text, unsafe fields, and malformed measurements cannot create a disk claim.
3. Twenty concurrent advisory requests sharing one source filesystem reuse one read. Separate executor namespaces stay separate, and existing write-safety behavior remains unchanged.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -race -trimpath -tags fts5 ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agentctl/server/instance ./internal/agentctl/types/streams ./internal/system/metrics ./internal/agent/runtime/agentctl -run 'DeliveryStorage|DiskReserve|DiskCapacity' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agentctl/journal/journal.go apps/backend/internal/agentctl/server/process/delivery_pressure.go apps/backend/internal/agentctl/server/api/durable_delivery.go apps/backend/internal/agentctl/server/instance/manager.go apps/backend/internal/agentctl/types/streams/agent.go apps/backend/internal/system/metrics/collector.go apps/backend/internal/agent/runtime/agentctl/client.go
git diff --check
```

Use `/tdd`. New source, API, status, and decoder tests use `TestDeliveryStorage*` names.
Read scoped backend and agentctl guidance, including the API guidance, before implementation.

## Files likely touched

- `apps/backend/internal/agentctl/journal/journal.go`, `disk_capacity.go`, `pressure.go`, and `journal_submissions.go`.
- New journal `storage_error.go` and `storage_error_test.go`.
- New `apps/backend/internal/agentctl/types/streams/delivery_storage.go` and its tests.
- New `apps/backend/internal/agentctl/types/streams/runtime_storage.go` and source snapshot tests.
- New `apps/backend/internal/system/metrics/disk_capacity_sampler.go` and source sample sharing tests.
- New `apps/backend/internal/agentctl/server/process/runtime_storage.go` and `server/api/runtime_storage.go`, with route registration and tests.
- `apps/backend/internal/agentctl/server/instance/manager.go` and `server/config/config.go` for a shared source sampler across instances.
- `apps/backend/internal/agentctl/types/streams/agent.go`.
- `apps/backend/internal/agentctl/server/process/delivery_pressure.go`, `delivery_batch.go`, and host error emission in `manager.go`.
- `apps/backend/internal/agentctl/server/api/durable_delivery.go` and status response coverage.
- `apps/backend/internal/agent/runtime/agentctl/client.go`, `client_delivery.go`, and their tests.

## Dependencies

None.

## Risks

- A fresh capacity sample can differ from the failed write's evidence.
- Wrapping an error must preserve retry classifications and `errors.Is` behavior.
- New diagnostic fields must not change retained event identity on replay.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/delivery-storage-diagnostics.md), requirement 001 and source sampling criteria 002.11-002.13.
- [Design](../../specs/platform/system-design/delivery-storage-diagnostics.md), Source classification and Wire and persistence.
- Existing `disk_capacity_test.go`, `delivery_pressure_test.go`, and `client_delivery_test.go` patterns.

## Results

Source classification, bounded wire evidence, shared sampler, read-only runtime storage route, and status/client retention are implemented. Targeted race tests and changed-Go lint pass.
