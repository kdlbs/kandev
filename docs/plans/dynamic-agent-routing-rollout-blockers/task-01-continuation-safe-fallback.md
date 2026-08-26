---
id: "dynamic-routing-blockers-01"
title: "Continuation-safe fallback"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing-rollout-blockers.md"
---

# Task 01: Continuation-safe fallback

Add regression coverage for partial output and tool-effect failures, then make
the conductor build, persist, and deliver bounded continuation data. Walk all
eligible candidates with generation fencing and never transfer a provider
native ACP identity across profiles.

**Verification:** dynamic engine and conductor tests plus focused orchestrator
failure tests.

## Results

Completed. Dynamic attempts now require explicit no-output/no-effect evidence;
ambiguous and stale execution events fail closed. The conductor persists and
delivers bounded provider-neutral continuation packages, fences native ACP
identity, and walks every eligible candidate. Focused dynamic and orchestrator
tests passed.
