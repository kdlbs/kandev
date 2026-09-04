---
status: draft
system: office
created: 2026-09-01
owners:
  - kandev
---

## Overview

This document specifies what a single Office config sync run *does* to the
workspace's config entities: the identity and ownership rules, and the
reconciliation that makes managed configuration match the repository without
touching anything a user made by hand.

It is one third of a single capability. [Office Config Sync](config-sync.md)
covers declaring and validating a source, scheduling, status, coexistence with
the raw-git and filesystem surfaces, and the settings page.
[Office Config Sync Fetch](config-sync-fetch.md) specifies the traversal that
produces the fetched set this document reconciles against, including the file
selection, error classification, and limits its criteria refer to. Read both
before this one.

## Terminology

This document uses the terms defined in
[Office Config Sync § Terminology](config-sync.md), unchanged: *config source*,
*run*, *config entity*, *managed*, *unmanaged*, and *applied manifest*.

## Requirements

### REQ-OFFICE-CONFIG-SYNC-003: Applying and reconciling

**Intent:** A sync run makes the workspace's managed configuration match the
repository, without ever touching configuration a user created by hand.

**User story:** As an operator, I want a deleted definition file to remove the
agent it defined, so the repository is genuinely the source of truth.

#### Acceptance criteria

- **AC-OFFICE-CONFIG-SYNC-003.1:** A config entity's identity shall be the pair
  (kind, key), where key is the declared `name` for an agent, project, or
  routine and the directory name for a skill. Identity shall exclude the file
  path, so relocating a file within its kind's directory updates the existing
  entity rather than creating a second one.
- **AC-OFFICE-CONFIG-SYNC-003.1a:** The applied manifest shall additionally
  record, for each managed entity, the repository path of the file it was last
  applied from. That path is metadata only: not part of identity, no effect on
  matching, rewritten when a file moves. It exists so a file the run cannot read
  or parse can still be mapped to the entity it defines
  (AC-OFFICE-CONFIG-SYNC-002.6a, AC-OFFICE-CONFIG-SYNC-003.12), which the file's
  own contents cannot do when the contents are what failed.
- **AC-OFFICE-CONFIG-SYNC-003.2:** When an agent, project, or routine file's
  declared `name` differs from its filename stem, the system shall use the
  declared `name` as the key and record a warning naming both. This criterion
  does not apply to skills: a skill's file is always `SKILL.md`, whose stem
  carries no name, and AC-OFFICE-CONFIG-SYNC-003.1 already makes the skill's key
  its directory name.
- **AC-OFFICE-CONFIG-SYNC-003.2a:** When a skill directory contains no
  `SKILL.md`, it shall define no skill: the run shall record a warning naming
  the directory and shall apply no entity for it, whatever it holds under
  `references/`. A skill has no content without its definition file, and
  AC-OFFICE-CONFIG-SYNC-002.1a makes `references/` an inventory attached to that
  content rather than a definition in its own right. A previously managed skill
  whose `SKILL.md` was removed is therefore deleted by the sweep
  (AC-OFFICE-CONFIG-SYNC-003.6). That is correct rather than destructive: the
  definition is genuinely absent from the listing, which is not the unreadable
  file that AC-OFFICE-CONFIG-SYNC-002.6a exempts.
- **AC-OFFICE-CONFIG-SYNC-003.3:** When two files of the same kind resolve to
  the same key, the system shall apply the one whose full repository path sorts
  first byte-wise and record a warning naming the key and every losing path.
  The winner shall not depend on the listing order the provider returned.
- **AC-OFFICE-CONFIG-SYNC-003.4:** When a fetched entity has no matching
  entity in the workspace, the system shall create it and add it to the
  applied manifest.
- **AC-OFFICE-CONFIG-SYNC-003.5:** When a fetched entity matches one in the
  applied manifest, the system shall update its *owned fields* to the fetched
  definition, repairing edits made in the Kandev UI since the last run.
