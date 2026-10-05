# Architecture maintenance

This roadmap tracks incremental simplification after the September architecture changes.
It is a backlog, not approval to implement every proposal.
Requirements and system designs remain authoritative under `docs/specs/`.
Finite delivery packages remain under `docs/plans/`.

**Last inventory:** 2026-09-27, main commit `359b5ffdbb6`.
**Next proposed review:** 2026-10-11.
**Coordination owner:** repository maintainers. A named assignee is required before each work item starts.
**Umbrella issue:** not created. This documentation PR does not create tasks or start agents.

## Tracking locations

| Record                                                | Purpose                                                      | Update point                                       |
| ----------------------------------------------------- | ------------------------------------------------------------ | -------------------------------------------------- |
| This roadmap                                          | Priorities, status, and links across systems                 | Each completed increment and each scheduled review |
| [Server-state migrations](server-state-migrations.md) | Resource ownership and Query migration candidates            | The resource's implementation PR                   |
| [Dependency cleanup](dependency-cleanup.md)           | Store typing, package boundaries, and compatibility removal  | The cleanup PR                                     |
| [Linter roadmap](lint-roadmap.md)                     | Existing protections and proposed checks                     | A rule or baseline change                          |
| [Historical audit disposition](historical-audit.md)   | Which July findings remain useful                            | A fresh investigation changes their disposition    |
| GitHub umbrella issue                                 | Discussion, next milestone, and links to these files         | Milestone review                                   |
| Child issue or Kandev task                            | One assignee, bounded scope, and PR link                     | Delivery progress                                  |
| `docs/plans/<initiative>/`                            | Approved implementation scope, work orders, and verification | The implementation PR                              |

Markdown belongs on main. Each change uses a short-lived PR.
An open draft PR is not the permanent status database.
The umbrella issue can remain open across milestones without duplicating these tables.

## Next finite milestone: prove the second resource

The first proposed milestone contains three independent outcomes:

1. Migrate database statistics after approval of its identity and freshness design.
2. Remove unsafe composition casts from one additional small Zustand slice.
3. Inventory consumers of the 31 Office aliases and select the first removable group.

The milestone is complete when those outcomes have merged evidence or an explicit deferred decision with a reason.
Backups and disk usage follow the database result. Broad task/session migration is not part of this milestone.

| Priority | Item                                  | Status      | Next action                                                                     |
| -------- | ------------------------------------- | ----------- | ------------------------------------------------------------------------------- |
| 1        | `QUERY-02`: database statistics       | In progress | Implement the reviewed Query owner for the database card and backup description |
| 2        | `DEP-01`: next typed slice            | Proposed    | Select one small slice after reading its setter/getter dependencies             |
| 3        | `DEP-02`: Office alias consumers      | Proposed    | Group consumers and record removal evidence per alias                           |
| 4        | `LINT-01`: library Query checks       | Proposed    | Evaluate existing plugin rules before custom scanners                           |
| 5        | `QUERY-03`: backup list and mutations | Proposed    | Inventory mutation invalidation and reload callers                              |
| 6        | `QUERY-04`: disk usage and job events | Proposed    | Design the first bounded Query/WS reconciliation path                           |

These IDs identify backlog entries, not requirements or work orders.
`Proposed` means no implementation assignment exists.
Use `planned` only with a reviewed delivery package, `in_progress` with an assignee, and `done` with a merged PR.
Use `blocked` or `deferred` with a reason and a next review date.

## Maintenance procedure

1. At the next review, assign a maintainer and select at most two small increments.
2. For each increment, record the assignee, task link, scope, completion conditions, and next review date.
3. Create its requirement/design references and finite delivery package before implementation, using the repository's existing workflow.
4. Update the matching tracker row in the implementation PR.
5. Record the merged PR and actual verification results in the work order.
6. At each fortnightly review, resolve blocked items or explicitly defer them with a reason.
7. Each month, run the linter checks and review compatibility targets from their source files.
8. Close the finite milestone before selecting another one.

No scheduled automation exists for this cadence. The assigned maintainer owns the reviews.
The first operational follow-up is a concise umbrella issue with an assignee and the next review date.
Its description needs only the goal, this directory's link, the current milestone, and links to active tasks.

## What counts as progress

- Fewer duplicate request effects, server-state copies, and custom invalidation paths for each migrated resource.
- Fewer exact baseline findings and compatibility entries, without moving the same dependency elsewhere.
- Stable user behavior, identity isolation, and passing regression tests.
- Useful linter diagnostics with low false-positive rates and acceptable execution time.

A migration count alone is not success. Query does not own every Zustand value.
A larger rule count alone is not success. Rules protect agreed boundaries, not folder preferences.

## Completed foundation

| Outcome                                         | Merged PR                                          | Delivery record                                                                              |
| ----------------------------------------------- | -------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| Architecture tooling documentation coverage     | [#3967](https://github.com/kdlbs/kandev/pull/3967) | [Work order](../plans/pr-documentation-coverage/task-06-exempt-architecture-lint-tooling.md) |
| Features slice composition without unsafe casts | [#3971](https://github.com/kdlbs/kandev/pull/3971) | [Plan](../plans/features-slice-root-typing/plan.md)                                          |
| State no longer imports UI modules              | [#3973](https://github.com/kdlbs/kandev/pull/3973) | [Plan](../plans/frontend-state-ui-ownership/plan.md)                                         |
| Runs owns shared run data contracts             | [#3974](https://github.com/kdlbs/kandev/pull/3974) | [Plan](../plans/shared-run-contract-ownership/plan.md)                                       |
| Explicit deprecations have ledger enforcement   | [#3975](https://github.com/kdlbs/kandev/pull/3975) | [Plan](../plans/architecture-deprecation-ledger/plan.md)                                     |
| SystemInfo uses TanStack Query                  | [#3977](https://github.com/kdlbs/kandev/pull/3977) | [Plan](../plans/system-info-query-pilot/plan.md)                                             |

Completed work orders stay available as evidence. They are not deleted to make the backlog shorter.
Later milestones can have separate plans without resetting this history.
