---
status: draft
system: agents
created: 2026-08-31
owners:
  - kandev
---
# Injected Skill Naming Requirements

## Overview

Kandev materializes the skills assigned to an agent profile into the session
worktree before every launch. The agent's skill loader resolves a skill by its
on-disk directory name, so that name is the identifier a user or agent must type
to invoke the skill. Kandev derives it by prefixing the skill slug, so the
invocable name and the name Kandev shows can differ. When they differ the skill
is materialized correctly and remains unreachable.

The agent system owns this contract because injected skills are an agent-facing
runtime capability shared by kanban and Office launches. Office consumes the
contract and does not own it.

## Terminology

- **Skill slug:** The workspace-unique identifier persisted on a skill record.
- **Well-formed slug:** A slug that is non-empty and matches
  `^[a-zA-Z0-9_-]+$`. This is the charset predicate only; it says nothing about
  the prefix.
- **Canonical slug:** A well-formed slug that also begins with `kandev-`. Every
  canonical slug is well-formed; the reverse does not hold.
- **Injected skill:** A skill Kandev materializes into a session worktree for
  that session's duration.
- **Project skill directory:** The CWD-relative directory an agent type scans for
  skills, such as `.claude/skills` or `.agents/skills`.
- **Repository skill:** A skill directory committed to the user's repository and
  not managed by Kandev.
- **Invocable name:** The name an agent must use to invoke a skill. It is the
  skill's directory name within the project skill directory.
- **Delivery path:** A code path that writes an injected skill's `SKILL.md` for a
  launch. There are exactly two: the local worktree injector, writing to the
  filesystem, and the Sprites upload path, writing into a remote sandbox. An
  executor that writes no injected `SKILL.md` is not a delivery path and is
  outside every claim here about "every delivery path".
- **Ownership marker:** A regular file named `.kandev-injected` that Kandev
  writes inside every skill directory it creates, identifying that directory as
  Kandev-managed. A directory *carries* the marker only when that name resolves,
  without following symlinks, to a regular file. A symlink, a directory, or an
  entry whose type cannot be determined does not count as carrying it. The
  marker's content is not read and carries no meaning.
- **Repository-tracked path:** A path the repository's version-control index
  records. Kandev uses this only as a veto: it never causes a removal, it only
  prevents one. A worktree that is not a version-controlled repository tracks
  nothing.

## Prior art

*No prior Kandev reasoning was available to cite: this host has no configured
Obsidian vault, so that leg could not run.*

### What other products shipped

**Warp (Oz)** solves the identical problem and is the closest analogue. For
third-party harnesses such as Claude Code and Codex, a Warp cloud run publishes
each externally supplied skill "into that harness's own skill root before launch,
so the harness discovers them through its native skill system" — the same
publish-then-let-the-harness-discover shape Kandev uses. Two things differ:

- **Warp does not rename to avoid collisions.** It keeps repository skills and
  externally supplied skills in separate roots under the same name, resolving by
  precedence at resolution time: home/global first, then directories closer to
  the repository root. For an explicit invocation it shows every match and the
  user chooses by description.
- **Warp treats `.claude/skills/`, `.codex/skills/`, `.agents/skills/` and seven
  more as project skill roots** and scans all of them, so one repository can
  serve several harnesses at once.

**OpenHands** exposes `load_skills_from_dir()` over a directory whose direct
children are skill folders, and documents "user skills not loading" as a known
container-packaging failure. It publishes no naming or conflict rule.

### What we are doing differently, and why

Kandev keeps the `kandev-` prefix and reconciles the declared name to it rather
than adopting Warp's scope separation. Warp's model is the better user experience
and was considered seriously, but it does not port:

- Warp separates roots because **it owns the resolver**. Kandev does not: it
  writes into a root a third-party harness scans, with no channel to say "prefer
  this one". Precedence is not available to us.
- Kandev injects into the **project** skill directory, per worktree, because a
  session's skill set is chosen per agent profile and sessions run concurrently.
  Warp's home-directory placement is machine-global and would leak one session's
  skills into another.
- Writing into the same root the repository commits into leaves **name** as the
  only channel for partitioning the namespace, which is what makes the prefix
  load-bearing rather than decorative.

One piece of that evidence changes this design rather than being rejected: Warp
documents the frontmatter `name` as the identifier, while Claude Code resolves by
directory name. Harnesses genuinely disagree about which field is authoritative,
and Kandev injects into several of them, so trusting one field is not safe. Making
the two equal is the only rule correct under both resolvers, which is why AC-001.2
constrains frontmatter as well as the directory.

The prefix partitions the namespace but does not establish **ownership**. A
repository may commit a directory beginning with `kandev-`, so "the name starts
with our prefix" cannot be the test for "we created this". Ownership is recorded
explicitly instead, and the prefix is retained only as the namespace convention
that keeps collisions rare.

