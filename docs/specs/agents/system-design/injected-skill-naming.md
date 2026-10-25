---
status: draft
system: agents
requirements:
  - REQ-AGENTS-INJECTED-SKILL-NAMING-001
  - REQ-AGENTS-INJECTED-SKILL-NAMING-002
---

# Injected Skill Naming System Design

## Purpose and boundaries

The agent runtime materializes skills into a session worktree so the agent's own
skill loader discovers them. That loader resolves a skill by directory name, so
the directory name is the skill's identity, not a storage detail. This design
makes one name authoritative for a skill across the database, the injected
directory, the injected frontmatter, and the workspace skill list.

This design owns the naming rule, the ownership rule, and the collision rule. The
one-time rename migration is a separate lifecycle and has its own document,
[Injected Skill Naming Migration](injected-skill-naming-migration.md); this
document covers only per-launch behavior. Neither owns the skill record schema,
the Office system-skill sync trigger, or the per-executor delivery strategies.

## Decision: the `kandev-` prefix is identity

A skill's slug carries the `kandev-` prefix, and the slug is used verbatim as the
directory name. Kandev normalizes the slug when a skill record is written rather
than deriving a second name at injection time.

The rejected alternative was to drop the prefix and track Kandev ownership with a
marker file or an injected-slug manifest. It produces a shorter invocable name
but is materially riskier here:

- A project skill directory can already contain repository skills. The Kandev
  repository itself commits 33 skill directories under `.agents/skills`,
  including common names such as `commit`, `debug`, `plan`, and `verify`.
- `.claude/skills` in that repository is a tracked symlink to `../.agents/skills`,
  and the Claude agent type sets `ProjectSkillDir` to `.claude/skills`. Injection
  for a Claude agent therefore resolves onto the tracked tree.
- Injection writes `SKILL.md` unconditionally. Without the prefix, a workspace
  skill named `commit` truncates a tracked repository file.

Keeping the prefix confines the namespace Kandev writes into to one it already
owns, so collision handling stays an edge case rather than the common path.

