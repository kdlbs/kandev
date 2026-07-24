---
id: "13-async-lifecycle-review-and-verification"
title: "Async lifecycle review and verification"
status: done
wave: 13
depends_on: ["10-preserve-prompt-cycle-identity", "11-account-async-subagent-completion", "12-async-subagent-browser-coverage"]
plan: "plan.md"
spec: "../../specs/fine-grained-background-running-status-indicator/spec.md"
---

# Task 13: Async lifecycle review and verification

- **Acceptance:** A test-engineer audits every prompt/subagent ordering and
  cleanup boundary; QA validates both reported user outcomes; code review finds
  no lifecycle ownership or stale-event blockers; full format, typecheck, tests,
  lint, desktop, and mobile verification pass.
- **Verification:** `make fmt`; `make typecheck test lint`; focused backend race
  suites and desktop/mobile async-subagent Playwright scenarios.
- **Files likely touched:** Only review-driven corrections and this task's
  status metadata.
- **Dependencies:** Tasks 10-12.
- **Inputs:** Code-review, test-engineer, QA, simplify, verify, and mobile-parity
  guidance.
- **Output contract:** Report coverage matrix, review/QA findings, exact full
  verification, residual provider limitations, and update only this task status.
