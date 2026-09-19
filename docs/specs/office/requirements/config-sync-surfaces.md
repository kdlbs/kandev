---
status: draft
system: office
created: 2026-09-01
owners:
  - kandev
---

## Overview

This document specifies how provider-routed Office config sync coexists with
the two configuration surfaces Office already has (a filesystem-to-database
diff page and a raw-git section), and what an operator sees and can do while a
config source is active.

It is one part of a single capability. [Office Config Sync](config-sync.md)
covers declaring and validating a source, scheduling, and status; read it
first, because this document assumes the source binding it establishes.
[Office Config Sync Fetch](config-sync-fetch.md) covers what a run reads, and
[Office Config Sync Reconciliation](config-sync-reconciliation.md) covers what
a run does to config entities.

## Terminology

This document uses the terms defined in
[Office Config Sync § Terminology](config-sync.md), unchanged: *config entity*,
*config source*, *sync run*, *applied manifest*, *managed*, and *unmanaged*.

## Requirements

### REQ-OFFICE-CONFIG-SYNC-005: Coexistence with the existing surfaces

**Intent:** Office already has a filesystem-to-database apply and a raw-git
section. Two reconcilers that each delete what they do not see would fight, so
the interaction is stated rather than discovered.

**User story:** As an operator, I want Kandev to tell me provider sync owns my
configuration, so I do not half-apply a stale local checkout over it.

#### Acceptance criteria

- **AC-OFFICE-CONFIG-SYNC-005.1:** Provider sync shall apply to the database
  only. It shall not write to the workspace directory on disk, so it cannot
  collide with a raw-git checkout.
- **AC-OFFICE-CONFIG-SYNC-005.2:** When a config source is configured for a
  workspace, every action that writes that workspace's config entities or its
  workspace directory shall be refused with a conflict naming provider sync as
  the active source. The refused set is exactly:
  - the filesystem-to-database apply (`ApplyIncoming`);
  - the definition-bundle import (`ApplyImport`), which writes the same four
    kinds from an uploaded bundle rather than from disk;
  - the database-to-filesystem export (`ApplyOutgoing`), for the round-trip
    reason in AC-OFFICE-CONFIG-SYNC-005.2a;
  - raw-git `clone` and `pull`.
- **AC-OFFICE-CONFIG-SYNC-005.2a:** The database-to-filesystem export is
  refused for a different reason than the other three, and the difference is
  recorded so a later reader does not "simplify" it away. It does not race
  provider sync (AC-OFFICE-CONFIG-SYNC-005.1 keeps sync out of the workspace
  directory entirely). It is refused because it manufactures a repository write
  path this capability excludes: it writes provider-sourced database state onto
  the checkout, where raw-git `push`
  (AC-OFFICE-CONFIG-SYNC-005.5) would commit it back as if authored locally.
  For a managed entity that push is a no-op or a loop; for an unmanaged entity
  it promotes a key into the repository that the next run then reports as a
  permanent collision under AC-OFFICE-CONFIG-SYNC-003.7.
- **AC-OFFICE-CONFIG-SYNC-005.2b:** The list in
  AC-OFFICE-CONFIG-SYNC-005.2 is closed, not illustrative: it is stated against
  the config and dashboard route tables so that a writing route absent from the
  list is a defect in this criterion rather than an implementer's judgment call.
  Adding a route that writes a config entity or the workspace directory requires
  amending that list.
- **AC-OFFICE-CONFIG-SYNC-005.3:** When a config source is configured, the
  read-only filesystem diff views and the read-only bundle export shall continue
  to work unchanged. "Read-only" is the test, not the word *export*: the bundle
  export returns a document to the caller and writes nothing, while the
  database-to-filesystem export writes the workspace directory and is refused by
  AC-OFFICE-CONFIG-SYNC-005.2.
- **AC-OFFICE-CONFIG-SYNC-005.4:** When no config source is configured, every
  existing filesystem and raw-git behavior shall be unchanged.
- **AC-OFFICE-CONFIG-SYNC-005.5:** The raw-git push path shall remain the only
  way to write Office configuration back to a repository, and shall keep using
  the backend host's git credentials. It is never refused. What it pushes is
  whatever the operator placed in the checkout by hand or by `git`, never a
  Kandev-generated export (AC-OFFICE-CONFIG-SYNC-005.2a).

### REQ-OFFICE-CONFIG-SYNC-006: Settings surface

**Intent:** The Office workspace settings page gains provider-aware
configuration and status consistent with shipped workflow sync settings.

