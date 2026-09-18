---
title: "Personal assistant backend"
description: "Experimental assistant intake, objectives and scoped context API reference."
status: experimental
---

# Personal assistant backend

This reference describes the in-development backend for a user-owned assistant built on [workspace orchestration](orchestration-personas.md), independently of Office.

It is **not ready for production use**. Native attention, input resolution, pause/stop and the central Assistant UI are implemented. Maintenance, workspace grants and combined qualification are still pending.

Both `features.orchestration` (`KANDEV_FEATURES_ORCHESTRATION`) and
`features.personalAssistant` (`KANDEV_FEATURES_PERSONAL_ASSISTANT`) must be enabled
for the assistant. Both default off in every shipped profile and require a restart
after a change through Settings > System > Feature Toggles. An explicit environment
value overrides and locks the corresponding setting. Enabling orchestration alone
keeps ordinary workspace coordinators available without enabling the assistant.

With the assistant disabled, assistant routes return 404 with
`personal_assistant_disabled`; new private conversation turns, runtime tools and
queued/native launches are blocked. Pending intake, schemas and retained ownership
remain intact. Owners can still read protected conversation history. Re-enabling
rechecks current ownership, intent and context before work can execute.

Inspection uses a restricted native broker on managed Claude ACP 0.75.1 with a
local, repository-free executor. Profiles with custom CLI/environment/launcher
overrides, fallback routing or executor scripts are unsupported. Other providers
and versions are unsupported for this assistant path. These restrictions do not
change ordinary coordinator or worker execution. Provider availability still
depends on the selected profile's existing credentials.

## Assistant interface

