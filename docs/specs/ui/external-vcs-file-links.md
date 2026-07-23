---
status: shipped
created: 2026-07-22
owner: kandev
---

# External VCS File Links

## Why

People reviewing or editing task files need a quick way to open the same file in its external repository and share that provider page with colleagues. Local worktree paths and Kandev-only diff views do not provide a portable collaboration link.

## What

- Task file and diff toolbars expose an `Open file in <provider>` action whenever Kandev can construct a valid external file URL for the file's repository and revision.
- The action appears across task Changes diffs, built-in file viewers and editors, mobile equivalents, and Review diffs. It opens the external provider page in a new browser tab without replacing the Kandev task.
- The first version supports GitHub, GitLab, and Azure DevOps. GitLab links preserve the configured host for self-hosted installations.
- The external URL targets the linked pull-request or merge-request source branch for the same repository when that published branch is known. Otherwise it targets that task repository's base branch.
- Multi-repository tasks resolve the repository, revision, and repository-relative file path from the file's own repository context. A file in one repository never links through another repository's provider metadata or published branch.
- GitHub links use the repository's web origin and `blob` route. GitLab links use the repository's configured web origin and `-/blob` route. Azure DevOps links use the repository web URL with its file `path` and Git branch `version` query parameters.
- Provider URL components, revisions, and file paths are encoded without changing their semantic values. Generated links contain no embedded credentials, access tokens, or local filesystem paths.
- Added or untracked files require a known published source branch; Kandev does not link them to a base revision where they do not exist. Deleted files link to their base-branch version. Renamed files use the published new path when available, otherwise the base revision's previous path when that path is known.
- The action is unavailable when the repository is local-only, its provider is unsupported, required provider metadata is missing, the repository context is ambiguous, or no revision/path combination is expected to exist externally. Kandev does not guess an unknown provider's web URL shape.
- The action has an accessible provider-specific name and tooltip. On touch layouts it remains directly reachable, has at least a 44 px active dimension, and does not introduce document-level horizontal overflow.

## Failure modes

- If published-review metadata is absent or incomplete, the action falls back to the task repository's base branch when the file is expected to exist there.
- If provider or repository metadata cannot produce a credential-free HTTPS web URL, the action is omitted rather than opening a malformed or sensitive URL.
- If a popup blocker prevents the new tab, Kandev remains on the current task surface and does not lose local state.

## Scenarios

- **GIVEN** a GitHub task with a linked pull request for the file's repository, **WHEN** the user activates the file action, **THEN** a new tab opens the file on the pull request's head branch.
- **GIVEN** a self-hosted GitLab task with a linked merge request, **WHEN** the user activates the file action, **THEN** a new tab opens the `-/blob` file route on the configured GitLab host and merge-request head branch.
- **GIVEN** an Azure DevOps task with no linked pull request, **WHEN** the user activates the file action for an existing file, **THEN** a new tab opens that file using the task repository's base branch.
- **GIVEN** a multi-repository task whose repositories have different providers and base branches, **WHEN** the user opens a file from each repository, **THEN** each action uses that file's repository, repository-relative path, provider URL shape, and revision.
- **GIVEN** an added or untracked file with no published source branch, **WHEN** its toolbar renders, **THEN** no external-file action is offered.
- **GIVEN** a deleted file on a published task branch, **WHEN** the user activates its file action, **THEN** the external provider opens the file's base-branch version instead of a missing head-branch path.
- **GIVEN** a renamed file with a published source branch, **WHEN** the user activates its file action, **THEN** the external provider opens the new path on that published branch.
- **GIVEN** a renamed file without a published source branch but with a known previous path, **WHEN** the user activates its file action, **THEN** the external provider opens the previous path on the base branch.
- **GIVEN** an unsupported, local-only, ambiguous, or incompletely configured repository, **WHEN** a file or diff toolbar renders, **THEN** it does not show an external-file action or expose credentials/local paths.
- **GIVEN** a supported task file on a phone viewport, **WHEN** the user opens its file or diff surface, **THEN** the provider-specific action is visible, touch-reachable, opens the same external file target as desktop, and causes no horizontal page overflow.
- **GIVEN** a supported file in Changes, a built-in editor/viewer, and Review, **WHEN** each toolbar renders, **THEN** each surface offers the same provider-specific open action for that file context.

## Out of scope

- Copying the external URL directly from Kandev.
- Guessing routes for generic or unknown Git hosting providers.
- Publishing, pushing, or creating a pull request or merge request so a local-only file can be linked.
- Adding line-range anchors or linking directly to a diff hunk.
- Changing external repository permissions or making a private repository accessible to a colleague.

## Implementation plan

[External VCS file links](../../plans/external-vcs-file-links/plan.md)
