---
status: draft
system: office
created: 2026-09-07
owners:
  - kandev
---

# Office Session Identity Graduation Requirements

## Overview

`features.officeSessionIdentity` decides whose session an Office run binds to.
With it off, a reviewer's or approver's run binds to the task's *runner seat*, so
its verdict is recorded under an identity that does not own the seats the step is
guarded on, the guard counts zero decisions, and Review reports **"Stuck —
Blocked: no active session to re-evaluate this step."** With it on, each
participant agent gets its own durable session per task and multi-agent Office
completes autonomously. The flag is therefore the difference between multi-agent
review working and deadlocking. This capability makes the on-behaviour
unconditional, then retires the identity.

Office owns this contract because it owns the notion that a `(task, agent)` pair
names one durable conversation. The platform owns the toggle machinery
(`REQ-PLATFORM-FEATURE-TOGGLES-001`); the task system owns `task_sessions` and
every non-Office creation path. This document changes neither, but narrows one
tasks-owned re-evaluation rule; see *Supersession*.

## Terminology

- **Runner seat** — the agent profile on the task as its assignee
  (`AssigneeAgentProfileID`): the agent that does the task's work.
- **Participant identity** — the agent profile of the run being started, captured
  by the scheduler before step and routing overrides can mutate it. A reviewer or
  approver run's differs from the runner seat.
- **Pair** — a `(task_id, agent_profile_id)` tuple with non-empty
  `agent_profile_id`.
- **Terminal state** — `COMPLETED`, `FAILED`, `CANCELLED`. **Live** is its
  complement, not an enumerated list.
- **Active state** — `CREATED`, `STARTING`, `RUNNING`, `WAITING_FOR_INPUT`: the
  set `GetActiveTaskSessionByTaskID` and decider-session resolution both use.
  `IDLE` is in neither this set nor *Terminal*, so an Office session between turns
  is unresolvable for re-evaluation though not terminal.
- **Default-on release** — the first release whose shipped `prod` profile value
  for this flag is `"true"`.
- **Retirement release** — the release in which the live flag is removed and its
  identity appended to the append-only retired-identity set.

## Prior art

**Both outside legs could not run**, recorded rather than faked: no `wiki-query`
vault on this runner, no `search_fsm_docs` tool. The predecessor spec
[`task-session-identity.md`](task-session-identity.md) ran the saas-kb leg on
2026-08-31 and found no vendor documenting per-agent session identity; receipts
are in the task plan.

**In-repo prior decisions, load-bearing:**

- [`task-session-identity.md`](task-session-identity.md)
  (`REQ-OFFICE-SESSION-IDENTITY-001` … `-004`) **explicitly rejects** a unique
  index on `task_sessions(task_id, agent_profile_id)`:
  `AC-OFFICE-SESSION-IDENTITY-004.1` forbids enforcing the invariant "through any
  table-level database constraint", `-004.5` forbids any new index, column,
  backfill or repair of `task_sessions`, and it records that a previous attempt at
  that index broke three shipped flows.
- `AC-OFFICE-TASKS-001.2` asserted one session per `(task, agent)` pair before
  either document existed; where they could drift, `tasks.md` wins.
- **ADR 0005** made `agent_profile_id` shared between kanban and Office, so the
  pair cannot be constrained at schema level: no discriminator lets an index apply
  to Office rows only.

**Divergence from the card.** The card names a `UNIQUE(task_id,
agent_profile_id)` index plus a dedup migration as the blocking precondition. It
is obsolete: both halves of the hazard were closed on 2026-09-01 by a different
mechanism. Duplicate live rows are prevented by an in-transaction guard on the
Office creation path only (`REQ-OFFICE-SESSION-IDENTITY-001`), not a constraint,
so kanban relaunch and workflow replacement keep working; stranding is closed by
the live-preferring lookup (`REQ-OFFICE-SESSION-IDENTITY-002`). The precondition
is **already satisfied, by selection rather than repair**; adding the index would
regress `AC-OFFICE-SESSION-IDENTITY-004`.

