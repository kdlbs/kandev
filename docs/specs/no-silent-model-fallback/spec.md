# No Silent Model Fallback

**Status**: approved (design package)
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

**Bottom line: the user never asked for a different model, yet the system
switches models for them, invisibly.**

## Goals

- A configured/selected model that is unavailable ("gone") is **never**
  silently replaced. Either the run/session fails explicitly with an
  actionable "change the model" message, or — only when the user opted in —
  it switches to an explicitly configured alternative.
- "Gone" models render **greyed out and unselectable** in every model
  picker, while remaining visible so the user can see what was configured.
- Agent profiles **keep** their previously configured start model even when
  gone (shown red + unselectable in the editor) instead of having it
  auto-healed.
- Profiles whose start model is gone are **blocked** in the new-task /
  new-agent profile picker, unless the profile opts into an explicit
  fallback.
- A new optional per-profile **agent fallback model** allows an explicit,
  single-model automatic switch when the start model becomes unavailable
  (session start and mid-session), replacing the current "walk the whole
  candidate list" behavior.
- A new explicit per-profile toggle **"Fallback automatically to next
  model"** restores the legacy automatic-fallback behavior (session start
  best-effort + office routing re-dispatch). Enabling the toggle hides and
  disables the optional fallback-model field (the two are mutually
  exclusive opt-ins).

## Non-Goals

- Changing the workspace provider-routing configuration model (tiers,
  provider order, execution profiles) itself.
- Provider-level auth flows (login/refresh UI) — only the *consequence* of
  auth expiry (unavailable models) is addressed.
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
- **Strict mode**: profile has `auto_fallback = false` and no
  `fallback_model`. No automatic switching anywhere; unavailable start
  model ⇒ explicit failure.
- **Fallback-model mode**: profile has `auto_fallback = false` and a
  non-empty `fallback_model`. The only permitted automatic switch is to
  that single model.
- **Auto-fallback mode**: profile has `auto_fallback = true`. Legacy
  behavior (session-start best-effort; office routing re-dispatch to next
  candidate). `fallback_model` is ignored.

## Behavior Matrix

Per agent profile, one of three modes (precedence: `auto_fallback` wins
over `fallback_model`):

| Scenario | Strict | Fallback-model | Auto-fallback |
|---|---|---|---|
| Session start, start model gone | **Fail launch explicitly** with "start model X is no longer available — change the model in the profile or configure a fallback". Run/session enters failed state; no session starts. | Apply `fallback_model` instead; surface an explicit "using fallback model Y because X is unavailable" note (log + UI). | Legacy: `SetModel` best-effort (warn + continue on provider default). |
| Session start, `SetModel` fails for another reason | Fail explicitly (strict + fallback-model modes). | Same | Legacy warn + continue. |
| Mid-session model/auth failure (office run, post-start) | **No re-dispatch.** Run fails explicitly with an actionable message (map `model_unavailable`/`auth_required` codes to "change the model" copy). | One-shot retry: re-dispatch **once** with `fallback_model` on the same provider; if that attempt fails too ⇒ explicit failure (no further candidates). | Legacy: re-dispatch to next candidate in the provider order (`HandlePostStartFailure` requeue). |
| Boot reconciliation | Never overwrite a gone start model (keep it; UI shows it red). Same for a gone `fallback_model`. | Same | Same (reconciler is mode-independent). |
| New-task / new-agent profile picker | Profile **blocked** (greyed, unselectable, reason tooltip). | Profile **selectable with a warning** ("start model X is gone — fallback Y will be used"). | Selectable normally. |
| Model picker (profile editor, session toolbar) | Gone models greyed out, unselectable, visible. | Same. | Same. |

`SetModel` failures that mean "this agent does not support model selection"
(JSON-RPC `-32601`, `sessionmodel.MethodNone` / `IsMethodNotFound`) are never
treated as availability failures — a profile without model selection simply
continues as today.

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

### 3. Session start: strict model application (agentctl runtime)

In `internal/agent/runtime/lifecycle/session.go`, the profile-model
application block (`if profileModel != "" && execution.agentctl != nil`)
changes from best-effort to policy-driven, using the session's advertised
model list (`execution.GetModelState()` after `InitializeSession`) and the
profile's mode:

1. `auto_fallback` ON → keep today's warn-and-continue behavior.
2. Start model gone (advertised list non-empty and model absent):
   - `fallback_model` set → `SetModel(fallback_model)`; on success log +
   surface an explicit note (extend the model state / emit an event the UI
   can render, e.g. a `session.model_fallback` event or a field on the
   model state); on failure → fail session init explicitly.
   - otherwise → fail session init explicitly with an actionable message.
3. Start model present in list → `SetModel(model)`; on error:
   - `sessionmodel.IsMethodNotFound(err)` → continue silently (no model
     selection support).
   - otherwise → strict: fail explicitly (both strict and fallback-model
     modes); auto-fallback mode keeps warn-and-continue.

The explicit failure must propagate as the session/run error message so the
chat and run detail render "start model unavailable — change the model".

The same policy applies to the context-reset re-application path
(`reapplySessionModelAfterReset` / `effectiveSessionModelForReset` in
`manager_interaction.go`) via a shared helper, so a context reset cannot
silently drop a gone model either.

### 4. Office post-start failure gating

`HandlePostStartFailure` in
`internal/office/scheduler/routing_lifecycle.go` receives the profile
(`agent *models.AgentInstance` — already a parameter). After classification:

