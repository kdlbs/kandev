# Journey data-efficiency implementation evidence

This file records implementation results separately from the completed journey
investigation in [`evidence.md`](evidence.md). The investigation and its seed,
captures, and measurements remain historical evidence and were not rewritten.

## Task 01: Completion-gate reader isolation

### Change and correctness evidence

`GetTaskCompletionGate` now reads through the repository's separate reader
handle. SQLite uses a native reader transaction. PostgreSQL uses a read-only,
repeatable-read transaction so the task, criteria, and evidence observations
remain one snapshot. Mutation-owned completion checks, their writer
transactions, and lock ordering are unchanged.

The held-writer tests failed before the production change: a standalone gate
read and a warm homepage/board snapshot did not finish before the two-second
barrier. After the change, both completed while a separate SQLite writer
transaction remained open. The gate error/cancellation lifecycle test also
passed. The PostgreSQL test inspected the actual transaction settings and
passed against disposable PostgreSQL 17.

Race-enabled SQLite, task-service, and backend-app tests passed:

```text
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/service ./internal/backendapp -run 'Test(CompletionGate|JourneyRead)' -count=1)
```

The PostgreSQL snapshot test passed:

```text
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN='<disposable PostgreSQL DSN>' go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run '^TestCompletionGateReadPostgres$' -count=1 -v)
```

### Matched route measurements

The two raw logs below are new implementation measurements. Both runs used the
same `BenchmarkJourneyReadLoad`, seeded route fixture, Go 1.26.0, and host. The
baseline ran before the gate-read change; the candidate ran after it. Both runs
include the separately completed message-update optimization. The idle route
contains 100 tasks. The write-loaded route contains one visible task and eight
concurrent writers alternating 16 MiB message bodies. Each workload has ten
warm samples. The fixture uses a fresh temporary SQLite database per sample.

| Workload | Samples | Baseline latency p50 / p95 | Candidate latency p50 / p95 | Response bytes, baseline / candidate (median) |
| --- | ---: | ---: | ---: | ---: |
| Idle, 100 tasks | 10 | 5.117 / 7.390 ms | 7.773 / 20.371 ms | 261,393 / 261,406 |
| Eight 16 MiB writers, one visible task | 10 | 858.458 / 2,898.051 ms | 1.482 / 1.987 ms | 9,608 / 9,609 |

All 20 candidate route requests completed and all background writers exited
without an error. The held-writer tests provide the structural isolation gate;
these timings do not replace it. No health-probe deadline or connection
wait/occupancy metric was collected by this route benchmark. The separate
integrated measurements in Task 08 own those signals.

The comparison ran on Linux x86_64, AMD Ryzen 5 7640HS, with 11 online logical
CPUs. Other Kandev and browser processes were active on the shared host during
the runs. Idle p95 was higher in the candidate run, and shared-host variance
limits absolute latency claims. The write-loaded route result is consistent
with moving inspection away from the single SQLite writer, but does not
establish a general availability guarantee.

Raw outputs:

- [`runs/01-gate-baseline.log`](runs/01-gate-baseline.log)
- [`runs/02-gate-candidate.log`](runs/02-gate-candidate.log)

## Task 02: Batched completion-gate summary observations

`GetTaskCompletionGateSummaries` reads unique task IDs in chunks of at most
100. Each chunk uses one native reader snapshot and no more than six data
queries for task identity, gate revision, criteria, pull-request heads, and
completed execution evidence. Immutable artifact revisions are checked
locally. The service passes each keyed observation to both existing-summary
comparison and missing-summary repair, so it does not repeat a standalone
gate read per task. Missing task IDs are explicit; an existing task with no
gate has a present nil observation.

The expected-red repository test started with no batch reader and failed as
expected. The completed fixture seeds 1,000 tasks with ten verified artifact
criteria each. The green result used ten reader transactions and at most sixty
data queries. A parity fixture compares all supported evidence behavior with
standalone gate snapshots, including stale evidence, missing tasks, and
tasks without a gate. The service test also started red because reconciliation
made zero batch calls. It then passed for 130 tasks: missing summaries
converged from one service batch read, and warm reconciliation kept stored
revisions unchanged with no published events.

Race-enabled SQLite, service, and backend-app tests passed:

```text
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/service ./internal/backendapp -run 'Test(CompletionGate|TaskStatusSummary|JourneyRead)' -count=1)
```

The PostgreSQL 17 parity test passed on the disposable test database:

```text
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN='<disposable PostgreSQL DSN>' go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run '^TestCompletionGateBatchPostgres$' -count=1 -v)
```

The documentation catalog, full specification lint, and `git diff --check`
passed. This order establishes bounded query structure rather than latency;
Task 08 owns the integrated measurements.

## Task 03: Narrow session summary projections

Navigation now reads a typed session observation that excludes metadata and
configuration snapshots. SQLite and PostgreSQL queries extract only the
visible agent/repository labels, the live executor label, and
`last_agent_error`; batch reads use the reader pool. The compact list adds
worktree associations with one batched read. Full selected-session and
mutation methods retain their complete model. Primary-session info and compact
session lists now share one observation batch in task/workflow enrichment and
boot.

The initial size comparison correctly rejected fixtures with different
session labels, exposing a test-fixture issue rather than payload leakage.
After aligning unrelated fields, a comparison of 4 KiB and 1 MiB metadata and
configuration padding passed with equal normalized projection bytes; both
encoded observations stayed below 16 KiB. The query-shape assertion rejects
selecting any full metadata or snapshot column. The full selected-session
control retained its 1 MiB metadata and profile snapshot. DTO coverage checks
all existing compact fields and keeps an explicit null for `last_agent_error`
so retained UI errors clear. Handler and boot coverage includes mixed states
and a task with a session but no primary.

The service projection tests were first run before the service capability was
implemented and failed on the missing methods; they passed after the bounded
repository/service path and fallback were added. Race-enabled SQLite,
service, handler, and backend-app coverage passed:

```text
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/backendapp -run 'Test(SessionSummaryProjection|TaskSummaryProjection|TaskStatusSummary|.*PendingAction|.*RunnerMutability|.*PrimarySession|.*StatusSummary.*Boot|JourneyRead)' -count=1)
```

The same large-padding projection and compact list assertions passed on
disposable PostgreSQL 17:

```text
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN='<disposable PostgreSQL 17 DSN>' go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run '^TestSessionSummaryProjectionPostgres$' -count=1 -v)
```