- **AC-OFFICE-CONFIG-SYNC-003.5c:** The owned fields are enumerated per kind
  below, and no other column shall be written. For agent, project, and routine
  the enumeration is exactly what that kind's shipped config-field writer already
  updates; for skill it is deliberately wider, for the reason
  AC-OFFICE-CONFIG-SYNC-003.5d gives:
  - **agent:** role, icon, budget, max concurrent sessions, desired skills,
    executor preference, and `reports_to` through its own narrow write.
  - **project:** description, color, budget, repositories, executor config.
  - **routine:** description, task template, concurrency policy.
  - **skill:** name, description, source type, the `SKILL.md` content, the file
    inventory built from `references/` (AC-OFFICE-CONFIG-SYNC-002.1a), and the
    content hash derived from content, inventory, and the package source locator
    together.
- **AC-OFFICE-CONFIG-SYNC-003.5d:** A skill's owned fields include its file
  inventory, which the shipped config-import writer deliberately does not write,
  because the two paths own different things. A filesystem config import reads a
  single `SKILL.md` and has no `references/` to declare, so the inventory belongs
  to whatever materialized the package. A provider sync run reads the whole skill
  directory, so the inventory is part of the fetched definition, and a skill
  directory with no `references/` declares an *empty* inventory that the run
  shall write as empty rather than leaving the prior value in place. The package
  *source locator* stays outside the projection in both paths: sync never writes
  it, because it records where a package was materialized from and sync does not
  materialize.
- **AC-OFFICE-CONFIG-SYNC-003.5e:** A skill's content, inventory, and content
  hash shall be written as one indivisible change, conditional on the package
  source locator being unchanged since the run read it. If it changed, the run
  shall leave that skill untouched, record a warning naming it, and continue; the
  next run reconciles it. The hash shall never be written from an inventory
  older than the content it accompanies. Because the hash is derived rather than
  fetched, it shall be recomputed on write rather than treated as an input to the
  changed-or-not comparison of AC-OFFICE-CONFIG-SYNC-003.5b; otherwise a package
  locator moved out of band between two runs over an unchanged repository would
  report that skill as updated.

  Everything else shall be left at whatever a concurrent writer set it to:
  runtime state (status, pause reason, last-run timestamps, routine variables
  and catch-up counters), the agent links AC-OFFICE-CONFIG-SYNC-003.9a excludes,
  and the agent `permissions` field. The reason is concurrency: a run reads,
  then writes, and a full-row write would revert any column the runtime changed
  in between. A field a repository declares but sync does not apply shall be
  ignored silently rather than warned about, because Office's own export writes
  those fields and warning would fire on almost every entity of a repository
  this capability is meant to consume.
- **AC-OFFICE-CONFIG-SYNC-003.5a:** Every run shall reconcile the full fetched
  set against the workspace, and the recorded content digest shall never be used
  to skip reconciliation. UI drift is only observable when the repository has
  *not* changed, so stopping early on an unchanged digest would make
  AC-OFFICE-CONFIG-SYNC-003.5 unreachable in the case it exists for. Shipped
  workflow sync works the same way.
- **AC-OFFICE-CONFIG-SYNC-003.5b:** A run shall be reported as unchanged when it
  produced no creates, no updates, and no deletes. That verdict shall be derived
  from what the apply step did, not from comparing digests, so a run that
  repaired UI drift is reported as changed even though the repository was
  byte-identical. Warnings shall not enter the verdict, because a run can warn
  on every pass without writing anything: a repository with no `kandev.yml`
  warns every run under AC-OFFICE-CONFIG-SYNC-002.1b, and counting warnings
  would make "unchanged" permanently unreachable for it. Warnings are reported
  alongside the verdict, never folded into it.
- **AC-OFFICE-CONFIG-SYNC-003.6:** When a key is in the applied manifest and
  absent from the fetched set, the system shall delete that entity and remove
  it from the manifest.
- **AC-OFFICE-CONFIG-SYNC-003.6a:** A manifest entry shall be exempted from
  deletion under AC-OFFICE-CONFIG-SYNC-003.6, and left untouched, when this run
  recorded at least one file of that entry's kind that it could not read or
  could not parse and whose repository path matched no manifest entry. The run
  shall warn naming the kind, every key exempted, and every such unmapped file.
  This covers the one case the path lookup in AC-OFFICE-CONFIG-SYNC-002.6a and
  AC-OFFICE-CONFIG-SYNC-003.12 cannot reach: a file both moved and made
  unreadable since the last run has a new path the manifest never recorded and
  an old path the listing no longer contains, so neither end matches and the
  entity would be deleted for a failure that is not a removal. The exemption is
  coarse on purpose, because a file whose contents could not be read cannot say
  which entity it defines, and a postponed deletion is corrected next run while
  a wrongly deleted entity is not recoverable.
