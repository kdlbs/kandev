# Config Import

Import when a teammate committed `.kandev/` changes, when seeding a fresh workspace from disk, or when rolling back by applying an older commit.

The Office service scans `.kandev/workspaces/<slug>/`, diffs YAML against the database, and presents a pending import diff for approval before changes land.

Use the UI at Settings > Config. There is no agentctl `kandev config import` command yet.

Do not import unexpected destructive changes. Reject the diff and investigate first.
