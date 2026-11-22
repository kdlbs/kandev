---
id: P37-01
title: Bound Queue Acquisition and Cleanup
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-BOUNDED-QUEUE-ADMISSION-001
acceptance_criteria:
  - AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.1
  - AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.2
  - AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.3
  - AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.4
system_design:
  - ../../specs/tasks/system-design/bounded-session-queue-admission.md
---

# Work Order: Bound Queue Acquisition and Cleanup

## Outcome

Implement bounded queue acquisition and its existing recovery-owner integration for
`REQ-TASKS-BOUNDED-QUEUE-ADMISSION-001`, criteria .1-.4, following the
[system design](../../specs/tasks/system-design/bounded-session-queue-admission.md).

## Scope and exclusions

Update messagequeue service acquisition and add its FIFO gate; add the timeout
requeue predicate, handler `UNAVAILABLE` mapping, cleanup attempt budget, and
all affected web API mutation wrappers. Preserve existing user input, provider
order, queue identity/replay, callback context, and cleanup authorization.
Retain permanent production-path regression coverage. Preserve the already
merged #4479 private readiness-expiry classification and #4501 sanitization.
PR remediation and hosted verification are authorized within this delivery.

## Acceptance

1. Contended acquisition returns within its context/budget with no leaked waiter
   or queue mutation; an admitted callback retains its original context/error.
2. A taken message is requeued on the new timeout and operator actions expose
   the existing retryable feedback; identity and attachment semantics remain.
3. Cleanup exhaustion leaves the durable row, and existing restart recovery
   still settles the obligation.

## Files

`orchestrator/messagequeue/service.go`, `messagequeue/session_admission_gate.go`,
`orchestrator/event_handlers_agent.go`, `handlers/queue_operations_handlers.go`,
`handlers/queue_handlers.go`, `orchestrator/queued_dispatch.go`, and web
`lib/api/domains/queue-api.ts`. Retain permanent production-path tests and update the complete affected web
request assertion cohort when timeout arguments change.

## Verification

Hosted focused Go race coverage:

```text
go test -race -p 2 -count=1 -v ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers ./internal/orchestrator -run 'TestSessionAdmission|TestQueueMutationsReturnRetryableAdmissionTimeout|TestQueueAdmissionTimeoutThroughSQLiteHandlers|TestAttachmentCleanup(Budget|Acknowledgements|EditLease)|TestQueuedAdmissionRecovery' -timeout=5m
```

Hosted web coverage:

```text
pnpm exec vitest run lib/api/domains/queue-api.test.ts lib/queue-mutation-error.test.ts
pnpm e2e:run --host --project chromium -- tests/chat/queue-admission-busy.spec.ts --retries=0
pnpm e2e:run --host --no-build --project mobile-chrome -- tests/chat/mobile-queue-admission-busy.spec.ts --retries=0
```

Run the final actual full backend, frontend, and E2E families on the exact
published head. Their normal static checks validate lint, types, docs, and
specification metadata. No local contributor compiler, test, dependency install,
or hook execution is permitted in this review environment.

## Review remediation

Retain the regression cohort and correct retryable feedback across queue
mutations and their callers. Preserve the original grant/reference and callback
lifetime contract, compensation ownership, and durable cleanup recovery. Include
the final cleanup/acknowledgement error in exhaustion diagnostics. Capture the
callback owner with each reservation so an old worker cannot use a new Start
generation for compensation or deferred queue drains.

Implementation and targeted regression coverage are complete:

