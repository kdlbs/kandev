---
status: draft
system: integrations
created: 2026-09-15
owners:
  - kandev
---

# Alert Ingest Requirements

## Overview

Kandev turns external signals into tasks an agent can act on. Crash reporters,
error trackers and alert managers are a large class of such signals, and today
each one costs a full backend package. This capability makes an external alert
source a declaration rather than a package, so a source is added in days.

The integration system owns this contract because it owns connections to
external services and provider identity. The task system owns the resulting
Kandev task, and this capability consumes that contract without owning it.

## Terminology

- **Alert source:** A configured connection to an external system that emits
  alerts, for example a Sentry instance or a Datadog account.
- **Alert watch:** A user-configured rule on an alert source that decides which
  alerts become Kandev tasks, and with which workflow, agent and repository.
- **Descriptor:** The declared metadata for a source type: display name,
  category, authentication fields, defaults and fingerprint fields.
- **Normalized alert:** The single internal model every source produces,
  regardless of vendor.
- **Fingerprint:** The stable identity of an alert, derived from fields the
  descriptor declares, used to decide whether an alert has been seen before.
- **Generic webhook path:** The existing automation webhook trigger, used to
  ingest a source through configuration alone with no source type registered.

## Requirements

### REQ-INTEGRATIONS-ALERT-INGEST-001: Configuration-only ingest from a webhook-capable source

**Intent:** Any external system that can POST JSON becomes a usable task source
without shipping code, so a new vendor never waits on a release.

**User story:** As a Kandev operator, I want to point an external alerting system
at a webhook URL and have matching alerts become deduplicated, repository-bound
tasks, so that I do not need a native integration for every vendor.

#### Acceptance criteria

- **AC-INTEGRATIONS-ALERT-INGEST-001.1:** When a webhook trigger declares a
  dedup key path and an inbound payload resolves that path to a non-empty value,
  the system shall use the resolved value as the firing dedup key.
- **AC-INTEGRATIONS-ALERT-INGEST-001.2:** When a dedup key is in force and a
  later payload resolves to a key already recorded for that automation, the
  system shall skip the firing and record the skip with its reason, creating no
  second task.
- **AC-INTEGRATIONS-ALERT-INGEST-001.3:** When a webhook trigger declares no
  dedup key path, or the configured path is absent from the payload, the system
  shall fire without deduplication and record that the key was unresolved.
- **AC-INTEGRATIONS-ALERT-INGEST-001.4:** When a webhook trigger declares
  payload filters, the system shall fire only if every filter matches, and shall
  otherwise record a skipped firing naming the filter that rejected it.
- **AC-INTEGRATIONS-ALERT-INGEST-001.5:** When a webhook trigger declares a
  repository selector path, the system shall bind the created task only to a
  repository already configured on that automation, matching by the resolved
  value.
- **AC-INTEGRATIONS-ALERT-INGEST-001.6:** When a resolved repository selector
  matches no configured repository, the system shall create the task with no
  repository binding rather than resolving any other repository.
- **AC-INTEGRATIONS-ALERT-INGEST-001.7:** The system shall never resolve a
  repository named only by an inbound payload, because the webhook endpoint is
  authenticated by a shared secret rather than by a user identity.
- **AC-INTEGRATIONS-ALERT-INGEST-001.8:** When an inbound payload supplies text
  that reaches an agent prompt, the system shall render it as quoted data and
  the default prompt shall instruct the agent to treat it as data rather than as
  instructions.

### REQ-INTEGRATIONS-ALERT-INGEST-002: Declarative alert source descriptors

**Intent:** Adding a first-party source costs a declaration and one
normalization function, and never a table, a handler set or a settings page.

**User story:** As a Kandev contributor, I want to add a new alert source by
declaring its fields and writing one transform, so that I can ship a source in
days instead of weeks.

#### Acceptance criteria

- **AC-INTEGRATIONS-ALERT-INGEST-002.1:** The system shall register an alert
  source type from a descriptor and a normalization function, without a source
  specific database table, HTTP handler or settings page.
- **AC-INTEGRATIONS-ALERT-INGEST-002.2:** A descriptor shall declare each
  configuration field with its type, requiredness, description, default and
  whether it is sensitive.
- **AC-INTEGRATIONS-ALERT-INGEST-002.3:** When a configuration field is declared
  sensitive, the system shall store it through the shared secret store and shall
  never return its value from a read API.
- **AC-INTEGRATIONS-ALERT-INGEST-002.4:** The system shall emit a machine
  readable schema for every registered descriptor, and the source settings UI
  shall render its form from that schema rather than from source specific
  components.
- **AC-INTEGRATIONS-ALERT-INGEST-002.5:** When a descriptor is registered with a
  type identifier already in the registry, the system shall fail at startup
  rather than serve an ambiguous registry.
- **AC-INTEGRATIONS-ALERT-INGEST-002.6:** Registering a new source type shall
  require no database migration.
- **AC-INTEGRATIONS-ALERT-INGEST-002.7:** When a source declares an enrichment
  capability, the system shall call it after normalization and before task
  creation, and shall create the task from the unenriched alert if enrichment
  fails.

### REQ-INTEGRATIONS-ALERT-INGEST-003: Normalized alerts, deduplication and volume control

**Intent:** Deduplication, in-flight limits and workspace scoping are decided
once for every source, so a fix cannot land for one vendor and miss the others.

**User story:** As a Kandev operator, I want one alert to produce one task and a
crash storm to be bounded, so that an incident does not flood my board.

#### Acceptance criteria

- **AC-INTEGRATIONS-ALERT-INGEST-003.1:** Every registered source shall produce
  the same normalized alert model, and downstream deduplication, limiting and
  task creation shall read only that model.
- **AC-INTEGRATIONS-ALERT-INGEST-003.2:** The system shall compute an alert
  fingerprint from the fields the descriptor declares, and the same upstream
  alert delivered twice shall produce the same fingerprint.
- **AC-INTEGRATIONS-ALERT-INGEST-003.3:** When an alert fingerprint already has
  an open task for its watch, the system shall not create a second task.
- **AC-INTEGRATIONS-ALERT-INGEST-003.4:** When a watch declares a maximum number
  of open tasks, the system shall stop creating tasks for that watch once the
  count of open watcher-created tasks reaches it.
- **AC-INTEGRATIONS-ALERT-INGEST-003.5:** When a registered source does not
  supply the task-metadata key used to count its open tasks, the system shall
  reject the registration at startup rather than leave the limit unenforced.
- **AC-INTEGRATIONS-ALERT-INGEST-003.6:** When alert sources or alert watches
  are listed for a caller with a user identity, the system shall return only
  rows belonging to workspaces that caller can access.
- **AC-INTEGRATIONS-ALERT-INGEST-003.7:** When a task created from an alert is
  closed or archived, the system shall release the fingerprint so a later
  recurrence of the same alert can create a new task.

## Out of scope

- Migrating Jira, Linear, GitHub, GitLab or Azure DevOps onto this contract.
  Those own bidirectional issue and change synchronization, and their protocol
  clients, issue models and filter semantics stay separate.
- Outbound notification to external systems. This capability is inbound only.
- A native Firebase Crashlytics poller. Crashlytics exposes no issue list method
  and its read API is `v1alpha`, so it is served by the generic webhook path.
- Incident grouping, correlation or on-call routing. An alert produces at most
  one Kandev task; it does not produce an incident object.
- Replacing the task system's contract for task creation, workflow placement or
  agent dispatch.
