---
id: "02-retry-delivery-ownership"
title: "Close retry delivery and launch ownership gaps"
status: done
wave: 2
depends_on:
  - "01-durable-retry-lifecycle"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-RETRIES-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-RETRIES-001.1
  - AC-OFFICE-AUTOMATION-RETRIES-001.3
  - AC-OFFICE-AUTOMATION-RETRIES-001.4
  - AC-OFFICE-AUTOMATION-RETRIES-001.9
  - AC-OFFICE-AUTOMATION-RETRIES-001.10
system_design:
  - ../../specs/office/system-design/automation-retries.md
---

# Task 02: Close Retry Delivery and Launch Ownership Gaps

## Outcome

Retry events reach the correct snapshot-owned destination, replay preserves exact identity and history, and run controls remain safe through uncertain provider acceptance.

## Scope and dependencies

- Resolve persisted run/snapshot routing before the destination-reader gate and exercise a capability-complete real event handler.
- Reject conflicting exact binding and preserve original action/reason on replay.
- Canonicalize disabled policy before active-only validation.
- Preserve timeline refresh/mode switching, surface Stop errors, serialize Stop submissions, and label the retry radio group without new copy.
- Trace session preparation, turn admission, durable delivery submissions, queued starts, and termination before implementing launch coordination. Persist one launch identity before external side effects; uncertainty must retain ownership and prevent automatic replay. A task external ID alone is insufficient.
- Reconcile with authoritative main, preserving current catalogue and selector identities, and address actual failed E2E leaves without weakening assertions or timeouts.

No provider idempotence assumption, live automation, real agent, operator workspace transfer, or credentials are permitted. No new service abstraction, rollout flag, or unrelated Settings refactor is included.

## Implementation acceptance

1. Run-only initial/recovery events and invalid snapshot/read failures are covered at the real event entrypoint; exact replay preserves history and conflicting bindings fail at persistence.
2. Two instances, lease expiry, queued-start replay, and crash after acceptance cannot produce a second launch. Stop/disable/delete retain unknown accepted ownership until resolved or stopped, on both SQLite and PostgreSQL.
3. Desktop and phone controls retain history mode and accessible naming; Stop failure is caught and duplicate taps do not duplicate requests. All hosted checks pass on the final batch.

## ASCII UI preview

