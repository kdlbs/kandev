# Workspace coordinator scope audit

This is the historical prototype audit. The [private publication receipt](publication.md)
accounts for its sanitized import and additional planning files; the original
local snapshot below is not published as Git ancestry. Current execution order is
in the [delivery plan](../../plans/orchestration-delivery/plan.md).

Audited 2026-09-17 against local commit `b1cd0d2ea03336ce882d782f0f1ea61975db520c`, directly above Kandev v0.94.0 (`bf819a0228e742d069c528293d848c985a4d1bd1`). This records implementation at that commit, not a release claim. New design documents written after it are explicitly pending.

The original coordinator capabilities are implemented in the local prototype. The initial contribution's release conditions are not all satisfied: the workspace-wide central task view is new work, the contribution has not been isolated from unfinished assistant work, and the actual live database has not been rehearsed. The exact v0.94.0 baseline is retained. No PR or live deployment has occurred.

## Scope sources and interpretation

Two baselines matter, so this audit checks both:

1. The original local [Chief of staff](../../specs/chief-of-staff/spec.md) and [workspace agent connections](../../specs/workspace-agents/spec.md) requirements. Their Office-specific implementation was subsequently superseded for new coordinators by [unified workspace orchestration](../../specs/unified-workspace-orchestration/spec.md) and [workspace orchestrators](../../specs/workspace-orchestrators/spec.md). Legacy Office behavior remains compatibility scope.
2. The public contribution proposal in [issue #3752](https://github.com/kdlbs/kandev/issues/3752), including its optional automation destination and acceptance criteria.

The [maintainer feedback](https://github.com/kdlbs/kandev/issues/3752#issuecomment-5713059573) adds a dedicated workspace Coordinator page with grouped task status alongside agent chat. The user accepted that direction. It does not imply that every control in the example screenshots, an autonomous permission resolver, or a second scheduling engine is now required.

Status meanings: **implemented** means source and corresponding test coverage are present; **partial** means some source exists but the full outcome is unfinished; **pending** means it is not delivered; **historical** describes the earlier Office implementation. Test results below distinguish previous validation from this audit's source inspection.

## Original scope coverage

All source references below are relative to the repository root (backend package paths additionally start with `apps/backend/`).

| ID | Original requirement | Finding and evidence | Remaining limit |
| --- | --- | --- | --- |
| I01 | Configurable coordinator, instructions and execution identity | **Implemented.** `internal/orchestration/handler.go`, `configuration.go`, global roles and the workspace editor; `TestOrchestratorsUseProfilesAndScopeConfiguration` and `TestGlobalRoleOwnsIdentityWhileAssignmentsKeepAccountsAndContext`. | Names/icons/instructions moved to global roles by the later approved design. Existing permission and execution controls remain authoritative; this is not a new permissions editor. |
| I02 | Attach to existing Kanban workspace and workflow | **Implemented.** Workspace registrations and `backendapp/adapters_workspace_tasks.go` use canonical tasks/workflows/profiles; no second delivery workflow is required. Existing-task adoption is tested by `TestChiefCanObserveExistingRunningTaskWithoutStoppingIt`. | The older Office setup/primary-chief UX is compatibility behavior, not the intended first-class Coordinator page. |
| I03 | Persistent conversation, desktop/mobile entry points and return links | **Implemented.** Conversation registry/runtime, `app/settings/orchestration/conversation-route.tsx`, sidebar/mobile navigation and task-to-coordinator links. `workspace-orchestrators.spec.ts` covers navigation and repeated turns. | Current conversation occupies the main pane. It does not yet have the proposed task overview beside it. |
| I04 | Reusable roles with edits applied to every assignment | **Implemented.** Role reads occur at each turn; assignment responses project current role identity. `TestEachTurnUsesCurrentGlobalRoleAndWorkspaceContext` and `TestGlobalRoleMigrationPreservesDistinctAssignments`. | Conversation history, account and local context remain assignment-owned. |
| I05 | Delegate/create/follow normal tasks using selected worker accounts | **Implemented.** Workspace task adapters support explicit profiles, adoption, bounded worker results and follow-ups; profile/account validation and foreign-session rejection have tests. | Delegation quality and “coordinator does not implement” guidance are not an enforced prohibition on every provider-native tool. Real subscription identity is not proved by mocks. |
| I06 | Completion/blocker/review feedback reaches the right coordinator | **Implemented for task lifecycle callbacks.** `runtime/events.go`, ownership metadata and durable callback keys; `TestTaskCallbacksPreserveEveryResultAndDeduplicateTransitionEvents`, `TestOrchestratedCompletionRespectsReviewAndReopen`, browser callback coverage. | Session-only questions/permissions without a task transition do not yet provide the planned assistant attention inbox or automatic resolution. |
| I07 | Bounded management context with history retained separately | **Implemented with explicit limits.** `runtime/service.go` adds four recent comment excerpts of up to 1,000 bytes, fetches the current source comment by ID, and uses scoped memory selection. Assistant handoffs have a 12-KiB packet budget and 1-KiB memory excerpts. | This does not impose a total model-context/token limit on editable role instructions or the full current user message. Full memory/handoff work order 03 remains partial. |
| I08 | Manual and scheduled wakeups without duplicate dispatch | **Implemented at the supported boundaries.** Native chat dispatch, task callback deduplication, optional core Automation destination and firing-key deduplication. `TestAutomationDispatchesOnceWithoutOwningConversation` and runtime automation tests. | Legacy chat, assistant intake, worker follow-ups and schedules are different paths. Do not generalize assistant operation receipts into an exactly-once guarantee for all external actions or every lost worker-message acknowledgement. |
| I09 | Restart/retry preserves work and exposes failures | **Implemented.** Interrupted coordinator runs become failed; explicit retry uses fresh credentials. `TestRestartDoesNotReplayAnInterruptedConversation` and `TestRetryQueuesOriginalIntentWithoutReusingCredentials`. | Unknown external effects remain unknown and are not automatically replayed. Full host-outage recovery was excluded from the initial scope. |
| I10 | Workspace, conversation, profile and session scope checks | **Implemented.** Scoped runtime credentials and task adapters; retained private conversation ownership plus core task/session guards. Runtime/ownership/credential regression tests cover wrong owners and finished runs. | Keep these guards when splitting the PR. Removing the assistant UI/backend extras must not reopen retained private history. This audit is not a new exhaustive security review. |
| I11 | Paused or disabled coordinators do not launch; configuration survives | **Implemented.** Independent `features.orchestration` flag, admission/launch guards and disabled-route coverage. `TestOrchestrationFlagGuardsRuntimeAndLegacyRoutes`; browser coverage spans Office/Orchestration flag combinations. | Defaults remain off. No claim of replaying every event missed while paused. |
| I12 | Office compatibility and migration preserve existing records | **Implemented and tested on fixtures.** Independent Orchestration store/runtime; one-time transfer markers, role migration, conversation/owner retention, Office route rejection for registered personas. SQLite/PostgreSQL conformance passed after rebase. | No migration rehearsal against a copy of the current live database has been run. Older preview migration evidence is historical, not a v0.94.0 live-data rehearsal. |
| I13 | Optional daily prompt delivered to the existing conversation | **Implemented.** Automation target, profile/context inheritance, scheduling/timezone/Run now, dispatch status, links and retention-safe `conversation_task_id`. Both browser specs passed together. | `dispatched` means accepted, not work completed. v0.94.0 YAML/ZIP export cannot represent this target and deliberately returns an error. No live recurring job was created. |
| I14 | Migration/regression checks against contribution base | **Partial release condition.** Exact v0.94.0 rebase, affected backend/frontend/browser checks, database conformance and normal commit hooks passed. | The public proposal originally says current main. The user's later baseline is v0.94.0; that is the local base, not an agreed upstream PR target branch. Full repository tests, contribution extraction and final integration validation remain. |

There is no omitted core coordinator capability identified by this crosswalk. There are unfinished acceptance/rollout conditions and narrower guarantees than a blanket “everything is complete” statement would imply.

## Entire implemented and partial scope

### Coordinator product and runtime

- Multiple assignments per workspace; reusable global role name/icon/instructions; profile, executor, workspace context and lifecycle controls per assignment.
- Global role and workspace configuration pages, shared Save/Reset behavior, settings discovery, workspace cards/tabs, desktop/mobile navigation, shared avatars and coordinator links on normal tasks.
- Persistent conversations, comments, streamed replies, execution feedback and explicit recovery using shared core chat/session rendering without delivery-task property controls.
- Independent Orchestration storage, instructions, memory, conversation mapping, HTTP/CLI runtime and feature gating. The full backend Orchestration dependency closure is independent of Office.
- One-time import/migrations preserving identifiers and history; compatibility behavior for existing Office personas/workspace chiefs and rejection of registered coordinators through Office endpoints.
- Server-resolved execution identity, per-turn/session credentials, finished-run revocation, task/profile validation, normal workflow/review gates, task ownership callbacks and bounded result reads.
- Core run integration, restart failure reporting and explicit retry. The v0.94.0 integration uses the existing core run owner and dispatcher rather than introducing a second claim loop.

### Optional automation extension

- Coordinator destination alongside ordinary task automation; existing trigger interpolation, timezone, scheduling and manual run controls.
- Stable firing identity, one admitted run, inherited role/profile/executor/context, rejection/clearing of task-only overrides, visible failed/uncertain delivery states.
- Audit history and links to persistent chat; deleting run history cannot delete the shared conversation.
- Explicit export rejection where the v0.94.0 automation package format lacks this target. Portable export support is not implemented.

### Personal-assistant extension beyond the public issue

These changes are in the same local commit. They are not a finished second product and should not silently enlarge the first coordinator PR.

| Work order | Actual state at audited commit | Implemented portion / missing outcome |
| --- | --- | --- |
| 01 Durable intake and ownership | **Done, backend** | Selected owner binding, durable owner retained across switching/removal, source-comment identity/revision, idempotent intake receipts/outbox and restart repair. Core task/session/configuration/runtime checks preserve privacy even when the feature is off. |
| 02 Objectives and proportional routing | **Done, backend** | Objectives, acceptance revisions, answer/inspect/execute/design metadata, task links, validated workflow entry, operation receipts and completion evidence checks. “Inspect” routing does not itself enforce read-only provider execution. |
| 03 Scoped memory and handoffs | **Partial** | Scoped/provenanced/confirmed memory, revision/expiry/forgetting, deterministic bounded packets, credential reference descriptors, native dispatch/queue context guards, CLI/API. Remaining descriptor validation-time metadata and dedicated pagination/scope coverage are recorded in the work order. Rebase lint passing does not finish those outcomes. |
| 04 Native capability inventory | **Pending** | Existing profile/task catalogs are not the planned unified integrations/plugin/MCP capability directory. |
| 05 Enforced read-only tool execution | **Pending** | No whole-provider/native-tool read-only guarantee. Instructions and metadata hints are insufficient. |
| 06 Attention reconciliation/wakeups | **Pending** | No complete session/question/permission/authentication attention ledger or missed-event repair. Existing task callbacks are narrower. |
| 07 Native input resolution | **Pending** | No assistant-wide central answer/permission resolution with request identity and native task UI convergence. |
| 08 First-class assistant interface | **Pending** | No finished app-level personal Assistant shell, objectives, attention and memory controls. Existing workspace coordinator chat is separate. |
| 09 Workflow improvement loop | **Pending** | No delivered friction threshold/candidate/granted maintenance workflow. |
| 10 Linked-workspace grants | **Pending** | No implemented general cross-workspace assistant observation/coordination/export grants. Workspace-scoped APIs do not imply these grants. |
| 11 Read-only experiments/rollout | **Pending** | No complete assistant E2E experiment suite or live-provider readiness result. |

See the [assistant plan](../../plans/personal-assistant/plan.md) and [task 03 results](../../plans/personal-assistant/task-03-memory-context.md). The latter is intentionally still `in_progress`. Real credential retrieval is not claimed; descriptors carry references and scope, and unavailable resolvers remain unavailable.

### Shared infrastructure and v0.94.0 integration

- Core runtime authentication, run models/storage/dispatch, task execution context, per-message queue references and launch/resume/steer/PTY guards.
- SSH runtime environment propagation and managed-runtime integration, with the newer release's lifecycle behavior retained.
- Conversation MCP mode, agentctl workspace/task/objective/context/memory surfaces and synchronized tool guidance.
- Durable conversation privacy enforced through native task/session access, not just the coordinator HTTP routes; shared redaction helpers.
- SQLite/PostgreSQL dialect/schema compatibility, required-store registration, conformance adapters, role/profile cleanup and upgrade/replay fixtures.
- Shared frontend chat transports, retry handling, identity, session activity, task links and translations; Office configuration/navigation compatibility.
- Feature flags, documentation/specification/ADR records, browser fixtures and test cleanup. The new task view will reuse task-status summaries already shipped in v0.94.0; those summaries are not a new feature of this prototype.

## Complete file accounting

The committed delta is **476 paths, 19,777 added lines and 1,743 removed lines**. The generated [CSV inventory](scope-file-inventory.csv) accounts for every path exactly once and records added/removed lines; [baseline metadata](scope-baseline.json) pins the comparison. Categories group files for review, not independently shippable commits. Shared files may contain multiple concerns.

| Area | Paths |
| --- | ---: |
| Orchestration runtime and assistant extensions | 83 |
| Earlier Office integration and compatibility | 73 |
| Composition, task adapters, ownership and context | 24 |
| Automation destination/export guard | 8 |
| Shared run ownership/dispatch | 14 |
| Core task execution/message queue context | 24 |
| Database portability/conformance | 7 |
| Other backend CLI/auth/lifecycle/flags/guidance | 50 |
| Browser fixtures/scenarios | 7 |
| Shared chat/identity/session rendering | 50 |
| Coordinator/Office/automation/navigation UI | 99 |
| Specs/plans/ADRs/public documentation | 37 |
| **Total** | **476** |

## Accepted addition: central workspace task view

**Pending implementation.** Current `OrchestratedTasks` returns at most 100 linked tasks with ID/title/state. Its configuration-page list and chat links are not a workspace-wide dashboard.

The new [requirements and design](../../specs/orchestration/README.md) define one workspace Coordinator destination: canonical task status groups, workflow/repository/search filters, optional selected-coordinator filtering, task/PR links, diff/activity summaries when available, and persistent side chat. Multiple coordinator assignments remain supported. Mobile uses task/chat tabs over the same state.

Reuse v0.94.0's authoritative task status summaries. Pending questions/permissions take precedence over running indicators. Quiet time alone is not proof of a stall; missing Git/provider data is unknown, not zero or “healthy.” A merged PR is not proof that a task satisfies its workflow or objective. First delivery navigates to native task controls to answer/approve; it does not implement the unfinished assistant resolver.

The [implementation plan](../../plans/workspace-coordinator-view/plan.md) contains dependency-ordered work orders and synthetic media requirements. Screenshots and videos must be captured from a disposable fixture after implementation. The already-published demo predates the rebase and does not show this new page.

## Plugin findings and contribution boundary

The [pinned plugin review](plugin-review.md) recommends reusing patterns for stable conversation identity, verified scope, explicit configuration/failure states, bounded reports and schedule occurrence identity. Its host conversation APIs are absent from both the exact v0.94.0 release and this prototype. No plugin code or prompt was copied, no plugin was installed, and its test suite was not run.

Suggested contribution sequence, subject to maintainer agreement:

1. Coordinator foundation plus the central task view, preserving existing Kanban execution and privacy. Extract only required runtime/migration/UI seams; include fresh generic desktop/mobile media and targeted validation.
2. Optional automation destination as a focused follow-up, including its explicit delivery semantics and export limitation or a separately reviewed export contract.
3. Personal-assistant objectives/context/authority/attention as separately scoped work. Retain any ownership guards necessary for data already created by the prototype in the first contribution or its migration path.

This sequence is a proposal, not a claim that the 476-file commit has already been split. Do not push the whole snapshot or backup branch: historical local notes and prompt customization remain in that history. This audit contains capability descriptions and synthetic examples only.

## Validation and unresolved delivery conditions

Existing validation for the pinned commit is recorded in [rebase.md](rebase.md): affected Go suites; 27 frontend files with 206 passing tests and four skipped; TypeScript; two coordinator browser specs together without retries; fresh builds; SQLite/PostgreSQL conformance with race detection; SQL guard; normal commit hooks. The original source manifest was preserved. These are prior implementation checks, not plugin tests or proof of the pending central view.

This audit adds a full diff inventory, requirement-to-code/test crosswalk, pinned plugin source inspection and a new design package. Documentation validation passed: 30 specification-linter tests, all-specification lint, relative-link checks, six requirement / 19 acceptance-criterion mappings, and exact reconciliation of all 476 inventory paths including two renames. No production code or permanent tests changed during this audit; implementation suites were not rerun for documentation-only changes. Remaining conditions are:

- Implement and validate the central task view with workspace-switch, stale-status, pagination, hidden-conversation, desktop/mobile and native-input navigation coverage.
- Agree the core/plugin contribution boundary and actual upstream PR base while preserving the requested v0.94.0 baseline.
- Extract and sanitize source/history; keep necessary ownership safeguards and migrations together.
- Refresh synthetic screenshots/video and the PR description around the final contribution.
- Before live cutover, rehearse migration of a private copy of live data and follow the existing [deployment/rollback runbook](../../plans/orchestration-delivery/dogfood-runbook.md).

No private conversations were exported for this audit; no issue reply, PR, deployment or recurring automation was created.

## Subsequent implementation checkpoint: central view

On 2026-09-17 the independent assistant rollout gate and all three central-view
work orders were completed in the private integration branch. The central view
uses native Kanban task queries/status summaries and persistent conversations,
with paged coverage, filters, owner/workspace isolation, separate drafts and
mobile touch tabs. It does not start work by observation. See the
[work-order evidence](../../plans/workspace-coordinator-view/plan.md) and
[synthetic media](media/coordinator-view/README.md). This supersedes earlier
statements in this historical audit that the central page was only planned;
it leaves the remaining assistant and live-delivery limits intact.


### Memory/context completion after the original audit

Assistant task 03 is now complete on the private workbench. It adds bounded
owner and full-context pagination, native scope validation, typed metadata-only
credential observations and clock-stable context identities. Race, SQLite and
PostgreSQL conformance/upgrade, CLI and contract tests passed; see the updated
[task 03 evidence](../../plans/personal-assistant/task-03-memory-context.md).
The original audit above remains a historical record of its stated commit.


### Capability-directory completion after the original audit

Assistant task 04 now supplies owner-scoped native capability discovery with
bounded generation-aware pagination, structural schema projections, stored
integration health and current session attachment evidence. Manifest API version
2 permits explicit conversation-tool opt-in; task-only plugins keep their scope.
Discovery enables no operation. See the [task 04 evidence](../../plans/personal-assistant/task-04-capability-inventory.md)
for tests and the remaining authority boundary in task 05.