- **AC-OFFICE-CONFIG-SYNC-003.7:** When a fetched entity's key collides with an
  unmanaged entity of the same kind, the system shall neither modify nor adopt
  that entity, and shall record a warning naming the kind and key. Sync never
  silently takes ownership of something a user made.
- **AC-OFFICE-CONFIG-SYNC-003.8:** An entity that is not in the applied
  manifest shall never be modified or deleted by a sync run.
- **AC-OFFICE-CONFIG-SYNC-003.9:** Entities shall be applied in a fixed kind
  order (skills, then agents, then projects, then routines), and deletions shall
  run in reverse. The order exists for determinism, not reference resolution:
  after AC-OFFICE-CONFIG-SYNC-003.9a no cross-kind reference remains, and the
  one that does remain points *within* a kind and is handled by
  AC-OFFICE-CONFIG-SYNC-003.9b. Fixing it still matters, because it makes the
  sequence of writes, the order of warnings, and the state left by a partial
  failure the same for a given repository on every run. Within a kind, applies
  follow ascending `source_path`, byte-wise, and deletions follow ascending
  `entity_key`, byte-wise. `source_path` rather than file order because an
  entity is not always one file (AC-OFFICE-CONFIG-SYNC-003.9d). No further
  tiebreak is needed: repository paths are unique within one tree, and
  `(workspace_id, kind, entity_key)` is the manifest's primary key, so
  `entity_key` is unique within a kind.
- **AC-OFFICE-CONFIG-SYNC-003.9d:** A skill spans `SKILL.md` plus every file
  under its `references/`, so the file order of AC-OFFICE-CONFIG-SYNC-002.7
  does not by itself order skills. Every entity shall be applied exactly once,
  as a unit, after every file it spans has been read, and a skill's position
  shall be its `source_path`, which AC-OFFICE-CONFIG-SYNC-002.6a already
  defines as the skill's directory. A skill shall never be applied at its
  `SKILL.md` and amended as its `references/` files arrive: that would let a
  file exempted under AC-OFFICE-CONFIG-SYNC-002.6a change an entity after it
  was written, which AC-OFFICE-CONFIG-SYNC-003.14 relies on not happening.
- **AC-OFFICE-CONFIG-SYNC-003.9e:** Ordering by directory needs no extra rule
  to be well defined, because a skill's files are already contiguous under
  AC-OFFICE-CONFIG-SYNC-002.7: every file of skill `S` begins with `skills/S/`,
  no other skill's file can, since a directory name cannot contain `/`.
  Ordering by a skill's first constituent file and by its last therefore give
  the same sequence. Nothing here shall depend on `SKILL.md` sorting before
  `references/`, which holds only because `S` is `0x53` and `r` is `0x72`, and
  would silently invert if a future layout put a skill's definition in a
  lowercase file.
- **AC-OFFICE-CONFIG-SYNC-003.9a:** The one reference field is an agent's
  `reports_to`, which names one agent. No other field shall be resolved against
  the fetched set. Three fields that look like references are deliberately not
  resolved, and this criterion makes that a contract rather than an omission:
  - `desired_skills` is a late-bound slug list, not a reference. It shall be
    stored exactly as declared; a run shall neither decompose, reorder, nor
    validate it. Slugs resolve only when an agent's skill manifest is built,
    where an unmatched slug is already skipped without error.
  - A project's `lead_agent_name` and a routine's `assignee_name` are
    export-only in the Office config format. The shipped filesystem importer
    resolves neither, and the config-field writers for both kinds deliberately
    leave `lead_agent_profile_id` and `assignee_agent_profile_id` untouched.
    Provider sync shall make the same choice, introducing no resolver and no
    write path those kinds do not already have.

  Fields naming things owned by other systems, such as an agent's
  `executor_preference`, are not reference fields for this criterion.