- `!classified.FallbackAllowed` → unchanged (escalate).
- `agent.AutoFallback` → unchanged (requeue to next candidate).
- `agent.FallbackModel != ""` → one-shot retry with the fallback model:
  record `runs.fallback_model = agent.FallbackModel` (new nullable column)
  and requeue. The next dispatch builds **one** candidate from
  `run.ResolvedExecutionProfileID` + `run.ResolvedProviderID` +
  `run.FallbackModelOverride`, skipping the resolver, and clears the
  override after the attempt so a subsequent failure escalates to the
  terminal explicit-failure path. On success the run records
  `ResolvedModel = fallback`.
- otherwise (strict) → return `(false, nil)` so the caller escalates to the
  terminal failure path (`HandleAgentFailure`) — the run fails explicitly.

Surface the classified code in the failure message: map
`model_unavailable` / `auth_required` / `missing_credentials` /
`subscription_required` to an actionable "provider/model unavailable —
change the model" hint on the run/session error (helper in the scheduler or
routingerr, used when composing the failure message).

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
  same model list bound to `profile.fallback_model`. Hidden entirely when
  `auto_fallback` is ON; otherwise shown, with the same gone-red/disabled
  treatment if the fallback model itself is gone.
- **"Fallback automatically to next model" toggle** (new row): a switch
  bound to `profile.auto_fallback`. When ON, the fallback-model row is
  hidden/disabled. Helper text explains the semantics.
- The lighter editor `apps/web/components/agent/cli-profile-editor.tsx`
  (`ModelModeFields`) gets the same two rows for parity.

All new copy is externalized via `t()` into the `settings` i18n namespace
(`apps/web/src/locales/{en,pseudo}/settings.json`) — the i18n ratchet
judges added lines even in unmigrated files.

### 8. Profile picker gating (new-task / new-agent)

`apps/web/lib/state/slices/settings/types.ts` — `AgentProfileOption` gains
`model`, `fallbackModel`, `autoFallback` (populated in
`toAgentProfileOption`).

`apps/web/components/task-create-dialog-options.tsx`
(`useAgentProfileOptions`): compute gone-ness (`profile.model` set and not
in the agent's `model_config.available_models`):

- strict → `disabled: true` with `disabledReason` ("start model X is no
  longer available — change it in the agent profile").
- fallback-model mode → selectable with an amber warning icon/tooltip
  ("start model X is gone — fallback Y will be used").
- auto-fallback → normal.

`apps/web/app/office/setup/agent-profile-setup-controls.tsx`
(`useSelectableProfileOptions`): same gating for office agent setup.

### 9. i18n

New keys in `apps/web/src/locales/{en,pseudo}/settings.json` (camelCase,
`settings:` namespace), covering: gone-model reason, fallback-model row
label + placeholder, auto-fallback toggle label + helper, profile-picker
blocked reason, profile-picker fallback warning. No hardcoded literals on
added/edited lines (ratchet).

## Tests

Backend (Go, `*_test.go` beside source):

- Store: `fallback_model` / `auto_fallback` round-trip + migration.
- Reconciler: gone start model is kept, not overwritten; empty model still
  seeded; gone fallback_model kept.
- Runtime session start: strict mode fails init with actionable message when
  start model gone; fallback-model mode applies the fallback and notes it;
  auto-fallback keeps legacy warn-continue; `-32601` continues silently.
  (`session_test.go` or the manager test harness.)
- Office post-start: strict → escalate (no requeue); auto-fallback →
  requeue to next candidate (existing behavior preserved); fallback-model →
  single forced retry with the fallback model, then escalate on second
  failure; success records resolved model = fallback.
- Error mapping helper unit tests.

Frontend (Vitest, `*.test.ts(x)`):

- `model-config-selector`: disabled option not selectable; greyed class.
- `session-models` WS handler: stale active model is kept (not cleared).
- `useAgentProfileOptions`: strict profile disabled with reason; fallback
  profile selectable with warning; auto-fallback selectable.
- Profile editor: gone start model renders red + disabled; toggle hides the
  fallback row.
- `agent-profile-normalize`: new fields round-trip.

E2E (Playwright, `apps/web/e2e`):

- Mock backend (`KANDEV_E2E_MOCK=true`): create a profile whose start model
  is not in the mock agent's advertised list; assert the profile editor
  shows it red/disabled, the task-create profile picker blocks it (greyed,
  unselectable) or shows the fallback warning when a fallback is set, and
  the toggle hides the fallback row.

## Persistence & Migration

- `agent_profiles.fallback_model TEXT NOT NULL DEFAULT ''`
- `agent_profiles.auto_fallback INTEGER NOT NULL DEFAULT 0`
- `runs.fallback_model TEXT NULL` (office one-shot fallback override,
  cleared after one attempt)

Existing rows default to strict mode (`auto_fallback = 0`, no fallback) —
the safest default: no behavior change until the user opts in. Users who
relied on legacy automatic fallback must enable the toggle, which is the
explicit opt-in the feature is about.

## Risks & Open Questions

- **Behavior change for existing strict-mode profiles**: after this change,
  a session whose start model is gone will fail at launch instead of
  starting on the provider default. That is the requested behavior; the
  failure message must be actionable (covered by the error-mapping helper).
- **`runs.fallback_model` one-shot retry** interacts with the existing
  `MaxAttemptsPerRun` cap and route-cycle baseline — the forced candidate
  must not bypass the cap (it counts as one attempt row via the existing
  `recordAttemptStart`).
- **Probe staleness**: the advertised list can be stale (probe cached).
  Membership checks use the freshest available signal (session
  `models_updated` at start; probe cache for pickers). A stale cache
  showing a gone model as available is acceptable — the agent's own
  `validateAvailableModel` fails fast at `SetModel` time, and the runtime
  treats that as unavailable.
- **Office vs. kanban surfaces**: both share the same agent-profiles rows;
  the picker gating in §8 covers the kanban task-create dialog and the
  office setup flow. Office run-detail routing surfaces are unchanged.
