---
created: 2026-10-02
status: implemented
requirements:
  - REQ-WORKSPACES-HIDDEN-FOLDERS-001
system_design:
  - ../../specs/workspaces/system-design/hidden-folder-browsing.md
legacy_specs: []
---

# Implementation plan: Hidden folder browsing

## Overview

Add a persisted **Hidden folders** switch to the shared in-app directory
browser. The HTTP directory-listing endpoint accepts an explicit
`include_hidden` input. The default listing does not change.

## Evidence and scope

Investigation on 2026-10-02 found the exclusion in one place:
`collectSubdirs` in `apps/backend/internal/task/service/directory_listing.go`
drops any entry whose name starts with `.`, and the `GET /api/v1/fs/list-dir`
handler comment records that as intended. One shared hook and one shared body
(`apps/web/components/folder-picker.tsx`) back the repo-less starting folder, the
folder workspace source row, and the new-repository parent browser, so a single
change reaches all three.

Listing a hidden path directly already succeeds, which is why the gap is entry
and not access. The repository manual-path field cannot substitute: it accepts
only a Git repository, and `ValidateLocalRepositoryPath` runs `filepath.Abs` on
the raw string, so `~` is literal. The desktop native picker is a separate
surface that already reaches hidden directories.

A search of `kdlbs/kandev` issues and pull requests for hidden, dotfile, dot
directory, folder picker, chezmoi, and list-dir returned nothing on this topic.

### In scope

- [Requirements](../../specs/workspaces/requirements/hidden-folder-browsing.md),
  all twelve acceptance criteria.
- The list-dir visibility input, the shared browser control, the
  persisted preference, localized copy, and focused unit, integration, and
  end-to-end coverage.

### Out of scope

- Listing symbolic-link directories.
- Tilde expansion in any path field.
- Discovery scan roots, repository validation, and the Files panel.
- The desktop native folder picker and its operating-system defaults.

## Technical approach

Follow the [system design](../../specs/workspaces/system-design/hidden-folder-browsing.md).
Treat the hidden-entry exclusion as a caller-supplied visibility decision instead
of a constant. The backend gains one optional request input and no new response
field. The web client gains one persisted UI preference read by the shared
listing hook and written by one control in the shared browser body, so no
consumer gains a prop and no consumer owns a second copy of the state.

## ASCII UI previews

Structural requirements: the control is a pinned sibling of the breadcrumb's
scrolling region on the breadcrumb row's trailing edge, separated by a divider;
it adds no full-width row; the breadcrumb scrolls while the control stays fixed;
the entry list keeps its existing scroll owner; the footer stays fixed. Spacing
is illustrative, not a pixel specification. Both views are the same composition,
so one desktop view plus explicit phone notes cover the requirement.

```text
UI-01: Directory browser, reveal off (default) and on
Entry point: folder chip in Create New Task (repo-less starting folder),
            "Add folder" workspace source row, Create new repository parent browser.
State: the browser sends requests to the Kandev host. The native folder picker
does not render this control; the new-repository browser stays in-app.

┌──────────────────────────────────────────────────────────────┐
│ Folders                                        [ + New folder]│ ← toolbar (only when
│                                                              │   creation is offered)
├──────────────────────────────────────────────────────────────┤
│ / home maxmini .local          ·  [ o 隐藏文件夹 ]              │ ← breadcrumb scrolls;
├──────────────────────────────────────────────────────────────┤   control pinned right
│ projects                                                  ›  │ ┐
│ work                                                      ›  │ │ entry list scrolls
│ Downloads                                                 ›  │ ┘
│                                                              │
├──────────────────────────────────────────────────────────────┤
│ [ Use scratch ]                            [ Use this folder ]│ ← fixed footer
└──────────────────────────────────────────────────────────────┘

Same view after the switch is activated (only the switch thumb moves; the label
is one stable noun, so nothing else in the row changes):

┌──────────────────────────────────────────────────────────────┐
│ Folders                                        [ + New folder ]│
├──────────────────────────────────────────────────────────────┤
│ / home maxmini .local          ·  [ o 隐藏文件夹 ]              │
├──────────────────────────────────────────────────────────────┤
│ .config                                                  ›  │ ┐ revealed entries
│ .local                                                   ›  │ │ lead the list, then
│ .ssh                                                     ›  │ │ ordinary entries in
│ projects                                                  ›  │ │ their existing order
│ work                                                      ›  │ ┘
│ Downloads                                                 ›  │
├──────────────────────────────────────────────────────────────┤
│ [ Use scratch ]                            [ Use this folder ]│
└──────────────────────────────────────────────────────────────┘
```