The documentation catalog, full specification lint, and `git diff --check`
passed. This order records a structural payload-size bound; route-level
latency and health comparisons remain assigned to Task 08.

## Task 04: Visible session detail demand

The initial Chromium capture showed eight rich subscriptions for eight mounted
sibling chats. Hook-level render tracing identified the remaining hidden-tab
consumer in `useUtilityAgentGenerator`, which called the session git-status
hook without the panel's `detailActive` value. The expected-red hook test
observed that omission; after propagation, the compact siblings retained one
rich stream for the visible chat.

The first keyboard-tab E2E attempt exposed a second gap. Dockview activated the
next panel, but session-tab synchronization restored the old selection because
the compact task-session row existed before its environment mapping. A new
resolver regression failed on that state before the change. The guard now
accepts a session row owned by the active task as sufficient evidence while
retaining the environment mapping as the early-event path. Deleted sessions
are still rejected because their task membership and environment mapping are
removed together; cross-task sessions remain rejected.

Verification passed:

```text
(cd apps/web && pnpm exec vitest run components/task/visible-session-demand.test.tsx components/task/dockview-session-tabs.test.ts components/task/dockview-session-tabs.hook.test.tsx hooks/domains/session/use-session-messages.test.ts lib/ws/handlers/session-pending-action.test.ts)
  5 files, 81 tests passed
(cd apps/web && pnpm exec vitest run hooks/use-utility-agent-generator.test.tsx hooks/domains/session/use-session-git-status.test.tsx components/task/visible-session-demand.test.tsx components/task/chat/chat-input-area.test.tsx)
  4 files, 49 tests passed
(cd apps/web && pnpm e2e:run --project chromium tests/session/visible-session-demand.spec.ts)
  1 test passed; one visible stream among eight tabs, keyboard selection, retained draft
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-visible-session-demand.spec.ts)
  1 test passed; phone picker released the prior stream and acquired the selected session
(cd apps/web && pnpm run typecheck)
  passed
(cd apps/web && pnpm exec eslint --max-warnings 0 <Task 04 changed source and test files>)
  passed
git diff --check
  passed
```

The E2E traces ran against the managed production bundle. They establish the
initial desktop and phone subscription behavior and draft/selection flow; the
integration E2E does not measure historical writer health or database latency.

## Task 05: Normalized, route-scoped boot

Boot version 2 stores unique task/session records in entity tables and uses ID
membership in initial state and route data. The browser still decodes version
1. A selected board boots only that snapshot; other board containers fetch
their own snapshots when needed. Task detail carries the requested authorized
session, compact siblings, a single saved-view sidebar page, and no complete
workflow snapshot. The phone workflow picker lists active-workspace workflows
even when their snapshots are not loaded; an unloaded task count is omitted
until its board is selected and read.

The first mobile E2E run caught that the picker only listed workflows with a
loaded snapshot. The regression failed in the new test before the fix. A
focused hook test then failed before and passed after including unloaded
workflows. The final managed production-build tests passed: Chromium, two tests
for selected/all-workflow boot and task detail; mobile-Chrome, one test for
single-board payload and on-demand selection. The mobile selector retains
unknown counts for unloaded snapshots rather than displaying zero.

Race-enabled backend boot tests passed:

```text
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/webapp ./internal/backendapp -run 'Test(Boot|.*Boot|HandlerInjects|DevHandler|Journey|NormalizeBootPayloadGraph|TaskDetailSidebarQuery)' -count=1)
  internal/webapp and internal/backendapp passed
```

The focused browser tests passed in five files (92 tests), web typecheck and
targeted ESLint passed, and the measurement parser's two v1/v2 tests passed.
The parser preserves the historical v1 data and emits boot version, normalized
entity counts, route membership counts, bytes, and response time for v2.

An isolated `dev-isolated --web` instance captured three samples per route.
It used the repository's synthetic seed only. At 1,000 tasks, the selected
workflow held 495 tasks; its boot was 1,025,986 bytes. Task detail held 101
task entities and one or eight session entities, depending on the target, and
measured 315,817–320,269 bytes. All were below the 1.25 MiB board and 512 KiB
detail limits.

The 10,000-task fixture appended 9,000 tasks to another workflow and retained
the selected board's 495-task membership. The selected-board payload remained
1,025,986 bytes with 495 task entities. Task detail retained 101 task entities
and one or eight session entities; payload sizes were 315,821–320,273 bytes.
The four-byte increase reflects changed aggregate page metadata, not additional
task/session records. SQLite backup `.kandev/diagnostics/journey-data-efficiency/seed-10000.db`
passed `integrity_check` and had no foreign-key violations. The owned isolated
backend and temporary directory were removed after the SQLite backup.

At 10,000 tasks, the three detail-route samples took 6.7–7.3 seconds for the
one-session task, 11.2–12.6 seconds for the eight-session task, and 8.3–9.1
seconds for its requested-session route. At 1,000 tasks, corresponding medians
were 230, 261, and 307 ms. This reproducible scale observation is not explained
by payload growth. Other build/test processes were active on the shared host,
so it is recorded as an unresolved route-latency observation for Task 08's
integrated comparison and query-source investigation. No availability or
latency improvement is claimed from the boot-size result alone.

Raw captures: [`boot-10-v2.json`](evidence/boot-10-v2.json),
[`boot-1000-v2.json`](evidence/boot-1000-v2.json), and
[`boot-10000-v2.json`](evidence/boot-10000-v2.json).


## Task 06: Message-window turn context

HTTP and WebSocket message listing accept `include_turns`. One reader snapshot
returns the requested message window, its referenced turns, the active turn,
and explicit coverage. Boot uses the same builder with 50 messages. Client cold
and reconnect reads keep their existing 100-message window. Older paging and
search merge covered turns without marking the full session history loaded.
The full-turn endpoint and older-backend fallback remain available.

The repository test expands unrelated history from 200 to 2,000 older turns.
The 50-message read still returns exactly 51 turns, including the active turn.
SQLite and disposable PostgreSQL 18 tests passed. Handler tests cover the
additive option and existing message/turn behavior. Client tests cover coverage,
legacy fallback, pagination, stale response protection, active-turn completion,
and authoritative reconnect repair. The core recovery callback now reuses the
message-window read instead of separately loading all turns.

