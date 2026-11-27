---
status: active
system: workspaces
created: 2026-10-08
owners:
  - kandev
---

# File Entry Mutations

## Overview

Deleting, renaming, or moving an eligible Files entry must operate on that entry.
A symbolic link shown in the inventory must not silently transfer a destructive
action to its target. Workspaces owns filesystem mutation authority and entry
identity; UI consumes the resulting state on desktop and phones.

This contract is independent of [link identification](symlink-identification.md)
and [saved file contents](saved-file-content.md). Reading and editing a readable
link can continue to follow its target under the existing access policy.

## Terms

- **Leaf entry:** The final directory entry selected for an operation.
- **Parent routing:** Traversal through a directory link before the leaf.
- **Authority root:** The canonical workspace or a registered durable-source
  root admitted for that operation. A registered source is not blanket access
  to other external directories.
- **Occupied destination:** Any existing final entry, including a link whose
  target is absent or cannot be resolved.

## Requirements

### REQ-WORKSPACES-FILE-ENTRY-MUTATIONS-001: Preserve selected mutation identity

#### Acceptance criteria

- **AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.1:** Deleting an eligible contained
  file or directory link shall remove only that link. Its target entry, target
  contents, and every unselected workspace entry shall remain unchanged.
- **AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.2:** Renaming or moving an eligible
  contained link to an available destination within the same authority root
  shall move the link entry and retain its stored link value. The source entry
  shall be absent, the destination shall remain a link, and target entries,
  target bytes, and unselected entries shall remain unchanged. A relative link
  moved to another directory can consequently resolve differently; retaining
  its stored value does not promise target rewriting or continued readability.
- **AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.3:** A distinct occupied destination
  shall reject rename/move without replacing an entry or changing bytes. This
  includes ordinary files, directories, readable links, dangling links, and
  looping links. An otherwise admitted same-entry rename shall retain its
  existing no-op outcome and emit no mutation notification.
- **AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.4:** Ordinary files and directories
  shall retain delete/rename behavior, including creation of missing rename
  parent directories. Contained directory links and registered durable-source
  links used as parents shall continue to route eligible descendant operations.
  Reading and editing through eligible links shall retain target behavior.
- **AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.5:** Mutation shall not acquire new
  authority from preserving a leaf. Workspace and registered-source roots,
  including aliases resolving to those roots, shall not be deleted or moved.
  Leaf links targeting a different authority root or an unregistered external
  location, cross-root moves, and escaping parent paths shall be rejected with
  entries and bytes intact. Dangling and looping source links remain rejected.
  A parent changed to an external link at the mutation boundary shall not
  cause external contents to be removed, moved, or created.
- **AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.6:** Existing operation responses
  shall retain selected request paths and success/failure semantics. Immediate
  successful notifications shall identify the mutated leaf, using existing
  parent routing and repository metadata; rejected operations and no-ops shall
  emit none. Explicit repository selection and aggregate inventory paths shall
  affect only the selected entry. Desktop and phone shall receive the same
  corrected state through their existing actions and refresh behavior.

## Exclusions

No new API fields, schema, settings, feature flags, localized copy, frontend
composition, or ownership coordinator. This contract does not change general
read/write resolution, grant authority to remove durable-source root aliases,
enable dangling/loop source mutations, or provide a general filesystem security
framework. Concurrent destination creation and within-root entry replacement
are not given a new atomic transaction guarantee.

## Implementation plan

- [Preserve leaf link mutations](../../../plans/preserve-leaf-link-mutations/plan.md)
