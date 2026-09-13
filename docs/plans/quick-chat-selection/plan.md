---
created: 2026-09-13
status: draft
requirements:
  - REQ-UI-QUICK-TERMINAL-003
  - REQ-AGENTS-GOAL-VISIBILITY-001
system_design:
  - ../../specs/ui/system-design/quick-chat-selection.md
  - ../../specs/agents/system-design/goal-visibility.md
legacy_specs: []
---

# Implementation Plan: Quick Chat selection and goal visibility

## Overview

Restore the last selected conversation for each workspace and conversation kind.
Keep the preference across reloads without allowing background updates to change it.
A second outcome exposes active provider goals beside the existing Todos/PR information.
Two sequential work orders deliver selection first, then retained goal state and its chat disclosure.
Implementation is pending an explicit implementation request.

## Evidence and classification

The reported conversation was `List nova28’s open PRs`.
The exact screenshot trigger is not recorded. Two source-proven reproductions establish the gap:

1. In workspace A, select its second ordinary conversation. Visit a conversation
   in workspace B. Reopen ordinary Quick Chat in A: the launcher selects A's first conversation.
2. Select the second ordinary conversation, reload the page, then reopen Quick Chat.
   A fresh store loses the prior selection and the launcher uses its first matching conversation.

`useQuickChatLauncher` reads one global `activeSessionId`, then uses
`matchingSessions[0]` when that ID does not match. `createUISlice` initializes
the ID to null. Closing the dialog alone does not clear it.
Terminal activation can also clear that ID, so terminal visits expose the same gap.

The current requirement defines launcher kinds and portable tab order, but lacks
remembered conversation selection. This package adds `REQ-UI-QUICK-TERMINAL-003`
to the existing owning requirement and extends its paired design.
The UI system owns this presentation preference; task membership and runtime remain task-owned.

### Goal evidence and ownership

A fresh raw ACP frame from the reported session contains `session_info_update`
with `_meta.goal`, including objective, status, timestamps, and a control-method hint.
Its normalized `session_info` event preserves the field. The inspected Codex ACP
1.11.0 bridge confirms active/paused/blocked/limited/complete snapshots and explicit null clearing.
The observed snapshot is complete, not active. No production session is changed for testing.

`mergedACPSessionInfo` and `registerSessionInfoHandlers` currently replace the entire
metadata object. Unrelated thread-status updates erase goal information.
The first goal regression retains an active goal after such an update.
The agent system owns this provider lifecycle and its visible disclosure; the UI
system continues to own the independent conversation-selection preference.
The approved UI is a static **Goal Active** chip after PR information, with hover,
focus, and click details on desktop and a bottom drawer on phone/coarse pointers.

## Scope

### In scope

- Browser-local selection per user, workspace, and conversation kind.
- Explicit-selection precedence, reload recovery, and list-readiness handling.
- Ordered fallback after authoritative removal; existing setup and terminal policies.
- Desktop and phone regression coverage through the existing launchers.
- Retained provider goal lifecycle, shared Goal Active chip, and accessible details.
- ACP-to-UI regression coverage, localization, and goal-disclosure documentation.

### Out of scope

- Goal controls, provider scheduling, and polling cadence changes.
- Transcript scroll restoration, cross-device selection synchronization, and tab-order changes.
- Runtime auto-start policy, terminal persistence, and passthrough-toolbar redesign.

## Technical approach

Follow [remembered conversation selection](../../specs/ui/system-design/quick-chat-selection.md).
Add typed remembered state and readiness in the UI slice. Add a bounded, versioned
storage codec under `apps/web/lib/quick-chat/` and integrate it with the root store.
Bound the stored map to 200 workspace entries per identity, retaining the most
recent explicitly selected entries. Malformed entries do not block valid siblings.

Use `auth.user.id` for signed-in scope and a separate disabled-auth identity.
Reset remembered state on identity transitions. Keep storage side effects outside
Immer recipes and do not promote background hydration choices into saved preferences.

Resolve generic opens from current store state, remembered selection, and
`orderQuickChatTabs`. Audit `openQuickChat`, new-chat activation, tab selection,
close/delete, `activateQuickTerminal`, hydration, and resync. Preserve the existing
explicit config-chat and terminal launch paths. An explicit open after a pending
generic open cancels that pending restoration.