**Session-selection contract — Tasks owns the shared rule.**
[`workflow-quorum-decision-recording-reevaluation.md`](../../tasks/requirements/workflow-quorum-decision-recording-reevaluation.md)
`AC-TASKS-QUORUM-REEVALUATION-001.1`/`-001.4` own re-evaluation session
selection for every decision surface. A decision with a validated calling
session uses that session. A decision without a calling session uses
`GetActiveTaskSessionByTaskID`: the task's most recently started session in an
active state, ordered by `started_at` descending, limit one. This document adds
Office-specific acceptance criteria and does not supersede the shared Tasks
contract.

## Requirements

**Release staging — how to read REQ-001 and REQ-002.** REQ-004 stages this
capability across two releases. REQ-001 and REQ-002 state the **end state**: they
bind **as of the retirement release**. In the default-on release the same outcomes
hold while the toggle is on, with the off path governed by
`AC-OFFICE-IDENTITY-GRADUATION-001.7` and `-002.7`; nothing in REQ-001 or REQ-002
requires removing the conditional early. REQ-003 binds in both.

**This capability's implementation delivers the default-on release only**:
`AC-OFFICE-IDENTITY-GRADUATION-004.1`, `-004.2`, `-004.3` and all of REQ-003. The
retirement criteria (`-004.5` … `-004.10`) define the successor release and are
not built here; `-004.4` forbids delivering both at once.

### REQ-OFFICE-IDENTITY-GRADUATION-001: Per-agent session binding is unconditional

**Intent:** An Office run binds to its own agent's session, for every
installation, with no configuration that can restore runner-seat binding.

**User story:** As an operator running multi-agent Office review, I want each
participant's run to use that agent's own session, so a reviewer's verdict counts
against the seats it occupies instead of deadlocking the step.

#### Acceptance criteria

- **AC-OFFICE-IDENTITY-GRADUATION-001.1:** When a run is started for an Office
  task and the run carries a participant identity, the system shall bind that
  run's session to the participant identity, whatever the task's runner seat is.
  As of the retirement release it shall reach that outcome with no dependence on
  any profile default, environment variable, or stored installation override.
- **AC-OFFICE-IDENTITY-GRADUATION-001.2:** While a run on an Office task carries
  no participant identity and the task has a runner seat, the system shall bind
  the run's session to the runner seat. Removing the toggle shall not change this
  case: both positions already produce it.
- **AC-OFFICE-IDENTITY-GRADUATION-001.3:** While an Office task has no runner
  seat yet, the system shall bind the run's session to the participant identity
  when the run carries one, and otherwise to the run's resolved execution
  profile identity. While at least one of those two identities is non-empty, the
  system shall not fall back to a generic workspace session. While both are
  empty, the system shall prepare a session by the pre-existing per-launch path,
  adding no new error for it.
- **AC-OFFICE-IDENTITY-GRADUATION-001.4:** When the task is not Office-owned,
  the system shall prepare a session by the pre-existing per-launch path,
  unchanged.
- **AC-OFFICE-IDENTITY-GRADUATION-001.5:** The system shall leave the Office
  find-or-create contract — `REQ-OFFICE-SESSION-IDENTITY-001` through `-003`,
  covering the creation guard, the live-preferring lookup and its `started_at` /
  `id` tiebreak, and convergence with bounded retry under concurrency —
  unchanged. This capability shall add no session-creation path,
  no lookup ordering, no reuse rule, and no coordination point.
- **AC-OFFICE-IDENTITY-GRADUATION-001.6:** When two runs for the same pair are
  started concurrently, the system shall behave exactly as
  `AC-OFFICE-SESSION-IDENTITY-003.1` already requires. It shall not change what
  happens when two callers contend for the same pair.
