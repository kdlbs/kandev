---
created: 2026-11-05
status: implemented
requirements:
  - REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001
system_design:
  - ../../specs/ui/system-design/message-queue-auto-sort-merge.md
legacy_specs: []
---

# Implementation Plan: Message Queue Auto-sort-merge

## Design revision: fold into Auto-merge

The maintainer/user decision on #5086 supersedes the separate-policy design in
tasks 01-05. Sorted sender-task folding is included in effective Auto-merge,
which remains ON by default. Only session Auto-merge OFF-to-ON compacts pending
rows. Global changes affect later messages only.

Current work order:
[Task 06: Fold into Auto-merge](task-06-fold-into-auto-merge.md) (`done`).
It owns backend/frontend removal, regression coverage, localized copy,
desktop/mobile E2E, and reconciliation of the existing public/spec contracts.

### Current ASCII UI preview

```text
Desktop: Queued 3/10  [Auto-run off] [Auto-merge on]  Clear all
Phone:   Queued 3/10                         Clear all
         [Auto-run off] [Auto-merge on]
         A1 + A2 + A3                    Send / Remove
         B1 + B2                         Send / Remove
Settings: Automatically merge queued messages             [on]
```

The existing inline panel remains the mobile surface with one message-list
scroll owner and wrapping 44px coarse-pointer targets. There is no third pill,
dependent draft, or sort help drawer. Remaining tooltip Escape boundaries stay.
This preview supersedes historical UI-01, UI-02, and UI-03 below.

### Revision evidence

- Backend RED: default sorted admission failed in memory/SQLite, including
  full-capacity paths; session Auto-merge OFF-to-ON removed 0 instead of 4 rows.
- Minimal GREEN: the same focused command passed after gating sorted folds and
  compaction directly on Auto-merge.
- After clean policy removal, the full messagequeue package passed.
- Targeted race checks passed. Chromium: 22 passed; mobile-chrome: 13 passed,
  both with retries disabled. Verification results and limits are in Task 06.

## Historical implementation through Task 05

The remaining plan records the original separate-option implementation and its
historical verification. It is not the current product contract; use Task 06
and the revised requirement/design for the fold-in.

## Overview

Add Auto-sort-merge as a third message-queue automation policy. Work order 01
adds the policy storage first, so every later behaviour and control is backed
by authoritative persistence and status. Work order 02 adds sorted admission
folds and work order 03 adds the session compaction pass. Work order 04 exposes
the dependent global and session switches. Work order 05 proves the flows in
the browser, updates the public docs, and reconciles the specs that this
feature amends.

## Scope

### In scope

- A global `auto_sort_merge_enabled` setting (default OFF) with an internal
  revision, and a nullable per-session override with a revision. Inheritance
  is the same as for Auto-merge.
- Effective policy = effective Auto-merge AND the Auto-sort-merge preference,
  with the preference kept while Auto-merge is OFF.
- Admission folds of agent sender-task messages into their latest same-sender
  entry (only if compatible; never an older one), across user and other-agent
  rows but never across barriers, including at full capacity.
- A one-transaction compaction pass when a session switch turns effective
  Auto-sort-merge from OFF to ON.
- The queue header pill, the dependent settings switch, status ordering,
  locales, E2E, public docs, and spec amendments.

### Out of scope

- Moving messages that cannot merge, sorted folds for user messages, and
  child-only sender filtering.
- Compaction on global setting changes.
- Changes to Auto-merge compatibility, Send Now, Auto-run, append,
  coalescing, or retry semantics. The always-active ordering, tied-predecessor,
  index, and edit-lease lock-order corrections that also affect manual merge,
  reorder, and restore are defined in the design's "Always-active queue
  corrections" section and must be called out in the PR description.

## Technical approach

### Policy and persistence (Task 01)

- `apps/backend/internal/system/queuesettings/{types,store,service,handler,resolver}.go`:
  add the `AutoSortMergeEnabled` and `AutoSortMergeRevision` fields, the patch
  and effective fields, legacy-omitted → `false`, a revision bump only when
  the value changes, and publication of one combined `GlobalQueuePolicy`
  (both value/revision pairs) through one target interface whose loader reads
  the settings envelope once.
- `apps/backend/internal/backendapp/orchestrator.go`: hydrate the combined
  global snapshot at startup.
- `apps/backend/internal/orchestrator/messagequeue/types.go`: add
  `AutoSortMergePolicy` and `AutoSortMergeOverride`, plus the `QueueStatus`
  fields `auto_sort_merge_available`, `auto_sort_merge_enabled`,
  `auto_sort_merge_source`, and `auto_sort_merge_revision`.
- `repository.go`, `repository_sqlite.go`, `repository_memory.go`: add the
  `queue_session_state.auto_sort_merge_override` and
  `auto_sort_merge_revision` columns in the fresh and `ALTER` paths, the
  `Get`/`SetAutoSortMergeOverride` functions, the incarnation reset, and
  transfer/delete cleanup alongside `auto_merge_override`.
