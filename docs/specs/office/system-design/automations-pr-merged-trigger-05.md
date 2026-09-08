---
status: draft
system: office
requirements:
  - REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001
created: 2026-08-09
updated: 2026-08-09
owners:
  - nova28
---
# Automations — "Pull request merged" trigger System Design Part 5

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Failure modes

| Condition | Behavior |
|---|---|
| Event payload is nil, malformed, or neither a typed `TaskPR` nor its NATS JSON object form | Ignored; no trigger evaluated. |
| Listing enabled `github_pr_merged` triggers fails | Logged at error; the event is dropped. No retry. |
| A trigger's `config` JSON is invalid | That trigger is skipped with a debug log; the other triggers for the same event are still evaluated. |
| The trigger's automation cannot be loaded **at fire time** (inside `FireTrigger`, synchronously, before any row is written) | That trigger is skipped with a debug log and **no run row of any status** — `FireTrigger` returns a skip result the subscriber discards. This is the *first* of two stages at which "automation not found" can be observed; the next row is the other. |
| The trigger's automation cannot be loaded **at task-creation time** (the orchestrator's later, asynchronous reload on its own goroutine) | `recordFailedRun` writes a **`failed`** run row carrying an **empty** `dedup_key`, so the run log *does* show a row for this one. Both stages fail closed and create no task; they differ only in whether a row is visible. Named because the write-site table's third row lists "automation not found" among `recordFailedRun`'s call sites, and without the stage split that reads as contradicting the row above. |
| Task lookup cannot resolve the task (row missing or deleted) | Fail closed: no trigger is listed and no run row is written. Logged at **debug** — an ordinary outcome. |
| Task lookup fails for an infrastructural reason (query error) | The adapter collapses it to `ok == false`, so it fails closed identically — but it is logged at **warn** with the task id and the underlying error, because nothing else records that a merge was dropped for a reason that was not the data's fault. See [Task lookup](#task-lookup). |
| No task lookup wired | Fail closed for each event. **The subscriber still subscribes**, and its `Start` emits exactly one `warn` naming `github_pr_merged`; if the lookup is wired later, the next event is evaluated normally. |
| The orchestrator or automation subscriber starts **after** the GitHub poller | Unsupported configuration. The poller's immediate startup sweep can publish into a chain with a missing consumer, so a merge that landed during the outage is dropped and may never republish. See [Bus subscription](#start-up-ordering-is-part-of-this-contract-not-an-implementation-detail). |
| `PRNumber <= 0`, or `Owner` / `Repo` empty | Fail closed at the per-event gates; no trigger is listed. |
| `PRURL` empty | Not a gate. The trigger fires and `{{data.pr_url}}` renders as an empty string. |
| `FireTrigger` returns an error (dedup-query failure, cap-count failure, publish failure) | Logged at error. **Not retried within this event**, and **no run row of any kind is written** — these paths return before any row is created, so the run log shows nothing. A later `github.task_pr.updated` for the same row is admitted, because no task was created and therefore the key was never consumed (see [Idempotency](#which-runs-consume-the-dedup-key)) — but no such event is guaranteed to arrive. **Manual Run is not a recourse for this trigger type** (next row). |
| The operator clicks Run on an automation carrying this trigger | Manual runs fire with trigger type `manual` and trigger data `{"source":"manual"}` — there is **no `task_id`**, and the interpolator strips the unresolved `{{data.task_id}}` to an empty string. The pinned default prompt handles this: the agent reports that no task id was supplied and stops without calling the tool. Manual Run therefore cannot replay a missed merge, and must not be documented as a recovery path. |
| The target task is already archived | The agent's `archive_task_kandev` call succeeds with `already_archived: true`. The run succeeds. |
| The target task is moved to a **different workspace** after gate 8 but before the agent's tool call | The archive may be denied: in-session MCP scope resolves the **run task's** workspace owner, and the target now lives elsewhere. The agent reports the error; the **run still reads `succeeded`** if the turn ended cleanly, per [Run task shape](#run-task-shape). Nothing else is archived. Not defended against — gate 8 is a point-in-time check and `tasks.workspace_id` is writable; see [Permissions](#permissions). The window is one agent turn and the outcome is a no-op, so no re-check is added. |
| The target task has been deleted | The agent's `archive_task_kandev` call fails and the agent reports the error. The **run is still recorded as `succeeded`** if the turn itself ended cleanly — run status reflects the turn, never the archive (see [Run task shape](#run-task-shape)). Nothing else is archived; the transcript is the evidence. |
| A review watch with `cleanup_policy: auto` deletes the target **before** the per-event task lookup | The lookup cannot resolve the task, so gate 6 fails, **no trigger is listed and no run row is created at all**. There is no tool call and nothing to succeed or fail. This is the more likely of the two phases, because both subsystems react to the same merge and the watch does not have to create a task first. See [Why](#why). |
| A review watch with `cleanup_policy: auto` deletes the target **after** the run has started | The lookup succeeded, so a run task exists and an agent is running. The agent's `archive_task_kandev` call then fails and it reports the error; the run is still recorded as `succeeded` if the turn ended cleanly. Same observable as the deleted-target row above. |
| A `github_pr_merged` run asks to archive a task other than its persisted event target, or its binding metadata is missing/malformed | Rejected before mutation; no task is archived. The tool returns an error and the transcript records it. |
| The automation is at `max_concurrent_runs` | A `skipped` run row is recorded with the cap as its reason, carrying an **empty** `dedup_key` — so no task was created and the key is not consumed, and a later event for the same PR is admitted. If no later event arrives, the task is never archived and there is no automatic recovery; this is the accepted residual recorded in [Idempotency](#which-runs-consume-the-dedup-key). |
| The automation is at `max_concurrent_runs` and further matching events keep arriving | **Each one writes another `skipped` row.** Nothing collapses them, because the rows carry no key to match on. The run log shows several identical skip rows for one pull request and the summary reads `skipped` until **a later firing records a row** — freeing the cap slot writes nothing and recomputes nothing, so the summary does not clear on its own. Accepted, and bounded by the number of post-merge sync events for that PR — see [Idempotency](#which-runs-consume-the-dedup-key). |
| The workspace has no repository | The firing is recorded as a `failed` run with "no repository available", carrying an **empty** `dedup_key` — no task was created, so the key is not consumed. |
| The run created a task and then failed (auto-start aborted, or the agent's turn errored) | `MarkRunFailedByTaskID` flips the existing `task_created` row to `failed`. That row **already carries the key**, so the key stays consumed and the firing is **not** retried. Deliberate: a task was created and an agent was launched. See [Idempotency](#which-runs-consume-the-dedup-key). |
| The task was created but its run row could not be written (`recordSuccessRun` fails) | The orchestrator logs at error, calls `deleteAbandonedTask` to remove the task it just created, and returns. **No run row of any status exists**, so the run log shows nothing and **the dedup key is not consumed** — a later `github.task_pr.updated` for the same row is admitted and evaluated normally. This is the correct outcome: no task survives and no agent was launched, so the firing never had its chance. It is also why the consume rule is phrased "created a task **and recorded its run row**" — see [Idempotency](#which-runs-consume-the-dedup-key). Requires no code change; the absence of a row is already the absence of a key. |

### Idempotency, retry and concurrency

- **Dedup key**: `pr_merged:<task_id>:<owner>/<repo>#<pr_number>`, with `owner` and
  `repo` lowercased so a case difference between two sync paths cannot double-fire. The
  key includes `task_id` because one pull request can be linked to several tasks and each
  linkage is a separate thing to archive.
- `github.task_pr.updated` is published on *any* change to a linked-PR row, so a merged PR
  can keep producing events as reviews, checks or titles settle afterwards. The dedup key is
  what collapses those into one firing.

#### Which publish paths touch a merged row

Two contracts below — the down-time recovery guarantee and the bound on repeated cap-skip
rows — are derived from how often a *merged* row republishes. That makes the publish
predicates load-bearing rather than background, and they differ per path, so they are named
here instead of being generalised as "the sync paths".

| Path | Publishes when | Runs on a row already `merged`? |
|---|---|---|
| `SyncTaskPR` (watch-driven) | **any of sixteen** fields changed — `State`, `PRTitle`, `Additions`, `Deletions`, `ReviewState`, `ChecksState`, `MergeableState`, `ReviewCount`, `PendingReviewCount`, `RequiredReviews`, `ChecksTotal`, `ChecksPassing`, `UnresolvedReviewThreads`, `BaseBranch`, `MergedAt`, `ClosedAt` | **Yes** — watches stay active over merged rows |
| `reconcileTaskPRLifecycle` (unwatched reconcile) | only `state`, `merged_at` or `closed_at` changed | **No** — `taskPRNeedsUnwatchedSync` returns `false` for `merged` and `closed` |
| `associatePRWithTask`, `RestoreTaskPR` | unconditionally | Yes, when invoked |

**The narrow three-field predicate is the one path that never sees a merged row.** Any
statement in this spec about what a merged row does or does not republish must therefore be
derived from `SyncTaskPR`'s sixteen-field predicate, not from the three-field one. Several
of those sixteen keep changing after a merge in normal use — reviews get submitted, threads
get resolved, post-merge checks complete on the merge commit, and titles get edited — so a
merged row republishing is **ordinary, not exceptional**.

Two consequences, both stated where they bite: the Residual note below is *less* pessimistic
than it reads, and the cap-skip bound below is *weaker* than a naive reading suggests.

#### Which runs consume the dedup key

This is the one place where this trigger type cannot inherit the engine's existing dedup
behavior, and getting it wrong loses user data silently. The rule is stated in terms of what
a run **did**, never in terms of what status it currently reads — those two framings diverge,
and the divergence is the whole subtlety.

The engine writes a run row carrying the dedup key in three situations: a real firing
(`recordSuccessRun`, status `task_created`), a `skipped` row when `max_concurrent_runs` is
reached (`maybeSkipForConcurrencyCap`), and a `failed` row when the firing could not produce
a task at all (`recordFailedRun`, e.g. no repository available). The existence check behind
dedup, `HasRunWithDedupKey`, counts rows of **any** status. For `github_push` and
`github_ci` that is harmless, because the next event carries a new commit SHA or a new
check-run id and therefore a **new key**.

For this trigger type the key is derived from `(task_id, owner, repo, pr_number)` — a fact
that happens exactly **once**. A cap-skip or a pre-task failure would therefore burn that key
permanently and the task could never be archived, ever. With `max_concurrent_runs`
defaulting to 1, two pull requests merging inside one poll cycle is enough to trigger it.

**Contract, in origin terms:** for `github_pr_merged`, a run consumes its dedup key **if and
only if it created a task AND recorded its run row**. A firing that was capped, that failed
before a task existed, or whose task was created but whose run row could not be written,
does not consume the key, and a later `github.task_pr.updated` event carrying that key MUST
be admitted and evaluated normally.

**Why the rule names both halves.** "Created a task" alone is not sufficient, because there
is a path where a task is created and then removed again: `createAutomationTask` writes the
task, calls `recordSuccessRun`, and if that write **fails** it logs, calls
`deleteAbandonedTask` to remove the task it just created, and returns. That path leaves **no
run row of any status** — so nothing carries the key, and the mechanism below admits the
next event. A rule phrased only as "created a task" would say the opposite (consumed, never
retried) and the two would disagree on the one path where they diverge. They are reconciled
by naming the run row, and the outcome is the correct one: no task survives, no agent was
launched, nothing happened, so the firing has not had its chance and MUST be retriable.

**A run that created a task and then failed still consumes the key.** `MarkRunFailedByTaskID`
transitions an already-`task_created` run into `failed` when auto-start aborts or the agent's
turn errors. That run is **not** re-admitted: a task was created, an agent was launched, and
the firing got its chance. Retrying it would launch a second agent against the same merge —
and, with events continuing to arrive for the row as reviews and checks settle, would repeat
indefinitely for as long as the underlying fault persists. This is deliberate, and it is why
the rule is written about task creation rather than about the `failed` status.

**Mechanism — the write side, not the read side.** The rule is implemented by controlling
what is written into `automation_runs.dedup_key`, not by filtering the existence query:

| Write site | Package | `dedup_key` written for `github_pr_merged` |
|---|---|---|
| `recordSuccessRun` (status `task_created`) | `internal/orchestrator` | the key |
| `maybeSkipForConcurrencyCap` (status `skipped`) | `internal/automation` | **empty** |
| `recordFailedRun` (status `failed`, no task created) | `internal/orchestrator` | **empty** |
| `MarkRunFailedByTaskID` (`task_created` → `failed`) | `internal/automation` | unchanged — the row already carries the key |
| `recordSuccessRun` **fails** → `deleteAbandonedTask` | `internal/orchestrator` | **no row is written at all**, so no key — requires **no code change**, and the key is correctly left unconsumed |

The fifth row is listed because it is the path that makes the "AND recorded its run row"
half of the rule necessary. It needs no implementation: the absence of a row is already the
absence of a key. It is in the table so that a builder reading the table as the definitive
list of outcomes does not conclude the case was overlooked, and so a reviewer can see the
rule and the mechanism agree on every path rather than on four out of five.

**All three `recordFailedRun` call sites are pre-task-creation** — automation not found,
repository resolution failed, and task creation itself failed. That is what makes the third
row's blanket "empty" correct: there is no `recordFailedRun` path where a task survives, so
blanking the key there can never strand a firing that actually launched an agent.

**"Automation not found" appears twice in this spec, at two different stages, and they have
different observables.** `FireTrigger` reloads the automation **synchronously** and returns a
skip result when it is missing — no row of any kind. The orchestrator then reloads it again
**asynchronously**, on the goroutine that creates the task, and *that* miss is the
`recordFailedRun` call site named above, which writes a keyless `failed` row. So the same
underlying condition yields no row at the first stage and a visible `failed` row at the
second. Both fail closed and neither creates a task; only the run log differs.
[Failure modes](#failure-modes) carries one row per stage.

`HasRunWithDedupKey` is **not modified**: it keeps its signature, its query and its meaning
for every trigger type, which is what [Out of scope](#out-of-scope) promises. It already
returns `false` for an empty key. `MarkRunFailedByTaskID` needs no change at all — the
origin semantics fall out of the fact that the key was written when the task was created.
Both branches are conditioned on the trigger type, so `github_push` and `github_ci` keep
their current semantics, in which a run of any status consumes its key.

**The visible cost, stated rather than discovered at build time:** a `skipped` or
pre-task-`failed` run row for this trigger type carries an **empty** `dedup_key` column,
unlike the equivalent rows for other trigger types. Nothing in the run-log UI reads that
column, so this is invisible to users, but a test or query that asserts "every run row
carries its firing's dedup key" would be wrong for this type.

**Residual, stated rather than hidden:** re-admission depends on a *further*
`github.task_pr.updated` arriving for that row, and **nothing guarantees one**. Per
[the publish-path table](#which-publish-paths-touch-a-merged-row),
a merged row republishes from `SyncTaskPR` whenever any of sixteen fields changes, which in
normal use is likely rather than rare — post-merge reviews, thread resolutions, completing
checks and title edits all qualify. So in practice a cap-skipped firing usually does get
another chance.

But "usually" is not a guarantee, and the spec does not offer one. A merged PR that nobody
touches again — no further review activity, no post-merge checks, no title edit — publishes
nothing further, and its watch is eventually removed. In that case the task is never
archived and **there is no automatic recovery**: there is no backfill sweep, and
[Manual Run is not a recourse for this trigger type](#failure-modes). This residual is
accepted. It is the price of the do-not-consume rule being implemented on the write side
rather than by a persisted retry queue, which is
[out of scope](#out-of-scope).

**Repeated cap-skips are not collapsed.** Because the `skipped` row carries no key, nothing
suppresses the next one: every further matching event that arrives while the automation is
still at its cap writes **another** `skipped` row. One capped merge can therefore leave
several identical skip rows in the run log, and the automation's "last run" summary reads
`skipped` **until a later firing records a row**. Note what does *not* clear it: freeing the
concurrency slot writes no row and touches no existing one, and nothing recomputes the
summary, so the `skipped` reading persists past the condition that caused it — possibly
indefinitely, if no further event ever arrives for that automation. This is accepted rather
than engineered away — collapsing the rows would require storing the key on the very rows
this rule exists to keep keyless, which is the mechanism that loses the data.

**The bound is the number of post-merge `SyncTaskPR` publishes, and it is looser than it
first appears.** It would be wrong to justify this as "small and self-limiting because a
merged PR stops changing" — per
[the publish-path table](#which-publish-paths-touch-a-merged-row),
the watch-driven sync republishes on any of sixteen fields, several of which keep moving
after a merge, and a PR-detail panel open drives that same sync. A busy PR merged while its
automation sits at the cap can therefore accrue **more than a handful** of keyless `skipped`
rows, not one or two.

It is still bounded, and the bound is what makes it acceptable: post-merge activity on a
given pull request is finite and decays quickly, each event costs one row and no task, no
agent and no concurrency slot, and the noise ends the moment a slot frees. The honest
statement is "bounded but not tight", and it is recorded that way so nobody later reads a
skip storm in the run log as a defect. If it ever needs tightening, the fix is a persisted
retry queue — [out of scope](#out-of-scope) — not storing the key on these rows.

#### Concurrency

- **Delivery semantics depend on the configured bus, and this spec does not lean on them.**
  With the in-memory bus — the default — delivery is synchronous on the publishing
  goroutine, so a single publisher delivers one event at a time; multiple publishers still
  deliver concurrently, so even there the engine is not globally serialized. With the
  NATS-backed bus (`internal/events/bus/nats.go`, selected when NATS is configured) delivery
  is asynchronous and that per-publisher ordering does not hold either. Every statement below
  is written to be true under both: nothing in this trigger type's contract may assume
  serialized delivery.
- Dedup is **best-effort, not atomic**: the existence check is a plain count with no unique
  constraint behind it, so two events for the same key delivered concurrently can both pass
  it. This is the pre-existing behavior of every trigger type and this spec does not change
  it.
- The concurrency cap is **equally non-atomic** — the run row is written after task
  creation, so two concurrent firings can both observe zero active runs. It is therefore
  wrong to state that the cap catches the duplicate: it *usually* does, and it may not.
  What actually makes a duplicate harmless is that `archive_task_kandev` is idempotent, so
  two live runs converge on one archived task and the second reports
  `already_archived: true`.
- Deleting an automation's runs deletes its dedup memory. A later merged-state event for
  the same PR then fires again. This is accepted: run deletion is an explicit user action
  and the resulting archive is a no-op.

#### Retroactivity and first-observation semantics

The trigger fires on the **first observed** `github.task_pr.updated` event whose payload
reports the merged state for a given `(task_id, pull request)` pair, subject to dedup. It
does not, and cannot, distinguish an observed *transition* into the merged state from an
observation of a row that was already merged: the payload carries no prior state, and the
association and restore publish paths publish unconditionally rather than only on change.

Three consequences, all intended and all scenario-covered:

- **Creating the trigger performs no backfill.** Creating or enabling a trigger publishes no
  event, so nothing fires until the next `github.task_pr.updated` for a linked PR.
- **Linking an already-merged pull request to a task fires the trigger.** Associating a
  merged PR with a task publishes the event with `state = merged`, so the automation fires
  and the task is archived. This is correct rather than surprising: the condition the user
  configured — this task's pull request is merged — is true. The run row and its transcript
  record which task was archived, so the action is auditable.
- **A pre-existing merged pull request can fire once** if some later sync touches its row
  after the trigger is created. This is **more likely than a passing reader would assume**,
  not a narrow window: per
  [the publish-path table](#which-publish-paths-touch-a-merged-row),
  the watch-driven sync republishes a merged row on any of sixteen field changes. So enabling
  this condition in a workspace with recently-merged linked PRs can archive some of them as
  their rows next settle. This is accepted rather than defended against; defending against it
  would require persisting a per-row observation checkpoint, which is
  [out of scope](#out-of-scope). It is called out here so the behaviour is a documented
  consequence rather than a surprise on first enable.
  **This bullet is the authority on the question, and it does not conflict with the
  no-backfill-sweep exclusion in [Out of scope](#out-of-scope).** That exclusion is about the
  *sweep* — nothing scans for already-merged PRs at creation time — not about the pull
  requests, which remain eligible the moment their rows next republish. The two are stated
  together in both places because the short form of the exclusion reads like suppression, and
  suppression is neither the behaviour nor buildable within this spec's scope.
