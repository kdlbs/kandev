---
status: draft
system: agents
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
created: 2026-08-23
owners:
  - kandev
---
# No Silent Model Fallback System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

**Status**: implemented (executor-authority amendment 2026-08-15)
**Date**: 2026-08-04
**Slug**: `no-silent-model-fallback`

## Problem

Agents such as OMP are configured with models from multiple providers (e.g.
Claude + GPT). When a provider's login/auth expires (e.g. Claude), its models
become unavailable. Today Kandev silently compensates in several places:

1. **Session start (agentctl runtime)** — `SessionManager.InitializeSession`
   applies the profile's start model via ACP `SetModel` as *best-effort*:
   on failure it logs a warning and continues the session on the provider
   default model. `SetModel` itself fails fast when the model is not in the
   agent's advertised model list (`validateAvailableModel`), so the failure
   is known but ignored.
2. **Profile reconciler (boot)** — `healProfile` overwrites a profile's
   start model with the probe's `CurrentModelID` when the configured model
   is no longer advertised. The user's explicit model choice is silently
   replaced.
3. **Office post-start fallback** — when a launched run fails mid-session
   with a fallback-allowed error (`auth_required`, `model_unavailable`,
   …), `HandlePostStartFailure` silently re-dispatches the run to the next
   provider in the workspace routing order.
4. **Frontend model lists** — model pickers have no concept of an
   unavailable ("gone") model; `clearStaleActiveModel` silently drops an
   active model that disappeared from the ACP list, so the session continues
   on whatever the agent picks.

**Bottom line: the host and executor can advertise different model catalogs.
Kandev must keep the task operational and explain the effective model.**

## Goals

- A configured model that the executor does not advertise is never applied.
  The agent continues with its current or default model.
- Every default or explicit fallback creates a persisted warning in task chat.
  The warning survives reload and shows the effective model when known.
- A Claude model that is valid for the user's account can start in a cold,
  isolated executor even when the initial Claude ACP model list omits it.
- "Gone" models render **greyed out and unselectable** in every model
  picker, while remaining visible so the user can see what was configured.
- Agent profiles **keep** their previously configured start model even when
  gone (shown red + unselectable in the editor) instead of having it
  auto-healed.
- Profiles remain selectable when the host probe does not advertise the saved
  model. The host probe is an editing hint, not an executor launch gate.
- A new optional per-profile **agent fallback model** allows an explicit,
  single-model automatic switch when the start model becomes unavailable at
  session start. Kandev applies it only when the executor advertises it.
  Office post-start routing is unchanged and remains governed
  by the workspace routing configuration (see ADR
  `2026-08-08-provider-neutral-agent-error-recovery.md`): the profile's
  model policy is the owner of the session-start decision, not of office
  post-start provider fallback.
- A new explicit per-profile toggle **"Fallback automatically to next
  model"** restores the legacy automatic-fallback behavior (session start
  best-effort + office routing re-dispatch). A failed apply continues with the
  agent default and creates a persisted warning. Enabling the toggle disables the
  optional fallback-model controls without clearing their saved value (the two
  are mutually exclusive opt-ins).
- Agent-profile editors group both fallback choices in a collapsed **Fallback
  settings** disclosure. Expanding it shows the two choices side by side on
  desktop and as one vertical flow on phone-sized screens. Each choice keeps a
  short visible explanation and adds contextual help that works by hover or
  keyboard focus with a fine pointer and by tap on a coarse pointer.

## Non-Goals

- Changing the workspace provider-routing configuration model (tiers,
  provider order, execution profiles) itself.
- Provider-level auth flows (login/refresh UI) — only the *consequence* of
  auth expiry (unavailable models) is addressed.
- Requiring equal model catalogs on the Kandev host and executor.
- Requiring portable configuration copying before a task can start.
- Mid-turn model switching on the live ACP session (retry the current turn
  on a different model) — fallback applies to the next attempt/launch, not
  to an in-flight turn.

## Definitions

- **Available models**: the model IDs an agent currently advertises —
  `hostutility.AgentCapabilities.Models` (probe cache, surfaced via
  `GET /api/v1/agents/available` and `GET /api/v1/agent-models/:agentName`)
  or the ACP session's `models_updated` list.
- **Gone model**: a model ID that is configured (profile start model, active
  session model, fallback model) but absent from the currently advertised
  list. Deterministic on the frontend: `configured ∉ advertised`.
