---
title: "Workflow Import and Export"
description: "Move Kanban workflows between workspaces with Kandev's portable YAML format."
---

# Workflow Import / Export

Kandev's versioned portable format moves Kanban workflow definitions between workspaces or installations. It carries prompts, step behavior, portable agent-profile descriptors, WIP rules, and supported events. It deliberately omits database IDs and workspace ownership.

Use this for snapshots and one-time copies. Use [Workflow Sync](workflow-sync.md) when a GitHub repository should remain the source of truth.

## Use the UI

Open **Settings → Workspaces → select a workspace → Workflows**.

- **Export All** opens a YAML dialog for the saved Kanban workflows visible on that settings page. Unsaved drafts and Office-style workflows are excluded. Choose **Copy** to put the text on the clipboard.
- A workflow card's **Export** button exports only that workflow.
- **Import** accepts a `.yml` or `.yaml` file, or pasted YAML. The result reports created and skipped workflow names.

Export does not download a file or change the workflow. Import creates new workflows; it never overwrites a same-named workflow. Delete an unwanted imported workflow through the normal workflow settings flow.

## HTTP routes

All routes are on the Kandev backend under `/api/v1`. The current backend does not authenticate these routes, so treat anyone who can reach them as trusted to read or create workspace workflows. Keep the backend on loopback or put it behind an authenticated, origin-protected reverse proxy before exposing it to a network.

| Method | Route | Behavior |
|--------|-------|----------|
| `GET` | `/workflows/:id/export` | Export one workflow as `application/x-yaml`. |
| `GET` | `/workspaces/:id/workflows/export` | Export all non-hidden workflows in the workspace. |
| `GET` | `/workspaces/:id/workflows/export?ids=id1,id2` | Export only the listed workflow IDs. Whitespace and empty comma elements are ignored. |
| `POST` | `/workspaces/:id/workflows/import` | Parse a portable YAML request and return `{"created": [...], "skipped": [...]}`. |

The workspace export route treats an absent `ids` parameter as “all”; `ids=` is an explicit empty selection and returns an envelope with no workflows. Such an envelope cannot be imported because validation requires at least one workflow. The UI supplies IDs for its Kanban-only selection. A direct “all” HTTP export can include workflow styles the portable converter cannot completely represent, so prefer the UI for user-managed Kanban workflows.

The import handler reads at most 1 MiB. It uses a limited reader rather than a dedicated `413` check, so an oversized document is truncated and normally fails YAML parsing or validation. The HTTP route always uses the YAML decoder. The same structs have JSON tags because GitHub workflow sync accepts `.json` files, but YAML is the import endpoint's documented request format.

Example with `curl`:

```bash
curl -fsS \
  "http://localhost:38429/api/v1/workspaces/WORKSPACE_ID/workflows/export?ids=WORKFLOW_ID" \
  -o workflow.yml

curl -fsS \
  -H 'Content-Type: application/x-yaml' \
  --data-binary @workflow.yml \
  "http://localhost:38429/api/v1/workspaces/WORKSPACE_ID/workflows/import"
```

If Kandev is behind a reverse proxy, use its externally protected base URL rather than the loopback example.

## Envelope

Every document uses this envelope:

```yaml
version: 1
type: kandev_workflow
workflows:
  - name: My Workflow
    steps: []
```

| Field | Type | Validation |
|-------|------|------------|
| `version` | integer | Must be exactly `1`. |
| `type` | string | Must be exactly `kandev_workflow`. |
| `workflows` | list | Must contain at least one item. |

Unknown YAML fields are ignored by the current decoder. Do not use that as an extension mechanism: ignored data will disappear on the next export.

## Workflow fields

```yaml
- name: Delivery
  description: Plan, implement, then review.
  agent_profile:
    agent_name: Claude Code
    model: optional-model-id
    mode: optional-mode-id
  steps: []
```

