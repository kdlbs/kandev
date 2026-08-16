---
title: Pull request outcome attribution
status: draft
created: 2026-08-12
amended: 2026-08-16
owner: kandev
---

# Pull request outcome attribution

## Amendment history

**2026-08-15 — narrowed to upstream observation only.** The reviewer of PR #2614
decided that a Kandev-recorded closure reason does not belong in core: a user who
needs that workflow adds it in a plugin, which can own the reason taxonomy, the
replacement links, its own storage, and its own UI. Core keeps only the facts
GitHub already hands us.

Removed from this contract: the `disposition`,
`disposition_superseded_by_url` and `disposition_recorded_at` columns, the
`PATCH /api/v1/github/task-prs/:associationId/disposition` endpoint, the CI
popover disposition control, and their E2E coverage and translations. The
retained contract is the five upstream-observation columns, the clients that
source them, the activation instant, the no-backfill rule, and the writer-health
invariants.

**2026-08-16 — spec-review round 1 amendments.** Spec Review returned NEEDS
RETHINK; the human accepted the two writer-health findings and directed that the
rest be fixed. **AC-36 and AC-37 are therefore unchanged and were accepted as
written**, with their known limits: AC-36 has a legitimate `NULL` class (a merger
whose account was deleted, per AC-12), and AC-37's antecedent — whether the most
recent sync was a populating one — is not persisted on the row and so cannot be
evaluated from a snapshot. Both limits are accepted deliberately; do not "fix"
either AC by weakening or restating it.

Amended in this round: AC-05's activation trigger, which was unsatisfiable on a
fresh install (the columns arrive inline from `CREATE TABLE`, so no `ALTER` ever
adds one); AC-07 plus a new AC-43, because the "exactly one writer" claim was
false — four statements write the row, and `ReplaceTaskPR`'s `DELETE`+`INSERT`
could clear the auto-merge latch; the Concurrency section, which described a
read-then-write race whose stated outcome contradicted AC-17; new AC-08a and
AC-15a fixing the closed-event selection and the reopened-open-row reading; new
AC-12a for omitted or null `isDraft` / `changedFiles`; AC-14's null-actor
definition; new AC-18a / AC-18b for publish failure and no-op publication; a
clock-skew note; and four new verification surfaces, including one pinning
AC-30's explicit-`null` JSON that nothing previously checked.

**2026-08-16 — spec-review round 2 amendments.** Spec Review returned FIX FIRST /
BUILDABLE-WITH-GAPS with six defects, none of which required any acceptance
criterion to change meaning. All six are closed here, plus one optional
clarification that was taken rather than left.

Amended: **AC-04**, whose "retain `NULL` permanently" clause was true only for
rows already terminal at activation and, read literally, contradicted the Data
model, AC-12 and AC-37 — an open pre-existing row does populate on its next
post-activation sync, and a test asserting otherwise asserts a defect. The
**Nil, empty, and error** section, whose blanket "an empty-string login writes
`NULL`" was correct for `merged_by_login` but wrong for `closed_by_login`, where
AC-14 already defines an empty login as not an observation and AC-15 therefore
leaves the column unchanged; the two columns are now contrasted explicitly, and
the difference is whether an absent value overwrites or preserves. The
**Defaults and boundaries** paragraph, which called `changed_files` three-state
while also requiring an out-of-range negative be stored verbatim. **AC-18**,
which now says its identity comparison is against the sync's pre-statement
snapshot rather than a read-back, which is what makes AC-18b consistent instead
of contradictory. Three verification surfaces: the column-list drift check now
names all four AC-07 writers including `RestoreTaskPR`, the AC-43 preservation
check now covers all five columns rather than the latch alone, and the migration
check is explicitly scoped to "no sync in between" so it cannot be widened into
the AC-04 misreading above.

New: **AC-43a**, requiring `ReplaceTaskPR`'s carry-forward read, `DELETE` and
`INSERT` to execute in a single transaction. AC-43 required the value be carried
forward but never said the read had to be atomic with the write, so a read taken
outside the transaction could observe `NULL`, lose a race to a concurrent
populating sync, and write the stale `NULL` back — clearing the latch and
violating AC-17 while satisfying every word of AC-43. This is the
`DELETE`+`INSERT` counterpart to the `COALESCE` guard that protects
`UpdateTaskPR`, and it is now named for the same reason that one is.

**AC-36, AC-37 and AC-41b remain unchanged and were not touched in this round
either.** They were accepted with their known limits in round 1 by human
decision; AC-04's fix was deliberately scoped so that AC-37 needed no edit.

**2026-08-16 — spec-review round 3 amendments.** Spec Review returned FIX FIRST /
BUILDABLE-WITH-GAPS with seven defects, none requiring an acceptance criterion to
change meaning. All seven are closed here. Both outside-voice legs converged on
AC-43; the cross-model leg additionally found a writer this spec declared did not
exist.

Amended: **AC-07**, whose claim that its four writers are "the COMPLETE set of
statements that write a row of this table" was **false** — the same defect class
round 1 fixed when it killed the earlier "exactly one writer" claim.
`migratePRTablesForMultiRepo` rebuilds `github_task_prs` when the legacy
`UNIQUE(task_id, pr_number)` constraint is present, and its
`INSERT INTO github_task_prs_new … SELECT … FROM github_task_prs` rewrites every
row in the table. AC-07 now separates **row-value writers** (the four service-layer
statements) from the **schema-rebuild copy statement**, and new **AC-07a** binds the
rebuild explicitly. **AC-40** now reaches the rebuild's `CREATE TABLE` and copy list,
because that `CREATE TABLE` *becomes* the live `github_task_prs` DDL after the
rename — leaving the three disposition columns there would abort startup on any
pre-multi-repo database once the migration stops adding them. **AC-43** gains a
per-column provenance rule and a caller obligation, because `nil` alone cannot
distinguish "observed absent, overwrite" from "not observed, preserve". **AC-43a**
extends its in-transaction read from the latch alone to all five carried-forward
columns. **AC-18**'s no-read-back prohibition is now scoped to the change
comparison, so it no longer contradicts AC-18b's premise that the published payload
is the row as stored. **AC-03** states partial-migration atomicity; **AC-38** names
its increment point.

**AC-36, AC-37, AC-41b and AC-14 were not touched in this round either**, and were
verified verbatim after every edit above.

**2026-08-16 — spec-review round 4 amendments.** Spec Review returned FIX FIRST /
BUILDABLE-WITH-GAPS with eight defects, and — for the fourth consecutive round —
both outside-voice legs independently found a defect in AC-43. That tripped the
oscillation rule, so the reviewer stopped and asked rather than routing. **The human
decided: rewrite the AC-43 family rather than patch it a fourth time.** This round
implements that decision.

**AC-43, AC-43a and AC-43p were rewritten as one rule** (`Writer set and row
replacement`, which now carries its own "Why this shape, and what was rejected"
subsection). The round-3 contract was internally impossible: AC-43p made the
*caller* resolve the five values, AC-43 made the store write them "as supplied" and
forbade "second-guessing its input", and AC-43a made the store read all five columns
in-transaction — so the in-transaction read was data the store was barred from
using, and the `merged_by_login` race it claimed to close stayed open. Round 3's
`RestoreTaskPR` exemption left the same race open on the *more* reachable writer,
since a relink requires the row to exist. The new rule: **`ReplaceTaskPR` and
`RestoreTaskPR` resolve all five columns inside their own write transaction**,
against the outgoing row read in that transaction, from a caller-supplied
observation plus explicit populated-ness flags. Callers no longer resolve. The
per-column split, the write-as-supplied rule and the `RestoreTaskPR` exemption are
all deleted; the latch's first-write-wins now falls out of the same resolution.
The alternative — naming the race as an accepted gap — was considered and rejected,
because AC-36 is a human-locked invariant a downstream extract reads as a
writer-fault detector.

Also amended: **AC-07** and the **Data model**, which claimed all four row-value
writers were bound by AC-43 when AC-43's head clause exempts `UpdateTaskPR`; AC-07
now states the split and records that `CreateTaskPR` has **no production caller**
(the HTTP create path routes through `AssociateExistingPRByURLForWorkspace` to
`ReplaceTaskPR`). The **Concurrency** section, whose claim that out-of-order polls
are "self-correcting" because "the next poll reconciles" contradicted the
Persistence guarantees for terminal rows; the merge-boundary inversion is now named
as an accepted AC-36 limit, with the asymmetry against AC-43's closed-not-accepted
treatment spelled out. **AC-41**, extended to telemetry counters, plus new
**AC-41c** pinning the disposition expvar counter — the one removal surface that
hides behind a retained AC, since it cites AC-38 in its own comment and sits outside
every other removal enumeration and grep.

New: **AC-05a** (a `kandev_meta` activation write failure aborts startup, set-if-absent
so AC-06 survives concurrent first boots) and **AC-18c** (a failed sync-side row
write returns its error, publishes nothing, leaves the row untouched, and does not
roll back the AC-38 counter). Both behaviours were previously asserted only by a
Failure modes row or by prose — and the prose called the write "best-effort", which
is wrong. Ten **verification surfaces** were added, covering the entire Writer health
section (AC-36, AC-37, AC-38, AC-39), which had none at all, plus AC-05a, AC-09,
AC-18a/18b/18c, AC-19, AC-41b and AC-42.

A **does-not-modify note** in Writer health now enumerates the three legitimate
`NULL` classes under AC-36 and AC-37 in one place, including the AC-12a omission
case, so a consumer can subtract them and a test does not assert either invariant
unconditionally.

**AC-36, AC-37, AC-41b and AC-14 were not touched in this round either**, per the
round-1 human decision, and were verified verbatim after every edit above.

**Acceptance criteria are not renumbered.** AC-20 through AC-29c and AC-31
through AC-35 are **retired** and MUST NOT be reused for new criteria: tests and
commits already in this branch cite AC numbers, and renumbering would silently
re-point that traceability at different behaviour. Gaps in the sequence are
deliberate. AC-40 and AC-41 are new and state the removal as a positive,
checkable contract.

## Problem

`github_task_prs` records that a pull request reached `closed`, but nothing
about **who** ended it or under what upstream conditions. Four facts GitHub
already hands us on every sync are simply never asked for: who merged the PR,
who closed it, whether auto-merge was armed, and how many files it changed. A
fifth, `draft`, arrives on every sync and is thrown away after being collapsed
into `mergeable_state = 'draft'`.

