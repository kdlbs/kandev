# Threads navigation E2E consolidation

The first four desktop Threads cases each launched the same standard mock
agent, waited for its primary session to settle and exercised a short
navigation check. Keep the first case and move the other three original titles
into executed named steps. Twelve scenarios now run in nine cases.

The keeper still enters through the Kanban card, opens Threads through the
view toggle, checks the live task title and Ready for review indicator, and
excludes the never-started task. It then verifies the active toggle and Home
preference after navigation, explicitly loads `/threads`, opens the full task
through the column button, and returns through the task's Open in Threads
button. The task-specific URL, visible focused column, absent return button
and cleared focus after clicking the composer remain asserted. Three repeated
agent launches and page fixtures are removed; the real primary-session setup
and all route interactions remain.

The eight other cases remain separate: width allocation, saved-view persistence,
deletion confirmation, sort explanations, cursor indication, stable reply
placement, empty state and viewport-bounded session switching. Mobile cases,
shared helpers, retries, timeouts and product code are unchanged. The relevant
contract remains [Threads Conversation Deck](../specs/ui/requirements/threads-conversation-deck.md).

## Verification and limits

Both complete file runs use the same local runtime image and existing builds,
one worker, tracing and zero retries. The baseline passes 12 cases in 107.038
summed seconds; the candidate passes nine in 87.831 seconds (17.94% locally).
Blob reports verify every original full scenario identity as either a retained
case or one of the three successfully executed steps. Logs, source snapshot
and reports are retained in ignored `e2e-focus/threads-*`.

Hosted run 38439998051 previously spent 180 seconds retrying the full-task case
after its setup Kanban card detached. The complete local baseline did not
reproduce that failure. The keeper still performs the same initial Kanban
entry; this consolidation is not a diagnosis or claimed fix for that failure,
and the timed-out attempt is excluded from the local speed comparison.
The subsequent [fixture isolation investigation](kanban-fixture-column-isolation-2026-10-13.md)
identified a persisted hidden-column leak and verifies its separate repair.
The complete hosted E2E target remains unverified and unmet.
