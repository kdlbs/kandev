---
status: active
system: integrations
created: 2026-05-04
updated: 2026-10-08
owners:
  - tbd
---
# GitLab Integration Requirements

## Overview

Teams whose code lives on GitLab cannot complete the same task, review, and automation workflows available for GitHub without leaving Kandev. Existing GitLab support can browse merge requests and issues and contains partial watch and review plumbing, but its connection is installation-wide and the main workflows are not usable end to end.

## Requirements

### REQ-INTEGRATIONS-GITLAB-INTEGRATION-001: GitLab Integration

**Intent:** Teams whose code lives on GitLab cannot complete the same task, review, and automation workflows available for GitHub without leaving Kandev. Existing GitLab support can browse merge requests and issues and contains partial watch and review plumbing, but its connection is installation-wide and the main workflows are not usable end to end.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.1:** GitLab and GitHub can be connected at the same time. Each integration only reads or mutates its own provider.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.2:** Each Kandev workspace owns exactly one GitLab connection: one normalized host URL, one authentication method, one credential, and one health record.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.3:** The default host is `https://gitlab.com`; self-managed `http://` and `https://` origins are supported for API calls, web links, clone URLs, and merge request creation. Kandev preserves the configured scheme.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.4:** A workspace can authenticate with a personal access token or a `glab` login for its configured host. `GITLAB_TOKEN` remains an explicit deployment fallback, but it is never persisted and only applies to workspaces configured to use that fallback.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.5:** GitLab browse, task-link, review, watch, and write endpoints require an authoritative `workspace_id` and resolve that workspace's connection. Data or credentials from another workspace are never used as fallback.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.10:** Merge-request and issue project choices shall accumulate across pages within the same workspace and browse context. Changing workspace shall discard projects accumulated in the previous workspace, including when the new workspace returns no results. An explicitly selected project filter remains represented without being reset; it does not authorize retaining other projects from the previous workspace.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.6:** Task creation is the narrow unauthenticated exception: branch discovery for an explicitly entered public `gitlab.com` repository URL works without a saved workspace connection. It does not expose private projects, browse results, merge requests, issues, or write actions.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.7:** GitLab repository matching uses provider, normalized provider host, and full subgroup project path. Repositories with unknown or mismatched provider hosts are not eligible for GitLab linking or merge-request actions. Decision: ADR-2026-07-20-repository-provider-origin-identity.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.8:** Users can browse and search merge requests and issues, then launch a task from either row with the same configurable action presets used by GitHub.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.9:** When the selected workspace changes during merge-request or issue browsing, the result page shall reset to page 1 of the new workspace. A populated first page shall remain reachable even when the new workspace has fewer pages than the previous workspace. Ordinary page navigation and unchanged search inputs within the same workspace shall preserve the selected page. Existing preset, custom-query, and result-kind changes shall continue to reset the page; workspace switching shall retain current-workspace result visibility, latest-request response handling, and the existing disabled, refresh, empty, and failure behavior.

Criterion .9 is the pagination clarification delivered by the
[workspace-pagination package](../../../plans/gitlab-workspace-pagination/plan.md).
Its local implementation and behavioral verification are complete; hosted
review, CI, and merge remain pending. Unrelated criteria retain their lifecycle.

#### Discussion reply drafts

The following clarification extends the existing GitLab review contract.
Local implementation and targeted checks are complete in the
[reply-draft package](../../../plans/gitlab-reply-draft-preservation/plan.md).
Hosted review, CI and merge remain pending; unrelated criteria retain their
existing lifecycle.

- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.11:** When a user submits a nonblank discussion reply, only the trimmed text captured at submission shall be sent to that discussion. Empty or whitespace-only input shall not be submitted. The textarea shall remain editable while the reply is pending, with submission unavailable during the pending action.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.12:** When the reply succeeds, the textarea shall clear only if its current exact text equals the raw text captured at submission. A different current value shall be preserved exactly, including whitespace-only differences and an intentional clear. Text edited and then restored exactly to the submitted raw value is eligible to clear.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.13:** When the reply fails, the current textarea value shall remain intact, whether unchanged, edited, or cleared. After the pending action settles, a user can retry nonblank current text; retry shall send the current trimmed text, and its success shall obey the same clearing rule.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.14:** A successful reply shall keep the existing success feedback and refresh the review. While refreshed data retains the same discussion, its newer unsent text shall survive a delayed, successful, or failed refresh. A refresh failure shall not turn a successful reply into a failed submission or restore a successfully cleared draft.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.15:** Reply settlement shall affect only the submitted discussion's draft. Other discussion drafts shall remain intact, including when refreshed discussions are reordered. Desktop and phone shall share these draft semantics within the existing reply surface.

This clarification excludes draft persistence after unmount, reload, or
discussion removal, changes to MR/workspace switching, and changes to layout,
copy, other review actions, or provider APIs.

## System design

The migrated technical source is split into [part 1](../system-design/gitlab-integration-01.md), [part 2](../system-design/gitlab-integration-02.md).