- `service.go`: replace the Auto-merge-only global pointer and loader with one
  `GlobalQueuePolicy` pointer/loader, add `ResolveAutoSortMergePolicy`,
  `Snapshot` projection with an unavailable marker, and
  `SetSessionAutoSortMerge` (compaction is wired in Task 03).
- `handlers/queue_handlers.go`, `handlers/queue_operations_handlers.go`, and
  `pkg/websocket/actions.go`: add `message.queue.auto_sort_merge.set` and its
  controller interface.

### Sorted admission folds (Task 02)

- New `messagequeue/auto_sort_merge.go`: the pure `sortedFoldTarget(rows,
  source)` and its barrier predicate.
- Combine both global snapshots into `AdmissionPolicy` in
  `resolveAdmissionAutoMergePolicy`. Validate the sort policy in the
  transaction next to `validateAutoMergePolicyTx`.
- Use the sorted target in `finalizeAutoMerge`/`autoMergeIntoAbove`,
  `mergeInsertedAdmissionCandidateTx`, `autoMergeCandidateIntoAbove`, and
  `admitFullQueueCandidateTx`, plus their memory equivalents in
  `repository_memory.go`. Move the edit-lease check to the chosen target.

### Compaction pass (Task 03)

- `compactSessionQueueTx` in SQL and memory, sharing `sortedFoldTarget` and
  `buildAutoMergedEntry`.
- `SetSessionPolicyAndCompact` repository operation. `SetSessionAutoMerge` and
  `SetSessionAutoSortMerge` both use it and return `SessionPolicyResult`
  (both policies plus `CompactedCount`); controller interfaces and test
  doubles migrate.
- Compaction uses a per-segment latest-by-sender index (O(1) lookup per row)
  over the full persisted row stream, which is not bounded by capacity.

### Frontend (Task 04)

- `apps/web/lib/types/system.ts`, `lib/api/domains/settings-api.ts`, and
  `components/settings/system/message-queue-settings.tsx`: the dependent
  `AutoSortMergeToggleFields`.
- `lib/types/session-events.ts`, `lib/state/slices/session/{types,session-slice}.ts`,
  and `lib/ws/handlers/agent-session.ts`: the tuple and its per-policy
  ordering.
- `lib/api/domains/queue-api.ts` (`setQueueAutoSortMerge`) and
  `hooks/domains/session/use-queue.ts`.
- `components/task/chat/queued-ghost-panel-header.tsx` and
  `queued-ghost-list.tsx`: the third pill.
- `src/locales/*/{system,settings}.json` (and the chat namespace that holds
  the pill labels): every shipped locale. Run `pnpm run i18n:zh-hant` for
  Traditional Chinese.

## ASCII UI preview

### UI-01: Queue header automation pills (expanded queue panel, desktop)

```text
+--------------------------------------------------------------------------------------+
| [=] Queued 7/10   ( Auto-run [on] ) ( Auto-merge [on] ) ( Auto-sort-merge [on] )   Clear all  [pin] [v] |
+--------------------------------------------------------------------------------------+
| A1  from Sub-task A  ...                                             [Send now] [x]  |
| B1  from Sub-task B  ... (B2 folded in)                              [Send now] [x]  |
| A2  from Sub-task A  ...                                             [Send now] [x]  |
+--------------------------------------------------------------------------------------+
```

State: Auto-merge OFF. The Auto-sort-merge pill is disabled and renders OFF,
with a tooltip and accessible description saying "Requires Auto-merge".

```text
( Auto-run [on] ) ( Auto-merge [off] ) ( Auto-sort-merge [off] )   <- greyed, not clickable
```

### UI-02: Queue header on a phone (`mobile-chrome`)

```text
+--------------------------------------+
| [=] Queued 7/10        Clear all [v] |
| ( Auto-run [on] ) ( Auto-merge [on] )|
| ( Auto-sort-merge [on] )             |
+--------------------------------------+
| B1  from Sub-task B ...    [Send] [x]|  <- single internal scroll owner
+--------------------------------------+
```

Pills wrap as whole groups. Each pill is at least 44×44 CSS px on coarse
pointers. There is no document horizontal overflow and no drawer.

### UI-03: Settings > Task Behavior > Message Queue

```text
Automatically merge consecutive messages                          [on ]
  Sessions inherit this until changed in the queue panel.
Automatically sort and merge messages from the same sender task   [off]
  Requires automatic merge. Moves a sub-task's new message up to
  merge with its latest queued message. Sessions inherit this
  until changed in the queue panel.
```

When "Automatically merge" is OFF, the second switch is disabled and renders
OFF. Its saved value is kept and reappears when merging is turned back on.

Structural requirements: the pill order, the pill label text, the disabled-OFF
dependency rendering, and wrap-not-scroll. Spacing is illustrative. Maps to
`AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.1`, `.2`, `.4`, `.5`, and `.16`.

