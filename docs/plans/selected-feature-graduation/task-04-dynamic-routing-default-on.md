---
id: selected-feature-graduation-04
title: Promote dynamic agent routing
status: pending
wave: 4
depends_on:
  - "04-core-route-engine"
  - "05-acp-conductor"
  - "06-logical-session-integration"
  - "07-utility-profile-integration"
  - "09-routed-chat-presentation"
  - "10-office-routing-handoff"
  - "11-core-routing-observability"
  - "12-profile-settings-e2e"
  - "13-routed-session-e2e"
  - "14-caller-selection-e2e"
  - "16-office-rollout-e2e"
plan: plan.md
requirements:
  - REQ-AGENTS-DYNAMIC-ROUTING-GRADUATION-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-ROUTING-GRADUATION-001.1
  - AC-AGENTS-DYNAMIC-ROUTING-GRADUATION-001.3
  - AC-AGENTS-DYNAMIC-ROUTING-GRADUATION-001.4
  - AC-AGENTS-DYNAMIC-ROUTING-GRADUATION-001.5
system_design:
  - ../../specs/agents/system-design/dynamic-routing-graduation.md
---

# Promote dynamic agent routing

## Summary

After the existing routing implementation package is complete, make dynamic
profiles available by default while retaining the flag as a kill switch for
one stable release.

## In scope

- Confirm [the base routing plan](../dynamic-agent-routing/plan.md) and its
  lifecycle, caller, Office, observability, and E2E work orders have completed
  with recorded results. That work remains owned by its existing package; the
  separate rollout-blocker plan does not close it.
- Set `KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING` true in every shipped profile.
  Keep all current flag gates and explicit override behavior for this release.
- Prove concrete-profile behavior, dynamic selection, cross-provider fallback,
  ambiguous/post-result fail-closed behavior, restart recovery, and desktop/
  phone settings and task use. Update public feature status and agent guide.

## Out of scope

- Flag retirement, automatic conversion of existing bindings, or new
  telemetry-driven candidate selection.

## Acceptance

1. The base routing plan's recorded open work orders and project matrix are
   completed with exact results before a default-on release is cut.
2. Production-profile installations can explicitly create/select a dynamic
   profile without an override; `false` still disables new dynamic execution.
3. Desktop and phone profile and recovery flows pass, while concrete bindings
   and ambiguous provider failures retain their existing safe behavior.

## ASCII UI preview

`UI-01` and `UI-02` from [the plan](plan.md#ascii-ui-preview):

```text
Desktop Settings > Agents: Dynamic agents [Add profile] > [Edit]
Phone Settings > Agents > Dynamic profile: Candidates [Reorder] [Edit] [Save]
```

Phone editing remains a direct focused route, not a compressed desktop card.
Existing touch targets, internal scroll, and shared profile state remain.

## Verification

The base plan's exact open-work-order commands must have passed before this
task starts. Run these focused promotion checks:

```sh
cd apps/backend && go test ./internal/profiles ./internal/runtimeflags ./internal/agent/runtime/dynamic ./internal/agent/settings/controller ./internal/orchestrator
cd apps && pnpm --filter @kandev/web exec vitest run lib/state/slices/features/features-contract.test.ts
make -C apps/backend lint
cd apps/web && pnpm run typecheck && pnpm run lint
git diff --check
```

After rebuilding backend and web E2E artifacts, run the base plan's routed
session, caller, Office, settings, and mobile project matrix without a flag
override. The expected focused commands, after that plan creates the dedicated
projects and specs, include:

```sh
cd apps/web && pnpm e2e:run --project=dynamic-routing tests/task/dynamic-agent-routing.spec.ts tests/settings/dynamic-utility-profile.spec.ts
cd apps/web && pnpm e2e:run --project=dynamic-routing-mobile tests/task/mobile-dynamic-agent-routing.spec.ts
```

## Files likely touched

- `apps/backend/internal/profiles/profiles.yaml` and profile/registry tests.
- Dynamic settings/task E2E fixtures; `docs/public/feature-status.md` and
  `docs/public/agents-and-profiles.md`.

## Dependencies and risks

The base routing plan is in progress and records missing lifecycle ownership,
Office handoff, observability, and E2E evidence. Promotion waits for those
results. Fallback must never retry after effects or persist an invisible
provider switch. This task retains the kill switch.

## Results

Pending.