Do not reject a saved ID before an authoritative workspace list arrives.
Reuse boot hydration and `useQuickChatResync`; add no fetch in a component.
Avoid briefly mounting the fallback conversation because mounting can trigger
session resumption. A late response must respect the current selection revision.

Storage is local by design. The existing portable tab-order ADR still owns order.
This local preference adds no architecture boundary needing a separate ADR.
Public documentation stays unchanged during planning. During implementation, check
`docs/public` for Quick Chat reopening guidance and update any conflicting statement.

### Goal retention and disclosure

Follow the [agent goal design](../../specs/agents/system-design/goal-visibility.md).
Preserve the recognized goal separately from unrelated opaque metadata updates,
using the existing session metadata and session-info delivery path. Keep null clear
semantics and stale hydration guards. No new scheduler or backend schema is needed.

`ChatStatusBar` reads the selected session's goal and renders `AgentGoalChip` after
registered PR status and before the queue chip. An active goal keeps the status
row visible even without task/Todo/PR content. Idle does not hide it. Completion,
clear, paused, blocked, limited, or unsupported state does not show an active chip.
Use the shared status row for ordinary task chat and Quick Chat.

Task 02 owns a typed parser, narrow retention behavior, shared details, localized
copy, and mock ACP scenarios. It must not infer active goals from transcript text
or change provider wakeup frequency. The chip opens no provider-control request.
Update `docs/public/developer-tools.md` during implementation to explain the chip
and the difference between an active goal and a currently generating turn.

## ASCII UI preview

### UI-01: Reopen after reload or a workspace visit

Entry: desktop Quick Chat launcher or keyboard shortcut. Brackets mark selection.

```text
Fallback today: [Archive tasks]  List nova28's open PRs
After fix:      Archive tasks    [List nova28's open PRs]
               +--------------------------------------+
               | Previously selected conversation     |
               | Existing messages and composer       |
               +--------------------------------------+
```

### UI-02: Phone reopen

Entry: listing menu > Quick Chat. The menu closes before the full-height surface opens.

```text
+--------------------------------+
| Quick Chat               Close |
| ... [List nova28's open PRs] ...|  tab strip scrolls horizontally
|                                |
| Previously selected messages   |  content scrolls vertically
|                                |
| Existing composer              |
+--------------------------------+
```

These previews require selection identity and existing surface hierarchy, not new
copy or pixel geometry. Reuse `QuickChatModal` and the shipped mobile listing menu.
Keep dynamic viewport sizing, safe-area handling, touch targets, and focus return.
During initial loading, retain existing loading UI without mounting another conversation.
Without eligible conversations, retain the current setup. A failed load remains dismissible.
UI-01 covers `.1`–`.5`; UI-02 additionally covers `.7` of `AC-UI-QUICK-TERMINAL-003`.

### UI-03: Goal Active above the composer

Entry: hover, focus, or click the active-goal chip. Shared by task chat and Quick Chat.

```text
+[Todos 2/5] [PR #123] [Goal Active] [Queue]----------+
+| + Goal ---------------------------------------+ |
+| | Active                                      | |
+| | Coordinate contributor PR reviews            | |
+| | The agent may continue automatically         | |
+| | between replies.                            | |
+| +---------------------------------------------+ |
+| Existing chat input                             |
++-------------------------------------------------+
+```

The details popover anchors above the chip; its drawn position is illustrative.
The icon is static. Idle retains the chip. Complete/clear removes it and closes details.
With no Todos/PR, the row contains only Goal Active and any existing right controls.

### UI-04: Phone goal details

```text
+[Todos] [PR #123]----------------+
+| [Goal Active]                 |  wraps when needed; 44px touch target
+| Existing chat input           |
++-------------------------------+
+
+Tap opens an inset bottom drawer:
++-------------------------------+
+| Goal                    Close |  fixed header
+| Active                        |
+| Coordinate contributor PRs... |  objective scrolls vertically
+|                               |
+| The agent may continue         |
+| automatically between replies.|
++-------------------------------+  clears bottom safe area
+```

UI-03 and UI-04 cover `AC-AGENTS-GOAL-VISIBILITY-001.1`–`.8`.
Use existing status-row hierarchy and Drawer primitives. Labels shown here require
localization. Long text wraps without document overflow. Keyboard Escape dismisses
only the details; completion or session switching closes them without stale content.

## Tests