## Tests

| AC | Evidence |
| --- | --- |
| .1, .2 (backend), .3, .13, .14, .15 (backend) | `queuesettings/queuesettings_test.go`, `queuesettings/revision_test.go`, `backendapp/message_queue_settings_test.go`, `messagequeue/repository_auto_sort_merge_override_test.go`, `messagequeue/service_auto_sort_merge_policy_test.go`, `handlers/queue_handlers_auto_sort_merge_test.go` |
| .6, .7, .8, .9, .14 | `messagequeue/auto_sort_merge_test.go` (`TestSortedFoldTarget*`), `messagequeue/service_auto_sort_merge_test.go`, `messagequeue/repository_auto_sort_merge_test.go` (SQLite + PostgreSQL) |
| .10, .11, .12, .13 | `messagequeue/repository_compaction_test.go`, `messagequeue/service_auto_sort_merge_compaction_test.go`, `messagequeue/repository_auto_sort_merge_race_test.go`, `backendapp/message_queue_auto_sort_merge_retry_test.go` |
| .2, .4, .5, .12, .15 | `message-queue-settings.test.tsx`, `queued-ghost-panel-header.test.tsx`, `queue-api.test.ts`, `use-queue.test.ts`, `agent-session.test.ts`, `session-slice` tests |

## E2E tests

- `apps/web/e2e/tests/chat/message-queue-auto-sort-merge.spec.ts` (project
  `chromium`) covers `.4`, `.5`, `.6`, `.7`, `.10`, `.11`, and `.13`. It
  drives sender-task messages through the mock agent's
  `e2e:mcp:kandev:message_task_kandev(...)` directive.
- `apps/web/e2e/tests/system/message-queue-settings.spec.ts` (`chromium`)
  covers `.1` and `.2`.
- `apps/web/e2e/tests/chat/mobile-message-queue-auto-sort-merge.spec.ts`
  (`mobile-chrome`) covers `.5`, `.6`, `.10`, `.12`, and `.16`: tap the pill,
  observe compaction and a later sorted admission, check the disabled-OFF
  dependency and preference restoration, and reload. Touch/overflow checks are
  added to `mobile-message-queue-management.spec.ts`.
- `apps/web/e2e/tests/system/mobile-message-queue-settings.spec.ts`
  (`mobile-chrome`) covers `.1` and `.2` on a phone.

## Work orders

- [x] [Task 01: Persist Auto-sort-merge policy](task-01-persist-auto-sort-merge-policy.md) (cancelled: superseded by Task 06)
- [x] [Task 02: Sorted admission folds](task-02-sorted-admission-folds.md)
- [x] [Task 03: Session compaction pass](task-03-session-compaction-pass.md)
- [x] [Task 04: Auto-sort-merge controls](task-04-auto-sort-merge-controls.md) (cancelled: superseded by Task 06)
- [x] [Task 05: E2E, docs, and spec reconciliation](task-05-e2e-docs-spec-reconciliation.md)

Order: 01 → 02 → 03 → 04 → 05. Task 04 depends only on Task 01's contract and
can run in parallel with Tasks 02 and 03.

## Verification results

All five work orders are done; their Results sections contain RED/GREEN and
targeted verification evidence. Chromium: 23 passed. Mobile-chrome: 14 passed.
Public docs and specification validation passed. PostgreSQL is unverified:
`KANDEV_TEST_POSTGRES_DSN` is unset and those cases skip.

- `make fmt`: passed, `/tmp/kandev-run.format.I9HZI8cI.log`.
- First `make typecheck test lint`: stopped at typecheck when inherited Node 22
  exhausted its approximately 2 GiB heap,
  `/tmp/kandev-run.verify.dG1xydhC.log`. Tests and lint did not start.
- Retry with Node 24.22.2 and an 8 GiB heap passed typecheck, then failed the
  backend suite in six packages, `/tmp/kandev-run.verify.L4RhSOc5.log`.
  Ambient production port/proxy/service overrides polluted config, launcher,
  and update tests; login-shell fixtures read a profile that sources a missing
  file. The large SQLite package reached its aggregate 10-minute deadline.
- A temporary launcher removes `KANDEV_*`/`AGENTCTL_*` runtime overrides and
  token variables, uses a fresh HOME/config directory, preserves existing Go
  and linter caches, and pins Node 24 with an 8 GiB heap. The final launcher
  uses an 18 GiB, 512-task systemd scope and `GOMEMLIMIT=10GiB`; four Go
  workers and a 30-minute package deadline are selected through Make's `GOFLAGS`
  argument, since the backend Makefile overrides environment-only GOFLAGS.
- Targeted rechecks of every failing case in those six packages passed,
  including the SQLite test active at timeout:
  `/tmp/kandev-run.environment-recheck.ZGtSD1Si.log`. No production behavior or
  test assertions changed for this environment remediation.
