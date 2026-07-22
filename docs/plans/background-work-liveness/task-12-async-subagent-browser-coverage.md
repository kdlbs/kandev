---
id: "12-async-subagent-browser-coverage"
title: "Faithful async-subagent browser coverage"
status: completed
wave: 12
depends_on: ["10-preserve-prompt-cycle-identity", "11-account-async-subagent-completion"]
plan: "plan.md"
spec: "../../specs/fine-grained-background-running-status-indicator/spec.md"
---

# Task 12: Faithful async-subagent browser coverage

- **Acceptance:** The mock/replay path reproduces async-subagent launch,
  foreground idle, same-prompt final output, prompt completion, and Claude's
  later ID-less task-notification completion; desktop and Pixel 5 tests prove
  instant send while only the child runs and no background indicator after the
  singleton fallback or execution teardown. Exact-ID completion is covered at
  the backend unit level because the provider browser path exposes no ID.
- **Verification:** Production build plus focused desktop/mobile Playwright
  busy-signal specs.
- **Files likely touched:** mock-agent async lifecycle behavior, existing
  busy-signal desktop/mobile E2E specs, and narrowly related test helpers.
- **Dependencies:** Tasks 10 and 11.
- **Inputs:** Existing shell `/detached-background` coverage and diagnosed false
  positive from synthetic ideal ordering.
- **Output contract:** Report faithful event ordering, RED/GREEN browser
  evidence, desktop/mobile results, artifacts/flakes, and update only this task
  status.
- **Mobile parity:** Shared state behavior only; no composition, navigation,
  touch, scrolling, or layout changes. Pixel 5 verifies the same user outcome.