- **Default-on-mismatch mode**: profile has `auto_fallback = false` and no
  `fallback_model`. If the executor omits the start model, Kandev skips model
  selection and the agent uses its default. Kandev persists a warning.
- **Fallback-model mode**: profile has `auto_fallback = false` and a
  non-empty `fallback_model`. The only permitted automatic switch is to
  that single model when the executor advertises it. Otherwise, the agent uses
  its default and Kandev persists a warning.
- **Auto-fallback mode**: profile has `auto_fallback = true`. Legacy
  behavior (session-start best-effort; office routing re-dispatch to next
  candidate). Every session-start deviation creates a warning.
  `fallback_model` is ignored.
- **Cold isolated executor**: a non-standalone executor that does not share the
  host probe's Claude settings or model cache. Docker, remote Docker, Sprites,
  and SSH executors can have this property.

## Behavior Matrix

Per agent profile, one of three modes (precedence: `auto_fallback` wins
over `fallback_model`):

| Scenario | Default-on-mismatch | Fallback-model | Auto-fallback |
|---|---|---|---|
| Session start, start model not advertised | Do not call `SetModel`. Continue on the agent default and persist a warning. | If the fallback is advertised, apply it and persist a warning. Otherwise, use the agent default and persist a warning. | Do not call `SetModel`. Continue on the agent default and persist a warning. |
| Session start, advertised start model fails to apply | Fail explicitly. | Fail explicitly. | Continue on the agent default and persist a warning. |
| Session start, model selection unsupported | Continue on the agent default and persist a warning. | Same | Same |
| Mid-session model/auth failure (office run, post-start) | Unchanged ADR behavior: office re-dispatches via the workspace routing chain (`routingerr.Decide(ContextOffice)`; availability codes → `DecisionFallback`). The profile's model policy does **not** gate office fallback — the workspace routing configuration is the office authorization owner. | Same as default-on-mismatch: `fallback_model` is a session-start policy, not an office routing input. | Legacy: re-dispatch to next candidate in the provider order (unchanged). |
| Boot reconciliation | Never overwrite a gone start model (keep it; UI shows it red). Same for a gone `fallback_model`. | Same | Same (reconciler is mode-independent). |
| New-task / new-agent profile picker | Profile selectable with a host-catalog warning. | Profile selectable with a host-catalog warning. | Profile selectable with a host-catalog warning. |
| Model picker (profile editor, session toolbar) | Gone models greyed out, unselectable, visible. | Same. | Same. |

`SetModel` failures that mean "this agent does not support model selection"
(JSON-RPC `-32601`, `sessionmodel.MethodNone` / `IsMethodNotFound`) do not stop
the launch. The agent uses its default and Kandev persists a warning.

The executor ACP catalog is authoritative for launch.
The host probe remains an editing hint and does not block profile selection.

## Backend Changes

### 1. Agent profile schema (new columns)

`agent_profiles` gains:

- `fallback_model TEXT NOT NULL DEFAULT ''` — optional single fallback model
  ID (ACP model ID, same vocabulary as `model`).
- `auto_fallback INTEGER NOT NULL DEFAULT 0` — explicit opt-in to legacy
  automatic fallback.

Migration follows the existing `r.migrate.Apply("agent_profiles.<col>",
"ALTER TABLE ... ADD COLUMN ...")` pattern in
`apps/backend/internal/agent/settings/store/sqlite.go`. Wire through:

- `models.AgentProfile` (`internal/agent/settings/models/models.go`)
- store scan/insert paths (`settings/store/sqlite.go`)
- `dto.AgentProfileDTO` + create/update request structs
  (`settings/dto/dto.go`, `settings/controller/profile_crud.go`)
- frontend `AgentProfile` type, `normalizeAgentProfile` /
  `toAgentProfilePayload` (`apps/web/lib/types/agent-profile.ts`,
  `apps/web/lib/api/domains/agent-profile-normalize.ts`)

Validation: `fallback_model` may be set independently; when `auto_fallback`
is enabled the UI hides/disables the field and the backend ignores
`fallback_model` at runtime (precedence rule). No cross-field rejection —
saving both is allowed; runtime precedence is `auto_fallback`.

### 2. Reconciler stops healing gone models

`healProfile` in `internal/agent/settings/controller/reconciler.go` currently
replaces a gone `p.Model` with `caps.CurrentModelID`. Change: keep the
user-configured model when it is gone (log an info line; do not overwrite).
The `p.Model == ""` seed-default branch is unchanged. Apply the same
keep-when-gone rule to `fallback_model` (no auto-heal; UI surfaces it).
Mode healing is unchanged (modes are not part of this feature).

