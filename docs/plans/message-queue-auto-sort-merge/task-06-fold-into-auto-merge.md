---
id: "06-fold-into-auto-merge"
title: "Design revision: fold into Auto-merge"
status: done
wave: 5
depends_on:
  - "05-e2e-docs-spec-reconciliation"
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.6
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.10
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.13
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.16
system_design:
  - ../../specs/ui/system-design/message-queue-auto-sort-merge.md
---

# Task 06: Design revision: fold into Auto-merge

## Summary

Implement the maintainer/user decision for issue #5086: sorted folding is part
of effective Auto-merge, which remains ON by default. Remove the separate
Auto-sort-merge policy completely. This work order supersedes the separate
settings, policy, controls, and dependency contracts in work orders 01-05.

## Scope and ownership

- Backend queue admission, policy persistence, compaction, status producers,
  settings, startup wiring, and focused memory/SQLite/PostgreSQL tests.
- Frontend queue API, state ordering, hooks, settings drafts, two-pill header,
  localized copy, and desktop/mobile browser scenarios.
- Existing requirements, system designs, plan, and public queue/wire docs.
- Keep ordering/index, tied predecessor, keyset walk, edit-lease lock order and
  snapshot, management fence, and compaction accumulator. Keep ordinary
  `auto_merge.go` unchanged from `origin/main`.
- Do not change plugin SDK release metadata, stash entries, push, or create PRs.

## Acceptance

- Default Auto-merge folds agent messages into their latest compatible pending
  same-sender entry across intervening rows, respecting barriers and leases.
- Compatible adjacent messages still merge under the unchanged predicate.
  User and sender-less sources never move. OFF disables both fold paths.
- Only session effective OFF-to-ON writes compact pending rows atomically.
  Global changes, ON-to-ON and OFF writes never compact.
- No separate sort setting, override columns, policy, action, status tuple,
  frontend control, dependent draft, or exclusive locale keys remain.
- Existing Auto-merge override resilience and status ordering are retained.

## Mobile composition and ASCII preview

Reuse the shipped queue panel and its Auto-run/Auto-merge pills. Remove the
third pill and its help drawer without adding an overlay. The pending list
remains the single scroll owner. Shared state/actions serve both viewports;
coarse-pointer pills keep their existing 44px targets and wrap without overflow.
Desktop hover tooltips keep their Escape boundary.

```text
Desktop: Queued 3/10  [Auto-run off] [Auto-merge on]  Clear all
Phone:   Queued 3/10                         Clear all
         [Auto-run off] [Auto-merge on]
         A1 + A2 + A3                    Send / Remove
         B1 + B2                         Send / Remove
```

## Verification

TDD backend RED/GREEN for default sorted admission and session compaction.
Run changed backend packages with `-race`, touched Vitest suites, typecheck,
i18n generation/check/ratchet, Oxlint, changed-Go lint, spec/public-doc checks,
and desktop/mobile queue/settings E2E with retries disabled after fresh builds.
Use cache-drive temp/build output and isolated Kandev test environment.
PostgreSQL requires `KANDEV_TEST_POSTGRES_DSN`; record skips as unverified.

## Risks

- Existing Auto-merge tests may encode the superseded no-compaction contract.
- Settings and queue storage share a writer: never load settings inside a queue
  transaction, including policy-changed retries.
- Preserve latest-only target selection, management fences, and edit leases.

## Results

### Implementation

- Removed the separate setting/revision, session columns/map/override API,
  transaction validator, WebSocket action/controller, status tuple/producers,
  frontend API/action/meta, third pill/help drawer, dependent settings draft,
  and exclusive locale keys. No compatibility aliases remain.
- Default effective Auto-merge now enables sorted agent folding. Only a
  session effective OFF-to-ON write invokes atomic sorted-plus-adjacent
  compaction. Global changes and ON-to-ON/OFF writes do not compact.
- Preserved total ordering/index, tied predecessors, keyset walk, lease lock
  order/snapshot, management fences, compaction accumulator, and explicit
  Auto-merge override resilience when the global loader fails.
- `git diff --exit-code origin/main --
  apps/backend/internal/orchestrator/messagequeue/auto_merge.go`: passed.
- Browser scenarios use the default setting without seeding a separate sort
  preference, then use the Auto-merge pill for session compaction. Removed the
  old Quick Chat retry override so `--retries=0` is effective throughout.

### Verification completed

Commands below run from the repository root unless a backend working directory
is stated. Tests unset inherited Kandev port/home/service/install variables.
Temporary files, output, module/build caches, and logs use the cache drive.
Node-based final checks use installed Node 24.19.0. Existing dependencies were
used without `pnpm install`.

- Backend directory: `go test -trimpath -tags fts5,e2e
  ./internal/orchestrator/messagequeue -run
  'TestAutoSortMerge(InterleavedAdmissions|LegacyAdmissionUsesChosenTarget|SessionSwitchCompaction)$'
  -count=1`: RED on default sorted target/full-capacity and compaction
  assertions in memory/SQLite; GREEN after minimal implementation.
