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

The owner uses `GET /assistant/memory` (optional `scope` and `after`) and `GET/PUT/DELETE /assistant/memory/:id`. PUT accepts key/content, scope/scope ID, an owner-authored source comment, confirmation, priority, expiry and `expected_revision`. DELETE requires the current revision. Legacy memories remain unconfirmed and workspace-scoped.

Context is fetched with:

```sh
kandev context --objective OBJECTIVE_ID --profile PROFILE_ID
```

This calls `GET /runtime/context/:objectiveId`. Optional `--task`, `--project` and `--environment` select an existing task, workspace repository and that task's environment. New-task handoffs cannot name an existing task or environment.

Packets are deterministic, at most 12 KiB of JSON, and include provenance and the selected profile revision. Memory excerpts are at most 1 KiB UTF-8. Confirmed preferences precede inferred activity; confirmed priority-100 memories are mandatory. Oversized indispensable constraints cause refusal, not silent omission. `omitted_memory`, excerpt truncation flags and stable memory IDs identify incomplete context; `kandev memory get --id MEMORY_ID` retrieves an exact entry.

Pass the returned packet ID as `--context`, with `--objective`, when delegating. The server attaches the packet. Edits, forgetting, expiry and account changes invalidate stale packets at dispatch, including queued messages. Idle-task reassignment can attach a fresh packet, but cannot authorize an older queued message under that new context.

Forgetting affects future assembly and delivery. It cannot erase text already sent to a provider or retained in historical messages/backups. Known credential-shaped tokens are redacted; this is not a guarantee that arbitrary pasted secrets will be detected. Never store secrets in memory.

## Credential descriptors

`GET /assistant/credentials` and `GET/PUT/DELETE /assistant/credentials/:id` manage separately typed references. Writes use `expected_revision`; descriptors specify resolver, stable reference, purpose, execution profile, account/environment, scope, required field names and unlock policy. Unknown fields, including secret-value fields, are rejected.

Only matching profile/scope context receives a descriptor. Kandev resolver health uses scoped metadata lookup without revealing a value. Bitwarden is reported unavailable until a supported resolver is attached; a descriptor alone does not install or unlock it. Locked/unavailable states carry the specified unblock action and never substitute another account or scan the user's vault.
