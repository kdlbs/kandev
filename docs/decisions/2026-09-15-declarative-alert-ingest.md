# ADR-2026-09-15-declarative-alert-ingest: Declarative alert ingest instead of per-vendor integration packages

**Status:** proposed
**Date:** 2026-09-15
**Area:** integrations

## Context

Kandev turns external signals into tasks an agent can act on. Today each
external source is a full backend package. Sentry is 5,681 production lines and
5,701 test lines; Jira is 6,064 production lines; Linear is 4,814.

Only a minority of that is about the vendor. In Sentry, `rest_client.go` and
`models.go` total 995 lines, roughly 17 percent. The remaining 83 percent is a
per-vendor reimplementation of concerns that are not vendor specific: its own
SQL tables, its own store CRUD, its own HTTP handlers, its own mock client and
mock controller, its own settings page and hooks.

The duplication is measurable. `sentry/store_issue_watch.go` and
`linear/store_issue_watch.go` expose 14 identically named functions out of 15.
`sentry_issue_watches` and `linear_issue_watches` share 18 identical columns,
including column comments that are copied verbatim between the two files.

The duplication has already produced divergence:

- Sentry gained `ClearIssueWatchError` and `StampIssueWatchError`. Linear did
  not.
- The per-watch in-flight cap silently did not apply to Sentry, because the
  repository counted open tasks by a metadata key the source did not return.
- `ListAllIssueWatches` gained per-user workspace scoping in Jira, Linear and
  Slack. Sentry and GitLab still lack it, so an authorization filter exists in
  three copies and is missing from two.

The near-term demand is a set of alerting and crash-reporting sources:
Crashlytics, Datadog, Grafana Alertmanager, Bugsnag, Rollbar, New Relic, Better
Stack and PagerDuty. At the current cost per source that is unaffordable, and
each new copy widens the divergence surface above.

Kandev already has half of the required abstraction. `WatcherSource` and
`IssueTaskRequest` in `internal/orchestrator` are a normalized contract that
five integrations already implement. The convergence happens only at the last
step, at task creation. Every source still carries a bespoke stack from its HTTP
handler down to its own database table.

Comparable open-source systems converge on the opposite arrangement. Keep
normalizes every one of 132 integrations to a single `AlertDto`; a provider
declares `FINGERPRINT_FIELDS` and implements one `_format_alert` function, and a
minimal provider is 40 to 60 lines. Grafana OnCall makes the per-vendor unit a
template rather than code, and ships a generic webhook integration so an
unlisted source never waits for a release. Airbyte reports a drop from about
three hours for a language-specific connector to about thirty minutes for a
declarative manifest, and states the reason as fixing each bug once.

## Decision

An external alert source is a declaration, not a package.

1. **One normalized alert model.** All inbound sources produce a single
   `AlertDTO`. Its field set starts from the Prometheus Alertmanager payload
   shape, because a large part of the ecosystem already emits it, so accepting
   that shape natively admits several sources with no adapter at all.

2. **One persistence and HTTP surface.** Alert sources and alert watches live in
   shared tables with a shared store, shared handlers, and shared mock control.
   A source does not own a table.

3. **Declared, not implemented, behavior.** A source supplies a descriptor: its
   display name, category, authentication fields with sensitivity flags,
   defaults, and the fingerprint field list used for deduplication. The
   descriptor drives validation, documentation and the settings UI. The only
   vendor code is a `Normalize` function and an optional `Enrich` call used when
   the alert payload does not carry enough context.

4. **A generic webhook path that never blocks.** The existing automation webhook
   trigger gains a dedup key, payload filters, and repository binding, so a
   source with an outbound webhook is usable through configuration alone, before
   and independently of any descriptor work.

5. **Plugins remain the out-of-tree escape hatch, on the same contract.** The
   plugin host already exposes `HandleWebhook` with raw headers plus
   `CreateTask`, `RevealSecret` and state storage, which covers vendor HMAC
   verification, enrichment call-backs and durable dedup. A plugin adapter emits
   the same `AlertDTO`, so in-tree and out-of-tree adapters are interchangeable.

Sentry is ported onto the framework as the proof. A framework validated only
against new sources would not be evidence that it fits the sources Kandev
already has.

## Consequences

A new alert source becomes a descriptor plus one normalization function, on the
order of 150 to 300 lines, instead of a package of roughly 5,700. Framework
defects are fixed once instead of once per vendor, which is the direct remedy
for the three divergences recorded above.

The field-descriptor type and the schema-driven settings form are new
infrastructure with no Go equivalent to copy from Pydantic. Benthos demonstrates
the pattern in Go: a chained config spec with `Secret`, `Default`,
`Description`, `Example`, `Optional` and `Advanced`, plus schema emission and a
config linter. That renderer is the largest single piece of this work and is
paid once.

Sources whose upstream contract is genuinely richer than an alert stream stay
where they are. Jira, Linear, GitHub, GitLab and Azure DevOps own bidirectional
issue and change synchronization; this decision does not migrate them, and their
protocol clients, issue models and filter semantics remain deliberately separate
per the existing guidance that JQL and Linear structured filters must not merge.

Crashlytics specifically is served by the generic webhook path and not by a
descriptor with a poller, because its alert payload carries no stack trace, its
read API is `v1alpha`, and it exposes no issue list method to poll.

## Alternatives Considered

- **Build each vendor as its own package.** Predictable and already understood,
  but at roughly 5,700 production lines per source the named vendor list is
  unaffordable, and every copy adds another place for a fix to be missed.
- **Extract only the shared watcher layer from the existing integrations.** This
  removes about 20 to 25 percent per source. It is worth doing and is subsumed
  by this decision, but on its own it leaves each source owning a table, handler
  set, mock and settings page, so it does not reach a days-per-source cost.
- **Generic webhook only, with no framework.** Cheapest and lands first, and it
  is retained here as the first track. Alone it cannot poll, cannot enrich a
  thin payload, and cannot present per-source settings, so it does not serve a
  source such as Bugsnag or Rollbar with a real issue API.
- **Plugins only, with no in-tree registry.** Moves all cost out of core but
  requires a separate repository, release and install for every source, which is
  a worse trade for a first-party source that is only a descriptor.

## Related design

- [Alert ingest](../specs/integrations/system-design/alert-ingest.md)
