---
status: draft
created: 2026-09-02
updated: 2026-09-02
revision: 4
owner: nova28
---

# Cross-workspace task handoff (`handoff_task_kandev`)

## Why

An Office agent can reach a GO decision and has no authorised way to turn it into
work. Discovery runs in one workspace and delivery in another; a task cannot move
between workspaces, so the decision has to **create** a card in the target
workspace. Nothing on the Office agent's surface can do that.

Both available paths fail, for different reasons, and both were read rather than
assumed:

- **Runtime syscall `POST /runtime/tasks`.** `Actions.CreateTask`
  (`apps/backend/internal/office/runtime/actions.go:81`) reads
  `runCtx.WorkspaceID` and passes it straight to
  `CreateOfficeTaskAsAgent(ctx, runCtx.AgentID, runCtx.WorkspaceID, …)` at
  `:116`. `RunContext.WorkspaceID` (`runtime/context.go:25`) is the agent's own
  workspace and is not a request field. There is no target-workspace input.
- **MCP `create_task_kandev`.** It accepts an explicit `workspace_id`
  (`internal/mcp/server/server.go:1332`) and so *could* cross, but it is not in
  the Office toolset (`apps/backend/config/prompts/office-context.md` lists the
  Office inventory and does not include it), and that same file instructs Office
  agents to "not search for additional Kandev MCP tools".

So a correct gate decision could not translate into work and a human created the
delivery card by hand.

Widening `create_task_kandev` into the Office surface is the wrong fix twice
over. It would grant every Office agent the ability to create a task in any
workspace when only the decision-maker needs it, and the resulting row would be
indistinguishable in the activity log from ordinary task creation — making the
highest-consequence action in the discovery flow the least auditable one.

### What this spec is not

It is not a delivery-argument resolver. Which workspace, workflow, repository and
profiles a discovery project delivers into is carried elsewhere (today, in the
Office project record's prose). This spec supplies the **call**; that supplies the
**arguments**. Both are needed, and neither is sufficient alone.

It is also not a second idempotency mechanism. Create-idempotency already exists,
is specified, and is reused verbatim here; see R6.

## Prior art

### Our own prior reasoning

**Receipt.** `wiki-query @henry`; resolved
`OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki`, QMD collection `wiki`
(443 docs) over the `mcp__qmd__query` MCP transport, not the grep fallback. Three
searches (lex + vec + hyde) on cross-boundary delegation and provenance
back-links, then one on least-privilege tool surfaces. `obsidian-wiki` is not
installed, so the GraphRAG pre-pass was skipped. The skill's `log.md` append was
refused by the sandbox (the vault is outside this worktree); not routed around.

Two pages already hold positions that outrank fresh reasoning here.

**`[[closure-governance]]`** (tier core, draft, updated 2026-08-05) diagnoses this
exact shape one layer up: *"We do not have a gate shortage. We have zero enforced
dispositions."* Ours is the same failure displaced by one step: the agent **may**
decide, and then nothing may act on the decision. Two of its five closure elements
are adopted directly:

- **Named authority** — "a config file naming who may approve". Here: a
  capability granted per agent or per role, not a global widening (R2).
- **Evidence that cannot be invented** — "chat-only output is not sufficient";
  a canonical stored record. Here: provenance written to both task rows and to
  the activity log, not narrated in a comment or a prompt (R4).

Its third element, **a clock** (a deadline on an open disposition), is
deliberately not adopted: a handed-off card's clock is the delivery workflow it
lands in, which already has one. Named, not silent.

**`[[queryable-company]]`** (draft, updated 2026-05-26) supplies the recording
obligation: *"If it is recorded, it happened to the AI. If it did not get
recorded, it did not happen to your intelligence."* That is the argument for the
handoff being a distinct, queryable activity verb rather than an ordinary
`create_task` row.

**Where we depart from it.** The same page argues for trust-by-default *instead
of* a permission model, resting on an egalitarian org. We take the recording
obligation and reject the permission conclusion. Kandev's workspace boundary is
also its **ownership** boundary under per-user scoping
(`internal/task/service/service_access.go:37`), the callers are unattended agents
rather than employees whose transcripts colleagues read, and a card created in
someone else's workspace can start an executor. Auditability *and* least
privilege, not auditability instead of it.

**`[[agent-tool-registry]]`** contributes the DRY+MECE resolver discipline: two
tools that do the same thing make the agent pick poorly. That is an independent
argument for R5's same-workspace refusal.

### What other products shipped

**Receipt.** `saas-kb` `search_fsm_docs`, `category: "ai_sdlc"`, queries "cross
workspace task delegation agent handoff between projects" (a first query
containing commas failed with an fts5 syntax error and was rewritten). Two
documents read in full.

**Augment Code / Cosmos, "Delegating Work"** is the closest shipped analogue. A
manager Expert launches a **Worker** — "a separate Cosmos session with its own
VM, Environment, integrations, and permissions". Two vendor claims map onto this
spec: a session "should be explicitly prompted about when to launch a worker …
Cosmos does not launch one implicitly" (a boundary crossing gets its own verb);
and "keep worker permissions narrow".

**Warp, "Handoff between local and cloud agents"** names three directions, states
explicitly *what carries over*, and is honest that handoff is *best-effort*: when
the receiving side cannot apply the prior state it "reports which changes failed
to apply and continues". The borrowing is reporting partial success rather than
implying completeness — see AC-29.

**What we do differently.** Neither records a **two-way** link as a hard
requirement. Warp forks forward; Augment's Expert-to-Expert coordination uses a
shared integration — "the pull request is the medium" — which works only because
a PR already exists. Here the whole point is that the work has not started, so
there is no shared artefact to coordinate through: the reverse link is
load-bearing, and R4 makes it a contract rather than a best effort. We also cross
an ownership boundary neither product's handoff crosses, which is why R2 exists.

## Terminology

**Source task** — the Office task whose session calls the tool; its workspace is
the **source workspace**. **Target workspace** — the one named by
`target_workspace_id`, which must differ. **Delivery task** — the task created
there.

## Input inventory

Sampled from the tree at `646ff0063`, not assumed. Revisions 2 and 4 append further
rows, sampled while closing the round-1 and round-3 review findings respectively.

| Fact | Where |
|---|---|
| Office and kanban tasks share one `tasks` table and one workspace concept; "office" is `Task.IsFromOffice` plus a project and an office workflow | `internal/backendapp/adapters_office.go:167`, `internal/orchestrator/executor/executor_execute.go:88` |
| Office task creation writes **no** launch metadata | `adapters_office.go:171` |
| `CreateTaskInput` has no `agent_profile_id` or `executor_profile_id` field, so those keys in a `POST /runtime/tasks` body are dropped by `encoding/json` silently; `unsupportedField()` rejects six other keys but not these two | `internal/office/runtime/actions.go:44-76` |
| `create_task_kandev` does **not** silently discard an explicit executor: `resolveMCPInheritedExecutors` threads `executorProfileID` through and only fills it when empty | `internal/mcp/handlers/handlers.go:1444` |
| `create_task_kandev` already refuses an unresolvable agent profile with `errMCPAgentProfileRequired` mapped to `ErrorCodeValidation` | `handlers.go:847`, `:1315` |
| Tool registration is a declarative profile registry: `profileToolGroup{name, enabled, register}` with `surfaceEnabled` / `capabilityEnabled` / `andProfilePredicates` | `internal/mcp/server/server.go:980-1020` |
| Capabilities are typed and additive; the Office surface is `SurfaceOfficeTask` | `internal/mcp/profile/profile.go:16-30` |
| The session profile is derived from the task row at launch | `executor_execute.go:61-100` |
| Office role permissions are role defaults merged with per-agent JSON overrides, with an anti-escalation check | `internal/office/shared/permissions.go` |
| An Office `AgentInstance` **is** an `AgentProfile` (type alias), carrying `WorkspaceID`, `Role`, `Permissions` | `internal/office/models/models.go:57`, `internal/agent/settings/models/models.go:134-189` |
| In-session MCP calls carry a server-derived principal (`WorkspaceID`, `CallerTaskID`, `CallerSessionID`, `Surface`) built from the execution, never from the payload | `internal/mcp/scope/principal.go:35-72` |
| They also carry the task owner's identity, so `authorizeWorkspaceID` applies and a foreign-owner workspace is denied with a *not-found* sentinel (no existence leak) | `internal/mcp/scope/scope.go:1-18`, `internal/task/service/service_access.go:43-56` |
| Tool discovery is explicitly **not** an authorisation boundary: "an agent can still send a raw WebSocket action" | `internal/mcp/handlers/automation_authorization.go:33-35` |
| `record_step_decision_kandev` precedes this as an Office-only, role-gated MCP tool: session → `AgentProfileID` → role → `shared.ErrForbidden` → `ws.ErrorCodeForbidden` | `internal/mcp/handlers/agent_decision_handlers.go:31-91` |
| Task titles are capped at 60 runes with a typed error | `internal/task/service/task_title.go:11-21` |
| Activity entries are workspace-scoped and logging failures are swallowed by design | `internal/office/shared/activity.go:29-60` |
| `resolveMCPDestinationStep(workflowID, startAgent)` returns the first `auto_start_agent` step when `startAgent` is true, otherwise the start step | `handlers.go:1597-1620` |
| `TestSyspromptToolNames_ExactlyMatchMCPOfficeMode` asserts the Office prompt advertises **exactly** the `ModeOffice` inventory (set equality, not subset) | `internal/mcp/server/sysprompt_sync_test.go:243-259` |
| The Office prompt already has a precedent for a conditionally advertised instruction: the `{step_complete_instruction}` placeholder | `internal/sysprompt/sysprompt.go:170-186`, `config/prompts/office-context.md:21` |
| `TestOfficeContext_ContainsOnlyOfficeCapabilities` asserts `create_task_kandev` never appears in the Office prompt | `internal/sysprompt/sysprompt_test.go:248-262` |
| **Create-idempotency is already specified and shipped.** Four outcomes — Created, Found-settled, Found-unsettled, Created-identity-lost — and `creation_complete` means only "the returned task finished its required synchronous work" | `docs/specs/tasks/requirements/external-id-idempotency.md` |
| **Both Found outcomes have no side effects**: no task row, session, launch, repository attachment, workspace-policy write, branch, or `task.created` event | same file, *What* |
| Releasing an identity because a create reported `creation_complete: false`, then re-creating, is the documented **unsafe** move; it reintroduces the duplicate | same file, *The one unsafe thing a caller can do* |
| **Cross-workspace uniqueness and a global external-id namespace are explicitly out of scope**; uniqueness is `(workspace_id, external_id)` | `docs/specs/tasks/requirements/external-id-idempotency-boundaries.md` |
| MCP gets create-if-absent, **not** a side-effect-free probe; there is no MCP lookup tool | same file |
| `auto_start_on_create` is a **positive opt-in**. Absence deliberately means *do not launch*; an earlier version that launched on absence is documented as a fixed bug. Only `CreateOfficeTaskInWorkflow` sets it | `internal/orchestrator/event_handlers_workflow.go:805-855`, `internal/task/models/models.go:205-246` |
| `UpdateTaskMetadata` is `GetTask` → shallow key-merge → `UpdateTask`. No lock, no compare-and-set, and `tasks` has no version column | `internal/task/service/service_workflow.go:359-391` |
| The in-repo precedent for the **typed conflict error** half of a same-row TOCTOU is `ErrWorkflowResolutionConflict`. Its partner `UpdateTaskIfWorkflowMatches` is **not** a precedent for a key-scoped write — see the row below | `internal/task/repository/sqlite/task.go:545-556`, `internal/task/repository/repoerrors/errors.go:54-61` |
| `validateMCPWorkflowWorkspace`'s message **embeds the owning workspace id**: `"workflow_id %q belongs to workspace_id %q, not %q"`. It has no no-leak property | `handlers.go:1229-1245` |
| `service.ValidateTaskTitle`'s message states only the limit: `"task titles must be %d characters or fewer"` | `task_title.go:21` |
| `description` is unconstrained `TEXT`; there is no column-level length limit on a task description | `internal/task/repository/sqlite/base_schema.go:375` |
| Task **priority exists**: a `priority` column, `defaultPriority = "medium"`, validation against a four-value enum, a create-time write and an update-time patch | `base_schema.go:377`, `service_tasks.go:33,785-790,830`, `service_requests.go:77,125` |
| The activity log is entirely Office-scoped: models, repository and every reader live under `internal/office/`, read by `ListActivityEntries(workspaceID, …)` | `internal/office/repository/sqlite/activity.go:65-130` |
| `LogActivityWithRun`'s `details` parameter is a plain `string`, and its `runID` / `sessionID` are documented as "pass empty when genuinely user-initiated" | `internal/office/shared/activity.go:38-60` |
| `internal/mcp/server/handoff_handlers.go` already exists and implements an **unrelated** same-workspace parent/child *document* handoff | `handoff_handlers.go:28-32` |
| `ws.NewError` carries a `details map[string]interface{}`; the MCP tool surface's error helper is `mcp.NewToolResultError(string)`, a single string | `pkg/websocket/message.go:114`, `handoff_handlers.go:80` |
| A legacy spec over 32,768 bytes registers a ceiling in `spec-lint-exceptions.tsv` that must **equal the file size exactly** — `size < ceiling` is a `stale-size-exception` violation, so a ceiling may not carry headroom | `scripts/lint-spec-files.py`, `check_size_exception_catalog` |
| The size **ratchet** compares only against the sidecar in HEAD's first parent; a path absent there is skipped, so a **new** entry may be set to any size at or above the default limit. It may only be lowered once committed | `scripts/lint-spec-files.py`, `load_previous_size_exceptions`, `check_legacy_size_ratchet` |
| `isOfficeRequest(req)` is the **write-time** office predicate — true when `Origin` is `agent_created`, `routine` or `onboarding`, **or** `ProjectID` is set. It gates `assignIdentifier`, which consumes the workspace task sequence and stamps a `PREFIX-N` identifier | `internal/task/service/service_tasks.go:116,293,844-855` |
| `Task.IsFromOffice` is a **different**, read-time SQL projection (`IsFromOfficePredicate`) over office-workflow membership and `project_id`. `Origin` does not affect it | `internal/task/repository/sqlite/task.go:79,138` |
| `create_task_kandev` sets **no** `Origin`, so `buildTask` defaults it to `TaskOriginManual`. `TaskOriginAgentCreated` is written only by the Office runtime create path | `handlers.go` (no `Origin:` assignment), `service_tasks.go:777-780`, `adapters_office.go:168` |
| `Service.CreateTask` returns only **three** outcomes. `CreatedIdentityLost` "is not produced here — it is decided by the handler during settlement, after this call returns", via `SettleExternalID`; the existing handler runs it after policy attach and before launch, and maps `settled == false` to identity-lost with `creation_complete: true` | `service_tasks.go:123-144`, `handlers.go:955-980` |
| The existing launch dispatch `launchAutoStartTask` has **no return value**, calls `LaunchSession` inside a goroutine and logs-and-discards its error, and returns silently when the launcher is nil or the profile is empty | `handlers.go:1663-1695` |
| `SelectAutoStartStep` replaces its candidate only on `step.Position < best.Position` (strict), and `SelectStartStep` returns the first slice element carrying `IsStartStep`. Neither defines a tiebreak for equal `position` | `internal/workflow/models/models.go:260-289` |
| A **key-scoped** metadata compare-and-set already exists: `SetTaskMetadataKeyIfStamp` patches a single JSON key via `json_set`/`jsonb_set` under a `FOR UPDATE` row lock, leaving sibling keys untouched | `internal/task/repository/sqlite/metadata_launch_error_cas.go:37,119,165-167` |
| `UpdateTaskIfWorkflowMatches` guards a scalar column but writes the **whole** marshalled metadata blob through `updateTaskTx`, exactly as plain `UpdateTask` does | `task.go:556-578` |
| `validateMCPWorkflowWorkspace` has **three** branches, not two: not-found → `Validation`, any other read error → `InternalError`, wrong workspace → `Validation` | `handlers.go:1233-1242` |
| The only office-run resolver is task-keyed — `ResolveRunForTask` → `GetClaimedRunByTaskID`. The `Run` record itself carries `SessionID` and `AgentProfileID` | `internal/office/service/activity.go:55-64`, `internal/office/models/models.go:383,394` |
| `models.PublicTaskMetadata`, which `dto.FromTask` applies, is a **denylist**: it clones the map and strips only two keys inside `deferred_launch`. Arbitrary new top-level keys pass through unredacted | `internal/task/models/public_metadata.go:8-22`, `dto.go:885` |

