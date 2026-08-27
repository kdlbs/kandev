---
id: "15-core-routing-public-docs"
title: "Core routing public docs"
status: in_progress
wave: 10
depends_on: ["08-dynamic-profile-settings", "09-routed-chat-presentation", "10-office-routing-handoff"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 15: Core routing public docs

- **Acceptance:** Document dynamic profiles, fixed-order fallback, waiting and
  recovery actions, workflow and Office selection, the experimental runtime
  flag, all-off defaults, and restart behavior. Do not document telemetry rules
  as part of this feature.
- **Files likely touched:** `docs/public/agents-and-profiles.md`,
  `docs/public/configuration.md`, `docs/public/feature-status.md`, and
  `docs/public/tasks-and-workflows.md`.
- **Dependencies:** Tasks 08, 09, and 10.
- **Parallelism:** parallel-safe with Task 13. This task owns only
  `docs/public/**` files.
- **Inputs:** Spec Profile configuration, Settings interaction, Use in Kanban
  and workflows, Use in Office, and Delivery and rollout, Tasks 08 through 10,
  `/docs-maintainer`, runtime feature-flag public documentation patterns.
- **Output contract:** Report each public page and its Diataxis type, files
  changed, exact commands and results, blockers, risks, and synchronized task
  and plan status.
- **Verification:** `node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs && rg -n "dynamicAgentRouting|Dynamic profile|dynamic profile" docs/public`
- **Risks:** Public documentation must not present deferred telemetry behavior
  as part of dynamic agent routing.

## Results

In progress. Updated the public agents, configuration, feature-status, and task
workflow guides with the experimental flag, all-off production default,
restart requirement, fixed-order behavior, logical-session boundary, and
settled-turn recovery actions. Office handoff wording is intentionally not
claimed because Task 10 is still pending.

Verification:

- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `rg -n "dynamicAgentRouting|Dynamic profile|dynamic profile" docs/public`

All three checks passed. Office handoff wording is intentionally not claimed
because Task 10 is still pending.
