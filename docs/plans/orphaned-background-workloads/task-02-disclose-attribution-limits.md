---
id: "02-disclose-attribution-limits"
title: "Disclose orphan attribution limits"
status: done
wave: 2
depends_on:
  - "01-attribute-orphaned-workloads"
plan: "plan.md"
requirements:
  - REQ-DW-ORPHAN-002
acceptance_criteria:
  - AC-DW-ORPHAN-002.6
system_design:
  - ../../specs/disambiguate-waiting/system-design/orphaned-background-workloads.md
---

# Task 02: Disclose Orphan Attribution Limits

## Summary

The legacy failure-modes table in `docs/specs/disambiguate-waiting/spec.md` has
no row for a reparented workload, which is why the behaviour read as intended
rather than as a defect. Add the row, stating which platforms attribute the
workload and what the rest report.

## In scope

- One row in the legacy failure-modes table.
- A pointer from the legacy probe section to the new requirement and design
  documents, so a reader of `spec.md` finds the current contract.

## Out of scope

- Rewriting or migrating the rest of `spec.md`. The system README records
  `migration: in_progress` deliberately.
- Any behaviour change; task 01 owns all of it.

## Acceptance conditions

1. The failure-modes table names the reparented-workload condition and its
   behaviour per platform.
2. The row matches what task 01 actually implemented, not what was planned.

## Verification

```sh
/opt/homebrew/bin/python3 scripts/lint-spec-files.py --all
/opt/homebrew/bin/python3 scripts/list-docs.py validate
```

Use any Python 3.10 or newer; the host default `python3` is 3.9.6 and fails
before linting.

## Likely files

- `docs/specs/disambiguate-waiting/spec.md`

## Risks

- `spec.md` has a 32 KiB legacy ceiling. One row is well inside it, but check
  the linter output rather than assuming.
