---
title: "Office Config Sync"
description: "Keep an Office workspace's agents, skills, projects, and routines in sync with a GitHub or GitLab repository."
---

# Office Config Sync

Office Config Sync makes a GitHub or GitLab repository the source of truth for an Office workspace's agents, skills, projects, and routines. Each run reads the repository's Office config layout, creates or reconciles the entities sync owns, and safely removes entities that no longer exist. Entities created by hand in the Kandev UI are left alone.

Office mode is currently feature-flagged. Config sync settings appear under **Office workspace Settings > Config Sync** when Office is enabled.

This is a separate capability from [Workflow Sync](workflow-sync.md): Workflow Sync reconciles regular Kanban workflows from a flat directory of portable workflow files; Office Config Sync reconciles an Office workspace's agents, skills, projects, and routines from the Office config directory tree described below. The two do not interact and share no configuration.

## Quick path

1. Commit an Office config tree (`agents/`, `skills/`, `projects/`, `routines/`) to a GitHub or GitLab repository.
2. Grant the workspace connection read access.
3. Pick the provider, then fill in the repository (or project path), branch, and directory in **Office workspace Settings > Config Sync**.
4. Save, then select **Sync now** for the first immediate reconciliation. Saving alone does not fetch anything.

## Prerequisites and credentials

You need a repository and a branch containing a valid Office config tree. The Kandev backend, not the browser and not a task executor, reads the repository, using the same workspace-routed provider connections as Workflow Sync:

- **GitHub:** the workspace automation connection needs contents-read access to the configured branch (a personal access token, a named `gh` CLI host/login, or a GitHub App installation with Contents: read).
- **GitLab:** the workspace's already-configured GitLab connection (Settings > Workspaces > select a workspace > Integrations > GitLab) is used as-is; config sync has no credential or host field of its own.

The config record stores no provider host and no credential. Host, auth method, and credential are resolved per run from the workspace's provider connection, so a self-managed GitLab host needs no configuration here. If the configured provider has no connection for the workspace, a run fails with a message naming that provider; saving the config still succeeds, so you can configure the connection and the sync source in either order.

## Configure a workspace

A workspace has at most one config sync source, and that source is a property of a single record: reconciliation deletes managed entities absent from the fetched set, so two sources would each read the other's entities as deletions.

1. Choose **GitHub** or **GitLab** and enter the repository identity. GitHub uses a repository owner and name; GitLab uses a namespace project path (`group/project`, or a nested `group/subgroup/project`). The two are mutually exclusive: a saved config carries exactly one, and switching provider clears the other provider's fields.
2. Set **Branch** and **Directory**. An empty directory addresses the repository root, and that root is preserved through a read-modify-write, unlike Workflow Sync's directory field. A `path` value is stored as given (only Unicode NFC normalization is applied) rather than trimmed, so a leading slash, a `..` segment, or a trailing slash is rejected rather than silently corrected.
3. Set **Sync interval** (60 seconds to 30 days; default 300) and whether **Sync automatically** is on. With it off, entities sync only on **Sync now**.
4. Save, then select **Sync now** for the first reconciliation.

Saving a config for a workspace that already has one replaces it and resets the recorded status, so the next run reconciles from scratch. Entities already applied by the previous source stay managed under the new one: the first run against the new source updates or deletes them rather than orphaning them as unmanaged.

### Stored fields and defaults

| Field | Requirement and default |
|-------|--------------------------|
| `provider` | `"github"` or `"gitlab"`. Required; there is no default. |
| `repo_owner` / `repo_name` | GitHub only. Rejected together with `project_path`. |
| `project_path` | GitLab only. A namespace path of at least two segments. Rejected together with `repo_owner`/`repo_name`. |
| `branch` | Defaults to `main`; must be a valid Git branch name. |
| `path` | Omitted or empty addresses the repository root and is preserved as such. Leading slash, `..`/`.` segments, repeated-slash empty segments, backslashes, and NUL bytes are rejected; the value is not trimmed. |
| `interval_seconds` | `0` or omitted defaults to `300`; valid range is `60` through `2592000` (30 days). |
| `poll_enabled` | Omitted defaults to `true`; `false` allows only **Sync now**. |

