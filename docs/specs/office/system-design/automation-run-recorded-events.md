---
status: current
system: office
requirements:
  - REQ-OFFICE-AUTOMATION-RUNS-001
created: 2026-11-24
owners:
  - kandev
---

# Automation Run-Recorded Events

## Purpose and boundaries

This design maps AC-OFFICE-AUTOMATION-RUNS-001.23 of
[automation runs](../requirements/automation-runs.md). The Automations page and
an automation's history page re-read runs on the shared live-refresh cadence
only while a run is open. A run recorded while nothing was open, such as one
from the GitHub poller, a schedule, or a webhook, stayed invisible until the
reader navigated or reloaded. Issue
[#5464](https://github.com/kdlbs/kandev/issues/5464) testing surfaced the gap.

The automation service owns the event because it owns run rows. The WebSocket
gateway owns delivery and workspace scoping. The run pages own refresh. Status
changes of open runs keep the existing polling contract in
[automation runs](automation-runs.md); this design adds no status stream.

## Backend

- `Store` holds an optional run-recorded observer. It is called after a run row
  is committed by `CreateRun`, `CreateRetryAdmission`, and
  `FinalizeRetryFailure` when that call inserts the next attempt.
  `Service.admitWebhookReceipt` calls it after its own insert commits.
- `NewService` installs the observer. It reads the automation's workspace and
  publishes `events.AutomationRunCreated` (`automation.run.created`) with
  `workspace_id`, `automation_id`, `run_id`, and `status`. A lookup or publish
  error is logged at warn level and never fails the write.
- `gateway/websocket` subscribes to the subject and sends the
  `automation.run.created` notification with the same four fields through
  `BroadcastToWorkspaceOrDrop`. An event without a workspace is dropped. The
  payload never carries prompts, trigger data, or titles.

## Frontend

- `lib/ws/handlers/automation-runs.ts` handles the notification and, when the
  workspace is the active one, emits it on the module-level
  `automationRunRecorded` channel in `lib/automation-run-recorded.ts`.
- `useAutomationRunRecorded(workspaceId, automationId, refresh)` subscribes to
  the channel and calls the page's existing `refresh` once per burst, after a
  300 ms trailing debounce. An `automationId` limits it to that automation.
- `RunsListPage` uses it for the workspace. `AutomationDetailPage` uses it for
  its automation. The refresh reuses each page's request-order guards, so a
  pushed refresh cannot publish an older response over a newer one. After the
  refresh sees an open run, the existing live-refresh cadence takes over.

## Failure modes

| Situation | Behavior |
| --- | --- |
| The WebSocket is disconnected when the run is recorded | No event arrives. The existing foreground refresh and navigation reads recover the page. |
| Several runs are recorded in one poll | The debounce coalesces them into one refresh. |
| A client lacks access to the workspace | The hub drops the notification for that client. |
| The workspace lookup fails | No event is published; the page recovers as above. |

## Tests

- Store tests prove that each insert path calls the observer once after
  commit, and that a failed insert does not.
- A gateway test proves workspace scoping and the four-field payload.
- Web tests prove that the handler ignores other workspaces, that the hook
  debounces and filters by automation, and that both pages refresh on an
  event while no run is open.
