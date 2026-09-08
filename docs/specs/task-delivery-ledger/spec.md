---
status: draft
created: 2026-08-12
updated: 2026-08-13
revision: 5
owner: nova28
decisions:
  - ../../decisions/0027-replayable-schema-migrations.md
  - ../../decisions/2026-08-02-class-aware-git-subprocess-admission.md
---

# Task delivery ledger

## Why

A task that never opened a pull request is currently indistinguishable from a task
that produced nothing. In the reference store that is the majority of the board:
of 75 observed `(task, repository)` pairs, only 16 carry a merged pull or merge
request. The remaining 59 are one undifferentiated bucket mixing research cards,
chores, work delivered by committing straight to the default branch, and genuine
failure.

Kandev already holds enough to split most of that bucket. It does not, and cannot
today, answer the one question that would close it: **did this card's work ever
reach the repository's default branch?** No merge-into-default detection exists
anywhere. `patRepository.DefaultBranch` (`apps/backend/internal/github/pat_client.go:1017`)
is parsed for clone metadata and, at `:1160`, copied onto `pr.BaseDefaultBranch`;
neither use infers a merge. The push webhook — `processPush` in
`apps/backend/internal/github/webhook_service.go` — publishes branch and SHA but
is consumed only by the automation engine
(`apps/backend/internal/automation/github_webhook_subscriber.go`, its
`GitHubPushReceived` subscription), never for merge inference.

Separately, Kandev's own success metric is wrong in a way that lands in the same
analysis. `FinishRun` writes `status='finished'` on six terminal call sites,
**four** of which did no work, and `agent_summary.go` counts every `finished` row as
a success. Only two of the six — the agent-completed subscribers — follow an agent
actually running. The four scheduler guards record the reason that work did not
run, as spelled out under **Office run outcome**.

This spec defines a **delivery ledger** that records how, and whether, each
`(task, repository)` pair delivered; and a **run outcome** that stops Office from
reporting non-work as success.

### Citation convention

Every `path:line` in this document is a convenience pointer, not the contract.
Where a line number is paired with a symbol name, a function name, or a semantic
label, **the label is authoritative and the line number is not**. If the two
disagree at build time, follow the label and correct the line. This matters most
for the `FinishRun` call-site table under **Office run outcome**, where each site
carries the outcome it must record: a drifted line number must never be allowed
to mis-map an outcome.

## Evidence the classification already works

Every number below was measured against the reference store
(`~/.kandev/data/kandev.db`, snapshotted 2026-08-12) before any schema was
proposed, as the card required. They are the input inventory, not estimates.

| Observation | Value |
|---|---|
| `task_session_git_snapshots` rows | 1,655 |
| ... with `ahead > 0` | 1,390 |
| ... with empty `head_commit` | 0 |
| Sessions with `ahead = 0` and one distinct `head_commit` (produced nothing) | 31 |
| Sessions with `ahead > 0` and a moving `head_commit` (authored commits) | 48 |
| Sessions with `ahead > 0` and a static `head_commit` (inherited a branch) | 8 |
| Sessions with `ahead = 0` and a moving `head_commit` | 0 |
| Sessions working directly on their repository's default branch | 1 (3 distinct heads, `ahead` 2) |

Applying the precedence in **Classification** to that store, with no Kandev
change, classifies **38 of 75** `(task, repository)` pairs:

| Outcome | Pairs |
|---|---|
| `pr_merge` | 16 |
| `direct_commit` | 1 |
| `no_delivery_observed` | 21 |
| `unknown` | 37 |

Three findings from that pass change the design and are therefore normative
context, not commentary:

1. **`ahead` is base-branch-relative, not upstream-relative**
   (`apps/backend/internal/agent/runtime/lifecycle/event_types.go`, the `Ahead`
   field). It counts commits the branch has that its base does not, and it does
   not fall to zero when the branch is pushed. It is therefore a valid "this
   branch carries work" signal and an invalid "this session authored work"
   signal — the 8 sessions above with `ahead > 0` and a static head inherited
   someone else's branch. **This is why the ancestry precondition in
   **Default-branch observation** keys off a moving `head_commit` and not off
   `ahead`.** Note the grain: every row in the table above counts **sessions**, not
   pairs. Head movement is only evidence of authorship *within one session*, so every
   predicate derived from this finding is evaluated per session and never by pooling a
   pair's sessions together — see **Classification**, "Head movement is a per-session
   fact".
2. **Merge cannot be detected by commit ancestry in this repository.** Kandev
   squash-merges by policy, so a merged branch head never becomes an ancestor of
   the default branch. Receipt: PR #2514's branch head
   `4dfa4d545 "fix(web): key mobile theme toggle off resolvedTheme"` is present
   in the object store and is **not** an ancestor of `origin/main`; the commit
   that actually landed is the squash
   `15524de62 "fix(web): show dark mode toggle in mobile menu (#2514)"`. Ancestry
   is a valid positive signal and an invalid negative one.
3. **Pull requests are not a GitHub-only concept here.** Three provider tables
   exist — `github_task_prs`, `gitlab_task_mrs`, `azure_devops_task_prs` — and
   the reference store's GitLab table holds 4 merged merge requests on a
   repository with no GitHub provider at all. A GitHub-only reading of
   `pr_merge` undercounts by those 4. The three tables do **not** share a schema,
   which the **Provider predicates** table below resolves column by column:
   `azure_devops_task_prs` has no `merged_at` and no `detached_at`;
   `gitlab_task_mrs` has `merged_at` but no `detached_at`.

A fourth finding corrects the card's premise about the Office metric: only
**three** of the `FinishRun` paths in `scheduler_integration.go` write an
`office_activity_log` row — `run_idle_skipped`, `run_budget_blocked` and
`run_processed`. The agent-inactive, task-tree-hold, checkout-error and
checkout-unavailable paths log nothing, and the two `event_subscribers.go`
completion paths log a run event rather than an activity row. The activity log
therefore cannot carry this on its own.

## What

- Kandev maintains a **delivery ledger**: at most one row per
  `(task, repository)`, recording a delivery outcome, the evidence that produced
  it, and — separately — whether the pair's work was ever observed reaching the
  repository's default branch.
- The ledger is a new table. It is **not** a pull-request state and does not
  extend `github_task_prs`, `gitlab_task_mrs`, or `azure_devops_task_prs`.
- `delivery_outcome` takes exactly one of
  `pr_merge | direct_commit | no_delivery_observed | unknown`, or `NULL` when the
  pair has never been evaluated.
- Reaching the default branch is recorded as its own observation, because the
  route by which work reached the default branch is frequently unknowable while
  the fact that it arrived is not.
- An Office run records a structured **outcome** alongside its terminal status,
  so a budget-blocked, idle-skipped, agent-inactive, tree-held, checkout-blocked or
  never-launched run is no longer counted as a success.
- Every column added by this feature is nullable, legacy rows are `NULL` and
  never `0` or `''`, and the instant each mechanism became authoritative is
  published in the database so a point-in-time extract can tell "not observed"
  from "observed as nothing".

## Delivery slices

This spec defines two contracts that share no build-order dependency. They are
specified together because they are the same defect at two grains, and they are
**built in this order**, each closing on its own:

- **Slice A — Office run outcome.** One nullable column, six call sites, **one new
  field on `models.Run`**, one reshaped repository query and the two dashboard response
  shapes it feeds. Depends on nothing in Slice B.

  **`models.Run` is not optional.** Ten production queries read `SELECT * FROM runs`
  and `StructScan` into `office/models.Run` (eight in
  `internal/runs/repository/sqlite/runs.go`, plus `office/repository/sqlite/failure.go`
  and `run_routing.go`), and no `sqlx` handle in this backend is `Unsafe()`. A column
  that exists in the table with no matching `db:` tag on the struct makes every one of
  those queries fail at runtime, which would take out the whole Office run pipeline.
  `models.Run` therefore gains `Outcome *string` with `json:"outcome,omitempty"` and
  `db:"outcome"`, pointer-typed for the same reason the provider-routing columns on that
  struct are pointer-typed — so `StructScan` reads the `NULL` that every pre-activation
  row carries.

  Adding that field does **not**, by itself, change any response body. `models.Run` is
  never serialized to a client: every run surface flattens it through an explicit DTO with
  its own field list. **No run-grain DTO gains a field in this card** — specifically
  `RunListItem` (via `runToListItem`), `AgentRunSummaryDTO` (via `buildRunSummaryDTO`) and
  `RunRuntimeDTO` (via `buildRuntimeDTO`), all in `office/dashboard/`, are all unchanged.
  So no per-run response body changes and the "Any user-visible surface" exclusion in
  `## Out of scope` stands as written.

  **Two day-grain dashboard shapes DO gain fields, and that is in scope.** `AgentRunActivityDay`
  gains `skipped` and `unclassified`, and `AgentSuccessRateDay` gains `unclassified`; the
  contract is specified under `## Office run outcome` § Response shapes and summarized in
  `## API surface`. All three are additive JSON fields that no frontend renders yet. The
  distinction is run-grain versus day-grain, not "nothing changes" — an earlier revision of
  this document said flatly "no DTO gains a field", which contradicted the Response shapes
  section and could have been read as licence to ship the reshaped query with no wire fields
  at all.

  **Additive on the wire is not the same as unchanged on screen.** Both shapes already feed
  rendered charts on the agent dashboard, and this card changes the *values* they carry: the
  Success Rate chart in particular reports 0% for every pre-activation day once `succeeded`
  narrows to `outcome = 'processed'`. No `apps/web/` file changes and no new copy is
  introduced, so the E2E path allowlist is unaffected — but nobody should read "no DTO field
  is removed" as "no operator sees a difference". The new `unclassified` field is what makes
  that difference explainable in-band; see § Response shapes.
- **Slice B — the delivery ledger.** New table, evaluator, sweep, upsert guard,
  metrics, ancestry, two-dialect migrations.

Sequencing is a plan-level decision recorded here so it survives context resets;
it changes no acceptance criterion. Both slices are in scope for this card.

## Data model

### `task_delivery_ledger` (new)

One row per `(task_id, repository_id)`. Every column that carries **evidence** — the
classification triple, the default-branch observation triple, `observed_branch_commits` and
`first_classified_at` — is nullable, so "not observed" is always representable and is never
spelled as `0` or `''`. The columns that are `NOT NULL` are exactly: the identity columns
(`id`, `task_id`, `repository_id`), the denormalized scope column `workspace_id`, the
bookkeeping columns (`last_evaluated_at`, `evaluation_seq`, `created_at`, `updated_at`), and
`evidence_rank`, whose non-nullability is a correctness requirement argued in its own row
below. The table is authoritative; this sentence enumerates rather than generalizes,
because an earlier revision said "all columns other than the identity, sequence and
timestamp columns are nullable" and its own table contradicted it two rows later.

| Column | Type | Meaning |
|---|---|---|
| `id` | TEXT PK | Row identity. |
| `task_id` | TEXT NOT NULL | FK to `tasks(id)` `ON DELETE CASCADE`. |
| `repository_id` | TEXT NOT NULL | FK to `repositories(id)` `ON DELETE CASCADE`. Never empty. The `ON DELETE` clause is stated rather than left to the implementation, for the same reason it is stated on `task_id`: `repositories` itself declares `FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE` and `DELETE FROM workspaces` is a live production path, so repository rows **are** removed in ordinary operation even though nothing hard-deletes them directly. A bare FK (dialect default `NO ACTION`) would make workspace deletion begin failing the moment any ledger row referenced one of that workspace's repositories, which is a regression this feature would introduce into an existing user-facing operation. |
| `workspace_id` | TEXT NOT NULL | Denormalized from the task, for extract scoping. **Refreshed on every persisted evaluation** — it is not write-once and not rank-guarded, because it is an identity attribute of the task rather than evidence about delivery. A task moved between workspaces is picked up because `tasks.updated_at` is a due source (see **Sweep selection predicate**). |
| `delivery_outcome` | TEXT NULL | `pr_merge \| direct_commit \| no_delivery_observed \| unknown`. `NULL` means never evaluated. |
| `delivery_basis` | TEXT NULL | The evidence class that produced the outcome. See **Basis vocabulary**. |
| `delivery_ref` | TEXT NULL | The identifying evidence: a provider pull/merge request URL, or a commit SHA. |
| `evidence_rank` | INTEGER NOT NULL DEFAULT 0 | The rank of the `(outcome, basis)` pair that produced this row. See **Evidence rank**. Persisted so the upsert guard needs no lookup table at write time. **NOT NULL by design** — the upsert guard compares this column in SQL, and `x >= NULL` is `NULL` (falsy) on both dialects, so a nullable rank would silently suppress the first real write. This table is new, so it has no legacy rows and the "legacy rows get `NULL`" rule does not apply to it; that rule governs columns added to *existing* tables, i.e. `runs.outcome`. |
| `reached_default_at` | TIMESTAMP NULL | When work for this pair was first *observed* reaching the repository's default branch. Write-once. |
| `reached_default_basis` | TEXT NULL | How that observation was made. See **Basis vocabulary**. |
| `reached_default_ref` | TEXT NULL | The commit SHA or pull/merge request URL that carried the observation. |
| `observed_branch_commits` | INTEGER NULL | Monotonic high-water mark of `ahead`. See **Ordering, idempotency, concurrency**. |
| `first_classified_at` | TIMESTAMP NULL | When a non-`NULL` `delivery_outcome` was first written. Write-once. **Its instant is the one the evaluation began reading its inputs at**, the same source as `last_evaluated_at`, and it is fixed here for the same comparability reason this document gives for `reached_default_at`: a timestamp column whose source varies between implementations cannot be compared across rows. |
| `last_evaluated_at` | TIMESTAMP NOT NULL | Advances on every persisted evaluation, including no-change evaluations. **Its value is the instant the evaluation began reading its inputs, never the instant the upsert committed** — see **Sweep selection predicate**, "Which instant `last_evaluated_at` records". |
| `evaluation_seq` | INTEGER NOT NULL | Monotonic per row; incremented on every persisted evaluation. |
| `created_at` | TIMESTAMP NOT NULL | |
| `updated_at` | TIMESTAMP NOT NULL | Advances only when a classification or observation column changes. |

`UNIQUE(task_id, repository_id)`.
Index on `(workspace_id, last_evaluated_at)` for the extract and the stall check.

`reached_default_at` is deliberately **not** derivable from `delivery_outcome`
and does not participate in it. A pair may reach the default branch while its
outcome stays `unknown` (see **Classification**, rule 3).

### Basis vocabulary

`delivery_basis`:

| Value | Meaning |
|---|---|
| `provider_pr_merged` | A non-detached provider pull/merge request for this pair reports merged. |
| `default_branch_commit` | The head commit advanced on snapshots whose branch is the repository's default branch. |
| `reached_default_unattributed` | Work reached the default branch, but by no route Kandev observed. |
| `branch_commits_unmerged` | Commits exist on a non-default branch and no delivery evidence was found. |
| `no_commits_observed` | Snapshots exist for this pair and every one shows no commits. |
| `provider_pr_unmerged` | No snapshot exists, and **no** provider pull/merge request row for this pair reports merged-and-not-detached. This is deliberately weaker than "every row reports not-merged": a row that merged and was later **detached** satisfies this basis, because rule 1 excludes detached rows from counting as delivery. The wording matches rule 5's predicate exactly, so a consumer must not read this basis as "no merged request ever existed" — it asserts that no *attributable* merged request exists now. |
| `no_observations` | Neither a snapshot nor a provider pull/merge request row exists. |
| `default_branch_unknown` | The repository's `default_branch` is empty, so default-branch rules could not be evaluated. Supersedes the matched rule's own basis; see **Degraded evaluation**. |

`reached_default_basis`:

| Value | Meaning |
|---|---|
| `provider_pr_merged` | Derived from a merged, **non-detached** provider pull/merge request whose base branch equals the repository's `default_branch`. Non-detached matters and is not inherited by implication: rule 1 excludes detached rows, so without the word here a detached row could offer an observation whose ref-selection set is empty. |
| `default_branch_commit` | The pair's session committed on the default branch itself. |
| `ancestor_of_default` | The pair's selected head commit is an ancestor of the default branch in the local checkout. |
| `push_webhook_default` | A verified push webhook reported this pair's head commit on the default branch ref. Defined for a later card; nothing writes it here (see **Out of scope**). |

### Evidence rank

The upsert guard and the monotonicity rule are both defined over a single
integer derived from the `(delivery_outcome, delivery_basis)` pair. **Rank is a
property of the evidence, not of the outcome enum** — that is what makes
`unknown` with observed commits stronger than `no_delivery_observed`, which the
outcome enum alone cannot express.

| Rank | `delivery_outcome` | `delivery_basis` |
|---|---|---|
| 0 | `NULL` | `NULL` (never evaluated) |
| 1 | any | `default_branch_unknown` |
| 2 | `unknown` | `no_observations` |
| 3 | `unknown` | `provider_pr_unmerged` |
| 4 | `no_delivery_observed` | `no_commits_observed` |
| 5 | `unknown` | `branch_commits_unmerged` |
| 6 | `unknown` | `reached_default_unattributed` |
| 7 | `direct_commit` | `default_branch_commit` |
| 8 | `pr_merge` | `provider_pr_merged` |

**The rank is injective over ranks 2–8, and deliberately not over ranks 0 and 1.** State
this precisely, because an earlier revision of this document claimed injectivity over the
whole ladder and that claim was false:

| Rank | Injective? | Why |
|---|---|---|
| 0 | n/a | The never-evaluated sentinel. It is not a classification and no evaluation ever computes it. |
| 1 | **No, by design** | Every degraded evaluation lands here regardless of which rule matched, so rank 1 admits `(pr_merge, default_branch_unknown)`, `(unknown, default_branch_unknown)` and `(no_delivery_observed, default_branch_unknown)`. See **Degraded evaluation**. |
| 2–8 | **Yes** | Each maps to exactly one `(outcome, basis)` pair and each reachable non-degraded pair maps to exactly one rank. |

Rank 1 is non-injective on purpose and the alternative was considered and rejected: giving
each degraded `(outcome, default_branch_unknown)` pair its own rank would destroy the
property that makes degraded rows safe — that a degraded row is the *weakest* evidence and
is therefore superseded by **any** real evaluation once `default_branch` is populated. A
degraded `pr_merge` at some higher rank would block a later, sighted
`unknown` / `no_observations` at rank 2, which is exactly backwards: the degraded row was
computed blind to the default branch and must never outrank a row that was not.

What injectivity over 2–8 buys is narrow and is the only thing that rests on it: at ranks
2–8, equal rank implies the same `(outcome, basis)`, which is what makes the equal-rank
rule under **Monotonic evidence** a no-op in outcome and basis. It does **not** imply the
`delivery_ref` is unchanged — see that section. Rank 1 has its own equal-rank rule, stated
under **Degraded evaluation**.

Do not add a value to ranks 2–8 without giving it its own rank.

The rank order is the order in which evidence accumulates in practice: a pair
begins unobserved, may acquire an unmerged pull request, then snapshots showing
nothing, then commits, then a default-branch observation, then a route. No
transition in that sequence is a demotion, which is the property **Monotonic
evidence** relies on.

### `runs.outcome` (new column)

Nullable TEXT on the existing `runs` table:
`processed | budget_blocked | idle_skipped | agent_inactive | task_tree_held`.

`NULL` for every row written before activation, for any run that never reaches a terminal
status, and for every run that reaches `status = 'failed'` (see **Office run outcome**,
the `FailRun` bullet). On the **finished** path the writer writes one of the five values
above; it never writes `''`. No database constraint is added —
the reader is total over every possible value (see **Office run outcome**), so
an unrecognised value degrades to `skipped` rather than breaking a query.
`runs.status` semantics are unchanged: a blocked run still reaches `finished`,
so every existing consumer of `status` keeps working.

### Activation points

Each mechanism writes its activation instant into the existing `kandev_meta`
key/value table, alongside the `schema_initialized_at` key already written by
`persistence.Provide`:

- `telemetry.delivery_ledger.activated_at`
- `telemetry.run_outcome.activated_at`

Both are RFC3339 UTC and never overwritten on replay.

**An activation instant is written only after its schema is verified present.** This is
the single most important rule in this section, because the migration runner
(`MigrateLogger.Apply`) logs a failure at WARN and **swallows it**, so a migration that
did not apply is invisible at boot. Writing the key "on first application" would therefore
publish an activation instant for a table or column that does not exist — and a consumer
reading that key would conclude the mechanism was live and treat every absent row as
"observed as nothing", which is the exact inversion the activation point exists to
prevent.

So each key is written only after a positive check, at the same boot, that its schema
actually exists:

- `telemetry.delivery_ledger.activated_at` — only after a probe confirms
  `task_delivery_ledger` is present and queryable.
- `telemetry.run_outcome.activated_at` — only after a probe confirms `runs.outcome` is
  present.

If the probe fails the key is not written, nothing is logged as success, and the next boot
retries both the migration and the probe. An absent activation key therefore means "this
mechanism is not live", which is a true statement a consumer can act on; a present one
means the schema was verified. The two are never ambiguous.

**The activation key does not gate the writer.** The sweep starts and runs on its normal
cadence whether or not either key was written, and a failed probe never disables it. This
is stated because the opposite is the tempting reading and it would be self-defeating: if
a failed probe stopped the sweep, then a swallowed migration would produce no upsert
attempts, so `delivery_ledger_write_errors_total` would stay at `0` and
`delivery_ledger.write_failed` would never be logged — the migration-failure signals under
**Failure modes** would all be silenced by the very condition they exist to detect. The
key is a published fact *for consumers*, describing when the data became trustworthy. It
is not a feature flag.

**Reset parity.** `internal/system/database/reset.go` drops every user table but
deliberately preserves `kandev_meta`. Left alone, that would leave an activation
instant pointing at a moment before data that no longer exists — inverting the
guarantee the activation point exists to provide, because a post-reset task with
no ledger row would read as "observed as nothing" rather than "not observed".
The reset path therefore **deletes both `telemetry.*.activated_at` keys** along
with the user tables it drops, so the next boot re-establishes an activation
point that matches the data actually present. This is the only change this spec
makes to the reset path.

**Ordering is fixed, because the reset drops tables one at a time and is not
transactional.** The keys are deleted **first**, before any user table is dropped. The two
orders are not equivalent under partial failure and only one of them fails safe:

- **Keys first, then drops.** If a drop then fails, the database holds ledger rows with no
  activation key. A consumer reads "this mechanism is not live" and treats the rows as
  untrusted — conservative, and recoverable on the next boot, which re-probes the schema
  and rewrites the key.
- **Drops first, then keys.** If the key deletion then fails, the database holds an
  activation instant pointing at data that no longer exists. A post-reset task with no
  ledger row reads as "observed as nothing" rather than "not observed" — the precise
  inversion this section exists to prevent, and one nothing later detects, because a
  present key is exactly the state a healthy system is in.

The asymmetry is the whole argument: a missing key is a true statement that costs a boot to
correct, while a stale key is a false statement that survives indefinitely. If the key
deletion itself fails, the reset fails and is reported as such rather than proceeding to
drop tables.

## Classification

For each candidate `(task, repository)` pair, rules are evaluated in this exact
order and the **first** match wins.

**What a predicate may read.** Every predicate below reads the inputs observed on *this*
evaluation, plus the default-branch observation *this* evaluation has just computed. No
rule reads a stored **classification** column (`delivery_outcome`, `delivery_basis`,
`delivery_ref`, `evidence_rank`, `observed_branch_commits`). Rule 3 is the only rule that
reads an observation, and it reads the freshly computed one, never the stored
`reached_default_at`. Evaluation order within a single evaluation is therefore fixed:
**the default-branch observation is computed first, then the rules are matched.**

The consequence is deliberate and must not be treated as a defect: if a pair's ancestry
check errors or is skipped on a later evaluation, that evaluation computes
`branch_commits_unmerged` (rank 5) rather than `reached_default_unattributed` (rank 6).
That is a demotion, so **Monotonic evidence** suppresses it, the stored row keeps rank 6,
and `delivery_ledger_demotions_suppressed_total` increments. Reading the stored column
instead would hide a real evidence regression behind an equal-rank no-op; counting it is
the point.

Normalization applied before any rule runs:

- A `head_commit` that is `NULL`, empty, or whitespace-only is **not** a distinct head
  value and is ignored **everywhere in this document**, not merely in this section. It is
  never counted toward a distinct-head predicate, never selected as a ref, and never
  submitted to git. The scope is stated document-wide because two selection rules that
  consume `head_commit` — the ancestry head under **Default-branch observation**, "Which
  commit", and the `direct_commit` ref under **Ordering** — live in other sections and
  select by greatest `created_at`; scoped to this section alone, a newer empty head would
  outrank a real one and reach either `git --is-ancestor` or a write-once column. Both rules
  restate the filter locally so neither can be read in isolation and get it wrong.
- An `ahead` that is `NULL` or negative is read as `0`.
- A `repositories.default_branch` that is `NULL` is read as the empty string, and an
  empty `default_branch` triggers **Degraded evaluation**. The column is
  `TEXT DEFAULT ''` and not `NOT NULL`, so both forms occur.
- A `task_sessions.repository_id` that is `NULL` is read as the empty string, and an
  empty value attributes the session to no pair (see **Failure modes**). That column is
  also `TEXT DEFAULT ''` and not `NOT NULL`.
- Branch comparison against `repositories.default_branch` is exact and
  case-sensitive, and **never matches when `default_branch` is empty** — an empty
  `default_branch` is not a branch name, and treating it as one would let a provider row
  or snapshot with an equally empty branch column compare equal. See **Degraded
  evaluation**.

**Head movement is a per-session fact.** "Two or more distinct non-empty `head_commit`
values" is evaluated **within a single session's snapshots**, and a pair satisfies such a
predicate when **at least one of its sessions** does. It is never computed by pooling
every session's snapshots together. A task may have many sessions on one repository, and
two sessions that each merely *inherited* a different branch would pool into two distinct
heads and falsely read as authored work — the exact false positive the ancestry
precondition exists to prevent. The Evidence table above was measured per session for the
same reason.

1. **`pr_merge`** — a provider pull/merge request row exists for this pair, is
   not detached, and reports merged (see **Provider predicates**). Basis
   `provider_pr_merged`.
2. **`direct_commit`** — at least one of the pair's sessions has two or more distinct
   non-empty `head_commit` values among **its own** snapshots whose `branch` equals the
   repository's `default_branch`. Basis `default_branch_commit`.
3. **`unknown`** — commits were observed: the greatest `ahead` across the pair's
   snapshots is `> 0`, **or** at least one of the pair's sessions has two or more
   distinct non-empty `head_commit` values among **its own** snapshots.
   - Basis `reached_default_unattributed` when **this evaluation's** default-branch
     observation produced a result, by any basis, whether or not that result was the one
     persisted (a write-once column already set by an earlier evaluation is not
     re-written, but the observation still counts here).
   - Basis `branch_commits_unmerged` otherwise — including when the observation was
     skipped or errored on this evaluation, which is a suppressed demotion rather than a
     reclassification.
4. **`no_delivery_observed`** — at least one snapshot exists for this pair, every
   snapshot shows `ahead = 0`, and **no** session of the pair has two or more distinct
   non-empty `head_commit` values among its own snapshots. Basis `no_commits_observed`.
5. **`unknown`** — no snapshot exists for this pair, at least one provider
   pull/merge request row exists, and none of them reports merged-and-not-detached.
   Basis `provider_pr_unmerged`.
6. **`unknown`** — neither a snapshot nor a provider pull/merge request row
   exists. Basis `no_observations`.

**Totality.** The rules are exhaustive over every candidate pair, and this is
checkable by inspection. Rules 2–4 cover every pair with at least one snapshot:
rule 3's predicate and rule 4's predicate are exact complements of each other
(some `ahead > 0` **or** some session has ≥2 distinct heads of its own, versus all
`ahead = 0` **and** no session has ≥2 distinct heads of its own), and rule 2 is a strict
subset of rule 3's domain taken first — a session with ≥2 distinct heads on the default
branch has ≥2 distinct heads. Rules 5 and
6 cover every pair with no snapshot: provider rows exist, or they do not. Rule 1
crosses both and is taken first. No candidate pair can reach the end of the list
unmatched, and no evaluation ever leaves `delivery_outcome` at `NULL` — `NULL`
means only "no evaluation has been persisted for this row".

Rule 3 is the deliberate consequence of the outcome enum being fixed at four
values. "Reached the default branch by an unobserved route" is real, and it is
recorded in `reached_default_at` / `reached_default_basis` rather than being
forced into `pr_merge` or `direct_commit`, both of which would assert a route
that was not observed. `delivery_basis` carries the distinction the enum cannot.

### Provider predicates

Each provider table is read through the predicates below. There is no shared
schema, so each column is named per table.

| Concept | `github_task_prs` | `gitlab_task_mrs` | `azure_devops_task_prs` |
|---|---|---|---|
| **Merged** | `merged_at IS NOT NULL` | `merged_at IS NOT NULL` | `LOWER(TRIM(status)) IN ('completed','merged')` |
| **Detached** | `detached_at IS NOT NULL` | never detached (no column; a GitLab detach deletes the row) | never detached (no column) |
| **Merge instant** | `merged_at` | `merged_at` | `updated_at` |
| **Request number** | `pr_number` | `mr_iid` | `pull_request_id` |
| **URL (`delivery_ref`)** | `pr_url` | `mr_url` | `pull_request_url` |
| **Base branch** | `base_branch` | `base_branch` | `target_branch` |

Azure's merged vocabulary is taken from the terminal-status set this repository
already uses in `internal/azuredevops/watch_cleanup.go`
(`completed`, `abandoned`, `closed`, `merged`); `completed` and `merged` are the
merged states and every other value — including an unrecognised one — is
not-merged. An unrecognised status is **not** an error and is never guessed at:
the pair simply fails rule 1 and falls through. Azure carries no merge
timestamp at all, so `updated_at` is its merge instant; this is a sort key only
and is never written to `reached_default_at`.

### Candidate pairs

A pair is a candidate when **all three** hold:

1. its `repository_id` is non-empty and it appears in any of `task_repositories`,
   `task_sessions.repository_id`, or any provider pull/merge request table; **and**
2. that `repository_id` **resolves to a row in `repositories`**; **and**
3. its `task_id` **resolves to a row in `tasks`**.

A task with no repository produces **no ledger row** — absence means "not applicable" and
is distinct from `unknown`.

**Why condition 2 is not redundant.** A non-empty `repository_id` is not a guaranteed
foreign key anywhere it is read from: `task_sessions.repository_id` is
`TEXT DEFAULT ''` with **no** FK to `repositories`, and the three provider tables declare
their `repository_id` the same way (`github_task_prs.repository_id` is
`TEXT NOT NULL DEFAULT ''`). A dangling non-empty value is therefore representable in the
data, while `task_delivery_ledger.repository_id` **is** a real FK to `repositories(id)`.

Without condition 2 such a pair is a candidate every sweep, every input read succeeds, the
upsert fails the foreign key forever, no row is ever written, and — because "no ledger row"
makes a pair unconditionally due — it is retried on every pass for the life of the
database, consuming work and producing nothing.

So the pair is **excluded from candidacy and counted**, never attempted:
`delivery_ledger_pairs_missing_repository_total` is set per pass to the number of such
pairs. Counting is what keeps this consistent with "a pair is never silently skipped" —
the exclusion is explicit, observable and bounded, rather than being an INNER JOIN that
drops rows invisibly. A repository row that later appears makes the pair a candidate
again with no special handling, because candidacy is recomputed every pass.

**Why condition 3 exists, and why it is not implied by condition 2.** The hazard is
structurally identical and it is reachable through a different table, so guarding only the
repository side leaves the same failure loop open on the task side.
`github_task_prs.task_id` is `TEXT NOT NULL` with **no** FK to `tasks`, and no code path
deletes `github_task_prs` rows when a task is hard-deleted — the GitHub task-deleted
subscriber prunes PR **watches** and credentials only. (Azure DevOps does clean up, with a
`DELETE FROM azure_devops_task_prs WHERE task_id = ?`; GitHub does not, and the divergence
is why this cannot be reasoned about per provider.) Because the provider tables are a
declared candidacy source in condition 1, an orphaned GitHub pull request row yields a
candidate pair whose task row is gone, while `task_delivery_ledger.task_id` **is** a real
FK to `tasks(id)`.

Three separate things break if such a pair is admitted, and none of them is recoverable at
build time from the rest of this document:

- Its upsert fails the `tasks(id)` foreign key on every pass, forever, exactly as in the
  repository case — and because every failed upsert increments
  `delivery_ledger_write_errors_total`, the counter this spec designates as **the primary
  migration-failure signal** would climb continuously on a completely healthy schema. That
  does not merely add noise; it destroys the one signal a `/debug/vars` reader is told to
  watch, by making its resting value non-zero for an unrelated reason.
- The sweep order under **Ordering** is `tasks.created_at` ascending, then `tasks.id`. A
  pair with no task row has neither value. An INNER JOIN would drop it silently, which
  contradicts "a pair is never silently skipped"; a LEFT JOIN would sort a `NULL` in a
  position neither dialect fixes.
- `workspace_id` is `NOT NULL` and is defined as denormalized *from the task*. With no task
  row there is no value to denormalize and no honest default.

So the pair is **excluded from candidacy and counted**, never attempted, on the same terms
as condition 2: `delivery_ledger_pairs_missing_task_total` is set per pass to the number of
candidate pairs rejected because their `task_id` matched no `tasks` row. It is a separate
gauge from the repository one rather than a combined "unresolvable pairs" count, because
the two have different causes and different owners — a missing repository points at
repository lifecycle, a missing task points at provider-row cleanup — and a single number
would tell an operator neither. A `tasks` row that later appears (or a provider row that is
finally pruned) changes the count on the next pass with no special handling, because
candidacy is recomputed every pass.

Note that both conditions are different from soft-deletion. A soft-deleted repository
(`repositories.deleted_at` non-`NULL`) still **has** a row, so its pairs remain candidates
and are governed by the freeze under **Persistence guarantees**; a missing row is not a
candidate at all. The two must not be collapsed: one retains its ledger rows, the other
never had any. An **archived** task likewise still has a row and is still evaluated
(**Persistence guarantees**); only a hard-deleted task removes the row, and its ledger rows
go with it by cascade.

### Degraded evaluation

