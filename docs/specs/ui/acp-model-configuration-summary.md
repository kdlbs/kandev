---
status: shipped
created: 2026-07-15
owner: kandev
---

# ACP Model Configuration Summary

## Why

ACP agents can advertise an arbitrary ordered set of model-adjacent session configuration options. Showing every selected value in the task chat model trigger makes the compact chat toolbar difficult to scan, while showing values without their provider-supplied context makes unfamiliar modes hard to understand. Users need a compact indication of configuration changes without Kandev hard-coding provider-specific option knowledge.

## What

- Kandev records a write-once baseline of the effective ACP select-option values with which a task session starts. The baseline is stored in the task-session database metadata and survives backend restart, process recreation, and ACP session resume.
- The baseline is captured after profile and startup session configuration has settled. It is separate from mutable runtime configuration and is never used to restore provider state.
- In the task chat input and task context surfaces, the closed model selector always shows the current model name followed by every non-model config value whose raw current value differs from its baseline value, in ACP-provided option order. Values are joined by a slash with surrounding spaces; option names are omitted. Example: `GPT-5.6-Sol / Low / On`.
- Until a session baseline is available, the closed task selector shows every current non-model value rather than hiding options whose changed state cannot yet be determined.
- A value that returns to its baseline disappears from the closed summary. A currently advertised option with no baseline entry is treated as changed. Baseline entries for options the provider no longer advertises are ignored.
- Hovering or keyboard-focusing the closed desktop trigger exposes the current option names, selected value names, and provider descriptions when supplied. Opening the selector exposes the same provider descriptions and remains the complete configuration surface on touch devices.
- Kandev preserves optional ACP descriptions for both top-level config options and selectable values throughout the adapter, backend event, WebSocket, store, and selector pipeline. Missing descriptions produce no invented or hard-coded explanatory text.
- The compact baseline-aware summary applies only to task chat input and task context model selectors. Shared selector uses such as agent-profile settings and utility configuration continue to list every selected value in the closed trigger.
- Dynamic `config_option_update` payloads replace the live option set while retaining the original persisted baseline. Provider-added, removed, reordered, or dependent options are compared by stable option ID and raw value.
- Legacy task sessions that have no stored baseline establish one from their first fully settled effective configuration after this feature is deployed. They do not attempt to reconstruct historical defaults.

## Scenarios

- **GIVEN** a task session starts with model `GPT-5.6-Sol`, collaboration `Default`, reasoning `High`, and fast mode `Off`, **WHEN** no option changes, **THEN** the closed task-chat selector shows only `GPT-5.6-Sol`.
- **GIVEN** that baseline, **WHEN** reasoning changes to `Low`, **THEN** the closed task-chat selector shows `GPT-5.6-Sol / Low`.
- **GIVEN** reasoning is `Low` and fast mode is `On`, **WHEN** the selector is closed, **THEN** it shows `GPT-5.6-Sol / Low / On` in ACP option order without collapsing the changed values into a count.
- **GIVEN** a changed value is returned to its baseline, **WHEN** the selector rerenders, **THEN** that value is removed from the closed summary.
- **GIVEN** a task session has changed values, **WHEN** the backend restarts and recreates or resumes the ACP session, **THEN** the same baseline is loaded from task-session metadata and the closed summary still identifies the changes.
- **GIVEN** an ACP option or value supplies a description, **WHEN** the user opens the selector or inspects the closed desktop trigger, **THEN** Kandev shows the provider text. Missing descriptions leave the description region absent.
- **GIVEN** the same shared selector is rendered in agent-profile settings, **WHEN** it is closed, **THEN** it continues to list all selected values regardless of the task-session baseline.
- **GIVEN** a narrow touch viewport, **WHEN** the user taps the selector, **THEN** all current options and available descriptions remain reachable without hover or horizontal page scrolling.

## Data Model

- Task-session metadata contains a dedicated write-once ACP configuration baseline keyed by config option ID with raw selected values.
- The provider's latest mutable state remains in runtime configuration metadata. Explicit user selections are stored separately and applied as overrides after that provider state, preventing delayed provider events from replacing resume intent. Baseline, live state, and explicit overrides have distinct ownership and lifecycle semantics.
- ACP config option and option-value transport types carry optional descriptions.

## Failure Modes

- Failure to persist the first baseline must not prevent the session from running or configuration from being changed. Kandev reports the persistence failure and retries on a later settled configuration event without overwriting a baseline that was successfully stored.
- Unknown option types remain ignored according to existing ACP graceful-degradation behavior.
- Missing option names, value names, or descriptions fall back only to existing raw identifiers/values; Kandev does not infer provider semantics.

## Persistence Guarantees

- Once stored, the baseline is not replaced by later ACP updates, user selections, agent-initiated selections, backend restarts, or session resume.
- Baseline comparison is scoped to the task session, not the agent profile or provider globally.

## Out of Scope

- Defining or inferring provider defaults beyond the task session's recorded initial effective configuration.
- Hard-coded descriptions, aliases, importance rankings, or default values for individual ACP providers.
- Changing the closed-label behavior in agent-profile settings or other non-task selector surfaces.
- Adding support for ACP input control types that Kandev does not otherwise render.
