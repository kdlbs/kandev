---
status: current
system: workspaces
created: 2026-10-02
updated: 2026-10-02
owners:
  - kandev
requirements:
  - REQ-WORKSPACES-HIDDEN-FOLDERS-001
---

# Hidden Folder Browsing System Design

## Overview

The directory browser gains one explicit, reversible visibility input. The
backend stops hard-coding the hidden-entry exclusion and instead honors a
request flag. The web client owns one shared, persisted preference for that flag
and renders one control in the shared browser body, so every existing consumer
inherits the capability without new props or duplicated state.

This extends the local-directory-browsing contract already owned by
[Local Repository Discovery](local-repositories.md). It does not add an access
path, a grant, or a new picker surface.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-WORKSPACES-HIDDEN-FOLDERS-001 | API surface, Backend behavior, Shared visibility state, Directory browser UI, Accessibility and phone composition, Internationalization, Failure handling, Security and privacy, Verification strategy |

## Goals

- Make hidden directories reachable from every existing directory browser.
- Keep the default listing byte-for-byte identical to today's behavior.
- Hold the reveal state in exactly one place for the whole application.
- Reuse the existing browse, create, and select flows without parallel paths.

## Non-goals

- Listing symbolic-link directories.
- Tilde expansion in any path field.
- Changing discovery scan roots, scan results, or repository validation.
- Changing the desktop native folder picker or its operating-system defaults.
- Changing the Files panel, Changes, or editor hidden-file visibility.
- Adding a cap, a search box, or a favorites list to the browser.

## API surface

`GET /api/v1/fs/list-dir` accepts one new optional query parameter.

| Parameter | Values | Default | Behavior |
| --- | --- | --- | --- |
| `path` | absolute path, empty means the backend user's home | home | Unchanged |
| `include_hidden` | exactly `true` activates the reveal | inactive | Any other value, including absence, keeps current behavior |

The response shape is unchanged: `path`, `parent`, `entries[]` with `name` and
`path`, and `choosable`. Only the membership of `entries` changes. No new
response field is added, so an older client that ignores the request value keeps
its existing contract.

`POST /api/v1/fs/create-dir` takes no new field. It returns a listing of the
folder it just created, a newly created folder is always empty, and the browser
navigates into that folder and re-lists it through `list-dir`. A created
dot-prefixed name is therefore only ever rendered by its parent's listing, which
already honors the reveal. Adding a visibility input here would be unobservable.

The strict `true` comparison is deliberate. It cannot fail, so a malformed
value degrades to the safe default instead of producing a 400 for a display-only
concern. This satisfies AC-WORKSPACES-HIDDEN-FOLDERS-001.2.

Virtual directory roots, the Windows drive and UNC share listings, contain no
hidden-entry filtering and ignore the flag.

## Backend behavior

`ListDirectory` takes the flag explicitly rather than reading configuration, so
the display decision stays with the caller and remains observable:

```go
func (s *Service) ListDirectory(ctx context.Context, path string, includeHidden bool) (DirectoryListing, error)
```

`collectSubdirs` keeps its existing two filters in the same order and adds the
hidden check as the last filter, gated by the flag. The directory-only filter
stays unconditional, so a hidden file is never listed and no new stat call is
introduced. The existing `os.Root` handling, path resolution, parent
computation, and the `os.Root`-based read are untouched, so the CodeQL
path-injection sanitizer and the volume-root containment reasoning still hold.

Sorting stays case-fold alphabetical. Dot-prefixed names sort before
alphanumeric names, but other punctuation can sort earlier. Ordinary entries
keep their relative order (AC-WORKSPACES-HIDDEN-FOLDERS-001.6).

The new-folder action already accepts a dot-prefixed name. `validateLocalRepositoryName`
rejects only the empty name, `.`, `..`, separators, and NUL, so no name
validation changes are required for AC-WORKSPACES-HIDDEN-FOLDERS-001.7.

The list-dir handler and `CreateDirectory` call `ListDirectory`. The
create-directory endpoint does not accept the visibility input. Its returned
listing is for the new, empty directory.

## Shared visibility state

One persisted UI preference owns the flag. It is read by the shared listing
hook and written by the shared control, so the three consumers cannot disagree.

- Field: `directoryBrowserShowHidden` on the existing persisted UI slice.
- Storage: `loadDirectoryBrowserShowHidden` reads the namespaced key through
  `getLocalStorage`. The UI-slice action writes it through `setLocalStorage`.
- Default: `false`.

The API client gains an options argument that appends `include_hidden=true` only
when the preference is active, so the default request URL is unchanged.

