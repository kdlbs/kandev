---
title: "Workflow Import and Export"
description: "Move workflows between workspaces or Kandev installs with portable YAML."
---

# Workflow Import / Export — Portable YAML Format

Kandev workflows can be exported to a portable YAML file and imported into
another workspace (or another Kandev install). This page is the reference for
that file format: the envelope, every field, the trigger config shapes, and the
matching rules applied on import.

Everything below is derived from the source of truth:

- `apps/backend/internal/workflow/models/export.go` — the portable structs (`WorkflowExport`, `WorkflowPortable`, `StepPortable`, `AgentProfilePortable`) and `Validate()`.
- `apps/backend/internal/workflow/models/models.go` — the `StepEvents` triggers and their action types.
- `apps/backend/internal/workflow/service/service.go` — `ImportWorkflows` / `importSingleWorkflow` (matching + position→ID mapping).

> **Looking for the built-in workflows instead?** See [Workflows](workflow-tips.md)
> for the default templates and when to use each.

---

## How import/export is exposed

The format is YAML over three HTTP endpoints (all under `/api/v1`):

| Method | Path | Purpose | Response / Body |
|--------|------|---------|-----------------|
| `GET` | `/workflows/:id/export` | Export a single workflow | `application/x-yaml` |
| `GET` | `/workspaces/:id/workflows/export` | Export **all** workflows in a workspace | `application/x-yaml` |
| `POST` | `/workspaces/:id/workflows/import` | Import workflows into a workspace | YAML request body, **max 1 MB** |

Export marshals the structs to YAML; import unmarshals the request body, runs
`Validate()`, then creates each workflow. The same struct shapes also marshal to
JSON (every field carries both `yaml` and `json` tags), but the endpoints speak
YAML.

---

## Envelope

The top-level document is a `WorkflowExport`:

```yaml
version: 1
type: kandev_workflow
workflows:
  - # WorkflowPortable …
  - # WorkflowPortable …
```

| Field | Type | Required | Notes |
|-------|------|:--------:|-------|
| `version` | int | yes | Must be exactly `1` (`ExportVersion`). Any other value is rejected. |
| `type` | string | yes | Must be exactly `kandev_workflow` (`ExportType`). |
| `workflows` | list | yes | Must contain at least one workflow; an empty list is rejected. |

`Validate()` rejects the document with a descriptive error if `version`,
`type`, or the workflow list fails these checks.

---

## `WorkflowPortable`

Each entry under `workflows:`:

```yaml
- name: My Workflow
  description: Optional human description.
  agent_profile:        # optional, workflow-level default agent
    agent_name: Claude Code
    model: claude-opus-4-7
    mode: default
  steps:
    - # StepPortable …
```