### 3. Session start: executor-authoritative model application

In `internal/agent/runtime/lifecycle/session.go`, the start-model policy uses
the executor session state after `InitializeSession`.

The policy returns a typed model decision with these values:

- Requested model.
- Advertised fallback model, when one is applied.
- Effective model, when the agent reports it.
- Outcome and warning reason.
- Whether Kandev called `SetModel`.

The policy applies this order:

1. If the start model is advertised, call `SetModel(start_model)`.
2. If the start model is absent and the explicit fallback is advertised, call `SetModel(fallback_model)`.
3. Otherwise, do not call `SetModel` and continue with the agent default.

An empty advertised list follows step 3.
It does not authorize a speculative `SetModel` request.

If an advertised start or fallback model fails to apply, fail the launch.
Transport and protocol errors remain explicit.

If `auto_fallback` is enabled, an apply error remains best-effort.
The launch continues with the agent default and persists a warning.

If `sessionmodel.IsMethodNotFound(err)` is true, continue with the agent default.
Persist a warning because the profile requested a model that Kandev could not apply.

The start-model policy owns every model-selection attempt.
Later profile or configuration layers must not repeat a handled attempt.

The same policy applies after a context reset.
A reset cannot send a model that the new executor session does not advertise.

### 3a. Persisted model-selection warning

If Kandev does not apply the requested model, it emits a provider-neutral
`session_model_selection_warning` event.

The orchestrator persists one `status` message for the session-start decision.
The message uses `metadata.variant = "warning"` and
`metadata.kind = "model_selection_warning"`.

The metadata contains the reason, requested model, effective model, fallback
model, agent ID, executor type, and executor profile ID.

The message content is structured and localized by the frontend status renderer.
If the agent reports no effective model, the UI shows
`provider default, model not reported`.

The warning includes this remediation guidance:

- Inspect the executor credentials.
- Inspect copied agent configuration.
- Inspect the agent version in the executor.

Message persistence is best-effort and does not stop the agent launch.
Persistence errors are logged without model data from configuration files.

The persistence path uses an idempotency key for the session-start decision.
Reconnect, replay, and browser reload do not create duplicate warning messages.

The existing ephemeral `session.model_fallback` signal becomes a compatibility
projection of the provider-neutral decision event.

### 3b. Claude ACP model exposure in cold executors

The host probe can advertise a Claude model from its warm settings or model
cache. A cold isolated executor can start with only the bridge's baseline model
list. This difference does not prove that the configured model is unavailable
to the user's account.

Before the initial Claude ACP process starts in a non-standalone executor, the
lifecycle exposes the effective start model through Claude's documented launch
environment. The effective model is the request model override when present.
Otherwise, it is the profile start model. The lifecycle exposes an explicit
profile fallback as a selectable custom model when `auto_fallback` is false.
During workspace recovery, a persisted session runtime model is treated as the
selected model for launch-environment derivation and takes precedence over the
profile start model.

Provider launch environment values use fill-missing precedence. Request,
executor, and profile environment values remain authoritative. The lifecycle
does not replace an existing `ANTHROPIC_MODEL` or
`ANTHROPIC_CUSTOM_MODEL_OPTION` value.

This behavior is an optional agent capability. Other agents receive no Claude
environment values. A standalone Claude process keeps the current host behavior
because it shares the settings and cache used by the probe.

SSH keeps its credential allowlist unchanged, but projects these non-secret
Claude model values into the remote agent controller so its child process sees
the same launch environment. Recovery fails before environment construction if
the execution profile cannot be resolved, rather than silently dropping its
model and environment values.

Kandev does not relax model validation after the ACP session starts.
If Claude still omits the model, the executor-authoritative policy uses the default and warns.

Kandev does not copy the bridge private cache.
The separate portable-configuration feature can copy `settings.json` after explicit user selection.

### 4. Office post-start failure gating (unchanged, ADR-governed)