| `defaultPermsForRole` lists **every** boolean permission key explicitly in all seven branches (six named roles plus `default:` for specialist and unknown), including the `false` ones; `HasPermission` separately reads a missing key as false | `internal/office/shared/permissions.go` |
| The agent permission surface is backed by **two** lists that must agree: `shared.AllPermissionKeys()` and `allPermissionMeta()`, the latter carrying `Key`, `Label`, `Description`, `Type`. `TestPermissionMetadataMatchesKnownPermissionKeys` asserts `reflect.DeepEqual` over their key slices — order-sensitive | `internal/office/shared/permissions.go`, `internal/office/dashboard/handler_settings.go`, `internal/office/dashboard/handler_settings_test.go` |
| Permission `Label` / `Description` are English string literals in Go, served by the backend settings endpoint. **No** permission key appears in `apps/web/src/locales/**` | `handler_settings.go`, `apps/web/src/locales/` |
| Both profile-belongs predicates already ship side by side: `AgentProfileBelongs` returns `WorkspaceID == "" \|\| WorkspaceID == workspaceID`; `ExecutorProfileBelongs` takes the workspace as `_ string` and returns existence only | `internal/backendapp/turn_adapters.go` |
| `ExecutorProfile` has **no** workspace field; `AgentProfile.WorkspaceID` is documented "Empty = global / kanban-legacy" | `internal/task/models/models.go`, `internal/agent/settings/models/models.go` |
| `internal/mcp/handlers` imports `office/dashboard`, `office/shared`, `office/models` — and **not** `office/service`. Its Office dependency is `dashboardSvc *dashboard.DashboardService`, wired by `SetDashboardService` | `internal/mcp/handlers/handlers.go`, `internal/mcp/handlers/agent_decision_handlers.go` |
| `DashboardService`'s exported run surface is `ListRuns(wsID)`, `GetLiveRuns`, `GetRunsByCommentIDs` — no single-run-by-id accessor. `RunResolver` is `ResolveRunForTask(ctx, taskID) string`, held unexported and wired via `SetRunResolver` from the office service at startup | `internal/office/dashboard/service.go`, `internal/backendapp/main.go` |
| `office/service.Service.GetRun(ctx, id) (*models.Run, error)` exists and returns the full run; `GetClaimedRunByTaskID` is `ORDER BY claimed_at DESC LIMIT 1`, so it yields exactly one candidate | `internal/office/service/failure.go`, `internal/runs/repository/sqlite/runs.go` |

Two corrections to the originating card, recorded so no one re-derives them:

1. The card says "Passing the executor alone silently discards it today." That is
   true of `POST /runtime/tasks`, where the field does not exist on
   `CreateTaskInput` — not of `create_task_kandev`, which preserves it. R3 stands
   on its own grounds and does not rest on the wrong premise.
2. The card lists `priority` as optional. It is excluded, but **not** because the
   concept is missing — see **Out of scope**, which revision 2 corrects.

## Determinism and boundary rules

- **D1 — the delivery task is a kanban task, not an Office task.** It is created
  through the same task-service path `create_task_kandev` uses, and
  `IsFromOffice` is false. Rationale: the Office creation path writes no launch
  metadata, and `IsFromOffice` selects `SurfaceOfficeTask` at session launch
  (`executor_execute.go:88`) — a delivery card marked office-owned would come up
  with the Office toolset and no kanban tools.
- **D2 — the tool supplies no `Origin`, so the delivery task is `manual`.** It
  passes no origin at all, exactly as `create_task_kandev` does, and `buildTask`
  defaults the column to `TaskOriginManual`. Revision 2 said the delivery task
  "keeps the existing agent-created origin"; that was wrong twice and is corrected
  here. There is no agent-created origin to keep on the path D1 mandates —
  `create_task_kandev` sets none, and `TaskOriginAgentCreated` is written only by
  the Office runtime create path that D1 excludes. And setting it would *break*
  D1: `Origin == TaskOriginAgentCreated` makes `isOfficeRequest(req)` true, which
  is the task service's own write-time definition of "should create an office
  task", and that runs `assignIdentifier` — consuming the target workspace's task
  sequence and stamping the delivery card with an office `PREFIX-N` identifier no
  other kanban card on that board carries. No *new* origin value is introduced
  either: `Task.Origin` selects behaviour (`TaskOriginAutomationRun` switches the
  whole MCP surface), so a new value would be a behavioural change wearing an
  audit label. The handoff is distinguished by metadata and by the activity verb.
  Observed by AC-23a.
- **D2a — "office" means two different things here, and the delivery task must be
  non-office under both.** `isOfficeRequest` is the **write-time** predicate, keyed
  on `Origin` and `ProjectID`, and it gates office-only creation side effects.
  `Task.IsFromOffice` is the **read-time** SQL projection over office-workflow
  membership and `project_id`, and it is what selects `SurfaceOfficeTask` at
  session launch. `Origin` does not affect the second, so passing the
  `IsFromOffice` test alone does not satisfy D1. The delivery task therefore
  carries no `project_id`, lands in a non-office target workflow, and carries no
  office-triggering `Origin`.
- **D3 — three independent gates.** (1) discovery: the profile capability decides
  registration; (2) execution: the backend handler re-derives the permission from
  the trusted principal; (3) ownership: the existing per-user workspace scoping on
  the target. Each is load-bearing. (1) alone is not a boundary; (2) alone would
  let an agent act in a workspace its owner cannot see; (3) alone would let any
  Office agent hand off inside its own owner's estate.
- **D3a — evaluation order is a total order, because a partial one leaks and
  because "which error is named" is otherwise undefined.** Checks run in exactly
  this sequence, and the **first** failure is the one reported; no call reports two
  failures, and no later check runs after an earlier one fails:

  1. **Argument shape**, naming no target resource: unknown argument present;
     required argument absent, empty or whitespace-only; optional string argument
     present but whitespace-only; `title` over 60 runes. Within this group, ties
     are broken by the argument order of the R1 table, top to bottom.
  2. **Same-workspace refusal** (AC-22). It compares the payload's
     `target_workspace_id` against the principal's own workspace and reads no
     resource, so it cannot leak; placing it here also means a caller that
     mistakenly targets itself is told so rather than being told it lacks a
     permission.
  3. **Gate (2) — the handoff permission**, re-derived from the trusted principal.
  4. **Gate (3) — target workspace ownership**, via the existing scoping.
  5. **Target-resource checks**, in this order: `workflow_id`; destination step
     resolution; `agent_profile_id`; `executor_profile_id`; `repository_id`;
     `base_branch`.
  6. **Idempotency resolution and the create itself** (R6).

  Steps 1 and 2 name no target resource. Every check from step 5 on reads one, and
  running any of them before steps 3 and 4 would let an unauthorised agent
  distinguish a real workflow id from a fabricated one by the error it gets back,
  defeating AC-10 and AC-11.
