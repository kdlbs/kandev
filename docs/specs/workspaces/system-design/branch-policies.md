---
status: draft
system: workspaces
created: 2026-08-24
owners:
  - kandev
requirements:
  - REQ-WORKSPACES-BRANCH-POLICIES-001
  - REQ-WORKSPACES-BRANCH-POLICIES-002
  - REQ-WORKSPACES-BRANCH-POLICIES-003
  - REQ-WORKSPACES-BRANCH-POLICIES-004
  - REQ-WORKSPACES-BRANCH-POLICIES-005
---

# Branch policies system design

## Summary

A branch policy is repository-owned configuration. Task creation sends a policy
ID, the backend resolves it, and the resulting task-repository row owns an
immutable snapshot. Repository settings manage policies through immediate CRUD,
while task creation shows policies and Git branches as typed option groups in
the existing selector.

This extends, rather than replaces, the repository
`worktree_branch_template`. That field remains the compatibility fallback for
raw branch selections and pre-policy tasks.

## Domain model

Add `repository_branch_policies`:

| Field | Contract |
| --- | --- |
| `id` | Stable UUID. |
| `repository_id` | Required repository owner; delete cascades from repository. |
| `name` | Trimmed display name, 1 to 100 characters. |
| `description` | Optional text, at most 500 characters. |
| `base_branch` | Safe Git ref used as task base. |
| `branch_template` | Template validated and rendered by `internal/worktree`. |
| `pull_request_target` | Safe Git ref; defaults to `base_branch` when omitted by a client. |
| `created_at`, `updated_at` | UTC timestamps. |

A unique index on `(repository_id, lower(name))` enforces name uniqueness. Lists
sort by case-folded name and stable ID. The table is part of both the SQLite
base schema and replayable SQLite/Postgres migrations.

Extend `task_repositories` with snapshot fields:

- `branch_policy_id`;
- `branch_policy_name`;
- `worktree_branch_template`;
- `pull_request_target`.

`base_branch` already stores the selected base. The policy ID is historical
provenance, not a foreign key. This keeps the complete snapshot after a policy
is deleted. Empty values identify raw-branch and legacy rows.

## Backend boundaries

### Repository policy service

The task repository interface gains list, get, create, update, delete, and
atomic Gitflow-starter operations. The service owns:

- workspace/repository authorization;
- normalized validation and case-insensitive conflicts;
- defaulting an omitted pull-request target to the normalized base branch;
- branch-template validation through the existing renderer;
- transactions and structured mutation logs.

The Gitflow starter accepts only production and development branch refs. It
checks that both refs are present in the current local or remote branch list,
checks that the repository has no policies inside the same transaction, and
inserts all four policies or none. The branch can still disappear after this
check, so task creation and later runtime operations retain their existing
missing-branch recovery behavior.

### API and events

Expose:

- `GET /api/v1/repositories/{repository_id}/branch-policies`;
- `POST /api/v1/repositories/{repository_id}/branch-policies`;
- `POST /api/v1/repositories/{repository_id}/branch-policies/gitflow`;
- `PATCH /api/v1/repository-branch-policies/{policy_id}`;
- `DELETE /api/v1/repository-branch-policies/{policy_id}`.

REST and WebSocket transports call the same service methods. Semantic
`repository_branch_policy.created`, `.updated`, and `.deleted` events keep
clients synchronized. The active-workspace boot payload includes policies
grouped by repository so task creation does not add a first-open request.

Conflicts return `409`; invalid refs, templates, or Gitflow pairs return a
validation error; an inaccessible or cross-repository policy is indistinguishable
from not found.

### Task creation and snapshot resolution

Add optional `branch_policy_id` to each task repository input. In the task-create
transaction, the service:

1. loads the selected repository under the authorized workspace;
2. loads the policy by ID and verifies repository ownership;
3. copies its name, base, template, and pull-request target to
   `task_repositories`;
4. creates no task if any selected policy cannot be resolved.

The backend ignores any browser-derived policy template or target. A policy
selection supplies its base branch authoritatively, even though the UI also
shows that base for feedback.

Runtime worktree creation, local fresh-branch creation, and title-triggered
branch rename read the task snapshot first. They read the repository template
only when the snapshot template is empty. The web pull-request flow reads the
snapshot target first and then the task base branch.

The orchestrator also derives trusted agent context from policy-backed
task-repository snapshots. The context lists the repository, working branch,
and pull-request target. It tells agents to pass the target to the provider CLI
instead of inferring it from the base branch. First launches and agent-context
resets receive the context. Office agents receive the same trusted block.
Passthrough agents receive a compact plain-text instruction because they do not
receive hidden Kandev system blocks. Raw-branch tasks add no instruction.

Prompt construction reads only the task snapshot. It does not read the live
policy. Repeated record-and-launch wrapping replaces the same hidden target
block, so the agent receives one copy. Agent skills and shell environments do
not receive a separate target value.

## Frontend state and transport

Add a repository-policy API domain and a `repositoryBranchPolicies` store slice
keyed by repository ID. Boot hydration and semantic events update the same
slice. Store mutations occur only after API success.

Task repository draft rows add a tagged selection:

