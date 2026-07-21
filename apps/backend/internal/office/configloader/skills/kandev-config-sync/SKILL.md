---
name: kandev-config-sync
description: Synchronize Office workspace configuration with the .kandev folder when exporting reviewable config, importing committed config, seeding a workspace, or reviewing a config diff.
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo]
---

# Config Sync

Use this skill when the Office database and the reviewable `.kandev/` configuration need to be compared or synchronized.

## Operations

- Export current workspace state to disk: read `references/export.md`.
- Import `.kandev/` changes back into the Office database: read `references/import.md`.

System skills are bundled with the kandev binary and are not user-exported configuration.