`HandlePostStartFailure` in
`internal/office/scheduler/routing_lifecycle.go` is **not** modified by this
feature. Office post-start fallback authorization stays with the workspace
routing configuration and provider chain
(`routingerr.Decide(ContextOffice)`), per ADR
`2026-08-08-provider-neutral-agent-error-recovery.md`. Rationale: the
feature's model policy is owned by the session-start decision
(`execution.AgentProfileID`); the office post-start path sees the stable
office identity (`run.AgentProfileID`), and the two can disagree. Reading
`agent.FallbackModel` / `agent.AutoFallback` there would create a second,
conflicting policy owner. Office runs still get the session-start guarantee.
Kandev does not send an unadvertised model, and it persists a warning.
Mid-session Office failures keep the workspace-configured routing behavior.

Terminal office failures (for example after max attempts) surface an
actionable "provider/model unavailable — change the model" hint: map
`model_unavailable` / `auth_required` / `missing_credentials` /
`subscription_required` codes onto the failure message via
`routingerr.ModelUnavailableMessage` when composing it.

### 5. Error message mapping

Add a small helper (backend) that renders an actionable message for
model/auth failure codes, e.g. `"Model unavailable: the configured model
<id> is no longer available. Change the model in the agent profile."` Used
by the session-start failure and the office terminal-failure path so both
chat and run detail show the request-to-change-model copy. The frontend may
additionally map the same codes to a banner (see frontend).

## Frontend Changes

### 6. Model pickers: disabled ("gone") support

`apps/web/components/model-config-selector.tsx`:

- `ModelSelectorOption` gains `disabled?: boolean` and
  `disabledReason?: string`.
- `ModelRow` renders disabled options greyed (`opacity-40`,
  `cursor-not-allowed`), with the reason in a tooltip, and `onSelect` is
  guarded (`!option.disabled`). Follow the existing disabled pattern from
  `apps/web/components/combobox.tsx` (separate visual treatment + tooltip).

Session toolbar picker (`apps/web/components/task/model-selector.tsx`):

