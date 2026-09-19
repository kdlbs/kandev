---
spec: docs/specs/ui/requirements/ci-pr-automation.md
created: 2026-06-18
status: building
---

# Implementation Plan: CI PR Automation Controls

## Overview

Add task-level PR automation options to the existing GitHub PR CI popover. The backend owns durable options, per-PR dedupe/checkpoints, default prompt resolution, and automation execution from existing PR watch events. The frontend adds API/state/hooks and renders controls plus a task-specific prompt editor in the existing desktop popover and mobile drawer. E2E verifies the visible controls and the automation behavior against mocked PR states.

The 2026-07-23 extension adds agent prompts for review requested, merged,
and closed transitions. Per ADR-0051, it expands this subsystem and its
existing menu/MCP boundaries rather than introducing generic task-bound GitHub
query automations.

The approved 2026-07-26 refinement keeps that architecture and closes the
remaining product gaps: connected-account rebinding, safe server-owned
lifecycle prompts, lifecycle delivery errors in the shared UI, archived-task
stop semantics, and accurate Review Watch cleanup copy. Workflow-step
destinations and GitLab parity remain follow-up work.

The final security remediation makes lifecycle prompt text immutable and
server-owned. It interpolates only a validated canonical PR URL, rejects HTTP
and MCP lifecycle overrides, clears and ignores legacy override columns, and
linearizes lifecycle queue acceptance with archival.

The final review remediation extends that archival guard through every queued
lifecycle retry. It also makes the lifecycle session claim explicit, restores a
claimed session's prior state after dispatch failure, preserves a same-pass
auto-fix or auto-merge error after lifecycle evaluation, and removes legacy
lifecycle-override fields from the frontend contract. The completed claim path
also reconciles the task to `IN_PROGRESS` and publishes session `RUNNING` before
delivery, classifies a zero-row guarded claim by re-reading active state, and
closes a rollback turn only when the current dispatch created it. Lifecycle
queue rows now use reserve/ack delivery: they remain durable through dispatch
until PTY/executor prompt acceptance, while ordinary queue rows keep destructive
dequeue behavior. Executor failures use captured lifecycle rollback state.
Passthrough lifecycle reservations defer until the ready guard releases and
then enter that same dispatcher. SQLite-backed task cleanup purges persistent
queue rows and advances generations in the enclosing task transaction; only a
registered/fallback ephemeral mirror receives a post-commit purge callback.
Workspace cascades perform that persistent cleanup for every captured task in
their one transaction and notify the ephemeral mirror once per task afterward.
Reserved agent/workflow/server ownership blocks client mutation, and a
per-session in-process guard prevents concurrent duplicate dispatch. Privileged
archive/delete cleanup purges reserved rows and advances a task-queue generation
so stale accepted, reserved, or in-flight work cannot reappear after unarchive.
This extends the existing queue without a new schema or subsystem.

The 2026-07-29 UI refinement keeps all task-wide controls functional while
grouping the three lifecycle prompt switches in a shared desktop/mobile
`PR events` disclosure. Auto-fix and auto-merge remain primary, and the
disclosure opens whenever one of its options is enabled.

The 2026-08-10 composer-tray refinement surfaces those lifecycle options next
to the existing auto-fix and auto-merge badges without adding three more pills.
One task-wide `PR events N/3` badge summarizes enabled lifecycle prompts, the
accessible chip description names them, and the status row wraps complete
controls under width pressure instead of overflowing the document.

---

## Backend

### GitHub persistence and models

Files:

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/store_test.go` or focused `store_ci_automation_test.go`

Add durable CI automation option and per-PR state models:

- `TaskCIAutomationOptions`
- `TaskCIAutomationOptionsPatch`
- `TaskCIPRAutomationState`
- `TaskCIPRAutomationOptionsResponse`

Add tables:

- `github_task_ci_options`
- `github_task_ci_pr_state`

Store methods:

- `GetTaskCIOptions(ctx, taskID string) (*TaskCIAutomationOptions, error)`
- `UpdateTaskCIOptions(ctx, taskID string, patch TaskCIAutomationOptionsPatch) (*TaskCIAutomationOptions, error)`
- `ListTaskCIPRStates(ctx, taskID string) ([]*TaskCIPRAutomationState, error)`
- `GetTaskCIPRState(ctx, taskID, repositoryID string, prNumber int) (*TaskCIPRAutomationState, error)`
- `RecordTaskCIFixAttempt(ctx, state TaskCIFixAttempt) error`
- `RecordTaskCIMergeAttempt(ctx, state TaskCIMergeAttempt) error`
- `RecordTaskCIError(ctx, taskID, repositoryID string, prNumber int, message string) error`
- `ClearTaskCIError(ctx, taskID, repositoryID string, prNumber int) error`

The store returns default disabled options when no options row exists.

### Default prompt

Files:

- `apps/backend/config/prompts/ci-auto-fix.md`
- `apps/backend/config/prompts/embed.go`
- `apps/backend/internal/prompts/store/sqlite.go`
- `apps/backend/internal/prompts/service/service.go`
- Prompt store/service tests under `apps/backend/internal/prompts/...`

Seed a built-in prompt:

- `id = "builtin-ci-auto-fix"`
- `name = "ci-auto-fix"`
- `builtin = true`

Add prompt service resolution by name with embedded fallback so automation can resolve the current default prompt even if the database row is missing.

### GitHub API

Files:

- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/github/handlers.go` if a websocket action is added
- `apps/backend/pkg/websocket/actions.go` if a websocket action/event is added
- `apps/backend/internal/github/controller_test.go`

