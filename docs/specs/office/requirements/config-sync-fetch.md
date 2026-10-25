---
status: draft
system: office
created: 2026-09-01
owners:
  - kandev
---

## Overview

This document specifies what a single Office config sync run *reads* from the
configured repository: the bounded traversal of the Office config layout, which
files are selected, how a listing or fetch failure is classified, and the limits
a run is held to.

It is one third of a single capability. [Office Config Sync](config-sync.md)
covers declaring and validating a source, scheduling, status, coexistence with
the raw-git and filesystem surfaces, and the settings page; read it first,
because this document assumes the source binding it establishes.
[Office Config Sync Reconciliation](config-sync-reconciliation.md) specifies
what a run then *does* to the workspace's config entities, and reads the fetched
set this document defines.

## Terminology

This document uses the terms defined in
[Office Config Sync § Terminology](config-sync.md), unchanged: *config source*,
*run*, *config entity*, *managed*, *unmanaged*, and *applied manifest*.

## Requirements

### REQ-OFFICE-CONFIG-SYNC-002: Fetching the Office config tree

**Intent:** Office configuration is a directory tree, not the single flat
directory workflow sync reads, and both providers expose only non-recursive
listings. The traversal must be complete, bounded, and honest about
truncation.

**User story:** As an operator, I want every agent, skill, project, and
routine in my directory picked up, so a synced workspace matches the repository
I reviewed.

#### Acceptance criteria

- **AC-OFFICE-CONFIG-SYNC-002.1:** A run shall read every `*.yml` and `*.yaml`
  file directly in `agents/`, `projects/`, and `routines/` beneath the
  configured path, and, in each immediate subdirectory of `skills/`, its
  `SKILL.md` plus every file directly in that skill's `references/` directory.
  It shall not fetch the contents of any other file. This is the Office config
  layout as committed to a repository; it is deliberately *not* a description of
  what the Office config loader reads from disk, which is narrower
  (AC-OFFICE-CONFIG-SYNC-002.1c).
