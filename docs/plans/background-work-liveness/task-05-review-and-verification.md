---
id: "05-review-and-verification"
title: "Review and verification"
status: completed
wave: 5
depends_on: ["01-acp-detached-lifecycle", "02-session-activity-ownership", "03-client-state-contract", "04-realistic-e2e-coverage"]
plan: "plan.md"
spec: "../../specs/fine-grained-background-running-status-indicator/spec.md"
---

# Task 05: Review and verification

- **Acceptance:** Changed code is simplified and reviewed; formatting,
  typecheck, unit tests, lint, targeted desktop/mobile E2E, and ACP regression
  tests pass; any provider signal limitation is explicitly documented.
- **Verification:** `make fmt`; `make typecheck test lint`; `cd apps/web && pnpm e2e:run tests/chat/busy-signal.spec.ts tests/chat/mobile-busy-signal.spec.ts`.
- **Files likely touched:** only review-driven corrections and plan/task status
  metadata.
- **Dependencies:** Tasks 01–04.
- **Inputs:** code-review, simplify, verify, QA, and docs-maintainer guidance.
- **Output contract:** Report review findings/fixes, exact commands and results,
  remaining risks, and mark all plan/task status fields done.
