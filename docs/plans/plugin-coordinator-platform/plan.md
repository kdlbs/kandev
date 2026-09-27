---
created: 2026-09-25
status: complete
requirements:
  - REQ-PLUGINS-COORDINATION-001
  - REQ-PLUGINS-COORDINATION-002
  - REQ-PLUGINS-COORDINATION-003
  - REQ-PLUGINS-COORDINATION-004
  - REQ-PLUGINS-COORDINATION-005
  - REQ-PLUGINS-COORDINATION-006
  - REQ-PLUGINS-COORDINATION-007
  - REQ-PLUGINS-COORDINATION-008
  - REQ-PLUGINS-COORDINATION-009
  - REQ-PLUGINS-COORDINATION-010
  - REQ-PLUGINS-COORDINATION-011
  - REQ-PLUGINS-COORDINATION-012
  - REQ-TASKS-COORDINATION-001
  - REQ-TASKS-COORDINATION-002
  - REQ-AGENTS-MANAGED-TOOLS-001
  - REQ-AGENTS-MANAGED-TOOLS-002
  - REQ-OFFICE-PLUGIN-TARGETS-001
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
  - ../../specs/tasks/system-design/coordination-controls.md
  - ../../specs/agents/system-design/managed-tool-policy.md
  - ../../specs/office/system-design/plugin-conversation-targets.md
legacy_specs:
  - ../../specs/task-cost-ledger/spec.md
---

# Implementation plan: Extensible coordinator plugins

## Overview

Make Kandev a stable host for user-defined coordinators. Deliver generic contracts,
then a separately packaged reference coordinator and a second policy variant.
Kandev owns execution and task invariants; plugins own coordination policy and UI
composition. Users can change prompts, roles, tools, scheduling, approval rules,
and reports without maintaining a host fork.

This is the approved design package requested on 2026-09-25. Implementation is
underway in the approved order below. The
[ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md)
records the agreed boundary. Work-order status and results track implementation.

## Scope

### In scope

- Versioned, capability-approved Host queries and exact commands, using the existing
  approval substrate and shared domain services.
- Retained managed conversations, durable ordered input, restricted agent tools,
  guarded execution and native interaction consent.
- Canonical task/PR/usage observations, delegation, optional management claims,
  evidence-based completion gates, typed workspace configuration, and linked issue writes.
- Native automation destinations, reusable Host chat, native mobile parity, and
  localized capability/task controls.
- A reference chief-of-staff plugin and a distinct proposal-first observer consumer,
  in dedicated repositories, with packaged compatibility evidence.

### Out of scope

- Porting the fork's coordinator product runtime or database wholesale into core.
- A universal coordinator DSL, dedicated coordinator principal, separate plugin MCP
  server, or private REST/database escape hatch.
- Agent-granted permission bypass, provider credential/login-lock repair, automatic
  merge, release, deployment, history rewriting, or cross-workspace approval.
- Host-native writing to arbitrary PR discussions or every tracker provider. Initial
  linked-issue writeback covers existing Jira and Linear links; other integrations
  remain independent plugin work with explicit credentials and permissions.
- A general terminal-resource cleanup engine or automatic destructive housekeeping.
- Creating/publishing remote plugin repositories, marketplace releases, or installing
  the coordinator into a user's active production workspace during this package.

## Design package

| Owner   | Requirements                                                                                             | System design                                                             |
| ------- | -------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| plugins | [Plugin managed coordination](../../specs/plugins/requirements/managed-coordination.md)                  | [Design](../../specs/plugins/system-design/managed-coordination.md)       |
| tasks   | [Task coordination controls](../../specs/tasks/requirements/coordination-controls.md)                    | [Design](../../specs/tasks/system-design/coordination-controls.md)        |
| agents  | [Managed agent tool policy](../../specs/agents/requirements/managed-tool-policy.md)                      | [Design](../../specs/agents/system-design/managed-tool-policy.md)         |
| office  | [Plugin conversation automation targets](../../specs/office/requirements/plugin-conversation-targets.md) | [Design](../../specs/office/system-design/plugin-conversation-targets.md) |

The [2026-08-31 Host proposal](../../decisions/2026-08-31-generic-plugin-host-boundary.md)
is related architecture, not evidence that its APIs exist. This package adopts its
exact naming where relevant and adds separate durable enqueue and retention APIs.
The [capability-approval plan](../plugins-capability-approval/plan.md) is done and
remains done. Extend its public substrate; do not reopen or duplicate its work order.

## Source baseline and fork mapping

