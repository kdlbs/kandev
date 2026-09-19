---
status: accepted
date: 2026-08-07
area: backend, frontend, protocol
---

# Model-aware provider capability resolution

## Context

ACP providers can change their selectable configuration options after a model
changes. OpenCode is an example: the live task selector receives a
`reasoning_effort` option for some selected models, while the settings pages
currently read one agent-level capability snapshot taken before a model is
selected. The workflow session override editor and the OpenCode agent profile
editor therefore cannot show model-dependent options. The shared selector is
already generic; the missing capability is a model-aware source of truth.

The same problem will occur for future providers and for future option types,
so provider names and option identifiers must not be embedded in the settings
UI.

## Decision

Add a sessionless, model-aware capability resolver behind the existing host
utility ACP boundary.

- Keep the existing agent-level capability discovery as the fast baseline for
  agent status, model lists, modes, commands, and options available for the
  provider's default session.
- When a settings surface selects a model, resolve that model in a short-lived
  ACP probe session. Apply the model through the generic ACP/session-model
  machinery, capture the provider's complete resulting config-option snapshot,
  and normalize it through the shared `internal/agentctl/acpcompat` boundary.
- Treat the provider's returned snapshot as authoritative for that resolution
  context. The frontend renders arbitrary ACP `select` options and choices; it
  does not map OpenCode, Grok, or any other provider by name.
- Expose resolution through an explicit HTTP endpoint with a request context
  containing the model and future-dependent inputs such as mode and selected
  config options. Keep the existing baseline models endpoint unchanged.
- Cache successful and unsupported resolutions in memory for a short bounded
  TTL, keyed by agent type and the canonical resolution context. Deduplicate
  concurrent requests and invalidate the cache when the provider capability
  baseline is refreshed. Do not persist probe results in the database.
- A resolution response replaces the current option snapshot for the selected
  context. If resolution fails, settings retain the existing draft/persisted
  values and do not invent new controls; the user can retry or save unchanged
  data.

The initial settings implementation uses the selected model as the only
context input. The API and cache key are shaped so that mode and selected
options can become dependencies without another provider-specific design.

## Consequences

Positive consequences:

- Chat, workflow settings, and agent profile settings can share one provider
  capability contract.
- A provider can add or remove options dynamically without a frontend release.
- The expensive part of discovery is bounded by caching and single-flight
  deduplication.
- Provider compatibility quirks remain in the ACP compatibility package rather
  than leaking into React components.

Costs and risks:

- Selecting a model in settings may require a short asynchronous probe before
  dependent controls appear.
- A provider that cannot return a complete post-model snapshot will show a
  degraded discovery state rather than being guessed at by the UI.
- The backend must carefully bound probe time and process resources because a
  settings page can request several models in succession.
- A model change may make previously selected option identifiers invalid; the
  editable draft must reconcile them only after a successful authoritative
  snapshot, while preserving persisted data when discovery fails.

## Alternatives considered

### Keep one static capability snapshot per agent

This is simple and preserves the current API, but it cannot represent options
whose choices depend on the selected model. It was the behavior observed on
both settings surfaces, so it is insufficient.

### Probe every model during initial discovery

This would make the UI immediately complete, but multiplies provider process
startup and ACP calls by the number of models, delays the baseline response,
and still needs a context model for mode- or option-dependent capabilities.

### Hard-code provider-specific option maps in the frontend

This would make OpenCode work quickly but duplicates provider behavior, goes
stale as providers change, and violates the generic ACP capability boundary.

### Reuse the active task session for settings discovery

Settings pages may be opened without a task, and mutating an active task to
discover options would create user-visible side effects. A sessionless probe
keeps discovery isolated.

## Follow-up

The behavioral contract and implementation work are specified in
[`Dynamic Provider Model Options`](../specs/agents/requirements/dynamic-provider-options.md).
