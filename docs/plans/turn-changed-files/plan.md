---
created: 2026-10-07
status: in_progress
requirements:
  - REQ-TASKS-TURN-CHANGES-001
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-006
  - REQ-TASKS-TURN-CHANGES-007
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
legacy_specs: []
---

# Implementation plan: Turn changed-files cards

## Overview

Deliver an immutable repository interval beneath each captured terminal turn, with navigation into existing desktop and mobile diff surfaces.
Implement policy and durable models first, then executor capture and retention, then lifecycle ordering, APIs, and UI.
The feature is complete only after cross-executor qualification and measured performance evidence.

Implementation is authorized for this task. Execute the work orders in dependency order,
update the current work-order and manifest status after its checks pass, and record any
acceptance gaps in the final report.

## Inputs and ownership

- [Requirements](../../specs/tasks/requirements/turn-changed-files.md): REQ-TASKS-TURN-CHANGES-001 through 007.
- [System design](../../specs/tasks/system-design/turn-changed-files.md): canonical contracts and proposed operational limits.
- [ADR](../../decisions/2026-10-07-immutable-turn-change-intervals.md): confirmed immutable capture constraints.
- [Prompt generation ADR](../../decisions/0035-version-agent-ready-events-by-prompt-generation.md): existing completion identity.
- Source baseline: `4f2a7e3aa17d00b62e2282afeb28ba07f4c251c4`.
- Reference baseline: `pingdotgg/t3code` at `611132c171f3a821bd2e32f22261135cef6330ac`.

Tasks owns this vertical requirement/design pair because the durable interval belongs to a turn.
No separate frontend specification duplicates that contract.
Public documentation was updated with the feature implementation.

## Scope

In scope: persistent default-on preference, initiating-user policy, typed models, database parity, executor capture,
ordered lifecycle integration, durable content, lazy authenticated reads, transcript tree, desktop/mobile historical selection,
accessibility, localization, expiry, overlap attribution, and performance evidence.

Out of scope: delegated-task cards, delegation tools, rollback/restore/undo, new diff applications, per-tool capture,
historical backfill, completion-gate/autopilot/task-transition changes, and an off-by-default release flag.

## Technical approach

Use `show_turn_changed_files` / `showTurnChangedFiles` with pointer-aware backend decoding and nullish frontend defaults.
Reuse user settings revisions and General Save/Discard drafts.
Persist the resolved settings identity and effective policy once per admitted turn.

Add task-owned change-set/checkout/file/content relations; retain metadata separately from compressed rendering payloads.
Use replayable SQLite/PostgreSQL migrations and conditional endpoint acceptance.
Do not repurpose `GitSnapshot` or its mutable status-digest cache.

Add executor-owned private-index capture and exact-OID comparisons behind agentctl/runtime capabilities.
Use the actual attached checkout manifest and existing `GitOperatorFor` conventions.
Keep start/end capture within runtime prompt admission and terminal fences.
The existing `beforeAdmission` callback precedes prompt-generation admission, so add a generation-aware production seam before `triggerPrompt`.
Run capture I/O outside prompt/store mutexes and preserve callback lease ordering.

Before cleanup, export bounded per-file canonical/whitespace-filtered patches and old/new rendering content to durable database storage.
Historical reads never create a new execution or fall back to a current-workspace expansion fetch.
Add compact revisioned summaries to history/boot and `session.turn.changes.updated` fan-out.
Use server-owned change-set/file IDs in authenticated application reads.

Add a shared transcript projection/card/tree and a typed `HistoricalTurnDiffTarget`.
Reuse Dockview Changes and `MobileDiffSheet` with existing `FileDiffViewer`/diff adapters.
Use a shared domain query/selection model; frontend components do not fetch directly.
Keep historical review read-only and retain existing live, committed, PR, and commit-detail behavior.

## Dependency order

