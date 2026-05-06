---
name: kandev-config-export
description: Export the office workspace config to the .kandev/ folder for version control
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo]
---

# Config export — write office state to disk

Office workspaces persist in SQLite, but the canonical, reviewable representation is the `.kandev/` folder. Export when you need to:

- Commit a configuration change to git
- Diff the current state against what the team agreed on
- Hand the workspace off to another machine

## Export everything

The export action is exposed as an HTTP endpoint that the office config service consumes. There is no dedicated `agentctl kandev config export` command yet — the round-trip happens through the UI's Settings → Config page, or you can invoke the underlying API:

```bash
$KANDEV_CLI kandev config export --workspace $KANDEV_WORKSPACE_ID
```

(Implementation pending: see [office-config spec](../docs/specs/office-config/spec.md). Until shipped, prefer the Settings UI.)

## What gets exported

- `workspaces/<slug>/kandev.yml` — workspace settings
- `workspaces/<slug>/agents/*.yml` — agent profiles
- `workspaces/<slug>/projects/*.yml` — projects
- `workspaces/<slug>/routines/*.yml` — routines + triggers
- `workspaces/<slug>/skills/*` — user-imported skills

System skills (`is_system: true`) are **not** exported — they ship with the kandev binary and are re-installed on every kandev release.

## When to export

- Before a major refactor of the org structure — keep a checkpoint.
- After provisioning a new workspace via UI — commit the initial state to your repo.
- Periodically, as a manual backup, when you can't trust the DB.