The status also records `last_synced_at`, `last_ok`, `last_error`, and up to the first 10 warnings from the most recent run (with a count of any beyond that). Auto-sync checks due workspaces on the same 60-second outer ticker Workflow Sync uses, so a configured interval is a minimum cadence, not an exact schedule.

## The Office config directory

A run reads only the fixed Office config layout beneath the configured directory, non-recursively per subdirectory:

- Every `*.yml`/`*.yaml` file directly in `agents/`, `projects/`, and `routines/`.
- In each immediate subdirectory of `skills/`, that skill's `SKILL.md` plus every file directly in its `references/` directory (populating the same file inventory a bundled system skill has).

`kandev.yml` (workspace name, description, approval and budget defaults, default executor and agent profile, permission handling mode, recovery lookback) is never read or applied by config sync. A repository cannot use this capability to change those operator safety controls; only its presence at the root is used as a sanity signal that the configured path looks like an Office config root.

A repository with no `routines/` directory (or no `skills/`, `agents/`, `projects/`) is a valid, partial config source: a missing subdirectory below a successfully listed root contributes no files rather than failing the run. A missing or unreadable *root*, by contrast, fails the run, since both providers report the same not-found status for an absent directory and for one the credential cannot see.

Limits: at most 200 skill subdirectories and 1000 files per run, and 1 MiB per file. Exceeding either cap fails the run without applying anything (a truncated fetch must never be allowed to look like a deletion of everything beyond the cap), and a warning names which cap was hit. A run also does not pin a commit, so a push landing mid-run can leave its view mixed across two revisions; this self-corrects on the next run while polling stays on, but persists until an explicit sync when polling is off.

## Identity and reconciliation rules

- An entity's identity is `(kind, key)`: the declared `name` for an agent, project, or routine, and the directory name for a skill. Identity excludes the file path, so moving a file within its kind's directory updates the same entity.
- When a file's declared `name` differs from its filename stem, the declared `name` wins and a warning names both.
- When two files of the same kind resolve to the same key, the file whose full path sorts first (byte-wise) wins; a warning names the key and every losing path.
- A skill directory with no `SKILL.md` defines no skill, even if it has `references/` content; a previously managed skill whose `SKILL.md` disappears is deleted by the same rule that deletes any other removed entity.
- Every run reconciles the *full* fetched set against the workspace; a run never skips reconciliation because the repository looks unchanged, because that is exactly when a UI edit needs repairing. Running twice against a genuinely unchanged repository produces no creates, updates, or deletes.
- A fetched entity's key colliding with an entity you created by hand is never adopted or modified; only entities in the applied manifest (this capability's own record of what it owns) are ever changed or deleted by a run.
- A file that fails to parse or validate, or that could not be fetched, leaves the entity it previously defined untouched and exempt from deletion for that run, with a warning naming the file and the reason. A file that cannot be read is not a file that was removed.
- The one cross-entity reference config sync resolves is an agent's `reports_to`, resolved in a second pass after every agent in the run is applied, against only the entities *this run applied* (never against entities the workspace already had). A self-reference or a cycle is left empty with a distinguishing warning. `desired_skills`, a project's `lead_agent_name`, and a routine's `assignee_name` are never resolved by sync, matching the existing filesystem importer.

Full field-by-field ownership per kind, and the complete warning ordering, are in [Office Config Sync Reconciliation](../specs/office/requirements/config-sync-reconciliation.md).

## Coexistence with the filesystem and raw-git surfaces

Office already has two other configuration surfaces: a filesystem-to-database diff/apply page and a raw-git `clone`/`pull`/`push` section. Config sync applies to the database only and never writes to the workspace directory on disk, so it cannot collide with a raw-git checkout by itself. But two reconcilers acting on the same entities would fight, so while a config sync source is configured for a workspace, these actions are refused with a conflict naming config sync as the active source:

- the filesystem-to-database apply (**Import from filesystem**);
- the definition-bundle import;
- the database-to-filesystem export (**Export to filesystem**), refused for a different reason: it would manufacture a repository write path this capability deliberately excludes, since a subsequent raw-git push would commit provider-sourced state back as if authored locally;
- raw-git **Clone** and **Pull**.

Raw-git **Push** is never refused; it remains the only way to write Office configuration back to a repository, using the backend's git credentials, and pushes only whatever you placed in the checkout by hand or by `git`. The read-only filesystem diff view and the read-only bundle export keep working unchanged regardless of whether config sync is active. The settings UI shows every refused control as unavailable, stating that config sync is the active source, instead of letting you hit the 409 the server would otherwise return.

## HTTP API

The settings UI uses these backend routes:

| Method | Route | Success behavior |
|--------|-------|-------------------|
| `GET` | `/api/v1/office/workspaces/:wsId/config-sync/config` | `200` with the configuration, or `204 No Content` when absent. |
| `POST` | `/api/v1/office/workspaces/:wsId/config-sync/config` | Validate/upsert the JSON configuration and return it. Does not sync. |
| `DELETE` | `/api/v1/office/workspaces/:wsId/config-sync/config` | Release managed entities to unmanaged ownership, delete the configuration, and return `{"deleted":true}`. |
| `POST` | `/api/v1/office/workspaces/:wsId/config-sync/sync` | Run immediately and return the current `config` plus `result` or `error`. |

Example (GitHub):

```bash
curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  -d '{
    "provider": "github",
    "repo_owner": "acme",
    "repo_name": "office-config",
    "branch": "main",
    "path": "",
    "interval_seconds": 300,
    "poll_enabled": true
  }' \
  'http://localhost:38429/api/v1/office/workspaces/WORKSPACE_ID/config-sync/config'

curl -fsS -X POST \
  'http://localhost:38429/api/v1/office/workspaces/WORKSPACE_ID/config-sync/sync'
```

Except for a missing configuration, a completed force-sync request returns HTTP `200` even when the response contains an `error`; inspect the JSON and `config.last_ok`, not only the HTTP status. A force sync without a configuration returns `404`.

Deleting a config releases every previously managed entity to unmanaged ownership (edits made to them afterward are no longer reverted by a future run) before removing the configuration row. If release fails partway, the configuration is retained and the failure names the entity it stopped at; retrying the delete resumes from there rather than repeating work already done.

> **Network security:** The HTTP API is unauthenticated and can read or change sync configuration with the backend's stored credentials. Keep the backend on loopback or behind an authenticated, origin-protected reverse proxy before exposing it.

## Troubleshooting

- **Authentication error (GitHub):** run `gh auth status --hostname github.com` in the backend environment, or configure one of the token sources above.
- **Authentication error (GitLab):** check the workspace's GitLab connection status under Integrations; config sync has no credentials of its own.
- **"Connect &lt;provider&gt;" failure with an otherwise saved config:** the config was accepted, but the workspace has no connection for that provider yet. Connect it, then use **Sync now**.
- **Directory or branch not found:** verify the resolved owner/repository (or project path), branch, and directory. Config sync does not accept a pasted repository link the way Workflow Sync does; enter each field directly.
- **A control is shown as unavailable:** another config sync source is active for this workspace. Remove it or edit it in place; the refused action is stated in the control itself.
- **Completed with warnings:** read every warning. Files that failed to parse freeze their previous entity; a key colliding with a hand-made entity is never adopted; an unresolvable `reports_to` is left empty.
- **Nothing happens after Save:** save stores only the configuration. Use **Sync now**, or wait until both the configured interval and the poller's next 60-second check have elapsed.
- **Rate limits or intermittent network failures:** lengthen `interval_seconds`, use **Sync now** after recovery, and inspect the GitHub/GitLab integration status and backend logs.

Related guides: [Workflow Sync](workflow-sync.md), [Configuration](configuration.md), and [Operations](operations.md).