- **D3b — the post-create sequence is a total order too, because settlement,
  linking, auditing and launching are not interchangeable.** After D3a step 6 the
  tool performs exactly these, in this order:

  1. **Resolve the idempotency outcome.** On either Found outcome, skip step 2 and
     step 5 entirely — AC-24a forbids all target-side work there — and continue at
     step 3 via AC-25/AC-25a.
  2. **Settle the external id** (AC-24d). Created path only.
  3. **Write the reverse link** (AC-17, AC-27), or repair it (AC-25).
  4. **Write both activity entries** (AC-19), which never fail the call (D6).
  5. **Dispatch the launch** (AC-32). Created path only, and only when
     `start_agent` was true.

  A step that refuses ends the call and no later step runs; AC-25a's mismatch
  refusal at step 3 is the reachable case, and it writes no reverse link, no
  activity entry and no launch.

  Rationale, stated so nobody reorders these by accident: settlement must precede
  launch because an unsettled or identity-lost create must not start an agent —
  the rule the existing create path already follows — and the reverse-link
  **attempt** must precede the launch so that the source-side record is given its
  chance while the call can still report the result in one response. Activity sits
  between them because it is the only step that cannot fail the call, so its
  position changes no outcome.

  **What this ordering does and does not buy.** It does **not** make the launch
  conditional on the reverse link, and no reader should infer that it does: what
  makes a running delivery task's source findable is the **forward** link, which
  D4 guarantees unconditionally by writing it inside the create itself, "so a
  delivery task with no source is unreachable by any interleaving". The reverse
  link is a source-side **index** over that same fact, with AC-25's replay as its
  repair path. A reverse-link failure is therefore not a refusal (AC-29 makes it a
  non-error), step 3 does not end the call, and steps 4 and 5 still run — including
  the launch. AC-32 states this as an explicit exhaustion of its own conditions.
  An earlier revision of this rationale claimed the ordering guaranteed that "a
  delivery task that is already running is never one whose source card cannot be
  found"; ordering alone cannot deliver that and only gating could, so the sentence
  overclaimed and is corrected here rather than being turned into a gate.
- **D3c — a lookup that fails to execute is never a validation error.** Across
  every target-resource check in D3a step 5, a backend read that fails for a
  reason other than absence SHALL be reported as `ErrorCodeInternalError` and
  SHALL be safe to retry; only a read that executes and returns no row, or returns
  a row belonging to another workspace, SHALL be `ErrorCodeValidation`. AC-15b
  already says this for step resolution; AC-12b generalises it to the rest.
  Rationale: telling an automated caller that a transient database failure was its
  own input mistake makes it give up on a call it should have retried, and the
  four other checks in that group would otherwise each be decided by whoever
  implements them.
- **D4 — the forward link is written in the same create that makes the task.** It
  is part of the create request's metadata, never a follow-up update, so a
  delivery task with no source is unreachable by any interleaving.
- **D5 — the reverse link is a compare-and-set append, not a read-modify-write.**
  The mechanism is named in AC-27 and is required, not suggested; the ordering it
  must preserve is in AC-28.
- **D6 — activity logging never fails the call.** `ActivityLoggerImpl.LogActivity`
  swallows repository errors by design; that contract is inherited, not
  re-litigated. Activity is the human-visible record, not the durable one.
- **D7 — one clock, server-side.** `handed_off_at` is the backend's UTC time at
  the moment the delivery task is created. The same instant is written to both
  sides (AC-16, AC-17); neither is re-stamped on a replay. No caller-supplied
  timestamp is accepted. Consequently a replay has **no clock of its own**: every
  `handed_off_at` a Found outcome writes or reports is read back from the found
  task's stored `handoff_source` (AC-25, AC-33), and a stored value that cannot be
  read is reported as missing rather than replaced (AC-25b).
- **D8 — provenance has a named schema, not "metadata".** Both records live in
  `Task.Metadata` under the keys AC-16 and AC-17 name. Timestamps are RFC 3339 in
  UTC with a `Z` offset and millisecond precision. Neither record carries a schema
  version field: adding one now would be a version nobody reads, and the keys are
  additive, so a later shape change introduces a new key rather than reinterpreting
  this one.
- **D9 — this tool never stamps `auto_start_on_create`.** That marker is a
  positive opt-in whose absence deliberately means *do not launch*, and a create
  path that launched on absence is recorded in the code as a fixed bug. Stamping
  it here would reintroduce that bug for cross-workspace cards specifically — the
  riskiest possible place. Consequence, stated so nobody re-derives it:
  `start_agent: false` **is** honoured at create time even when the destination
  step carries an `auto_start_agent` `on_enter` action.
- **D10 — a supplied profile is authoritative.** Neither the target workflow, nor
  the destination step, nor the target workspace's defaults, nor the source
  session's profile may override `agent_profile_id` or `executor_profile_id`. See
  AC-14a.

## What

### R1 — one narrow tool, on the Office surface only

- **AC-1.** The system SHALL register an MCP tool named `handoff_task_kandev` as
  one entry in the declarative profile registry (`profileToolGroups`) whose
  predicate is `andProfilePredicates(office, capabilityEnabled(<the handoff
  capability>))`. The tool SHALL therefore be absent from the Kanban, External,
  Configuration and Automation surfaces regardless of capability, and absent from
  the Office surface without it.

- **AC-2.** The system SHALL define the handoff capability as a new value of the
  existing `mcpprofile.Capability` type. No new gating mechanism SHALL be
  introduced.

- **AC-2a.** The tool's registration, handler and tests SHALL NOT be added to
  `internal/mcp/server/handoff_handlers.go`, which already implements the
  unrelated same-workspace parent/child **document** handoff
  (`list_task_documents_kandev`, `write_task_document_kandev`). The word "handoff"
  is already taken in that package; this tool lives in its own file so the two
  concepts are not conflated by name.

- **AC-3.** `apps/backend/config/prompts/office-context.md` SHALL advertise the
  tool through a **placeholder** resolved at prompt-format time, filled only when
  the capability is granted, in the manner of `{step_complete_instruction}`. The
  raw template returned by `sysprompt.OfficeContext()` SHALL NOT contain the
  literal `handoff_task_kandev`, so
  `TestSyspromptToolNames_ExactlyMatchMCPOfficeMode` continues to hold unchanged
  for the ungranted profile. A new assertion SHALL cover the granted form: the
  resolved prompt's `_kandev` tool set equals the registered tool set of an
  Office profile carrying the capability.

- **AC-4.** The same file's instruction not to search for additional Kandev MCP
  tools SHALL be amended so it does not contradict a tool the same prompt
  advertises. The amended sentence SHALL still forbid discovering tools **not**
  listed in that prompt.

- **AC-5.** The tool SHALL accept exactly these arguments, and reject any call
  carrying an argument outside this set with `ErrorCodeValidation` naming the
  offending argument:

  | Argument | Required | Notes |
  |---|---|---|
  | `target_workspace_id` | yes | Must differ from the caller's workspace (R5) |
  | `workflow_id` | yes | Must belong to `target_workspace_id` |
  | `title` | yes | ≤ 60 runes |
  | `prompt` | yes | The delivery agent's first user message |
  | `agent_profile_id` | yes | See R3 |
  | `executor_profile_id` | yes | See R3 |
  | `repository_id` | no | Must already exist in `target_workspace_id` (AC-5b) |
  | `base_branch` | no | Only with `repository_id` (AC-5b) |
  | `start_agent` | no | Default **false** (R7) |
  | `external_id` | no | Create-idempotency key (R6) |

  A required argument that is absent, empty, or whitespace-only SHALL be rejected
  with `ErrorCodeValidation` naming that argument. Rejection SHALL happen before
  any write.

- **AC-5a.** An **optional string** argument that is present but empty or
  whitespace-only SHALL be rejected with `ErrorCodeValidation` naming it, rather
  than being silently treated as absent. A caller that means "no repository" omits
  the key; a blank value is a caller bug and is reported as one.

- **AC-5b.** `repository_id`, when supplied, SHALL attach exactly one repository
  to the delivery task as its only repository entry. It SHALL be validated to
  exist in `target_workspace_id` and SHALL be refused with `ErrorCodeValidation`
  otherwise. `base_branch` supplied **without** `repository_id` SHALL be refused
  with `ErrorCodeValidation` naming `base_branch`, rather than ignored.
  `repository_id` supplied without `base_branch` SHALL use that repository's
  `default_branch`, matching the existing explicit-repository behaviour. When
  `repository_id` is omitted the delivery task SHALL be created with no
  repositories, and no repository SHALL be inherited from the source task.

- **AC-6.** A `title` longer than 60 runes SHALL be rejected with
  `ErrorCodeValidation` whose message states **both** the 60-rune limit and the
  actual rune length. Length SHALL be counted in runes against the same limit
  `service.ValidateTaskTitle` enforces; a title of exactly 60 runes is accepted.
  The tool SHALL compose this message itself: `ValidateTaskTitle`'s own message
  states only the limit (`"task titles must be %d characters or fewer"`) and
  reusing it verbatim would not satisfy this criterion. The title SHALL NOT be
  truncated.

- **AC-6a.** No length limit SHALL be imposed on `prompt`. This is a decision, not
  an omission: the task `description` column is unconstrained `TEXT`, so there is
  no column limit to defer to, and inventing one here would put a cap on
  cross-workspace handoffs that same-workspace creation does not have. The
  transport's own message-size limit is the only bound, and exceeding it is a
  transport error rather than a validation failure.

### R2 — authorisation is granted per agent or per role, and enforced at execution

- **AC-7.** The system SHALL define a new Office permission key (`can_handoff_tasks`)
  in `internal/office/shared/permissions.go`, as the constant `PermCanHandoffTasks`,
  and SHALL default it to **true for the `ceo` role and false for every other role**,
  including `specialist` and any unknown role. Because `defaultPermsForRole` lists
  every boolean key explicitly in every branch rather than relying on absence, the key
  SHALL be added to **all seven** returned maps: `true` in the `AgentRoleCEO` branch,
  and `false` in `AgentRoleAssistant`, `AgentRoleWorker`, `AgentRoleSecurity`,
  `AgentRoleQA`, `AgentRoleDevOps` and the `default:` branch that serves `specialist`
  and unknown roles. (`HasPermission` already reads a missing key as false, so the
  `false` entries are for the settings surface's benefit, not the gate's — they make
  the toggle render unchecked instead of absent.)

- **AC-7a.** The key SHALL appear in the agent permission surface, which requires
  **two** lists to be updated in lockstep, not one:
  - `shared.AllPermissionKeys()` (`internal/office/shared/permissions.go`), which is
    display order; and
  - `allPermissionMeta()` (`internal/office/dashboard/handler_settings.go`), which
    supplies the surface's rendered copy.

  `TestPermissionMetadataMatchesKnownPermissionKeys`
  (`internal/office/dashboard/handler_settings_test.go`) asserts
  `reflect.DeepEqual` between `allPermissionMeta()`'s key slice and
  `AllPermissionKeys()` — **order-sensitive exact equality** — so updating either list
  alone SHALL fail that test. This criterion is stated because AC-7 alone reads as a
  single-file change and is not one.

  The entry SHALL be inserted at the **same index in both lists**: immediately after
  `can_manage_own_skills` and immediately before `max_subtask_depth`, keeping the
  boolean permissions contiguous ahead of the one integer permission.

- **AC-7b.** The `allPermissionMeta()` entry SHALL be exactly:

  | Field | Value |
  |---|---|
  | `Key` | `shared.PermCanHandoffTasks` |
  | `Label` | `Hand off tasks` |
  | `Description` | `Allow this agent to hand off work to another workspace` |
  | `Type` | `bool` |

  These strings are fixed here rather than left to the implementer because they are
  **user-visible copy**, and copy invented at build time is copy nobody reviewed. The
  `Label` / `Description` register matches the existing entries (`"Create tasks"` /
  `"Allow this agent to create new tasks"`).

  These two strings are **English literals in Go** and SHALL NOT be routed through the
  web i18n catalogs. The Office permission surface renders `Label` and `Description`
  as delivered by this backend endpoint; no locale key exists for any permission
  (`can_create_tasks`, `can_approve`, `can_manage_own_skills` and `max_subtask_depth`
  are all absent from `apps/web/src/locales/**`). Adding locale entries for this key
  is therefore **not** part of this change, and no `i18n:check` obligation arises from
  it. Revision 3 of this spec asserted the opposite under
  `## User-visible surfaces touched`; that was wrong and is corrected there too.

- **AC-8.** An Office session's MCP profile SHALL carry the handoff capability if
  and only if `shared.HasPermission(shared.ResolvePermissions(role, permissions),
  can_handoff_tasks)` is true for the agent profile backing that session. Role
  defaults merged with per-agent overrides is the single source; no second
  resolution path SHALL be added.

