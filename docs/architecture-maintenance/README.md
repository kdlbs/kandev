# Architecture maintenance

This roadmap tracks incremental simplification after the September architecture changes.
It is a backlog, not approval to implement every proposal.
Requirements and system designs remain authoritative under `docs/specs/`.
Finite delivery packages remain under `docs/plans/`.

**Historical inventory:** 2026-09-27, main commit `359b5ffdbb6`.
**Current main snapshot:** 2026-10-09, commit `3fed5570cec533f468c25ed03c84967bdc972588`.
**Next proposed review:** 2026-10-11.
**Coordination owner:** repository maintainers. A named assignee is required before each work item starts.
This roadmap does not create an umbrella issue or schedule automation.

## Tracking locations

| Record                                                | Purpose                                                      | Update point                                       |
| ----------------------------------------------------- | ------------------------------------------------------------ | -------------------------------------------------- |
| This roadmap                                          | Priorities, status, and links across systems                 | Each completed increment and each scheduled review |
| [Server-state migrations](server-state-migrations.md) | Resource ownership and Query migration candidates            | The resource's implementation PR                   |
| [Dependency cleanup](dependency-cleanup.md)           | Store typing, package boundaries, and compatibility removal  | The cleanup PR                                     |
| [Linter roadmap](lint-roadmap.md)                     | Existing protections and proposed checks                     | A rule or baseline change                          |
| [Historical audit disposition](historical-audit.md)   | Which July findings remain useful                            | A fresh investigation changes their disposition    |
| Child issue or Kandev task                            | One assignee, bounded scope, and PR link                     | Delivery progress                                  |
| `docs/plans/<initiative>/`                            | Approved implementation scope, work orders, and verification | The implementation PR                              |

Markdown belongs on main. Each change uses a short-lived PR.
An open draft PR is not the permanent status database. PRs and their linked
delivery packages record scoped work. A roadmap item moves to `done` only
after its PR merges.

## Completed milestone: prove the second resource

The milestone is complete. Database statistics moved to Query, a second small
slice was typed, and the first Office alias group was retired. The later backup
list and disk-usage snapshots also moved to Query. Remaining System resources,
Office aliases, and architecture candidates stay individually proposed or
deferred; this roadmap does not select another migration.

| Outcome | Merged evidence |
| --- | --- |
| Typed Azure DevOps slice without root-store casts | [#4009](https://github.com/kdlbs/kandev/pull/4009), `acfba523ff363096e0793045ffc94e27479e8a76` |
| First Office run-alias retirement increment | [#4010](https://github.com/kdlbs/kandev/pull/4010), `5907a4619757254cbb645fad87b20fc3e8742489` |
| Refreshed architecture overview | [#4011](https://github.com/kdlbs/kandev/pull/4011), `d2b66efd7e48fc518a9d62131d6d1392397b43db` |
| Selected official TanStack Query ESLint rules | [#4012](https://github.com/kdlbs/kandev/pull/4012), `a1e2edadb9cd40a08d23a0f9b72665146ec3fea5` |
| Database-statistics Query owner | [#4225](https://github.com/kdlbs/kandev/pull/4225), `059260b30fc68bbcbead629f7fa7e80f1ee0a5e8` |
| Backup-list Query owner | [#4271](https://github.com/kdlbs/kandev/pull/4271), `d149627883ca74de0dde98c1900415729af44bbd` |
| Disk-usage Query snapshot | [#4291](https://github.com/kdlbs/kandev/pull/4291), `122f52018c77b9fb3c5c93f58f52bc7ea9b76904` |
| LINT-02 migrated-System owner guard | [#4357](https://github.com/kdlbs/kandev/pull/4357), `9ad5964ca0a6bdce1e32b462450f7ba78f9cf2b8` |

## Current main snapshot

The measurements below are from `3fed5570cec533f468c25ed03c84967bdc972588`
on 2026-10-09. The dated 2026-09-27 inventory remains historical.

| Inventory | Current count or owner | Evidence |
| --- | ---: | --- |
| `ARCH-FRONTEND-ROOT-STATE-CAST` | 45 findings | The inventory contains three Jira and three Linear creator entries; the reviewed slice-typing plan removes only those six. |
| Compatibility ledger | 29 registrations, including 27 Office run aliases | PR #4010 removed the first four Office alias registrations from the dated count of 31. |
| System server snapshots | Four Query-owned snapshots | About SystemInfo, database statistics, backup list, and disk-usage snapshot; the live System job stream remains in Zustand. |

## Remaining proposed and deferred work

| Item | Status | Next action or reason |
| --- | --- | --- |
| Jira and Linear issue-watch slice typing | Implementation complete, delivery pending in [PR #4375](https://github.com/kdlbs/kandev/pull/4375) | Wait for merge evidence before marking the item done. The reviewed [delivery package](../plans/architecture-maintenance-jira-linear-root-typing/plan.md) records its scope and local results. |
| Further Office run-alias retirement | Proposed | Keep the 27 remaining registrations until each ledger removal condition and its consumers are verified. |
| Other System resources | Deferred | Inventory each resource and its owner, identity, freshness, and event boundary before choosing another migration. |
| Tasks, sessions, integrations, and workspaces | Deferred | Their ownership and event/data contracts need separate inventories and designs. |
| Additional architecture rules and dependency removals | Proposed or needs design | See the [linter roadmap](lint-roadmap.md) and [dependency cleanup](dependency-cleanup.md) for item-specific evidence gaps. |

`Proposed` means no implementation assignment exists. Use `planned` only with
a reviewed delivery package, `in_progress` with an assignee, and `done` with
a merged PR. Use `blocked` or `deferred` with a reason and review date.

## Maintenance procedure

1. At the next review, assign a maintainer and select only explicitly approved small increments.
2. For each increment, record the assignee, task link, scope, completion conditions, and next review date.
3. Create its requirement/design references and finite delivery package before implementation, using the repository's existing workflow.
4. Update the matching tracker row in the implementation PR.
5. Record the merged PR and actual verification results in the work order.
6. At each fortnightly review, resolve blocked items or explicitly defer them with a reason.
7. Each month, run the linter checks and review compatibility targets from their source files.
8. Close the finite milestone before selecting another one.

No scheduled automation or umbrella issue coordinates this cadence. The assigned maintainer owns each review.

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