| Order | Work order | Depends on | Status |
| --- | --- | --- | --- |
| 01 | [Capture preference and authority](task-01-capture-preference.md) | None | done |
| 02 | [Typed change sets and database parity](task-02-change-set-persistence.md) | 01 | done |
| 03 | [Executor immutable capture](task-03-executor-capture.md) | 02 | done |
| 04 | [Durable rendering content and retention](task-04-durable-content.md) | 02, 03 | done |
| 05 | [Turn lifecycle ordering](task-05-lifecycle-ordering.md) | 01, 02, 03, 04 | done |
| 06 | [History projection and historical APIs](task-06-history-apis.md) | 02, 04, 05 | done |
| 07 | [Transcript card and desktop navigation](task-07-transcript-desktop.md) | 06 | done |
| 08 | [Mobile navigation and localization](task-08-mobile-localization.md) | 07 | done |
| 09 | [Executor qualification, performance, and docs](task-09-qualification.md) | All preceding | in_progress |

Each work order owns one reviewable result and exact validation commands.
Update its status and the manifest only after that result passes its targeted checks.
No wave in this package authorizes subagents.

## Compatibility matrix

| Environment/shape | Capture and history | Planned evidence | Failure fallback |
| --- | --- | --- | --- |
| Local checkout, linked worktree | Private executor index; separate checkout identity | Real Git fixtures and desktop E2E | Explicit checkout capture reason |
| Local Docker, remote Docker | Agentctl capture and export before container cleanup | Containers project with real daemon | Partial/unavailable content; no local-path fallback |
| SSH | Executor-owned Git and transport-bound export | SSH container harness | Unreachable/boundary-lost state |
| Kubernetes | Same capability through task Pod/PVC identity | Kind containers harness | Typed content loss; no replacement Pod snapshot |
| Sprites | Existing lifecycle transport to executor agentctl | Available sandbox integration or explicit unavailable evidence | Unsupported/unreachable state; no cross-executor completion claim |
| Plugin-owned executor | Advertised checkpoint access or explicit unsupported capability | Runtime capability contract tests and available fixture | Unsupported state |
| Multi-repository, same URL/worktrees, same relative paths | One pair per actual admitted checkout | Multi-checkout Git/API/UI fixtures | Repository-specific partial state |
| Sparse index/cone checkout | Validated reuse or safe rebuild | Real sparse Git fixtures | Unsupported rather than false deletion |
| Non-cone unsafe index, unsupported Git | Explicit unsupported capture shape | Failure fixtures | Unavailable/partial; no fabricated summary |
| Registered submodule/nested checkout | Separate declared capture; parent gitlink metadata | Real nested/submodule fixture | No recursive promise for undeclared repository |
| Authenticated, synthetic, queued, deferred, automated | Persist validated settings identity/policy | Authority tests across entry boundaries | Policy read failure disables capture, records reason |
| Passthrough or generation-zero activity | Capture only with admitted pre-provider boundary | Passthrough/runtime correlation tests | Uncaptured history when no boundary proof |

Shared code is not cross-executor validation.
Record untested environments and any acceptance failures in the final implementation report.

## ASCII UI preview

All views below are proposed UI. Counts, paths, times, and source text are illustrative.
ASCII spacing does not define pixels. Muted cards, borders, type, colors, and icons use existing Kandev tokens.
`M`, `A`, `D`, and `R` stand for labeled status icons; plus/minus values use green/red with explicit signs.
Required structure: final-reply placement, separate folder/action targets, visible Open diff, exact turn selection,
partial/expired explanations, and native phone composition.

### UI-01: General chat preference

Entry: Settings > General > chat preferences. State: saved default on.

```text
+------------------------------------------------------------------+
| Settings / General                              [Discard] [Save] |
+------------------------------------------------------------------+
| Chat preferences                                                 |
|                                                                  |
| Show changed files after each turn                     [ ON  o ] |
| Show a file summary and diff beneath completed agent replies.    |
+------------------------------------------------------------------+
```

Phone uses the existing stacked SettingsRow composition:

```text
+--------------------------------------+
| General                              |
| Chat preferences                     |
|                                      |
| Show changed files after each turn    |
| [ ON o ]                             |
| Show a file summary and diff beneath  |
| completed agent replies.             |
|                                      |
| [Discard]                    [Save]   |
+--------------------------------------+
```

