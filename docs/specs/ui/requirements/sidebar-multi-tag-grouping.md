---
status: draft
system: ui
created: 2026-11-17
owners:
  - kandev
---

# Multi-tag Combinations in Saved Sidebar Views

## Ownership and intent

UI owns this extension to the reusable saved-sidebar grouping interaction.
Tags retains shared tag ownership and classification; task state is unchanged.
The existing [complete grouping contract](sidebar-plugin-grouping.md) remains
applicable, including its authorization, tree, persistence and phone behavior.

The user selected every nonempty combination, not exact-set-only grouping or
nested tag paths. For a root with distinct tags A, B and C, its matching groups
are A; B; C; A+B; A+C; B+C; A+B+C. Singletons are intentionally included.

## Requirements

### REQ-UI-SIDEBAR-MULTI-TAG-001: Complete combination grouping

#### Acceptance criteria

- **AC-UI-SIDEBAR-MULTI-TAG-001.1:** A compatible active Tags plugin shall offer a separate Multi-tag saved-sidebar grouping alongside unchanged Tag. Selecting it shall use the existing normal generated/saved-view behavior on desktop and phone, including draft/view-limit protection, reuse, rename, duplicate, reload and workspace isolation.
- **AC-UI-SIDEBAR-MULTI-TAG-001.2:** For each root's distinct effective shared tags, Multi-tag shall include that root once under every nonempty subset, including singletons and its complete tag set, and under no subset containing a tag it lacks. Human/agent duplicate applications shall not multiply memberships. No-tag roots shall appear only in Untagged. Existing descendant-tree inheritance shall apply in each occurrence; no tag or task mutation is implied.
- **AC-UI-SIDEBAR-MULTI-TAG-001.3:** Combination identity shall depend on member tag identities, not application order, names or colors. Rename/recolor shall refresh headings without losing the disclosure of an unchanged combination. Addition/removal/deletion shall update all affected combinations without reload, with no stale workspace/provider result crossing existing fences.
- **AC-UI-SIDEBAR-MULTI-TAG-001.4:** Complete authorized filtered-root grouping shall precede the existing 100-occurrence pagination boundary. Unique task totals shall remain unique; occurrences and continuation shall count every visible repeated tree. Pins, task-scoped expansion/selection/actions and independent group collapse shall retain existing behavior.
- **AC-UI-SIDEBAR-MULTI-TAG-001.5:** Combination expansion shall retain the existing maximum of 1,000 values, 10,000 memberships, 256-byte value labels and 1 MiB response. Overflow, corruption or incompatible providers shall fail explicitly through the existing unavailable/retry flow, never return partial combinations or falsely classify tasks as Untagged. Existing Tag mode shall remain independently usable when only combination expansion exceeds its limits.

## Boundaries

Only workspace-shared effective tags participate. No private/legacy browser tag
merge, exact-set mode, nested hierarchy, new settings, raised transport limits,
automatic page fetching or change to the page-local /tasks facet is included.
The mode adds no host allowlist, tag-state database reads or new shared SDK API.
Existing broad verification failures are not waived by this design.

## Implementation plans

- [Multi-tag extension](../../../plans/sidebar-multi-tag-grouping/plan.md)
