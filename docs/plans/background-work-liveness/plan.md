---
spec: docs/specs/fine-grained-background-running-status-indicator/spec.md
created: 2026-07-21
status: in_progress
---

# Implementation Plan: Background work liveness

## Overview

Correct the busy signal by separating foreground ownership, launch-tool
completion, and launched-workload completion. The adapter first exposes the
detached workload lifecycle, the orchestrator then retains it across foreground
turn completion, and the existing frontend indicators consume the corrected
session/task aggregate. The final E2E scenario must end the foreground turn
before the background workload ends; the current mock holds one turn open and
therefore cannot catch the reported false-done regression.

The ACP evidence for the design is reproducible with `acpdbg`: Claude ACP 0.60
returns a terminal Bash tool update containing
`_meta.claudeCode.toolResponse.backgroundTaskId` and an output-file path, while
the provider explicitly promises a later task notification. Async subagents use
the parallel `agentId` / `outputFile` launch metadata. The human-origin
`usage_update` arrives after the foreground reply even when `session/prompt`
remains held open for that subagent; the eventual autonomous completion cycle
ends with `_meta._claude/origin.kind=task-notification`. A terminal launch-tool
frame therefore cannot be treated as terminal evidence for either foreground
idleness or detached-work completion.

## Backend

### ACP detached-work lifecycle

- Extend `apps/backend/internal/agentctl/types/streams/agent.go` with
  protocol-neutral foreground-yield and detached-background completion events.
  Keep tool-card status unchanged:
  a Bash or Task launch can be complete while its workload remains live.
- In `apps/backend/internal/agentctl/server/adapter/transport/acp/`, recognize
  Claude's human-origin and task-notification-origin usage boundaries. Retain
  the originating top-level tool-call registration across prompt completion.
- Convert human-origin usage completion into an explicit foreground-yield event
  and task-notification-origin usage completion into one workload-completion
  event. Claude ACP does not expose the completed task ID on the latter frame,
  so the orchestrator retires one outstanding registration per notification;
  an ambiguous remainder stays live rather than falsely showing done. Monitor
  notification routing continues updating its own correlated tool card.
- Prompt-end active-tool sweeps continue closing launch cards but do not invent
  detached-work completion. Agent execution teardown drains any remaining
  registrations and publishes terminal cleanup.

### Session activity state machine

- Refactor `apps/backend/internal/orchestrator/turn_activity.go` so foreground
  ownership and background registrations are independent. The read order is:
  prompt claim/top-level foreground activity → generating; otherwise any
  background registration → background; otherwise idle/default.
- Replace the turn-close reset in
  `apps/backend/internal/orchestrator/service.go::completeTurnForSession` with a
  foreground-idle transition. Reserve full activity deletion for execution
  stop, cancellation/teardown, and session removal paths.
- Handle detached-background lifecycle events separately from tool-card
  terminal updates. A launch-tool completion must not remove its workload;
  an ID-bearing workload completion removes its registration, while an
  uncorrelated provider completion retires one session registration and fails
  closed when ambiguity remains.
- Preserve atomic prompt admission: a foreground claim always wins over
  background liveness, concurrent claims admit exactly one prompt, and a failed
  claim restores background-idle only when its generation/epoch is still
  current.

### DTO, aggregate, and live updates

- Update session DTO enrichment so `foreground_activity=background` may be
  emitted for a settled coarse session when detached work remains. Unknown
  activity on a `RUNNING` session still defaults to generating.
- Update task most-active-wins aggregation to rank generating `RUNNING`
  sessions first, then every session with live background work, then existing
  settled-state behavior.
- Publish `session.activity_changed` and task refreshes for the foreground-turn
  close → background and final background → idle transitions, including after
  the coarse session has left `RUNNING`.

## Frontend

### Shared session flags and store transitions

- In `apps/web/hooks/domains/session/use-session-state.ts`, derive background
  work from `foreground_activity` independently of coarse `RUNNING` state.
  `isAgentBusy` remains foreground-only; `isWorking` includes background work.
- Audit `session.state_changed` and `session.activity_changed` reducers so a
  coarse settled transition does not erase a concurrently supplied background
  value, and final liveness completion clears the working affordance.
- Keep the existing indicators and layout. This is shared state/data
  normalization, so desktop and mobile use the same view model with no new
  composition, touch, navigation, or responsive behavior.

## Tests

- **Foreground precedence:** background + foreground output/claim reads
  generating and queues a second simultaneous prompt. File:
  `apps/backend/internal/orchestrator/foreground_busy_signal_test.go`.
- **Turn outlived:** turn completion with a registered detached workload reads
  background and stays promptable; only workload completion reads idle. Files:
  `foreground_busy_signal_test.go`,
  `foreground_activity_signal_test.go`.
- **Launch versus workload terminal:** terminal Bash/async-Task launch updates
  preserve detached liveness; human-origin usage yields the foreground;
  task-notification-origin usage retires one workload; unknown or duplicate
  notifications are harmless. Files under
  `apps/backend/internal/agentctl/server/adapter/transport/acp/` and
  `foreground_busy_signal_test.go`.
- **Task aggregate:** a non-`RUNNING` session with live background work outranks
  done, while any generating session outranks background. Files:
  `apps/backend/internal/task/service/task_activity_test.go` and DTO tests.
- **Frontend flags/store:** settled+background is working but not busy;
  `RUNNING`+generating is busy; foreground always wins; final completion is
  idle. Files: `use-session-state.test.ts` and relevant session WS slice tests.

## E2E Tests

- Add a mock-agent command that launches background work, completes its
  foreground turn immediately, emits a later terminal notification, and does
  not hold the prompt RPC open.
- Update `apps/web/e2e/tests/chat/busy-signal.spec.ts` to assert, after the
  foreground turn settles, that the green background spinner remains, the
  composer sends immediately rather than queues, a new foreground prompt shows
  orange for its duration, and the indicator returns to background until the
  detached workload ends.
- Update `apps/web/e2e/tests/chat/mobile-busy-signal.spec.ts` with the same
  foreground-ended/background-live outcome at Pixel 5 width. No mobile layout
  contract changes; this covers the shared state normalization already rendered
  by the existing mobile surface.

## Implementation Waves

Wave 1:

- [x] [Task 01: ACP detached-work lifecycle](task-01-acp-detached-lifecycle.md)

Wave 2:

- [x] [Task 02: Session activity ownership](task-02-session-activity-ownership.md)

Wave 3:

- [x] [Task 03: Client state contract](task-03-client-state-contract.md)

Wave 4:

- [x] [Task 04: Realistic E2E coverage](task-04-realistic-e2e-coverage.md)

Wave 5:

- [x] [Task 05: Review and verification](task-05-review-and-verification.md)

Wave 6:

- [x] [Task 06: Publish completion-time foreground yield](task-06-publish-completion-foreground-yield.md)

Wave 7:

- [x] [Task 07: Consolidate session input mode](task-07-consolidate-session-input-mode.md)

Wave 8:

- [x] [Task 08: Required behavior coverage audit](task-08-required-behavior-coverage-audit.md)

Wave 9:

- [ ] [Task 09: Follow-up review and verification](task-09-follow-up-review-and-verification.md)
