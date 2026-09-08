---
id: "06-migrate-ui-standalone"
title: "Migrate UI and standalone sources"
status: completed
wave: 6
depends_on:
  - "05-migrate-plugins-runtimes"
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 06: Migrate UI and standalone sources

## Summary

Move UI capability documents and the remaining root feature specs into the UI
system or their owning backend system. Preserve responsive and mobile behavior
as part of each capability's observable requirements.

## In scope

- `docs/specs/ui/**`
- Walkthrough, quick-chat, PR, settings, kanban, and other presentation roots
  assigned to UI.
- Standalone sources that are presentation-owned after the ownership audit.

## Out of scope

- New UI behavior.
- Backend contracts already owned by tasks, integrations, plugins, or platform.

## Acceptance

- UI requirements include desktop and mobile observable criteria where the
  source describes both surfaces.
- Technical UI architecture is separated into design documents when it has an
  independent boundary.
- The UI index reaches `migration: complete` with no editable legacy source.

## Verification

```bash
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans
```

## Files likely touched

- `docs/specs/ui/**`
- `docs/specs/ui/**`
- `docs/specs/ui/requirements/*walkthrough*.md`
- `docs/specs/ui/requirements/*quick-chat*.md`

## Dependencies

Task 05.

## Risks

- Some UI specs define API or persistence contracts owned elsewhere. Keep the
  UI requirement focused on observable presentation and link the owning design.

## Parallelism

`sequential`

## Inputs

- UI system README.
- `mobile-parity` guidance when a source covers responsive behavior.
- Existing UI plans and E2E references.

## Results

Completed. Migrated UI and remaining presentation-owned standalone sources,
including their responsive behavior and design material.