- **AC-OFFICE-IDENTITY-GRADUATION-001.7:** During the default-on release, while
  the toggle is off and a run on an Office task carries a participant identity
  that differs from the task's runner seat, the system shall bind the run's
  session to the runner seat. This is the pre-graduation behaviour the kill
  switch restores and the observable difference between the toggle's two
  positions. It shall become unreachable at the retirement release.

### REQ-OFFICE-IDENTITY-GRADUATION-002: Decisions re-evaluate against the decider's own session

**Intent:** An agent's recorded decision is checked against the seats its own
calling session occupies, so a reviewer whose agent is not the runner can move
a step.

#### Acceptance criteria

- **AC-OFFICE-IDENTITY-GRADUATION-002.1:** When an agent records a decision
  through the agent-decision tool, the system shall re-evaluate the step against
  the decider's own calling session. As of the retirement release this holds with
  no dependence on the toggle; during the default-on release it holds while the
  toggle is on, with `AC-OFFICE-IDENTITY-GRADUATION-002.7` governing the off
  path.
- **AC-OFFICE-IDENTITY-GRADUATION-002.2:** As of the retirement release, the
  system shall not resolve an agent decision against the task's
  most-recently-started session. That fallback shall be unreachable for agent
  decisions, enforced **at the agent-decision tool boundary**: the system shall
  reject a call supplying no calling session, or one whose session does not exist
  or belongs to another task, and write no decision row for it, rather than
  silently selecting a session.
- **AC-OFFICE-IDENTITY-GRADUATION-002.3:** **At the re-evaluation resolution
  step**, once a decision has been accepted, while the supplied calling session
  is unresolvable — absent, belonging to a different task, or not in an
  *active state* — the system shall record the decision, skip re-evaluation, and
  not reject it or error for that reason. It shall keep all three cases
  defensively: `AC-OFFICE-IDENTITY-GRADUATION-002.2`'s boundary already rejects
  the first two for agent decisions, and a session bound and active at that
  boundary can reach this test unresolvable only by leaving an active state
  first. An agent recording through its own live session is active by
  construction, so this criterion never contradicts
  `AC-OFFICE-IDENTITY-GRADUATION-002.5`. A repository or transport error raised
  while resolving is **not** an unresolvable session: the system shall surface it
  as an error rather than recording and skipping.
- **AC-OFFICE-IDENTITY-GRADUATION-002.4:** When a decision is recorded by a human
  decider rather than an agent, the system shall resolve the task's active session
  per `AC-TASKS-QUORUM-REEVALUATION-001.4`, unchanged.
- **AC-OFFICE-IDENTITY-GRADUATION-002.5:** Given an Office task whose reviewer
  participant is a different agent from its runner seat, when that reviewer
  records a decision satisfying the step's quorum, the system shall transition the
  step and not report "no active session to re-evaluate this step".
- **AC-OFFICE-IDENTITY-GRADUATION-002.6:** When the step write succeeds but the
  post-write re-evaluation fails, the system shall report success and retain the
  recorded decision's identity, and shall not convert a re-evaluation failure into
  a write failure.
- **AC-OFFICE-IDENTITY-GRADUATION-002.7:** During the default-on release, while
  the toggle is off, the system shall resolve an agent decision against the
  task's active session, as it did before graduation. The kill switch shall
  revert session binding and decision re-evaluation together; reverting one
  without the other is not conformant.
- **AC-OFFICE-IDENTITY-GRADUATION-002.8:** When two participants record decisions
  concurrently on one task, the system shall behave as the workflow engine's
  existing decision-write and quorum-evaluation contract already requires. It
  shall not change the engine's duplicate-write handling, decision ordering, or
  which caller owns a resulting transition, and adds no decision write and no new
  ordering.

### REQ-OFFICE-IDENTITY-GRADUATION-003: Pre-existing duplicate rows stay safe by default

