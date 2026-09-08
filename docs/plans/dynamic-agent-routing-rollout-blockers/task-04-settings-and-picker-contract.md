---
id: "dynamic-routing-blockers-04"
title: "Settings and picker contract"
status: completed
wave: 4
depends_on: ["dynamic-routing-blockers-02", "dynamic-routing-blockers-03"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing-rollout-blockers.md"
---

# Task 04: Settings and picker contract

Render all dynamic profiles with create and direct-edit actions. Add profile
and candidate enablement plus provider-error action controls. Preserve profile
kind in picker options, centrally filter dynamic profiles when the feature is
off, and hide unsupported duplication.

**Verification:** focused web unit/component tests, typecheck, i18n checks, and
the existing mobile settings flow.

## Results

Completed. Settings renders every dynamic profile with creation and editing
controls, candidate enablement, ordering, and provider-error actions. Profile
kind is preserved and dynamic profiles are excluded from new selections when
the flag is off; duplicate is hidden for dynamic profiles. The editor keeps
44px touch targets and uses the existing mobile picker sheet. Focused web
tests, typecheck, i18n checks, and the ratchet passed.