- Backend directory: `go test -trimpath -tags fts5,e2e
  ./internal/orchestrator/messagequeue -count=1`: passed after clean removal.
- `make fmt`: first attempt reached web formatting but timed out at 300s.
  Repeated under Node 24 with a larger deadline: passed.
- Touched Vitest selection: eight hook/API/WS suites passed (149 tests).
  Four component suites initially failed dependency loading because relocating
  Vite's cache changed Node's dependency search root. Linking the cache root to
  the web workspace's installed dependencies fixed loading, without an install.
  Three component suites passed; the settings suite exposed its mock Switch
  dropping `data-testid`, which was corrected. Removed an incidental default/
  wording-only test rather than re-pinning copy.
- `pnpm --dir apps/web exec vitest run
  components/settings/system/message-queue-settings.test.tsx
  lib/state/slices/session hooks/domains/session/use-queue-refetch.test.ts`:
  45 files, 526 tests passed.
- `pnpm --dir apps/web exec vitest run
  lib/ws/handlers/agent-session.test.ts`: 79 tests passed after removing an
  import made unused by deleting the sort-only test.
- `pnpm --dir apps/web run typecheck`: passed.
- `pnpm --dir apps/web run i18n:zh-hant`: passed; no residual Simplified Chinese.
- `pnpm --dir apps/web run i18n:pseudo`: passed. Unrelated key-order-only drift
  in `pseudo/task.json` was removed.
- `pnpm --dir apps/web run i18n:check`: passed; all six real locales complete,
  pseudo synchronized, no untranslated new copy or em dashes.
- `pnpm --dir apps/web run i18n:ratchet`: passed.
- `pnpm --dir apps/web run lint`: initially reported the unused `toTaskId` test
  import; passed after removing it.
- `python3 scripts/list-docs.py validate`: passed (455 decisions, 2032 specs).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: 63 passed.
- `node scripts/validate-public-docs.mjs`: 55 published pages validated.
- `pnpm --dir apps/web run build:e2e`: passed. Chunk-size and
  ineffective-dynamic-import warnings were nonfatal.
- Playwright discovery for the six queue/settings specs: 35 tests,
  22 Chromium and 13 mobile-chrome.
- Backend directory: `go test -trimpath -race -tags fts5,e2e
  ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers
  ./internal/system/queuesettings -count=1`: passed (40.139s, 64.330s, 2.394s).
- Backend directory: `go test -trimpath -race -tags fts5,e2e
  ./internal/backendapp -run AutoMerge -count=1`: passed (2.719s).
- Isolated current-build browser smoke: desktop help explains both fold paths,
  session compaction and later-only global changes. Global Auto-merge saved OFF
  and survived reload; mobile saved ON and survived reload. At 390px, the
  combined control wraps without horizontal overflow. Desktop/mobile
  screenshots were captured and inspected; browser/backend were stopped.
- Initial changed-Go lint timed out loading packages, without a lint finding.
  Host load was 61; overlapping analysis, integration and broad-Make jobs were
  stopped for serial execution. The backendapp test had already passed.
- Fresh runtime build completed host/cross-platform agentctl, backend,
  mock-agent and debug tools plus the delivery fixture, then timed out building
  the send-now fixture. After interruption, the remaining fixture/package
  targets completed successfully without rebuilding completed runtime targets:
  `make -C apps/backend build-e2e-send-now-fixture -o e2e-plugin-ui
  e2e-plugin-package GOFLAGS=-mod=mod`. The unchanged tracked fixture UI was used.
- A verbose PostgreSQL-skip probe also timed out during cold compilation.
  No PostgreSQL result is claimed; the DSN remains absent.
- Desktop browser execution: 22 passed with retries disabled:

  ```sh
  pnpm --dir apps/web e2e:run --host --shards 1 --no-build --project chromium \
    tests/chat/message-queue-auto-sort-merge.spec.ts \
    tests/chat/message-queue.spec.ts tests/system/message-queue-settings.spec.ts \
    -- --retries=0 --reporter=list --output="$CACHE/e2e-chromium"
  ```

- Mobile browser execution: 13 passed with retries disabled. The initial run
  passed 12 and found one stale touch-target assertion for the removed third
  pill. Removed that assertion, retaining the remaining controls' touch/pin
  checks, then reran all three specs successfully:

  ```sh
  pnpm --dir apps/web e2e:run --host --shards 1 --no-build --project mobile-chrome \
    tests/chat/mobile-message-queue-auto-sort-merge.spec.ts \
    tests/chat/mobile-message-queue-management.spec.ts \
    tests/system/mobile-message-queue-settings.spec.ts \
    -- --retries=0 --reporter=list --output="$CACHE/e2e-mobile"
  ```

  `$CACHE` denotes the invocation-owned directory on the cache drive.
  Desktop/mobile queue screenshots were captured and inspected, showing only
  Auto-run/Auto-merge and the folded sender messages without horizontal overflow.

- Backend directory: `go vet -tags fts5,e2e ./internal/backendapp
  ./internal/mcp/handlers ./internal/orchestrator ./internal/orchestrator/handlers
  ./internal/orchestrator/messagequeue ./internal/system/queuesettings
  ./pkg/websocket`: passed after serial cache warmup.
