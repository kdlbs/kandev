---
status: shipped
created: 2026-04-28
owner: cfl
---

# Cheap Model Profiles

## Why

Office agents burn expensive model capacity on routine work that does not require high reasoning depth. A heartbeat wakeup — the agent checking in to see if there is anything new — or a `routine_trigger` processing a simple status summary consumes the same Opus-class tokens as a complex implementation task. At a heartbeat interval of minutes and any non-trivial agent count, this is the dominant cost driver even after idle-skip is applied.

The fix is simple: let operators configure a secondary, cheaper model profile per agent. Routine wakeups route to the cheap model; complex wakeups (new task assignments, unblocks, review results) continue to use the primary model. No change to behavior — only to which model is invoked.

## What

### Cheap model profile on an agent instance

Each agent instance can optionally reference a second agent profile as its `cheap_agent_profile_id`. When set, this profile is used for routine wakeups instead of the primary `agent_profile_id`.

- Stored on `AgentInstance` (and in the agent YAML) as `cheap_agent_profile_id`.
- Must reference a valid `AgentProfile` for the same underlying agent type.
- If unset, all wakeups use the primary profile (no behavior change).
- Both profiles may differ only in model (e.g. primary: `claude-opus-4-5`, cheap: `claude-haiku-4-5`). Other profile fields (CLI flags, mode) from the primary are preserved unless explicitly set differently in the cheap profile.

### Wakeup routing

At wakeup claim time, the scheduler resolves which profile to use before calling `taskStarter.StartTask`. Resolution order (first match wins):

1. **Task-level override** (`model_profile` field on the task): `"primary"`, `"cheap"`, or `""` (default routing).
2. **Wake reason routing** (see table below): routine wakeup reasons → cheap model; substantive wakeup reasons → primary model.
3. **Fallback**: primary model (when no cheap profile is configured or no override applies).

| Wakeup reason | Default profile |
|---|---|
| `heartbeat` | cheap |
| `routine_trigger` | cheap |
| `budget_alert` | cheap |
| `task_assigned` | primary |
| `task_comment` | primary |
| `task_blockers_resolved` | primary |
| `task_children_completed` | primary |
| `approval_resolved` | primary |
| `agent_error` | primary |

If no `cheap_agent_profile_id` is set, the primary is used regardless of reason.

### Task-level model profile override

Tasks gain an optional `model_profile` field (`"primary"` | `"cheap"` | `""`, default `""`). When set, it overrides the wake-reason routing for all wakeups that include this task's ID in their payload.

This is a user-set field editable via the task detail panel in the UI.

### Profile merge at launch

When the cheap profile is selected, the scheduler passes the cheap profile's `agent_profile_id` to `taskStarter.StartTask`. The existing profile resolution logic (model flag, CLI flags, mode) already operates on `AgentProfile.ID` — no changes needed there. The cheap profile must be pre-configured with the desired cheap model.

## UX

### Agent detail page (`/office/agents/[id]`) — Settings tab

A "Model Profiles" section with two rows:

- **Primary model**: read-only display of the current `agent_profile_id` (e.g. "Claude Opus 4.6"). Label: "Used for complex work: implementations, reviews, planning."
- **Cheap model**: dropdown to select an `AgentProfile` from the same agent type, or "None". Label: "Used for routine tasks: heartbeats, status checks, budget alerts."

On save, writes `cheap_agent_profile_id` to the agent YAML and syncs to the DB.

### Task detail panel — Properties section

A "Model profile" field (dropdown):

- **Default** — follow the agent's wake-reason routing.
- **Primary** — always use the primary profile for this task.
- **Cheap** — always use the cheap profile for this task.

Visible only when the assigned agent has a cheap profile configured; hidden otherwise.

### Agent card on `/office/agents`

When a cheap profile is configured, show both models under the agent name:

- Format: `Opus 4.6 / Haiku 4.5`
- No additional badge needed; the dual display is self-explanatory.

## Scenarios

- **GIVEN** an agent with `cheap_agent_profile_id` set to a Haiku profile, **WHEN** a `heartbeat` wakeup is processed, **THEN** the scheduler resolves the cheap profile and passes it to `StartTask`, launching the agent with the Haiku model.

- **GIVEN** the same agent, **WHEN** a `task_assigned` wakeup is processed, **THEN** the scheduler resolves the primary profile and the agent launches with Opus.

- **GIVEN** an agent with a cheap profile and a task with `model_profile = "primary"`, **WHEN** a `heartbeat` wakeup for that task is processed, **THEN** the task-level override takes precedence and the primary profile is used.

- **GIVEN** an agent with a cheap profile and a task with `model_profile = "cheap"`, **WHEN** a `task_assigned` wakeup for that task is processed, **THEN** the cheap profile is used despite `task_assigned` defaulting to primary.

- **GIVEN** an agent with no `cheap_agent_profile_id`, **WHEN** any wakeup is processed, **THEN** the primary profile is always used and no routing change occurs.

- **GIVEN** the agent card on the agents list, **WHEN** the agent has a cheap profile configured, **THEN** both model names are shown (e.g. "Opus 4.6 / Haiku 4.5").

## Out of scope

- Automatic routing based on task priority, complexity score, or estimated token count.
- More than two model tiers per agent (primary + cheap only).
- Cheap profile inheriting CLI flags or mode from the primary profile at launch time (users configure the cheap profile independently).
- Analytics distinguishing cheap-vs-primary token spend (covered by existing cost tracking if profiles are named descriptively).
- Configuring which wakeup reasons map to cheap vs primary (fixed routing table; not user-configurable).
