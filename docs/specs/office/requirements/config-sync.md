---
status: draft
system: office
created: 2026-09-01
owners:
  - kandev
---

## Overview

An Office workspace's configuration (its agents, skills, projects, and
routines) can be kept in lockstep with definition files committed to a
repository, so configuration changes are reviewed, versioned, and rolled out
like code. The repository is reached through the workspace's own GitHub or
GitLab connection, on the same polled/forced cadence and with the same status
vocabulary as the workflow sync that already ships.

Office owns this contract because the durable outcome is *what an Office
workspace's configuration is and where it comes from*. Provider connections
(hosts, credentials, auth methods) are owned by the
[integration system](../../integrations/README.md); this capability consumes
them and stores none of its own.

Today Office has two unrelated "sync" surfaces, neither provider-aware: a
filesystem-to-database diff page, and a raw `git clone/pull/push` section
shelling out to `git` in `<basePath>/workspaces/<name>` with whatever ambient
credentials the backend inherited. Neither carries a provider, a
workspace-routed credential, a self-managed host, a schedule, or an outcome.

## Terminology

- **Config entity:** One agent, skill, project, or routine belonging to an
  Office workspace. These four kinds are the sync unit.
- **Config source:** The repository coordinates a workspace syncs from:
  provider, repository identity, branch, and directory.
- **Sync run:** One fetch-parse-apply cycle for one workspace, started by the
  poller or by an explicit request.
- **Applied manifest:** The record, owned by this capability, of which config
  entities sync currently owns and the repository path each was last applied
  from. It tracks what was *written*, not what a run intended: a run that fails
  partway leaves the manifest describing exactly the entities it wrote before
  failing (AC-OFFICE-CONFIG-SYNC-003.14 in
  [Office Config Sync Reconciliation](config-sync-reconciliation.md)). Defining
  it against the last *successful* run would make a partially failed run's
  manifest a lie, and the next run would delete entities it had applied. It lets
  a later run delete what it previously applied, and identify the entity behind
  a file it could not read.
- **Managed entity:** A config entity named in the applied manifest, owned by
  sync rather than by the user.
- **Unmanaged entity:** A config entity created in Kandev and never applied by
  a run. Sync never modifies or deletes one.

- **Provider read surface:** The workspace-routed, read-only listing and
  file-content calls the integration system exposes.
- **Reference:** A field in a definition file that names another config entity
  by its key rather than carrying it inline. The complete set is enumerated in
  AC-OFFICE-CONFIG-SYNC-003.9a
  ([Office Config Sync Reconciliation](config-sync-reconciliation.md)).
- **Workspace settings:** The workspace-level governance values `kandev.yml`
  carries (name, description, approval and budget defaults, default executor
  and agent profile, permission handling mode, recovery lookback). They are not
  a config entity and are not one of the four sync unit kinds.

## Requirements

This document owns the source-and-lifecycle half of the operator-facing
contract: declaring a config source, running it on a schedule, and reporting
its outcome. What a run *reads* is
[Office Config Sync Fetch](config-sync-fetch.md)
(`REQ-OFFICE-CONFIG-SYNC-002`); what it *does* to config entities is
[Office Config Sync Reconciliation](config-sync-reconciliation.md)
(`REQ-OFFICE-CONFIG-SYNC-003`); how it coexists with the surfaces Office
already has, and the settings page, are
[Office Config Sync Surfaces](config-sync-surfaces.md)
(`REQ-OFFICE-CONFIG-SYNC-005` and `REQ-OFFICE-CONFIG-SYNC-006`). One
capability, split where each part is independently reviewable.

### REQ-OFFICE-CONFIG-SYNC-001: Provider-routed config source

**Intent:** A workspace can declare where its Office configuration comes from,
against either supported provider, reusing the field vocabulary shipped
workflow sync already proved stable across a provider addition.

