---
status: draft
system: agents
created: 2026-11-17
requirements:
  - REQ-AGENTS-SESSION-SETTING-SEMANTICS-001
owners:
  - Kandev
---

# Session setting semantics system design

## Purpose and boundaries

The agent runtime owns setting meaning and confirmation. The web UI renders normalized meaning.
This supplements [grouped model catalogs](grouped-model-catalog.md), [permission controls](agent-permission-control-integrity.md), and [startup failure explanations](session-startup-failure-explanations.md).
The [fix plan](../../../plans/pi-thinking-settings/plan.md) records delivery and reproduction evidence.

## Requirement mapping

| Criteria | Design sections |
| --- | --- |
| 001.1, 001.8 | Classification and compatibility; Failure evidence |
| 001.2, 001.3 | Model-dependent application |
| 001.4, 001.5, 001.7 | Saved intent and defaults |
| 001.6, 001.9 | Presentation and recovery |

## Classification and compatibility

Add a small shared semantic classification at `internal/agentctl/acpcompat`, reused by runtime adapters and utility probes.
Preserve ACP select categories. A `thought_level` option denotes thinking; a `mode` option denotes permission/session mode under the existing contract.
Pi's legacy `modes` and `session/set_mode` are aliases for thinking. Qualify this mapping by the trusted built-in `pi-acp` binding and its advertised thinking configuration, never a model-name suffix or mode display label.
Do not interpret another provider's legacy modes as thinking simply because their values resemble off/low/high.

Use the existing trusted agent binding and the projected `ConfigOption.category` and `ConfigOption.id` as the semantic carriers.
The implementation does not add parallel `session_mode_kind` or `session_mode_config_id` fields. Discovery, session streams, and saved snapshots already carry these categories and identities. A second carrier could disagree with the configuration snapshot.
Older records remain readable through the qualified Pi binding and its historical `thought_level` config ID.
Ambiguous multiple thinking options do not acquire an invented alias. Surface unavailable setting evidence and retain saved intent.
Use existing streams and caches; add no capability database or setting registry.

For qualified Pi, render one thinking control. Suppress the duplicate legacy permission selector.
Prefer `session/set_config_option` on the advertised thinking option. Fall back to legacy `session/set_mode` only for a qualified legacy shape or method-not-found with an advertised compatible legacy value.
A provider refusal, timeout, or malformed response is not permission to try another mutation route.
Real permission options remain on the existing strict `SetMode` path.

## Model-dependent application

`SessionManager.InitializeAndPromptWithLayers` resolves the winning exact model first.
After model confirmation, consume the complete configuration snapshot from that same active session and configuration generation.
Replace Pi's derived thinking choices from that snapshot, including an off-only or empty catalog. Do not retain the initial legacy mode list after a model change. Without a new catalog, clear the previous model's thinking choices and wait for the provider update. Stamp a model report and its paired thinking report when the adapter commits the snapshot. If the model-change reply arrives before the catalog update, wait briefly for the selected model's snapshot. When that snapshot is unavailable, use the adapter's live validation and require confirmation before prompt dispatch.
Use existing config-change serialization and session/generation fencing in `adapter_session.go`, `adapter_updates.go`, and `adapter_mode_state.go`.
Settings probes use their authorized selected-model context through the existing capability resolver. A settings probe never becomes authoritative live state.

Resolve thinking intent once across profile, workflow, and persisted runtime layers, following the existing winning-source rules.
Apply it once after the model, through the canonical option or qualified legacy route. Consume the equivalent legacy alias so `applyExplicitSessionMode` cannot replay it afterward as a permission setting.
Do not let the generic best-effort config application swallow this explicit startup failure.
Validate exact choice membership against current-model capabilities, then confirm the provider's effective value.
A correlated response or a post-request observation can confirm application under the existing operation-generation rules. A stale observation cannot.
An unsupported choice, mismatch, refusal, or unknown confirmation holds the first prompt. An absent profile override causes no mutation, subject to stronger existing overrides.

## Saved intent and defaults

Use existing `config_options` for new Pi thinking selections. No new SQL column is required.
At each source layer, an explicit selection under the advertised thinking ID wins over the stable saved Pi key. Either key wins over a nonempty legacy Pi `mode` alias. Remove both keys from the generic replay options.
Then apply the existing precedence between sources. Do not let a lower-source canonical value beat a higher-source legacy alias.
Read compatibility is pure: preserve stored values, original-config baselines, attribution, and historical snapshots.
On profile creation or an explicit update to thinking settings, store Pi's thinking intent under the bridge's stable `thought_level` ID and clear the duplicate mode alias. At runtime, use the option ID advertised by the active session when available.
A legacy PATCH that supplies only `mode` sets or clears the saved thinking level. If the same PATCH supplies a canonical thinking value, that value takes precedence.

