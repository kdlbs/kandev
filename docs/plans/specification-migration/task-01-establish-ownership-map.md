---
id: "01-establish-ownership-map"
title: "Establish specification ownership map"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 01: Establish specification ownership map

## Summary

Create the system indexes that own the legacy sources and record the
source-to-capability map. Keep each system in `migration: complete` after its
editable legacy files are removed.

## In scope

- Define boundaries and exclusions for agents, auth, costs, integrations,
  Office, platform, plugins, system-page, UI, workspaces, desktop, CLI,
  executors, and release/runtime systems.
- Add `README.md` files with valid system frontmatter and initial
  specification maps.
- Update `docs/specs/INDEX.md` so it is an accurate migration catalog.

## Out of scope

- Moving or rewriting capability documents.
- Changing the completed task-system index.

## Acceptance

- Every legacy source has one owning system and a documented destination.
- Every system with new artifacts has a README with `migration: complete`.
- The catalog contains no links that are already known to be absent.

## Verification

```bash
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs
```

## Files likely touched

- `docs/specs/*/README.md`
- `docs/specs/INDEX.md`

## Dependencies

None.

## Risks

- Incorrect ownership creates duplicate sources later. Resolve ambiguous
  sources by linking to the system that owns the contract.

## Parallelism

`sequential`

## Inputs

- `docs/specs/guide/structure-and-ownership.md`
- `docs/specs/guide/traceability-and-lifecycle.md`
- `docs/specs/templates/system-readme.md`
- `docs/specs/INDEX.md`

## Results

Completed. Added the system READMEs and ownership catalog for all 15 systems,
including dedicated CLI, desktop, executor, release, and residual task
ownership.
