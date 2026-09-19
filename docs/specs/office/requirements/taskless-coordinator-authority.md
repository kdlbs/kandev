---
status: active
system: office
created: 2026-09-06
owners:
  - kandev
---

# Taskless Coordinator Authority Requirements

## Overview

A lightweight (taskless) routine fire is specified to produce a real agent run
(`../system-design/scheduler-01.md`, "Heavy vs lightweight routines"). The pre-installed
"Coordinator heartbeat" routine is that shape: empty `task_template`, cron `*/5 * * * *`,
and the job "monitor the workspace, surface blockers, and react to events while no human
is driving the loop."

That run cannot do its job today, and the reason is **not** the missing session seam: two
runtime-contract defects stop it either way, both evidenced in the design's
[current-state sections](../system-design/taskless-coordinator-authority-01.md#the-read-that-already-exists):
the capability vocabulary holds no task-enumeration key, though the board read is already
reachable ungoverned via the dashboard route; and a taskless run's task scope is
`[""]`, which matches nothing.

This capability makes a scheduled taskless run's authority explicit and enforced. It does
not create the session seam, launch the run, or change a task-bound run's mutation or
annotation authority beyond the two exceptions named in
AC-OFFICE-COORDINATOR-AUTHORITY-005.5.
[Prior art](../system-design/taskless-coordinator-authority-01.md#prior-art) and
[concurrency](../system-design/taskless-coordinator-authority-01.md#concurrency-and-revocation)
are in the design.

## Terminology

- **Taskless run:** a `runs` row whose `payload.task_id` is absent, empty, or
  whitespace-only, i.e. empty after trimming. This one test gates both the scope
  derivation of REQ-003 and the annotation branch of REQ-004, and is applied before
  either branch.
- **Task-bound run:** any run that is not taskless.
- **Runner set:** the tasks the run's agent is already responsible for in the run's
  workspace: runner projection resolves to that agent, `state` in `TODO` or
  `IN_PROGRESS`, not archived, not automation-origin, **and `workspace_id` is the run's
  workspace**. The first four clauses are `CountActionableTasksForAgent`'s; the
  workspace clause is **not** in it and is added here
  ([derivation](../system-design/taskless-coordinator-authority-01.md#task-scope-derivation)).
- **Board read:** enumerating workspace tasks, mutating none.
- **Annotation:** appending a task comment. Additive, attributed, changes no task field.
- **State mutation:** changing a task's status, or creating a parented task.
- **Scope snapshot:** the task-scope list materialized into `runs.capabilities` and the
  run JWT, beside the already-frozen capability booleans, with a marker naming which
  source produced it and whether that source is final.

## Requirements

### REQ-OFFICE-COORDINATOR-AUTHORITY-001: Board read is a granted, governed capability

**Intent:** A scheduled coordinator must be able to enumerate the tasks it coordinates,
and that enumeration must be authorized, workspace-pinned from the token, and recorded
like every other runtime action.

**User story:** As a workspace owner, I want an unattended agent's board reads gated and
audited like its writes.

#### Acceptance criteria

- **AC-OFFICE-COORDINATOR-AUTHORITY-001.1:** The capability vocabulary shall include a
  `list_tasks` key, and `Allows("list_tasks")` shall report the granted state rather
  than falling through to the closed-vocabulary default.
- **AC-OFFICE-COORDINATOR-AUTHORITY-001.2:** `FromAgent` shall grant `list_tasks` for
  every agent role, independent of `permissions`.
- **AC-OFFICE-COORDINATOR-AUTHORITY-001.3:** The runtime action surface shall expose a
  board-read endpoint resolving its workspace from the run token's claim, never from a
  caller-supplied value.
- **AC-OFFICE-COORDINATOR-AUTHORITY-001.4:** When a run without `list_tasks` calls that
  endpoint, the system shall refuse with `ErrCapabilityDenied` (`403`) and append a
  `runtime.denied` event with `action=list_tasks`.
- **AC-OFFICE-COORDINATOR-AUTHORITY-001.5:** On success the system shall append a
  `runtime.action` event with `action=list_tasks`.
- **AC-OFFICE-COORDINATOR-AUTHORITY-001.6:** When the run token carries an empty
  workspace claim, the system shall refuse with `ErrWorkspaceOutOfScope` (`403`) and
  return no rows.
- **AC-OFFICE-COORDINATOR-AUTHORITY-001.7:** A board read shall succeed for a taskless
  run and shall not be restricted to the run's runner set.
- **AC-OFFICE-COORDINATOR-AUTHORITY-001.8:** `agentctl kandev tasks list` shall call the
  runtime board-read endpoint, not the dashboard workspace task route.
- **AC-OFFICE-COORDINATOR-AUTHORITY-001.9:** The dashboard workspace task route shall
  remain available to browser callers, unchanged.
- **AC-OFFICE-COORDINATOR-AUTHORITY-001.10:** `agentctl kandev tasks list` shall expose
  one flag per query parameter the endpoint accepts, including page size, sort column,
  sort direction, both cursor components and the system-workflow opt-in, and a repeatable
  flag for every parameter the design's table marks repeatable.
- **AC-OFFICE-COORDINATOR-AUTHORITY-001.11:** Board-read events shall not be
  deduplicated: each accepted read shall append exactly one `runtime.action` and each
  refusal exactly one `runtime.denied`, and an identical repeat shall append a further
  row rather than coalescing with the first.

### REQ-OFFICE-COORDINATOR-AUTHORITY-002: Board read is ordered, bounded, and deterministic

**Intent:** An unattended reader pages with no human to notice a duplicated or skipped
row, so ordering must be total, page size bounded by contract, and malformed parameters
must fail loudly rather than silently change the page.

**User story:** As a coordinator agent, I want a second page that continues exactly where
the first ended, so I neither double-act nor skip a task.

Parameter names, response fields and parse order are
[declared in the design](../system-design/taskless-coordinator-authority-02.md#request-and-response-contract).

#### Acceptance criteria

- **AC-OFFICE-COORDINATOR-AUTHORITY-002.1:** A board read shall order results by a named
  sort column and break ties on the `tasks.id` column in the same direction, so the
  ordering is total.
- **AC-OFFICE-COORDINATOR-AUTHORITY-002.2:** When no sort column is requested, the system
  shall sort by `tasks.updated_at` descending, and shall request that direction
  explicitly rather than relying on a repository default.
- **AC-OFFICE-COORDINATOR-AUTHORITY-002.3:** When a sort column outside the allow-list
  (`updated_at`, `created_at`, `priority`) is requested, the system shall refuse with
  `ErrInvalidListParams` (`400`) and return no rows.
- **AC-OFFICE-COORDINATOR-AUTHORITY-002.4:** When no page size is requested, or one above
  500 or at or below zero is, the system shall return at most 100 rows.
- **AC-OFFICE-COORDINATOR-AUTHORITY-002.5:** When more rows match than the page size, the
  system shall return a continuation cursor pairing the tail row's sort value with its
  `tasks.id`, as two separately addressable response fields.
- **AC-OFFICE-COORDINATOR-AUTHORITY-002.6:** On the final page the system shall return an
  empty continuation cursor.
- **AC-OFFICE-COORDINATOR-AUTHORITY-002.7:** When no task matches, the system shall return
  `200 OK` with an empty list, an empty cursor and no error.
- **AC-OFFICE-COORDINATOR-AUTHORITY-002.8:** A board read shall unconditionally exclude
  archived, ephemeral and automation-origin tasks, and shall exclude tasks in
  kandev-managed system workflows unless the caller opts in through the declared opt-in
  parameter. Only the system-workflow exclusion has an opt-in.
- **AC-OFFICE-COORDINATOR-AUTHORITY-002.9:** A board read shall take the same ordered,
  bounded path whether or not filter parameters were supplied.
- **AC-OFFICE-COORDINATOR-AUTHORITY-002.10:** A board read shall mutate no task, and
  repeating it with the same parameters shall return the same page while the matched rows
  are unchanged.
- **AC-OFFICE-COORDINATOR-AUTHORITY-002.11:** When the query fails, the system shall
  return an error, no rows and no cursor, never a partial page.
- **AC-OFFICE-COORDINATOR-AUTHORITY-002.12:** The endpoint shall accept a continuation
  cursor as its two components and resume strictly after the row they identify, under the
  sort column and direction supplied on that same request. The endpoint shall neither
  encode nor verify the sort that issued a cursor; reusing one under a different sort is a
  caller error it does not detect.
- **AC-OFFICE-COORDINATOR-AUTHORITY-002.13:** When any of these holds, the system shall
  refuse the whole read with `ErrInvalidListParams` (`400`), return no rows, and shall not
  serve a partially-parsed page even when other parameters were valid: only one cursor
  component supplied; a non-integer page size; a sort direction outside `asc` / `desc`; a
  non-boolean opt-in; or a repeat of a parameter the design's parameter table marks
  single-valued. A parameter that table marks repeatable shall instead match any of its
  supplied values, and shall not be an error. An empty value shall be treated as absent,
  so an empty `cursor` beside a non-empty `cursor_id` is the one-component refusal above;
  enumerated values shall match exactly, with no case folding or trimming.

### REQ-OFFICE-COORDINATOR-AUTHORITY-003: A taskless run's task scope is materialized, not empty

**Intent:** `AllowedTaskIDs = [""]` cannot match anything while presenting as a grant.
Replace it with an honest scope: empty when the run has no writable tasks, otherwise the
runner set — not the workspace: `tasks-01.md` already decided workspace membership
alone must not authorize mutating another task.

**User story:** As a workspace owner, I want an unattended run's write authority readable
from `runs.capabilities`.

#### Acceptance criteria

- **AC-OFFICE-COORDINATOR-AUTHORITY-003.1:** `WithTaskScope` shall discard empty and
  whitespace-only task identifiers, so no scope list contains an entry that can never
  match.
- **AC-OFFICE-COORDINATOR-AUTHORITY-003.2:** When a run is taskless, the task scope shall
  be materialized from the runner set at run-claim time.
- **AC-OFFICE-COORDINATOR-AUTHORITY-003.3:** When a run is task-bound, the task scope
  shall be exactly its `task_id` with surrounding whitespace removed, with no runner-set
  expansion, and the run context's own task identifier shall carry that same trimmed
  value.
- **AC-OFFICE-COORDINATOR-AUTHORITY-003.4:** The materialized scope shall be ordered by
  `tasks.updated_at` descending with `tasks.id` descending as the tiebreak, and capped at
  500 entries.
- **AC-OFFICE-COORDINATOR-AUTHORITY-003.5:** When the runner set exceeds the cap, the
  system shall retain the first 500 under that ordering and, per
  AC-OFFICE-COORDINATOR-AUTHORITY-003.14, append a `runtime.scope_truncated` event at
  level `warn` recording the cap and the true total.
- **AC-OFFICE-COORDINATOR-AUTHORITY-003.6:** When the runner-set query fails, the system
  shall fail closed with an empty task scope, append a `runtime.scope_unavailable` event
  at level `warn` recording the error per AC-OFFICE-COORDINATOR-AUTHORITY-003.14, and
  still build the run context.
- **AC-OFFICE-COORDINATOR-AUTHORITY-003.7:** When the runner set is empty, the task scope
  shall be empty and every task-scoped mutation shall be refused with `ErrTaskOutOfScope`.
- **AC-OFFICE-COORDINATOR-AUTHORITY-003.8:** The materialized scope shall be serialized
  into `runs.capabilities` and `runs.input_snapshot` and carried in the run JWT, as the
  capability booleans already are.
- **AC-OFFICE-COORDINATOR-AUTHORITY-003.9:** The scope snapshot shall be taken once per
  run and, once final, not recomputed for that run's life, so a task assigned to the agent
  mid-run is not writable by that run.
- **AC-OFFICE-COORDINATOR-AUTHORITY-003.10:** The system shall not assign
  `WildcardTaskScope` to a run on the basis that the run is taskless.
- **AC-OFFICE-COORDINATOR-AUTHORITY-003.11:** When a run token is re-minted for an
  existing run carrying a final scope snapshot, the system shall reproduce it, not
  re-derive the runner set.
- **AC-OFFICE-COORDINATOR-AUTHORITY-003.12:** The serialized snapshot shall record which
  source produced the scope — run payload or runner set, both **final**, or unavailable
  runner set, **provisional** — and that marker shall govern every later context build for
  the run: (a) a run carrying a final marker shall have its task scope reused verbatim,
  with no runner-set query; (b) a run whose marker is absent, provisional, unrecognized, or
  in a snapshot that cannot be parsed — including one persisted before this capability
  shipped — shall have its scope derived afresh and persisted with its marker, so a
  transient runner-set failure is retried rather than frozen; (c) when two processors build context for
  one run, the first persisted final marker shall win and the second shall reuse rather
  than overwrite it; (d) reuse shall apply to the task scope only, and capability booleans
  shall keep being derived from the agent on every build, unchanged from today.
- **AC-OFFICE-COORDINATOR-AUTHORITY-003.13:** A runner-set-materialized scope shall
  contain only tasks whose workspace is the run's workspace. The task-bound scope of
  AC-OFFICE-COORDINATOR-AUTHORITY-003.3 is not workspace-filtered, unchanged from today.
- **AC-OFFICE-COORDINATOR-AUTHORITY-003.14:** A scope event shall be appended only by the
  build whose scope becomes the run's persisted scope, and only when an appender is
  configured; a build that loses AC-OFFICE-COORDINATOR-AUTHORITY-003.12(c)'s race, or has
  no appender, shall append none and shall still build the run context.
- **AC-OFFICE-COORDINATOR-AUTHORITY-003.15:** When the runner set cannot be derived — no
  reader configured, or the run's workspace empty after trimming — the system shall issue
  no query and shall take AC-OFFICE-COORDINATOR-AUTHORITY-003.6's outcome, never an empty
  final runner set, recording the unmet condition in place of a query error.

### REQ-OFFICE-COORDINATOR-AUTHORITY-004: A taskless run's annotation is workspace-scoped; state mutation is not

**Intent:** Surfacing a blocker is the scheduled coordinator's stated job and requires
commenting on a task it does not run. Changing that task's state does not, and a
task-bound worker needs neither.

**User story:** As a coordinator agent, I want to comment on any task in my workspace to
flag a blocker, without being able to re-status another agent's work.

The widening is granted to **taskless runs only**, and the branch lives in the
enforcement predicate rather than in who holds `post_comment`. The rejected parity argument is in the design's
[authority boundary](../system-design/taskless-coordinator-authority-01.md#the-authority-boundary).

#### Acceptance criteria

- **AC-OFFICE-COORDINATOR-AUTHORITY-004.1:** When a **taskless** run holds `post_comment`
  and the target task belongs to the run's workspace, the system shall record the comment,
  whether or not the task is in the run's task scope.
- **AC-OFFICE-COORDINATOR-AUTHORITY-004.2:** When the target task belongs to another
  workspace, the system shall refuse with `ErrTaskOutOfScope` (`403`) and record no
  comment.
- **AC-OFFICE-COORDINATOR-AUTHORITY-004.3:** When no task matches the target id, the
  system shall refuse with `ErrTaskOutOfScope` (`403`) and record no comment, identically to
  AC-OFFICE-COORDINATOR-AUTHORITY-004.2 so annotation is not an existence oracle. When the lookup itself fails, the system shall return that error (`500`) and
  record no comment, so an outage is not read as a refusal.
- **AC-OFFICE-COORDINATOR-AUTHORITY-004.4:** When no target task is supplied and the run
  has no bound task, the system shall refuse with `ErrTaskOutOfScope` (`403`) and record
  no comment.
- **AC-OFFICE-COORDINATOR-AUTHORITY-004.5:** A recorded comment shall be attributed to the
  calling agent and shall append a `runtime.action` event naming the target task.
- **AC-OFFICE-COORDINATOR-AUTHORITY-004.6:** When the comment body is empty or
  whitespace-only, the system shall refuse with `ErrCommentBodyRequired` (`400`) and
  record no comment.
- **AC-OFFICE-COORDINATOR-AUTHORITY-004.7:** Annotation shall not be idempotent: an
  identical repeat shall record a second comment, with no deduplication on body, agent or
  run.
- **AC-OFFICE-COORDINATOR-AUTHORITY-004.8:** When a run attempts to change the status of a
  task outside its task scope, the system shall refuse with `ErrTaskOutOfScope`, whether or
  not the task is in the run's workspace.
- **AC-OFFICE-COORDINATOR-AUTHORITY-004.9:** When a run creates a subtask under a parent
  outside its task scope, the system shall refuse with `ErrTaskOutOfScope`.
- **AC-OFFICE-COORDINATOR-AUTHORITY-004.10:** When a run creates a task parented to a
  task outside its task scope, the system shall refuse with `ErrTaskOutOfScope`.
- **AC-OFFICE-COORDINATOR-AUTHORITY-004.11:** Root task creation shall keep requiring only
  `create_task` and a non-empty workspace, and shall succeed on a taskless run.
- **AC-OFFICE-COORDINATOR-AUTHORITY-004.12:** When a **task-bound** run targets any task
  other than its own bound task, the system shall refuse the annotation with
  `ErrTaskOutOfScope` and record no comment, whether or not the target is in the run's
  workspace, which is today's reach exactly.
- **AC-OFFICE-COORDINATOR-AUTHORITY-004.13:** When a **taskless** run's workspace is empty
  after trimming, the system shall refuse the annotation with `ErrWorkspaceOutOfScope`
  (`403`) and record no comment, checked before the target task's workspace is resolved. A
  task-bound run is unaffected.

### REQ-OFFICE-COORDINATOR-AUTHORITY-005: Denied authority stays denied

**Intent:** This capability widens read and defines write; it must not quietly grant the
two things Office already decided an agent may not do, nor add a second way to cast a
step verdict.

**User story:** As a workspace owner, I want an unattended agent to stay unable to move,
archive, or manufacture a decision on a card.

`record_step_decision_kandev` is excluded: it resolves task, step and role **from the
calling session** (`internal/mcp/server/agent_decision_tool.go`), so a runtime key would
put a second identity model on one decision ledger.

#### Acceptance criteria

- **AC-OFFICE-COORDINATOR-AUTHORITY-005.1:** No workflow-step-move key shall be added, and
  `agentctl kandev tasks move` shall keep failing locally without issuing an HTTP request.
- **AC-OFFICE-COORDINATOR-AUTHORITY-005.2:** No archive key shall be added, and `agentctl
  kandev tasks archive` shall keep failing locally without issuing an HTTP request.
- **AC-OFFICE-COORDINATOR-AUTHORITY-005.3:** No step-decision key shall be added, and no
  runtime endpoint shall write a step decision row.
- **AC-OFFICE-COORDINATOR-AUTHORITY-005.4:** `Allows` shall keep returning false for any
  key outside the vocabulary, so it remains closed after `list_tasks`.
- **AC-OFFICE-COORDINATOR-AUTHORITY-005.5:** A task-bound run's task scope, its
  state-mutation refusals, and its annotation reach shall be unchanged by this capability,
  with exactly two exceptions: the read-only board-read grant of
  AC-OFFICE-COORDINATOR-AUTHORITY-001.2; and a `task_id` carrying surrounding whitespace,
  which AC-OFFICE-COORDINATOR-AUTHORITY-003.3 now trims.
- **AC-OFFICE-COORDINATOR-AUTHORITY-005.6:** A taskless run holding `post_comment` shall be
  able to annotate a task that is in its workspace and outside its task scope, pinning the
  positive half of the branch that AC-OFFICE-COORDINATOR-AUTHORITY-004.12 pins negatively.

## Out of scope

Each exclusion is a contract; reasons are
[in the design](../system-design/taskless-coordinator-authority-01.md#out-of-scope).
The taskless session seam; recording a step decision from a taskless run; workflow-step
moves and archival for agents; narrowing `spawn_agent_run`; a tiebreak on comment
ordering; preconditions or optimistic concurrency on task-status mutation; backfilling
scope markers onto pre-existing runs; per-task ACLs or role-varying authority; live
re-resolution of task scope during a run; changing `checkIdleSkip`; and frontend surfaces.

## System design

[Taskless coordinator authority, part 1](../system-design/taskless-coordinator-authority-01.md).
