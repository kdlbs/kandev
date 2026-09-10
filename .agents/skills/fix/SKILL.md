---
name: fix
description: Diagnose a Kandev bug, reconcile its requirements and system design, create a reviewable fix plan and work orders, then hand off for explicit TDD implementation.
---

# Fix

Use the same durable workflow as feature work. Diagnose the defect first. Then
reconcile specifications and create the fix plan and work orders before you
change production code.

## Core flow

```text
Evidence -> Root cause -> Requirement conformance -> System-design check -> Fix plan + work orders -> Handoff -> Explicit implementation request -> TDD -> PR review
```

Do not patch production code before the planning checkpoint unless the user
explicitly opts out.

## Phase 0: Evidence and root cause

When a bug comes from an issue tracker, read the canonical issue and each image
attachment. Reproduce the behavior with existing tests, a read-only trace, or a
minimal temporary reproduction.

State:

- The incorrect behavior and its conditions.
- The root cause.
- The smallest reliable reproduction.
- The intended regression-test level and path.

If the cause remains uncertain, stop and ask the user.

## Phase 1: Reconcile specifications

Read `docs/specs/README.md`, the owning system index, and the relevant
requirement and system-design documents. Use the legacy catalog only when the
system has not migrated.

Search adjacent systems before you create or move an artifact. A UI symptom does
not make the repair UI-owned. Update the system that owns the failed contract,
and include observable UI recovery there when applicable.

Classify the bug:

1. **Implementation violates an active acceptance criterion.** Reference the
   existing `AC-*` ID. Do not create or rewrite a requirement.
2. **The intended behavior is missing from requirements.** Add the smallest
   requirement and acceptance criteria that define it.
3. **The intended product behavior changes.** Amend or supersede the affected
   requirement. Record the compatibility effect.
4. **The technical design is wrong or incomplete.** Update the system design.
   Keep observable behavior in requirements.

Do not create a standalone repair specification. The requirement is the durable
behavioral source. The plan and work orders record this repair.

For predicates over collections or inventory, state the quantifier explicitly:
distinguish existential conditions such as “at least one live row” from
universal conditions such as “all rows.” When behavior depends on that
distinction, require a mixed-state regression containing both live and failed or
deleted rows.

Do not create parallel feature and UI requirements for one repair. A separate UI
requirement is valid only when the repair changes an independent reusable UI
contract.

Use `/record` when the correction establishes a durable boundary, ownership
rule, public contract, persistence rule, or security invariant with meaningful
alternatives.

## Phase 2: Create the fix package

Create `docs/plans/<fix-slug>/plan.md` and sibling work orders through `/plan`.

The package must:

- Reference the affected `REQ-*` and `AC-*` IDs.
- Reference the applicable system design.
- State the confirmed root cause.
- Name the regression test that fails before the correction.
- Name exact files, dependencies, acceptance conditions, and commands.
- Use dependency waves only when they clarify implementation order.

Keep work orders sequential by default. A wave does not authorize delegation.

## Phase 3: Design-package handoff

Before you change production or permanent test code, report:

- Root cause and reproduction evidence.
- Requirement IDs and system-design paths.
- Plan and work-order paths.
- Dependency order and exact commands.
- Risks and exclusions.

Then end the turn. Do not ask the user to approve the package or switch models.
Wait for a later explicit implementation request.

## Phase 4: Implement with TDD

After the explicit request:

1. Change the work order to `in_progress`.
2. Write and run the regression test.
3. Make sure that the test fails for the expected reason.
4. Implement the minimum correction.
5. Run the work order's exact commands.
6. Change the work order to `done` and synchronize `plan.md`.

For persisted defaults or coupled settings, inspect every write and reset path.
Cover explicit empty and null values when they affect the contract.

If the user authorizes subagents, obey `/planner-orchestration`. Use native
delegation, compact work-order context, no recursive delegation, and no
full-history fork.

## Phase 5: PR review

After all work-order checks pass, commit, push, and open the PR. Do not add an
automatic local simplify, QA, broad review, security review, or verification
pass.

Use `/pr-fixup` only for a CI error or an actionable reviewer finding. Run the
affected work-order check after a correction.

## Stop conditions

Stop when the root cause is uncertain, specifications and code disagree, the
repair needs a new material design, or the same targeted check fails three
times.

## Final report

Report the root cause, requirement IDs, system-design path, plan and work-order
paths, changed files, commands, results, authorized subagents, and PR status.