- **AC-9.** The backend handler SHALL re-derive the permission at execution time
  from the trusted principal (`mcpscope.PrincipalFromContext` →
  `CallerSessionID` → session → `AgentProfileID` → agent profile → role and
  permissions) and SHALL refuse with `ws.ErrorCodeForbidden` when it is not
  granted. It SHALL NOT read the caller's identity, workspace, role or capability
  from the request payload. This holds even when the tool was never advertised to
  that session: discovery is not a boundary
  (`automation_authorization.go:33-35`). When no principal is present, or the
  session or agent profile it names cannot be loaded, the call SHALL be refused
  with `ws.ErrorCodeForbidden` and no write SHALL occur. A lookup failure SHALL
  NOT fall through to an unscoped internal caller, which the task service reads
  as "allow everything".

- **AC-10.** An unauthorised call SHALL return a message that names the missing
  permission and says it is granted per agent or per role. The message SHALL NOT
  disclose whether `target_workspace_id` exists.

- **AC-11.** The target workspace SHALL be authorised by the existing per-user
  scoping before any write. A workspace whose owner is not the source task's
  owner SHALL be refused indistinguishably from a workspace that does not exist,
  preserving `authorizeWorkspaceID`'s no-existence-leak property. This check
  SHALL NOT be bypassed, duplicated, or reimplemented.

- **AC-12.** `workflow_id` SHALL be validated to belong to `target_workspace_id`.
  A workflow that does not exist, and a workflow that exists in another
  workspace, SHALL both be refused with `ErrorCodeValidation` and the **same**
  message, which names only `workflow_id` and states that it is not a workflow of
  the target workspace. The message SHALL NOT name, echo, or otherwise disclose
  the workspace the workflow actually belongs to, and the two cases SHALL NOT be
  distinguishable by message, code, or timing class.

- **AC-12a.** The tool SHALL NOT reuse `validateMCPWorkflowWorkspace`'s message.
  That helper's message embeds the owning workspace id
  (`"workflow_id %q belongs to workspace_id %q, not %q"`), which is exactly the
  disclosure AC-12 forbids; it has no no-leak property and is not a model for
  this tool. `create_task_kandev`'s use of it is unchanged — that tool's caller
  already names its own workspace, so the same message discloses nothing new
  there. Reusing the helper's *validation* is permitted; emitting its message to
  a cross-workspace caller is not.

- **AC-12b.** Every target-resource check in D3a step 5 — `repository_id`
  (AC-5b), target workspace ownership (AC-11), `workflow_id` (AC-12),
  `agent_profile_id` and `executor_profile_id` (AC-14), and step resolution
  (AC-15b) — SHALL distinguish absence from failure per D3c: a read that executes
  and finds nothing (or finds a row in another workspace) is
  `ErrorCodeValidation`; a read that fails to execute is `ErrorCodeInternalError`
  and SHALL be safe to retry. In both cases no write SHALL occur. The tool SHALL
  NOT fold a transient backend failure into a `Validation` refusal for any of the
  five. `validateMCPWorkflowWorkspace` already models this correctly with three
  branches — not-found → `Validation`, other read error → `InternalError`, wrong
  workspace → `Validation` — and AC-12/AC-12a govern only the two `Validation`
  ones; this criterion covers the third for `workflow_id` and imposes the same
  shape on the other four. AC-11's refusal remains indistinguishable from
  not-found, so its *failure* branch SHALL NOT disclose existence either.

- **AC-12c.** `workflow_id` SHALL additionally be refused with
  `ErrorCodeValidation` when it equals the target workspace's
  `office_workflow_id`, with a message saying the target must be a delivery
  workflow rather than the workspace's office workflow. Without this, D1 is
  defeated through an argument the caller controls: `IsFromOffice` is true when a
  task's `workflow_id` matches its workspace's `office_workflow_id`, so a delivery
  card placed in the office workflow comes up with `SurfaceOfficeTask` and no
  kanban tools at launch — the exact failure D1 exists to prevent, reached without
  any origin being set. A workspace whose `office_workflow_id` is empty has
  nothing to compare against and SHALL NOT be refused on this ground. This check
  occupies D3a step 5's `workflow_id` position and is subject to D3c.

### R3 — both profiles are required, and the delivery task is startable

- **AC-13.** The tool SHALL reject a call omitting `agent_profile_id` **or**
  `executor_profile_id` with `ErrorCodeValidation` naming the missing one, even
  when a workspace, workflow or step default could have supplied it. Rationale:
  the caller is choosing on behalf of a workspace it does not run in, and a
  silently-defaulted profile is exactly the failure this tool exists to stop
  reproducing. This is a deliberate divergence from `create_task_kandev`, whose
  resolution chain remains unchanged.

- **AC-14.** Both ids SHALL be validated before any write; one that does not resolve
  SHALL be rejected with `ErrorCodeValidation` naming which of the two failed. When
  both are unresolvable, `agent_profile_id` SHALL be the one named, per D3a's order.
  "Resolves" is defined per id type in AC-14b, because it is **not** the same test for
  the two and neither test is derivable from the phrase "usable in the target
  workspace".

- **AC-14b.** The two predicates SHALL be:

  | Id | Predicate |
  |---|---|
  | `agent_profile_id` | the profile exists **and** (`WorkspaceID` is empty **or** `WorkspaceID` equals `target_workspace_id`) |
  | `executor_profile_id` | the profile exists |

  **`agent_profile_id`.** `AgentProfile.WorkspaceID` has three meaningful states, and
  the spec must say which pass, because all three are storable: empty means
  global/kanban-legacy and SHALL be accepted; equal to `target_workspace_id` SHALL be
  accepted; **any other value SHALL be refused** — and that explicitly includes a
  profile scoped to the **source** workspace. Refusing the source-scoped case is the
  point: accepting it would let a caller run its own workspace's office agent inside a
  workspace it does not run in, which is the cross-boundary leak D3's gate (3) exists
  to prevent, reached through an argument the caller fully controls.

  **`executor_profile_id`.** `ExecutorProfile` has **no workspace field at all**, so
  there is no workspace test to apply and existence is the whole predicate. This is
  stated rather than left implicit so that no implementer invents a scoping rule for
  it, and so that nobody reads AC-14's older "usable in `target_workspace_id`" wording
  as demanding one — that wording was unsatisfiable for this id and has been removed.

  **Precedent.** Both predicates already ship, side by side, on
  `gitLabWatchDependencyValidator` (`internal/backendapp/turn_adapters.go`):
  `AgentProfileBelongs` returns
  `profile.WorkspaceID == "" || profile.WorkspaceID == workspaceID`, and
  `ExecutorProfileBelongs` takes the workspace id as `_ string` and returns existence
  only. This tool SHALL match that shape. It need not call those methods — they hang
  off a GitLab-watch validator — but it SHALL NOT diverge from their semantics, because
  two different answers to "does this profile belong to this workspace?" in one codebase
  is the defect, whichever one is written second.

  Per AC-12b these are `Validation` refusals only when the read executes and the
  predicate fails; a read that fails to execute is `InternalError` and retryable.

- **AC-14a.** The supplied ids SHALL be the ones recorded and used. No inheritance
  or defaulting chain — the target workflow's launch profile, the destination
  step's pinned profile, the target workspace's defaults, or the source session's
  own profile — SHALL override either value, and none SHALL be consulted. A
  workflow or step whose own launch profile differs SHALL NOT cause a refusal and
  SHALL NOT change the outcome. Rationale: `resolveMCPInheritedExecutors`
  and the step launch profile exist to *fill* an omitted value, and here omission
  is already impossible (AC-13); a caller that named a profile explicitly and got
  a different one is the precise failure R3 exists to prevent, and silently
  running an unexpected agent in a workspace the caller cannot observe is worse
  than in the same-workspace case.

- **AC-15.** On success the delivery task SHALL carry the supplied agent profile
  and executor in its launch metadata, so it can be started later from the board
  even when `start_agent` was false.

- **AC-15a.** The tool SHALL NOT accept a `workflow_step_id` argument. The
  destination step SHALL be resolved server-side by the existing
  `resolveMCPDestinationStep` semantics for the requested `start_agent` value:
  the workflow's first auto-start step when `start_agent` is true, otherwise its
  start step. Rationale: the caller does not run in the target workspace and
  pinning a step there is a decision it cannot make competently.

- **AC-15b.** Step resolution SHALL distinguish configuration from failure, which
  `resolveMCPDestinationStep`'s bare empty-string return does not: it yields `""`
  for a nil controller, an empty workflow id, a failed step listing, a nil
  response, and a genuine no-match alike. The tool SHALL therefore read the target
  workflow's steps such that it can tell these apart, and:
  - a workflow whose steps list successfully but yield no resolvable step, and a
    workflow with zero steps, SHALL be refused with `ErrorCodeValidation`;
  - a failure to read the steps SHALL be refused with `ErrorCodeInternalError`
    and SHALL be safe to retry;
  - in both cases no write SHALL occur, and no card SHALL be created that sits on
    no step.

- **AC-15c.** Step selection SHALL be deterministic, with the tiebreak named.
  AC-15a's word "first" is not defined by the existing selectors: for
  `start_agent: true`, `SelectAutoStartStep` replaces its candidate only on
  `step.Position < best.Position`, so two auto-start steps sharing a `position`
  resolve by the order `ListStepsByWorkflow` happened to return; for
  `start_agent: false`, `SelectStartStep` returns the first slice element carrying
  `IsStartStep`, so two steps flagged `is_start_step` resolve the same accidental
  way. Neither ambiguity is prevented by a constraint, so both are storable. The
  tool SHALL therefore order the target workflow's steps by `position` ascending
  with `id` ascending (lexicographic over UTF-8 code points) as the tiebreak
  before selecting, and SHALL select:
  - when `start_agent` is true, the earliest step in that order carrying an
    `auto_start_agent` `on_enter` action, falling back to the start-step rule
    below when there is none;
  - otherwise the earliest step in that order carrying `IsStartStep`, falling back
    to the earliest step overall when no step carries it.

  Two calls with identical arguments against an unchanged workflow SHALL resolve
  the same step. `id` is the tiebreak because `position` is the only ordering
  column and is not unique, and a workflow step id is stable for the row's
  lifetime; slice order is not a column and SHALL NOT be relied on.

### R4 — provenance is written in both directions

- **AC-16.** The delivery task SHALL record, in its own `Task.Metadata` under the
  single key `handoff_source`, written as part of the create request that produces
  it (D4), an object with exactly these fields:

  | Field | Value |
  |---|---|
  | `source_task_id` | the principal's `CallerTaskID` |
  | `source_workspace_id` | the principal's `WorkspaceID` |
  | `source_session_id` | the principal's `CallerSessionID` |
  | `source_agent_profile_id` | the agent profile resolved in AC-9 |
  | `handed_off_at` | D7's timestamp, RFC 3339 UTC, millisecond precision |

  The record is **write-once**. It SHALL NOT be added, patched or re-stamped on
  any Found outcome (R6), consistent with the external-id contract's rule that a
  second create returns the existing task unchanged.

  Every field is required. When the principal carries an empty `CallerTaskID` or
  `CallerSessionID`, the call SHALL be refused with `ws.ErrorCodeForbidden` and no
  write SHALL occur, on the same fail-closed grounds as AC-9: a delivery task
  whose provenance names no source, or a source task that cannot be found to
  receive the reverse link, is worse than no delivery task.

- **AC-17.** The source task SHALL record, in its own `Task.Metadata` under the
  single key `handoffs`, an append-only JSON array whose entries are objects with
  exactly these fields:

  | Field | Value |
  |---|---|
  | `task_id` | the delivery task id |
  | `target_workspace_id` | the target workspace id |
  | `handed_off_at` | the same instant as this handoff's AC-16 record |

- **AC-18.** The provenance SHALL NOT be injected into the delivery task's
  `prompt` or description. The prompt is the delivery agent's first user message
  and provenance is not an instruction to it.

