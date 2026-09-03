---
status: draft
system: office
requirements:
  - REQ-OFFICE-CONFIG-SYNC-002
  - REQ-OFFICE-CONFIG-SYNC-003
---

# Office Config Sync Reconciliation System Design

## Purpose and boundaries

This design covers what a single Office config sync run reads from the
repository and what it does to the workspace's config entities: the bounded
walk of the Office config layout, how provider errors are classified at that
boundary, the applied manifest that records ownership, the reconciliation
itself, and what happens when a run fails partway.

It implements `REQ-OFFICE-CONFIG-SYNC-002` from
[Office Config Sync Fetch](../requirements/config-sync-fetch.md) and
`REQ-OFFICE-CONFIG-SYNC-003` from
[Office Config Sync Reconciliation](../requirements/config-sync-reconciliation.md).

It is the second half of one design. Both halves live in a single new package,
`internal/office/configsync`. The first half,
[Office Config Sync System Design](config-sync.md), owns the package boundary,
the config row and HTTP contracts, scheduling, surface arbitration, persistence,
security, observability, and the frontend. The types named here (`DirEntry`,
`Poller`, `Runner`, `Controller`, `ErrNotFound`, `ErrUnavailable`) are declared
there; read it first.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-CONFIG-SYNC-002` | [Bounded walk](#bounded-walk), [Error classification at the provider boundary](#error-classification-at-the-provider-boundary), [Workspace settings are not synced](#workspace-settings-are-not-synced), [Limits](#limits) |
| `REQ-OFFICE-CONFIG-SYNC-003` | [Identity and ownership](#identity-and-ownership), [`office_config_sync_manifest`](#office_config_sync_manifest), [Sync run](#sync-run) |

## The provider boundary

### Error classification at the provider boundary

"Provider-typed values never escape" must not mean "the status is lost". The walk
must distinguish an absent directory from an unreadable one, so the boundary
converts, rather than hides, the typed errors the clients expose —
`*github.GitHubAPIError` and `*gitlab.APIError`, each carrying `StatusCode` and
recoverable with `errors.As`. GitHub's CLI-routed client types only not-found
today; the two additive extensions in
[Config Sync § `internal/github`](config-sync.md#internalgithub-extended) are what
make this table hold on that path:

| Upstream | Neutral result | Caller behavior |
| --- | --- | --- |
| 404 | `ErrNotFound` | Absent directory or file; meaning depends on position in the walk. |
| 401, 403, 429, any 5xx, timeout, transport failure | `ErrUnavailable`, wrapping the original for the log | Always fails the run. |
| Anything else | `ErrUnreadable`, wrapping the original for the log | On a file: warning plus deletion exemption. On a listing: fails the run. |

`ErrUnreadable` is the residue class: typed errors do not cover everything a
client returns. Both clients report an oversized or
undecodable file with a bare `fmt.Errorf` — GitLab's `file exceeds the %d byte
limit`, GitHub's `unsupported encoding %q` — satisfying `errors.As` against
neither typed error. Classifying by exclusion rather than by matching those
strings makes the mapping total: a client that grows a new failure mode lands in
`ErrUnreadable` rather than falling through undefined. The direction of the
default is deliberate: for a *file*, unreadable means warn and exempt from
deletion — nothing wrong is applied and nothing wrongly deleted. For a *listing*
there is no safe middle: an unreadable directory is indistinguishable from an
empty one, and reading it as empty would feed the deletion sweep, so it fails the
run (`AC-OFFICE-CONFIG-SYNC-002.3a`).

Position disambiguates 404, because both providers answer 404 for a path the
credential cannot see. Round 1's listing of the configured root is therefore the
access probe: it must succeed, and any failure there — 404 included — fails the
run (`AC-OFFICE-CONFIG-SYNC-002.4`). Once it has, the credential is proven able
to read this repository on this branch, so a 404 beneath it is a genuinely absent
directory (`AC-OFFICE-CONFIG-SYNC-002.3`). An `ErrUnavailable` is never an empty
directory: reading a rate-limit or a 500 as "this kind has no files" would feed
the deletion sweep and wipe every managed entity of that kind on a transient
outage.

## Data and contracts

### `office_config_sync_manifest`

```text
workspace_id TEXT NOT NULL
kind         TEXT NOT NULL   -- skill | agent | project | routine
entity_key   TEXT NOT NULL
source_path  TEXT NOT NULL   -- repository path last applied from
PRIMARY KEY (workspace_id, kind, entity_key)
CREATE INDEX ... ON office_config_sync_manifest (workspace_id, source_path)
```

`source_path` is recorded metadata, not identity. The primary key is unchanged,
so a file that moves within its kind's directory still updates the existing
entity and simply rewrites `source_path`
(`AC-OFFICE-CONFIG-SYNC-003.1`). It exists to answer one question the entity's
own contents cannot: *which entity does this file define, when the file is the
thing that failed?* A file over the size limit or one that will not parse yields
no declared `name`, and the key is the declared `name`, so without a recorded
path there is no way to exempt that entity from the deletion sweep — and the
failure mode is deleting a managed entity because its file was briefly
unreadable. The index makes that path lookup a point read during apply.

`source_path` is written for the file that *won* an identity collision
(`AC-OFFICE-CONFIG-SYNC-003.3`); a losing file's path is named in the warning
and not recorded, so an unreadable losing file exempts nothing, which is correct
because it defined nothing.

Its granularity differs by kind, because a skill is a directory rather than a
file: for an agent, project, or routine it is the definition file's path; for a
skill it is the skill's directory. The lookup is a point read on that column, but
only after the failing path is reduced to the same granularity: a skill-kind path
is truncated to its first two segments below the configured root
(`skills/<dir>`) before the read, and the other three kinds are read whole. That
truncation is what makes an unreadable `SKILL.md` and an unreadable
`skills/<dir>/references/foo.md` resolve to the same skill, which is what the
exemption needs — a support file that will not load must not look like a deleted
skill. Without it the `references/` path matches no row and the sweep deletes a
skill whose only failure was a support file.

The lookup has one blind spot, covered by a second rule rather than left to the
sweep. A file renamed *and* made unreadable in the same commit presents a new
path the manifest never recorded and an old path the listing no longer returns,
so neither direction matches: the file is not applied, its entity is not
exempted, and it would be deleted for a failure that is not a removal.
`AC-OFFICE-CONFIG-SYNC-003.6a` closes it by suspending deletion for a whole kind
whenever the run holds an unreadable or unparseable file of that kind that mapped
to no manifest entry. The rule is coarse on purpose: a file whose contents could
not be read cannot say which entity it defines, so precision is unavailable, and
the two errors are not symmetric — a postponed deletion is corrected by the next
clean run, while a deleted entity is gone along with every column outside the
owned projection, and its replacement carries a new identifier nothing else in
the workspace points at.

The manifest lives in its own table rather than as a `source` column on each
entity, because three kinds live in Office-owned tables while agents live in
`agent_profiles`, owned by the agent system. An ownership column there would push
an Office concern across a system boundary and couple this capability's
migrations to another system's schema. A separate table keeps ownership inside
this capability, deletes in one statement on config removal, and iterates cheaply
by kind. The rejected alternative is recorded deliberately: copying workflow
sync's per-entity `source` column is the smaller diff and what a reader would
expect, and it is wrong here for that ownership reason alone.

## Control flow

### Bounded walk

The walk is a bounded breadth-first traversal expressed in rounds, built only
from the non-recursive listing both providers already expose. No recursive
listing mode and no GitHub Trees API is required, and no provider client gains a
method.

| Round | Office requests | Workflows requests |
| --- | --- | --- |
| 1 | `<path>`, `<path>/agents`, `<path>/projects`, `<path>/routines`, `<path>/skills` | `<path>` |
| 2 | for each subdirectory `S` of `<path>/skills`: `S` and `S/references` | none |
| 3 | none | — |

Round 2 asks for both the skill directory and its `references/` in the same
round because both names are already known from round 1's `skills/` listing; a
third round would buy nothing.

`Select` keeps `*.yml`/`*.yaml` from the three flat kind directories, `SKILL.md`
from each skill directory, and every regular file from each `references/`
listing. Support files become that skill's `file_inventory`, so a
repository-synced skill gets the progressive disclosure ADR 0031 gave bundled
system skills; a skill with no `references/` syncs with an empty inventory.

Populating that inventory is new work, and the design says so rather than
implying a read path it can reuse. `references/` and `file_inventory` are real:
ADR 0031 defines the layout, the column lives in `repository/sqlite/skills.go`,
and the bundled system-skill sync fills it. The Office workspace config loader
does not — `loadSkillsLocked` reads `skills/*/SKILL.md` and nothing else, and
neither string appears in that package — so a skill imported from the filesystem
today arrives with no inventory. Provider sync diverges from that loader
deliberately: a repository is precisely where a skill's reference files belong
(`AC-OFFICE-CONFIG-SYNC-002.1c`).
Files are fetched through the neutral `fileGetter` and sorted by full repository
path, byte-wise ascending, before parsing.

That sort orders *files*; apply is over *entities*, and for skills the two are
not the same shape. Files are grouped into entities first and each entity applied
once, complete, in `source_path` order (`AC-OFFICE-CONFIG-SYNC-003.9d`). The
grouping is free rather than a second pass with its own ordering rule: a skill's
files are already adjacent in the sorted slice, since they all begin
`skills/<dir>/` and a directory name cannot contain `/`, so the walk emits each
skill as consecutive entries. Nothing keys off `SKILL.md` preceding `references/`
inside that run (`AC-OFFICE-CONFIG-SYNC-003.9e`).

Every listing and fetch names the configured branch, not a commit, so a push
landing mid-run can spread the view across two revisions. Pinning would mean
resolving the branch to a SHA, and neither client exposes a resolve call, so it
cannot be had without the new client method `AC-OFFICE-CONFIG-SYNC-002.2` rules
out. The exposure is bounded rather than eliminated: full reconciliation every
run corrects a mixed view within one poll interval, and no retry-on-drift rule is
used because on an actively committed repository it can fail to converge while
the plain next run always does (`AC-OFFICE-CONFIG-SYNC-002.8`,
`AC-OFFICE-CONFIG-SYNC-002.8a`).

Listing failures follow
[Error classification at the provider boundary](#error-classification-at-the-provider-boundary):
the root listing is the access probe and any failure there fails the run;
below it, `ErrNotFound` is an absent directory that contributes nothing and
`ErrUnavailable` fails the run.

#### Workspace settings are not synced

`kandev.yml` is not fetched. Its presence in the root listing is used only as the
marker that the configured path looks like an Office config root; its absence
warns and the run continues. Its payload is `configloader.WorkspaceSettings`
(`permission_handling_mode`, `approval_default`, `budget_default`,
`default_executor`, `default_agent_profile`, `recovery_lookback_hours`):
workspace governance and safety controls with their own live update path, not
config entities. Applying them from a repository would let a poll widen an
agent's permission posture with no human in the loop; a fetch never made cannot. The four kinds are the whole sync unit set
(`AC-OFFICE-CONFIG-SYNC-002.1b`); a named exclusion in the requirements'
`Out of scope`, and the one place provider sync deliberately reads less than the
raw-git surface does.

#### Limits

`Limits{MaxSkills: 200, MaxFiles: 1000}`. Both are counted in the units
`AC-OFFICE-CONFIG-SYNC-002.5` promises, and the listing budget is *derived* from
them (`5 + 2·MaxSkills`, at most 405 listings) rather than configured beside
them. An earlier draft capped listings at 200 directly, silently converting the
promised 200 skills into roughly 97 and failing a legal 150-skill repository;
deriving the budget makes that unrepresentable.

The file cap is sized against the skill cap: 200 skills at up to four files each
is 800, leaving room for `agents/`, `projects/`, and `routines/`. Either can bind
first. A repository exceeding both names the skill cap, evaluated while planning
round 2 before any of that round's files are fetched, so a given repository
produces the same message on every run. The warning names
the cap and the dropped count, and the run fails without applying, because a
truncated fetch would read as "everything past the cap was deleted."

Per-file size is capped at 1 MiB, measured on received content before parsing.
What is identical across providers is the *outcome*, not the mechanism
(`AC-OFFICE-CONFIG-SYNC-002.6`). Two paths reach it: a file the client returns is
measured here and, if oversized, warns and earns a deletion exemption; a file the
client refuses — GitLab's `maxRepoFileBytes` is itself 1 MiB, so this
capability's own check never fires there — returns an untyped error, lands in
`ErrUnreadable`, and produces the same warning and exemption.

Neither delegating the cap to the read surface nor checking before fetching is
available. Only GitLab's client caps a read; GitHub's returns an undecodable
encoding. Pre-fetch, `github.RepoContentEntry` carries `Size int` but
`gitlab.RepoTreeEntry` carries none, so a listing-based check would exist on one
provider only — the exact asymmetry the criterion removes. Measuring
received bytes is the one check identical on both, and `ErrUnreadable` absorbs
the rest without either client changing shape.

### Sync run

1. Acquire the per-workspace lock; hold it across the whole run.
2. Load the config; authorize the workspace.
3. Resolve the provider client for the workspace. Absent connection fails here
   with a provider-named message.
4. Walk, select, fetch, sort.
5. Parse each file. A parse or validation failure becomes a warning and removes
   that file from the applied set; the entity it previously defined is looked up
   by `source_path` and exempted from deletion.
6. Resolve identity and collisions, then `Apply`, then resolve `reports_to` in
   the second pass.
7. Compute `contentHash` and record it with status and warnings.

There is no digest short-circuit. `contentHash` is recorded for diagnostics and
is never compared to decide whether to reconcile. Skipping a run on an unchanged
digest would make `AC-OFFICE-CONFIG-SYNC-003.5` — repairing edits made in the
Kandev UI — unreachable in precisely the case it exists for, since UI drift is
only observable when the repository has *not* changed. Shipped workflow sync
already works this way and says so at the function: the digest is "recorded on
the config row for observability only — every sync reconciles regardless
(repairing local drift), with the applier writing only actual differences." That
last clause is where idempotency comes from
(`AC-OFFICE-CONFIG-SYNC-003.11`): a second run over an unchanged repository
writes nothing. The "unchanged" verdict is derived from the apply result — zero
creates, zero updates, zero deletes — and from neither the digest nor the warning
count (`AC-OFFICE-CONFIG-SYNC-003.5b`); see [Apply](#apply) for the comparison
rule that decides an update, and
[Config Sync § HTTP](config-sync.md#http) for why Office's formula deliberately
differs from shipped workflow sync's.

### Identity and ownership

Identity is `(kind, key)`: the declared `name` for agents, projects, and
routines; the directory name for skills. The path is excluded on purpose —
workflow sync keys on `(source path, name)`, so moving a file there creates a
duplicate the original refuses to release; that failure must not be reproduced.

Conflict resolution is deterministic and independent of provider listing order:

| Situation | Outcome |
| --- | --- |
| Declared `name` differs from the filename stem | The declared `name` wins; warning names both. |
| Two files of one kind resolve to the same key | Lexicographically first full path wins; warning names the key and every losing path. |
| Key collides with an **unmanaged** entity | Neither modified nor adopted; warning names kind and key. |

#### Release

Deleting a config takes the same per-workspace lock a run takes and holds it
across the whole release (`AC-OFFICE-CONFIG-SYNC-004.3`), so no run observes the
half-released manifest. Under that lock it walks the manifest in the deletion
order of `AC-OFFICE-CONFIG-SYNC-003.9` (reverse kind order, `entity_key`
ascending) and deletes each entity's manifest row; the entity itself is not
written at all.

`AC-OFFICE-CONFIG-SYNC-004.9` requires each entity's ownership change and its
manifest-row removal to be one unit, "so an entity is either released and
unlisted or managed and listed, never both and never neither". Under
[Terminology](../requirements/config-sync.md) and
`AC-OFFICE-CONFIG-SYNC-003.8`, managed **is** manifest membership: there is no
ownership column, because ADR 0005 puts Office agents in `agent_profiles`
alongside entities this capability does not own. So the two halves of that unit
are the same write, and the invariant holds by construction rather than by
transaction. Release is one `DELETE` per manifest row and touches no other
table; an empty manifest is the same walk over no rows, and the config is removed
with nothing else happening.

That also settles why the criterion states the outcome as "edits made to them
afterwards shall no longer be reverted" rather than as an unlock: nothing locks
a managed entity in the UI, and `AC-OFFICE-CONFIG-SYNC-003.5` exists precisely
because a managed entity *can* be edited by hand. Release removes the manifest
row that made the next run revert the edit; there is no lock to lift.

Because the unit is a single row delete, the partial state is legible without a
resume cursor: release reads the manifest, so an entity released by a failed
earlier attempt is simply not in the set — the whole of the idempotency
(`AC-OFFICE-CONFIG-SYNC-004.9b`).

The remaining exposure is stated rather than engineered away: a retained config
keeps polling, and a released entity whose file is still in the repository now
collides as unmanaged, so the next run warns under
`AC-OFFICE-CONFIG-SYNC-003.7` instead of re-adopting it. The two ways out are
already-shipped operator actions (`AC-OFFICE-CONFIG-SYNC-004.9c`): retry the
delete, or delete the loose entity and let the next run recreate it as managed.

#### Apply

Every key the run has in hand falls in exactly one of these cases. The two
inputs are the fetched set for that kind and the manifest for that workspace;
nothing else is consulted, and no entity is inspected to discover whether it is
managed.

| Key | In fetched set | In manifest | Outcome |
| --- | --- | --- | --- |
| New | yes | no | Created, manifest row added with its `source_path`. Counted in `Created`. |
| Existing | yes | yes | The owned projection is written and the manifest row's `source_path` refreshed. Counted in `Updated` only when a field actually changed. |
| Removed upstream | no | yes | Deleted, and its manifest row removed. Counted in `Deleted`. |
| Exempt | no (fetch or parse failed) | yes | Not written and **not** deleted; manifest row retained unchanged. Warned, not counted. |
| Gone out of band | no | yes, but the entity no longer exists | Manifest row dropped and the run continues (`AC-OFFICE-CONFIG-SYNC-003.15`). Not counted. |
| Foreign | yes | no, and an unmanaged entity holds the key | Neither modified nor adopted; warned (`AC-OFFICE-CONFIG-SYNC-003.7`). Not counted. |

"Counted in `Updated` only when a field actually changed" needs a comparison
rule, since `AC-OFFICE-CONFIG-SYNC-003.5b` derives the verdict from what apply
did. The rule is: build the owned projection from the fetched definition, read
the entity's current one, and compare field by field. Equal means no write and
no count; unequal means one write of the whole projection and one entry in
`Updated`. Comparison is over the owned projection only, so a hand edit to a
field sync does not own never makes a run look changed, and it happens before the
write rather than by inspecting a rows-affected count, which SQLite reports as 1
for a no-op `UPDATE`.

"Removed upstream" and "Exempt" are the same observable input — a key present in
the manifest and absent from the fetched set — separated only by *why*, so the
fetch phase must record the exemption at the moment it warns rather than letting
apply infer it from absence. When the
exemption cannot be attributed to a key at all, because the file was renamed and
unreadable in the same commit, deletion is suspended for that whole kind for the
run rather than guessed at.

Apply order is skills, agents, projects, routines, and deletion runs in reverse.
The order exists for determinism — a fixed sequence of writes, warnings, and
deletes — not for reference resolution: after the table below there is no
cross-kind reference left to resolve.

The Office config file format has four fields that look like references. Only
one is:

| Kind | Field | Treatment |
| --- | --- | --- |
| agent | `reports_to` | The one real reference. Resolved, within the kind, by a second pass. |
| agent | `desired_skills` | Not resolved. A list of skill *slugs*, stored verbatim. |
| project | `lead_agent_name` | Not resolved. Export-only; no write path exists. |
| routine | `assignee_name` | Not resolved. Export-only; no write path exists. |

`desired_skills` is late-bound by design, not by omission. The column holds
slugs and is read at skill-manifest build time, where `ParseDesiredSlugs` accepts
either the canonical JSON array or the legacy comma-separated form and skips an
unmatched slug without error. Resolving it at apply time would replace
a working lazy binding with an eager one that fails earlier and repairs itself
never; sync stores the declared value and neither decomposes, reorders, nor
validates it. `skill_ids` is a separate, already-resolved column that sync does
not write.

`lead_agent_name` and `assignee_name` are not resolved because there is nothing
to resolve *into*. `UpdateProjectConfigFields` leaves `lead_agent_profile_id`
untouched and `UpdateRoutineConfigFields` leaves `assignee_agent_profile_id`
untouched, each with a comment saying so, and
`TestApplyImport_NameReferencesAreNotResolved` pins that as the contract for
filesystem import. Provider sync makes the same choice, so it adds no resolver
and no write path. Wiring resolution in is a separate change to a shipped
contract and belongs to whatever requirement asks for it.

That leaves `reports_to` as the whole of reference resolution. An unresolvable
name leaves the field empty, records a warning, and does not fail the run; because
a `reports_to` names exactly one agent, there is no partial-resolution case and no
multi-valued path. A name fetched but not applied — its file failed to
parse, was unreadable, or lost a collision — counts as unresolvable, because
resolution is against what was applied
(`AC-OFFICE-CONFIG-SYNC-003.10b`).

#### Sync writes an owned projection, not a whole row

"Update it to the fetched definition" means: write the columns
`AC-OFFICE-CONFIG-SYNC-003.5c` enumerates for that kind, and no others.

| Kind | Writer |
| --- | --- |
| agent | `UpdateAgentInstanceConfigFields`, plus `UpdateAgentReportsTo` for the second pass |
| project | `UpdateProjectConfigFields` |
| routine | `UpdateRoutineConfigFields` |
| skill | a new sync-owned writer |

The narrowness is a concurrency decision made upstream: each shipped writer's
comment leaves status, pause reason, settings and last-run timestamp at whatever a
concurrent writer set them to. A sync run is exactly that concurrent writer — it
lands on a live workspace where agents are running — so whole-row writes would let
a poll stomp a status the runtime had just advanced. Two consequences are stated
in the requirements: a declared field outside the projection (agent `permissions`)
is ignored rather than warned about, because Office's own export writes those
fields; and idempotency (`AC-OFFICE-CONFIG-SYNC-003.11`) claims only the
projection, not the row.

**The skill row is not the shipped writer.** `UpdateSkillConfigFields`
deliberately excludes `file_inventory` — it CAS-guards on the inventory and
retries — because a filesystem import reads one `SKILL.md` and cannot replace what
materialization produced; `skills_config_fields_test.go` pins that. Sync reads the
whole directory, so the inventory is part of the fetched definition and sync's to
write (`AC-OFFICE-CONFIG-SYNC-002.1a`,
`AC-OFFICE-CONFIG-SYNC-003.5d`). Sync therefore uses its **own** writer, not a
widened shipped one, leaving `SkillConfigFields`, its single caller
(`internal/office/config/import.go`) and that test untouched. It sets all six in
one `UPDATE` guarded on `source_locator`, computing the hash inside it:
`SkillPackageContentHash` folds content, inventory and locator together, so a split write would leave the hash describing an inventory already
gone (`AC-OFFICE-CONFIG-SYNC-003.5e`). Zero rows means another writer moved the
locator, so the entity fails, rolls back, warns, and the run continues; it does
**not** retry inside its own transaction, where a re-read would see the same
snapshot and never observe the writer it lost to.

#### `reports_to` needs a second pass

`reports_to` is the one reference that points *within* a kind, so kind ordering
cannot resolve it: an agent may report to one applied later in the same run.
Office already solves this for filesystem import and this design reuses that
shape. `internal/office/config/import.go` runs `applyAgentReportsTo` after every
agent is written, re-listing the workspace so identifiers assigned to new rows
are visible and "resolution is independent of bundle order"; `resolveReportsTo`
warns and empties the field for a self-reference, a name matching nothing, or an
edge `reportsToCycleExists` shows would close a cycle. That walk
is bounded by a visited set *and* a hop cap, so a graph already cyclic in the
database cannot hang resolution.

The one parameter set differently is that function's `allowExternalManagers`,
which provider sync passes as false: a `reports_to` naming an agent outside the
fetched set is unresolvable rather than resolved against an unmanaged agent.
Otherwise a managed entity would depend on one sync does not own and may not
modify (`AC-OFFICE-CONFIG-SYNC-003.8`), and a later hand-deletion of
that unmanaged agent would silently break a managed one. Filesystem import
passes false because its snapshot is authoritative.

The atomic unit is one entity plus its manifest row, and nothing larger: a
create-or-update commits with its manifest write, a delete with its manifest-row
removal. The manifest is therefore written as each entity is, not batched at the
end, so a mid-run failure leaves it describing exactly what was persisted and a
retry converges (`AC-OFFICE-CONFIG-SYNC-003.14`).

**The seam that makes that possible.** The four shipped config-field writers this
design reuses (`UpdateAgentInstanceConfigFields`, `UpdateAgentReportsTo`,
`UpdateProjectConfigFields`, `UpdateRoutineConfigFields`) take `(ctx, id, fields)`
and call `r.db.ExecContext` directly, so none accepts a transaction today. Each is
re-bodied over `sqlx.ExtContext` — the interface both `*sqlx.DB` and `*sqlx.Tx`
satisfy, `Rebind` included — with a tx-accepting variant beside it. Public
signatures and behavior are unchanged, so no shipped test is touched and no
caller edited. `UpdateSkillConfigFields` is deliberately **not** in that list: it is a
read-then-CAS retry loop, not a single `ExecContext`, and sync never calls it —
the skill writer above is new and takes `sqlx.ExtContext` from the start.
The transaction needs no new plumbing:
`runs/repository/sqlite.Repository.Writer() *sqlx.DB` is already exported,
inherited by the Office `Repository` through embedding, and returns the very
handle those writers use, so `repo.Writer().BeginTxx(ctx, nil)` reaches them
(`blockers_test.go` already calls it). Manifest writes take the same
`sqlx.ExtContext`, putting an entity and its manifest row in one transaction.
`AC-OFFICE-CONFIG-SYNC-003.14` is satisfied as written, not weakened to an
ordering rule.

There is no run-wide transaction, and the design does not want one: a run spans
up to 1000 files and minutes of provider I/O, and holding a SQLite write
transaction across that blocks every other writer in the workspace. Per-entity
atomicity suffices for convergence, because every run reconciles in full rather
than resuming a position. Run status and the
content digest are written once at the end, outside every entity transaction;
they are diagnostics, and a crash between the last entity write and the status
write leaves a correct manifest beside a stale status the next run overwrites.

## Failure and recovery

| Condition | Behavior |
| --- | --- |
| Provider not connected for the workspace | Run fails with a message naming that provider. Saving the config still succeeds, so either order works. |
| Token cannot read the repository | The root listing fails; run fails, wrapped with repository, branch, and directory. |
| Configured root missing on the branch | Run fails and is recorded. Indistinguishable from the row above by design, since both providers answer 404 for a path the credential cannot see. |
| A kind directory returns not-found | That kind contributes no files; run succeeds. Safe only because the root listing already succeeded. |
| A kind directory listing fails for any other reason | Run fails. Never treated as an empty directory, which would delete that kind's managed entities on a transient outage. |
| A kind directory present but empty | Its managed entities are deleted. This is a real delete, not an error. |
| File over the 1 MiB limit, or gone at fetch time | Warning; the entity is found by `source_path` in the manifest, keeps its current state, and is not deleted. |
| File fetch fails for any other reason | `ErrUnreadable`; same warning and same deletion exemption as an oversized file. Never fails the run. |
| A file is renamed and unreadable in the same commit | No manifest entry matches either path, so deletion is suspended for that whole kind this run, with a warning naming the exempted keys and the unmapped file. |
| Walk exceeds a cap | Warning naming which cap and the dropped count; run fails without applying. |
| File fails to parse or validate | Warning; the entity is found by `source_path` and left untouched, not deleted. |
| `reports_to` self-reference or cycle | Field left empty with a distinguishing warning; run succeeds. |
| `reports_to` names an agent this run did not apply | Field left empty with a warning; the entity is still applied and the run succeeds. |
| Run exceeds its 10 minute deadline | Abandoned and recorded as a failure naming the deadline. Entities already written stay written; the manifest matches them and the next run converges. |
| A push lands mid-run | Accepted. The view may mix revisions; full reconciliation on the next run corrects it within one poll interval. |
| Apply fails partway | Manifest reflects what was actually written; status records the failure; retry converges. |
| Any failure | `last_hash` cleared so the recorded digest never claims an unapplied state; poller continues. Reconciliation is unaffected either way, since no run consults the digest. |
| Release fails partway on config delete | Stops at the failing entity; entities already released stay released and have no manifest row; config retained; retry resumes from the remainder. |
| Manifest names an entity deleted out of band | Manifest entry dropped, run continues. |
| Delete of a nonexistent config | Succeeds with no effect. |

