---
status: draft
system: office
created: 2026-09-07
owners:
  - kandev
---

# Office Routine Catch-Up Requirements

## Overview

A routine's cron trigger can come due while the backend is not running to
process it. On resume its armed `next_run_at` is in the past, sometimes far in
the past. What the system does at that moment is a correctness property of an
unattended loop: it decides how much work resumes, how much is spent in the
first minute after a restart, and what the woken agent believes about the
world.

Four surfaces describe this moment, and they do not agree.

| Surface | What it says |
|---|---|
| Policy value `enqueue_missed_with_cap` | missed ticks are enqueued, up to a cap |
| Field `catch_up_max`, default 25 | the cap is 25 |
| `scheduler-01.md` and `scheduler-02.md` | "fire missed ticks up to the cap (default 25)" |
| Web UI label `office:enqueueMissedWithCap` (5 locales) | "Enqueue missed (with cap)" |
| The code (`routines/service.go`) | counts up to 25 elapsed ticks, dispatches **exactly one** run |

The code is silently authoritative. An operator sizing spend or work-in-progress
from the policy name, the field, the design doc, or the UI control would be
wrong by a factor of 25.

This capability resolves the disagreement in favour of the code and makes
every other surface state it. **One resume produces one wake.** It also
supplies what the code lacks: a durable, observable account of the gap crossed,
so collapsing the backlog is a reported fact rather than a silent one.

### Why one wake, and not twenty-five