A closure **reason** is a separate matter and is deliberately not core's. A
GitHub pull request carries no `state_reason` (that field exists on issues and
is `null` for every PR, verified below), and Kandev never closes a PR: there is
no `ClosePR`, no `gh pr close`, and the only `gh pr` subcommands in the code are
`view` and `list` (`gh_client.go`). There is no action to
hook and no upstream field to read, so any reason would have to be invented and
recorded by a human. That taxonomy is a workflow opinion, not a GitHub fact, and
it lives in a plugin (see Out of scope).

Without the observation half, "closed unmerged" cannot even be attributed to a
person. The Ops Cost dashboard called it abandoned on n=2 and retracted on
2026-08-12. This spec exists so the retraction does not have to happen twice: it
records what was observed and refuses to infer the rest.

## Input inventory (sampled, not assumed)

Everything below was read from the code or fetched live from the GitHub API on
2026-08-12. Where a claim in the originating card turned out to be wrong, the
correction is marked.

### Upstream field availability

Live REST sample, `GET /repos/kdlbs/kandev/pulls/2554` (merged):

| Field | Present | Value |
|---|---|---|
| `draft` | yes | `false` |
| `merged_by` | yes | `{login: "carlosflorencio", ...}` |
| `changed_files` | yes | `8` |
| `auto_merge` | yes | `null` |
| `closed_by` | **key absent from the response** | — |

Live REST sample, `GET /repos/kdlbs/kandev/pulls/2476` (closed, unmerged):
`merged_by = null`, `changed_files = 114`, `auto_merge = null`, `draft = true`,
`closed_by` absent.

Live REST sample, `GET /repos/kdlbs/kandev/issues/2476` (the same PR, via the
issues endpoint): `closed_by = {login: "nova28", ...}`, and
**`state_reason = null`** — confirming that `state_reason` is serialized for PRs
but never populated, so it is not a usable closure reason.

`gh pr view --json` field list (gh 2.87.3, printed locally): includes
`autoMergeRequest`, `changedFiles`, `isDraft`, `mergedBy`. It does **not** include
`closedBy`.

**Correction to the originating card:** `merged_by`, `auto_merge` and
`changed_files` are near-free additions. `closed_by` is **not** — it is absent
from the pulls REST endpoint and from the gh CLI's PR field set, and the GraphQL
`PullRequest` type has no `closedBy` field either. Acquiring it requires either a
`timelineItems(itemTypes: CLOSED_EVENT)` selection on the GraphQL path, or a
second REST call to the issues endpoint. This spec picks the GraphQL route and
names the resulting gap (AC-14, AC-15).

### Existing code shape

*Citations below are by SYMBOL, deliberately. An earlier revision carried
`path:line` hints; every one of them had drifted by a few hundred lines within
three review rounds while the symbols stayed correct, so the line numbers were
removed rather than refreshed. Cite the symbol; let the reader's tooling find it.*

- GraphQL field selection: `prFieldsBlock()` in `graphql.go`; decoded into
  `batchedPRResult` and converted at `convertBatchedPRResult`.
- gh CLI single-PR field list in `gh_client.go`. REST shape: `patPR`
  (`pat_client.go`).
- Table DDL: `createTablesSQL` in `store.go`. Column projection used by **every**
  read: `taskPRColumns` and `taskPRColumnsQualified`. Reads use an explicit list,
  not `SELECT *`, so a new column is invisible until it is added to both lists
  (`store_taskpr_schema_drift_test.go` documents why), and a column left behind in
  the table but absent from both lists is inert.
- Row-value writers: `CreateTaskPR`, `ReplaceTaskPR`, `RestoreTaskPR`,
  `UpdateTaskPR` (all `store.go`), driven by `SyncTaskPR`
  (`service_pr_watch.go`). See AC-07 — these are the four that write these five
  columns from a caller value, and they are NOT the only statements that write a
  row of this table.
- **Schema-rebuild writer**, load-bearing and easy to miss:
  `migratePRTablesForMultiRepo` calls `rebuildIfHasLegacyConstraint` for
  `github_task_prs`, gated on the literal `UNIQUE(task_id, pr_number)` substring in
  that table's stored `sqlite_master` DDL. It creates `github_task_prs_new`, copies
  every row with an explicit `INSERT … SELECT` column list, drops, and renames — so
  its `CREATE TABLE` becomes the live DDL. Governed by AC-07a and AC-40.
  Ordering matters: `addTaskPROutcomeColumns` runs in `initSchemaUpgrades`,
  the rebuild later in `initSchemaData`, and the rebuild's copy list relies on that
  ordering having already added the columns it names.
- Other writers that touch a row but name none of the five, so preserve them
  trivially: `DetachTaskPR` (`UPDATE … SET detached_at`) and
  `backfillTaskPRsRepositoryID` (dedup `DELETE` plus `UPDATE … SET repository_id`).
- Migration idiom in this store: `applyIdempotentSchemaColumns` swallows every
  error via `_, _ = s.db.Exec(...)`; `addWatchSelfHealColumns` and
  `addTaskPROutcomeColumns` are the fail-loud variants, used precisely because
  their readers scan the column unconditionally. The added-column list lives in
  `taskPROutcomeColumnDDL`.
- `Populated`-flag precedent for partial sync paths: `PRStatus.ChecksPopulated`,
  `ReviewCountsPopulated`, `UnresolvedReviewThreadsPopulated` (`models.go`).
- Terminal rows are excluded from the orphan sweep (`taskPRNeedsUnwatchedSync`,
  `service_pr_unwatched.go`): once a row is `merged` or `closed` it is never
  re-fetched by that path.
- Schema-metadata table: `kandev_meta` (`internal/persistence/meta.go`), created
  during `persistence.Provide` before any repository schema init, and deliberately
  preserved by the database reset path (`internal/system/database/reset.go`).

### Confirmed non-defects