**User story:** As an operator whose code lives on GitLab, I want to point my
Office workspace at a repository there, so my agent and routine definitions are
versioned and reviewed instead of hand-edited in a UI.

#### Acceptance criteria

- **AC-OFFICE-CONFIG-SYNC-001.1:** A workspace shall have at most one config
  source; the provider is a property of that single record. Reconciliation
  deletes managed entities absent from the fetched set, so two sources would
  each read the other's entities as deletions.
- **AC-OFFICE-CONFIG-SYNC-001.2:** When the provider is GitHub, the system
  shall identify the repository by `repo_owner` and `repo_name`, and shall
  reject a request that also carries `project_path`.
- **AC-OFFICE-CONFIG-SYNC-001.3:** When the provider is GitLab, the system
  shall identify the repository by `project_path` holding a namespace path of
  at least two non-empty segments, and shall reject a request that also
  carries `repo_owner` or `repo_name`.
- **AC-OFFICE-CONFIG-SYNC-001.4:** When a request omits the provider, the
  system shall reject it. Office config sync is new, so it has no pre-provider
  clients to keep working and no basis for a default.
- **AC-OFFICE-CONFIG-SYNC-001.5:** When a request names a provider outside
  `github` and `gitlab`, the system shall reject it with a validation error
  naming the two accepted values.
- **AC-OFFICE-CONFIG-SYNC-001.6:** When `path` is omitted or empty, the system
  shall store the repository root, and that root shall survive a
  read-modify-write through the settings UI. This departs deliberately from
  workflow sync, whose `Normalize` rewrites an empty path back to a non-empty
  default and so cannot address a repository root at all; Office's own
  `git clone` flow lays the config tree out at the repository root, so Office
  needs a root-addressable path.
- **AC-OFFICE-CONFIG-SYNC-001.7:** The system shall reject a `path` that
  contains a `..` segment, a `.` segment, an empty segment from a repeated
  slash, a leading slash, a backslash, or a NUL byte. Because
  AC-OFFICE-CONFIG-SYNC-001.6 makes the empty string a meaningful value, `path`
  is not trimmed into validity: a whitespace-only value or a trailing slash
  shall be rejected rather than rewritten, so what is stored is what the walk
  uses. The one normalization is Unicode NFC. This strictness is Office's own:
  shipped workflow sync trims whitespace and surrounding slashes and shall keep
  doing so, since rejecting input it accepts today would break a shipped
  contract. Office config sync is a separate package and imposes its own
  strictness on its own column.
- **AC-OFFICE-CONFIG-SYNC-001.8:** When `branch` is omitted or empty, the
  system shall store `main`. When `branch` is not a valid git branch name, the
  system shall reject it.
- **AC-OFFICE-CONFIG-SYNC-001.9:** When `interval_seconds` is omitted or zero,
  the system shall store 300. The system shall reject a value below 60 or
  above 2592000.
- **AC-OFFICE-CONFIG-SYNC-001.10:** When `poll_enabled` is omitted, the system
  shall store true.
- **AC-OFFICE-CONFIG-SYNC-001.11:** The config record shall store no provider
  host and no credential. Host, auth method, and credential shall be resolved
  per run from the workspace's provider connection, so a self-managed GitLab
  host needs no configuration here.
- **AC-OFFICE-CONFIG-SYNC-001.12:** When a config is saved for a workspace that
  already has one, the system shall replace it and reset the recorded status
  and content digest so the next run reconciles from scratch. It shall retain
  the applied manifest, so entities applied by the previous source stay managed
  and the first run against the new source updates or deletes them rather than
  orphaning them as unmanaged.
- **AC-OFFICE-CONFIG-SYNC-001.13:** Every config read, write, delete, and
  forced run shall be authorized against the caller's access to the named
  workspace, and a denied workspace shall be indistinguishable from a missing
  one.

### REQ-OFFICE-CONFIG-SYNC-004: Scheduling, concurrency, and status