Save/Discard remain in the existing settings shell. An unsaved switch is a draft.
Map: AC-001.1-.3, .6-.7. Rendered checks: preference E2E, narrow geometry, false save/reload.

### UI-02: Desktop final reply and initially collapsed tree

Entry: task transcript. State: ready changes, folders initially collapsed.

```text
Assistant
Updated validation and its tests. All focused checks pass.

+------------------------------------------------------------------+
| 4 changed files   +42  -8              [Expand all] [Open diff]   |
|------------------------------------------------------------------|
| > src/                       2 files                 +30  -5     |
| > tests/                     1 file                  +12  -3     |
| A logo.png                   Binary                             |
+------------------------------------------------------------------+
```

Collapsed tool activity, if present, stays separate and above the final reply.
Folder toggles never select a diff. Open diff opens the complete exact turn.
Header totals include known text counts; binary/unknown markers explain missing textual counts.
Map: AC-005.1-.5, .9. Rendered checks: final-anchor association, folder-only action, scroll preservation.

### UI-03: Expanded tree, multiple checkouts, and rename

Entry: expand folders. State: two contributing repository checkouts.

```text
+------------------------------------------------------------------+
| 4 changed files   +42  -8            [Collapse all] [Open diff]   |
|------------------------------------------------------------------|
| v app (feature/login)                                            |
|   v src/                     2 files                 +30  -5     |
|     M validation.ts                                 +20  -5     |
|     R input.ts                                      +10  -0     |
|       Renamed from form.ts                                      |
|   v tests/                   1 file                  +12  -3     |
|     M validation.test.ts                            +12  -3     |
| v assets (main)                                                  |
|     A logo.png               Binary                             |
+------------------------------------------------------------------+
```

Root labels use repository names plus useful branch/worktree context, never raw IDs.
A file row opens its exact checkout/file. Full paths appear on keyboard focus or pointer hover.
Phone disclosure also exposes full paths through a touch action.
Map: AC-002.5-.6, AC-005.3-.5. Checks: duplicate paths in two checkouts; renamed/binary identity; keyboard selection.

### UI-04: Desktop existing diff surface with historical scope

Entry: card Open diff or file row. State: explicit historical Turn 7.

```text
+----------------------------+-------------------------------------+
| Task transcript            | Changes                             |
|                            | [Turn 7, 14:32                 v]   |
| Assistant reply            | 4 changed files  +42 -8             |
| [changed-files card]       | [ ] Ignore whitespace               |
|                            |-------------------------------------|
|                            | app / src / validation.ts  +20 -5  |
|                            | - old validation                    |
|                            | + new validation                    |
|                            |                                     |
|                            | [Files v]                           |
+----------------------------+-------------------------------------+

Scope picker:
+-------------------------------------+
| Current changes                     |
| Latest captured turn                |
| Turn 7, 14:32                   *   |
| Turn 6, 14:10                       |
| Turn 4, 13:42 (history expired)      |
+-------------------------------------+
```

This is the existing docked surface, with its current renderer and file navigation.
An explicit Turn 7 selection remains Turn 7 after later turns.
Latest captured turn resolves once on selection; selecting it again refreshes that choice.
Current changes retains current-workspace, committed, PR, and commit-detail controls.
Historical scope omits mutation/restore controls.
Map: AC-006.1-.4. Checks: exact historical selection, same pair/totals, existing-source regression.

### UI-05: Phone transcript card and direct full-height diff drawer

Entry: same final reply. State: narrow card and historical file destination.

```text
+--------------------------------------+
| Assistant                            |
| Updated validation and its tests.    |
|                                      |
| +----------------------------------+ |
| | 4 changed files      +42  -8      | |
| | [Expand all]       [Open diff]    | |
| |----------------------------------| |
| | > src/              +30  -5      | |
| | > tests/            +12  -3      | |
| | A logo.png          Binary      | |
| +----------------------------------+ |
|                                      |
| Message...                           |
+--------------------------------------+

Tap file or Open diff:
+--------------------------------------+
| Turn 7, 14:32                [Close] |
| [Turn 7 v]                +42  -8   |
| [app / validation.ts v]              |
| [ ] Ignore whitespace               |
|--------------------------------------|
| - old validation                     |
| + new validation                     |
|                                      |
|        existing diff renderer        |
|        internally scrolling          |
|                                      |
|                safe area             |
+--------------------------------------+
```