**Intent:** Turning identity separation on everywhere must not expose the
duplicate `(task_id, agent_profile_id)` rows already in shipped databases, nor fix
them by a mechanism that breaks non-Office flows that deliberately create
duplicates.

#### Acceptance criteria

- **AC-OFFICE-IDENTITY-GRADUATION-003.1:** Given a pair holding one live row and
  one or more terminal rows, when an Office session is requested for that pair,
  the system shall return the live row and shall not fail.
- **AC-OFFICE-IDENTITY-GRADUATION-003.2:** Given a pair holding more than one
  live row written before the guard shipped, when an Office session is requested
  for that pair, the system shall return exactly one of those rows and shall not
  fail. The row returned shall be determined by the live-preferring order
  incorporated at `AC-OFFICE-IDENTITY-GRADUATION-001.5` — `started_at` descending,
  then `id` descending as a total tiebreak — so repeated requests against
  unchanged data return the same row.
- **AC-OFFICE-IDENTITY-GRADUATION-003.3:** The system shall satisfy
  `AC-OFFICE-IDENTITY-GRADUATION-003.1` and `.2` by selection alone: it shall run
  no dedup or repair migration, and shall not insert, merge, cancel or delete any
  `task_sessions` row or move a `metadata.acp_session_id` between rows. The reuse
  writes `AC-OFFICE-SESSION-IDENTITY-001.9` permits on the single row a lookup
  returns — `IDLE` to `RUNNING`, and execution-profile rebinding including
  clearing that row's own `metadata.acp_session_id` — are preserved by
  `AC-OFFICE-IDENTITY-GRADUATION-001.5` and are not forbidden here.
- **AC-OFFICE-IDENTITY-GRADUATION-003.4:** The system shall introduce no unique
  index and no other table-level constraint on `task_sessions`, and no new
  column on `task_sessions`, in service of this capability. This restates the
  binding obligation of `AC-OFFICE-SESSION-IDENTITY-004.1` and `-004.5` where it
  is most likely to be violated.
- **AC-OFFICE-IDENTITY-GRADUATION-003.5:** When a kanban task is relaunched for
  an agent profile whose session for that task is still live, the system shall
  create a second, distinct live session and succeed, with participant binding on
  by default.
- **AC-OFFICE-IDENTITY-GRADUATION-003.6:** When a workflow replacement session is
  prepared for an existing environment while the current session for the same
  pair is live, the system shall create the replacement and succeed, with
  participant binding on by default.
- **AC-OFFICE-IDENTITY-GRADUATION-003.7:** No operator-visible description of
  this flag shall state that an unshipped `(task_id, agent_profile_id)` unique
  index is a precondition for enabling it, and none shall state a default that
  contradicts the shipped profile values. This obligation is to the whole set of
  operator-visible surfaces describing the flag, not to an enumeration; at the
  time of writing: shipped profile metadata, the runtime flag registry's risk
  description, the public configuration reference, and the public operations
  reference. It applies for as long as the flag remains present.

### REQ-OFFICE-IDENTITY-GRADUATION-004: Staged graduation, then identity retirement

**Intent:** The default flips first and keeps a working kill switch for one
release; only after that does the flag identity disappear, permanently.

#### Acceptance criteria

- **AC-OFFICE-IDENTITY-GRADUATION-004.1:** In the default-on release, the shipped
  `prod` profile value for `KANDEV_FEATURES_OFFICE_SESSION_IDENTITY` shall be
  `"true"`.
- **AC-OFFICE-IDENTITY-GRADUATION-004.2:** In the default-on release, the
  shipped `dev` and `e2e` profile values shall also be `"true"`, flipping with
  `prod` in the same release.
- **AC-OFFICE-IDENTITY-GRADUATION-004.3:** In the default-on release, the toggle
  shall remain listed at **Settings > System > Feature Toggles**, and an
  administrator who sets it off and restarts shall observe runner-seat binding
  restored per `AC-OFFICE-IDENTITY-GRADUATION-001.7` **and** decision
  re-evaluation restored to the task's active session per
  `AC-OFFICE-IDENTITY-GRADUATION-002.7`. The kill switch shall be operable, not
  merely present.
