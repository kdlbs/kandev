---
status: current
system: ui
requirements:
  - REQ-UI-FILE-TREE-NATURAL-ORDER-001
---

# File Tree Natural Order System Design

## Purpose and boundaries

The UI owns the Files tree's presentation order and its relationship to range
selection. The workspace supplies node identities and data; this design does
not change filesystem operations, transport, storage, hidden-file filtering, or
search results. Related path and selection boundaries remain in
[Files path scope](file-tree-path-scope.md) and
[Files keyboard scope](file-tree-keyboard-scope.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-FILE-TREE-NATURAL-ORDER-001` | [Shared ordering](#shared-ordering), [Responsive surfaces and verification](#responsive-surfaces-and-verification) |

## Shared ordering

`compareTreeNodes` in `apps/web/components/task/file-tree-utils.ts` keeps its
directory-first check and compares names with a shared `Intl.Collator` using the
runtime default locale and `{ numeric: true }`. When that comparison returns
zero, the existing `name.localeCompare` supplies the lexical tie-break. This
retains the existing default comparison's equivalences; do not introduce a
second code-point tie-break or a parser. Collation of nonnumeric names, dots,
and case remains governed by the runtime locale. Numeric collation treats digit
runs, not semantic versions, signed numbers, or decimal values.

`sortRootChildren` copies and sorts the root list. The `getChildren` callback
in `file-browser-hooks.ts` copies and sorts nested siblings. `insertNodeInTree`,
`renameNodeInTree`, and `getVisiblePaths` already use the same comparator;
retain those paths so rendered rows and depth-first range-selection order agree.
The sort must not mutate input arrays or alter node names, paths, or contents.
No new dependency, configuration, persistent state, or backend contract is
needed.

## Responsive surfaces and verification

Desktop uses the existing Files panel and range-selection interaction. Phone
enters Files through bottom navigation and uses the shared tree before opening
the existing full-height read-only viewer. The nearby
`mobile-file-tree-path-copy.spec.ts` and `mobile-file-viewer.spec.ts` show the
shipped entry and interaction patterns. Existing panel scroll ownership,
safe-area behavior, navigation, touch targets, and composition remain unchanged.

Focused utility regressions cover both levels and both node types, directory
precedence, zero-padded ties in reversed input orders, dot and nonnumeric names,
input immutability, insertion, rename, and visible DFS order. Dedicated desktop
and configured `mobile-chrome` Playwright cases create isolated real workspace
fixtures and assert rendered row order. Desktop exercises range selection with
real keyboard input; phone taps a numbered file and verifies its unique content
and no document horizontal overflow. Preserve existing fixtures and assertions.