`review_count` reading 0 everywhere is **correct**: it counts distinct reviewers
whose latest state is `APPROVED` (`client_helpers.go`,
`countApprovedReviewerNodes` in `graphql.go`) and is written on every sync.
`comment_count` genuinely is frozen (`service_pr_watch.go` — "CommentCount is
no longer updated from polling"). Neither is in scope here.

## Scope

**Upstream-supplied outcome fields.** Persist `is_draft`, `changed_files`,
`merged_by_login`, `closed_by_login`, and an auto-merge observation, sourced from
the syncs Kandev already performs. Every value is a direct observation of a
GitHub fact.

**Cross-cutting.** Nullable-only columns, no backfill, a published activation
point, and a writer-health invariant that a downstream extract can check.

**Not in scope.** Any Kandev-recorded closure reason, any superseding-PR link,
and any UI for either. See Out of scope.

## Data model

Five nullable columns on `github_task_prs`. Every one is `NULL` on an existing
row and stays `NULL` until a post-activation populating sync writes it. None has
a non-`NULL` default.

Their *values* originate in exactly one place — the sync writer's observation of
upstream — but **four SQL statements carry those values to disk** (`CreateTaskPR`,
`ReplaceTaskPR`, `RestoreTaskPR`, `UpdateTaskPR`; AC-07). Of those four, **two are
bound by AC-43**: `ReplaceTaskPR` and `RestoreTaskPR`, the statements that can
destroy a stored value while carrying no observation of their own. `UpdateTaskPR`
is the sync writer's own statement and is exempt by AC-43's head clause;
`CreateTaskPR` inserts a new row, so "preserve the stored value" is vacuous for it,
and it has no production caller. AC-07 states that split and why. No user-facing
mutation writes any of the five, and no endpoint accepts them as input.

A **fifth** statement writes these columns without carrying a caller value: the
`github_task_prs` schema rebuild copies every row column-to-column
(AC-07a). It cannot write a wrong value, but it can drop all five for every row at
once by omitting them from its copy list, so it is enumerated here rather than left
out because it is "just a migration".

| Column | Type | Meaning when `NULL` |
|---|---|---|
| `is_draft` | BOOLEAN NULL | never observed by a populating sync |
| `changed_files` | INTEGER NULL | never observed (distinct from `0`, a real value) |
| `merged_by_login` | TEXT NULL | not merged, or never observed |
| `closed_by_login` | TEXT NULL | not closed, or closure never observed by the GraphQL path. A non-`NULL` value does NOT imply the PR is currently closed — see AC-15a |
| `auto_merge_observed_at` | DATETIME NULL | auto-merge never observed armed while Kandev was watching |

`auto_merge_observed_at` is a **latched observation**, not a merge cause. GitHub
clears `auto_merge` once it fires (`pulls/2554` is merged and reports
`auto_merge: null`), so a poller can only ever learn "auto-merge was armed at
some instant while we were looking." It must not be read, named, or charted as
"merged by auto-merge."

`NULL` never means zero, false, or empty. A reader that coalesces any of these
five columns to a non-`NULL` default destroys the only signal that says whether
the writer is running at all (AC-36, AC-37).

## Acceptance criteria

### Migration and activation

- **AC-01** — WHEN backend startup runs the GitHub store's schema
  initialization against a database that lacks any of the five columns, THE
  SYSTEM SHALL add each missing column via `ALTER TABLE ... ADD COLUMN` with no
  `NOT NULL` and no `DEFAULT` clause.
- **AC-02** — WHEN schema initialization runs a second time against the same
  database, THE SYSTEM SHALL complete without error and without altering any row.
- **AC-03** — IF an `ADD COLUMN` statement fails for any reason other than the
  column already existing, THEN THE SYSTEM SHALL return that error from schema
  initialization and abort startup. Duplicate-column classification SHALL use
  `db.IsDuplicateColumnError` per ADR 0027; this path SHALL NOT use
  `_, _ = s.db.Exec(...)` and SHALL NOT add a new local error-string classifier.
  Rationale: the five columns are added to `taskPRColumns`, so a silently failed
  migration turns every task-PR read into a scan error rather than a missing
  value.

  The five `ADD COLUMN` statements are applied **one at a time and are not rolled
  back as a group**. If the third fails, the first two remain applied and startup
  aborts. This is deliberate and safe rather than merely tolerated: the statements
  are individually idempotent (AC-02 re-checks `PRAGMA table_info` before each),
  so the next startup adds only what is missing and converges on the same schema,
  and AC-05's activation stamp is gated on all five existing — so a half-migrated
  database is never stamped and never reported on. No serving happens between the
  abort and the retry.
- **AC-04** — WHEN the migration runs, THE SYSTEM SHALL NOT execute any `UPDATE`
  or backfill against `github_task_prs`. Every pre-existing row SHALL hold `NULL`
  in all five columns at the instant the migration completes, and SHALL acquire a
  value only from a later populating sync that observes it (AC-12, AC-14, AC-16) —
  never from this or any other migration. A pre-existing row that was already
  **terminal** (`merged` or `closed`) at activation therefore keeps `NULL`
  permanently, because terminal rows are excluded from the orphan sweep
  (`taskPRNeedsUnwatchedSync`) and are never re-fetched. A pre-existing row that
  was still **open** at activation is not permanently `NULL`: its next populating
  sync writes `is_draft` and the rest of the group like any other row. The
  distinction is stated because "no backfill" and "never populated" are different
  claims, and conflating them puts this AC in direct conflict with the Data model
  ("stays `NULL` until a post-activation populating sync writes it"), with AC-12,
  and with AC-37, whose whole value as a writer canary depends on pre-existing
  open rows populating after activation. A test asserting that an open
  pre-existing row stays `NULL` through a populating sync is asserting a defect.
- **AC-05** — WHEN schema initialization completes for the first time on a given
  database after which all five columns exist — whether they arrived inline from
  the `github_task_prs` `CREATE TABLE` on a fresh install, or via `ALTER TABLE`
  on an existing one — THE SYSTEM SHALL persist the current UTC instant in
  `kandev_meta` under key `github_task_pr_outcome_activated_at`, in RFC 3339
  form. The trigger is deliberately NOT "an `ALTER TABLE` ran". A fresh install
  receives the five columns inline from the `CREATE TABLE` and never executes an
  `ADD COLUMN` for any of them, so an ALTER-gated stamp would leave **every new
  database** with the columns present and no activation instant — the exact state
  the Failure modes table below declares unreportable and unshippable, and one
  that makes AC-36, AC-37 and AC-39 unevaluable. The stamp is therefore evaluated
  on every startup and written at most once (AC-06).
- **AC-06** — WHILE `github_task_pr_outcome_activated_at` already has a value,
  THE SYSTEM SHALL leave that value unchanged on every subsequent startup,
  including startups that add a later column from the set. A database that
  already activated while an earlier revision of this branch added eight columns
  therefore keeps its original activation instant; the narrowing does not
  re-stamp it and MUST NOT.
- **AC-05a** — IF writing the activation instant to `kandev_meta` fails for any
  reason — including a failure to ensure the `kandev_meta` table exists — THEN THE
  SYSTEM SHALL return that error from schema initialization and abort startup. This
  path SHALL NOT swallow the error, SHALL NOT log-and-continue, and SHALL NOT use
  the `_, _ = s.db.Exec(...)` form.

  The write itself SHALL be expressed as a set-if-absent (`INSERT … ON CONFLICT DO
  NOTHING` or equivalent) so that AC-06's write-at-most-once holds against two
  processes initializing the same database concurrently, rather than depending on a
  read-then-write sequence that both can pass.

  This is stated as its own criterion because the behaviour was previously asserted
  **only** by a row in the Failure modes table, and a table row is not a
  requirement. The default it guards against is the house idiom, not an exotic
  mistake: `applyIdempotentSchemaColumns` in this same store swallows every error
  via `_, _ = s.db.Exec(...)`, so "log and carry on" is the shape an implementer
  copies unless told otherwise. AC-03 spells out the equivalent fail-loud rule for
  `ADD COLUMN` and explicitly bans that form; AC-05 said only "SHALL persist" and
  named no failure behaviour at all. The consequence of getting it wrong is the
  exact state AC-05's own rationale calls unshippable — a database that has the
  five columns but no activation instant, which makes AC-36, AC-37 and AC-39
  unevaluable and every report over these columns unscoped.
- **AC-07** — THE SYSTEM SHALL include all five columns in `taskPRColumns` and
  `taskPRColumnsQualified`, and in the column lists of `CreateTaskPR`,
  `ReplaceTaskPR`, `RestoreTaskPR` and `UpdateTaskPR`, so every existing read
  path returns them, every statement that writes a `github_task_prs` row can
  write them, and no read path regresses to `SELECT *`. Those four are the
  complete set of **row-value writers** — statements that set these five columns
  to a caller-supplied value. A row-value writer added later joins this list.

  **Which of the four AC-43 actually binds, and which it does not.** This AC
  previously said "each is bound by AC-43", and the Data model said all four were
  bound by AC-43 and AC-43p. Both over-claimed, because AC-43's own head clause
  scopes itself to statements **other than** the sync writer's per-row `UPDATE`.
  The accurate split:
  - `ReplaceTaskPR` and `RestoreTaskPR` — **bound by AC-43.** They can destroy a
    stored value while carrying no observation of their own. These two are AC-43's
    entire subject.
  - `UpdateTaskPR` — **exempt, deliberately.** It is the sync writer's own
    statement: its values were produced by `resolveTaskPROutcomeFields` against the
    row it is about to write, and its latch is protected in SQL by
    `COALESCE(auto_merge_observed_at, ?)`. AC-12, AC-13, AC-14 and AC-15 govern it.
    Applying AC-43 to it would be circular — it would require the sync writer to
    preserve values against itself.
  - `CreateTaskPR` — **vacuous, and has no production caller.** AC-43 says
    "preserve the stored value"; an `INSERT` of a new row has no stored value to
    preserve. The store method exists and is exercised by tests, but the HTTP
    create path (`httpCreateTaskPR`) routes through
    `AssociateExistingPRByURLForWorkspace` to `associatePRWithTask`, which writes via
    `ReplaceTaskPR`. Nothing in production calls `CreateTaskPR`. It stays in the
    column lists above (AC-07's read/write completeness requirement is unchanged)
    and it owes AC-43 nothing.

  A conformance check written from the old sentence would assert AC-43 against
  `UpdateTaskPR`, which AC-43 exempts, and would look for a caller obligation on
  `CreateTaskPR` that has no caller to carry it.

  They are **not** the complete set of statements that write a row of this table,
  and this AC no longer claims they are. Three others exist, and the distinction
  between them is what decides whether each is a risk:
  - `DetachTaskPR` (`UPDATE … SET detached_at`) and `backfillTaskPRsRepositoryID`
    (a dedup `DELETE` plus an `UPDATE … SET repository_id`) name none of the five
    columns, so they preserve them trivially. They need no rule.
  - The **schema-rebuild copy statement** in `migratePRTablesForMultiRepo` is a
    different shape and a real risk. AC-07a governs it.

  This AC previously asserted the four were exhaustive. That was false, and it is
  recorded here rather than quietly corrected because it is the second time this
  spec has understated the writer set: round 1 removed an "exactly one writer"
  claim on the same grounds. An enumeration that is trusted and wrong is worse
  than no enumeration, because the column-list drift check is scoped to it.

- **AC-07a** — THE SYSTEM SHALL include all five columns in **both halves** of the
  `github_task_prs` rebuild performed by `migratePRTablesForMultiRepo` via
  `rebuildIfHasLegacyConstraint`: the `CREATE TABLE github_task_prs_new` DDL and
  the `INSERT INTO github_task_prs_new (…) SELECT … FROM github_task_prs` copy
  list, on both the insert side and the select side.

  This statement is a full-row rewrite of **every row in the table**, executed when
  the legacy `UNIQUE(task_id, pr_number)` constraint is found in `sqlite_master`,
  followed by a drop and rename — so its `CREATE TABLE` *becomes* the live
  `github_task_prs` DDL. Two distinct failures follow from getting it wrong, and
  neither is visible on inspection of the statement:
  - A column omitted from the **copy list** is silently dropped for every row at
    once, including a latched `auto_merge_observed_at`, violating AC-17 without any
    `UPDATE` ever running. This is the same loss shape AC-43 exists to prevent,
    at table scale rather than row scale.
  - A column omitted from the **new DDL** disappears from the table entirely, which
    then breaks every read through `taskPRColumns` (AC-03's rationale).

  The rebuild is *not* bound by AC-43's preservation rule, because it carries no
  caller-supplied values at all: it copies each column to itself. Its obligation is
  simply that the five columns appear in both lists.

### Sourcing outcome fields from upstream

- **AC-08** — THE SYSTEM SHALL request `isDraft`, `changedFiles`, `mergedBy`,
  `autoMergeRequest` and a closed-event actor selection in the shared GraphQL PR
  field block, so the batched PR query and the batched branch query return the
  same fields.
- **AC-08a** — THE SYSTEM SHALL make that closed-event selection
  `timelineItems(last: 1, itemTypes: CLOSED_EVENT)`, so that WHEN a pull request
  has been closed, reopened and closed again, the MOST RECENT closure wins and
  `closed_by_login` names the person who ended it last. `first: 1` is the
  explicitly rejected alternative: it pins the column to the original closer
  forever, which for a PR reopened and then closed by someone else attributes the
  ending to the wrong person. This is the one ordering decision the feature
  introduces, and it is named here so it cannot be flipped silently later.
- **AC-09** — THE SYSTEM SHALL request `changedFiles`, `mergedBy` and
  `autoMergeRequest` in the gh CLI single-PR `--json` field list, and
  `changed_files`, `merged_by` and `auto_merge` on the REST single-PR path.
  `closedBy` SHALL NOT be requested on either path, because neither exposes it.
- **AC-10** — WHEN a sync path has fetched a full single pull request or a
  batched GraphQL result, THE SYSTEM SHALL mark the outcome-field group as
  populated on the resulting status value.
- **AC-11** — WHEN a sync path has not fetched those fields (list or search
  results, or the noop client), THE SYSTEM SHALL leave the group unpopulated.
- **AC-12** — WHILE the outcome-field group is marked populated, THE SYSTEM
  SHALL write `is_draft`, `changed_files` and `merged_by_login` from the observed
  values, writing `NULL` for `merged_by_login` when upstream reports no merger.
- **AC-12a** — WHILE the outcome-field group is marked populated, IF the upstream
  response omits `isDraft` or `changedFiles`, or returns either as `null`, THEN
  THE SYSTEM SHALL write `NULL` for that column rather than the decoder's zero
  value, leaving the other columns of the group unaffected. **Presence, not
  truthiness, decides.** A Go `bool` decodes an absent `isDraft` to `false` and a
  Go `int` decodes an absent `changedFiles` to `0`; persisting those as
  observations manufactures precisely the fact this feature exists to distinguish
  from "not observed". It is worst for `changed_files`, where `0` is also a
  legitimate reading (AC-12 and the Defaults section both require a real `0` to
  be stored), so a fabricated zero is indistinguishable from a true one after the
  fact. A group marked populated is therefore not a promise that every field in
  it arrived; each field is judged on its own presence.
- **AC-13** — IF the outcome-field group is not marked populated, THEN THE
  SYSTEM SHALL leave all three columns at their stored values, including `NULL`.
- **AC-14** — WHEN a GraphQL sync observes a closed event whose `actor` is
  non-null AND whose login is a non-empty string, THE SYSTEM SHALL mark closure
  attribution as populated and write that login to `closed_by_login`. A
  closed-event node with a `null` actor (a deleted account) or an empty login is
  NOT an observation of an actor: attribution stays unpopulated and AC-15
  applies. The exclusion is stated inside this AC rather than left to the Failure
  modes table alone, because AC-14 read by itself would otherwise be
  implementable as "write whichever login field came back", which persists `''`
  as though it were an observation.
- **AC-15** — IF closure attribution is not marked populated, THEN THE SYSTEM
  SHALL leave `closed_by_login` unchanged. A pull request whose only sync after
  closure came through the REST or gh CLI fallback therefore keeps
  `closed_by_login = NULL` permanently, because terminal rows are excluded from
  the orphan sweep (`taskPRNeedsUnwatchedSync`, `service_pr_unwatched.go`). This
  gap is accepted and must
  be stated wherever the column is consumed.
- **AC-15a** — THE SYSTEM SHALL NOT treat `closed_by_login` as a closed-state
  indicator. A pull request that was closed and later reopened retains its
  `CLOSED_EVENT` history, so a populating GraphQL sync of a currently-OPEN row
  may legitimately write or retain a non-`NULL` `closed_by_login`; the value is
  never cleared on reopen. Any consumer deciding whether a pull request is closed
  SHALL read `state` or `closed_at`. Reading `closed_by_login IS NOT NULL` as
  "this PR is closed" is wrong by construction, and the pairing of AC-08a
  (latest closure wins) with a reopen makes it reachable in normal operation
  rather than only in edge cases.
- **AC-16** — WHEN a populating sync observes auto-merge armed on a pull request
  AND `auto_merge_observed_at` is `NULL`, THE SYSTEM SHALL set it to the current
  UTC instant.
- **AC-17** — WHILE `auto_merge_observed_at` is non-`NULL`, THE SYSTEM SHALL
  never clear it or overwrite it, including when a later sync observes auto-merge
  disarmed or absent.
- **AC-18** — WHEN a sync changes any of `is_draft`, `changed_files`,
  `merged_by_login`, `closed_by_login` or `auto_merge_observed_at`, THE SYSTEM
  SHALL treat the row as changed for the purposes of publishing
  `github.task_pr.updated`. WHEN a sync writes values identical to the stored
  ones, THE SYSTEM SHALL NOT publish the event. "Identical to the stored ones"
  means identical to **the sync's own pre-statement snapshot of the row**, not to
  a value re-read after the statement executed. The comparison is therefore made
  from data the sync already holds, and **the change comparison** SHALL NOT
  introduce a read-back query.

  That prohibition is scoped to the comparison and to nothing else. It does NOT
  forbid the read-back that produces the **event payload**, which is a separate
  step with the opposite requirement: the payload SHALL be the row as stored,
  re-read after the write, falling back to the in-memory value only if that read
  fails. The two must not be conflated. The decision "did anything change" is
  answered from the snapshot so the sync stays one query; the question "what do we
  publish" is answered from storage so consumers converge on committed state.
  Deleting the payload read-back to satisfy a broad reading of this AC would
  publish a sync's own discarded `auto_merge_observed_at` candidate — a value that
  was never stored — and AC-18a makes that permanent, since delivery is
  at-most-once and no later sync republishes an unchanged row.

  This scoping is what makes AC-18b consistent rather than contradictory: under a
  snapshot comparison a sync that lost the `auto_merge_observed_at` latch race
  still counts as having changed the row, because its snapshot said `NULL` and
  its candidate did not, even though the statement ultimately changed nothing —
  while the payload it publishes still carries the winning stored value.
- **AC-18a** — IF the row `UPDATE` succeeds and publishing
  `github.task_pr.updated` then fails, THEN THE SYSTEM SHALL log the failure and
  proceed: it SHALL NOT retry the publish, SHALL NOT roll back the row, and SHALL
  NOT re-publish from a later sync that observes no change. Event delivery for
  these five columns is therefore **at-most-once**, and the persisted row is the
  source of truth. A consumer that must not miss a transition reads the row; the
  event is a convergence hint, not a ledger.
- **AC-18b** — WHEN a sync's `auto_merge_observed_at` candidate is discarded by
  the first-write-wins latch (AC-16, AC-17) but the sync is otherwise treated as
  having changed the row, THE SYSTEM MAY publish `github.task_pr.updated` even
  though that candidate changed nothing. This is an accepted no-op publication
  and does not violate AC-18: the payload carries the row as stored (AC-18's
  payload rule), so every consumer still converges on the same state. Suppressing
  it would mean feeding that payload read-back into the change comparison as well,
  solely to detect a race whose outcome is already correct — which is exactly the
  extra comparison query AC-18 declines to require.
- **AC-18c** — IF the sync-side row write itself fails, THEN THE SYSTEM SHALL
  return that error to the sync's caller, SHALL NOT publish
  `github.task_pr.updated`, SHALL NOT retry the write inside the sync, and SHALL
  leave the row exactly as it was. **The next ordinary poll IS the retry**: there is
  no immediate retry, no backoff loop, and no queue. None of the five columns is
  partially written, because they all ride on the single row `UPDATE` that failed.

  The counter required by AC-38 has already been incremented by this point and
  SHALL NOT be rolled back: AC-38 counts at the moment the outcome-field group's
  populated-ness is decided, which happens before the write is attempted, and a
  sync whose write then fails has still reached its decision. A counter that only
  advanced on successful writes would go quiet during exactly the storage failure
  it is meant to expose, which is the opposite of what AC-36 and AC-37 use it for.

  This is stated as its own criterion because the behaviour previously lived only
  in the Nil/empty/error prose, and that prose additionally described it as
  following "the surrounding best-effort logging convention" — which is not what
  this path does or should do. A best-effort read of that sentence licenses
  swallowing the write error and continuing, which would let a sync report success
  while persisting nothing. Publication is the part that is best-effort here
  (AC-18a), not the write.
- **AC-19** — THE SYSTEM SHALL continue to derive `mergeable_state = 'draft'`
  exactly as today. `is_draft` is additive and SHALL NOT change any existing
  mergeability, auto-merge, or CI-automation behaviour.

*(AC-20 through AC-29c retired 2026-08-15 — the disposition endpoint. Numbers
are not reused; see Amendment history.)*

### Read surface

- **AC-30** — THE SYSTEM SHALL surface all five columns on the task-PR JSON
  representation returned by the existing task-PR endpoints and carried by
  `github.task_pr.updated`, and in the frontend `TaskPR` type. The JSON keys
  SHALL always be present with an explicit `null` rather than omitted, because
  "not observed" is the fact this feature exists to preserve and an absent key
  cannot express it.
- **AC-30b** — THE SYSTEM SHALL NOT render any of the five values in the core
  web UI. The frontend type is kept in sync with the API payload so a consumer
  is typed correctly; core adds no component, no control, and no new user-facing
  copy for this feature. Consequence, stated so it is not rediscovered: this
  feature has **no** user-visible surface and therefore requires no E2E coverage
  and no new translation keys.

*(AC-31 through AC-35 retired 2026-08-15 — the disposition UI and its i18n
obligation. Numbers are not reused; see Amendment history.)*

### Writer health

- **AC-36** — THE SYSTEM SHALL guarantee the invariant: for any
  `github_task_prs` row where `merged_at >= github_task_pr_outcome_activated_at`,
  `merged_by_login` is non-`NULL`. `merged_at` is only ever written by the sync
  writer, so a row that merged after activation was necessarily observed by a
  post-activation writer; a `NULL` there is a writer fault, not a data gap.
- **AC-37** — THE SYSTEM SHALL guarantee the invariant: for any row where
  `last_synced_at >= github_task_pr_outcome_activated_at`, `is_draft` is
  non-`NULL` whenever the row's most recent sync was a populating one. Because
  `is_draft` is supplied by every populating path on every sync, this is the
  primary canary for "the writer stopped."
- **AC-38** — THE SYSTEM SHALL increment process-local `expvar` counters, named
  under a `github_task_pr_outcome_` prefix, for populating syncs and
  non-populating syncs, following the existing metrics-map idiom in
  `internal/office/scheduler/metrics_vars.go` and `internal/common/subproc`.
  These are process-local and dev-mode-visible only; AC-36 and AC-37 are the
  durable, snapshot-checkable signals.

  The increment point is named so two implementations produce the same numbers:
  exactly one counter SHALL be incremented **once per sync of one pull request,
  at the point the outcome-field group's populated-ness is decided** — the
  populating counter when the group is marked populated (AC-10), the
  non-populating counter when it is not (AC-11). It counts sync attempts at that
  decision point, NOT fetch attempts, NOT decoded responses, NOT completed poll
  batches, and NOT successful row writes: a sync whose subsequent row write fails
  has still been counted, because the counters measure whether the writer is
  reaching its decision, which is what "the writer stopped" would show up in.
- **AC-39** — THE SYSTEM SHALL NOT state or imply either invariant for rows whose
  `merged_at` / `closed_at` predates the activation instant. Those rows are
  legitimately and permanently `NULL`.

*Note on the legitimate `NULL` classes — added 2026-08-16 (spec-review round 4).
This note does not modify AC-36 or AC-37; both stand exactly as written, per the
round-1 human decision. It enumerates, in one place, the cases in which a `NULL`
under either invariant is NOT a writer fault, so a consumer can subtract them
before raising an alarm and a test does not assert either invariant
unconditionally.*

For **AC-36** (`merged_at >= activation` implies `merged_by_login` non-`NULL`):
1. **Deleted merger account.** Upstream reports no merger login for a merge that
   did happen, so AC-12 writes `NULL`. Recorded since round 1.
2. **Merge-boundary poll inversion.** Two polls of the same pull request straddle
   its merge and land out of order; the earlier-reading poll writes last, setting
   `merged_by_login = NULL`, and the row is terminal so no later poll repairs it.
   Named and accepted in the Concurrency section, which also records why this is
   accepted while the AC-43 writers' version of the same loss is closed instead.

For **AC-37** (`last_synced_at >= activation` implies `is_draft` non-`NULL` when
the most recent sync was populating):
3. **Populating response that omits `isDraft`.** AC-12a requires `NULL` rather
   than the decoder's `false` when a populating response omits the field or
   returns it as `null`. AC-37's premise sentence — "`is_draft` is supplied by
   every populating path on every sync" — is precisely the assumption AC-12a
   exists to stop trusting, so the two are reconciled here rather than by editing
   either. A consumer treating every such `NULL` as "the writer stopped" raises a
   false alarm on an upstream or decoder fault, which is a different problem with a
   different fix.

None of these three is a writer fault, and none of them is a data gap of the kind
AC-39 covers (rows predating activation). They are the residual the invariants
knowingly carry. Any *other* `NULL` under either invariant is a writer fault, which
is what makes the invariants worth checking.

*Note on clocks — added 2026-08-16. This note does not modify AC-36 or AC-37;
both stand exactly as written.* `merged_at` and `closed_at` are GitHub-clock
instants, while the activation instant is a Kandev-clock instant written by the
local process, so the comparisons in AC-36, AC-37 and AC-39 span two clocks.
Skew between GitHub and an NTP-synchronised host is bounded well below one poll
interval, so at most the handful of rows whose terminal event falls within that
window either side of activation can be misfiled. A consumer working near the
boundary should treat rows within a few minutes of the activation instant as
indeterminate rather than as evidence in either direction.

### Narrowed-scope removal contract

- **AC-40** — THE SYSTEM SHALL NOT define `disposition`,
  `disposition_superseded_by_url` or `disposition_recorded_at` in the
  `github_task_prs` DDL, in `taskPRColumns`, in `taskPRColumnsQualified`, in the
  column list of any writer, in the migration's added-column list
  (`taskPROutcomeColumnDDL`, which drops from eight entries to five), or in
  **either half of the rebuild statement AC-07a names** — neither the
  `CREATE TABLE github_task_prs_new` DDL nor the copy list, on either side of its
  `SELECT`. THE SYSTEM SHALL NOT expose any of the three on the task-PR JSON
  representation or the frontend `TaskPR` type.

- **AC-40a** — Removing the three columns from the rebuild statement is
  **mandatory, not optional**, and the reason is a startup abort rather than
  tidiness. The migration that adds the outcome columns runs in
  `initSchemaUpgrades`; the rebuild runs later, in `initSchemaData`. Today the
  rebuild's copy list selects the disposition columns directly rather than through
  `COALESCE`, and its in-code comment states plainly that this is safe *because*
  the earlier migration has already added them. Narrowing that migration to five
  columns destroys the premise: a database still carrying the legacy
  `UNIQUE(task_id, pr_number)` constraint — a live upgrade path, not a
  hypothetical — triggers the rebuild, executes `SELECT … disposition` against a
  table that no longer has the column, and fails with `no such column: disposition`
  through a fail-loud path that aborts startup.

  This is stated as its own criterion because the failure is invisible to the
  checks that would otherwise be trusted. It is not reachable on a developer
  database that already ran the wide revision of this branch (that database
  rebuilt long ago, so the rebuild no-ops), and it is not reachable on a fresh
  install (no legacy constraint). It is reachable only on an older user database,
  which is exactly the population no one runs during development.

  **This does not conflict with AC-42.** AC-42 forbids a *destructive statement*
  (`DROP COLUMN`) against the live table; AC-40a requires *editing a DDL string and
  a column list* so a future rebuild stops naming columns the migration no longer
  creates. A database that already rebuilt under the wide revision keeps its three
  inert columns, exactly as AC-42 intends, because its rebuild no longer fires. A
  database that has not yet rebuilt never had the columns. Neither loses data, and
  no `DROP COLUMN` is emitted. The interaction is recorded here because a builder
  reading AC-42 alone could reasonably conclude the safe move is to *keep* the
  disposition columns in the rebuild — which is the one choice that breaks startup.
- **AC-41** — THE SYSTEM SHALL NOT register any route matching
  `/api/v1/github/task-prs/:associationId/disposition`, and SHALL NOT ship a
  service method, store method, permitted-value set, component, translation key,
  **or telemetry counter** whose purpose is recording a pull request disposition.
- **AC-41c** — Specifically, THE SYSTEM SHALL NOT publish the expvar map
  `github_task_pr_outcome_dispositions_total`, and SHALL NOT ship the
  `taskPROutcomeDispositionsTotal` variable or the `incTaskPROutcomeDisposition`
  helper in `metrics_vars.go`. The `github_task_pr_outcome_syncs_total` map and
  `incTaskPROutcomeSync` are RETAINED — they are what AC-38 requires — as is the
  shared `outcomeMetricLabel` helper.

  This is called out separately because the disposition counter is the one removal
  surface that hides behind a **retained** acceptance criterion. Its declaration and
  its helper both cite AC-38 in their own comments, so a builder auditing
  "everything AC-38 asks for" finds them and keeps them, while a builder auditing
  "everything the removal contract lists" never looks at `metrics_vars.go` at all:
  AC-40 enumerates DDL sites, column projections, writer column lists,
  `taskPROutcomeColumnDDL`, both halves of the rebuild, the JSON representation and
  the frontend type, and AC-41 previously enumerated routes, service and store
  methods, permitted-value sets, components and translation keys. A metrics
  variable is in neither list. The removal-pinning verification surface has the same
  blind spot — it greps the two DDL sites, the projections,
  `taskPROutcomeColumnDDL`, the rebuild copy list and the route table — so a live
  `github_task_pr_outcome_dispositions_total` would ship in the narrowed release
  with every check green, publishing a dev-mode metric for a feature that does not
  exist. AC-38's scope is the populating / non-populating sync counters and nothing
  else.
- **AC-41b** — WHEN the disposition component is deleted, THE SYSTEM SHALL also
  remove its entry from `i18nGuardFiles` in `apps/web/eslint.i18n.options.mjs`,
  in the same change. This is the one case where removing a guard entry is
  correct and the repo's "never remove an entry" rule does not apply:
  `check-guard-allowlist.mjs` flags a removal only while the entry's path still
  resolves to a file (line 90, `filesFor(entry).length > 0`), and separately
  fails an entry that matches no file (line 132). Keeping the entry for a deleted
  component therefore fails the build, and removing it passes — the script's own
  comment names "a deleted or renamed file" as the legitimate removal. Deleting
  the component without dropping the entry, or dropping the entry without
  deleting the component, are both defects.
- **AC-42** — THE SYSTEM SHALL NOT emit a `DROP COLUMN` or any other
  destructive statement against `github_task_prs` for the three removed columns.
  A developer database that booted an earlier revision of this branch retains
  three unused nullable columns; they are inert because every read uses an
  explicit column projection (AC-07) and no writer names them. No shipped
  release ever carried them — the change reached `main` only in its narrowed
  form — so no user database is affected, and a migration to remove them would
  add a destructive statement to a runner that swallows nothing and protects
  nobody here.

### Writer set and row replacement

*This section was rewritten in spec-review round 4 by human decision, after AC-43
produced a finding in four consecutive review rounds. The rewrite is described,
including what was rejected, at the end of the section. Read it before amending any
of AC-43 / AC-43a / AC-43p — the three are one rule and must not be patched apart.*

- **AC-43** — WHEN `ReplaceTaskPR` or `RestoreTaskPR` writes a `github_task_prs`
  row, THE SYSTEM SHALL determine the values of all five columns **inside that
  statement's own write transaction**, by applying the resolution rules of AC-12,
  AC-13, AC-14, AC-15 and AC-16 to (a) the outgoing row read in that same
  transaction and (b) the observation and populated-ness flags supplied by the
  caller. The resolved values are what the statement writes.

  These two are AC-43's entire subject. `UpdateTaskPR` is exempt and `CreateTaskPR`
  is vacuous; AC-07 states that split and why.

  The consequence that matters: a caller with no observation to contribute
  (populated-ness false) resolves to the outgoing row's stored values, so the write
  preserves them — and it preserves the values the row holds **at write time**, not
  the ones it held when some earlier read happened to run.

  **When there is no outgoing row**, the resolution runs against absent stored
  values, which is not an error and needs no special case: a populating observation
  writes what it observed (including latching `auto_merge_observed_at` per AC-43b),
  and a non-populating one writes `NULL` in all five, because there is nothing to
  preserve. This is `ReplaceTaskPR`'s ordinary production path — its caller reaches
  the write only after establishing that no row exists for that
  (task, repository, PR number) — so it is the common case, not the edge case, and
  an implementation that treats a missing outgoing row as a failure or a reason to
  skip the write is wrong.

- **AC-43p (provenance)** — Populated-ness SHALL ride in the call as an explicit
  flag, and SHALL NOT be inferred from a value being `nil`. AC-12 requires a
  populating sync to write `NULL` for `merged_by_login` when upstream reports no
  merger, so `nil` means either "observed absent, overwrite the stored value" or
  "not observed, preserve it" — opposite actions behind an identical value. No
  implementation SHALL try to tell them apart from the value.

  THE SYSTEM SHALL therefore have `ReplaceTaskPR` and `RestoreTaskPR` accept the
  **raw observation plus its populated-ness flags** — the same
  `OutcomeFieldsPopulated` / `ClosureAttributionPopulated` pair the sync path
  already carries — and SHALL NOT have them accept five pre-resolved values.
  Callers do not resolve; the store does, in-transaction, per AC-43.

  This is the same `Populated`-flag-and-preserve idiom the sync path already uses
  for four other field groups (`ChecksPopulated`, `UnresolvedReviewThreadsPopulated`,
  `ReviewCountsPopulated`, and `RequiredReviews`' nil-means-unknown rule), so it
  introduces no new concept — it moves an existing pure function to the one place
  that can run it without a stale read. The mock controller's E2E association
  endpoint, which builds its struct straight from a request body, is a caller of
  `ReplaceTaskPR` and is in scope: it supplies an observation with populated-ness
  false unless it is deliberately simulating a populating fetch.

- **AC-43a** — For **both** `ReplaceTaskPR` and `RestoreTaskPR`, the read of the
  outgoing row, the resolution required by AC-43, and the write SHALL execute
  inside a **single transaction**. There is no exemption for either writer, and in
  particular none for `RestoreTaskPR` on the grounds that it is a single `UPDATE`.

  Why no exemption: `RestoreTaskPR`'s `UPDATE` is protected in SQL by
  `COALESCE(auto_merge_observed_at, ?)`, which covers the latch and **only** the
  latch. `is_draft`, `changed_files`, `merged_by_login` and `closed_by_login` are
  written from values the caller computed against a row it read earlier, outside
  any transaction. A populating sync landing in that gap is overwritten. On a row
  that has just reached a terminal state this is permanent: the orphan sweep never
  re-fetches terminal rows (`taskPRNeedsUnwatchedSync`), so a `merged_by_login`
  zeroed this way is a permanent **AC-36 violation** with no later poll to repair
  it. `RestoreTaskPR` is in fact the **more** reachable of the two writers here,
  because a row demonstrably exists — that is the precondition for a relink —
  whereas `ReplaceTaskPR`'s production caller reaches it only when no row exists.

  Reading, resolving and writing in one transaction closes the window for both,
  because the row the resolution reads and the row the statement overwrites are the
  same snapshot.

- **AC-43b** — `auto_merge_observed_at` remains first-write-wins (AC-16,
  AC-17) under this rule, and falls out of it rather than needing separate
  machinery: the in-transaction resolution sets the latch only when the outgoing
  row's value is `NULL`, and that read is no longer stale. A fresh insert with no
  outgoing row still latches immediately from a populating observation, which is
  required — `ReplaceTaskPR`'s production path is the no-outgoing-row path, so a
  rule that only ever carried a value forward would write `NULL` there and never
  latch. Retaining the SQL-level `COALESCE` on `RestoreTaskPR` and `UpdateTaskPR`
  is permitted and harmless; it is redundant with, not a substitute for, AC-43a.

### Why this shape, and what was rejected

Recorded because AC-43 failed four review rounds and the next reader needs to know
which shapes are dead.

**Rejected: caller pre-resolves, store writes as supplied.** That was the round-3
contract — AC-43p required callers to pass values "already resolved by
`resolveTaskPROutcomeFields`", AC-43 required the store to write them "as supplied"
and forbade "the statement second-guessing its input", and AC-43a required the store
to read all five columns in-transaction. Those three cannot hold at once: on a
non-populating write the caller's pre-call resolution **is** the stale snapshot
AC-43a exists to forbid, and the in-transaction read becomes data the store is
contractually barred from using. The rule read as if it closed the
`merged_by_login` race while closing nothing, which is precisely how it survived
three rounds of review.

**Rejected: name the race as an accepted gap.** Recording a permanent AC-36
violation as tolerated would hollow out AC-36, which is a human-locked invariant a
downstream extract reads as a writer-fault detector. A detector with a known
permanent false-positive path is not a detector, and shipping one would reproduce
the Ops Cost overclaim this feature exists to prevent.

**Chosen: one place owns provenance and the race.** The store method that writes
the row is the only code that can read the row and write it without a gap, so it
owns both. This also removes machinery rather than adding it: the per-column split,
the "written as supplied / no second-guessing" rule, and the `RestoreTaskPR`
exemption all disappear, and `resolveTaskPROutcomeFields` — already a pure function
of (stored five values, observation) — simply runs one layer down.

Rationale for having a positive contract here at all: AC-17 forbids clearing the
latch, and these two writers can violate it, and AC-36, without any `UPDATE` ever
looking wrong. In `ReplaceTaskPR` the value is lost with the deleted row rather
than overwritten in place; in `RestoreTaskPR` it is overwritten by a value that was
correct when it was read. Neither defect is visible on inspection of the statement.

## Ordering, idempotency, concurrency, and boundaries

**Ordering.** This feature introduces exactly ONE new ordered collection: the
`timelineItems(itemTypes: CLOSED_EVENT)` node list on the GraphQL PR selection,
whose ordering rule — latest closure wins — is fixed by AC-08a. It adds no new
list endpoint and changes the ordering of no existing one. Where succession
between a task's pull requests is derived downstream (see Out of scope), the
ordering is `github_task_prs` rows filtered by `task_id`, ordered by
`created_at ASC`, tiebroken by `pr_number ASC`, then by `id ASC`. `created_at`
alone is not unique — two PRs opened in the same second by a multi-repo launch
collide — and `id` is a random UUID, so it is the final tiebreak only, never the
primary key of the ordering.

**Idempotency.** Schema initialization is replay-safe (AC-02) and the activation
instant is written at most once per database (AC-06). A sync that observes
unchanged values performs its `UPDATE` but publishes no event (AC-18), matching
today's `SyncTaskPR` behaviour. Re-running a populating sync with an identical
upstream response leaves the row byte-identical, including
`auto_merge_observed_at`, which is written only on the `NULL` → non-`NULL`
transition (AC-16, AC-17).

**Concurrency.** No read-modify-write on these columns spans a request boundary,
and no user-facing mutation writes them. Four statements write these columns from
a caller value (AC-07), all server-side; of those, `ReplaceTaskPR` and
`RestoreTaskPR` are bound by AC-43 / AC-43a / AC-43p, `UpdateTaskPR` is exempt as
the sync writer's own statement, and `CreateTaskPR` has no production caller. A
fifth copies them column-to-column during the schema rebuild (AC-07a), which runs
at startup before any sync is scheduled and therefore races nothing.

Two concurrent populating syncs for the same pull request are serialized by the
row `UPDATE` and are last-write-wins on the four directly observed columns;
because both report the same upstream state, the race produces the same values.

Where two polls legitimately observe *different* upstream states and land out of
order, the later statement to execute wins. **On a row that is still open that is
self-correcting; on a row that has just become terminal it is not, and the
difference is load-bearing.** `resolveTerminalMergeState` pins `state`,
`merged_at` and `closed_at` for a row that already reached `merged`, so a stale
poll cannot un-merge the row — but it does not pin `merged_by_login`, and AC-12
requires a populating sync that observed no merger to write `NULL` there. A poll
that read the pull request before the merge, landing after the poll that read it
after, therefore writes `merged_by_login = NULL` onto a row whose `merged_at`
post-dates activation. Terminal rows are excluded from the orphan sweep
(`taskPRNeedsUnwatchedSync`), so there is no next poll: the value is gone
permanently and AC-36 reads it as a writer fault.

This is stated rather than left implicit because an earlier revision of this
section claimed the opposite — "the next poll reconciles: these four columns carry
no cross-poll invariant, so a transiently stale value is self-correcting" — which
contradicts the Persistence guarantees section ("a value absent at the moment a PR
reached its terminal state is absent forever") and would license an implementation
that treats the ordering as harmless.

**Scope of this residual, and why it is accepted here but NOT accepted for the
AC-43 writers.** The two cases look alike — both end in a permanent `NULL`
`merged_by_login` on a terminal row — and the spec deliberately resolves them
differently. The asymmetry is the cost of the fix, and it is recorded so the next
reader does not "harmonise" them:

- **AC-43 writers (`ReplaceTaskPR`, `RestoreTaskPR`) — closed, not accepted.** The
  fix is to read, resolve and write in one transaction: local, inside one store
  method, no new concept, and it removes contract machinery rather than adding it.
  A gap that cheap to close is not a gap worth documenting. AC-43a closes it, and
  the AC-43 section records why naming it instead was rejected.
- **The ordinary poll path (`UpdateTaskPR`) — accepted, and named.** Closing this
  one needs something the feature does not have: a cross-poll ordering guarantee,
  i.e. a fetched-at watermark carried on the observation and compared on write, so
  a poll that read older upstream state loses. That is a new mechanism on the
  hottest write path, for a window bounded by one poll interval on one pull request
  as it merges. Out of proportion.

So this is a **named limit of AC-36**, listed with AC-36's other legitimate `NULL`
class in the Writer health section, and carried in the Failure modes table. What it
is not, and must never become, is a licence to leave the AC-43 writers unfixed:
those are the never-self-correcting losses that a single transaction removes
entirely. AC-39 already tells consumers to treat rows near the activation boundary
as indeterminate; this is the same class of caveat, at the merge boundary instead.

`auto_merge_observed_at` is the exception, and it is **first-write-wins, decided
in SQL rather than in Go**. The Go-side check — `resolveTaskPROutcomeFields`
seeing a stored `NULL` — reads a snapshot that can already be stale when the
statement executes, so two syncs can both compute a candidate instant. The write
MUST therefore be expressed as
`auto_merge_observed_at = COALESCE(auto_merge_observed_at, ?)`: one statement
that evaluates the row's current value at execution time, so the first write to
land wins atomically and every later candidate is discarded rather than
overwriting it. A plain `SET` satisfies the Go-side check and still violates
AC-17 under exactly this race, which is why the mechanism is named here and not
left to the implementer. The latch is never cleared, so no race can return it to
`NULL`, and both racing candidates satisfy the column's contract ("auto-merge was
armed at some instant while we were watching") to within one poll interval.

For `ReplaceTaskPR` and `RestoreTaskPR` the protection is different in kind, not
just in scope: those two resolve **all five** columns against the outgoing row
inside their own write transaction (AC-43, AC-43a, AC-43p), so the latch needs no
special case there — the resolution simply never sees a stale `NULL`. `COALESCE`
on `RestoreTaskPR` is retained as belt-and-braces and is redundant with that rule,
not a substitute for it. The transaction is what closes the `merged_by_login`
race, which is the one with the worse, permanent outcome (AC-36).

**Nil, empty, and error.** Upstream `merged_by: null` writes `NULL`, never `''`.
An empty-string `merged_by` login is treated as absent and also writes `NULL`,
never `''`: on a populating sync `merged_by_login` is always rewritten, so
"absent" and "not merged" are the same outcome for that column.

Upstream `auto_merge: null` leaves the latch untouched, never clears it.

`closed_by_login` does **not** follow the `merged_by_login` rule, and the
difference is deliberate. An absent closed-event actor leaves `closed_by_login`
untouched; a closed-event actor with an empty login leaves it untouched **too**,
because AC-14 defines an empty login as not an observation at all, so closure
attribution is never marked populated and AC-15's leave-unchanged rule governs.
Writing `NULL` there instead would erase a real closer already recorded by an
earlier sync — the exact class of loss this feature exists to prevent. `''` is
never persisted in either column; the two columns differ only in whether an
absent value **overwrites** (`merged_by_login`: yes) or **preserves**
(`closed_by_login`: no).

`changed_files = 0` is a real observation and is written as `0`, distinct from
`NULL`.

A response that OMITS `isDraft` or `changedFiles`, or returns either as `null`,
is not an observation of `false` or `0`; AC-12a governs it and requires `NULL`.
This is the one place where a language-level decoder default silently destroys
the distinction the whole feature exists to preserve.

A failed sync-side write is governed by **AC-18c**: the error propagates to the
sync's caller, nothing is published, the row is left untouched, and the next
ordinary poll is the retry. An earlier revision of this paragraph described that
path as following "the surrounding best-effort logging convention", which was
wrong and is corrected in AC-18c — the write is not best-effort. The converse
case, where the write succeeds and the publish then fails, is the one that IS
best-effort, and is governed by AC-18a.

**Defaults and boundaries.** Every column defaults to `NULL` at the SQL level
(AC-01) — no `DEFAULT 0`, no `DEFAULT ''`, no `DEFAULT false`. `changed_files`
is whatever upstream reports, including `0`; the spec sets no upper bound and
performs no clamping. A negative `changed_files` is not a value GitHub produces;
if one is ever observed it is stored as reported rather than clamped or dropped,
because normalizing an impossible value would hide the upstream or decoding
fault that produced it — and an absent field is `NULL` under AC-12a, not a
negative sentinel. `is_draft` is a three-state field at the storage layer
(`NULL` / `true` / `false`) and must be read as such.

`changed_files` is **not** three-state. Its storage layer admits `NULL`, `0`, a
positive count, and — only if upstream ever produces one — a negative value that
the no-clamping rule above requires be stored verbatim. A reader must therefore
branch on `NULL` versus not-`NULL` and treat the non-`NULL` value as an integer
it did not choose, rather than assuming "not `NULL` and not `0`" means positive.
A stored negative is a signal that upstream or the decoder is faulty; it is not
in this feature's contract as a meaningful file count, and no consumer should
aggregate it as one.

## Persistence guarantees

- No column in this feature is ever populated by a backfill, an inference, or a
  heuristic. Every non-`NULL` value is a direct upstream observation.
- The activation instant is durable in `kandev_meta`, which survives the database
  reset path and is therefore readable by any point-in-time snapshot without
  schema versioning. Any consumer that reports on these columns must read it and
  scope its window to rows at or after it.
- Terminal rows are never re-fetched by the orphan sweep, so a value absent at
  the moment a PR reached its terminal state is absent forever. This is
  deliberate: it keeps the columns a record of what was observed, not a
  best-effort reconstruction.

## Failure modes

| Failure | Behaviour |
|---|---|
| `ADD COLUMN` fails for a non-duplicate reason | Startup aborts with the error (AC-03) |
| `kandev_meta` write fails during activation | Startup aborts with the error; not swallowed, not logged-and-continued. A database with the columns but no activation instant is unreportable and must not be shipped (AC-05a) |
| Two processes initialize the same database concurrently at first activation | Set-if-absent write means exactly one instant is stored; the loser is discarded, not overwritten (AC-05a, AC-06) |
| GraphQL returns no closed-event node | `closed_by_login` untouched (AC-15) |
| GraphQL returns a closed-event node with a null actor (deleted account) | Closure attribution not marked populated; `closed_by_login` untouched (AC-14, AC-15) |
| Sync path is the noop client | Nothing marked populated; no column written (AC-11, AC-13) |
| Upstream reports auto-merge disarmed after it was observed armed | Latch retained unchanged (AC-17) |
| Populating response omits `isDraft` or `changedFiles`, or returns either as `null` | `NULL` written for that column, not `false` / `0` (AC-12a) |
| Row `UPDATE` succeeds, then the `github.task_pr.updated` publish fails | Logged and dropped; no retry, no rollback. Delivery is at-most-once and the row is the source of truth (AC-18a) |
| Two syncs race the auto-merge latch | First write wins via SQL `COALESCE`; the loser's instant is discarded and may still publish a no-op event (AC-16, AC-17, AC-18b) |
| `ReplaceTaskPR` / `RestoreTaskPR` runs from a non-populating source | In-transaction resolution against the outgoing row yields the stored values, so all five are preserved (AC-43) |
| `ReplaceTaskPR` or `RestoreTaskPR` races a populating sync over the latch | The resolution reads the row inside the write transaction, so it never sees a stale `NULL` and never clears the latch (AC-43a, AC-17) |
| `RestoreTaskPR` races a populating sync that just wrote `merged_at` + `merged_by_login` | Closed by AC-43a's transaction. This is the MORE reachable of the two writers — a row must exist for a relink — and `COALESCE` does not help, because it protects only the latch (AC-43a, AC-36) |
| `ReplaceTaskPR` inserts a FRESH row for a PR that already has auto-merge armed | No outgoing row, so the in-transaction resolution latches the observed instant immediately. This is `ReplaceTaskPR`'s production path, and a rule that only carried values forward would write `NULL` and never latch (AC-16, AC-43) |
| A caller passes pre-resolved values instead of an observation plus populated-ness flags | Defect under AC-43p. Callers do not resolve; the store resolves in-transaction. Populated-ness never inferred from `nil`, because `nil` is also a legitimate populating observation of "no merger" (AC-12) |
| Two polls of one PR straddle its merge and land out of order | Later-executing poll wins, so `merged_by_login` can be left `NULL` on a terminal row with no later poll to repair it. ACCEPTED and named as an AC-36 limit; closing it would need a cross-poll watermark (Concurrency, Writer health note) |
| Sync-side row `UPDATE` fails | Error returns to the caller; nothing published, row untouched, next ordinary poll is the retry. The AC-38 counter has already been incremented and is not rolled back (AC-18c) |
| Pre-multi-repo database boots the narrowed release | Rebuild fires, and its DDL and copy list name the five outcome columns and no disposition column, so it succeeds. Leaving `disposition` in the copy list aborts startup with `no such column` (AC-07a, AC-40a) |
| Rebuild copy list silently omits an outcome column | Every row loses that column at once, including a latched `auto_merge_observed_at`, with no `UPDATE` involved — the table-scale form of the AC-43 loss (AC-07a) |
| Third of five `ADD COLUMN` statements fails | First two stay applied, startup aborts, next boot adds only what is missing; no activation instant is stamped while any column is absent (AC-03, AC-05) |
| Row `UPDATE` succeeds, publish payload read-back fails | Logged; the in-memory row is published instead. The change decision is unaffected because it never used the read-back (AC-18) |
| Closed-event actor present but its login is `''` | `closed_by_login` left unchanged, NOT set to `NULL` — an empty login is not an observation (AC-14, AC-15) |
| Pre-existing OPEN row syncs after activation | Populates normally; only rows terminal at activation stay `NULL` permanently (AC-04, AC-37) |
| PR closed, reopened, and closed again | Latest closure wins; `closed_by_login` may be non-`NULL` on an open row (AC-08a, AC-15a) |
| Disposition expvar counter left in `metrics_vars.go` | Defect under AC-41c. It cites retained AC-38 in its own comment and sits outside every other removal enumeration and every removal-pinning grep, so it ships green (AC-41, AC-41c) |

## Permissions

This feature adds no endpoint, no mutation, and no new capability. The five
columns ride on the existing workspace-scoped task-PR read paths and are visible
to exactly the callers who can already read the association.

## Out of scope

Each exclusion below is a contract, not an oversight.

- **A Kandev-recorded closure reason, in any form.** No `disposition` column, no
  enum of reasons, no `superseded_by_url`, no recorded-at timestamp, no endpoint,
  and no UI (AC-40, AC-41). Decided by the reviewer of PR #2614 on 2026-08-15:
  the reason taxonomy is a workflow opinion, not a GitHub fact, and core stores
  GitHub facts. A plugin owns the taxonomy, the replacement links, its own
  storage, and its own UI. Core does not reserve those column names, does not
  provide a hook for them, and does not persist them on behalf of a plugin.
  Reintroducing any of this into core is a new contract requiring a new spec,
  not an implementation detail of this one.
- **Inferring supersession between a task's pull requests.** `task_id` plus
  `created_at` ordering is the only succession signal that exists, and it is
  implicit. It is fully derivable downstream from columns the extract already has
  (`task_id`, `created_at`, `pr_number`, `id`) using the ordering stated above,
  so it needs no column and no writer here. No derived succession field is added
  to the row: an inference persisted next to recorded facts is exactly how a
  column's meaning changes without anyone deciding to change it.
- **Backfilling any historical row.** Explicitly forbidden by AC-04. No row that
  predates activation acquires a value, and none is labelled `abandoned` — a word
  this feature does not define and does not use.
- **Dropping the three columns from developer databases that ran an earlier
  revision of this branch.** AC-42.
- **`closed_by_login` on the REST and gh CLI paths.** Would cost a second REST
  call per closed PR. The GraphQL path is the production path; the gap is named
  in AC-15 rather than closed.
- **`comment_count`.** Genuinely frozen (`service_pr_watch.go`) and a real
  defect, but a different one.
- **`review_count`.** Not a defect; see Confirmed non-defects.
- **Any UI for the five columns.** AC-30b. They are a data-layer and API-layer
  addition consumed by an extract or a plugin, not by a core screen.
- **An MCP tool exposing these fields.** The existing task-PR JSON is the read
  surface.
- **Postgres support for `internal/github`.** This store is not Postgres-capable
  today, independently of this feature: its DDL uses `DATETIME` column types and
  its column prechecks use `PRAGMA table_info`, neither of which Postgres accepts.
  This feature does not fix that and does not make it worse: per ADR 0027 it uses
  the shared `db.IsDuplicateColumnError` classifier rather than adding a new
  dialect-specific one (AC-03), so the migration is portable the day the store is.
  A Postgres replay test is therefore not required for this change.
- **Any change to mergeability, auto-merge, or CI auto-fix behaviour.** AC-19.

## Verification surfaces

- **Backend unit** — `internal/github`: migration applied to a database seeded
  with a pre-migration row, `initSchema` invoked twice, asserting the columns
  exist, the seeded row is still `NULL` in all five, and the activation key is
  written once and not advanced by the second run. Modelled on
  `internal/task/repository/sqlite/task_external_id_migration_test.go`. The
  all-`NULL` assertion here is scoped to **schema initialization with no sync in
  between** — it proves the migration backfills nothing (AC-04). It must NOT be
  extended into "an open pre-existing row stays `NULL` after a populating sync",
  which AC-04 explicitly does not claim and AC-37 forbids.
- **Backend unit** — sync writer: populated and unpopulated paths, the
  `auto_merge_observed_at` latch including the disarm case, the `NULL` vs `0`
  distinction for `changed_files`, and event publication on change only.
- **Backend unit** — column-list drift: all five columns present in
  `taskPRColumns`, `taskPRColumnsQualified`, the column lists of **all four**
  row-value writers AC-07 names — `CreateTaskPR`, `ReplaceTaskPR`,
  `RestoreTaskPR` and `UpdateTaskPR` — **and both halves of the rebuild statement
  AC-07a names**: the `CREATE TABLE github_task_prs_new` DDL and the
  `INSERT … SELECT` copy list, on both sides of the `SELECT`. `RestoreTaskPR` is
  listed explicitly because it takes its five values as separate parameters rather
  than off a `TaskPR` struct, so dropping one is a silent omission no struct-shaped
  check catches. The rebuild is listed explicitly because it is not a writer anyone
  thinks of as a writer, and a check scoped to "the four writers" — which is what
  this surface said before round 3 — structurally cannot see it.

- **Backend unit** — legacy-constraint rebuild replay (AC-07a, AC-40a): seed a
  database whose `github_task_prs` carries the legacy `UNIQUE(task_id, pr_number)`
  constraint and a row with non-`NULL` values in all five outcome columns, run
  schema initialization, and assert that (a) startup does not error, (b) the
  rebuild actually fired, (c) all five values survived the copy, and (d) the
  resulting table has no `disposition` column. Run initialization **twice** and
  assert idempotence, per the `task_external_id_migration_test.go` precedent.
  This surface is the one that would have caught the round-3 defect: without a
  seeded legacy constraint the rebuild no-ops, so every other test in this suite
  passes while the narrowed release aborts on a real user database.
- **Backend unit** — explicit-null JSON serialization (AC-30): a `github_task_prs`
  row with all five columns `NULL`, serialized both through the task-PR endpoint
  and through the `github.task_pr.updated` payload, has all five keys PRESENT
  with the value `null`. This surface exists because every other check in this
  list — including frontend typecheck, whose fields are `field?: T | null` —
  passes unchanged if someone adds `omitempty` to the DTO fields and the keys
  start disappearing, which destroys the "not observed" signal silently.
- **Backend unit** — writer preservation, both writers (AC-43): over a row holding
  non-`NULL` values in **all five** columns, call `ReplaceTaskPR` and separately
  `RestoreTaskPR` with an observation whose populated-ness flags are **false**;
  all five values survive in the resulting row. Assert all five, not the latch
  alone — a latch-only implementation passes a latch-only test while silently
  zeroing `is_draft`, `changed_files`, `merged_by_login` and `closed_by_login`.
- **Backend unit** — in-transaction resolution, both writers (AC-43a): the
  `ReplaceTaskPR` and `RestoreTaskPR` paths each read the outgoing row, resolve the
  five columns, and write, **inside one transaction**. Assert the transaction
  boundary, not only the happy-path value: the defect this AC prevents is invisible
  in a serial test, because a resolution run before the transaction returns the
  right answer every time until a concurrent sync interleaves. `RestoreTaskPR` is
  the one to write first — it is the more reachable of the two (a row must exist for
  a relink) and it is the one round 3 exempted, so a suite that covers only
  `ReplaceTaskPR` reproduces exactly the gap this round closed.
- **Backend unit** — fresh-row latching through `ReplaceTaskPR` (AC-16, AC-43):
  associating a task with a PR that already has auto-merge armed, where **no row
  exists yet**, writes a non-`NULL` `auto_merge_observed_at` on the inserted row.
  This is `ReplaceTaskPR`'s production path — its caller reaches the write only when
  no row exists — and it is the direction the preservation surface above cannot
  cover: with no outgoing row, a rule that only ever carried values forward writes
  `NULL` and the latch is never set.
- **Backend unit** — caller provenance (AC-43p): no production caller of
  `ReplaceTaskPR` or `RestoreTaskPR` pre-resolves the five values; each passes the
  observation plus its populated-ness flags, and the store performs the resolution.
  Assert this at the call sites and in the method signatures rather than by
  inspecting values, because the whole point of AC-43p is that the values themselves
  cannot distinguish "observed absent" from "not observed". The mock controller's
  E2E association endpoint is a caller and is in scope.
- **Backend unit** — missing-field handling (AC-12a): a populating response with
  `isDraft` / `changedFiles` absent or `null` writes `NULL`, distinguishable from
  a response that genuinely reports `false` / `0`.
- **Backend unit** — closure attribution (AC-08a, AC-14, AC-15a): a
  close → reopen → close timeline attributes the LATEST closure; a null-actor
  closed event leaves `closed_by_login` untouched; a reopened row retains a
  non-`NULL` `closed_by_login` while `state` is open.
- **Backend unit** — activation write failure (AC-05a): a `kandev_meta` write that
  fails during activation makes schema initialization return the error, and startup
  aborts; assert the error propagates rather than that a log line was emitted. Plus
  a set-if-absent assertion: a second activation against a database that already
  holds the key leaves the stored instant unchanged (AC-06).
- **Backend unit** — upstream field requests (AC-09): the gh CLI single-PR `--json`
  field list contains `changedFiles`, `mergedBy` and `autoMergeRequest`; the REST
  single-PR path requests `changed_files`, `merged_by` and `auto_merge`; and
  **neither** requests `closedBy`, which neither exposes. Assert the negative too —
  adding `closedBy` to either list is a silent no-op that looks like coverage.
- **Backend unit** — publish semantics (AC-18a, AC-18b, AC-18c): a publish failure
  after a successful write is logged and dropped, with no retry, no rollback, and no
  republication from a later unchanged sync; a sync that loses the latch race but is
  otherwise treated as changed MAY publish, and its payload carries the stored
  winning value; and a failed row write returns its error, publishes nothing, and
  leaves the row untouched.
- **Backend unit** — `mergeable_state` regression guard (AC-19): a draft PR still
  derives `mergeable_state = 'draft'` exactly as before this feature, and no
  mergeability, auto-merge or CI-automation behaviour changes. This is the surface
  that catches `is_draft` being wired into a decision instead of just being stored.
- **Backend unit** — writer-health invariants (AC-36, AC-37, AC-39): over a seeded
  database with a known activation instant, assert that a row merged after
  activation by a populating sync has non-`NULL` `merged_by_login`; that a row
  synced after activation by a populating sync has non-`NULL` `is_draft`; and that
  rows whose `merged_at` / `closed_at` predate activation are NOT asserted on at
  all. Assert the three legitimate `NULL` classes named in the Writer health note
  as exemptions rather than failures — a test that asserts either invariant
  unconditionally fails against a correct implementation. These are the invariants
  a downstream extract checks, and nothing in this suite exercised them before.
- **Backend unit** — outcome counters (AC-38): exactly one counter increments per
  sync of one pull request, keyed by populated-ness, and the increment happens at
  the decision point — specifically, **a sync whose row write then fails is still
  counted** (AC-18c). Assert the failing-write case explicitly: a counter that only
  advances after a successful write goes quiet during the storage failure it exists
  to expose, and every happy-path test still passes.
- **Backend unit** — removal pinning (AC-40, AC-41, AC-41c): **both**
  `github_task_prs` DDL sites — the `createTablesSQL` definition and the rebuild's
  `CREATE TABLE github_task_prs_new` — plus every column projection,
  `taskPROutcomeColumnDDL`, and the rebuild's `INSERT … SELECT` copy list contain
  no `disposition` substring; the registered route table contains no `disposition`
  path; and **`metrics_vars.go` publishes no disposition expvar map and ships
  neither `taskPROutcomeDispositionsTotal` nor `incTaskPROutcomeDisposition`**,
  while retaining `github_task_pr_outcome_syncs_total` and `incTaskPROutcomeSync`.
  This is the check that keeps the narrowing from silently regressing. It names the
  DDL sites in the plural deliberately: a check written against "the DDL" finds the
  first one, passes, and leaves the rebuild — the site that actually aborts startup
  — unexamined. It names `metrics_vars.go` for the same reason: that file is in no
  other removal enumeration, and its disposition counter cites retained AC-38 in its
  own comment, so nothing else in this suite would ever look at it.
- **Repo check** — guard-allowlist consistency (AC-41b): after the disposition
  component is deleted, `i18nGuardFiles` in `apps/web/eslint.i18n.options.mjs`
  contains no entry for it, and `check-guard-allowlist.mjs` passes. Both halves are
  the assertion: keeping the entry for a deleted file fails the script's
  matches-no-file check, and removing an entry whose path still resolves fails its
  removal check, so component deletion and entry removal must land together. This
  is verified by running the existing script, not by a new bespoke test.
- **Backend unit** — no destructive migration (AC-42): the migration surface emits
  no `DROP COLUMN` and no other destructive statement against `github_task_prs`,
  and a database that booted the earlier eight-column revision still holds its three
  inert columns after initialization. Assert the absence of the statement, not just
  that the data survived — a migration that drops and recreates would pass a
  data-survival check on an empty table.
- **Frontend unit** — none required. `TaskPR` gains five typed fields and no
  component consumes them (AC-30b); typecheck is the verification.
- **E2E — not required.** This narrowed feature has no user-visible surface
  (AC-30b). The two disposition specs written against the removed UI
  (`pr-disposition.spec.ts`, `mobile-pr-disposition.spec.ts`) are deleted along
  with the control they exercised; no replacement spec is added, because there is
  nothing on screen to assert.