Production-build desktop and phone tests passed. They check the 50-message boot,
100-message client recovery window, older-history paging, one covered turn in
the one-turn fixture, and complete access to the oldest message. The desktop
trace checks that no full-turn HTTP read occurs. A first test fixture had only
55 messages and could not exercise paging after a 100-message client read. The
final fixture contains 105 distinct messages. The recorder covers both WebSocket
initial reads and HTTP older-history requests.

## Task 07: Shared navigation and board reads

Task session lists and workflow snapshots now use store-scoped resource owners.
Route and hook consumers share an in-flight request and a bounded trailing
refresh. Final release cancels obsolete work. Another consumer keeps its lease.
Authorization/workspace generation changes retire old owners. Existing task
navigation tombstones, revision merges, saved views, boot-to-live repair, retry,
and archive behavior remain covered by deferred-response tests.

The required navigation, task-session, workflow snapshot, concurrent all-board,
and route-state tests passed. The final combined browser unit run passed in 45
files with 665 tests. The route recovery and startup tests are included. Desktop
and phone boot tests preserve selected-board loading and on-demand workflows.

## Task 08: Shared metadata and integrated evidence

Repository projections, workspace/workflow lists, user settings, CI options,
and MCP configuration share store-scoped owners. Resource keys preserve
`includeScripts` and `includeHidden`. Existing agent-list and environment owners
remain in use. Both route bootstraps and route enrichment use the same owners.
Route repository failures retain their existing immediate error handling;
compact picker consumers retain bounded transient retries.

A cancellation regression failed before and passed after synchronous final
release. Without it, a replacement consumer joined an obsolete request in
the microtask before release. Two-consumer tests prove that one departing panel
does not cancel the other panel's CI, repository, or MCP read. Tests also cover
different stores/projections, retired authorization generations, bounded
refreshes, retry, and settings overlays.

The integration rerun found two additional contract defects. A newly rebased
prompt hook subscribed for hidden panels; its detail visibility guard and
regression test now match the other rich subscribers. Compact sibling boot DTOs
copied into an interface before adding `pending_action` lost a pending
permission. The actual boot-response regression fails with the old copy in a
source overlay and passes with the final DTO. These fixes preserve visibility
and permission freshness required by this package.

The final traces in [desktop evidence](evidence/final-browser-desktop.json) and
[phone evidence](evidence/final-browser-phone.json) record resource counts by
actual authorization/workspace generation and response projection. The recorder
observes pending fetches until response headers or synchronous abort. Unit tests
cover the shared owner's longer response-processing lifetime. It does not count
an aborted request as active until Playwright later emits `requestfailed`.
All captured metadata resource peaks are one. No captured metadata response is
503. Four repeated selection generations each start at most one read per
resource. Real gateway reconnect retains the selected chat and draft. Desktop
also checks two visible split chats, collapse to one, eight mounted sibling
tabs, keyboard selection, responsive transitions, and old-stream unsubscribe.
Phone checks one stream through repeated picker navigation and reconnect.

### Bounded payloads and the scale regression

At 10,000 tasks, final boot captures carry 495 task entities for the selected
board and 101 for task detail. Detail has one or eight compact session entities.
The board payload is 1,025,986 bytes. Detail payloads are 265,602–270,306 bytes
with 50 messages and 25 referenced turns. All satisfy the 1.25 MiB board and
512 KiB task-detail gates. The original 1,000-task task-detail payload was
2.11–2.17 MB. The new wire graph removes duplicate and unrelated collections.

The Task 05 latency observation was repeatable and resolved here. Scratch-table
`ANALYZE` saw 10,000 empty parent IDs and estimated that parent lookups matched
the entire relation. SQLite then scanned the scratch table for every recursive
root. Candidate staging took about 94 ms, while page execution took 13.7 s in
the instrumented sample. A flat-forest query-plan test failed before the fix.
Default estimates now retain indexed parent lookups. No main-database index or
migration changed. PostgreSQL query behavior is unchanged. The fixed diagnostic
query completed in about 0.7 s. Native reader snapshot, scratch cleanup,
cancellation, queue hydration, and workspace isolation regressions passed.
The existing hierarchical 100,000-task benchmark passed too. It did not expose
the flat-tree planner choice.

The isolated production route captures in
[final scale evidence](evidence/boot-10000-verified.json) have task-detail medians
of 481, 456, and 380 ms. Before the planner fix, the same integrated fixture
took roughly 5.4–8.0 s. Home has a 190 ms median. Shared-host variance limits
these timing comparisons. The indexed query-plan regression is deterministic.

The [1,000-task capture](evidence/boot-1000-verified.json) and final scale capture
retain three samples per route. Selected-board membership stays at 495 tasks.
Detail stays at 101 task entities and one or eight compact session entities.
Each detail response carries 50 messages and 25 referenced turns. Additional
unrelated tasks add no boot entities. With three samples, nearest-rank p95 is
the largest sample.

| Route | 1,000 tasks: bytes / median / p95 (ms) | 10,000 tasks: bytes / median / p95 (ms) |
| --- | --- | --- |
| Selected board | 1,025,986 / 215.5 / 234.9 | 1,025,986 / 190.3 / 240.8 |
| Primary session | 265,601 / 438.4 / 469.7 | 265,602 / 481.1 / 537.4 |
| Eight-session task | 270,305 / 324.2 / 329.5 | 270,306 / 456.1 / 492.0 |
| Selected sibling | 270,305 / 322.0 / 323.2 | 270,306 / 379.7 / 390.8 |

### Matched route workload and remaining health limits

[Integrated benchmark data](evidence/integrated-benchmarks.json) retains ten
warm HTTP route samples per mode and the Task 01 baseline/candidate samples.
The fixture and eight 16 MiB message writers are unchanged. This fixture
measures the original board HTTP builder, rather than the complete browser
journey or route boot payload above. Nearest-rank p95 for ten samples is the
largest sample.

| HTTP fixture | Baseline p50 / p95 (ms) | Final p50 / p95 (ms) |
| --- | --- | --- |
| Idle, 100 tasks | 5.117 / 7.390 | 3.763 / 5.276 |
| Eight writers, one task | 858.458 / 2,898.051 | 1.460 / 3.244 |