Static investigation compared local host `852a867fd5c` with
[Corey's fork at 8017d2a32b7e72de3e0b14fff48a345d87b9891d](https://github.com/Corey-Fogg/kandev/tree/8017d2a32b7e72de3e0b14fff48a345d87b9891d).
The fetched snapshot differs from v0.95.1 in 514 files, with 34,691 additions and
863 deletions. It is newer than the six-commit screenshot. These are inspection
facts, not a claim that the fork passed runtime tests in this workspace.

The fork's `internal/orchestration` models and workspace tools, orchestration specs,
automation destination, and agentctl process policy were inspected. The fork did
not add plugin API contracts. Use its product behavior as evidence, not as a patch
to cherry-pick. Its named roles and policy stay outside the host.

| Fork feature                                   | Host responsibility                                           | Plugin responsibility                                          | Orders         |
| ---------------------------------------------- | ------------------------------------------------------------- | -------------------------------------------------------------- | -------------- |
| Named chief roles and workspace assignments    | Opaque instance keys, approved profile/executor               | Role templates, names, icons, instructions, multiple instances | 03, 04, 15     |
| Persistent chat and busy input                 | Retained conversations, FIFO receipts, replay protection      | Route/layout and prompt construction                           | 03, 05, 14, 15 |
| Broker-only agent execution                    | Adapter enforcement, live provenance, capability checks       | Declared tools and their policy                                | 01, 04         |
| Workspace/task overview and outcomes           | Canonical status, relations, PR evidence, available usage     | Grouping, filters, reports                                     | 06, 14, 17     |
| Delegate, edit, assign, move, message, archive | Shared exact task commands and source dedup                   | Task selection, worker policy, follow-up                       | 07, 15         |
| Adopt existing work                            | Claim fencing and human takeover                              | Explicit adoption intent and watches                           | 09, 17         |
| Stop, recover, modes, permissions, questions   | Exact generation controls and native human consent            | Decide when to request or surface help                         | 08, 14, 17     |
| Callbacks, private memory, pause               | Events as hints, authoritative queries, runtime pause         | Durable memory, cursors, filters, outbox, loop limits          | 05, 06, 16     |
| Proposals and approval before task creation    | Idempotent task creation                                      | Revisioned proposal approval and outbox                        | 07, 16         |
| Acceptance criteria and verified completion    | Task-owned versioned evidence and transition gate             | Criteria policy and evidence collection                        | 10, 17         |
| Recurring prompts and routines                 | Generic destination and owned-schedule lifecycle APIs         | Routine purpose and schedule policy                            | 13, 16         |
| Workspace/workflow/repository administration   | Typed defaults, ordering, registration, and domain invariants | Optional administration tools                                  | 11, 17         |
| Jira/Linear linked issue updates               | Credentials, provider adapter, uncertain-write receipts       | Decide content and timing                                      | 12, 17         |
| Metrics and budget policy                      | Measured/estimated/unknown usage with units                   | Outcome reports and discretionary budget decisions             | 06, 16, 17     |
| Per-user coordination flavor                   | Stable public contracts and composable UI                     | Independent prompts, tools, policy, persistence                | 15, 18         |
| Delete and repair helpers                      | Human-confirmed deletion; guarded lifecycle recovery          | Request the supported operation                                | 07, 08         |

Provider login repair and broad autonomous permission approval in the fork do not
become plugin authority. Repository scripts and secret-binding configuration also
remain native human actions rather than programmatic administrative writes.
Native provider settings and human consent remain the
supported path. A provider that cannot enforce tool restrictions is unavailable
for managed coordination until its adapter proves the contract.

## Technical approach

### Contract and authorization foundation

Extend `apps/backend/proto/kandev/plugin/v1/plugin.proto`, `pkg/pluginsdk`, manifest
validation, and `apps/packages/plugin-sdk`. Use additive exact RPCs; do not mutate
v1 semantics. Reuse `internal/plugins/approval*.go` for installation identity,
approval revision, and the immutable human-reserved capability policy.

Add durable command intents/receipts and couple operation identity to domain
transactions. Transport derives installation and agent provenance. Authorize again
at effect admission. Keep unknown external outcomes explicit. Maintain one method
registry for required capability, result types, limits, and support discovery.

Every work order adding a public API updates its concrete contract documentation:
`docs/plans/plugins/PLUGIN-API.md`, `GRPC-CONTRACT.md`, the relevant public authoring
sections, manifest examples, and matching SDK tests. Work order 18 verifies the
assembled consumer recipe; documentation is not deferred until that final order.

### Managed execution and shared UI

Extend `internal/task/service/agent_conversations.go` through an opt-in managed
lifetime. Add SQL conversation/input records and reuse orchestrator runtime and
messagequeue admission. Keep v1 delete-on-disable behavior and new retained lifetime
explicitly separate. An upgrade preserves history, but authority is re-evaluated.

The shared runtime and agentctl adapters enforce tool policy through the existing
Kandev MCP transport. Add only a managed tool surface and validated allowlist.
Unsupported providers fail before launch. Plugins do not own process lifecycle or
hold credentials that grant arbitrary host access.

Export `host.ui.WorkspaceAgentChat` and typed frontend conversation methods in
`apps/web/lib/plugins/{host-api,types}.ts`, the registry, and the standalone SDK.
Reuse native consent, task status, and recovery components. Add a retained transcript
view that does not depend on installed plugin UI. Existing task-panel consumers stay
compatible. Task detail and plugin settings own their native controls.

### Shared task and integration invariants

Implement task commands in shared services. Host and MCP adapters do not own
workflow, relation, WIP, claims, source deduplication, or completion rules. Claims
are independent from assignment; all plugin management mutations check an existing
claim. Completion gates run at the final transition commit for every entry path.

Workspace configuration commands call existing validation. Jira/Linear writeback
uses the task's exact linked issue and native credentials. Persist external intent
before send and provider receipt after; unknown results require reconciliation.
Automation stores a destination reference and occurrence receipt, never ownership
of the shared conversation's cleanup.

### Plugin-owned policy and independent consumers

Use the official template at the inspected baseline
[`be2f0c51b6fca92cf752c12f4c071961276782be`](https://github.com/kdlbs/kandev-plugin-template/tree/be2f0c51b6fca92cf752c12f4c071961276782be).
During implementation, create local sibling repositories
`../kandev-plugin-coordinator` and `../kandev-plugin-observer`; these are working
checkout conventions, not a claim that remote repositories already exist. Never
overwrite an existing unrelated checkout. If execution uses other paths, update
the work orders and package-fixture configuration together before running them.

The reference owns roles, instance settings, SQLite memory/proposals/outbox, event
reconciliation, optional operational tools, and reports. The observer has a separate
manifest identity and policy: propose work, then wait for explicit human approval.
Both use public capabilities without host branches keyed to their names.

### Delivery stages

Execute work orders sequentially in this primary session unless the user later
authorizes delegation. Dependency metadata names hard prerequisites; numeric order
is the planned execution order, not permission to spawn workers.

1. **Host foundation (01-06):** approved exact calls, retained/restricted conversation,
   ordered dispatch, and canonical observations. Work order 05 produces the first
   usable managed turn through a fixture; work order 06 makes it workspace-aware.
2. **Safe operations (07-13):** delegation, runtime controls, task claims/gates,
   configuration, source writes, and native scheduling. Each capability remains
   unavailable until its own checks pass.
3. **Usable product (14-15):** shared chat and a packaged reference coordinator with
   named instances. Ordinary delegation/chat is independently usable here.
4. **Full policy and extensibility (16-18):** durable decisions, advanced operations,
   and the second packaged consumer. No universal framework is added speculatively.

Feature exposure follows declared supported methods and explicit workspace grants.
This package does not introduce a new release flag. If a later rollout requires a
flag, run the runtime-feature-flags workflow and record its lifecycle explicitly.

## ASCII UI preview

These are proposed views, not screenshots of shipped behavior. Structural choices
are requirements; spacing, names, example counts, and visual styling are illustrative.
Use existing components and tokens. Localize host copy through `t()` in all supported
languages. Desktop and phone have equivalent actions with different composition.

### UI-01: Capability settings

```text
Desktop: Settings > Plugins > selected plugin > workspace
+----------------------------------------------------------------+
| Coordinator plugin                 Workspace: Product            |
| Requested capabilities     Current grant      Audit history       |
| [x] Read tasks             [ ] Manage tasks                      |
| [ ] Run agents             [ ] Write linked issues               |
| Manifest changed: review added permissions                       |
| [Revoke access]                           [Save approval]         |
+----------------------------------------------------------------+
Phone: same entry, full-height settings
+----------------------------+
| < Plugin access   Product  |
| Read capabilities          |
| [x] Tasks                  |
| Write capabilities         |
| [ ] Manage tasks           |
| [ ] Run agents             |
| [ ] Linked issues          |
| [View audit history]       |
| [Save approval]            |
| [Revoke access]            |
+----------------------------+
```

Permission groups and explicit workspace are required. The phone body scrolls; actions stay reachable above the safe area. Saving, stale revision, missing approval, and revoked states appear inline. A plugin cannot render a grant as its own agent action.

### UI-02: Workspace conversation

```text
Desktop: plugin navigation > Coordinator
+---------------------------------------------------------------------+
| Product workspace   [Delivery lead v] [Settings] [Pause]              |
| Outcomes: 8 completed  1 blocked  Usage: USD 2.10 measured             |
+----------------------------------+----------------------------------+
| Chat                             | Tasks  [Search] [Filters]        |
| You: follow up the review        | Blocked                          |
| Agent: proposal ready            | [Task A] Waiting for answer      |
| [Proposal: Review fix] [Approve] | Running                          |
|                                  | [Task B] Implementing            |
| Input queued: 1  [Cancel]         | Completed                        |
| [Message composer]        [Send] | [Task C] Verified                |
+----------------------------------+----------------------------------+
Phone: plugin navigation > Coordinator
+------------------------------+
| < Product [Delivery lead v] : |
| [Chat] [Tasks] [Outcomes]     |
+------------------------------+
| Chat messages                |
| Proposal: Review fix         |
| [Inspect] [Approve]          |
| Input queued: 1 [Cancel]     |
+------------------------------+
| [Message]             [Send] |
+------------------------------+
Phone instance/filter selection: bottom drawer
+------------------------------+
| Choose instance       [Done] |
| (o) Delivery lead            |
| ( ) Reviewer                 |
| [Add instance]               |
+------------------------------+
```

Desktop split composition and phone tabs are required; widths and wording are illustrative. Header/composer are pinned, only the active tab body scrolls, and selectors use bottom drawers. Empty: create an instance. Loading: retain the shell. Disconnected: retain draft and retry identity. Paused: show retained input count and Resume. Revoked/unsupported: explain the blocked capability and link to host settings. Pending human interactions use native shared controls.

### UI-03: Task management and completion

```text
Desktop: task detail > management and completion
+----------------------------------------------------------------+
| Manager: Delivery lead                       [Transfer] [Release]|
| Completion: 1 of 2 verified                                      |
| [ok] Regression test passes     [Evidence]                        |
| [!] Review resolved             [Inspect blocker]                |
| Cannot complete: current review evidence is missing               |
| [Record human override...]                      [Complete: off]   |
+----------------------------------------------------------------+
Phone: task detail > completion
+------------------------------+
| Manager: Delivery lead [Manage] |
| Completion: 1 of 2    [Inspect] |
+------------------------------+
```

Show claims independently from the worker assignee. On phone, combine both summaries and the Manage/Inspect actions in one compact toolbar below the fixed top bar, with 44px touch targets so the chat stays above bottom navigation. Manage and Inspect open native bottom drawers; desktop retains the full task-detail rows and shared dialogs. An override requires a reason and targets one observed move. Stale evidence and an unavailable manager remain visible; no hidden automatic takeover.

### UI-04: Automation destination

```text
Desktop: automation editor > target
+------------------------------------------------------------+
| Target: [Managed conversation v]                           |
| Plugin: Coordinator   Instance: [Delivery lead v]           |
| Schedule: [Weekdays 09:00]                                  |
| Prompt:   [Summarize blocked work...]                       |
|                                               [Save]      |
| Last firing: Accepted. Waiting for conversation            |
+------------------------------------------------------------+
Phone: automation editor
+------------------------------+
| < Automation          [Save]|
| Target                      |
| [Managed conversation v]    |
| Instance                    |
| [Delivery lead v]           |
| Schedule                    |
| [Weekdays 09:00]            |
| Prompt                      |
| [Summarize blocked work...] |
| Accepted; not yet running   |
+------------------------------+
```

Use a full-height phone editor and bottom-drawer instance selector. Hide task-only repository/workflow fields for this destination. An unavailable target remains visible with Repair action. History separates delivery from agent outcome.

### UI-05: Instance settings and proposals

```text
Desktop: Coordinator > Settings / proposal detail
+---------------------------------------------------------------+
| Instance: Delivery lead   Role: [Chief of staff v]              |
| Profile: [Supported agent v]  Executor: [Local v]               |
| Instructions: [User-defined coordination rules...]             |
| Scope: [Selected tasks v]   Budget: [USD 10 estimated]          |
| [Pause instance]                                    [Save]    |
| Proposal: Review fix  Revision 3   [Inspect] [Approve] [Reject] |
+---------------------------------------------------------------+
Phone: instance settings, separate full-height view
+------------------------------+
| < Delivery lead       [Save]|
| [Role v]                    |
| [Supported profile v]       |
| [Executor v]                |
| Instructions                |
| [User rules...]             |
| [Task scope v]              |
| [Budget and limits]         |
| [Pause instance]            |
+------------------------------+
```

Phone proposal detail is a full-height view with fixed Approve/Reject actions. Role/profile/executor/scope selection uses drawers. Unsupported profiles show a reason. Duplicate approval reads one receipt; stale proposals require inspection of the new revision. Pause preserves memory and pending inputs.

| View  | Primary criteria                                                                            | Rendered evidence                                              |
| ----- | ------------------------------------------------------------------------------------------- | -------------------------------------------------------------- |
| UI-01 | AC-PLUGINS-COORDINATION-001.3                                                               | Order 02, desktop and mobile capability specs                  |
| UI-02 | AC-PLUGINS-COORDINATION-009.1, AC-PLUGINS-COORDINATION-009.2, AC-PLUGINS-COORDINATION-009.3 | Orders 14-15, managed conversation and reference package specs |
| UI-03 | AC-TASKS-COORDINATION-001.3, AC-TASKS-COORDINATION-002.3                                    | Orders 09-10, claim and completion specs on both projects      |
| UI-04 | AC-OFFICE-PLUGIN-TARGETS-001.3                                                              | Order 13, automation desktop and mobile specs                  |
| UI-05 | AC-PLUGINS-COORDINATION-010.1, AC-PLUGINS-COORDINATION-011.1, AC-PLUGINS-COORDINATION-011.2 | Orders 15-16, instance and proposal specs on both projects     |

Use 44px minimum phone targets, safe-area composer spacing, keyboard avoidance,
fixed drawer headings/actions, and one scroll container per active phone view.
Desktop keeps normal control density. Test long task names, long messages, keyboard
focus, drawer scroll, and no horizontal page overflow. A disabled capability must
explain its state and point to the applicable native settings or human action.

## Tests

Each order owns its implementation tests and TDD sequence. Add the named tests
before implementing behavior. New test paths in work orders are planned files,
not commands claimed to pass today. A zero-test regex match is not evidence:
verify each listed test exists with `go test -list` or the runner's listing before
recording results. Record the actual cases, commands, counts, and failure resolution
in the owning work order; do not mark a whole wave done based on another order.

Run commands from the host repository root unless a command includes `cd`. For a
fresh host worktree, run `(cd apps && pnpm install --frozen-lockfile)` once. External
plugins retain the template's `npm ci`, `make test`, `make vet`, and
`verify-package-host` targets; adapt those targets as production source is added.
The template packaging target accepts `KANDEV_SDK` pointing to this host's backend.

For changed protobuf, run `make -C apps/backend proto`, inspect generated outputs,
and run SDK/manifest compatibility tests. For changed public docs, run
`node --test scripts/validate-public-docs.test.mjs` and
`node scripts/validate-public-docs.mjs` in the same order. For host UI, run typecheck,
scoped lint, i18n checks, and the targeted desktop/mobile E2E specs. Use existing
repository runners and their memory limits; never overlap full E2E suites.

The table maps every new acceptance criterion to its primary work order and named
evidence. Cross-boundary integration can repeat a criterion without changing its
ownership. Existing capability-approval requirements remain covered by their active
specification and regression suite.

| Acceptance criteria                                                                                                                                                                                  | Owner                                   | Named evidence to add                                                                                                                                                                                                                                                                                                                        |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `AC-PLUGINS-COORDINATION-001.1`, `AC-PLUGINS-COORDINATION-001.2`                                                                                                                                     | [01](task-01-exact-host-foundation.md)  | `apps/backend/internal/plugins/host_exact_test.go`: TestExactHostAdmission; `apps/backend/internal/task/service/service_exact_operation_test.go`: TestExactTaskUpdateRecovery                                                                                                                                                                |
| `AC-PLUGINS-COORDINATION-001.3`                                                                                                                                                                      | [02](task-02-capability-settings.md)    | `apps/web/e2e/tests/plugins/managed-capabilities.spec.ts`: grant, revoke, upgrade review, stale revision; `apps/web/e2e/tests/plugins/mobile-managed-capabilities.spec.ts`: phone grant and revoke                                                                                                                                           |
| `AC-PLUGINS-COORDINATION-002.1`, `AC-PLUGINS-COORDINATION-002.2`, `AC-PLUGINS-COORDINATION-002.3`                                                                                                    | [03](task-03-managed-lifetime.md)       | `apps/backend/internal/plugins/host_managed_conversations_test.go`: TestManagedConversationLifetime; `apps/backend/internal/task/service/managed_conversations_test.go`: TestManagedConversationIdentity                                                                                                                                     |
| `AC-AGENTS-MANAGED-TOOLS-001.1`, `AC-AGENTS-MANAGED-TOOLS-001.2`, `AC-AGENTS-MANAGED-TOOLS-001.3`, `AC-AGENTS-MANAGED-TOOLS-002.1`, `AC-AGENTS-MANAGED-TOOLS-002.2`, `AC-AGENTS-MANAGED-TOOLS-002.3` | [04](task-04-restricted-tools.md)       | `apps/backend/internal/agent/runtime/managed_tool_policy_test.go`: TestManagedToolPolicyLifecycle; `apps/backend/internal/agentctl/server/process/managed_tool_policy_test.go`: TestManagedToolPolicyAdapter; `apps/backend/internal/mcp/handlers/plugin_tools_managed_test.go`: TestManagedToolProvenance                                   |
| `AC-PLUGINS-COORDINATION-003.1`, `AC-PLUGINS-COORDINATION-003.2`, `AC-PLUGINS-COORDINATION-003.3`                                                                                                    | [05](task-05-durable-input.md)          | `apps/backend/internal/plugins/host_managed_inputs_test.go`: TestManagedInputReceipts; `apps/backend/internal/orchestrator/messagequeue/managed_inputs_test.go`: TestManagedInputRecovery                                                                                                                                                    |
| `AC-PLUGINS-COORDINATION-004.1`, `AC-PLUGINS-COORDINATION-004.2`, `AC-PLUGINS-COORDINATION-004.3`                                                                                                    | [06](task-06-workspace-observations.md) | `apps/backend/internal/plugins/host_exact_queries_test.go`: TestExactWorkspaceSnapshot; `apps/backend/internal/plugins/host_exact_evidence_test.go`: TestExactEvidenceAndUsage                                                                                                                                                               |
| `AC-PLUGINS-COORDINATION-005.1`, `AC-PLUGINS-COORDINATION-005.2`, `AC-PLUGINS-COORDINATION-005.3`                                                                                                    | [07](task-07-task-commands.md)          | `apps/backend/internal/plugins/host_exact_tasks_test.go`: TestExactTaskCommands; `apps/backend/internal/task/service/service_source_dedup_test.go`: TestSourceIdentityDedup; `apps/web/e2e/tests/plugins/managed-task-commands.spec.ts`: delegation, retry and deletion consent                                                              |
| `AC-PLUGINS-COORDINATION-006.1`, `AC-PLUGINS-COORDINATION-006.2`, `AC-PLUGINS-COORDINATION-006.3`                                                                                                    | [08](task-08-execution-controls.md)     | `apps/backend/internal/plugins/host_exact_execution_test.go`: TestExactExecutionControls; `apps/backend/internal/plugins/host_exact_interactions_test.go`: TestExactHumanInteraction                                                                                                                                                         |
| `AC-TASKS-COORDINATION-001.1`, `AC-TASKS-COORDINATION-001.2`, `AC-TASKS-COORDINATION-001.3`                                                                                                          | [09](task-09-task-claims.md)            | `apps/backend/internal/task/service/management_claims_test.go`: TestTaskManagementClaimFencing; `apps/web/e2e/tests/plugins/task-management-claims.spec.ts`: claim conflict and human takeover; `apps/web/e2e/tests/plugins/mobile-task-management-claims.spec.ts`: phone transfer and release                                               |
| `AC-TASKS-COORDINATION-002.1`, `AC-TASKS-COORDINATION-002.2`, `AC-TASKS-COORDINATION-002.3`                                                                                                          | [10](task-10-completion-gates.md)       | `apps/backend/internal/task/service/completion_gates_test.go`: TestCompletionGateEntryPoints; `apps/web/e2e/tests/plugins/task-completion-evidence.spec.ts`: stale evidence, blocked move and override; `apps/web/e2e/tests/plugins/mobile-task-completion-evidence.spec.ts`: phone inspect and override                                     |
| `AC-PLUGINS-COORDINATION-007.1`, `AC-PLUGINS-COORDINATION-007.2`, `AC-PLUGINS-COORDINATION-007.3`                                                                                                    | [11](task-11-workspace-admin.md)        | `apps/backend/internal/plugins/host_exact_workspace_test.go`: TestExactWorkspaceAdministration                                                                                                                                                                                                                                               |
| `AC-PLUGINS-COORDINATION-008.1`, `AC-PLUGINS-COORDINATION-008.2`, `AC-PLUGINS-COORDINATION-008.3`                                                                                                    | [12](task-12-source-writeback.md)       | `apps/backend/internal/plugins/host_source_writeback_test.go`: TestSourceWritebackReceipts; `apps/backend/internal/jira/service_plugin_writeback_test.go`: TestPluginWriteback; `apps/backend/internal/linear/service_plugin_writeback_test.go`: TestPluginWriteback                                                                         |
| `AC-OFFICE-PLUGIN-TARGETS-001.1`, `AC-OFFICE-PLUGIN-TARGETS-001.2`, `AC-OFFICE-PLUGIN-TARGETS-001.3`, `AC-OFFICE-PLUGIN-TARGETS-001.4`                                                               | [13](task-13-automation-destination.md) | `apps/backend/internal/automation/managed_destination_test.go`: TestManagedConversationDestination; `apps/web/e2e/tests/plugins/managed-automation.spec.ts`: schedule delivery, cleanup and portable rebinding; `apps/web/e2e/tests/plugins/mobile-managed-automation.spec.ts`: phone destination editor                                     |
| `AC-PLUGINS-COORDINATION-009.1`, `AC-PLUGINS-COORDINATION-009.2`, `AC-PLUGINS-COORDINATION-009.3`                                                                                                    | [14](task-14-host-conversation-ui.md)   | `apps/web/lib/plugins/managed-conversation.test.ts`: receipt ordering, retry keys and revoked state; `apps/web/e2e/tests/plugins/managed-conversation.spec.ts`: conversation and lifecycle states; `apps/web/e2e/tests/plugins/mobile-managed-conversation.spec.ts`: phone tabs, drawer, keyboard and long content                           |
| `AC-PLUGINS-COORDINATION-010.1`, `AC-PLUGINS-COORDINATION-010.2`, `AC-PLUGINS-COORDINATION-010.3`                                                                                                    | [15](task-15-reference-plugin.md)       | `../kandev-plugin-coordinator/server/instances_test.go`: TestCoordinatorInstances; `apps/web/e2e/tests/plugins/reference-coordinator.spec.ts`: packaged instance creation and delegation; `apps/web/e2e/tests/plugins/mobile-reference-coordinator.spec.ts`: phone instance selection and settings                                           |
| `AC-PLUGINS-COORDINATION-011.1`, `AC-PLUGINS-COORDINATION-011.2`                                                                                                                                     | [16](task-16-durable-policy.md)         | `../kandev-plugin-coordinator/server/policy_test.go`: TestProposalOutboxRecovery and TestCoordinatorPolicyLimits; `apps/web/e2e/tests/plugins/reference-coordinator-policy.spec.ts`: proposal approval, pause and schedule; `apps/web/e2e/tests/plugins/mobile-reference-coordinator-policy.spec.ts`: phone proposal inspection and approval |
| `AC-PLUGINS-COORDINATION-011.3`                                                                                                                                                                      | [17](task-17-reference-operations.md)   | `../kandev-plugin-coordinator/server/operations_test.go`: TestCoordinatorOperationalTools; `apps/web/e2e/tests/plugins/reference-coordinator-operations.spec.ts`: adoption, evidence, issue writeback and outcomes; `apps/web/e2e/tests/plugins/mobile-reference-coordinator-operations.spec.ts`: phone blockers and outcomes                |
| `AC-PLUGINS-COORDINATION-012.1`, `AC-PLUGINS-COORDINATION-012.2`, `AC-PLUGINS-COORDINATION-012.3`                                                                                                    | [18](task-18-independent-consumer.md)   | `../kandev-plugin-observer/server/observer_test.go`: TestObserverPolicy; `apps/web/e2e/tests/plugins/coordinator-compatibility.spec.ts`: two independent packages and lifecycle contract; `apps/web/e2e/tests/plugins/mobile-coordinator-compatibility.spec.ts`: phone policy parity and unavailable capability                              |

Required failure evidence includes crash after domain commit, execution replacement,
revocation between queue and effect, source-link changes, stale PR head evidence,
callback loss, plugin disable/uninstall, duplicate proposal approval, and phone
interaction recovery. Use deterministic injected faults and fake providers for CI.
A supported real adapter smoke is required before advertising restricted execution;
record adapter/version and observed blocked tools without including credentials.

## E2E tests

All paths below are under `apps/web/e2e/tests/plugins/`. Work orders provide exact
commands. Use `chromium` and separate `mobile-chrome` runs. The existing
`conversation-recovery.spec.ts` and plugin fixture provide patterns for isolated
state, causal transport waits, and packaged installation. Do not use personal tasks,
provider accounts, or production data in these tests.

| Flow                                             | Desktop spec                             | Phone spec                                      | Criteria owner     |
| ------------------------------------------------ | ---------------------------------------- | ----------------------------------------------- | ------------------ |
| Workspace grants and revocation                  | managed-capabilities.spec.ts             | mobile-managed-capabilities.spec.ts             | 01.3               |
| Delegation and human-confirmed delete            | managed-task-commands.spec.ts            | mobile-managed-task-commands.spec.ts            | 05                 |
| Management ownership and takeover                | task-management-claims.spec.ts           | mobile-task-management-claims.spec.ts           | tasks 001          |
| Completion gate and override                     | task-completion-evidence.spec.ts         | mobile-task-completion-evidence.spec.ts         | tasks 002          |
| Automation enqueue and portable binding          | managed-automation.spec.ts               | mobile-managed-automation.spec.ts               | office 001         |
| Retained chat, queue, consent and recovery       | managed-conversation.spec.ts             | mobile-managed-conversation.spec.ts             | 002, 003, 006, 009 |
| Named instances and delegated work               | reference-coordinator.spec.ts            | mobile-reference-coordinator.spec.ts            | 010                |
| Proposal approval, pause and schedules           | reference-coordinator-policy.spec.ts     | mobile-reference-coordinator-policy.spec.ts     | 011.1-2            |
| Adoption, evidence, writeback and outcomes       | reference-coordinator-operations.spec.ts | mobile-reference-coordinator-operations.spec.ts | 007, 008, 011.3    |
| Independent consumer lifecycle and compatibility | coordinator-compatibility.spec.ts        | mobile-coordinator-compatibility.spec.ts        | 012                |

The reference-package smoke fixture introduced in order 15 loads the actual sibling
archive, derived from its manifest, and fails if it is absent. Order 18 adds a second
archive and an old v1 fixture. Host-owned fixture UI may exercise generic components;
it is not substituted for the packaged consumers in release evidence.

## Work orders

- [x] [Task 01: Exact Host authorization and durable command receipts](task-01-exact-host-foundation.md)
- [x] [Task 02: Workspace capability approval UI](task-02-capability-settings.md)
- [x] [Task 03: Retained managed conversation lifecycle](task-03-managed-lifetime.md)
- [x] [Task 04: Enforced managed agent tool policy](task-04-restricted-tools.md)
- [x] [Task 05: Durable ordered managed conversation input](task-05-durable-input.md)
- [x] [Task 06: Canonical workspace and evidence queries](task-06-workspace-observations.md)
- [x] [Task 07: Exact task delegation and follow-up commands](task-07-task-commands.md)
- [x] [Task 08: Guarded execution, recovery, and human interactions](task-08-execution-controls.md)
- [x] [Task 09: Task management claims and human takeover](task-09-task-claims.md)
- [x] [Task 10: Native completion criteria and evidence gates](task-10-completion-gates.md)
- [x] [Task 11: Typed workspace configuration commands](task-11-workspace-admin.md)
- [x] [Task 12: Linked Jira and Linear issue writeback](task-12-source-writeback.md)
- [x] [Task 13: Native automation delivery to managed conversations](task-13-automation-destination.md)
- [x] [Task 14: Reusable workspace conversation and task-status UI](task-14-host-conversation-ui.md)
- [x] [Task 15: Packaged reference coordinator and named instances](task-15-reference-plugin.md)
- [x] [Task 16: Reference proposals, reconciliation, and routine policy](task-16-durable-policy.md)
- [x] [Task 17: Reference adoption, evidence, writeback, and outcomes](task-17-reference-operations.md)
- [x] [Task 18: Second policy consumer and packaged compatibility contract](task-18-independent-consumer.md)

## Verification results

Documentation validation completed on 2026-09-25:

- `python3 scripts/list-docs.py validate`: passed, 306 decisions and 1,162 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/decisions docs/plans/plugin-coordinator-platform`: passed.
- Local link, whitespace, dependency, and traceability audit: passed. All 52 acceptance
  criteria have exactly one primary owner across 18 sequential work orders.

Four requirement/design pairs and one ADR were added, with scoped cross-links in
existing task/automation specs and the earlier Host proposal. Implementation
completed on 2026-09-26: all 18 work orders are complete. WO04 deliberately
advertises no supported managed provider until a real adapter completes its
launch/resume smoke matrix. WO05 added
durable FIFO input, replay-safe receipts, exact cancellation, periodic coalescing,
pause-safe delivery, and immediate dispatch with a live-state busy check. Its
managed-input race and service tests, lifecycle/pause and adapter regressions,
multi-target backend build, public-doc validation (62 tests / 47 pages), and
whitespace checks passed. WO06 canonical workspace and evidence queries and WO07
exact task commands are complete. WO07's race tests, adapter tests, 41 focused web
tests, desktop/mobile deletion-consent E2E, typecheck, scoped lint, documentation
validation, and whitespace checks passed. WO08 added generation-fenced execution
controls, safe recovery and session modes, and receipt-gated human interaction
responses. Required race and orchestrator regression tests, backend plugin/SDK/
backendapp coverage, proto generation, documentation checks, and whitespace
passed. WO09 added task-owned claim generations, human takeover controls,
legacy-write fencing, queue/deferred-move revalidation, and desktop/phone claim
UI. Its race-focused backend tests, phone claim flow, typecheck, i18n, public-doc
validation, specification validation, and whitespace checks passed. The broader
SQLite/service rerun initially hit disk exhaustion; the claim and affected
promotion regressions passed after a metadata-preservation fix.

WO10 (native completion criteria and evidence gates) is complete. Implemented
revisioned criteria, task-revision-bound evidence, exact Host set/verify calls,
audit history, final-commit completion enforcement, blocker projections, and
native reasoned one-move override on desktop and phone. Tasks without criteria
retain compatible completion behavior. Also fixed and regression-tested workflow
snapshot projection of the terminal completion flag. Race tests, Host/SDK tests,
generated protobufs, backend build, web API/snapshot tests and typecheck, scoped
ESLint, i18n, desktop and phone E2E, documentation/spec validation, and whitespace
checks passed. WO11 (workspace administration) is complete. Its Host race and SDK
tests, workspace adapter tests, repository-level stale-version tests, affected task
and workflow package suites, public documentation validation, formatting, and
whitespace checks passed. WO12 (linked Jira and Linear issue writeback) is
complete. Added exact Jira and Linear linked-issue reads, comments, and
transitions through workspace-owned credentials, with durable versioned receipts
and no automatic resend after an uncertain provider outcome. The required race
suite, full plugin package tests, backend source adapter tests, public-doc
validation (62 tests / 47 pages), and whitespace checks passed. Focused package
lint still reports existing findings in adjacent Exact Host and workspace-admin
code; none were reported in the source adapter or writeback receipt
implementation. WO13 (native automation delivery to managed conversations) is
complete. The destination APIs, durable delivery receipt projection, desktop/phone
editor, portable rebinding, and cleanup ownership passed the required backend race,
web test, desktop/mobile E2E, typecheck, i18n, public-doc (62 tests / 47 pages),
build, package, and whitespace checks. A blocked queue-wake regression established
that durable admission returns before agent startup; the run-list projection fix
made accepted delivery visible to history. The mock provider remains correctly
unavailable behind the restricted-tool support gate.

WO14 (reusable workspace conversation and task-status UI) is complete. Added the
typed SDK and Host contracts, canonical task-status/usage facades, reusable
desktop/phone conversation UI, exact recovery fences, and a retained host transcript
that survives plugin removal. The source-backed fixture route uses public Host APIs
and the production Host UI. Desktop and phone E2E passed, covering retry identity,
lifecycle states, instance selection, usage/status, retained history, tabs, keyboard
submission, long content, and overflow. The Host UI drawer now closes on instance
selection, and the fixture recognizes uppercase exact-command statuses. SDK tests and
typecheck, six focused web suites (40 tests), web typecheck, i18n, ESLint, fixture
package build, public-doc tests (62) and validation (47 pages), specification
validation (306 decisions / 1,162 specifications), and whitespace checks passed.
WO15 added the packaged reference coordinator with independent named instances,
delegation, and isolated memory. WO16 added durable proposal, reconciliation, watch,
schedule, pause, concurrency, and budget policy. WO17 added adoption, claims,
completion evidence, Jira writeback, and outcome reporting. WO18 added a distinct
proposal-first observer package and compatibility coverage with the coordinator and
v1 fixture. Both packages passed their full tests, vet, and package validation.
The final desktop coordinator aggregate passed all four packaged E2E specs, including
two-consumer lifecycle compatibility and Jira/outcome operations; the phone aggregate
passed all four corresponding specs. The final coordinator archive includes the
`usage` read grant required by outcome reporting. Public-doc validation, typecheck,
backend build and focused race tests, specification validation, and whitespace checks
passed. No remote plugin repository was created or published. Changes are carried
on the feature branch.

## Risks

- **Provider enforcement:** the available adapters may not all disable native and
  ambient tools. Advertise only proven support; retain an explicit unsupported state.
- **Crash consistency:** separate receipt/domain commits can duplicate work. Use
  shared operation identities and reconciliation; test each interruption boundary.
- **External uncertainty:** Jira/Linear comments may lack provider idempotency.
  Keep uncertain outcomes visible and avoid automatic replay.
- **Domain bypass:** UI-only completion or adapter-only claim checks are insufficient.
  Shared services and final transaction checks own these invariants.
- **Compatibility:** existing v1 conversation cleanup differs from the new retained
  lifetime. Keep the new contract opt-in and preserve old consumer tests.
- **Two-repository delivery:** public types and plugins must align to a real supported
  host version. Package from recorded SDK revisions and test installed archives.
- **Scope growth:** host-native PR writeback, terminal cleanup, and a policy DSL are
  separate initiatives; they are not implied by coordinator extensibility.

## Assumptions and handoff

The first coordinator is a reference implementation, not the only supported product.
The second policy is proposal-first to demonstrate a real difference in authority and
behavior. Jira/Linear are the initial host writeback providers. Unsupported providers
and optional operations remain visibly unavailable instead of blocking unrelated
features. Budgets are disclosed policy limits, not exact financial guarantees.

There are no blocking design questions for this package. Implementation proceeds
through the sequential work orders above. Remote repository publication,
marketplace release, and production workspace rollout remain outside this package.

## Code-review remediation (2026-09-27)

Fixed all five review findings: managed tool callbacks no longer hold the approval
mutex across plugin RPCs; uncertain exact execution replays are nil-safe and
successful replay payloads remain faithful; Host installation attribution permits
valid writes on never-claimed and released tasks; all exact execution controls are
fenced by the observed claim through the effect boundary; and managed-chat intent,
reads, and controller calls stay scoped to immutable conversation identity.
Regression tests cover real Host callbacks and Host-to-SQLite updates, pending and
successful execution replays, claim transfer/release races, and deferred instance
switches in both desktop and phone coordinator E2E.

Final verification passed: affected Go regression tests with the race detector,
task-claim service race coverage, SDK tests, web hook tests, web typecheck and
focused ESLint, backend build, web E2E build, coordinator package validation,
documentation validation (62 tests and 47 pages), and packaged desktop and phone
coordinator E2E. Changes are carried on the feature branch.

Additional verification on 2026-09-27: full automation, task-handler, config,
launcher, and agentctl process/probe package tests passed after fixing their
regressions and isolating managed-runner config variables in tests. The Linux
process-start clock now also accounts for time-namespace offsets. The full
backend `make test` run exceeded Go's 10-minute package timeout in the task
service and SQLite repository packages; rerunning the SQLite repository package
alone with eight-way test parallelism reached the same timeout while still
initializing repository fixtures. No assertion failure was reported before
those timeouts. The multi-target backend build passed after these changes.

## PR fixup verification (2026-09-27)

The PR fixup preserves task-delete confirmation IDs through bulk deletion and
uses the human-confirmed delete helper in preparation-attachment cleanup. It
dismisses the LSP status popover before editor interactions. On phone, manager
and completion summaries share a compact toolbar below the fixed top bar, with
44px Manage/Inspect actions and no duplicate top inset, so chat controls stay
above bottom navigation.

Local verification passed: web typecheck and scoped ESLint; the mobile session
layout unit suite (25 tests); the Vite E2E build; five phone E2E regressions for
clarification, full-queue layout, oversized messages, claims, and completion;
four desktop E2E regressions for LSP, file-tree drag/drop, completed-session
resume, and detached-turn cancellation; three additional cancellation repeats;
five bulk-delete/preparation-attachment E2E checks; documentation validation
(312 decisions and 1,195 specifications); all specification files; 83 PR-docs
unit tests; and whitespace checks.

The prior PR documentation-coverage check remains blocked by the trusted base
validator's 200 referenced-document limit. The `pull_request_target` workflow
checks out `github.workflow_sha`, so changing the validator in this feature
branch cannot repair that run. No `no-docs-allow` bypass was used. A fresh
current-head run must confirm the result; if it persists, the trusted mainline
validator must be expanded or this change must be split before the PR can be
considered clear.
