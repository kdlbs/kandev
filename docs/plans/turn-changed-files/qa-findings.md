# Turn changed-files QA

PR: https://github.com/kdlbs/kandev/pull/4332
Starting head: `aabbae4074`. Date: 2026-10-08.

## Todo

- [x] Understand intent and compare t3code UI/UX.
- [x] Trace capture, persistence, history, and UI wiring.
- [x] Run desktop and mobile happy paths and inspect screenshots.
- [x] Exercise boundary/error states, keyboard/touch navigation, and performance.
- [x] Record reproducible findings and missing test coverage.
- [x] Fix confirmed findings and run focused regression checks.
- [x] Push changes to PR 4332.
- [x] Wait 15 minutes after push, then start PR fixup against the current head.
- [ ] Confirm terminal CI and complete current-head review disposition.
- [x] Record verified behavior and remaining qualification limits.

## Findings

Eleven confirmed findings were fixed and covered by focused regressions. The fixes are pushed. Current-head PR checks remain pending.

## Reference comparison

Inspected t3code at the feature's pinned revision [611132c](https://github.com/pingdotgg/t3code/tree/611132c171f3a821bd2e32f22261135cef6330ac).
Sources: `ChangedFilesTree.tsx`, `MessagesTimeline.tsx`, `DiffPanel.tsx`, and `turnDiffTree.ts`.
The card follows the assistant reply. Folder actions expand the tree; file actions select the turn diff.
The reference uses compact rows, colored counts, persistent expansion, and a folder-only expand action.
Kandev preserves that interaction and adds retained content, checkout identity, and explicit unavailable states.
This is source-based reference inspection; no claim is made that the t3code app was run.

## Confirmed findings

- [x] QA-01, high: A copied-file patch includes the independently modified source file. A real-Git regression reproduces two diff headers in one file export.
- [x] QA-02, high: One failed checkout discards healthy sibling changes. A coordinator regression reports zero files without comparing the healthy checkout.
- [x] QA-03, high: Late file-list requests can overwrite a newly selected turn. Test initial errors, pagination completion, and empty repository transitions.
- [x] QA-04, medium: File-list failures lack recovery. The viewer can show an empty result instead of the request error.
- [x] QA-05, medium: Phone layouts with a fine pointer render 28px card actions. The required minimum is 44px.
- [x] QA-06, medium: File choices expose raw checkout UUIDs and the header exposes an RFC3339 timestamp. These values crowd the review controls.
- [x] QA-07, medium: Deep renamed paths lose useful detail on phones. Preserve readable paths and rename context in the selected-file view.
- [x] QA-08, low: A single root file shows “Collapse all” even though there are no folders.

Initial visual evidence: `/tmp/turn-qa-desktop-before.png`, `/tmp/turn-qa-diff-before.png`, `/tmp/turn-qa-deep-phone-before.png`.
The 390px fine-pointer probe measured every card action at 28px.
- [x] QA-09, high: Old checkpoint cleanup ignores the admission deadline. A 20ms deadline waits five seconds for one cleanup request.
- [x] QA-10, high: Comparison reads current `.gitattributes`. Later attributes change the binary classification of already captured endpoints.

## Validation notes

- Browser fixtures run an isolated production build with a local Git repository and mock agent.
- Existing two-turn/reload coverage checks real captured content after a later commit.
- Renamed-path layout coverage changes the file-list response only. It tests presentation, not Git rename capture.
- A simulated HTTP 503 reproduces file-list failure. The new viewer retry test passes.
- Tree/card unit checks: 10 passed. Request-race/retry hook checks: 9 passed.
- Web typecheck and localization checks passed after the UI fixes.
- All seven focused PostgreSQL turn-change contracts passed under the race detector using a disposable PostgreSQL 16 container.
- One browser failure used a five-second DOM wait before capture completion. The test now waits for the captured turn first.
- Another test reused an unchanged fixture file and correctly produced no card. The fixture now resets that file before each new scenario.

## Remaining qualification

Work order 09 remains open. Kubernetes, Sprites, and plugin executors still need feature-specific qualification.
The local Git benchmark does not qualify remote transport, cold/slow storage, or database-retention growth.
Deadline regressions use a controlled clock; they do not measure physical slow-storage performance.
- [x] QA-11, medium: Phone history repeats the Close action and stacks the drawer title above its close control. Keep one close action in the drawer header.

Mobile composition stays in the existing full-height `MobileDiffSheet`.
The header owns dismissal; the history controls own turn/file selection; the diff body owns scrolling and bottom safe-area clearance.
The selected-file details expose full paths without requiring hover. Desktop retains its docked viewer and close action.

The revised desktop run passed all four tests: two-turn persistence, HTTP failure recovery, narrow fine-pointer geometry, and saved preference.
Screenshots confirm that the new date and repository labels fit the desktop controls.

## Measured review performance

Production Vite build, isolated local Go backend, SQLite, Chromium, warm cache, one captured text file.
Each result uses ten samples after one warmup. P95 is the maximum for this small sample.
Timing includes Playwright request/interaction overhead. Other QA processes ran on the host.

