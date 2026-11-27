---
id: selected-feature-graduation-07
title: Retire dynamic routing flag
status: pending
wave: 7
depends_on:
  - selected-feature-graduation-04
plan: plan.md
requirements:
  - REQ-AGENTS-DYNAMIC-ROUTING-GRADUATION-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-ROUTING-GRADUATION-001.2
  - AC-AGENTS-DYNAMIC-ROUTING-GRADUATION-001.3
  - AC-AGENTS-DYNAMIC-ROUTING-GRADUATION-001.4
  - AC-AGENTS-DYNAMIC-ROUTING-GRADUATION-001.5
system_design:
  - ../../specs/agents/system-design/dynamic-routing-graduation.md
---

# Retire dynamic routing flag

## Summary

After a stable default-on release, make dynamic profile configuration,
selection, execution, and recovery permanently available and retire the flag.

## In scope

- Remove flag-dependent refusal branches from profile CRUD, execution
  resolver, route actions, recovery scheduler, and backend composition. Keep
  validation, pre-result fallback evidence, circuit/probe leases, generation
  fencing, and actionable failure states.
- Remove frontend flag checks from settings, pickers, task and utility
  launchers, watchers, and routed chat while keeping profile-kind and
  permission decisions.
- Remove the live profile/config/registry/frontend identity, retire its exact
  key/environment pair, preserve old overrides, and update public agent,
  configuration, and feature-status pages.

## Out of scope

- Automatically routing an existing concrete profile, converting old Office
  routing rows, or adding telemetry-based candidate selection.

## Acceptance

1. An old false environment or installation override cannot disable dynamic
   profiles; the old identity is absent from active metadata and permanently
   reserved in the retired registry.
2. Concrete bindings stay concrete, and ambiguous or post-result failures do
   not launch another provider; waiting routes recover through their existing
   actions after restart.
3. Desktop and phone users can create/select a dynamic profile and recover a
   routed session on a normal production-profile installation.

## ASCII UI preview

`UI-01` and `UI-02` from [the plan](plan.md#ascii-ui-preview):

```text
Desktop Settings > Agents: Dynamic agents [Add profile] > [Edit]
Phone Settings > Agents > Dynamic profile: Candidates [Reorder] [Edit] [Save]
```

Phone remains a direct route with visible touch actions and one scroll owner.
The preview does not change provider state or picker semantics.

## Verification

```sh
cd apps/backend && go test ./internal/agent/runtime/dynamic ./internal/agent/settings/controller ./internal/orchestrator ./internal/runtimeflags ./internal/common/config ./internal/profiles
cd apps && pnpm --filter @kandev/web exec vitest run lib/state/slices/features/features-contract.test.ts
make -C apps/backend lint
cd apps/web && pnpm run typecheck && pnpm run lint
git diff --check
```

After rebuilding backend and web E2E artifacts and completing the base plan's
dedicated Playwright project setup:

```sh
cd apps/web && pnpm e2e:run --project=dynamic-routing tests/task/dynamic-agent-routing.spec.ts tests/settings/dynamic-utility-profile.spec.ts
cd apps/web && pnpm e2e:run --project=dynamic-routing-mobile tests/task/mobile-dynamic-agent-routing.spec.ts
```

The base plan's Office and caller-selection E2E commands remain required by
Task 04 and are rerun here if the retirement changes their entry paths.

## Files likely touched

- `apps/backend/internal/backendapp/services.go`, agent settings controller,
  dynamic runtime composition, orchestrator recovery, and route-action tests.
- `apps/web/components/settings/`, task/utility pickers and watcher dialogs,
  routed chat, feature slice/types, and mobile E2E.
- `apps/backend/internal/runtimeflags/registry.go`, config/catalog, profile
  YAML, exact-key contracts; public agent/configuration/status docs.

## Dependencies and risks

Requires Task 04 to have shipped in a stable release after the base routing
plan is complete. Removing the disabled recovery path makes routing always
eligible for selected dynamic profiles; preserve the fail-closed evidence and
generation checks. Old false overrides cease to suppress the feature.

## Results

Pending.
