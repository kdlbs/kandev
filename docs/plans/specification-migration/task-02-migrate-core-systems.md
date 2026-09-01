---
id: "02-migrate-core-systems"
title: "Migrate core ownership systems"
status: completed
wave: 2
depends_on:
  - "01-establish-ownership-map"
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 02: Migrate core ownership systems

## Summary

Migrate the agent, authorization, cost, and workspace sources into their
owning system indexes. Add stable requirement and acceptance-criterion IDs,
split technical material when it has an independent boundary, and remove the
old editable paths after each system is complete.

## In scope

- `docs/specs/agents/**`
- `docs/specs/auth/**`
- `docs/specs/costs/**`
- `docs/specs/workspaces/**`
- Standalone agent, credential, workspace, and runtime sources assigned by the
  ownership map.

## Out of scope

- Office-specific autonomous-agent behavior.
- Integration-provider contracts.

## Acceptance

- Each migrated capability has observable requirements with matching AC IDs.
- Technical contracts are represented by a design document or explicitly
  documented as part of the requirement when no separate design is needed.
- The four systems reach `migration: complete` with no editable legacy source.

## Verification

```bash
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans
```

## Files likely touched

- `docs/specs/agents/**`
- `docs/specs/auth/**`
- `docs/specs/costs/**`
- `docs/specs/workspaces/**`
- `docs/plans/**`

## Dependencies

Task 01.

## Risks

- Agent and workspace contracts overlap with task and Office behavior. Preserve
  ownership links instead of copying task requirements.

## Parallelism

`sequential`

## Inputs

- The four system READMEs from Task 01.
- `docs/specs/guide/requirements.md`
- `docs/specs/guide/system-design.md`
- `docs/specs/templates/requirement.md`
- `docs/specs/templates/system-design.md`

## Results

Completed. Migrated agents, auth, costs, workspaces, and standalone sources
assigned to those owners into canonical requirements and bounded system-design
documents.
