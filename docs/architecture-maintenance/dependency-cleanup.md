# Dependency cleanup

[Roadmap](README.md) · Historical inventory at main `359b5ffdbb6`, 2026-09-27.
Current root-cast and compatibility measurements are recorded separately below.

The [baseline files](../../config/architecture-lint/) and compatibility ledger own current counts.
Values labeled as dated inventory remain historical. Counts labeled current
main are measurements of the named current snapshot below, not a second live
database.

| ID     | Boundary                       | Inventory                                              | Status              | Next bounded result                                                                |
| ------ | ------------------------------ | ------------------------------------------------------ | ------------------- | ---------------------------------------------------------------------------------- |
| DEP-01 | Typed root-store composition   | 45 findings on current main; the Jira/Linear increment removes six exact entries | Implementation complete, delivery pending in [#4375](https://github.com/kdlbs/kandev/pull/4375) | Wait for merge evidence; Azure DevOps is complete in [#4009](https://github.com/kdlbs/kandev/pull/4009). |
| DEP-02 | Office run aliases             | 31 aliases in the dated inventory; 27 remain on current main | Proposed, first increment complete | Migrate a coherent consumer group, then remove aliases only when their ledger conditions pass ([#4010](https://github.com/kdlbs/kandev/pull/4010)). |
| DEP-03 | Runs importing Office          | Six exact edges in the dated inventory                 | Needs design        | Classify policy adapters before selecting an edge                                  |
| DEP-04 | Runtime implementation imports | 61 exact findings in the dated inventory              | Needs investigation | Select one caller group and identify missing facade capability                     |
| DEP-05 | Task importing Office          | Seven exact findings in the dated inventory           | Needs design        | Define the required domain contract without copying Office policy                  |
| DEP-06 | Unregistered deprecations      | 15 exact declarations in the dated inventory          | Proposed            | Triage each declaration for removal or justified registration                      |

## Current snapshot

At main `3fed5570cec533f468c25ed03c84967bdc972588` on 2026-10-09, the
root-state-cast inventory contains 45 entries. The compatibility ledger has
29 registrations, including 27 Office run aliases. The 2026-09-27 counts in
the table remain historical measurements; other rule counts have not been
remeasured here.

## DEP-01: type one slice

Use the [Features delivery record](../plans/features-slice-root-typing/plan.md) as an example, not a universal setter template.
The Azure DevOps slice uses only an Immer recipe setter; other slices can need getters or additional operations.
Avoid a root-store redesign or new unsafe casts elsewhere.

The Azure DevOps slice now accepts its recipe-only setter directly, and root composition passes `set` without an assertion.
The change removed exactly one obsolete baseline entry, reducing the count from 46 to 45.
Focused Azure, root-store, and hydration tests passed (47 tests); web typecheck and lint, architecture lint, all 99 architecture-lint tests, and `git diff --check` passed.
Unrelated task and workspace state references remain unchanged.
Delivery: [PR #4009](https://github.com/kdlbs/kandev/pull/4009).

The Jira and Linear issue-watch slices now accept only their Immer recipe
setters, and root composition passes `set` directly. The focused regressions
cover the existing initial-state merge, hydration behavior, separate-store
isolation, and unrelated root references. The exact root-state-cast inventory
fell from 45 to 39 by removing only the three Jira and three Linear entries.
Provider, API, cache, and UI behavior remains unchanged. Local verification is
recorded in the reviewed [delivery plan](../plans/architecture-maintenance-jira-linear-root-typing/plan.md).
Delivery is pending under [PR #4375](https://github.com/kdlbs/kandev/pull/4375).

The first Office alias-retirement increment removed four registrations in
[PR #4010](https://github.com/kdlbs/kandev/pull/4010). The current ledger still
contains 27 Office aliases; each remaining registration keeps its own removal
condition.

## DEP-02: retire Office aliases

The [shared-run plan](../plans/shared-run-contract-ownership/plan.md) records the completed ownership split.
`internal/runs/models` owns generic run data. Office retains its launch and safety policies.
Temporary aliases in `internal/office/models/run_compat.go` preserve existing callers.

The dated inventory contained 31 declaration registrations for these aliases: eight types and 23 constants.
Each alias has its own removal condition.
Consumer migration can span small PRs, but alias deletion and ledger deletion belong in the same PR.
Do not replace these aliases with another compatibility barrel.

The first task must enumerate production and test consumers by symbol.
Text searches help inventory callers, but deleting the alias and compiling provides stronger removal evidence.
Use each ledger entry's stated verification, plus the affected Runs/Office tests.
No schema, serialized value, or launch policy change belongs in this cleanup.

## DEP-03: classify the six remaining edges

| Source under `apps/backend/internal/runs/` | Dependency      | Responsibility to preserve                 |
| ------------------------------------------ | --------------- | ------------------------------------------ |
| `repository/sqlite/causation_refusal.go`   | `office/models` | Office-specific durable refusal records    |
| `repository/sqlite/gate_failure_state.go`  | `office/models` | Office-specific durable gate state         |
| `repository/sqlite/runs.go`                | `office/models` | Office continuation policy                 |
| `repository/sqlite/claim.go`               | `office/shared` | Priority policy or related shared behavior |
| `service/causation.go`                     | `office/shared` | Office policy or metrics                   |
| `service/service.go`                       | `office/shared` | Office policy or metrics                   |

A smaller count is not sufficient evidence of a better boundary.
Do not move Office policy into generic Runs solely to satisfy the linter.
An adapter or contract change needs an approved ownership design and behavioral tests first.

## Compatibility review

The [ledger](../../config/architecture-lint/compatibility-ledger.json) contains 33 entries at this inventory.
All have a target date of **2027-02-01**, including the 31 Office aliases.
The other entries also need review by their recorded owners.

**Proposed decision checkpoint:** 2027-01-15, before the shared deadline.
Date targets remain valid through their target day and fail afterward.
SemVer targets are review checkpoints, not automatic calendar expiry.

At each monthly review:

1. Read owners, removal conditions, and targets from the ledger.
2. Assign the next removable group to a bounded task.
3. Record blockers before the January checkpoint.
4. If removal is unsafe, document a reviewed reason and revised target per entry.

Do not extend every date merely to make CI pass.
Registering a legacy deprecation improves accountability but does not remove the underlying compatibility cost.
