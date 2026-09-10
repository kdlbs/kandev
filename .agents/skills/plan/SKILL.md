---
name: plan
description: Create a committed implementation plan and work orders from approved requirements and current system designs. Use after specification work and before implementation.
---

# Create an Implementation Plan

Translate requirements and system designs into a delivery plan under
`docs/plans/<initiative>/`. The plan and its work-order files are implementation
records. They are not product specifications.

Read `docs/specs/guide/plans-and-work-orders.md` and
`docs/specs/guide/traceability-and-lifecycle.md` before you write the plan.

## Inputs

Read these sources in full:

- The applicable requirement documents and `REQ-*` or `AC-*` IDs.
- The applicable system-design documents.
- Related ADRs.
- The relevant source, tests, and one similar implementation.

Use the legacy feature specification during migration only when no replacement
requirement or design exists.

Read each spec's `Implementation Plans` and related-plan links, then inventory
all existing companion plan directories. Treat linked packages as one
synchronized implementation record: reconcile every affected task scope, E2E
scenario matrix, status, and exact final verification results/counts before
completion.

## Outputs

Create:

```text
docs/plans/<initiative>/plan.md
docs/plans/<initiative>/task-<NN>-<short-slug>.md
```

`plan.md` is the work-package manifest. Each `task-*.md` file is one work order.

## Workflow

### 1. Map the change

Identify:

- The required outcomes and acceptance criteria.
- The design boundaries that implementation must preserve.
- Existing models, repositories, services, handlers, clients, stores, and UI.
- Existing tests and end-to-end patterns.
- The dependency order for implementation.

Check `docs/decisions/INDEX.md` for relevant decisions. Stop when the
requirements, system design, ADRs, and code disagree on a material boundary.

### 2. Write `plan.md`

Use this structure:

```markdown
---
created: YYYY-MM-DD
status: draft
requirements:
  - REQ-<SYSTEM>-<CAPABILITY>-001
system_design:
  - ../../specs/<system>/system-design/<capability>.md
legacy_specs: []
---

# Implementation Plan: <Initiative>

## Overview

State the result, the implementation order, and the reason for that order.

## Scope

### In scope

- List the owned outcomes.

### Out of scope

- List explicit exclusions.

## Technical approach

Name exact files, symbols, schema changes, contracts, and integration points.
Organize this section by implementation boundary or vertical slice.

## Tests

Map every relevant acceptance criterion to its unit or integration evidence.
Name the exact test file and method.

## E2E tests

Include this section when the change has user-visible behavior. Map each flow
to the applicable `AC-*` IDs and name the Playwright file and project.

## Work orders

- [ ] [Task 01: <Title>](task-01-<slug>.md)

## Verification results

Pending.

## Risks

- Name concrete delivery or compatibility risks.

## Open questions

Delete this section when it is empty.
```

Do not place complete work-order bodies in `plan.md`.

### 3. Write work orders

Use this structure:

````markdown
---
id: "01-<slug>"
title: "<Title>"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-<SYSTEM>-<CAPABILITY>-001
acceptance_criteria:
  - AC-<SYSTEM>-<CAPABILITY>-001.1
system_design:
  - ../../specs/<system>/system-design/<capability>.md
---

# Task 01: <Title>

## Summary

State the implementation outcome in two or three sentences.

## In scope

- List the responsibilities that this work order owns.

## Out of scope

- List adjacent work that this work order does not own.

## Acceptance

- Give one to three concrete implementation conditions.

## Verification

```bash
<exact targeted command>
```

## Files likely touched

- `path/to/file`

## Dependencies

Name prior work orders or write `None`.

## Risks

- Name concrete implementation or compatibility risks, or write `None`.

## Parallelism

`sequential`

## Inputs

- Requirement and system-design sections.
- Existing code and test patterns.

## Results

Pending.
````

Each work order must fit one focused implementation pass. Split work that has
unrelated outcomes, independent verification, or a title that contains “and.”

Use `parallel-safe` only when files are disjoint and the tasks share no schema,
migration, generated contract, lockfile, or package configuration. A wave does
not authorize subagents.

### 4. Define verification

Every work order needs exact commands. Use repository `make` targets when they
exist. Frontend work in a fresh worktree includes the workspace dependency
installation before the first package command.

Write each verification block so that a user can run the complete block in one
shell. If commands change directories, use one directory change or isolated
subshells.

When implementation is complete, replace `Pending` with every required command
and its result. Include specification and diff gates that run outside the
product test commands.

User-facing behavior needs end-to-end evidence somewhere in the work package.
A low-level work order does not need an artificial browser test.

After implementation and targeted checks pass, run every listed verification
command from its documented working directory. In a multi-command block, root
each command independently with `(cd <dir> && ...)` or an explicit root reset;
never rely on a preceding `cd`. Confirm each referenced path exists and stays
inside the tool or package scope, and that every changed test suite is covered
before marking Results complete. Record actual results, not planned or stale counts.

Before marking a new plan package complete, run `git diff --check --
docs/plans/<initiative>` and `git status --short -- docs/plans/<initiative>`;
the status check catches untracked work orders. Confirm every work order names
existing `REQ-*`/`AC-*` IDs and an existing system-design path.

Do not add generic QA, review, simplify, security, or full-verification tasks.
Task checks provide pre-PR evidence. Configured PR reviewers provide semantic
review after the PR opens.

### 5. End the design turn

Report the requirement IDs, system designs, plan, work orders, dependency
order, exact checks, and open risks. Then end the turn.

Do not ask for plan approval or a model switch. The user reviews the artifacts
and sends a later explicit implementation request.
