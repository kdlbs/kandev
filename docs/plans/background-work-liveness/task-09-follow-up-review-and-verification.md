---
id: "09-follow-up-review-and-verification"
title: "Follow-up review and verification"
status: done
wave: 9
depends_on: ["06-publish-completion-foreground-yield", "07-consolidate-session-input-mode", "08-required-behavior-coverage-audit"]
plan: "plan.md"
spec: "../../specs/fine-grained-background-running-status-indicator/spec.md"
---

# Task 09: Follow-up review and verification

- **Acceptance:** The completion publication, shared input-mode behavior, and
  coverage matrix are independently reviewed; focused QA confirms instant input
  while detached work remains and foreground precedence when a new turn starts;
  formatting, typecheck, tests, and lint pass or environment-only blockers are
  reported precisely.
- **Verification:** `make fmt`; `make typecheck test lint`; focused orchestrator
  race tests, web unit/integration tests, and relevant desktop/mobile E2E.
- **Files likely touched:** Only review-driven corrections and this task's
  status metadata.
- **Dependencies:** Tasks 06–08.
- **Inputs:** Code-review, QA, simplify, verify, and mobile-parity guidance.
- **Output contract:** Report review findings, behavior validation, exact
  commands/results, remaining risks, and update only this task status.
