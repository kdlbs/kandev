---
id: "06-registry-ingest-and-http"
title: "Registry, ingest service and shared HTTP surface"
status: pending
wave: 1
depends_on:
  - "04-alert-model-and-descriptor"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-ALERT-INGEST-002
  - REQ-INTEGRATIONS-ALERT-INGEST-003
acceptance_criteria:
  - AC-INTEGRATIONS-ALERT-INGEST-002.1
  - AC-INTEGRATIONS-ALERT-INGEST-002.5
  - AC-INTEGRATIONS-ALERT-INGEST-002.6
  - AC-INTEGRATIONS-ALERT-INGEST-002.7
  - AC-INTEGRATIONS-ALERT-INGEST-003.4
  - AC-INTEGRATIONS-ALERT-INGEST-003.5
  - AC-INTEGRATIONS-ALERT-INGEST-003.6
system_design:
  - ../../specs/integrations/system-design/alert-ingest.md
---

# T06: Registry, ingest service and shared HTTP surface

## Outcome

A registered source type is reachable end to end: an inbound payload becomes a
normalized, deduplicated, limited alert that creates a task through the existing
orchestrator path.

## In scope

- Descriptor registry with startup registration, rejecting a duplicate type
  identifier and rejecting an empty `WatchMetadataKey`.
- Ingest service implementing the control flow: normalize, optional enrich,
  fingerprint, reserve, limit check, publish `NewAlertEvent`.
- `AlertWatcherSource` in `internal/orchestrator`, implementing `WatcherSource`
  once for all sources, returning the descriptor's `WatchMetadataKey` from
  `WatchMetadataKey()`.
- Shared HTTP handlers for source and watch CRUD plus the ingest route, with the
  ingest route authorized by a per-source secret in constant time and the CRUD
  routes workspace scoped in the service layer.
- One poller loop over watches whose source declares `Poll`.
- A shared mock source and mock controller, replacing the per-vendor pair.

## Exclusions

- No settings UI. That is T07.
- No migration of Sentry. That is T08.
- No concrete source implementations.

## Applicable specifications

- `REQ-INTEGRATIONS-ALERT-INGEST-002`, `REQ-INTEGRATIONS-ALERT-INGEST-003`
- `AC-INTEGRATIONS-ALERT-INGEST-002.1`, `AC-INTEGRATIONS-ALERT-INGEST-002.5`,
  `AC-INTEGRATIONS-ALERT-INGEST-002.6`, `AC-INTEGRATIONS-ALERT-INGEST-002.7`,
  `AC-INTEGRATIONS-ALERT-INGEST-003.4`, `AC-INTEGRATIONS-ALERT-INGEST-003.5`,
  `AC-INTEGRATIONS-ALERT-INGEST-003.6`
- [Alert ingest system design](../../specs/integrations/system-design/alert-ingest.md),
  Components, Control flow, Failure and recovery, Observability.

## Implementation acceptance conditions

1. Registering a descriptor with an empty `WatchMetadataKey`, or with a type
   identifier already present, fails at startup with a named error.
2. A delivery that exceeds the watch's in-flight limit creates no task and
   increments `alert_ingest_received_total` with outcome `capped`.
3. A failing `Enrich` still creates the task from the unenriched alert and
   increments `alert_ingest_enrich_failed_total`.

## Verification

    make -C apps/backend test
    make -C apps/backend lint
    cd apps/backend && go test ./internal/integrations/alertsource/... ./internal/orchestrator/... -count=1

## Likely files

- `apps/backend/internal/integrations/alertsource/registry.go`
- `apps/backend/internal/integrations/alertsource/service.go`
- `apps/backend/internal/integrations/alertsource/handlers.go`
- `apps/backend/internal/integrations/alertsource/poller.go`
- `apps/backend/internal/orchestrator/source_alert.go`
- `apps/backend/cmd/kandev/services.go` for the `initAlertSourceService` helper
- `apps/backend/internal/auth/httpmw/middleware.go` for the ingest route exemption

## Dependencies

T04, T05.

## Results

Not started.
