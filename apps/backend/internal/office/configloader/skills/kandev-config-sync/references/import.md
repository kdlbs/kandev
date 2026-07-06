# Config Import

Import when a teammate committed `.kandev/` changes, when seeding a fresh workspace from disk, or when rolling back by applying an older commit.

```bash
$KANDEV_CLI kandev config import --workspace $KANDEV_WORKSPACE_ID
```

The Office service scans `.kandev/workspaces/<slug>/`, diffs YAML against the database, and presents a pending import diff for approval before changes land.

Do not import unexpected destructive changes. Reject the diff and investigate first.
