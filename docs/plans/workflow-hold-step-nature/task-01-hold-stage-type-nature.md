---
id: "01-hold-stage-type-nature"
title: "Store the hold step nature"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MODEL-UNIFICATION-001
acceptance_criteria:
  - AC-TASKS-MODEL-UNIFICATION-001.7
system_design:
  - ../../specs/tasks/system-design/model-unification.md
---

# Task 01: Store the hold step nature

## Summary

Whether a task was held depended on the step's name, so renaming a hold step
dropped the reading and a step merely named like a hold was misread. Make
`stage_type="hold"` a stored classification of the step's nature: accept it in
the model, validators, and normalizers, carry it through export, import, and
workflow sync, convert existing hold-named steps once, and let the editor set
it.

## In scope

- `internal/workflow/models/phase2.go` gains `StageTypeHold`; the step model and
  portable step carry an optional `stage_type`.
- `engine.ValidStageType`, config loader `convertStageType`, workflow repository
  `normalizeStageType`, and task bootstrap `normalizeBootstrapStageType` accept
  `hold`.
- Export copies the stored value; import and sync apply an explicit value while
  preserving the matched existing type when the source omits it.
- A one-time, marker-guarded backfill converts hold-named workflow steps and
  template steps to `stage_type="hold"`.
- The settings step editor gains a hold checkbox; web types gain `hold`; the
  locale catalogs gain the new strings.

## Out of scope

- Any overview surface that consumes the hold reading, and any schema, public
  field, or event-ordering change.
- Making the classification start, stop, move, or gate a task.

## Acceptance

- A stored step with `stage_type="hold"` round-trips through export and import
  and survives workflow sync; an omitted value keeps the matched existing type.
- A step named like a hold before the change is converted once and its previous
  reading is preserved; a non-hold or differently named step is untouched.
- The value does not start, stop, move, or gate a task, and the existing
  `work`/`review`/`approval`/`custom` handling is unchanged.

## Verification

- Focused Go tests pass for the workflow engine, repository, service, and config
  loader; a one-off check exercises the backfill and the export round-trip.
- Changed-delta formal review approved the implementation head.
