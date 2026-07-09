# Agent Editing

Rename or re-icon:

```bash
$KANDEV_CLI kandev agents update --id A-1 --name "Senior Reviewer"
```

Adjust monthly budget:

```bash
$KANDEV_CLI kandev agents update --id A-1 --budget-monthly-cents 5000
```

Pass `0` for unlimited subject to workspace policy. Pass `-1` to ignore the flag in scripted optional updates.

Cap concurrent sessions:

```bash
$KANDEV_CLI kandev agents update --id A-1 --max-concurrent-sessions 1
```

Retire an idle agent:

```bash
$KANDEV_CLI kandev agents delete --id A-1
```

The backend rejects deletes for agents that are currently `working`. You cannot change an agent role after creation; retire and re-hire for role mistakes.