When `repositories.default_branch` is empty — or `NULL`, which normalizes to empty —
**every default-branch-dependent step is suspended**: rule 2, the ancestry observation,
*and* the `provider_pr_merged` default-branch observation. All three are suspended, not
just the first two, because the branch comparison is exact: a provider row whose own base
branch column is empty would compare **equal** to an empty `default_branch` and
permanently set the write-once `reached_default_*` triple during an evaluation that is
blind to what the default branch actually is. That is the one outcome degraded mode exists
to prevent, and it is why **Classification**'s normalization states that an empty
`default_branch` never matches any branch.

Rule 1 itself still runs — whether a pull request merged does not depend on knowing the
default branch — so a degraded pair can still be classified `pr_merge`. Only the
*observation* derived from that row's base branch is suspended.

Classification still runs and still matches exactly one
rule, but the result is degraded: `delivery_basis` records
`default_branch_unknown` **in place of** the matched rule's own basis, and
`evidence_rank` is `1` regardless of which rule matched. `delivery_outcome` is
whatever the matched rule produced. `delivery_ref` is `NULL`, per **Ordering**.

Recording the degradation rather than the rule is deliberate: "this pair was
classified blind to the default branch" is the fact a consumer must not miss,
and rank 1 guarantees the degraded row is replaced by any later real evaluation
once `default_branch` is populated.

**Rank 1 admits more than one outcome, and that has consequences the builder must not
have to derive.** Because every degraded evaluation lands at rank 1 whatever rule matched,
two successive degraded evaluations of the same pair can carry *different* outcomes at the
*same* rank. Worked example, which is reachable and not hypothetical: a degraded pair with
a merged pull request matches rule 1 and stores `pr_merge` / `default_branch_unknown` at
rank 1; the pull request is later detached, so rule 1 no longer matches, every snapshot
reports `ahead = 0`, rule 4 matches, and the pair re-evaluates to `no_delivery_observed` /
`default_branch_unknown` — also rank 1.

The rules that govern that write are:

- The write **happens**. The classification columns are written at equal rank, so
  `delivery_outcome` becomes `no_delivery_observed`. A degraded row is a blind placeholder
  and must track its inputs; freezing it at the first outcome it ever saw would be worse.
- It is **not** a suppressed demotion and
  `delivery_ledger_demotions_suppressed_total` does **not** increment — the rank did not
  fall, and that counter means what it says.
- It is **not silent**:
  `delivery_ledger_degraded_outcome_changed_total` increments whenever an equal-rank write
  at rank 1 changes `delivery_outcome`, so a pair thrashing while blind is visible rather
  than being invisible in both counters.
- `updated_at` advances, because a classification column changed.

None of this applies at ranks 2–8, where the rank is injective and an equal-rank write
cannot change the outcome. See **Ordering, idempotency, concurrency**, "Equal-rank
writes".

### Default-branch observation

`reached_default_at` may be set by any of the four bases in **Basis vocabulary**.
It is **write-once and monotonic**: once set it is never cleared, moved, or
overwritten by a later observation with a different basis.

**Basis precedence within one evaluation.** A single evaluation can satisfy more than one
basis at once — a merged pull request whose base branch is the default branch, snapshots
committing on the default branch, and a positive ancestry check are not mutually
exclusive. Because the triple is write-once, whichever basis is chosen is permanent, so
the choice is fixed here and is not the builder's:

| Precedence | Basis | `reached_default_ref` written |
|---|---|---|
| 1 (strongest) | `provider_pr_merged` | the **URL** column of the row selected by the **default-branch observation ref rule** below — *not* the `pr_merge` ref rule |
| 2 | `default_branch_commit` | the **commit SHA** selected by the `direct_commit` ref rule under **Ordering** |
| 3 | `ancestor_of_default` | the commit SHA submitted to the ancestry check, selected by **Which commit** below |
| 4 (weakest) | `push_webhook_default` | defined for a later card; nothing writes it here |

The evaluation computes every basis it can, then writes the highest-precedence one that
produced a result. The order is by strength of attribution: a merged provider request
names the route, a default-branch commit names the act, ancestry names only the fact of
arrival.

