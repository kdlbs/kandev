---
status: current
system: tasks
requirements:
  - REQ-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001
---

# Task Create Agent Compatibility Recovery System Design

## Purpose and boundaries

The task-create dialog (`apps/web/components/task-create-dialog*.ts` and
`.tsx`) owns agent and executor selection for new tasks. This design covers how
the dialog derives an agent compatibility state, replaces an incompatible
selection, and presents each state on desktop and phone.

The design uses, and does not own, two adjacent contracts:

- The compatibility rule `isAgentConfiguredOnExecutor` in
  `apps/web/lib/agent-executor-compat.ts`, which reads the executor profile's
  `remote_credentials` and `remote_auth_secrets` configuration.
- The remote-auth catalog served by `GET /api/v1/remote-credentials` and cached
  by `useRemoteAuthSpecs` in
  `apps/web/hooks/domains/settings/use-remote-auth-specs.ts`.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Presentation](#presentation) |

## Components and responsibilities

- `apps/web/components/task-create-dialog-computed.ts`:
  `useExecutorProfileCompat` keeps filtering `agentProfiles` into
  `compatibleAgentProfiles` and derives `agentCompatState` through a pure
  `computeAgentCompatState` helper. `noCompatibleAgent` remains the derived
  boolean that gates submission.
- `apps/web/components/task-create-dialog-autopick.ts`:
  `decideAgentProfileAutopick` gains a replacement path for a selected profile
  that is no longer compatible. `useAgentProfileAutopickEffect` applies it.
- `apps/web/components/task-create-dialog-form-body.tsx`: `AgentColumn` renders
  the selector, the empty state, or the incompatible note per state.
- `apps/web/components/task-create-dialog-prop-builders.ts`: forwards the
  state, the selected agent profile label, and the effective workflow name.
- `apps/web/components/task-create-dialog-footer.tsx`: `computeDisabledReason`
  returns a state-specific reason key, and `resolveDisabledReason` fills the
  agent and executor names.
- Locale catalogs `apps/web/src/locales/<locale>/task.json` own the copy.

## Data and contracts

```ts
export type AgentCompatState = "compatible" | "selected-incompatible" | "none-compatible";
```

`computeAgentCompatState` derives the state from the selected executor profile,
`compatibleAgentProfiles`, and the effective agent profile id (the user value or
the workflow override):

1. No executor profile selected: `compatible`.
2. `compatibleAgentProfiles` is empty: `none-compatible`.
3. No effective agent id: `compatible`.
4. The effective agent id is absent from `compatibleAgentProfiles`:
   `selected-incompatible`.
5. Otherwise: `compatible`.

`noCompatibleAgent` equals `agentCompatState !== "compatible"`, so the footer
and submit gates keep their existing input.

`DialogComputedValues` adds `agentCompatState` and
`selectedAgentProfileName: string | null` (the label of the effective agent
profile). `DialogFormBodyProps` and `CreateEditSelectorsProps` add
`agentCompatState`, `selectedAgentProfileName`, and
`effectiveWorkflowName: string | null`.

The autopick decision keeps its shape and adds an optional `replaces` field:

```ts
{ kind: "pick"; source: "lastId" | "defId" | "first"; id: string; replaces?: string }
```

The replacement gate runs before the existing `already-set` skip. It yields a
pick with `replaces` set to the current id when all of these hold: the dialog is
open, an agent id is set, no workflow lock applies (`workflowAgentProfileId` is
empty and the workflow has no agent), the auth catalog is loaded, an executor
profile is selected, `compatibleAgentProfiles` is non-empty, and the current id
is absent from `compatibleAgentProfiles`. The candidate order is unchanged:
last-used, workspace default, first compatible.

### Presentation

| State | Workflow lock | Agent column |
| --- | --- | --- |
| `compatible` | any | Existing selector (`agent-profile-selector`). |
| `none-compatible` | any | `agent-profile-empty-state` with `task:noCompatibleAgentProfilesFor` and the credentials link. |
| `selected-incompatible` | no | Selector with the compatible options plus `agent-profile-incompatible-note` with `task:agentNotConfiguredOnExecutor` and the credentials link. This state is transient until the replacement applies. |
| `selected-incompatible` | yes | `agent-profile-incompatible-note` with `task:workflowAgentNotConfiguredOnExecutor` (workflow, agent, executor) and the credentials link. No selector. |

The credentials link keeps its existing target
`/settings/executors/<executor-profile-id>`.

Footer reason keys:

- `none-compatible`: existing `REASON_NO_COMPATIBLE_AGENT`
  (`task:noCompatibleAgentProfileFor`).
- `selected-incompatible`: new `REASON_SELECTED_AGENT_INCOMPATIBLE`
  (`task:selectedAgentNotConfiguredFor`, with `agent` and `target`).

New `task` namespace keys, added in `en`, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`,
and regenerated `pseudo`:

- `agentNotConfiguredOnExecutor`: "{{agent}} is not configured on {{target}}."
- `workflowAgentNotConfiguredOnExecutor`: "The {{workflow}} workflow uses
  {{agent}}, which is not configured on {{target}}."
- `selectedAgentNotConfiguredFor`: "{{agent}} is not configured on {{target}}.
  Configure agent credentials in Settings → Executors."

### Mobile composition

The dialog renders one component tree for both viewports. The agent and
executor selectors sit in a grid that collapses to one column below the `sm`
breakpoint, so the note renders under the selector at full dialog width and
wraps. No drawer, new surface, or touch control is added. The nearest shipped
mobile exemplar is the create-task dialog flow in
`apps/web/e2e/tests/task/mobile-create-task-branch-policy.spec.ts`, which
already drives the executor profile selector on a phone viewport.

## Control flow

1. The user picks an executor profile. `handleExecutorProfileChange` sets
   `fs.executorProfileId` and queues the last-used executor. This handler is
   unchanged.
2. `useExecutorProfileCompat` recomputes `compatibleAgentProfiles` and
   `agentCompatState` in the same render.
3. `useAgentProfileAutopickEffect` evaluates the replacement gate. A pick is
   applied through `setAgentProfileId` on the next microtask, the same deferral
   the existing autopick uses. The effect does not call
   `syncTaskCreateLastUsed`, so the stored last-used agent stays untouched.
4. `AgentColumn` renders per the presentation table, and the footer resolves
   the disabled reason.

When the workflow locks the agent, `useWorkflowAgentProfileEffect` keeps
setting the locked id, the replacement gate is skipped, and the locked note is
shown until the user changes the workflow or configures the credentials.

## Failure and recovery

- Auth catalog not loaded: `compatibleAgentProfiles` equals the selectable list
  and the state resolves to `compatible`. No replacement runs. This matches the
  existing deferral in the autopick gate.
- Catalog request failure: the cached specs are empty, every agent on a remote
  executor is blocked, and the state is `none-compatible`. The empty state and
  credentials link remain the recovery path. This behavior is unchanged.
- Agent profiles transiently empty during a refresh: `compatibleAgentProfiles`
  is empty, so the replacement gate does not fire and a valid selection is not
  cleared. The existing `agentProfilesLoading` guard keeps the empty state
  hidden while profiles load.

## Persistence

None. The replacement is dialog state only.
`users.settings.task_create_last_used.agent_profile_id` changes only through
the manual selection handler and successful task creation, as recorded in
[ADR 0028](../../../decisions/0028-task-create-last-used-source-of-truth.md)
and [ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md).

## Observability

The `executor-compat:autopick` debug logger records the `replaces` id and the
source of a replacement decision. The existing `[executor-compat]` per-agent
check logs (`ok`, `reason`) remain the diagnostic for why a profile is
incompatible.

## Related decisions

- [ADR 0028](../../../decisions/0028-task-create-last-used-source-of-truth.md)
- [ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md)
- [Resolve Task Executor Policy Before Last-Used Profile](../../../decisions/2026-08-01-repository-task-executor-defaults.md)