| Operation | Median | P95 / maximum | Response bytes |
| --- | ---: | ---: | ---: |
| History API | 4 ms | 7 ms | 1,046 |
| File-list API | 3 ms | 5 ms | 352 |
| Retained patch API | 2 ms | 3 ms | 438 |
| Open diff until captured text renders | 92 ms | 155 ms | n/a |

A separate synthetic 250-entry file-list probe measured 160ms to expand the first 100 files and 292ms to load the next page.
Document horizontal overflow was zero. This probes UI pagination, not 250-file Git capture or transport.
Raw local evidence: `/tmp/turn-qa-browser-performance.log` and `/tmp/turn-qa-large-performance.log`.

Ten additional real mock-agent turns alternated the same text file between two contents.
All eleven captured turns reached `ready`. Submit-to-finalized-capture times ranged from 787ms to 3,386ms, including mock-agent work and UI submission.
This is an observed end-to-end local scenario, not isolated capture overhead. Backend builds ran concurrently.
Retained content grew from two unique blobs (227 compressed bytes, three links) to five unique blobs (656 compressed bytes, 43 links).
This confirms reuse across the repeated tiny-file scenario. It does not qualify large-content retention or disk-budget eviction.

## Fix and coverage record

- QA-01: Select only the requested file’s patch section. Real-Git copy fixtures include a modified source and quoted names.
- QA-02: Persist checkout-level endpoint failure and process healthy siblings. Regression tests cover partial totals and retry ownership.
- QA-03: Fence file-list requests by generation. Nine hook tests cover races, pagination, partial errors, and cleared selection.
- QA-04: Both the card and viewer show file-list failures with Retry. The HTTP 503 browser scenario verifies recovery.
- QA-05: Phone-width and coarse-pointer controls use 44px targets. Narrow fine-pointer and Pixel 5 tests measure the controls.
- QA-06: Format timestamps for the active locale. Show repository names with numeric disambiguation only when names repeat.
- QA-07: Keep rename context in the card and show full wrapped paths in the viewer. Desktop-width and touch tests cover disclosure.
- QA-08: Hide expansion controls when one checkout has no folders. The original one-file E2E now asserts this.
- QA-09: Share the admission deadline and aggregate cleanup budget. Controlled-clock race tests prove the short deadline.
- QA-11: The phone drawer owns its single Close action. Settled geometry measures 390 × 844px with no document overflow.

Focused frontend validation: six Vitest files, 36 tests passed. Four desktop and three mobile browser scenarios passed.
Light/dark phone views and the desktop viewer were inspected visually. Final checkpoint race tests, changed-package lint, SQL guard, and the full checkpoint group on actual Git 2.39.5 passed.
Public guidance now explains full-path disclosure and file-list retry.

- QA-10: Compare and export through one disposable Git view per request, with captured attributes and a private index. Later worktree, repository, and user attributes cannot change historical classification. Git 2.39.5 compatibility includes SHA-256 repositories.

Final desktop rebuild: four scenarios passed in 43.1 seconds.

Ten-observation local checkpoint benchmarks after the attribute fix:

| Fixture | Mean before | Mean after | Median after | P95 after |
| --- | ---: | ---: | ---: | ---: |
| 100 files | 270.43 ms | 166.88 ms | 163.26 ms | 195.33 ms |
| 20,000 files | 1,207.57 ms | 1,229.57 ms | 1,198.67 ms | 1,704.99 ms |

Shared-host load differed. These observations do not establish a causal speedup.
Raw logs: `/tmp/turn-attributes-benchmark-before.log` and `/tmp/turn-attributes-benchmark-after.log`.

## Delivery and executor recheck

The QA fixes were committed with normal hooks and pushed to PR 4332. Six refreshed desktop/mobile screenshots were published in the PR description.

Docker and SSH retention E2Es passed on the pushed code (two tests, 1.4 minutes). Docker content remained readable after container removal and backend restart. SSH content remained readable after deleting the remote checkout.

The first executor attempt stopped on a stale Linux mock-agent artifact. The next attempt failed before capture because the backend and remote-helper manifest had different build identities. Rebuilding the backend and Linux helpers together resolved setup; the subsequent run passed without retries.

Command: `KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e:run --host --no-build --project containers e2e/tests/git/turn-changed-files-executors.spec.ts e2e/tests/ssh/turn-changed-files.spec.ts -- --retries=0`. Raw success log: `/tmp/kandev-run.e2e.dPAxKMoh.log`.

The full post-push hold ran from 20:53:37 UTC to 21:08:37 UTC before PR fixup started. The initial fixup snapshot found no failed checks or unresolved inline threads, but CI remained queued. A sampled job had `runner_id=0` and no steps. GitHub API rate limiting temporarily blocked complete policy/review evidence; the standard waiter restarted after the quota reset. Terminal current-head CI and review completion remain pending until verified.

A disposable synthetic merge checked the QA code against base `5aa06bcefe02b6ce7b7c321f7092efe89f0c8992`. It merged without conflicts. Focused checkpoint/lifecycle/boot-state tests, the coordinator package, six frontend files (36 tests), localization, and public-doc validation passed. This is compatibility evidence for those exact inputs, not a completed CI verdict.

