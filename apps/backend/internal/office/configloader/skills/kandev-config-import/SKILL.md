---
name: kandev-config-import
description: Apply config changes from the .kandev/ folder back into the office DB
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo]
---

# Config import — apply .kandev/ to the office DB

The mirror of `kandev-config-export`. Use when:

- A teammate committed a config change to git that should now take effect
- You're seeding a fresh workspace from an existing `.kandev/` folder
- You want to roll back a recent change by reapplying an older commit

## How it works

The office service scans `.kandev/workspaces/<slug>/` and diffs the on-disk YAML against the DB. Any divergence becomes a `pending_import` entry the CEO (or a user) must approve before it lands.

```bash
$KANDEV_CLI kandev config import --workspace $KANDEV_WORKSPACE_ID
```

(Implementation pending: see [office-config spec](../docs/specs/office-config/spec.md). Until shipped, prefer the Settings → Config page.)

## What can be imported

- Workspace settings
- Agents (excluding `is_system` flags — those are kandev-owned)
- Projects
- Routines + triggers
- User-imported skills

## What can't

- System skills (`is_system: true`) — those flow from the binary on every kandev start. Editing them in `.kandev/` is a no-op.
- Active sessions and run history — not config; not exportable, not importable.
- Approvals — operational state, never in `.kandev/`.

## Safety

The import flow always shows a diff first. If you see a destructive change you didn't expect (e.g. "delete agent A-1"), reject the diff and investigate before applying.