Add HTTP routes under `/api/v1/github`:

- `GET /tasks/:taskId/ci-options`
- `PATCH /tasks/:taskId/ci-options`

Response shape follows the spec. The response includes the effective prompt and current per-PR automation state for linked PRs.

Optionally add websocket push:

- `github.task_ci_options.updated`

### Automation execution

Files:

- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/messagequeue/types.go` if metadata constants are needed
- `apps/backend/internal/github/service_pr.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/orchestrator/event_handlers_github_test.go`
- New focused orchestrator test file if needed, e.g. `event_handlers_github_ci_automation_test.go`

Subscribe the orchestrator to PR state events that can drive automation:

- `events.GitHubTaskPRUpdated`
- existing `events.GitHubPRFeedback`

Keep the 1-minute poller lightweight. Full `GetPRFeedback` should happen only after a candidate status/review state is observed.

Add pure helpers for:

- auto-fix candidate detection
- merge-readiness detection
- fix checkpoint/signature creation
- merge readiness signature creation
- feedback delta extraction against the last checkpoint
- prompt rendering variables

Auto-fix sends the rendered prompt through `PromptTask` when possible or `QueueMessageWithMetadata` when the session is busy. Auto-merge calls `MergePR(ctx, owner, repo, number, "")` and records attempt/error state.

### PR lifecycle reliability refinement

Files:

- `apps/backend/config/prompts/pr-review-requested.md`
- `apps/backend/config/prompts/pr-merged-final.md`
- `apps/backend/config/prompts/pr-closed-final.md`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/service_ci_automation.go`
- `apps/backend/internal/github/store_ci_automation_test.go`
- `apps/backend/internal/github/service_ci_automation_test.go`
- `apps/backend/internal/github/service_task_events_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_pr_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_pr_automation_test.go`

Keep the existing lifecycle evaluator inside
`handleTaskPRCIAutomationWithRefresh`; do not add another poller, event type,
goroutine, or in-flight map.

Add an atomic store operation such as
`RebindTaskPRReviewer(ctx, taskID, login) (changed bool, err error)` that:

- compares the saved `review_reviewer_login`;
- updates it only when the task workspace's connected GitHub login changed; and
- resets `review_request_initialized` / `last_review_requested` for every
  `github_task_ci_pr_state` row owned by the task in the same transaction,
  without changing terminal or CI checkpoints.

Expose that operation through the GitHub service. Review-request evaluation
resolves the task workspace's automation credential and authenticated login
before loading the per-PR checkpoint; it never falls back to the service's
ambient client. A changed identity rebinds and then establishes a quiet
baseline; auth failure does not mutate the saved login or checkpoints.

Route lifecycle evaluation and delivery failures through
`RecordTaskCIError`, publish the refreshed `github.task_ci_options.updated`
payload, and leave the lifecycle edge unstamped. After a lifecycle prompt is
accepted or durably queued, call `ClearTaskCIError`, persist the lifecycle
checkpoint, and publish the refreshed options so open clients remove the
error. Preserve the existing primary-session selection, busy-session queue,
task/repository/PR/event coalesce key, and no-new-session behavior.

Replace the three embedded lifecycle prompt bodies with the immutable
server-owned templates from the spec, interpolating only a validated canonical
PR URL. Add regression coverage proving archived/deleted task events prune PR
watches and cannot leave lifecycle evaluation active; no new archive
implementation is expected unless the existing guard proves insufficient.

---

## Frontend

### Types and API client

Files:

- `apps/web/lib/types/github.ts`
- `apps/web/lib/api/domains/github-api.ts`
- `apps/web/lib/api/domains/github-api.test.ts`

Add types:

- `TaskCIAutomationOptions`
- `TaskCIAutomationOptionsPatch`
- `TaskCIPRAutomationState`

Add API functions:

- `getTaskCIAutomationOptions(taskId, options?)`
- `updateTaskCIAutomationOptions(taskId, patch, options?)`

### State and hook

Files:

- `apps/web/lib/state/slices/github/types.ts`
- `apps/web/lib/state/slices/github/github-slice.ts`
- `apps/web/lib/state/slices/github/github-slice.test.ts`
- `apps/web/hooks/domains/github/use-task-ci-options.ts`
- `apps/web/hooks/domains/github/use-task-ci-options.test.tsx`
- `apps/web/lib/ws/handlers/github.ts` if websocket updates are added

