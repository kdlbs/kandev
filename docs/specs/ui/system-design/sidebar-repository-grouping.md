---
status: current
system: ui
requirements:
  - REQ-UI-SIDEBAR-REPOSITORY-GROUPING-001
---

# Sidebar Repository Grouping System Design

## Purpose and boundaries

The task store supplies ordered repository links. The workspace store supplies repository records for user-facing slugs.

The sidebar projection combines these sources before `applyView` groups tasks. This design changes no backend API or saved-view schema.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-SIDEBAR-REPOSITORY-GROUPING-001` | [Projection contract](#projection-contract), [Group identity](#group-identity), [Responsive behavior](#responsive-behavior) |

## Components and responsibilities

- `repositorySlug` supplies one canonical slug for each workspace repository.
- `buildSidebarItem` projects repository slugs for the desktop sidebar.
- `toSheetItem` projects the same slugs for the mobile task-switcher surface.
- `TaskSwitcherItem` carries the ordered repository slugs and the persisted repository links.
- `applyGroup` creates repository group keys and labels from the projected slugs.
- `TaskSwitcher` renders the same grouped model on desktop and mobile.

## Projection contract

Each sidebar projector resolves every task repository through the workspace repository map. The projector keeps the task link order.

The projector writes the resolved slugs to `TaskSwitcherItem.repositories`. It also keeps `repositoryLinks` so grouping can detect incomplete metadata.

The existing `repositoryPath` remains the single-repository and compatibility value. Repository filters continue to use this primary value. A pull-request summary can continue to supply it.

## Group identity

For one repository, `applyGroup` keeps the canonical slug as the key and label. This behavior preserves existing single-repository groups.

For a complete repository combination, the label uses `repositories.join(", ")`. The internal key uses a fixed prefix and a JSON array encoding. Distinct links can retain the same canonical slug, so the ordered array is not deduplicated by display value.

This encoding prevents collisions when a repository slug contains a separator. It also keeps distinct ordered combinations in distinct groups.

Multi-repository combination groups stay before single-repository groups. Combination groups sort by their display labels.

When metadata is incomplete, `repositoryLinks.length` remains greater than the resolved slug count. The task uses the existing generic multi-repository group.

After repository hydration, the projectors recompute the item. Then `applyView` moves the task to its named combination group.

## Responsive behavior

Desktop and mobile keep their current sidebar containers, scroll owners, and controls. Both surfaces pass the same `TaskSwitcherItem` contract to `applyView`.

The mobile exemplar is `SessionTaskSwitcherSheet`. This fix changes shared data normalization and does not change mobile composition or touch behavior.

## Tests

Projection tests cover desktop and mobile mappings. They use two ordered task links and two workspace repository records.

Grouping tests cover complete combinations, different combinations, incomplete metadata, and single-repository compatibility.

Desktop and mobile Playwright tests seed one task with two repositories. Each test asserts the complete heading and the task location.

## Failure and recovery

An unknown repository record does not hide the task or show an empty heading. The generic multi-repository group remains until metadata resolves.

## Persistence

This design adds no persisted field. Existing collapsed keys for the generic multi-repository group do not apply to named combination groups.

The new combination key is deterministic for the same ordered slugs. Existing single-repository collapsed keys remain unchanged.

## Security

The UI uses repository names that the workspace repository store already exposes. This design adds no new data source or permission boundary.