| Field | Type | Required | Notes |
|-------|------|:--------:|-------|
| `name` | string | yes | Workflow name. Required by `Validate()`. Used for **dedup on import** (see [Import rules](#import-matching-rules)). |
| `description` | string | no | Omitted from export when empty. |
| `agent_profile` | object | no | Workflow-level default agent profile. See [Agent profiles](#agent-profiles). Omitted when the workflow has no profile. |
| `steps` | list | — | The workflow's steps. See [`StepPortable`](#stepportable). |

Instance-specific fields (IDs, timestamps, workspace association) are **not**
part of the portable format — they are regenerated on import.

---

## `StepPortable`

Each entry under `steps:`:

```yaml
- name: In Progress
  position: 1
  color: bg-blue-500
  prompt: |
    Optional per-step prompt sent to the agent.
  is_start_step: true
  show_in_command_panel: true
  allow_manual_move: true
  auto_archive_after_hours: 24
  agent_profile:           # optional, step-level agent override
    agent_name: Claude Code
    model: claude-opus-4-7
    mode: default
  events:
    # triggers — see "Triggers" below
```

| Field | Type | Required | Default in export | Notes |
|-------|------|:--------:|-------------------|-------|
| `name` | string | yes | — | Required by `Validate()`. |
| `position` | int | yes | always emitted | **0-based** index. **Must be unique** within the workflow — duplicates are rejected. Also the anchor for `move_to_step` references (see below). |
| `color` | string | — | always emitted | Tailwind background class, e.g. `bg-blue-500`, `bg-green-500`, `bg-neutral-400`. |
| `prompt` | string | no | omitted when empty | Per-step prompt template. Supports placeholders such as `{{task_prompt}}`. |
| `events` | object | — | always emitted | Triggers and their actions. See [Triggers](#triggers). |
| `is_start_step` | bool | — | always emitted | Marks the step new tasks start in. |
| `show_in_command_panel` | bool | — | always emitted | Whether the step appears in the command panel. |
| `allow_manual_move` | bool | — | always emitted | Whether users can drag the task into this step manually. |
| `auto_archive_after_hours` | int | no | omitted when `0` | Auto-archive a task this many hours after it lands in the step. `0` / omitted = never. |
| `agent_profile` | object | no | omitted when none | Step-level agent profile, overriding the workflow default. See [Agent profiles](#agent-profiles). |

> **Note:** `position`, `color`, `events`, and the three booleans carry no
> `omitempty`, so they always appear in exported files (even when `false` or
> empty). The `prompt`, `auto_archive_after_hours`, and `agent_profile` fields
> are omitted when unset.
>
> **Not in the portable format:** office/Phase-2 step metadata — `stage_type`,
> step participants (reviewers/approvers), and recorded decisions — is **not**
> exported or imported. Only the fields listed above round-trip.

---

## Agent profiles

`agent_profile` appears at both the workflow level and the step level
(`AgentProfilePortable`):

```yaml
agent_profile:
  agent_name: Claude Code   # required
  model: claude-opus-4-7    # optional, omitted when empty
  mode: default             # optional, omitted when empty
```

| Field | Type | Notes |
|-------|------|-------|
| `agent_name` | string | The agent's **display name** (`AgentDisplayName`), not its internal ID. |
| `model` | string | Model identifier. Omitted when empty. |
| `mode` | string | Agent mode. Omitted when empty. |

Profiles are stored by **value** (name/model/mode) rather than by ID precisely
so they can be re-matched in a different workspace. See the matching rule below.

---

## Triggers

`events` holds the step's triggers. Each trigger is a list of actions; an
action is `{ type, config }` where `config` is an optional map.

There are two families of triggers.

### Kanban-era triggers (round-trip today)

These four triggers use typed action slices and are fully supported by
import/export:

| Trigger | Allowed action `type`s | `config` |
|---------|------------------------|----------|
| `on_enter` | `enable_plan_mode`, `auto_start_agent`, `reset_agent_context`, `set_session_mode`, `clear_decisions`, `queue_run`, `queue_run_for_each_participant` | the first three take no config; `set_session_mode` takes `mode` (the agent permission mode to apply, e.g. `acceptEdits`); `queue_run` / `queue_run_for_each_participant` use the same config keys as the office triggers (see [Office triggers](#office--phase-2-triggers-are-not-portable)) |
| `on_turn_start` | `move_to_next`, `move_to_previous`, `move_to_step` | `move_to_step` needs `step_position` |
| `on_turn_complete` | `move_to_next`, `move_to_previous`, `move_to_step`, `disable_plan_mode` | `move_to_step` needs `step_position` |
| `on_exit` | `disable_plan_mode` | — |

Example:

```yaml
events:
  on_enter:
    - type: auto_start_agent
  on_turn_start:
    - type: move_to_next
  on_turn_complete:
    - type: move_to_step
      config:
        step_position: 2
```

#### `move_to_step` uses `step_position`, not `step_id`

This is the one transformation the portable format performs. Internally a step
transition references a target step by its database `step_id`. Because IDs are
not portable, export rewrites every `move_to_step` action's
`config.step_id` → `config.step_position`, and import rewrites it back to a
freshly generated `step_id`.

So in a portable file you **always** write:

```yaml
- type: move_to_step
  config:
    step_position: 2     # the target step's `position`, NOT a step id
```

`Validate()` enforces that every `move_to_step` `step_position` matches an
existing step's `position` in the same workflow; a dangling reference is
rejected. Any additional keys in the action's `config` are preserved verbatim
through the conversion.

> **Built-in template YAMLs differ.** The embedded templates under
> `apps/backend/config/workflows/*.yml` use string `step_id`s (e.g.
> `step_id: review`) because they are *template definitions*, a different schema
> from this portable export format. Do not copy their `step_id:` form into a
> portable import file — use `step_position:`.

### Office / Phase-2 triggers are not portable

The portable import/export format is for kanban workflows. The runtime model
also defines `on_comment`, `on_blocker_resolved`, `on_children_completed`,
`on_approval_resolved`, `on_heartbeat`, `on_budget_alert`, and
`on_agent_error`, but these office-style triggers are outside this format.

> [!WARNING]
> Do not hand-author office-style triggers in a portable workflow file. The
> conversion in `export.go` preserves only `on_enter`, `on_turn_start`,
> `on_turn_complete`, and `on_exit`; unsupported triggers are discarded. The
> Settings UI excludes office workflows from export. [Issue
> #1109](https://github.com/kdlbs/kandev/issues/1109) records the original bug
> report and the UI-side mitigation.

---

## Import matching rules

`ImportWorkflows` → `importSingleWorkflow` applies these rules:

1. **Validation first.** The whole document is run through `Validate()`
   (envelope + per-workflow name/position/`move_to_step` checks) before
   anything is created. A failure aborts the entire import.

2. **Workflow dedup by name.** Existing workflows in the target workspace are
   listed; any imported workflow whose `name` already exists is **skipped**
   (reported under `skipped`). The rest are **created** (reported under
   `created`). The result is `{ created: [...], skipped: [...] }`.

3. **Fresh step IDs + position→ID mapping.** Every step gets a newly generated
   UUID. Import builds a `position → new step ID` map, then rewrites each
   `move_to_step` action's `step_position` back into the new `step_id`. This is
   why step positions must be unique and why `move_to_step` references positions.

4. **Agent profile matched by name/model/mode.** For each `agent_profile`
   (workflow-level and step-level), import searches the target workspace's
   agents and profiles for one whose **`agent_name` (display name), `model`, and
   `mode` all match exactly**. On a match, that profile's ID is assigned. On **no
   match**, the field is left empty — the workflow/step is created **without** an
   agent profile (silently; no error). Match the names/models/modes in the
   target workspace if you need the profile wired up.

---

## Complete worked example

A self-contained, valid import file with two workflows: a four-step kanban loop
(using `move_to_step` with `step_position`) and a two-step planning flow with a
per-step agent profile and prompt.

```yaml
version: 1
type: kandev_workflow
workflows:
  - name: Simple Kanban
    description: Assign → run → review loop.
    steps:
      - name: Backlog
        position: 0
        color: bg-neutral-400
        is_start_step: false
        show_in_command_panel: false
        allow_manual_move: true
        events:
          on_turn_start:
            - type: move_to_next

      - name: In Progress
        position: 1
        color: bg-blue-500
        is_start_step: true
        show_in_command_panel: true
        allow_manual_move: true
        events:
          on_enter:
            - type: auto_start_agent
          on_turn_complete:
            - type: move_to_step
              config:
                step_position: 2

      - name: Review
        position: 2
        color: bg-yellow-500
        is_start_step: false
        show_in_command_panel: true
        allow_manual_move: true
        events:
          on_turn_start:
            - type: move_to_previous

      - name: Done
        position: 3
        color: bg-green-500
        is_start_step: false
        show_in_command_panel: false
        allow_manual_move: true
        auto_archive_after_hours: 168
        events:
          on_turn_start:
            - type: move_to_step
              config:
                step_position: 1

  - name: Plan & Build
    description: Plan first, then implement.
    agent_profile:
      agent_name: Claude Code
      model: claude-opus-4-7
      mode: default
    steps:
      - name: Plan
        position: 0
        color: bg-purple-500
        is_start_step: true
        show_in_command_panel: true
        allow_manual_move: true
        prompt: |
          Analyze the task and produce an implementation plan. Do not write code.
          Save the plan with create_task_plan_kandev, then stop for review.
        agent_profile:
          agent_name: Claude Code
          model: claude-opus-4-7
          mode: plan
        events:
          on_enter:
            - type: enable_plan_mode
            - type: auto_start_agent
          on_exit:
            - type: disable_plan_mode

      - name: Implementation
        position: 1
        color: bg-blue-500
        is_start_step: false
        show_in_command_panel: true
        allow_manual_move: true
        prompt: |
          Retrieve the plan with get_task_plan_kandev, then implement it.
        events:
          on_enter:
            - type: auto_start_agent
```

Importing this into a fresh workspace creates both workflows
(`created: [Simple Kanban, Plan & Build]`). Re-importing the same file leaves
them untouched (`skipped: [Simple Kanban, Plan & Build]`). The `Claude Code`
agent profiles wire up only if a profile with that exact display name, model,
and mode exists in the target workspace.
