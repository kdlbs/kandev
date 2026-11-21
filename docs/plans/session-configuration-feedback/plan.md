---
created: 2026-11-20
status: complete
requirements:
  - REQ-AGENTS-SESSION-CONFIG-FEEDBACK-001
system_design:
  - ../../specs/agents/system-design/session-configuration-feedback.md
legacy_specs: []
---

# Session configuration feedback implementation plan

## Why the current feedback is confusing

The picker returns `ok` whether the agent changed a live setting or Kandev
only saved it. A slash command is sent as a prompt. If the agent updates a
setting without replying in text, chat treats the silent turn as empty.
[Issue #5376](https://github.com/kdlbs/kandev/issues/5376)

## Change

1. Return the evidence from the existing picker request. Show a passive,
   one-second confirmation beside the picker. Give failures a readable,
   dismissible result outside the trigger's hit area.
2. For a recognized silent configuration command, include its outcome in
   the existing turn completion. Show it as a system message beneath
   the command in place of the generic empty-turn hint.

The existing mutation order, task chat controls, and conversation delivery
remain the entry points. Scope the change to structured task chat on desktop
and phone. Use plain text for provider details and existing locale catalogs
for Kandev copy. [Requirements](../../specs/agents/requirements/session-configuration-feedback.md)
· [System design](../../specs/agents/system-design/session-configuration-feedback.md)

| Source | Evidence for the notice |
| --- | --- |
| Any ACP picker | The correlated config response or existing mode result reports the effective value. A bare acknowledgement leaves it unconfirmed. |
| Plain `/model <id>` on any ACP agent | The ID matches an advertised model, and an attributable update during that turn reports the change. Claude can use this path when its bridge reports the switch. |
| Codex ACP `/plan` | The validated command action identifies the target. A matching provider update in that prompt supplies the observed mode. |
| Copilot ACP `/model auto hydrafusion` | An attributable transition to Auto permits a receipt. Fusion High remains the requested tier. An unchanged or ambiguous snapshot yields an unconfirmed notice. |

## ASCII UI preview

The conversation owns the scroll area. Picker feedback is anchored to the
existing controls. On phone, text wraps within the keyboard viewport and
the Dismiss target is at least 44 px.

### UI-01 Picker feedback

Select a setting in task chat. The check appears as soon as the trigger is
visible. The user can immediately reopen the picker.

```text
Desktop, confirmed for one second
                         +-----------+
                         | [OK] Plan |
                         +-----------+
[ Model: Sonnet v ]   [ Mode: Plan v ]   [Send]

Phone, picker closed after rejection
      +-----------------------+
      | [X] High is           |
      |     unavailable.      |
      |             [Dismiss] |
      +-----------------------+
[ Model v ]   [ Mode v ]   [Send]
```

`[OK]` is a green check. `[X]` is red. The sketch offsets both indicators
to show which trigger they belong to. Trigger, Send, and input targets
remain available.

### UI-02 Silent-command feedback

Send the command from the task chat composer.

```text
[ Conversation scrolls ]
You
  /model auto hydrafusion

System
  Copilot reported Auto.
  Fusion High requested.

[ Composer ] [ Existing controls ]
```

These views cover ACs .1 through .6. The System receipt is a temporary row
in the same turn. Unconfirmed outcomes stay readable until dismissed or
superseded. The wording is illustrative.

## Verification

- Task 01 covers effective values, saves, partial failure, and stale picker
  responses for ACs .1 through .3 and .5.
- Task 02 covers the same slash-command path with a plain model ID and
  Copilot's extra arguments. It covers ordinary empty turns, stale
  completions, and replay for ACs .4 and .5.
- Both tasks run focused desktop and phone Playwright checks for AC .6.
  Picker checks reopen the control while the check is visible. Command
  checks assert that the system message is in the conversation turn.
- `make build`, the Vite and e2e web builds, typecheck, web lint, i18n checks,
  focused Go tests, changed-package Go lint, and docs validation passed.
  Focused frontend tests passed 139 cases. Desktop Playwright passed 11 tests;
  phone Playwright passed all 3 picker, command-receipt, and empty-turn tests.
  The phone picker case verified the long rejection, 44 px dismiss target,
  retained draft, unchanged reading position, and a 420 px keyboard viewport.

## Work orders

- [x] [Task 01 Picker outcomes and selector feedback](task-01-picker-outcomes.md)
- [x] [Task 02 Configuration slash-command feedback](task-02-slash-command-feedback.md)

## Review follow-up

- [x] Fence picker replies, state writes, rollback, and Retry by current session and execution.
- [x] Require attributable slash-command evidence and deduce command targets from validated actions.
- [x] Separate live results, persistence failures, explicit rejection, and ambiguous transport outcomes.
- [x] Gate receipts on the initiating user command and prevent replay or duplicate reappearance.
- [x] Preserve the strict behavior of existing error-only agentctl setters.
- [x] Keep composed Apply and Retry aligned with effective values and show readable partial failures.
- [x] Correct feedback colors, passive success hit area, phone viewport fitting, and visible-time expiry.

## Risk

A provider response or update can acknowledge less than the user requested.
The notice can only confirm what that specific request or turn establishes.
