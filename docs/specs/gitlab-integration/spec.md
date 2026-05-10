---
status: shipped
created: 2026-05-04
owner: tbd
---

# GitLab Integration

## Why

Kandev's code-host integration is GitHub-only: PR browsing, review queue, issue
watches, repo cloning, and the agent `pr` skill all assume `github.com` and the
`gh` CLI. Users on GitLab (gitlab.com or self-managed) can't connect their
account, can't see their merge requests in the MyGitHub-style page, can't have
agents auto-clone or open MRs, and have to manually paste tokens for each task.
This excludes a large chunk of professional users — and self-managed GitLab is
common in regulated/enterprise environments where users have no choice.
Tracked in [#820](https://github.com/kdlbs/kandev/issues/820).

## What

- A GitLab integration runs alongside the GitHub one. Both can be configured at
  the same time; neither replaces the other.
- Users authenticate GitLab via one of:
  - `glab` CLI (auto-detected if installed and logged in).
  - Personal access token stored in the secret store as `GITLAB_TOKEN` (or set
    via the env var of the same name on the backend host).
- Self-managed instances are supported by configuring a GitLab host URL
  (default `https://gitlab.com`). The host applies to API calls, clone URLs,
  and web links.
- Repositories can be added with `provider = "gitlab"`. The repo cloner builds
  SSH (`git@<host>:<owner>/<name>.git`) and HTTPS clone URLs against the
  configured host, and authenticates HTTPS clones with the GitLab token when
  one is available.
- A "GitLab" page mirrors the existing GitHub page: a sidebar of MR / issue
  presets, a search bar, paginated lists, save-preset support, and the
  quick-task launcher. MRs are labelled "Merge requests" throughout the UI;
  the underlying state model is shared with PRs (open/closed/merged, draft,
  approvals, pipelines).
- For an MR linked to a task, the agent UI shows the same review surface it
  shows for PRs: title, description, head/base branches, mergeable state,
  approval status, pipeline status, and threaded discussions.
- A review-watch and an issue-watch poller mirror the GitHub ones: users can
  subscribe to MRs needing their review, or to GitLab issues matching a
  filter, and Kandev creates tasks when matches appear.
- The agent `pr` skill works for GitLab repos: when the task's repo is
  `provider = "gitlab"`, the skill opens a merge request via `glab` (or the
  REST API with the stored token), targeting the same base branch logic used
  for GitHub.
- Settings live under `/settings/integrations/gitlab` with a page parallel to
  the GitHub one: connection status, host URL field, auth method (CLI vs PAT),
  reconnect CTA, and a token field that writes to the secret store.
- The orchestrator and credential providers pass `GITLAB_TOKEN` into agent
  environments the same way `GITHUB_TOKEN` is passed today, so agents running
  in containers can run `glab` or `git push` without extra setup.

## Scenarios

- **GIVEN** a user with `glab` authenticated against `gitlab.com`, **WHEN**
  they open Kandev for the first time, **THEN** the GitLab integration shows
  as connected without requiring any manual token entry, and their merge
  requests appear on the GitLab page.

- **GIVEN** a user on a self-managed GitLab at `https://gitlab.acme.corp`,
  **WHEN** they enter the host URL and a personal access token in
  Settings → Integrations → GitLab, **THEN** the connection status flips to
  connected, MR/issue lists populate against the custom host, and clone URLs
  for added repos use `gitlab.acme.corp`.

- **GIVEN** a task whose repository has `provider = "gitlab"`, **WHEN** the
  agent runs the `pr` skill, **THEN** a merge request is opened on the
  configured GitLab host targeting the repo's default branch, and the MR URL
  is recorded against the task.

- **GIVEN** the user has a GitLab review watch for `assignee=@me`, **WHEN** a
  new MR is assigned to them upstream, **THEN** within one poll interval a
  Kandev task is created in the configured workflow step, linked to that MR.

- **GIVEN** GitLab is connected and GitHub is also connected, **WHEN** the
  user opens the kanban top bar, **THEN** both the GitHub and GitLab page
  buttons are visible and each lists items from its own provider only.

- **GIVEN** the user revokes their GitLab token externally, **WHEN** the
  background poller next runs, **THEN** the integration's status flips to
  "auth required" with a reconnect banner on the GitLab page and the settings
  page, and no further API calls are issued until the user reconnects.

- **GIVEN** no GitLab auth is configured, **WHEN** the GitLab page is opened,
  **THEN** it shows a "not connected" notice linking to settings, and no
  network calls are made to any GitLab host.

## Out of scope

- GitLab webhook ingestion. v1 polls only, like the GitHub integration.
- GitLab CI pipeline editing or job log streaming inside Kandev. Pipeline
  status is shown read-only as part of MR feedback.
- GitLab Issues used as a Jira/Linear-style task source (link issue → task
  with state syncing, structured field mapping). Kandev's Jira and Linear
  integrations cover that pattern; this spec treats GitLab as a code host
  parallel to GitHub, not a project-tracker integration.
- Group-level MR / issue listings outside the user's own scope (no
  org-wide dashboards in v1).
- GitLab-specific features without a GitHub analogue: approval rules editing,
  protected-branch management, MR templates authoring, Duo / GitLab AI
  integration.
- OAuth login via GitLab as a Kandev sign-in method.
- Migration tooling for moving tasks/repos between GitHub and GitLab.
- Bitbucket. The placeholder in `repoclone/protocol.go` stays a placeholder.