Add task-keyed automation option state with loading/saving/error information. Add a hook that loads the options when a PR popover needs them and provides update/reset helpers.

### Popover controls

Files:

- `apps/web/components/github/pr-ci-popover.tsx`
- `apps/web/components/github/multi-pr-ci-popover.tsx`
- `apps/web/components/github/pr-status-chip.tsx`
- New `apps/web/components/github/pr-automation-controls.tsx`
- New `apps/web/components/github/pr-automation-controls.test.tsx`
- Existing `apps/web/components/github/pr-ci-popover.test.tsx`
- Existing `apps/web/components/github/pr-status-chip.test.tsx`

Render automation controls in `PRCIPopover` after status/review rows and before the existing manual merge button. The prompt editor must work in the desktop hover card and the mobile drawer, with reset-to-default support and disabled/error states.

The automation section includes:

- An info icon/help affordance explaining both toggles, the 1-minute lightweight PR watch cadence, candidate-only full feedback fetches, feedback snapshots/dedupe, and auto-merge readiness gates.
- An edit button for the task-specific auto-fix prompt.
- A prompt editor dialog/drawer that links to Settings > Prompts so users can edit the default `ci-auto-fix` prompt.

No separate changes should be needed in `apps/web/components/task/chat/chat-input-area.tsx` or `apps/web/components/task/passthrough-toolbar.tsx` if controls live inside the shared `PRCIPopover`.

### Lifecycle labels, errors, and cleanup copy

Files:

- `apps/web/components/github/pr-ci-automation-controls.tsx`
- `apps/web/components/github/pr-ci-popover.automation.test.tsx`
- `apps/web/components/github/review-watch-dialog.tsx`
- a focused Review Watch dialog/component test if needed

Use the visible switch label `Your review is requested` with on-demand help
(hover tooltip on fine pointers, tap popover on touch, plus a screen-reader
description) clarifying it covers any new request, including re-review after
changes. Keep merged and closed as separate compact switches sharing the same
style of on-demand explanation that both wake the agent when review work ends;
no inline descriptions or visible group header, so the rows stay single-line.
Extend the existing automation
help text with workspace-connected-account scope and the quiet first baseline.
Do not add lifecycle prompt edit buttons; the existing edit action remains
explicitly auto-fix-only.

Resolve the selected PR's `TaskCIPRAutomationState` with the existing
`findCIAutomationStateForPR` helper and render a compact, accessible error row
when `last_error` is non-empty. The row lives inside
`PRCIAutomationControls`, so desktop hover popovers, passthrough surfaces, and
the mobile drawer share one implementation.

Update the Review Watch `Auto (recommended)` description to state that user
engagement or enabled PR lifecycle prompts retain a terminal review task.
`Always delete` remains the explicit override.

#### Mobile design contract

- **Desktop outcome:** the existing PR hover popover shows the same switches
  and selected-PR error row.
- **Mobile entry point:** tap the existing PR status chip.
- **Nearest shipped exemplar:** `PRStatusChipDrawer` in
  `apps/web/components/github/pr-status-chip.tsx`; retain its inset bottom
  drawer, fixed header, `max-h-[80vh]`, and single `min-h-0 overflow-y-auto`
  body. `apps/web/components/kanban/mobile-menu-sheet.tsx` remains the curated
  safe-area/internal-scroll geometry reference.
- **Hierarchy and action:** status first, automation controls next, inline
  error adjacent to those controls; toggling a switch remains the primary
  action.
- **Shared versus mobile-specific:** option state, labels, error derivation,
  and mutations stay shared in `PRCIAutomationControls`; only the existing
  desktop popover/mobile drawer shells differ.
- **Geometry:** add no nested scroll owner or fixed control. Existing switch
  rows and drawer close action retain their touch targets and safe-area
  behavior.
- **Proof:** the desktop and `mobile-chrome` PR automation specs assert the
  connected-account label and visible error without horizontal overflow.

### Prompt settings

Files:

- `apps/web/components/settings/prompts-settings.tsx`
- `apps/web/app/settings/prompts/page.tsx`
- Existing prompt settings tests if present

The settings page should list the seeded built-in `ci-auto-fix` prompt through the existing prompt list API. Add tests only if the UI needs explicit handling for the new built-in prompt.

### Composer tray automation summary

Files:

