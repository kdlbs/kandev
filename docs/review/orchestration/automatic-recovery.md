# Orchestrator automatic recovery

A temporary Claude account refresh lock was being treated as terminal. Manual
recovery worked after the lock cleared, but required repeated intervention.

The execution owner now delegates Orchestrator failures to its durable run queue
before rendering a terminal error. Eligible failures automatically launch a new
execution with current authority and fresh run/session credentials. The original
request, account and run identity remain intact. Retries use the stored original
prompt; later messages cannot be absorbed into an earlier request. Both workspace and private
Orchestrator conversations use this path; ordinary chats retain their existing
recovery policy, with the added refresh-contention classification.

Recovery waits 15/30/60/60/60 seconds, at most five retries. The conversation shows
the existing retry notice and Cancel action. Stop cancels a scheduled retry too.
The notice is removed when recovery begins. Later turns cannot overtake the
recovering request, and safe queued retries persist across backend restart.

Automatic replay requires identified, known prompt-attempt evidence with no
output and no tool effects. Unknown outcomes, actual authentication failures,
exhausted attempts and dynamic-route-owned failures retain manual recovery.
Interrupted active runs remain manual; the change never assumes their writes
were unsuccessful. The runtime keeps revalidating permissions on relaunch.

Regression coverage reproduces the original failure and checks relaunch,
credential rotation, stale/duplicate events, cancellation, ordering, retry budget,
unsafe replay rejection, warning state and persisted notice cleanup. Fixtures
contain synthetic requests only. No user prompts or transcripts are included.

No schema migration, provider package update, new API key or permission broadening
is required. Public documentation: `docs/public/orchestration-personas.md` (how-to).

## Qualification

The final bundle passed an isolated Claude ACP trial. A local protocol proxy
returned the production refresh-contention error for exactly one prompt without
forwarding that prompt to the provider. Kandev queued the original run, showed
its waiting state, relaunched automatically and received the expected generic
response. There was one run, one retry, and no leftover retry notice. The fixture
used a disposable workspace and generic request; it never loaded live chat
history. See [provider receipt](automatic-recovery-provider-receipt.json).

- Runtime, core orchestrator, routing classification, run repository and backend
  application test suites passed.
- Race checks passed for recovery, cancellation and existing transient-retry
  paths; the complete Orchestrator runtime suite passed under the race detector
  after the final change.
- SQL guard and store conformance passed for the available SQLite environment.
  PostgreSQL conformance was skipped because no test DSN was configured.
- All commit hooks, including the full branch comparison for Go lint, passed.
- Public docs validator tests passed; all 48 published pages validated.

## Live deployment

Deployed `0.94.0-orchestration.20260919.sha0521e64c651c` after the final provider
trial passed and the live service had no active runs or sessions. The final cold
backup passed database/key hash matching, integrity and foreign-key checks.
Post-restart health reported the exact candidate; authentication remained
required and all tracked conversation/history rows were retained. The previous
bundle and matched backup remain available locally for rollback.

[Deployment receipt](automatic-recovery-live-receipt.json).