The inline card participates in transcript scrolling; it creates no nested list scroller.
The full-height drawer has fixed context/actions and one vertical content scroll owner.
File/turn choice uses the existing inset picker drawer, not a squeezed sidebar.
Code overflow stays inside the renderer. Close returns focus to the card action.
Phone/coarse-pointer controls have measured 44px minimum hit targets.
Map: AC-005.5, AC-006.1, .5-.6. Checks: direct file open, drawer geometry, keyboard/back/focus, breakpoint transitions.

### UI-06: Availability and canceled-turn fallback

Entry: terminal transcript row. States: pending, failed, partial, expired, and no final reply.

```text
Preparing changes...

Turn changes unavailable
Could not capture the start of this turn.

+------------------------------------------------------------------+
| 2 files with available changes  +18 -4               [Open diff] |
| Partial: assets changes unavailable                             |
| > app/src/                                  +18 -4              |
+------------------------------------------------------------------+

+------------------------------------------------------------------+
| 4 changed files  +42 -8                         [Open diff: off] |
| History expired. The file summary is still available.            |
+------------------------------------------------------------------+

Turn 8 canceled, 14:40
+------------------------------------------------------------------+
| 1 changed file  +6 -2                                [Open diff] |
| M validation.ts                                 +6 -2           |
| Shared checkout: another execution overlapped this turn.         |
+------------------------------------------------------------------+
```

Pending does not keep the agent-running indicator active.
Failures never show successful zero totals. Partial Open diff navigates available files and lists unavailable repositories.
Expired navigation explains its disabled state on both keyboard and touch.
Ready zero-change, setting off, uncaptured, and no-eligible-checkout states render no card.
Map: AC-003.4-.6, AC-004.6-.7, AC-005.6-.9. Checks: availability projection and cancellation terminal anchor.

## Acceptance-to-evidence map

All identifiers below use prefix `AC-TASKS-TURN-CHANGES-`.
Test names are planned names. Implement them with TDD in their owning work order; a no-tests-matched run is not evidence.

| Criteria | Work orders | Evidence boundary |
| --- | --- | --- |
| 001.1-.3 | 01 | Store/service/DTO/hydration default and false round trips; Settings Save/Discard E2E |
| 001.4-.5 | 01, 05 | Direct, queue, deferred, automated/synthetic authority; persisted mid-turn policy |
| 001.6-.7 | 01, 05, 07 | Disabled no-capture spy; retained history off/on UI |
| 002.1-.4 | 03, 05 | Git fixture interval oracle; provider dispatch ordering and repository state snapshots |
| 002.5-.8 | 03, 06, 07 | Strict NUL parser; real special-path/rename/binary/mode/sparse/submodule fixtures; file navigation |
| 002.9 | 05, 07 | Shared-checkout interval tracking and visible attribution |
| 003.1-.3 | 05 | Blocked capture barriers, async-bus fake, duplicate/replacement/successor races |
| 003.4-.6 | 04, 05, 07 | Stop/Cancel boundedness; failed/interrupted capture; crash proof; terminal pending UI |
| 004.1-.2 | 02, 06, 07 | SQL replay/read, conversation pagination/revisions, final assistant association |
| 004.3-.4 | 06 | Authenticated API denial and ownership tests; compact payload assertions |
| 004.5-.7 | 04, 06, 09 | Export then executor destruction; restart; eviction; partial content; real executor matrix |
| 005.1-.4 | 07 | Shared projection/tree units and desktop integrated transcript/file action E2E |
| 005.5 | 07, 08 | Reloaded expansion/selection; keyboard; full path disclosure; touch geometry |
| 005.6-.9 | 07, 08 | Availability and scroll-position matrix on desktop/phone |
| 006.1-.4 | 07, 08 | Existing diff surfaces, exact scope, whitespace totals, source compatibility |
| 006.5-.6 | 08 | Phone drawer geometry/focus, light/dark and locale checks |
| 007.1-.2 | 03, 04, 06 | Admission/output/deadline/export bounds and no whole-checkout copies |
| 007.3-.4 | 09 | Reproducible capture benchmarks, retention bytes, environment and limitations report |