- `apps/web/components/github/pr-status-chip.tsx`
- `apps/web/components/github/pr-status-automation-badges.tsx`
- `apps/web/components/github/pr-status-automation-badges.test.tsx`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/src/locales/en/github.json`
- `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-ci-chip.spec.ts`

Extend the task-wide automation flags already derived by `PRStatusChip` with
the three lifecycle booleans. Render one translated `PR events N/3` badge when
at least one is enabled; keep the existing `Auto-fix N/10` and `Auto-merge`
badges independent. The count is visual compression, not the complete
accessible contract: the chip's accessible description names each enabled
event using the established switch labels.

Use the same summary in the single-PR and multi-PR chip branches because the
options are task-wide. Do not add a badge when all lifecycle booleans are false
or while no options have loaded, and do not create another click target. The
surrounding PR chip continues to open `PRCIPopover` or `MultiPRCIPopover` on
fine pointers and the existing `PRStatusChipDrawer` surface on touch.

Allow `ChatStatusBar` to wrap complete top-level controls when its row cannot
fit. Preserve the existing right-control group and keep each PR chip internally
intact so automation labels do not split or become horizontally scrollable.
The document must not gain horizontal overflow at the configured phone
viewport or a narrow desktop window.

#### Mobile design contract

- **Desktop outcome:** the composer tray shows auto-fix, auto-merge, and one
  grouped lifecycle badge; hovering/focusing the existing PR chip still opens
  the full automation popover.
- **Mobile entry point:** tap the existing PR status chip in the composer tray.
- **Nearest shipped exemplar:** `PRStatusChipDrawer` in
  `apps/web/components/github/pr-status-chip.tsx`; retain its current inset
  bottom drawer, fixed header, `max-h-[80vh]`, and single internal scroll body.
- **Hierarchy and primary action:** PR/CI status remains first, followed by
  compact active-automation badges. The whole chip remains the one primary tap
  target; detailed switches stay in the drawer.
- **Surface rationale:** the badge is glanceable task status, while three
  independently editable switches are deeper temporary controls that already
  fit the existing drawer. A second drawer or nested pill menu would add an
  unnecessary interaction step.
- **Geometry:** the composer tray owns wrapping but not scrolling; the drawer
  remains the only automation-detail scroll owner. No new fixed control or
  safe-area behavior is introduced.
- **Shared versus mobile-specific:** automation derivation, count, translated
  labels, accessible description, and badge markup remain shared. Only the
  existing popover/drawer shells vary by pointer and viewport.
- **Proof:** focused component tests cover zero-to-three lifecycle counts and
  single/multi-PR parity; desktop and `mobile-chrome` E2E cover all active
  badges, drawer reachability, tray containment, and absence of document-level
  horizontal overflow.

---

## Tests

- **What:** default options read disabled and persist partial updates.
  **File:** `apps/backend/internal/github/store_ci_automation_test.go`
  **How:** real SQLite store tests.

- **What:** built-in `ci-auto-fix` prompt is seeded and resolvable with fallback.
  **File:** prompt store/service tests under `apps/backend/internal/prompts/`
  **How:** real SQLite prompt repository plus service resolver test.

- **What:** GET/PATCH CI options API returns defaults, effective prompt, per-PR states, and reset-to-default behavior.
  **File:** `apps/backend/internal/github/controller_test.go`
  **How:** HTTP controller test with service/store test fixture.

- **What:** auto-fix no-ops when disabled and queues one prompt when enabled for a failing PR.
  **File:** `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
  **How:** orchestrator unit tests with mock GitHub service and fake queue/prompt behavior.

- **What:** repeated same feedback snapshot does not duplicate prompts.
  **File:** `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
  **How:** table-driven checkpoint/signature tests plus event handler test.

- **What:** new or materially changed feedback after a checkpoint produces a new prompt containing only the new/changed items.
  **File:** `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
  **How:** pure delta helper tests and one handler test.

