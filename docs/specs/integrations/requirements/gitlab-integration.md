---
status: active
system: integrations
created: 2026-05-04
updated: 2026-08-05
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
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.6:** Task creation is the narrow unauthenticated exception: branch discovery for an explicitly entered public `gitlab.com` repository URL works without a saved workspace connection. It does not expose private projects, browse results, merge requests, issues, or write actions.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.7:** GitLab repository matching uses provider, normalized provider host, and full subgroup project path. Repositories with unknown or mismatched provider hosts are not eligible for GitLab linking or merge-request actions. Decision: ADR-2026-07-20-repository-provider-origin-identity.
- **AC-INTEGRATIONS-GITLAB-INTEGRATION-001.8:** Users can browse and search merge requests and issues, then launch a task from either row with the same configurable action presets used by GitHub.

## System design

The migrated technical source is split into [part 1](../system-design/gitlab-integration-01.md), [part 2](../system-design/gitlab-integration-02.md).
