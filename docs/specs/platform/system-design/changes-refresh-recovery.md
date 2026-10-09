---
status: current
system: platform
created: 2026-10-01
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
---

# Changes Refresh Recovery Design

## Purpose and boundaries

Platform owns the finite Git read attempts and their recovery schedule.
UI owns the [toolbar presentation](../../ui/system-design/changes-loading-feedback.md).
This extension replaces the manual-retry requirement after a failed Changes refresh.
Backend observation, subprocess admission, source authority, and wire contracts remain unchanged.
The existing refresh implementation remains the baseline until the follow-up work order is complete.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| 001.25, 001.26, 001.28, 001.29 | Attempt and state ownership |
| 001.27, 001.31, 001.35, 001.37 | Lifecycle and admission |
| 001.33, 001.36, 001.38 | Delayed recovery |

## Attempt and state ownership

Extend `hooks/domains/session/git-status-refresh-coordinator.ts`.
Keep `requestGitStatusRefresh`, `retainGitRefreshScope`, and their request-identity guards.
An attempt still runs `fresh`, then at most one `recover` when complete membership remains unavailable.
The existing one-shot detail `replay` retains its current deadline.
Neither attempt becomes an unbounded network request.

Recovery state belongs to the retained client/environment scope, alongside existing attempt and replay ownership.
Store one failure count and one retry timer per scope, shared by all active consumers.
Observe environment and repository refresh states, status quality, and detail quality.
Unavailable enrichment counts as recoverable read failure, including after replay settles.
Diff skip reasons such as binary or budget limits are completed outcomes, not retry triggers.

Preserve request correlation, tracker identity, revision ordering, and environment validation.
Do not clear repository failure merely because the overall request promise resolves.
A partial success keeps the remaining failed repositories recoverable.
Valid prior file rows remain visible throughout warning, retry, and recovery.
Missing membership never proves a clean workspace.

## Delayed recovery

After a finite attempt settles with recoverable unavailable state, schedule one timeout.
Use delays of 5, 10, 20, then 30 seconds for consecutive failures.
Cap subsequent delays at 30 seconds while the scope remains eligible.
Measure the delay from completion of the failed attempt, not its start.
Do not use `setInterval`, retry within render, or start another request during an active attempt.
The schedule ends on successful live recovery or loss of eligibility.

At timeout, recheck ownership, client generation, session/environment binding, connection, focus, page visibility, and current failure state.
If eligible, start or join `requestGitStatusRefresh` through the existing coordinator.
Clear the timer before starting the attempt.
When unrelated fresh work already exists, join it rather than adding another request.
After settlement, reevaluate every repository and either reset backoff or schedule the next delay.
Retain failure counts across same-scope rerenders and duplicate consumers.

Accepted ready notifications can repair state while a timeout remains scheduled.
On full recovery, cancel that timeout and reset the failure count immediately.
An older or rejected notification cannot reset recovery or hide an unavailable sibling repository.
An unresolved failed detail snapshot schedules a fresh read, because replay cannot restart unavailable enrichment.
Keep the existing explicit refresh API for other callers and diagnostics.
The Changes summary no longer requires its manual Retry control.

## Lifecycle and admission

`useSessionGitRefresh` already owns active-surface attachment and scope release.
Use the foreground event pattern in `hooks/use-foreground-refresh.ts` for reactivation.
Track visibility changes and window focus/blur explicitly for timer eligibility.
Do not require another manual panel activation after browser foreground return.
Apply the same ownership on desktop and phone, without separate presentation timers.
When the last eligible consumer leaves, cancel retry and replay timers and release owned subscriptions and attempts.
Disconnection, environment replacement, and hidden or unfocused pages cancel scheduled recovery.
On return, the existing activation path starts or joins one fresh attempt.
Late callbacks recheck the client generation and current environment before writes.

Retain the four-operation WebSocket admission limit and all tracker command budgets.
Retries request reads only. Never repeat stage, discard, commit, pull, or other Git mutations.
Do not change credentials, transports, source selection, or persisted fallback eligibility.
Keep data for healthy repositories while the failed subset recovers through the existing multi-repository endpoint.
No new backend endpoint, store schema, global polling loop, or runtime setting is necessary.

## Presentation integration

The existing refresh companion map identifies pending attempts.
Extend the Changes status projection to include unavailable detail quality without discarding complete membership.
During backoff, unavailable state drives the toolbar warning.
During an active fresh/recovery attempt, pending refresh state drives the spinner in the same position.
After full recovery, loading details can retain the spinner until enrichment settles.
All ready state removes the indicator.
The warning tooltip identifies stale prior data and failed repository names when applicable.
Desktop and phone use the same projection.
The UI design owns copy, accessibility, and toolbar geometry.

## Rationale and verification

Manual Retry interrupts the requested workflow after a transient read error.
Immediate recursive retry can overload Git and the WebSocket gateway.
One delayed timeout with capped backoff preserves bounded attempts and focuses work on visible Changes surfaces.
The requirement and this design preserve that operational rationale. A separate ADR is unnecessary.

Use fake timers and deferred responses in `git-status-refresh-coordinator.test.ts`.
Cover each delay, duplicate owners, pending attempts, partial failure, ready notifications, and rejected stale frames.
Cover last-owner release, hidden/unfocused state, disconnect, environment replacement, and reactivation.
Extend `use-session-git-refresh.test.tsx` for eligibility and hook cleanup.
Browser tests force failed responses, then allow recovery without a Retry interaction.
Record warning/spinner transitions and stable file-row geometry on desktop and phone.

## Related contracts and delivery

- [Progressive Git status](workspace-git-status.md)
- [Environment source authority](../../tasks/system-design/environment-owned-git-status.md)
- [Publication decision](../../../decisions/2026-09-30-progressive-workspace-git-refresh.md)
- [Follow-up plan](../../../plans/changes-loading-feedback/plan.md)