- **What:** auto-merge calls merge only for ready PRs and throttles repeated failed attempts.
  **File:** `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
  **How:** table-driven readiness tests plus mock merge call assertions.

- **What:** frontend API functions call the expected endpoints and serialize reset-to-default payloads.
  **File:** `apps/web/lib/api/domains/github-api.test.ts`
  **How:** fetch mock tests.

- **What:** frontend hook loads/saves options and handles errors.
  **File:** `apps/web/hooks/domains/github/use-task-ci-options.test.tsx`
  **How:** React hook tests with mocked API.

- **What:** popover renders controls, toggles options, opens prompt editor, and resets override.
  **File:** `apps/web/components/github/pr-automation-controls.test.tsx`
  **How:** component tests with mocked hook/API state.

- **What:** automation help affordance explains cadence, watched states, snapshots, and merge gates.
  **File:** `apps/web/components/github/pr-automation-controls.test.tsx`
  **How:** component test activates the info icon and asserts the explanatory content is visible.

- **What:** task prompt editor links to default prompt settings.
  **File:** `apps/web/components/github/pr-automation-controls.test.tsx`
  **How:** component test opens the editor and asserts the Settings > Prompts link points at the prompt settings page.

- **What:** mobile drawer renders automation controls without overflow.
  **File:** `apps/web/components/github/pr-status-chip.test.tsx` or E2E.
  **How:** mobile viewport render test or Playwright screenshot/assertions.

- **What:** changing authenticated GitHub account A to B atomically updates the
  saved login and resets every linked PR review baseline without changing
  terminal checkpoints.
  **File:** `apps/backend/internal/github/store_ci_automation_test.go` and
  `apps/backend/internal/github/service_ci_automation_test.go`
  **How:** real SQLite transaction test plus mock-client service test.

- **What:** account rebinding followed by a currently requested review
  establishes a quiet baseline rather than prompting.
  **File:** `apps/backend/internal/orchestrator/event_handlers_github_pr_automation_test.go`
  **How:** lifecycle decision/handler test with two linked PR states.

- **What:** lifecycle delivery failure persists `last_error`, publishes updated
  options, leaves the edge retryable, and successful retry clears the error.
  **File:** `apps/backend/internal/orchestrator/event_handlers_github_pr_automation_test.go`
  **How:** fake prompt dispatcher/message queue plus captured event-bus payloads.

- **What:** lifecycle defaults contain only the agreed event facts.
  **File:** `apps/backend/internal/github/service_ci_automation_test.go`
  **How:** effective-prompt assertions against embedded fallbacks.

- **What:** archive/delete prunes task PR watches so lifecycle automation cannot
  reactivate the task.
  **File:** `apps/backend/internal/github/service_task_events_test.go`
  **How:** real SQLite watch rows plus task event bus handlers.

- **What:** lifecycle dispatch runs turn-start/runtime/model preparation before
  a final active-task claim under the cancel guard and current queue token. The
  claim has explicit claimed, busy, and inactive outcomes; `IDLE` and
  `WAITING_FOR_INPUT` primary sessions deliver immediately; busy/transient
  failures and active tasks without a promptable session retain one durable
  retry; only missing/inactive tasks discard it; a newly selected task session
  receives the requeued event; and reset, supersede, visible-message persistence
  failure, or dispatch failure after a claim restores pre-claim state before
  retrying without creating a duplicate visible chat message. Message
  persistence failure also completes the dispatch-created turn before retry and
  prevents any executor prompt for that attempt. Executor handoff failure uses
  the captured lifecycle rollback state rather than a newly observed state.
  **File:** `apps/backend/internal/orchestrator/event_handlers_queue_test.go`
  **How:** deterministic claim barriers, failure injection, and queue/message
  assertions.

- **What:** ordinary queued messages retain destructive dequeue, while
  lifecycle rows remain durable from `ReserveHead` until
  `AcknowledgeByID` after PTY/executor acceptance. Agent/workflow/server queue
  ownership is reserved from client mutation; one per-session dispatch guard
  prevents concurrent in-process duplicates. Restart before acknowledgement
  redelivers, and the external-acceptance-before-ack crash window is explicitly
  at-least-once. Returned reservation and retry copies strip the persisted
  in-flight marker and carry only process-local reservation evidence, so a
  failed post-restart attempt becomes visible again.
  **File:** `apps/backend/internal/orchestrator/messagequeue/`,
  `apps/backend/internal/orchestrator/handlers/queue_handlers.go`, and
  `apps/backend/internal/orchestrator/event_handlers_queue_test.go`
  **How:** repository/service/handler ownership tests, reserve-without-delete
  assertions, restart-style redelivery, acknowledgement ordering, and
  concurrent drain coverage.

- **What:** archive/delete uses privileged queue cleanup that purges reserved
  lifecycle rows as well as unreserved rows. Task-queue generation invalidates
  accepted, reserved, and in-flight lifecycle work, preventing a stale retry
  from being reinserted or delivered after a later unarchive.
  **File:** `apps/backend/internal/orchestrator/event_handlers_queue_test.go`,
  `apps/backend/internal/orchestrator/messagequeue/`, and task archive/delete
  handlers.
  **How:** deterministic archive-before-requeue and archive-after-reservation
  schedules with no sleeps, including an archive/unarchive generation check.

- **What:** lifecycle evaluation cannot clear an auto-fix or auto-merge error
  produced in the same CI automation pass.
  **File:** `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
  **How:** lifecycle-success regression with a deferred shared-error write.

- **What:** frontend CI automation types expose lifecycle configuration as
  booleans only and omit legacy lifecycle prompt override fields.
  **File:** `apps/web/lib/api/domains/github-api.test.ts`
  **How:** request/response contract assertions.

- **What:** shared automation controls render the connected-account label and
  selected PR `last_error`; lifecycle rows have no prompt-edit affordance.
  **File:** `apps/web/components/github/pr-ci-popover.automation.test.tsx`
  **How:** component test with mocked task options and multiple per-PR states.

- **What:** Review Watch cleanup copy explains lifecycle retention and the
  Always override.
  **File:** focused Review Watch dialog/component test
  **How:** render the dialog and assert the selected policy description.

