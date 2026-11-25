---
status: active
system: agents
created: 2026-08-07
updated: 2026-11-04
owners:
  - kandev
---
# Dynamic Provider Model Options Requirements

## Overview

Provider capabilities are not always properties of an agent family alone.
Some ACP providers change the available configuration options when a model is
selected. OpenCode currently demonstrates this split: the task chat selector
receives model-dependent options such as reasoning effort from the live
session, while the workflow session override editor and the OpenCode agent
profile editor only have the agent-level capability snapshot. The settings
surfaces therefore show the model but omit valid dependent options.

Kandev needs one provider-neutral contract that works for OpenCode and future
providers without maintaining a list of provider-specific option names.

The grouped/composed catalog extension also covers live sessions.
It preserves provider catalog structure and exact model identity across all existing selection surfaces.
Agents owns that meaning. The shared UI renders the normalized data.

## Requirements

### REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001: Dynamic Provider Model Options

**Intent:** Settings users can discover the selected model's provider options through one shared contract.

#### Acceptance criteria

- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001.1:** The existing agent-level capability response remains the baseline for agent availability, model lists, modes, commands, and options available before a model is selected.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001.2:** After a user selects a model in the workflow session override editor or an agent profile editor, Kandev requests a model-aware capability snapshot from the provider through a sessionless ACP probe.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001.3:** In an agent profile editor, the open model selector stays open while Kandev resolves the selected model. The area below the model list shows a localized loading spinner instead of stale dependent options.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001.4:** The probe applies the requested model using generic ACP session-model logic, captures the complete post-model `config_options` response or update, and applies the existing ACP compatibility normalization.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001.5:** The response is a complete snapshot for that resolution context. It may contain zero, one, or many arbitrary ACP `select` options and their choices. The UI does not recognize provider-specific option identifiers.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001.6:** The settings pages share the resolver and its short-lived in-memory cache. Live chat remains session-authoritative and never consumes a settings probe as its live state.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001.7:** The initial UI sends the model as the dependency context. The contract also accepts mode and currently selected options so providers with additional dependencies can be supported without changing the endpoint shape.

### REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002: Structured model catalogs

**Intent:** Users can navigate provider groups without losing exact selectable model values.

#### Acceptance criteria

- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.1:** Settings and live catalogs shall preserve native group identities, labels, ordering, leaf values, and supplied descriptions.
  Flat catalogs shall retain their existing selection behavior.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.2:** Subgroups shall reflect supplied or verified catalog relationships.
  The selector shall support search, Back navigation, selected-state visibility, and distinct paths for repeated labels.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.3:** Config updates and existing live-session snapshots shall retain the structured catalog through reload, resume, and boot hydration.
  Settings discovery shall retain its existing transient cache lifetime.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.4:** The shared catalog shall accept only bounded presentation data.
  Invalid richer metadata shall retain the ordinary exact-model selection path without executable metadata or inferred billing information.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.5:** An ordinary group containing one model shall show one model row without a redundant heading.
  Descriptions, order, distinct occurrences, and useful path context shall remain available.
  Multi-choice groups and verified compositions shall retain their presentation, including during search.

### REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003: Exact composed-model selection

**Intent:** Users can select a verified combination of model dimensions without searching an expanded list of pairings.

#### Acceptance criteria

- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.1:** A complete dimension selection shall resolve to exactly one currently advertised model value.
  Missing, invalid, or ambiguous combinations shall not apply or silently select another pairing.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.2:** Thinking, Speed, and other provider configuration options shall use the selected model's complete authoritative option snapshot.
  Model names and IDs shall not determine their effective values.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.3:** Devin Fusion shall expose Lead and Sidekick only through a verified relationship source or narrowly qualified compatibility mapping.
  Unrecognized entries shall remain available in the ordinary catalog.
  Ordinary fallback alone shall not satisfy verified Fusion composition acceptance.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.4:** A second synthetic provider with different dimension names shall use the same renderer and combination logic.
  The renderer shall not recognize agent names or vendor-specific dimension identifiers.

### REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004: Shared configuration flow

**Intent:** Users can configure the same advertised model on desktop and phone through each existing owning surface.

#### Acceptance criteria

- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.1:** Profile drafts, workflow overrides, and live sessions shall share catalog navigation and composition behavior.
  Apply shall update the owning draft or live session. Existing settings Save shall retain persistence authority.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.2:** Changing dimensions shall preserve valid choices and visibly require replacements for invalid choices.
  Loading, failure, retry, cancellation, viewport changes, and stale responses shall preserve the correct draft and context.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.3:** Live application shall report confirmed provider state after each accepted mutation.
  A later option failure shall expose partial application and retry without claiming atomic success or resetting the conversation.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.4:** Reload, resume, reset, and historical turns shall retain existing configuration attribution and baseline semantics.
  Live changes shall not rewrite profile defaults. Session permission mode shall remain separate from model dimensions.
