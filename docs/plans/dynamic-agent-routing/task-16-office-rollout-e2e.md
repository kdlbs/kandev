---
id: "16-office-rollout-e2e"
title: "Office rollout E2E"
status: pending
wave: 11
depends_on: ["10-office-routing-handoff", "13-routed-session-e2e"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 16: Office rollout E2E

- **Acceptance:** Prove desktop and phone Office identities can select concrete
  or dynamic execution profiles. Prove legacy routing rows remain unread. Cover
  `office=on` with dynamic routing off, persisted dynamic restart, re-enable
  reconciliation, and concrete Office execution.
- **Files likely touched:**
  `apps/web/e2e/tests/office/dynamic-agent-execution-profile.spec.ts`,
  `apps/web/e2e/tests/office/mobile-dynamic-agent-execution-profile.spec.ts`,
  `apps/web/e2e/tests/task/dynamic-agent-routing-rollout.spec.ts`, and focused
  Office or runtime-flag E2E helpers matched by the dedicated routing projects.
- **Dependencies:** Tasks 10 and 13.
- **Parallelism:** sequential. Office and rollout scenarios share restart and
  feature-flag fixture state.
- **Inputs:** Spec Use in Office, Delivery and rollout, Persistence guarantees,
  and legacy-row and flag scenarios, Tasks 10 and 13, `/e2e`, `/mobile-parity`,
  and current Office fixtures.
- **Output contract:** Report the flag matrix, mobile selector outcome,
  legacy-row evidence, RED and GREEN runs, discovered test counts, files
  changed, exact commands and results, cleanup, blockers, risks, and
  synchronized task and plan status.
- **Verification:** `cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run --project dynamic-routing tests/office/dynamic-agent-execution-profile.spec.ts tests/task/dynamic-agent-routing-rollout.spec.ts && pnpm e2e:run --project dynamic-routing-mobile tests/office/mobile-dynamic-agent-execution-profile.spec.ts`
- **Risks:** Every restart must restore baseline environment state. Poll exact
  session state before lifecycle assertions. Re-enabling the flag must reconcile
  durable state without a duplicate launch.

## Results

Not started in this implementation slice. Office selector, legacy-row, flag
matrix, and mobile rollout E2E coverage remains pending with Task 10.
