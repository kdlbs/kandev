---
id: "04-realistic-e2e-coverage"
title: "Realistic E2E coverage"
status: done
wave: 4
depends_on: ["03-client-state-contract"]
plan: "plan.md"
spec: "../../specs/fine-grained-background-running-status-indicator/spec.md"
---

# Task 04: Realistic E2E coverage

- **Acceptance:** The mock background scenario closes its foreground turn before
  its workload; desktop and mobile tests observe background-working plus
  instant-send during that interval; a foreground reply temporarily takes
  orange precedence; done appears only after workload completion.
- **Verification:** `cd apps/web && pnpm e2e:run tests/chat/busy-signal.spec.ts tests/chat/mobile-busy-signal.spec.ts`.
- **Files likely touched:** `apps/backend/cmd/mock-agent/handler.go` and tests,
  `apps/web/e2e/tests/chat/busy-signal.spec.ts`, and
  `mobile-busy-signal.spec.ts`.
- **Dependencies:** Tasks 01–03.
- **Inputs:** existing `/background` test path demonstrates the old blind spot
  and should remain available for held-open-turn coverage.
- **Output contract:** Report the RED failure, GREEN run, desktop/mobile
  outcomes, artifacts/blockers, and mark this task plus its plan checkbox done.

## Result

The new `/detached-background` mock command returns the foreground response,
leaves a Claude-shaped async workload alive, and emits a later
task-notification lifecycle boundary. The first browser run caught the chat
status row's coarse-state dependency; after fixing that consumer, all four
desktop tests and both Pixel 5 tests pass, including foreground precedence and
reload hydration.
