---
title: "Personal assistant backend"
description: "Experimental assistant intake, objectives and scoped context API reference."
status: experimental
---

# Personal assistant backend

This reference describes the in-development backend for a user-owned assistant built on [workspace orchestration](orchestration-personas.md), independently of Office.

It is **not ready for production use**. The central assistant UI, provider-enforced read-only execution and native attention/input handling are not implemented yet.

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

Do not treat an `inspect` label as an enforced sandbox; the remaining authority
work is required before using that mode with a provider.

## Ownership and intake

Routes below use the `/api/v1/orchestration` prefix. Human endpoints use Kandev's authenticated identity; runtime credentials cannot configure the user's assistant, confirm memory or edit credential descriptors.

- `GET /assistant` reads the owner's current binding.
- `PUT /assistant` accepts `orchestrator_id` and `expected_version` (zero for initial selection).
- `POST /tasks/:id/comments` accepts a stable `client_message_id` with `body`. Initial durable acceptance returns 201; an identical retry returns 200; a changed payload under the same ID returns 409.
- `GET /tasks/:id/comments` uses `before` for cursor-based history. A receipt can be accepted before a run ID exists.

Selecting an assistant claims its conversation for that user. Changing the default or removing its binding does not make old history, memory or configuration shared. The same owner can read old history and select it again; another user cannot claim it. Existing bindings are backfilled during migration, but ownership already lost before this repair cannot be reconstructed automatically.

A previously selected private conversation must be reselected before accepting new turns. Old run credentials and queued launches are rejected after a binding change. Pending instructions that lose authority receive `receipt_status: "superseded"`, without being executed. Background automations currently cannot target private assistants because automation records do not yet carry durable owner authorization; never-claimed workspace orchestrators remain supported automation targets.

## Objectives and operations

Runtime objective routes are `GET/POST /runtime/objectives` and `PATCH /runtime/objectives/:id`. The owner can list goals at `GET /assistant/objectives`. Lists return `next_cursor`, passed as `after`.

Create an objective with `title`, `mode`, `source_comment_id` and `acceptance` entries containing `id` and `description`. Supported modes are `answer`, `inspect`, `execute` and `design`; answer/inspect do not create delivery tasks. Updates use `expected_revision`; changing acceptance invalidates prior evidence.

Assistant mutations require `operation_id` and `expected_intent_revision`. Reuse an operation ID only for an identical retry. An unknown outcome is not permission to repeat an external action; inspect its native result first. Newer user intent invalidates old run writes.

The runtime CLI exposes `kandev objective list|create|update`. Delivery creation accepts `--mode execute|design` and an explicit permitted `--step`. Workflow defaults and repository-required design/review gates are not changed. Completion requires current evidence plus settled workers and required reviews; REVIEW alone is not completion.

## Memory and context

The owner uses `GET /assistant/memory` (optional `scope`, `scope_id`, `limit` and
`after`) and `GET/PUT/DELETE /assistant/memory/:id`. Lists default to 50 entries
and cap at 100, ordered by stable memory ID. Pass the opaque `next_cursor` as
`after` with the same filters. A cursor is bound to the owner and current binding;
malformed or mismatched cursors return 400. Expired, forgotten, foreign-owned
and unavailable-scope entries are excluded. Explicit invalid scope references
return 422 without changing stored data. PUT accepts key/content, scope/scope ID, an owner-authored source comment, confirmation, priority, expiry and `expected_revision`. DELETE requires the current revision. Legacy memories remain unconfirmed and workspace-scoped.

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
