# Managed-runtime cancellation synchronization

E2E run 38432683722, job 115423582398, failed all four attempts of
`cancelling during retry backoff prevents the replacement launch`. Two attempts
observed launch count 2 instead of 1; two missed the retry banner entirely.
The test navigates and hydrates a page after starting the agent, then calls the
real WebSocket stop API. Production backoff is only two to three seconds, so
navigation can consume the cancellation window before the test reaches Stop.

Keep this cancellation case and its independent oracles: the real stop response,
the original session becoming CANCELLED, attempt count 1 immediately afterward,
and attempt count still 1 after the original 3.5-second negative-observation
window. Do not extend production backoff, change the retry budget or remove the
negative observation.

The duplicate retry-banner visibility assertion remains in the first case,
`retries a transient npm failure in the same session and preserves shared files`.
That case still opens the real page and checks the exact banner. The cancellation
case can synchronize directly with the persisted boot message's
`startup_retrying` metadata, published by `updateBootMessageStartupRetryProgress`
before the real backoff. It continues to call the same real WebSocket stop path.
No UI click is removed: this case already stops through `apiClient.stopSession`.

Owners are `manager_startup.go`, `managed_runtime_startup.go`, the script-execution
message component and the deterministic npx fixture. The browser assertion and
component tests continue to protect rendering. The cancellation case protects
the lifecycle boundary independently of rendering speed. No shared fixture,
production behavior or mobile case is removed.

The unchanged complete four-case file passed in 31.4 seconds in the cached CI
runtime image, with one worker and no retries, before this repair. The hosted
failure remains separate evidence; a passing local run alone does not prove
the full concurrent suite is fixed.

The complete repaired file passes all four cases in 32.8 seconds in the same
CI runtime image, with one worker and retries disabled. The first case retains
the browser banner assertion; cancellation retains both launch-count assertions
and the full negative-observation window. This is a correctness result, not a
local speed improvement. Evidence is in the ignored
`e2e-focus/managed-runtime-cancel-candidate.*` reports.