The app-level `/assistant` route uses the selected private conversation, with
native questions and permission decisions, objectives/evidence, memory editing
and forgetting, capability health and activity. Desktop and phone layouts share
the same transports and revision checks. The disabled route does not read private
assistant data or create a binding. See the [usage guide](orchestration-personas.md#use-the-personal-assistant).

## Ownership and intake

Routes below use the `/api/v1/orchestration` prefix. Human endpoints use Kandev's authenticated identity; runtime credentials cannot configure the user's assistant, confirm memory or edit credential descriptors.

- `GET /assistant` reads the owner's current binding.
- `PUT /assistant` accepts `orchestrator_id`, `expected_version` (zero for initial selection), and `execution_mode` (`answer`, `inspect`, `design`, `execute`; default `inspect`). Changing mode revokes older binding versions.
- `POST /tasks/:id/comments` accepts a stable `client_message_id` with `body`. Initial durable acceptance returns 201; an identical retry returns 200; a changed payload under the same ID returns 409.
- `GET /tasks/:id/comments` uses `before` for cursor-based history. A receipt can be accepted before a run ID exists.

Selecting an assistant claims its conversation for that user. Changing the default or removing its binding does not make old history, memory or configuration shared. The same owner can read old history and select it again; another user cannot claim it. Existing bindings are backfilled during migration, but ownership already lost before this repair cannot be reconstructed automatically.

A previously selected private conversation must be reselected before accepting new turns. Old run credentials and queued launches are rejected after a binding change. Pending instructions that lose authority receive `receipt_status: "superseded"`, without being executed. Background automations currently cannot target private assistants because automation records do not yet carry durable owner authorization; never-claimed workspace orchestrators remain supported automation targets.

## Objectives and operations

Runtime objective routes are `GET/POST /runtime/objectives` and `PATCH /runtime/objectives/:id`. The owner can list goals at `GET /assistant/objectives`. Lists return `next_cursor`, passed as `after`.

Create an objective with `title`, `mode`, `source_comment_id` and `acceptance` entries containing `id` and `description`. Supported modes are `answer`, `inspect`, `execute` and `design`; answer/inspect do not create delivery tasks. Updates use `expected_revision`; changing acceptance invalidates prior evidence.

Assistant mutations require `operation_id` and `expected_intent_revision`. Reuse an operation ID only for an identical retry. An unknown outcome is not permission to repeat an external action; inspect its native result first. Newer user intent invalidates old run writes.

The runtime CLI exposes `kandev objective list|create|update`. Delivery creation accepts `--mode execute|design` and an explicit permitted `--step`. Workflow defaults and repository-required design/review gates are not changed. Completion requires current evidence plus settled workers and required reviews; REVIEW alone is not completion.

## Supervised workflow maintenance

Maintenance is available behind the personal-assistant feature flag, with proposal
and review controls in the assistant's Details view. Lists and details use
`GET /assistant/improvements` and
`GET /assistant/improvements/:id`; current broker runs have equivalent
`/runtime/improvements` read routes. Lists use bounded `limit` and opaque `after`
cursors. Evidence contains typed native metadata, not private prompt bodies.
Three matching incidents across two tasks in seven days create a proposal.
Reading old native events again does not create new occurrences. Incidents are
retained for 30 days; explicit human review receipts remain. Historical incidents
from an earlier profile configuration are not reassigned to the current account.

Only the owner can `PUT /assistant/improvements/:id/grant`, with
`expected_binding_version`, `expected_revision`, `candidate_revision`,
`expires_at` and `scope`. Scope names `repository_id`, `workflow_id`,
`workflow_step_id`, `profile_id`, exact `files`, local `actions`
(`read`, `patch`, `test`, `commit`), an installed Linux container `image`, and
`positive_check`/`negative_check` argument arrays. The backend qualifies local
Docker isolation, resolves the immutable image and committed repository base,
and refuses unsupported configurations. Expiry is at most seven days.
`DELETE` on the grant route requires the current binding and grant revisions.
`GET /assistant/maintenance-options` pages through native repository, ordinary
workflow, safe entry-step and execution-profile choices without exposing secrets.

`POST /assistant/improvements/:id/maintenance` and its `/runtime` counterpart
accept `prepare`, `patch`, `check` or `commit`, plus operation ID, expected intent
and binding revisions, candidate revision and grant revision. Patch supplies
`file: {path, sha256, content}`. Preparation creates a private checkout and one
native review task. A general agent cannot run that task. Checks run offline in
a read-only container and can read the entire committed repository snapshot;
only the exact granted files can be patched. Commit requires passing positive
and negative checks for the same tree and grant revision. Any successful patch
invalidates earlier validation. The named broker tools are `improvements`,
`improvement`, `maintenance_file`, `maintenance_artifact` and `maintenance`.

The resulting local commit is a prepared review artifact, not proof that the
affected workflow recovered. Maintenance does not publish, create a PR, deploy,
restart the application or modify the original checkout. Revocation blocks
subsequent effects while preserving completed receipts.

`GET /assistant/improvements/:id/evidence` returns a bounded page from that
proposal's incident cohort. `GET .../:id/file` requires `path`,
`expected_binding_version` and `grant_revision`. `GET .../:id/artifact` returns
the complete local patch, base/commit/tree IDs, patch hash and matching check
receipt. Current native read access permits human artifact review after revocation.
Runtime file/artifact reads require the current grant and its revisions.

`GET .../:id/successes` supplies later completed affected native task/session
results, excluding unsettled workers or review gates. Human-only
`POST .../:id/review` takes `expected_binding_version`, `expected_revision`,
`action` (`rejected` or `resolved`) and, for resolution, `evidence` with
`source_kind: "task_message"`, `task_id`, `session_id`, `source_id`. The backend
revalidates the result and requires a prepared local repair before resolution.
Model assertions, fewer permission prompts and old results are insufficient.
A closed proposal retains its own evidence; a recurrence requires three new
incidents rather than reopening the old review.

Interrupted dispatch is shown as unknown. Human-only `POST .../:id/reconcile`
accepts current binding/candidate revisions and can record an already created,
validated local commit. It never repeats a patch, check, task creation or commit.
An interrupted repair without a verifiable commit remains unknown and can be
inspected or rejected. Grant, review and reconciliation routes are unavailable
to the assistant broker.

## Memory and context

The owner uses `GET /assistant/memory` (optional `scope`, `scope_id`, `limit` and
`after`) and `GET/PUT/DELETE /assistant/memory/:id`. Lists default to 50 entries
and cap at 100, ordered by stable memory ID. Pass the opaque `next_cursor` as
`after` with the same filters. A cursor is bound to the owner and current binding;
malformed or mismatched cursors return 400. Expired, forgotten, foreign-owned
and unavailable-scope entries are excluded. Explicit invalid scope references
return 422 without changing stored data. PUT accepts key/content, scope/scope ID, an owner-authored source comment, confirmation, priority, expiry and `expected_revision`. DELETE requires the current revision. Legacy memories remain unconfirmed and workspace-scoped.

`GET /assistant/memory/:id/source` returns the visible memory's owner-authored
source instruction from this private conversation, bounded to 6,000 Unicode
characters. `truncated` identifies an excerpt. Missing, foreign or no-longer-visible
memory returns 404; runtime credentials cannot use this human provenance endpoint.
Owner-only `orchestration.assistant.updated` events contain binding/revision hints,
not private content. Clients refetch authorized snapshots after changes and
reconnecting.

Context is fetched with:

```sh
kandev context --objective OBJECTIVE_ID --profile PROFILE_ID
```

This calls `GET /runtime/context/:objectiveId`. Optional `--task`, `--project` and `--environment` select an existing task, workspace repository and that task's environment. New-task handoffs cannot name an existing task or environment.

Packet identities are deterministic, packets are at most 12 KiB of JSON, and include provenance and the selected profile revision. Memory excerpts are at most 1 KiB UTF-8. Confirmed preferences precede inferred activity; confirmed priority-100 memories are mandatory. Oversized indispensable constraints cause refusal, not silent omission. `omitted_memory`, excerpt truncation flags and stable memory IDs identify incomplete context; the returned `memory_reference` retrieves full scoped entries in bounded pages.
For a CLI continuation, repeat the packet scope and use:

```sh
kandev context --objective OBJECTIVE_ID --profile PROFILE_ID --task TASK_ID \
  --memory --limit 50 --after OPAQUE_CURSOR
```

Omit `--after` for the first page. The endpoint is
`GET /runtime/context/:objectiveId/memory`. Its cursor includes the context
revision, so edits require a fresh packet. Legacy coordinator memory commands
remain available for ordinary workspace coordinators.

Pass the returned packet ID as `--context`, with `--objective`, when delegating. The server attaches the packet. Edits, forgetting, expiry and account changes invalidate stale packets at dispatch, including queued messages. Idle-task reassignment can attach a fresh packet, but cannot authorize an older queued message under that new context.

Forgetting affects future assembly and delivery. It cannot erase text already sent to a provider or retained in historical messages/backups. Known credential-shaped tokens are redacted; this is not a guarantee that arbitrary pasted secrets will be detected. Never store secrets in memory.

## Credential descriptors

`GET /assistant/credentials` and `GET/PUT/DELETE /assistant/credentials/:id` manage separately typed references. Writes use `expected_revision`; descriptors specify resolver, stable reference, purpose, execution profile, account/environment, scope, required field names and unlock policy. Unknown fields, including secret-value fields, are rejected.

Only matching profile/scope context receives a descriptor. Reads include a
server-generated `validation` object: `status`, `descriptor_revision`,
`profile_id`, `reference`, `configuration_generation`, `reason`, and
`validated_at` only after an actual metadata check. The supported statuses are
`ready`, `locked`, `missing`, `unavailable` and `unknown`. Validation fields are
not accepted in descriptor writes.

Every read checks the selected profile and referenced metadata again. Lookup
runs under the assistant owner's identity, never an unscoped internal identity.
`ready` confirms metadata availability; it does not prove external authentication
or grant permission to use a secret. No value-reveal or vault-list method is used.
Descriptor, account/profile, reference, resolver-generation and health changes
invalidate queued context. A fresh check timestamp alone preserves packet
identity. Validation observations are not stored in the descriptor record. Bitwarden is reported unavailable until a supported resolver is attached; a descriptor alone does not install or unlock it. Locked/unavailable states carry the specified unblock action and never substitute another account or scan the user's vault.

## Capability directory

`GET /assistant/capabilities` and `GET /runtime/capabilities` return the same
owner-scoped directory. Use `kind` to select `native`, `profile`, `workflow`,
`executor`, `integration`, `plugin` or `mcp`. Lists default to 50 entries and
cap at 100. Pass `next_cursor` as `after` with the same filters. A malformed or
foreign cursor returns 400; a changed directory generation returns 409 and
requires restarting pagination. Every page rechecks current workspace access.

```sh
kandev capabilities --kind mcp --session SESSION_ID --limit 50
```

Add `--after OPAQUE_CURSOR` for later pages. `--session` selects attachment
evidence for a session of the bound assistant conversation; runtime calls
otherwise use their own session. Configured profile servers appear separately
from tools observed on that session. Missing, disconnected, disabled and stale
states are explicit. Stored integration health does not trigger a fresh provider
probe or credential reveal.

Entries include stable identity, kind, name, resource scope, generation/revision,
surfaces, effect, health/reason, `configured`, `attached` and `inspect_allowed`.
An entry grants no authority. Executors and unverified plugin/MCP operations are
not marked inspect-safe. Conversation plugin tools require explicit manifest
API-version-2 opt-in and keep their existing per-operation MCP names.

`input_schema` is a structural projection bounded to 8 KiB, six nested levels
and 64 properties per object. Descriptions, defaults, examples, references and
extensions are omitted. `schema_partial: true` means the invocation's full
native schema must still be consulted and validated. No provider configuration,
environment, credential value or worker transcript is returned.

## Coordinator task observations

`GET /api/v1/workspaces/:workspaceId/tasks?view=kanban` is the read-only task
source for the Coordinator page. It requires the normal workspace read
permission and applies ordinary-task, archive, ephemeral, configuration, hidden
workflow and Office-workflow exclusions before totals, filtering and pagination.
Existing callers that omit `view=kanban` retain their existing behavior.

The page supplies `page_size=100`; `page`, `query`,
`workflow_id` and `repository_id` use the existing task-list query contract.
Text search in this view does not add command-palette pull-request-number search
results. Native `task.status_summary.updated` events update only the matching
workspace; lifecycle events and reconnects refresh the loaded window. No
conversation history or worker transcript is fetched to classify task rows.

## Execution authority

Private assistant runtime tokens have a separate audience and are valid only
for the active run/session, binding, intent and current profile/executor. A
changed profile, unavailable workspace or stale queue requires fresh admission.
The central assistant receives named tools through `agentctl kandev assistant-mcp`.
It has no provider-native shell or file tools and no external MCP/plugin tools.

Inspect permits authorized reads and internal objective/conversation receipts.
Design and execute can manage native worker tasks under their existing workflow,
context and approval gates. A runtime cannot change its owner's selected mode,
confirm memory, edit credential descriptors or approve native permissions.
Unknown operation outcomes remain unknown until native evidence resolves them.
Use `GET /runtime/memory` for bounded, redacted, scoped runtime memory; its
`next_cursor` is passed as `after`. Human memory editing remains separate.


## Attention

`GET /assistant/attention` and `GET /runtime/attention` return owner-authorized
cards for managed objective/task links. Lists default to 50, cap at 100, and
accept `next_cursor` as `after`. Cursors bind owner, binding and binding version.
Every page rechecks task access and current management links.

A card identifies the native task, session and request, with bounded redacted
summary, source revision, projection revision and state. `pending`, `resolved`,
`expired`, `unknown` and `inactive` are distinct. A waiting session alone is not
a question. All sessions are inspected; newer work cannot hide an older request.
Permission cards require a matching live provider handle. Lost handles expire;
the card does not recreate a permission or restart a worker.

Native events refresh attention without a model request. The existing run
scheduler repairs missed events every 60 seconds in batches of 100 managed task
links. Duplicate observations and unchanged scans do not wake the model. Each
new native occurrence commits a wake outbox with its projection; retries keep
the same queue identity. Pause/disable preserves cards and prevents dispatch.
Before launching a queued wake, the server re-reads its native source and scope.

The `orchestration.assistant.updated` WebSocket notification reaches only the
owner and contains `binding_id` and `revision`. Re-fetch the authorized page on
notification or reconnect. Native resolution controls are added separately.


## Native questions and controls

`GET /assistant/attention/:id/input` returns the current attention revision and
native input, including every offered option. Submit a human response to
`POST /assistant/attention/:id/resolve` with `operation_id`,
`expected_intent_revision`, `expected_binding_version`, `expected_revision`,
`source_revision`, and `session_id`. Questions use `answers` entries with
`question_id`, `selected_options` and/or `custom_text`; a permission uses its
actual `option_id`. The original native authorization, current-turn and delivery
checks apply. Expired requests remain expired. Reuse the exact operation ID and
payload after a lost response; an unknown receipt must not trigger a blind retry.

The broker exposes `attention_input` and `answer_question`. An assistant answer
requires the native question's explicit `assistant_delegable: true`, a current
worker `context_ref`, and `memory_ids` citing confirmed, untruncated memory in
that context. The selected execution mode must permit worker writes. Native
messages retain the assistant identity and cited memory IDs. A runtime cannot
approve a permission, resolve authentication, or label its answer as a human's.

Human `POST /assistant/control` accepts `pause`, `resume`, or
`stop_managed_work` plus the same operation, intent and binding identities.
Pause blocks new assistant turns while existing workers keep running. Stop
invalidates older queued assistant commands and returns a receipt per managed
session: `stopped`, `already_finished`, `failed` or `unknown`, with `partial`
and a scope-bound `next_cursor`. Follow each cursor explicitly using the returned
intent revision and a new operation ID. Stop neither deletes history nor
undoes external changes. Both controls remain separate from native task status.

## Linked workspaces

In Assistant → Details → Linked workspaces, choose a workspace, review the named
receiving profile, select context fields and explicitly confirm access. Observation
is required; coordination is optional. Workers retain their selected profiles and
native workflow/approval rules. Nothing automatically adopts another workspace.

Human APIs:

| Method and path | Behavior |
| --- | --- |
| `GET /assistant/workspace-options` | Native-visible choices and current receiving profile/revisions |
| `GET /assistant/workspace-links` | Current grants, activity state and reason |
| `PUT /assistant/workspace-links/:workspaceId` | Grant/reconfirm exact scope and receiving account |
| `DELETE /assistant/workspace-links/:workspaceId` | Revoke future access |
| `POST /assistant/workspace-links/:workspaceId/forget` | Delete saved handoff packets and invalidate old queued references |
| `GET /assistant/workspace-links/:workspaceId/events` | Grant/revoke/forget audit |
| `GET /assistant/workspace-exports` | Metadata-only possible-delivery records |

Mutations require `expected_binding_version` and `expected_revision` (zero for a
new grant). Grant requests also include the exact `receiver` returned by options
and `scope: {operations, context_exports}`. Operations are `observe` and optional
`coordinate`. Exports are `directory`, `task_summary`, `task_result`, `task_input`
and `handoff`; results/input require summary scope. List APIs accept `after` and
`limit` with a maximum of 100 and return `entries` and `next_cursor`.

The assistant broker's `workspace_links` tool discovers current authorized links.
For each supported linked read/write, send `workspace_id` and
`workspace_grant_revision` in `query`. The broker rechecks them against current
owner access, binding, profile, intent and native authority. `workspace_tasks`
returns bounded task summaries. Linked `workspace` returns names/routing IDs;
`task_details` adds bounded agent-result excerpts only with `include_result=true`
and matching result permission. Raw foreign tasks/comments, capability/plugin
configuration and maintenance operations are unavailable through linked targets.
Use current explicit targets for objectives, context, attention and task controls.

Linked handoffs contain the owner request, objective, acceptance and user-wide
preferences. Home workspace memory and account-bound credential descriptors are
not copied. A new receiving account requires reconfirmation covering historical
exports before central conversation history can be reused. Revocation blocks
new reads/wakes/writes; it cannot erase text already in chat or at a provider.
Forgetting deletes saved handoff packets, while native tasks, conversation history
and metadata-only audit receipts remain. It does not restore revoked access.