- `clearStaleActiveModel` in `apps/web/lib/ws/handlers/session-models.ts`
  stops clearing the active model when it disappears from the ACP list.
  Instead the active model is **kept and marked gone**: the picker shows it
  greyed out with a reason ("model no longer available — select a new
  model"), so the user is explicitly asked to change it.
- `buildModelOptions` / `resolveAvailableModels` mark configured-but-absent
  models (profile model, active model) as `disabled`.

### 7. Profile editor rows

`apps/web/components/settings/profile-form-fields.tsx` (`CapabilitiesRow` /
`ModelPicker`):

- **Start model**: when `profile.model` is not in the current model list,
  keep it as the current value but render it red
  (`text-destructive`) + disabled with a reason ("no longer available").
- **Agent fallback row** (new, under Start model): optional select of the
  same model list bound to `profile.fallback_model`. It remains visible but is
  disabled when `auto_fallback` is ON, preserving the configured value while
  making the mutual exclusion understandable. It uses the same
  gone-red/disabled treatment if the fallback model itself is gone.
- **"Fallback automatically to next model" toggle** (new row): a switch
  bound to `profile.auto_fallback`. When ON, the fallback-model controls are
  disabled. Helper text explains the semantics.
- **Fallback settings disclosure**: both choices sit inside a semantic
  collapsible section that is closed on initial render. Its closed header
  summarizes the effective mode (executor default, automatic, or the configured explicit
  fallback) so saved state and hidden dirty state remain discoverable. The
  trigger is keyboard-operable, reports expanded state, and has a touch target
  of at least 44px.
- **Responsive layout and help**: the expanded choices use two equal columns at
  the desktop breakpoint and one column below it. Each label has an info-icon
  button. Fine pointers expose localized help through a tooltip on hover and
  focus; coarse pointers open the same help in an inset bottom drawer. The
  visible helper sentence remains in the option card, so the icon is
  supplementary rather than the only explanation.
- The lighter editor `apps/web/components/agent/cli-profile-editor.tsx`
  (`ModelModeFields`) gets the same disclosure, layout, help, and mutual
  exclusion behavior for parity.

All new copy is externalized via `t()` into the `settings` i18n namespace
  (`apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/settings.json`) — the i18n ratchet
judges added lines even in unmigrated files.

### 8. Profile picker warnings (new-task / new-agent)

`apps/web/lib/state/slices/settings/types.ts` — `AgentProfileOption` gains
`model`, `fallbackModel`, `autoFallback` (populated in
`toAgentProfileOption`).

`apps/web/components/task-create-dialog-options.tsx`
(`useAgentProfileOptions`) can compute a host-catalog difference.
This difference is advisory only.

- Every profile remains selectable.
- A missing host model shows one amber warning icon beside the profile name.
- On fine pointers, hovering or focusing the warning icon reveals the full
  localized advisory that the executor decides availability at launch. On
  coarse pointers, tapping the icon opens the same advisory in a drawer. The
  advisory is not shown as an always-visible secondary row in the option list.
- The warning does not promise that an explicit fallback will be available.
- The warning does not change the saved profile model.

`apps/web/app/office/setup/agent-profile-setup-controls.tsx`
(`useSelectableProfileOptions`) uses the same advisory behavior.

The persisted `model_selection_warning` chat message renders through
`apps/web/components/task/chat/messages/status-message.tsx`.
The renderer uses localized copy and structured metadata.

The message appears in the normal chat flow on desktop and mobile.
It does not require a new drawer or a hover-only control.

### 9. i18n

New keys in `apps/web/src/locales/{en,pseudo}/settings.json` (camelCase,
`settings:` namespace), cover the gone-model hint and fallback controls.

New keys in the chat namespace cover the persisted model-selection warning.
No added or edited line contains hardcoded user copy.

## Tests

Backend (Go, `*_test.go` beside source):

- Store: `fallback_model` / `auto_fallback` round-trip + migration.
- Reconciler: gone start model is kept, not overwritten; empty model still
  seeded; gone fallback_model kept.
- Runtime session start: an unadvertised start model causes no `SetModel` call.
  The session continues with the reported current or default model.
- Runtime session start: an advertised explicit fallback is applied.
  An unadvertised fallback causes no call and the agent default remains active.
- Runtime session start: an empty catalog and method-not-supported both continue
  with the agent default and create a model-selection warning.
- Runtime session start: an advertised model apply error fails explicitly.
  Auto-fallback keeps best-effort behavior and creates a warning.
- Runtime session start: each decision produces at most one model-selection
  attempt with no profile-layer retry.
- Runtime launch environment: a non-standalone Claude launch exposes the
  effective start model before ACP initialization. It exposes an explicit
  fallback only when auto fallback is off. Request, executor, and profile
  environment values take precedence. Standalone Claude launches and other
  agents receive no new environment values.
- Office post-start: unchanged — availability failures requeue via the
  workspace routing chain regardless of the profile's fallback settings
  (regression test pins that the profile policy does not gate office).
- Model-decision event and warning metadata tests.
- Orchestrator persistence tests for message type, warning metadata,
  idempotency, reload hydration, and persistence errors.

Frontend (Vitest, `*.test.ts(x)`):

- `model-config-selector`: disabled option not selectable; greyed class.
- `session-models` WS handler: stale active model is kept (not cleared).
- `useAgentProfileOptions`: every host-mismatch profile remains selectable and
  shows an advisory warning.
- Profile editor: gone start model renders red + disabled; auto-fallback keeps
  the explicit fallback choice visible but disables its controls.
- Profile editors: fallback settings start collapsed; expanding exposes both
  choices, auto-fallback disables rather than removes the explicit fallback,
  and desktop tooltip help is available through pointer and keyboard focus.
- `agent-profile-normalize`: new fields round-trip.
- Status message: structured model-selection metadata renders localized
  requested, effective, agent, executor, and remediation text.

E2E (Playwright, `apps/web/e2e`):

- Mock backend (`KANDEV_E2E_MOCK=true`): create a profile whose start model
  is not in the host catalog. Make sure that the task-create picker keeps the
  profile selectable, shows one warning icon, and reveals the advisory warning
  through the fine-pointer tooltip or coarse-pointer drawer.
- Launch with an executor catalog that omits the profile model. Make sure that
  no model-selection call occurs, the task continues, and chat shows one warning.
- Reload the task page. Make sure that the warning remains in chat.
- Desktop profile settings: the disclosure starts closed, expands to two
  horizontally aligned option columns, summarizes the current fallback mode,
  and exposes each info explanation on hover/focus.
- Mobile profile settings: tapping the same disclosure reveals one stacked
  flow; tapping either 44px info target opens the localized help drawer without
  horizontal document overflow.

## Persistence & Migration

- `agent_profiles.fallback_model TEXT NOT NULL DEFAULT ''`
- `agent_profiles.auto_fallback INTEGER NOT NULL DEFAULT 0`

Existing rows use `auto_fallback = 0` and no fallback model.
This state means default-on-mismatch behavior.

If the executor omits the saved model, the agent uses its default and Kandev
persists a warning. The saved profile model does not change.