A separate benchmark runs the actual persistence health checker concurrently
with route reads, checking two real required fixture tables. All ten idle
probes passed. Eight of ten final write-loaded probes hit their two-second
deadline, while all ten route reads succeeded (route median 1.426 ms). Their
writer-pool deltas sum to 148 waits and 149,286 aggregate waiter milliseconds.
These are totals across waiters, not one transaction's duration or identity.
An earlier corrected run had ten deadline failures. An initial invalid health
fixture referenced absent tables and is excluded from comparisons. Setup now
checks healthy persistence before starting writers. Calibration samples are
marked separately and excluded from the ten measured samples.

Warm isolated route captures completed with HTTP 200. Both fixture sizes passed
`/ready`, and persistence diagnostics reported `healthy`. The separate `/health`
response confirms liveness only. Retained checks are in
[10,000-task readiness](evidence/final-warm-health-http.json) and
[1,000-task readiness](evidence/final-warm-health-1000-http.json).
These observations do not establish
universal 503 recovery: saturated writer health can still reject admission.
Health checks, writer pool size, admission guards, and live configuration remain
unchanged. Historical incident ownership and the separate agentctl filesystem
failure remain unresolved.

### Reproduction and final verification

Run from the repository root. Each subshell owns its working directory:

```sh
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-turns-hydration.test.ts hooks/domains/session/use-session-turns.test.ts hooks/domains/session/use-session-messages.test.ts hooks/domains/session/use-session-messages.live-refresh.test.tsx hooks/domains/session/load-message-window.test.ts hooks/domains/session/older-message-pagination.test.ts lib/state/slices/session/turn-actions.test.ts lib/api/domains/session-api.test.ts lib/state/hydration/hydrator.test.ts)
(cd apps/web && pnpm exec vitest run lib/state/task-navigation-reads.test.ts lib/state/task-session-reads.test.ts hooks/use-task-sessions.test.ts hooks/use-workflow-snapshot.test.ts hooks/domains/kanban/use-all-workflow-snapshots-inflight.test.ts lib/ssr/session-page-state.test.ts)
(cd apps/web && pnpm exec vitest run hooks/journey-metadata-resources.test.ts hooks/domains/settings/agent-list-resource.test.ts hooks/domains/session/environment-live-resource.test.ts hooks/domains/github/use-task-ci-options.test.tsx hooks/domains/session/use-session-mcp.test.ts hooks/domains/workspace/use-repositories.test.ts hooks/use-ensure-user-settings.test.ts hooks/use-workflows.test.ts src/kanban-route-startup.test.tsx src/spa-routes.workspace.test.tsx)
(cd apps/web && pnpm e2e:run --host --project chromium tests/task/journey-boot-loading.spec.ts tests/session/visible-session-demand.spec.ts tests/session/message-turn-window.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-journey-boot-loading.spec.ts tests/session/mobile-visible-session-demand.spec.ts tests/session/mobile-message-turn-window.spec.ts)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/backendapp ./internal/webapp -run 'Test(MessageTurnWindow|.*ListMessages|.*ListTurns|.*CompletionGate|.*StatusSummary|.*SessionSummary|Boot|.*Boot|NormalizeBoot|WriterWorkloadObserver)' -count=1)
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN='<disposable PostgreSQL 18 DSN>' go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run '^(TestMessageTurnWindowPostgres|TestCompletionGateReadPostgres|TestCompletionGateBatchPostgres|TestSessionSummaryProjectionPostgres)$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run 'TestSidebar' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/backendapp -run '^$' -bench '^BenchmarkJourneyReadLoad$' -benchtime=1x -count=10)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/backendapp -run '^$' -bench '^BenchmarkJourneyReadHealthLoad$' -benchtime=10x -count=1)
(cd apps/web && pnpm run typecheck)
(cd apps/backend && make build)
(cd apps/backend && golangci-lint run ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/task/dto ./internal/backendapp ./internal/webapp --allow-serial-runners --new-from-rev=HEAD --timeout=5m)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node docs/plans/journey-data-efficiency/validate-package.cjs
node scripts/validate-public-docs.mjs
node --test scripts/validate-public-docs.test.mjs
python3 docs/plans/journey-data-efficiency/measure_boot_test.py
git diff --check
```

Task-defined suites and their adjacent regressions are included in the combined
665-test run. Targeted ESLint passed across all 120 changed TS/TSX files. TypeScript passed.
The i18n ratchet passed. Backend cleanup splits boot task detail/projection and summary
reconciliation into files below the configured limit. Targeted backend lint
passed after helper extraction. The final backend build and affected race suites
passed. PostgreSQL 18 parity and persistence health/admission regressions passed.
Raw run logs are in `runs/`. Normalized route, browser, and health evidence is retained above.
[Verification context](evidence/final-verification-context.json) records the source,
platform, and retained fixture fingerprints. Final lint helper extraction followed
the route captures. Boot and summary race tests passed again after extraction.
Reader wait and transaction occupancy were not measured by these route or health benchmarks.
Managed E2E runs build the backend, Vite assets, and fixture plugins afresh.
Follow-up capture runs use the guarded raw runner with those built artifacts.

Owned isolated runtimes, browsers, and diagnostic instrumentation were removed
after verification. Synthetic backup databases remain available for reproduction.
No commit, push, deployment, pool increase, or live-data mutation is included.


## Review remediation

A source review after handoff identified six correctness gaps. This follow-up
keeps the original evidence and its measured limitations intact.

Compact session lists now authorize the task before either repository branch.
Real scoped service, HTTP, and WebSocket tests cover foreign users and
organizations, owner access, internal callers, and missing tasks. Both narrow
and full-model fallback paths retain the prior denial behavior.

Client task entry reads lightweight workflow steps and its bounded sidebar page.
It never acquires a full board snapshot for enrichment. Partial task coverage
remains incomplete. Board consumers own snapshots for visible desktop lanes,
the 300 px adjacent preload region, and the focused phone board. Collapsed
lanes release demand. Disconnected observer callbacks cannot restore it.
Unloaded lanes retain placeholders so scrolling can activate them. Cold All
mode seeds the first displayed board using existing lightweight coverage.
Known-empty boards are skipped while saved column preferences remain respected.
The former independent `useKanbanData` snapshot owner was removed.

Window reconciliation retires demonstrably completed or retired active markers.
Authoritative null observations carry request-start identity, epoch, and row
freshness. Delayed null responses cannot clear newer WebSocket turns. HTTP
pagination, latest/gap reads, route enrichment, and hydration use those guards.

Compact sibling boot rows retain foreground activity, cancellation state and
revision, and parked state, epoch, and revision. They use the existing runtime
summary enrichers and do not load unrelated rich metadata.

