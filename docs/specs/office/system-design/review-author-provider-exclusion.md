---
status: current
system: office
requirements:
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-001
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-002
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-003
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-004
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-005
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-006
---

# Office Review Author-Provider Exclusion System Design

## Context and boundaries

Three existing paths meet here:

- **Seat action.** `ensure_participant_seat` is an `on_enter` action stored in
  `workflow_steps.events` as `{type, config}`. `config` is a free-form map.
  The template loader (`config/workflows/loader.go`) and workflow
  export/import (`internal/workflow/models/export.go`) copy it verbatim. The
  engine compiles it into `engine.EnsureParticipantSeatAction`
  (`internal/workflow/engine/types.go`), which today carries only `Role`.
- **Seat runs.** The step's fan-out (`queue_run_for_each_participant`, on
  entry and on comment) queues one run per seat through the engine run queue.
  `runs/service.QueueRun` writes the queuing step into the run payload as
  `workflow_step_id` and the task as `task_id`. Work-step launches
  (`queueOfficeAutoStartRun`, `on_comment` `queue_run`) go through the same
  path, so work-stage runs carry their step too. Office reactivity wakes
  (assignment, comment, mention), onboarding, recovery and manual resume
  queue through the same service without a step; see §2a.
- **Routing.** `SchedulerService.DispatchWithRouting`
  (`internal/office/scheduler/dispatch_routing.go`) resolves candidates with
  `routing.Resolver.Resolve` on every dispatch: first launch, post-start
  fallback, park lift, and manual retry. A successful launch writes
  `runs.resolved_provider_id` through `SetRunResolvedRoute`. A launch the
  session ceiling deferred parks the run with a `deferred_capacity` route
  attempt that carries the candidate. `SettleCeilingDeferredRun` and
  `FinishCeilingDeferredRunOnCompletion` both copy the attempt's execution
  profile, provider, and model onto the run in the same update. A replayed
  Work run records its author provider in either order
  (AC-OFFICE-REVIEW-AUTHOR-EXCL-002.6).
- **Scope during the dynamic-routing migration.** An Office agent with an
  `execution_agent_profile_id` skips `DispatchWithRouting`
  (`SchedulerIntegration.tryRoutingDispatch`) and launches through its bound
  profile, so the exclusion never reaches it (AC-OFFICE-REVIEW-AUTHOR-EXCL-002.7).
  The setup evaluator warns about each such seat-pool agent (§7). The Office
  routing handoff (dynamic-agent-routing task 10) moves Office launches to the
  shared resolver and must carry the author skip with it.

The policy is evaluated at dispatch time, not stored on the seat or the run.
Every resolution re-reads the declaration and the author provider, so retries
and fallbacks cannot skip it, and a workflow revisit is picked up on the next
resolution (AC-OFFICE-REVIEW-AUTHOR-EXCL-002.2, .3). No schema change is
needed.

## Components

### 1. Seat-action config

- `engine.EnsureParticipantSeatAction` gains `ExcludeAuthorProvider bool`.
  `readEnsureParticipantSeatConfig` reads `config["exclude_author_provider"]`
  as a boolean; any other type reads as `false`.
- The engine does not act on the field. It is compiled so that engine tests
  and any future editor share one parser.
- A pure helper in `internal/workflow/models`,
  `AuthorProviderExclusionRoles(events StepEvents) map[string]bool`, returns
  the roles whose seat-ensuring action on that step sets the flag. Office
  calls it after loading the step's events; it shares no state with the
  engine.

### 2. Exclusion policy reader (Office repository)

Two read-only queries in `internal/office/repository/sqlite`:

- `GetWorkflowStepEvents(ctx, stepID) (wfmodels.StepEvents, error)`: reads
  `workflow_steps.events` and decodes it. A missing step returns empty events.
- `GetTaskAuthorProvider(ctx, taskID) (string, error)`: returns
  `resolved_provider_id` of the most recent run whose payload `task_id`
  matches, whose payload step (`workflow_step_id`, else
  `queued_workflow_step_id`) names a step with `stage_type = 'work'`, and
  whose `resolved_provider_id` is non-empty. It orders by
  `COALESCE(claimed_at, requested_at) DESC, requested_at DESC, id DESC`. It
  uses `dialect.JSONExtract` for every payload key, so SQLite and PostgreSQL
  behave the same. An empty result means that no author is recorded.