- `go run ./cmd/sqlguard ./internal`: passed.
- `go test -trimpath -race ./internal/persistence/storeconformance -count=1`:
  passed after migrating the external admission adapter to `AdmissionPolicy`
  (118.635s).
- The neutral full rerun passed typecheck and the complete SQLite package
  (701.791s), but two unrelated agent-update endpoint tests hit their
  10-second terminal-event wait,
  `/tmp/kandev-run.verify.35FJGZyr.log`.
- Targeted mock-mode recheck of those updater cases still failed,
  `/tmp/kandev-run.updater-recheck.wAfQ4GiR.log`. A deliberate three-second
  diagnostic timeout captured their blocked goroutine in
  `BroadcastAvailableAgents -> discovery.Detect -> HermesACP.IsInstalled`,
  executing the host Hermes CLI,
  `/tmp/kandev-run.updater-stack.tZHCjS51.log`.
- Verification now excludes the host-agent bin directory from PATH and
  exposes real Go, pnpm, npm/npx, lint, and hook executables through an owned
  tool-only directory. Full `go test -trimpath -tags fts5
  ./internal/agent/settings/handlers -count=1` passed with this isolation,
  `/tmp/kandev-run.updater-isolated.XQDbwe2j.log`. No mocks, production code,
  or assertions were added or changed.
- Standalone `make lint` was terminated at the 600-second command deadline
  while still in full-backend lint, with no linter diagnostics,
  `/tmp/kandev-run.lint.SqBQZUNt.log`. The owned scope was inactive afterward.
  One monitored tmux retry uses an 18 GiB scope and a 10 GiB Go soft limit.
- Full backend lint then identified three mechanical findings (a redundant
  wrapper and two equivalent Boolean expressions). After those fixes it passed.
  Four web lint findings were resolved with equivalent helper extraction and
  control-flow simplification. The exact Task 04 regression passed again:
  48 files, 675 tests. Native web typecheck and full web lint also passed.
- The next root run passed typecheck and the entire backend suite, then failed
  109 web tests in 28 files under the default eight-worker budget:
  `/tmp/kandev-run.verify.Mec898m8.log`. Most failures were deadlines under load.
  A two-worker recheck passed 363 tests in 25 files and left three failures:
  `/tmp/kandev-run.web-failure-recheck.udKLuGIt.log`.
- Host HTTP Git unit fixtures now select the existing loopback option, as the
  sibling host fixture tests already do, rather than requiring absent Docker.
  Their original assertions pass. Missing `unzip` was installed from the official
  Ubuntu package into the owned verification directory and `~/.local/bin`
  after sudo and the HTTP apt mirror were unavailable. The Git 2.53 wording test was
  removed, not repinned; the retained origin tests still exercise lock
  preservation, successful later refresh, and live/stale worktree behavior.
- A one-worker recheck of all residual failures passed: four files, 49 tests,
  `/tmp/kandev-run.residual-web-recheck.DSyWTwk8.log`.
- Final formatting passed, `/tmp/kandev-run.format.U1UzDG5l.log`. The next
  `make typecheck test lint` passed typecheck, the full backend suite, all
  3291 web files (29993 tests passed, four skipped), and all five CLI tests.
  It stopped in `test-scripts` because `cargo` was absent from the isolated
  PATH, `/tmp/kandev-run.verify.H0PW7xUp.log`. Installed Cargo, rustc, and rustup
  launchers were added to the tool-only PATH; no source or assertion changed.
- `make test-scripts lint` retry reached Cargo but could not fetch
  `tauri-plugin-updater` 2.14.1, `/tmp/kandev-run.remaining-gates.uKK6k98Y.log`.
  `CARGO_NET_OFFLINE=true bash scripts/release-desktop.test.sh` also failed:
  locked `tauri-macros` 2.7.4 is missing,
  `/tmp/kandev-run.desktop-signature-offline.KiDayd89.log`.
- Direct HTTPS probes of `index.crates.io`, `static.crates.io`, `crates.io`,
  `rsproxy.cn`, and the SJTU crate mirror all failed to connect. The official
  GitHub index is reachable, but the crate archives are not; the existing Cargo
  cache is insufficient. No dependency, verifier, or test was bypassed.
- Full `make lint` passed, `/tmp/kandev-run.lint.mAiyfvCz.log`.
- The independent remaining `test-scripts` commands (Makefile lines 597–603)
  passed after `pnpm --dir apps/desktop run build:vite` supplied the required
  preview assets. Logs: `/tmp/kandev-run.desktop-frontend-build.wDjzEQpK.log`
  and `/tmp/kandev-run.remaining-script-checks.D9R4nw1T.log`. The first preview
  attempt failed because `dist` did not exist,
  `/tmp/kandev-run.remaining-script-checks.pl2yclat.log`.
