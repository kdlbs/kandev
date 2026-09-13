---
status: draft
system: ui
requirements:
  - REQ-UI-QUICK-TERMINAL-003
created: 2026-09-13
owners:
  - kandev
---

# Quick Chat Selection System Design

## Purpose and boundaries

This design extends the shared [Quick Chat tab contract](quick-terminal.md).
It owns remembered conversation selection, not tab membership or agent lifecycle.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-QUICK-TERMINAL-003` | Ownership and persistence; selection and restoration; responsive behavior and verification |

## Ownership and persistence

UI owns the reading preference. Task services still own conversation membership,
and the existing session-resumption hook owns runtime recovery.
This section is the proposed design for `REQ-UI-QUICK-TERMINAL-003`.

Add a typed selection map to `QuickChatState`, separate from the visible
`activeSessionId` and terminal-selection fields. Key it by workspace and conversation
kind (`chat` or `config`). Values identify persisted session IDs, never titles.
Track in-memory setup selection separately from the persisted choice.

Use a small browser-storage module under `apps/web/lib/quick-chat/`.
Store a versioned map in `localStorage`, scoped by authenticated user ID.
Use a distinct local identity only when authentication is disabled. Anonymous
authentication-enabled pages must not read or write another identity's preference.
Validate parsed shapes and entries. Retain at most 200 workspace entries per identity,
ordered by the latest explicit selection. Catch unavailable storage and quota failures.
On identity change, clear the old in-memory map before loading the new identity.
Store only identifiers; never store conversation content or credentials.

Selection belongs to the local reading context, unlike portable tab order.
Backend user settings would couple independent devices and add a write for every
selection. Browser storage provides restart recovery without that coupling.
Do not subscribe to storage events to change an open dialog. Each mounted client
keeps its own selection; a fresh client reads the last stored choice.
No backend schema, endpoint, or preference migration is needed.

## Selection and restoration

Centralize remembered-choice updates for explicit `openQuickChat`,
`setActiveQuickChatSession`, and successful new-conversation activation.
`activateQuickTerminal` must not erase either conversation-kind preference.
Background upserts and hydration fallbacks must not write a new remembered choice.
Keep storage side effects outside Immer recipes. The root store integration must
cover direct action callers, including configuration launchers and new-chat flows.

`useQuickChatLauncher` resolves the requested workspace and kind against current
state at activation time. Prefer an explicitly selected current-page setup, then
the valid remembered conversation. Otherwise use `orderQuickChatTabs` with the
current optimistic or saved workspace order, filtered to eligible conversations.
Without a candidate, retain the existing kind-specific setup behavior.
An explicit session-ID open always wins over remembered selection.

Add per-workspace list readiness to the existing hydration/resync path. An accepted
boot snapshot or successful `syncQuickChatSessions` marks that workspace ready.
An initial empty state, failed request, or discarded stale response does not.
An open request before readiness displays the existing loading presentation without
mounting another persisted session view. A pending request carries workspace,
kind, and a selection revision. Dismissal, a workspace change, or explicit selection
invalidates it. Resolve it once after an accepted snapshot; never poll separately.
Existing resync error handling remains authoritative. A failed load preserves the
remembered ID and leaves the surface dismissible.

`restoreQuickChatSelection`, `reconcileQuickChatSessions`, and deletion paths must
distinguish missing-before-readiness from authoritative removal. After removal,
clear only the matching workspace/kind entry. Preserve existing adjacent-tab
behavior when closing the visible tab; record its replacement only for that kind.
Never prune unrelated workspace preferences from a partial workspace snapshot.

## Responsive behavior and verification

Keep the existing desktop floating dialog and phone full-height dialog.
The phone listing menu remains the entry point and closes before Quick Chat opens.
The tab strip owns horizontal overflow; the selected content owns vertical scrolling.
No new toolbar, sizing, or focus policy is introduced.

Unit tests cover storage failure, identity separation, selection precedence,
terminal/config visits, ordered fallback, authoritative deletion, and delayed hydration.
Desktop and phone E2E select a non-first conversation, dismiss, reload, and reopen.
They assert the active content identity and unchanged conversation count.
The fix package is [Quick Chat selection](../../../plans/quick-chat-selection/plan.md).

