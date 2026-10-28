---
status: active
system: ui
created: 2026-10-28
owners:
  - kandev
---

# File Tree Natural Order Requirements

## Overview

People browsing the task Files tree should find numbered names in a predictable
order. UI owns this presentation and selection-order contract; workspace
systems retain ownership of filesystem identities and mutations.

## Requirements

### REQ-UI-FILE-TREE-NATURAL-ORDER-001: Natural Files tree order

**Intent:** Keep numbered files and folders easy to scan while retaining the
existing directory-first hierarchy.

#### Acceptance criteria

- **AC-UI-FILE-TREE-NATURAL-ORDER-001.1:** At the root and within each expanded
  folder, the Files tree shall show directories before files, and order names
  within each group using default-locale numeric collation. For example,
  `folder2` precedes `folder10` and `step2.md` precedes `step10.md`.
- **AC-UI-FILE-TREE-NATURAL-ORDER-001.2:** When numeric collation considers two
  distinct names equal, their order shall follow the existing default-locale
  lexical comparison, independent of the response order when that comparison
  distinguishes them. Nonnumeric, dot-prefixed, and case-sensitive names retain
  ordinary default-locale ordering to the extent numeric collation permits.
- **AC-UI-FILE-TREE-NATURAL-ORDER-001.3:** Insertions and renames shall leave
  displayed siblings in the same order as a fresh listing. A desktop range
  selection shall follow the displayed visible tree order.
- **AC-UI-FILE-TREE-NATURAL-ORDER-001.4:** The phone Files surface shall present
  the same ordering and allow a person to open the intended numbered file by
  touch without document-level horizontal overflow.

## Out of scope

- Semantic-version, decimal-value, or negative-number interpretation.
- File or path identity, contents, filesystem mutations, backend ordering,
  hidden-file policy, search-result ordering, or APIs.
- Sort preferences, controls, storage, labels, catalogs, or layout changes.