### 2a. Queue-time step for runs queued without one

`runs/service.Service.SetTaskStepResolver` (wired in `backendapp` to the
Office repository's `GetTaskWorkflowStepID`) stamps
`runsmodels.QueuedWorkflowStepPayloadKey` (`queued_workflow_step_id`) with the
task's current step when a run payload names a task and carries no
`workflow_step_id`. It is a separate key on purpose: `workflow_step_id` opts a
run into the step-change staleness cancellation in
`service/scheduler_staleness.go`, and runs without it must stay exempt. Only
the author lookup reads the new key. A failed step read leaves the key unset
and never blocks the enqueue.

Seat membership uses the existing workflow-scoped `ListTaskParticipants`.

### 3. Scheduler: `resolveAuthorExclusion`

A new file, `internal/office/scheduler/dispatch_author_exclusion.go`, holds:

```go
type authorExclusion struct {
    Applies        bool              // run is a seat run at a declaring step
    AuthorProvider routing.ProviderID // "" when none recorded
}
func (ss *SchedulerService) resolveAuthorExclusion(
    ctx context.Context, run *runsmodels.Run, agent *models.AgentInstance,
) (authorExclusion, error)
```

1. Read `task_id` and `workflow_step_id` from the payload. If either is
   missing, return `Applies=false`.
2. Load the step's events and the declared roles. If none, return
   `Applies=false`. This keeps undeclared seats on today's path with no added
   reads (AC-OFFICE-REVIEW-AUTHOR-EXCL-001.3).
3. List the task's participants. `Applies` is true when the agent holds a seat
   in a declared role (AC-OFFICE-REVIEW-AUTHOR-EXCL-002.4).
4. When it applies, read the author provider.

Any storage error is returned. `DispatchWithRouting` wraps it as a dispatch
infrastructure error, which the caller already routes to
`HandleRunFailure` (AC-OFFICE-REVIEW-AUTHOR-EXCL-004.4).

### 4. Resolver: author skip

`routing.ResolveOptions` gains `AuthorProvider ProviderID`. It is kept apart
from `ExcludeProviders` on purpose. `ExcludeProviders` drops a provider with
no record, because the provider already has a failed attempt in this cycle.
The author skip must leave a record. In the enabled loop of `Resolve`, before
`evaluateProvider`, a provider equal to `AuthorProvider` appends:

```go
SkippedCandidate{ProviderID: pid, Reason: SkipReasonAuthorProvider}
```

`AutoRetry` is false and `RetryAt` is zero, so `aggregateBlock` yields:

- `waiting_for_provider_capacity` with the earliest retry when any other
  skipped provider is auto-retryable (AC-OFFICE-REVIEW-AUTHOR-EXCL-003.3);
- `blocked_provider_action_required` when the author is the only provider
  left (AC-OFFICE-REVIEW-AUTHOR-EXCL-003.4).

The disabled-routing branch (`!cfg.Enabled`) ignores `AuthorProvider`.

### 5. Dispatcher wiring

In `DispatchWithRouting`, after the max-attempts check and before `Resolve`:

```go
excl, err := ss.resolveAuthorExclusion(ctx, run, agent)
// err -> return false, false, fmt.Errorf("dispatch: author exclusion: %w", err)
opts.AuthorProvider = excl.AuthorProvider // only when excl.Applies
```

After `Resolve`, when `excl.Applies` is true, append one run event per
resolution (AC-OFFICE-REVIEW-AUTHOR-EXCL-004.1 to .3):

| Case | `event_type` | level | payload |
|---|---|---|---|
| Author excluded | `routing.author_exclusion` | info | `{"applied": true, "author_provider_id": "<id>"}` |
| No author recorded | `routing.author_exclusion` | info | `{"applied": false, "reason": "no_author_provider"}` |
| Routing disabled (`!res.Enabled`) | `routing.author_exclusion` | info | `{"applied": false, "reason": "routing_disabled", "author_provider_id": "<id or empty>"}` |

The scheduler reaches `service.AppendRunEvent` through its existing service
handle. Run events already appear in the run detail Events log.

`skipOutcome` maps `SkipReasonAuthorProvider` to the new route-attempt outcome
`skipped_author_provider`. `parkRunBlocked` then writes the attempt row
(AC-OFFICE-REVIEW-AUTHOR-EXCL-003.2). When other candidates exist,
`tryCandidates` never sees the author provider, because `Resolve` did not
return it. If every remaining candidate then fails and the run parks through
`exhaustedCandidates`, `tryCandidates` first writes the author provider's
`skipped_author_provider` attempt, so every park keeps that history entry.

### 6. Web: route-attempt outcome

- `RouteAttemptOutcome` (`apps/web/lib/state/slices/office/routing-types.ts`)
  gains `"skipped_author_provider"`.
- `route-panel.tsx` maps it in `OUTCOME_LABEL` to
  `office:routeOutcomeSkippedAuthorProvider` and in `OUTCOME_VARIANT` to
  `outline`.
- The new key is added to all seven catalogs. Its English value is "skipped:
  wrote this task".

### 7. Setup-warning evaluator (Office routing)

A pure function in `internal/office/routing/author_exclusion_setup.go`:

```go
type ExclusionDeclaration struct {
    WorkflowID, WorkflowName, StepID, StepName, Role string
}
type SetupWarningCode string // closed set:
//   "routing_disabled"        exclusion will not be enforced (AC-005.2)
//   "single_provider"         workspace order has exactly 1 provider (AC-005.1)
//   "agent_single_provider"   a seat-pool agent's effective order has exactly 1
//   "agent_bound_profile"     a seat-pool agent is bound to its own execution
//                             profile, which the exclusion does not reach (AC-005.2)
type SetupWarning struct {
    Code         SetupWarningCode
    Declaration  ExclusionDeclaration
    ProviderID   ProviderID // the only provider; single_provider and agent_single_provider
    AgentID, AgentName string // agent_single_provider and agent_bound_profile
}
func EvaluateExclusionSetup(
    cfg *WorkspaceConfig, agents []settingsmodels.AgentProfile,
    decls []ExclusionDeclaration,
) []SetupWarning
```

- `cfg == nil`, or `!cfg.Enabled`, yields `routing_disabled` per declaration.
- These are possible halts, not certain ones: a run waits only when the author
  provider equals that one provider. The evaluator cannot know the author in
  advance, so each warning carries `ProviderID` and the copy states the
  condition.
- Otherwise `single_provider` when `len(cfg.ProviderOrder) == 1` and either
  the seat pool is empty (casting falls back to the runner, whose order is
  unknown) or at least one pool agent inherits the workspace order. Then, for each
  agent whose Office role is in the declared role's seat pool and whose status
  is neither `stopped` nor `pending_approval`: `agent_bound_profile` when it
  has an `execution_agent_profile_id` (its routing override is then
  irrelevant, and it does not count as inheriting the workspace order), else
  `agent_single_provider` when
  its override (`provider_order_source: override`) has exactly one entry. An agent with an unreadable override blob is skipped; the resolver
  already fails such an agent's dispatch.
- The seat-pool mapping moves from `engine_adapters/seat_caster.go` to
  `internal/office/models` (`SeatPoolAgentRoles`, `IsSeatPoolEligible`). The
  caster and the evaluator both call `IsSeatPoolEligible`, so the warning and
  casting cannot disagree.
- The output is sorted by workflow name, step name, role, code, and agent name,
  so every surface is deterministic.

`routing.Provider` gains `ExclusionSetup(ctx, workspaceID, decls)`, which
loads the routing config and the workspace's Office agents, then calls the
evaluator; empty `decls` reads nothing. `StoredExclusionSetup(ctx,
workspaceID)` first reads the workspace's stored declarations through
`ProviderRepo.ListAuthorExclusionDeclarations`. That query reads `workflows`
joined to `workflow_steps` for the workspace and decodes each step's events
with `wfmodels.AuthorProviderExclusionRoles`. A step that does not decode is
an error.

### 8. Warning surfaces

| Surface | Contract change | Declarations from |
|---|---|---|
| `GET /workspaces/:wsId/routing` | `RoutingConfigResponse.author_exclusion_warnings []AuthorExclusionWarningDTO` (never null) | stored |
| `PUT /workspaces/:wsId/routing` | response becomes `RoutingUpdateResponse{ok, author_exclusion_warnings}`, computed after the save | stored |
| `POST` workflow import preview | `service.ImportProfilePreview.Warnings []ImportWarning` | the parsed import document |
| `POST` workflow import | `service.ImportResult.Warnings []ImportWarning` | the parsed import document |
| Workflow sync apply | `SyncApplyResult.Warnings` gains one English string per warning | the applied documents |

`AuthorExclusionWarningDTO` and `ImportWarning` share one shape:
`{code, workflow_name, step_name, role, provider_id?, agent_name?}`, where `code` is the
closed set above plus `check_failed`. A failed evaluation is reported as a
single `check_failed` warning on every surface; the request still succeeds. The web maps `code` to a localized
string and interpolates the names. No server copy is shown in the import or
routing UIs.

The workflow system must not import Office. `internal/workflow/service`
declares an optional port:

```go
type ImportExclusionAdvisor interface {
    AdviseExclusionSetup(ctx context.Context, workspaceID string,
        decls []models.AuthorExclusionDeclaration) ([]ImportWarning, error)
}
```

It is set with `SetImportExclusionAdvisor`, and `internal/backendapp` wires it
to the Office method through an adapter. When the port is unset, or when the
document declares nothing, no warning is produced and nothing is read. A port
error produces one `check_failed` warning; the import continues
(AC-OFFICE-REVIEW-AUTHOR-EXCL-005.8). Workflow sync reuses the same port and
renders the warnings with `syncWarningTexts`: one English string per group of
warnings sharing code, provider, and agent (AC-OFFICE-REVIEW-AUTHOR-EXCL-005.10),
in the AC-OFFICE-REVIEW-AUTHOR-EXCL-005.9 shape: the consequence, then
"Affected steps: <workflow> workflow, <step> step (<role>); ...", then the fix
in Office > Routing for "this step" or "these steps".

### 9. Web

- **Routing settings page** (`app/office/workspace/routing/page.tsx`): a new
  `AuthorExclusionWarningBanner` above `RoutingEnableCard`. It reads
  `author_exclusion_warnings` from `useWorkspaceRouting`. A successful save
  refetches the config, so the banner reflects the saved state; when the save
  response carries warnings, the success toast is replaced by a warning toast.
  The banner uses the existing `Alert` primitive (default variant) with the
  amber `IconAlertTriangle` treatment of `provider-health-banner.tsx`, one
  list item per warning, and wrapping text, so the composition is the same on
  phone.
- **Import dialog** (`use-workflow-import.ts`,
  `workflow-import-profile-selection.tsx`): when the preview carries warnings,
  the import no longer auto-submits. It stops on the review view, which shows
  the shared `AuthorExclusionWarningList` above the existing confirm action
  (desktop dialog and phone drawer). With no profile steps to resolve, the
  view is titled "Review imported workflows". Confirming imports unchanged.
- **Shared component:** `components/author-exclusion-warning-list.tsx` takes
  the wire warnings (`lib/types/author-exclusion.ts`) and an optional provider
  formatter. The routing page passes `providerLabel`, and import shows raw
  provider IDs.
- **Warning copy** (AC-OFFICE-REVIEW-AUTHOR-EXCL-005.9, .10):
  `groupAuthorExclusionWarnings` (`lib/author-exclusion-warning-groups.ts`)
  groups the wire warnings by code, provider, and agent, in first-seen order.
  Each group renders the localized problem-and-consequence sentence once, then
  a list of "<workflow> workflow, <step> step (<role>)" entries (the first
  three, plus a "Show all N steps" toggle when there are more), then one fix
  sentence whose "this step" or "these steps" wording uses i18next `count`
  plurals. A `check_failed` group has no step list. The list
  takes a `surface` prop: `routing` fixes point at controls on the same page;
  `import` fixes add "in Office > Routing". Copy calls the setting **Review on
  a different provider** and the routing switch **Automatic provider
  fallback**, never "exclusion" or "routing is off".
- **Locales:** `workflows:` keys for each code's problem sentence and each
  code-and-surface fix sentence, the item lead, the banner title, the
  review-view title and description, the routing-page section copy (§10), and
  `office:routingSavedWithExclusionWarnings`, in all seven catalogs.
- **Import while submitting:** the review view renders only in the `resolving`
  phase or while submitting from it. An import that auto-submits (nothing to
  review) keeps the YAML dialog with its importing state, so an empty review
  view with disabled actions is never shown.

### 10. Routing-page control (REQ-OFFICE-REVIEW-AUTHOR-EXCL-006)

This is the control's home until issue #5347 ships an editable Office workflow
surface; the control then moves there and reuses this write path.

**Models** (`internal/workflow/models/author_exclusion.go`):
`ParticipantSeatRoles(events) map[string]bool` returns each
`ensure_participant_seat` role with whether it sets the flag.
`SetExcludeAuthorProvider(events, role, enabled) (StepEvents, bool)` returns a
copy whose matching seat actions set `exclude_author_provider: true` or drop
the key, and reports whether any action matched. Other actions and config
keys are untouched.

**Read:** the Office repository's `ListSeatSteps(ctx, workspaceID)` selects the
workspace's steps (hidden workflows included) whose events mention
`ensure_participant_seat`, decodes them, and returns one
`routing.SeatStep{WorkflowID, WorkflowName, StepID, StepName, Role,
ExcludeAuthorProvider}` per seat action, ordered by workflow name, step
position, and role. `GET /workspaces/:wsId/routing` adds
`author_exclusion_steps` (never null).

**Write:** `workflow/service.SetSeatAuthorExclusion(ctx, workspaceID, changes)`
takes `[]SeatAuthorExclusionChange{StepID, Role, Enabled}`. It first
authorizes and validates every change: the step is visible and belongs to a
workflow in the workspace, the workflow and its executor allow the change
(`authorizeExecutorMutation`), the step has a seat action for the role, and
the edited step passes `ValidateWorkflowStep`. Any failure returns before a
write (AC-OFFICE-REVIEW-AUTHOR-EXCL-006.4). It then writes every changed step
in one transaction through the workflow repository's
`UpdateStepEventsIfUnchanged`, which sets only `events` and `updated_at`, and
only where `updated_at` still equals the version that was read. If any step
changed after the read, nothing is written and the error wraps
`models.ErrWorkflowStepChanged`; the service then re-reads and re-applies the
batch, up to three attempts (AC-OFFICE-REVIEW-AUTHOR-EXCL-006.9). It returns
the updated steps with their new versions. The Office dashboard reaches it through a port that
`internal/backendapp` wires to the workflow service plus a
`stepevents.Publisher` with source `office-routing`, so every update publishes
`workflow_step.updated` (AC-OFFICE-REVIEW-AUTHOR-EXCL-006.2).

`PUT /workspaces/:wsId/routing/author-exclusion` takes
`{changes: [{step_id, role, enabled}]}` and returns
`{author_exclusion_steps, author_exclusion_warnings}` computed after the
write. The Office scope middleware requires `workspace.manage` for this route
(`requiresOfficeWorkspaceManage`), and the handler refuses an agent JWT caller
with 403 before reading the body (AC-OFFICE-REVIEW-AUTHOR-EXCL-006.11). An
invalid change is a 400, a step the caller cannot see is a 404, and
a read-only workflow, or a step that kept changing through every attempt, is
a 409. A failed request writes nothing, so it publishes nothing.

After a successful write the handler calls the routing provider's
`ReleaseAuthorExclusionParkedRuns` with the steps a change turned off. The
repository method of the same name selects the workspace's runs that are
queued and parked (any status except `session_recovery_required`), whose
current retry cycle (route attempts with `seq > route_cycle_baseline_seq`)
has a `skipped_author_provider` attempt and no `deferred_capacity` attempt,
and whose payload `workflow_step_id` is one of those steps. It re-queues them
with an update that repeats the same predicate, so a run that launched or
changed park reason in between is left alone. A launch the session ceiling
deferred stays parked because the ceiling's replay owns it, and re-queueing
it would launch it twice. A run that is still blocked parks again. A failure
to release is logged, because the switches are already saved
(AC-OFFICE-REVIEW-AUTHOR-EXCL-006.8).

The write has succeeded once the release starts, so the response is 200
whatever follows. If the seat steps cannot be read back,
`author_exclusion_steps` is `null` and the client reloads the page data
(AC-OFFICE-REVIEW-AUTHOR-EXCL-006.12).

**Web:** `useWorkspaceRouting` exposes `authorExclusionSteps` and
`updateAuthorExclusion(changes)`. It returns steps, warnings, and execution
profiles only once they were loaded for the current workspace, and ignores a
save response that arrives after the operator switched workspace
(AC-OFFICE-REVIEW-AUTHOR-EXCL-006.10). When a save response has null steps,
the browser applies the saved switch values to the current list and calls
`refresh()`. If the refresh fails, the saved values stay in the list. The
response from a later save supersedes an earlier refresh
(AC-OFFICE-REVIEW-AUTHOR-EXCL-006.12). The routing page keeps the switches in its
existing draft, so one Save applies routing changes first and then the
switch changes; the warnings come from the last response. The section is an
`AuthorExclusionStepsCard` under the provider order: a title, a description
of what the setting does and that reviews wait when no other provider is
available, and one row per seat step with a role badge and a `Switch`
(44px target on coarse pointers). The rows wrap on phone widths; there is no
separate phone composition.

**Save bar** (AC-OFFICE-REVIEW-AUTHOR-EXCL-006.7): the `/office/workspace/routing`
route mounts `SettingsSaveProvider` with `placement="content"`, as
`/office/workspace/settings` does. The page registers one
`useSettingsSaveContributor` (`office-routing`) whose `isDirty` covers the
routing draft and the switch changes, whose `save` runs the two writes in the
order above and rethrows a failure so the bar shows the error, and whose
`discard` resets both drafts. The page's own bottom Save button is removed;
the provider's navigation guard covers leaving with unsaved changes.

## Failure and recovery

- **Park lift.** Lifting a parked run bumps `route_cycle_baseline_seq` and
  re-dispatches. The author skip is recomputed and is never subject to the
  baseline, so a lifted run still excludes the author.
- **Post-start fallback** re-enters `DispatchWithRouting` and therefore
  re-applies the exclusion.
- **Undecodable step events.** Treated as a storage error under
  AC-OFFICE-REVIEW-AUTHOR-EXCL-004.4, not as "no declaration". A corrupted row
  must not silently allow a self-review.

## Observability

- Run events, as listed above.
- Route attempts with the `skipped_author_provider` outcome.
- The existing `routing_parked_runs_total` counter counts parks by status.
  No new metric label is added. Task, run, and provider identifiers stay out
  of metric labels.

## Requirement mapping

| Requirement | Section |
|---|---|
| AC-001.1, .2 | §1 (verbatim config copy; loader and export tests) |
| AC-001.3 | §3 step 2, §5 (no event when not applicable) |
| AC-001.4 | `config/workflows/office-default.yml` unchanged; builtin test |
| AC-001.5 | §1 (engine does not read the field in casting) |
| AC-002.1 to .4 | §2, §2a, §3, §4, §5 |
| AC-002.5 | §4 (disabled branch), §5 (`routing_disabled` event) |
| AC-002.6 | Context (ceiling settlement) |
| AC-002.7 | Context (scope), §7 (`agent_bound_profile`) |
| AC-003.1 to .4 | §4, §5 (`parkRunBlocked`), §6 |
| AC-004.1 to .4 | §3, §5 |
| AC-005.1, .2 | §7 |
| AC-005.3 | §8 (import preview and import), §9 (import dialog) |
| AC-005.4 | §8 (workflow sync) |
| AC-005.5, .6 | §8 (routing GET/PUT), §9 (banner) |
| AC-005.7 | §9 (locales, wrapping layout) |
| AC-005.8 | §8 (`check_failed`) |
| AC-005.9, .10 | §8 (sync text), §9 (warning copy) |
| AC-006.1, .5 | §10 (read, web) |
| AC-006.2, .3, .4 | §10 (write) |
| AC-006.6 | §10 (web), §9 (locales) |
| AC-006.7 | §10 (save bar) |
| AC-006.8, .9, .11 | §10 (write) |
| AC-006.10, .12 | §10 (web, write) |
