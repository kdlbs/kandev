---
created: 2026-10-29
status: complete
requirements:
  - REQ-TASKS-MODEL-UNIFICATION-001
system_design:
  - ../../specs/tasks/system-design/model-unification.md
legacy_specs: []
---

# Implementation Plan: Hold steps by stored nature

## Overview

A workflow step's hold nature was read from the step's name. This plan makes
`stage_type="hold"` a stored classification of a step's nature, carries it
through portable export, import, and workflow sync, converts existing hold-named
steps once, and exposes it in the step editor. The value is a classification
only: it stops no run and moves no task.

## Scope

- Workflow step model, validators, and normalizers; the portable export/import
  and sync carry; a one-time backfill of hold-named steps and templates; the
  settings step editor and the locale catalogs.
- No new schema, public field, or event-ordering change beyond the existing
  `workflow_steps.stage_type` column. The office-overview reader that consumed
  the name-based hold reading is outside this plan.

## Work orders

- [x] [Task 01: Store the hold step nature](task-01-hold-stage-type-nature.md) (done)

## Verification

- Focused Go tests for the workflow engine, repository, service, and config
  loader, plus a one-off backfill and export round-trip check.
- Changed-delta formal review on the implementation head; the same head is not
  retested without new code or failure evidence.