- After the mobile geometry-readiness fix, `make fmt typecheck lint` passed:
  `/tmp/kandev-run.final-reachable-gates.Bgi42OJv.log`. The strict 44px
  assertions were retained. Final Chromium replay: 23 passed; final mobile
  replay with retries disabled: 14 passed. See Task 05 for all replay logs,
  including the initial mobile flaky verdict and its readiness fix.
- Final `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.py --all`, `git diff --check -- docs`,
  `node scripts/validate-public-docs.mjs`, `pnpm --dir apps/web run i18n:check`,
  and `pnpm --dir apps/web run i18n:ratchet` passed:
  `/tmp/kandev-run.final-docs.FNycIrHo.log`. All-source i18n checked 4324 files
  and all six non-English shipped catalogs; the unstaged ratchet found 12
  modified files clean (new untracked files are covered by the all-source check).
- Final combined root gate and commit are blocked by unavailable locked Rust
  dependencies for the existing desktop updater-signature script. Commit was
  not attempted. The parent was notified; completing that gate requires Rust
  registry/archive access or a preseeded Cargo cache for the desktop lockfile.
  No push, PR, or archive action was performed.

### Resumed delivery attempt

- The shared worktree is still at
  `69b07e585b6406a458a7a78e87e886f8db11afdc`, with the implementation and
  reviewed package uncommitted. No production or permanent test changes were
  needed during this attempt.
- HTTPS probes of `https://index.crates.io/config.json` and the
  `static.crates.io` archive endpoint still fail with connection refused.
  DNS resolves both hosts, but TCP port 443 connections are refused.
- Fresh `bash scripts/release-desktop.test.sh` passed its first 28 checks,
  then failed resolving the desktop verifier's `base64` dependency because
  Cargo could not fetch `https://index.crates.io/config.json`.
  Log: `/home/clem/.cache/tmp/asm-final.wgrLi1/desktop.log`.
- Fresh `go test -trimpath -race -p=4 -timeout=10m
  ./internal/orchestrator/messagequeue/... -run
  'AutoSortMerge|SortedFold|Compaction' -count=1` passed.
  Log: `/home/clem/.cache/tmp/asm-final.wgrLi1/queue-race.log`.
- The resumed `make fmt` reached frontend formatting but exceeded its
  300-second command deadline. Backend formatting completed, and all frontend
  files processed before the deadline were unchanged.
  Log: `/home/clem/.cache/tmp/asm-final.wgrLi1/format.log`.
- New temporary homes, tool caches, compiler output, and logs use the checked
  `/dev/sdd` cache mount. PostgreSQL remains unverified without its DSN.
  Full root-gate completion and the normal-hook commit still require registry
  access or a complete cached desktop dependency set; neither was bypassed.
- Retried `make fmt` without a command deadline: passed.
  Log: `/home/clem/.cache/tmp/asm-final.wgrLi1/format-final.log`.
- `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.py --all`, and `git diff --check -- docs`:
  passed after recording the resumed attempt. Logs:
  `/home/clem/.cache/tmp/asm-final.wgrLi1/docs-validate.log` and
  `/home/clem/.cache/tmp/asm-final.wgrLi1/docs-lint.log`.

### Implementation review round 1 remediation

- Fold target lease checks no longer call back into `Service` while a
  repository lock or SQL transaction is held. Admission captures the
  session's in-process lease set under the admission gate
  (`AdmissionPolicy.leased`); durable SQL lease checks stay in the
  transaction.
- `sqliteRepository.Snapshot` reads both overrides before its single commit,
  so an unreadable Auto-merge override no longer fabricates an inherited
  Auto-sort-merge tuple.
- A failing global settings loader no longer discards explicit session
  overrides at admission; only a policy that needs the inherited value fails
  closed.
- RED on the prior code, GREEN after: `go test -tags fts5
  ./internal/orchestrator/messagequeue/ -run
  'TestAutoFoldInsideRepositoryDoesNotAcquireEditLeaseMutex|TestSnapshotResolvesSortPolicyWhenAutoMergeOverrideIsUnreadable|TestAdmissionUsesExplicitSessionPoliciesWhenGlobalLoaderFails'`
  (memory and SQLite).
- `go test -race -tags fts5 ./internal/orchestrator/messagequeue/...`,
  `go test -tags fts5 ./internal/orchestrator/handlers/...
  ./internal/system/queuesettings/...`, and `scripts/lint-go-changed`: passed.

### Implementation review round 2 remediation

- `RenewEdit`, `EndEdit`, and `EndEditAfterSave` acquire `editLeaseMu` before
  the repository session fence (`withEditLeaseTransferFence`), matching
  lease-guarded saves that hold it across their repository write. This
  inversion predates the feature; it is fixed here because admissions now read
  the lease set under the same mutex.
- RED on the prior code, GREEN after:
  `TestEditLeaseFencedOperationsHoldLeaseMutexBeforeRepositoryFence`.
