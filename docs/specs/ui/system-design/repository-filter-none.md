---
status: current
system: ui
requirements:
  - REQ-UI-REPOSITORY-FILTER-NONE-001
---

# Repository Filter None System Design

## Purpose and boundaries

Expose the existing missing-repository value through the task-view option
provider. UI owns these personal presentation filters, including their
server and client projections.

The implementation reuses the existing filter schema and evaluation rules.
It requires no new API, storage field, operator, or migration.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-REPOSITORY-FILTER-NONE-001` | Option provider, Matching and persistence, Responsive interaction, Verification |

## Option provider

In `apps/web/components/task/sidebar-filter/use-filter-value-options.ts`,
prepend a synthetic option only in the repository branch of
`useFilterValueOptions`. Keep `repositoryOptions` responsible for named
repository slugs. Preserve their values and source order.

Use the persisted string `"undefined"` as the option value. This is the existing
missing-dimension representation, rather than JavaScript `undefined`, JSON null,
an empty string, or the visible label. Explain this non-copy value with an
`i18n-exempt` line comment if a literal needs an exemption.

Resolve the label with `t("task:none")` during hook evaluation. This key exists
in English, all six translated catalogs, and pseudo. Retain the active locale
as a memo dependency so the label changes without a repository-store update.

`FilterClauseEditor` already sends dynamic options to
`TypedFilterClauseEditor`. Both single-value and list controls receive the
option through this existing path. Other dimension providers remain separate.

## Matching and persistence

Existing matching already supports the value:

- `apply-view.ts` compares the string form of the primary `repositoryPath`.
- `sidebar-local-projection.ts` passes the same string form to
  `matchesSidebarClause` in `sidebar-local-filter.ts`.
- Backend `sidebar_task_query_repositories.go` projects a missing resolved
  repository as `"undefined"`. `sidebarClauseCondition` in
  `sidebar_task_query_base.go` applies the existing operators to that value.

An `in` array containing `"undefined"` and a repository slug therefore produces
the requested union. Existing negative operators produce the complement.
An empty `in` array matches nothing, and an empty `not_in` array excludes nothing.
Other clauses retain their existing conjunction behavior.

Keep named-repository matching on its current primary compatibility value.
Repository grouping continues to use complete combinations under the
[grouping contract](sidebar-repository-grouping.md).

Existing workspace-scoped drafts and saved views carry ordinary string values.
Save and restore the missing value through that path. Merely opening or
searching the picker must not change the clause.

This extends the existing representation without adding a new architectural
constraint. No new ADR is required. Backend-owned preference storage remains
governed by [ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md).

## Responsive interaction

Use the shipped `SidebarFilterPopover` and `MobileSidebarFilterSurface` as
the nearest desktop and phone exemplars. The phone entry remains the task
picker followed by its filter gear. The existing drawer contains the editor.

The searchable value popover remains appropriate for this short temporary
choice under the mobile UI language. Keep search above `CommandList`, which
owns option scrolling. Retain drawer safe-area handling and viewport bounds.

The synthetic row uses the existing `FilterMultiSelectOptions` rendering.
Selected-first partitioning, search ranking, checked indicators, keyboard
activation, and focus return therefore remain shared. Reuse existing surfaces,
icons, tokens, and motion. No new row animation is needed.

Preserve desktop density and existing phone/coarse-pointer row sizing.
Measure the phone row after finite animations settle to prove its actual
target is at least 44px. Check viewport containment and document overflow.

## Failure and compatibility

The synthetic option remains available with an empty repository catalog.
Unknown saved repository values retain the existing behavior. This change
does not reinterpret incomplete metadata or dangling repository links.

The current missing-value representation can also equal a repository slug
literally named `undefined`. This pre-existing ambiguity remains part of the
current scalar filter contract. A new identity model would require separate scope.

## Verification

Hook tests cover an empty catalog, named options, dimension isolation, and a
locale switch with unchanged store data. Shared conformance fixtures exercise
missing-only and mixed positive/negative clauses in local and SQL projections.
`applyFilters` tests cover the compatibility projection.

Desktop and phone E2E tests select None, combine it with a repository, remove it,
and save/reload the result. Existing selected-first browser tests must account
for the additional option without weakening their ordering or geometry checks.

## Delivery

- [Plan and work order](../../../plans/repository-filter-none/plan.md)