| Field | Export behavior | Import behavior |
|-------|-----------------|-----------------|
| `name` | Always emitted. | Exact empty string is rejected. Existing target-workspace name causes a skip. |
| `description` | Omitted when empty. | Stored as supplied. |
| `agent_profile` | Omitted when no workflow profile resolves. | Exact value match; see [Profile matching](#profile-matching). |
| `steps` | Always emitted, possibly empty. | Empty lists are currently accepted, although an empty workflow is not useful. |

IDs, workspace ID, ordering among workflows, source/sync ownership, style, visibility, and timestamps do not round-trip.

## Step fields

```yaml
- name: Work
  position: 1
  color: bg-blue-500
  prompt: |
    Implement this task:
    {{task_prompt}}
  events:
    on_enter:
      - type: auto_start_agent
    on_turn_complete:
      - type: move_to_step
        config:
          step_position: 2
  is_start_step: true
  show_in_command_panel: true
  allow_manual_move: true
  auto_archive_after_hours: 0
  auto_advance_requires_signal: true
  wip_limit: 2
  pull_from_step_position: 0
  agent_profile:
    agent_name: Claude Code
```

| Field | Type | Exact behavior |
|-------|------|----------------|
| `name` | string | Always exported; exact empty string is rejected. |
| `position` | integer | Always exported and must be unique inside its workflow. It is an ordering key and reference anchor; built-ins use contiguous zero-based values, but validation does not require that. |
| `color` | string | Always exported. Kandev normally stores a background utility class such as `bg-blue-500`; portable validation does not allow-list colors. |
| `prompt` | string | Omitted when empty. `{{task_prompt}}` is the editor-supported task-description placeholder. |
| `events` | object | Always exported, even when empty. Supported portable triggers are below. |
| `is_start_step` | boolean | Always exported. Missing input decodes as `false`. The editor enforces one start step, but portable validation does not. During import, later `true` steps demote earlier ones. |
| `show_in_command_panel` | boolean | Always exported. Missing input decodes as `false`. |
| `allow_manual_move` | boolean | Always exported. Missing input decodes as `false`. |
| `auto_archive_after_hours` | integer | Omitted when `0`; `0` disables auto-archive. The portable validator currently does not reject negative values, so use only `0` or a positive value. |
| `agent_profile` | object | Omitted when unset; exact-match behavior is below. |
| `auto_advance_requires_signal` | boolean | Always exported. `true` makes `on_turn_complete` transitions wait for `step_complete_kandev`; missing input is `false`. |
| `wip_limit` | integer | Omitted when `0`. Must be non-negative; `0` is unlimited. |
| `pull_from_step_position` | integer | Optional feeder reference using another step's `position`. It must exist, cannot point to itself, and cannot form a pull cycle. |

`stage_type`, Office participants, recorded decisions, task data, and step history are not portable. Imported steps receive new UUIDs and the default internal stage type.

## Portable events

An event contains an ordered list of actions. Each action has a `type` and an optional `config` map.

| Trigger | Runtime meaning | Recognized action types |
|---------|-----------------|-------------------------|
| `on_enter` | Step-entry processing. | `enable_plan_mode`, `auto_start_agent`, `reset_agent_context`, `set_session_mode`, `clear_decisions`, `queue_run`, `queue_run_for_each_participant` |
| `on_turn_start` | A user sends a message. | `move_to_next`, `move_to_previous`, `move_to_step` |
| `on_turn_complete` | An agent turn completes. | `move_to_next`, `move_to_previous`, `move_to_step`, `disable_plan_mode` |
| `on_exit` | A task leaves the step. | `disable_plan_mode` |

`set_session_mode` requires `config.mode` to be a non-empty string. `move_to_step` requires `config.step_position` pointing to a position in the same workflow:

```yaml
on_turn_complete:
  - type: disable_plan_mode
  - type: move_to_step
    config:
      step_position: 3
```

Internally, transitions use database `step_id` values. Export converts `step_id` to `step_position`; import creates all new IDs and converts positions back to them. Additional config keys are copied. Do not copy the embedded template files verbatim: those are an internal template schema and use symbolic `step_id` values rather than the portable envelope and positions.

Portable validation is deliberately narrow. Beyond `set_session_mode` and position references, it does not currently reject every unknown action string or malformed action config. An accepted file can therefore contain an inert action. Use the action names and shapes documented here and exercise the workflow after import.

### Office triggers do not round-trip

The runtime model also has `on_comment`, `on_blocker_resolved`, `on_children_completed`, `on_approval_resolved`, `on_heartbeat`, `on_budget_alert`, and `on_agent_error`. The current portable conversion copies only the four triggers in the table above. Hand-authored Office triggers in a portable file are discarded during import conversion, and Office fields are omitted on export.

The Workflows settings UI filters Office-style workflows from its list and Export All selection for this reason. Manage Office workflow behavior through its product surface; do not use portable Kanban import/export as an Office backup.

## Profile matching

Profile IDs are installation-specific, so the portable descriptor stores values:

```yaml
agent_profile:
  agent_name: Claude Code
  model: optional-model-id
  mode: optional-mode-id
```

`agent_name` is the agent display name, not an internal ID. On import Kandev searches for a profile whose display name, model, and mode all match exactly. Empty optional values also participate in the match.

If there is no exact match, import still succeeds and silently leaves that workflow or step profile unset. Before moving a file between installs, compare the destination's agent display names and supported model/mode identifiers. Never assume an illustrative model name exists in another install.

## Import reconciliation and failure behavior

Import follows these rules:

1. The complete envelope is decoded and validated before creation begins.
2. Existing workflows are compared by exact name. Matches are reported under `skipped`; they are not updated or merged.
3. Each new workflow and its steps receive fresh IDs. Step-position references are remapped to those IDs.
4. Profile descriptors are matched by value.

Validation failure writes nothing. Creation itself is not one transaction across the file, however. A database or profile-update failure after creation begins can leave earlier workflows, a workflow without all steps, or other partial state. Inspect the workspace after a runtime error and delete incomplete workflows before retrying.

The validator currently does **not** require a step, contiguous or non-negative positions, unique step names, unique workflow names within the same document, a valid color, or exactly one start step. GitHub workflow sync adds a unique-step-name requirement because it reconciles by name. For predictable results, enforce all of those constraints in authored files even when the one-time importer accepts them.

## Executable example

This file creates a three-step queue. Work has a capacity of two and pulls from Backlog whenever a slot opens. Its turn-complete transition only runs after the agent emits the explicit completion signal.

```yaml
version: 1
type: kandev_workflow
workflows:
  - name: Review Queue
    description: Bounded implementation queue with explicit completion.
    steps:
      - name: Backlog
        position: 0
        color: bg-neutral-400
        events: {}
        is_start_step: false
        show_in_command_panel: false
        allow_manual_move: true
        auto_advance_requires_signal: false

      - name: Work
        position: 1
        color: bg-blue-500
        prompt: |
          Complete the task, verify it, then call step_complete_kandev.

          {{task_prompt}}
        events:
          on_enter:
            - type: auto_start_agent
          on_turn_complete:
            - type: move_to_step
              config:
                step_position: 2
        is_start_step: true
        show_in_command_panel: true
        allow_manual_move: true
        auto_advance_requires_signal: true
        wip_limit: 2
        pull_from_step_position: 0

      - name: Review
        position: 2
        color: bg-yellow-500
        events:
          on_turn_start:
            - type: move_to_step
              config:
                step_position: 1
        is_start_step: false
        show_in_command_panel: true
        allow_manual_move: true
        auto_advance_requires_signal: false
```

After import, assign a workflow-level or Work-step agent profile if the destination did not produce an exact portable profile match. Create a disposable task, verify Backlog → Work pulling, the WIP rejection at capacity, explicit completion, and Review feedback before adopting it.

## Troubleshooting

- **`unsupported export version/type`:** keep `version: 1` and `type: kandev_workflow` exactly.
- **Duplicate step position:** give every step in that workflow a unique integer and update every position reference.
- **Missing `step_position`:** portable `move_to_step` never accepts a database or template `step_id`.
- **Pull reference error:** ensure the target position exists, is not the same step, and does not participate in a cycle.
- **Workflow skipped:** rename either the destination workflow or the imported workflow; import is create-or-skip, not update.
- **Profile missing after import:** match display name, model, and mode exactly, or select a profile in settings afterward.
- **Event vanished:** only the four portable triggers round-trip; Office triggers and metadata do not.
- **Large import reports strange YAML:** keep the request below 1 MiB; the route truncates at that boundary.
- **Import failed after creating something:** creation is not an all-or-nothing transaction; remove partial results and retry a corrected file.

Related pages: [Workflows](workflow-tips.md), [Workflow Sync](workflow-sync.md), and [WebSocket API](websocket-api.md) for the separate live-message protocol.
