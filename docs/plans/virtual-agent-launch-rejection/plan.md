---
created: 2026-10-12
status: done
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-01.md
legacy_specs: []
---

# Implementation Plan: Fail closed when a virtual agent family reaches launch

## Overview

The `dynamic` agent family is virtual: it owns no subprocess and exposes no CLI
command (AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.1). The profile resolver already
rejects virtual profiles with `ErrVirtualProfile`, but the launch paths look
the agent type up in the registry directly. An execution whose agent type
resolves to the virtual family therefore reaches command building, and the
launch fails with a generic `agent command cannot be empty` instead of the
typed virtual-profile error.

## Scope

### In scope

- Resolve launch agents through one helper that rejects virtual families with
  `ErrVirtualProfile`.
- Use it at every lifecycle launch entry point that looked the agent up
  directly, and reject the virtual family when the dynamic resolver names an
  execution profile's agent.

### Out of scope

- Dynamic routing selection itself.
- Making virtual profiles launchable.

## Technical approach

`Manager.resolveLaunchableAgent` wraps the registry lookup and returns
`agent type %q: ErrVirtualProfile` for a virtual agent. Managed tool policy
admission, workspace promotion, `launchInternal`, execution create requests,
and profile-backed restarts use it. `ProfileExecutionResolver.agentNameForProfile`
returns `ErrVirtualProfile` when an execution profile resolves to the dynamic
family.

## Tests

`manager_launch_virtual_test.go` covers
`TestResolveLaunchableAgent_RejectsVirtualFamily` and
`TestRestartAgentProcess_VirtualAgentFailsClosed`, and
`dynamic_resolver_virtual_test.go` covers
`TestAgentNameForProfile_RejectsVirtualFamily`:
AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.1.

## E2E tests

Lifecycle and resolver tests cover the change. No rendered UI changes require
browser coverage.

## Work orders

- [x] [Task 01: Reject virtual families at launch](task-01-reject-virtual-families-at-launch.md)

## Verification results

The new tests failed without the launch-time rejection and pass with it.

## Risks

Low: a launch that already failed with an empty command now fails earlier with
the typed error.

### PR review verification

Managed-policy admission now wraps both the policy rejection and the underlying
virtual-family error, preserving errors.Is for each identity. The focused
regression fails before this correction and passes afterward; missing agents
remain policy errors without a virtual-family identity. The contained proof
uses exact admission/lookup source and explicit registry and policy boundary
fakes. Full lifecycle and routing package verification remains a hosted CI
delivery gate; historical author results above do not certify the integrated
head.