**The prefix is a namespace, not a proof of ownership.** Nothing prevents a
repository from committing a directory that begins with `kandev-`, so a
prefix-glob delete would destroy tracked content. Ownership is tracked separately,
under [Ownership and removal](#ownership-and-removal). This is the one place the
original design was wrong rather than merely incomplete: it treated the prefix as
sufficient for both jobs.

A separate finding decides how far the naming rule reaches. Surveyed harnesses
disagree about which field identifies a skill: Warp documents the frontmatter
`name` as the identifier, while Claude Code resolves by directory name. Kandev
injects through two delivery paths, so trusting either field alone is unsafe. The
rule constrains both: the directory name and the frontmatter `name` are made
equal, which is correct under either resolver. See `## Prior art` in the
requirements document.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-INJECTED-SKILL-NAMING-001` | [Naming rule](#naming-rule), [Injection](#injection) |
| `REQ-AGENTS-INJECTED-SKILL-NAMING-002` | [Ownership and removal](#ownership-and-removal), [Collision](#collision) |

## Components and responsibilities

- **Slug rule** (`internal/common/skillslug`, new) owns the well-formed pattern
  and the normalization function. It is a leaf package with no Kandev imports.
  It exists because the two callers sit on opposite sides of a layering
  boundary: `internal/office/skills` may not import `runtime/lifecycle`
  (`apps/backend/AGENTS.md:139`), and `AGENTS.md:219` requires cross-tier shared
  code to live in `internal/common/`. The two existing copies of `validSlugRe`
  (`skill/paths.go:12` and `executor_sprites_operations.go:27`) collapse onto
  this package.
- **Skill service** (`internal/office/skills/service.go`) owns slug generation
  and validation. It becomes the single point that normalizes a slug, so every
  write path (manual creation, package import, home-directory discovery) gets the
  rule without repeating it.
- **Runtime skill injector** (`internal/agent/runtime/lifecycle/skill/inject.go`)
  owns directory creation, the ownership marker, removal, and frontmatter
  reconciliation. It also owns the marker filename as an exported constant,
  because the Sprites path tests for the same name.
- **Skill delivery dispatcher**
  (`internal/agent/runtime/lifecycle/skill/delivery.go`) chooses the delivery path
  and, for Sprites, calls `normalizeManifestSkills`, which renders every skill
  through `renderSkillMarkdown` before the manifest is marshalled. This file is
  why frontmatter reconciliation reaches the remote path without the executor
  changing. It was absent from the previous revision of this section, which is how
  that revision reached a false conclusion about where Sprites renders.
- **Sprites executor upload path**
  (`internal/agent/runtime/lifecycle/executor_sprites_operations.go`) mirrors the
  injector for the remote sandbox. It already resolves the directory name through
  the shared helper and already receives rendered content, so it needs **no**
  rendering change. It does need the ownership rule: its removal step is still a
  prefix glob and must become the same two-condition test, and its
  empty-`ProjectSkillDir` default must go. It sits inside `runtime/lifecycle`, so
  importing `skill` crosses no boundary.
- **Bundled skill set** (`internal/office/configloader/skills/`) is the source of
  the system skills and must satisfy the naming rule at rest.

## Data and contracts

### Naming rule

Two predicates, deliberately distinct, because conflating them is what made the
previous revision self-contradictory:

- A slug is **well-formed** when it is non-empty and matches
  `^[a-zA-Z0-9_-]+$`. This is what `isValidSlug` already checks.
- A slug is **canonical** when it is well-formed and begins with `kandev-`.

Normalization maps a well-formed slug to a canonical one: a slug already
beginning with `kandev-` is returned unchanged; any other well-formed slug gains
the prefix. It is idempotent by construction.

**Order is fixed and load-bearing: validate, then normalize, then check
uniqueness.** Well-formedness is evaluated on the raw input, before normalization
can alter it. This matters at the empty-string boundary: normalizing first would
turn `""` into `"kandev-"`, which is well-formed and would silently become a real
directory. Validating first rejects it. Every caller applies the same order.

Normalization moves to slug write time in the skill service, so the persisted
slug is authoritative, instead of only at injection time, where it produced a
second name the user never sees.

That move is observable, and AC-001.11 and AC-001.12 exist to observe it. It is
not a refactor of where a function is called; it changes what the database
contains. `ValidateAndPrepareSkill` today generates a slug only when the caller
supplies none, and applies neither the well-formed check nor normalization to a
slug the caller does supply, so a caller can persist `my skill` or `protocol`
today and nothing downstream corrects the row. After this change a
not-well-formed slug is **rejected** at create and update with no row written and
a reported reason, and a well-formed non-canonical slug is **normalized before**
the uniqueness check, so `protocol` and `kandev-protocol` conflict at creation
rather than colliding later in a worktree. Without these two criteria the headline
architectural claim here would have no test that fails when it is unimplemented,
and AC-003.6's "re-running writes nothing" would be unfalsifiable for the
normalization phase.

`DirName` remains in the injector as a defensive normalizer so a legacy row that
escaped migration still lands in the Kandev-owned namespace rather than in the
repository's namespace. A well-formed but non-canonical slug is therefore
normalized and injected, not skipped; only a slug that is not well-formed is
skipped (AC-001.7). `DirName` delegates to `internal/common/skillslug` so there
is exactly one implementation of the rule.

### Injection

For each skill in the launch manifest, the injector writes
`<worktree>/<ProjectSkillDir>/<slug>/SKILL.md`.

**Frontmatter reconciliation.** `renderSkillMarkdown` currently preserves author
frontmatter verbatim and only synthesizes frontmatter when none is present. It
gains one responsibility: the `name` field is set to the skill's invocable name.
A skill whose frontmatter declares a different `name` has that field replaced; all
other fields, including `description` and the `kandev:` block, are preserved. This
keeps the guarantee ADR 0031 established, that a skill's description survives
injection, while removing the only field that can contradict the directory.

**The rewrite is line-level, not a YAML round-trip, and that choice is forced.**
AC-001.2 requires every other frontmatter field to be preserved *unchanged*.
Parsing the block and re-emitting it cannot satisfy that: it reorders keys, drops
comments and anchors, and rewrites quoting and block scalars. So the renderer does
not parse YAML. It operates on the block as text:

- The block is the region between a leading `---` line and the next line that is
  exactly `---`. Only that region is examined; the body after it is copied
  byte-for-byte. A trailing carriage return is ignored when testing a delimiter,
  so CRLF files are recognized, and existing line endings are preserved rather
  than normalized — rewriting them would change every line and break AC-001.2's
  "unchanged" for the whole block.
- The `name` key is the first line in the block matching `name:` at column zero.
  Its value is replaced. An indented `name:`, nested inside another mapping such
  as the `kandev:` block, is not the top-level key and is left alone.
- With no top-level `name` key, one is inserted as the block's first line.
- With more than one top-level `name` key, the first is rewritten, the rest are
  left, and the condition is logged. Duplicate keys are already invalid YAML; the
  renderer does not make that worse by guessing which one the author meant.
- If the existing value is not a plain scalar (a block scalar, flow sequence, or
  mapping), it is replaced by the plain scalar anyway and logged. `name` is
  Kandev-controlled, so no author form of it is authoritative.
- A block that opens with `---` but never closes is treated as having no
  frontmatter: a synthesized block is prepended and the original text preserved
  verbatim beneath it. Today `hasYAMLFrontmatter` tests only the opening
  delimiter, so this case currently returns unreconciled content.
- With no frontmatter at all, the synthesized block uses the invocable name. Today
  it uses the raw slug, which for a legacy non-canonical row is a different string
  from the directory name — exactly the mismatch this design removes.

Every line except the one `name:` line is byte-identical to the author's input,
which makes AC-001.2's second clause literally rather than approximately true.

**Both delivery paths already render through this one function, and no executor
change is needed to keep it that way.** The local injector calls
`renderSkillMarkdown` directly. Sprites reaches it one step earlier:
`deliverSprites` in `delivery.go` calls `normalizeManifestSkills`, which sets
`Content` to the rendered result for every skill *before* the manifest is
marshalled and handed to the executor. By the time `uploadSkillFiles` writes
`[]byte(sk.Content)`, that content is already the renderer's output. Upgrading
`renderSkillMarkdown` therefore propagates to both paths automatically.

This corrects the previous revision, which asserted that Sprites "writes
`[]byte(sk.Content)` directly and must instead write the rendered result". That
was false, and following it literally would not compile: `uploadSkillFiles` holds
an anonymous struct unmarshalled from the manifest JSON, not a `skill.Skill`, so
it cannot call the renderer at all. The correct placement is the one already in
force — reconciliation lives in the renderer, upstream of the manifest hand-off,
so any future delivery path inherits it by consuming a normalized manifest rather
than by remembering to call a function.

**Per-skill failure is contained.** Injection is best-effort per skill: a failure
to create a directory, write `SKILL.md`, or write a support file is logged and
the remaining skills continue. This is a behavior change. `injectSkills` today
returns an error on `MkdirAll`, `WriteFile`, or `writeSkillFiles` failure, which
aborts the whole pass and, upstream, the launch; AC-001.7 and AC-001.10 require
it to skip and continue instead.

**Partial writes are retained, not rolled back.** A directory whose `SKILL.md`
was written but whose support file failed is left in place: the skill is degraded
rather than absent, and rollback would risk deleting more than the pass created.
The directory still carries its ownership marker, so the next session removes it
normally. There is no retry within a pass.

### Ownership and removal

Ownership is proven per directory, not inferred from the name. This section is
what makes AC-002.1 and AC-002.3 true. **Removal requires two independent
conditions, and the absence of either vetoes it:**

1. The directory **carries the ownership marker** — Kandev created it.
2. The directory is **not a repository-tracked path** — the repository has not
   since adopted it.

Every directory the injector creates gets an **ownership marker**: an empty
`.kandev-injected` file inside the directory. A directory carries the marker only
when that name resolves *without following symlinks* to a regular file. The check
is an `Lstat`, not a `Stat`, so a symlink named `.kandev-injected` pointing at a
regular file elsewhere does not qualify — otherwise a planted symlink would be
enough to nominate any directory for deletion. A directory, or an entry whose type
cannot be determined, likewise does not qualify. The content is never read.

**Why the marker alone is insufficient, and why authenticating it would not
help.** The marker is presence-based, so the obvious objection is that anything
can create a `.kandev-injected` file. The obvious answer — give it content only
Kandev can produce, a per-install secret or per-run token, and verify before
removing — was considered and **rejected, because it does not address the failure
it appears to address.** The realistic way a marked directory becomes repository
content is not forgery. It is that Kandev wrote the directory legitimately, with a
genuine marker, and the user then committed it — plausible here because injected
directories currently show as untracked in this repository's own symlink layout,
so a routine `git add -A` sweeps one in. In that scenario the marker found next
session is authentic under any signing scheme, written by this very install, and
every content check passes. Kandev would delete tracked content while holding
perfect proof it was entitled to.

The evidence is not the problem; the inference is. "I created this" was true when
written and has since stopped implying "this is still mine to delete". No artifact
Kandev writes inside the directory can detect that change, because the change
happened outside the directory. The only component that knows whether the
repository has adopted a path is the repository's own index.

**The tracked-path veto.** Before removing anything, the pass asks the repository
which paths under the project skill directory it tracks. This is one query per
pass, not per directory, scoped to that directory and resolved the same way the
version-control exclusion pattern already resolves it, so a symlinked skill
directory is asked about under the path the repository actually records. Three
outcomes:

- **Not a version-controlled repository.** Nothing can be tracked, the veto is
  vacuous, and removal proceeds on the marker alone.
- **The query succeeds.** A marked directory whose path is reported is left
  entirely alone: not removed, not written into, path logged, and any skill whose
  invocable name equals it is skipped (AC-002.8). A marked directory not reported
  is removed.
- **The query fails otherwise.** Kandev cannot tell tracked content from its own,
  so it removes nothing in that pass, logs the reason, and continues into
  injection, where every occupied name is skipped (AC-002.9). Failing closed costs
  a session of stale skills; failing open costs the user's committed work.

This is the one place the design accepts a dependency on version control inside
the launch path. It is justified narrowly: read-only, consulted once, failure
contained to "remove nothing", and the code path already invokes version control
to maintain the exclusion pattern, so no new capability is introduced.

The marker is a file inside the skill directory rather than a manifest elsewhere
in the worktree, and that placement is deliberate. A shared manifest is a single
mutable object that two concurrent launches overwrite, and whichever write lands
second erases the other's names, leaking directories no later pass can attribute.
A per-directory marker has no shared state: each directory answers for itself, so
concurrency degrades to the interleaving already conceded in AC-002.4 rather than
to permanent leakage. It also removes a file format, a parser, and a parse-failure
branch. The marker is a dotfile, so it is not skill content to the agent, and it
sits beside `SKILL.md` under the layout ADR 0031 defines.

**When ownership is decided, and why there is no check-then-write window.**
Ownership is evaluated at exactly two moments. At removal, by the two conditions
above. At creation, **implicitly, by creating the directory exclusively**: the
injector creates the project skill directory with `MkdirAll` once, before the loop
and only when at least one skill will be written, then creates each leaf directory
with a plain `Mkdir`. Creating the parent is conditional so an empty manifest
never brings a project skill directory into existence the repository did not have;
a removal pass over a directory that does not exist finds nothing and is not an
error. A plain `Mkdir` fails if the name exists, so success *is* proof that
nothing else held the name, established atomically by the filesystem rather than
inferred from a stat taken a moment earlier.

The alternative — stat the name, decide it is free, then `MkdirAll` — has a window
in which another writer can create the directory, and `MkdirAll` on an existing
directory succeeds silently, so Kandev would write into something it did not
create. AC-002.4 does not cover this: it is scoped to two Kandev sessions, and the
other writer need not be Kandev. Exclusive creation closes the window without a
lock and without widening AC-002.4. A leaf that already exists is a name Kandev
may not claim this pass, whatever the reason, so the skill is skipped and logged.

Ordering and failure semantics:

- **The marker is written first**, immediately after the directory is created and
  before `SKILL.md` or any support file. This is load-bearing: a crash between
  directory creation and marker write would otherwise leave an unmarked directory
  that no later pass may remove and none may write into, permanently wedging that
  skill name in that worktree.
- **The marker cannot be written:** the injector removes the directory it created
  moments earlier in this pass, skips the skill, and logs (AC-002.6). Removing it
  is safe precisely because this pass created it; leaving it would wedge the name.
- **Writing a marker into a directory that already has one** is a no-op, and
  removing a marked directory that has already disappeared is not an error, so a
  retried or duplicated pass converges.
- **A directory Kandev was entitled to remove but could not** is logged and the
  pass continues. The skill whose name it holds is then skipped, because exclusive
  creation fails on the surviving directory (AC-002.7). The previous revision let
  injection write into it and interleave the earlier session's support files; that
  permission is withdrawn. It bought nothing — a directory holding two skills'
  files is worse than a skipped skill with a logged path — and it was the only
  clause letting Kandev write into a directory this pass did not create.
- **A directory that is left alone** is left alone completely: not removed, not
  written into, path logged, colliding skill skipped. Three populations land here,
  and the mechanism deliberately does not distinguish them, because the correct
  action for all three is identical: unmarked directories the repository committed
  (AC-002.5), which is the defect this design closes; unmarked directories left by
  a Kandev version predating the marker (AC-002.5); and marked directories the
  repository has since adopted (AC-002.8).

The second population is the upgrade cost, accepted deliberately. Those
directories are not removed, so a skill they shadow is unreachable in that
worktree until an operator deletes the directory or the worktree is recreated.
Deleting unmarked directories that merely match the prefix on a first pass is the
exact behavior this design removes, and it cannot be distinguished from the
repository-committed case: both are unmarked directories named `kandev-*`. The
asymmetry decides it. A wrong deletion loses tracked content and is unrecoverable;
a shadowed skill is recoverable and logged with the path to delete. Task worktrees
are per-task and short-lived, so the affected population is long-lived reused
worktrees only.

**Removal runs whenever a project skill directory is declared, including when the
manifest is empty.** Removal and writing are independent: removal is a function of
what is on disk, not of what the manifest contains. A profile whose skills have
all been unassigned must not keep serving the previous session's skills, which is
what skipping removal on an empty manifest would do, and the difference is
user-visible rather than a corner case. AC-001.8 therefore says "writes no skill
directory", not "does nothing". This preserves today's behavior, where
`injectSkills` cleans unconditionally before its loop.

Neither pass runs for an agent type that declares **no** project skill directory:
there is nothing to scan and nothing to write into, so both paths do nothing
(AC-001.9), and neither substitutes a default. This changes the Sprites path,
which today replaces an empty `ProjectSkillDir` with `.agents/skills` and then
removes and uploads into it — the directory this repository commits its own 33
skills into — while the local path already declines to inject. The two paths
disagreed, and the safe reading wins: a missing directory is missing, not an
invitation to guess. In practice the branch is defensive on both paths, because
`resolveProjectSkillDir` falls back to `DefaultProjectSkillDir` and every agent
type declares a directory, so no production manifest reaches it. It is specified
anyway, because the default that made it unsafe was itself defensive code nobody
had reason to look at.

**The remote path runs the same rule, not a name pattern.** Sprites executes
removal as a script inside the sandbox, which holds a clone of the user's
repository, so REQ-002 governs it identically. Today that script is
`rm -rf /workspace/<projectSkillDir>/kandev-*` — precisely the prefix-glob delete
this design removed locally, surviving on the other delivery path. It is replaced
by the same two-condition test per entry: enumerate the direct children of the
project skill directory, and remove a child only when it carries a regular-file
`.kandev-injected` and the repository does not track it. The three version-control
outcomes above apply unchanged, including removing nothing when the tracked set
cannot be determined. Creation is likewise exclusive — the leaf is created without
"make parents, succeed if present" — so the remote path cannot write into a
directory it did not create.

Remote failure semantics mirror the local ones per skill. The marker is uploaded
first; a failed marker upload removes the directory just created and skips the
skill (AC-002.6). A failed `SKILL.md` upload skips that skill's remaining support
files and leaves the directory and marker in place so the next pass can remove it,
then continues with the next skill (AC-001.10). `writeFileWithRetry` is compatible
with "no retry within a pass": its retries are transport retries of a single write,
invisible above the write, whereas the prohibition is on re-attempting a skill or
a pass after a write has been reported as failed.

### Collision

Two collision classes exist, and they need different rules.

**Manifest-internal.** Normalization is not injective: `protocol` and
`kandev-protocol` resolve to the same directory. Normalizing at slug write time
makes the uniqueness check catch this at creation, but rows created before the
migration, and rows in a workspace whose sync has not yet run, can still collide
at injection time. The injector tracks the directory names it has claimed during
the current pass. The skill earliest in manifest order claims the directory; a
later skill resolving to the same name is skipped and logged with its own slug,
the owning slug, and the directory. Manifest order is the launch manifest's
existing order and is the tiebreak; injection introduces no reordering of its own.
Skipping rather than overwriting matters because support files are written into
the same directory, so an overwrite would silently interleave two skills' files
rather than replacing one cleanly.

**Pre-existing directory.** A directory that survives removal is one Kandev may
not claim, for one of four reasons: it carries no ownership marker, so Kandev did
not create it; it carries the marker but the repository now tracks it; its removal
was attempted and failed; or removal was skipped entirely because the tracked set
could not be determined. Injection does not write into any of them. The skill is
skipped and logged at warning level with the slug and the resolved path (AC-002.2,
AC-002.5, AC-002.7, AC-002.8, AC-002.9). The injector does not need to tell these
apart to behave correctly, because exclusive creation fails on all four
identically; it distinguishes them only to log a useful reason.

A skipped skill in either class never fails the launch, consistent with the
existing contract that a missing skill must not abort a session.

**Concurrency.** Injection is a remove-then-write sequence and takes no lock, so
two sessions sharing a worktree can interleave. The guarantee is deliberately
narrow: neither launch fails, and each session writes what it can. The resulting
directory set may be the union of both manifests, and a single directory may hold
files from both. AC-002.4 states exactly this and no more.

The previous revision claimed the directory would hold "exactly the skill set of
the session that injected last", which no unlocked remove-then-write sequence can
deliver: `A removes, B removes, B writes, A writes` leaves both sets. That claim
was withdrawn rather than mechanized, because the three mechanisms that would make
it true (a session-scoped directory, a lock held across the whole sequence, or
publication by atomic rename) each change either the path the harness scans or the
launch path's synchronization model. The requirement records the withdrawal and
its reasoning under `## Out of scope`.

## Control flow

1. A user creates or imports a skill. The skill service validates the raw slug is
   well-formed, normalizes it, then checks uniqueness on the normalized value, so
   two inputs that normalize to the same slug conflict at creation. Separately,
   the sync pass reconciles bundled and user slugs; see the
   [migration design](injected-skill-naming-migration.md).
2. A session launches. The manifest builder resolves the profile's skill keys to
   skill records. If the agent type declares no project skill directory, the
   delivery path stops here. Otherwise the injector asks the repository which paths
   under that directory it tracks, removes every directory that both carries the
   ownership marker and is untracked — whether or not the manifest contains any
   skills — then creates each skill's directory exclusively and writes its
   `SKILL.md` with reconciled frontmatter and its support files, skipping any name
   it could not claim.
3. The agent's loader scans the project skill directory and registers each skill
   under its directory name, which now equals the slug shown in the workspace
   skill list.

## Failure and recovery

Injection is best-effort per skill and must never abort a launch. A skipped
collision, an unwritable directory, a failed `SKILL.md` write, or a failed
support-file write is logged and the remaining skills continue. Partial
directories are retained and cleaned up by the next session's removal pass, with
one bounded exception: a partial directory that has since been committed is
tracked, so the veto keeps it permanently. That is the intended trade. It is
logged on every pass with its path, and the alternative is deleting content the
repository now owns.

## Persistence

The skill record's slug column is authoritative. Injected directories are
ephemeral: they are rewritten at the start of every session and are not a
persistence tier. Ownership markers are likewise ephemeral per worktree, living
and dying with the directories they mark.

## Security

The well-formed pattern is unchanged and continues to reject path separators and
traversal sequences, so a slug cannot escape the project skill directory.
Validating before normalizing means a rejected slug is never prefixed into a
directory name. Normalization only prepends a constant, so it cannot introduce a
traversal that validation would otherwise have rejected. Support-file paths keep
their existing containment checks.

Removal is bounded by two independent conditions rather than by a name pattern:
evidence Kandev itself wrote, and the repository's own statement that it has not
adopted the path. The second is what makes the bound trustworthy. The first alone
is evidence of origin, not of continued ownership, and it stays true after a
directory is committed — so a design resting on it would delete tracked content
while holding authentic proof of its own authorship. This is why the marker is not
cryptographically authenticated: strengthening evidence of authorship does not
strengthen a claim that authorship was never the question.

Removal still enumerates only direct children of the project skill directory, and
each candidate name must be a single well-formed path component. The marker test
does not follow symlinks, so a symlinked `.kandev-injected` cannot nominate a
directory for deletion, and the tracked-path query is read-only.

## Observability

Injection logs a warning per skipped skill with the slug and resolved path, and
one warning per directory it leaves in place, naming the resolved path and which
of the four reasons applied, so an operator can tell an upgrade leftover from a
committed directory from a failed removal. When the tracked set cannot be
determined, that is logged once for the pass rather than once per directory, since
the cause is shared.

## Related decisions

- [ADR 0031: Office Skill Reference Files](../../../decisions/0031-office-skill-reference-files.md)
- [Injected Skill Naming Migration](injected-skill-naming-migration.md), which
  covers `REQ-AGENTS-INJECTED-SKILL-NAMING-003`.
