---
id: coordinator-created-task-agent-design
title: Agent for created tasks design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-10-01
last_updated: 2026-10-01
requirements:
  - REQ-COORDINATOR-CREATED-TASK-AGENT-001
  - REQ-COORDINATOR-PROPOSALS-002
  - REQ-COORDINATOR-PROPOSALS-005
---

# Agent for created tasks System Design

## Purpose and boundaries

This design adds one per-coordinator setting, the pair "Agent for created
tasks", and one step in the approval path that adds the pair to a created task
only when nothing else names an agent. It reverses decision D1 of phase 1 (an
approved task carries no profile) because that left every approved task of a
workspace with no step, workflow or workspace default unable to start (live
finding F-07; the owner decided on 2026-10-01).

It changes neither whether an agent starts (`proposals.md#no-agent-starts`,
the phase-2 start-agent policy, `AC-COORDINATOR-AUTOMATIC-003.1`) nor the
coordinator's own conversation, which keeps running on its own agent profile
and executor profile. It is not behind `features.coordinatorPhase2`, `3` or
`31`: it ships under `features.coordinator` alone.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-CREATED-TASK-AGENT-001` (`001.1` to `001.4`, `001.9`) | [Store](#store), [Routes](#routes), [Validation](#validation) |
| `REQ-COORDINATOR-CREATED-TASK-AGENT-001` (`001.5` to `001.8`) | [Settings UI](#settings-ui) |
| `REQ-COORDINATOR-PROPOSALS-002` (`002.13`, `002.16` to `002.18`) | [The chain](#the-chain), [At approve](#at-approve) |
| `REQ-COORDINATOR-PROPOSALS-005` (`005.11`) | [Runs with](#runs-with) |

## Store

`coordinators` gains two columns by additive `ALTER`, in the required migrate
logger beside the phase-2 columns, on SQLite and PostgreSQL:

| Column | Type | Notes |
| --- | --- | --- |
| `task_agent_profile_id` | text not null default '' | may dangle after a profile is deleted, like `agent_profile_id` |
| `task_executor_profile_id` | text not null default '' | |

`coordinatorColumns`, `insertCoordinatorColumns`, the row struct and the DTO
carry both. After the `ALTER`s, every start runs two idempotent statements,
`UPDATE coordinators SET task_agent_profile_id = agent_profile_id WHERE
task_agent_profile_id = ''` and the executor counterpart (`001.4`). A value
that is already set is never touched, and a second start matches no row. The
backfill is the only writer of an empty value's replacement, so a row created
through a route (which refuses an empty value) is never rewritten. The
upgrade test runs from `storeconformance/testdata/upgrades/v0.93.0/` and from
a phase 3 database holding coordinators, on both dialects.

## Routes

- `POST .../coordinators` (phase 1) and `POST .../coordinators/setup` (phase
  2) take two further required strings, `task_agent_profile_id` and
  `task_executor_profile_id`, trimmed. An absent, `null` or empty value is 400
  naming the field (`001.2`). Their position in the validation order is after
  the coordinator's own two profiles: for the setup route, step `identity`,
  field order `name`, `agent_profile_id`, `executor_profile_id`,
  `task_agent_profile_id`, `task_executor_profile_id`, first failure wins
  ([coordinators](coordinators.md#guided-setup)).
- `PATCH .../coordinators/:cid` accepts the same two fields. Absent is
  unchanged; `null`, or empty after trimming, is 400 naming the field; a value
  equal to the stored one is accepted and changes nothing. A change to these
  fields alone does not clear `conversation_task_id`, does not increment
  `config_revision` and does not archive the conversation (`001.9`): the
  existing change test compares only `context`, `agent_profile_id` and
  `executor_profile_id`. The PATCH publishes `coordinator.updated` as every
  PATCH does, so open proposal cards refetch ([Runs with](#runs-with)).
- `GET .../coordinators/:cid` adds `task_agent_profile_status` (`ok`,
  `missing`, `passthrough`) and `task_executor_profile_status` (`ok`,
  `missing`), computed by the same `profileStatus` helper as the first pair. A
  read failure other than not found is 500 and is never reported as `missing`.
  The list route carries the two ids and no statuses, as for the first pair.
- The conversation route and its 409 body are unchanged and ignore the task
  pair (`001.8`).
- Two PATCHes of the task pair at once apply in commit order, the last commit
  determining the fields it sets (`AC-COORDINATOR-COORDINATORS-002.6`).

## Validation

`Validator` gains `ValidateAgentProfileFor(ctx, workspaceID, id, field)` and
`ValidateExecutorProfileFor(ctx, id, field)`; the existing
`ValidateAgentProfile` and `ValidateExecutorProfile` call them with their own
field names and keep their messages. The task pair uses the same checks (the
profile exists in the workspace and is not CLI passthrough; the executor
exists) with `FieldError.Field` `task_agent_profile_id` or
`task_executor_profile_id`. The passthrough message reads "this agent profile
uses CLI passthrough, which created tasks cannot use here" and the not-found
messages read "task agent profile not found" and "task executor profile not
found". `profileStatus` is called a second time for the task pair; it already
returns a plain error for a failed read.

## The chain

The orchestrator's order (`resolveTaskAgentProfile`,
`orchestrator/session_ensure.go`) is one function in a new leaf package,
`internal/task/agentprofile`, that imports neither the orchestrator nor the
coordinator: `Resolve(Input) (id string, source Source)`. `Input` carries the
workflow step (nil allowed), the per-task step replacement, the workflow's
default profile, the task's `metadata.agent_profile_id`, the Office assignee
profile and the workspace default profile; `Source` is one of `step`,
`workflow`, `task_metadata`, `assignee`, `workspace`. The order is the
orchestrator's today: the step's replacement or pinned profile (skipped when the
step has a session target), the workflow's default, the task metadata, the
assignee, the workspace default. The orchestrator's `resolveTaskAgentProfile`
keeps loading its inputs and calls `Resolve`; its existing table tests in
`session_ensure_test.go` pass unchanged, which is the proof that the extraction
kept the order. Nothing is copied: the coordinator package does not restate
the order.

The coordinator loads the inputs for a task that does not exist yet: the
frozen spec's step and workflow default, and the workspace default; the
metadata, assignee and replacement inputs are empty. Reads go through narrow
interfaces the coordinator package declares (a step by id, the workflow's
`AgentProfileID`, the workspace's `DefaultAgentProfileID`), implemented by the
owning services; none authorizes, because the approve path has authorized the
workspace. A read that fails for a reason other than not found is an error and
never an empty input: an empty input would stamp over a real default.

The stamp is conditional because task metadata outranks the workspace default
in the chain: adding the coordinator's pair unconditionally would override a
workspace default the manager set.

## At approve

`completeClaimedApproval` runs one new call between `stepStillEligible` and
`createApprovedTask`, for every create attempt: the original claimer's, a
stale re-claim's, a failed row's retry and an automatic approval's, because
all of them reach it (`approve.go`). It returns one of:

- *no addition*: `Resolve` yielded a profile. Nothing is added; the coordinator
  is not read.
- *addition*: `Resolve` yielded none. The coordinator row is read (by id and
  workspace), its task pair is checked with `profileStatus`, and the task's
  create request metadata gains `agent_profile_id` and `executor_profile_id`
  (`taskmodels.MetaKeyAgentProfileID`, `MetaKeyExecutorProfileID`), merged
  into the map that already carries `auto_start_on_create` when the proposal
  starts an agent. No other request field changes.
- *refusal*: the pair is not usable (agent `missing` or `passthrough`, or
  executor `missing`), or the coordinator row is gone. `failApproval` settles
  the row `failed` with an error such as "Agent for created tasks is not
  usable: the agent profile was removed" (the field and status named), and no
  create call runs. The card then shows the existing `failed` state
  (`AC-COORDINATOR-PROPOSALS-005.3`), and a manager fixes the setting and
  approves again. An automatic approval leaves the same `failed` row
  (`AC-COORDINATOR-AUTOMATIC-003.3`), which counts toward its 10 per 24 hours
  (`003.2`). When the coordinator row is gone its proposals are gone too, so
  the settle matches no row and the request returns 404 as any approval of a
  deleted proposal does.
- *error*: a read failed. The row stays `approving` under its claim, a warn
  log names the proposal id and the failed read, and an approve request
  returns 500; the stale re-claim recovers it, exactly as a failed step-graph
  read does.

The pair is read once per attempt, at that moment. An edit of the setting
between the claim and the create is therefore honoured by that create; an edit
after the create changes nothing, because the stamp is copied into the task.

A recovery that finds the task of an earlier attempt completes with it and
never updates its metadata (`002.18`). The lookup that finds it
(`GetTaskByExternalID`) runs before this call, so no read or refusal happens
for it. A second attempt that races a first reaches the create, which stays
idempotent on the reserved external id and returns the first task
(`CreateTaskOutcomeFoundSettled` or `FoundUnsettled`); what this attempt would
have stamped is dropped.

Each create logs at info "coordinator.created_task_agent" with the proposal id
and `source` (`step`, `workflow`, `workspace` or `coordinator`), and each
refusal logs at warn with the field and status. No metric is added.

## Runs with

The proposal DTO gains `runs_with`, present on `pending` and `failed` rows and
`null` on every other row:

```json
{"agent_profile_id": "ap-1", "agent_profile_name": "Claude", "source": "workspace"}
```

`source` is `step`, `workflow`, `workspace` or `coordinator`; when the chain
is empty and the coordinator's pair is not usable, or the coordinator is
missing, it is `none` with empty id and name. The list and read routes compute
it with the same call as [At approve](#at-approve) (a function that returns
the chain's outcome and the pair's usability, used by both, so the card and the
approval cannot disagree except by time), over `final_spec` when the row has
one, else `spec`. The name is read from the agent profile and is the id when
the profile cannot be found. Per request the workspace default and the
coordinator are read once. A read failure never fails the list or the read: the
member is `null`, a warn is logged, and the card shows no line.

The card (`proposal-cards.md#cards`) shows "Runs with: <name>" for any source
but `none`, and the warning line for `none`, below the workflow and step line,
for managers and readers, on Needs you and in the transcript. `runs_with`
refreshes when the row is refetched after `coordinator.updated`; it does not
preview a pending Edit, and the approval's own result is what counts.

