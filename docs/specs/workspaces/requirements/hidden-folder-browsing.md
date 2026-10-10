---
status: active
system: workspaces
created: 2026-10-02
updated: 2026-10-02
owners:
  - kandev
---

# Hidden Folder Browsing Requirements

## Overview

Users browse the filesystem of the machine running Kandev to choose a starting
folder for a repo-less task, to attach a folder workspace source, to pick the
parent of a new local repository, and to select a discovery root. That directory
browser currently omits every entry whose name starts with `.`, so a project
stored under `~/.local/share/chezmoi`, a configuration tree such as
`~/.config/<tool>`, or a tool's own runtime directory such as `~/.minimax` can
not be reached at all from a browser client.

Listing a hidden path directly succeeds; only descending into one is impossible.
The gap is entry, not access, so this capability adds a reversible reveal control
rather than a new access path or a new permission.

## Terms

- **Hidden directory**: an immediate child entry of a listed directory whose name
  begins with `.`.
- **Directory browser**: the shared in-application folder-listing surface backed
  by the HTTP directory-listing endpoint, used by every client that browses the
  backend host filesystem.
- **Native folder picker**: the operating-system folder dialog used by the
  desktop shell. It is a different surface and is not changed here.

## Requirements

### REQ-WORKSPACES-HIDDEN-FOLDERS-001: Reach hidden directories from the directory browser

**Intent:** A user who keeps a project or a configuration tree in a hidden
directory must be able to reach it through the same directory browser that
already serves ordinary directories, without a special case, a different tool,
or a filesystem rename.

**User story:** As a user whose project lives in a hidden directory, I want to
reveal hidden folders in the directory browser, so that I can select that
directory like any other.

#### Acceptance criteria

- **AC-WORKSPACES-HIDDEN-FOLDERS-001.1:** The directory browser shall offer a
  control that, when active, lists entries whose name begins with `.` alongside
  ordinary entries in the directory currently displayed. The control shall be off
  by default.
- **AC-WORKSPACES-HIDDEN-FOLDERS-001.2:** While the control is off, a listing
  shall not contain any hidden entry. Existing client behavior shall be
  unchanged, including for a request that omits or mis-spells the control's
  request value.
- **AC-WORKSPACES-HIDDEN-FOLDERS-001.3:** Activating or deactivating the control
  shall re-list the directory currently displayed. It shall not change the
  displayed path, the breadcrumb, upward navigation, or whether the current
  directory can be selected.
- **AC-WORKSPACES-HIDDEN-FOLDERS-001.4:** The control's state shall apply to
  every directory browser in the application, including the repo-less starting
  folder, an attached folder source, and the new-repository parent browser, and
  shall not require a separate setting per surface.
- **AC-WORKSPACES-HIDDEN-FOLDERS-001.5:** The control's state shall be a stored
  user preference. It shall survive closing and reopening a directory browser
  and a page reload, so a user who works in a hidden directory sets it once.
- **AC-WORKSPACES-HIDDEN-FOLDERS-001.6:** Revealing hidden entries shall list
  directories only. A hidden file shall not appear, and no ordinary entry shall
  disappear or change position relative to other ordinary entries.
- **AC-WORKSPACES-HIDDEN-FOLDERS-001.7:** A folder created through the browser's
  new-folder action whose name begins with `.` shall be accepted, shall be
  enterable, and shall appear in its parent directory's listing when the control
  is active.
- **AC-WORKSPACES-HIDDEN-FOLDERS-001.8:** A selected hidden directory shall be
  validated, canonicalized, stored, and used exactly like any other selected
  directory. Repository Git-metadata rules, permission boundaries, and executor
  rules shall not change because a path contains a hidden segment.
- **AC-WORKSPACES-HIDDEN-FOLDERS-001.9:** The control shall be reachable by
  keyboard, shall report its state to assistive technology independently of its
  accessible name, and shall carry one localized accessible name that does not
  change with the state. It shall not rely on color alone to convey state.
- **AC-WORKSPACES-HIDDEN-FOLDERS-001.10:** In a phone or coarse-pointer
  composition, the control shall meet the existing minimum touch-target size and
  shall not reduce the reachability of the breadcrumb, the entry list, or the
  confirmation action.
- **AC-WORKSPACES-HIDDEN-FOLDERS-001.11:** Revealing hidden entries shall not
  widen filesystem access. A directory the browser could not read shall fail the
  same way with the control active or inactive, and a failure response shall not
  reveal host paths beyond the directory the user already requested.
- **AC-WORKSPACES-HIDDEN-FOLDERS-001.12:** The desktop shell's native folder
  picker shall be unchanged. The desktop webview shall neither display this
  control nor call the HTTP directory browser.

## Exclusions

- Symbolic-link entries. A symlink that resolves to a directory is not an
  immediate directory child today, because the listing uses each entry's own
  type. Revealing hidden entries does not change that. A separate change is
  required before a symlinked directory is browsable.
- Tilde expansion. No path field gains `~` handling in this change. Where a
  manual absolute-path field exists today, it keeps treating `~` as a literal
  path segment.
- Automatic discovery scans. The reveal control changes what a person browses.
  It does not change which roots discovery scans or which repositories a scan
  returns.
- The Files panel, Changes, and file-editor views. Hidden-file visibility inside
  an open task workspace is a separate capability.
- The native folder picker's operating-system defaults for showing hidden files.
- Filesystem permissions, macOS privacy prompts, and the trusted-local-user
  access model.