**The default-branch observation ref rule.** This is a *different row set* from the
`pr_merge` ref rule under **Ordering**, and conflating the two writes the wrong ref into a
write-once column. The `pr_merge` ref rule selects among the pair's merged, non-detached
provider rows with **no base-branch filter**, because rule 1 asks only whether a pull
request merged. The observation asks a strictly narrower question — whether work reached
*the default branch* — so it selects among the pair's merged, non-detached provider rows
**whose base branch equals the repository's `default_branch`**, and then applies the same
tiebreak keys in the same order (earliest merge instant, then request number ascending,
then provider name ascending in the order `azure_devops`, `github`, `gitlab`, then that
provider's scope column ascending).

The two rules therefore agree whenever every merged row targets the default branch, and
diverge exactly when they should. Concretely: a pair with a merged pull request into a
release branch and a later merged pull request into `main` selects the release-branch row
for `delivery_ref` (it merged first, and rule 1 does not care where) and the `main` row
for `reached_default_ref`. Selecting the release-branch URL for the observation would
permanently record, in a write-once column, a pull request that never reached the default
branch.

Because both rules resolve through named columns with total tiebreaks, repeated
evaluations of unchanged inputs select the same ref, and no new tiebreak is introduced.

This precedence governs only which basis is *offered* to the write-once triple. Whether
it is *stored* is decided by **Monotonic evidence**: if `reached_default_at` is already
non-`NULL`, none of the three columns is written, regardless of precedence.

Its **value** is the UTC instant at which the observation was first made — the
evaluation's own clock reading — not the snapshot's `created_at`, not the
provider's `merged_at`, and not the commit's author or committer date. Only the
observation instant is available for all four bases, and a column whose source
silently varies by basis cannot be compared across rows.

The absence of `reached_default_at` never means "did not reach the default
branch". Ancestry in particular produces false negatives for every squash-merged
branch — see **Evidence**, finding 2.

**Ancestry precondition.** An ancestry check is attempted **only when at least one of the
pair's sessions shows two or more distinct non-empty `head_commit` values among its own
snapshots** — that is, only when some single session's head actually moved and that
session therefore authored commits. The grain is per session, never pooled across the
pair; see **Classification**, "Head movement is a per-session fact". Pooling would let two
sessions that each inherited a different branch satisfy the precondition without either
one authoring anything, which is the precise failure this precondition exists to prevent.
This precondition is load-bearing, not defensive. A session that produced
nothing reports its inherited base commit as its head, and that commit is
already on the default branch's history, so an ungated `--is-ancestor` returns
**true** and would permanently set a write-once column on a pair that delivered
nothing. `ahead` cannot substitute for this test: per **Evidence** finding 1 it
is base-relative, so an inherited branch reports `ahead > 0` with a head that
never moved.

When the precondition is not met the check is skipped,
`delivery_ledger_ancestry_skipped_total` increments, and nothing is written.

**Which commit.** The head submitted to the check is selected from the snapshots of the
**session(s) that satisfied the moving-head precondition** — that is, only sessions
showing two or more distinct non-empty `head_commit` values among their own snapshots
contribute candidate heads — and, within that set, **only from snapshots whose
`head_commit` is non-empty** after the normalization in **Classification**. A `NULL`,
empty or whitespace-only head is not a candidate for selection at all, and a snapshot
carrying one is skipped over rather than winning on recency. Among that doubly filtered
set, the head is the `head_commit` of the snapshot with the greatest `created_at`; ties
are broken by the lexicographically greatest `head_commit` among those sharing that
`created_at`.

The filter is restated here rather than inherited by implication because the ordering key
is `created_at` and the empty value would otherwise win whenever it is newest: a session
that recorded a final snapshot with no head would send an empty string to
`git merge-base --is-ancestor`, whose result is meaningless and whose failure would be
counted as an ancestry error rather than as the selection bug it is.

**The session restriction is load-bearing, and an earlier revision of this document had it
backwards.** That revision said the selection was "across the pair's snapshots, not
restricted to the session that satisfied the precondition", on the reasoning that the
precondition decides *whether* to check while recency decides *what* to check. That
reasoning established only that a non-empty head exists **somewhere** across the pair. It
never established that pooling every session's snapshots is safe, and it is not.

The failure it admits is the precondition's own, one step later. A session that authored
nothing reports its inherited base commit as its head, and that commit is already on the
default branch's history. Give a pair one authoring session on an unmerged feature branch
and one idle session sitting on the default branch, and the idle session's snapshot can
carry the **later** `created_at` — so pair-wide recency selects the inherited head,
`git merge-base --is-ancestor` returns **true** trivially (it never diverged, so it is
already an ancestor), and the **write-once** `reached_default_*` triple is permanently set
with a ref naming a commit the pair did not produce. Rule 3 then reads that observation and
promotes the classification from `branch_commits_unmerged` to
`reached_default_unattributed` for the life of the row.

That is precisely the false positive **Ancestry precondition** above exists to prevent. The
precondition closes it at the *gate*, by refusing to check at all unless some session's head
moved; restricting selection closes the same hole at *selection*, which the gate alone does
not reach. Both are required, because the gate and the selector can otherwise resolve to
different sessions.

The restricted set is still never empty when a check is attempted: the precondition
requires some session to hold two or more distinct non-empty heads, and that session's
snapshots are exactly what the restriction keeps.

**Which checkout.** The repository's local checkout is `repositories.local_path`, and it is
**never read from the column directly**. The behaviour this card requires is exactly what
`resolveRepositoryLocalPath` in `internal/task/service/repository_discovery.go` already
implements: reject an empty repository ID, load the repository row, reject an empty
`local_path`, resolve the stored path through the explicit local-repository path
resolution, and reject a stored path that canonicalizes to a different location than the
one saved. This card inherits those canonicalization and containment checks rather than
restating them, because restating them is how they drift apart.

**The seam, stated because that helper is not directly callable.**
`resolveRepositoryLocalPath` is an **unexported method** on the task service's `Service`
type and depends on that service's repository-entity handle, so it cannot be invoked from
the evaluator as written, and the evaluator does not belong inside the task service. The
contract is therefore stated as a port, not as a function call:

- The evaluator depends on a **narrow, read-only, single-method port** — repository ID in,
  validated absolute checkout path or error out — and on nothing else from the task tier.
- That port is satisfied by the task service's existing resolution behaviour, exposed for
  this consumer. Whether it is exposed by exporting the method, by an adapter, or by a
  wrapper is an implementation choice, and the choice is free **provided the validation
  performed is the existing one and is not reimplemented**.
- The evaluator must **not** take a dependency on the concrete task service type, and must
  **not** duplicate the canonicalization or containment logic. Duplicating it is the one
  outcome this paragraph exists to prevent: the copy would silently omit the
  "saved path resolves elsewhere" rejection, which is the check that stops a moved or
  substituted checkout from being read.
- The port returns an error for every rejection above and never a sentinel empty path, so
  the caller has exactly one failure shape to handle.

A session worktree is never used: worktrees are per-session and transient, and the ancestry
question is about the repository. When `local_path` is empty or the port returns an error —
the ordinary case for a provider-cloned repository that has never been materialized locally
— the check is an error under **Failure modes**, never a negative result.

**Which default branch.** Within that checkout the check resolves the default branch as
`refs/remotes/origin/<default_branch>`, falling back to the local branch
`<default_branch>` when the remote-tracking ref does not exist. If neither resolves, the
check is an error under **Failure modes** — never a negative result.

## Evaluation triggers and cadence

**The periodic sweep is the sole guaranteed mechanism.** Every pair is evaluated by the
sweep, on the cadence below, and every acceptance criterion in this document is satisfied
by the sweep alone. An implementation that ships only the sweep is complete against this
spec.

Event triggers are **permitted accelerators, not requirements**. They reduce latency from
up to one sweep interval to near-immediate; they add no outcome the sweep would not
eventually reach, because evaluation is idempotent and the sweep predicate re-selects any
pair whose inputs moved. Where a trigger is wired it must use an existing bus event:

| Input that moved | Event available today |
|---|---|
| A task session reached a terminal state | `events.TaskSessionStateChanged` (`task_session.state_changed`) |
| A GitHub pull request row changed | `events.GitHubTaskPRUpdated` (`github.task_pr.updated`) |
| A GitLab merge request row changed | `events.GitLabTaskMRUpdated` (`gitlab.task_mr.updated`) |

Two inputs have **no** event on the bus today: a git snapshot write, and an Azure DevOps
pull request sync. They are named here so the absence is a recorded fact rather than a gap
a builder discovers and improvises around — this card adds no new event type for them, and
their latency is one sweep interval.

A trigger, where wired, runs the same evaluation as the sweep, asynchronously, and never
inside the caller's transaction: a telemetry writer must not be able to fail or slow a
session transition or a provider sync. A trigger that is dropped costs latency and nothing
else.

Every evaluation reads **all** input sources for the pair. There is no partial
evaluator: if any input query fails, the evaluation is abandoned under
**Failure modes** rather than classifying from what it managed to read.

### Sweep selection predicate

**The freeze is evaluated first and overrides all three conditions.** A pair whose
repository has a non-`NULL` `repositories.deleted_at` is **never** due, whichever
condition below would otherwise select it. This precedence is stated explicitly because
two of the three conditions would otherwise contradict the freeze promised under
**Persistence guarantees**: a soft-deleted repository's pair with no ledger row would be
unconditionally due forever, and one with `reached_default_at IS NULL` would be selected
by stale refresh every 24 hours. Restoring the repository (clearing `deleted_at`) moves
`repositories.updated_at`, which is itself a due source, so a restored pair resumes
evaluation on the next sweep without special handling.

A candidate pair is **due** when the freeze does not apply and any of the following three
conditions holds:

- it has **no ledger row** — a pair with no row is unconditionally due, on every
  sweep, until one is written. This is what makes rule 6 reachable: a pair with
  no snapshot and no provider row has no input observation to compare against,
  and would otherwise never be selected. It also means **the first sweeps after
  activation evaluate every pre-existing pair**, once each, until each has a row. That is
  intended and is not the historical backfill excluded under **Out of scope** — see the
  reconciliation there; or
- its `last_evaluated_at` is older than its **most recent input observation**,
  defined as the greatest of the following, ignoring sources with no rows:

| Source | Column |
|---|---|
| Git snapshots for the pair's sessions | `MAX(task_session_git_snapshots.created_at)` |
| Sessions for the pair | `MAX(task_sessions.updated_at)` |
| Task/repository membership | `MAX(task_repositories.updated_at)` |
| **The task row itself** | **`tasks.updated_at`** |
| **The repository row itself** | **`repositories.updated_at`** |
| GitHub pull requests | `MAX(github_task_prs.updated_at)` |
| GitLab merge requests | `MAX(gitlab_task_mrs.updated_at)` |
| Azure DevOps pull requests | `MAX(azure_devops_task_prs.updated_at)` |

`repositories.updated_at` is in this table for a specific reason and must not be dropped
as redundant with `task_repositories.updated_at`, which is a **different table** tracking
task/repository membership. `repositories` is where `default_branch` and `deleted_at`
live, so without this row: a **Degraded evaluation** row could never be promoted once
`default_branch` was populated — contradicting the promotion this spec promises and the
scenario that asserts it — and a soft-delete or restore would never re-evaluate the freeze
described under **Persistence guarantees**.

`tasks.updated_at` is in this table so a task moved between workspaces re-evaluates and
refreshes the denormalized `workspace_id` (see **Data model**). It re-selects pairs more
often than the other sources do, which is accepted: evaluation is idempotent and its
inputs are indexed reads, so a redundant evaluation costs a little work and changes
nothing. The alternative — leaving `workspace_id` to drift — silently mis-scopes the
extract that the column exists to serve.

Third condition:

- **Stale refresh.** A pair whose `reached_default_at` is `NULL` is due when
  `last_evaluated_at < (sweep pass start instant − 24 hours)`, even when no input moved.
  The comparison is strict, and the right-hand side is computed once per pass from the
  pass's own start instant rather than per pair, so every pair in one pass is tested
  against the same boundary and a long pass cannot let a pair drift past it mid-sweep.

  This exists because the two conditions above are both driven by *Kandev's own tables*,
  and the central question this feature asks — did this work reach the default branch? —
  can be answered by evidence that never touches them. Two concrete cases, both ordinary:
  a branch that merges weeks after its task's last session ends moves no row in any table
  above, and a provider-cloned repository whose local checkout did not exist when ancestry
  was first attempted moves no row when it is later materialized. Under the first two
  conditions alone, neither pair is ever re-examined, and its `reached_default_at` stays
  `NULL` forever — which would defeat the card for exactly the finished-card population
  the ledger exists to classify.

  It is bounded on both sides. It applies **only** while `reached_default_at` is `NULL`,
  so a pair that has been observed reaching the default branch is never re-swept by this
  condition — the column is write-once, so there would be nothing to gain. And at 24 hours
  it is far longer than the 5-minute sweep interval, so it adds at most one evaluation per
  unresolved pair per day and cannot interfere with the "not selected again on the next
  sweep" behaviour asserted in **Scenarios**.

  This condition also subsumes ancestry retry, which is why no separate retry state is
  persisted: an ancestry check that errored left `reached_default_at` at `NULL`, so the
  pair is refreshed a day later and the check is attempted again. **Failure modes** relies
  on this — an ancestry error is explicitly not an abandonment, so `last_evaluated_at`
  advances and only this condition brings the pair back.

### When the ledger itself cannot be read

All three due conditions read `task_delivery_ledger`: condition 1 asks whether a row exists,
conditions 2 and 3 compare `last_evaluated_at` and test `reached_default_at`. So dueness
cannot be computed at all when the ledger table is missing or unreadable — which is
precisely the state a swallowed migration leaves behind. Left unstated, the natural
implementation is a single query joining candidates to the ledger, that query errors, the
pass selects nothing, **no upsert is ever attempted, and
`delivery_ledger_write_errors_total` stays at `0`** — silencing the counter this document
designates as the primary migration-failure signal, and making the missing-table scenario
under **Scenarios** unreachable. That is the same defect as the withdrawn "the stall
detector fires" claim, one signal over, so it is closed here rather than left to be
discovered.

The contract is therefore a two-stage selection with a defined fallback:

1. **Candidacy is computed without touching the ledger.** The candidate set comes from
   `task_repositories`, `task_sessions`, the three provider tables, `tasks` and
   `repositories` — all of which are **Candidate pairs** inputs. This stage never reads
   `task_delivery_ledger`, so a missing ledger cannot suppress it.
2. **Dueness is then computed by reading the ledger rows for those candidates.** If that
   read **succeeds**, the three due conditions apply exactly as written. If it **fails** —
   the table does not exist, or the read errors for any other reason — the pass **treats
   every candidate as due**, which is the same answer condition 1 gives for a pair with no
   ledger row, and proceeds to evaluate and upsert each one.

The fallback is what makes the designated signal true: each of those upserts then fails and
increments `delivery_ledger_write_errors_total`, and `delivery_ledger.write_failed` is
logged once for the pass, exactly as **Failure modes** promises. The sweep additionally
logs once per pass at WARN with the message `delivery_ledger.dueness_unavailable`, carrying
the error and the candidate count, so the fallback is distinguishable from a genuine
first-sweep-after-activation pass in which every pair really is new.

Three properties bound it, and none of them is optional:

- **A failed dueness read is not an input-query failure.** It does not abandon evaluations
  under **Failure modes** and does not increment
  `delivery_ledger_evaluation_errors_total`. The ledger is this feature's *output*, not one
  of the inputs a classification is computed from, and conflating the two would abandon
  every pair and re-silence the counter by a different route.
- **The fallback is idempotent and self-limiting.** When the read failure is transient and
  the table does exist, the pass performs one redundant round of evaluations whose upserts
  succeed; monotonic evidence and the high-water rules make each of those a no-op in value,
  so the only cost is work. The next pass reads dueness normally.
- **The freeze and the candidacy conditions still apply.** The fallback widens *dueness*
  only. A soft-deleted repository's pairs are still never due (see the freeze above), and a
  pair failing conditions 2 or 3 of **Candidate pairs** is still excluded and counted rather
  than attempted — so the fallback cannot turn an unresolvable pair into a write attempt.

**Which instant `last_evaluated_at` records.** The value written is the UTC instant at
which the evaluation **began reading its inputs**. It is *not* the instant the upsert
committed, and this is not a stylistic preference — the natural implementation is wrong
and fails silently.

Take an evaluation that reads its inputs at T0, and suppose an input row for the same pair
changes at T1 while the evaluation is still running, with the upsert committing at T2.
Writing `last_evaluated_at = T2` records that the pair was evaluated *after* the T1 change
when in fact it was evaluated before it. The dueness test is
`last_evaluated_at < MAX(input.updated_at)`, so T1 < T2 means the pair is **not** due and
the T1 change is masked permanently. Writing `last_evaluated_at = T0` leaves T1 > T0, so
the pair stays due and the next sweep sees the change.

This is a lost update, and the case where it bites is the ordinary one rather than a rare
race: the last snapshot a finishing session writes is frequently the last input movement
that pair will ever have, so a masked T1 leaves the pair pinned at whatever the evaluation
computed one moment too early — for example `no_delivery_observed` on a pair whose final
snapshot reported `ahead > 0`. Event triggers cannot rescue it, because they are
explicitly optional accelerators and an implementation that ships only the sweep is
complete against this spec.

The high-water rule under **Monotonic evidence** still applies on top of this: the stored
`last_evaluated_at` never moves backwards, so a slow evaluation that began before a fast
one cannot rewind the column.

The sweep evaluates every due pair in the order given under **Ordering**. It has
no result cap: a pair is never silently skipped. If a sweep is interrupted, the
next sweep resumes from the same ordering and re-evaluates whatever it did not
reach; because evaluation is idempotent, partial completion is safe.

Boundary values, all fixed by this spec so they are not invented downstream:

| Value | Setting |
|---|---|
| Sweep interval | 5 minutes, **completion-relative**: the next pass starts 5 minutes after the previous pass *finished*, not 5 minutes after it started |
| First sweep | runs at boot, immediately after the schema probe under **Activation points**, with no initial delay |
| Concurrent sweep passes | **never**. A single sweep actor runs at most one pass at a time; a pass that overruns the interval delays the next one rather than overlapping it |
| Refresh interval (stale-refresh due condition) | 24 hours |
| Ancestry call timeout | 10 seconds per `(task, repository)` |
| Ancestry calls per evaluation | at most 1 per pair |
| Ancestry admission class | `subproc.GitBackground` |
| `evaluation_seq` initial value | 1 on insert |
| Stall threshold for the data-side health signal | 15 minutes, i.e. three missed sweeps |
| Sweep result cap | none |

The three scheduling rows are fixed here rather than left to the implementation because
each one changes observable behaviour. **Completion-relative** with **no overlap** is what
makes the per-pass gauges (`delivery_ledger_sessions_unattributed_total`,
`delivery_ledger_pairs_missing_repository_total`) well defined — "set at the end of every
pass" has no single meaning if two passes can be in flight, and two overlapping passes
would race on the same rows, making `evaluation_seq` advance twice for one round of inputs
and letting the write-once `reached_default_*` triple be decided by scheduling. A
fixed-rate timer would allow exactly that whenever a pass ran long. **First sweep at boot**
matters because the alternative — waiting one interval — leaves a freshly migrated
database with an empty ledger for five minutes while publishing an activation instant that
says the mechanism is live.

A pair whose task still has running sessions is evaluated normally. Its outcome
may be promoted by any later evaluation; there is no "wait until the task is
finished" gate, because the rank order makes early classification safe.

## API surface

No new HTTP route, WebSocket message, or MCP tool, and no new frontend state or
user-visible copy. The ledger is a telemetry table read by the writer-health
signals below and by the external extract described under **Out of scope**.
Existing task, session, and pull-request responses are unchanged.

Slice A changes two existing day-grain dashboard response shapes additively:
`AgentRunActivityDay` gains `skipped` and `unclassified`, and `AgentSuccessRateDay` gains
`unclassified`. No field is removed or renamed, no run-grain DTO changes, and no new route
is added. That contract is specified under **Office run outcome** § Response shapes.
The values those shapes carry do change for pre-activation days, which is a behavioural
change to an already-rendered chart even though no frontend file changes; the new
`unclassified` field is what makes that change legible in-band.

## Persistence guarantees

- The ledger survives backend restarts. `last_evaluated_at` in the table is the
  authoritative writer-liveness record; no in-process state is required to detect
  a stalled writer.
- `reached_default_at` and `first_classified_at` are write-once for the life of
  the row.
- Deleting a task deletes its ledger rows by cascade. Archiving a task does not:
  archived tasks continue to be evaluated.
- Soft-deleting a repository (`repositories.deleted_at`) freezes evaluation for
  its pairs. Existing rows are retained and `last_evaluated_at` stops advancing.
- Migrations follow ADR 0027: nullable `ADD COLUMN` then idempotent backfill,
  replay-safe on SQLite and Postgres, classified through
  `db.IsDuplicateColumnError` / `db.IsAlreadyExistsError` rather than a local
  string match.

## Ordering, idempotency, concurrency

**Ordering.** A sweep evaluates due pairs ordered by `tasks.created_at`
ascending, then `tasks.id` ascending, then `repositories.id` ascending.

Within a pair, evidence is selected deterministically without relying on a random
identifier or a dialect-specific row number:

- `pr_merge` ref: among the pair's merged, non-detached provider rows, the one
  with the earliest **merge instant** (per **Provider predicates**: `merged_at`
  for GitHub and GitLab, `updated_at` for Azure DevOps); ties broken by
  **request number** ascending (`pr_number` / `mr_iid` / `pull_request_id`);
  further ties by provider name ascending in the order `azure_devops`, `github`,
  `gitlab`; and further ties by that provider's **scope column** ascending —
  `azure_devops_task_prs.azure_repository_id`, `gitlab_task_mrs.project_path`, and
  `github_task_prs.owner` then `github_task_prs.repo`. The ref written is that row's
  **URL** column.

  The scope column is not decoration, but it is load-bearing for **two of the three
  providers, not all three**, and the difference is stated exactly because the uniqueness
  keys are quoted here and a reader can check them:
  - `gitlab_task_mrs` is `UNIQUE(task_id, repository_id, project_path, mr_iid)` and
    `azure_devops_task_prs` is
    `UNIQUE(task_id, repository_id, azure_repository_id, pull_request_id)`. Both keys
    *include* the scope column, so one pair can legitimately hold two rows from the same
    provider with the same request number and different URLs. For these two the scope
    tiebreak is what makes the selection deterministic, and dropping it would leave exactly
    the ambiguity this section exists to rule out.
  - `github_task_prs` is `UNIQUE(task_id, repository_id, pr_number)`, which does **not**
    include `owner` or `repo`. That constraint already makes `(pair, pr_number)` unique for
    GitHub, so the collision the scope key resolves cannot occur there and the
    `owner`-then-`repo` tiebreak is **redundant for GitHub**. It is retained anyway so the
    ordering is total by the same construction for every provider, which keeps the rule
    checkable by inspection.

  **No schema change is implied.** The GitHub key is correct as it stands; the redundancy is
  in the tiebreak, not in the constraint. Do not "fix" the uniqueness key to match the other
  two — widening it would let a pair hold two GitHub rows with the same `pr_number`, which
  the current constraint deliberately prevents.
- `direct_commit` ref: restricted to the snapshots of the **session(s) that satisfied rule
  2's own predicate** — sessions holding two or more distinct non-empty `head_commit`
  values, among their own snapshots, whose `branch` equals the repository's
  `default_branch` — and, within that set, among the pair's default-branch snapshots
  **whose `head_commit` is non-empty** after the normalization in **Classification**, those
  sharing the greatest `created_at`, and of those the lexicographically greatest
  `head_commit`. The non-empty filter is applied **before** the `created_at` comparison, not
  after, so a newer empty-headed snapshot cannot displace a real commit SHA and cannot be
  written into `delivery_ref` — or, through the `default_branch_commit` basis, into the
  write-once `reached_default_ref`. Rule 2 requires two or more distinct non-empty heads on
  the default branch for this basis to be reachable at all, so the restricted set is never
  empty here.

  **Why the session restriction, stated because the same defect appears twice in this
  document.** This is the `direct_commit` twin of the restriction argued at length under
  **Default-branch observation**, "Which commit", and it exists for the same reason. Rule 2
  is earned **per session**, but an earlier revision selected the ref pair-wide, so a second
  session merely *sitting* on the default branch could supply a newer snapshot whose head is
  its inherited tip. That head would then be written as `delivery_ref` — and, because
  `default_branch_commit` is basis precedence 2, into the **write-once** `reached_default_ref`
  as well — permanently naming a commit that the session which actually earned the
  classification did not author. Restricting the candidate set to the session(s) that
  satisfied rule 2 is what keeps the ref attributable to the work that produced it.
- **Every other rule writes `delivery_ref` as `NULL`.** Rules 3, 4, 5 and 6 —
  `branch_commits_unmerged`, `reached_default_unattributed`, `no_commits_observed`,
  `provider_pr_unmerged`, `no_observations` — and any **Degraded evaluation** row identify
  no single artifact that carries the delivery, so there is nothing honest to name. In
  particular rule 5 does **not** write the unmerged pull request's URL: that row is
  evidence the pair did *not* deliver, and putting it in a column defined as "the
  identifying evidence" for a delivery would invite a consumer to read it as one. The
  head commit is likewise not written, because a branch head is not a delivery.

  This completes the ref rules over all six classification rules, which is what makes the
  equal-rank no-op claim under **Monotonic evidence** true for every rule rather than only
  for the two that name a ref.
- `observed_branch_commits` incoming value: the greatest `ahead` across the pair's
  snapshots after the `NULL`/negative normalization in **Classification**, which needs no
  tiebreak. **When the pair has no snapshots at all the incoming value is `NULL`, not
  `0`** — the greatest of an empty set is not zero, and `0` would assert "we looked and
  there were no commits" for a pair where nothing was ever observed, which is the exact
  `NULL`-versus-`0` confusion this feature exists to prevent. Combined with the high-water
  rule, a `NULL` incoming value can never lower a stored one: a pair that had snapshots
  and then lost them keeps its high-water mark.

**Idempotency.** Evaluation is a pure function of the observed inputs. Re-running
an evaluation whose inputs have not changed must leave `delivery_outcome`,
`delivery_basis`, `delivery_ref`, `evidence_rank`, `observed_branch_commits`,
`reached_default_at`, `reached_default_basis`, `reached_default_ref` and
`first_classified_at` byte-identical and must not advance `updated_at`. Only
`last_evaluated_at` and `evaluation_seq` advance.

**Monotonic evidence.** The classification columns move only up the **Evidence
rank** ladder. Comparing the incoming rank to the stored rank gives exactly three
cases, and every column's behaviour is fixed in all three:

| | incoming rank **>** stored | incoming rank **=** stored | incoming rank **<** stored |
|---|---|---|---|
| `delivery_outcome`, `delivery_basis`, `delivery_ref`, `evidence_rank` | written | written | **not** written |
| `first_classified_at` | written if currently `NULL` | written if currently `NULL` | not written |
| `observed_branch_commits` | high-water (below) | high-water | high-water |
| `reached_default_at` / `_basis` / `_ref` | written **only** if `reached_default_at` is currently `NULL`, as one unit | same | same |
| `last_evaluated_at` | high-water (below) | high-water | high-water |
| `evaluation_seq` | `stored + 1` | `stored + 1` | `stored + 1` |
| `workspace_id` | written | written | written |
| `delivery_ledger_demotions_suppressed_total` | — | — | incremented |
| `delivery_ledger_degraded_outcome_changed_total` | — | incremented **only at rank 1** when `delivery_outcome` changed | — |

`workspace_id` is written unconditionally in all three columns of that table, including
the demotion case. It is not evidence about delivery and the rank guard has no business
gating it: a pair that moved workspace has moved workspace regardless of what its
classification did.

**Equal-rank writes, stated separately for ranks 2–8 and for rank 1.** The two cases are
genuinely different and collapsing them is what made an earlier revision of this section
wrong.

*At ranks 2–8.* Equal rank implies the same `(outcome, basis)`, because the rank is
injective there (**Evidence rank**). So `delivery_outcome` and `delivery_basis` are always
a no-op in value. `delivery_ref` is **not** guaranteed unchanged: the ref names the
currently-selected evidence, and the evidence set can move while the rank does not — a
pair already at rank 8 that acquires a second merged pull request with an earlier merge
instant re-selects to that row's URL. That is intended, not a defect: the ref must name
evidence that exists. When the ref changes, `updated_at` advances and
`delivery_ledger_rows_written_total` increments; when nothing changed, neither does.

*At rank 1.* Equal rank does **not** imply the same outcome, because rank 1 is
deliberately non-injective. The classification columns are still written, and the outcome
therefore tracks the current inputs — which is the correct behaviour for a degraded row,
since it is a blind placeholder and pinning it to a stale outcome would be worse than
letting it move. But this write can change `delivery_outcome` without any demotion being
recorded, so it is **counted rather than left silent**: an equal-rank write at rank 1 that
changes `delivery_outcome` increments
`delivery_ledger_degraded_outcome_changed_total` (see **Writer health**).

The concurrency consequence at rank 1 is stated honestly rather than papered over: two
evaluators racing at rank 1 with different outcomes are last-writer-wins, so arrival order
decides which outcome is stored. This is bounded and self-repairing — the next sweep
recomputes from the same inputs and converges, and the whole row is superseded by any
rank ≥ 2 evaluation once `default_branch` is populated. The "converge regardless of
arrival order" guarantee below therefore holds for ranks 2–8 and is explicitly **not**
claimed for rank 1.

**High-water columns, stated as behaviour and not as a SQL function.**
`observed_branch_commits` and `last_evaluated_at` take the greater of the stored and
incoming values, unconditionally, in all three rank cases, with a **`NULL` stored value
treated as lower than any non-`NULL` incoming value**. `observed_branch_commits`
therefore never decreases when a session — and with it, by `ON DELETE CASCADE`, its
snapshots — is deleted. It is a stored high-water mark and **not** a recomputed
aggregate; this is why rule 3's predicate reads the freshly observed `ahead` values
rather than this column.

That behaviour is the contract. The obvious SQL spelling does **not** implement it, and
the difference is silent, so it is called out rather than left to be discovered:

- SQLite's two-argument `max(X, Y)` returns `NULL` if *either* argument is `NULL`, so
  `max(observed_branch_commits, excluded.observed_branch_commits)` erases a real value the
  first time either side is `NULL` — the opposite of a high-water mark.
- PostgreSQL has no two-argument `max` at all; the equivalent is `GREATEST`, whose
  `NULL` handling differs again.

So the implementation must make the `NULL` handling explicit — coalescing to a floor
before comparing, and using each dialect's own spelling — rather than relying on a bare
`MAX`. This is the one place where **Concurrency**'s "never by a read-then-write in
application code" needs qualifying: the rule stays, and the `NULL` handling belongs
*inside* the statement, not in a Go read-modify-write.

The same trap applies to the rank guard itself. `x >= NULL` evaluates to `NULL` on both
dialects and a `CASE` on it takes the `ELSE` branch, so a nullable `evidence_rank` would
silently suppress every write against a row that had one. That is why `evidence_rank` is
declared `NOT NULL DEFAULT 0` in the data model.

Both dialects are in scope for this statement. ADR 0027 governs the migrations, and the
exemption recorded under **Office run outcome** covers only the *pre-existing*
SQLite-only `RunCountsByDayForAgent`; new code added by this card is expected to run on
SQLite and PostgreSQL alike.

A demotion is never written. If a pull request is detached after it merged, or a
snapshot is deleted, the row keeps its stronger evidence and the suppression is
counted. This is what makes the ledger safe: an evaluation that runs after the
evidence has been removed can never erase what an evaluation that could see it
recorded.

**Concurrency.** The ledger is written with a single
`INSERT ... ON CONFLICT (task_id, repository_id) DO UPDATE` statement. Every rule
in the table above is expressed inside that statement's `SET` clause — as
`CASE WHEN excluded.evidence_rank >= task_delivery_ledger.evidence_rank THEN ... END`
for the classification columns (sound precisely because `evidence_rank` is `NOT NULL`),
`CASE WHEN task_delivery_ledger.reached_default_at
IS NULL THEN ... END` for the write-once triple, and the explicitly `NULL`-handled,
dialect-appropriate greatest-of expression described under **Monotonic evidence** for the
high-water columns — never by a read-then-write in application code. Consequences, all of
which are contract:

- Two evaluators racing on the same pair with **different** ranks — say one computing
  rank 1 and one rank 8 — converge on rank 8 regardless of arrival order, and neither can
  lose the other's write. This is the rank guard doing its job and it holds for every
  unequal pair of ranks.
- Two evaluators racing at **equal rank 1** with different outcomes are the one exception
  and are **not** order-independent: rank 1 is deliberately non-injective, so the
  last writer's outcome is stored. This is bounded and self-repairing — see
  **Degraded evaluation** and "Equal-rank writes" above. It is called out here so the
  bullet above is not read as a guarantee it does not make.
- Two evaluators supplying a **first** default-branch observation concurrently:
  whichever statement commits first writes all three of `reached_default_at`,
  `reached_default_basis` and `reached_default_ref`; the second sees a non-`NULL`
  `reached_default_at` and writes none of them. The triple is therefore always
  internally consistent — it is never assembled from two different observations.
- `evaluation_seq` increments on every persisted evaluation, so a consumer can
  tell that the row was re-evaluated even when nothing changed.

Ancestry checks run through the existing class-aware git subprocess admission
pool at class `subproc.GitBackground` — the sweep is background work and must not
contend with user-facing git — with a bounded timeout, at most one per
`(task, repository)` per evaluation.

## Failure modes

- **An input query fails** — a snapshot, session, membership or provider read
  returns an error. The evaluation for that pair is **abandoned**: no column is
  written, not even `last_evaluated_at`, so the pair stays due and is retried on
  the next trigger or sweep. `delivery_ledger_evaluation_errors_total`
  increments. A classification is never persisted from a partially-read input
  set.
- **The ledger upsert itself fails** — the `INSERT ... ON CONFLICT` returns an error:
  the table does not exist because its migration was swallowed, a constraint is violated,
  or the write times out. This is a **distinct** failure from an input-query failure and
  must not be folded into it, because only this one indicates the ledger's own schema or
  storage is broken. No column is written, so the pair stays due and is retried on the
  next sweep. `delivery_ledger_write_errors_total` increments — this counter is the
  primary migration-failure signal and the one a `/debug/vars` reader should watch. The
  sweep logs once per pass at WARN with the message `delivery_ledger.write_failed`,
  carrying the error and the count of pairs that failed in that pass; once per pass rather
  than once per pair, so a missing table cannot emit one log line per candidate. A failing
  upsert **does not abort the pass** — the sweep continues to the next due pair, so one
  poisoned pair cannot stop the writer.
- **Ancestry cannot be computed** — the repository has no readable local
  checkout, neither default-branch ref resolves, the head commit object is
  absent, or the git call fails or times out. No evidence is produced:
  `reached_default_at` stays `NULL` and the outcome is unaffected.
  `delivery_ledger_ancestry_errors_total` increments. The error is never
  persisted as a negative result, and it does not abandon the evaluation — the
  rest of the classification still runs.
- **The ancestry precondition is not met** — fewer than two distinct non-empty
  `head_commit` values. The check is skipped,
  `delivery_ledger_ancestry_skipped_total` increments, and this is not an error.
- **A negative ancestry result** is not evidence. It must not set any column and
  must not influence the outcome. Squash-merge makes it a routine false negative.
- **`repositories.default_branch` is empty** — handled by **Degraded
  evaluation**: rules still run, `delivery_basis` is `default_branch_unknown`,
  `evidence_rank` is 1.
- **A session has an empty or `NULL` `repository_id`** — the column is `TEXT DEFAULT ''`
  and not `NOT NULL`, so both forms occur; both normalize to empty. The session
  contributes to no pair, is excluded from the stall comparand, and is counted in
  `delivery_ledger_sessions_unattributed_total`. Three such sessions exist in the
  reference store.
- **`head_commit` is empty, whitespace-only, or `NULL`** — it is not a distinct head value
  and is ignored.
- **`repositories.default_branch` is `NULL`** — the column is `TEXT DEFAULT ''` and not
  `NOT NULL`. It normalizes to empty and takes the **Degraded evaluation** path, the same
  as an empty string. There is no third behaviour.
- **Azure DevOps has no `merged_at`** — resolved by **Provider predicates**:
  merged is read from `status`, and `updated_at` is the merge instant used for
  ordering only. An unrecognised status is not-merged, so the pair falls through
  to a later rule rather than being guessed at.
- **The same pull request is attached to more than one task** — this occurs in
  the reference store (PR #2418, two tasks). Each task's pair is classified
  independently and both may read `pr_merge`. The ledger does not deduplicate
  credit across tasks.
- **A migration fails** — the migration runner logs at WARN and swallows the
  error (`apps/backend/internal/db/migratelog.go`, `MigrateLogger.Apply`), so a
  failed migration is invisible at boot. Three signals make it visible, and it is worth
  being exact about which one does the work, because an earlier revision of this document
  claimed "the stall detector fires" and that was **false**:
  - `telemetry.delivery_ledger.activated_at` is **absent** from `kandev_meta`, because
    the schema probe under **Activation points** found no table. This is the cheapest
    check and the one a consumer should make first.
  - `delivery_ledger_write_errors_total` climbs on every sweep pass and
    `delivery_ledger.write_failed` is logged, per the upsert-failure mode above. This is
    the signal that actually fires — **and it fires only because of the dueness fallback**
    under **Sweep selection predicate**, "When the ledger itself cannot be read". Without
    that fallback the missing table would break dueness computation first, the pass would
    select nothing, and this counter would stay at `0`. The two statements are load-bearing
    for each other: do not implement one without the other, or this bullet becomes as false
    as the withdrawn stall-detector claim below.
  - `delivery_ledger.dueness_unavailable` is logged once for the pass, which is what
    distinguishes a broken ledger from a legitimate first sweep in which every candidate is
    genuinely new.
  - `delivery_ledger_last_evaluated_unix` publishes the unavailable sentinel `-1`, per
    **Writer health**.

  The stall detector specifically does **not** fire, and relying on it would have been the
  mistake: with the table missing there is nothing to take `MAX(last_evaluated_at)` over,
  and the empty-table rule would report `delivery_ledger_stall_seconds = 0`, which reads
  as healthy. That is why the stall signal is not the migration-failure signal and why
  the sentinel above exists.

## Writer health

If this writer silently stops, nothing today would notice. Both signals below are
required, because they fail in different ways.

**In-process counters**, following the `subproc_*` / `routing_*` precedent
(`apps/backend/internal/common/subproc/metrics.go`), published at package init
and exposed through the existing dev-mode `/debug/vars` handler:

| Counter | Increments |
|---|---|
| `delivery_ledger_evaluations_total` | once per **persisted** evaluation, keyed by the `delivery_outcome` that evaluation computed. An abandoned evaluation persists nothing and is **not** counted here; it is counted only by `delivery_ledger_evaluation_errors_total`. A suppressed demotion **is** counted, keyed by the outcome the evaluation *computed*, not the one that stayed stored — otherwise a pair stuck in demotion looks idle. |
| `delivery_ledger_rows_written_total` | once per upsert that **changed at least one** classification or observation column, i.e. exactly when `updated_at` advances. Not once per evaluation: a re-evaluation with unchanged inputs advances only `last_evaluated_at` and `evaluation_seq`, and counting it would make the counter a duplicate of `evaluations_total`. |
| `delivery_ledger_demotions_suppressed_total` | once per upsert whose incoming rank was strictly lower than the stored rank. |
| `delivery_ledger_evaluation_errors_total` | once per abandoned evaluation (an input query failed). |
| `delivery_ledger_ancestry_errors_total` | once per ancestry check that was attempted and could not produce a result. |
| `delivery_ledger_ancestry_skipped_total` | once per evaluation where the ancestry precondition was not met. |
| `delivery_ledger_write_errors_total` | once per pair whose ledger upsert returned an error (see **Failure modes**). Kept separate from `evaluation_errors_total` because an input-query failure means the *inputs* are unreadable while this means the *ledger's own* schema or storage is broken — the migration-failure case. This is the counter that fires when a swallowed migration left the table missing. |
| `delivery_ledger_degraded_outcome_changed_total` | once per equal-rank write at rank 1 that changed `delivery_outcome` (see **Degraded evaluation**). Rank 1 is deliberately non-injective, so such a write is neither a promotion nor a suppressed demotion and would otherwise be invisible in every other counter here. |
| `delivery_ledger_pairs_missing_repository_total` | **a gauge, not a running total**, set per sweep pass for the same reason as `sessions_unattributed_total`: the count of candidate pairs the pass rejected because their `repository_id` matched no `repositories` row (see **Candidate pairs**). |
| `delivery_ledger_pairs_missing_task_total` | **a gauge, not a running total**, set per sweep pass: the count of candidate pairs the pass rejected because their `task_id` matched no `tasks` row (see **Candidate pairs**, condition 3). Kept separate from the repository gauge because the two have different causes — a missing repository points at repository lifecycle, a missing task at provider-row cleanup — and a combined number would identify neither. A non-zero resting value here is a data-hygiene fact about orphaned provider rows, **not** a ledger fault, and specifically not a migration failure. |
| `delivery_ledger_sessions_unattributed_total` | **a gauge, not a running total.** It is set — not incremented — at the end of every sweep pass to the count of distinct sessions with an empty `repository_id` that the pass encountered. It is the one value here whose natural reading is "how many are there", and incrementing per encounter would add the same three reference-store sessions on every 5-minute pass, turning a fact about the data into a number that only measures uptime. The `_total` suffix is retained for consistency with the `subproc_*` precedent. |

Every counter is keyed and cadenced above so that two implementations produce the same
numbers from the same inputs. A counter whose meaning is left to the writer is not a
health signal.

**The data-side stall signal.** Counters reset to zero on restart, so a writer
that stopped before the last restart looks healthy by counters alone. The stall
signal is computed from the database instead, which is what makes it survive a
restart:

- **Actor and cadence:** the sweep, at the end of every pass, including a pass
  that evaluated nothing.
- **What it computes:** `MAX(task_delivery_ledger.last_evaluated_at)` across the
  whole table, and as the comparand, `MAX(task_sessions.updated_at)` across only those
  sessions that **could have produced work for the writer**:

  | Filter | Column | Why |
  |---|---|---|
  | session attributes to a pair | `task_sessions.repository_id` non-empty after normalization | A session with no repository contributes to no pair, so it can never advance `last_evaluated_at`. Three such sessions exist in the reference store. |
  | repository still live | the joined `repositories.deleted_at IS NULL` | Pairs of a soft-deleted repository have `last_evaluated_at` frozen **by design** under **Persistence guarantees**. |

  Each filter removes a session whose progress the writer is *supposed* to ignore. Without
  them the comparand advances on work the ledger is correctly not doing, and the signal
  reports a stall on a perfectly healthy writer — a false alarm, which is how a real one
  gets ignored.

  **There is deliberately no archived filter**, and the earlier "non-archived sessions"
  wording is withdrawn. It named no real column — `task_sessions` has no archived column
  in its `CREATE TABLE` or in any migration, and archival lives on `tasks.archived_at` —
  but more importantly it was wrong on the merits: **Persistence guarantees** states that
  archived tasks continue to be evaluated, so their sessions are work the writer really is
  expected to keep up with. Excluding them would have hidden a genuine stall on a board
  whose recent activity is mostly archived. The comparand covers every session the writer
  is responsible for, and nothing else.
- **What it publishes:** two expvar Ints alongside the counters,
  `delivery_ledger_last_evaluated_unix` and `delivery_ledger_stall_seconds`. **Two
  independent queries feed them** — the ledger query
  (`MAX(task_delivery_ledger.last_evaluated_at)`) and the comparand query (the filtered
  `MAX(task_sessions.updated_at)`) — and either can fail or return an empty result on its
  own. The table below is total over both, in that order of precedence; states that look
  identical without a sentinel are given distinct ones:

  | State | `..._last_evaluated_unix` | `..._stall_seconds` | Meaning |
  |---|---|---|---|
  | The **ledger** query errored (table missing or unreadable) | `-1` | `-1` | The ledger is **unavailable**. Distinct from healthy-and-empty, which is the whole point of the sentinel. Takes precedence over every row below: if the ledger cannot be read, the comparand's state is irrelevant. |
  | The ledger query succeeded and the table is empty | `0` | `0` | No pair has been evaluated yet. Not a stall: an empty table is not an infinite stall. |
  | Ledger rows exist and the **comparand** query errored | `MAX(last_evaluated_at)` | `-1` | The stall figure is **unknown**, not zero. The last-evaluated value is real and is still published, because it was read successfully; only the derived figure is unavailable. |
  | Ledger rows exist and the comparand set is **empty** (no session survives the filters) | `MAX(last_evaluated_at)` | `0` | Not a stall. There is no work the writer is responsible for keeping up with, so it cannot be behind. This is the ordinary state of a fresh database whose pairs come from `task_repositories` and whose sessions have not run yet. |
  | Ledger rows exist and the comparand is present | `MAX(last_evaluated_at)` as Unix seconds | comparand minus it, in seconds, clamped at `0` | The real values. |

  The comparand-empty and comparand-error rows are separated deliberately: an empty
  comparand is a true statement about the data and must read healthy, while a failed
  comparand read is an absence of information and must not be able to masquerade as
  healthy. Publishing `0` for both would let a repeatedly failing comparand query report a
  permanently healthy writer.

  When the **ledger** query errors the sweep logs once per pass at WARN with the message
  `delivery_ledger.unavailable`; when the **comparand** query errors it logs once per pass
  at WARN with `delivery_ledger.comparand_unavailable`. Without the `-1` sentinels a missing
  table would publish `0`/`0`, which is the healthy-and-empty state — a broken ledger would
  be indistinguishable from a fresh one, which is exactly the inversion this section exists
  to prevent.

  A consumer reading `delivery_ledger_stall_seconds` therefore treats `-1` as "no
  measurement", never as a magnitude, and never compares it against the 15-minute
  threshold.

  Because all of these are recomputed from the database on every pass, the first sweep
  after a restart republishes the true values.
- **What "fires" means:** when `delivery_ledger_stall_seconds` exceeds the
  15-minute threshold, the sweep logs once per pass at WARN with the message
  `delivery_ledger.stalled`, carrying both values as fields.

**What these signals do NOT cover, stated as an accepted residual rather than left to be
discovered.** Every signal in this section is published *by the sweep being monitored*. A
sweep goroutine that panics on its first pass, or is never started, publishes nothing at
all — so the counters keep whatever value they last held (typically `0`, which reads as
healthy) and no WARN is ever emitted. This is the literal "the writer silently stops" case
this section opens with, and no in-process signal can close it: a process cannot report its
own death.

It is accepted rather than solved, for a specific reason that makes the residual small.
The authoritative liveness record is `task_delivery_ledger.last_evaluated_at`, which lives
**in the database**, not in the process. Any consumer — the external extract above all,
which is the reason this table exists — can compute
`now() - MAX(last_evaluated_at)` for itself without the sweep participating at all, and
that computation is correct whether the sweep is running, wedged, or was never started.
The in-process signals are a convenience for a live `/debug/vars` reader; the data-side
record is the ground truth, and this spec's obligation is to keep it accurate. No
acceptance criterion asserts the in-process signals detect a dead sweep, because they
cannot.

## Office run outcome

A second, delimited contract in the same card, because it is the run-grain form
of the same defect: a terminal status is not an outcome.

**Call sites.** Every call site that finishes a run records the outcome that
describes it. The semantic label in the right-hand column is authoritative; the
line number is a pointer (see **Citation convention**).

| Call site | Outcome |
|---|---|
| `office/service/scheduler_integration.go:218` — agent not active | `agent_inactive` |
| `office/service/scheduler_integration.go:247` — idle skip | `idle_skipped` |
| `office/service/scheduler_integration.go:517` — task-tree hold | `task_tree_held` |
| `office/service/scheduler_integration.go:829` — pre-execution budget block | `budget_blocked` |
| `office/service/event_subscribers.go:408` — agent-completed for a task-bearing run | `processed` |
| `office/service/event_subscribers.go:512` — `handleTasklessAgentCompleted` | `processed` |

Checkout errors and checkout contention do not finish a run. A checkout error
uses the normal retry and failure path. A contention result requeues the run
and retries it. Neither path reports work that did not run.

**Reachability of `:512`.** `handleTasklessAgentCompleted` resolves its run through
`GetClaimedTasklessRunForAgent`, which requires `status = 'claimed'` and an empty
`payload.task_id`. Taskless runs fail before they can remain claimed, so the lookup
normally returns `sql.ErrNoRows` and the site is a no-op. It is mapped to `processed`
because an `AgentCompleted` event may still arrive for a legacy claimed run, and it is
retained so the mapping is total over the call sites that exist.

`:408` is the path where an agent actually ran to completion and is the ordinary source
of `processed`. Leaving any terminal site unset would grow `unclassified` forever after
activation and falsify the guarantee that the activation point tells a consumer when
`unclassified` stops growing.

**Concurrency, retry and error behaviour for this column.** Slice A does not introduce a
transaction or a guard, because it must not change `runs.status` semantics:

- `outcome` is written **in the same `UPDATE` statement as `status`**, so a run can never
  hold a terminal status with a stale outcome from a different transition.
- **Where that statement actually lives, stated precisely, because the obvious answer is
  wrong.** `transitionRunTerminal` (`office/service/scheduler_runs.go:52`) contains **no**
  `UPDATE`. It pre-fetches the run, calls `s.repo.FinishRun(ctx, id, status)`, and
  publishes `OfficeRunProcessed`. The single `UPDATE runs SET status = ?, finished_at = ?
  WHERE id = ?` lives one layer down, in
  `internal/runs/repository/sqlite`, `func (r *Repository) FinishRun(ctx context.Context,
  id, status string) error` — which is the only implementation of that method in the
  repository. Per **Citation convention** the semantic label is authoritative: the statement
  to extend is *the one inside the runs repository's `FinishRun`*, not a statement inside
  `transitionRunTerminal`, which does not exist. This distinction is not pedantic, because
  the statement's real home has a caller the service layer does not.
- **`outcome` becomes a parameter of both layers.** The runs repository's `FinishRun` gains
  an outcome parameter alongside `status` and writes both in its one `UPDATE`;
  `transitionRunTerminal` gains the same parameter and forwards it. `FinishRun`
  (`scheduler_runs.go:38`) takes the value from the call-site table above and `FailRun`
  (`:44`) passes `NULL`. `NULL` is correct rather than a placeholder — a run that reached
  `status = 'failed'` is bucketed by `RunCountsByDayForAgent` on its status alone and never
  reaches the `succeeded` / `skipped` / `unclassified` buckets, so no value from the
  five-value vocabulary would ever be read, and inventing one would assert a classification
  nothing consumes. This also makes the sentence under `### runs.outcome` exact: the
  five-value claim describes the **finished** path; the failed path writes `NULL` in the
  same statement.
- **The second caller pair of the shared repository method, and what it passes.** Changing
  that signature necessarily reaches
  `office/scheduler.SchedulerService.FinishRun` and `.FailRun`
  (`internal/office/scheduler/run_processing.go`), which call the same
  `repo.FinishRun` **without** going through `transitionRunTerminal` or the six-site table.
  Both **pass `NULL`**, and neither is added to the call-site table. The reasoning, so a
  builder does not have to guess and does not "improve" it into a vocabulary value:
  - `SchedulerService.FailRun` writes `RunStatusFailed`. It is reached in production (via
    that package's own retry path), and `NULL` is the same value `FailRun` passes at the
    service layer, for the same reason — a failed run is bucketed on status alone.
  - `SchedulerService.FinishRun` writes `RunStatusFinished` and has **no production caller
    at the time of writing**. It is a dormant parallel copy of the service-layer method. It
    passes `NULL` because there is no outcome to describe: no call site means no semantic
    label, and inventing `processed` for it would be the exact defect this slice exists to
    remove — a run in which nothing is known to have happened counted as a success. `NULL`
    routes it to `unclassified`, which is the honest bucket for a path whose meaning is
    unestablished.
  - Consequently the activation guarantee below is scoped to the six sites and is **not**
    weakened by these two: neither is a terminal site this card classifies, and if
    `SchedulerService.FinishRun` is ever wired to something real, the card that wires it
    owns choosing its outcome and adding it to the table. Deleting the dormant method is
    **not** in scope here (see `## Out of scope`).
- That statement is an unconditional `UPDATE runs SET status = ?, outcome = ?,
  finished_at = ? WHERE id = ?` with **no status guard**, matching today's behaviour.
  If two terminal callers ever reach the same row, **last writer wins** for status and
  outcome together. This is the existing contract for `status`, extended unchanged.
- A zero-row update (the run was deleted) is **not** an error and is not retried.
- Four of the six sites discard `FinishRun`'s error (`_ = si.svc.FinishRun(...)`). That
  is unchanged by this card. A discarded failure leaves the row non-terminal with
  `outcome` `NULL`; if it later reaches `finished` by another path it is bucketed by
  whatever that path wrote, and if it never does it is not `finished` and so never
  reaches the `succeeded` / `skipped` / `unclassified` buckets at all.
- Consequently the activation guarantee is stated precisely: after activation,
  `unclassified` stops growing **for runs that reach `finished` through one of the six
  sites above**. It is not a claim that no `finished` row can ever carry a `NULL`
  outcome, because this card does not add error handling to paths that discard errors
  today.

`runs.status` is unchanged. Nothing that reads `status` today changes behaviour.

**Repository query.** `RunCountsByDayForAgent`
(`office/repository/sqlite/agent_summary.go`) reports **five** buckets where it
reports three today. The bucketing is total over every possible column value:

| Bucket | Predicate |
|---|---|
| `succeeded` | `status = 'finished'` **and** `outcome = 'processed'` |
| `skipped` | `status = 'finished'` **and** `outcome` is any other non-`NULL` value |
| `unclassified` | `status = 'finished'` **and** `outcome IS NULL` |
| `failed` | `status IN ('failed','timed_out')` (unchanged) |
| `other` | every remaining status (unchanged) |

**Response shapes.** Two shapes in `office/dashboard/agent_summary.go` derive
from those rows, and both change additively:

- `AgentRunActivityDay` (built by `padAgentRunActivity`) gains `skipped` and
  `unclassified` JSON fields. Its documented invariant becomes
  `total = succeeded + skipped + unclassified + failed + other`, so `total` is
  unchanged in value for any given day.
- `AgentSuccessRateDay` (built by `buildSuccessRate`) gains **one** JSON field,
  `unclassified`. `succeeded` now counts processed runs only, while `total` remains the
  same full denominator, so the success rate falls to the truth rather than being inflated
  by non-work.

  **Why it gains a field rather than keeping its two.** Without one, this shape changes
  meaning mid-series with no in-band marker, which is the exact discontinuity the
  `NULL`-not-`0` rule in `## What` exists to prevent — and it is worse here than elsewhere,
  because the consequence is not a subtle miscount. `succeeded` is today
  `status = 'finished'` and becomes `status = 'finished' AND outcome = 'processed'`, while
  every run finished before activation carries `outcome IS NULL`. So **every pre-activation
  day reports `succeeded = 0` against an unchanged denominator** — the Success Rate chart
  reads 0% across all of history the moment this ships. That is not wrong data (those runs
  genuinely have no recorded outcome), but presented as two fields it is indistinguishable
  from a real collapse in agent success, and a reader has no way to tell without going
  outside the response to `kandev_meta`.

  With `unclassified` present the discontinuity is legible in the response itself: a day
  where `unclassified` accounts for the whole non-`failed`, non-`other` remainder is a
  pre-activation day, and a day where it is `0` is fully classified. This is the same
  reasoning, and the same additive treatment, already applied to `AgentRunActivityDay`; the
  earlier revision that left this shape at two fields was inconsistent with its own
  principle rather than deliberately excepting it.

  `succeeded + unclassified` is deliberately **not** offered as a compatibility value and no
  consumer should compute it: it would resurrect exactly the finished-as-success count this
  slice removes, because `unclassified` contains pre-activation budget-blocked and
  idle-skipped runs alongside genuine work.

The `Succeeded` field in `office/repository/sqlite/dashboard.go` is sourced from
`task_sessions.state`, not from `runs.status`, and is **not** changed by this
spec.

No new user-visible copy is introduced: the existing charts keep rendering the
fields they already render, and the two new fields are additive on the wire.
Consequently no `t()` string, no locale catalog entry, and no Playwright spec is
added by this card. A later card that surfaces `skipped` or `unclassified` in the
UI owns that copy and that coverage.

`RunCountsByDayForAgent` is pre-existing SQLite-only code (it uses `strftime`).
This spec does not make it dialect-portable; the Postgres parity requirement
under **Migration and activation** binds the migrations, which is what ADR 0027
governs.

**Legacy rows** land in `unclassified`, not `succeeded`. They are not
reinterpreted and not backfilled: the pre-activation series keeps its own bucket,
so the discontinuity is visible in the data rather than silent. The activation
point tells a consumer when `unclassified` should stop growing.

"Visible in the data" is meant literally and is satisfied at **both** grains, which is why
`AgentSuccessRateDay` gains `unclassified` above. A consumer reading either response shape
can separate "this day had no successes" from "this day predates the mechanism" without
consulting `kandev_meta` — the activation instant remains the authoritative answer for
*when* the change happened, but no shape requires it to avoid a false reading.

## Scenarios

### Classification

- **GIVEN** a task whose repository has a non-detached merged pull request,
  **WHEN** the ledger is evaluated, **THEN** the pair records
  `delivery_outcome = pr_merge`, `delivery_basis = provider_pr_merged`,
  `delivery_ref` set to that pull request's URL, and `evidence_rank = 8`.
- **GIVEN** a task whose repository is tracked on GitLab and whose merge request
  has a non-`NULL` `merged_at`, **WHEN** the ledger is evaluated, **THEN** the
  pair records `pr_merge` on the same basis as a GitHub pull request.
- **GIVEN** a task whose only Azure DevOps pull request has
  `status = 'Completed'`, **WHEN** the ledger is evaluated, **THEN** the pair
  records `pr_merge` — the status match is case-insensitive and whitespace-trimmed.
- **GIVEN** a task whose only Azure DevOps pull request has
  `status = 'abandoned'`, and no snapshots, **WHEN** the ledger is evaluated,
  **THEN** the pair records `unknown` with `delivery_basis = provider_pr_unmerged`
  and `evidence_rank = 3`.
- **GIVEN** a task whose only Azure DevOps pull request has an unrecognised
  `status`, and no snapshots, **WHEN** the ledger is evaluated, **THEN** the pair
  records `unknown` with `provider_pr_unmerged` — the status is not guessed at and
  the evaluation does not error.
- **GIVEN** a task whose only session ran on a branch equal to the repository's
  `default_branch` and whose snapshots carry three distinct head commits,
  **WHEN** the ledger is evaluated, **THEN** the pair records
  `delivery_outcome = direct_commit`, `delivery_basis = default_branch_commit`,
  and `reached_default_at` set.
- **GIVEN** a task with snapshots showing `ahead = 12` on a feature branch, no
  provider pull request, and no default-branch observation, **WHEN** the ledger
  is evaluated, **THEN** the pair records `delivery_outcome = unknown` with
  `delivery_basis = branch_commits_unmerged`, `observed_branch_commits = 12` and
  `evidence_rank = 5`.
- **GIVEN** that same task, **WHEN** a later evaluation observes its head commit
  as an ancestor of the default branch, **THEN** `reached_default_at` and
  `reached_default_basis = ancestor_of_default` are set and `delivery_basis`
  becomes `reached_default_unattributed`, while `delivery_outcome` remains
  `unknown`.
- **GIVEN** a task whose every snapshot shows `ahead = 0` and one distinct head
  commit, **WHEN** the ledger is evaluated, **THEN** the pair records
  `no_delivery_observed` with basis `no_commits_observed` and `evidence_rank = 4`.
- **GIVEN** a task whose only snapshot shows `ahead = 0` and an **empty**
  `head_commit`, **WHEN** the ledger is evaluated, **THEN** the pair records
  `no_delivery_observed` — zero distinct non-empty heads satisfies rule 4's
  "at most one", so no pair falls through the rule set.
- **GIVEN** a task whose snapshots carry `ahead = NULL` and `ahead = -1`,
  **WHEN** the ledger is evaluated, **THEN** both are read as `0` and the pair is
  classified as if every snapshot reported `ahead = 0`.
- **GIVEN** a task with a declared repository but no snapshot and no provider
  pull request, **WHEN** the ledger is evaluated, **THEN** the pair records
  `unknown` with basis `no_observations` — not `no_delivery_observed`.
- **GIVEN** a task with no repository at all, **WHEN** the ledger is evaluated,
  **THEN** no ledger row is written for it.
- **GIVEN** a repository whose `default_branch` is empty and a task with
  snapshots showing `ahead = 12`, **WHEN** the ledger is evaluated, **THEN** the
  pair records `delivery_outcome = unknown`,
  `delivery_basis = default_branch_unknown` and `evidence_rank = 1` — the rule's
  own basis is superseded — and **WHEN** `default_branch` is later populated and
  the pair is re-evaluated, **THEN** it is promoted to `branch_commits_unmerged`
  at rank 5.
- **GIVEN** that same degraded pair, **WHEN** `repositories.default_branch` is populated
  and nothing else changes, **THEN** the next sweep selects the pair as due — because
  `repositories.updated_at` advanced and is one of the named due sources — so the
  promotion above is reachable without any other input moving.
- **GIVEN** a repository whose `default_branch` is empty and a merged pull request whose
  `base_branch` is also empty, **WHEN** the pair is evaluated, **THEN**
  `reached_default_at` stays `NULL` — an empty `default_branch` matches no branch, so the
  provider-derived observation is suspended along with rule 2 and ancestry.
- **GIVEN** a repository whose `default_branch` is `NULL` rather than empty, **WHEN** the
  pair is evaluated, **THEN** it takes the same degraded path as an empty string, with
  `delivery_basis = default_branch_unknown` and `evidence_rank = 1`.
- **GIVEN** a task with two sessions on one repository, each on its own inherited branch,
  each reporting a single distinct `head_commit` that differs between the sessions,
  **WHEN** the pair is evaluated, **THEN** no session has two or more distinct heads of
  its own, so the pair is **not** classified `direct_commit`, no ancestry check is
  attempted, and `reached_default_at` stays `NULL` — distinct heads are never pooled
  across sessions.
- **GIVEN** a pair classified by rule 3, 4, 5 or 6, or degraded by
  **Degraded evaluation**, **WHEN** the row is written, **THEN** `delivery_ref` is
  `NULL` — including for `provider_pr_unmerged`, whose provider row URL is evidence of
  non-delivery and is deliberately not recorded as a delivery ref.

### Evidence rank and reclassification

- **GIVEN** a pair whose first snapshot shows `ahead = 0` with one head commit,
  classified `no_delivery_observed` at rank 4, **WHEN** a later snapshot shows
  `ahead = 12` and the pair is re-evaluated, **THEN** the row is promoted to
  `unknown` / `branch_commits_unmerged` at rank 5,
  `delivery_ledger_demotions_suppressed_total` does **not** increment, and
  `observed_branch_commits` reads 12. Mid-session evaluation must not pin a
  working pair at `no_delivery_observed`.
- **GIVEN** a pair classified `unknown` / `provider_pr_unmerged` at rank 3,
  **WHEN** its pull request merges and the pair is re-evaluated, **THEN** the row
  reads `pr_merge` at rank 8.
- **GIVEN** a repository whose `default_branch` is empty and a pair with a merged pull
  request, stored degraded as `pr_merge` / `default_branch_unknown` at rank 1, **WHEN**
  the pull request is detached, every snapshot reports `ahead = 0`, and the pair is
  re-evaluated while `default_branch` is still empty, **THEN** the row reads
  `no_delivery_observed` / `default_branch_unknown` still at rank 1 — the equal-rank write
  happens — **AND** `delivery_ledger_degraded_outcome_changed_total` increments **AND**
  `delivery_ledger_demotions_suppressed_total` does **not** increment **AND** `updated_at`
  advances. Rank 1 admits more than one outcome, so this is neither a promotion nor a
  suppressed demotion, and it must not be silent in both counters.
- **GIVEN** that same degraded pair, **WHEN** `default_branch` is later populated and the
  pair is re-evaluated, **THEN** it is promoted to a rank ≥ 2 classification — a degraded
  row at rank 1 is superseded by any sighted evaluation, whichever outcome it currently
  holds.
- **GIVEN** a pair stored `pr_merge` / `provider_pr_merged` at rank 8 whose `delivery_ref`
  names pull request A, **WHEN** a second merged, non-detached pull request B with an
  **earlier** merge instant is synced and the pair is re-evaluated, **THEN** the rank is
  still 8 and `delivery_outcome` and `delivery_basis` are unchanged, but `delivery_ref`
  becomes B's URL, `updated_at` advances and
  `delivery_ledger_rows_written_total` increments — equal rank guarantees the same
  `(outcome, basis)`, never the same ref.
- **GIVEN** a pair whose `delivery_outcome` is `unknown` and whose `workspace_id` is W1,
  **WHEN** its task is moved to workspace W2 and the pair is re-evaluated, **THEN** the
  ledger row's `workspace_id` reads W2 — it is refreshed on every persisted evaluation and
  is not gated by the rank guard — **AND** the pair was selected as due because
  `tasks.updated_at` advanced.

### Squash-merge, ancestry preconditions and negative evidence

- **GIVEN** a merged pull request whose branch head is not an ancestor of the
  default branch because the merge was squashed, **WHEN** ancestry is evaluated,
  **THEN** the negative result is discarded, no column records `false`, and the
  pair's `pr_merge` classification from rule 1 is unaffected.
- **GIVEN** a pair whose snapshots all report the same single `head_commit` —
  the inherited base commit, which is trivially an ancestor of the default
  branch — **WHEN** the pair is evaluated, **THEN** no ancestry check is
  attempted, `delivery_ledger_ancestry_skipped_total` increments, and
  `reached_default_at` remains `NULL`.
- **GIVEN** a pair reporting `ahead = 4` on an inherited branch whose
  `head_commit` never moved across its snapshots, **WHEN** the pair is
  evaluated, **THEN** the ancestry precondition is still not met and no ancestry
  check is attempted — `ahead > 0` does not satisfy it.
- **GIVEN** a pair one of whose **sessions** carries two distinct head commits among its
  own snapshots, and whose most recent two snapshots share the greatest `created_at`,
  **WHEN** the ancestry head is selected, **THEN** it is the lexicographically greatest
  `head_commit` of those two, and repeated evaluations select the same one. The session
  grain is stated in the GIVEN because selection is restricted to the
  precondition-satisfying session(s); two heads split across two sessions would satisfy no
  session's precondition and no check would be attempted at all.
- **GIVEN** a pair whose session holds two distinct non-empty head commits and whose
  **newest** snapshot by `created_at` carries a whitespace-only `head_commit`, **WHEN** the
  ancestry head is selected, **THEN** the whitespace-only snapshot is skipped and the newest
  **non-empty** head is submitted to the check — an empty value is never sent to git and
  never counted as an ancestry error.
- **GIVEN** a pair classified `direct_commit` whose newest default-branch snapshot carries
  an empty `head_commit`, **WHEN** `delivery_ref` is selected, **THEN** it is the newest
  non-empty `head_commit` among the default-branch snapshots **of the session(s) that
  satisfied rule 2**, and the same value is offered as `reached_default_ref` — no empty
  string reaches either column.
- **GIVEN** a pair with two sessions on one repository — session A, on a feature branch,
  holding two distinct non-empty head commits of its own and therefore satisfying the
  moving-head precondition, and session B, which merely sat on the repository's
  `default_branch` and wrote a **single** snapshot carrying its inherited head, with a
  **later** `created_at` than any of session A's — **WHEN** the ancestry head is selected,
  **THEN** it is session **A's** newest non-empty head and **not** session B's inherited
  head, even though B's snapshot is the newest on the pair. Selection is restricted to the
  precondition-satisfying session(s), so a non-authoring session can never supply the
  commit submitted to `git merge-base --is-ancestor`.
- **GIVEN** that same pair, **WHEN** the evaluation completes, **THEN**
  `reached_default_at` is **not** set from session B's inherited head — an inherited head
  is trivially an ancestor of the default branch, so pair-wide selection would have set the
  write-once triple with `reached_default_basis = ancestor_of_default` on a pair whose only
  authored work is unmerged, and promoted rule 3's basis to
  `reached_default_unattributed`. This is the same false positive the **Ancestry
  precondition** prevents at the gate, and it is prevented here at selection.
- **GIVEN** a pair with two sessions on one repository — session C, which committed on the
  repository's `default_branch` and holds two distinct non-empty heads there, and session
  D, which sat on the same `default_branch` holding a **single** inherited head whose
  snapshot has a **later** `created_at` — **WHEN** `delivery_ref` is selected for the
  resulting `direct_commit` classification, **THEN** it is session **C's** newest non-empty
  head, and the same value is offered as `reached_default_ref`. Only the session(s)
  satisfying rule 2's own predicate contribute candidate heads, so session D's inherited
  head reaches neither column.
- **GIVEN** a repository whose local checkout is missing, **WHEN** ancestry is
  evaluated, **THEN** `delivery_ledger_ancestry_errors_total` increments,
  `reached_default_at` stays `NULL`, `delivery_outcome` is unchanged, and the
  rest of the classification is still persisted.
- **GIVEN** a repository whose `refs/remotes/origin/<default_branch>` does not
  exist but whose local branch `<default_branch>` does, **WHEN** ancestry is
  evaluated, **THEN** the local branch is used and the check proceeds.
- **GIVEN** a provider-cloned repository whose `local_path` is empty, **WHEN** ancestry is
  evaluated, **THEN** the checkout-resolution port returns an error rather than a path,
  `delivery_ledger_ancestry_errors_total` increments, `reached_default_at` stays `NULL`,
  and the rest of the classification is still persisted.
- **GIVEN** a repository whose stored `local_path` canonicalizes to a different location
  than the one saved, **WHEN** ancestry is evaluated, **THEN** the port returns an error and
  no git command is run against the resolved location — the evaluator inherits that
  rejection instead of implementing its own path check.
- **GIVEN** a pair with a merged pull request whose base branch is the default branch,
  snapshots committing on the default branch, and a head commit that is an ancestor of
  the default branch — all three observable in one evaluation — **WHEN**
  `reached_default_at` is first written, **THEN** `reached_default_basis` is
  `provider_pr_merged` and `reached_default_ref` is that pull request's URL, because
  provider merge outranks both other bases; and repeated evaluations of the same inputs
  select the same basis and ref.
- **GIVEN** a pair whose only default-branch evidence is a positive ancestry check,
  **WHEN** `reached_default_at` is written, **THEN** `reached_default_basis` is
  `ancestor_of_default` and `reached_default_ref` is the commit submitted to the check.
- **GIVEN** a repository whose `default_branch` is `main`, and a pair with two merged,
  non-detached pull requests — request A into `release/1.2` with the **earlier** merge
  instant, and request B into `main` — **WHEN** the pair is evaluated, **THEN**
  `delivery_ref` is **A's** URL (the `pr_merge` ref rule does not filter on base branch)
  while `reached_default_ref` is **B's** URL and `reached_default_basis` is
  `provider_pr_merged` (the observation ref rule selects only rows whose base branch
  equals `default_branch`). The two refs are drawn from different row sets and must not be
  the same selection.
- **GIVEN** a pair whose only merged pull request is **detached** and whose base branch is
  the repository's `default_branch`, plus snapshots showing `ahead > 0`, **WHEN** the pair
  is evaluated, **THEN** rule 1 does not match, no `provider_pr_merged` observation is
  offered, `reached_default_at` stays `NULL`, and the pair classifies `unknown` /
  `branch_commits_unmerged` at rank 5 — a detached row is excluded from the observation
  exactly as it is from rule 1.

### Idempotency, ordering and concurrency

- **GIVEN** a ledger row classified `pr_merge`, **WHEN** it is evaluated again
  with unchanged inputs, **THEN** every classification and observation column is
  unchanged, `updated_at` is unchanged, and `last_evaluated_at` and
  `evaluation_seq` advance.
- **GIVEN** a pair with two merged pull requests, **WHEN** `delivery_ref` is
  selected, **THEN** it is the one with the earliest merge instant, and on an
  exact tie the one with the lower request number.
- **GIVEN** a pair with one merged GitHub pull request and one completed Azure
  DevOps pull request, **WHEN** `delivery_ref` is selected, **THEN** the GitHub
  row's `merged_at` and the Azure row's `updated_at` are compared as merge
  instants, and on an exact tie at equal request numbers `azure_devops` sorts
  before `github`.
- **GIVEN** a pair with two default-branch snapshots sharing the same
  `created_at`, **WHEN** `delivery_ref` is selected, **THEN** it is the
  lexicographically greatest `head_commit` of the two, and repeated evaluations
  select the same one.
- **GIVEN** a row classified `pr_merge` at rank 8 with
  `observed_branch_commits = 12`, **WHEN** the pull request is detached and the
  pair is re-evaluated so the computed evidence ranks lower, **THEN** the stored
  outcome, basis, ref and rank are all unchanged,
  `delivery_ledger_demotions_suppressed_total` increments, and
  `observed_branch_commits` still reads 12.
- **GIVEN** a row classified `pr_merge` whose snapshots are all deleted with
  their session, **WHEN** the pair is re-evaluated, **THEN**
  `observed_branch_commits` does not decrease.
- **GIVEN** two evaluators writing the same pair concurrently, one computing
  rank 1 and one computing rank 8, **WHEN** both statements commit in either
  order, **THEN** the row reads `pr_merge` at rank 8.
- **GIVEN** a row whose `reached_default_at` is `NULL` and two concurrent
  evaluations each supplying a different first observation, **WHEN** both commit,
  **THEN** `reached_default_at`, `reached_default_basis` and `reached_default_ref`
  all come from the same one of the two — the triple is never mixed.
- **GIVEN** a row whose `reached_default_at` is set, **WHEN** a later evaluation
  observes the default branch again through a different basis, **THEN**
  `reached_default_at` and `reached_default_basis` are unchanged.
- **GIVEN** a pair with no ledger row and no snapshot and no provider row,
  **WHEN** a sweep runs, **THEN** the pair is selected as due and a row is
  written with basis `no_observations`; **AND WHEN** the next sweep runs with
  nothing changed, **THEN** the pair is not selected again.
- **GIVEN** an evaluation whose snapshot query returns an error, **WHEN** the
  pair is evaluated, **THEN** no column is written — including
  `last_evaluated_at` — `delivery_ledger_evaluation_errors_total` increments,
  `delivery_ledger_evaluations_total` does **not** increment, and
  the pair is still due on the next sweep.
- **GIVEN** a row whose stored `observed_branch_commits` is `NULL` and an incoming value
  of 7, **WHEN** the upsert runs, **THEN** the column reads 7 — a `NULL` stored value is
  lower than any incoming value and must not swallow it.
- **GIVEN** a pair whose inputs have not changed, **WHEN** it is re-evaluated, **THEN**
  `delivery_ledger_evaluations_total` increments but `delivery_ledger_rows_written_total`
  does **not**, because no classification or observation column changed and `updated_at`
  did not advance.
- **GIVEN** a pair with two merged GitLab merge requests carrying the same `mr_iid` under
  different `project_path` values, **WHEN** `delivery_ref` is selected, **THEN** the
  lower `project_path` wins and repeated evaluations select the same URL.
- **GIVEN** a sweep pass that encounters the same three sessions with empty
  `repository_id` on two consecutive passes, **WHEN** each pass publishes, **THEN**
  `delivery_ledger_sessions_unattributed_total` reads 3 after both — it is a gauge set per
  pass, not a running total.
- **GIVEN** a repository that is soft-deleted, **WHEN** sweeps run, **THEN** its pairs'
  `last_evaluated_at` stops advancing and their existing rows are retained unchanged.
- **GIVEN** a soft-deleted repository with a pair whose `reached_default_at` is `NULL` and
  whose `last_evaluated_at` is three days old, **WHEN** sweeps run, **THEN** the pair is
  **not** selected — the freeze is evaluated before the three due conditions and overrides
  stale refresh.
- **GIVEN** a soft-deleted repository with a candidate pair that has **no ledger row**,
  **WHEN** sweeps run, **THEN** no row is written and the pair is not selected on any
  pass — the freeze also overrides the unconditionally-due rule.
- **GIVEN** that same soft-deleted repository, **WHEN** `deleted_at` is cleared, **THEN**
  `repositories.updated_at` advances and the pair is selected as due on the next sweep.
- **GIVEN** an archived task with a live repository, **WHEN** sweeps run, **THEN** its
  pairs remain candidates and remain **eligible** for selection under the same three due
  conditions as any other pair — archiving a task neither excludes it from candidacy nor
  freezes it, and no predicate anywhere reads `tasks.archived_at`.
- **GIVEN** that same archived task, **WHEN** one of its sessions writes a new git snapshot,
  **THEN** the pair is selected as due on the next sweep and `last_evaluated_at` advances —
  archived pairs are re-evaluated on input movement exactly like unarchived ones.
- **GIVEN** that same archived task with `reached_default_at` already set and no input row
  moving, **WHEN** sweeps run, **THEN** the pair is **not** selected and
  `last_evaluated_at` does **not** advance — it is due under none of the three conditions,
  which is correct and is not a freeze. Eligibility is what archiving preserves; continuous
  advance is not something any pair gets.
- **GIVEN** an evaluation that begins reading its inputs at T0 and commits its upsert at
  T2, and a snapshot for the same pair written at T1 with T0 < T1 < T2, **WHEN** the
  upsert commits, **THEN** the stored `last_evaluated_at` is **T0**, so
  `last_evaluated_at < MAX(input.created_at)` still holds and the pair is selected as due
  on the next sweep. Recording T2 would mask the T1 snapshot permanently.
- **GIVEN** a pair whose evaluation began at T0 and a concurrent, faster evaluation of the
  same pair that began at T3 > T0 and committed first, **WHEN** the slower evaluation
  commits, **THEN** `last_evaluated_at` still reads T3 — the high-water rule means a slow
  evaluation cannot rewind the column.
- **GIVEN** a pair with no snapshots at all, **WHEN** it is evaluated, **THEN**
  `observed_branch_commits` is `NULL` and not `0`.
- **GIVEN** a pair whose stored `observed_branch_commits` is 12 and whose snapshots have
  all been deleted, **WHEN** it is re-evaluated so the incoming value is `NULL`, **THEN**
  the column still reads 12.
- **GIVEN** a candidate pair whose non-empty `repository_id` matches no row in
  `repositories`, **WHEN** a sweep runs, **THEN** no ledger row is written for it, no
  upsert is attempted, `delivery_ledger_pairs_missing_repository_total` counts it for that
  pass, and `delivery_ledger_write_errors_total` does **not** increment — the pair is
  excluded from candidacy rather than attempted and failed.
- **GIVEN** that same pair, **WHEN** the missing `repositories` row is later created,
  **THEN** the pair becomes a candidate on the next sweep and a ledger row is written.
- **GIVEN** a `github_task_prs` row whose `task_id` matches no row in `tasks` — the task was
  hard-deleted and the provider row was not pruned with it — **WHEN** a sweep runs, **THEN**
  no ledger row is written for that pair, no upsert is attempted,
  `delivery_ledger_pairs_missing_task_total` counts it for that pass, and
  `delivery_ledger_write_errors_total` does **not** increment — so the primary
  migration-failure signal keeps its resting value of `0` on a healthy schema.
- **GIVEN** that same orphaned provider row, **WHEN** two consecutive sweep passes run with
  nothing changed, **THEN** `delivery_ledger_pairs_missing_task_total` reads the same count
  after both — it is a gauge set per pass, not a running total — and neither pass writes a
  row or logs `delivery_ledger.write_failed`.
- **GIVEN** a candidate pair rejected for a missing `tasks` row, **WHEN** the sweep orders
  its due pairs, **THEN** the pair is absent from the ordering entirely rather than sorting
  on a `NULL` `tasks.created_at`, and the exclusion is observable through the gauge rather
  than being an invisible join drop.
- **GIVEN** a ledger table that does not exist because its migration was swallowed,
  **WHEN** a sweep pass runs, **THEN** candidacy is still computed from the input tables,
  the dueness read fails, **every candidate is treated as due**,
  `delivery_ledger.dueness_unavailable` is logged exactly **once** for that pass,
  `delivery_ledger_write_errors_total` increments once per pair whose upsert failed,
  `delivery_ledger.write_failed` is logged exactly **once** for that pass,
  `delivery_ledger_evaluation_errors_total` does **not** increment, and the pass still
  reaches its last candidate rather than aborting on the first failure.
- **GIVEN** that same missing table and a candidate pair whose repository is soft-deleted,
  **WHEN** the pass runs under the dueness fallback, **THEN** that pair is still **not**
  evaluated and no upsert is attempted for it — the fallback widens dueness only, and the
  freeze is evaluated ahead of it.
- **GIVEN** that same missing table and a candidate pair whose `task_id` matches no `tasks`
  row, **WHEN** the pass runs under the dueness fallback, **THEN** the pair is excluded by
  **Candidate pairs** condition 3 and counted in
  `delivery_ledger_pairs_missing_task_total`, and no upsert is attempted for it — so it
  contributes nothing to `delivery_ledger_write_errors_total`.
- **GIVEN** a healthy ledger table and a **transient** failure of the dueness read on one
  pass, **WHEN** that pass runs, **THEN** every candidate is evaluated once, every upsert
  succeeds, `delivery_ledger_write_errors_total` does **not** increment, no classification
  or observation column changes value for a pair whose inputs did not move, and the next
  pass computes dueness normally.
- **GIVEN** that same missing table, **WHEN** the sweep publishes at the end of the pass,
  **THEN** `delivery_ledger_last_evaluated_unix` reads `-1` and
  `delivery_ledger_stall_seconds` reads `-1`, and `delivery_ledger.unavailable` is logged
  — **not** `0`/`0`, which is the healthy-and-empty state and would make a broken ledger
  indistinguishable from a fresh one.
- **GIVEN** a boot at which the `task_delivery_ledger` migration failed, so
  `telemetry.delivery_ledger.activated_at` is absent from `kandev_meta`, **WHEN** the
  backend runs, **THEN** the sweep still starts and still attempts its upserts — the
  activation key does not gate the writer, and gating it would silence the very counters
  that detect the failed migration.
- **GIVEN** a pair whose ancestry check errored and whose `reached_default_at` is
  therefore `NULL`, and no input row moves afterwards, **WHEN** 24 hours pass, **THEN**
  the pair is selected as due by the stale-refresh condition and the ancestry check is
  attempted again.
- **GIVEN** a pair whose `reached_default_at` is `NULL`, whose session authored two or more
  distinct non-empty head commits so the ancestry precondition is met, and whose selected
  head commit **is** an ancestor of `refs/remotes/origin/<default_branch>` in the
  repository's local checkout — a merge that landed long after the task's last session
  ended, moving no row in any Kandev table — **WHEN** the next stale refresh evaluates it,
  **THEN** `reached_default_at` is set, `reached_default_basis` is `ancestor_of_default`,
  and `reached_default_ref` is that commit. The ancestry relation is stated as a
  precondition rather than assumed: this repository squash-merges by policy, so a merged
  branch head is normally **not** an ancestor (see **Evidence**, finding 2), and a test that
  omits the relation is asserting nothing.
- **GIVEN** that same pair where the merge was **squashed**, so the selected head is not an
  ancestor of the default branch, **WHEN** the next stale refresh evaluates it, **THEN** the
  negative result is discarded, `reached_default_at` stays `NULL`, and the pair is selected
  again by stale refresh 24 hours later — the condition retries indefinitely while the
  observation is missing, and that is the intended cost of not being able to see a squash.
- **GIVEN** a pair whose `reached_default_at` is already set, **WHEN** 24 hours pass with
  no input movement, **THEN** it is **not** selected by the stale-refresh condition — the
  condition applies only while the observation is missing.
- **GIVEN** a pair evaluated by a sweep pass, **WHEN** the next sweep runs 5 minutes after
  that pass **finished**, **THEN** at most one pass is ever in flight, so a pass that
  evaluates that pair advances its `evaluation_seq` by exactly **one, never two**, and each
  per-pass gauge is set exactly once for that pass. This says nothing about whether the
  pair is evaluated at all on a given pass — a pair that is not due is not evaluated and
  its `evaluation_seq` does not move, per the dueness scenarios above. The guarantee is
  against double-counting within a pass, not a promise of one increment per pass.

### Migration and activation

- **GIVEN** a database created before this feature, **WHEN** the backend boots,
  **THEN** `task_delivery_ledger` exists, `runs.outcome` exists and is `NULL` on
  every pre-existing row, and no pre-existing row's values change.
- **GIVEN** the backend boots twice against the same database, **WHEN**
  migrations replay, **THEN** they apply cleanly the second time and
  `telemetry.delivery_ledger.activated_at` retains the value written on the
  first boot.
- **GIVEN** a Postgres deployment, **WHEN** the same migrations run fresh and
  then replay, **THEN** both succeed and duplicate-object errors are classified
  through `internal/db`.
- **GIVEN** a boot in which the `task_delivery_ledger` migration fails and the runner
  swallows the error at WARN, **WHEN** the boot completes, **THEN**
  `telemetry.delivery_ledger.activated_at` is **absent** from `kandev_meta`, because the
  schema probe found no table; **AND WHEN** a later boot applies the migration
  successfully, **THEN** the key is written with that boot's instant.
- **GIVEN** a boot in which the `runs.outcome` migration fails, **WHEN** the boot
  completes, **THEN** `telemetry.run_outcome.activated_at` is absent for the same reason,
  so no consumer reads the mechanism as live while the column is missing.
- **GIVEN** an activated database, **WHEN** a database reset runs, **THEN**
  `telemetry.delivery_ledger.activated_at` and `telemetry.run_outcome.activated_at`
  are absent from `kandev_meta` afterwards, and **WHEN** the backend next boots,
  **THEN** both are rewritten with the new instant.
- **GIVEN** an activated database, **WHEN** a reset deletes both activation keys and then
  fails partway through dropping user tables, **THEN** `task_delivery_ledger` rows may
  remain but both keys are absent — so no consumer reads the mechanism as live over data
  it cannot trust — and **WHEN** the backend next boots and the schema probe succeeds,
  **THEN** the keys are rewritten with that boot's instant.
- **GIVEN** a reset in which the activation-key deletion itself fails, **WHEN** the reset
  runs, **THEN** it fails and reports the failure, and **no** user table is dropped — the
  keys are never left pointing at data that a later step removed.
- **GIVEN** a task created before `telemetry.delivery_ledger.activated_at` that
  has no ledger row, **WHEN** the database is read, **THEN** the two facts are
  separately observable — the activation instant from `kandev_meta` and the
  absence of a row from `task_delivery_ledger` — so "not observed by this
  mechanism" is distinguishable from `no_delivery_observed` without inspecting
  any consumer.

### Office run outcome

- **GIVEN** a run blocked by the pre-execution budget check, **WHEN** it
  finishes, **THEN** `runs.status = 'finished'` and `runs.outcome =
  'budget_blocked'`.
- **GIVEN** a run skipped because the agent is not active, **WHEN** it finishes,
  **THEN** `runs.outcome = 'agent_inactive'` — even though that path writes no
  `office_activity_log` row.
- **GIVEN** a run held by an active task-tree hold, **WHEN** it finishes,
  **THEN** its outcome is `task_tree_held`.
- **GIVEN** a run whose task checkout fails transiently, **WHEN** the scheduler
  handles it, **THEN** it is retried or escalated after the retry budget and is
  not finished as completed work.
- **GIVEN** a run whose task is already checked out, **WHEN** the scheduler
  handles it, **THEN** it is requeued for retry and is not finished as completed
  work.
- **GIVEN** a task-bearing run whose agent was launched by the task starter, **WHEN** the
  agent-completed subscriber finishes it, **THEN** `runs.outcome = 'processed'` and it is
  not left `NULL` after activation.
- **GIVEN** a taskless run, **WHEN** the scheduler cannot launch it, **THEN** it is
  marked `failed` with a diagnostic error and does not report completed work.
- **GIVEN** a task-bearing run in a deployment with no task starter configured, **WHEN**
  the scheduler handles it, **THEN** it is marked `failed` with a diagnostic error,
  its checkout is released, and the agent is not launched.
- **GIVEN** two terminal callers reaching the same run row, **WHEN** both write, **THEN**
  the row's `status` and `outcome` both come from the later write — they are set in one
  statement and can never disagree.
- **GIVEN** a run that fails, **WHEN** `FailRun` transitions it through the shared
  `transitionRunTerminal` statement, **THEN** `runs.status = 'failed'` and
  `runs.outcome IS NULL` — the failed path writes `NULL` in the same statement, and no
  value from the five-value vocabulary is invented for it.
- **GIVEN** a day containing one failed run written that way, **WHEN**
  `RunCountsByDayForAgent` reports that day, **THEN** it counts in `failed` and in
  neither `unclassified` nor `skipped` — the `NULL` outcome is never read, because the
  three outcome-derived buckets are all conditioned on `status = 'finished'`.
- **GIVEN** a run whose `FinishRun` call fails and whose error the call site discards,
  **WHEN** the run is later inspected, **THEN** it is **not** `finished`, so it counts in
  neither `succeeded` nor `skipped` nor `unclassified`.
- **GIVEN** a run transitioned to a terminal status through
  `office/scheduler.SchedulerService.FailRun`, which reaches the runs repository's
  `FinishRun` without passing through `transitionRunTerminal`, **WHEN** the row is
  inspected, **THEN** `runs.status = 'failed'` and `runs.outcome IS NULL`, and the row
  counts in `failed` and in neither `unclassified` nor `skipped`.
- **GIVEN** a run transitioned through `office/scheduler.SchedulerService.FinishRun`,
  **WHEN** the row is inspected, **THEN** `runs.status = 'finished'` and
  `runs.outcome IS NULL`, so it counts in `unclassified` and **not** in `succeeded` — a
  terminal path with no established semantic label is never counted as a success.
- **GIVEN** a day containing one processed run, one budget-blocked run, one
  failed run and one pre-activation `finished` run with `outcome IS NULL`,
  **WHEN** `RunCountsByDayForAgent` reports that day, **THEN** it returns
  `succeeded = 1`, `skipped = 1`, `unclassified = 1`, `failed = 1` and
  `other = 0`.
- **GIVEN** a `finished` run whose `outcome` holds a value outside the five
 named ones, **WHEN** that day is reported, **THEN** it counts in `skipped` —
  the bucketing is total and no query errors.
- **GIVEN** that same day, **WHEN** the agent dashboard renders it, **THEN**
  `AgentRunActivityDay.total` equals
  `succeeded + skipped + unclassified + failed + other`, and
  `AgentSuccessRateDay` reports `succeeded = 1` and `unclassified = 1` against the same
  total.
- **GIVEN** a day made up **entirely** of pre-activation `finished` runs with
  `outcome IS NULL` — three of them, and nothing else — **WHEN**
  `RunCountsByDayForAgent` reports that day, **THEN** `succeeded = 0`, `skipped = 0`,
  `unclassified = 3`, `failed = 0`, `other = 0`; **AND WHEN** the dashboard renders it,
  **THEN** `AgentSuccessRateDay` reports `succeeded = 0`, `unclassified = 3` and
  `total = 3`, so a reader can tell from the response alone that the day predates the
  mechanism rather than recording three failures to succeed.
- **GIVEN** a day made up entirely of post-activation runs, **WHEN** the dashboard renders
  it, **THEN** `AgentSuccessRateDay.unclassified` is `0` — the field is the marker for the
  pre-activation series and reads zero once every terminal run carries an outcome.

### Writer health

- **GIVEN** the ledger writer has not evaluated any pair for longer than its
  15-minute stall threshold while task sessions continue to complete, **WHEN**
  the sweep completes a pass, **THEN** `delivery_ledger_stall_seconds` exceeds
  900 and a WARN log with the message `delivery_ledger.stalled` is emitted once
  for that pass.
- **GIVEN** the backend restarts, **WHEN** the first sweep after the restart
  completes, **THEN** `delivery_ledger_last_evaluated_unix` reports the true
  `MAX(last_evaluated_at)` from the database even though every counter has reset
  to zero.
- **GIVEN** an empty ledger table, **WHEN** the sweep publishes,
  **THEN** `delivery_ledger_last_evaluated_unix` is `0` and
  `delivery_ledger_stall_seconds` is `0` — an empty table is not reported as an
  infinite stall.
- **GIVEN** a ledger holding evaluated rows and a database in which **no** session survives
  the comparand filters — every session has an empty `repository_id`, or belongs to a
  soft-deleted repository, or there are no sessions at all — **WHEN** the sweep publishes,
  **THEN** `delivery_ledger_last_evaluated_unix` reports the true
  `MAX(last_evaluated_at)` and `delivery_ledger_stall_seconds` is `0`, and no
  `delivery_ledger.stalled` WARN is emitted — an empty comparand means there is no work to
  be behind on, not that the writer is stalled.
- **GIVEN** a ledger holding evaluated rows and a comparand query that errors while the
  ledger query succeeds, **WHEN** the sweep publishes, **THEN**
  `delivery_ledger_last_evaluated_unix` reports the true `MAX(last_evaluated_at)`,
  `delivery_ledger_stall_seconds` reads `-1`, and
  `delivery_ledger.comparand_unavailable` is logged once for that pass — the stall figure
  is unknown rather than zero, so a repeatedly failing comparand read can never report a
  healthy writer.
- **GIVEN** a healthy writer that has evaluated every due pair, and a session with an
  empty `repository_id` that updated one hour ago, **WHEN** the sweep publishes, **THEN**
  `delivery_ledger_stall_seconds` is `0` and no `delivery_ledger.stalled` WARN is emitted
  — an unattributed session contributes to no pair and must not drive the comparand.
- **GIVEN** a healthy writer and a session belonging to a soft-deleted repository that
  updated one hour ago, **WHEN** the sweep publishes, **THEN**
  `delivery_ledger_stall_seconds` is `0` — that pair's evaluation is frozen by design.
- **GIVEN** a writer that has stopped, and an **archived** task whose session updated
  twenty minutes ago, **WHEN** the sweep publishes, **THEN**
  `delivery_ledger_stall_seconds` exceeds 900 and `delivery_ledger.stalled` is emitted —
  archived tasks are still evaluated, so their sessions still count toward the comparand
  and a stall on them is a real stall.

## Out of scope

Each exclusion below is a contract, not an omission.

- **The extract itself.** The downstream point-in-time extract that reads this
  table is external to this repository; no extract code exists here and none is
  added. This spec's obligation is to make the ledger *readable without
  ambiguity* — the activation instant in `kandev_meta`, `NULL` rather than `0`
  for legacy rows, and `evaluation_seq` — and every scenario above is stated as
  an observable property of the database, never of a consumer.
- **A structured stop reason for a task or session.** `task_sessions` carries
  free-text `error_message` ("task archived", "task tree archived", raw
  rate-limit JSON) and a `state` enum, and nothing else. This is a different
  grain (session, not run), a different table, and its enum cannot be derived
  without enumerating every writer of `error_message` and every `state`
  transition. It gets its own card. The `runs.outcome` contract in this spec does
  **not** cover it and must not be extended to `task_sessions`.
- **Consuming the GitHub push webhook for merge inference.** The
  `push_webhook_default` basis is defined in the vocabulary so a later card can
  populate it without a schema change, but no subscriber is added here. The
  webhook fires only for GitHub App installations; 10 of the 12 repositories in
  the reference store have no provider at all, so it can never be the primary
  mechanism.
- **Reconstructing history.** No ledger column is ever backdated or inferred from
  evidence the mechanism did not itself observe. `first_classified_at` and
  `reached_default_at` are the instants *this* mechanism made the observation, never the
  instant the underlying work happened, so a pair whose branch merged months before
  activation records the observation instant and not the merge. There is no import, no
  reconstruction from `office_activity_log`, and no derivation from a provider's history
  API.

  This is **not** a claim that old pairs go unevaluated. Every pre-existing
  `(task, repository)` pair is a candidate, has no ledger row, and is therefore due on
  the first sweep after activation and classified from whatever evidence is still
  present — see **Sweep selection predicate**. Those two statements were previously in
  tension in this document; the distinction that resolves them is *reconstruction* versus
  *evaluation*. Classifying an old pair from evidence visible today is ordinary
  evaluation. Asserting when that pair delivered, from evidence the ledger never saw, is
  reconstruction, and only reconstruction is excluded. The activation instant in
  `kandev_meta` is what lets a consumer tell the two apart: every row is timestamped at or
  after it.
- **Making `RunCountsByDayForAgent` dialect-portable.** It is pre-existing
  SQLite-only code; this card reshapes its buckets and nothing else.
- **Retiring the dormant `office/scheduler.SchedulerService.FinishRun` / `.FailRun`
  pair.** They call the runs repository's `FinishRun` directly, bypassing
  `transitionRunTerminal` and therefore also bypassing the `OfficeRunProcessed` publish
  that the service-layer path performs. This card threads `outcome` through them (both pass
  `NULL`, see **Office run outcome**) because the shared signature change forces the
  compiler to reach them, and stops there. Deciding whether a second terminal-transition
  path should exist at all — and whether the missing bus publish is a defect — is a
  behavioural question about run lifecycle, not about telemetry, and it gets its own card.
- **The second `status = 'finished'`-as-success consumer.**
  `apps/backend/internal/office/summary/inputs.go` counts `r.status = 'finished'` as
  `completed` when it assembles continuation summaries for an agent. It has exactly the
  defect this card fixes in `RunCountsByDayForAgent` — a budget-blocked or idle-skipped
  run reads as completed work — and it is **deliberately not changed here**. It is a
  different consumer at a different grain, feeding agent prompt context rather than a
  dashboard, and changing what an agent is told about its own history is a behavioural
  change that deserves its own card and its own review. It is named here rather than left
  silent so that after this card ships, nobody concludes from `## Why` that every
  finished-as-success reader has been corrected. It gets its own card.
- **Distinguishing a research card from a chore.** The ledger separates delivery
  from non-delivery. It does not classify intent, and
  `no_delivery_observed` is not a judgement about whether the card should have
  delivered.
- **Attributing delivery to a session, a turn, or a workflow step.** The ledger's
  grain is `(task, repository)`. A session's snapshots attribute to that
  session's single `task_sessions.repository_id`; a session spanning more than
  one repository is not modelled.
- **Deduplicating delivery credit when one pull request serves several tasks.**
  Both tasks read `pr_merge`.
- **Recording a closed pull request's disposition** (`superseded`, `duplicate`,
  `withdrawn`). That is a separate card against the provider tables.
- **Any user-visible surface.** No board column, badge, filter, dashboard tile,
  or settings control is added, and no new copy is rendered. If a later card
  surfaces the ledger or the new run-outcome buckets in the web UI, that copy
  goes through `t()` and that card owns the E2E coverage.
- **Changing `runs.status` semantics**, or changing what any existing consumer of
  `runs.status` reports.
- **Populating `task_session_commits`, `pre_commit_snapshot_id`, or
  `post_commit_snapshot_id`.** Those are separate cards and this spec does not
  depend on them.
