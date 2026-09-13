---
created: 2026-09-13
status: draft
requirements:
  - REQ-UI-QUICK-TERMINAL-003
system_design:
  - ../../specs/ui/system-design/quick-chat-selection.md
legacy_specs: []
---

# Implementation Plan: Restore the previous Quick Chat

## Overview

Restore the last selected conversation for each workspace and conversation kind.
Keep the preference across reloads without allowing background updates to change it.
One work order delivers the state, persistence, launcher, and rendered regression tests together.
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

## Scope

### In scope

- Browser-local selection per user, workspace, and conversation kind.
- Explicit-selection precedence, reload recovery, and list-readiness handling.
- Ordered fallback after authoritative removal; existing setup and terminal policies.
- Desktop and phone regression coverage through the existing launchers.

### Out of scope

- Goal/wakeup UI, ACP metadata changes, and agent scheduling.
- Transcript scroll restoration, cross-device selection synchronization, and tab-order changes.
- New visual controls, runtime auto-start policy, and terminal persistence changes.

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

## E2E tests

- Desktop: add `restores the selected conversation after reload` and workspace-switch
  scenarios to `e2e/tests/chat/quick-chat.spec.ts`, project `chromium`.
- Phone: add the same non-first selection, dismissal, reload, and reopen flow to
  `e2e/tests/chat/mobile-quick-chat-tabs.spec.ts`, project `mobile-chrome`.
- Seed distinct conversation bodies. Assert the restored active body, selected tab,
  and unchanged session count. Use real UI selection, not direct store injection.
- Preserve existing tab-order, rename, cross-device membership, and terminal tests.

## Work orders

- [ ] [Task 01: Remember and restore conversation selection](task-01-restore-selection.md)

## Verification results

Planning validation passed:

- `python3 scripts/list-docs.py validate`: 265 decisions and 827 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/quick-chat-selection`: passed.

No production or permanent test changes in this package.
The earlier baseline ran three existing suites with 55 passing tests.
That result does not prove the proposed restoration behavior.

## Risks

- Initial hydration can wrongly invalidate a saved ID or briefly resume the wrong agent.
- Partial snapshots can erase another workspace's preference unless readiness is scoped.
- Direct callers can bypass launcher-only persistence; audit all activation actions.
- Global storage subscribers can save transient fallback choices or leak identity state.
- Storage failure must degrade to in-memory behavior without blocking the dialog.