**Intent:** Sync runs happen on a schedule and on demand, never overlap for a
workspace, and always leave a recorded outcome an operator can act on.

**User story:** As an operator, I want to see when the last sync ran and why it
failed, so a broken config source is visible without reading server logs.

#### Acceptance criteria

- **AC-OFFICE-CONFIG-SYNC-004.1:** When `poll_enabled` is true and at least
  `interval_seconds` have elapsed since the last attempt, the system shall run
  a sync for that workspace. When `poll_enabled` is false, the system shall
  run only on explicit request.
- **AC-OFFICE-CONFIG-SYNC-004.1a:** Within one poller tick, due workspaces
  shall be processed in ascending `workspace_id` order. No tiebreak is needed:
  `workspace_id` is the config table's primary key. The order is stated so a
  tick failing partway resumes over the same sequence rather than a
  provider-dependent or map-iteration one.
- **AC-OFFICE-CONFIG-SYNC-004.2:** When a workspace has never been synced, the
  system shall treat it as due.
- **AC-OFFICE-CONFIG-SYNC-004.3:** Sync runs for one workspace shall be
  serialized. When a run is in flight, a second run, a config write, and a
  config delete for that workspace shall wait rather than interleave.
- **AC-OFFICE-CONFIG-SYNC-004.3a:** Config writes and deletes for one workspace
  shall serialize against each other, not only against a run in flight. Two
  writes arriving together shall apply one after the other and the later shall
  win completely; the stored record shall never mix fields from both, nor carry
  a GitHub identity from one beside a GitLab identity from the other. Because a
  workspace has at most one config (AC-OFFICE-CONFIG-SYNC-001.1), the write is a
  single-row replace, and that row's uniqueness makes the outcome well defined.
- **AC-OFFICE-CONFIG-SYNC-004.4:** Workspaces shall be independent in exactly
  three respects, and this criterion claims no more. A run shall never wait on
  another workspace's lock, since AC-OFFICE-CONFIG-SYNC-004.3 serializes per
  workspace. A run that fails, times out, or is slow shall not prevent the
  poller attempting any other due workspace, in that tick or a later one. And no
  workspace's outcome, digest, warnings, or manifest shall be affected by another
  workspace's run (AC-OFFICE-CONFIG-SYNC-003.13). It does *not* promise
  parallelism: a tick processes due workspaces one at a time in
  AC-OFFICE-CONFIG-SYNC-004.1a's order, so a slow workspace delays later ones
  *within that tick*. That is latency, not blocking, and it is bounded by
  AC-OFFICE-CONFIG-SYNC-004.4a. Sequential dispatch is deliberate: it is what
  the shipped poller does and keeps a tick deterministic and resumable, where
  concurrent dispatch would need a worker limit and a rate-limit policy no
  requirement here asks for.
- **AC-OFFICE-CONFIG-SYNC-004.4a:** A run shall be bounded by a deadline of 10
  minutes, after which it shall be abandoned and recorded as a failure naming
  the deadline. Without one, AC-OFFICE-CONFIG-SYNC-004.4 is unenforceable: a run
  hung on a provider call holds the tick forever and every other workspace stops
  syncing with no signal. The bound is on the run, not on a single call, because
  a run spans up to 405 listings and 1000 fetches
  (AC-OFFICE-CONFIG-SYNC-002.5), where per-call timeouts still admit an
  unbounded total. Ten minutes sits above any honest run of that size; a run
  needing longer has outgrown these caps, and failing loudly is correct.
  Abandoning a run shall not roll back what it wrote, which
  AC-OFFICE-CONFIG-SYNC-003.14 already makes safe to retry.
- **AC-OFFICE-CONFIG-SYNC-004.5:** Every run shall record its outcome on the
  config record: attempt time, success or not, the failure message, and any
  warnings. A failure shall be recorded and logged and shall not stop the
  poller attempting other workspaces or the next tick.