- **What:** the PR status chip renders no lifecycle badge for zero enabled
  lifecycle options, one grouped `PR events N/3` badge for one through three
  enabled options, and an accessible description that names the enabled
  events in both single- and multi-PR branches.
  **File:** `apps/web/components/github/pr-status-automation-badges.test.tsx`
  **How:** table-driven component cases using task-wide automation options.

- **What:** the composer status row can wrap complete controls without
  splitting the PR chip or changing its existing click target.
  **File:** `apps/web/components/task/chat/chat-input-area.test.tsx` when a
  meaningful component assertion is possible; otherwise prove geometry in the
  focused E2E scenarios below.
  **How:** render/class contract assertion or browser geometry, not a snapshot.

---

## E2E Tests

- **Scenario:** GIVEN a task with a linked open PR, WHEN the user opens the CI popover, THEN all five automation controls are visible.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
  **What to verify:** desktop hover/popover and mobile drawer both expose controls.

- **Scenario:** GIVEN a task with a linked open PR, WHEN the user enables auto-fix and auto-merge, THEN the settings persist after reload.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
  **What to verify:** checkbox/switch state after reload.

- **Scenario:** GIVEN a task uses the default prompt, WHEN the user customizes and then resets the task prompt, THEN the UI shows override state and returns to default state.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
  **What to verify:** prompt editor save/reset behavior.

- **Scenario:** GIVEN the CI automation section is visible, WHEN the user opens the help affordance, THEN the popover/drawer explains watch cadence, candidate feedback fetches, snapshots, and merge gates.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
  **What to verify:** help content is visible on desktop and does not overflow on mobile.

- **Scenario:** GIVEN the task auto-fix prompt editor is open, WHEN the user clicks the default prompt settings link, THEN the app navigates to Settings > Prompts.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
  **What to verify:** link target/navigation for the default prompt settings page.