**Use provider default** in the profile editor submits `mode: ""` and an updated config map without the thinking override. It preserves unrelated options and the selected model.
Test actual PATCH omission/empty handling: an omitted mode field is not a clear operation. Generic sanitization that drops blank map values must not accidentally retain a stored value during merge.
An explicit off remains stored as off. No database sweep clears existing choices.
Profile duplication must preserve semantic intent. Normal resume/reset must normalize old aliases before reapplication.
The provider-restored recovery policy remains authoritative and skips saved reapplication as it does today.

Workflow inheritance and live Reset to profile retain their existing meaning. Do not label either as provider default when a profile still supplies a value.
For this repair, provider-default selection is a profile operation; session/workflow recovery names the stronger source and opens its existing editor/reset control.
No new tri-state runtime override or retry endpoint is introduced.

## Failure evidence

Extend the bounded startup cause projection in `lifecycle/bootstrap_failure.go` and `task/models/launch_errors.go` with thinking-specific codes:
`thinking_level_unavailable`, `thinking_level_failed`, `thinking_level_mismatch`, and `thinking_level_unconfirmed`.
Carry bounded requested/effective setting values, the canonical config ID when known, and the winning source through shared API types and UI view models.
Retain operation, attempt identity, prompt_not_sent, and existing correlation/recovery filtering.
Use dedicated setting fields instead of claiming these are permission modes. Raw provider messages remain outside safe UI evidence.
Historical permission-mode causes stay readable and are not rewritten. New qualified Pi failures use thinking causes.
Older clients can fall back to the existing generic startup explanation for an unknown new cause.
No task/model/value is a metric label. Existing structured diagnostics suffice; add no new metrics for this repair.

## Presentation and recovery

Reuse `ModelConfigSelector`, `profile-form-fields.tsx`, `profile-model-fields.tsx`, and the model-aware capability hooks.
Show Thinking level, its supported choices, and Use provider default even for a singleton off option. Do not hide the reset action with singleton-choice filtering.
Keep unsupported saved values visible as unavailable. Do not use `reconcileConfigOptionValues` to silently remove this draft before the user chooses a replacement or default.
Loading hides stale choices. Failed resolution preserves the draft and presents Retry.

Reuse the existing startup recovery card and its evidence view model. Show the requested value, selected model, effective/available values when known, and the winning source.
For a profile-owned choice, Edit profile opens that profile. Saving the explicit default does not submit the old prompt. Explain that a fresh session uses the saved choice.
For session/workflow-owned intent, expose the appropriate existing configuration editor/reset route and explain precedence. Do not promise a profile edit repairs that override.
Retain all current session ownership, read-only workspace, and explicit recovery gates.

Desktop uses the shared compact selector/popover. Phone uses the existing model-config sheet pattern with fixed header, one internal scroller, visible Back/Close, and focus return.
Use 28px fine-pointer controls and at least 44px phone/coarse-pointer targets. Preserve safe-area padding and draft state across viewport changes.
Reuse existing radius, border, icon, and reduced-motion tokens. Add no decorative motion or new component library.
All new text uses i18next and all required locales; generate Traditional Chinese through the repository script.

## Alternatives and scope

Globally disabling mode confirmation would weaken real permission enforcement and is excluded.
Blindly clearing every saved Pi mode would lose intentional thinking choices and is excluded.
Guessing reasoning support from model IDs or setting unknown Cursor models to reasoning=true would invent provider capabilities and is excluded.
The selected approach reuses canonical ACP categories and the existing Pi compatibility boundary. This local rationale is sufficient; no new ADR or repository-wide provider framework is needed.

## Verification boundaries

Use captured, sanitized Pi 0.0.34 shapes and synthetic transport peers for deterministic tests. No credentials are required in CI.
Cover the full bootstrap path, post-model catalog changes, restored old records, omission versus clear, and a real permission-provider control.
Mock-agent E2E exercises profile Save/reload, unavailable-choice recovery, fresh startup, and phone interaction.
Authenticated smoke tests are supplemental. A ready session with empty model output proves only prompt delivery, not a useful provider response.