- `go test -race -tags fts5 ./internal/orchestrator/messagequeue/...
  ./internal/orchestrator/handlers/...` and `scripts/lint-go-changed`: passed.

### Implementation review round 5 remediation

- On touch devices the Auto-sort-merge pill adds a 44px info button
  (`queue-auto-sort-merge-help`) that opens a drawer with the current help,
  dependency, or unavailable text; desktop keeps the tooltip. The pill's
  accessible description now follows the same state-specific text. New
  `chat:queueAutoSortMergeAbout` label in every shipped locale.
- RED on the prior code, GREEN after: `queued-ghost-panel-header.test.tsx`
  ("explains a disabled Auto-sort-merge pill through a touch drawer").
- `pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome
  tests/chat/mobile-message-queue-auto-sort-merge.spec.ts -- --retries=0`:
  1 passed (now taps the help with Auto-merge OFF and dismisses the drawer).
  Chromium `message-queue-auto-sort-merge.spec.ts` and `message-queue.spec.ts`
  with `--retries=0`: 16 passed. Web typecheck, `i18n:check`, Prettier, and
  Oxlint on changed files: passed.

### Implementation review round 6 remediation

- The touch help drawer now opens with `autoFocus` and has a localized 44px
  Close button (`queue-auto-sort-merge-help-close`), matching `SettingsInfo`,
  so keyboard focus enters the dialog instead of staying on the hidden
  background.
- RED on the prior code, GREEN after: the focused-drawer case in
  `queued-ghost-panel-header.test.tsx` (focus inside the dialog on open).
- Mobile E2E now opens the drawer from the keyboard, checks that Tab and
  Shift+Tab stay inside it, closes it, and checks focus returns to the info
  button: `pnpm --dir apps/web e2e:run --host --shards 1 --project
  mobile-chrome tests/chat/mobile-message-queue-auto-sort-merge.spec.ts --
  --retries=0`: 1 passed. Web typecheck and Oxlint on changed files: passed.

### Implementation review round 8 remediation

- SQL compaction loads each row's `queue_task_management_fences` claim and
  folds two rows only when their claims are identical (or both unfenced). A
  new-generation plugin instruction can no longer be folded into an obsolete
  generation's row and then discarded with it at reservation. AC `.11` and the
  design's compaction section record this exception.
- RED on the prior code, GREEN after:
  `TestCompactionKeepsMessagesFromDifferentManagementClaimsSeparate`, for both
  session-switch compaction triggers.

### Implementation review round 12 remediation

- SQL sorted admission no longer loads and decodes every persisted row. User,
  sender-less, and barrier sources use the existing one-row predecessor read.
  Eligible agent sources walk rows newest-first reading only `id`,
  `queued_by`, and `metadata_json`, stop at the first barrier or same-sender
  row, then load only the chosen target (or adjacent row) in the same
  transaction.
- RED on the prior code, GREEN after:
  `TestSortedAdmissionDoesNotDecodeBacklogPayloads` (an undecodable backlog
  payload above the stop point no longer fails admission), with space and at
  capacity.
- `go test -race -tags fts5 ./internal/orchestrator/messagequeue/...
  ./internal/orchestrator/handlers/...`, backendapp `AutoSortMerge`, and
  `scripts/lint-go-changed`: passed.

### Implementation review round 13 remediation

- Compaction collects each survivor's non-empty bodies and joins them once
  after the pass, instead of re-copying the growing merged body on every
  fold. Compatibility is still evaluated per fold against the current logical
  target. RED on the prior code (542 MB allocated for 512 x 4 KiB rows), GREEN
  after: `TestCompactionAllocationIsLinearInBacklogBytes`.
- The sorted-admission metadata walk reads in keyset batches of
  `sortedFoldScanBatch` (64) rows, so a match near the tail no longer makes
  PostgreSQL stream and drain the rest of the backlog.
  `TestSortedAdmissionScansPastOneMetadataBatch` covers a match beyond three
  batches on SQLite. PostgreSQL remains unverified (no DSN).
- `go test -race -tags fts5 ./internal/orchestrator/messagequeue/...
  ./internal/orchestrator/handlers/...` and `scripts/lint-go-changed`: passed.

### Implementation review round 14 remediation

- `buildAutoMergedEntry` is now one fold of an `autoMergeAccumulator` that
  validates each input once and keeps attachments, entity references, context
  files, and queue admission IDs as ordered sets. Compaction keeps one
  accumulator per survivor and materializes it once, so provenance IDs and a
  large kept attachment are no longer re-copied or re-decoded on every fold.
  Single admissions use the same code path and limits.
- The sorted metadata walk orders and continues on `(position, id)`, because a
  restored reservation can share a position with another row.
- RED on the prior code, GREEN after:
  `TestCompactionAdmissionProvenanceIsLinear`,
  `TestCompactionValidatesKeptAttachmentOnce`, and
  `TestSortedAdmissionFindsTargetAcrossTiedBatchBoundary` (reserve, reorder,
  restore, then a full-queue sorted fold into each tied row).