- **AC-19.** The system SHALL write one activity entry in the **source**
  workspace with the action verb `task.handed_off` targeting the source task, and
  one in the **target** workspace with the action verb `task.handoff_received`
  targeting the delivery task. Both verbs SHALL be distinct from the verb used by
  ordinary task creation, so a handoff is filterable in the activity log.

  Both entries SHALL be written on **every** call that reaches D3b step 4, including
  replays that resolved to a Found outcome — D3b's skip list names only steps 2 and 5,
  and AC-19a's `outcome` field is what distinguishes a replay's entries from the
  original's. The activity log records **calls**, not tasks: a replay that repaired a
  reverse link is an event a reader needs to see, and suppressing it would make the
  repair invisible. The entries are therefore **not** idempotent by design, and
  nothing here SHALL attempt to deduplicate them.

- **AC-19a.** Each entry's fields SHALL be: actor type `agent`; actor id the
  source agent profile id from AC-16; session id the principal's
  `CallerSessionID`; run id per the rule below; and `details` a JSON-encoded
  string with exactly the fields in the table below. The principal carries no run
  id, so it is looked up, and a lookup that finds nothing SHALL NOT fail the call
  (D6).

  **Run id.** It SHALL be the id of an office run whose `SessionID` equals the
  principal's `CallerSessionID`, and the **empty string** otherwise —
  `LogActivityWithRun` documents empty as the correct value rather than a defect.
  The tool SHALL NOT write a candidate run's id unverified: two agents can hold
  claimed runs against the same source task, so a task-keyed lookup can return a run
  belonging to a different agent, and a wrong run id is worse than none in the one
  record R4 exists to make trustworthy.

  **The comparison is not reachable from the calling package today, so this criterion
  names the seam.** No new *repository* method is required — `GetRunByID` already
  returns a `Run` carrying `SessionID`, as does `office/service.Service.GetRun` — but
  `internal/mcp/handlers` cannot reach either: it imports `office/dashboard`,
  `office/shared` and `office/models` and has **no** import of `office/service`, and
  the Office dependency it holds (`dashboardSvc *dashboard.DashboardService`, wired by
  the existing `SetDashboardService`) exposes no single-run-by-id accessor —
  `ListRuns`, `GetLiveRuns` and `GetRunsByCommentIDs` only. The `RunResolver` seam
  (`internal/office/dashboard/service.go`) returns a bare id and discards the `Run`.
  Leaving this unsaid would make the wiring an unreviewed implementation choice with at
  least three defensible answers, so:

  - The existing `RunResolver` interface SHALL gain **one** method that resolves and
    verifies in a single call — `ResolveRunForTaskAndSession(ctx, taskID, sessionID)
    string` — returning the claimed run's id when that run's `SessionID` equals
    `sessionID`, and the **empty string** otherwise (including no claimed run, and a
    read error, per D6).
  - It SHALL be implemented by the office service that already satisfies `RunResolver`
    and is wired through `SetRunResolver` at startup, reusing its existing run read
    rather than adding a repository method.
  - `DashboardService` SHALL expose it as an **exported pass-through**, so the tool
    reaches it through the `dashboardSvc` the handler package already holds. No new
    dependency, field, or import SHALL be added to `Handlers`.

  Rationale for putting the comparison behind the seam rather than in the tool: `Run`
  lives in the office packages, and `handlers` deliberately sees a bare id today. A
  seam that returns an already-verified id preserves that boundary, whereas handing
  `handlers` a whole `Run` widens it for one field. Any equivalent wiring that keeps
  `Handlers` free of a new Office dependency and returns an id already verified against
  `sessionID` satisfies this criterion; filtering `ListRuns` client-side does **not**,
  because it reads every run in a workspace to answer a single-row question.

  **Not a defect:** when the source task's newest claimed run belongs to another
  session, the result is the empty string even though a matching run may exist
  elsewhere. That is the intended, explicitly-blessed outcome — empty is a correct
  value here and D6 makes activity non-load-bearing — not a case for widening the
  lookup.

  **`details` fields**, identical on both sides, with `counterpart` meaning the
  other side of the handoff:

  | Field | Value |
  |---|---|
  | `counterpart_task_id` | on the source entry, the delivery task id; on the target entry, the source task id |
  | `counterpart_workspace_id` | on the source entry, `target_workspace_id`; on the target entry, the source workspace id |
  | `outcome` | the R6 outcome, one of the four AC-24 values |

  The keys are named here for the same reason D8 names the metadata fields: an
  audit record whose field names are chosen per implementation is not queryable by
  the reader AC-19b anticipates. No other fields SHALL be written, so the payload
  cannot become a place to smuggle unreviewed data into the activity log.

- **AC-19b.** The **target-side** entry is written for durable audit only and has
  no reader today: the activity log is entirely Office-scoped, while D1 makes the
  delivery task a kanban task in a delivery workspace, which need not have an
  Office surface at all. This is stated rather than left to be discovered. The
  entry SHALL still be written, because the alternative is that the
  highest-consequence action in the flow leaves no record on the side that
  receives it; AC-21's metadata is the target side's queryable contract, and
  rendering either one is out of scope.

- **AC-20.** Activity-entry failure SHALL NOT fail the call or roll back either
  provenance write (D6).

- **AC-21.** Both provenance records SHALL be readable through the task read path
  that already returns task metadata, for each task, without a new endpoint.
  Neither key SHALL be redacted from that projection.

### R5 — same-workspace handoff is refused

- **AC-22.** When `target_workspace_id` equals the caller's own workspace as
  derived from the trusted principal, the call SHALL be refused with
  `ErrorCodeValidation` and a message naming `POST /runtime/tasks` as the path
  for same-workspace creation. The comparison SHALL use the principal's
  workspace, never a payload-supplied source workspace, and SHALL run at D3a
  step 2 — before the permission check, so a caller that targets itself is told
  what it actually did wrong.

- **AC-23.** `POST /runtime/tasks` and `Actions.CreateTask` SHALL be unchanged: no
  new field, capability or workspace input on the Office runtime create path.

- **AC-23a.** The delivery task SHALL be observably a kanban task, per D1 and D2a.
  Specifically it SHALL be created with **no** `Origin` supplied by the tool — so
  the stored origin is `manual` — with no `project_id`, and in the caller-supplied
  target workflow. Consequently `isOfficeRequest` SHALL be false for the create
  request, the task SHALL NOT be assigned an office identifier (its `identifier`
  SHALL be empty, and the target workspace's task sequence SHALL NOT be
  incremented by this call), and the stored task SHALL read back with
  `IsFromOffice` false. The tool SHALL NOT pass `TaskOriginAgentCreated`, and
  SHALL NOT introduce a new origin value. Testable directly: create a delivery
  task and assert the origin, the empty identifier, the unchanged workspace task
  sequence, and `IsFromOffice`. Asserting `IsFromOffice` alone is insufficient
  (D2a) — it is a read-time projection that `Origin` does not affect, so it would
  pass even against the office-triggering origin this criterion forbids.

### R6 — idempotency reuses the shipped contract; retry and partial failure are reported

The external-id mechanism is already specified in
`docs/specs/tasks/requirements/external-id-idempotency.md`, its boundaries
document, and `docs/specs/tasks/system-design/external-id-idempotency.md`. This
spec **reuses** it and does not restate, reinterpret, or extend it.

- **AC-24.** When `external_id` is supplied, the tool SHALL obtain its outcome
  from that mechanism unchanged and SHALL surface **which of the four outcomes**
  occurred — `created`, `found_settled`, `found_unsettled`,
  `created_identity_lost` — together with `creation_complete`, whose meaning is
  the one that contract already defines and nothing more. A single boolean SHALL
  NOT be used: `found_unsettled` is diagnostic and cannot be collapsed into
  "already existed" without discarding the one signal that contract calls
  safety-critical.

- **AC-24a.** On either **Found** outcome the tool SHALL perform no target-side
  work whatsoever — no second task, no session, no launch, no repository
  attachment, no workspace-policy write, no `task.created` event — matching the
  no-side-effect rule the external-id contract states and the data-loss guard
  `create_task_kandev` already implements for found outcomes.

  That enumeration is **exhaustive, not illustrative**, and two things it does not
  name are consequently still performed on a Found outcome, exactly as D3b's skip
  list says (it skips only steps 2 and 5): AC-25's source-side reverse-link repair,
  and AC-19's **two** activity entries — including the one written in the target
  workspace. This is stated because "no target-side work whatsoever" otherwise reads
  as forbidding the target-side audit row. It does not: the contract's no-side-effect
  rule governs the returned task and its workspace *state*, and an audit row recording
  that a call happened changes no state. Suppressing it would leave a replay with no
  trace on the side that received the work, which is precisely what R4 exists to
  prevent.

- **AC-24b.** On `found_unsettled` the tool SHALL return the task with
  `creation_complete: false` and SHALL NOT release the identity, SHALL NOT create
  a second task, and SHALL NOT wait, poll, or retry internally for settlement.
  Releasing and re-creating is the documented unsafe move, and this tool SHALL
  NOT expose or perform it. The response message SHALL say that the delivery task
  exists, that another create may still be finishing it, and that the safe
  responses are to proceed with the returned id or escalate to a human.

- **AC-24c.** On `created_identity_lost` the tool SHALL report that outcome
  explicitly and SHALL state in the response message that the delivery task
  exists but no longer holds the `external_id`, so an identical replay would
  create a **second** task. The caller is to record the returned id rather than
  replay.

- **AC-24d.** On the `created` path the tool SHALL **settle the external id
  itself**, because the create call cannot do it. `Service.CreateTask` returns
  only three outcomes and its own contract states that the fourth,
  `CreatedIdentityLost`, "is not produced here — it is decided by the handler
  during settlement, after this call returns". The tool SHALL therefore call
  `SettleExternalID` for the task it created, at D3b step 2 — after the create and
  before any launch dispatch — and SHALL map its result as follows:
  - settled: `outcome` is `created` and `creation_complete` is true;
  - **not** settled: `outcome` is `created_identity_lost`, `creation_complete` is
    true, the surviving task is the one returned, and no launch SHALL be
    dispatched (AC-32);
  - a settlement error: `ErrorCodeInternalError`. The delivery task SHALL NOT be
    deleted, and the message SHALL carry the task id so the caller does not lose
    it.

  The call SHALL be made unconditionally on the created path, including when
  `external_id` was omitted: `SettleExternalID` short-circuits to settled for an
  empty id, which is exactly AC-29a's `created` / `creation_complete: true`, so no
  separate branch is needed and none SHALL be added. The returned task SHALL be
  the one the create produced when settled, and the **survivor** the settlement
  call returns when not settled — those are different rows and the survivor is
  populated only in the second case.

  This criterion exists because omitting it is silent and load-bearing: an
  unsettled task still holds its `external_id` unsettled, so `found_settled`
  becomes unreachable and **every** later replay reports `found_unsettled`
  forever, which is the one outcome AC-24b tells the caller to escalate.
  Scenario 4 depends on this step having run.

- **AC-25.** On either Found outcome the tool SHALL still ensure the source
  task's reverse-link entry for that delivery task exists, adding it when absent
  and leaving it untouched when present. This is a **source-side** write, in the
  source workspace, to a task the external-id mechanism knows nothing about; it
  is outside that contract's enumerated no-side-effect set, all of which concerns
  the returned task and its workspace. This makes an identical replay a repair
  for a call that created the task and then failed before AC-17.

  **The repaired entry's `handed_off_at` SHALL be read from the found task's
  stored `handoff_source.handed_off_at`** (AC-16), not taken from this call's
  clock. AC-17 defines that field as "the same instant as this handoff's AC-16
  record", and on a replay the AC-16 record is the one already stored on the found
  task — write-once by AC-16 and never re-stamped by D7. The tool therefore reads
  that value during the AC-25a check it already performs, and writes it verbatim.
  Substituting a fresh timestamp SHALL NOT happen: it would contradict D7, and
  because AC-28 sorts by this field it would also place a repair at the end of a
  list whose order is supposed to record when handoffs happened.

  A repair entry consequently carries an **older** timestamp than entries appended
  after it, so AC-28's re-sort on every append is what keeps the stored array
  ordered; a repair SHALL NOT be assumed to belong at the end of the list.

