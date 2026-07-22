---
id: "07-consolidate-session-input-mode"
title: "Consolidate session input mode"
status: done
wave: 7
depends_on: ["06-publish-completion-foreground-yield"]
plan: "plan.md"
spec: "../../specs/fine-grained-background-running-status-indicator/spec.md"
---

# Task 07: Consolidate session input mode

- **Acceptance:** One pure session-scoped selector derives `direct`, `queue`,
  or `unavailable`; the main composer and other actual send/queue actions use
  it consistently; `RUNNING` plus background sends directly while generating
  queues and terminal sessions remain unavailable.
- **Verification:** `cd apps && pnpm --filter @kandev/web test -- --run <focused input-mode and migrated-consumer tests>`; `cd apps/web && pnpm run typecheck`.
- **Files likely touched:**
  `apps/web/hooks/domains/session/use-session-state.ts` and tests,
  `apps/web/hooks/use-message-handler.ts`, request-changes/review-comment send
  hooks and their focused tests, plus narrowly related queue-mode consumers.
- **Dependencies:** Task 06 provides reliable live per-session activity.
- **Inputs:** Spec composer requirement; diagnosis and architecture review of
  duplicated coarse `RUNNING` checks.
- **Output contract:** Report the input-mode truth table, migrated call sites,
  RED/GREEN evidence, commands run, blockers/risks, and update only this task
  status.
- **Mobile parity:** Shared state normalization only. No layout, navigation,
  touch target, scrolling, or responsive composition changes; focused shared
  selector/consumer tests cover both desktop and mobile use of the composer.