Shared reads reject results after scope retirement, disposal, or final release,
even when transport ignores cancellation. Task-session consumers fence data,
error, and loading commits against retired hook ownership. A live consumer keeps
its attempt, and trailing refresh remains bounded.

The regression logs retain failing pre-fix checks. The browser control used
preserved pre-review artifacts with freshness checks deliberately disabled.
It reproduced a full workflow request after actual sidebar navigation and zero
cold All-mode snapshots. The navigation assertion waits for its enrichment
response before checking traffic. Sidebar budgets count task entries and exclude
group headers. The final browser runs use fresh managed production builds.

### Corrected-tree verification

All six findings are fixed. The combined frontend run passed 728 tests across
52 files. TypeScript passed. ESLint passed for all 140 changed TS/TSX files,
and the final browser-assertion edit passed separately. The i18n ratchet passed.
Focused worker suites retained pre-fix failures and passed after correction:
154 board/navigation tests and 260 turn/read-owner tests.

The affected five-package Go race run passed after two old All-mode boot
assertions were updated to the approved first-board seed. Real service and
HTTP/WS authorization regressions exercised both repository branches.
Runtime boot tests verified sibling fields and a compact payload below 32 KiB
with large unrelated metadata. Backend lint passed across six affected packages
with zero issues. The final `make build` passed.

Six desktop and five phone production-build browser cases passed. The first
managed desktop run built fresh artifacts. Its last case used a nonexistent
diagnostic attribute, so the test was corrected to inspect actual geometry,
collapsed content, and traffic. The corrected desktop rerun reused those fresh
frontend artifacts. The phone run rebuilt the final tree after the Go helper
extraction. Every E2E run used one isolated worker and disposed its fixture.

The [desktop capture](evidence/review-browser-desktop.json) and
[phone capture](evidence/review-browser-phone.json) retain the assertions and
JSON attachments. Actual SPA navigation between two tasks in a workflow with
205 unrelated tasks kept 100 task entities and incomplete board coverage.
Both devices issued zero complete workflow snapshot reads for task entry.
Sidebar responses contained at most 100 task entries. The twenty-workflow
cases exercised offscreen and collapsed desktop lanes, expansion after scroll,
phone focus changes, and a persisted desktop collapse preference on phone.
Cold All mode seeded one board. Phone subsequently requested only the newly
selected board in that case.

Documentation checks passed: 370 decisions and 1,521 specifications; all spec
lint; eight-work-order coverage preflight; 47 public documentation pages and
validator unit tests; two measurement-script unit tests; relative links and
owned source paths; changed-Go formatting; and `git diff --check`.
The manifest now checks Tasks 06–08 and records completed implementation.

[Corrected verification context](evidence/review-verification-context.json)
records 194 changed source files and their individual SHA-256 values. The
PR-delivery digest was
`6a7f7e590698816b71623ffe8dcb2f3a808753bd47fe9910e30dc1ead4c0c500`. The verification context now records the corrected PR fixup source separately.
The retained 1,000- and 10,000-task seed fingerprints are unchanged.
The earlier verification context and benchmark/route captures remain historical;
this follow-up did not regenerate their timing or health observations.

Representative final commands (run from the repository root):

```sh
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium tests/task/journey-boot-loading.spec.ts tests/task/journey-review-loading.spec.ts tests/session/visible-session-demand.spec.ts tests/session/message-turn-window.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-journey-boot-loading.spec.ts tests/task/mobile-journey-review-loading.spec.ts tests/session/mobile-visible-session-demand.spec.ts tests/session/mobile-message-turn-window.spec.ts)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/backendapp ./internal/webapp -run 'Test(MessageTurnWindow|.*ListMessages|.*ListTurns|.*CompletionGate|.*StatusSummary|.*SessionSummary|.*CompactSession.*|.*Session.*Authorization.*|Boot|.*Boot|NormalizeBoot|WriterWorkloadObserver)' -count=1)
(cd apps/backend && golangci-lint run ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/task/dto ./internal/backendapp ./internal/webapp --allow-serial-runners --new-from-rev=HEAD --timeout=5m)
(cd apps/backend && make build)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node docs/plans/journey-data-efficiency/validate-package.cjs
node scripts/validate-public-docs.mjs
node --test scripts/validate-public-docs.test.mjs
python3 docs/plans/journey-data-efficiency/measure_boot_test.py
git diff --check
```

Final logs are `runs/review-final-frontend.log`, `review-final-typecheck.log`,
`review-final-eslint.log`, `review-e2e-eslint-final.log`,
`review-final-i18n-ratchet.log`, `review-final-backend-race-corrected.log`,
`review-final-backend-lint-corrected.log`, `review-final-backend-build.log`,
`review-final-browser-desktop.log`, and `review-final-browser-phone.log`.
The shared host remains a limit for historical timing comparisons.
Eight of ten write-loaded probes reached their two-second deadline. Historical
writer ownership and separate agentctl filesystem failures remain unresolved.
This follow-up does not claim universal health recovery.

## PR delivery

After review correction, the user authorized commit, push, and PR publication.
Prettier normalized ten source/test files before delivery. Their emitted
JavaScript syntax is unchanged. The corrected verification context records
these formatted bytes; earlier runtime captures remain valid. Normal commit
hooks run without a bypass. No deployment or live data mutation is included.

Delivery hooks also exposed the approved lifecycle import baseline still pointing
to `boot_state.go`; its entry now follows the extracted task-detail boot file.
Formatted source pushed three functions to 101 lines. Redundant aliases were
removed from chat input/panel state, and unchanged TipTap editor/context markup
was extracted. Seven affected test files passed 141 tests. TypeScript and fresh desktop and phone
visible-chat browser cases passed before publication. Document whitespace from
previously untracked work orders was normalized.

## PR #4404 fixup

The review at `f68dc6bc4fa865e22890656f45bca61f143df361` identified four valid findings.
Task entity expansion now applies only to normalized boot collection/detail paths.
Saved selected-task views, drafts, and ordinary task identities retain their IDs.
A retained session consumer receives data or errors and settles loading after
its initiating hook unmounts. Retry remains available. Scope retirement and final
release still reject obsolete results. Queued refresh callers receive the last
attempt's failure instead of an earlier successful value.

