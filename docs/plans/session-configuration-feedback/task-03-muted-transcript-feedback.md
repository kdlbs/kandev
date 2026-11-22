---
id: "03-muted-transcript-feedback"
title: "Muted transcript feedback"
status: done
wave: 3
depends_on:
  - "01-picker-outcomes"
  - "02-slash-command-feedback"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CONFIG-FEEDBACK-001
acceptance_criteria:
  - AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.1
  - AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.2
  - AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.3
  - AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.4
  - AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.5
  - AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.6
  - AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.7
system_design:
  - ../../specs/agents/system-design/session-configuration-feedback.md
---

# Task 03 Muted transcript feedback

## Scope

Move picker feedback into task chat and use the same muted status treatment
for slash receipts. Group model, mode, and provider options changed in one
session before the next turn. Use the correlated result and Retry guards from
Tasks 01 and 02. The work is limited to client-side presentation and grouping.
[Feedback presentation design](../../specs/agents/system-design/session-configuration-feedback.md#feedback-presentation)

## Acceptance

1. A model change, an intervening mode change, and a later reasoning change
   update one muted text row in the transcript. Repeating a setting updates
   its value in place. After the next turn, another picker change creates a new
   row. Reconnects preserve the rows in the current view and reloads clear them.
2. Supported silent commands render one muted receipt under their own user
   message. Positive receipts keep their expiry. Failure and uncertainty
   remain readable, with Dismiss and eligible Retry in the row.
3. The desktop picker remains usable with feedback visible. On a phone,
   the same row wraps in the transcript above the keyboard. Verify the draft,
   reading position, and 44 px targets for the row's available actions.

## ASCII UI preview

### UI-03 Muted settings row

```text
[ Transcript scrolls ]
  Assistant reply

  Changed model Sonnet, mode Plan, effort High.  (muted)

[ Composer ]
[ Model v ]   [ Mode v ]   [ Send ]
```

### UI-04 Muted slash receipt

```text
[ Transcript scrolls ]
  You: /model auto hydrafusion
  Copilot reported Auto. Fusion High requested.  (muted)

[ Composer ]
```

The existing slash receipt supplies the placement for both screens. The
phone uses its existing picker sheets and transcript scroll. For a long
failure, the reason wraps and the row's actions can wrap below it.

```text
[ Transcript scrolls ]
  Changed model Sonnet.                          (muted)
  Could not confirm mode Plan.                   (red)
  [ Dismiss ]

[ Composer ]
[ Model v ] [ Mode v ] [ Send ]
```

[Full preview](plan.md#ascii-ui-preview)

## Files likely touched

- Publish accepted picker outcomes from `session-configuration-feedback.tsx`
  and the two selectors. Remove the feedback overlay and its positioning
  and visibility timers.
- Keep rows in the session runtime slice. Project them into task chat in
  `use-chat-panel-state.ts` and use `StatusMessage` for both styles. Derive
  the group boundary from the existing accepted turn identity.
- Revise the locale copy if needed and the how-to text in
  `docs/public/tasks-and-workflows.md`.

## Verification

Start with the desktop regression `merges picker changes before the next
turn in one muted transcript row` in
`e2e/tests/chat/session-configuration-feedback.spec.ts`. Change the model
and mode, close the pickers, and advance the fake clock beyond the old
timeout. Then change reasoning. Check the transcript row count and contents
before and after sending a prompt. On the PR head the picker result is
positioned by the selector.

Cover the stable row ID, per-setting replacement, late replies, and a new
turn in the session runtime test. Cover a first launch that reaches the
picker before its turn history. In the desktop flow, save the model and
mode, start the agent, and verify that a later change appears in a new row
below the first prompt. Retain the composed Apply and Retry tests. In the
existing desktop and phone slash specs, verify the muted
row under the command. Update the phone keyboard-height flow to verify
that outcomes from separate picker sheets merge even when one is a rejection.
Check wrapping, preserved draft and reading position, and the Dismiss
target. Test that the row vanishes on reload.

```bash
(cd apps/web && pnpm run test \
  lib/state/slices/session-runtime/session-configuration-feedback.test.ts \
  components/task/session-configuration-feedback.test.tsx \
  components/task/model-selector-composed-config.test.tsx \
  components/task/model-selector-consecutive-switch.test.tsx \
  components/task/chat/messages/status-message.test.tsx \
  hooks/use-processed-messages.test.ts \
  lib/state/slices/session-runtime/purge-session.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium \
  tests/chat/session-configuration-feedback.spec.ts tests/chat/model-selector-error.spec.ts \
  -- --grep 'Picker feedback|shows inline uncertainty')
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome \
  tests/chat/mobile-session-configuration-feedback.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Dependencies and risks

Tasks 01 and 02 supply the picker evidence and slash receipts. Use the
existing transcript flow so a result cannot scroll the user away from
earlier messages. Capture turn and execution scope when a picker action
starts so a late response cannot update a later group.

## Results

The desktop regression first failed on the original build because the
transcript had no picker feedback row. The updated build passed all three
focused desktop Playwright cases and both phone cases. The phone check used
a 390 by 420 viewport and confirmed the combined row, preserved reading
position, and reachable Dismiss action.

All 83 focused web tests passed. Typecheck, changed-file Oxlint, localization,
the specification catalog, specification lint, and `git diff --check` passed.

The first-launch regression passed in the focused suite of 86 web tests.
Four desktop and two phone Playwright checks passed. The desktop startup case
confirmed the saved model and mode at launch and the new row below the prompt.