## Requirements

### REQ-AGENTS-INJECTED-SKILL-NAMING-001: Injected skills are reachable by the name Kandev shows

**Intent:** A skill a user assigns to an agent profile is usable by that agent.
A skill that is materialized but unreachable is indistinguishable from a skill
that was never assigned, and the failure is silent at every layer.

**User story:** As a workspace owner, I want an agent to invoke an assigned skill
by the name Kandev shows for it, so assigning a skill is sufficient to make the
agent able to use it.

#### Acceptance criteria

- **AC-AGENTS-INJECTED-SKILL-NAMING-001.1:** **GIVEN** any skill assigned to an
  agent profile, **WHEN** Kandev injects it into a session worktree, **THEN**
  the skill's invocable name equals the name Kandev shows for that skill in the
  workspace skill list.
- **AC-AGENTS-INJECTED-SKILL-NAMING-001.2:** **GIVEN** an injected skill,
  **WHEN** any delivery path writes its `SKILL.md`, **THEN** the frontmatter
  `name` field equals the skill's invocable name, and every other frontmatter
  field the author supplied is preserved unchanged.
- **AC-AGENTS-INJECTED-SKILL-NAMING-001.3:** **GIVEN** one skill and the two
  delivery paths named in Terminology, **WHEN** each path injects that skill,
  **THEN** both produce the same directory name and the same frontmatter `name`
  value.
- **AC-AGENTS-INJECTED-SKILL-NAMING-001.4:** **GIVEN** a skill whose slug is
  already canonical, **WHEN** Kandev injects it, **THEN** the invocable name
  equals the slug and Kandev does not derive a second name.
- **AC-AGENTS-INJECTED-SKILL-NAMING-001.5:** **GIVEN** two skills in one launch
  manifest whose invocable names are equal, **WHEN** Kandev injects them,
  **THEN** exactly one directory is written for that name, the skill appearing
  earliest in manifest order owns it, and Kandev records the skipped skill and
  the skill it collided with in the session log.
- **AC-AGENTS-INJECTED-SKILL-NAMING-001.6:** **GIVEN** a well-formed slug,
  **WHEN** Kandev normalizes it, **THEN** normalizing the result again returns
  it unchanged, and a slug that is already canonical is returned byte-identical.
- **AC-AGENTS-INJECTED-SKILL-NAMING-001.7:** **GIVEN** a launch manifest
  containing a skill whose slug is not well-formed, **WHEN** Kandev injects the
  manifest, **THEN** Kandev skips that skill before normalizing it, records it
  in the session log, injects every remaining skill, and does not fail the
  launch.
- **AC-AGENTS-INJECTED-SKILL-NAMING-001.8:** **GIVEN** a launch manifest with no
  skills and an agent type that declares a project skill directory, **WHEN**
  Kandev prepares the session, **THEN** it writes no skill directory, still
  performs the removal pass of AC-002.3 so the previous session's skills do not
  survive into this one, leaves every directory that pass may not remove
  untouched, and does not fail the launch.
- **AC-AGENTS-INJECTED-SKILL-NAMING-001.9:** **GIVEN** an agent type that
  declares no project skill directory, **WHEN** Kandev prepares the session on
  either delivery path, **THEN** that path writes nothing, removes nothing,
  substitutes no default, and does not fail the launch.
- **AC-AGENTS-INJECTED-SKILL-NAMING-001.10:** **GIVEN** a skill whose `SKILL.md`
  or support file cannot be written after its directory and ownership marker
  were created, **WHEN** Kandev injects the manifest, **THEN** Kandev logs the
  failure, leaves whatever was already written for that skill in place with its
  marker intact so a later pass can remove it, continues with every remaining
  skill, and does not fail the launch.
- **AC-AGENTS-INJECTED-SKILL-NAMING-001.11:** **GIVEN** a request to create or
  update a skill whose supplied slug is not well-formed, **WHEN** Kandev
  validates the request, **THEN** Kandev rejects it, creates or modifies no
  skill row, and reports the reason. Kandev does not coerce the slug into a
  well-formed one.
- **AC-AGENTS-INJECTED-SKILL-NAMING-001.12:** **GIVEN** a request to create or
  update a skill whose supplied slug is well-formed but not canonical, **WHEN**
  Kandev persists it, **THEN** the persisted slug is canonical, the uniqueness
  check is evaluated on that canonical value, and a request whose canonical slug
  already exists in the workspace is rejected without modifying either row.

### REQ-AGENTS-INJECTED-SKILL-NAMING-002: Injection never modifies repository skills

