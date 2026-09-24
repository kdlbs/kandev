---
status: current
system: ui
requirements:
  - REQ-UI-COPY-FILE-PATH-ACTIONS-001
---

# Copy File Path Actions System Design

## Purpose and boundaries

The UI system owns the transient copy controls and their desktop and phone
presentation. Existing task and workspace data remains authoritative for file
paths, repository identity, and diff selection. The change adds no backend
contract, persistent state, or path conversion.

## Requirement mapping

| Criterion                                     | Design section                                                       |
| --------------------------------------------- | -------------------------------------------------------------------- |
| `AC-UI-COPY-FILE-PATH-ACTIONS-001.1`          | [Changes row action](#changes-row-action)                            |
| `AC-UI-COPY-FILE-PATH-ACTIONS-001.2`, `001.3` | [Review diff action](#review-diff-action), [Path value](#path-value) |
| `AC-UI-COPY-FILE-PATH-ACTIONS-001.4`          | [Responsive composition](#responsive-composition)                    |
| `AC-UI-COPY-FILE-PATH-ACTIONS-001.5`          | [Path value](#path-value)                                            |

## Components and responsibilities

- `ChangesPanel` continues to provide each working-tree
  `ChangedFile.path`.

### Changes row action

- `FileRowActions` in
  `apps/web/components/task/changes-panel-file-row.tsx` adds the
  desktop Copy path control beside existing row actions.
- `TouchFileRowActions` in
  `apps/web/components/task/changes-panel-touch-file-row.tsx` adds
  Copy path to the existing row action menu. The row itself remains the primary
  diff action.

### Review diff action

- `DesktopFileDiffToolbar` and `MobileFileMenuItems` in
  `apps/web/components/review/review-diff-toolbar.tsx` replace the
  Review toolbar's Copy diff action with Copy path.
- The shared file-action menus also offer Copy path, but that action copies an
  absolute worktree path. The Review toolbar suppresses that generic item in
  its desktop and phone menus so the Review surface has one unambiguous Copy
  path action. Other editor menus keep their existing behavior.
- `copyToClipboard` in
  `apps/web/lib/utils/copy-to-clipboard.ts` remains the clipboard
  boundary, including its secure-context fallback.
- The existing `task:copyPath` translation supplies the accessible
  name and tooltip. `task:copyPathWithControlCharacters` supplies refusal
  feedback when a path contains control characters.

## Path value

Copy path passes the repository-relative path already represented by the
surface: `file.path` for a Changes row and the current
`filePath` for the Review diff toolbar. It does not prepend the
repository name, resolve a local filesystem root, or normalize the value. A
renamed file copies its current path; the existing previous-path value remains
reserved for rename cues and external links. Each copy action stops row-click
propagation where applicable.

Paths containing C0 or DEL ASCII control characters are rejected before the
clipboard write. For these paths the action reports the refusal and leaves the
clipboard unchanged. Other paths are copied exactly as represented by the
surface.

## Control flow

1. The row or active diff supplies its current path.
2. The shared path-copy helper rejects values containing C0 or DEL ASCII
   control characters and reports the refusal.
3. For other values, the helper calls the shared clipboard helper with the
   exact repository-relative path.
4. The Changes row remains selected and the Review diff remains open.
5. If the modern clipboard API is unavailable or rejects the write, the shared
   helper attempts its existing DOM fallback. The action remains available for
   retry if copying still fails.

## Responsive composition

- **Desktop Changes outcome:** the existing file row keeps its path and
  statistics layout. Copy path joins the row's hover actions and becomes
  visible while keyboard focus is within the actions. It does not replace the
  row's click-to-open behavior.
- **Phone Changes entry point and surface:** the existing Changes bottom
  navigation opens `MobileChangesPanel`. The visible 44px row menu
  trigger exposes Copy path with the existing Stage/Unstage, Edit, and Discard
  actions. The row identity continues to open the diff.
- **Nearest mobile exemplar:** `TouchFileRowActions` already presents
  temporary row actions in a responsive menu with 44px controls. Reuse that
  menu and its existing full-path label.
- **Review diff outcome:** fine-pointer desktop keeps a direct Copy path icon
  in the file toolbar. Phone users open the existing 44px file-actions trigger
  and select Copy path from its menu.
- **Shared behavior:** desktop and phone use the same path value and clipboard
  helper. No new scroll owner, drawer, saved preference, or page-level
  horizontal overflow is introduced.

## Failure and recovery

Clipboard writes use the shared helper and its current fallback. A failed
attempt does not alter the selected file, current diff, or path value; users can
retry from the same control. A path containing C0 or DEL ASCII control
characters is not copied and displays localized refusal feedback. A normal
clipboard failure remains available for retry from the same control.

## Persistence

None. Clipboard actions are transient.

## Security

Repository filenames are untrusted input. C0 and DEL ASCII control characters
are refused before clipboard access to prevent a copied name from injecting
additional terminal input. For other paths, only the repository-relative value
already shown by the UI is copied. No filesystem root, credential, or
repository label is added.

## Observability

No new logs or metrics are needed.

## Related specifications

- [Clipboard browser fallbacks](../../auth/requirements/secure-context-browser-fallbacks.md)
- [Changes file row containment](../requirements/changes-file-row-containment.md)
- [Review file status cues](../requirements/review-file-status.md)

## Implementation plan

- [Copy file path actions](../../../plans/copy-file-path-actions/plan.md)