```ts
type TaskRepositoryBranchSelection =
  | { kind: "branch"; branch: string }
  | { kind: "policy"; policyId: string; baseBranch: string };
```

The submit adapter converts the tagged selection to `base_branch` plus optional
`branch_policy_id`. It never recognizes a policy by label. Last-used repository
and raw branch behavior stays backend-owned under ADR 0028; the first release
does not persist a last-used policy.

## Repository settings experience

Each saved repository editor contains a collapsed `Branch policies` disclosure
with a count. Expanded content has one visible explanatory sentence, the policy
list, Add policy, and, when empty, Add Gitflow policies. Policies use immediate
modal CRUD and a delete confirmation, so they do not register a dirty settings
contributor or add a local Save button.

The policy form contains name, optional description, base branch, branch
template, and pull-request target. Every technical field has concise visible
supporting text. Focusable info controls explain examples, template placeholders,
and how base differs from pull-request target. Base and target fields reuse the
task branch option model: local refs keep their short names, remote refs keep
their remote prefix, and badges distinguish their source. The shared selector
provides filtering and force-refresh. A saved ref that disappeared remains a
temporary option while the user edits the policy.

Desktop uses a dialog. At phone breakpoints the same form logic renders in a
full-height drawer with one scrolling body and a safe-area footer. Help uses a
tooltip/popover for fine pointers and the established touch drawer pattern for
coarse pointers. The disclosure and list remain inline in the repository
editor; list actions meet the 44 CSS pixel touch target.

The Gitflow starter is a separate guided dialog/drawer with production and
development branch selectors and a preview of the four resulting policies.

## Task-create experience

Extend the existing branch `Pill` selector to accept typed grouped options. It
shows `Branch policies` first and `Branches` second. A policy row keeps its name,
localized `Policy` badge, and information control on one line. Hover or keyboard
focus opens the base, template, target, and unavailable-base details on fine
pointers; tap opens the same details in a drawer on coarse pointers. The closed
chip renders a compact policy name plus base branch.

On a local executor, choosing a policy explicitly enables the existing `Fork a
new branch` state. It does not bypass dirty-tree consent. Choosing a raw branch
restores the existing semantics. A policy with a base branch known missing from
the latest branch list is disabled with repair guidance.

The shared New Task/New Subtask repository picker gets this behavior. Quick Chat,
Remote URL, unsaved-path discovery, Add Sources, and Add Branch do not.

On phone, the existing task selector remains a popover because it is already a
compact, viewport-contained selection surface. Policy rows keep one-line
identity and use a touch drawer for their detailed preview.

## Accessibility and localization

- Disclosure triggers expose expanded state and policy count.
- Group labels, badges, help triggers, field descriptions, errors, and
  confirmations use translations.
- Help triggers are focusable, have accessible names, and expose equivalent
  content by hover, focus, and tap.
- Dialog and drawer focus management uses existing shadcn primitives.
- English copy is added to all supported locale catalogs; Traditional Chinese
  variants use the repository conversion workflow.

## Failure and recovery

- A stale/deleted policy at submit fails task creation atomically. The client
  refreshes policies, keeps the dialog open, and asks the user to choose again.
- A policy base that disappeared after creation follows current worktree launch
  recovery; Kandev does not rewrite the policy.
- A failed CRUD mutation leaves the confirmed list unchanged and shows the
  backend error.
- Concurrent Gitflow starter calls are serialized by the empty-set check and
  unique constraints.

## Security and observability

Policy reads and writes use repository membership authorization. IDs from other
workspaces do not reveal existence. Templates remain data for the safe renderer;
they are not shell fragments.

Structured logs cover policy mutations, Gitflow starter results, task policy
resolution, and compatibility fallback. Logs include identifiers, not policy
descriptions. No new production metric is introduced initially.

## Migration and rollout

The database migration is additive and replayable. Existing repositories gain
no policies automatically, and their fallback templates are unchanged. Existing
task requests and rows remain valid because all new fields are optional or
empty by default. No runtime feature flag is required because the no-policy path
is unchanged and policies are opt-in.

## Verification strategy

- Repository tests cover persistence, replay, normalization, conflicts,
  authorization, cascade, and atomic Gitflow seeding in SQLite and Postgres.
- Service/runtime tests cover snapshot resolution, stale IDs, raw fallback,
  local fresh branches, title rename, pull-request targets, and agent context.
- Frontend unit tests cover the settings CRUD state, responsive surface choice,
  tagged selector options, local fork transition, and submit payload.
- Desktop and `mobile-chrome` Playwright tests cover collapsed settings, help
  access, Gitflow seeding, policy selection, and visible post-selection state.

## Requirement traceability

| Requirement | Design areas |
| --- | --- |
| REQ-WORKSPACES-BRANCH-POLICIES-001 | Domain model, repository service, settings experience |
| REQ-WORKSPACES-BRANCH-POLICIES-002 | Repository service, Gitflow starter, failure recovery |
| REQ-WORKSPACES-BRANCH-POLICIES-003 | Frontend state, task-create experience |
| REQ-WORKSPACES-BRANCH-POLICIES-004 | Task snapshot resolution, runtime, agent context, migration |
| REQ-WORKSPACES-BRANCH-POLICIES-005 | Responsive UI, accessibility, localization, compatibility |