**Intent:** A repository can commit its own skills into the same project skill
directory Kandev injects into, including directories that begin with `kandev-`.
Kandev deletes directories at the start of every session, so it must decide what
to delete from evidence that it created them, never from the directory's name.
Evidence that Kandev created a directory is necessary but not sufficient: one it
legitimately created can afterwards be committed, at which point the repository
owns it and Kandev's evidence is genuine but no longer a licence to delete. Deleting tracked content is unrecoverable, so this requirement is
absolute and admits no exception.

**User story:** As a developer whose repository commits its own skills, I want
injection to leave them untouched, so assigning a workspace skill cannot damage
tracked repository content.

#### Acceptance criteria

- **AC-AGENTS-INJECTED-SKILL-NAMING-002.1:** **GIVEN** a project skill directory
  that contains repository skills, including any whose name begins with
  `kandev-`, **WHEN** Kandev injects skills, **THEN** Kandev does not create,
  modify, or delete any repository skill directory.
- **AC-AGENTS-INJECTED-SKILL-NAMING-002.2:** **GIVEN** a workspace skill whose
  invocable name would collide with an existing directory that does not carry
  the ownership marker, **WHEN** Kandev injects skills, **THEN** Kandev does not
  write to that directory and records the skipped skill in the session log.
- **AC-AGENTS-INJECTED-SKILL-NAMING-002.3:** **GIVEN** skill directories present
  in the project skill directory, **WHEN** Kandev starts a new session in that
  worktree, **THEN** Kandev removes exactly those directories that both carry the
  ownership marker and are not repository-tracked paths, and no other directory.
- **AC-AGENTS-INJECTED-SKILL-NAMING-002.4:** **GIVEN** two sessions that share
  one worktree and inject skill sets concurrently, **WHEN** both injections have
  completed, **THEN** neither launch fails and each session has written every
  skill it was able to write. Kandev makes no guarantee about which session's
  directories remain; the set may be the union of both.
- **AC-AGENTS-INJECTED-SKILL-NAMING-002.5:** **GIVEN** a directory whose name
  begins with `kandev-` but which does not carry the ownership marker, whether
  committed by the repository or left by a Kandev version that predates the
  marker, **WHEN** Kandev injects skills, **THEN** Kandev neither removes it nor
  writes into it, skips any skill whose invocable name equals it, logs the
  resolved path, and does not fail the launch.
- **AC-AGENTS-INJECTED-SKILL-NAMING-002.6:** **GIVEN** a skill directory Kandev
  created during this pass whose ownership marker cannot be written, **WHEN**
  Kandev injects skills, **THEN** Kandev removes the directory it just created,
  skips that skill, logs the failure, and does not fail the launch.
- **AC-AGENTS-INJECTED-SKILL-NAMING-002.7:** **GIVEN** a directory that Kandev
  is permitted to remove but whose removal fails, **WHEN** Kandev starts a
  session, **THEN** Kandev logs the failure, continues removing the remaining
  removable directories, skips any skill whose invocable name equals that
  directory rather than writing into it, and does not fail the launch.
- **AC-AGENTS-INJECTED-SKILL-NAMING-002.8:** **GIVEN** a directory that carries
  the ownership marker and is also a repository-tracked path, **WHEN** Kandev
  starts a session, **THEN** Kandev neither removes it nor writes into it, skips
  any skill whose invocable name equals it, logs the path, and does not fail the
  launch.
- **AC-AGENTS-INJECTED-SKILL-NAMING-002.9:** **GIVEN** a worktree whose tracked
  paths cannot be determined, **WHEN** Kandev starts a session, **THEN** Kandev
  removes no directory in that pass, logs the reason, injects every skill whose
  invocable name is free, skips the rest, and does not fail the launch.

### REQ-AGENTS-INJECTED-SKILL-NAMING-003: Renaming a bundled skill preserves existing assignments

**Intent:** Changing a bundled skill's identifier must not silently unassign it
from agents already using it, and must not leave duplicate rows in the workspace
skill list after an upgrade.

#### Acceptance criteria

- **AC-AGENTS-INJECTED-SKILL-NAMING-003.1:** **GIVEN** a workspace whose agents
  reference a bundled skill by its previous slug, **WHEN** Kandev upgrades and
  syncs bundled skills, **THEN** each affected agent references the renamed
  skill and no agent loses the capability.
- **AC-AGENTS-INJECTED-SKILL-NAMING-003.2:** **GIVEN** the same upgrade,
  **WHEN** the sync completes, **THEN** the workspace skill list contains
  exactly one row for the renamed skill and no row for the previous slug.
- **AC-AGENTS-INJECTED-SKILL-NAMING-003.3:** **GIVEN** a workspace that already
  contains a user skill whose slug equals a bundled slug, **WHEN** the sync
  runs, **THEN** the sync detects the conflict before attempting to write,
  leaves the user skill unchanged, creates no row for that bundled slug, reports
  the conflict, and reconciles every other bundled skill in that workspace.
