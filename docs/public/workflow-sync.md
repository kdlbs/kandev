---
title: "Workflow Sync"
description: "Reconcile workspace workflows from version-controlled definitions in a GitHub repository."
---

# Workflow Sync

Workflow Sync makes a GitHub directory the source of truth for selected workflows in one Kandev workspace. Each run reads portable workflow files, creates or reconciles GitHub-owned workflows, and safely removes definitions that no longer exist. Workflows created manually in the same workspace are left alone.

Choose sync when workflow changes should be reviewed and versioned in Git. Choose [Workflow Import / Export](workflow-import-export.md) for a one-time copy that remains editable in Kandev.

## Prerequisites and credentials

You need a GitHub repository and a branch containing valid portable workflow files. The Kandev backend—not the browser and not a task executor—reads the repository. The workspace automation connection must therefore have contents-read access to the configured branch, including for private repositories, and the repository must be inside the workspace's effective scope.

Configure one automation connection under the workspace's GitHub integration:

- a personal access token;
- an exact named `gh` CLI host/login; or
- a GitHub App installation with **Contents: read** permission for the repository.

Without a usable workspace automation connection, sync fails. Existing migrated workspaces may temporarily use **Legacy shared**, which alone consults ambient backend `gh`, `GITHUB_TOKEN`, `GH_TOKEN`, or old globally named secrets. Grant only the repository access that this operation needs, protect the backend environment and secret store, and do not commit a token into a workflow file.

## Configure a workspace

Open **Settings → Workspaces → select a workspace → Workflows → GitHub Sync**.

1. Paste a GitHub repository link. The field accepts an HTTPS URL with or without a scheme, a `www.github.com` URL, a `.git` suffix, or an SSH form such as `git@github.com:OWNER/REPO.git`.
2. A `/tree/BRANCH/DIRECTORY` link fills the branch and directory. A `/blob/BRANCH/FILE` link uses the file's containing directory. With a plain repository link, the initial defaults remain `main` and `.kandev/workflows`; when editing an existing configuration, a plain link preserves its current branch and path.
3. Enable **Auto-sync** if the background poller should run. Set an interval of at least 60 seconds; the default is 300 seconds.
4. Save the configuration, then select **Sync now** for the first immediate reconciliation. Saving alone does not fetch definitions.

The URL parser treats the first segment after `/tree/` or `/blob/` as the complete branch name. A GitHub URL cannot unambiguously split a branch containing `/` from the directory, so enter or verify such a branch separately rather than relying on a pasted tree link.

### Stored fields and defaults

There is at most one sync configuration per workspace.

| JSON field | Requirement and default |
|------------|-------------------------|
| `repo_owner` | Required after trimming; cannot contain a slash or space. |
| `repo_name` | Required after trimming; cannot contain a slash or space. |
| `branch` | Defaults to `main`; must be a valid Git branch name. |
| `path` | Leading and trailing slashes are removed. Empty defaults to `.kandev/workflows`; a `..` path segment is rejected. |
| `interval_seconds` | `0` defaults to `300`; valid range is `60` through `2592000` (30 days). |
| `poll_enabled` | Omitted JSON defaults to `true`; `false` allows only **Sync now**. |

The status also records `last_synced_at`, `last_ok`, `last_error`, and `last_warnings`. Auto-sync checks due configurations on a 60-second outer ticker and waits one full tick after backend startup. A configured interval is therefore a minimum cadence, not an exact schedule; a due sync can start roughly another minute later.

## Definition directory

Sync reads only immediate files in the configured directory. It does not recurse. Extensions are case-insensitive: `.yml` and `.yaml` use YAML decoding, while `.json` uses JSON decoding; other files and directory entries are ignored. Paths are processed in sorted order.

Every file must use the version 1 `kandev_workflow` portable envelope documented in [Workflow Import / Export](workflow-import-export.md). A file may contain one or several workflows. The safest authoring loop is to build and test a workflow in a disposable workspace, export it, commit the export, and then configure the target workspace.

```yaml
version: 1
type: kandev_workflow
workflows:
  - name: Delivery
    steps:
      - name: Todo
        position: 0
        color: bg-slate-500
        events: {}
        is_start_step: true
        show_in_command_panel: true
        allow_manual_move: true
        auto_advance_requires_signal: false
      - name: Done
        position: 1
        color: bg-green-500
        events: {}
        is_start_step: false
        show_in_command_panel: true
        allow_manual_move: true
        auto_advance_requires_signal: false
```

Commit the file, then use **Sync now**. The status card reports created, updated, deleted, warning, or unchanged results.

## Reconciliation rules

A synced workflow is keyed by its exact repository `source_path` and exact workflow `name`. A matched workflow keeps its database ID. Within it, steps are matched by exact name and keep their IDs, so tasks remain attached when a prompt, color, event, WIP rule, profile, or position changes.

These rules matter when editing definitions:

- Renaming a step is equivalent to removing the old step and creating a new one. If the old step has tasks, Kandev skips the whole workflow update and reports a warning.
- Renaming or moving a workflow definition changes its `(source path, name)` key. Kandev creates the new workflow and treats the old one as removed. If the old workflow still has tasks, both remain and a warning explains why.
- A removed step is deleted only when it has no tasks. A removed workflow is deleted only when the entire workflow has no tasks.
- Duplicate step names in either the desired definition or the existing synced workflow make name matching unsafe; Kandev skips that workflow update and warns.
- Manual workflows are never matched, updated, or removed by sync, even when their name is identical.
- Synced workflows are read-only in normal workflow mutation paths. Edit the repository and sync again. Every run performs a full reconciliation, so it also repairs drift; the stored content hash is for status/observability, not a skip condition.

The portable format does not carry every internal or Office field. Sync reconciles the portable Kanban fields and preserves non-portable internal stage type. Do not use this facility as an Office-workflow backup.

### Invalid and empty sources

Parsing and validation happen per file. If one file is invalid, its error becomes a warning, workflows last synced from that exact file are frozen for that run, and valid files continue. This protects existing workflows from deletion because of one broken edit. Fix the file and sync again.

A valid fetch that returns no supported files is different: it is an empty desired set. Synced workflows with no tasks are removed; workflows with tasks remain with warnings. Pointing at the wrong but existing empty directory can therefore remove unused synced workflows.

Repository listing or file-download failures fail the run before apply. Per-workspace locking serializes sync, configuration changes, and removal, so two requests cannot interleave their changes.

## HTTP API

The settings UI uses these backend routes. All require a `workspace_id` query parameter. They currently have no backend authentication: any network client that can reach them can inspect or change sync configuration and trigger repository reads with the backend's GitHub credentials. Keep the backend on loopback or put it behind an authenticated, origin-protected reverse proxy before exposing it.

| Method | Route | Success behavior |
|--------|-------|------------------|
| `GET` | `/api/v1/workflow-sync/config?workspace_id=ID` | `200` with the configuration, or `204 No Content` when absent. |
| `POST` | `/api/v1/workflow-sync/config?workspace_id=ID` | Validate/upsert the JSON configuration and return it. Does not sync. |
| `DELETE` | `/api/v1/workflow-sync/config?workspace_id=ID` | Release synced workflows to manual ownership, delete the configuration, and return `{"deleted":true}`. |
| `POST` | `/api/v1/workflow-sync/sync?workspace_id=ID` | Run immediately and return the current `config` plus `result` or `error`. |

Example:

```bash
curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  -d '{
    "repo_owner": "acme",
    "repo_name": "engineering",
    "branch": "main",
    "path": ".kandev/workflows",
    "interval_seconds": 300,
    "poll_enabled": true
  }' \
  'http://localhost:38429/api/v1/workflow-sync/config?workspace_id=WORKSPACE_ID'

curl -fsS -X POST \
  'http://localhost:38429/api/v1/workflow-sync/sync?workspace_id=WORKSPACE_ID'
```

Except for “not configured,” a completed force-sync request returns HTTP `200` even when the response contains an `error`; inspect the JSON and `config.last_ok`, not only the HTTP status. A force sync without a configuration returns `404`.

## Stop syncing and clean up

Choose **Remove sync** to stop polling. Kandev first clears GitHub ownership from all synced workflows in the workspace, making them normal editable workflows, and then removes the configuration. It does not delete those workflows. If releasing any workflow fails, removal fails and the configuration remains so the operation can be retried.

Deleting an individual repository definition has the different reconciliation behavior described above. Move or archive tasks first when you intend the corresponding synced step or workflow to disappear.

## Troubleshooting

- **Authentication error:** run `gh auth status --hostname github.com` in the backend environment or configure one of the token sources above. Confirm that identity can read the repository and branch.
- **Directory or branch not found:** verify the resolved owner, repository, branch, and directory shown in the dialog. Check branch names containing `/` manually.
- **Nothing happens after Save:** save stores only the configuration. Use **Sync now** or wait until both the configured interval and the poller's next 60-second check have elapsed.
- **Completed with warnings:** read every warning. Invalid files freeze their previous workflows; tasks in removed steps or workflows block deletion; duplicate step names block safe matching.
- **Unexpected duplicate after rename:** restore the original `(file path, workflow name)`, or move/archive tasks from the old workflow before deleting it.
- **Changes appear to revert:** a synced workflow is repository-owned. Commit the change to its source file; the next reconciliation repairs local drift.
- **Rate limits or intermittent network failures:** lengthen `interval_seconds`, use **Sync now** after recovery, and inspect the GitHub integration status and backend logs.

Related guides: [Workflow Tips](workflow-tips.md), [Workflow Import / Export](workflow-import-export.md), [Configuration](configuration.md), and [Operations](operations.md).
