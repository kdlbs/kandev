# E2E Fixture State

Load this reference for tests that depend on asynchronous capability discovery,
remembered workflow selection, correlation of asynchronous records, restart
persistence, or provider-backed GitHub PR review/check eligibility.

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

## Correlate asynchronous records

When one action creates related persisted records and runtime objects, match
them by a stable shared request, causation, or correlation ID. A workspace-wide
list can contain unrelated concurrent work; do not associate records by taking
the first, newest, or merely unseen item. If no correlation ID is available,
filter by the full relation and assert that exactly one candidate matches.

## Restart and reconnect persistence

When restart or reconnect recovery promises to restore client or agent
configuration, set a non-default value before the restart and assert that the
reconnected consumer receives that value. A task/session row or transcript
reappearing proves stored records survived, but does not prove the effective
configuration was replayed. When possible, verify the value through a
post-reconnect interaction as well as the restored client state.

## GitHub PR review and check state

When refreshed PR eligibility depends on reviews or check runs, seed the mock
provider's feedback records as well as aggregate PR counts. Refresh recomputes
those counts from provider state, so aggregate-only fixtures can revert to
ineligible results. Use `ApiClient.mockGitHubSeedPRFeedback` for the required
approvals and passing checks, then assert the refreshed UI. Verify the focused
desktop and mobile tests with `--retries=0`.
