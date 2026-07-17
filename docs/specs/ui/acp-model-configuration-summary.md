---
status: shipped
created: 2026-07-15
owner: kandev
---

# ACP Model Configuration Summary

## Why

ACP agents can advertise an arbitrary ordered set of model-adjacent session configuration options. Showing every selected value in the task chat model trigger makes the compact chat toolbar difficult to scan, while showing values without their provider-supplied context makes unfamiliar modes hard to understand. Users need a compact indication of configuration changes without Kandev hard-coding provider-specific option knowledge.

## What

- Kandev records a write-once baseline of the initial ACP select-option values advertised by the provider before agent-profile or runtime overrides are applied. The baseline is stored in the task-session database metadata and survives backend restart, process recreation, and ACP session resume.
- The provider-default baseline is separate from mutable runtime configuration and is never used to restore provider state. A profile-selected value that differs from the provider default is therefore shown as changed as soon as the task session starts.
- In the task chat input and task context surfaces, the closed model selector always shows the current model name followed by every non-model config value whose raw current value differs from its baseline value, in ACP-provided option order. Values are joined by a slash with surrounding spaces; option names are omitted. Example: `GPT-5.6-Sol / Low / On`.
- Until a session baseline is available, the closed task selector shows every current non-model value rather than hiding options whose changed state cannot yet be determined.
- A value that returns to its baseline disappears from the closed summary. A currently advertised option with no baseline entry is treated as changed. Baseline entries for options the provider no longer advertises are ignored.
- Hovering or focusing the closed task selector shows a compact tooltip containing every currently rendered selector option as provider-supplied `Name: Value` rows, including baseline-matching values. The tooltip contains no descriptions or inferred provider knowledge. Opening the selector shows compact option names and selected values; entering an option submenu shows that option's provider description and the provider descriptions of its selectable values when supplied.
- Kandev preserves optional ACP descriptions for both top-level config options and selectable values throughout the adapter, backend event, WebSocket, store, and selector pipeline. Missing descriptions produce no invented or hard-coded explanatory text.
- Task-detail boot data includes the last persisted model list, live config options, and provider-default baseline so the compact label is complete on the first render instead of repainting after WebSocket reconnection.
- The compact baseline-aware summary applies only to task chat input and task context model selectors. Shared selector uses such as agent-profile settings and utility configuration continue to list every selected value in the closed trigger.
- Dynamic `config_option_update` payloads replace the live option set while retaining the original persisted baseline. Provider-added, removed, reordered, or dependent options are compared by stable option ID and raw value.
- Legacy task sessions that have no stored baseline establish one from their first fully settled provider configuration after this feature is deployed. They do not attempt to reconstruct historical defaults.

## Scenarios

- **GIVEN** a provider advertises reasoning `Medium` by default and the selected agent profile requests `High`, **WHEN** the task session starts, **THEN** the closed task-chat selector shows `GPT-5.6-Sol / High`.
- **GIVEN** a task session starts with provider-default collaboration `Default`, reasoning `Medium`, and fast mode `Off`, **WHEN** no profile or runtime option changes, **THEN** the closed task-chat selector shows only the model name.
- **GIVEN** that baseline, **WHEN** reasoning changes to `Low`, **THEN** the closed task-chat selector shows `GPT-5.6-Sol / Low`.
- **GIVEN** reasoning is `Low` and fast mode is `On`, **WHEN** the selector is closed, **THEN** it shows `GPT-5.6-Sol / Low / On` in ACP option order without collapsing the changed values into a count.
- **GIVEN** a changed value is returned to its baseline, **WHEN** the selector rerenders, **THEN** that value is removed from the closed summary.
- **GIVEN** a task session has changed values, **WHEN** the backend restarts and recreates or resumes the ACP session, **THEN** the same baseline is loaded from task-session metadata and the closed summary still identifies the changes.
- **GIVEN** an ACP option or value supplies a description, **WHEN** the user enters that option's submenu, **THEN** Kandev shows the provider text. The closed trigger and top-level option list remain compact, and missing descriptions leave the description region absent.
- **GIVEN** a task selector has current model and config values, **WHEN** the user hovers or focuses its closed trigger, **THEN** a compact tooltip lists every selector option as `Name: Value` without provider descriptions, including values omitted from the changed-only trigger label.
- **GIVEN** a task session has persisted dynamic model configuration, **WHEN** the task-detail page is refreshed, **THEN** the first rendered selector label includes all changed values without waiting for a WebSocket event.
- **GIVEN** the same shared selector is rendered in agent-profile settings, **WHEN** it is closed, **THEN** it continues to list all selected values regardless of the task-session baseline.
- **GIVEN** a narrow touch viewport, **WHEN** the user taps the selector, **THEN** all current options and available descriptions remain reachable without hover or horizontal page scrolling.

## Data Model

- Task-session metadata contains a dedicated write-once ACP provider-default baseline keyed by config option ID with raw selected values.
- Task-session metadata also contains the latest complete ACP model selector state needed for task-detail boot hydration, including provider-supplied model and option metadata.
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

- Defining or inferring provider defaults beyond the task session's initial provider-advertised configuration.
- Hard-coded descriptions, aliases, importance rankings, or default values for individual ACP providers.
- Changing the closed-label behavior in agent-profile settings or other non-task selector surfaces.
- Adding support for ACP input control types that Kandev does not otherwise render.
