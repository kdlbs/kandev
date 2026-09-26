---
id: "05-migrate-plugins-runtimes"
title: "Migrate plugins and runtime surfaces"
status: completed
wave: 5
depends_on:
  - "04-migrate-platform-systems"
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 05: Migrate plugins and runtime surfaces

## Summary

Migrate plugin contracts and the remaining desktop, native CLI, SSH executor,
release, and system-page sources. Keep plugin ownership distinct from host UI
and move implementation-specific technical material into system designs.

## In scope

- `docs/specs/plugins/**`
- `docs/specs/desktop/**`
- `docs/specs/cli/**`
- `docs/specs/executors/**`
- `docs/specs/system-page/**`
- Release, packaging, executor, and storage-maintenance standalone sources.

## Out of scope

- Plugin repository implementation work.
- Changes to release automation or executor code.

## Acceptance

- Plugin, desktop, CLI, executor, and system-page indexes state ownership and
  exclusions clearly.
- Large documents are split into bounded, linked capabilities.
- Existing plans and public plugin references point at the new canonical paths.

## Verification

```bash
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans docs/public
```

## Files likely touched

- `docs/specs/plugins/**`
- `docs/specs/desktop/**`
- `docs/specs/cli/**`
- `docs/specs/executors/**`
- `docs/specs/system-page/**`
- `docs/specs/release/**`

## Dependencies

Task 04.

## Risks

- Host plugin APIs and UI extension points have separate contract owners. Link
  them instead of duplicating the host contract.

## Parallelism

`sequential`

## Inputs

- Plugin authoring guidance and host API contract.
- Desktop, CLI, executor, and storage source documents.
- Relevant ADRs and public documentation.

## Results

Completed. Migrated plugin, desktop, CLI, executor, release, system-page, and
storage-maintenance sources and moved the three misplaced plan files under
`docs/plans/`.