- **AC-25a.** Before performing AC-25's repair the tool SHALL compare the found
  task's `handoff_source.source_task_id` (AC-16) with the calling source task id.
  When they differ, or when the found task carries **no** `handoff_source` record
  at all, the tool SHALL refuse with `ErrorCodeValidation`, SHALL write no
  reverse-link entry, and SHALL state that the `external_id` is already held by a
  task this source did not hand off. Rationale: external-id uniqueness is
  `(workspace_id, external_id)` and cross-workspace or global uniqueness is
  explicitly out of scope in that contract, so two different source tasks can
  choose the same key. Without this check the second source would acquire a
  reverse link to a delivery task whose write-once forward record names someone
  else — provenance that reads as true in one direction and false in the other.

- **AC-25b.** AC-25a's check passing does not make the rest of the stored record
  usable, so the tool SHALL additionally verify that the found task's
  `handoff_source.handed_off_at` is **readable** before writing AC-25's repair.
  It is unreadable when the field is missing, is not a string, or does not parse
  as an RFC 3339 timestamp. That list is **exhaustive**, and matches the malformed
  test AC-27 applies to a reverse-link entry, because a value that fails here would
  produce exactly such an entry if written.

  On an unreadable value the tool SHALL surface the **AC-29 partial failure** — the
  full AC-33 object, `reverse_link_recorded: false`, and a `reverse_link_error`
  naming the unreadable stored timestamp — and SHALL make **no write** to
  `handoffs`. It SHALL NOT substitute a fresh timestamp, SHALL NOT write an entry
  omitting the field, SHALL NOT repair the found task's `handoff_source`, and SHALL
  NOT retry. `handed_off_at` in the AC-33 response SHALL be the **empty string**,
  because the instant this handoff occurred is precisely what could not be read.
  An unreadable stored timestamp is the **only** thing that makes that field empty,
  in either of the two shapes below; on every other path it carries a value.

  **Except when the entry is already there.** AC-25 writes nothing when the
  reverse-link entry for that delivery task already exists, and in that case the
  reverse link **is** recorded, so reporting `reverse_link_recorded: false` would
  be false. When the entry is present and the stored timestamp is unreadable the
  tool SHALL report `reverse_link_recorded: true` with **no** `reverse_link_error`
  — nothing failed in this call — and `handed_off_at` still empty, because the
  instant remains unreadable. This case is reachable only when `handoff_source`
  degraded *after* an earlier call had already written the entry from it, which is
  exactly the residual under **Out of scope**; the link survives its own source
  record. The presence test therefore runs **before** this criterion's partial
  failure is raised, and the empty `handed_off_at` is the only observable symptom.

  **Why this is a partial failure and not AC-25a's refusal**, since the two checks
  sit one after the other and land in different places. AC-25a refuses because the
  `external_id` belongs to a task **this source did not hand off**: withholding
  that task's id is the point, as the caller has no claim to it. Here the found
  task **is** this source's own — `source_task_id` matched — so refusing would
  discard the caller's own delivery task id, which is the exact failure AC-29
  exists to prevent. Order is therefore total: `handoff_source` present and
  `source_task_id` matching (AC-25a, `Validation` refusal) is decided **first**;
  only on a task that passes it does this criterion run.

  Writing the entry anyway is the outcome this criterion exists to forbid: an entry
  whose `handed_off_at` is missing or unparseable is malformed by AC-27's own
  exhaustive definition, so the next handoff from this source task would read a
  corrupt list and be refused — leaving the source card unable to hand off again
  "until a human repairs it" (AC-27), caused by the tool itself. This case is
  reachable because `handoff_source` is not immune to the whole-blob metadata
  writers recorded under **Out of scope**, which is the same exposure AC-27 already
  accepts for `handoffs`.

- **AC-26.** The reverse-link list SHALL be idempotent by delivery task id: an
  entry for a given task id SHALL appear at most once regardless of how many
  times the call is replayed. Like AC-28's sort, this comparison SHALL only ever run
  on a list that has passed AC-27's corruption check, so every entry it inspects has a
  non-empty string `task_id`; an entry without one is refused there rather than being
  treated as a distinct id here.

- **AC-26a.** Two concurrent calls carrying the same `external_id` and the same
  `target_workspace_id` SHALL produce exactly one delivery task, and both callers
  SHALL receive that one task id. The winner reports `created`; the loser reports
  whichever Found outcome it observed, and SHALL still satisfy AC-25. Neither
  SHALL receive an error on account of losing the race.

- **AC-27.** The reverse-link append SHALL be performed as a **key-scoped
  compare-and-set on the `handoffs` key**: read the source task's current
  `handoffs` value, compute the appended list, and write **only that key**
  conditionally on the previously-read value still being current, retrying from
  the re-read on conflict.
  - The in-repo precedent is
    `internal/task/repository/sqlite/metadata_launch_error_cas.go`
    (`SetTaskMetadataKeyIfStamp` / `setMetadataKeyIfStamp`): a row-locked
    compare-and-set that patches one JSON key through `json_set` / `jsonb_set`,
    leaving sibling keys untouched. A new repository method SHALL follow **that**
    shape. Revision 2 named `UpdateTaskIfWorkflowMatches` instead; that was wrong
    and is corrected here. It guards a scalar column but then writes the whole
    marshalled metadata blob through `updateTaskTx`, exactly as plain `UpdateTask`
    does, so modelling on it would reproduce the read-modify-write this criterion
    exists to forbid. `repoerrors.ErrWorkflowResolutionConflict` remains the model
    for the **typed conflict error**, which is the part of that precedent that
    does transfer.
  - The append SHALL NOT go through `Service.UpdateTaskMetadata`. That path is
    `GetTask` → shallow key-merge → `UpdateTask` with no lock, no compare-and-set,
    and no version column, which is precisely the last-write-wins read-modify-write
    this criterion forbids.
  - **Scope of the guarantee.** The write SHALL touch no metadata key other than
    `handoffs`, and a concurrent write to any other key SHALL NOT cause a spurious
    conflict. Durability is guaranteed **against other `handoffs` writers**: two
    concurrent handoffs from the same source task SHALL both appear in the list,
    and neither SHALL be lost. It is **not** guaranteed against the whole-blob
    writers that still exist (`Service.UpdateTaskMetadata`, and `Service.UpdateTask`
    via `protectedTaskMetadataUpdate`): one of those that reads the row before this
    CAS commits and writes after it can still revert the append, and no primitive
    available to this tool can prevent that. Revision 2 promised the stronger
    guarantee; it was undeliverable and is narrowed here rather than left as prose
    nobody could satisfy. The residual is recorded under **Out of scope** with its
    mitigation, and AC-25's replay-as-repair is that mitigation.
  - An **absent** `handoffs` key SHALL compare equal to the empty array, so the
    first handoff from a source task is an ordinary append and two concurrent
    first handoffs still cannot lose one.
  - **Corrupt `handoffs` data SHALL NOT be overwritten, at either level.** Two
    shapes count as corrupt, and they are treated identically:
    - the `handoffs` value is present but **not an array**; or
    - the value is an array containing at least one **malformed entry** — an element
      that is not a JSON object, or an object whose `task_id` is missing or not a
      non-empty string, or whose `handed_off_at` is missing or does not parse as an
      RFC 3339 timestamp.

    That list is **exhaustive**. No other property of an entry makes it malformed: in
    particular an entry carrying **additional unknown fields** is well-formed, SHALL
    NOT be refused, and SHALL be preserved unchanged through the append. AC-17 defines
    what this tool *writes*; refusing to read anything wider would let one future
    additive change brick every source task that had already been handed off. An empty
    array is likewise well-formed — it has no entries to be malformed — and behaves
    exactly as an absent key.

    The second shape needs its own rule because AC-26's at-most-once check and
    AC-28's sort both read `task_id` and `handed_off_at` off **every** pre-existing
    entry, so every append must decide what to do with one that has neither. Without
    this clause the choice is the implementer's, and the three available answers —
    drop the entry, error, or sort it arbitrarily — differ in whether provenance is
    silently lost.

    On either shape the tool SHALL make **no write** to `handoffs` and SHALL surface
    the **AC-29 partial failure**: the full AC-33 object, `reverse_link_recorded:
    false`, and a `reverse_link_error` naming the corruption. It SHALL NOT retry, and
    SHALL NOT drop, coerce, normalise or re-sort the malformed data. Discarding an
    unreadable entry to make the append succeed would silently destroy the record R4
    exists to make trustworthy; refusing leaves it intact for repair and still returns
    the delivery task id, which is the thing the caller must not lose.

    **This is a correction, not only an addition.** Revision 3 said the non-array case
    SHALL be "refused with `ErrorCodeInternalError`", which contradicted AC-29 and this
    criterion's own result contract. By D3b the `handoffs` write happens at step 3,
    which is reached only after the delivery task exists — on the created path and on
    both Found paths alike — so a hard error here would discard the task id in exactly
    the situation AC-29 was written to prevent. There is no reachable case in which
    this write is attempted and no delivery task exists, so `ErrorCodeInternalError` was
    unreachable-as-intended and wrong where it was reachable.

    A source task whose `handoffs` is corrupt therefore cannot hand off again until a
    human repairs it. That is the intended direction: the alternative is a tool that
    quietly rewrites provenance it could not read.
  - **Result contract**, so the four failure shapes are not conflated:
    - *stale value* — the stored `handoffs` no longer equals the value read. This
      is the retryable conflict: return the typed conflict error and retry from a
      fresh read.
    - *source task missing* — no row with that id, whether deleted concurrently or
      never present. This SHALL NOT be retried, SHALL NOT be reported as a
      conflict, and SHALL surface as the AC-29 partial failure with
      `reverse_link_recorded: false`, because the delivery task exists and no
      retry can bring the source back.
    - *any other write failure* — SHALL NOT be retried and SHALL surface directly
      as the AC-29 partial failure. Only the stale-value case is retryable: a
      failing write is not made more likely to succeed by repeating it, and
      AC-25's replay-as-repair is the caller-side remedy.
    - *corrupt data* — the stored `handoffs` is a non-array or contains a malformed
      entry, per the clause above. SHALL NOT be retried, SHALL NOT be overwritten,
      and SHALL surface as the AC-29 partial failure.
    Precedence, as a total order so that no call has to decide between two applicable
    shapes: **source-missing > corrupt data > stale value > any other write failure.**
    A deleted row has no value to be corrupt and none to compare, so source-missing
    outranks both; corruption is detected on the read that precedes the compare, so it
    outranks stale value; and the catch-all ranks last by construction.
  - Retries SHALL be bounded at 5 attempts and SHALL apply to the stale-value case
    only; exhausting them is the AC-29 partial failure, not a silent drop.

- **AC-28.** The reverse-link list SHALL be ordered by `handed_off_at` ascending,
  with `task_id` ascending (lexicographic over UTF-8 code points) as the tiebreak
  for equal timestamps. "Insertion order" is not an ordering and SHALL NOT be
  relied on — and it genuinely diverges here rather than merely being untrustworthy:
  AC-25's repair appends an entry carrying the **original** handoff's timestamp, so
  a repair written today can sort ahead of entries appended before it. The order
  SHALL be re-established on every append, so the stored
  array is sorted at rest and a reader needs no sort of its own. This sort SHALL only
  ever run on a list that has passed AC-27's corruption check, so it never encounters
  an entry missing the keys it sorts by; a list containing one is refused before any
  ordering is computed, and SHALL NOT be partially sorted, filtered, or written back.

- **AC-29.** When the delivery task exists but the AC-17 reverse-link write fails
  — whether after a `created` outcome or during AC-25's repair — the tool SHALL
  return a **non-error** result carrying the full AC-33 object with
  `reverse_link_recorded: false` and a `reverse_link_error` message. It SHALL NOT
  return an error result, and SHALL NOT delete the delivery task.
  - Rationale: the delivery task genuinely exists, and its id is the single most
    important thing the caller must not lose. An error result on this surface is
    `mcp.NewToolResultError(string)` — one string — so the id would have to be
    parsed out of prose, and AC-25's replay-as-repair depends on the caller
    holding it reliably. A structured field is machine-checkable in a way an error
    string is not, and `reverse_link_recorded: false` is a stronger claim than a
    generic failure because it names exactly what is broken.
  - "Success" here means the tool completed and the delivery task exists. It does
    **not** mean the handoff is fully recorded, and the object says so.
  - `reverse_link_error` SHALL instruct an identical replay when `external_id` was
    supplied, which AC-25 turns into a repair. When it was not, it SHALL say so
    and SHALL warn that a replay would create a second task, so the caller records
    the id instead.
  - This resolves what would otherwise be a contradiction with AC-26a: both
    concurrent callers receive an object, and neither is turned into an error by a
    reverse-link failure.

