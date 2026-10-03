---
status: current
system: plugins
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-EXTRACTION-001
created: 2026-09-23
owners:
  - kandev
---

# Prompt History Extraction System Design

## Purpose and boundaries

This design removes core's built-in Prompt history panel and the core-only code
that fed it, while preserving every generic Host contract a replacement plugin
uses. The plugin system owns the extraction outcome because the durable contract
after removal is "the review surface is a plugin task panel over published Host
APIs", not "core renders a panel".

Adjacent contracts this design uses but does not own:

- [Prompt History Plugin Host Prerequisites](../requirements/prompt-history-extraction-host.md)
  and its design own the browser conversation façade, task-panel registration,
  the navigation capability, `host.ui.PromptMentionText`, and favorite state.
- [Prompt History Panel System Design](../../ui/system-design/prompt-history-panel.md)
  owns the product behavior of the panel being removed.
- The [task system](../../tasks/README.md) owns message and turn storage,
  including the durable per-session prompt ordinal.
- [UI prompt alias rendering](../../ui/system-design/prompt-alias-rendering.md)
  owns the shared alias presentation that both the transcript and plugins use.

## Requirement mapping

| Requirement                                 | Design section                                                                                                                                                                                                            |
| ------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-PLUGINS-PROMPT-HISTORY-EXTRACTION-001` | [Components and responsibilities](#components-and-responsibilities), [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Persistence](#persistence) |

## Components and responsibilities

### Removed from core

- The built-in panel UI, its prompt-history older-page reader and paging hooks,
  and panel-only transcript sentinel options. The distinct latest-prompt
  projection for pinned transcript affordances remains under UI ownership.
- Prompt-entry derivation and compact duration formatting; shared transcript
  duration rendering remains in core.
- The retired panel/component identity, desktop and mobile entry points,
  layout-profile placeholder, locale keys, and core panel E2E fixtures.
- Generic plugin task-panel registration, conversation APIs, navigation, and
  mobile placement remain the replacement surface.

These are implementation groupings, not new Host contracts. The requirements
document records the observable removal and retained behavior.

### Retained in core

These are the contracts the replacement depends on. They keep their current
public shape and must not gain prompt-history-specific names or branches.

- Browser conversation façade: `apps/web/lib/plugins/conversation-host.tsx`,
  `conversation-source-scope.ts`, `conversation-reconciliation.ts`,
  `host-runtime-resources.ts`, and the Go handlers in
  `apps/backend/internal/plugins/conversation_handlers*.go`.
- Alias presentation: `host.ui.PromptMentionText` and the shared prompt-mention
  presentation module under `apps/web/components/task/chat/messages/`.
- Task-panel registration and navigation:
  `apps/web/components/task/plugin-task-panel.tsx`,
  `scrollTranscriptToMessage` in `lib/state/dockview-extra-panel-actions.ts`,
  the generic `plugin-panel` dockview component, and the mobile plugin-panel
  picker path.
- Read-only favorite state: `apps/web/lib/state/slices/message-favorites/`.
- Message contract: `prompt_index` in `models.Message` and `v1.Message`, the
  persisted per-session sequence read by
  `apps/backend/internal/task/repository/sqlite/message_prompt_index.go`, and
  the `author_type=user` message filter.
- Shared transcript machinery: `hooks/use-lazy-load-messages.ts` (including the
  visible-pagination stop at prompt `#1`), `hooks/use-lazy-load-sentinel.ts`
  without the panel-only option, and `message-list-native-scroll.ts`.
- Pinned prompt availability: the session-scoped `messagePrompts` projection
  and `useSessionPrompts` provide the latest user prompt to `TaskChatPanel`.
  This projection does not load older prompt pages or expose a Prompt History
  surface.
- Turn-duration helpers used by the transcript hover row:
  `messageTurnDurationSeconds`, `formatPromptDuration`, and
  `PromptDurationUnits`, relocated to `apps/web/lib/turn-duration.ts` and
  consumed by `apps/web/components/task/chat/messages/message-actions.tsx`.
  The panel-specific "earlier of turn completion and next prompt" bound is not
  part of that module.

### Changed at a shared boundary

`apps/web/lib/state/layout-manager/renderable-components.ts` owns a static list
of registered component names. The static source works on both the task route,
which dynamically loads Dockview, and the settings route, which validates saved
profiles without loading the task UI.

`apps/web/lib/state/layout-manager/sanitize-serialized-layout.ts` provides two
forms of the same filter:

- `sanitizeSerializedLayout` removes serialized panels whose components are
  unavailable before Dockview receives the payload.
- `filterLayoutStateByComponents` filters reusable `LayoutState` values,
  removes a group or column emptied by that filtering, retains pre-existing
  empty groups, repairs `activePanel`, and preserves `rootOrientation`.

The same component set is used at all persisted-layout boundaries: on-ready and
environment-switch restore, maximize restore, saved-profile apply and default
profile validation. Hidden-right-pane metadata is filtered when read and again
before a captured pane is restored. The mobile `Panels` entry is derived only
from canvases and registered plugin panels.

When a maximized group survives filtering, its sanitized overlay is restored.
On-ready overlay and pre-maximize layouts also exclude session panels known to
belong to another environment. When the group does not survive, the on-ready
readers (`applyFixupsWithMaximize` and `tryRestoreMaximizeOnly` in
`dockview-layout-restore.ts`) and the environment switch reader
(`restoreMaximizeFromStorage` in `dockview-store.ts`) use the filtered
`preMaximizeLayout` rather than a possibly stale environment slot. They persist
that layout before removing the unusable maximize record, keeping both restore
routes consistent.

## Data and contracts