- **AC-OFFICE-IDENTITY-GRADUATION-004.4:** The system shall not remove the live
  flag in the same release that first makes it default-on. The default-on
  release and the retirement release shall be distinct releases.
- **AC-OFFICE-IDENTITY-GRADUATION-004.5:** In the retirement release, the exact
  key `features.officeSessionIdentity` and the exact environment variable
  `KANDEV_FEATURES_OFFICE_SESSION_IDENTITY` shall be present in the append-only
  retired-identity set, and shall fail the generic active-reuse check. Neither
  identity shall ever be bound to a different capability.
- **AC-OFFICE-IDENTITY-GRADUATION-004.6:** In the retirement release, a stored
  installation override row for the retired key shall be inert: it shall produce
  no active runtime state, shall not change behaviour, and shall not be deleted.
- **AC-OFFICE-IDENTITY-GRADUATION-004.7:** In the retirement release, setting the
  retired environment variable shall produce no active runtime state, shall not
  change behaviour, and shall not prevent startup.
- **AC-OFFICE-IDENTITY-GRADUATION-004.8:** In the retirement release, the retired
  key shall be absent from the `/api/v1/features` response body, from the
  runtime flag definitions the Feature Toggles page reads, and from every
  shipped profile. Frontend consumers observe absence as `false` by the
  fail-closed rule in `AC-PLATFORM-FEATURE-TOGGLES-001.8`; this adds no new
  frontend behaviour.
- **AC-OFFICE-IDENTITY-GRADUATION-004.9:** In the retirement release, every
  behaviour required by `REQ-OFFICE-IDENTITY-GRADUATION-001`, `-002` and `-003`
  shall retain at least one test that does not reference the toggle, so removing
  the flag's own enabled/disabled tests does not reduce behavioural coverage.
- **AC-OFFICE-IDENTITY-GRADUATION-004.10:** In the retirement release, no
  startup-configuration surface shall continue to describe the retired
  environment variable as an active runtime feature flag, and any audited
  inventory fixture mirroring that surface shall agree with it. Retiring the
  identity in the flag registry alone, while a configuration catalog still
  classifies the variable as live, is not conformant.

## Out of scope

- **A unique index or dedup migration on `task_sessions`.** Requested by the card,
  rejected per *Prior art*. The blocker is **discharged, not deferred**: nothing
  left to schedule. `AC-OFFICE-IDENTITY-GRADUATION-003.4` makes the exclusion
  binding, not advisory.
- **Repairing the existing duplicate pairs.** No count is load-bearing: the census
  drifts while the system runs, and `-003.3` forbids repair at any count. The
  card's 2026-09-06 measurements motivate this document; no criterion depends on
  them.
- **Coupling `features.officeSessionIdentity` to `features.office`.** Rejected: it
  keeps two identities alive forever. The end state is one flag fewer.
- **When the retirement release happens.** Only that it be later than the
  default-on one (`-004.4`); bake duration and cadence belong to the release
  process, not this contract.
- **Cross-process concurrency on one database file**, and **convergence of
  concurrent recoveries carrying different execution profiles** (last-writer-wins).
  Both inherited unchanged from `task-session-identity.md`.
- **Any new user-visible surface.** The default-on release updates existing
  Feature Toggles and public configuration entries. The later retirement release
  removes those entries. No new copy, no i18n work, no route gate.
- **The stale DDL in `../system-design/tasks-01.md`.** Editorial, separate
  change.

## System design

No paired system design is required: this capability removes a conditional and
retires an identity, adding no new component, model, contract or control flow. It
preserves [`task-session-identity-01.md`](../system-design/task-session-identity-01.md).