- **AC-29a.** When `external_id` is omitted there is no identity to resolve, so
  `outcome` SHALL be `created` and `creation_complete` SHALL be true on every
  successful call. The three Found and identity-lost outcomes are reachable only
  when `external_id` was supplied.

- **AC-30.** When `external_id` is omitted, the call is not idempotent. The tool
  description SHALL say so and SHALL tell the caller to derive a stable
  `external_id` from the deciding artefact when a retry is possible. The tool
  SHALL NOT invent one from `title`, which changes freely between attempts, and
  SHALL NOT generate one itself — the external-id contract forbids
  system-generated identities.

### R7 — start semantics are reported, not assumed

- **AC-31.** `start_agent` SHALL default to **false**, diverging from
  `create_task_kandev`'s default of true. The tool description SHALL state the
  divergence and its reason: a cross-workspace card starting immediately in a
  workspace the caller does not run in is the riskiest available default.

- **AC-32.** The response SHALL report `started`, which SHALL be `true` **if and
  only if** all three hold: `start_agent` was true; the outcome was `created`;
  and the launch call the tool made for the delivery task **returned to the tool
  without error**. That return is the linearization point, and it is the only one
  — `started` SHALL NOT be derived from the destination step's configuration, and
  SHALL NOT be inferred from anything the tool did not itself observe.
  - **The launch SHALL be dispatched synchronously and its error observed.** The
    tool SHALL call the session launcher directly, in-line at D3b step 5, bounded
    by `constants.AgentLaunchTimeout` — the same bound the existing auto-start
    path applies — and SHALL wait for its result before composing the response. It SHALL NOT reuse
    `launchAutoStartTask`: that helper has no return value, runs `LaunchSession`
    inside a goroutine and logs-and-discards the error, and returns silently when
    the launcher is nil or the profile is empty. Reusing it would make this
    criterion's condition trivially true, `start_error` unreachable, and a
    launcher-less deployment report `started: true` with nothing launched.
    Blocking is acceptable here precisely because `start_agent` defaults to false
    (AC-31), so only a caller that explicitly asked to start a cross-workspace
    card waits, and that caller is the one that wants the answer.
  - `started: true` does **not** assert that the executor process is up, that a
    session is ready, or that the first prompt was accepted. It asserts that the
    launch call returned without error.
  - When the launch is attempted and fails — including a nil launcher, and
    including the timeout elapsing — `started` SHALL be `false` and the response
    SHALL carry a `start_error` message naming which. The delivery task SHALL NOT
    be deleted and the call SHALL NOT become an error: the card exists and is
    startable from the board, which is the outcome that matters.
  - On either Found outcome, and on `created_identity_lost`, `started` SHALL be
    `false` and no launch SHALL be attempted — AC-24a forbids it for the former,
    and AC-24d for the latter. Neither case SHALL carry a `start_error`: nothing
    was attempted, so there is no failure to report.
  - **`reverse_link_recorded` is deliberately NOT a fourth condition, and the three
    above are exhaustive.** A `created` call with `start_agent: true` whose
    reverse-link write failed — AC-27's stale-value retries exhausted, source task
    deleted, or corrupt `handoffs`; AC-25b's case is Found-only and so never
    co-occurs with a launch — SHALL
    still dispatch the launch and SHALL report `started` on the same three
    conditions as any other created call. The response then carries
    `reverse_link_recorded: false` **and** `started: true` together, and that
    combination is correct rather than contradictory. Reasons, stated because the
    opposite reading is available to anyone who reads D3b's ordering as a
    dependency: the delivery task's own **forward** provenance is already durable
    (D4 writes `handoff_source` inside the create), so the launched agent's source
    is findable from the delivery side regardless; the reverse link is a
    source-side index whose repair path is an identical replay (AC-25); and
    withholding the launch would contradict AC-29's governing philosophy that "the
    card exists and is startable from the board, which is the outcome that
    matters". Gating here would buy nothing D4 does not already guarantee while
    turning a repairable index failure into a silent refusal to do the work the
    caller asked for. Anyone who wants the launch gated on the reverse link is
    proposing a **change to this criterion**, not an implementation detail of it.

- **AC-32a.** `start_agent: false` **is** honoured at create time, including when
  the step AC-15a resolves carries an `auto_start_agent` `on_enter` action. Per
  D9 this tool does not stamp `auto_start_on_create`, and without that positive
  opt-in `task.created` does not evaluate the destination step's `on_enter`
  actions. A delivery task may still be launched later by the delivery workflow —
  for example when someone moves the card onto an auto-start step — but that is
  the target workflow's behaviour, happens after this call has returned, and
  `started` SHALL NOT attempt to predict or report it.

- **AC-33.** The response SHALL be a single object carrying at least:

  | Field | Meaning |
  |---|---|
  | `task_id` | the delivery task id |
  | `workspace_id` | the target workspace id |
  | `workflow_id` | the target workflow id |
  | `workflow_step_id` | the step resolved by AC-15a/AC-15b |
  | `outcome` | one of `created`, `found_settled`, `found_unsettled`, `created_identity_lost` |
  | `creation_complete` | as defined by the external-id contract, and nothing more |
  | `started` | AC-32 |
  | `reverse_link_recorded` | false exactly in the AC-29 case |
  | `handed_off_at` | D7's timestamp |

  **On a Found outcome these fields describe the task that already exists, not the
  request that replayed.** The distinction is load-bearing and is not derivable
  from the table: external-id uniqueness is `(workspace_id, external_id)` and is
  **not** keyed on workflow, so a replay carrying a different but otherwise valid
  `workflow_id` passes D3a step 5 and still resolves to the same task, and the step
  D3a resolved for *this* call is not necessarily the step that task occupies. The
  sources SHALL therefore be:

  | Field | On `created` | On either Found outcome |
  |---|---|---|
  | `workflow_id` | the target workflow id | the found task's **stored** `workflow_id` |
  | `workflow_step_id` | the step resolved by AC-15a/AC-15b | the found task's **current** `workflow_step_id`, as read at that moment |
  | `handed_off_at` | D7's timestamp | the found task's stored `handoff_source.handed_off_at`, or empty per AC-25b |

  A divergence between what the caller asked for and what is reported SHALL NOT be
  a refusal. `outcome` already tells the caller it received a pre-existing task
  rather than the one it just described, and refusing would break AC-25's
  replay-as-repair — the remedy AC-29 and AC-30 both instruct the caller to use —
  in exactly the case where a delivery card had since been moved. `workflow_step_id`
  is a snapshot of a value the delivery workflow may change at any time, and
  reporting it asserts nothing about where the card will be next.

  `reverse_link_error` SHALL be present only when `reverse_link_recorded` is
  false. `start_error` SHALL be present only when `started` is false **and a
  launch was actually attempted**; `started: false` with no `start_error` is the
  correct shape whenever no launch was attempted, which per AC-32 is every Found
  outcome, `created_identity_lost`, and any call with `start_agent: false`.
  Neither field SHALL appear otherwise. The field `idempotent_hit` SHALL NOT exist:
  `outcome` replaces it, and reintroducing a boolean alongside it would give two
  ways to ask one question and lose the `found_unsettled` distinction.

## Failure modes

| Condition | Result | Code |
|---|---|---|
| Unknown argument, or missing/blank required argument, or blank optional string | refuse, name it | `Validation` |
| Title over 60 runes | refuse, naming limit and actual length | `Validation` |
| Target workspace equals source workspace | refuse, name `POST /runtime/tasks` | `Validation` |
| Capability absent, or revoked since session start | refuse, no write | `Forbidden` |
| No principal, or session/agent profile unloadable | refuse, no write | `Forbidden` |
| Target workspace not visible to the source task's owner | refuse, indistinguishable from missing | not-found |
| `workflow_id` missing or in another workspace | refuse, one message for both, no owner disclosed | `Validation` |
| Workflow has no resolvable step, or no steps | refuse | `Validation` |
| Step listing failed | refuse, retryable | `InternalError` |
| `agent_profile_id` or `executor_profile_id` unresolvable | refuse, name which | `Validation` |
| `repository_id` not in target workspace, or `base_branch` without it | refuse, name it | `Validation` |
| `external_id` held by a task this source did not hand off | refuse, no reverse link | `Validation` |
| `external_id` already used by this source in target workspace | return existing task, repair link, report the Found outcome | success |
| A target-resource lookup fails to execute (repository, workspace, workflow, profile, steps) | refuse, retryable | `InternalError` |
| Settlement fails after the task was created | refuse, message carries the task id; task not deleted | `InternalError` |
| Settlement reports the identity was lost | `outcome: created_identity_lost`, no launch | success |
| Source task gone when the reverse link is written | AC-33 object with `reverse_link_recorded: false`, no retry | success |
| Delivery task exists, reverse link failed | AC-33 object with `reverse_link_recorded: false` | success |
| Source task's `handoffs` is a non-array, or has a malformed entry | AC-33 object with `reverse_link_recorded: false`; nothing overwritten, no retry | success |
| Found task's stored `handoff_source.handed_off_at` unreadable, reverse-link entry **absent** | AC-33 object with `reverse_link_recorded: false` and **empty** `handed_off_at`; no write, no substituted clock, no retry | success |
| Same, but the reverse-link entry is already **present** | AC-33 object with `reverse_link_recorded: true`, no `reverse_link_error`, and **empty** `handed_off_at` | success |
| Reverse link failed on a `created` call that asked to start | launch still dispatched; `reverse_link_recorded: false` and `started` decided by AC-32's three conditions alone | success |
| Launch call failed, timed out, or no launcher | AC-33 object with `started: false` and `start_error` | success |
| Activity write failed | success; logged, not surfaced | — |

## Out of scope

Each exclusion is a contract, not a silence.

- **`priority`.** The originating card lists it as optional. Kandev **does** have
  task priority — a `priority` column, a `defaultPriority` of `"medium"`,
  validation against a four-value enum, a create-time write and an update-time
  patch all exist. Revision 1 of this spec claimed otherwise and was wrong; the
  exclusion stands on different grounds. It is excluded because the Office runtime
  create path rejects it outright via `unsupportedField()`, so the two creation
  paths would disagree; because prioritising a card is a decision belonging to the
  workflow that will deliver it, not to the workspace handing it over; and because
  no named consumer wants it. A follow-up that wants it should say who reads it.
- **Resolving the delivery binding.** Which workspace, workflow, repository and
  profiles a discovery project delivers into. Today that lives in the Office
  project record's prose. This spec takes those as arguments. A follow-up giving
  the project record typed delivery fields would supply them; it does not change
  this tool's contract.
- **`blocked_by`, `parent_id`, `workspace_mode`, `autopilot`, `repository_url`,
  `local_path`.** Cross-workspace dependencies are already rejected by the task
  service's edge validator, and a cross-workspace parent has no defined meaning.
  Repository-by-URL and by-local-path create repository rows as a side effect, a
  second boundary crossing inside one call. `repository_id` (which must already
  exist in the target workspace) is the safe subset.
- **Hardening the whole-blob metadata writers.** `Service.UpdateTaskMetadata` and
  `Service.UpdateTask` (via `protectedTaskMetadataUpdate`) both read the task's
  metadata and write the entire blob back with no version check. One of them that
  reads the source task before AC-27's compare-and-set commits, and writes after
  it, silently reverts the `handoffs` append. AC-27's guarantee is narrowed to
  exclude this, and closing it properly means converting every metadata writer to
  key-scoped writes — a change to a shared path used by unrelated features
  (the GitHub issue store among them), which is a second feature wearing this
  one's clothes. The mitigation that already exists is AC-25: an identical replay
  with the same `external_id` repairs a missing reverse link, and AC-30 tells the
  caller to supply one. A follow-up that wants the stronger guarantee should
  convert those writers, not widen this tool.