UI-01 extends the [plan preview](plan.md#ui-01-retry-policy-target-eligibility-and-history-presentation). Composition is shared; phone controls retain their existing full-width 44px touch targets.

```text
Retry policy [named radio group]
  [Do not retry] [Finite] [Infinite]
Run history [Attempts / Timeline]
  [Stop retries] -> [busy, disabled] -> refreshed same history mode
  Failed stop -> localized error; run ownership remains visible
```

The preview specifies control state and accessible naming, not spacing or translated text. Existing selectors and localized keys remain authoritative.

## Files and verification

Primary boundaries: `internal/automation/{service,store,retry_operations,retry_recovery}.go`, `internal/orchestrator/{event_handlers_automation,automation_ceiling_start,task_operations}.go`, the automation run hook, retry policy section, and desktop/phone automation E2E coverage.

Hosted verification commands:

```sh
make -C apps/backend test
make -C apps/backend lint
cd apps/backend && go test -race ./internal/automation ./internal/orchestrator
cd apps/web && pnpm run typecheck && pnpm test --run
cd apps/web && pnpm run lint && pnpm run i18n:check
cd apps/web && pnpm e2e:run
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

Commands are separate CI steps with the appropriate working directories. Broad checks run on hosted CI; only tiny synthetic proofs and explicit formatter paths run locally in inspected credential-free containment. PostgreSQL concurrency tests use the existing hosted fixture contract.

## Results and remaining gate

Implementation complete. Independent corrections have permanent regressions for snapshot-owner routing, immutable exact binding and history, disabled-policy normalization, timeline mutation/mode refresh, Stop failure and duplicate requests, and accessible radio naming on desktop and phone. The dated progress notes below describe evidence available at each earlier head; their pending statements are historical, not the current implementation status. The final hosted summary supersedes them for the delivered implementation. Final-head delivery remains subject to the required checks and current-main integration gate.

### Reviewed launch ownership contract

The local implementation separates task creation from provider launch. Task-only operation commitments retain their creation identity but cannot authorize provider replay. The launch ledger reserves the immutable task/session/turn/submission tuple before effects, then fences admission and acceptance across service instances. Acceptance that cannot be resolved remains visibly pending; neither lease expiry nor absence from one executor releases ownership.

The implemented boundaries covered by the hosted regressions are:

1. Reservation and capacity admission serialize on the automation row in both dialects. Protocol provenance is recorded with admission and inherited by scheduled children; unresolved legacy provenance cannot reserve another launch.
2. Fresh and restored-workspace starts carry the same reserved identity into the existing initial-submission admission and acceptance receipts. Commandless workspace promotion decides its full launch path before consuming the single-use provider admission.
3. Queued release carries the same tuple and claims a fresh token only from definitively deferred work. Preparing, dispatching and uncertain work cannot be reclaimed after a lease expires. Accepted evidence can be reconciled only onto the original tuple.
4. Controls revoke group generations and future publication before attempting termination. Preparing, dispatching, accepted and unresolved legacy ownership remain until an exact terminal event or definitive synchronous stop. Retained owner tombstones fence delayed session/turn creation after settlement and history deletion; transaction-scoped exact manual permits preserve explicit manual creation without metadata authority.

Required semantic tests exercise the actual automation event, generic start, ceiling queue release, recovery and control endpoints: two service instances; lease expiry during blocked admission; acceptance followed by crash before binding; delayed queue release; stop/disable/delete racing admission; replay with changed binding or generation; and concurrent capacity admission. Provider boundaries use deterministic fakes. The source must not be pushed while these guarantees remain unimplemented.

### Integration and existing CI evidence

The contributor rebase `3ebe7b3072a5065d8c8be3ffda2ad1a7e367e707` already includes authoritative main `02ff0578357040b0546ea17cd9a00dff9ca9ee3b`. Its source/history and transitive inputs were statically audited before reconciliation. Forty saved owned files merged without conflicts; the previous integration/index is preserved in the private recovery ref and external byte/hash evidence. This retains catalogue, profile ordering, clarification, task transfers, agentctl authentication, journal shutdown/transactions, and the current CI project partitions and test-only filters. Corrective PR #4399 remains open at the last authenticated read; its PostgreSQL profile-order fixture correction must enter through a normal merged main rather than a duplicate patch. Final live-main reconciliation and hosted validation remain outstanding.

The old exact-head E2E failures remain real. Shard 5 failed visible diff line metadata; the reconciled helper addresses the observed positioning boundary but needs hosted validation. Shard 3's prompt label tap left its checkbox unchecked. The first failure has a screenshot and accessibility snapshot but no trace; its retry passed. Those artifacts do not establish a product root cause, and neither the assertion nor fail-on-flake policy is weakened.

### Launch implementation progress (unverified)

The current local batch adds versioned admissions and immutable launch records, non-expiring claim/acceptance fencing, identity propagation through session and turn creation, same-identity queue replay, cancellation/deletion retention, and actual provider acceptance callbacks. Database insertion guards cover both versioned and unresolved legacy retry rows. The exact SQLite legacy INSERT bypass has a semantic RED/GREEN proof with ordinary task behavior preserved (`../pr3898-evidence/ownership/guard-*.log`). Added permanent tests cover actual task turn and executor session persistence and the normal automation launch entrypoint with a fake provider boundary; these have not run in hosted CI yet.

The stop boundary audit found that the existing coordinator acknowledgment precedes detached teardown. A separate exact-execution synchronous stop now retains uncertainty on a failed acknowledgment and permits retrying that same execution without adopting a successor. Cold continuation recovery, all persisted mappings/SQL dialects, full entrypoint coverage, lint limits, and latest-main integration remain under review. No push or merge has been attempted; these partial results do not satisfy the delivery gate.

The contributor rebase to 3ebe7b307 was statically reconciled onto main 02ff0578; all 40 saved owned files merged without conflicts. The pre-binding task-creation interval now has metadata association guards in both SQL dialects and a regression through the actual StartTask and older PrepareSession boundaries. These Go tests remain unexecuted pending hosted CI.

Static legacy entrypoint tracing found that startTurnForSessionWithOwnershipChecked may reuse an existing turn, so INSERT guards alone do not fence that warm path. Canonical recovery-block quarantine/backfill now uses the existing checkSessionRecoveryBlock boundary and actual incarnation/generation, without fabricated acceptance evidence. Permanent tests cover the actual block lookup, older warm prompt admission, ordinary sessions, schema replay and both dialects. These remain unexecuted pending hosted CI.

### Retired admission compatibility

Run-history deletion and a known-execution stop do not revoke a separate delayed worker by themselves. Persist retry admission owner tombstones independently of history rows, including the task owner acquired during binding and the run owner carried in late task metadata. Session and turn INSERT guards must consult this retained evidence after cancellation, supersession, terminal settlement, and history or automation deletion. Only a matching live reserved tuple may admit an automatic creation. Preserve explicit manual creation through a server-owned request-origin context authorizing the exact creation ID inside the repository transaction; neither task metadata nor inherited session metadata grants that exception. The transaction inserts a temporary exact-kind/task/creation permit, performs the guarded INSERT, and removes the permit before commit; failure rolls both back. Older dispatchers cannot inherit a permit from product metadata. Tests must cover a delayed older PrepareSession, late task creation with automation_run_id after stop/delete, unrelated ordinary creation, explicit manual session/turn creation, and a new independently reserved retry on a reused task. Retained tombstones carry ownership and revocation only, never fabricated provider acceptance or termination.


The retained-admission contained SQLite proof reproduces cancellation's old INSERT bypass (semantic RED exit 1), then rejects delayed session/turn creation after stop, history deletion, and automation deletion (GREEN exit 0). It covers late task metadata ownership, copied manual metadata refusal, exact transaction manual permits, ordinary tasks, independent reserved retry reuse, disabled admission, and legacy owner tombstones. Source hashes and scope are recorded in `../pr3898-evidence/ownership/retired-proof/`; this is literal SQL/template validation, not full Go or PostgreSQL execution. Permanent tests exercise real executor preparation after exact StopRun and history deletion, automatic session opening, explicit manual preparation and turn creation, and transactional permit cleanup/rollback on both dialects. These permanent tests remain unexecuted pending hosted CI.


Control revocation and ownership settlement are separate transactions. Stop, disable, and deletion first revoke the affected live group's generation, future outbox publication, and any prepared/queued admission. Scheduled or proven unstarted work may settle immediately; preparing, dispatching, accepted, or unresolved legacy work retains its run and exact ownership evidence until definitive stop/terminal evidence. A failed control therefore cannot revive an earlier retry chain when enablement changes later. Final history/automation deletion still requires resolved ownership.


The current control proof rejects re-enabled future generations, revokes outbox work and retains preparing and legacy attempts (contained literal SQLite SQL, exit 0). The legacy stop proof checks exact task/session/turn/generation and retained committed, ambiguous or revoked operation tuples; a conflicting retained owner cannot certify termination (exit 0). The Go helper also checks the receipt write's affected-row count. Missing local execution and unbound legacy reconciliation have permanent regressions requiring visible uncertainty and refusal of deletion. A conversation fork checks pending retry ownership before its provider boundary and authorizes only its exact manual destination creation after existing access checks. The task repository's temporary permit schema replay, cleanup and rollback have both-dialect regressions.

Current-head CI evidence independently downloaded all additional failed frontend/E2E leaves and aggregates: frontend run 38041169276 failed 12 tests in five catalogue/MCP/mock files; E2E run 38041169290 shard 14 timed out in the saved profile-order selector test on all four attempts. Its retained trace reaches Quick Chat cancellation after Escape, then waits for a missing Cancel button. These shared-main failures must be mapped to normal merged #4399 before final integration; no duplicate selector patch, assertion weakening or timeout change is included here. Full Go, PostgreSQL, race, React and E2E validation remain outstanding.


The stop-entrypoint audit found that a task-only projection lookup omitted accepted-but-unprojected legacy operations. The local batch now includes observed operation ownership in the task lookup, validates partial binding consistency and hydrates only the returned view. Operation capture/backfill retains task ownership and applies canonical legacy quarantine before projection. A contained SQLite causal proof failed without those operation triggers (semantic RED) and passes with them, including history deletion and ordinary-task preservation (GREEN). Initial capsule-key and fixture-column setup failures are retained separately and are not semantic RED. A permanent actual StopRun regression covers unknown runtime acknowledgment, retained unprojected IDs, successful exact acknowledgment and cleanup-claim completion; PostgreSQL has an operation-owner/quarantine/schema-replay regression. Full package execution remains pending hosted CI.


The final main integration includes the normally merged profile-ordering fixture repairs and durable delivery reconstruction/retention. Retry initial launch persists the executor's final composed prompt under its pre-reserved canonical `prompt:retry:` submission identity before harness start. Commandless workspace promotion consumes admission only after command configuration. The shared resume entry point fences both ordinary and reserved-capacity recovery, while explicit Resume, Fresh Start and Continue from History reject unresolved retry ownership before settlement or continuation checkpoint mutation. State-only delivery reconciliation retains main's authenticated identity checks and does not authorize a new retry launch.

Verification remains bounded: the exact retry callback with deterministic dependency fakes failed when SQL admission was omitted (exit 1), then passed with final prompt, reserved identity, attachments and refusal cases (exit 0). This does not execute the complete Go/SQL/provider path. Permanent actual-entrypoint tests add canonical SQL-before-provider admission and unchanged turn identity across explicit recovery actions; full Go/PostgreSQL/race/E2E execution is pending hosted CI. The owned sandbox was re-inspected and restarted after machine shutdown; the initial stopped-container error was setup failure, not semantic RED.


Silent restart recovery also treats a retry continuation on an ordinary-origin task as automation-owned. Initial owner eligibility and each owner revalidation fence that session before source cleanup, queue pause, candidate allocation or checkpoint advancement. Candidate allocation revalidates ownership independently. The ordinary non-retry path remains eligible. Exact owner-method proofs with dependency fakes fail without these guards and pass with them; the permanent real-repository regression asserts source execution, queue and checkpoint preservation. Full package and race execution remain hosted-CI work.


Hosted CI on remediation head `13467a6c` exposed a legacy recovery visibility gap: terminalization rejected an unknown launch binding without the typed uncertainty error expected by reconciliation. Unbound legacy recovery now records uncertainty before considering outbox replay or terminalization, retaining its task/session/turn fields and open status. Modern replayable admissions remain untouched. An exact reconciliation-method proof with deterministic dependencies failed against the previous source and passed after the correction; the actual SQL update also passed an isolated in-memory SQLite proof with modern, stopped, ordinary and terminal controls. Full service and PostgreSQL verification remains pending the replacement hosted run.

The dispatch contract does not equate a failed provider response with termination. Updated fixtures reserve their immutable identities before dispatch and assert that ambiguous work never invokes a second callback or schedules a child. The retry-child fixture supplies exact terminal evidence before separate, idempotent failure finalization. Pre-binding history deletion succeeds only after a definitive stop of the complete retained operation tuple; task-only legacy operation fixtures require deletion to fail closed with no stop or task deletion. These permanent regression assertions are authored, awaiting hosted execution.


Crash-resumed integration through normal main merge `1bba2c18` retains the early retry-owner fences before silent restore eligibility, candidate/checkpoint allocation and explicit recovery. Main's already-resumed reconciliation revalidates those owners before queue and provider operations. Exact eligibility/revalidation methods still fail the ordinary-origin retry-owner fixture when the guards are removed and pass with them in the owned contained capsule. The full recovery path remains a hosted-CI obligation.

All recovered `627b5bdc` failure logs and structured artifacts were inspected. Two compilation errors (unused dialect import and repeated error declaration) are corrected, and the reserved-turn test now seeds its task and asserts session creation instead of silently relying on an invalid fixture. The PostgreSQL protected-gap accounting failure maps to main's retained settlement fence; no duplicate patch or weaker assertion was added. E2E artifacts recorded zero browser executions after the failed build. Exact quarantine-installer compilation with deterministic dependencies is verified separately from semantic RED/GREEN; full application, PostgreSQL, race and browser verification awaits the integrated head.


Hosted `732c96a5` verification compiled the integrated application, passed desktop smoke and the PostgreSQL 18 gate, and exposed three orchestrator regressions plus 15 style findings. The queue regression identified mutable launch claim fields in the serialized deferral identity. Deferred starts now serialize only the immutable run/automation/generation/task/session/turn/submission tuple, and admission comparison applies the same normalization to retained older payloads. Replay still reloads the current SQL claim and preserves every existing token/state/generation/cancellation gate; payload equivalence grants no launch authority. An exact comparator/JSON normalization proof fails against the prior source and passes with both legacy-wire and current typed payloads, unchanged inputs, and rejection of every conflicting tuple field and prompt. The permanent actual queue-release/replay regression retains its original timeout and single-provider assertions; replacement hosted execution is pending.

The durable prompt fixture compares the complete final provider prompt with the persisted submission, including task identity and the original prompt suffix. The silent-recovery fixture now uses the repository's supported SQLite dialect label. Reported style findings are addressed by extracting existing binding, provenance, exact stop evidence, snapshot hydration and transactional supersession helpers, while retaining all SQL gates, assertions and limits. No lint threshold or waiver was added. Full replacement unit, race, PostgreSQL, lint and browser verification remains required.

### Hosted queue ownership and fixture race correction

At `aa7925a7`, hosted shard 2 passed canonical submission, commandless
workspace fallback, two-service blocked launch, legacy persistence fences
and ordinary-origin silent recovery tests, but its queued
launch test reported a fixture race while replacing the Stop callback during
detached cancellation. Queued dispatch was also cancelling a returned
execution after retry acceptance became uncertain. That cancellation is not
termination evidence and conflicts with retained ownership: the queue wrapper
now returns the pending error while retaining the execution. Ordinary binding
failure still stops its execution. An actual queue-entrypoint regression has
a reachable ordinary-stop control, and the fixture callback setter shares the
mock manager mutex with its reader. Exact queue wrapper/converter proof
failed with one unexpected cancellation on old source; replacement proof and
full hosted race verification are recorded separately. Helper moves to small
files preserve the file-length gate and add no waiver.

### Current-main recovery integration

Main `bc0d3b904` adds loaded native-delivery retirement to shared admission and
restart presentation repair. These boundaries must consult durable retry
ownership before invoking retirement or changing recovery presentation, even
for an ordinary-origin task. The existing ordinary-origin regression now
asserts no loaded-retirement callback and unchanged block, source execution,
queue and restore checkpoint. Existing native recovery tests preserve ordinary
non-retry restoration. An extracted exact loaded-recovery function with small
repository/runtime seams failed with one unauthorized callback before the
fence and passed with zero callbacks after it; the ordinary positive control
still invokes the runtime boundary. This is a contained causal proof, not full
Go/SQL/provider execution. The first proof compile failed because its repository
seam was concrete rather than an interface; that setup failure is not RED.
Final integrated full Go/race/PG/UI verification remains hosted.


### Hosted implementation completion and delivery gate

Hosted Backend Tests on `6cf6050a3a846b61489bb3595935e536df449e7f` passed unit, race, PostgreSQL 16/18 and platform jobs. Its structured shard-2 artifact contains passing actual entrypoint tests for canonical reserved submission, commandless workspace promotion, cross-instance blocked launch, ceiling queue release/replay, legacy persistence/quarantine, pre-binding Stop/deletion and ordinary-origin silent/loaded recovery fences. `TestQueuedRetryUncertainAcceptanceRetainsExecution` passed both retained-uncertain and ordinary-error-stop branches. Full Frontend Tests also passed. These results execute the integrated code and deterministic provider fakes; they do not establish universal provider exactly-once delivery or live native recovery.

Full E2E and Frontend workflows passed on `10bf0f42dca2cf2d000349b45817ef21a36d3b6c`. The E2E result includes the responsive confirmation fixture corrected to inspect Tasks title geometry while that drawer is still open, plus the separate Delete dialog and final task-deletion assertions. All normal, container, Kubernetes and desktop jobs completed. The retention fixture correction in `22ce943e3113fe656b518a74692f86634ecd15d1` leaves retry production logic unchanged; its final full hosted run is pending when this record is written. Final delivery requires the actual latest-head workflow conclusions, fully paginated reviews/statuses, protected-main integration proof and a normal exact-head squash merge.

Remediation used no live automation, real provider or operator runtime. Local proofs were bounded synthetic capsules in the inspected non-root, network-isolated, read-only-root sandbox. Git hooks were bypassed under that containment policy; hosted static, formatting, localization and test checks provide the required equivalents. Historical handles lost after interruption have unknown exits, recorded outside the repository rather than inferred from later runs.


### Prepared-prompt integration on current main

Final-head `cdb5423a` completed all 69 checks (53 success, 16 skipped), including full Backend, Frontend and E2E workflows. The PostgreSQL 16 MVCC/monotonic fixture passed in its actual hosted job. Before merge, authoritative main advanced to `48d13207` (#4435), changing lifecycle/executor admission callbacks to receive prepared text and attachment descriptors and replacing the pre-start payload helper with identity binding. Static synthetic integration found an obsolete helper reference, callback type/call incompatibilities, and premature retry payload persistence. This is a shared launch-path interaction requiring remediation, rather than disjoint base drift.

Preserve the immutable task/session/turn/submission tuple and non-expiring claim before harness effects. Reuse main's prepared admission boundary: bind identity after the single-use retry admission, revalidate exact ownership before SQL payload admission, persist the final prepared bytes once, then transition the initial submission to dispatching. Acceptance receipts, control revocation, retained uncertainty, canonical identity and all existing budgets remain unchanged. Add actual retry-entrypoint coverage for transformed runtime text/descriptors, absence of premature SQL admission, final SQL payload/hash equality, immutable duplicate refusal and no dispatch after owner revocation. Broad integrated Go/race/PostgreSQL/UI verification remains hosted CI; bounded contained proofs must distinguish signature/setup failures from semantic RED.

The bounded exact-callback proof produced semantic RED with a valid pre-preparation persistence mutant, then GREEN with the prepared callback composition. It covers exact text/descriptors, immutable refusal, single-use admission, revoked/cancelled/foreign execution and storage failure. This capsule uses deterministic ownership and persistence seams, so it is not a full Go/SQL/runtime test. The permanent retry executor/repository regressions remain pending hosted execution. Explicit formatting/syntax and qualifier scans passed on all 163 changed Go paths; documentation catalog and specification validation passed. An initial import scan misidentified the runtime client package, and a later argument separator was parsed as a filename; both were tooling setup failures, corrected without changing production assertions.

Main `8d662b1f` was audited and integrated in the same batch. Its macOS process identity checks refuse ambiguous orphan signaling, while resumed generation-one delivery accepts only authenticated, retained, unacknowledged setup evidence through the normal SQL owner transaction. These incoming recovery boundaries are preserved alongside early retry-owned recovery exclusion. Final combined-head hosted checks remain the delivery gate.
