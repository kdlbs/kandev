---
status: draft
system: agents
created: 2026-11-24
owners:
  - kandev
---

# Cursor ACP Shell Watch Requirements

A Cursor shell watch observes one or more shells. Users need its state in
Background Work while the Cursor turn runs. The agents system owns the watch
identity and state. The [shared Background Work requirements](background-work.md)
own the viewer.

## Requirements

### REQ-AGENTS-BACKGROUND-WORK-011: Cursor ACP shell watch inspection

**Intent:** Let users follow a Cursor shell watch from active work to History.

- **AC-AGENTS-BACKGROUND-WORK-011.1:** When Background Work is enabled and Cursor
  confirms that a registered watch is active, the viewer shall count it as one
  Monitor. Its details shall show the supplied watch reason and shell references.
  The record shall retain its original task and session. It shall keep the origin
  turn when the turn is known.
- **AC-AGENTS-BACKGROUND-WORK-011.2:** When Cursor completes the matching watch,
  only that watch shall leave the running count and appear in History. Duplicate
  or delayed progress shall preserve the terminal record. Each update shall
  apply only to the matching watch in its original session and execution.
- **AC-AGENTS-BACKGROUND-WORK-011.3:** If the feature is disabled, the provider is
  different, or the call lacks valid watch identity or shell references, the
  call shall appear in the transcript only.
- **AC-AGENTS-BACKGROUND-WORK-011.4:** When users inspect a Cursor watch on desktop
  or phone, the shared viewer shall indicate that output is unavailable. Workload
  controls and shell exit status shall be absent. The watch status describes the
  watch itself.
- **AC-AGENTS-BACKGROUND-WORK-011.5:** While a Cursor turn is active, the composer
  shall continue to queue follow-up messages, including during a watch.
- **AC-AGENTS-BACKGROUND-WORK-011.6:** When Cursor reports a matching watch as
  failed, that watch shall appear as Failed in History. A sibling watch shall
  remain in its current state, and delayed progress shall not reopen the failed
  watch.
- **AC-AGENTS-BACKGROUND-WORK-011.7:** After a successful prompt response confirms
  cancellation and queued notifications are processed, each still-running watch
  from that prompt shall appear as Interrupted. Explicit terminal updates shall
  remain unchanged, and the watch shall not report a shell exit status.
- **AC-AGENTS-BACKGROUND-WORK-011.8:** When the native session binding is
  successfully replaced, each still-running watch without terminal evidence
  shall appear as Unknown. Existing terminal records shall remain unchanged, and
  a successor watch with the same provider call ID shall have an independent
  record.

## Out of scope

- Shell controls and output capture.
- Cursor foreground handoff and same-turn input.

## Related contracts

- [Cursor ACP shell watch design](../system-design/cursor-shell-watches.md)
- [Shared Background Work lifecycle and viewer](background-work.md)
- [Queue admission](../../tasks/requirements/queue-admission.md)
- [Issue #5485 and captured notifications](https://github.com/kdlbs/kandev/issues/5485)