- **AC-OFFICE-CONFIG-SYNC-003.9b:** Because `reports_to` names an agent, kind
  order cannot resolve it: an agent may report to one applied later in the same
  kind. The system shall therefore resolve it in a second pass, after every
  agent in the run is written, re-reading the workspace so identifiers assigned
  to new agents are visible and resolution does not depend on file order. A
  `reports_to` naming an agent outside the fetched set shall be unresolvable
  under AC-OFFICE-CONFIG-SYNC-003.10 rather than resolved against an unmanaged
  agent, so sync never makes a managed entity depend on one it does not own.
- **AC-OFFICE-CONFIG-SYNC-003.9c:** When an agent's `reports_to` names itself,
  or names an agent whose own resolved chain leads back to it, the system shall
  leave the field empty and record a warning distinguishing the self-reference
  from the cycle. Neither shall fail the run, and cycle detection shall
  terminate on a reporting graph that is already cyclic in the database.
- **AC-OFFICE-CONFIG-SYNC-003.10:** When a reference names an entity this run
  did not apply, the system shall apply the referring entity with that reference
  left empty and record a warning naming the referring entity, the field, and
  the unresolved name. An unresolvable reference does not fail the run.
  Resolution is against the set this run applied and never against entities the
  workspace already had; that is one rule for one field, since
  AC-OFFICE-CONFIG-SYNC-003.9a leaves no other reference field. Because a
  reference names exactly one entity, there is no partial-resolution case.
- **AC-OFFICE-CONFIG-SYNC-003.10b:** A reference that names an entity present in
  the fetched set but not applied — because its file failed to parse, was
  unreadable, or lost a key collision — shall be treated as unresolvable under
  AC-OFFICE-CONFIG-SYNC-003.10, with a warning distinguishing it from a name
  that appears nowhere. Being fetched is not being applied.
- **AC-OFFICE-CONFIG-SYNC-003.11:** Running a sync twice against an unchanged
  repository shall produce no creates, no updates, and no deletes on the second
  run, and shall leave every owned field (AC-OFFICE-CONFIG-SYNC-003.5c) of every
  managed entity unchanged. It claims nothing about columns outside that
  projection: the runtime may advance a status or a timestamp between the runs,
  and idempotency is a statement about what sync writes, not what the row
  contains.
- **AC-OFFICE-CONFIG-SYNC-003.12:** When a file fails to parse or validate, the
  system shall record a warning naming the file and the reason, and shall leave
  the entity that file previously defined untouched rather than deleting it: a
  file that cannot be parsed is not a file that was removed. The entity is found
  by looking the file's path up in the applied manifest
  (AC-OFFICE-CONFIG-SYNC-003.1a), because contents that failed to parse cannot
  yield the declared `name` AC-OFFICE-CONFIG-SYNC-003.1 makes the key. A skill
  is identified by its directory name, which the listing already gave. When no
  manifest entry carries the path, there is nothing to exempt.
- **AC-OFFICE-CONFIG-SYNC-003.13:** Apply shall be scoped to the workspace
  named in the config record. It shall not read or write config entities
  belonging to another workspace.
- **AC-OFFICE-CONFIG-SYNC-003.14:** When the apply step fails partway, the
  applied manifest shall reflect exactly the entities actually written, so a
  retry converges rather than orphaning or double-deleting.
- **AC-OFFICE-CONFIG-SYNC-003.15:** When a manifest entry names an entity that
  no longer exists, the run shall drop the manifest entry and continue, treating
  the out-of-band deletion as already applied rather than failing on it.

## Out of scope

- **Adopting existing unmanaged entities into sync ownership**, excluded by
  AC-OFFICE-CONFIG-SYNC-003.7; it would let a repository silently take over
  hand-made configuration.
- **Fixing the pre-existing workspace scoping of the filesystem-to-database
  path.** `ScanFilesystem` reads a hardcoded `default` workspace directory and
  `ExportBundle` lists entities with an empty workspace filter. Provider sync is
  workspace-scoped by AC-OFFICE-CONFIG-SYNC-003.13 and does not reuse those
  paths; repairing them is separate work with its own migration question.
- **Declaring a source, scheduling, status reporting, coexistence with the
  raw-git and filesystem surfaces, and the settings page**, in
  [Office Config Sync](config-sync.md); and **the traversal, file selection,
  error classification, and limits**, in
  [Office Config Sync Fetch](config-sync-fetch.md).
