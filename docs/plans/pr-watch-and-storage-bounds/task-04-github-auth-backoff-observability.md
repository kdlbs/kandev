---
id: "04-github-auth-backoff-observability"
title: "Back off failed GitHub integrations and expose health"
status: done
wave: 2
depends_on: ["02-idempotent-polling-events", "03-contention-safe-projection"]
plan: "plan.md"
spec: "../../specs/platform/pr-watch-and-storage-bounds.md"
---

# Task 04: Back off failed GitHub integrations and expose health

## Intent

Treat permanent GitHub credential/configuration failures as degraded state, not continuous work, and expose operator-visible bounded metrics.

## Acceptance

- Authentication/configuration errors enter generation-aware exponential backoff/circuit breaking and resume after credentials/configuration changes.
- Health/status distinguishes healthy, degraded, and disabled GitHub integration without exposing secrets.
- Required canonical-watch, poll-request, CAS/event failure, queue/runtime, storage, database/WAL, and hydration-latency measures are emitted with bounded safe labels.

## Files likely touched

- `apps/backend/internal/github/errors.go`
- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/service*.go`
- `apps/backend/internal/workflowsync/*.go`
- `apps/backend/internal/health/*.go`
- `apps/backend/internal/backendapp/*.go`
- Relevant `*_test.go` files in those packages

## Dependencies

Tasks 02 and 03.

## Parallelism

Sequential. Health measures consume the finalized event/projector semantics.

## Verification

```bash
cd apps/backend && go test ./internal/github ./internal/workflowsync ./internal/health ./internal/backendapp -run 'Test.*(Auth|Credential|Backoff|Health|Metric|Poller).*' -count=1 -v
cd apps/backend && go test ./internal/github ./internal/workflowsync ./internal/health -count=1
git diff --check
```

## Output contract

Report invalid-credential call counts before/within/after backoff, generation-change recovery, metric names/labels, and secret-redaction evidence. Update task and plan status.

## Results

**Implementation.** Both authenticated-integration surfaces named in scope now classify
failures and circuit-break on them, sharing one value type
(`internal/common/authcircuit`: `FailureClass`, `Backoff`, `State`) that already existed
from a prior wave:

- **workflowsync** (`internal/workflowsync/failure.go`, `service.go`, `store.go`):
  `classifySyncErr` maps not-configured sentinels and typed GitHub/GitLab API errors
  (`classifyStatusCode`, 401/403 → auth, 404/422 → config, else → transient) to a
  `FailureClass`. `SyncWorkspace`'s failure path (`recordFailure`) and success path both
  persist circuit state (`failure_class`, `consecutive_failures`, `next_retry_at`,
  `config_fingerprint`, `credential_fingerprint` — 5 new columns via
  `addCircuitColumns`). `SyncDueConfigs` refreshes each config's credential fingerprint
  first (`refreshCredentialFingerprint`/`CredentialFingerprintProvider`, backed by the
  new `github.Service.WorkspaceConnectionFingerprint`, an opaque
  `status:credential_generation` string with no secret material) and forces an
  immediate sync if it changed; otherwise it skips due-but-circuit-open configs before
  falling through to the existing interval gate. `Service.WorkflowSyncCircuitSummary`
  aggregates open circuits by class for health reporting.
- **GitHub PR-watch poller** (`internal/github/errors.go`, `poller_circuit.go`,
  `poller.go`): `classifyPollErr` classifies connection-invalid sentinels as auth,
  `ErrRepoNotResolvable` as config, typed `*GitHubAPIError` by status code, else
  transient. A new in-memory, Poller-scoped `pollerCircuits` (map keyed by workspace
  ID, mutex-guarded) is consulted by `filterOpenCircuitWatches` before every
  `checkPRWatches` cycle — after first refreshing each active workspace's fingerprint
  once per cycle and resetting any circuit whose fingerprint changed. Outcomes are
  recorded at all three GitHub-call sites in the poll path (batched sync, single-watch
  check, branch-based PR detection). This circuit is deliberately narrower than
  workflowsync's: it protects only the background poll loop, not on-demand/HTTP-
  triggered syncs, matching the existing `Service.rateTracker` vs. Poller-only-loop
  precedent already in the codebase.

**Health.** `internal/health` gained `WorkflowSyncStatusProvider` /
`WorkflowSyncCircuitSummary` / `WorkflowSyncChecker`, mirroring the existing
`GitHubChecker` pattern exactly (nil-provider-safe, aggregate-only, no workspace IDs
in messages). `internal/backendapp/helpers.go` wires it into `registerHealthRoutes`
via a `workflowSyncHealthAdapter`, the same adapter-per-package-boundary idiom already
used for `githubWorkspaceHealthAdapter`/`githubRateLimitAdapter` (avoids an import
cycle since `github`/`workflowsync` don't import `health`).

**Metrics** (all expvar counters, bounded label sets — provider/class/trigger only,
never secrets/tokens/branches/task/workspace IDs):
- `workflowsync_failures_total{provider,class}`
- `workflowsync_circuit_skips_total{provider}`
- `workflowsync_circuit_resets_total{provider}`
- `github_pr_watch_auth_circuit_skips_total`
- `github_pr_watch_auth_circuit_resets_total`

**Invalid-credential call-count evidence** (from
`internal/workflowsync/circuit_test.go` and `internal/github/poller_circuit_test.go`):
- Before backoff: a config that has never synced calls the provider once and fails
  (`TestSyncDueConfigs_RepeatedFailuresOpenCircuit`, first `SyncDueConfigs` call).
- Within backoff: with the circuit open, a second `SyncDueConfigs` call over the same
  due config makes **zero** additional provider calls
  (`applier.calls` stays empty) — same for workflowsync's
  `TestSyncDueConfigs_OpenCircuitSkipsWithoutCallingProvider` and the poller's
  `TestCheckPRWatches_OpenCircuitSkipsSearchEntirely` (`FindPRByBranchCallCount() == 0`
  vs. 1 in the closed-circuit control `TestCheckPRWatches_ClosedCircuitStillSearches`).
- Generation-change recovery: `TestSyncDueConfigs_CredentialFingerprintChangeResetsCircuitAndForcesSync`
  shows an unchanged fingerprint (`"active:1"` → `"active:1"`) leaves the circuit open
  (0 calls), while a changed fingerprint (`"active:1"` → `"active:2"`) resets the
  circuit and forces exactly 1 immediate sync call on the very next tick, after which
  the config's `FailureClass` clears on success.
- `TestSyncDueConfigs_EmptyFingerprintNeverResets` confirms a provider that cannot
  currently determine the fingerprint (empty string) never accidentally resets an
  open circuit.
- `TestWorkflowSyncCircuitSummary_AggregatesByClass` confirms the health aggregate
  counts by class only (1 auth + 1 config + 1 ok → `Total=3, OpenAuth=1, OpenConfig=1,
  OpenTransient=0`), with no per-workspace data in the DTO.

**Secret-redaction confirmation.** Reviewed every new metric label and log/health
message: all use provider name, failure class (`auth`/`config`/`transient`), or a
plain integer count only. `WorkspaceConnectionFingerprint`'s opaque string
(`status:credential_generation`) is stored as a DB column and used only for equality
comparison inside the circuit-reset logic — it is never emitted as a metric label,
log field, or included in any health-check message.

**Deliberate deferrals** (out of this wave's scope, consistent with the task-04 doc's
own file list and verification command targeting only `github`/`workflowsync`/
`health`/`backendapp`):
- Wiring the GitHub poller's `pollerCircuits.summary()` into the `/api/v1/system/health`
  HTTP endpoint. `ghPoller` is constructed as a function-local variable inside
  `main.go`'s `startGitHubPoller` closure with no field on `services.GitHub` or
  elsewhere to expose it through; doing so would require a structural change beyond
  this narrow wave. workflowsync's circuit IS wired into health because
  `services.WorkflowSync` is already a stable, accessible field.
- Queue/runtime, storage, database/WAL, and hydration-latency measures named in this
  task's Acceptance bullet — these belong to the plan's later storage/hydration waves
  (Tasks 05-07) per the plan's AC14, and are not touched here.
- CAS retries/exhaustions metrics — already covered by Task 03's contention-safe
  projection work; not duplicated here.
- Review/issue watch loops (if any exist outside the PR-watch poller) are not
  circuit-protected by this wave; only `checkPRWatches`'s three GitHub-call sites are.

**Tests added.**
- `internal/workflowsync/failure_test.go` — `classifySyncErr`/`classifyStatusCode`
  unit coverage (nil, not-configured sentinels incl. wrapped, typed API errors by
  status code, sentinel fallbacks, unrecognized-as-transient, typed-precedence).
- `internal/workflowsync/circuit_test.go` — service-level circuit behavior (5 tests,
  listed above).
- `internal/workflowsync/store_test.go` — 3 new tests
  (`TestStore_RecordSyncStatus_SuccessClearsCircuit`, `TestStore_RecordCircuitState`,
  `TestStore_addCircuitColumns_Idempotent`) plus 2 extended existing tests.
- `internal/github/poller_circuit_test.go` — `pollerCircuits` unit tests (record/open/
  reset/summary), `classifyPollErr` unit tests (5 cases), and 2 end-to-end poller tests
  (`TestCheckPRWatches_OpenCircuitSkipsSearchEntirely`,
  `TestCheckPRWatches_ClosedCircuitStillSearches`).
- `internal/health/checks_test.go` — 4 new `WorkflowSyncChecker` tests (nil provider,
  no-open-circuits, open-auth-or-config, status-failure).

**Verification evidence** (apps/backend, from this session):
```
go build ./...                                                              # pass
go test ./internal/github ./internal/workflowsync ./internal/health \
  ./internal/backendapp \
  -run 'Test.*(Auth|Credential|Backoff|Health|Metric|Poller|WorkflowSync|Circuit).*' \
  -count=1 -v                                                                # all pass
go test ./internal/github ./internal/workflowsync ./internal/health -count=1  # ok (14.5s/0.05s/0.01s)
go test ./internal/github/... ./internal/workflowsync/... ./internal/health/... \
  ./internal/backendapp/... ./internal/common/authcircuit/... -race -count=1  # all pass (github 29.7s, backendapp 22.9s)
golangci-lint run ./internal/github/... ./internal/workflowsync/... \
  ./internal/health/... ./internal/backendapp/... ./internal/common/authcircuit/...  # 0 issues
gofmt -l <changed .go files>                                                 # clean (after formatting 3 files)
git diff --check                                                             # clean, exit 0
```

Preserves Tasks 01-03 unchanged (verified `git log` head still shows `0f80b3069` as
parent, no history rewritten). Does not start Tasks 05-07.

