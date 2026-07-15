# Workflow Sync — Manage Workflows from a GitHub Repo

Kandev can keep a workspace's workflows in sync with definition files stored
in a GitHub repository. Point a workspace at a repo directory, commit workflow
export files there, and Kandev polls the repo and creates, updates, or removes
the matching workflows automatically. Synced workflows coexist with workflows
you create by hand — manual workflows are never touched by a sync.

This is useful for:

- **Sharing workflows across a team:** everyone's Kandev pulls the same
  definitions from one reviewed repo.
- **Versioning workflows:** changes go through pull requests and history
  instead of ad-hoc UI edits.
- **Provisioning new workspaces:** configure the repo once and the standard
  workflows appear on the next sync.

---

## Setup

1. Open **Settings → Workspaces → \<workspace\> → Workflows** and click the
   **GitHub Sync** button at the top.
2. In the dialog, paste a GitHub link into **Repository link** — a plain repo
   URL, an SSH remote, or a `…/tree/<branch>/<directory>` link that also
   carries the branch and directory. The resolved target (owner/repo and
   directory) is shown under the field; the directory defaults to
   `.kandev/workflows` when the link doesn't include one.
3. Adjust **Branch** — picked from the repository's actual branches once the
   repo is resolved (defaults to `main`) — and **Auto-sync**: when on, Kandev
   polls the repo every **Poll interval** seconds (default 300, minimum 60);
   when off, nothing syncs until you press **Sync now**.
4. Save. A status card appears on the page showing what is syncing, the last
   sync result, and any warnings; use its **Sync now** button to run a sync
   immediately instead of waiting for the poller. Reopen the dialog via
   **GitHub Sync** to change or remove the configuration.

Authentication reuses Kandev's existing GitHub access (the `gh` CLI login, a
`GITHUB_TOKEN`/`GH_TOKEN` environment variable, or a PAT stored in the secret
manager). Private repos work as long as that identity can read them.

## File format

The directory should contain `.yml`, `.yaml`, or `.json` files in the portable
`kandev_workflow` export format — exactly what **Export** produces on the
Workflows settings page. See
[workflow-import-export.md](workflow-import-export.md) for the full field
reference. Files with other extensions are ignored; subdirectories are not
scanned.

A minimal file:

```yaml
version: 1
type: kandev_workflow
workflows:
  - name: Dev Flow
    steps:
      - name: Todo
        position: 0
        is_start_step: true
      - name: In Progress
        position: 1
      - name: Done
        position: 2
```

The easiest authoring path: build the workflow in the Kandev UI, export it,
and commit the exported YAML to the repo.

## Sync semantics

- **Matching:** a synced workflow is identified by its source file path plus
  its `name` inside that file. Renaming a workflow in the repo counts as
  removing one workflow and adding another.
- **Create:** definitions with no matching workflow are created and marked as
  synced (they show a **Synced** badge in the workflow list).
- **Update:** matched workflows are updated in place. Steps are matched **by
  name**, so tasks sitting in a step keep their position when the step's
  color, prompt, events, or order change.
- **Read-only:** the repo is the source of truth. Synced workflows cannot be
  renamed, edited, or deleted from the UI (the API rejects such changes) —
  edit their definitions in the repo instead. Any drift that slips in anyway
  is repaired on the next sync. Reordering workflows on the board and
  exporting them remain available.
- **Delete:** a previously-synced workflow whose definition disappeared from
  the repo is deleted — but only if it holds no tasks.
- **Manual workflows** (created in the UI) are never modified or deleted by a
  sync, even if they share a name with a synced definition.

### When a workflow can't be updated

Some changes can't be applied safely, and Kandev records a **warning** instead
of forcing them. The warning appears in the status banner of the Sync from
GitHub section until you resolve it and sync again. Cases:

- A step was removed from the definition but still has tasks in it.
- A removed workflow still has tasks.
- Step names inside a definition (or the existing workflow) are not unique,
  so steps can't be matched reliably.
- A file is not valid workflow-export YAML/JSON. The file is skipped and its
  previously-synced workflows are left untouched.

A failed sync (repo unreachable, directory missing, GitHub not authenticated)
is also surfaced in the same banner with the error message.

## Notes

- Sync configuration is **per workspace**; different workspaces can track
  different repos, branches, or directories.
- Every sync — periodic or manual — reconciles the workspace against the repo,
  but only writes what actually differs, so a no-drift sync changes (and
  broadcasts) nothing.
- Removing the sync configuration stops future syncs but leaves the already
  synced workflows in place.
