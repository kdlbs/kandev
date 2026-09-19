---
id: "14-caller-selection-e2e"
title: "Caller selection E2E"
status: pending
wave: 11
depends_on: ["07-utility-profile-integration", "13-routed-session-e2e"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 14: Caller selection E2E

- **Acceptance:** Prove workflow and utility selectors pass one logical dynamic
  profile ID and receive routed concrete attribution. Prove a utility fallback
  returns one result without creating a task tab.
- **Files likely touched:**
  `apps/web/e2e/tests/workflow/dynamic-agent-profile.spec.ts`,
  `apps/web/e2e/tests/settings/dynamic-utility-profile.spec.ts`, and focused E2E
  helpers already matched by the dedicated routing project.
- **Dependencies:** Tasks 07 and 13.
- **Parallelism:** sequential.
- **Inputs:** Spec Use in Kanban and workflows and Use by utility agents, Tasks
  07 and 13, `/e2e`, and current workflow and utility fixtures.
- **Output contract:** Report caller attribution, utility tab-count evidence,
  RED and GREEN runs, discovered test counts, files changed, exact commands and
  results, cleanup, blockers, risks, and synchronized task and plan status.
- **Verification:** `cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run --project dynamic-routing tests/workflow/dynamic-agent-profile.spec.ts tests/settings/dynamic-utility-profile.spec.ts`
- **Risks:** Poll exact session state before lifecycle assertions. Use API state
  only as supporting evidence for a visible UI outcome.

## Results

Not started in this implementation slice. Workflow and utility selector E2E
coverage remains pending.