- The Go race/SQLite contract step in [run 39062354502](https://github.com/kdlbs/kandev/actions/runs/39062354502)
  reported 77 test/subtest PASS entries, including both old-worker/new-owner
  orderings through actual Stop and the callback-owner reset used by Start,
  restore/requeue compensation, FIFO/grant/reference handling,
  transport mapping, and cleanup recovery. That mixed job failed separate,
  subsequently corrected web timeout and collapsed-panel fixture assertions;
  it is not reported as an overall pass.
- [Run 39063127889](https://github.com/kdlbs/kandev/actions/runs/39063127889)
  passed all four cleanup race/budget/acknowledgement cases. The inner lease
  test acquires a real edit lease after the empty precheck and proves that
  admitted cleanup still retains its one actual attempt and captured owner.
- The frontend unit step in [run 39062945804](https://github.com/kdlbs/kandev/actions/runs/39062945804)
  passed 195 tests across six files, including unchanged validation wire
  details and the complete timeout assertion cohort. Its standalone typecheck
  exhausted Node's default heap, so no typecheck or lint pass is claimed.
  The normal full frontend workflow retains its existing 8 GiB heap setting.
- [Run 39063277202](https://github.com/kdlbs/kandev/actions/runs/39063277202)
  passed the desktop and Pixel 5 cases with retries set to zero. A delayed
  unavailable response preserves ten backend rows; reopening the existing
  queue chip shows all ten rows, and an actual backend retry clears them.
  Documentation metadata checks also passed.

The original busy-feedback, stopped-owner, after-check cancel-guard, and
validation-wire failures were reproduced in hosted runs before their respective
production fixes. Incorrect retained-row and panel-open fixture assumptions
remain excluded from product RED evidence. Permanent tests replace deleted
author overlays as standing coverage.

Full exact-head backend, frontend, and E2E qualification remains mandatory before
merge. Its actual attempts, raw logs, artifacts, source bindings, review
dispositions, and ordinary merge receipt are retained by the review task and PR
checks; targeted step results are not an aggregate full-suite pass.

## Historical author results (superseded as qualification)

The following records describe the author's original port. Temporary overlays
were removed and cannot substitute for the permanent tests and hosted final
qualification required above. Historical scope restrictions below do not
constrain the authorized PR remediation.


The initial six-path slice was published at
`7fb36ac129344d73b4a996f782a6c7e678e386d8` on base
`15ed289c050f8bd2bd675c4542ffb796525bf991`. The follow-up below adds the existing
queued-dispatch ownership seam and corrects one existing web assertion. No
standing test case was added. The requirements, design, manifest and this work
order form the same delivery package; operating recovery remains separate.

Compared with the historical patch, deadline conversion now occurs only on
failed lock acquisition. A callback's `context.DeadlineExceeded` is returned
unchanged, preserving the acquisition-only contract rather than treating
admitted work as an acquisition failure.

- Existing messagequeue command above: exit 0, three tests passed, package
  result `ok 0.953s`.
- Explicit temporary overlay, `go test -trimpath -p 2 -race -overlay
<temporary-overlay.json> ./internal/orchestrator/messagequeue -run
'^TestP37Once' -count=1 -v -timeout=60s`: exit 0, four temporary checks passed,
  package result `ok 1.195s`. Covered session/purge contention, own/caller
  deadlines, callback context/error, FIFO cancellation, cancellation after
  grant, and reference release.
- Explicit temporary overlay, `go test -trimpath -p 2 -overlay
<temporary-overlay.json> ./internal/orchestrator/handlers -run
'TestP37Once|TestWsQueueMessageMapsDeniedIdentifiedSessionToAdmissionUnavailable|TestWsQueueMessageReplaysOrdinaryAdmissionWithoutDuplicatingContent|TestWsRemoveEntryRetriesFailedAttachmentRelease|TestPendingAttachmentCleanupResumesAfterHandlerRestart|TestWsUpdateMessageCleanupRetryPreservesAuthAndPublishesStatus|TestPublishStatusDetachesCancelledRequest'
-count=1 -v -timeout=90s`: exit 0, two temporary and six existing checks passed,
  package result `ok 0.338s`. The temporary budget check retained a durable row
  after exhaustion and settled it after reopening the existing fixture, with
  the captured user authorization preserved.
- Explicit temporary overlay, `go test -trimpath -p 2 -overlay
<temporary-overlay.json> ./internal/orchestrator -run
'^TestP37OnceAdmissionTimeoutRequeuesTakenWork$' -count=1 -v -timeout=90s`:
  exit 0, the taken message retained its content and identity after requeue,
  package result `ok 0.409s`.
- Web: `pnpm exec vitest run lib/api/domains/queue-api.test.ts`: exit 0,
  one existing file and 30 tests passed. Changed-file ESLint and Prettier:
  exit 0. Dependencies were installed once with
  `pnpm install --frozen-lockfile`; no dependency files changed.
- Specification metadata checked through the existing linter's scoped
  metadata functions: exit 0, two files. Documentation was formatted through
  existing Prettier; `git diff --check` passed.
- The first normal changed-file Go-lint run across all three affected packages
  returned exit 1 with `0 issues` followed by `Timeout exceeded`. This is not a
  lint pass or a demonstrated source/baseline defect. The invocation used the
  correct comparison base above, process-local repository defaults, and no
  operating/model/profile limit changes. A narrower package split is recorded
  separately below.

All three temporary Go backing files and their overlay JSON were removed after
their actual results were recorded. Their virtual test paths were never
created in the repository, and temporary budget changes existed only in the
test processes. No raw temporary source or overlay remains for collection.

The normal changed-file lint split for `messagequeue` and `handlers` returned
exit 0 with `0 issues`. The isolated orchestrator package also returned exit 0
with `0 issues`. Both used `scripts/lint-go-changed` and the fixed comparison
base above; splitting the initial invocation completed validation without
changing the repository's resource defaults or production code.

Integration documentation: the existing public coordination explanation now
describes the acquisition-only 30-second wait and retryable busy outcome for
Remove and Auto-run. No public action, setting, or screenshot changes.
`node scripts/validate-public-docs.mjs` returned exit 0 and validated 48
published pages. This documentation-only delta does not repeat code tests.

Publication preflight found missing work-order linkage metadata. Added the
existing required frontmatter to this work order and its plan, retaining the
same requirements and criteria. The existing `pr-docs.cjs` coverage evaluator
returned exit 0, `status: covered`, and no errors for all eleven changed paths.
Production and test sources did not change.

## Publication follow-up

At published head `7fb36ac129344d73b4a996f782a6c7e678e386d8`, source review
confirmed that the browser's five-second default could discard Auto-run and
Remove responses before the 30-second admission result. Both requests now use
the existing 35-second transport allowance. The existing Auto-run assertion was
updated; no new permanent test case was added.

An active edit lease also consumed the cleanup budget without a cleanup attempt.
The loop now counts actual cleanup/acknowledgement attempts and excludes the
post-admission lease refusal. The existing wake, stop and durable recovery paths
remain. One collector-excluded temporary check held an ordinary edit beyond a
one-attempt budget, confirmed retained pending ownership and zero cleanup calls,
then ended the lease and completed cleanup once. Its source/overlay were removed.

Existing web tests: one file, 30 tests, exit 0. Final temporary plus six existing
handler checks: exit 0, package result `ok 0.650s`. Scoped web ESLint: exit 0.
The first Go-lint attempt was rejected because the normal P46 commit was already
running golangci-lint; this is not a source finding or a pass. A sequential
changed-handler lint was started after that owner's Go lint completed.

Failed requeue/restore now waits for compensation settlement before the existing
queued worker can clear its reservation. Only acquisition timeouts are retried,
using the existing reserved-prompt callback owner's cancellation and Stop join.
The reservation phases, durable claims, original queue identity/FIFO, caller
context and terminal-error handling remain. Stop also suppresses pending drain.

Source review of the two recovery paths found no mandatory defect. Temporary
native-worker checks reproduced the original failure at virtual 61 seconds,
after two 30-second admission waits. With the correction, ordinary taken work
and a valid retained lifecycle reservation kept their markers until contention
cleared, then restored visible FIFO work and metadata. Actual Service.Stop
cancelled/joined compensation and retained durable claim data; its disposable
fixture had no running scheduler, so full shutdown success is not claimed.
Caller and terminal errors were not retried. The ordinary/Stop/error cases passed inside
a mixed command whose unsupported retained-ordinary fixture failed; those valid
results are reused, not labelled an aggregate command pass. The corrected native
retained-lifecycle case passed separately (exit 0, package `ok 0.902s`). Fixture
compile/lifetime errors and the unsupported reservation combination are recorded
separately from product failures. Final changed-handler and changed-orchestrator
lint each exited 0 with zero issues; scoped orchestrator vet also exited 0.
All four recovery-source/overlay temporary files were removed, with leftovers
`[]`. No operating provider or new standing case is used.

## Upstream conflict refresh

The existing PR head `ad57ce7416b111b40702168ca7db2fc70c58934a` was merged
with current upstream `e9b836454c75e70a4be6660a8f8c82182312d356` locally.
The only conflict was the queued-error retry condition. The resolution keeps
both `ErrSessionAdmissionTimeout` and the upstream classified runtime-wait
timeout; neither a foreign provider deadline nor a permanent failure becomes
retryable. The other twelve PR files retained their published source before
this verification note was added.

Focused `go test -trimpath -p 2 -race -overlay <temporary-overlay.json>
./internal/orchestrator -run <five-named-tests> -count=1 -v -timeout=90s`
returned exit 0 (`ok 1.139s`). Four existing tests cover runtime-wait requeue,
permanent failure, foreign deadline and classified provider deadline. One
collector-excluded temporary test covers direct and wrapped admission timeouts
through the actual queued-error handler, retaining the original message ID,
target, content and FIFO timestamp without an in-flight reservation. Both
temporary cases passed; the backing source and overlay were removed. The
unchanged queue/cleanup/web scopes reuse their recorded checks above. This is
a local conflict-refresh candidate, not a publication, merge or operating change.

The first normal merge-commit hook ended with Go-lint exit 4: it printed
`0 issues` and then `Timeout exceeded`. This is a failed hook, not a lint pass
or a demonstrated source defect. The staged resolution was retained, no commit
was created, and the same normal hook is retried without changing its limits,
configuration or bypass flags. Exact hook receipts accompany the candidate.
