---
id: "09-datadog-source"
title: "Datadog alert source, and measurement of the per-source cost"
status: pending
wave: 4
depends_on:
  - "08-port-sentry"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-ALERT-INGEST-002
acceptance_criteria:
  - AC-INTEGRATIONS-ALERT-INGEST-002.1
  - AC-INTEGRATIONS-ALERT-INGEST-002.6
system_design:
  - ../../specs/integrations/system-design/alert-ingest.md
---

# T09: Datadog alert source, and measurement of the per-source cost

## Outcome

Datadog is available as an alert source, delivered purely as a descriptor plus a
normalization function, and the cost of adding it is measured.

This work order exists as much to produce evidence as to produce a source. Its
recorded result is what substantiates or refutes the plan's goal.

## In scope

- A Datadog descriptor: API and application key fields declared `Secret`, site
  field with a default, fingerprint fields chosen from Datadog's alert identity.
- `Normalize` mapping a Datadog webhook payload to the `Alert` model.
- `Enrich` where a payload lacks context that the Datadog API can supply.
- Fixtures covering a firing alert, a recovery, and a payload with a missing
  optional field.
- A recorded measurement: production lines added, test lines added, and elapsed
  implementation time.

## Exclusions

- No Datadog dashboards, metrics queries or APM browsing. This is alert ingest
  only.
- No frontend work. If any is required, that is a T07 defect.
- No new table or migration. If either is required, that is a T04 defect.

## Applicable specifications

- `REQ-INTEGRATIONS-ALERT-INGEST-002`, `AC-INTEGRATIONS-ALERT-INGEST-002.1`,
  `AC-INTEGRATIONS-ALERT-INGEST-002.6`
- [Alert ingest system design](../../specs/integrations/system-design/alert-ingest.md).

## Implementation acceptance conditions

1. Datadog is added with no new table, no migration and no frontend change.
2. A recovery payload does not create a task, and the corresponding firing
   payload does.
3. The recorded production line count for the source is reported in Results,
   whatever it turns out to be.

## Verification

    make -C apps/backend test
    make -C apps/backend lint
    cd apps/backend && go test ./internal/integrations/alertsource/... -count=1

## Likely files

- `apps/backend/internal/integrations/alertsource/sources/datadog/`
- Fixtures alongside the source

## Dependencies

T08. Adding a second source before the first is ported would validate the
framework only against a source chosen to fit it.

## Results

Not started.
