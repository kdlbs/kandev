---
id: "01-lifecycle-evidence-guidance"
title: "Add lifecycle evidence guidance"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-IMPROVE-VALIDATION-001
acceptance_criteria:
  - AC-WORKSPACES-IMPROVE-VALIDATION-001.1
  - AC-WORKSPACES-IMPROVE-VALIDATION-001.2
  - AC-WORKSPACES-IMPROVE-VALIDATION-001.3
  - AC-WORKSPACES-IMPROVE-VALIDATION-001.4
  - AC-WORKSPACES-IMPROVE-VALIDATION-001.5
  - AC-WORKSPACES-IMPROVE-VALIDATION-001.6
  - AC-WORKSPACES-IMPROVE-VALIDATION-001.7
system_design:
  - ../../specs/workspaces/system-design/improve-kandev-validation.md
---

# Task 01: Add lifecycle evidence guidance

## Summary

Require relevant lifecycle scenarios and exact evidence in Improve work orders and completion reports.

## In scope

- Add one conditional lifecycle reference linked from Improve, plan, TDD, and existing review guidance.
- Clarify durable intent, shared wakeup dispatch, and low-level run admission in Office guidance.
- Use the current work order's checks and existing isolated-instance procedure.
- Preserve planning, manual phase transitions, model choice, and publication authorization.

## Out of scope

Unrelated behavior changes, generic verification passes, and automatic publication.

## Acceptance

1. Each relevant scenario names its owning AC, observable assertion, test, command, result, or justified exclusion.
2. Missing evidence remains visible. The workflow cannot describe a blocked scenario as completely verified.
3. One reference owns the checklist. No duplicate ledger, generic review gate, or unrelated test requirement is added.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath ./config/workflows -count=1)
python3 scripts/lint-harness-files.test.py
python3 .github/scripts/lint-harness-files.py --all
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- .agents/skills apps/backend/config/workflows/improve-kandev.yml apps/backend/internal/office/AGENTS.md
```

## Files likely touched

- `.agents/skills/tdd/references/lifecycle-evidence.md (new)`
- `.agents/skills/tdd/SKILL.md`
- `.agents/skills/plan/SKILL.md`
- `.agents/skills/code-review/SKILL.md`
- `apps/backend/config/workflows/improve-kandev.yml`
- `apps/backend/internal/office/AGENTS.md`

## Dependencies

None.

## Risks

Instruction lint protects shape and routing, not feature semantics. Avoid prose snapshot tests that only mirror the new instructions.

## Parallelism

`sequential`

## Inputs

Read Lifecycle evidence and Office boundary proof in the validation design, the phase template, and existing TDD concurrency guidance.

## Results

Completed. Added the single lifecycle evidence checklist, linked it from Improve, planning, TDD, and review guidance, and clarified Office wakeup ownership.

Verification passed:

- `(cd apps/backend && go test -trimpath ./config/workflows -count=1)`
- `python3 scripts/lint-harness-files.test.py` (19 tests)
- `python3 .github/scripts/lint-harness-files.py --all` (205 files)
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check -- .agents/skills apps/backend/config/workflows/improve-kandev.yml apps/backend/internal/office/AGENTS.md`