The decision is chosen here, not inherited from the code. Enqueueing up to
`catch_up_max` runs was rejected on four grounds: the idempotency key already
collapses a fan-out to one wake; the concurrency policy would then skip or merge
whatever survived; a fan-out creates a burst hazard before either bounding
control exists; and there is no per-tick work to replay. Each is verified
against the tree, with its mechanism and citations, in
[Why the fan-out was rejected](../system-design/routine-catch-up-01.md#why-the-fan-out-was-rejected).
The wake is a "look at the world now" signal, not a queued work item.

## Terminology

- **Claim:** the atomic acquisition of a due cron trigger by one processor;
  exactly one may hold a given claim.
- **Processing instant:** the time the scheduler uses for the tick in which a
  claim is taken.
- **Gap:** the interval from the trigger's armed `next_run_at` at claim time up
  to the processing instant.
- **Elapsed ticks:** how many times the trigger's cron expression matches within
  the gap, including the tick that came due and is being dispatched now.
- **Missed ticks:** elapsed ticks minus one — the ticks that came due and will
  never be dispatched.
- **Gap summary:** the durable record of a gap: missed-tick count, first-missed
  timestamp, and whether the count was truncated.
- **Summarizing policy:** the catch-up policy that reports the gap to the woken
  agent, named `summarize_missed` here.
- **Lightweight / heavy routine:** empty versus non-empty `task_template`, as
  in [Office Scheduler](scheduler.md).

## Requirements

### REQ-OFFICE-ROUTINE-CATCHUP-001: One resume, one wake

**Intent:** Freeze the resume behaviour as a stated contract rather than an
emergent property of a counting loop, and close the boundary cases the
implementation leaves to invention: ordering, concurrent claims, a failed
computation, a crash-orphaned trigger, and a dispatch that fails after arming.

**As an** operator running an unattended workspace, **I want** a restart after
an outage to produce a bounded, predictable amount of agent work, **so that** a
long outage cannot turn into a burst of spend I did not authorize.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-CATCHUP-001.1:** When a due cron trigger is claimed, the
  system shall dispatch exactly one routine run for that claim, whatever the
  number of elapsed ticks in the gap and whatever the catch-up policy. The only
  cases producing none are a failed arming write (AC-001.2) and a run that
  cannot be created (AC-001.12); no case produces two.
- **AC-OFFICE-ROUTINE-CATCHUP-001.2:** When a claim is processed and the
  elapsed-tick computation succeeds, the system shall arm the trigger's
  `next_run_at` to the first match of its cron expression strictly after the
  processing instant. This shall hold identically under every catch-up policy.
  The failure path is governed by AC-001.6 and AC-001.11 instead, because the
  computation fails only when no cron match can be produced at all. When the
  arming write itself fails, the system shall dispatch nothing for that claim
  and leave `next_run_at` null for AC-001.9 to arm later; that tick is lost and
  no routine run records it.
- **AC-OFFICE-ROUTINE-CATCHUP-001.3:** `catch_up_max` shall bound only the
  number of elapsed ticks the system counts. It shall have no effect on the
  number of runs, tasks, or wakeup requests created per claim, which is at most
  one.
- **AC-OFFICE-ROUTINE-CATCHUP-001.4:** When the stored `catch_up_max` is less
  than 1, the system shall use 25; when it is greater than 1000, it shall use
  1000. Both clamp rather than reject, so no stored value can stop a routine
  firing.
- **AC-OFFICE-ROUTINE-CATCHUP-001.5:** When two processors evaluate the same
  due trigger concurrently, exactly one shall take the claim and dispatch;
  the other shall dispatch nothing, record no gap summary, and change no
  trigger state.
- **AC-OFFICE-ROUTINE-CATCHUP-001.6:** When the elapsed-tick computation fails,
  the system shall dispatch one run and record no gap summary, never reporting a
  gap whose size it did not measure. It shall arm `next_run_at` strictly after
  the processing instant, at the time AC-001.11 fixes.
- **AC-OFFICE-ROUTINE-CATCHUP-001.7:** Within one scheduler tick, due triggers
  shall be processed in ascending `office_routine_triggers.next_run_at` order,
  with ties broken by ascending `office_routine_triggers.id`.
- **AC-OFFICE-ROUTINE-CATCHUP-001.8:** When a cron trigger is created, the
  system shall arm `next_run_at` to the first match of its expression strictly
  after the creation instant, and shall reject the creation when that
  expression is empty or when it or the timezone cannot be parsed. A trigger of
  kind `cron` shall never be persisted with a null `next_run_at` at creation,
  on every creation path, including one that writes the trigger row directly.
  **No operation that mutates an existing trigger's cron expression, timezone,
  or enabled flag exists** (see `## Out of scope`); changing any of the three is
  a delete followed by a create, which arms by this same criterion. A recreated
  trigger shall never report a gap spanning the interval in which it did not
  exist, so a deliberate pause is never a gap.
- **AC-OFFICE-ROUTINE-CATCHUP-001.9:** When an enabled cron trigger has a null
  `next_run_at`, a subsequent scheduler tick shall arm it to the first match
  strictly after that tick's processing instant and shall not dispatch a run for
  it, so that a process stopping between claiming a trigger and arming it does
  not leave the trigger permanently unable to fire. The write shall be
  conditional on `next_run_at` still being null when applied, so a concurrent
  reconciliation is never overwritten: exactly one processor shall arm it, and
  none shall dispatch by way of this path. A trigger whose claim is still in
  flight shall not be armed by it. A trigger with `enabled` false shall not be
  armed by this path. When the expression or timezone cannot be parsed, the system
  shall leave `next_run_at` null, warn, and dispatch nothing; AC-001.11's
  fallback does not apply, because no claim was taken and no run is owed.
- **AC-OFFICE-ROUTINE-CATCHUP-001.10:** When a routine run has been created and
  its materialisation then fails — the heavy path failing to create the task, or
  the lightweight path failing to create or to dispatch the wakeup request — the
  system shall set that run's status to `failed` and shall not re-dispatch it. A
  wakeup request refused because its idempotency key already exists is **not** a
  materialisation failure; that run's status shall be left as the outcome the
  concurrency policy assigned.
- **AC-OFFICE-ROUTINE-CATCHUP-001.11:** When the elapsed-tick computation fails,
  the system shall arm `next_run_at` to the processing instant plus 24 hours and
  shall emit a warning naming the trigger and the underlying error. A trigger
  whose expression or timezone can never parse shall therefore dispatch at most
  one run per day until it is deleted and recreated.
- **AC-OFFICE-ROUTINE-CATCHUP-001.12:** When the routine run for a claim cannot
  be created at all, the system shall dispatch nothing and record no gap
  summary, and shall not re-attempt that tick. The trigger remains armed forward
  per AC-001.2 or AC-001.11, whichever governed the claim, and the absence of a
  routine run is the only record of that tick.
- **AC-OFFICE-ROUTINE-CATCHUP-001.13:** `catch_up_max` shall be normalized to
  the value AC-001.4 defines before it is persisted, on every create and update
  path, and rows written before this change shall be normalized once on upgrade.
  A routine read back through the API, and every web UI control showing it,
  shall report the same `catch_up_max` the tick honours.

### REQ-OFFICE-ROUTINE-CATCHUP-002: The gap is reported, not discarded silently

**Intent:** Answer what is discarded past the cap and whether it is recorded
anywhere. Nothing is discarded, because no run was ever going to be created for
a missed tick; what is bounded is the counting. The exact gap boundary is
preserved even when the count is truncated.

**As an** agent woken after an outage, **I want** to know how long the gap was
and whether the count I was given is exact, **so that** I can decide whether the
gap changes what I should do.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-CATCHUP-002.1:** When a claim's gap contains at least one
  missed tick and the policy is `summarize_missed`, the system shall record a
  gap summary on the routine run containing the missed-tick count, the
  timestamp of the first missed tick, and whether the count was truncated.
- **AC-OFFICE-ROUTINE-CATCHUP-002.2:** The missed-tick count shall exclude the
  tick being dispatched. When that count is zero, the system shall record no
  gap summary, whatever the value of truncated.
- **AC-OFFICE-ROUTINE-CATCHUP-002.3:** When no gap summary is recorded, the
  routine run shall carry no missed-tick field, and the agent's assembled
  prompt shall contain no missed-tick statement. Absence shall not be
  represented as a count of zero.
- **AC-OFFICE-ROUTINE-CATCHUP-002.4:** When `catch_up_max` is greater than 1
  and the elapsed-tick count reaches it while further ticks remain within the
  gap, the system shall record the missed-tick count as `catch_up_max - 1`,
  shall record truncated as true, and shall record the first-missed-tick
  timestamp exactly and un-truncated. At a `catch_up_max` of 1 this criterion
  does not apply and AC-002.11 governs.
- **AC-OFFICE-ROUTINE-CATCHUP-002.5:** When the routine is lightweight and the
  policy is `summarize_missed` and a gap summary was recorded, the assembled
  prompt for the run created by that claim shall state the missed-tick count and
  the first-missed-tick timestamp, and, when truncated is true, shall state that
  the count is a lower bound.
- **AC-OFFICE-ROUTINE-CATCHUP-002.6:** When the routine is heavy and the policy
  is `summarize_missed` and a gap summary was recorded, it shall be readable
  through the routine-run API. The created task's title and description shall be
  exactly what the routine's template renders, unmodified by the gap. A heavy
  routine under `skip_missed` records none, per AC-002.7.
- **AC-OFFICE-ROUTINE-CATCHUP-002.7:** When the policy is `skip_missed`, the
  system shall record no gap summary and the assembled prompt shall contain no
  missed-tick statement, whatever the number of elapsed ticks.
- **AC-OFFICE-ROUTINE-CATCHUP-002.8:** A recorded gap summary shall remain
  readable after the routine run reaches a terminal status and after a backend
  restart.
- **AC-OFFICE-ROUTINE-CATCHUP-002.9:** The system shall record the gap as
  measured, without attributing a cause. A gap produced by a backend outage, a
  suspended host, and a forward clock correction shall be recorded identically.
- **AC-OFFICE-ROUTINE-CATCHUP-002.10:** A gap summary shall be written exactly
  once, as part of the creation of the routine run it belongs to, and shall not
  be modified afterwards by any later status transition, including `skipped`,
  `coalesced`, `failed`, `done`, and `cancelled`. When more than one routine run
  is created for the same routine — two triggers due in the same tick, or a
  manual fire alongside a cron fire — each run shall carry only the summary
  measured for its own claim, and shall neither read nor overwrite another run's.
  A wakeup request refused as an idempotency duplicate, or coalesced into an
  in-flight run, shall not rewrite any gap summary, shall not alter the gap
  statement that run's agent already carries, and shall not give a gap statement
  to a run that had none.
- **AC-OFFICE-ROUTINE-CATCHUP-002.11:** When `catch_up_max` is 1, the
  missed-tick count is zero for every gap, and the system shall therefore record
  no gap summary for that routine however long the gap. Setting `catch_up_max`
  to 1 is an opt-out of gap reporting.
- **AC-OFFICE-ROUTINE-CATCHUP-002.12:** A routine run created by a manual or a
  webhook fire shall record no gap summary. Gap measurement applies only to a
  cron trigger's armed `next_run_at`.

### REQ-OFFICE-ROUTINE-CATCHUP-003: Every surface states the same behaviour

**Intent:** `enqueue_missed_with_cap` is a false statement in an enum value, two
design documents, and a UI control in five locales. Renaming it is the
deliverable; accepting the old value forever is what makes the rename safe for
installs that already store it.

**As an** operator reading the policy control, **I want** its name to describe
what happens, **so that** I do not size spend or work-in-progress from a name
that is 25 times wrong.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-CATCHUP-003.1:** The summarizing catch-up policy shall be
  named `summarize_missed`. A routine created with no catch-up policy supplied
  shall be persisted with `summarize_missed`, **whatever creation path is
  used** — the HTTP create handler, config-sync reconciliation, the
  pre-installed coordinator routine, or any direct repository insert. No
  creation path shall persist the deprecated alias.
- **AC-OFFICE-ROUTINE-CATCHUP-003.2:** `enqueue_missed_with_cap` shall be
  accepted on routine create and routine update as a deprecated alias for
  `summarize_missed`, and shall be normalized to `summarize_missed` before
  persistence.
- **AC-OFFICE-ROUTINE-CATCHUP-003.3:** A routine persisted before this change
  with `catch_up_policy = 'enqueue_missed_with_cap'` shall read as
  `summarize_missed`, and the column's stored default shall be
  `summarize_missed` **on an upgraded install as well as on a fresh one**.
  Inspecting the live schema of a database that predates this change shall not
  show the retired value.
- **AC-OFFICE-ROUTINE-CATCHUP-003.4:** `enqueue_missed_with_cap` shall not
  appear in any API response body or in any web UI control.
- **AC-OFFICE-ROUTINE-CATCHUP-003.5:** A stored `catch_up_policy` value that is
  neither `summarize_missed`, nor `skip_missed`, nor the deprecated alias shall
  be treated as `summarize_missed` and shall not prevent the routine from
  firing.
- **AC-OFFICE-ROUTINE-CATCHUP-003.6:** The web UI shall label the summarizing
  policy so that it states a single summarized wake, and shall label
  `catch_up_max` so that it states the bound is on the number of ticks counted
  and reported, not on the number of runs created. This copy shall be provided
  in `en`, `pt-pt`, `zh-cn`, `zh-hk`, and `zh-tw`.
- **AC-OFFICE-ROUTINE-CATCHUP-003.7:** `system-design/scheduler-01.md` and
  `system-design/scheduler-02.md` shall not describe missed cron ticks as
  fired, enqueued, or dispatched as multiple runs, and shall reference this
  requirement for catch-up semantics rather than restating them. Neither
  document shall name `enqueue_missed_with_cap` as the catch-up policy value
  except where it explicitly labels it a deprecated alias. Both shall state the
  routine wakeup payload's catch-up fields wherever they describe that payload's
  shape. The constraint is scoped to those two files; this document necessarily
  quotes the retired value.
- **AC-OFFICE-ROUTINE-CATCHUP-003.8:** Under the summarizing policy the web UI
  shall render the `catch_up_max` control and allow it to be edited, in both the
  routine-create dialog and the routine-detail view. Under `skip_missed` it
  shall not be rendered. The rename shall not make the control unreachable.

## Out of scope

Each exclusion is a decision, not an omission.

- **Enqueueing N missed runs.** Rejected on the four grounds in the
  Overview. Reopening it depends on work-in-progress enforcement and an enforced
  budget ceiling landing first, and on the wakeup idempotency key gaining a
  generation component.
- **Renaming `catch_up_max`.** Its meaning changes (a bound on counting, not on
  runs) but the name stays accurate enough, and renaming the column, the API
  field and the web state would double the migration surface for no change in
  observable behaviour. The UI copy is corrected instead, under AC-003.6. Its
  stored-versus-effective divergence is **not** deferred: AC-001.13 closes it.
- **A trigger-update operation.** There is none today: the trigger surface is
  create, list, delete and webhook-fire, and the only mutation of an existing
  trigger row is the scheduler's `next_run_at` re-arm. Editing a cron
  expression, timezone or enabled flag therefore means delete-then-create.
  Building the endpoint is a feature in its own right, including a decision
  about what an in-flight claim does when the expression changes underneath it.
  An update path added later inherits AC-001.8's rule: arm strictly after the
  change instant.
- **Downgrade compatibility.** A payload carrying `summarize_missed` sent to a
  build predating this change will be rejected by it; the alias is read-forward
  only. No exported artifact carries it: the config-sync YAML has no catch-up
  field.
- **A paused routine still fires.** `office_routines.status` is never consulted
  in the cron tick path; only `office_routine_triggers.enabled` gates firing. A
  defect, but not catch-up specific; a follow-up card carries it.
- **Cron correctness at the tick level.** Day-of-month versus day-of-week
  conjunction, DST behaviour, and the silent "24 hours from now" fallback for an
  unsatisfiable expression are gap 23, carded. This document constrains what
  happens *between* ticks and takes tick times as given.
- **Work-in-progress limits and budget enforcement.** Gaps 11 and 19, carded.
  This document creates no work that would need them.
- **Retention of gap summaries.** They inherit whatever retention
  `office_routine_runs` gets, which is none today (gap 20, carded).

## Prior art

**Wiki leg — did not run.** Config resolved from `~/.obsidian-wiki/config.henry`
via the `@henry` pin, giving
`OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki`, collection `wiki`.
Reading the vault returns `EPERM` (macOS privacy protection on `~/Documents`
from this worktree), and neither `obsidian-wiki` nor `qmd` is on `PATH` or as an
MCP tool. An environment failure, not an empty result: the vault may hold a
position here and was not consulted.

**saas-kb leg — did not run.** The `saas-kb` MCP server and its
`search_fsm_docs` tool are absent from this session; the `ai_sdlc` slice was not
queried.

**What informed it instead:** the tree, and
`Forge/docs/kandev/office/BETA-REQUIREMENTS.md` gap 24 and section 7a.

## References

- [Office Scheduler](scheduler.md) — the wakeup pipeline this sits inside.
- Catch-Up System Design
  [part 1](../system-design/routine-catch-up-01.md),
  [part 2](../system-design/routine-catch-up-02.md)
- [Scheduler System Design part 1](../system-design/scheduler-01.md),
  [part 2](../system-design/scheduler-02.md) — the documents AC-003.7 corrects.
