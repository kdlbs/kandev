---
status: draft
system: agents
requirements:
  - REQ-AGENTS-INJECTED-SKILL-NAMING-003
---

# Injected Skill Naming Migration System Design

## Purpose and boundaries

Adopting the naming rule renames one bundled skill and normalizes existing user
slugs. That is a one-time, upgrade-time reconciliation over stored rows, with a
different lifecycle from the per-launch injection behavior in
[Injected Skill Naming](injected-skill-naming.md), so it is specified separately.
Read that document for the naming rule itself, which this one applies.

This design owns the rename migration and the sync pass's reporting contract. It
uses but does not own the skill record schema or the naming rule.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-INJECTED-SKILL-NAMING-003` | [Rename migration](#rename-migration), [Observability](#observability) |

## Components and responsibilities

- **System skill sync** (`internal/office/skills/system_sync.go`) reconciles the
  bundled set against each workspace and owns the rename migration. Its existing
  retired-slug replacement machinery is reused rather than extended.
- **Skill service** (`internal/office/skills/service.go`) owns slug validation and
  normalization at write time. The sync applies the same rule to rows that predate
  it.
- **Bundled skill set** (`internal/office/configloader/skills/`) is the source of
  the system skills and must satisfy the naming rule at rest.
- **Slug rule** (`internal/common/skillslug`, new) provides the well-formed
  predicate and normalization both callers share.

## Data and contracts

## Rename migration

Two populations need migrating, and they use different mechanisms.

**Bundled skills.** The bundled `memory` skill is renamed at source to
`kandev-memory`, in both its directory and its frontmatter `name`. All other
bundled skills already satisfy the rule. The sync's existing
`retiredDefaultSkillReplacements` map gains `"memory": "kandev-memory"`. On the
first sync after upgrade, `replaceSkillOnAgents` rewrites both `skill_ids` by row
ID and `desired_skills` by slug on every agent in the workspace, then the retired
row is deleted. This is the same path already used for eight earlier renames, so
no new migration mechanism is introduced.

**Conflict with a user row.** `syncWorkspace` builds its by-slug map from
`ListSystemSkills`, which returns system rows only, so it cannot today see a user
row holding a bundled slug. `office_skills` carries `UNIQUE(workspace_id, slug)`,
so creating over one fails, and `syncWorkspace` propagates that error, aborting
reconciliation for every remaining bundled skill in the workspace. The insert
phase therefore consults the full workspace skill set, using the same non-system
listing added for user-skill normalization below, and **checks before writing**: a
bundled slug already held by a non-system row is skipped, recorded in the conflict
list, and reconciliation continues with the next bundled slug. The bundled skill
is withheld from that workspace until an operator renames one of the two rows, and
the conflict is reported on every pass.

The pre-check is not itself atomic: a user row can be created between the check
and the insert, because skill creation through the service does not take the sync
lock. `UNIQUE(workspace_id, slug)` is therefore the authority and the pre-check
only improves reporting. A unique-violation error on a bundled insert is
classified as the same conflict and handled identically — skipped, recorded,
reconciliation continues — so the race degrades to the case the pre-check already
covers rather than aborting the workspace. Error classes other than this conflict
keep their current propagate-and-abort behavior.

**Ordering.** The bundled reconciliation runs in two phases and the order is
load-bearing. The insert/update phase walks bundled slugs and records each created
row in the in-memory by-slug map; the retirement phase then resolves a retired
slug's replacement out of that same map. `kandev-memory` is therefore present
before `memory` is reconciled, so agent references are rewritten to a row that
exists. Reversing the phases would rewrite references to a missing row. The
bundled phase already walks slugs in sorted order; the retirement phase iterates a
map, so it collects its results and sorts them before reporting.

**User skills.** Existing user rows with un-prefixed slugs are normalized during
the same sync pass, after the bundled reconciliation completes. This is the one
place new machinery is required: `SystemSyncRepo` today exposes `ListSystemSkills`
and a by-slug point lookup, neither of which can enumerate non-system rows. The
interface gains a workspace-scoped listing of non-system skills. The rename itself
reuses the existing helpers. For each non-system row whose slug is well-formed and
not canonical, the sync computes the normalized slug and:

- if no other row in the workspace holds the normalized slug, updates the row's
  slug in place, preserving the row ID, and rewrites `desired_skills` references
  on every agent from the old slug to the new one;
- if another row already holds it, leaves both rows untouched and logs the
  conflict with both row IDs, so an operator can resolve it. A conflict must not
  silently merge or drop a user's skill.

A non-system row whose slug is **not well-formed** is left untouched and reported,
not normalized (AC-003.9): prefixing it would produce a slug that still cannot be
a directory name. The sync reports it and moves on rather than failing the
workspace, because such a row can only predate the write-time validation of
AC-001.11, so it is a legacy artifact an operator must rename, not an error the
sync can resolve.

Because the row ID is preserved, `skill_ids` references need no rewrite. Slug
references in `desired_skills` do, and reuse `replaceJSONArrayValue`, whose
existing semantics AC-003.8 pins: it replaces every occurrence of the old value,
preserves first-occurrence order, drops empty entries, and de-duplicates, so a
slug appears at most once even when the agent already referenced both the old and
the new slug.

**Concurrency.** `SyncSystemSkills` has four production call sites: startup
(`backendapp/main.go`), the lazy sync when a workspace's skills are first listed
(`skills/service.go`), and two in the agents service. Nothing serializes them
today, so a startup pass can race a lazy pass over the same workspace and both can
attempt the same insert or normalization. Sync therefore takes a process-local
mutex keyed by workspace ID for the duration of a workspace's pass (AC-003.7). All
four call sites are in one process, so a process-local lock suffices; no
cross-process coordination is introduced. A pass that cannot acquire the lock waits
rather than skipping, so the caller still observes a completed sync.

## Control flow

1. Kandev starts, or a workspace lists skills for the first time. The sync takes
   the workspace lock, reconciles bundled skills (skipping and reporting any slug
   held by a user row), applies the bundled rename, then normalizes non-system
   slugs and rewrites agent references.
2. Subsequent passes observe the reconciled state and write nothing (AC-003.6),
   except to re-report unresolved conflicts.

## Failure and recovery

The sync is idempotent and safe to re-run. A rename interrupted between the row
update and the agent-reference rewrite leaves an agent referencing a slug that no
longer resolves; the next pass repeats the reference rewrite, because the
normalized row is already present and the stale reference is still detectable. If
a single agent's reference rewrite fails, the pass returns that error and the
workspace is left partially migrated; the next pass resumes from the current
state, since every step is expressed as a reconciliation against observed rows
rather than as a delta. A conflict is reported on every pass until an operator
resolves it, rather than being silently retried.

## Persistence

The skill record's slug column is authoritative. The rename preserves row IDs so
foreign references and `skill_ids` arrays survive. No schema change is required:
this is a data normalization within an existing column, applied through the
service layer so conflict detection and reference rewriting run with it, rather
than through an inline SQL statement that could not do either.

## Observability

The sync's existing single summary log line reports inserted, updated, and removed
slugs per workspace. It gains the normalized and conflicted slugs so an operator
can confirm what an upgrade did to a workspace. Every per-workspace list is sorted
lexicographically by slug before logging. Sorting is required because the
retirement and normalization phases iterate maps, whose order Go does not define.

Per-workspace sorting is not sufficient for the report `SyncSystemSkills` returns
across several workspaces. Its entries are scoped strings of the form
`<workspace_id>:<slug>`, so "sorted by slug" is not well defined for them, and
their order otherwise follows the caller-supplied workspace order, which no caller
guarantees. The aggregate is therefore sorted by workspace identifier and then by
slug (AC-003.5), which for the scoped-string form is plain lexicographic order of
the entries. Without this, the AC's stated purpose — a meaningful diff between two
upgrades — fails at exactly the level the report is emitted.

## Related decisions

- [Injected Skill Naming](injected-skill-naming.md), which owns the naming,
  ownership, and collision rules this migration applies.
- [ADR 0031: Office Skill Reference Files](../../../decisions/0031-office-skill-reference-files.md)