The shared `useDirectoryListing` hook observes the preference. A change
refreshes the path currently listed and keeps the last successful listing while
the request is pending. This keeps the displayed path selectable and preserves
the new-folder draft. Navigation still clears the listing before it loads a new
path.

## Directory browser UI

The control renders inside the shared browser body. The web app's repo-less
starting folder, workspace-source row, and new-repository parent browser all
receive it. In Tauri, the repo-less and workspace-source folder pickers use the
native trigger, so they show no control and send no list request. The
new-repository browser remains an in-app browser
(AC-WORKSPACES-HIDDEN-FOLDERS-001.12).

Structure:

- The control is a pinned sibling of the breadcrumb's scrolling region, on the
  trailing edge of the breadcrumb row, separated by a divider. It must not be
  placed inside the scrolling region, and it must not add a new full-width row.
- It is the design-system `Switch` (`@kandev/ui/switch`) at `size="sm"`, named
  "Hidden folders", which is copy and therefore localized. A switch names the
  thing it controls and reports its own state through `role="switch"` and
  `aria-checked`, so the label stays one short stable noun instead of a
  Show/Hide verb pair. `ConfigurationChatToggle` is the shipped precedent for a
  labelled boolean in this codebase.
- The switch sits inside a `<label>` that carries the trailing-edge slot. The
  whole band is the target, rather than the 14px switch alone. The band has a
  44px minimum at phone widths and for coarse pointers.
- Toggling does not move the displayed path or change whether the current
  directory can be selected. The refresh replaces the listing when it succeeds.
  Navigation still clears the previous listing, so a stale directory cannot be
  selected after a failed navigation request.
- The toolbar row, when the consumer offers folder creation, keeps its existing
  layout. The new-folder form is unaffected.

The full previews are in the
[implementation plan](../../../plans/hidden-folder-browsing/plan.md#ascii-ui-previews).
The plan and both UI work orders carry the same `UI-01` and `UI-02` labels.

## Accessibility and phone composition

- The control is a real switch button in Tab order with a visible focus ring, matching
  the surrounding row controls.
- The accessible name is the localized noun and never changes; the state is
  announced separately by `aria-checked`.
- In a phone or coarse-pointer composition the control uses the same minimum
  touch-target height the other browser actions use, and it stays inside the
  existing scroll region of the browser. It does not introduce a second scroll
  owner and does not displace the confirmation action.

## Internationalization

One new key in the shared `common` namespace:

- `hiddenFolders`

A switch needs one name, not a Show/Hide pair, so no verb-phrase keys exist.

All shipped locales require a value, including the pseudo locale, which is
generated rather than hand-written. Traditional Chinese is produced with the
project's existing conversion command instead of by hand. No user-facing string
in this change uses an em dash; plain punctuation only.

## Failure handling

| Condition | Observable behavior |
| --- | --- |
| `include_hidden` absent, empty, or not exactly `true` | Listing excludes hidden entries, as today |
| Directory unreadable with the reveal active | Same failure as with the reveal inactive; the response keeps the existing generic message and does not echo host paths |
| Reveal active on a directory with many hidden entries | The existing scrolling region renders them; no cap and no second scan are introduced |
| Stored preference unreadable or corrupt | The browser opens with the reveal inactive |
| New-folder creation succeeds with a dot-prefixed name | The returned listing shows the new folder when the reveal is active |
| Selected path later disappears | Existing selection and validation errors are unchanged |

## Security and privacy

The trusted-local-user model is unchanged. This capability adds no new path a
client may list or select: the browser already trusts the local process boundary
and may list any directory the backend can read, and revealing dot entries does
not change that. It does not add a filesystem grant, a discovery root, or a
repository trust.

A revealed listing can display the names of credential-bearing directories such
as `.ssh` or `.aws` in a home directory. That is the user's own machine and the
user's own request, and it is why the control is off by default rather than
removed (AC-WORKSPACES-HIDDEN-FOLDERS-001.11).

Diagnostic logging keeps the existing operation and trigger fields. The reveal
flag is not a filesystem target and does not become a log label.

## Verification strategy

- Go unit tests cover both flag values, the default, the directory-only rule
  under the reveal, the sort position of revealed entries, the
  `create-dir` response parity, and the handler's parameter parsing.
- Frontend unit tests cover the default request URL, the revealed request URL,
  the re-list on toggle, refresh-state preservation, preference persistence,
  the native path with no control, and the phone target.
- Desktop and mobile Playwright specs cover visibility, selection, Tab access,
  the narrow fine-pointer target, and the coarse-pointer target.
- Repository checks cover `gofmt`, the Go lint budget, web typecheck, web lint,
  and the i18n completeness and new-code ratchet.
