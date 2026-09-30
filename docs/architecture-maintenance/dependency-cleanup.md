# Dependency cleanup

[Roadmap](README.md) · Inventory at main `359b5ffdbb6`, 2026-09-27.

The [baseline files](../../config/architecture-lint/) and compatibility ledger own current counts.
Numbers here describe this dated inventory, not a second live database.

| ID     | Boundary                       | Inventory                                              | Status              | Next bounded result                                                                |
| ------ | ------------------------------ | ------------------------------------------------------ | ------------------- | ---------------------------------------------------------------------------------- |
| DEP-01 | Typed root-store composition   | 46 unsafe-cast findings after Features removed three   | Proposed            | One small slice accepts its actual dependencies without assertions                 |
| DEP-02 | Office run aliases             | 31 registered aliases after shared contract extraction | Proposed            | Migrate a coherent consumer group, then remove aliases with no remaining consumers |
| DEP-03 | Runs importing Office          | Six exact edges remain                                 | Needs design        | Classify policy adapters before selecting an edge                                  |
| DEP-04 | Runtime implementation imports | 61 exact findings                                      | Needs investigation | Select one caller group and identify missing facade capability                     |
| DEP-05 | Task importing Office          | Seven exact findings                                   | Needs design        | Define the required domain contract without copying Office policy                  |
| DEP-06 | Unregistered deprecations      | 15 exact declarations                                  | Proposed            | Triage each declaration for removal or justified registration                      |

## DEP-01: type one slice

Use the [Features delivery record](../plans/features-slice-root-typing/plan.md) as evidence, not a universal setter template.
That slice needs only an Immer recipe setter. Other slices can need getters or additional operations.
Avoid a root-store redesign or new unsafe casts elsewhere.

Completion requires root composition typechecks, focused slice tests, and unchanged unrelated root-state references.
The same PR removes only the obsolete baseline entries.

## DEP-02: retire Office aliases

The [shared-run plan](../plans/shared-run-contract-ownership/plan.md) records the completed ownership split.
`internal/runs/models` owns generic run data. Office retains its launch and safety policies.
Temporary aliases in `internal/office/models/run_compat.go` preserve existing callers.

The ledger contains 31 declaration registrations for these aliases: eight types and 23 constants.
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
