# Config Export

Export when you need to commit configuration changes, diff current state against the agreed setup, hand a workspace to another machine, or keep a manual checkpoint.

```bash
$KANDEV_CLI kandev config export --workspace $KANDEV_WORKSPACE_ID
```

Exported config includes workspace settings, agent profiles, projects, routines and triggers, and user-imported skills.

Until the CLI flow is fully shipped, prefer the UI at Settings > Config.