The reproduction work order now labels the fixed default and the historical
`--restore-message-reservation` comparison. Both correctness controls passed.
CI's architecture check rejected the relocated runtime-import exception because
baselines may only shrink. Metadata decoding helpers now remain in the already
approved `boot_state.go` file; no new exception is added.

The branch was rebased onto main `d92d40b67bf3ecabf74072c95325f3f86cdd5e30`.
Both incoming commits remain intact, including profile catalogue and Quick Chat
recovery behavior. The rebase required no conflict resolution.

Four new regression cases failed before the fixes. After correction and rebase,
eight frontend files passed 114 tests, including existing Quick Chat recovery
coverage. Changed web files passed ESLint with zero warnings. TypeScript passed.
Two boot/webapp packages passed focused race tests. Backend `make build` and
frontend `pnpm run build:vite` passed. The CI-equivalent architecture command,
all specification lint, and documentation catalog checks passed.

Representative commands, from the repository root:

```sh
(cd apps/web && pnpm exec vitest run src/boot-payload.test.ts hooks/use-task-sessions.test.ts hooks/use-task-sessions.foreground.test.ts hooks/use-task-sessions.stale-reads.test.ts lib/state/task-session-reads.test.ts lib/state/task-navigation-reads.test.ts hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption-manual-recovery.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run build:vite)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/backendapp ./internal/webapp -run 'Test.*(Boot|TaskDetail|Normalize)' -count=1)
make -C apps/backend build
python3 scripts/lint-architecture.py --all --baseline-base-ref d92d40b67bf3ecabf74072c95325f3f86cdd5e30 --allow-missing-base-baseline
python3 docs/plans/database-writer-contention/measure.py -- go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run 'Test(UpdateMessage|CreateMessage|.*Payload|.*ConversationSource)' -count=1
python3 docs/plans/database-writer-contention/measure.py --restore-message-reservation -- go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run 'Test(UpdateMessage|CreateMessage|.*Payload|.*ConversationSource)' -count=1
```

The corrected source digest is recorded in
[verification context](evidence/review-verification-context.json).
Earlier browser captures and probe timings remain historical. These fixes only
change data normalization and request ownership, so shared unit/hook regressions
cover both viewport consumers without changing layout or touch interaction.
Eight of ten historical write-loaded probes exceeded two seconds. Historical
writer ownership and separate agentctl filesystem failures remain unresolved.
This fixup does not establish universal health recovery.

### Frontend CI fixture remediation

At `b381612f3ca8e19d454a60de318ae355fd924872`, frontend run `38058965767`
(job `114233211580`) completed with 18 failing assertions in four test files.
All reproduced locally. The lazy-message fixture omitted turn observation state
and the `include_turns` request field. The add-panel fixture omitted the shared
session-loading action. Recovery still modeled full turn-history replacement
instead of bounded window context. The stale-tab fixture retained a valid compact
session row, which now legitimately permits activation before environment enrichment.

Test-only corrections restore the fixture contracts. The tab tests separately
cover compact activation and a removed session absent from both membership and
its environment map. Recovery asserts message/turn-window hydration with no full
turn-list request. Four deliberate production mutations each failed their intended
assertion, then source was restored byte for byte. No production change was part
of this initial fixture-only phase. The subsequent browser CI phase below changes production loading paths.

The fixture-only phase combined suite passed 198 tests across 12 files, including the four
corrected fixtures, older-message pagination, turn actions, and the earlier PR
review regressions. Corrected fixtures passed ESLint with zero warnings and web
TypeScript passed. See [CI remediation context](evidence/pr-4404-ci-remediation.json)
for source provenance and mutation results. The verification context records
source/test bytes for the latest delivered remediation; the fixture-only digest is retained as historical provenance. Historical desktop/phone captures, persistence
probe outcomes, unknown writer ownership, and agentctl filesystem limitations
remain unchanged. Hosted CI remains a separate required gate.

```sh
(cd apps/web && pnpm exec vitest run src/boot-payload.test.ts hooks/use-task-sessions.test.ts hooks/use-task-sessions.foreground.test.ts hooks/use-task-sessions.stale-reads.test.ts lib/state/task-session-reads.test.ts lib/state/task-navigation-reads.test.ts hooks/use-lazy-load-messages.test.ts components/task/dockview-add-panel-items.test.tsx components/task/dockview-session-tab-sync.test.ts hooks/domains/session/use-session-recovery.test.tsx hooks/domains/session/older-message-pagination.test.ts lib/state/slices/session/turn-actions.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/use-lazy-load-messages.test.ts components/task/dockview-add-panel-items.test.tsx components/task/dockview-session-tab-sync.test.ts hooks/domains/session/use-session-recovery.test.tsx)
(cd apps/web && pnpm run typecheck)
```


## PR #4404 browser CI remediation

CI exposed missing task IDs in nested route boot state, actions that depended on complete board records, and absent metadata for offscreen workflow choices. The fixes retain bounded boot, sidebar paging, and visibility-based session demand.

Compact session reads retain only the current model ID/name beside existing status fields. The scalar SQL projection excludes unrelated model choices and configuration. Explicit E2E inspection of preparation, persisted settings, and recovery metadata uses full session detail. Production list reads remain compact.

Task detail consumes canonical task authority after a live workflow move. Route readiness uses authorized workflow metadata and keeps the phone navigator mounted while board tasks load. Sidebar actions use their bounded page records. Explicit task mention search requests one page of 50 tasks and rejects retired scope responses.

Step filter, detail, route, and destination-menu reads share the existing metadata resource owner. They do not acquire task snapshots for offscreen boards. New regressions prove metadata demand, authorization, obsolete-response rejection, and overlap deduplication. Empty placeholder snapshots do not count as loaded step metadata. A sibling added during an older membership request is hydrated by a direct follow-up before the request owner settles; repair does not depend on an additional React render.

The branch was rebased onto main `43f55a6aad38c7f53c4b7eb56682016639f74405`. The Quick Chat conflict retains upstream journal-retirement verification and explicit full-session metadata inspection. The incoming read-only Resume fixture uses the same full-detail inspection while preserving its recovery and restart assertions. Main subsequently advanced to `8d2246e323eef5764e428b73b948818d5437e1cc` with browser-native demo support. The final rebase was conflict-free and preserved all 78 tracked and eleven untracked fixup files byte for byte. The new base passed 125 focused integration tests in thirteen files, including boot routing, task status, Quick Terminal reconciliation, and browser-demo selection. Backend source did not change in this base advancement. Final-base TypeScript and the production Vite build passed. A normal commit hook rejected the compact scanner at 84 lines against its 80-line limit. Extracting the existing metadata decoding into a helper preserved its projection and error behavior; race-enabled SQLite summary regressions passed afterward. No hook was bypassed.

