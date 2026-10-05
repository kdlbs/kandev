---
status: current
system: agents
created: 2026-10-05
requirements:
  - REQ-AGENTS-COMMAND-AUTOCOMPLETE-001
  - REQ-AGENTS-COMMAND-AUTOCOMPLETE-002
---

# Agent Command Autocomplete System Design

## Purpose and boundaries

The agent adapter identifies skills from a verified provider contract. The frontend separates display text from invocation text.
Agents owns these provider semantics. The design reuses the UI-owned composer selection and popup contracts.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-COMMAND-AUTOCOMPLETE-001` | Provider classification, Command transport, Composer model, Rendering and mobile behavior, Compatibility |
| `REQ-AGENTS-COMMAND-AUTOCOMPLETE-002` | Supported action metadata, Confirmed mode state, Rendering and mobile behavior |

## Provider classification

The initial implementation supports one positive classification rule: `codex-acp` command names with a nonempty dollar-prefixed skill name.
The rule uses the adapter's agent ID. A dollar prefix from an unknown provider does not establish skill identity.
The adapter preserves `Name` exactly and adds `kind: "skill"` to the normalized command.
The prefix rule rejects a bare `$`. It removes exactly one dollar marker for display and preserves other punctuation.

`acpdbg` captures on 2026-10-05 establish these shapes:

| Provider and version | Transport | Advertised retro entry | Initial presentation | Evidence and fallback |
| --- | --- | --- | --- | --- |
| Codex ACP 1.13.1 | ACP | `name: "$retro"`, description, input | `/retro` with Skill chip | Captured update and bridge `buildAvailableCommands`; preserve `/$retro` invocation |
| Claude ACP 0.81.2 | ACP | `name: "retro"`, description ending in `(project)`, input | `/retro` without a kind chip | No skill discriminator in captured entries; project descriptions also occur on custom commands |
| OpenCode 1.18.5 and 1.18.30 | ACP | `name: "retro"`, description | `/retro` without a kind chip | No skill discriminator in captured entries |
| Other or unrecognized shape | ACP | Unchanged | Existing command row | Omit kind; preserve name and invocation |

The configured OpenCode 1.18.32 probe failed with npm `ETARGET`. The installed 1.18.5 probe succeeded.
The cached 1.18.30 binary also sent its command notification. Its minimal prompt later timed out; the command capture remains authoritative.
These probes do not establish all historical or future provider versions.

Do not classify Claude entries from `(project)`, descriptions, or a list of known skill names.
Do not claim OpenCode skill coverage from an untyped command entry.
Provider-specific classification can expand later after protocol evidence supplies a reliable discriminator.
This package requires no discovery API or filesystem inventory.

### ACP category information

ACP v1 `AvailableCommand` defines `name`, `description`, optional `input`, and optional `_meta`.
It has no standard field for builtin commands, skills, custom commands, or agents.
Provider extensions can carry this information, but each extension needs a verified provider contract.

The Codex capture includes `_meta.commandAction` on `plan` and `goal`.
Those objects describe client actions, not command origin or skill identity.
Claude and OpenCode send no entry metadata in the captured command lists.
Claude description suffixes such as `(project)` indicate source context, without a structured skill/custom-command distinction.
An entry named `agents` is still a command entry; its name does not identify a selectable agent.
Session modes are separate protocol data and do not classify command entries.

The current `convertAvailableCommands` path ignores `cmd.Meta`.
This package extracts one verified mode action and the verified Codex skill classification.
It does not forward arbitrary metadata or infer a complete category taxonomy.

### Supported action metadata

For `codex-acp`, recognize the captured `plan` command's `_meta.commandAction` only when its complete shape matches:

```json
{
  "kind": "setConfigOption",
  "configId": "collaboration_mode",
  "value": "plan",
  "resetValue": "default",
  "presentation": "state"
}
```

Normalize this object into an optional typed command `action` with `kind`, `config_id`, `value`, and `reset_value`.
The command name remains unchanged. The raw action kind does not become a UI label.
Reject malformed values and unsupported provider/configuration combinations without blocking the command.
Skill classification takes precedence; a `$plan` skill is not the builtin mode command.
No name-only fallback gives Claude or OpenCode `plan` a Mode chip.

The captured `goal` metadata contains `kind: "prefixPrompt"` and `presentation: "state"`.
It does not provide a command category or current goal state.
Keep its description and `input_hint`; omit a metadata-derived category chip.
Selection continues through ordinary draft insertion, including for a supported mode action.
Kandev sends `/plan` through the existing prompt path; the bridge handles its mode change.

## Command transport

Add optional `Kind` to `streams.AvailableCommand` in `apps/backend/internal/agentctl/types/streams/agent.go`.
The JSON value is `kind: "skill"`; absence means classification is unknown.
`Adapter.convertAvailableCommands` in `adapter_updates.go` applies the provider rule before publishing the normalized event.
Existing name-based deduplication remains based on the provider name.

The lifecycle cache, event publisher, and reconnect notification already forward `streams.AvailableCommand` values.
Cover the actual cached and reconnect paths with tests so the new field survives transport.
Update the frontend command types in `session-runtime/types.ts` and `session-events.ts`.
The `session.available_commands` handler stores the provider name, optional kind, typed action, and input hint together.
No database migration or new persistent command catalog is required.

## Confirmed mode state

The captured Codex session exposes a select configuration option named `collaboration_mode`, with `plan` and `default` values.
Use this option rather than permission modes such as `agent` or `agent-full-access`.
The option ID and supported values must match the validated command action.

The existing `session-models.ts` handler can overlay saved runtime preferences onto `ConfigOptionEntry.currentValue`.
Therefore, that effective selector value alone cannot establish the Active chip.
Add a separate optional `confirmedConfigOptions` projection to the per-session `SessionModelsState` entry.
Build it from `session.models_updated.config_options` before preference overlays, only for a settled configuration snapshot.
Use `config_options_settled` as the settlement gate and preserve the provider-reported `current_value`.
An explicitly unsettled snapshot clears confirmation. A settled empty snapshot removes confirmed options.
Model-only updates can preserve confirmation within the same execution; new execution startup invalidates it until a settled snapshot arrives.
The backend carries `config_options_source` and `agent_execution_id` through `session.models_updated`.
After startup settlement, `provider_update` and `provider_response` events refresh the raw projection even when `config_options_settled` is omitted, provided the event belongs to the confirmed or current ready execution.
`provider_update` can carry only the changed options, so merge its values into the confirmed projection and retain values for omitted options. A settled snapshot and `provider_response` replace the projection.
Updates from a starting or different execution do not establish confirmation. While startup identity is unresolved, a delayed prior-execution snapshot cannot restore the hidden confirmation; matching execution identity can restore it, while a new execution requires a fresh settled snapshot.
When `session.state_changed` enters `STARTING` or `session.agentctl_starting` arrives, clear the visible confirmed projection immediately while preserving model-selector fallback data.
If startup identity arrives after the state transition, hold the prior values outside the visible projection until the execution ID is known; restore them only when it identifies the same execution, and discard them for a different execution.
The session runtime reset/cleanup path must clear the projection with the existing session state.
Reconnect restores it only from a settled backend snapshot, without treating saved preferences as confirmation.

Derive the command row from its session's confirmed option:

| Confirmed value | State chip | Localized description |
| --- | --- | --- |
| `plan` | Active | Turn plan mode off |
| `default` | None | Turn plan mode on |
| Missing, unsettled, or unrecognized | None | Toggle plan mode |

Confirmed updates refresh an open menu. Selecting a row does not optimistically alter this projection.
Preserve existing model-selector preference behavior; the projection serves command state display only.

## Composer model

Keep `agentCommandName` as the exact provider name in `SlashCommand`.
Add optional skill classification to the chat-local view model.
Derive the display name once in the shared command mapping used by `TipTapInput`.
Build IDs from the provider name, never the clean display name.
Builtin `retro` and skill `$retro` therefore remain distinct entries.

`createSlashSuggestion` filters by both the clean display name and provider name.
Prefix ranking uses the clean name, with provider-name matching retained for existing queries.
Names remain case-preserving for invocation. Matching retains its current case-insensitive behavior.

Selection stores a clean label, the raw `commandName`, and the optional classification in the TipTap node.
The visible chip uses the clean label. Plain-text serialization uses `commandName` with the existing leading slash.
Serialization must prefer raw `commandName` over the clean label when both exist.
Legacy nodes without a raw name retain the existing label-based fallback.

Update `tiptap-slash-command-utils.ts`, `tiptap-helpers.ts`, and the extension's HTML attributes as one change.
Preserve raw identity through HTML round trips and message recall.
Recall recognizes the raw text `/$retro`, restores the clean chip, and retains the raw text for the next send.
Do not rewrite arbitrary `/retro` text outside autocomplete selection.
Copying plain text intentionally exposes invocation syntax so pasted text keeps its meaning.
Rich clipboard HTML that contains a command-shaped custom node is pasted as its plain-text representation, when available. Untrusted clipboard attributes cannot create a structured command chip or alter the command text serialized on send. Autocomplete selection and validated draft restoration remain the sources of structured command nodes.

## Rendering and mobile behavior

`SlashCommandMenu` supplies localized, noninteractive Skill or Mode badges to a small optional badge slot in `PopupMenuItem`.
Supported mode rows also show Active when confirmed. The row order is icon, name, category badge, state badge, then description.
Use the existing `@kandev/ui` badge primitive and shared tokens.
The badge has no click handler, tab stop, or tooltip requirement.
The option's accessible text includes the name, classification, confirmed state, and argument hint when present.

Show `input_hint` on a secondary line below the row description, with a localized `Arguments` label.
Hints remain provider text; do not translate syntax tokens or replace them with guessed argument forms.
For `/goal`, show the existing `[<objective>|clear|pause|resume]` hint without a category badge.
The hint line stays inside the same selectable option and creates no additional scroll owner.
Mode descriptions, category/state badges, and the Arguments label use locale keys.

The same composition serves desktop and phone. The description truncates first and the badge does not shrink.
Long names can truncate inside the remaining width; description width can fall to zero.
Other popup consumers receive no badge and keep their existing composition.
New copy uses `t()` and complete locale catalogs, including the generated Traditional Chinese pair and pseudo locale.

The nearest mobile exemplar is the shipped composer `PopupMenu`, covered by `mobile-slash-command-composer.spec.ts`.
It contributes caret anchoring, a portal, focus-preserving pointer behavior, one scrolling listbox, and minimum 44-pixel option targets.
This frequent, transient text completion keeps the composer popup rather than adding a separate navigation surface.
The existing visual-viewport positioning handles the software keyboard and safe viewport bounds.
Selection and send state stay shared across viewports.

## Compatibility and failure behavior

Missing or unknown kind keeps the current display and serialization path.
Older frontends ignore the additive field; older backends omit it and keep the existing dollar-prefixed presentation.
Runtime commands without skill classification retain their names and input hints.
Do not strip dollar signs globally in `normalizeSlashCommandName`.
Keep name-based deduplication and session isolation unchanged.

The typed frontend handler currently casts incoming entries. Treat unknown runtime kind values as absent at view-model derivation.
No failed classification can block selection or sending.
Tests cover collisions, bare dollar markers, ordinary commands, unknown providers, legacy nodes, forged rich clipboard command nodes, and unknown kind values.
Action tests cover valid, malformed, missing, additive, and unsupported metadata, including a same-name skill and an untyped custom `plan` command.
State tests cover active/default/unknown values, preference disagreement, session isolation, partial provider updates, delayed old-execution snapshots, startup invalidation, settled empty snapshots, and reconnect.
Argument hints remain visible without establishing a category.

## Evidence and verification

Developer captures remain under `/tmp/kandev-retro-acp/`; they are not repository artifacts or production logs.
Relevant captures are `codex-acp-prompt-20261005-183946.jsonl`, `claude-acp-prompt-20261005-191511.jsonl`, `opencode-local-probe.jsonl`, and `opencode-cached-prompt.jsonl`.
The design records only command shapes and versions, without prompts, credentials, or session IDs.
No new metrics or persistent protocol logging is needed.

Targeted adapter and lifecycle tests prove classification and transport.
Composer unit and component tests prove presentation, raw serialization, and HTML restoration.
Desktop task chat, quick chat, and mobile Playwright tests prove selection without sending and explicit send with the raw command.
Browser checks use isolated mock sessions with representative provider entries, not live provider accounts.

## Related designs

- [Composer suggestion overlays](../../ui/system-design/composer-suggestion-overlays.md)
- [Implementation plan](../../../plans/skill-command-autocomplete/plan.md)
