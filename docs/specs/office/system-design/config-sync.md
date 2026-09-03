---
status: draft
system: office
requirements:
  - REQ-OFFICE-CONFIG-SYNC-001
  - REQ-OFFICE-CONFIG-SYNC-004
  - REQ-OFFICE-CONFIG-SYNC-005
  - REQ-OFFICE-CONFIG-SYNC-006
---

# Office Config Sync System Design

## Purpose and boundaries

Office owns the contract for what an Office workspace's configuration is, which
of its entities are owned by a repository, and what a sync run does to them.
This design implements the source-and-lifecycle half of that contract: how a
config source is declared and stored, the package it runs in, when a run
happens, how it coexists with the other config surfaces, and how it is
surfaced. The run's own traversal and reconciliation are designed in
[Office Config Sync Reconciliation System Design](config-sync-reconciliation.md),
which this document is read before. Both halves sit on top of two contracts
this capability uses but does not own:

- The **integration system** owns provider connections, hosts, auth methods,
  credentials, and the read-only repository listing and file-content calls.
  This design calls them and stores no host or credential.
- The **agent system** owns `agent_profiles`, where Office agents live after
  ADR 0005 Wave C. This design writes agent rows through Office's existing
  repository interface and adds no column to that table. That constraint is the
  reason ownership is tracked in a manifest owned by this capability rather
  than as a `source` column on each entity, the way workflow sync does it.

