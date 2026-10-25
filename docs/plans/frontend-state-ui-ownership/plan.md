---
created: 2026-09-26
status: implemented
requirements:
  - REQ-UI-SIDEBAR-HOVER-001
  - REQ-UI-PR-ONLY-COMMIT-DETAILS-001
system_design:
  - ../../specs/ui/system-design/sidebar-hover-reveal.md
  - ../../specs/ui/system-design/commit-detail-target-types.md
legacy_specs: []
---

# Implementation Plan: Frontend State/UI Ownership Refactor

## Overview

Move shared diff target contracts and sidebar geometry constants below the UI
component layer. Preserve their shapes, values, persistence identity, and
observable behavior. The refactor changes module ownership and preserves the
existing UI contracts.

## Scope

- Move shared diff target types to `apps/web/lib/state/diff-target-types.ts`
  and migrate state, dockview, desktop, and mobile consumers.
- Move sidebar width constants to `apps/web/lib/layout/app-sidebar-geometry.ts`.
- Keep sidebar section identifiers and presentation classes in the component
  constants module.
- Remove the four exact entries from
  `config/architecture-lint/frontend_state_ui_import.json`, reducing its
  baseline from four edges to zero.

## Existing contracts

- [Sidebar hover reveal](../../specs/ui/requirements/sidebar-hover-reveal.md):
  AC-UI-SIDEBAR-HOVER-001.2, .4, and .5 retain geometry, saved collapse
  behavior, and phone navigation.
- [PR-only commit details](../../specs/ui/requirements/pr-only-commit-details.md):
  AC-UI-PR-ONLY-COMMIT-DETAILS-001.1-.3 and .6 retain source identity and the
  shared desktop/mobile target.
- [Sidebar system design](../../specs/ui/system-design/sidebar-hover-reveal.md)
  and [commit target type design](../../specs/ui/system-design/commit-detail-target-types.md)
  record the ownership boundaries.

## Technical approach

The state store and component consumers import shared types from the neutral
state module. The sidebar state and component import numeric geometry from the
layout module. CSS classes and section identifiers remain component-owned.
Preserve `GitChangeLayer` as a type-only dependency with no reverse import.

The baseline cleanup follows [ADR 2026-08-01: Architecture lint budgets](../../decisions/2026-08-01-architecture-lint-budgets.md):
remove only the exact four grandfathered edges; do not change scanner policy or
other baselines.

## Verification

Run the focused architecture scan and scanner tests. Run the affected frontend
tests, typecheck, and lint. Run the specification checks and the local PR
documentation coverage evaluator. Record results in the work order.

## Work order

- [Task 01: Move shared UI contracts](task-01-move-shared-ui-contracts.md)

## Results

Validation results are recorded in the completed work order.
