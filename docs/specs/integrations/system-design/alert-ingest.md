---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-ALERT-INGEST-001
  - REQ-INTEGRATIONS-ALERT-INGEST-002
  - REQ-INTEGRATIONS-ALERT-INGEST-003
---

# Alert Ingest System Design

## Purpose and boundaries

The integration system owns the inbound contract for external alert sources:
source configuration, credentials, the normalized alert model, fingerprint
deduplication and per-watch volume limits.

It does not own task creation. The orchestrator's `WatcherSource` and
`IssueTaskRequest` contract already converges five integrations at task
creation, and this design reuses it unchanged. The contribution here is a second
convergence point at ingest, so a source no longer carries a bespoke stack from
its HTTP handler to its own table.

Adjacent contracts used but not owned: the task system for the created task, the
auth system for identity and workspace authorization, the secret store for
credentials, the plugin system for out-of-tree adapters, and the automation
system for the generic webhook trigger.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-ALERT-INGEST-001` | [Track A: generic webhook path](#track-a-generic-webhook-path) |
| `REQ-INTEGRATIONS-ALERT-INGEST-002` | [Descriptor and registry](#descriptor-and-registry) |
| `REQ-INTEGRATIONS-ALERT-INGEST-003` | [Normalized alert and fingerprinting](#normalized-alert-and-fingerprinting) |

## Components and responsibilities

The capability is delivered on two tracks that share the normalized alert model.
Track A is usable on its own and ships first; Track B does not block it.

### Track A: generic webhook path

Extends the existing automation webhook trigger. No new package.

- **Webhook handler** (`internal/automation/webhook.go`): resolves the dedup key
  and filter verdict from the inbound payload before firing, and passes the
  resolved key into the existing admission path.
- **Trigger configuration** (`internal/automation/trigger_registry.go`): the
  webhook trigger's config gains `dedup_key`, `filters` and `repository`.
- **Repository resolution** (`internal/orchestrator/event_handlers_automation.go`):
  the payload-driven branch currently gated on `github_pr` is generalized, and
  constrained for webhooks to the automation's configured repository set.

### Track B: alert source framework

New package `internal/integrations/alertsource`.

- **Descriptor registry:** holds one descriptor per source type, keyed by type
  identifier, populated at startup. Registration is rejected on a duplicate
  identifier or a missing watch metadata key.
- **Field spec:** the declarative configuration builder. Produces validation and
  a machine readable schema from the same declaration.
- **Source store:** one shared store for `alert_sources` and `alert_watches`,
  plus the fingerprint reservation table. No source owns a table.
- **Ingest service:** accepts a raw payload or a poll result, calls the source's
  `Normalize`, optionally `Enrich`, computes the fingerprint, reserves it, and
  publishes a `NewAlertEvent`.
- **Generic handlers:** CRUD for sources and watches under one route group,
  with workspace scoping applied in the service layer.
- **Poller:** one loop over enabled watches whose source declares a `Poll`
  capability. Sources that are webhook-only are skipped.
- **`AlertWatcherSource`:** the single `WatcherSource` implementation in
  `internal/orchestrator`, replacing one implementation per vendor.
- **Settings form renderer** (`apps/web`): renders a source's configuration form
  from its emitted schema. One component for all source types.

## Data and contracts

### Normalized alert

The field set starts from the Prometheus Alertmanager payload shape, so a source
that already emits that shape needs no adapter.

    type Alert struct {
        SourceID    string
        SourceType  string
        Status      string            // firing | resolved
        Labels      map[string]string // identity dimensions
        Annotations map[string]string // human text: summary, description
        StartsAt    time.Time
        EndsAt      time.Time
        GeneratorURL string           // permalink into the vendor UI
        ExternalID  string            // vendor issue id when it has one
        Severity    string
        Context     string            // stack trace or payload excerpt
        Raw         json.RawMessage   // original payload, retained for replay
    }

`Labels` carries identity, `Annotations` carries prose, and `Context` carries
the body an agent needs. `Raw` is retained so a normalization fix can be
replayed against stored payloads without asking the vendor to resend.

### Descriptor

    type Descriptor struct {
        Type            string      // stable identifier, e.g. "datadog"
        DisplayName     string
        Category        string      // "crash", "apm", "alerting"
        Config          *FieldSpec  // declared configuration
        FingerprintFields []string  // label keys forming alert identity
        WatchMetadataKey  string    // task-metadata key for the in-flight count
        Capabilities    Capabilities // Webhook, Poll, Enrich
    }

`WatchMetadataKey` is mandatory. The existing throttle gate counts open tasks by
this key, and an empty value silently disables the per-watch limit. Registration
rejects an empty value so the class of defect that left Sentry's cap unenforced
cannot recur.

### Field spec

A chained builder in the shape Benthos uses, because Go has no declarative
metadata equivalent to Pydantic.

    NewFieldSpec().
        Field(NewStringField("api_token").Secret().Description("...")).
        Field(NewStringField("site").Default("datadoghq.com")).
        Field(NewIntField("poll_interval_seconds").Default(300).Advanced())

The same declaration produces configuration validation, the JSON schema served
to the frontend, and the generated documentation. A field marked `Secret` is
written through the shared secret store and never returned by a read API.

### Source interface

    type Source interface {
        Descriptor() Descriptor
        Normalize(ctx context.Context, payload []byte) ([]Alert, error)
    }

    type Enricher interface {  // optional
        Enrich(ctx context.Context, cfg Config, a *Alert) error
    }

    type Poller interface {    // optional
        Poll(ctx context.Context, cfg Config, since time.Time) ([]Alert, error)
    }

A webhook-only source implements `Source` alone. Crashlytics is such a source:
its alert payload carries no stack trace and it exposes no issue list method, so
it has no `Poll` and its `Enrich` is optional and best effort.

### HTTP surface

    GET    /api/v1/alert-sources/types                 registered descriptors and schemas
    GET    /api/v1/alert-sources?workspace_id=          list sources
    POST   /api/v1/alert-sources                        create
    PATCH  /api/v1/alert-sources/:id                    update
    DELETE /api/v1/alert-sources/:id                    delete
    GET    /api/v1/alert-sources/:id/watches            list watches
    POST   /api/v1/alert-sources/:id/watches            create watch
    POST   /api/v1/alert-sources/:id/ingest             inbound webhook delivery

Only the ingest route is exempt from session authentication. It authenticates on
a per-source secret compared in constant time, matching the existing automation
webhook rule.

## Control flow

Inbound delivery:

1. The vendor POSTs to the source's ingest route.
2. The handler authenticates the per-source secret and caps the body.
3. The ingest service calls `Normalize`, producing zero or more alerts.
4. For each alert, `Enrich` runs when declared, and failure is non-fatal.
5. The fingerprint is computed from the descriptor's declared label keys.
6. The fingerprint is reserved for the matching watch. A reservation that
   already exists ends processing for that alert.
7. The per-watch in-flight limit is checked against open tasks counted by
   `WatchMetadataKey`.
8. A `NewAlertEvent` is published.
9. `AlertWatcherSource` builds the `IssueTaskRequest` and the orchestrator
   creates and starts the task through its existing path.

Polling follows the same steps from step 3, driven by the watch's interval.

Track A follows an independent path: the automation webhook handler resolves the
dedup key and filters, then uses the existing `FireTrigger` admission and the
orchestrator's automation task creation. It does not produce an `Alert`.

## Failure and recovery

- **Normalization failure:** the delivery is rejected with a 4xx and the raw
  payload is retained with the error, so a descriptor fix can replay it.
- **Enrichment failure:** non-fatal. The task is created from the unenriched
  alert with the failure noted in the task body, because a crash with no stack
  trace is still worth surfacing.
- **Reservation race:** the reservation is a unique constraint on
  `(watch_id, fingerprint)`, so concurrent deliveries resolve to one task.
- **In-flight limit reached:** the alert is recorded as skipped with its reason
  and no task is created. Skips are visible rather than silent.
- **Vendor retry:** a vendor that retries a delivery produces the same
  fingerprint and is absorbed by the reservation, so retries are safe.
- **Source credential invalid:** the watch is disabled with the error stamped on
  it, matching the existing self-heal behavior.

## Persistence

Three shared tables replace the per-vendor pairs.

    alert_sources    id, workspace_id, type, name, config_json,
                     enabled, health_status, last_error, last_error_at,
                     created_at, updated_at
                     UNIQUE(workspace_id, name)

    alert_watches    id, source_id, workspace_id, workflow_id, workflow_step_id,
                     repository_id, base_branch, filter_json, agent_profile_id,
                     executor_profile_id, prompt, enabled,
                     poll_interval_seconds, max_inflight_tasks,
                     last_polled_at, last_error, last_error_at,
                     created_at, updated_at

    alert_reservations  id, watch_id, fingerprint, task_id, alert_raw,
                        created_at, released_at
                        UNIQUE(watch_id, fingerprint) WHERE released_at IS NULL

`alert_watches` is deliberately the 18 columns that `sentry_issue_watches` and
`linear_issue_watches` already share. Source-specific configuration lives in
`alert_sources.config_json`, validated against the descriptor's field spec, so a
new source type needs no migration.

`alert_reservations.released_at` is set when the created task is closed or
archived, which is what lets a recurrence of the same alert open a new task.
`alert_raw` retains the original payload for replay and is subject to the same
retention policy as automation runs.

Sentry's existing tables are migrated to these in the porting work order.
Migration is one-way and copies rows; the Sentry tables are dropped only after
the ported path is verified.

## Security

- **The ingest route is unauthenticated by identity.** It is authorized by a
  per-source secret only, so nothing it carries may name a privileged object.
  This is the reason repository binding on the Track A webhook path selects from
  a configured allowlist rather than resolving a repository named in the
  payload. The existing `github_pr` payload-driven resolution is safe only
  because Kandev's own poller produced that payload.
- **Secrets** declared `Secret` in a field spec are written through the shared
  secret store, keyed per source id, and never returned by a read API.
- **Workspace scoping** is applied in the service layer for every list
  operation, so an unscoped list cannot leak rows from another user's
  workspaces. This closes the gap where the same filter exists in Jira, Linear
  and Slack but is missing from Sentry and GitLab.
- **Prompt injection:** alert titles and bodies are attacker-influenceable text
  reaching an agent prompt. Alert-derived text is rendered as quoted data, and
  the default prompt instructs the agent to treat it as data.
- **Body cap** on ingest matches the existing 1 MB webhook cap.

## Observability

Counters under `alert_ingest_*`, each labelled by `source_type` only, so no
alert identifier, workspace or repository becomes a label:

- `alert_ingest_received_total` labelled by `source_type` and `outcome`
  (`accepted`, `deduplicated`, `filtered`, `capped`, `normalize_failed`).
- `alert_ingest_enrich_failed_total` labelled by `source_type`.
- `alert_ingest_tasks_created_total` labelled by `source_type`.
- `alert_ingest_reservation_released_total` labelled by `source_type`.

Each is also emitted as a structured log, matching the existing convention for
routing and stall metrics. A skipped alert is always counted, because the
failure mode this capability most needs to expose is a source that quietly stops
producing tasks.

## Related decisions

- [ADR 2026-09-15 declarative alert ingest](../../../decisions/2026-09-15-declarative-alert-ingest.md)
- [ADR workspace-scoped integration settings](../../../decisions/0030-workspace-scoped-integration-settings.md)