Office config sync is built standalone, as `internal/office/configsync`. It
reuses workflow sync's *pattern* — the same field vocabulary, provider-dispatch
shape, and poll-and-record lifecycle — but not its *package*: no shared library
is extracted, and `internal/workflowsync` is neither modified nor imported.
Extracting the common mechanics is deferred so it can be designed against two
working implementations rather than one working and one imagined; the reasoning
is under [Prior art and alternatives](#prior-art-and-alternatives).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-CONFIG-SYNC-001` | [Data and contracts](#data-and-contracts), [Persistence](#persistence) |
| `REQ-OFFICE-CONFIG-SYNC-004` | [Scheduling and concurrency](#scheduling-and-concurrency) |
| `REQ-OFFICE-CONFIG-SYNC-005` | [Surface arbitration](#surface-arbitration) |
| `REQ-OFFICE-CONFIG-SYNC-006` | [Frontend](#frontend) |

`REQ-OFFICE-CONFIG-SYNC-001` and `REQ-OFFICE-CONFIG-SYNC-004` are specified in
[Office Config Sync](../requirements/config-sync.md);
`REQ-OFFICE-CONFIG-SYNC-005` and `REQ-OFFICE-CONFIG-SYNC-006` in
[Office Config Sync Surfaces](../requirements/config-sync-surfaces.md). This
design implements all four.

`REQ-OFFICE-CONFIG-SYNC-002` and `REQ-OFFICE-CONFIG-SYNC-003` are designed in
[Office Config Sync Reconciliation System Design](config-sync-reconciliation.md),
which also carries the failure-and-recovery table for a run that stops partway.

## Components and responsibilities

### `internal/office/configsync` (new)

Owns the whole capability: the config row, the schedule, the walk, the fetch,
the reconciliation, and the HTTP surface. Nothing in it is shared with
`internal/workflowsync`, and it imports nothing from that package.

| Element | Responsibility |
| --- | --- |
| `Provider` constants, `normalizeTarget` | Provider-conditional target validation. GitHub requires `repo_owner`+`repo_name` and forbids `project_path`; GitLab requires a multi-segment `project_path` and forbids the GitHub pair. An absent provider is rejected rather than defaulted (`AC-OFFICE-CONFIG-SYNC-001.4`). |
| `Config`, `SetConfigRequest`, `Normalize` | The field vocabulary and its validation, including the strict `path` rules of [Security](#security). |
| `GitHubClientProvider`, `GitLabClientProvider` | The two workspace-routed client interfaces, the same shape `workflowsync` uses today, satisfied by `github.Service` and `gitlab.Service`. |
| `DirEntry`, `listProviderEntries`, `fetchFile` | Provider dispatch, converting both upstream listing shapes to one neutral entry and both providers' errors to the neutral classification in [Reconciliation § Error classification](config-sync-reconciliation.md#error-classification-at-the-provider-boundary). Provider-typed values never escape this boundary; the classification does. |
| `Walker` | Drives the bounded multi-round directory walk. |
| `contentHash` | Stable sha256 over `path\x00len\x00content`, recorded for diagnostics only. |
| `Store` | CRUD over `office_config_sync_configs` and `office_config_sync_manifest`. |
| `Poller` | 60s outer tick, per-config interval check, per-run deadline. |
| `Runner` | Per-workspace lock, authorization, fetch, reconcile, `recordFailure`. |
| `Controller` | The four HTTP handlers. |

The field vocabulary, the status columns, and the poll-and-record lifecycle are
deliberately identical to `workflow_sync_configs` — that is the requirement the
task sets — but the identity is by *convention*, enforced by this design and by
tests, not by a shared type.

### `internal/workflowsync` (unchanged)

This capability does not modify, migrate, or import `internal/workflowsync`. Its
package path, exported surface, `workflow_sync_configs` table, poller, and 11
test files are untouched. Every divergence this design takes from shipped
workflow-sync behavior — rejecting an absent provider, an addressable repository
root, a run deadline, warnings recorded beside a failure, warnings excluded from
the unchanged verdict — is a divergence *from a package this design does not
edit*, so none of them is a change to a shipped contract.

### `internal/github` (extended)

`AC-OFFICE-CONFIG-SYNC-002.4a` requires a file fetch that fails with 401, 403,
429, or any 5xx to fail the run, which means the status has to survive the
client boundary. `PATClient` already promotes any non-2xx to a
`*GitHubAPIError`. `GHClient` does not: `ListRepoDirectory` and
`GetRepoFileContent` promote **only** not-found, and every other failure returns
a bare `fmt.Errorf` wrapping the `gh` exec error, so on a CLI-routed workspace a
403 would land in the unreadable-content class and the run would succeed with
`last_ok` set. Both satisfy the same `Client` interface and the choice between
them is made at runtime, so this is a live deployment path, not a corner.

Those two methods are therefore extended to promote the statuses the criterion
enumerates, mirroring `PATClient` and using the stderr-matching idiom the
package already ships (`isNotFoundErr`, `isForbiddenErr`). Matching stderr is a
heuristic and is named as one: a status phrased in a way no pattern matches
falls through to the bare error and lands in the residue class, which is exactly
where an unrecognized failure belongs (`AC-OFFICE-CONFIG-SYNC-002.4a` classifies
by exclusion). The failure mode is therefore under-promotion, never
mis-promotion, and PAT-routed workspaces are exact regardless. The change is
additive: the existing typed-404 behavior and the existing
`TestGHClient_ListRepoDirectory_Errors` and
`TestGHClient_GetRepoFileContent_Errors` expectations are unchanged, and new
cases cover the added statuses. This is the only file outside Office and the
frontend that this capability edits.
### `internal/office/config` and `internal/office/dashboard`

`config` owns every route that writes config entities from a local source:
`POST .../config/import` (`ApplyImport`), `.../config/sync/import-fs`
(`ApplyIncoming`), and `.../config/sync/export-fs` (`ApplyOutgoing`). All three
gain the active-source guard, so all three refusals in
`AC-OFFICE-CONFIG-SYNC-005.2` live there; its read-only routes (bundle export,
the two diffs) are untouched. `dashboard` owns the raw-git routes: `git/clone`
and `git/pull` gain the same guard, `git/push` and `git/status` do not
(`AC-OFFICE-CONFIG-SYNC-005.5`).

### Route registration

The four config-sync routes are registered by the new package on the existing
Office API group, inheriting `officeWorkspaceScopeMiddleware` and the coverage
of `TestOfficeRouteScopeCompleteness`.

## Data and contracts

### Config fields

A separate table, with column names, types, and semantics deliberately
identical to `workflow_sync_configs` so a later extraction is a merge rather
than a rename: `workspace_id` (primary key), `provider`,
`repo_owner`, `repo_name`, `project_path`, `branch`, `path`,
`interval_seconds`, `poll_enabled`, `last_synced_at`, `last_ok`, `last_error`,
`last_warnings`, `last_hash`, `created_at`, `updated_at`.

### Defaults and the addressable root

| Field | Value when omitted or empty | Criterion |
| --- | --- | --- |
| `provider` | none; the request is rejected | `AC-OFFICE-CONFIG-SYNC-001.4` |
| `branch` | `main` | `AC-OFFICE-CONFIG-SYNC-001.8` |
| `path` | `""`, the repository root | `AC-OFFICE-CONFIG-SYNC-001.6` |
| `interval_seconds` | 300, rejected below 60 or above 2592000 | `AC-OFFICE-CONFIG-SYNC-001.9` |
| `poll_enabled` | true | `AC-OFFICE-CONFIG-SYNC-001.10` |

`SetConfigRequest.Path` is a `*string`, because Office needs three states where
workflow sync has two:

| Value | Meaning |
| --- | --- |
| `nil` (field absent) | the default, which for Office *is* the root |
| `""` | the repository root, explicitly |
| `"a/b"` | that directory |

The pointer is what makes the root survive a read-modify-write through the
settings page (`AC-OFFICE-CONFIG-SYNC-001.6`). A plain `string` cannot tell an
absent field from an empty one, so a settings page that reads a root-addressed
config and writes it back unchanged would post `""` and have the default written
over it — which is exactly the shipped workflow-sync behavior this departs from.
Workflow sync's `Normalize` is not changed: it keeps collapsing both onto a
non-empty default, and keeps being unable to address a root.

### HTTP

Registered on the Office API group so `:wsId` is scope-checked by existing
middleware and covered by `TestOfficeRouteScopeCompleteness`:

```text
GET    /api/v1/office/workspaces/:wsId/config-sync/config
POST   /api/v1/office/workspaces/:wsId/config-sync/config
DELETE /api/v1/office/workspaces/:wsId/config-sync/config
POST   /api/v1/office/workspaces/:wsId/config-sync/sync
```

The workspace id is read from the `:wsId` path parameter the Office API group
already scope-checks. Status codes follow the shipped workflow-sync contract, by
convention rather than by shared code: a denied workspace is
indistinguishable from a missing one (404), an invalid config is 400, and a
forced run whose sync failed returns 200 with the error embedded beside the
updated config.

The sync route returns the run result and the updated config row together. The
result type is Office's own, and its field shape deliberately matches the
shipped `workflowsync.SyncResult` so one frontend card can render both:

```go
type SyncResult struct {
	Created   []string `json:"created"`
	Updated   []string `json:"updated"`
	Deleted   []string `json:"deleted"`
	Warnings  []string `json:"warnings"`
	Unchanged bool     `json:"unchanged"`
}
```

The three change fields are **name slices, not counters**, matching the shape
the shipped frontend already reads as `result.created ?? []`.

`Unchanged` is `len(Created)+len(Updated)+len(Deleted) == 0`, and **warnings are
not a term in it** (`AC-OFFICE-CONFIG-SYNC-003.5b`). This is the one place where
the shape's similarity is misleading, so the difference is stated rather than
left to be inferred from reuse: shipped `workflowsync` computes
`... && len(warnings) == 0`, so a warning there flips the verdict to changed.
Office must not, because a repository with no `kandev.yml` warns on every run by
design and would otherwise never report unchanged. Because the two packages are
independent, Office taking the other formula changes nothing about workflow
sync, and no shipped test constrains the choice.

There is deliberately **no `Error` field on the result**. The error travels on
the response envelope beside the config, mirroring
`WorkflowSyncForceSyncResponse { config; result?; error? }`, which is why a
forced run whose sync failed returns 200 with the config updated and no result
at all.

That is what makes "reported as unchanged"
(`AC-OFFICE-CONFIG-SYNC-003.5b`, `AC-OFFICE-CONFIG-SYNC-004.6`,
`AC-OFFICE-CONFIG-SYNC-006.5`) a checkable claim rather than a description: the
verdict is a field the caller reads, derived from the three change fields and
from nothing else — not the digest, not the warning list. `Warnings` is
populated on a failed run as well as a successful one
(`AC-OFFICE-CONFIG-SYNC-004.5b`).

## Control flow

### Surface arbitration

While an Office config source exists for a workspace, four handlers for that
workspace return 409 naming provider sync as the active source. The list is
taken from the route tables, not from memory, because
`AC-OFFICE-CONFIG-SYNC-005.2b` makes it closed:

| Handler | Route | Package | Why it is refused |
| --- | --- | --- | --- |
| `ApplyIncoming` | `POST .../config/sync/import-fs` | `office/config` | Second reconciler over the same rows |
| `ApplyImport` | `POST .../config/import` | `office/config` | Writes the same four kinds from an uploaded bundle |
| `ApplyOutgoing` | `POST .../config/sync/export-fs` | `office/config` | Manufactures a repository write path — see below |
| `gitClone`, `gitPull` | `POST .../git/{clone,pull}` | `office/dashboard` | Replaces the checkout under a live source |

Two reconcilers that each delete what they cannot see would otherwise alternate
deletions on every tick.

`ApplyOutgoing` is refused for a *different* reason than the other three, and
the distinction matters because the obvious reading gets it backwards. It does
not race provider sync: sync writes only the database, never the checkout, so on
the collision argument alone `export-fs` would be safe. It is refused because it
writes provider-sourced database state onto the checkout, where raw-git `push`
would commit it back as if authored locally, reconstructing the write-back path
this design excludes below. For a managed entity that is a no-op or a loop; for
an unmanaged one it promotes a key into the repository that the next run reports
as a permanent `AC-OFFICE-CONFIG-SYNC-003.7` collision.

The word *export* appears on both sides of this line, which is the trap. `GET
.../config/export` and `.../config/export/zip` return a bundle and write
nothing, so they are unaffected; `POST .../config/sync/export-fs` writes the
workspace directory and is refused. The test is what a handler writes, never its
name. Read-only filesystem diffs and raw-git `push` and `status` are likewise
unaffected.

**How each guard answers the question.** Neither `office/config` nor
`office/dashboard` depends on `office/configsync` today, and neither gains an
import to it. `configsync.Service` exposes:

```go
func (s *Service) HasActiveSource(ctx context.Context, workspaceID string) (bool, error)
```

Each consumer declares its own single-method interface over that signature and
receives it at construction, so the dependency points from the composition root
inward and the two packages stay independently testable with a stub. The
composition root is `internal/office/routes.go`, which already builds
`config.NewHandler(svcs.Config, log)` and calls
`dashboard.RegisterRoutes(router, svcs.Dashboard, …)`; both gain the guard as a
constructor argument, sourced from a new `Services.ConfigSync` field. A nil
guard means no source can be active, so the pre-existing wiring and every
existing test keep their current behavior without a special case.

A guard that fails to read (a database error, not a missing row) refuses rather
than allows: the guard's purpose is to prevent a second
writer, and an unknown answer is not evidence that there is no source.

Raw-git `push` stays the only write path back to a repository. Neither provider
client here has a file-write call, and shipped GitLab workflow sync already puts
write-back out of scope for that reason; adding one would need a commits API per
provider, a commit identity, and a conflict policy for concurrent upstream
changes — a larger capability than this requirement needs, and the same
read-for-sync / host-credentials-for-write split comparable products ship.

## Scheduling and concurrency

One `Poller`, on a 60s outer tick, selecting configs whose `poll_enabled` is
true and whose `interval_seconds` have elapsed since the last attempt. A
never-synced workspace is due immediately. Due configs are processed in ascending
`workspace_id`, which the `Store`'s listing query supplies via `ORDER BY
workspace_id` and which needs no tiebreak, `workspace_id` being the primary key.
The order is stated rather than left to the query because a tick that fails
partway resumes over the same sequence, not a map-iteration one.

The per-workspace lock is a `sync.Map` of mutexes held across the fetch, so a
config write, a config delete, and a second run for the same workspace queue
behind an in-flight run instead of interleaving. Config writes and deletes take
the same lock as runs do, so two writes arriving together also serialize against
each other and the later one wins whole; because a workspace has at most one
config row, the write is a single-row replace and the row can never end up
holding a GitHub identity from one request beside a GitLab `project_path` from
another (`AC-OFFICE-CONFIG-SYNC-004.3a`). No run ever waits on another
workspace's lock. This lock map is Office's own; workflow sync keeps its own,
and a workspace syncing workflows and Office config at the same moment is
expected and safe, because they reconcile disjoint entity sets and write
different tables.

Dispatch within a tick is sequential, and the design states that plainly because
`AC-OFFICE-CONFIG-SYNC-004.4` could otherwise be read as promising parallelism.
The tick is a synchronous loop over the due list, mirroring shipped
`SyncDueConfigs` without sharing its code, so a slow workspace does delay later
workspaces in the same tick. What the criterion actually guarantees is narrower
and all true of that loop: no run waits on another workspace's lock, a failing or slow workspace never
prevents the poller attempting the others, and no workspace's result touches
another's. Concurrent dispatch is rejected rather than deferred — it would need a
worker limit and a provider rate-limit policy no requirement asks for, and it
would give up the resumable, deterministic tick order the ascending
`workspace_id` sequence exists to provide.

Sequential dispatch is only safe if one run cannot hold the tick forever, and the
shipped poller has no bound: its 60s `time.Ticker` drops ticks while
`SyncDueConfigs` runs, so a run hung on a provider call stalls every workspace
with no signal. Every run therefore carries a 10 minute deadline
(`AC-OFFICE-CONFIG-SYNC-004.4a`). The deadline is derived by the `Runner`, not
by the `Poller`, so that it binds on **both** trigger paths: the poll loop and
the forced `POST .../config-sync/sync` handler enter the same `Runner`, which
wraps the caller's context in a `context.WithTimeout` **before** taking the
per-workspace lock. `AC-OFFICE-CONFIG-SYNC-004.4a` is trigger-agnostic and this
is what makes it so. Starting the clock before the lock means time spent queued
behind another run for the same workspace counts against the budget, and that is
the intent: the alternative lets a forced run block indefinitely behind a run
that is itself stuck, which is the hazard the deadline exists for. A run that
expires while still waiting has written nothing, so it is recorded as a failure
and the manifest is untouched. Putting it in the poller instead would leave a forced run
unbounded, which is what shipped workflow sync does — `httpForceSync` passes the
request context straight to `SyncWorkspace`, and `context.WithTimeout` appears
nowhere in that package. Expiry is recorded as an ordinary run failure. The deadline is per run rather than per call because a
run's cost is spread over up to 405 listings and 1000 fetches, where per-call
timeouts still admit an unbounded total. Nothing is rolled back on expiry;
per-entity manifest atomicity already makes a partially applied run safe to
retry.

Warnings replace the previous run's rather than accumulating, and are capped at
100 with a final entry naming the number dropped
(`AC-OFFICE-CONFIG-SYNC-004.5a`). The sort key is that criterion's four-part
tuple — phase, then path (path-bearing first), then `(kind, key)`, then rendered
text — as one comparator, so the ordering has a single site;
`AC-OFFICE-CONFIG-SYNC-004.5c` fixes which phase each warning class belongs to,
so the primary key needs no judgment at the call site. The last part makes the
order total: a cap-truncation warning names neither file nor entity, and several
warnings can share one `(kind, key)`, so a test may assert an exact sequence
without flaking. The cap exists because `last_warnings` is one `TEXT NOT NULL
DEFAULT '[]'` column holding a JSON array, which an uncapped list would grow
without bound.

## Persistence

Two new tables, `office_config_sync_configs` and
`office_config_sync_manifest`, created with idempotent `CREATE TABLE IF NOT
EXISTS` and, for later columns, the existing `ADD COLUMN` pattern that swallows
`db.IsDuplicateColumnError`. `workflow_sync_configs` is untouched: this
capability adds tables and rewrites no existing row.

Deleting a workspace removes its config and manifest rows with the workspace's
other Office state; no release runs, because the entities are going away too.

Both manifest columns are created together; `source_path` is not a later
addition, so no backfill question arises. Saving a config over an existing one
resets status and `last_hash` but retains the manifest, so entities applied by the previous source stay managed and the
first run against the new source updates or deletes them rather than orphaning
them as unmanaged. Deleting a config releases managed entities to unmanaged
ownership, one entity at a time, dropping each manifest row with the ownership
change it records; see [Reconciliation § Release](config-sync-reconciliation.md)
for the partial-failure semantics that follow from that.

## Security

Every route is workspace-scoped by the existing Office middleware; a denied
workspace is indistinguishable from a missing one. Credentials are never stored
on the config row and are resolved per run from the workspace's own provider
connection, so a workspace can only sync from a repository its own token can
read, and no cross-workspace credential reuse is added.

Repository content is untrusted input. `path` is validated rather than normalized
into validity: `..`, `.`, empty segments from repeated slashes, a leading slash,
backslashes, and NUL are rejected, and a whitespace-only or trailing-slash value
is rejected rather than trimmed. Trimming is the usual move and is wrong here
because `AC-OFFICE-CONFIG-SYNC-001.6` makes the empty string mean the repository
root: once `""` is load-bearing, silently rewriting `"  "` or `"a/"` stores a
target the operator never typed. The one normalization is Unicode NFC, so two
byte-different spellings of a directory name cannot address different paths. The walk is bounded by `Limits` so a
hostile repository cannot drive unbounded fetching; per-file size is capped at
1 MiB by this capability on both providers; and parsed definitions go
through the same validation as UI-created ones, so a repository cannot create an
entity shape the product would otherwise refuse. Synced agent definitions carry prompts
agents will execute, the same trust level as a UI-authored agent, bounded by the
workspace that configured the source.

Workspace settings are the one thing a repository deliberately cannot reach.
`kandev.yml` carries `permission_handling_mode`, `approval_default`, and
`budget_default`, so applying it would let a commit widen the workspace's
permission posture on the next poll with no human in the loop. The walk never
fetches the file, which makes that unreachable rather than merely disallowed.

## Observability

`configsync` exports expvar counters under `office_config_sync_*`: attempts,
successes, failures, unchanged runs, entities created, updated, deleted,
warnings emitted, and cap truncations. The prefix is distinct from workflow
sync's, so the two are separable without a shared label. The same events are
emitted as structured `office.configsync.*` zap logs carrying workspace,
provider, and repository for human debugging. Recorded
`last_error` and `last_warnings` remain the user-visible diagnostic.

## Frontend

Office gets its own card,
`components/office/settings/office-config-sync-card.tsx`, modelled on
`components/settings/workflow-sync-status-banner.tsx` but not shared with it.
`workflow-sync-status-banner.tsx` is not edited, extracted, or rewrapped.

That is also what gives one requirement a home.
`AC-OFFICE-CONFIG-SYNC-006.3` caps the displayed warnings at the first 10 with a
count of the remainder; the shipped banner's `WarningsAlert` maps the whole
`last_warnings` array with no cap. A shared card would have had to either change
shipped rendering to satisfy an Office criterion or carry a per-caller branch, so
the cap would have had no unambiguous owner. With a separate card it is one
component's rule. The two caps are distinct layers: the backend retains at most
100 warnings and appends a truncation entry
(`AC-OFFICE-CONFIG-SYNC-004.5a`); the card then displays at most 10 of what it
received.

New: `lib/types/office-config-sync.ts`, `lib/api/domains/office-config-sync-api.ts`,
and `hooks/domains/office/use-office-config-sync.ts`, mirroring
`use-workflow-sync.ts` including its provider switch that clears the other
provider's fields and its background refresh so poller results appear without a
reload. The directory field renders the repository root label when empty — a
state that, unlike in workflow sync, is reachable here.

Copy goes into the existing `office` namespace under a `configSync.` prefix.
**No key moves**: `workflows` keeps every key it has. Duplicating a handful of
short strings is the deliberate cost of not editing a shipped namespace, whose
call sites belong to a feature this capability does not own.

Adding keys spans **six** catalogs, not five: `en`, `pt-pt`, `zh-cn`, `zh-hk`,
`zh-tw`, and `pseudo`, which `check-i18n-keys.mjs` holds to the same key set as
`en`, failing on a missing *or* an extra key. `pnpm run i18n:zh-hant` generates
the Traditional pair and `pnpm run i18n:pseudo` regenerates `pseudo`.

`configSync.everySeconds` is a plural key: it must be added as the pair
`configSync.everySeconds_one` / `configSync.everySeconds_other` in all six
catalogs and called as `t("office:configSync.everySeconds", { count })`. The
base name alone registers nothing; one variant alone reads as an extra key.

## Prior art and alternatives

The `wiki-query` leg ran against a `wiki` collection of 441 indexed documents at
`~/Documents/henry/wiki`; the three notes cited below were retrieved from it.
Three positions from it are load-bearing.

`concepts/offline-first-sync.md` argues for **declarative entity registration**:
each synced entity type declares its shape once and the library owns auth,
retry, dirty-checking, persistence, and idempotency, because "bypassing the
library defeats the abstraction and creates drift". That is exactly the `Domain`
seam an earlier draft of this design proposed: a shared `internal/reposync`
library onto which `internal/workflowsync` would be migrated and Office then
built.

**That extraction is deferred, and this design builds Office standalone
instead.** The same note is the reason: it records a **god-object failure** from
a codebase where a `SyncCoordinator` reached 2,506 lines, and warns that the
failure begins when a shared coordinator absorbs per-caller variation. The two
callers here are already known to
diverge on five points, every one of which the shared seam would have had to
absorb before either half shipped: workflow sync defaults an absent provider
while Office rejects it; workflow sync rewrites an empty path to a non-empty
default while Office needs the empty path to mean the repository root; the walk
is one flat listing there and a bounded recursive descent here; `Unchanged`
counts warnings there and must not here; and warning retention is uncapped there
and capped at 100 with a truncation entry here. Reconciling five behavioral
differences in a library written before either caller exists is speculative
abstraction, which the repository's engineering principles rule out directly.

Deferring costs genuine duplication — two poll loops, two config stores, two
status cards — and that cost is accepted rather than argued away. It is bounded
by building Office to the *same* vocabulary (identical column names, the same
`SyncResult` field shape, the same poll-and-record lifecycle) so a later
extraction is a merge of two working implementations rather than a redesign. A
follow-up card carries the extraction and the five collisions above, sequenced
after this ships: doing it first re-imports the coupling this narrowing removed,
and "not worth extracting" is only a defensible verdict once both halves exist
to be compared.

`synthesis/pbt-from-ears-bridge.md` treats EARS acceptance criteria as
**universal properties** - for any input where the trigger holds, the response
holds — and distinguishes clauses that are logically testable from those that
are only prose. Applied here it is a drafting constraint, not a testing one: two
criteria whose triggers overlap but whose responses differ cannot both be
properties, which is the defect shape behind the reference-resolution
contradiction this round removed, and the reason
`AC-OFFICE-CONFIG-SYNC-003.11` was restated to quantify over a named field
projection instead of over a whole row.

Two further constraints are code-derived rather than wiki-derived. Workflow sync's
`SetConfigRequest.Normalize` rewrites an empty path back to its non-empty
default, so the repository root cannot be configured and an empty path written
straight to the database reverts the first time the settings page opens — the
reason `Path` is `*string` in Office's own request type above. And workflow sync keys a synced workflow
on `(source path, name)`, so moving a file creates a duplicate while the
original refuses to be removed — the reason identity here excludes the path.

The `saas-kb` `ai_sdlc` survey shows every surveyed product reaching a
repository through a workspace or instance-scoped token with *read* access:
Warp, Devin, OpenHands, and Factory.ai all document self-managed GitLab via a
personal or service-account token. Multica makes the same read/write split
adopted here — its provider integration connects with a `read_api`-scoped token
for mirroring, while writes (agents opening merge requests) deliberately bypass
that integration and use "the runtime host's own Git credentials", with "no
provider configuration" required. That is independent evidence that pull-only
provider sync beside host-credential push is a shipped shape, not a shortcut.

Two things differ from the survey. Those products mirror repository *activity*
(merge requests, CI status) into their own model; this mirrors *configuration*
out of a repository into the control plane, which makes reconciliation and
ownership the hard part rather than transport, and has no counterpart in the
surveyed documentation. And several pair polling with webhooks; this stays
polling-only, matching shipped workflow sync, because Kandev's GitLab
integration has no webhook support or rate-limit backoff to build on.

## Implementation order

No migration precedes this work. The two additive `internal/github` methods land
first, since the fetch path depends on their error classification. Then the
source binding and scheduling in this document, then the walk and reconciliation
in
[Office Config Sync Reconciliation System Design](config-sync-reconciliation.md),
then the settings card. `internal/workflowsync` is not touched at any point, so
no step in this order can regress a shipped feature.

## Related decisions

- [ADR 0031](../../../decisions/0031-office-skill-reference-files.md) defined
  skill support files and `file_inventory`, which round 2's `references/`
  request exists to populate.
- [ADR 0005](../../../decisions/0005-agent-model-unification.md) placed Office
  agents in `agent_profiles`, which is why ownership is tracked in a manifest
  rather than a per-entity column.
- [GitLab workflow sync requirements](../../integrations/requirements/gitlab-workflow-sync.md)
  is the shipped provider contract this design reuses and whose write-back
  exclusion it follows.