- **AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.5:** Desktop and phone shall offer equivalent complete configuration outcomes through localized, accessible controls.
  Phone navigation shall use one sheet with internal Back navigation, retained draft state, and one vertical scroll region.
  Controls shall meet 44px touch targets and clear safe areas without horizontal document overflow.

## Migrated source detail

## Why

Provider capabilities are not always properties of an agent family alone.
Some ACP providers change the available configuration options when a model is
selected. OpenCode currently demonstrates this split: the task chat selector
receives model-dependent options such as reasoning effort from the live
session, while the workflow session override editor and the OpenCode agent
profile editor only have the agent-level capability snapshot. The settings
surfaces therefore show the model but omit valid dependent options.

Kandev needs one provider-neutral contract that works for OpenCode and future
providers without maintaining a list of provider-specific option names.

## What

Kandev resolves model-dependent ACP configuration options on demand for the
settings surfaces that do not have a live task session:

- The existing agent-level capability response remains the baseline for agent
  availability, model lists, modes, commands, and options available before a
  model is selected.
- After a user selects a model in the workflow session override editor or an
  agent profile editor, Kandev requests a model-aware capability snapshot from
  the provider through a sessionless ACP probe.
- In an agent profile editor, the open model selector stays open while Kandev
  resolves the selected model. The area below the model list shows a localized
  loading spinner instead of stale dependent options.
- The probe applies the requested model using generic ACP session-model logic,
  captures the complete post-model `config_options` response or update, and
  applies the existing ACP compatibility normalization.
- The response is a complete snapshot for that resolution context. It may
  contain zero, one, or many arbitrary ACP `select` options and their choices.
  The UI does not recognize provider-specific option identifiers.
- The settings pages share the resolver and its short-lived in-memory cache.
  The live chat selector remains session-authoritative and never consumes a
  settings probe as its live state.
- The initial UI sends the model as the dependency context. The contract also
  accepts mode and currently selected options so providers with additional
  dependencies can be supported without changing the endpoint shape.

## Scenarios

### Model adds an option in workflow settings

Given an OpenCode agent family whose default model advertises only model and
mode, when a workflow author selects a model that supports reasoning effort,
then the workflow rule editor loads and displays the provider-supplied
reasoning-effort select and its choices. The option appears without an
OpenCode-specific frontend mapping.

### Model adds an option in an agent profile

Given the OpenCode agent profile settings page, when the profile model changes
to a model with additional selectable options, then the profile editor uses
the same resolved snapshot as workflow settings and allows those options to be
selected and saved.

### Agent profile shows model-option progress

Given an open agent profile model selector, when the author selects a model,
then option resolution starts. The selector stays open and shows a loading
spinner in the selected model row and below the model list. The selected row
does not show its check icon while it is resolving. The selector hides stale
dependent controls until the new option snapshot arrives.

Given that the option request is complete, when Kandev shows the resolved
snapshot or an error, then the selected row restores its check icon and the
loading spinners are not visible. The existing resolved controls or retryable
error state are available.

### Model removes an option

Given a draft with a selected option for the current model, when the author
selects a model whose authoritative snapshot does not advertise that option,
then the editor removes that option from the editable configuration before a
subsequent save. The model remains selected and the provider's current options
are shown.

### Provider adds arbitrary future options

Given a provider returns a new ACP `select` option with an identifier and
choices unknown to Kandev, when the model is resolved, then the shared selector
renders that option using its generic label, value, and choice data. No code
change is required for the new identifier.

### Resolution fails

Given a model selection whose capability probe times out, is unavailable, or
returns no complete post-model option snapshot, when the settings page receives
the failure, then it keeps already persisted and currently selected values,
does not show stale dependent controls or offer unverified new options, shows a
retryable discovery error, and allows a model-only save when the rest of the
form is valid.

### Responses arrive out of order

Given an author selects model A and then quickly selects model B, when the
resolution for A completes after the resolution for B, then the editor ignores
A's result and only displays options for B.

### Shared resolution work

Given workflow and profile settings request the same agent, model, and context
within the cache TTL, when both pages resolve capabilities, then Kandev
deduplicates the backend work and returns the same snapshot. A refresh request
or baseline capability refresh invalidates the cached result.

### Mobile settings

Given a phone viewport, when the author selects a model and configures the
resolved options in either settings flow, then controls remain in a one-column
touch-friendly layout, use the existing settings scroll owner, and do not
create document-level horizontal overflow.

Given an open agent profile model selector on a phone, when model options load,
then the same loading indicators are visible and remain inside the selector.

## Data model

No durable database schema is added.

The existing profile and workflow data continue to store the selected model
and `config_options` map. A successful resolution reconciles the editable map
against the returned option identifiers before save. A failed resolution does
not rewrite persisted profile or workflow data.

The runtime-only resolution context is:

```json
{
  "agent_name": "opencode-acp",
  "model": "opencode-go/glm-5.2",
  "mode": "build",
  "config_options": {
    "reasoning_effort": "high"
  }
}
```