- **AC-OFFICE-CONFIG-SYNC-002.1b:** A run shall not apply workspace settings.
  `kandev.yml` shall be neither fetched nor parsed; its presence in the root
  listing shall be used only as a signal that the configured path looks like an
  Office config root, and its absence shall warn without failing the run. A repository shall never be able to change a workspace's permission
  handling mode, approval default, budget default, default executor, or default
  agent profile: those are operator safety controls with their own update path,
  and a poll that widened an agent's permission posture would be a privilege
  escalation with no human in the loop. The four kinds in
  [Terminology](#terminology) are the entire sync unit set.
- **AC-OFFICE-CONFIG-SYNC-002.1a:** A skill's files under `references/` shall be
  applied as that skill's file inventory, so a repository-synced skill supports
  the same progressive disclosure as a bundled system skill (ADR 0031). A skill
  with no `references/` directory shall sync with an empty inventory.
- **AC-OFFICE-CONFIG-SYNC-002.1c:** AC-OFFICE-CONFIG-SYNC-002.1a is new
  behavior, not reuse of an existing read path. ADR 0031 defines `references/`
  and the file inventory, and the bundled system-skill sync populates it, but
  the Office workspace config loader reads `skills/*/SKILL.md` only, so a
  filesystem-imported skill gets no inventory today. Provider sync diverges from
  that loader here deliberately, and the divergence is limited to this one
  criterion.
- **AC-OFFICE-CONFIG-SYNC-002.2:** The traversal shall use only the existing
  non-recursive provider read surface: no recursive listing mode on either
  provider and no new provider client method.
- **AC-OFFICE-CONFIG-SYNC-002.3:** After the configured path has been listed
  successfully, when a listing of a directory beneath it reports not-found, the
  system shall treat that kind as contributing no files and continue with the
  remaining kinds. A repository with no `routines/` directory is a valid config
  source. The successful root listing is what makes this safe: it proves the
  credential can read this repository on this branch, so a not-found beneath it
  is a genuinely absent directory rather than a path the token cannot see.
- **AC-OFFICE-CONFIG-SYNC-002.3a:** When a listing fails for any reason other
  than not-found — including unauthorized, forbidden, rate-limited, any server
  error, a timeout, and any transport failure — the system shall fail the run
  and record the failure. It shall never treat such a failure as an empty
  directory, because doing so would feed AC-OFFICE-CONFIG-SYNC-003.6 and delete
  every managed entity of that kind on a transient outage.
- **AC-OFFICE-CONFIG-SYNC-002.4:** When the listing of the configured path
  itself fails for any reason, including not-found, the system shall fail the
  run and record the failure. Unlike AC-OFFICE-CONFIG-SYNC-002.3, a not-found at
  the root is not read as an empty config: both providers answer not-found for a
  path the credential cannot see, so an absent root and an unreadable repository
  are indistinguishable here and both are misconfigurations.
- **AC-OFFICE-CONFIG-SYNC-002.4a:** A failure to fetch one file shall be
  resolved into exactly one of three outcomes, and the three shall be exhaustive
  so that no fetch failure is left without a defined behavior:
  - **Unavailable** — a provider status of 401, 403, 429, or any 5xx, or a
    timeout or transport failure. The system shall fail the run: the
    *repository* could not be read, so continuing would feed
    AC-OFFICE-CONFIG-SYNC-003.6 a false picture.
  - **Not found** — the file was listed but had disappeared before it was
    fetched. Warn naming the file; treat it as unreadable under
    AC-OFFICE-CONFIG-SYNC-002.6a.
  - **Unreadable content** — any other failure. Warn naming the file and the
    reason, treat it as unreadable under AC-OFFICE-CONFIG-SYNC-002.6a, and do
    not fail the run.

  The third outcome makes the three exhaustive: both clients report a file they
  refuse to decode with an untyped error, so the residue must be classified by
  exclusion rather than by matching. Warn-and-exempt is the safe direction, since
  a warned run applies nothing wrong and deletes nothing.
- **AC-OFFICE-CONFIG-SYNC-002.5:** A run shall traverse at most 200 skill
  subdirectories and fetch at most 1000 files. Both are counted in those units,
  never in provider requests: the listing budget is derived from the skill cap
  rather than configured beside it, so the two can never disagree. When either
  cap would be exceeded, the system shall warn naming which cap was reached and
  how many skill directories or files were dropped, and shall fail the run
  without applying, because applying a truncated fetch would delete every
  managed entity beyond the cap. Either cap can bind first; when a repository
  would exceed both, the warning shall name the skill cap, which is evaluated
  while planning the second round and so is always reached first, making the
  message the same on every run.
- **AC-OFFICE-CONFIG-SYNC-002.6:** The system shall enforce a per-file size
  limit of 1 MiB on every file it receives, measured on the returned content
  before it is parsed, and a file over that limit shall be unreadable under
  AC-OFFICE-CONFIG-SYNC-002.6a. What is guaranteed identical across providers is
  the *outcome*, not the mechanism: a file the run receives is measured here, and
  a file a client refuses to return reaches the same warning and the same
  deletion exemption through AC-OFFICE-CONFIG-SYNC-002.4a's unreadable-content
  class.
- **AC-OFFICE-CONFIG-SYNC-002.6b:** The limit shall not be delegated to the
  provider read surface and shall not be checked before fetching. Neither is
  symmetric: only the GitLab client caps a read, and only a GitHub listing
  reports entry size. Measuring received content is the one check that runs
  identically on both.
- **AC-OFFICE-CONFIG-SYNC-002.6a:** When a file is unreadable — over the size
  limit, or not-found at fetch time under AC-OFFICE-CONFIG-SYNC-002.4a — the
  system shall record a warning naming the file and the reason, and shall exempt
  the entity that file previously defined from deletion, by looking it up in the
  applied manifest by the file's repository path. That path is the definition
  file for an agent, project, or routine and the skill's directory for a skill,
  so an unreadable `SKILL.md` and an unreadable file under that skill's
  `references/` resolve to the same skill. When no manifest entry carries the
  path, there is nothing to exempt. An unreadable file is never a removed file.
- **AC-OFFICE-CONFIG-SYNC-002.7:** Files shall be ordered by full repository
  path, ascending and byte-wise, before parsing and applying, so a run over an
  unchanged repository orders identically every time.
- **AC-OFFICE-CONFIG-SYNC-002.8:** A run shall read the configured branch as it
  stands at each call and shall not pin a commit. A run therefore has no
  snapshot guarantee: a push landing mid-run can leave its view mixed across two
  revisions, and the deletion sweep can act on that mixed set. This is accepted,
  and stated so the window is a known property. Pinning needs a branch-to-SHA
  resolve no client exposes and AC-OFFICE-CONFIG-SYNC-002.2 forbids adding, and
  the exposure is self-correcting *while polling is enabled*: full
  reconciliation every run (AC-OFFICE-CONFIG-SYNC-003.5a) fixes a mixed view on
  the next run, so the window is bounded by one poll interval.
- **AC-OFFICE-CONFIG-SYNC-002.8a:** A run shall not retry to escape a mixed
  view and shall not compare the branch head before and after the run to detect
  one. On a repository under active commit a detect-and-retry rule can fail to
  converge, while the next run does.
- **AC-OFFICE-CONFIG-SYNC-002.8b:** The self-correction in
  AC-OFFICE-CONFIG-SYNC-002.8 and AC-OFFICE-CONFIG-SYNC-002.8a depends on a next
  run existing, which is not guaranteed: a workspace with `poll_enabled` false is
  never selected by the poller (AC-OFFICE-CONFIG-SYNC-004.1) and runs only when a
  run is explicitly requested. With polling off, a mixed-revision view
  therefore persists until the operator requests another run, and any deletion
  that view produced persists with it. This is accepted rather than solved, on
  the same reasoning as AC-OFFICE-CONFIG-SYNC-002.8: pinning a commit needs a
  branch-to-SHA resolve AC-OFFICE-CONFIG-SYNC-002.2 forbids adding. A run shall
  not refuse to start, and shall not warn, merely because polling is off: the
  window requires a push landing during the run, so warning on every manual run
  would be noise on the overwhelming majority of them.

## Out of scope

- **Recursive sync of arbitrary depth**, including files nested below a skill's
  `references/`. The traversal is the fixed Office layout, bounded by
  AC-OFFICE-CONFIG-SYNC-002.5.
- **Applying workspace settings from `kandev.yml`**, excluded by
  AC-OFFICE-CONFIG-SYNC-002.1b. The file stays in the repository layout and the
  raw-git surfaces keep reading and writing it; what is excluded is provider
  sync writing those governance values into the workspace. A follow-up would
  need to decide which fields are safe for a repository to set and whether an
  operator can pin one against it.
- **Pinning a commit for the duration of a run**, excluded by
  AC-OFFICE-CONFIG-SYNC-002.8. It would require a branch-to-SHA resolve neither
  provider client exposes.
- **What a run does with the fetched set.** See
  [Office Config Sync Reconciliation](config-sync-reconciliation.md).
