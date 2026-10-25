---
status: draft
system: agents
requirements:
  - REQ-AGENTS-DYNAMIC-ROUTING-GRADUATION-001
created: 2026-09-25
owners:
  - kandev
---

# Dynamic Routing Graduation System Design

## Purpose and boundaries

The existing [dynamic routing designs](dynamic-agent-routing-01.md) and
[rollout blocker requirements](../requirements/dynamic-agent-routing-rollout-blockers.md)
own candidate, conductor, recovery, and shared-health behavior. This design
changes only availability and retires the release gate. The
[base implementation plan](../../../plans/dynamic-agent-routing/plan.md) is
still in progress; completing its open work orders is a promotion dependency.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-DYNAMIC-ROUTING-GRADUATION-001` | Promotion, retirement, and verification |

## Promotion and retirement

First reconcile the base plan's recorded results with current code and complete
its remaining conductor, lifecycle, Office, observability, and desktop/phone
E2E work. The completed rollout-blocker plan is not a substitute for those
unfinished work orders. Then promote all shipped profile defaults to true for a
stable release, retaining the active registry entry and disabled path as a
working kill switch.

In the retirement release, remove `FeaturesConfig.DynamicAgentRouting` from
backend composition and settings CRUD, conductor/recovery scheduling, and
route-action admission. Preserve candidate eligibility, pre-result evidence,
generation fencing, circuit/probe leases, and durable route recovery. Remove
frontend `useFeature("dynamicAgentRouting")` checks from agent settings,
profile pickers, task and utility launchers, watchers, and routed chat while
keeping profile-kind and permission checks. Existing concrete bindings stay
concrete; no data rewrite occurs.

Remove the profile key, typed config field, active runtime definition,
feature-response field, frontend default, and live startup-catalog
classification. Append `features.dynamicAgentRouting` /
`KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING` to the retired registry and retain old
SQLite override rows as inert data. An old false environment value is ignored.

## UI and verification

Desktop retains the Dynamic agents card and profile editor; phone retains the
direct editor route and touch picker. Both use the same profile and route data.
Verify concrete-profile flows, dynamic selection and fallback, waiting-route
recovery after restart, utility isolation, Office integration, and rendered
phone recovery controls under the production profile without an override.
Retain failure and persistence tests when disabled-flag assertions are removed.