- **Scenario:** GIVEN auto-fix is enabled and the mocked PR gets a failing check, WHEN the backend automation handler runs, THEN one auto-fix message is sent or queued and duplicate events do not create duplicates.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts` or backend integration test if E2E cannot observe queue state cleanly.
  **What to verify:** visible queued/chat message or backend mock assertion.

- **Scenario:** GIVEN auto-merge is enabled and the mocked PR becomes ready, WHEN the backend automation handler runs, THEN the merge endpoint is called and UI updates to merged.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
  **What to verify:** mock merge call and merged banner/state.

- **Scenario:** GIVEN a linked PR has a runtime automation error, WHEN the user
  opens the desktop PR popover, THEN the connected-account lifecycle label and
  selected-PR error are visible.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
  **What to verify:** shared controls render the exact label and an accessible
  error row for the selected PR.

- **Scenario:** GIVEN the same state on a phone viewport, WHEN the user taps the
  PR status chip, THEN the drawer shows the same label/error, keeps one internal
  vertical scroll owner, and does not create document horizontal overflow.
  **File:** `apps/web/e2e/tests/pr/mobile-ci-automation-options.spec.ts`
  **What to verify:** visible error, label, scrollable drawer containment, and
  `document.documentElement.scrollWidth <= clientWidth`.

- **Scenario:** GIVEN all five PR automations are enabled, WHEN the user views
  the desktop composer tray and opens the PR chip, THEN `Auto-fix N/10`,
  `Auto-merge`, and one `PR events 3/3` badge are visible and the existing
  detailed controls remain reachable.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
  **What to verify:** grouped badge text, one badge instance, accessible event
  names, and unchanged popover entry.

- **Scenario:** GIVEN all five PR automations and other composer-tray controls
  are visible on the configured phone viewport, WHEN the user views the tray
  and taps the PR chip, THEN complete controls fit or wrap, the drawer opens,
  and neither the tray nor document overflows horizontally.
  **File:** `apps/web/e2e/tests/pr/mobile-pr-ci-chip.spec.ts`
  **What to verify:** grouped badge count, tap outcome, tray/control bounding
  boxes, and `scrollWidth <= clientWidth`.

---

## Implementation Waves

Wave 1 (sequential foundations):

- [x] [task-01-backend-persistence-prompts](task-01-backend-persistence-prompts.md)

Wave 2 (backend contracts and behavior):

- [x] [task-02-backend-ci-options-api](task-02-backend-ci-options-api.md)
- [x] [task-03-backend-automation-execution](task-03-backend-automation-execution.md)

Wave 3 (frontend integration):

- [x] [task-04-frontend-api-state-hook](task-04-frontend-api-state-hook.md)
- [x] [task-05-frontend-popover-controls](task-05-frontend-popover-controls.md)

Wave 4 (original end-to-end coverage):

- [x] [task-06-e2e-ci-automation](task-06-e2e-ci-automation.md)

Wave 5 (PR lifecycle agent prompts):

- [x] [task-08-pr-lifecycle-agent-prompts](task-08-pr-lifecycle-agent-prompts.md)

Wave 6 (approved lifecycle refinement, parallel):

- [x] [task-09-backend-lifecycle-reliability](task-09-backend-lifecycle-reliability.md)
- [x] [task-10-frontend-lifecycle-feedback](task-10-frontend-lifecycle-feedback.md)

Wave 7 (after backend/frontend behavior):

- [x] [task-11-lifecycle-e2e](task-11-lifecycle-e2e.md)
- [x] [task-12-lifecycle-public-docs](task-12-lifecycle-public-docs.md)

Wave 8 (review remediation, sequential):

- [x] [task-13-lifecycle-state-review-remediation](task-13-lifecycle-state-review-remediation.md)
- [x] [task-14-lifecycle-delivery-review-remediation](task-14-lifecycle-delivery-review-remediation.md)
- [x] [task-15-final-review-remediation](task-15-final-review-remediation.md)
- [x] [task-16-ci-error-precedence](task-16-ci-error-precedence.md)
- [x] [task-17-final-lifecycle-transition-coverage](task-17-final-lifecycle-transition-coverage.md)

Wave 9 (final integration):

- [x] [task-07-qa-verify-and-docs](task-07-qa-verify-and-docs.md)

Wave 10 (final lifecycle security remediation):

- [x] [task-18-lifecycle-prompt-security-remediation](task-18-lifecycle-prompt-security-remediation.md)

Wave 11 (final review remediation):

- [x] [task-19-final-review-remediation](task-19-final-review-remediation.md)

Wave 12 (PR events presentation):

- [x] [task-20-role-aware-automation-controls](task-20-role-aware-automation-controls.md)

Wave 13 (composer-tray PR events summary):

- [x] [task-21-composer-tray-pr-events-badge](task-21-composer-tray-pr-events-badge.md)

---

## Verification Commands

Targeted backend:

```bash
make -C apps/backend test
```

Targeted frontend:

```bash
cd apps && pnpm --filter @kandev/web test -- pr-ci-popover.automation
cd apps && pnpm --filter @kandev/web test -- pr-status-chip pr-status-automation-badges
cd apps && pnpm --filter @kandev/web i18n:check
cd apps/web && node --max-old-space-size=4096 node_modules/typescript/bin/tsc --noEmit
```

Targeted E2E:

```bash
cd apps/web && pnpm e2e:run tests/pr/ci-automation-options.spec.ts tests/pr/mobile-ci-automation-options.spec.ts
cd apps/web && pnpm e2e:run tests/pr/ci-automation-options.spec.ts -- --grep "composer tray"
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/pr/mobile-pr-ci-chip.spec.ts -- --grep "PR event automations"
```

Final verification:

```bash
make fmt
make typecheck test lint
```

---

## Verification Results

Task 21 completed on 2026-08-10 with Red-Green-Refactor evidence. The focused
component suite first failed because `pr-status-pr-events-chip` was absent, and
the compact desktop and Pixel 5 browser scenarios then failed with computed
`flex-wrap: nowrap`. After implementation and refactor:

- `pnpm --filter @kandev/web test -- pr-status-chip
  pr-status-automation-badges` passed 47 tests in 3 files.
- `pnpm run i18n:check && pnpm run i18n:ratchet` passed; the pseudo catalog is
  synchronized and real-locale gaps remain advisory under the existing English
  fallback policy.
- Targeted ESLint passed with zero findings for the changed frontend and E2E
  files.
- `node --max-old-space-size=4096 node_modules/typescript/bin/tsc --noEmit`
  passed. The package script's default 2 GB heap had exhausted when it ran in
  parallel with other checks, so the successful isolated run used 4 GB.
- The managed production-build compact desktop scenario passed 1 test in the
  Chromium project, including badge content, accessible event names, complete
  target geometry, popover reachability, and horizontal containment.
- The final `--no-build` Pixel 5 scenario reused that unchanged production
  build and passed 1 test in `mobile-chrome`, including tray containment,
  drawer reachability, and all five independent switches.
- A Pixel 5 screenshot was captured at
  `apps/web/.pr-assets/mobile-pr-ci-chip--mobile-pr-pr-events-automation-tray.png`,
  inspected for density and legibility, then removed with the other ignored
  capture artifacts after inspection.

---

## Risks

- The auto-fix path must not fetch full PR feedback for every watched PR every minute. Candidate detection must stay lightweight until full details are necessary.
- Prompt dedupe must be durable in the database, not dependent on in-memory caches.
- Auto-merge must fail closed. The backend readiness predicate should be stricter than the frontend display predicate when data is unknown.
- Hover-card UI must remain usable with interactive controls; if hover dismissal fights editing, the controls may need a nested dialog or click-pinned state.
- Connected-account rebinding must reset every linked PR's review baseline in
  the same database transaction. A login change must never be interpreted as a
  false-to-true review request.
- `last_error` is shared per task/repository/PR. Lifecycle changes must reuse
  the existing error contract without inventing a parallel error store, and
  websocket refreshes must keep desktop/mobile state current.
- Archive/delete is a hard stop. Neither polling recovery nor session fallback
  may recreate a task PR watch for an archived/deleted task.
- Lifecycle prompt templates are immutable server-owned release content. The
  only dynamic value is a validated canonical PR URL; untrusted GitHub text and
  caller-supplied lifecycle prompt content must never reach an agent turn.
- Lifecycle queue acceptance and prompt claim must be conditional on an active
  task. Archive/delete wins leave no checkpoint, message, or prompt; acceptance
  wins are canceled through normal archive semantics.
- The same active-task condition applies to every lifecycle retry, not merely
  its initial queue acceptance. Busy and transient failures retain one durable
  coalesced retry, as does an active task with no currently promptable session.
  Only a missing or inactive task discards the event, including after a later
  unarchive.
- Final task-level session resolution may select a different session than the
  one that originally accepted the event. The event must move as one durable
  coalesced retry to the newly selected session before the source reservation
  is acknowledged; it must not dispatch through the stale session or be lost.
- Lifecycle dequeue is reserve/ack rather than destructive: the queue row
  survives until PTY/executor prompt acceptance. The per-session dispatch guard
  prevents concurrent duplicates inside one process, but a restart before
  acknowledgement redelivers. A crash after external acceptance and before
  acknowledgement may duplicate the prompt; this at-least-once boundary is
  preferred to silent loss.
- Agent-, workflow-, and server-owned queue entries are backend-reserved.
  Browser/MCP clients may create and mutate only user-owned entries and cannot
  spoof a reserved owner to rewrite, append to, cancel, or delete lifecycle
  work.
- A lifecycle entry records its visible user message only after the final
  guarded active-task claim succeeds. Turn-start, runtime resume, and model
  preparation can occur first; reset or queue-token supersession after the
  claim restores the prior state and requeues without leaving a stale message.
- Failure to persist that post-claim visible message restores the prior session
  state, completes any turn created for the attempted dispatch, and requeues
  before any executor prompt is sent. Task-state rollback is a compare-and-set
  from `IN_PROGRESS`, so a concurrent terminal transition or archive wins.
- Executor handoff failure must use the lifecycle dispatch's captured rollback
  state, not state read after the handoff attempt, so retry cleanup cannot
  clobber an intervening state transition.
- Archive/delete privileged queue cleanup must include reserved lifecycle rows.
  A task-queue generation must invalidate stale accepted, reserved, and
  in-flight lifecycle work, so no cancellation or retry path can revive it
  after a later unarchive.
- Passthrough lifecycle reservations must leave the ready-handler guard before
  entering the normal lifecycle dispatcher; direct PTY delivery is restricted
  to ordinary queue entries so it cannot bypass final lifecycle claim and
  acknowledgement semantics.
- A supplied SQLite queue is purged and generation-advanced inside the task
  archive/delete or workspace-cascade transaction. Only registered or fallback
  ephemeral mirrors receive a post-commit notification, exactly once for every
  task captured by a workspace cascade; a persistent queue must not receive a
  duplicate callback purge.
- A current primary lifecycle session in either `IDLE` or
  `WAITING_FOR_INPUT` is immediately eligible; it must not be misclassified as
  inactive or unnecessarily queued.
- Lifecycle evaluation may clear a resolved lifecycle error, but a shared
  auto-fix or auto-merge error from the same CI automation pass must be
  persisted after lifecycle evaluation so it remains visible.
- Lifecycle summary is task-wide even when rendered beside one selected PR.
  Single- and multi-PR chips must derive one count from task options, never
  multiply the badge by linked PR count or selected-PR state.
- The compact visual count is not sufficient accessible detail. The PR chip's
  accessible description must name every enabled lifecycle event, and the
  translated badge must remain concise enough to keep the existing compact
  composer chrome usable.
- Wrapping the composer status row must preserve complete interactive targets
  and the existing right-control group. It must not introduce horizontal
  scrolling, split one PR chip across lines, or hide automation controls.
- The product currently trusts any client that can reach the Kandev backend as
  an operator, and auto-fix intentionally passes provider feedback to a
  privileged task agent. HTTP authentication/capability tokens and
  least-privilege execution of untrusted provider text are repo-wide security
  follow-ups rather than lifecycle-specific changes.

## Required PR handoff notes

When this branch is opened as a PR, the body must explicitly note:

- lifecycle prompt text is immutable and server-owned; HTTP and current-task
  MCP expose only the lifecycle booleans, while the UI edits only auto-fix
  prompt text;
- destination workflow steps and GitLab lifecycle parity are follow-up work;
  and
- the feature reuses the existing one-minute task PR poller and does not add a
  scheduler.

Residual follow-up suggestion: the immutable lifecycle templates currently
exist as embedded prompt files and as hard-coded orchestrator text. Consolidate
them behind one server-owned source only if their wording needs to evolve; this
is non-blocking because both paths preserve the accepted immutable prompt
contract.

## Open Questions

- None.