## Conflict and CI follow-up (2026-10-09)

- [x] Inspect current-head CI and review evidence. The earlier head had one failing leaf job: Windows process tests.
- [x] Reconcile upstream prompt delivery, retained runtime failure, canonical transcript, schema, and Git security changes.
- [x] Replace Windows-invalid quoted-filename coverage with a portable Unicode filename; retain the control-character case on Unix.
- [x] Add failing regressions for canonical reply anchors and capture finalization on retained failed turns, then implement the fixes.
- [x] Verify the merged code locally.
- [ ] Push and confirm terminal current-head CI/reviews.

The Windows failure occurred when the fixture tried to create a filename containing a tab and double quote. The portable Unicode case still requires Git path quoting. No product behavior was relaxed to pass the test.

Conflict resolution preserves both turn-change tables and upstream delivery/continuity tables. Stream disconnects retain the upstream uncertain-delivery and cancellation guards, while definitive failure captures the end interval before publishing the error. Prompt dispatch retains its delivery submission identity and capture-failure callback.

Post-merge validation passed: focused race tests for lifecycle, checkpoint processing, change coordination, Git security, and the required-store catalog; the full lifecycle, change-coordinator, backendapp, and requiredstores packages; focused SQLite turn-change tests; six frontend files (36 tests); typecheck; localization; specification/catalog/public-doc checks; four desktop and three mobile E2Es on rebuilt assets. Windows execution remains a required remote CI check.

The first post-merge CI run exposed a fixture inventory mismatch: upstream now lists missing tables individually, and the v0.93.0 manifest omitted the six turn-history tables. The exact SQLite upgrade assertion failed locally before the manifest correction. Afterwards the full SQLite conformance suite passed under `-race`, and upgrade/manifest tests passed with a disposable PostgreSQL 16 DSN. The archived SQL fixtures were not changed; their missing-table metadata now matches the combined required-store catalog.

- [x] Reproduce storage and mobile FIFO failures without retries and fix successor admission across the terminal capture fence.
- [ ] Verify the complete failed E2E shards and all additional failures from the remaining remote jobs.

The post-merge E2E run exposed a capture-admission race. Completion published readiness while the immutable endpoint still held its generation fence. Immediate follow-ups received a settlement error, which durable delivery classified as an unknown outcome. Prompt admission now waits for capture completion before claiming its generation. Cancellation, shutdown, and the bounded wait preserve the existing fence. The admission regression failed before this fix and passed afterwards; the cancellation cases and repeated race checks also passed.

The storage cleanup flow passed three repeated runs after the admission fix. Clarification follow-up, mobile cancellation, and mobile animation also passed without retries. The ordered shard passed workflow lifecycle/cascade and mobile FIFO/Auto-run cases that had failed remotely. Repository-set tests also passed in order; the dot-only CI output did not preserve test names.

The ordered run exposed a separate tool-completion race. Saved diagnostic state showed completed tools on the server and pending tools in the browser. SQLite completion used `CURRENT_TIMESTAMP`, truncating the update to whole seconds and making it older than the earlier live message. The browser correctly retained what appeared to be the newer pending row. Completion now writes a precise UTC timestamp through a bound parameter. The repository timestamp regression failed before the correction. Temporary browser instrumentation was removed after saving the evidence.

A separate CI flake came from persisted hidden board columns. Running the preview hidden-column scenario immediately before the session-dialog cancellation scenario reproduced the missing-card assertion. The shared per-test settings reset now clears `kanban_hidden_step_ids`, preserving each test's own column controls while removing cross-test state.

Post-fix evidence: full lifecycle package passed; capture-admission cancellation/race tests passed ten repetitions; SQL timestamp/receipt regressions passed ten race repetitions; SQL portability guard, Go lint, frontend fixture lint, and typecheck passed. The complete first failed shard ran without retries: 285 passed, two skipped, and one tool-spinner failure used to identify the timestamp defect. After that correction the tool-spinner scenario passed three repetitions, followed by ten consecutive runs without retries. The hidden-column/session-dialog sequence passed three repetitions after resetting preferences. All named late CI failures exercised so far (desktop/mobile queued cancellation, auto-advance, autopilot question delivery, desktop/mobile manual start, mobile canvas publication, and mobile queue reordering) passed in focused runs without retries. The other two timed-out shards are being replayed in full. Terminal current-head CI remains pending.

The mobile inspection-contention test reproduced a stop/readiness race in its setup. `session.stop` persists cancellation before detached runtime teardown ends. Reloading during teardown can truthfully report the old execution as running and skip automatic workspace restoration. Both viewport tests now poll the real status API for a stopped, restorable workspace before reloading. The original restore-request, retry, conversation-identity, and draft assertions remain unchanged. Five consecutive mobile runs passed without retries.

Desktop recovery and Pierre diff reading/refresh continuity each passed three repetitions without retries. The diff failure did not reproduce in the isolated run or these repetitions; no renderer change was made. The fresh-head CI run remains the remote acceptance check.
