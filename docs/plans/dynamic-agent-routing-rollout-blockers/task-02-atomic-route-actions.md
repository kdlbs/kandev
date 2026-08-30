---
id: "dynamic-routing-blockers-02"
title: "Atomic route actions"
status: completed
wave: 2
depends_on: ["dynamic-routing-blockers-01"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing-rollout-blockers.md"
---

# Task 02: Atomic route actions

Make the route-action backend own selection, predecessor shutdown, successor
launch, and final state persistence. Remove the frontend's second launch
request and retain recovery controls after a failed handoff.

**Verification:** backend route-action tests and a frontend request-count test.

## Results

Completed. Retry and Try next now use one backend-owned route action. The
backend launches the successor and records an actionable waiting state when
launch fails; the frontend no longer sends a second `session.launch` request.
The route-action lifecycle suite and focused web tests passed.
