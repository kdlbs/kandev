---
id: "dynamic-routing-blockers-03"
title: "Utility and shared health"
status: completed
wave: 3
depends_on: ["dynamic-routing-blockers-01"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing-rollout-blockers.md"
---

# Task 03: Utility and shared health

Give every utility invocation an isolated route ID, stop fallback after partial
results, connect concrete binding descriptors to candidates, open circuits on
qualifying failures, and make expired circuits use production probe leases.

**Verification:** resolver, utility handler, circuit, and engine tests.

## Results

Completed. Utility calls use fresh transient identities and reject partial
results before fallback. Concrete credential bindings now feed shared circuit
keys, qualifying failures open scoped circuits, and expired circuits use
exclusive production probes. Focused utility, engine, resolver, and SQLite
tests passed.
