---
status: current
system: ui
requirements:
  - REQ-UI-PR-ONLY-COMMIT-DETAILS-001
---

# Commit Detail Target Types System Design

## Context and ownership

The Changes panel uses a shared source-aware contract when opening commit
details. Its pure TypeScript definitions live in
`apps/web/lib/state/diff-target-types.ts`, below the component layer. The
module defines `CommitDetailTarget`, `OpenDiffOptions`, `DiffSheetMode`,
`DiffSource`, and the `ChangeLayer` alias. It owns type shapes and source
identity; it does not implement navigation or request behavior.

## Requirement mapping

| Criteria for REQ-UI-PR-ONLY-COMMIT-DETAILS-001 | Design section                   |
| ---------------------------------------------- | -------------------------------- |
| .1                                             | Source-aware target contract     |
| .2, .3                                         | Local and GitHub target identity |
| .6                                             | Shared detail consumers          |

## Source-aware target contract

`CommitDetailTarget` is a discriminated union with `source: "local"` and
`source: "github"` variants. Both retain the exact `sha`. The GitHub variant
also retains `workspaceId`, `owner`, and `repo`; optional `repositoryName` is a
local display/group identity. The local variant may carry the optional local
repository subpath. This shape preserves one explicit source identity across a
commit row and its detail view.

`DiffSource`, `OpenDiffOptions`, and `DiffSheetMode` carry the existing diff
selection and sheet modes. `ChangeLayer` aliases `GitChangeLayer` from
`lib/state/slices/session-runtime/types.ts` through a type-only import. That
runtime type module does not import this contract, keeping the dependency
acyclic.

## Local and GitHub target identity

The target factories in `components/task/changes-panel-helpers.ts` retain
local-versus-GitHub source selection. A GitHub target carries the selected
workspace, owner, repository, and SHA. The shared types constrain the payload.
The factories and consumers preserve the existing behavior in
[REQ-UI-PR-ONLY-COMMIT-DETAILS-001](../requirements/pr-only-commit-details.md),
AC .1-.3.

## Shared detail consumers

State actions and the dockview store import these contracts as types. Desktop
and mobile consumers share `CommitDetailTarget` and keep source-aware data, as
required by AC .6. The definitions in `lib/state` remove the
component-to-state reverse dependency. They add no runtime module edge.

## Related delivery and decision

- [Frontend state/UI ownership refactor](../../../plans/frontend-state-ui-ownership/plan.md)
- [ADR 2026-08-01: Architecture lint budgets](../../../decisions/2026-08-01-architecture-lint-budgets.md)
