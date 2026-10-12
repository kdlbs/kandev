---
status: draft
system: office
created: 2026-09-06
owners:
  - kandev
---

# Office: Built-In Default Spend Ceiling Requirements

## Overview

[Office: Pre-Launch Budget Enforcement](budget-enforcement.md) specifies when a
run is admitted against a spend ceiling. Correct enforcement code with zero
policies configured is still no ceiling, and `office_budget_policies` holds
zero rows on the reference install. This document owns the bound that exists on
a fresh install without an operator opting in.

It does not change admission behavior, which stays in the companion document,
and it does not measure spend, which is
[Office: Budget Measurement Integrity](budget-measurement-integrity.md).

## Terminology

This document uses this capability's shared vocabulary, defined in the [Office
glossary](../glossary.md) under **Budget enforcement** — in particular
*attended run*, *unattended run*, *subcent*, *priced spend*, and *evaluation
window*.

- **Built-in default ceiling:** a single operator-tunable limit per workspace,
  evaluated for unattended runs when no operator policy supersedes it. It is
  **not** a row in `office_budget_policies`; see
  `AC-OFFICE-BUDGET-003.7`.

## Requirements

### REQ-OFFICE-BUDGET-003: A default ceiling ships enabled for unattended runs

**Intent:** Beta needs a bound that exists without an operator opting in.
Scoping the default to unattended runs is what makes shipping it safe: attended
work, which is all of the reference install's measured spend, is unaffected by
its presence. The default is a *floor under* the operator's configuration, not
a placeholder the operator's first policy replaces — so a policy that never
blocks cannot remove it, and editing its limit cannot convert it into a policy
that blocks attended work.

#### Acceptance criteria

- **AC-OFFICE-BUDGET-003.1:** When no operator policy supersedes it under
  `AC-OFFICE-BUDGET-003.4`, the system shall evaluate unattended runs in that
  workspace against the built-in default ceiling.
- **AC-OFFICE-BUDGET-003.2:** The built-in default ceiling shall be 500,000
  subcents (50.00 USD) of priced spend per workspace per UTC day, and shall
  block on reaching that limit in the manner of a `block_new_tasks` policy.
- **AC-OFFICE-BUDGET-003.3:** The built-in default shall apply to unattended
  runs only, and shall never block an attended run, including after an operator
  has changed its limit.
- **AC-OFFICE-BUDGET-003.4:** A workspace-scoped `daily` policy shall supersede
  the built-in default for that workspace, replacing it rather than being
  evaluated in addition to it, **only when that policy's action is
  `pause_agent` or `block_new_tasks`**. A workspace-scoped `daily` policy whose
  action is `notify_only` shall not supersede the built-in default, and both
  shall be evaluated: the operator's policy notifies and the built-in default
  still blocks. A policy that never blocks shall not remove the shipped
  ceiling. A policy skipped under `AC-OFFICE-BUDGET-002.5`,
  `AC-OFFICE-BUDGET-002.11` or `AC-OFFICE-BUDGET-002.14` shall not supersede the
  built-in default either: a policy that is not evaluated cannot stand in for the
  ceiling it displaces, so a malformed row restores the built-in ceiling rather
  than removing every bound. Nor shall a run deferred or failed under
  `AC-OFFICE-BUDGET-006.1`, or cancelled under `AC-OFFICE-BUDGET-006.7`, reach
  this gate at all — an admission decision that
  could not be completed does not become a decision that the default did not
  apply.
- **AC-OFFICE-BUDGET-003.5:** The built-in default's limit shall be readable and
  writable by an operator through the same surface that manages budget
  policies, and its current effective value shall be visible there without
  inspecting the database.
- **AC-OFFICE-BUDGET-003.6:** When the built-in default blocks a run, the
  activity entry shall state that the ceiling was the built-in default and not
  an operator-created policy.
- **AC-OFFICE-BUDGET-003.7:** Writing the built-in default's limit under
  `AC-OFFICE-BUDGET-003.5` shall update a single durable per-workspace setting
  addressed by a stable identifier, and shall not create, update, or delete any
  row in `office_budget_policies`. The value shall not appear in the policy
  list API's results, and the written value shall remain the built-in default:
  it shall stay unattended-only under `AC-OFFICE-BUDGET-003.3` and shall
  continue to be reported as the built-in default under
  `AC-OFFICE-BUDGET-003.6`. Editing the limit shall not cause the default to be
  superseded under `AC-OFFICE-BUDGET-003.4`.
- **AC-OFFICE-BUDGET-003.8:** When an operator writes a built-in default limit
  less than or equal to zero, the system shall reject the write with a
  validation error and leave the previous effective value in force.
- **AC-OFFICE-BUDGET-003.9:** When no operator has written a limit for a
  workspace, that workspace's effective built-in default shall be the value in
  `AC-OFFICE-BUDGET-003.2`. Reading the effective value shall return a limit
  for every workspace, and shall never report the ceiling as absent.
- **AC-OFFICE-BUDGET-003.10:** When two operators write the built-in default's
  limit for the same workspace concurrently, the last write ordered by the
  setting's own commit shall be the effective value, and no write shall leave
  the workspace with no effective limit.

- **AC-OFFICE-BUDGET-003.11:** The built-in default shall be evaluated by the
  same criteria as a workspace-scoped `daily` policy whose action is
  `block_new_tasks`, with its effective limit standing in for the policy limit
  wherever those criteria name one. In particular the measurement-integrity
  criteria apply to it: its window is `daily` under
  `AC-OFFICE-BUDGET-002.1`, its spend is priced spend under
  `AC-OFFICE-BUDGET-004.1`, and a pricing-degraded window blocks an unattended
  run against it under `AC-OFFICE-BUDGET-004.3` and carries
  `AC-OFFICE-BUDGET-005.7`'s outcome. Without this, a fresh install — whose
  only ceiling is the built-in default — would never test for pricing
  degradation at all, and measurement integrity would not in fact be a
  precondition for shipping the default.

## Out of scope

- **Turning the ceiling off entirely.** There is no "disable" switch. An
  operator who wants no effective unattended bound raises the limit, or creates
  a workspace-scoped `daily` policy with a blocking action and a limit high
  enough not to bind, which supersedes the default under
  `AC-OFFICE-BUDGET-003.4`. Named so the absence of an off switch is a decision
  rather than an omission.
- **Per-run, per-routine, per-agent and per-project defaults.** The built-in
  default is per workspace per UTC day only. Operator policies already carry
  `agent` and `project` scopes; nothing here ships a default for them.
- **Changing the default's value per install, per profile, or per environment
  variable.** The shipped value is a constant, overridable only per workspace
  through `AC-OFFICE-BUDGET-003.5`.
- **Notifying an operator that the built-in default is in force.**
  `AC-OFFICE-BUDGET-003.6` requires attribution on a block; nothing here
  announces the ceiling before it first binds.
- **Retention or pruning of the activity entries this capability writes.**
