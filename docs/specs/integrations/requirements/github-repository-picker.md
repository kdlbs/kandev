---
status: draft
system: integrations
created: 2026-11-11
owners:
  - kandev
---

# GitHub Repository Picker Requirements

## Overview

GitHub watch filters and the workspace GitHub repository scope identify
repositories by their GitHub `owner/name`. Today the watch filter picker only
queries GitHub after the user types an `owner/` prefix, and the scope setting is
a comma-separated text field. Both demand an exact slug from memory, while the
task-creation dialog's remote repository picker lists accessible repositories
as soon as it opens and filters them by any substring.

The integration system owns these GitHub repository selections because their
values are provider identities consumed by GitHub polling and scope
enforcement. The repository picker itself is shared with task creation; this
requirement fixes which repositories each GitHub selection offers and how the
user finds them.

## Terminology

- **Remote repository picker:** The searchable repository list used by the
  task-creation dialog's remote repository chip. It loads the user's accessible
  provider repositories and accepts a pasted repository URL.
- **GitHub watch filter:** The repository selection of an automation
  `github_pr` or `github_pr_merged` trigger, a GitHub review watch, or a GitHub
  issue watch. Stored as `RepoFilter` values (`owner` and `name`; an empty
  `name` selects every repository of that owner).
- **Repository scope:** The workspace GitHub setting that limits which
  repositories GitHub features may use (`all`, `orgs`, or `repos` mode).

## Requirements

### REQ-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001: Searchable GitHub repository selection

**Intent:** Let users select GitHub repositories for watches and repository
scope by browsing and searching, with the same picker they already use when
creating a task from a remote repository.

**User story:** As a user restricting an automation to specific repositories,
I want the repository picker to list my GitHub repositories and filter them as
I type, so that I do not have to recall and type an exact `owner/repo` path.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.1:** When a user opens the
  "Add repository" control of a GitHub watch filter or of the repository scope
  setting, the system shall show the remote repository picker with the user's
  accessible GitHub repositories listed without requiring any input, and a
  loading state until the list arrives.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.2:** When the user types in
  that picker, the system shall filter the listed repositories by a
  case-insensitive substring of `owner/name`; typing only a repository name
  such as `kandev` shall match `kdlbs/kandev`.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.3:** A GitHub watch filter
  shall offer only GitHub repositories. It shall not offer local-only,
  filesystem-discovered, GitLab, Azure DevOps, or plugin-provided repositories.
  When the workspace repository scope is `orgs` or `repos`, the watch filter
  shall offer only repositories that scope allows.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.4:** When the typed value is a
  well-formed `owner/name` or a GitHub repository URL, pressing Enter shall add
  that repository even if it is absent from the loaded list. A GitHub watch
  filter shall reject such a value with a visible message when the workspace
  repository scope does not allow it.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.5:** Selected repositories
  shall render as removable entries. A repository that is already selected
  shall be marked in the picker, and selecting it again shall not add a
  duplicate. GitHub watch filters shall keep organization (`owner/*`)
  selection, through the organization picker of
  AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.10, and the "All repositories
  allowed by this workspace" toggle.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.6:** In `repos` mode, the
  repository scope setting shall use the same picker and selected-entry list
  instead of a comma-separated text field. Its picker shall not be filtered by
  the saved scope. Saving, discarding, dirty-state, and the saved
  `repo_scope_repos` values shall keep their current behavior.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.7:** When GitHub is not
  connected or the repository list fails to load, the picker shall show the
  same unavailable or error state as task creation and shall still accept a
  typed `owner/name` per AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.4.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.8:** On a phone viewport the
  picker shall stay inside the viewport, keep its search input visible, and
  give options, the add control, and remove controls touch-sized hit areas,
  with the same selection outcome as desktop.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.9:** Persisted watch filters,
  trigger configs, and repository scope values, and their API payloads, shall
  keep their existing shape. Previously saved selections shall load and display
  unchanged.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.10:** The repository scope
  setting's organizations (in `orgs` mode) and GitHub watch filters'
  organizations shall use one organization picker instead of a comma-separated
  text field or a row of toggle badges. Selected organizations shall render as
  removable entries; "Add organization" shall list the authenticated account
  first, then the user's GitHub organizations, filter them by a
  case-insensitive substring as the user types, mark already-selected ones,
  and add a typed well-formed login on Enter even if it is not listed. A watch
  filter shall offer and accept only organizations the workspace scope allows,
  rejecting a typed one with a visible message, and shall hide the
  organization picker when the scope is in `repos` mode. Saved
  `repo_scope_orgs` values and `owner/*` watch filters shall keep their shape.
- **AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.11:** The repository picker
  and the organization picker shall each offer a refresh control that reloads
  the list from GitHub, bypassing the server's short-lived repository-list
  cache, and shows the loading state while it reloads. The refresh control
  shall be touch-sized on phones.

## Out of scope

- Changing the workspace repository chips used by task creation and automation
  "Repository access" (registered and discovered repositories with branches).
- Server-side repository search beyond the existing accessible-repository list
  limit; the typed `owner/name` entry covers repositories outside that list.
- Repository selection for GitLab, Azure DevOps, Jira, Linear, or plugin
  watches.