**User story:** As an operator, I want to configure and check Office config
sync the way I configure workflow sync, so I do not learn a second mental
model.

#### Acceptance criteria

- **AC-OFFICE-CONFIG-SYNC-006.1:** The settings surface shall let an operator
  choose the provider, enter that provider's repository identity, and set
  branch, directory, interval, and whether polling is on.
- **AC-OFFICE-CONFIG-SYNC-006.2:** When the operator switches provider, the
  fields belonging to the other provider shall be cleared, so a save cannot
  carry both identities.
- **AC-OFFICE-CONFIG-SYNC-006.3:** The surface shall show the last attempt
  time, success or failure, the failure message, and any warnings, and shall
  refresh without a page reload so poller results become visible. Warnings shall
  appear in the recorded order (AC-OFFICE-CONFIG-SYNC-004.5a), at most the first
  10, with a count of any beyond that rather than an unbounded list.
- **AC-OFFICE-CONFIG-SYNC-006.4:** An empty directory shall be displayed as the
  repository root rather than a blank field.
- **AC-OFFICE-CONFIG-SYNC-006.5:** The surface shall offer an immediate sync
  and a remove-configuration action, and shall report the outcome of each.
- **AC-OFFICE-CONFIG-SYNC-006.6:** When provider sync is active, every control
  that invokes an action refused by AC-OFFICE-CONFIG-SYNC-005.2 shall be shown
  as unavailable with the reason, rather than offering an action that will be
  refused. The rule is per action, not per page or per section, because neither
  existing surface groups its controls by whether provider sync refuses them:
  - in the raw-git section, `clone` and `pull` shall be shown as unavailable
    while `push` remains available, even though all three are rendered
    together;
  - on the configuration sync page, the apply-incoming and apply-outgoing
    controls shall be shown as unavailable while that page's read-only diff
    views continue to render.
- **AC-OFFICE-CONFIG-SYNC-006.6a:** A control shown as unavailable under
  AC-OFFICE-CONFIG-SYNC-006.6 shall state that provider sync is the active
  source, matching the conflict the server would have returned, so the two
  surfaces do not give an operator two different explanations for one refusal.
- **AC-OFFICE-CONFIG-SYNC-006.7:** All new *interface* copy shall be localized in
  every supported locale, with no hardcoded literal and no em dash. Interface
  copy is the text the surface itself owns: labels, field names, buttons,
  status phrasing, and the frame the warning list is rendered in.
- **AC-OFFICE-CONFIG-SYNC-006.7a:** Server-generated warning text and the
  recorded failure message are diagnostic strings, not interface copy, and shall
  be displayed verbatim rather than translated. They are produced by a run and
  persisted on the config row (AC-OFFICE-CONFIG-SYNC-004.5), so they carry
  repository paths, entity keys, and provider messages that must match what the
  log and the API return; translating them would break that correspondence and
  would require a message catalog on the server, which no requirement asks for.
  This resolves the apparent conflict with AC-OFFICE-CONFIG-SYNC-006.3, which
  requires those warnings to be shown: they are shown, in English, inside a
  localized frame. Any count or truncation notice the surface adds around them
  is interface copy and falls under AC-OFFICE-CONFIG-SYNC-006.7.

## Out of scope

- **Promoting a Kandev-created entity into the repository.** Refusing the
  database-to-filesystem export (AC-OFFICE-CONFIG-SYNC-005.2a) closes the one
  path an operator had for taking an entity created in the Kandev UI and
  committing it to the synced repository. That workflow is real and is
  deliberately left unbuilt here, because doing it properly is its own design:
  it needs a rule for which entities are eligible (unmanaged only, or managed
  edits too), a way for the applied manifest to adopt the entity on the next
  run instead of reporting the AC-OFFICE-CONFIG-SYNC-003.7 collision, and a
  decision about whether Kandev writes the file or only shows the operator what
  to commit. A follow-up would need all three. The available workflow in the
  meantime is to author the definition file in the repository directly.
- **Splitting the raw-git section into separate surfaces.**
  AC-OFFICE-CONFIG-SYNC-006.6 requires per-action availability within the
  existing section; it does not require the section to be reorganized, and no
  criterion here depends on where the controls are grouped.
- **Declaring, scheduling, or reporting a config source**, in
  [Office Config Sync](config-sync.md); **what a run reads**, in
  [Office Config Sync Fetch](config-sync-fetch.md); and **what it does to
  entities**, in
  [Office Config Sync Reconciliation](config-sync-reconciliation.md). Each has
  its own exclusions.
