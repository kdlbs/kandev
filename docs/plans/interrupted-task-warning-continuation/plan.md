---
created: 2026-11-25
status: completed
requirements:
  - REQ-TASKS-INTERRUPTED-TASK-INDICATOR-001
system_design:
  - ../../specs/tasks/system-design/interrupted-task-indicator.md
legacy_specs: []
---

# Keep the restart warning until work resumes

## Overview

On November 25, startup recovery restored seven interrupted conversations.
Each original prompt had an unknown outcome. One affected session still waited
for a new message after its task had lost the warning.

`markRecoveryResolvedForAttempt` cleared `interrupted_at` at agent boot. The
sidebar displayed its ordinary waiting status from the updated task. Reproduce
the gap by restoring a session that was running at restart without sending
another prompt. The current code clears the task flag when the agent boots.

## Scope

### In scope

- Keep the warning through prompt-free restoration and unsuccessful attempts.
- Resolve it when the agent accepts a prompt for the interrupted session.
- Preserve the warning for other interrupted sessions on the same task.
- Cover the desktop sidebar and phone task switcher in the existing restart
  scenarios. Update the public recovery paragraph with the implementation.

### Out of scope

Prompt replay policy, automatic recovery eligibility, historical backfill,
restart mitigation, new task states, and new warning controls.

## Technical approach

Use the existing task marker projection and shared icon. Store the affected
session and interruption token in private task metadata. Add and remove scoped
entries with the existing archive and recovery ownership guards. Preserve the
published scalar timestamp and its legacy read path.

Separate warning resolution from boot recovery. Snapshot the marker at prompt
admission. Resolve it from the existing accepted-dispatch callback. Use the
replacement launch's initial-prompt callback when that launch carries the
prompt. Publish the resulting task status through `task.updated`.

| Prompt path | Evidence for the task warning |
| --- | --- |
| ACP or Codex app-server, direct or queued | Current execution acknowledges a prompt for the captured session |
| Prompt carried by a replacement launch | Current replacement accepts its initial prompt |
| Passthrough with a tracked prompt | Existing acknowledged dispatch for the captured session |
| Prompt-free restoration or unknown dispatch | Keep the warning |

## ASCII UI preview

UI-01 is the task row after the conversation is available. It covers
AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.3, .4, .6, and .8.

```text
                             Desktop sidebar    Phone task switcher
After restore, current       [status] Fix sync  [status] Fix sync
After restore, proposed      /!\ Fix sync       /!\ Fix sync
After accepted prompt        [status] Fix sync  [status] Fix sync
```

The triangle uses the existing amber color and localized "Interrupted by
restart" label. The status slot stays before the task title. After acceptance,
the row shows its current task or agent state. The drawing shows order only.

On phone, open the task switcher from the task header. Its header stays fixed
and its existing list owns scrolling. Tap the task row to open the conversation.
The triangle is a status indicator. It adds no separate touch action.

## Tests

Extend `TestSessionStartKeepsInterruptedMarkerUntilRecovery` in
`event_handlers_streaming_test.go`. The first failing assertion must show that a
correlated boot preserves the task warning. Use `TestTaskInterruptionPromptAcceptance`
for accepted, rejected, queued, stale, and sibling dispatches. Exercise the real
repository with scoped marker addition, removal, legacy fallback, and deletion
on SQLite and PostgreSQL. [Work order](task-01-keep-warning-until-continuation.md)

## E2E tests

Extend the existing ordinary restart case in
`automatic-restart-recovery.spec.ts` and
`mobile-automatic-restart-recovery.spec.ts`. The shared fixture gives the
session a running turn before restart. After automatic restoration, the visible
row still shows the triangle. Reload preserves it. An accepted follow-up clears
it on the open client, and the original prompt remains interrupted.

Use Chromium for the workbench sidebar and mobile-chrome for the task switcher.
The actual icon and its accessible name prove AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.3,
.4, .5, .6, and .8.

## Work orders

- [x] [Task 01 - Keep the warning until continuation](task-01-keep-warning-until-continuation.md)

Task 01 delivers one vertical change. Its exact Go, Playwright, and documentation
commands cover the production paths.

## Related delivery records

The [earlier warning work order](../orphaned-session-open-recovery/task-03-interruption-warning.md)
records the shift from STARTING to boot readiness. This package changes the
clearing condition from boot readiness to prompt acceptance.

## Verification results

Design checks passed on 2026-11-25.

- `python3 scripts/list-docs.py validate` validated 464 decisions and 2114 specs.
- `python3 scripts/lint-spec-files.test.py` passed all 36 tests.
- `python3 scripts/lint-spec-files.py --all` passed.
- The PR documentation coverage preflight found one work order with full
  requirement and design coverage.
- The diff whitespace and new Markdown link checks passed.
- Playwright discovery found one matching ordinary restart case per project.

Runtime checks passed on 2026-11-25; see the work order for the command results.

## Risks

- A delayed acceptance callback can target an older interruption. The write
  must compare the entry token and the session incarnation.
- Clearing a whole task marker can hide a second interrupted session. The
  repository must update the session entries and scalar together.
- The test must check the warning after the follow-up completes. A spinner
  can temporarily cover a warning that still exists.
