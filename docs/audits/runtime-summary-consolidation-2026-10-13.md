# Runtime summary E2E consolidation experiment

Status: rejected for this batch; the original tests and helper are retained.

The desktop and phone runtime-notification files each run the same real
backend availability window and reconnect replay. Both invoke the same helper;
the phone adds its viewport and 44px action check. The local notification
provider broadcasts to all subscribed connections for the user, while durable
delivery claims belong to the user/provider/runtime version.

The candidate used one backend episode with two independently subscribed browser contexts:
the existing Pixel 5 test page and the fixture's Desktop Chrome page. It ran the
original per-client assertions on both contexts, preserving initial status and
subscription readiness, reconnection, immediate failed-update outcome, distinct
summary members and versions, one toast, viewport containment, Review updates
navigation, expanded Settings section, reload and the full negative replay
observation, including the original 31-second dwell and 45-second event deadline.
Neither production timers nor timeouts changed.

The candidate phone case remained in its original mobile project. The desktop case
name became an executed named step containing both observers. Other runtime
awareness, consent, native fallback, narrow fine-pointer and 767/768px boundary
tests stayed independent. The attempt combined backend setup and elapsed waiting,
retaining real events and both desktop and phone assertions.

Authority: [runtime summary requirements](../specs/agents/requirements/runtime-update-summary.md),
especially the fixed 30-second window, durable replay suppression, immediate
outcomes and desktop/phone geometry. Owners are the notification service,
`notifications/providers/local.go`, `use-update-available-toast.ts` and the
shared runtime status bridge. No product UI changes are needed.

## Verification

The unchanged two-file baseline passed all nine cases in the pinned CI runtime
image, one worker and retries disabled. The complete candidate run also included
the separate mobile goal repair; those cases do not enter the notification
comparison. This local experiment is not hosted acceptance of the 50% E2E goal.

The combined two-client candidate failed in its complete file run while
waiting for a reconnect `user.subscribe` response. The isolated traced keeper
then passed, including both viewport assertions and full replay windows. That
is insufficient evidence that the concurrent reconnect setup is reliable.
The original two independent cases and helper were restored. No removal or runtime
saving from this experiment is counted. Candidate sources, both reports and
logs are retained in ignored `e2e-focus/runtime-summary-*` and `candidate-*`.