| Criteria | Planned evidence |
| --- | --- |
| `.1`, `.3` | `use-quick-chat-launcher.test.ts`: restores A's second chat after selecting B; terminal/config visits preserve ordinary selection; explicit opens win |
| `.2`, `.6` | New `lib/quick-chat/selection-storage.test.ts`: reload codec, identity isolation, malformed/blocked storage, bounded retention |
| `.3`, `.5` | `quick-chat-actions.test.ts`, new `quick-chat-selection.test.ts`: background upserts do not save a choice; late restoration loses to explicit selection |
| `.4` | `quick-chat-sync.test.ts`: authoritative removal clears only its scope; fallback follows displayed order; no matching kind opens setup |
| `.5` | `use-quick-chat-resync.test.ts` and hydration tests: defer initial list, reject stale responses, preserve preference on failure, accept boot snapshot |
| `.6` | Store/auth tests: identity changes clear old in-memory selection; independent mounted clients do not follow storage events |

The first RED regression is `restores the last ordinary chat after visiting another workspace`
in `apps/web/hooks/use-quick-chat-launcher.test.ts`, using the real selection actions.
Assert the chosen session ID, not a missing test selector.

### Goal regression matrix

| Criteria | Planned evidence |
| --- | --- |
| `.2`, `.5`, `.6` | New `event_handlers_goal_test.go`: active goal survives unrelated metadata; completion and null are retained; stale snapshots cannot resurrect it |
| `.5`, `.6` | New parser tests and adapter conversion tests: supported status values, absent/null distinction, malformed input, attachment identity |
| `.2`, `.5`–`.7` | `session-info.test.ts` plus hydration tests: sparse updates, late HTTP response, missing row recovery, selected-session isolation |
| `.1`, `.3`, `.4`, `.7`, `.8` | New `agent-goal-chip.test.tsx` and status-row tests: goal-only row, placement, hover/focus/click, drawer, Escape, session changes, no provider calls |

## E2E tests

- Desktop: add `restores the selected conversation after reload` and workspace-switch
  scenarios to `e2e/tests/chat/quick-chat.spec.ts`, project `chromium`.
- Phone: add the same non-first selection, dismissal, reload, and reopen flow to
  `e2e/tests/chat/mobile-quick-chat-tabs.spec.ts`, project `mobile-chrome`.
- Seed distinct conversation bodies. Assert the restored active body, selected tab,
  and unchanged session count. Use real UI selection, not direct store injection.
- Preserve existing tab-order, rename, cross-device membership, and terminal tests.

- Goal desktop: new `e2e/tests/chat/agent-goal.spec.ts`, project `chromium`, covers
  Quick Chat and normal task chat through mock ACP notifications.
- Goal phone: new `e2e/tests/chat/mobile-agent-goal.spec.ts`, project `mobile-chrome`,
  taps the chip, checks drawer content/containment and 44px target, dismisses,
  reloads, then checks completion and clear. No arbitrary sleeps or store injection.
- Seed sibling sessions with different objectives. Switch while details are open
  and assert no cross-session objective. Include active-goal/idle-thread state.

## Work orders

- [ ] [Task 01: Remember and restore conversation selection](task-01-restore-selection.md)
- [ ] [Task 02: Retain and disclose active agent goals](task-02-goal-visibility.md)

Execute sequentially in this session. Each work order owns its exact checks.
Task 02 follows Task 01 for integration of the shared Quick Chat surface.

## Verification results

Expanded-package planning validation passed:

- `python3 scripts/list-docs.py validate`: 265 decisions and 829 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Package-relative file links: checked for all three plan/work-order files and the goal specification pair.
- `git diff --check -- docs/specs docs/plans/quick-chat-selection`: passed.

Implementation test commands remain pending in the two work orders.

No production or permanent test changes in this package.
The earlier baseline ran three existing suites with 55 passing tests.
That result does not prove the proposed restoration behavior.

## Risks

- Initial hydration can wrongly invalidate a saved ID or briefly resume the wrong agent.
- Partial snapshots can erase another workspace's preference unless readiness is scoped.
- Direct callers can bypass launcher-only persistence; audit all activation actions.
- Global storage subscribers can save transient fallback choices or leak identity state.
- Storage failure must degrade to in-memory behavior without blocking the dialog.

- Sparse provider metadata can erase goal state unless omission and null remain distinct.
- A stale session hydration response can resurrect a completed or cleared goal.
- Idle thread state does not establish goal state or a future wakeup time.
- Older ACP bridges may not report goals; these sessions show no chip.
- Long objectives and several status chips must not make the phone composer overflow.
