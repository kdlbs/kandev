---
id: coordinator-containment
title: Containment for unattended turns
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
---

# Containment for unattended turns Requirements

## Overview

Kandev guards the coordinator's Kandev tool surface, but not the agent CLI's
own tools. With authentication off, an agent with a shell on the backend's
host can call Kandev's REST API and external `/mcp` endpoint directly, for
example to approve its own proposal or to create a task on an auto-starting
step. Phase 1 accepted that because every turn was attended. An unattended
turn has no person present, so phase 3 starts one only when the system can
verify that the coordinator's session has no path to Kandev's API or database
other than its guarded Kandev surface. When it cannot, the turn is held and
the reason is shown: containment fails closed.

## Terminology

- **Isolated executor type:** `local_docker`, `remote_docker`, `sprites` or
  `k8s`, plus `mock_remote` only under the e2e runtime profile. `local`,
  `worktree`, `ssh`, `plugin_remote` and any type not listed are not
  isolated.
- **Containment conditions:** the four checks of
  `REQ-COORDINATOR-CONTAINMENT-001`, each with a stable name:
  `executor_isolated`, `auth_enabled`, `no_kandev_credential` and
  `no_extra_tools`.
- **Contained:** all four conditions verified at the time of the check.
- Other terms are defined in [wake](wake.md#terminology) and the
  [system README](../README.md#terms).

## Mockup

The source mockup has no containment view; the plan's ASCII preview UI-04 is
the reference for this document's user-facing criteria.

## Requirements

### REQ-COORDINATOR-CONTAINMENT-001: The containment check

**Intent:** Containment is a verified property of the coordinator's
configuration and the instance, not a claim.

#### Acceptance criteria

- **AC-COORDINATOR-CONTAINMENT-001.1:** The system shall treat a coordinator
  as contained only when all of these hold: its executor profile's executor
  type is an isolated executor type (`executor_isolated`); the instance's
  authentication mode is enabled, so Kandev's REST API and external `/mcp`
  require a session or personal access token (`auth_enabled`); the resolved
  launch environment of its executor and agent profiles contains no variable
  named `KANDEV_API_KEY` or `KANDEV_RUN_TOKEN` and no value beginning with
  `kandev_pat_` (`no_kandev_credential`); and its agent profile configures no
  MCP server other than Kandev's (`no_extra_tools`).
- **AC-COORDINATOR-CONTAINMENT-001.2:** When any input to a condition cannot be
  read, including a secret that cannot be resolved, the system shall treat
  that condition as failed with detail `unreadable`, never as passed.
- **AC-COORDINATOR-CONTAINMENT-001.3:** The system shall evaluate the check at
  every admission, from current state; a result shall never be cached across
  admissions.

### REQ-COORDINATOR-CONTAINMENT-002: Fail closed

**Intent:** Without containment, nothing unattended runs.

#### Acceptance criteria

- **AC-COORDINATOR-CONTAINMENT-002.1:** When a coordinator is not contained,
  the system shall start no unattended turn for it, keep its pending wakes
  `pending`, and report hold reason `containment` with the first failing
  condition's name.
- **AC-COORDINATOR-CONTAINMENT-002.2:** Attended turns shall not depend on
  containment: a manager's message shall start a turn whether or not the
  coordinator is contained.
- **AC-COORDINATOR-CONTAINMENT-002.3:** The Autonomy section of the
  coordinator settings shall list the four conditions, each as Met or Not met
  with a one-line fix, re-read each time the section opens and on Check
  again.

### REQ-COORDINATOR-CONTAINMENT-003: No one to ask

**Intent:** An unattended turn cannot wait on a person or be approved by its
own CLI settings.

#### Acceptance criteria

- **AC-COORDINATOR-CONTAINMENT-003.1:** During an unattended turn, when the
  coordinator's agent requests a permission that Kandev's exact-name
  auto-approve does not grant, the system shall deny it at once, record the
  resolution with actor kind `coordinator_unattended`, and let the turn
  continue.
- **AC-COORDINATOR-CONTAINMENT-003.2:** An unattended turn's session shall
  force the profile and environment auto-approve settings off exactly as an
  attended coordinator session does.
- **AC-COORDINATOR-CONTAINMENT-003.3:** An unattended turn that ends with a
  denied permission shall record the number of denials on the turn, and the
  "Woken by" transcript entry shall show it.

## Out of scope

- Sandboxing the `local`, `worktree` or `ssh` executors. They stay available
  for attended turns.
- Credentials that are not Kandev's, such as a Git host token in the
  executor's environment. The manager chooses the profile; the
  [ADR](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md#residual-risk)
  records this.
- Board content steering the coordinator (untrusted text in task titles,
  descriptions and transcripts). Accepted as in phase 1; containment bounds
  what steered behaviour can reach, it does not prevent the steering.
- A workspace-scoped or coordinator-scoped Kandev token. The coordinator's
  Kandev surface is already bound to a server-derived principal; no token is
  issued to the session.
