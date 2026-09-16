---
id: "04-alert-model-and-descriptor"
title: "Alert model, shared storage, and the source descriptor"
status: done
wave: 0
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-ALERT-INGEST-002
  - REQ-INTEGRATIONS-ALERT-INGEST-003
acceptance_criteria:
  - AC-INTEGRATIONS-ALERT-INGEST-002.2
  - AC-INTEGRATIONS-ALERT-INGEST-002.3
  - AC-INTEGRATIONS-ALERT-INGEST-002.4
  - AC-INTEGRATIONS-ALERT-INGEST-003.1
  - AC-INTEGRATIONS-ALERT-INGEST-003.2
  - AC-INTEGRATIONS-ALERT-INGEST-003.3
  - AC-INTEGRATIONS-ALERT-INGEST-003.7
system_design:
  - ../../specs/integrations/system-design/alert-ingest.md
---

# T04: Alert model, shared storage, and the source descriptor

## Outcome

The framework's foundation: one `Alert` model, three shared tables, and the
declarative descriptor that makes a source type a declaration rather than a
package.

This is one card rather than two because neither half ships anything on its own.
Splitting them produces two pull requests of unused scaffolding and two board
walks for one reviewable idea.

## In scope

New package `apps/backend/internal/integrations/alertsource`.

**Model and storage.**

- The `Alert` struct as specified, its field set derived from the Alertmanager
  payload shape.
- `alert_sources`, `alert_watches` and `alert_reservations`, with the partial
  unique index on `(watch_id, fingerprint) WHERE released_at IS NULL`.
- Fingerprint computation from a declared label key list, stable across
  deliveries and independent of key ordering.
- Reserve and release, with release driven by task close or archive.
- SQLite and Postgres parity, following the existing store test convention.

**Descriptor and field spec.**

- The `Descriptor` type including `FingerprintFields` and the mandatory
  `WatchMetadataKey`.
- A chained field spec builder with `NewStringField`, `NewIntField`,
  `NewBoolField`, `NewDurationField` and the modifiers `Secret`, `Default`,
  `Description`, `Example`, `Optional`, `Advanced`.
- Validation of a configuration map against a field spec, with typed errors.
- JSON Schema emission from the same declaration, for the frontend renderer.
- The `Source`, `Enricher` and `Poller` interfaces.

## Exclusions

- No registry, ingest service or HTTP surface. That is T06.
- No frontend consumption of the schema. That is T07.
- No concrete source implementation.
- No per-source column on any table. Source configuration lives only in
  `alert_sources.config_json`, which is what lets a new source type skip a
  migration entirely.

## Two constraints that carry the design

`alert_watches` is deliberately the 18 columns that `sentry_issue_watches` and
`linear_issue_watches` already share. Read both schemas before writing this one;
they are the evidence the shared shape exists rather than being invented here.

`WatchMetadataKey` is mandatory. The orchestrator's throttle gate counts open
watcher-created tasks by that key, and an empty value silently disables the
per-watch limit. Sentry shipped exactly that defect. Making the field required
here is what lets T06 reject an empty value at registration.

## Reference

The field spec builder follows Benthos `public/service` `NewConfigSpec`, the
closest Go precedent for declaration-driven configuration with secret marking
and schema emission. Read that API before designing this one:
`https://pkg.go.dev/github.com/redpanda-data/benthos/v4/public/service`

## Applicable specifications

- `REQ-INTEGRATIONS-ALERT-INGEST-002`, `AC-...-002.2`, `AC-...-002.3`, `AC-...-002.4`
- `REQ-INTEGRATIONS-ALERT-INGEST-003`, `AC-...-003.1`, `AC-...-003.2`, `AC-...-003.3`, `AC-...-003.7`
- [Alert ingest system design](../../specs/integrations/system-design/alert-ingest.md),
  Data and contracts, Persistence.

## Implementation acceptance conditions

1. The same alert delivered twice produces the same fingerprint regardless of
   label ordering, and the second reservation is rejected by the unique index
   rather than by a read check.
2. One field declaration produces both the validator and the emitted schema; no
   field is described twice, and a field declared `Secret` never appears with a
   value in emitted schema, examples, or any serialized read.
3. No source-specific column exists on any of the three tables.

## Verification

    make -C apps/backend test
    make -C apps/backend lint
    cd apps/backend && go test ./internal/integrations/alertsource/... -count=1

## Likely files

- `apps/backend/internal/integrations/alertsource/models.go`
- `apps/backend/internal/integrations/alertsource/store.go`
- `apps/backend/internal/integrations/alertsource/fingerprint.go`
- `apps/backend/internal/integrations/alertsource/descriptor.go`
- `apps/backend/internal/integrations/alertsource/fieldspec.go`
- `apps/backend/internal/integrations/alertsource/schema.go`
- Reference shapes: `internal/sentry/store_issue_watch.go`,
  `internal/linear/store_issue_watch.go`

## Dependencies

None. Runs in parallel with T01.

## Results

Implemented. `apps/backend/internal/integrations/alertsource` carries the
`Alert` model, the three shared tables (`alert_sources`, `alert_watches`,
`alert_reservations`) with the partial unique index on `(watch_id,
fingerprint) WHERE released_at IS NULL`, fingerprint computation, reserve/
release, and the declarative `Descriptor`/`FieldSpec` system with JSON Schema
emission. Went through 5 spec-review rounds (contract amendments A1-A7), 2
build rounds, and 2 review rounds before merge; all acceptance criteria have
passing SQLite and Postgres test coverage (`go test
./internal/integrations/alertsource/... -race -count=1`). No caller wires
`alertsource.NewStore` yet: registration with
`internal/persistence/requiredstores` is deferred to the initiative's later
work orders (T06/T07) that consume this framework.
