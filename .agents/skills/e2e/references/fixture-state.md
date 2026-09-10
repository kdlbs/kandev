# E2E Fixture State

Load this reference for tests that depend on asynchronous capability discovery or
remembered workflow selection.

## Asynchronous capability readiness

For tests using agent models, modes, commands, or options, treat
`not_configured` and `probing` snapshots as provisional. Select by stable name
or test ID and bounded-poll for the semantic capability before interacting.
Avoid arbitrary sleeps and `force: true` clicks.

## Managed-runtime capability probes

Host-utility capability checks call the managed agent's `IsInstalled()` probe
before they invoke the managed `npx` path. If a fixture replaces `npx` to test
package recovery, also put a discoverable executable for that agent on the
fixture `PATH` (for example, a scoped `opencode` shim). Assert that the initial
capability state is `ok` before exercising recovery; otherwise the fixture can
stop at `not_installed` and never test the intended path. Verify with the
focused containers test and `--retries=0`.

## Remembered workflow selection

After seeding tasks in multiple workflows, set
`task_create_last_used.workflow_ids_by_workspace[workspaceId]` to the dialog's
workflow, or clear it to test filter fallback. Assert the selector before
downstream checks: the remembered workflow outranks `workflow_filter_id`.
Verify the focused test with `--retries=0`.
