---
id: "01-event-delivery-and-startup"
title: "Repair event delivery and startup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/automations-pr-merged-trigger.md"
---

# Task 01: Repair Event Delivery and Startup

## Intent

Make merged-PR detection work with both supported event buses and preserve an outage merge through
the complete start-up consumer chain.

## Acceptance

- Typed in-memory and JSON-decoded NATS payloads produce the same firing data.
- Malformed payloads fail closed without listing triggers.
- Starting without a lookup still attaches the subscription; wiring the lookup later makes the next
  event work.
- The orchestrator subscribes to automation events before the merged-PR subscriber starts, and both
  are ready before the GitHub poller's immediate sweep.
- Shutdown stops the producer before its consumers.

## TDD sequence

1. Add subscriber tests that deserialize an event through the JSON wire shape and show the current
   type assertion drops it; add malformed and typed-payload controls.
2. Add a late-lookup test that starts unwired, wires the lookup, publishes, and currently stays inert.
3. Normalize payloads and make `Start` subscribe independently of lookup availability.
4. Add a behavioral backendapp start-order test using fakes or a narrow extracted start helper. Assert
   event delivery/readiness, not source line order.
5. Move starts to immediately after successful orchestrator start and register reverse cleanup.

## Files likely touched

- `apps/backend/internal/automation/github_pr_merged_subscriber.go`
- `apps/backend/internal/automation/github_pr_merged_subscriber_test.go`
- `apps/backend/internal/backendapp/main.go`
- a focused `apps/backend/internal/backendapp/*_test.go` start-up test

## Dependencies

None.

## Parallelism

`sequential` — this task establishes event delivery before mutation behavior is exercised end to end.

## Verification

- `cd apps/backend && go test ./internal/automation ./internal/backendapp`

## Risks

- Do not weaken per-event fail-closed lookup gates while allowing late wiring.
- Do not start the poller from two lifecycle locations after moving the block.
- Keep generic NATS envelope behavior unchanged.

## Output contract

Report RED/GREEN evidence, files changed, focused test results, and any lifecycle residuals. Update
this task and `plan.md` status in the same conversation.

## Completed validation

- RED: `TestGitHubPRMergedSubscriberAcceptsJSONDecodedPayload` and
  `TestGitHubPRMergedSubscriberLateLookupRecovers` failed against the original subscriber.
- GREEN: the new subscriber normalization and late-wiring behavior passed their focused tests.
- GREEN: `TestStartOrchestratorAndAutomationConsumersOrder` and its orchestrator-failure control pass.
- GREEN: `cd apps/backend && go test ./internal/automation ./internal/backendapp` (517 tests).

## Files changed

- `apps/backend/internal/automation/github_pr_merged_subscriber.go`
- `apps/backend/internal/automation/github_pr_merged_subscriber_test.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/startup_order_test.go`
