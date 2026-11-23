---
created: 2026-11-20
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
  - REQ-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001
system_design:
  - ../../specs/integrations/system-design/github-authentication-02.md
  - ../../specs/platform/system-design/git-credential-lease-reissue.md
legacy_specs: []
---

# Implementation Plan: Managed Git credentials on session relaunch

## Overview

A workspace read can create an agentctl execution between session stop and
relaunch. The new agent process needs the lease and reissue capability that
resume prepared for it. The work order repairs that handoff and verifies it
with both the Go tests and the existing browser restart scenario.

## Technical approach

`promoteWorkspaceExecution` applies the admitted launch's managed Git
credentials to the workspace runtime environment. It uses the same indexed Git
configuration composition as prepared agent startup. `configureManagedGitCredentials`
reports broker unavailability when an attached GitHub repository requires a
managed lease. See the [task credential policy decision](../../decisions/2026-07-27-task-git-credential-policy.md).

## Tests

- `AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.15` uses the Go promotion tests to
  check the environment sent to agentctl and rejection of an incomplete lease.
- `AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.10` uses the executor resume test
  and the existing `chromium` agent-survival restart spec to check fresh issuance
  and delivery after adoption of a lease-only agent.

## Work orders

- [x] [Task 01: Deliver managed credentials to the relaunched agent](task-01-deliver-managed-credentials.md)

## Verification results

The three-test sequence passed five consecutive runs. The full survival-restart
and repeated native resume specs passed. Both backend packages and the changed
Go lint passed. The work order records the root cause and exact checks.
