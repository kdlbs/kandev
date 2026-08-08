---
spec: docs/specs/agents/dynamic-provider-options.md
decision: docs/decisions/2026-08-07-model-aware-provider-capability-resolution.md
created: 2026-08-07
status: complete
---

# Implementation Plan: Dynamic Provider Model Options

## Overview

Add a model-aware, provider-neutral capability resolver for settings surfaces
that do not have a live ACP session. The resolver will use the existing host
utility agentctl process, apply a requested model through generic ACP session
model logic, and return the provider's complete normalized configuration-option
snapshot. Workflow session overrides and agent profile settings will consume
the same frontend hook and reconcile their draft option maps against the
returned snapshot.

The existing baseline agent capability endpoint remains responsible for the
model list, agent status, modes, commands, and default-session options. The
live chat selector remains session-authoritative. No database migration or
provider-specific frontend mapping is planned.

## Architecture

### Baseline versus resolved capabilities

`AvailableAgent.model_config` remains a fast baseline. It is used to render the
model picker immediately and to preserve a usable selector while a dependent
resolution is loading. It must not be treated as the final option set for a
selected model.

The new resolver accepts an agent name and a canonical context containing the
selected model, with optional mode and selected config options. It starts or
uses the existing host utility ACP boundary, performs a model change in the
probe session, waits for the provider's complete response or bounded config
update notification, and normalizes the result through
`internal/agentctl/acpcompat`. The returned option list is a full replacement
for the current context, including an intentional empty list.

The provider response is authoritative. The UI renders arbitrary ACP select
options and choices through `ModelConfigSelector`; it does not know that an
option came from OpenCode or that it is called reasoning effort.

### Resolution API and cache

Add `POST /api/v1/agent-models/:agentName/resolve`. The existing GET endpoint
continues to return the baseline dynamic model response. The POST body has a
required `model` and optional `mode`, `config_options`, and `refresh` fields.
The response returns the agent name, requested model, status, complete
normalized `config_options`, and a sanitized error when resolution is not
authoritative.

Host utility resolution is cached in memory by agent type plus canonicalized
context. Use a bounded TTL (five minutes unless existing runtime limits require
a shorter value), single-flight deduplication for identical concurrent keys,
and invalidation on baseline refresh. Cache failures only when they represent a
stable unsupported result; transient process/timeout failures should be
retryable and must not poison the cache indefinitely. The cache is runtime-only
and is safe to lose on restart.

### Settings behavior

The workflow rule card and profile form request resolution whenever their
selected model/context changes. The hook must provide baseline options,
resolved options, loading, error, and retry state while protecting against
out-of-order responses. A successful snapshot replaces, rather than merges
with, the previous option list. The editable `config_options` map drops keys
not present in a successful snapshot before the next save. A failed resolution
does not rewrite persisted data or silently remove saved values.

The shared selector stays generic. If a small loading/error affordance is
needed, add it as an optional status prop with localized copy. The chat path is
not changed because its live session already emits authoritative updates.

### Probe compatibility

The utility probe must use the same client capability metadata and ACP
compatibility normalization as live sessions. The model application path must
support typed config-option model changes and legacy model methods through the
existing `sessionmodel` abstraction. Provider-specific quirks remain in
`internal/agentctl/acpcompat`; no OpenCode branch belongs in settings or the
host utility cache.

### Mock provider contract

Make the mock ACP agent model-dependent for E2E. Its initial session should
advertise a small baseline, and applying one model should return no extra
options while applying another should return an effort-like select option.
This keeps tests provider-neutral while proving that settings resolve after a
model change instead of accidentally relying on static `AvailableAgent`
data.

## Backend

### Model-aware utility probe

Extend the utility probe request/result and ACP executor flow so a probe can
carry a model-resolution context. After `session/new`, apply the requested
model, capture the complete response config or a bounded configuration-update
notification, and pass it through the existing config-option conversion and
ACP dialect normalization helpers. Keep baseline probing behavior unchanged
when no model context is supplied.

