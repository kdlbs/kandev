---
status: active
system: office
created: 2026-04-25
updated: 2026-09-02
owners:
  - cfl
---
# Office: Cost Tracking & Budget Management Requirements

## Overview

Autonomous agents consume tokens on every turn, and when multiple agents run unattended across many tasks, spending can escalate without visibility. Kandev tracks analytics (task counts, turn counts, agent usage) but has no concept of monetary cost. Users have no way to know how much a task cost, which agent is most expensive, or to set guardrails that prevent runaway spending. Without cost tracking and budgets, autonomous operation is a financial black box.

## Terminology

- **Evaluation period:** The window over which a policy's spend is summed,
  identified by the window's start instant. A policy whose window is the whole
  history of the workspace has a single, non-resetting period.
- **Alert level:** `limit_subcents * alert_threshold_pct / 100`, using the
  existing integer arithmetic.
- **Limit level:** `limit_subcents`.
- **Reaches a level:** Spend is at or above the alert level and strictly below
  the limit (alert level), or spend is at or above the limit (limit level).
  These are mutually exclusive within one evaluation; this requirement does not
  change that.
- **Claim:** A durable record that a given (policy, evaluation period, level) has
  already produced its notification.
- **Emit:** Write the corresponding `budget.alert` or `budget.exceeded` row to
  the Office activity log.
- **Submit:** Hand that row to the Office activity logger. Submission is
  observable to the evaluation that performs it; the durability of the resulting
  write is not, because `ActivityLogger.LogActivity` returns nothing. Criteria
  describing what an evaluation *reports about itself* therefore say "submit";
  criteria describing the user-visible outcome say "emit". Submitting is not the
  same as holding a claim, and neither implies the other: an evaluation can hold a
  claim without submitting (AC-OFFICE-COSTS-002.5), and submit without holding one
  (AC-OFFICE-COSTS-002.14).

## Requirements

### REQ-OFFICE-COSTS-001: Office: Cost Tracking & Budget Management

**Intent:** Autonomous agents consume tokens on every turn, and when multiple agents run unattended across many tasks, spending can escalate without visibility. Kandev tracks analytics (task counts, turn counts, agent usage) but has no concept of monetary cost. Users have no way to know how much a task cost, which agent is most expensive, or to set guardrails that prevent runaway spending. Without cost tracking and budgets, autonomous operation is a financial black box.

#### Acceptance criteria

- **AC-OFFICE-COSTS-001.1:** Every agent session generates cost events as it runs.
- **AC-OFFICE-COSTS-001.2:** Cost events are aggregated on read into multiple views (by agent, project, task, model, time window).
- **AC-OFFICE-COSTS-001.3:** Budget policies SHALL enforce spending limits per agent, project, or workspace with `notify_only`, `pause_agent`, or `block_new_tasks` actions.
- **AC-OFFICE-COSTS-001.4:** Cost is resolved per cost event via a two-layer lookup (provider-reported then `models.dev` cache); no static fallback table exists.
- **AC-OFFICE-COSTS-001.5:** Stale `models.dev` pricing remains readable while one coalesced refresh runs per client. Concurrent background lookups and explicit refresh calls do not start duplicate network fetches.
- **AC-OFFICE-COSTS-001.6:** For providers that emit token telemetry on the ACP wire (claude-acp, opencode-acp, gemini, codex-acp), cost events are populated from the `complete` event at end-of-turn.
- **AC-OFFICE-COSTS-001.7:** For providers without usable wire telemetry (codex per-turn split, amp), a disk-runner subsystem reads on-disk session files via pinned `@ccusage/*` packages and feeds normalized cost events into the same pipeline.
- **AC-OFFICE-COSTS-001.8:** The cost explorer UI surfaces aggregations and lets the user manage budget policies.

### REQ-OFFICE-COSTS-002: Budget notifications fire on crossings, not on every evaluation

