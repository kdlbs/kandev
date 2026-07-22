---
id: "08-required-behavior-coverage-audit"
title: "Required behavior coverage audit"
status: done
wave: 8
depends_on: ["06-publish-completion-foreground-yield", "07-consolidate-session-input-mode"]
plan: "plan.md"
spec: "../../specs/fine-grained-background-running-status-indicator/spec.md"
---

# Task 08: Required behavior coverage audit

- **Acceptance:** A test-engineer maps every relevant spec behavior to an
  assertion at the correct layer; verifies the suite covers normal completion
  without provider idle, exactly-once live publication, foreground precedence,
  background instant-send, concurrent-session isolation, terminal/unavailable
  sessions, HTTP admission, WebSocket/store propagation, migrated send/queue
  actions, and desktop/mobile user outcomes; and adds or assigns any missing
  regression coverage without relying on helper-only assertions.
- **Verification:** Run the focused backend, frontend, and browser tests selected
  by the completed matrix; preserve deterministic event ordering and avoid
  timing-only race assertions.
- **Files likely touched:** Focused `*_test.go`, `*.test.ts`, and existing
  desktop/mobile busy-signal E2E specs only when a real coverage gap remains.
- **Dependencies:** Tasks 06 and 07 provide the integrated behavior to audit.
- **Inputs:** Spec state hierarchy and live-propagation sections; existing
  orchestrator activity, session-state, message-routing, WebSocket-store, and
  busy-signal tests.
- **Output contract:** Report the behavior-to-test matrix, missing or misleading
  prior coverage, tests added or delegated, exact commands/results, residual
  gaps, and update only this task status.