- **Any change to the external-id mechanism.** Its four outcomes, its
  no-side-effect rule, its refusal to detect liveness, and its
  `(workspace_id, external_id)` uniqueness are consumed as-is. AC-25a works
  *around* the absence of cross-workspace uniqueness; it does not add it.
- **A UI surface for the handoff**, including rendering the AC-19 activity verbs
  or the AC-21 metadata anywhere. AC-19b states plainly that the target-side
  activity entry has no reader today.
- **Handing off to a workspace owned by a different user.** AC-11 refuses it; a
  cross-owner handoff needs a consent model that does not exist.
- **Reverse direction.** The reverse link makes it *possible* to find the source
  card; nothing here writes back when the delivery task finishes.
- **A deadline on an open handoff** (`[[closure-governance]]`'s "clock"). The
  delivery workflow owns the clock once the card lands.
- **Predicting a later launch.** AC-32a is explicit that a delivery card may be
  started afterwards by the target workflow; observing or reporting that is not
  this tool's job.

## Scenarios

1. **The motivating case.** A CEO-role Office agent in workspace A reaches a GO
   decision and calls `handoff_task_kandev` with workspace B, B's delivery
   workflow, title, prompt, both profiles, and an `external_id` derived from the
   discovery epic. B's board shows a startable card; A's task records the id it
   created; both activity logs show a handoff, not a create. The response reports
   `outcome: created`, `started: false`, `reverse_link_recorded: true`.
2. **A worker tries the same call.** A `worker`-role agent never sees the tool;
   if it sends the raw action anyway, AC-9 refuses it with `Forbidden` and no
   write, and the message does not reveal whether workspace B exists.
3. **Executor omitted.** Refused naming `executor_profile_id` (AC-13), rather
   than creating a card that inherits from a workspace the caller does not run
   in.
4. **Retry after a dropped connection.** The identical call replays; AC-24
   returns the same task with `outcome: found_settled`, and AC-25 ensures the
   reverse link, so the discovery card shows exactly one entry. `found_settled`
   is reachable here **only because** the first call settled the identity per
   AC-24d; without that step this scenario would report `found_unsettled`
   forever. The repaired entry and the response both carry the **original**
   `handed_off_at`, read back off the delivery task (AC-25, AC-33), so the retry
   does not appear in the record as a second, later handoff.

12. **A replay names a different workflow.** The delivery card was moved to another
    workflow in the target workspace after it was created, and the caller replays
    its original arguments. `workflow_id` still validates against the target
    workspace, the `external_id` still resolves to the same task, and the response
    reports the workflow and step the card **actually occupies**, not the ones
    asked for (AC-33). The call is not refused: refusing would break the
    replay-as-repair the caller was told to use.

13. **The delivery card's forward record was clobbered.** An unrelated whole-blob
    metadata write (the residual recorded under **Out of scope**) left
    `handoff_source` present with a `source_task_id` that still matches but a
    `handed_off_at` that no longer parses. The replay is not refused — the task is
    this source's own — but no reverse-link entry is written, `handed_off_at` comes
    back empty, and `reverse_link_error` names the unreadable timestamp (AC-25b).
    Writing the entry anyway would have made the source task's `handoffs` corrupt
    by AC-27's definition and blocked every future handoff from that card.
5. **Retry while the first create is still running.** The replay observes
   `outcome: found_unsettled`, `creation_complete: false`. The tool returns the
   task id and says another create may still be finishing it. It does not release
   the identity and does not create a second task (AC-24b).
6. **Same workspace by mistake.** Refused, pointing at `POST /runtime/tasks`
   (AC-22) — one obvious way to do each thing — and refused *before* the
   permission check, so the message describes the real mistake.
7. **The landing step auto-starts.** `start_agent: false` was requested and the
   resolved start step carries an `auto_start_agent` `on_enter` action. No launch
   happens: this tool does not stamp `auto_start_on_create` (D9), so the response
   reports `started: false` truthfully (AC-32a). If someone later moves the card
   onto an auto-start step, the delivery workflow starts it; that is outside this
   response's horizon.
8. **Two decisions race on one key.** Two source tasks in different discovery
   projects both derive the same `external_id`. The first hands off; the second
   is refused by AC-25a naming the collision, and acquires no reverse link to a
   card it did not create.
9. **The reverse link fails.** The delivery task is created, the compare-and-set
   append exhausts its 5 attempts. The caller receives the AC-33 object with the
   task id, `reverse_link_recorded: false`, and instructions to replay
   identically — which AC-25 turns into a repair. If the call also asked to start,
   the launch still happens and `started` is decided by AC-32's three conditions
   alone: `reverse_link_recorded: false` alongside `started: true` is the correct
   shape, because D4 has already made the delivery task's forward provenance
   durable and only the source-side index is missing.
10. **The identity is lost mid-create.** Another actor releases the `external_id`
    while this create runs. Settlement reports not-settled, so the response is
    `outcome: created_identity_lost`, `creation_complete: true`, `started: false`
    with **no** `start_error` because no launch was attempted (AC-24d, AC-32), and
    the message tells the caller to record the returned id rather than replay
    (AC-24c).
11. **The delivery workspace's database blips.** The `agent_profile_id` lookup
    fails to execute rather than returning nothing. The caller gets
    `InternalError`, not `Validation` (AC-12b, D3c), so an automated caller
    retries instead of concluding it sent a bad profile id, and nothing is
    written.

## Verification notes

- The Office prompt/inventory equality test (`sysprompt_sync_test.go:243`) is the
  highest-risk regression: it is set equality, so any unconditional mention of the
  new tool in `office-context.md` breaks it. AC-3 keeps it passing.
- `TestOfficeContext_ContainsOnlyOfficeCapabilities` (`sysprompt_test.go:248`)
  asserts `create_task_kandev` never appears in the Office prompt.
  `handoff_task_kandev` does not contain that substring, so it is unaffected and
  must not be relaxed.
- AC-9 must be tested by sending the action directly, not by asserting the tool
  is unregistered. Checking registration does not test the boundary.
- AC-26a and AC-27 need real concurrent calls, not two sequential ones. AC-27's
  key-scoping clause needs a concurrent write to a *different* metadata key on the
  same source task, which is the case a handoff-versus-handoff test will miss.
- AC-12 needs both a non-existent workflow id and a real workflow in a third
  workspace, asserting the two responses are identical.
- AC-32a needs a target workflow whose start step carries an `auto_start_agent`
  `on_enter` action, asserting no session is created and `started` is false.
  Asserting only the response field would pass against a stamped
  `auto_start_on_create` too.
- AC-23a must assert the stored origin, the empty identifier and the unchanged
  target-workspace task sequence. Asserting `IsFromOffice` alone passes against
  the office-triggering origin the criterion forbids (D2a), so that assertion on
  its own is not a test of this criterion.
- AC-24d needs a test that creates with an `external_id` and then **replays**,
  asserting `found_settled`. A test that only checks the first call's response
  passes identically whether or not settlement ran, which is exactly how this
  defect stayed invisible.
- AC-27 needs **five** separate tests, because one covers only the easy case: two
  concurrent handoffs from the same source task (both appear); a concurrent write
  to a *different* metadata key (no spurious conflict, and the write touches only
  `handoffs`); a source task deleted between the read and the write (AC-29
  partial failure, not a retry loop); a `handoffs` value that is not an array; and a
  `handoffs` array containing a malformed entry. The last two must assert **both**
  halves — the AC-29 partial failure is returned **and** the stored metadata is
  byte-identical afterwards. A test that only checks the response would pass against
  an implementation that dropped the unreadable entry, which is the specific failure
  the clause forbids.
- AC-7a needs a test that adds the key and asserts
  `TestPermissionMetadataMatchesKnownPermissionKeys` still passes — that existing test
  *is* the check, so the work is to keep it green, and a change that updates only one
  of the two lists must be seen to fail it. AC-7b's `Label` and `Description` are
  asserted as literals; there is no locale assertion to write, because there is no
  locale key.
- AC-14b needs three agent-profile cases — global (empty `WorkspaceID`, accepted),
  target-scoped (accepted), and **source-scoped** (refused) — plus one executor-profile
  case proving a profile is accepted regardless of any workspace, since it has no
  workspace field. The source-scoped refusal is the one that matters: an implementation
  that validates existence only passes the other three.
- AC-19a's run id needs a source task carrying **two** claimed runs from different
  sessions, asserting the entry records the caller's run id when the caller's run is
  the newest and the **empty string** when it is not. Asserting only the happy path
  passes against an implementation that never compares `SessionID` at all.
- AC-32 must assert `start_error` is actually reachable — a nil launcher, or a
  launcher returning an error, with `started: false`. A test that only exercises
  the success path would pass against the fire-and-forget helper this criterion
  forbids.
- AC-32's exhaustion clause needs its own test: a `created` call with
  `start_agent: true` whose reverse-link write fails, asserting a session **was**
  launched and the response carries `reverse_link_recorded: false` together with
  `started: true`. This is the one assertion that fails against an implementation
  which read D3b's ordering as a dependency and added a fourth gate; every other
  AC-32 test passes against both.
- AC-25's timestamp rule needs a replay whose delivery task carries a **known,
  distinctly older** `handed_off_at`, asserting the repaired `handoffs` entry and
  the AC-33 response both carry that stored value and not the replay's clock. A
  test using a freshly-created task passes against an implementation that stamps
  `time.Now()`, because the two values are indistinguishable within the test's
  runtime — the stored value must be old enough that substituting the clock is
  visible.
- AC-25b needs three cases — `handed_off_at` absent, present but not a string, and
  present but unparseable — each asserting the AC-29 partial failure, an **empty**
  `handed_off_at` in the response, and that the source task's `handoffs` is
  byte-identical afterwards. It also needs two precedence cases: a found task whose
  `source_task_id` mismatches **and** whose timestamp is unreadable must produce
  AC-25a's `Validation` refusal, not this partial failure; and a found task whose
  timestamp is unreadable but whose reverse-link entry already exists must report
  `reverse_link_recorded: true` with no `reverse_link_error` and an empty
  `handed_off_at`. An implementation that checks the timestamp before the presence
  test passes the first four cases and fails that last one.
- AC-33's Found-path source table needs a replay against a delivery task that has
  been **moved** to a different workflow and step since creation, asserting the
  response reports the stored/current values rather than the request's. Passing the
  same arguments the create used cannot detect this — the two sources are equal
  there, which is exactly why the divergence went unstated.
- AC-28 needs an out-of-order case: append a handoff, then repair an older one, and
  assert the stored array is sorted by `handed_off_at` with the repair **not** last.
- AC-15c needs a workflow with two steps sharing a `position`, asserting the same
  step is chosen across repeated calls. Storing them in the other order and
  re-running is what catches a slice-order implementation.
- **This file exceeds the linter's 32,768-byte `legacy` limit and is registered in
  `docs/specs/spec-lint-exceptions.tsv`.** That ceiling must equal the file's size
  **exactly**: a file smaller than its ceiling is a `stale-size-exception`
  violation, and a raised ceiling is a `legacy-size-ratchet` violation. Any edit
  to this file therefore requires updating the TSV to the new size in the same
  commit, and the size may only go down.

### User-visible surfaces touched

- **Office activity log** — two new action verbs (AC-19). Rendered by the existing
  activity surface on the source side; AC-19b records that the target-side entry
  has no reader.
- **Agent permission settings** — one new permission key appears in the Office
  agent permission editor, which requires **both** `AllPermissionKeys()` and
  `allPermissionMeta()` to be updated at the same index (AC-7a). Its `Label` and
  `Description` are **English string literals in Go** (AC-7b), delivered by the
  backend settings endpoint. **No locale work is required**: no permission key
  exists in `apps/web/src/locales/**` today, so there is nothing to translate and no
  `i18n:check` obligation. Revision 3 said this needed "a translated label in all
  five locales"; that was false and is corrected here.
- **Target workspace board** — a new card appears. No new rendering.

No new page, route, or interaction flow. The Playwright surface is limited to the
permission toggle's presence and label; an end-to-end handoff needs two workspaces
and a real executor, which the mock harness does not provide.
