---
id: "03-migrate-integrations-office"
title: "Migrate integration and Office systems"
status: completed
wave: 3
depends_on:
  - "02-migrate-core-systems"
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 03: Migrate integration and Office systems

## Summary

Move provider integrations and autonomous Office specifications into their
system-owned requirement and design trees. Split oversized provider and Office
documents by capability or lifecycle before they exceed the new limits.

## In scope

- `docs/specs/integrations/**`
- GitLab, Jira, Azure DevOps, Bitbucket, and provider-aware root sources.
- `docs/specs/office/**`
- Office routing, scheduler, runtime, automation, inbox, dashboard, testing,
  and tier-selection sources.

## Out of scope

- Generic task lifecycle requirements owned by `docs/specs/tasks/`.
- Public plugin repositories and their authoring documentation.

## Acceptance

- Integration and Office indexes identify one authoritative document per
  capability and link cross-system dependencies.
- Oversized sources are split below the requirement/design limits without
  dropping source detail or stable IDs.
- All migrated files have valid owner/status frontmatter and no duplicate IDs.

## Verification

```bash
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans
```

## Files likely touched

- `docs/specs/integrations/**`
- `docs/specs/office/**`
- `docs/specs/integrations/**`
- `docs/plans/**`

## Dependencies

Task 02.

## Risks

- Provider-specific sources reference shared authentication and task
  contracts. Link to those owners and update all relative paths together.

## Parallelism

`sequential`

## Inputs

- Integration and Office system READMEs.
- `docs/specs/guide/traceability-and-lifecycle.md`
- Current source files and their related plans and ADRs.

## Results

Completed. Migrated all integration and Office sources, including oversized
documents split into linked design parts, and repaired their cross-system
references.
