---
status: active
system: tasks
created: 2026-08-01
owners:
  - kandev
---
# Conditional Workflow Session Settings Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-WORKFLOW-SESSION-SETTINGS-001: Conditional Workflow Session Settings

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-SESSION-SETTINGS-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

Workflow authors can currently select a different agent profile for a step, but that creates or activates another task session and fragments the conversation across tabs. Authors need to route model and model-adjacent ACP settings by the agent family that started the task while preserving the original conversation context.

## What

- A workflow step can use one of three mutually exclusive agent behaviors:
  - keep the current agent behavior;
  - switch to a fixed agent profile using the existing step profile override; or
  - conditionally control the original task session's ACP model settings.
- Conditional session settings contain at most one rule per original agent family, identified by the stable agent name such as `codex` or `claude`, rather than by a specific agent profile.
- A rule operation is one of:
  - `set`: apply an explicitly selected model and zero or more ACP select configuration values;
  - `keep`: acknowledge that settings inherited from an earlier step should remain active without changing them; or
  - `restore_original`: actively reapply the original session configuration captured when the session first initialized.
- A step can contain rules for multiple agent families. Exactly one rule can match a task, because the task has one original session family. If no rule matches, Kandev changes no session settings.
- Rules apply only to the task's original session. They never create a session, activate another tab, change an agent profile, or apply one agent family's settings to a later profile-switched session.
- Kandev applies a matching `set` or `restore_original` rule before the step's auto-start prompt. A start-step rule applies before the task's first prompt.
- A successful rule updates the same durable session runtime overrides used by the task chat model selector. Its values survive agent-process recreation and backend restart and remain active in later workflow steps.
- Applying a later `set` rule replaces only the fields named by that rule. Unnamed fields retain their current values.
- `restore_original` reapplies the session's immutable original effective model and every captured selectable ACP configuration value. It therefore restores both explicit profile choices and provider-default values that the profile did not override.
- Restored values are persisted as explicit session runtime overrides. A later resume uses the restored values even if provider defaults or the agent profile have since changed.
- The workflow editor reuses the shared model/configuration selector used by the task chat input. It shows the selected model and all supported ACP select options supplied by that agent family's capability data; it does not hard-code provider-specific option names or values.
- The editor performs conservative workflow-path analysis by agent family. When a step can be reached with settings changed by an earlier rule and has no explicit `set`, `keep`, or `restore_original` decision for that family, the step shows a warning that the earlier values may carry forward.
- From that warning, the workflow author can explicitly keep the inherited values, restore the original values, or choose new values for the affected family. Saving any of those choices resolves the warning for that family and path.
- Workflow-path warnings consider configured relative and explicit transitions, including cycles. They are advisory and do not attempt to predict arbitrary manual card moves.
- The conditional rule editor is fully usable on desktop and mobile. On phones, rule cards form a one-column flow; family, operation, model, and option controls remain reachable by touch without document-level horizontal scrolling.
- The step header places an `Override original session options` checkbox beside the fixed agent-profile selector. Its help text explains that checking it keeps the original conversation tab while allowing a workflow step to change session settings (for example, starting with model 5.6 Sol and switching to 5.6 Luna for implementation work).
- The conditional rule editor is shown below the WIP controls only when `Override original session options` is checked. The fixed profile selector and this checkbox are mutually exclusive.
- Agent-family choices in conditional rules come only from agent families represented by configured agent profiles; capability data for an unconfigured family does not create a new choice.
- Read-only synced workflows display their rules and carry-forward warnings but do not allow edits, matching existing workflow settings behavior.

Decision: [ADR-2026-08-01-workflow-session-original-configuration](../../../decisions/2026-08-01-workflow-session-original-configuration.md).

## Data model

### Workflow step rule action

Conditional rules are stored inside the existing `workflow_steps.events` JSON as one `on_enter` action. No workflow-step table column is added.

```json
{
  "type": "configure_session",
  "config": {
    "rules": [
      {
        "agent_name": "codex",
        "operation": "set",
        "model": "gpt-5.6-luna",
        "config_options": {
          "reasoning_effort": "max"
        }
      },
      {
        "agent_name": "claude",
        "operation": "restore_original"
      }
    ]
  }
}
```

Constraints:

- `agent_name` is non-empty and textually unique within the action. It is resolved against the agent registry before a rule is selected, so a canonical agent ID (`claude-acp`), a display name (`Claude`), or an agent name (`Claude ACP Agent`) all select the same family, case-insensitively.
- Resolution is refused rather than guessed when a reference does not name exactly one installed agent. Custom TUI agents choose their own slug and display name and share the built-ins' namespace, so a reference can name two agents at once; a canonical agent ID is unambiguous by construction and always resolves.
- Because uniqueness is validated on the written `agent_name` but selection happens after resolution, two textually different rules can name one family. That is detected when the step runs, not when the workflow is saved. Applying either would make array order decide the step's model, so neither is applied.
- Both refusals are scoped to the session being configured. A step routinely carries rules for agents this session is not running, and a reference is only refused when it could govern *this* session — that is, when the session's own family is among the agents it names. An ambiguous reference naming only other agents is the same deliberate no-op as any other rule for a different agent, and does not withdraw the rule that does match. Refusing the whole action instead would let one colliding agent name switch per-step model selection off for every agent at once, which is the defect this resolution exists to remove.
- Two outcomes are silent, both of them deliberate no-ops for this session: a rule naming a known but different agent, and a rule whose reference is ambiguous among agents that do not include this session's family. Naming no known agent, naming a reference ambiguous *for this session*, and two rules resolving to the session's family each apply nothing and raise a session warning saying which — a session setting that goes missing without explanation is the other half of the same failure mode.
- `operation` is `set`, `keep`, or `restore_original`.
- A `set` rule contains a non-empty `model` or at least one non-empty `config_options` entry.
- `keep` and `restore_original` rules do not contain `model` or `config_options`.
- A step contains at most one `configure_session` action.
- A step with `configure_session` has no step-level `agent_profile_id`; attempts to persist or import both are rejected.

Workflow import/export keeps the family-based rule unchanged because agent names, model IDs, and ACP option IDs are portable provider contracts rather than workspace profile IDs.

### Original session identity

The first task session is marked as the task's original workflow session in `task_sessions.metadata`. Its stable family name comes from the session's profile snapshot `agent_name`. Existing sessions created before this feature may be treated as original only when Kandev can identify the task's earliest non-workflow-switch session unambiguously.

### Original effective configuration

After the original ACP session initializes and profile settings settle, `task_sessions.metadata` stores one immutable original effective configuration snapshot:

```json
{
  "model": "gpt-5.6-sol",
  "config_options": {
    "reasoning_effort": "high"
  }
}
```

The snapshot includes the effective profile-selected model and every provider-advertised selectable option's settled raw value, including unchanged provider defaults. It is distinct from:

- the provider-default comparison baseline used by compact selector labels;
- the provider's latest mutable runtime state; and
- explicit session runtime overrides.

The snapshot is written once and is never replaced by workflow rules, user selections, provider events, profile edits, process recreation, or session resume.

## API surface

Existing workflow create/update, snapshot, import, and export contracts carry the new `events.on_enter` action. No new HTTP or WebSocket endpoint is required.

The backend validates the action on workflow-step create/update and import. Invalid rules return the existing validation response for the calling surface and do not partially persist the step.

Existing agent capability responses provide the editor's family names, models, and ACP select options. Existing task-session model/configuration mechanisms apply and persist runtime changes.

When an ACP provider's options depend on the selected model, the editor uses the model-aware capability contract in [Dynamic Provider Model Options](../../agents/requirements/dynamic-provider-options.md) to replace the baseline option snapshot before saving a rule.

## State machine

For each original agent family on each possible workflow path, the editor and runtime use these effective states:

| Incoming state | Matching operation | Outgoing state | Runtime effect |
| --- | --- | --- | --- |
| original or changed | `set` | changed | Apply the named values. |
| original | `keep` | original | No mutation; intent is explicit. |
| changed | `keep` | changed | No mutation; carry-forward is explicit. |
| original or changed | `restore_original` | original | Apply every captured original value. |
| original or changed | no matching rule | unchanged | No mutation; warn in the editor when the incoming state may be changed. |

When multiple configured paths reach a step with different states, the editor treats the incoming state as possibly changed and warns until that family has an explicit decision.

## Permissions

The same users who can edit a workspace workflow can create, change, or remove conditional session rules. Synced read-only workflows remain non-editable. Runtime application occurs as part of the authorized task workflow transition and does not add a separate user-facing mutation permission.

## Failure modes

- If the original session family does not match any rule, Kandev performs no configuration call, records no override, and proceeds with the step normally.
- If the active session is not the task's original session, Kandev does not activate or mutate another session. It proceeds with the step and adds a visible workflow warning to the conversation.
- A matching rule applies best-effort by field. Unsupported or rejected models/options do not roll back successful fields and do not block the step's auto-start prompt.
- Any failed field produces one visible warning that identifies the step, agent family, failed fields, and values actually retained. Provider errors are sanitized before display.
- If a live field applies but its durable override cannot be persisted, Kandev warns that the value may not survive restart; other fields continue.
- If the session is not running but can be launched or resumed, Kandev records the requested overrides first so initialization applies them before the next prompt.
- If `restore_original` has no trustworthy original effective snapshot, Kandev changes nothing, warns that restoration is unavailable for that legacy or incomplete session, and continues the step.
- If an option captured in the original snapshot is no longer advertised, restoration skips that option, restores the remaining fields, and reports the skipped option in the warning.
- Capability discovery keeps the conditional editor readable and shows a localized, retryable status. While discovery is pending, coordinated save is blocked. After a failed discovery, the author may save the selected model without new or unverified option values; existing persisted rules remain visible, and failed discovery does not remove their saved option values.

