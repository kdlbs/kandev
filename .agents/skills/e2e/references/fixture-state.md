# E2E Fixture State

Load this reference for tests that depend on asynchronous capability discovery or
remembered workflow selection.

## Asynchronous capability readiness

For tests using agent models, modes, commands, or options, treat
`not_configured` and `probing` snapshots as provisional. Select by stable name
or test ID and bounded-poll for the semantic capability before interacting.
Avoid arbitrary sleeps and `force: true` clicks.

## Remembered workflow selection

After seeding tasks in multiple workflows, set
`task_create_last_used.workflow_ids_by_workspace[workspaceId]` to the dialog's
workflow, or clear it to test filter fallback. Assert the selector before
downstream checks: the remembered workflow outranks `workflow_filter_id`.
Verify the focused test with `--retries=0`.