The initial frontend sends only `model`; `mode` and `config_options` are
optional inputs for providers whose capabilities depend on more than the
model. Cache keys canonicalize map ordering and include every supplied
context field.

## API surface

The existing baseline endpoint remains unchanged:

`GET /api/v1/agent-models/:agentName`

Add a model-aware resolver endpoint:

`POST /api/v1/agent-models/:agentName/resolve`

Request body:

```json
{
  "model": "opencode-go/glm-5.2",
  "mode": "build",
  "config_options": {
    "reasoning_effort": "high"
  },
  "refresh": false
}
```

`model` is required for the first version. `mode`, `config_options`, and
`refresh` are optional.

Successful response:

```json
{
  "agent_name": "opencode-acp",
  "model": "opencode-go/glm-5.2",
  "status": "ok",
  "config_options": [
    {
      "id": "reasoning_effort",
      "name": "Reasoning effort",
      "type": "select",
      "current_value": "high",
      "options": ["low", "medium", "high", "max"]
    }
  ],
  "error": null
}
```

The exact wire field names follow the existing `ConfigOptionEntry` DTO shape;
the important contract is that `config_options` is a complete normalized
snapshot, not a delta. Errors use the existing agent-settings error/status
conventions and never expose provider secrets or raw process output.

## State machine

Each settings surface follows this state machine for the selected resolution
context:

| State     | Event                            | Result                                                               |
| --------- | -------------------------------- | -------------------------------------------------------------------- |
| baseline  | model selected                   | Show baseline options and start one resolver request.                |
| resolving | matching response                | Replace options with the complete snapshot and reconcile the draft.  |
| resolving | stale response                   | Ignore it and keep the newer model's state.                          |
| resolving | timeout/error                    | Keep existing values, show retryable error, and do not add controls. |
| resolved  | same context requested           | Reuse the runtime cache.                                             |
| resolved  | refresh or baseline invalidation | Request a fresh snapshot.                                            |

An option snapshot is replaced atomically. The frontend must not merge an old
model's choices with a new model's choices.

## Permissions

The resolver uses the same authenticated agent-settings route and workspace
authorization as the existing dynamic-model endpoint. It does not create a
task session, change an agent profile, or mutate provider configuration.

## Failure modes

- If the agent is unavailable, the API returns the existing unavailable/error
  status and the UI keeps the baseline or last-known values.
- If the probe cannot apply the requested model, no partial option snapshot is
  returned as authoritative.
- If the provider applies the model but supplies neither a complete response
  nor a bounded configuration-update notification, the result is degraded and
  the UI does not guess at model-specific options.
- Probe execution has a bounded timeout. Concurrent identical requests share
  one in-flight resolution; distinct contexts do not block indefinitely behind
  one another.
- A stale browser response cannot overwrite a newer model selection.
- If a previously saved option is no longer advertised after a successful
  resolution, it is removed from the next editable save for that model. If
  discovery fails, it remains in persisted data and is not silently deleted.
- Provider errors are sanitized before reaching the HTTP response or UI.

## Persistence guarantees

- No settings-probe capability snapshot is persisted in SQLite or workflow JSON.
  Existing live-session selector snapshots retain their current persistence owner, including the structured catalog under requirement 002.
- Existing profile and workflow `config_options` persistence remains the source
  of truth for user selections.
- Runtime cache entries may disappear on process restart without affecting
  saved settings.
- A successful settings save stores the selected model and the reconciled
  option map. A failed discovery never rewrites saved data by itself.

## Out of scope

- Hard-coding OpenCode, Grok, or any provider's option names in React code.
- Replacing live-session configuration authority. Requirement 002 adds catalog data to the existing projection.
- Resolving every model eagerly during initial agent discovery.
- Persisting provider capability snapshots or introducing a database cache.
- Supporting non-ACP provider-specific configuration controls through this
  contract.
- Changing the semantics of workflow runtime application or profile loading.

## Success criteria

- The workflow session override editor displays model-dependent ACP select
  options returned by a provider, including OpenCode reasoning effort.
- The OpenCode agent profile settings page displays and saves the same resolved
  options.
- A newly added arbitrary select option renders without provider-specific UI
  code.
- A model change never displays options from a previous model after an
  out-of-order response.
- Resolver failures preserve saved data and provide a retryable, localized
  status.
- The agent profile model selector shows localized progress below the model
  list while model-dependent options load.
- Desktop and mobile E2E coverage verifies workflow and profile behavior, with
  touch sizing and overflow assertions for the mobile workflow editor.

## Grouped catalog delivery

- [Grouped/composed model design](../system-design/grouped-model-catalog.md)
- [Combined Devin delivery plan for issues #5023 and #5026](../../../plans/first-draft-profile-discovery/plan.md)
- [Selector repair for issue #5053](../../../plans/model-catalog-selector-repair/plan.md)
- [Configuration summary and attribution contract](../../ui/requirements/acp-model-configuration-summary.md)