## Verification strategy

Each work order specifies exact commands and planned new test names.
Use direct `go test -trimpath`, targeted Vitest, and guarded `pnpm e2e:run` commands from the correct directories.
Install dependencies once from `apps/` if the worktree lacks them.
The managed E2E runner rebuilds the tested UI/backend; do not use stale artifacts or all-worker overrides.
Run real PostgreSQL tests with `KANDEV_TEST_POSTGRES_DSN`; skipped tests do not prove database parity.
Run container suites sequentially with `KANDEV_E2E_CONTAINERS=1` and the owning project.

After docs changes, run catalog validation and specification lint.
After implementation, run web typecheck and localization checks once as specified by work order 08.
Broaden tests only for new failures or unresolved concerns. No automatic broad review/QA/verify phase is added.

## Design choices and risks

Confirmed: fresh endpoints, multi-checkout capture, exact OIDs, default-on missing settings, explicit-false preservation,
retained history across off/on, existing diff surfaces, interrupted-turn capture, and unchanged workflow authority.

Design-selected proposals: database-backed compressed rendering content; 30-day content retention;
1 GiB installation/128 MiB task budgets; 10-second start/15-second terminal/2-second cancel preservation bounds.
These defaults are reviewable and require measurements in order 09.
They are not an established performance promise.

Primary risks:

- Completion callbacks currently hold generation locks; external I/O must not create a reentrant deadlock.
- Queued/deferred launch identity must survive background dispatch without confusing reserved labels or assignment with the settings user.
- Sparse/manual flags, conflict stages, path encoding, filters, and gitlinks need real Git fixtures.
- Content export can exceed bounds or fail during remote loss; partial/unavailable states must remain truthful.
- Retained patches increase database and backup size; measure deduplication and budget eviction.
- Transcript anchoring and height changes must preserve reader position across pagination and final-output races.
- Availability on Sprites or plugin executors requires actual capability evidence and available integration environments.

## Implementation and validation status

Work orders 01–08 are implemented and validated. Work order 09 remains in progress because several executor environments and proposed performance dimensions have not been qualified; the plan remains `in_progress` until those gaps are reviewed and resolved.

Passed checks:

- `cd apps/backend && go test -trimpath ./...`: all Go packages passed.
- `cd apps/backend && go test -trimpath -race ./internal/orchestrator ./internal/agent/runtime ./internal/agent/runtime/lifecycle ./internal/task/changes -run 'TurnChange|DispatchCompletion|SendPromptSteer|SessionTurnSettlement|Cancel' -count=1`: passed in the orchestrator and lifecycle packages; the other two packages reported no matching tests. The complete `internal/task/changes` race suite and the new checkout-manifest lifecycle regressions also passed separately.
- `cd apps/web && pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`: passed.
- `cd apps/web && pnpm test`: 2,687 files passed; 23,689 tests passed and 4 skipped.
- Desktop Chromium changed-files and setting E2Es and mobile-chrome changed-files and setting E2Es: 2 passed in each project.
- Docker executor retention after container removal and backend restart: 1 passed.
- SSH executor content retention after remote checkout removal: 1 passed.
- Local capture/export benchmark: six cases completed with ten observations each; see [performance results](performance-results.md).
- Documentation, public-doc, and specification validation passed during implementation.

Qualification gaps remain open. Kind-backed Kubernetes, Sprites, and plugin executors have no feature-specific passing E2E evidence. The Kubernetes E2E seed currently creates no Git repository, so its Git capture path needs a repository-capable fixture. The local benchmark excludes executor transport, SQL writes, compression and retention growth, cold/slow storage, size-limit failures, and Stop/Cancel bounds. PostgreSQL-specific tests were not run because `KANDEV_TEST_POSTGRES_DSN` is unset. These are acceptance gaps, not claims of support or performance.