- `go test -race -tags fts5 ./internal/orchestrator/messagequeue/...
  ./internal/orchestrator/handlers/... ./internal/system/queuesettings/...`,
  backendapp `AutoSortMerge|AutoMerge|QueueSettings`, and
  `scripts/lint-go-changed`: passed.

### Implementation review round 15 remediation

- Every SQL query that orders a session's queued rows now uses one total FIFO
  order, `(position, id)` (reverse `(position DESC, id DESC)`): listing,
  reserve/drain, predecessor and tail reads, reorder reads, compaction, the
  sorted metadata walk, managed-input head and coalesce reads, and
  cross-session durable listings. The in-process tail and merge-target picks
  keep the last tied row in that order. A restored reservation can share a
  position, so sorted folds and compaction now see exactly the order the
  queue lists and drains and cannot cross a tied barrier.
- RED on the prior ordering, GREEN after:
  `TestSortedFoldsFollowListedOrderAcrossTiedBarrier` (both ID orders, for
  admission at capacity and for compaction).
- `go test -race -tags fts5` for `messagequeue`, `orchestrator/handlers`,
  `system/queuesettings`, and `internal/mcp/...`; `go test -tags fts5` for
  `internal/orchestrator` and `internal/backendapp/...`; and
  `scripts/lint-go-changed`: passed.

### Implementation review round 16 remediation

- Predecessor reads now use the full `(position, id)` source key:
  `readMergeTarget` and the first sorted-walk batch select
  `position < p OR (position = p AND id < sourceID)`, so a row tied with the
  source is its predecessor. The service's tail and manual-merge lease checks
  walk `ListBySession` order instead of comparing positions.
- The memory repository uses the same order (`queueOrderLess`) for restore
  insertion, listing, head selection, Send Now restore, reorder, merge target,
  and replacement.
- RED on the prior code, GREEN after:
  `TestFoldsTreatTiedRowAboveSourceAsPredecessor` (a tied barrier above the
  source blocks the fold; manual merge uses the tied row above), and the
  tied-barrier regressions now run on memory and SQLite.
- `go test -race -tags fts5` for `messagequeue`, `orchestrator/handlers`, and
  `system/queuesettings`; `go test -tags fts5` for `internal/orchestrator` and
  `internal/mcp/handlers`; and `scripts/lint-go-changed`: passed.

### Implementation review round 18 remediation

- `autoMergeCompatible` checks sender, task, model, plan mode, and
  non-additive metadata before an accumulator absorbs the target, both in
  `buildAutoMergedEntry` and before compaction creates a survivor's
  accumulator. An incompatible pair no longer decodes the target's attachment
  payload, restoring the pre-branch cost of a rejected ordinary Auto-merge.
- RED on the prior code (21 MB for four rejected folds against a 4 MiB
  attachment), GREEN after:
  `TestIncompatibleAutoMergeDoesNotDecodeTargetAttachment`.
- `go test -race -tags fts5` for `messagequeue` and `orchestrator/handlers`,
  and `scripts/lint-go-changed`: passed.

### Implementation review round 19 remediation

- `auto_merge.go` is restored byte-for-byte to `origin/main`, so ordinary
  single admissions (`buildAutoMergedEntry`) have exactly their pre-branch
  checks, order, results, and cost. `autoFoldValues` now rejects an
  incompatible pair before the durable edit-lease query.
- The chain accumulator moved to `compaction_accumulator.go` and is used only
  by compaction. `startCompactionMerge` checks identity, metadata, and the
  combined attachment count before decoding any payload; later folds use the
  accumulated count.
- RED on the prior code, GREEN after:
  `TestCompactionAttachmentCountOverflowDoesNotDecodeTarget`.
  `TestCompactionFoldMatchesSingleAutoMerge` pins that a compaction fold and
  `buildAutoMergedEntry` produce the same entry across text, empty bodies,
  attachments, admission IDs, context files, entity references, and rejected
  pairs.
- `go test -race -tags fts5` for `messagequeue` and `orchestrator/handlers`,
  and `scripts/lint-go-changed`: passed.

### Implementation review round 20 remediation

- The `(session_id, position)` index is replaced (replay-safe `DROP INDEX IF
  EXISTS` plus `CREATE INDEX IF NOT EXISTS`) by
  `idx_queued_messages_session_order` on `(session_id, position, id)`, and the
  sorted walk and `readMergeTarget` use the row-value bound
  `(position, id) < (?, ?)`. Each batch seeks the index from the previous
  boundary instead of rescanning and re-sorting from the session tail, which
  had made a distant or missing target quadratic.
- Compaction allocates a fresh sender map at each barrier instead of clearing
  the largest earlier one, keeping reset cost per segment.
