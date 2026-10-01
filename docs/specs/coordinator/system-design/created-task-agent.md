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
| `REQ-COORDINATOR-CREATED-TASK-AGENT-001` (`001.1` to `001.4`, `001.9`, `001.10`) | [Store](#store), [Routes](#routes), [Validation](#validation) |
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
task_agent_profile_id = ''` and the executor counterpart (`001.4`), in one
transaction, in the same startup step as the `ALTER`s and before the service
accepts a request, so no request sees a half-filled pair. Neither statement
writes `updated_at`, and an error aborts startup like any migration error. A value
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
  equal to the stored one is accepted, is not validated and changes nothing.
  A value that differs from the stored one is validated, and only then. The
  validation order on a PATCH is: every request-level 400 first (an empty or
  `null` task-pair field, agent then executor), then the merged-row validator
  (`patchValidator`) for the coordinator's own pair, then the task pair's
  check, all before the write and in the same place, so when the own pair and
  the task pair are both invalid the error names the own pair's field, and the
  task agent before the task executor. `patchValidator` keeps checking the
  coordinator's own pair on every write and does not check the task pair, so
  a stored task pair that has gone missing never blocks an edit of the
  context, the autonomy or the own pair; the task pair's check runs only for a
  field the request changes, the agent first. A change to these
  fields alone does not clear `conversation_task_id`, does not increment
  `config_revision` and does not archive the conversation (`001.9`): the
  existing change test compares only `context`, `agent_profile_id` and
  `executor_profile_id`. A PATCH that changes a stored task-pair value
  publishes `coordinator.updated` after the commit through the helper
  the approve path uses (`publishCoordinatorUpdated`, `autonomy_changed`
  false), because today only an autonomy change publishes one. A PATCH
  publishes at most one event: when it also changes the autonomy, the
  existing autonomy event (`autonomy_changed` true) is the only one and the
  pair adds none; a PATCH that changes no stored task-pair value adds
  nothing for the pair. Open proposal cards
  then refetch ([Runs with](#runs-with)). A publish failure is logged and
  never changes the PATCH result.
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
found". The status of the task pair comes from the validator's per-field
helpers (`agentProfileStatus` and `executorProfileStatus`, which
`Validator.ProfileStatus` combines for the first pair), called with the task
pair's ids; they already return a plain error for a failed read. Wherever this
design says `profileStatus`, it means those helpers. The agent check is
workspace-scoped and the executor check is existence only, because an executor
profile carries no workspace (`ValidateExecutorProfile` takes none today).
Neither check is transactional with the write: a profile deleted between the
validation and the commit leaves a stored pair that reads `missing`
(`001.8`), exactly as for the first pair.

## The chain

The orchestrator's order (`resolveTaskAgentProfile`,
`orchestrator/session_ensure.go`) is one function in a new leaf package,
`internal/task/agentprofile`, that imports neither the orchestrator nor the
coordinator: `Resolve(Input) (id string, source Source)`. `Input` carries the
workflow step (nil allowed), the per-task step replacement, the workflow's
default profile, the task's `metadata.agent_profile_id`, the Office assignee
profile and the workspace default profile; `Source` is one of `step`,
`workflow`, `task_metadata`, `assignee`, `workspace` or `none` (an empty id,
when no input names a profile). The order is the
orchestrator's today: the step's replacement or pinned profile, then the
workflow's default, then the task metadata, the assignee and the workspace
default. The step's profile and the workflow's default are both skipped when
the step has a session target, and the workflow's default is read only when
there is a step (`resolveStepAgentProfile`,
`orchestrator/event_handlers_workflow.go`). The orchestrator's
`resolveTaskAgentProfile` keeps loading its inputs and calls `Resolve`; its
existing table tests in `session_ensure_test.go` pass, and the extraction adds
two cases the tests do not cover today: a task whose step has no profile but
whose workflow has a default and whose metadata names another profile (the
workflow default wins), and a step with a session target and a workflow
default (neither applies). The comment above `resolveTaskAgentProfile`, which lists the task metadata before the workflow default, is corrected in the same change. Nothing is copied: the coordinator package does not
restate the order.

The coordinator loads the inputs for a task that does not exist yet: the frozen
spec's step (always present, since an empty step in a proposal means the
workflow's start step) and that workflow's default, and the workspace default;
the metadata, assignee and replacement inputs are empty. Reads go through
narrow interfaces the coordinator package declares (a step by id, the
workflow's `AgentProfileID`, the workspace's `DefaultAgentProfileID`, an agent
profile by id), implemented by the owning services; none authorizes, because
the approve path has authorized the workspace. Per loader:

- the step: it was just read by the eligibility check, so it is present. The
  chain needs the step's own profile and session target, which the step graph
  node does not carry, so it reads the step again by id: a step that is not
  found there (deleted since the check) settles the row `failed` with the
  ineligible-step text of step 4, creates no task and is not an error; any
  other read error is an error. On a card the same read finding no step makes
  `runs_with` `null`.
- the workflow's default: a workflow that does not exist gives an empty
  input; a read error is an error. This is deliberately stricter than the
  orchestrator's launch, which logs and falls through, because an empty input
  here would stamp the coordinator's pair over a default that exists.
- the workspace default: an unset default gives an empty input; a default
  whose profile does not exist in the workspace or is CLI passthrough also
  gives an empty input, since a task cannot use it; a read error is an error.
  A workspace that is not found is handled like a deleted coordinator: no
  task is created and the settle matches no row, so the request returns 404
  (`AC-COORDINATOR-PROPOSALS-002.11`, `003.4`); on a card it makes `runs_with`
  `null`.

A workspace default that is CLI passthrough is skipped because a task
cannot start on it, which would leave the dead card this feature removes, and
the workspace default is a fallback nobody chose for this task. A step or
workflow profile is taken as set whether or not it still exists, passthrough
or not: it
is the board owner's explicit choice and a dangling one surfaces as the existing
session start failure, which this feature does not change.

Two start paths take different chains. A task opened later resolves step,
workflow, metadata, assignee, then the workspace default
(`resolveTaskAgentProfile`). The launch an approval takes when the proposal
starts an agent (`auto_start_on_create`, then the workflow-step auto-start in
`event_handlers_workflow.go` and `resolveEffectiveAgentProfile`) takes the
step's and the workflow's profile, then `metadata.agent_profile_id`, and never
reads the workspace default. The addition therefore depends on `startsAgent`:

| `Resolve` source | Proposal does not start an agent | Proposal starts an agent |
| --- | --- | --- |
| `step` or `workflow` | nothing added | nothing added |
| `workspace` (usable) | nothing added | the workspace default is added as `agent_profile_id`; no executor |
| none | the coordinator's pair is added | the coordinator's pair is added |

The coordinator's pair is added only when nothing else names an agent, and the
workspace default is added only to keep it in force on the launch that would
otherwise ignore it. Task metadata outranks the workspace default in the
chain, which is why adding the coordinator's pair unconditionally would
override a workspace default the manager set.

## At approve

`completeClaimedApproval` runs one new call between `stepStillEligible` and
`createApprovedTask`, for every create attempt: the original claimer's, a
stale re-claim's, a failed row's retry and an automatic approval's (which
never starts an agent), because all of them reach it (`approve.go`). It
returns one of:

- *no addition*: the table above says nothing is added. The coordinator is
  not read.
- *workspace addition*: the table adds the workspace default. The task's
  create request metadata gains `agent_profile_id` only.
- *pair addition*: the table adds the coordinator's pair. The coordinator row
  is read (by id and workspace), its task pair is checked with
  `profileStatus`, and the request metadata gains `agent_profile_id` and
  `executor_profile_id` (`taskmodels.MetaKeyAgentProfileID`,
  `MetaKeyExecutorProfileID`).

  Either addition merges into the map that already carries
  `auto_start_on_create` when the proposal starts an agent. No other request
  field changes.
- *refusal*: the pair is not usable (agent `missing` or `passthrough`, or
  executor `missing`; the agent is named first when both are), or the
  coordinator row is gone. The reads run in order, agent status then
  executor status, and stop at the first status that is not `ok`: a definitive
  agent refusal never reads the executor, so it wins over a failing executor
  read, and a failing agent read is an *error* whatever the executor is.
  `failApproval` settles the row `failed` with exactly one of the texts of
  `AC-COORDINATOR-CREATED-TASK-AGENT-001.10`, and no create call runs. The card
  then shows the existing `failed` state (`AC-COORDINATOR-PROPOSALS-005.3`),
  and a manager fixes the setting and approves again. An automatic approval
  leaves the same `failed` row (`AC-COORDINATOR-AUTOMATIC-003.3`), which
  counts toward its 10 per 24 hours (`003.2`). When the coordinator row is
  gone its proposals are gone too, so the settle matches no row and the
  request returns 404 as any approval of a deleted proposal does.
- *error*: a read failed. The row stays `approving` under its claim, a warn
  log names the proposal id and the failed read, and an approve request
  returns 500; the stale re-claim recovers it, exactly as a failed step-graph
  read does. An automatic approval takes the same path as its
  `finishClaim` error today: the tool returns `{proposal_id, status: <the
  re-read status>, note: "automatic approval unavailable; a manager will
  decide"}` (`afterFailedFinish`), and the row counts toward the 10 per 24
  hours, as every row claimed automatically does (`CountAutomaticDecidedTx`
  counts whatever the status).

The pair and the workspace default are read once per attempt, immediately
before the create call, and that attempt uses what it read: a save committed
after the read applies to the next attempt, and one committed before it is
honoured. An edit after the create changes nothing, because the stamp is
copied into the task. A profile deleted between the read and the create is
not rechecked; the created task then fails to start with the existing
session start error.

A recovery that finds the task of an earlier attempt completes with it and
never updates its metadata (`002.18`). The lookup that finds it
(`GetTaskByExternalID`) runs before this call, so no read or refusal happens
for it. A second attempt whose create races a first one's reaches the create,
which stays idempotent on the reserved external id and returns the first task
(`CreateTaskOutcomeFoundSettled` or `FoundUnsettled`); what this attempt would
have added is dropped. The refusal branch runs before the
create and uses step 5's fence like the ineligible-step failure beside it, so
it shares its accepted limitation
([Approve](proposals.md#approve), step 5): when a stale re-claim fails the row
on a refusal while the first claimer's create is still running and then
commits, a task exists although the card says "Nothing was created", a later
approve of the `failed` row finds it through step 1's lookup, and a reject
leaves it on its board. The error branch writes nothing, so no fence applies to it: it returns 500
whether or not another caller has re-claimed the row meanwhile. The refusal
is a second way into that state beside the
ineligible step. It needs this request's create call to run for more than two
minutes, a second caller to re-claim the row during that call, and the pair to
be unusable at that caller's read.

Each create logs at info "coordinator.created_task_agent" with the proposal id
and `source` (`step`, `workflow`, `workspace` or `coordinator`), and each
refusal logs at warn with the field and status. No metric is added.

## Runs with

The proposal DTO gains `runs_with`, computed by the list and read routes only,
on `pending` and `failed` rows of the create-task kind and `null` on every
other row and kind. The approve, reject, undo and 409 response bodies carry
`null`: the card refetches the proposal after any of them.

```json
{"agent_profile_id": "ap-1", "agent_profile_name": "Claude", "source": "workspace"}
```

`source` is `step`, `workflow`, `workspace` or `coordinator`; when the chain
is empty and the coordinator's pair is not usable, or the coordinator is
missing, it is `none` with empty id and name. The list and read routes compute
it with the same call as [At approve](#at-approve) (a function that returns
the table's outcome and the pair's usability, used by both, so the card and the
approval cannot disagree except by time), over `final_spec` when the row has
one, else `spec`, and with the row's `starts_agent`. The agent profile shown
is the one the table adds, or the one `Resolve` returned when nothing is
added; a workspace addition shows `workspace`. The name is read from the agent
profile and is the id when the profile cannot be found.

Reads for one request are shared: the coordinator, the workspace default and
each agent profile name are read once per request, and a step or a workflow
default is read once per distinct id across the rows, so a list of N proposals
over one workflow makes one step read per distinct step. A failed read never
fails the list or the read: it makes `runs_with` `null` on the rows that
needed it only: a row needs the step and workflow reads for its own step, the
workspace default read only when its step and workflow name no profile, and
the coordinator read only when the chain is empty or the pair is to be
added, so a row resolved by its step is unaffected by a failed coordinator
read. A warn is logged, and those cards show no line. A read error on a card is
therefore different from an approve's, which returns 500.

The card (`proposal-cards.md#cards`) shows "Runs with: <name>" for any source
but `none`, and the warning line for `none`, below the workflow and step line,
for managers and readers, on Needs you and in the transcript. `runs_with`
refreshes when the row is refetched after `coordinator.updated`, which a
change to the setting publishes ([Routes](#routes)); it does not preview a
pending Edit, and the approval's own result is what counts.

## Settings UI

- One field group, "Agent for created tasks", built from the existing agent
  profile and executor pickers (passthrough profiles listed disabled with
  their reason), is added to the add form, the Identity section and the guided
  setup's Who runs it step. It carries the help text of
  `AC-COORDINATOR-CREATED-TASK-AGENT-001.5`. Readers see it disabled.
- Pre-fill is the pure function `prefillTaskPair({workspaceDefaultAgentProfileId,
  agentProfiles, ownAgent, ownExecutor, touched, current})` in
  `apps/web/lib/coordinator/`: agent is the workspace default when it is in
  `agentProfiles` and not passthrough, else `ownAgent`, else empty; executor is
  `ownExecutor`, else empty; while `agentProfiles` has not loaded the agent is
  empty, and the function runs again when it arrives; the same holds while
  `workspaceDefaultAgentProfileId` has not loaded (it is treated as unset
  and the field re-computes when it arrives, unless touched). It returns
  `{agent, executor}`: for a touched field `current` (the form's value for
  that field, `{agent, executor}`), for an untouched field the computed
  value. `touched` is
  `{agent: boolean, executor: boolean}` held in the form's or the setup's own
  state: a field becomes touched on the manager's first change to it through
  its picker, and stays touched until the form is closed or the setup is left
  (a Back or a Change inside the setup keeps it, because that state outlives
  the step). A touched field keeps its value and an untouched field is
  recomputed whenever `ownAgent`, `ownExecutor`, `agentProfiles` or
  `workspaceDefaultAgentProfileId` changes. An
  existing coordinator's page never calls it.
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

The agent profile id is checked against the coordinator's workspace on every
write that changes it and again at approve, so an agent profile of another
workspace is `missing` and never stamped. The executor profile id is checked
for existence only, as the coordinator's own executor profile is. Writes need `workspace.manage`; a reader sees the setting and the
resolved agent's name, as readers already see the workspace's agent profiles.
The stamp is written by the system into the task's metadata under keys that
are not coordinator-reserved; the coordinator's tools never carry the
profile.

## Related decisions

[ADR-2026-09-30-coordinator-phase-3-1-record-and-measure](../../../decisions/2026-09-30-coordinator-phase-3-1-record-and-measure.md#amendment-2026-10-01-agent-for-created-tasks-reverses-d1).