- **AC-OFFICE-CONFIG-SYNC-004.5c:** Each warning class shall belong to exactly
  one phase, so AC-OFFICE-CONFIG-SYNC-004.5a's primary sort key is determined
  rather than left to the implementer. **Walk:** a missing `kandev.yml`, a cap
  truncation, a skill directory with no `SKILL.md`. **Fetch:** an oversized file,
  a file that could not be read. **Parse:** a file that failed to parse or
  validate, a declared name differing from its filename stem, two files of one
  kind resolving to the same key. **Apply:** a key colliding with an unmanaged
  entity, a deletion sweep suspended for a kind, a manifest entry whose entity no
  longer exists, a skill whose package locator changed mid-run
  (AC-OFFICE-CONFIG-SYNC-003.5e). **Reference resolution:** an unresolvable, self-referencing, or
  cycle-closing `reports_to`. A warning is assigned the phase that produced it,
  not the phase whose work it affects, which is why an exemption warning is a
  fetch warning even though it changes what apply deletes.
- **AC-OFFICE-CONFIG-SYNC-004.5b:** Warnings accumulated before a run failed
  shall be recorded with the failure, not discarded. Failure and warning
  recording are separate paths, and where they meet is what matters: a run
  exceeding a traversal cap warns naming the cap and *then* fails
  (AC-OFFICE-CONFIG-SYNC-002.5), so discarding warnings throws away the only
  record of why. A failed run that produced no warnings shall record an empty
  list, not a stale one, per AC-OFFICE-CONFIG-SYNC-004.5a.
- **AC-OFFICE-CONFIG-SYNC-004.5a:** Recorded warnings shall belong to the run
  that just finished and shall replace the previous run's warnings rather than
  accumulate. They shall be ordered by the phase that produced them (walk, then
  fetch, then parse, then apply, then reference resolution) and within a phase
  by the repository path of the file concerned, ascending and byte-wise, so two
  runs over the same repository record the same warnings in the same order.
  Within a phase, warnings that name no file (a cap truncation, an unresolved
  reference, a `reports_to` cycle) shall sort after those that do, ordered by
  the `(kind, key)` of the entity they name, and anything still tied shall be
  ordered last by its own rendered message text, ascending and byte-wise. That
  tiebreak makes the order total: a cap-truncation warning names neither a file
  nor an entity, and several warnings can share one `(kind, key)`, so without
  it two runs over an identical repository could record the same warnings in
  different orders. Message text is the one key every warning has by
  construction. At most 100 warnings shall be retained, the list then ending
  with a single entry naming how many were dropped, since the warnings occupy
  one text column a pathological repository could otherwise grow without
  bound.
- **AC-OFFICE-CONFIG-SYNC-004.6:** An explicit sync request against a
  configured workspace shall report the run's outcome and the updated config
  record together, including when the run failed. Only a missing config makes
  the request itself fail.
- **AC-OFFICE-CONFIG-SYNC-004.7:** When the configured provider has no
  connection for the workspace, the run shall fail with a message naming that
  provider and directing the operator to connect it. Saving such a config
  shall still succeed, so the operator can configure in either order.
- **AC-OFFICE-CONFIG-SYNC-004.8:** The recorded content digest shall describe
  the file set of the most recent successful run and nothing else, and shall be
  cleared when a run fails so it never claims a state that was not applied.
  Clearing it changes no reconciliation behavior, since
  AC-OFFICE-CONFIG-SYNC-003.5a already makes every run reconcile in full; the
  digest is a diagnostic, not a gate.