## Persistence guarantees

- Workflow rules survive reload, workflow export/import, and backend restart through the existing workflow-step `events` JSON.
- The original-session marker and original effective configuration survive backend restart and agent-process recreation in task-session metadata.
- Successful `set` and `restore_original` values survive restart through explicit session runtime overrides.
- `keep` stores workflow author intent only; it writes no task-session runtime state.
- A provider update or profile edit does not rewrite a task session's original effective configuration.
- Deleting the task session deletes its original snapshot with the session row.

## Scenarios

- **GIVEN** a task starts with any Codex profile and the next step has `Codex -> Luna / max`, **WHEN** the task enters that step, **THEN** the existing original session tab remains active and its model and reasoning effort change before the step prompt runs.
- **GIVEN** the same step also has a Claude rule and the task started with Claude, **WHEN** the task enters the step, **THEN** only the Claude rule applies to the same original Claude session.
- **GIVEN** rules for Codex and Claude and a task that started with Grok, **WHEN** the task enters the step, **THEN** no settings change and the step proceeds without a no-match warning.
- **GIVEN** a Codex profile originally selected Sol while reasoning effort settled to the provider default High, **WHEN** a later Codex rule restores original settings, **THEN** Kandev actively sets both Sol and High and persists both values.
- **GIVEN** a prior Codex rule changed settings and a reachable later step has no Codex decision, **WHEN** the author edits that later step, **THEN** the editor warns that the prior values may remain and offers keep, restore, and choose-new actions.
- **GIVEN** that warning, **WHEN** the author chooses keep, **THEN** the warning resolves and entering the step performs no runtime mutation.
- **GIVEN** that warning, **WHEN** the author chooses restore, **THEN** the warning resolves and entering the step reapplies the captured original values before any auto-start prompt.
- **GIVEN** a workflow cycle can re-enter a step through both changed and original configuration paths, **WHEN** the author edits the step, **THEN** the editor conservatively reports that changed values may enter until the family has an explicit rule.
- **GIVEN** a matching rule whose model succeeds but one config option is rejected, **WHEN** the task enters the step, **THEN** the model remains changed, the prior option value remains, one visible warning describes the partial result, and auto-start continues.
- **GIVEN** a task whose current session was activated by an earlier fixed profile override, **WHEN** it enters a conditional-settings step, **THEN** Kandev does not switch tabs or apply the rule to that session and adds a visible warning.
- **GIVEN** a new task starts directly in a step with a matching `set` rule, **WHEN** its original ACP session initializes, **THEN** Kandev captures the pre-rule original effective configuration, applies the rule, and sends the first prompt with the selected settings.
- **GIVEN** a legacy task session with no trustworthy original effective snapshot, **WHEN** a restore rule matches, **THEN** no value is guessed or changed and the conversation shows that restoration is unavailable.
- **GIVEN** a saved conditional-settings step, **WHEN** the workflow is exported and imported into another workspace with the same agent family capability, **THEN** the family rule, model, option values, and operations round-trip without profile matching.
- **GIVEN** a phone viewport, **WHEN** the author adds Codex and Claude rules and chooses model/config values, **THEN** every action is reachable by touch, the rule list has one vertical scroll owner, and the page has no horizontal overflow.
- **GIVEN** a step with no fixed profile override, **WHEN** the author leaves `Override original session options` unchecked, **THEN** the conditional rule editor is not shown below the WIP controls.
- **GIVEN** the same step, **WHEN** the author checks `Override original session options`, **THEN** the conditional rule editor appears below the WIP controls and the original-session help text explains the Sol-to-Luna example.
- **GIVEN** capability data for Codex and Grok but configured profiles only for Codex, **WHEN** the author adds a conditional rule, **THEN** the family selector offers Codex but not Grok.

## Out of scope

- Matching one specific agent profile rather than an agent family.
- Creating, switching, reactivating, or renaming task sessions.
- Changing agent credentials, CLI flags, permission mode, executor, environment, MCP configuration, or profile settings.
- Automatically restoring settings merely because a task leaves a step.
- Predicting arbitrary manual card moves in the editor's carry-forward analysis.
- Inferring, translating, or aliasing provider-specific model and option identifiers.