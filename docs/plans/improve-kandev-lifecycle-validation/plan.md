---
created: 2026-11-16
status: complete
requirements:
  - REQ-WORKSPACES-IMPROVE-VALIDATION-001
  - REQ-WORKSPACES-IMPROVE-VALIDATION-002
  - REQ-OFFICE-BLOCKER-POLICY-002
system_design:
  - ../../specs/workspaces/system-design/improve-kandev-validation.md
  - ../../specs/office/system-design/blocker-terminal-policy.md
legacy_specs: []
---

# Implementation plan: Improve lifecycle validation

## Overview

Make lifecycle proof explicit, upgrade known stored instructions, and add persisted blocker-recovery coverage.
Deliver the guidance before its upgrade path. Then complete the independent Office regression proof.
The implementation remains in the primary conversation.

## Scope

Include lifecycle evidence, canonical Office entry-point guidance, prompt upgrades, and real database-reopen tests.
Exclude new approval gates, automatic phase moves, a general migration framework, and unrelated Office behavior changes.
Screenshot policy has a separate [plan](../ui-screenshot-enforcement/plan.md).

## Technical approach

Follow the [validation design](../../specs/workspaces/system-design/improve-kandev-validation.md).
Both stored template rows and materialized steps need conservative prompt reconciliation.
Do not select Improve instances by `is_system` alone.
Use reviewed historical content and exact-value conditional writes.
Preserve customized prompts, unrelated fields, identities, and accepted turns.

Add a file-backed fixture and reconstruct real Office services after store reopen.
The [Office design](../../specs/office/system-design/blocker-terminal-policy.md) retains delivery ownership.

## Compatibility and tests

| Surface or AC | Result and primary proof |
| --- | --- |
| IMPROVE-VALIDATION-001.1 through .7 | Scenario mapping in work orders. Existing workflow loader and harness lint protect instruction delivery. Review checks semantic coverage. |
| IMPROVE-VALIDATION-002.1 through .7 | New template, existing template, and reused instance cases. SQLite reopen and PostgreSQL tests protect both storage layers. |
| Customized or unknown prompts | Exact-content and concurrent-edit cases preserve user changes. |
| Active conversations | Identity and transcript assertions preserve accepted turns. |
| OFFICE-BLOCKER-POLICY-002.4, .6, .8 | Reopened-store regression, existing occurrence/merge/prompt tests, and intentional skip-on-pause positive controls. |

There is no rendered UI change. Product mobile layout and new browser capture tests are unnecessary.
The Test-phase instructions still give users the relevant isolated-instance exercise.

## Work orders

- [x] [Task 01: Add lifecycle evidence guidance](task-01-lifecycle-evidence-guidance.md)
- [x] [Task 02: Upgrade stored Improve prompts](task-02-upgrade-stored-prompts.md)
- [x] [Task 03: Prove blocker replay after restart](task-03-blocker-restart-proof.md)

Execute sequentially. Task 02 uses the final supplied prompts from Task 01.
Task 03 changes the Office regression boundary without changing prompt migration internals.

## Verification results

All three lifecycle work orders are complete. Their focused checks, prompt
repository checks, harness lint, specification validation, and whitespace
checks are recorded in each work order. The Office replay regression passed
under `-race`; its temporary mutation proved that missed-hook recovery depends
on the durable replay path.

The PostgreSQL prompt-upgrade test compiled but was skipped because
`KANDEV_TEST_POSTGRES_DSN` was unset. SQLite reopen and store-conformance checks
passed; live PostgreSQL behavior remains unverified in this environment.

## Risks

Unknown historical prompts remain unchanged. Include only revisions supported by exact repository evidence.
JSON updates must retain unknown properties and concurrent edits.
The blocker recovery tests use a file-backed database and reconstructed services.

## Design validation

- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local `pr-docs.cjs` reference preflight: both packages passed with zero reference errors. Rerun after scope changes.

The reference preflight supplied a planned runtime path to force graph validation.
It did not evaluate an implemented runtime diff or establish semantic coverage.
