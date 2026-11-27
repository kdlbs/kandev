---
id: "01-reject-virtual-families-at-launch"
title: "Reject virtual families at launch"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.1
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-01.md
---

# Task 01: Reject virtual families at launch

## Summary

Fail closed with `ErrVirtualProfile` when a launch path resolves to a virtual
agent family, instead of reaching command building.

## In scope

- Add lifecycle and resolver tests before the change.
- Add `resolveLaunchableAgent` and use it at the lifecycle launch entry points.
- Reject the dynamic family in `agentNameForProfile`.

## Out of scope

Dynamic routing selection and virtual profile launchability.

## Acceptance

- Resolving a virtual agent type for launch returns `ErrVirtualProfile`.
- A restart whose profile resolves to a virtual agent fails closed with
  `ErrVirtualProfile`.
- An execution profile resolving to the dynamic family is rejected by the
  dynamic resolver.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle ./internal/agent/runtime -run 'Virtual' -count=1)
(cd apps/backend && go vet -trimpath ./internal/agent/runtime/lifecycle ./internal/agent/runtime)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- apps/backend/internal/agent/runtime/dynamic_resolver.go
- apps/backend/internal/agent/runtime/dynamic_resolver_virtual_test.go
- apps/backend/internal/agent/runtime/lifecycle/manager_execution.go
- apps/backend/internal/agent/runtime/lifecycle/manager_launch.go
- apps/backend/internal/agent/runtime/lifecycle/manager_launch_virtual_test.go
- apps/backend/internal/agent/runtime/lifecycle/manager_profile.go
- docs/specs/agents/system-design/dynamic-agent-routing-01.md

## Dependencies

None.

## Risks

Low.

## Parallelism

`sequential`

## Inputs

- REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001 and AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.1.
- The linked dynamic agent routing design.

## Results

- RED: without the launch-time rejection, the lifecycle tests and the resolver
  test failed (the virtual agent resolved like a concrete one).
- GREEN: the new tests pass; the related lifecycle and runtime tests pass except
  Windows failures that also occur without this change.

### PR review verification

Managed-policy admission now wraps both the policy rejection and the underlying
virtual-family error, preserving errors.Is for each identity. The focused
regression fails before this correction and passes afterward; missing agents
remain policy errors without a virtual-family identity. The contained proof
uses exact admission/lookup source and explicit registry and policy boundary
fakes. Full lifecycle and routing package verification remains a hosted CI
delivery gate; historical author results above do not certify the integrated
head.