The recovered phone follow-up exposed intermittent dropped taps. Trace and passive listeners changed timing, so their passing samples did not retire the failure. A real-tooltip regression demonstrated that readiness changes remounted the phone submit target. Phone submit controls now keep a stable target without a desktop keyboard-tooltip wrapper; disabled state and accessible reasons remain intact. Desktop hints remain unchanged. Final phone verification uses ordinary taps without diagnostic listeners or trace snapshots.

Public documentation was inspected with the docs-maintainer skill. These fixes preserve existing controls, navigation, and mutation semantics. The public WebSocket API reference now documents compact session lists and the full-detail endpoint. The journey system design clarifies the projection; no new public setting or workflow is introduced.

Local focused validation passed: 84 composer/draft tests in five files, 37 membership/cancellation tests in four files, earlier bounded-navigation/resource regressions, changed-file ESLint, TypeScript, architecture budgets, four backend race packages, two exact compact boot runtime regressions, and three PostgreSQL 18 parity regressions. Documentation catalog, all specification lint, and eight-work-order coverage passed. The owned PostgreSQL fixture was removed.

Browser results preserve each failed phase: the broad desktop run passed 48 of 50 before the placeholder metadata fix; all 15 final desktop path cases subsequently passed. The broad phone run passed 35 of 37; membership repair and the stable touch target address its two failures. Three container Sources cases and focused sidebar filter/navigation checks passed. After rebasing, Quick Chat and both live workflow moves passed. The incoming read-only fixture was corrected, and its full desktop and phone flows passed. Three final recovered-follow-up phone samples passed without instrumentation, trace snapshots, or retries.

The initial full frontend run was stopped after a known corrected fixture failure to rebase; it is not counted as passing. The fresh two-worker run exposed eleven stale-mock failures in the task-list step and automation-selector fixtures. Both omitted the optional store exports now used by shared metadata reads. All thirteen original tests in those two files pass after correcting the mock interface, without changing their assertions. The destination-menu regression also retained a pre-sharing request signature. Its corrected assertion preserves the exact single-request count and now proves that closing the menu aborts the request. All fourteen tests in the three discovered fixture files pass. The complete run then finished with 2,831 passing files and the same twelve failures in those three files (25,343 passed tests and four skipped). It is a failed pre-correction run, not an all-green result. The full selection preceded the final fixture edits and stable-target regression; corrected focused tests cover those later changes. Hosted CI on the remediation head remains a separate pending gate until the fixup is pushed. The verification context records the latest source bytes, distinct from historical captures.

The historical measurement limits still apply: eight of ten write-loaded probes exceeded two seconds. Historical writer ownership and separate agentctl filesystem failures remain unresolved. No universal health recovery is claimed.


### Shared step metadata revision repair

An explicit preview refresh or workflow-step notification could join a pre-revision read retained by another consumer. Both producers now request one trailing shared refresh. The requesting control receives the newer observation while an unchanged consumer keeps its original request binding. Requests remain sequential.

Two deferred-response regressions use a real store; the notification case exercises the registered WebSocket handler. Final tests fail on both original producer implementations, then pass after restoration of the corrected source. The six-file metadata/resource selection passed 33 tests. Changed-file ESLint, formatting, and TypeScript passed. The earlier CI snapshot belongs to the preceding remediation head; final hosted checks and review remain pending until the follow-up is delivered and verified. Historical health limitations are unchanged. The follow-up rebased without conflicts onto main `3fb026365239bbc893fa4a5272d926640c7ef1f9`; the metadata selection passed again, and race-enabled lifecycle checks for incoming MCP ownership and restored-execution guards passed.

```sh
(cd apps/web && pnpm exec vitest run components/task/sidebar-filter/use-filter-value-options-demand.test.tsx hooks/domains/kanban/use-workflow-step-metadata-demand.test.tsx hooks/use-workflow-option-previews.test.ts hooks/use-task-list-workflow-steps.test.ts hooks/domains/kanban/use-workflow-steps-by-id.test.ts hooks/journey-metadata-resources.test.ts)
```

### Draft submit synchronization repair

A fresh phone recovery check on the committed metadata follow-up failed at the second submit: the draft remained visible and only one user message was persisted. This supersedes any claim that the stable touch target alone retired the submission problem. The failed run is retained at `/tmp/kandev-run.e2e.MYI63uSC.log`.

A deterministic regression using the actual input hook reproduces an independent ordering defect. An editor change during layout updates the synchronous submit snapshot, but the previous render's passive effect overwrites it before the next layout submits. The handler observes an empty draft and returns without calling its submitter. Synchronizing the snapshot in the layout phase preserves the entered text. The regression fails with zero submit calls on the original source and passes with exactly one call carrying the draft after correction. Existing draft retention, session ownership, attachment, and toolbar guards remain covered by 57 passing tests in four files; TypeScript and changed-file ESLint passed. The first fresh phone recovery flow passed, but the following three ordinary samples produced one pass and two failures. They remain failed evidence, not a green sample set.

This regression proves the input ordering defect; it does not establish which browser event path caused every historical missed tap. Historical health limits remain unchanged: eight of ten write-loaded probes exceeded two seconds, writer ownership is unknown, and separate agentctl filesystem failures remain unresolved.

Temporary diagnostics then captured the native click, the exact follow-up draft, and the authoritative rejection: `Agent is currently processing`. The input mode observation was `WAITING_FOR_INPUT`; no click was lost in those diagnostic failures. The fixture reloaded after the first user message persisted without proving that its prompt had completed. Its idle check could accept the initial waiting observation while the accepted prompt was still entering/running its turn. The fixture now waits for `completed_at` on the turn linked by `turn_id` to that exact first message, then retains the reload, idle check, ordinary follow-up tap, and both-message persistence assertions. It neither retries the send nor weakens the running-session guard. All temporary instrumentation was restored byte-for-byte. All three fresh ordinary phone samples passed with retries disabled after this causal completion gate. Both fresh desktop controls passed: recovered first-brief follow-up and read-only Resume, with retries disabled. This supersedes attribution of those browser failures to a lost touch event; the independently reproduced draft-effect ordering defect remains a valid correction.