Add focused coverage for response snapshots, notification fallback, unsupported
model application, provider normalization, and the existing client capability
metadata. Reuse `sessionmodel.ApplySDKWithConfigOptions` where its returned
config is authoritative instead of duplicating ACP request logic.

### Host utility resolution service

Add typed resolution inputs/results to the host utility package, a separate
runtime cache, canonical cache-key construction, TTL handling, single-flight
deduplication, refresh invalidation, and a bounded context timeout. Expose a
narrow public manager method for the settings controller. Do not persist
resolved capabilities or attach them to a task session.

### Settings HTTP contract

Add DTOs for the resolver request and response, controller validation, and the
POST route beside the existing dynamic-model route. Reuse the existing agent
availability and authorization checks and sanitize probe failures consistently
with the current settings APIs. Preserve the baseline GET response for existing
callers.

## Frontend

### API and hook

Extend `settings-api.ts` and `http-agents.ts` with the resolver request/response
contract. Extend `use-dynamic-models.ts` with a reusable model-resolution hook
or resolver returned by the existing hook. It must:

- key requests by agent, model, mode, and canonical option context;
- share in-flight and cached results across workflow/profile consumers;
- ignore stale responses after a newer model selection;
- replace the option snapshot atomically;
- reconcile draft option maps only after a successful response; and
- keep persisted values readable and retryable when discovery fails.

Add pure unit coverage for canonical context, reconciliation, stale responses,
cache reuse, refresh, and error recovery.

### Workflow editor

Wire `WorkflowSessionConfigRuleCard` to resolve options for the rule's selected
model. Keep the current static model list and rule serialization. On a
successful model-dependent response, render every returned select option and
save only the reconciled map. Preserve existing read-only and existing-rule
failure behavior.

### Agent profile editor

Wire `ProfileFormFields` to the same resolver using the profile's current
model. Keep model/mode/command discovery on the existing baseline hook. Ensure
changing a profile model refreshes dependent options, removes only
successfully-invalidated draft keys, and saves arbitrary resolved options.

All new loading, retry, and failure text must use i18next keys and be added to
the affected locale catalogs. Keep controls within the existing mobile
one-column layout and touch-target conventions.

## Tests

Backend unit/integration coverage must include:

- utility model application returning complete config snapshots;
- config-update notification fallback and bounded timeout;
- shared ACP dialect normalization;
- host utility cache key, TTL, refresh invalidation, and single-flight behavior;
- controller request validation and sanitized errors; and
- resolver HTTP response shape while the baseline GET contract remains stable.

Frontend unit/component coverage must include:

- resolver API request/response mapping;
- dynamic option replacement rather than stale merging;
- model-switch reconciliation and preservation on failure;
- latest-request-wins behavior;
- workflow rule rendering and serialization;
- profile form rendering and save behavior; and
- generic unknown option identifiers and choices.

## E2E Tests and Documentation

Update the mock ACP provider and existing settings specs so the selected model,
not the static boot snapshot, is what causes the dependent option to appear.
Cover both desktop flows:

- workflow session override selects a model and saves a dynamic option;
- OpenCode/ACP profile selects a model, saves a dynamic option, reloads, and
  displays it.

Cover the mobile workflow path with touch interaction, one-column layout, and
no document-level horizontal overflow. Keep the existing managed Playwright
runner and project split; do not run the mobile spec through desktop Chromium.

Update `docs/public/tasks-and-workflows.md` with the provider-neutral behavior,
model-dependent option discovery, and failure/retry expectations if the public
documentation coverage inventory requires the change. Keep the spec and ADR as
the detailed contract.

## Waves

Wave 1:

- [x] [task-01-model-aware-probe](task-01-model-aware-probe.md)

Wave 2 (depends on the probe contract):

- [x] [task-02-resolution-cache-api](task-02-resolution-cache-api.md)

Wave 3 (depends on the HTTP contract):