- **AC-OFFICE-CONFIG-SYNC-004.9:** When the config is deleted, previously
  managed entities shall be released to unmanaged ownership rather than
  deleted, and edits made to them afterwards shall no longer be reverted by a
  subsequent run. Release shall run entity by entity in the deletion order of
  AC-OFFICE-CONFIG-SYNC-003.9, so a repeated failure stops at the same entity
  every time, and each entity's ownership change and the removal of its applied
  manifest row shall be one unit, so an entity is either released and unlisted
  or managed and listed, never both and never neither. Because ownership is
  manifest membership (AC-OFFICE-CONFIG-SYNC-003.8), that unit is a single
  write, not a pair that must be coordinated.
- **AC-OFFICE-CONFIG-SYNC-004.9a:** When release fails partway, the system
  shall stop at the failing entity, retain the config, and record the failure
  naming that entity. Entities already released shall stay released: reverting
  them would need the manifest rows already deleted restored as a group, which
  the per-entity unit above deliberately does not provide. A partial release is
  a real state, and the manifest is its record.
- **AC-OFFICE-CONFIG-SYNC-004.9b:** Retrying the delete shall resume rather
  than repeat. An entity released by the earlier attempt has no manifest row,
  so release passes over it and a retry that reaches the end removes the
  config. Release is idempotent for that reason, not by re-reading ownership.
- **AC-OFFICE-CONFIG-SYNC-004.9c:** While the config is retained after a
  partial release, runs shall continue as configured. A released entity still
  defined in the repository is now unmanaged, so the next run neither adopts
  nor modifies it and warns under AC-OFFICE-CONFIG-SYNC-003.7. That state is
  not a dead end and needs no new mechanism: retrying the delete releases the
  remainder and removes the config, and deleting the unmanaged entity lets the
  next run create it as managed again.
- **AC-OFFICE-CONFIG-SYNC-004.10:** Deleting a config that does not exist shall
  succeed without effect.

## Out of scope

- **Writing Office configuration back to a repository through a provider API.**
  Neither client has a repository file write path today, and shipped workflow
  sync excludes write-back for the same reason. Adding one needs a write API
  per provider, a commit identity, and a conflict policy for concurrent
  upstream changes. Raw-git push remains the write path.
- **Multiple simultaneous config sources per workspace**, excluded by
  AC-OFFICE-CONFIG-SYNC-001.1.
- **Webhook-driven sync**, and **providers beyond GitHub and GitLab.**
- **Changing workflow sync's own path defaulting or path validation.**
  AC-OFFICE-CONFIG-SYNC-001.6 and AC-OFFICE-CONFIG-SYNC-001.7 apply to Office
  config sync only; tightening workflow sync would reject input it accepts
  today, and that contract belongs to the system that owns it.
- **What a run reads**, in [Office Config Sync Fetch](config-sync-fetch.md);
  **what it does to entities**, in
  [Office Config Sync Reconciliation](config-sync-reconciliation.md); and
  **how it coexists with the existing surfaces and is presented to an
  operator**, in [Office Config Sync Surfaces](config-sync-surfaces.md). Each
  has its own exclusions.

## Prior art

The `wiki-query` leg ran, against a `wiki` collection of 441 indexed documents
resolved at `~/Documents/henry/wiki`. A round-1 draft reported the leg
unavailable, which was false; the correction is recorded rather than silently
replaced. Three notes shaped this specification: a declarative-registration
argument for declaring each synced entity's shape once, a god-object warning
about shared sync coordinators absorbing per-caller variation, and an
EARS-as-universal-properties note that drove the removal of overlapping
reference-resolution criteria. Two constraints previously
credited to the wiki are in fact code-derived and now say so: they shaped
AC-OFFICE-CONFIG-SYNC-001.6 here and AC-OFFICE-CONFIG-SYNC-003.1 in
[Office Config Sync Reconciliation](config-sync-reconciliation.md). The
`saas-kb` `ai_sdlc` survey shows comparable products reaching repositories
through a read-scoped, workspace-routed token and keeping repository *writes* on
host git credentials, the split this specification adopts. The full survey,
those notes, and what this capability does differently are recorded in
[the system design](../system-design/config-sync.md#prior-art-and-alternatives).