```text
UI-02: Directory browser, phone composition
Entry point: same three surfaces at a phone viewport or coarse pointer.
State: reveal off. Only the control's size and label treatment differ.

┌────────────────────────────────────┐
│ Folders                [ + New      │
│                        folder ]    │
├────────────────────────────────────┤
│ / home maxmini         [ o 隐藏文件夹 ] │ ← 44px band, same pinned
├────────────────────────────────────┤   trailing-edge slot
│ projects                       ›   │
│ work                           ›   │
│ Downloads                      ›   │
│                                    │
├────────────────────────────────────┤
│ [ Use scratch ]                   │
│ [ Use this folder ]                │
└────────────────────────────────────┘
```

Phone notes: the control keeps the pinned trailing-edge slot and the localized
accessible name; the visible glyph is decorative, so the phone label may shorten
while the accessible name stays the full localized action. Entry rows and the
confirmation action keep their existing coarse-pointer heights. No second scroll
owner is introduced, and the breadcrumb keeps its own horizontal scroll.

## Dependency order

| Order | Work order | Wave | Depends on |
| --- | --- | --- | --- |
| 1 | [Task 01: backend visibility input](task-01-backend-visibility-input.md) | 1 | none |
| 2 | [Task 02: shared reveal control and preference](task-02-shared-reveal-control.md) | 1 | none |
| 3 | [Task 03: end-to-end coverage and public documentation](task-03-e2e-and-public-docs.md) | 2 | 01, 02 |

Tasks 01 and 02 are parallel-safe. The request contract is fixed by the system
design, and the frontend unit tests stub the API client, so neither waits on the
other's implementation.

## Risks

| Risk | Mitigation |
| --- | --- |
| A default-behavior regression in a shared endpoint | The default path is unchanged and asserted directly in the listing and handler tests |
| Copy drift across locales | Shipped locales, the pseudo locale, and the new-code ratchet are part of task 02's verification |
| The control landing in the scrolling region and scrolling away | The pinned-sibling structure is a stated requirement; the phone and desktop previews show the slot |
| A future consumer rendering the body without the preference | The control reads the same shared preference as the hook, so a consumer cannot diverge |
| Scope creep into symlinks, tilde, or discovery | Recorded as exclusions in the requirement document and repeated in every work order |

## Verification strategy

- Task 01 proves the request contract with Go unit and handler tests.
- Task 02 proves the control, the preference, and the copy with frontend unit
  tests plus the i18n checks.
- Task 03 proves the user-visible outcome with desktop and mobile Playwright
  specs and confirms the public documentation matches the shipped behavior.
- The whole package runs `make fmt`, `make typecheck test lint` as scoped per
  work order.

## Verification record

| Check | Result |
| --- | --- |
| Go listing and handler tests, full packages | pass |
| `golangci-lint run ./internal/task/...` (Go 1.26.0, v2.9.0) | 0 issues |
| Frontend targeted suites (4 files) | 88 passed |
| Frontend representative mocked-store sample (3 files) | 64 passed |
| `pnpm run typecheck` | clean |
| `pnpm run lint` | clean |
| `pnpm run i18n:check` and `pnpm run i18n:ratchet` | pass |
| Playwright desktop spec | 4 passed |
| Playwright mobile spec (Pixel 5) | 2 passed |
| `list-docs.py validate` and `lint-spec-files.py --all` | pass |

Not run: the complete web unit suite, which did not finish within 75 minutes in
the implementation environment. It is not a work-order-specified check. The
store-shape risk it would cover was reduced by typecheck, by the 88 targeted
tests, and by the 64-test mocked-store sample; a 347-file sweep of tests that
mock `useAppStore` should still run in CI, where a missing field degrades to the
default `includeHidden: false` rather than failing.
