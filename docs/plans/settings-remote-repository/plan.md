---
created: 2026-09-28
status: implemented
requirements:
  - REQ-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001
system_design:
  - ../../specs/workspaces/system-design/remote-repository-registration.md
legacy_specs: []
---

# Implementation Plan: Register remote repositories from workspace settings

## Overview

The workspace Repositories settings page only registered local checkouts;
remote repositories, including plugin-provider (Forgejo/Gitea) ones, had to be
registered by creating a task. Add a verified registration endpoint and an
**Add repository** menu whose **Remote repository** entry reuses the New Task
remote picker.

## Scope

### In scope

- `POST /api/v1/workspaces/:id/repositories/remote` reusing the task
  creation preflight and find-or-create path.
- **Add repository** menu (local / remote) and the remote registration
  dialog on the Repositories settings page.
- Public docs update, locale catalogs, unit tests, and Playwright coverage
  for desktop and phone.

### Out of scope

- Issue or pull-request URL registration, provider credential setup, and
  editing a registered repository's provider identity.

## Work orders

1. [Task 01: Remote repository registration](task-01-remote-repository-registration.md)

## ASCII UI preview

`UI-01: Repositories section action` (Settings > Workspace > Repositories,
writable workspace):

```text
Repositories                                   [ + Add repository v ]
Repositories in this workspace                 +---------------------------+
                                               | ⑂ Local repository        |
                                               |   Use a Git checkout that |
                                               |   already exists here.    |
                                               | ☁ Remote repository       |
                                               |   Register from GitHub,   |
                                               |   GitLab, Azure DevOps,   |
                                               |   or a plugin provider.   |
                                               +---------------------------+
```

`UI-02: Add Remote Repository dialog` (after choosing Remote repository):

```text
+------------------------------------------------------------+
| Add Remote Repository                                   x  |
| Search a connected provider or paste a repository URL.     |
| Kandev verifies the repository before it is saved.         |
|                                                            |
| [ ⌕ Pick or paste a repo ] [ ⑂ branch ] [x]                |
|   +------------------------------------------------------+ |
|   | Search repositories or paste a remote URL            | |
|   | ⚲ acme/site                                   private| |
|   | ⚲ team/api                                           | |
|   | ... provider tabs when more than one provider ...    | |
|   +------------------------------------------------------+ |
| The selected branch becomes the default base branch.       |
| (alert: Could not add the repository: <reason>)            |
|                                                            |
|                              [ Cancel ] [ Add to workspace ]|
+------------------------------------------------------------+
```

Structural requirements: menu order local then remote; one picker row; primary
action **Add to workspace** disabled until a repository is chosen; error stays
inside the dialog. Spacing is illustrative. On a phone the menu is the shared
bottom sheet and the dialog keeps the picker's touch sizing; no distinct phone
layout is introduced.

## Verification

- Backend: `go test ./internal/task/service/ ./internal/task/handlers/`.
- Web: `vitest run app/settings/workspace app/actions`, `eslint`, `tsc`,
  `pnpm run i18n:check`.
- Playwright: `tests/settings/repository-add-remote.spec.ts` (chromium) and
  `tests/settings/mobile-repository-add-remote.spec.ts` (mobile-chrome).

## Risks

Low. The endpoint reuses the existing verification path; the main risk is a
picker selection whose provider hints disagree with the verified descriptor,
which the preflight rejects before any write.