**Intent:** `budget.alert` and `budget.exceeded` are notifications, and the
[state machine](../system-design/costs-01.md#state-machine) defines them as
crossing events ("spend crosses `alert_threshold_pct`"). Budget evaluation is
implemented as a level check that runs on unrelated triggers: after every cost
event, before every agent launch, and after every task reassignment into a
project. Once a policy sits above its threshold each of those writes another
activity row, so one over-budget policy floods the Office inbox (which lists
`budget.alert` rows and counts them toward the badge) with duplicates carrying no
new information — a reassignment adding no spend at all still produces one. This
requirement makes the notification idempotent per policy, per period, per level,
while leaving enforcement (pausing an agent, blocking a run) the level check it
must remain.

**User story:** As a workspace owner with an over-budget project, I want one
budget notification per policy per period, so that my inbox stays usable and a
new notification still means something changed.

#### Acceptance criteria

Terminology used below is defined in [Terminology](#terminology).

- **AC-OFFICE-COSTS-002.1:** When a budget policy is evaluated and its spend is
  at or above its alert level but below its limit, and no alert-level claim
  exists for that policy and evaluation period, the system shall record the
  claim and submit a `budget.alert` row to the Office activity log.
- **AC-OFFICE-COSTS-002.2:** When a budget policy is evaluated and its spend is
  at or above its limit, and no exceeded-level claim exists for that policy and
  evaluation period, the system shall record the claim and submit a
  `budget.exceeded` row to the Office activity log.
- **AC-OFFICE-COSTS-002.2a:** Every criterion asserting an emission outcome
  (AC-OFFICE-COSTS-002.1, .2, .7, .8, .10, .14, .17, and any added later) requires the
  row to be *submitted*, not durably observed: `LogActivity` returns no error, so a
  failed activity-log write is invisible to the evaluation that claimed.
  At most one notification per (policy, evaluation period, level) may be lost this
  way, and the system shall not retry it, release the claim, or report that loss.
  This is the accepted direction of claim-then-emit ordering; the opposite ordering
  would re-introduce the flood this requirement exists to remove.
- **AC-OFFICE-COSTS-002.3:** When a budget policy is evaluated and a claim
  already exists for the level that the current spend reaches, for that policy
  and evaluation period, the system shall record no activity row for that level.
- **AC-OFFICE-COSTS-002.4:** AC-OFFICE-COSTS-002.1 through AC-OFFICE-COSTS-002.3 shall hold identically
  for every caller that evaluates a policy: the post-cost-event hook, the
  pre-execution budget gate, and the task-reassignment project evaluation. The
  behavior shall be implemented once, at the single evaluation function all
  callers converge on, so that no caller carries its own copy of it and a future
  fourth caller inherits it.
- **AC-OFFICE-COSTS-002.4a:** The task-reassignment caller named in
  AC-OFFICE-COSTS-002.4 does not exist in this repository yet; it arrives with
  PR #3276. Its clause of AC-OFFICE-COSTS-002.4 is therefore conditional on that
  call site being present. When it is absent at implementation time, the
  implementer shall record the deferral in the task plan under a heading reading
  exactly `## DEFERRED: AC-OFFICE-COSTS-002.4 reassignment coverage`, naming the
  absent symbol and the PR it waits on. That heading is the auditable record: a
  code comment does not satisfy this criterion, and AC-OFFICE-COSTS-002.4 shall not
  be reported as fully closed while it is present. The other two callers are
  unaffected and remain fully required.
- **AC-OFFICE-COSTS-002.5:** When an evaluation records an exceeded-level claim,
  the system shall also record the alert-level claim for the same policy and
  period, so that a spend that jumps from below the alert level to above the
  limit never emits a later `budget.alert` for the same period. This obligation is
  conditional on the claim store accepting that companion write. When the
  companion alert-level claim fails with a claim-store error, the system shall not
  retry it, shall not fail the evaluation, and shall not withhold the
  `budget.exceeded` row that the exceeded-level claim has already earned; it shall
  record the failure exactly as AC-OFFICE-COSTS-002.14 requires.
  AC-OFFICE-COSTS-002.14 takes precedence over this criterion, for the same reason
  it takes precedence over AC-OFFICE-COSTS-002.10: a degraded claim store must
  degrade to an extra notification, never to a failed evaluation or a withheld
  `budget.exceeded`. The cost is bounded and accepted — the de-escalation
  protection lapses for that one period, so a later evaluation landing in the alert
  band may emit one `budget.alert`. A test of this criterion shall therefore assert
  the companion claim on a healthy store, and a fault-injection test shall not
  assert that the companion claim was recorded.
- **AC-OFFICE-COSTS-002.6:** A claim shall be scoped to the policy's evaluation
  period, and the evaluation period shall be the same window used to compute
  that policy's spend, so a policy whose spend window never resets shall have a
  claim that never resets.
- **AC-OFFICE-COSTS-002.6a:** A single evaluation shall determine its period
  boundary exactly once, and shall use that one value both to compute the policy's
  spend and to identify the claim. An evaluation shall not derive the boundary a
  second time from a second reading of the clock. The unit here is the evaluation
  of one policy: when one trigger evaluates several policies, each policy's spend
  and claim shall agree with each other, but the policies need not share a boundary
  value, since no criterion compares one policy's period to another's. An evaluation spanning a period boundary would otherwise sum spend
  against one window and claim against the next, re-arming the notification once
  per boundary for every policy whose window resets — invisible in steady state,
  so it shall be closed structurally rather than left to the implementation.
- **AC-OFFICE-COSTS-002.7:** When the evaluation period advances to a new
  window, the system shall treat the new period as unclaimed and emit again on
  the first evaluation that reaches a level.
- **AC-OFFICE-COSTS-002.8:** When a budget policy is updated, the system shall
  discard that policy's claims for every period and level, so the next
  evaluation that reaches a level under the updated policy emits. The row update
  and the claim discard shall be atomic: either both apply or neither does. When
  the discard fails, the update shall fail with it and return an error to the
  caller, leaving the stored policy and its claims both unchanged. Partially
  applying the pair is prohibited: an update that succeeds while its discard fails
  leaves claims that silently suppress the notification this criterion mandates.
  The discard's guarantee is scoped to evaluations that begin
  after the update commits. An evaluation already in flight when the discard
  commits has read its policy, spend and levels beforehand, and may therefore
  insert a claim keyed to the pre-update period and level, suppressing the first
  post-update notification; that outcome is permitted rather than prevented. The
  window is bounded by one evaluation's duration, costs at most one suppressed
  notification, self-corrects at the next period, and is side-stepped entirely by a
  `period` change, which moves `period_key`. The alternative, a lock spanning both
  the evaluation and the update, is rejected: it would put a user-facing write
  behind a background evaluation.
- **AC-OFFICE-COSTS-002.9:** When a budget policy row is deleted by any path, the
  system shall delete that policy's claims. This shall hold for all three
  deletion paths that exist: single-policy deletion, workspace deletion, and the
  startup reconciliation that removes policies whose agent or project scope no
  longer exists. Claim deletion shall be a property of the policy row's removal,
  not of any one calling method, so a future fourth deletion path inherits it
  without change.
- **AC-OFFICE-COSTS-002.10:** When two evaluations of the same policy, period and
  level run concurrently **and the claim store returns no error to either
  evaluation**, the system shall submit exactly one activity row for that level.
  When the claim store returns an error to either evaluation,
  AC-OFFICE-COSTS-002.14 takes precedence over this criterion and duplicate rows
  are permitted: the healthy caller emits because it won the claim, the faulted
  caller because it fails open. Ordered this way deliberately — a broken claim store
  must degrade to duplicate notifications, never to silence, and a duplicate under
  an already-degraded store is the pre-existing behavior this requirement replaces,
  so it is no worse than today.
- **AC-OFFICE-COSTS-002.11:** Claim state shall survive a backend restart: a
  restart within an unchanged evaluation period shall not cause a policy that
  has already emitted at a level to emit again at that level.
- **AC-OFFICE-COSTS-002.12:** Suppressing a notification shall not suppress
  enforcement: when spend is at or above the limit, `pause_agent` shall still
  pause an unpaused in-scope agent, and `pause_agent` / `block_new_tasks` shall
  still deny a pre-execution budget check, regardless of whether a claim already
  exists.
- **AC-OFFICE-COSTS-002.13:** An evaluation result shall report, per level, both
  whether the policy is at or above that level and whether this evaluation
  submitted the notification, so a caller can distinguish "over budget" from
  "newly over budget". "Submitted" carries exactly the meaning fixed in
  [Terminology](#terminology) — this evaluation handed that level's row to the
  activity logger — and nothing beyond it. The claim is the mechanism that decides
  whether to submit; it is not part of what the field reports. The field shall
  therefore be true in every case where this evaluation put a row into the
  activity log and false in every case where it did not, which settles every case,
  including these:
  - reaches the level and wins the claim: row submitted, field true;
  - reaches the level but an earlier evaluation already holds the claim: no row,
    field false;
  - records the companion alert-level claim under AC-OFFICE-COSTS-002.5 while
    emitting `budget.exceeded`: no `budget.alert` row, so the alert-level field is
    false even though this evaluation held that claim;
  - the claim store errors and the evaluation emits anyway under
    AC-OFFICE-COSTS-002.14: a row went out, so the field is **true** even though no
    claim was held.
  The last case is the one a "held the claim *and* submitted" reading gets wrong:
  it would report false while duplicating a user-visible notification, hiding every
  fail-open emission from the callers this field exists to serve. Narrowed by
  AC-OFFICE-COSTS-002.2a in the other direction: `LogActivity` returns nothing, so
  a caller shall not read the field as a guarantee that the row is readable.
- **AC-OFFICE-COSTS-002.14:** When the claim store cannot be read or written, the
  system shall emit the notification and record the failure as an error log and
  a counter increment, so a broken claim store degrades to the previous
  duplicate-notification behavior rather than to silence. An evaluation that emits
  under this criterion reports `submitted` true, per AC-OFFICE-COSTS-002.13. This
  criterion is scoped to claim reads and writes performed **during an evaluation**,
  the only place a claim-store fault is otherwise silent: the evaluation continues,
  the caller sees no error, and without the counter nothing distinguishes a broken
  store from a quiet one. A failed claim *discard* during a policy update is
  deliberately outside it and shall not be counted or logged under it:
  AC-OFFICE-COSTS-002.8 governs it, rolling the update back and returning the error,
  so it is already visible to whoever made the edit, and a second weaker signal
  would imply the discard can fail silently, which AC-OFFICE-COSTS-002.8 forbids.
- **AC-OFFICE-COSTS-002.14a:** A claim rejected because the policy it references
  no longer exists is not a claim-store failure and shall be excluded from
  AC-OFFICE-COSTS-002.14: the system shall record no activity row, no error log and
  no counter increment. A policy deleted mid-evaluation warrants no notification,
  and counting its absence as a storage fault would both emit a notification naming
  a policy the user removed and report a healthy store as broken.
- **AC-OFFICE-COSTS-002.15:** When a policy's spend falls back below a level it
  has already claimed within the same period, the system shall keep the claim, so
  spend that re-crosses the same level in the same period does not emit again.
- **AC-OFFICE-COSTS-002.16:** Policies applicable to one evaluation shall be
  evaluated in ascending `created_at` order with ascending `id` as the tiebreak.
- **AC-OFFICE-COSTS-002.17:** On the first evaluation after this behavior is
  deployed, a policy already at or above a level shall have no claim for the
  current period and shall therefore emit once at that level, and then be quiet
  for the remainder of the period.

## System design

The migrated technical source is split into [part 1](../system-design/costs-01.md), [part 2](../system-design/costs-02.md).
`REQ-OFFICE-COSTS-002` is designed in [part 3](../system-design/costs-03.md) (the
claim store and control flow) and [part 4](../system-design/costs-04.md)
(suppression boundary, failure and recovery, persistence, security,
observability). Parts 3 and 4 are one design; part 3 carries the requirement
mapping for both.

## Out of scope

These exclusions scope `REQ-OFFICE-COSTS-002`.

- **Changing when a policy reaches a level.** The arithmetic named under
  [Terminology](#terminology) is preserved verbatim, including its existing edge
  behavior: `alert_threshold_pct = 100` never reaches the alert level (the
  alert level equals the limit and the alert branch requires spend strictly
  below the limit); `limit_subcents <= 0` is permanently at the limit level; and
  an evaluation never reaches both levels at once. This requirement changes how
  often a level emits, never whether it is reached. Correcting any of that is a
  separate contract change.
- **Unifying the three spend-window implementations.** `costs.periodCutoff`,
  `backendapp.budgetEvaluator.spendForPolicy` and `cron.periodStartFor` each
  compute a period boundary and disagree: the first two treat `daily` and
  `yearly` policies as lifetime windows even though `BudgetPeriod.Valid()`
  accepts them, and the third understands `weekly`, which the enum does not.
  AC-OFFICE-COSTS-002.6 sidesteps the disagreement by deriving the claim period from
  the same window the policy's spend already uses, so a claim can never reset on a
  schedule the spend does not. It does not fix the divergence. A follow-up should
  make one function authoritative and decide what `daily` and `yearly` mean; until
  then a `daily` policy keeps a lifetime spend window and a single non-resetting
  claim.
- **The cron budget-alert trigger.** `internal/scheduler/cron.BudgetHandler` fires
  `engine.TriggerOnBudgetAlert` on its own thresholds (50/80/90/100) with its own
  in-process dedup, and is inert in production because
  `budgetTaskScope.ResolveAlertTaskID` returns an empty task id. Prior art for the
  claim key shape (see the system design), but a different notification channel and
  unchanged here.
- **Resolving or expiring inbox items.** `budget.alert` rows are surfaced as inbox
  items with a hardcoded `active` status and no resolve action. This requirement
  reduces how many are created; it adds no lifecycle to the ones that exist.
- **`budget.exceeded` visibility.** Only `budget.alert` feeds the Office inbox list
  and badge count; `budget.exceeded` reaches the activity log only. This requirement
  makes both idempotent without changing which surfaces read them.
- **A user-facing setting for notification frequency.** Idempotency is
  unconditional; no per-policy "re-notify every N" control.
