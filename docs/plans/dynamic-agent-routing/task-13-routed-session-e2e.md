---
id: "13-routed-session-e2e"
title: "Routed session E2E"
status: pending
wave: 10
depends_on: ["09-routed-chat-presentation", "11-core-routing-observability"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 13: Routed session E2E

- **Acceptance:** Add isolated desktop and mobile Playwright projects for
  multi-provider routing. Prove one-tab fallback, immutable badges, localized
  route rows, capability replacement, waiting actions, stale-generation
  rejection, restart recovery, and retained composer state. Use explicit
  project matchers for every multi-provider spec in this package and exclude those
  paths from the default Chromium and mobile projects. The desktop project owns
  the task, rollout, workflow, utility, and Office files named in the plan. The
  mobile project owns the two phone files named there.
- **Files likely touched:** `apps/web/e2e/playwright.config.ts`,
  `apps/web/e2e/tests/task/dynamic-agent-routing.spec.ts`,
  `apps/web/e2e/tests/task/mobile-dynamic-agent-routing.spec.ts`, and focused
  dynamic-routing fixtures, helpers, or page-object methods.
- **Dependencies:** Tasks 09 and 11.
- **Parallelism:** parallel-safe with Task 15. This task owns E2E configuration
  and routing specs. Task 15 owns only public documentation.
- **Inputs:** Spec One logical chat, Route and capability events, API surface,
  Persistence guarantees, and route-action scenarios, Tasks 09 and 11, `/e2e`,
  `/mobile-parity`, current routing project isolation rules.
- **Output contract:** Report the dedicated project matchers, baseline reset,
  RED and GREEN runs, discovered test counts, files changed, exact commands and
  results, cleanup, blockers, risks, and synchronized task and plan status.
- **Verification:** `cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run --project dynamic-routing tests/task/dynamic-agent-routing.spec.ts && pnpm e2e:run --project dynamic-routing-mobile tests/task/mobile-dynamic-agent-routing.spec.ts`
- **Risks:** Each restart must restore baseline environment state. The desktop
  project must not match the mobile file, and neither spec can run in Chromium's
  default worker. Project matchers must also reserve the caller and Office specs
  owned by Tasks 14 and 16 without matching profile-settings specs.

## Results

Not started in this implementation slice. The isolated routing Playwright
projects and restart/recovery scenarios remain pending.