Final ordinary browser selection: three phone recovery flows and two desktop controls passed with retries disabled. The managed runs rebuilt production assets and isolated backend/plugin fixtures; all owned instances were stopped by runner cleanup. Logs: `/tmp/kandev-run.e2e.qn1Cn4dQ.log` (phone) and `/tmp/kandev-run.e2e.fdnLFKKd.log` (desktop). Earlier `ZHXiidmg` and `MYI63uSC` failures and instrumented `fx3EgyeD`/`CXHmgJmZ` samples remain diagnostic/historical evidence. Hosted checks remain pending until delivery and exact-head verification.

```sh
(cd apps/web && pnpm exec vitest run components/task/chat/use-chat-input-draft-order.test.ts components/task/chat/use-chat-input-state.test.ts components/task/chat/chat-input-toolbar.test.tsx components/task/chat/chat-input-toolbar-composer.test.tsx)
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/chat/mobile-initial-task-brief.spec.ts -- --grep 'keeps the first brief after prompt-free recovery' --repeat-each=3 --retries=0
pnpm --dir apps/web e2e:run --host --project chromium tests/session/read-only-resume.spec.ts tests/chat/initial-task-brief.spec.ts -- --grep 'Resume works after read-only|keeps the first brief after prompt-free recovery' --retries=0
```

### Final hosted E2E remediation

The preceding remote head's E2E workflow `38068936135` reported four failed shards (`114263234544`, `114263234374`, `114263234437`, and `114263234545`). Full job logs identify five failed cases: hidden transcript catch-up; created-session runtime inheritance; two workflow-peer parking checks; and phone cascade archive navigation. Four desktop cases reproduced locally with retries disabled before correction. The phone case passed in the first local reproduction, while a focused query assertion deterministically reproduced its ordering defect.

The parking and runtime fixtures now request full session detail when inspecting rich metadata. Compact membership remains unchanged. The transcript fixture proves the hidden message is persisted without entering the inactive cache, then retains its activation, newest-content, bottom-following, and reader-position assertions. It no longer requires a rich hidden-session stream.

The archive fallback queried newest activity after bounded route boot exhausted its cached candidates; it could choose the later task instead of the canonical board list's earlier survivor. The bounded page now uses ascending creation order, preserving recent-use priority, workspace restrictions, liveness/ancestry checks, and the 100-row limit. The request-order regression fails on the original sort; 44 removal/coordinator/action tests pass after correction. Final ordinary desktop/phone E2E checks and exact-head hosted results remain pending.

Two more terminal shards (`114263234356` and `114263234510`, same workflow/head) exposed seven additional stale-fixture failures. Both clarification fixtures waited for a full-history HTTP turns request that the window-based loader no longer makes. They now wait for the actual `message.list` response and assert that both seeded turn IDs are present in its `turns` context before exercising the unchanged clarification overlay/selection checks. Five model-selector persistence/error fixtures inspected rich ACP/runtime configuration on compact lists; their fixture reads now request full session detail. The nine-case selection reproduced those seven failures, then all nine passed with retries disabled after correction.

The first four corrected desktop cases also passed with retries disabled. Three intermediate phone archive samples reached the correct task, but failed the stale exact-URL assertion because a valid `sessionId` query was retained. The fixture now compares the exact pathname, preserving the expected task identity and the later SPA navigation/no-document-reload checks. All three final phone archive samples passed with retries disabled, including the later SPA selection and no-document-reload guards. Hosted delivery remains pending; failed intermediate samples are retained separately.

Final local hosted-failure correction checks: 13 desktop cases and three independent phone archive samples passed with retries disabled, using fresh production assets and isolated fixtures. The preceding recovered-brief correction separately passed three phone flows and two desktop controls. Forty-four removal/coordinator/action unit tests, TypeScript, changed-file ESLint, documentation catalog (371 decisions/1,524 specifications), and all specification lint passed. Final hosted CI is still a separate required gate. Commands:

```sh
pnpm --dir apps/web e2e:run --host --project chromium tests/workflow/workflow-peer-resume.spec.ts tests/task/subtask.spec.ts tests/chat/auto-scroll-toggle.spec.ts -- --grep 'review peer message resumes implement|opening parked implement resumes|a changed second session creates|inactive session transcript reopens' --retries=0
pnpm --dir apps/web e2e:run --host --project chromium tests/chat/clarification-lifecycle-turn-shadow.spec.ts tests/chat/model-selector-error.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/task/mobile-archive-task-redirect.spec.ts -- --repeat-each=3 --retries=0
(cd apps/web && pnpm exec vitest run hooks/use-task-removal.test.ts hooks/use-task-removal-coordinator.test.ts hooks/use-task-actions.test.ts)
```

Historical limits remain unchanged: eight of ten write-loaded probes exceeded two seconds; historical writer ownership and separate agentctl filesystem failures remain unresolved. No universal health recovery is claimed.

The seventh shard (`114263234422`) was cancelled at its 45-minute job limit and produced no final blob report. Its progress log nevertheless contains two failed cases before cancellation. A read-only catalog mapped them to inactive sibling transcript reconciliation and restored Quick Chat model-catalog inspection. Both reproduced locally with retries disabled: the former exhausted its existing three-minute limit while expecting a hidden rich stream; the latter could not obtain rich metadata from the compact membership list. These failures are not classified as external timeout noise.

The sibling regression now proves hidden message persistence and absence from the inactive cache, restores the disjoint cached window, activates the receiver, and then exercises foreground reconciliation and pagination to the older attributed sibling prompt. The restart fixture reads the exact session detail for its durable model catalog, preserving the existing restart, configuration, and resume assertions. Both passed without retries (`/tmp/kandev-run.e2e.3JuY9O09.log`); both preceding failures are retained at `/tmp/kandev-run.e2e.FTvCrkso.log`. Changed-file ESLint passed. Across the seven affected shards, 14 observed failed cases now have local correction evidence; the corrected desktop selections total 15 passing cases, alongside three independent phone archive samples. The complete final hosted run remains required after delivery.

```sh
pnpm --dir apps/web e2e:run --host --project chromium tests/chat/inactive-session-transcript-reconciliation.spec.ts tests/chat/quick-chat.spec.ts -- --grep 'reaches an attributed sibling prompt|resumes a restored session after backend restart' --retries=0
```