- RED on the prior schema, GREEN after:
  `TestSortedWalkBatchSeeksSessionOrderIndex` (SQLite query plan seeks the new
  index with the row-value bound and no temporary sort).
- `go test -race -tags fts5` for `messagequeue`, `orchestrator/handlers`, and
  `internal/persistence/...`; `go test -tags fts5` for `internal/backendapp/...`
  and `internal/orchestrator`; and `scripts/lint-go-changed`: passed.

### Implementation review round 22 remediation

- The touch help `DrawerContent` carries `data-queue-escape-boundary`, like the
  timestamp drawer, so Escape closes only the help drawer instead of also
  collapsing the queue panel through `useQueuePanelEscape`.
- The mobile E2E helper now presses Escape in the open drawer and asserts the
  drawer closes, focus returns to the help button, and the queue stays open:
  `pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome
  tests/chat/mobile-message-queue-auto-sort-merge.spec.ts -- --retries=0`:
  1 passed. The reviewer's component probe showed the parent collapse on the
  prior code; the browser RED run was not repeated.

### Implementation review round 23 remediation

- The three automation-pill tooltips carry `data-queue-escape-boundary`, and
  `useQueuePanelEscape` also yields to boundaries in Radix tooltip states
  `instant-open` and `delayed-open`. The first Escape on a shown tooltip
  dismisses only the tooltip; a later Escape collapses the queue.
- RED on the prior hook, GREEN after: the tooltip-state cases in
  `use-queue-panel-escape.test.tsx`. Header tests, Oxlint, and web typecheck
  passed.

### Final delivery verification after registry access was restored

- Verified implementation commit `fc04abc0f8` and review fixes through
  `8c06eb181b90f7facb101e5d549cdf0e42a1936c`. Registry and archive HTTPS
  probes now return 200; `bash scripts/release-desktop.test.sh` passes,
  including real signature verification.
- `make fmt` passed. Root gates and browser suites used an exact-commit
  disposable checkout on `/dev/sdd`, Node 24.19.0, isolated HOME/config/PATH,
  four Go workers, a 30-minute Go package deadline, and one Vitest worker.
  Build/test artifacts stayed on the cache drive.
- The Task 05 Chromium and mobile-chrome commands both passed with
  `-- --retries=0`: 23 and 14 cases respectively. Managed builds were fresh.
- Initial root verification exposed long temporary paths. A shorter
  cache-backed TMPDIR fixed Unix-socket and repository-directory cases.
  Two baseline test repairs were also required: generate the entire legacy
  Cursor slug using its legacy producer, and remove incidental worktree
  error-wording assertions while retaining failure identity/no-worktree checks.
  Targeted RED/GREEN evidence is in `short-path-recheck.log` and
  `baseline-green.log`.
- The subsequent `make typecheck test lint` passed typecheck and the full
  backend suite, then reported 30795 web tests passed, four skipped, and four
  timeouts in three files. It did not reach CLI/script/lint targets.
  The confirmation and workspace files passed isolated rechecks.
- The remaining Linear test exceeded its unchanged five-second deadline
  through cumulative select-query cost. Its helper now locates option text
  inside the opened listbox instead of repeatedly calculating accessible
  roles over the entire document. All 28 Linear lifetime cases then passed;
  no production behavior, assertions, or deadlines changed.
- `make test-cli test-scripts lint` passed independently. The combined root
  invocation was not clean end-to-end; its failed cases were rechecked
  separately after remediation. PostgreSQL remains unverified without
  `KANDEV_TEST_POSTGRES_DSN`.
- Logs are under `/home/clem/.cache/tmp/asm-deliver.Tl3lLz/`:
  `desktop.log`, `format-final.log`, `e2e-desktop.log`, `e2e-mobile.log`,
  `root-gates.log`, `root-gates-final.log`, `web-recheck.log`,
  `web-uncontended.log`, `linear-text.log`, and `remaining-gates.log`.

Final repository gates, run from the repository root after all work orders:

```bash
make fmt
make typecheck test lint
```

## Risks

- Generalizing the edit-lease check from the tail to an arbitrary target may
  miss the in-process `Service.editLeases` copy. Both the SQL and service
  checks must use the chosen target.
- The full-queue compare-and-swap currently assumes the tail. A sorted target
  needs the same CAS on its own position and content, or a concurrent edit can
  be overwritten.
- If a frontend policy-ordering helper keeps one shared revision field for both
  policies, a sort status can incorrectly preserve or overwrite merge state.
  Each tuple must be ordered on its own.
- The Auto-merge OFF-to-ON compaction amends active criterion
  `AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.5`. Task 05 must reconcile that
  criterion and the existing E2E that asserts rows stay separate (that E2E
  runs with Auto-sort-merge OFF, which is the default, so it should still
  pass).
- The term "Auto-merge" here can be confused with pull-request auto-merge.
  Copy and docs must keep the message-queue context explicit.