## Settings UI

- One field group, "Agent for created tasks", built from the existing agent
  profile and executor pickers (passthrough profiles listed disabled with
  their reason), is added to the add form, the Identity section and the guided
  setup's Who runs it step. It carries the help text of
  `AC-COORDINATOR-CREATED-TASK-AGENT-001.5`. Readers see it disabled.
- Pre-fill is the pure function `prefillTaskPair({workspaceDefaultAgentProfileId,
  agentProfiles, ownAgent, ownExecutor, touched})` in
  `apps/web/lib/coordinator/`: agent is the workspace default when it is in
  `agentProfiles` and not passthrough, else `ownAgent`, else empty; executor is
  `ownExecutor`, else empty. `touched` marks each field the manager edited; a
  touched field keeps its value and an untouched field is recomputed whenever
  `ownAgent` or `ownExecutor` changes. An existing coordinator's page never
  calls it.
- The add form and Finish stay disabled while either value is empty
  (`001.2`). A 400 naming `task_agent_profile_id` or `task_executor_profile_id`
  shows beside its field, and in the setup returns to Who runs it
  (`AC-COORDINATOR-COORDINATORS-008.6`).
- Review gains the row "Agent for created tasks" after Executor, owned by
  Identity, valued "<agent name>, <executor name>"
  ([coordinators](coordinators.md#guided-setup)).
- The Identity section shows the status messages of `001.8` under the field
  from `task_agent_profile_status` and `task_executor_profile_status`.
- Copy goes through `t()` in the six locales with no em dash; the Traditional
  Chinese pair through `pnpm run i18n:zh-hant`. Keys: the field group label,
  the help text, the three status messages, `runsWith` ("Runs with: {{name}}")
  and `noAgentAvailable`.

## Security

Both ids are checked against the coordinator's workspace on every write and
again at approve, so a profile of another workspace is `missing` and never
stamped. Writes need `workspace.manage`; a reader sees the setting and the
resolved agent's name, as readers already see the workspace's agent profiles.
The stamp is written by the system into the task's metadata under keys that
are not coordinator-reserved; the coordinator's tools never carry the
profile.

## Related decisions

[ADR-2026-09-30-coordinator-phase-3-1-record-and-measure](../../../decisions/2026-09-30-coordinator-phase-3-1-record-and-measure.md#amendment-2026-10-01-agent-for-created-tasks-reverses-d1).