- `bash scripts/lint-go-changed` with all 35 changed/new Go paths: passed,
  zero new issues. After the first package-loading timeout, a second attempt
  reported zero issues but still exited on its deadline. A direct analyzer-cache
  warmup over the same seven packages with `--timeout=30m --concurrency=2
  --new-from-rev=ea6af0d500e92a20420729a505c2b95c65707b36 --verbose` passed in
  eight seconds; the unchanged normal script then passed. No lint configuration
  or hook timeout was changed.
- Backend directory: `go run ./cmd/sqlguard ./internal`: passed.
- Backend directory: `go test -trimpath -race -tags fts5,e2e
  ./internal/persistence/storeconformance -count=1`: passed for available
  backends; PostgreSQL remains unverified.
- Backend directory: `go test -trimpath -tags fts5,e2e
  ./internal/orchestrator/messagequeue -run
  'TestAutoSortMergeInterleavedAdmissions/postgres|TestAutoMergeUsersKeepOrderAndAdjacentMerge/postgres'
  -v -count=1`: all six selected PostgreSQL cases explicitly skipped because
  `KANDEV_TEST_POSTGRES_DSN` is unset. This is not PostgreSQL coverage.

- `make typecheck test lint`: stopped at classic `tsc` heap exhaustion with a
  4 GiB heap. `make typecheck` then passed with an 8 GiB heap, two-CPU affinity
  and one pnpm workspace at a time. Package-local native typecheck had already
  passed. Release-note/changelog metadata was regenerated on the cache drive.
- `make test`: failed in five untouched backend packages before reaching web,
  CLI or script targets. The complete log is
  `kandev-run.make-test.MqaIqE7c.log` in the invocation's cache-drive log directory.
  The broad gate is not green:
  - `internal/agent/settings/handlers`: three agent-update jobs did not finish
    within their tests' ten-second deadlines.
  - `internal/common/config`: the shell-injected `KANDEV_TRUSTED_PROXIES`
    overrode the YAML fixture. After unsetting it, focused
    `go test -trimpath -tags fts5 ./internal/common/config -run
    '^TestConfigSourceLoadsStableYAMLFields$' -count=1` passed. No config source
    or fixture was changed.
  - `internal/improvekandev`: `TestImproveBootstrapUsesUpgradedInstructions`
    failed with `workflow step lister is not configured`; its fixture constructs
    the task service with only the workflows repository.
  - `internal/plugins`: goleak found a gRPC `resetTransportAndUnlock` goroutine
    after its individual tests passed.
  - `internal/task/repository/sqlite`: a retention-control context deadline and
    the package's ten-minute timeout while running the large-history test.
  These failures are not attributed to the queue revision without baseline
  evidence. No unrelated subsystem was modified to suppress them.

- `make -k -j1 test-web test-cli test-scripts lint` completed the independent
  broad targets after the backend failure:
  - Web: 3395 suites passed, one GitLab ownership suite failed; 30,974 tests
    passed, four timed out at 5000ms, four skipped.
  - CLI: all five tests passed.
  - Full backend Go lint: zero issues. Oxlint, all 206 harness files, and full
    specification lint passed.
  - Architecture lint initially encountered the deliberately deleted, still
    unstaged queue-sort handler. After staging the 108 explicit cutover paths,
    `python3 scripts/lint-architecture.py --all` passed. `gofmt -l` reported no
    unformatted staged Go files. No shim was restored to satisfy the checker.
- `pnpm --dir apps/web exec vitest run
  components/gitlab/watch-dialog-ownership.test.tsx --project browser-locales
  --testTimeout=15000`: all 38 passed in isolation. No GitLab source/test was
  changed. The original full-web timeout failures remain reported, not hidden
  by this narrower result.
- Script tests initially stopped because Cargo was absent from PATH. The
  installed Rust/Cargo 1.92 toolchain satisfies the manifest's Rust 1.90 minimum.
  `bash scripts/release-desktop.test.sh` passed using that toolchain with
  `CARGO_HOME`/`CARGO_TARGET_DIR` on the cache drive and debug info disabled.
- Completed the script recipe's previously unreached components successfully:
  `scripts/release/runtime-bundle.test.sh`,
  `scripts/release/retry-ghcr-command.test.sh`,
  `scripts/release/signpath-signing-ready.test.sh`,
  `apps/desktop/e2e/desktop-launch-smoke.test.mjs`,
  `.github/scripts/release-workflow-contract_test.py`,
  the six nightly/npm/Scoop/existing-release Node test files, and the public-doc/
  generic-plugin-boundary Node tests.
  The desktop smoke first reported a missing browser under the isolated HOME;
  setting `PLAYWRIGHT_BROWSERS_PATH` to the already-installed cache-drive browsers
  made it pass. No browser, Node dependency, or Rust toolchain install was needed.

### Verification limits

PostgreSQL remains unverified without its DSN. The broad backend and original
full-web runs are not green for the failures recorded above; focused mitigation
results do not constitute a full-matrix pass. No unrelated code was changed.