- [x] [task-03-resolution-hook](task-03-resolution-hook.md)

Wave 4 (depends on the hook):

- [x] [task-04-settings-integration](task-04-settings-integration.md)

Wave 5 (depends on the settings behavior):

- [x] [task-05-e2e-docs](task-05-e2e-docs.md)

The dependency chain is complete. The implementation uses a model-only request
from the current settings consumers, while the backend keeps the optional mode
and option context available for providers that need it later. Save
contributors remain blocked until a selected model has an authoritative
resolution and any stale draft options have been reconciled. Failed discovery
keeps the draft readable and exposes a localized retry action.

## Verification

Targeted backend checks:

```bash
cd apps/backend && go test ./internal/agentctl/server/utility ./internal/agentctl/acpcompat ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/settings/handlers
```

Targeted frontend checks:

```bash
cd apps && pnpm --filter @kandev/web exec vitest run hooks/domains/settings/use-dynamic-models.test.ts components/settings/profile-form-fields.test.tsx components/settings/workflow-session-config-editor.test.tsx components/settings/use-workflow-draft-contributor.test.ts components/settings/model-config-resolution-status.test.tsx components/settings/profile-model-config.test.ts components/model-config-selector.test.tsx --reporter=dot
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
```

Managed desktop and mobile E2E checks:

```bash
cd apps/web && pnpm e2e:run --host --project chromium tests/settings/agent-profile-acp.spec.ts tests/workflow/workflow-settings.spec.ts
cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/workflow/mobile-workflow-settings.spec.ts
```

Public docs checks when the public page changes:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Verification Results:

- Backend build passed; focused backend packages: 307 tests passed across five
  packages.
- Frontend focused unit tests: 42 tests passed across the seven listed files;
  typecheck, ESLint, i18n checks, and i18n ratchet passed.
- Managed E2E passed: 19 Chromium desktop tests and 3 mobile Chromium tests
  across profile and workflow settings. The tests also cover startup capability
  revalidation, so provider registration order and probe timing are not test
  assumptions.
- Public documentation checks were not required because this change updates
  the implementation plan and workflow spec, not `docs/public/**`.

## Review follow-up

The implementation completed the following review work:

- [x] Make the current settings resolver model-only. Do not send editable
  option values or mode as context from the profile and workflow consumers.
- [x] Remove probe mutations that are applied after the returned snapshot, or
  implement and test final-snapshot capture for a future multi-input contract.
- [x] Show localized resolution loading/failure state and provide retry in both
  settings surfaces.
- [x] Test controller error sanitization and authentication status mapping.
- [x] Reconcile only after a user model/context change so opening a saved
  profile does not create unsaved changes.
- [x] Complete the recommended cache, context-cancellation, conversion-helper,
  wrapper, mock-agent, and missing probe/host-utility test cleanup.
- [x] Remove E2E force-click workarounds after the selector no longer remounts
  for each option edit, then rerun desktop and mobile E2E.
- [x] Block profile and workflow saves during model-option reconciliation,
  require a snapshot after the final ACP option mutation, isolate forced
  refresh generations, use typed missing-agent errors, and keep zero-baseline
  resolution status visible.
- [x] Mark ACP inference agents as dynamic before their first cache snapshot,
  poll pending capability probes with a bounded retry, and make settings E2E
  fixtures independent of provider registration order.

## Risks and open questions

- Some ACP providers may advertise model changes only through delayed
  notifications. The probe needs a bounded wait that is long enough for normal
  providers but cannot hold a settings request indefinitely.
- Some providers may require a mode or prior option before returning the final
  snapshot. The API accepts those inputs now, but the first UI should only send
  values it can prove are current.
- Model resolution can be expensive for large model lists. The TTL,
  single-flight cache, and refresh semantics should be measured with the mock
  provider before broadening the UI to prefetch.
- If a provider cannot produce a complete post-model snapshot, the UI must
  remain safe and understandable without implying that the baseline is
  authoritative for the selected model.