- No backend API, DTO, table, migration, route, or plugin conversation
  projection change. The latest-prompt projection is frontend state for pinned
  transcript affordances, not a replacement for plugin-owned prompt history.
  Repository hard-delete paths remove session prompt-sequence rows with their
  owning task/session so a deleted identity cannot leave stale internal state.
- `prompt_index` stays on `models.Message` and the public `v1.Message`, and the
  plugin conversation DTO keeps `promptIndex`, because ordinals, durations, and
  prompt-only pages are plugin-facing behavior. Initial-task-brief fallback and
  live-session sequence allocation are unchanged.
- Retired browser identities: the fixed panel id and component name
  `prompt-history` and the `MobileSessionPanel` member `"prompt-history"`. They
  are removed from their registries and are not reused for a different panel.
  Plugin task panels use the `plugin:<pluginId>:<panelKey>` namespace and the
  generic `plugin-panel` component.
- The saved-layout entry shape is unchanged: a `LayoutPanel` keeps `id`,
  `component`, `title`, and optional `params`. The removal invalidates entries
  whose `component` is `prompt-history`. Saved settings layouts are not migrated
  in place; failed maximize restoration persists its filtered pre-maximize
  layout to the environment slot before clearing the unusable maximize record.
- The message-favorite store keeps its session-scoped shape and storage
  contract; the extracted panel was only one reader of it.

## Control flow

1. **Open the surface (desktop).** `AddPanelMenuItems` lists canonical panels
   plus registered plugin task panels. After removal the Prompt history row is
   gone; an installed replacement plugin contributes its own row and opens a
   `plugin-panel` dockview panel with a plugin panel id.
2. **Open the surface (phone).** The `Panels` bottom-navigation picker lists
   task canvases and plugin panels. After removal its only prompt-history path
   is the plugin registration; `showPromptHistory` and its dedicated button are
   deleted. The bottom-navigation `Panels` entry is rendered from
   `showPromptHistory || hasTaskCanvases || mobilePluginPanelsAvailable`, so
   dropping the first term means a task with no canvases and no enabled plugin
   panel no longer shows an entry that would open an empty sheet.
3. **Read prompt data.** The plugin calls the façade, which performs authorized
   reads and live reconciliation into its own scope cache for Prompt History.
   Separately, `TaskChatPanel` reads the latest-prompt projection for pinned
   transcript affordances; it does not page older prompt history.
4. **Navigate to a prompt.** The plugin's navigation capability calls
   `scrollTranscriptToMessage`, which activates the chat panel and queues the
   pending scroll target in the dockview store; the transcript consumes it
   exactly as before.
5. **Restore a saved layout.** `sanitizeSerializedLayout(layout, DESKTOP_VALID_COMPONENTS)`
   runs before `fromJSON`; with `prompt-history` no longer a known component or
   known panel id, the entry is dropped and the remaining panels restore
   unchanged. Layout capture uses `filterEphemeral`, which applies its own
   `KNOWN_PANEL_IDS` and `STRUCTURAL_COMPONENTS` rules. It can omit renderable
   panels and preserves empty groups for split layout state. Canonical-title
   normalization is a separate capture step for known panel ids.

## Failure and recovery

- Saved layouts and profiles are filtered before Dockview or profile validation
  consumes them. Removed components do not instantiate, and a profile that
  loses its retired-only group continues to apply with its remaining panels.
- Serialized maximize overlays and `preMaximizeLayout` use their respective
  filters. A surviving maximized group remains maximized; otherwise both
  restore paths apply the filtered pre-maximize layout and persist it before
  clearing the unusable maximize record. If that write fails, the record stays
  available for recovery on the next restore.
- No error, notice, empty panel, or user action is needed for stale records.
  Saved profiles, favorites, transcript state, prompt ordinals, and shared
  turn-duration rendering remain intact.
- Without a replacement plugin, there is no Prompt history entry, loading state,
  or passthrough panel in the workbench.

## Persistence

- Server-side storage is unchanged: no SQLite or PostgreSQL migration, no
  route, table, or column change, and no startup work. Saved layouts and saved
  layout profiles, including the active default, are persisted user settings
  (`saved_layouts`, written through the settings API), so a retired panel inside
  one is normalized when the settings are read and applied rather than rewritten
  in place.
- Browser-local state: the per-environment layout and its profile identity
  (`kandev.dockview.env-layout-v3.<envId>`,
  `kandev.dockview.env-layout-profile-v1.<envId>`) and the per-environment
  maximized-state blob
  (`kandev.dockview.env-maximize-v3.<envId>` in `apps/web/lib/local-storage.ts`)
  may still reference `prompt-history`; they stay inert, and the retired entry
  is dropped when the record is used. The message-favorite session storage stays
  readable. No saved layout, profile, or favorite is deleted during the upgrade;
  only a maximized-state blob that cannot survive sanitization is discarded.
- Restart behavior is unchanged: layouts reload through the same sanitize path,
  and the replacement plugin, when installed, re-registers its panel on load.

## Security

Authorization is unchanged: the removal deletes a consumer of already-authorized
data. The façade continues to require an active plugin, the `api_read:messages`
capability, and task-session access before returning messages or turns, and the
transcript navigation capability continues to validate session and generation.
No new privileged surface is introduced, and no prompt-history-specific
authorization path is added.

## Observability

No new metrics or counters. The dockview restore path already reports dropped
invalid panel ids at debug level (`dockview:restore`), which is the visible
evidence that a stale panel entry was ignored. Plugin conversation telemetry and
gateway logs are unaffected.

## Related decisions

- [Browser plugin conversation facade](../../../decisions/2026-09-06-browser-plugin-conversation-facade.md).
- [Conversation source reconciliation](../../../decisions/2026-09-16-conversation-source-reconciliation.md).