## Source inventory

Verified current boundaries:

- `task/models.Turn` has task/session identity, execution profile, route generation, timestamps, and metadata.
  It does not contain immutable content endpoints.
- `task/service.ReserveTurn`, `MarkReservedTurnDispatchAttempted`, and `PublishReservedTurn` separate reservation from dispatch acknowledgement.
  `turn.started` can follow provider dispatch. Capture cannot rely on that subscriber.
- `orchestrator.startTurnForSessionWithOwnershipChecked` adopts existing turns and binds accepted dispatch identity.
- `lifecycle.SessionManager.sendPrompt` waits for the prior prompt, prepares attachments, admits a generation, then calls `triggerPrompt`.
  The existing `beforeAdmission` callback runs before generation admission; the test-only `beforePromptDispatchHook` is not a production contract.
- `lifecycle.handleCompleteEventLeased` captures turn identity and uses `claimPromptCompletion` before readiness and completion publication.
  Numbered completion currently holds `promptLifecycleMu`; synchronous bus callbacks can reacquire other lifecycle locks.
- `handleCompleteStreamEventWithGuardRelease` persists final output and captures fresh mutable Git status.
  `finishAgentCompleted` can drain queued work before the existing exit-path status capture and cleanup.
- `user.Service.settingsUserID` resolves an authenticated non-synthetic identity, otherwise the default settings user.
  This settings identity is separate from actor attribution.
- `user/store/sqlite.go` stores settings in a JSON payload and already uses pointer booleans for missing-value defaults.
  Its dialect branches support PostgreSQL as well as SQLite.
- `GitOperatorFor` resolves repository subpaths; runtime owns actual executor access and environment identity.
  `workspace_git_index.go` can hard-link a read-only index snapshot. A writable capture index must use a separate inode.
- `DiffSheetMode`, `OpenDiffOptions`, and `DiffSource` currently cover live and commit targets.
  `MobileDiffSheet` already uses a Drawer; `FileDiffViewer` and `lib/diff` provide reusable rendering.

This inventory records the inspected pre-implementation boundaries; implementation details and current validation are recorded above and in the work orders.


## Reference inspection

The sibling checkout was absent. All fourteen requested files were fetched and inspected at pinned revision
[`611132c`](https://github.com/pingdotgg/t3code/tree/611132c171f3a821bd2e32f22261135cef6330ac).

| Reference files | Adopted idea | Kandev adaptation |
| --- | --- | --- |
| GitVcsDriver.ts; VcsProcess.ts | Private index, safe copied-index checks, object/ref durability, bounded process admission and retries | Fresh endpoint per turn/checkout; exact OIDs; no mutable-ref or HEAD fallback |
| CheckpointStore.ts; CheckpointService.ts | Executor-local capability, checkout lock, compact numstat summary | Multi-checkout manifest; real kinds; binary/null counts; failure distinct from zero |
| RunExecutionService.ts; CheckpointCaptureService.ts | Pre-provider baseline; idempotent terminal processing; stopped-turn capture | Synchronous runtime fence; preserve Kandev completion authority; cover failed turns too |
| CheckpointDiffQuery.ts; Diffs.ts | Range validation, lazy patch query, NUL path/rename parsing | Server-owned change-set/entry IDs; immutable pair; retained content after teardown |
| threadCheckpoints.ts; MessagesTimeline.logic.ts; MessagesTimeline.tsx | Summary-to-final-assistant association and footer outside activity | Durable turn/message IDs; terminal fallback row; pagination/revision protection |
| ChangedFilesTree.tsx; turnDiffTree.ts; DiffPanel.tsx | Folder aggregation, expansion, header/file diff selection | Checkout roots, exact path handling, Kandev primitives, native mobile drawer |

Do not copy Effect/TypeScript service architecture into Go.
Do not adopt endpoint reuse, binary-to-zero conversion, all-modified classification, successful-empty diff failure, or missing-ref fallback.