- **AC-AGENTS-INJECTED-SKILL-NAMING-003.4:** **GIVEN** a bundled rename whose
  new slug is not yet present in a workspace, **WHEN** the sync runs, **THEN**
  the row for the new slug exists before the retired row is reconciled, so every
  rewritten agent reference points at a row that resolves.
- **AC-AGENTS-INJECTED-SKILL-NAMING-003.5:** **GIVEN** a completed sync pass,
  **WHEN** it reports the slugs it inserted, updated, removed, normalized, or
  found in conflict, **THEN** each per-workspace list is ordered lexicographically
  by slug, and any list aggregated across workspaces is ordered by workspace
  identifier and then by slug, so two passes over identical data produce
  identical output regardless of the order the caller supplied the workspaces
  in.
- **AC-AGENTS-INJECTED-SKILL-NAMING-003.6:** **GIVEN** a sync pass that has
  already run to completion, **WHEN** it runs again against unchanged data,
  **THEN** it performs no write and reports no insert, update, removal, or
  normalization.
- **AC-AGENTS-INJECTED-SKILL-NAMING-003.7:** **GIVEN** two sync passes requested
  concurrently for one workspace, **WHEN** both run, **THEN** they are
  serialized so that one completes before the other begins, and the second
  observes the first's writes and reports no duplicate insert or normalization.
- **AC-AGENTS-INJECTED-SKILL-NAMING-003.8:** **GIVEN** an agent whose skill
  reference list contains a slug being renamed, **WHEN** the sync rewrites that
  list, **THEN** the renamed slug replaces every occurrence of the old slug, the
  list's existing order is preserved, empty values are dropped, and a slug
  appears at most once.
- **AC-AGENTS-INJECTED-SKILL-NAMING-003.9:** **GIVEN** a non-system skill row
  whose slug is not well-formed, **WHEN** the sync pass runs, **THEN** it leaves
  that row's slug unchanged, rewrites no agent reference on its behalf, reports
  it, and reconciles every other row in that workspace.

## Out of scope

- The mechanism that hides injected skill directories from version control.
  That behavior is a separate defect tracked outside this requirement. It is
  related to AC-002.8 but not a substitute for it: while injected directories stay
  visible to version control, committing one is the realistic way a Kandev-created
  directory becomes tracked, and AC-002.8 is what makes that survivable. Fixing
  the exclusion mechanism reduces how often this arises; it does not remove the
  need for the veto, since a user may commit one deliberately.
- Skill authoring, approval state, and the workspace skill management UI.
- Skill content, reference-file layout, and progressive disclosure, owned by
  [ADR 0031](../../../decisions/0031-office-skill-reference-files.md).
- Instruction files such as `AGENTS.md`, which use a different delivery path.
- Home-directory skill discovery, which does not write into a worktree.
- Per-session isolation of injected skill directories, and any guarantee about
  the contents of a worktree two sessions inject into at the same time. Kandev
  permits a shared worktree for an additional session on a task and for a subtask
  that inherits its parent's workspace. Injection is a remove-then-write sequence
  with no lock, so two concurrent injections can interleave and leave the union of
  both skill sets, or a directory holding files from both. AC-002.4 requires only
  that neither launch fails. Making the result correspond to exactly one session's
  manifest requires a session-scoped directory, a lock held across the sequence,
  or publication by atomic rename — each changing the path the harness scans or
  the launch path's synchronization, and each a separate contract. This
  requirement neither introduces the behavior nor fixes it.
- Automatic cleanup of skill directories left by a Kandev version that predates
  the ownership marker. Those directories carry no marker, so AC-002.5 leaves
  them in place and skips any skill whose name they shadow, so a skill can be
  unreachable in a worktree injected into before the upgrade until an operator
  deletes the directory or the worktree is recreated. This is deliberate: the
  alternative, deleting unmarked directories that merely match the prefix, is the
  exact defect this requirement closes, and a wrong deletion (unrecoverable loss
  of tracked content) is not comparable to a shadowed skill (recoverable, and
  logged with the exact path). Task worktrees are short-lived, so the affected
  population is long-lived reused worktrees only.
- The behavior of the sync when handed an empty but non-`nil` bundled skill set,
  which today reconciles that workspace against an empty set rather than treating
  it as "nothing supplied". No acceptance criterion here reaches that input and
  this contract neither introduces nor relies on the behavior. Recorded so it is
  not lost; it belongs to its own card.
- Recovering a workspace in which a user skill permanently occupies a bundled
  slug. AC-003.3 requires the conflict to be reported every pass and the bundled
  skill withheld; resolving it means renaming one of the two rows, an operator
  action with its own UI affordance.
