---
created: 2026-10-05
status: complete
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-003
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-004
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-005
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
  - ../../specs/tasks/system-design/managed-clone-relocation-experience.md
legacy_specs: []
---

# Implementation plan: Managed clone recovery experience

## Overview

Keep recovery artifacts out of working files and show server-owned progress
across reloads. Preserve existing relocation authority and every retained copy.
Deliver private storage first, then durable operation projection, then shared
desktop and phone presentation. This order gives the UI an authoritative source.

This package contains requirements, a system design, an ADR, and three work
orders. Implementation is complete across Tasks 01-03. Backend and frontend
checks, production build, desktop and phone recovery E2E cases, localization,
and documentation validation passed. The configured PostgreSQL gate remains
blocked because `KANDEV_TEST_POSTGRES_DSN` is unset.

## Confirmed failure and root cause

The observed task was [demo landing page](https://kandev.cfl.tools/t/56245922-751c-4e9f-98b7-1c9ce656dc13).
Its explicit relocation started at 12:04:30 Lisbon on 2026-10-05. It used
operation `2df82a80-6069-4de1-84a0-07ba585fe428` for two repository slots.
Both migrations finished by 13:01:56. The agent was ready at 13:02:08.

Read-only logs and filesystem checks established these facts:

- The first checkout contained about 136,000 files and 4.6 GB of content.
  The second contained about 96,000 files and 1.5 GB. Ignored dependencies
  formed a large part of those trees.
- The server continued snapshot verification and restoration after the original
  browser disconnected at 12:14:37. A reopened page offered repair again.
- The new page lost the browser-local pending map. Its persisted relocation
  error did not describe the still-running server operation.
- Background workspace requests logged inspection contention. Those failures
  did not prove that the accepted migration failed.
- Files showed adjacent snapshots, old failed snapshots, journals, and claim
  files alongside suffixed active checkouts. Originals were retained privately.

The source confirms two mechanisms: `session-recovery-pending.ts` uses a
module-local map, and `managed_clone_relocation.go` writes adjacent artifacts.
WebSocket closure rejects browser promises without proving server termination.
The copier preserves ignored files and performs repeated integrity checks.
This package addresses status and clutter; it makes no copy-speed promise.

### Requirement conformance and settled assumptions

The existing REQ-001/-002 preservation and authority contracts remain valid.
REQ-003 covers recovery choice and error persistence, but lacks live progress.
The draft extension adds REQ-004/-005 within the same task-owned capability.
No separate UI system or generic repair specification is introduced.

The user requested a fix package after migration diagnosis. Preservation is
already required. The package assumes repository names are display labels and
server liveness controls repair actions. Automatic deletion, copy optimization,
and manual changes to the affected live task are excluded. No material question
remains before planning.

### Minimal reproduction for implementation

Use the existing real-Git two-repository helper. Change both managed sources,
make a checkout dirty, and confirm relocation. Hold its small fixture at the
snapshot phase with an isolated test-only barrier. Reload or disconnect that
browser and open the same session in another browser. The current build loses
the pending indicator while the server owns the recovery claim. Release the
barrier and inspect the completed Files tree. No large dependency tree is needed.

## Scope

### In scope

- Private storage for new clone-relocation snapshots, journals, and locks.
- Versioned legacy readers and verified artifact ownership registration.
- Exact exclusions in Files and workspace search, with canonical file paths.
- Repository display labels and inventory-aware cache invalidation.
- Durable progress, read-only status lookup, reconnect hydration, and live guards.
- Honest failure, interruption, all-slot completion, and agent readiness states.
- Shared desktop/phone recovery UI, localization, and public documentation.

### Out of scope

- Automatic artifact deletion, expiry, or bulk movement of legacy backups.
- A backup management UI, generic metadata-recovery redesign, or new executor support.
- Copy optimization, skipping ignored trees, hardlinks, or weaker manifests.
- New cancellation behavior, autonomous retry, session states, or runtime toggles.
- Delegation, live repair, commits, deployment, or PR creation in this design turn.

## Technical approach

### Artifact ownership and compatibility

Task 01 owns the versioned private layout in `internal/worktree`, the injected
artifact registry, its schema/repository support, and runtime exclusions.
`managedCloneRelocationArchivePath` remains the private namespace anchor.
Extend journal discovery in `reconcilePublishedManagedCloneRelocation` and dirty
transfer before changing new snapshot placement. Keep legacy mode retry paths.

Use exact registry-owned paths for tree and search filtering. Apply exclusions
before traversal in agentctl, not by suffix matching in React. Refresh them
without restarting a live runtime. Keep unverified legacy artifacts visible.

### Durable operation contract

Task 02 owns `task_environment_recovery_operations`, fenced writes, runner
liveness, and phase reporting around existing manager calls. Keep recovery
claims authoritative and terminal projections available after claim release.
Distinguish migration completion from resume readiness in multi-slot operations.

Extend `TaskSessionDTO`, boot/session hydration, events, session notifications,
and web session projection merge. Add `session.workspace_recovery.get` and
`session.workspace_recovery.changed`. Status reads must bypass lifecycle workspace
reconstruction. Preserve the existing synchronous recovery action contract,
with an additive `in_progress` duplicate result.

### Shared presentation

Task 03 connects durable status to the recovery hook/model and all current
recovery surfaces. Keep the local pending map only for pre-acknowledgment taps.
Reconcile lost responses through a read-only status request. Show repository,
phase, last update, preserved-copy explanation, and terminal outcome.

Derive file labels from canonical worktree inventory. Include authoritative
environment/inventory revisions in Files and search cache fences. Add localized
copy in all seven locales. Update `docs/public/developer-tools.md` for working
files and retained artifacts, and `docs/public/executors.md` for recovery behavior.
Public docs change during implementation, when behavior is verified.

### Compatibility matrix

| Shape | Intended behavior | Evidence | Refusal or fallback |
| --- | --- | --- | --- |
| Host Worktree, new dirty GitHub/GitLab managed relocation | Private artifacts and durable progress | Real-Git manager tests; browser real-Git fixture | Existing source/claim/manifest proof |
| Host Worktree, clean or unchanged legacy slot | Preserve admission; report selected-slot progress when part of an operation | Existing clean/legacy tests and mixed-slot regression | No registry work for unchanged standalone reuse |
| Version 1 adjacent journals, including mode retry | Same operation and paths; verified registration only | Legacy restart and permission tests | Ambiguous artifacts remain visible; blocked adoption stays refused |
| Multiple slots with one failed sibling | Preserve earlier publications; no agent startup | Mixed-state integration and desktop/phone E2E | Whole workspace remains incomplete |
| Backend restart or lost browser response | Reconcile runner status without replay | Repository/service tests and reconnect E2E | Interrupted or unresolved status, never invented liveness |
| Local, Docker, SSH, Kubernetes, plugin remote | Existing behavior; no host relocation inspection | Non-Worktree early-return regression | No new recovery capability |
| Older server without projection fields | Existing recovery compatibility | Web merge/action tests | Missing optional field cannot clear newer state |

GitHub and GitLab share origin proof. Browser evidence uses the existing GitHub
fixture; GitLab proof is backend evidence. Do not claim provider-wide E2E coverage.

## ASCII UI preview

Copy is illustrative and must be localized. Grouping, action guards, status
hierarchy, and phone geometry are required. Existing components and tokens own
spacing. UI-01 maps to AC-005.1-.8; UI-02 maps to AC-004.1-.5.

### UI-01: Recovery card, active operation after reopen

Entry point: the existing task/session recovery surface.

```text
Desktop
+-------------------------------------------------------------+
| Moving workspace files                                      |
| Repository 2 of 2: kdlbs-landing                             |
| Verifying saved files                                       |
| Last update: 13:00                                          |
| Original files and backups remain preserved.                |
| [Move files and resume: disabled]     [Technical details v]  |
+-------------------------------------------------------------+

Phone: focused recovery card in the existing session view
+----------------------------------+
| < Task                           |
| Moving workspace files           |
| Repository 2 of 2                 |
| kdlbs-landing                    |
| Verifying saved files            |
| Last update: 13:00                |
| Original files and backups       |
| remain preserved.                |
| [Move files and resume: disabled]|
| [Technical details v]            |
+----------------------------------+
```

The card uses the existing scroll owner. Do not nest a new full-height pane.
Phone controls have 44-pixel targets; desktop keeps normal compact controls.
The repository name wraps or truncates with an accessible full label. The
confirmation stays a desktop dialog and a phone inset drawer. Cancellation
returns focus without changing operation state. No new cancel button is added.

### UI-01 states: status, migration complete, and refusal

```text
Connection unresolved: Checking recovery status...
                       Repair actions disabled; no automatic retry.
Migration complete:    Workspace files moved. Resuming agent...
                       Repair actions disabled until resume outcome.
Ready:                 Workspace ready. Agent ready for input.
                       Active recovery card retires; history remains.
Interrupted:           File migration stopped. Files remain preserved.
                       [Move files and resume] only if stamped proof permits.
Failed verification:   Workspace needs repair. Files remain preserved.
                       [Technical details v] and current safe guidance.
Resume failed:         Files moved. Show the actual resume error and its actions.
```

The interrupted state must not promise every operation is retryable. Active
progress replaces the actionable old failure presentation without erasing history.
Announce phase changes politely; suppress announcements for heartbeats.

### UI-02: Files after publication

Current source and screenshot show raw artifact and replacement names. The
proposed desktop tree and focused phone Files view show repository labels:

```text
Current desktop root              Proposed desktop Files
  .agents/                          .agents/
  .codex/                           .codex/
  repo.kandev-recovery-.../          kdlbs-kandev/
  repo.relocated-.../                kdlbs-landing/
  repo.kandev-recovery.json          notes.txt (user root file)
  repo.kandev-recovery.claim

Phone: existing Files view
+----------------------------------+
| < Task                 Files     |
| Search files                     |
| > .agents                        |
| > .codex                         |
| > kdlbs-kandev                   |
| > kdlbs-landing                  |
|   notes.txt                      |
+----------------------------------+
```

Files labels do not change file paths. Recovery details explain retained copies.
Unverified old artifacts and user lookalike names remain visible. Expanding a
repository, opening a file, searching, and attaching it work on both viewports.
Phone keeps its existing focused navigation and scroll area; no desktop pane
stack is added. Test long names and both sides of the responsive breakpoint.

## Tests

All new test names below are planned, not existing passing evidence. Implement
with TDD. Update expectations in existing companion suites only where version 2
placement changes; retain their version 1 fixtures and safety assertions.

| AC coverage | Test file and planned test or existing suite |
| --- | --- |
| 004.1; existing 001.2, 002.2 | `internal/worktree/managed_clone_relocation_artifacts_test.go`: `TestManagedCloneRecoveryPrivateArtifactsPreserveContent` |
| 004.3; existing 002.5 | Same file: `TestManagedCloneRecoveryLegacyJournalAndModeRetryContinuity`; existing mode-retry suites |
| 004.3 | Same file: `TestManagedCloneRecoveryArtifactRegistryRejectsSubstitution`; `internal/agentctl/server/process/workspace_tree_read_test.go`: `TestWorkspaceTreeRecoveryExclusionsKeepLookalikes`; new `workspace_recovery_search_test.go`: `TestWorkspaceSearchRecoveryExclusions` |
| 004.2, .5 | `apps/web/components/task/file-browser-repository-labels.test.ts`: canonical paths, duplicate labels; `file-browser-path.test.ts` and `file-browser-search-freshness.test.ts`: replacement fences |
| 005.1, .4; existing 001.3, 002.4 | `internal/orchestrator/managed_clone_recovery_progress_test.go`: `TestWorkspaceRecoveryProgressAllSlotsAndResumeOutcome` |
| 005.2, .3, .6 | `internal/task/repository/sqlite/task_environment_recovery_operations_test.go`: `TestRecoveryOperationProjectionCASAndClaimRelease`; conditional `task_environment_recovery_operations_postgres_test.go`: `TestPostgresRecoveryOperationSerializesWriters` |
| 005.5 | `internal/task/service/workspace_recovery_progress_test.go`: `TestWorkspaceRecoveryInterruptedRunnerDoesNotExpireClaim` |
| 005.2, .6, .8 | Same service file: `TestWorkspaceRecoveryStatusReadDoesNotBootstrap`; `internal/backendapp/status_summary_boot_test.go`: `TestWorkspaceRecoveryBootProjection`; `internal/gateway/websocket/session_notifications_test.go`: `TestWorkspaceRecoveryNotificationParity` |
| 005.2, .3, .6 | `apps/web/lib/state/slices/session/workspace-recovery-projection.test.ts`: stale operation/generation/revision, hydration omissions, orphan events; action and recovery-model suites |
| 004.4, 005.7 | Recovery component tests plus rendered desktop and phone evidence below |

Retain refusal tests for wrong origins, foreign tasks, slot drift, live runtimes,
claims, modes, ignored content, and unsupported executor early returns. Never
replace real-Git integrity proof with a mocked progress emitter.

## E2E tests

Extend these existing suites and their shared helper:

- `apps/web/e2e/tests/session/multi-repo-session-resume-recovery.spec.ts`, project `chromium`.
- `apps/web/e2e/tests/session/mobile-multi-repo-session-resume-recovery.spec.ts`, project `mobile-chrome`.
- `apps/web/e2e/helpers/multi-repo-managed-clone-recovery.ts` for isolated seeds,
  phase barrier, request counts, retained-copy checks, and cleanup.

| Planned scenario in both suites | AC coverage |
| --- | --- |
| `reopens an active migration without offering another transfer` | 005.1-.3, .6, .8 |
| `reconciles a lost response and another browser` | 005.2, .3, .6 |
| `keeps migration incomplete when a later repository fails` | 005.4, .5; existing 002.4 |
| `shows interruption without claim eviction or automatic replay` | 005.5, .8 |
| `distinguishes files moved from agent readiness` | 005.4, .6 |
| `shows repository labels with canonical search and attachment paths` | 004.1-.5 |
| `keeps legacy artifacts and user lookalikes safe` | 004.3, .4 |

The helper must expose a deterministic isolated test-only barrier at the actual
phase boundary. Test backend restart with the existing isolated fixture lifecycle.
No production route, runtime flag, or file-count workload is added. Include
selected mixed states: one unchanged healthy slot, one relocating slot, and a
failing sibling. Assertions cover every selected slot before readiness.

Phone tests use tap interactions, the inset confirmation drawer, 44-pixel hit
areas, viewport containment, no horizontal overflow, one scroll owner, and long
labels. Check layout near the responsive breakpoint and focus return on cancel.
Desktop tests check compact control dimensions and keyboard status behavior.

## Work orders

- [completed] [Task 01: Own recovery artifacts outside working files](task-01-private-artifacts.md)
- [completed] [Task 02: Persist and project recovery operation progress](task-02-durable-progress.md)
- [completed] [Task 03: Present recovery progress and working repositories](task-03-recovery-presentation.md)

Run sequentially. A work order does not authorize delegation or a model switch.

## Companion packages

| Package | Current status | Relationship |
| --- | --- | --- |
| [Original relocation](../managed-clone-relocation/plan.md) | complete | Preserve clean/dirty transfer, identity, and retained copies |
| [Legacy clone resume](../legacy-clone-resume/plan.md) | implemented | Preserve unchanged legacy reuse |
| [Recovery convergence](../managed-clone-recovery-convergence/plan.md) | done | Extend its durable error and recovery surfaces with operation progress |
| [Snapshot permissions](../workspace-recovery-permissions/plan.md) | complete | Retain permission proof and same-operation blocked retry |

This package does not reopen those completed work orders or overwrite their
historical results. Their relevant tests remain regression inputs. Record new
counts here after implementation and reconcile any changed scenario references.

## Verification results

Design-package checks on 2026-10-05:

- `python3 scripts/list-docs.py validate`: passed, 351 decisions and 1,347 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Package consistency check: passed for seven new files. Local links resolve,
  fenced blocks balance, and every work-order AC exists in its owning requirement.
- `git diff --check`: passed.

Product verification on 2026-10-06:

- Task 01 required Go package gate passed for worktree, SQLite task repository,
  agentctl process, and agent handlers. The orchestrator executor package also
  passed.
- Task 02's filtered Go run recorded 48 matching tests passed and 6 skipped.
  The gateway WebSocket and agent handler packages had no matching tests.
- Workspace recovery projection tests passed, 6/6. The focused recovery web
  suites passed, 177/177 tests; the three changed recovery suites passed,
  94/94 tests. Web typecheck and full ESLint passed.
- `i18n:zh-hant`, `i18n:check`, and `i18n:ratchet` passed.
- The production Vite build passed. It emitted existing chunk-size and
  ineffective dynamic-import warnings.
- Desktop multi-repository recovery passed 2/2; phone multi-repository recovery
  passed 2/2. The targeted single-repository desktop and phone cases each
  passed 1/1. E2E runs rebuilt the backend and Vite E2E bundle.
- Public documentation tests passed, 62/62, and all 47 published pages
  validated. Catalog validation passed for 351 decisions and 1,347 specs;
  specification lint tests passed 36/36; all specs passed the linter; and
  `git diff --check` passed.
- The PostgreSQL concurrency gate was not run because
  `KANDEV_TEST_POSTGRES_DSN` is unset. This remains a blocked environment gate,
  not passing PostgreSQL evidence.

Review corrections on 2026-10-06:

- Recovery UI now associates terminal readiness with the active error stamp and
  unfinished workspace migration. A later ordinary stop keeps resume and fresh
  start choices, while a matching successful repair card retires.
- Progress cleanup uses a bounded context independent of browser cancellation,
  retries terminal persistence after a transient write failure, and lets an
  exact authorized retry settle a dead same-process attempt without expiring its
  claim.
- WebSocket recovery notifications accept both memory-bus string slices and
  NATS JSON-decoded arrays. Sibling sessions accept newer same-environment
  attempts while rejecting foreign scopes and stale generations.
- Automatic clean relocation no longer creates explicit repair progress that
  can be marked as a resume failure. Runner liveness follows immutable attempt
  ownership across projection revisions.
- Legacy exclusions require bound relocation and recovery journals, snapshot
  namespace and retry proof, and a current filesystem identity. Existing path
  registrations cannot silently bless a replacement object.
- Focused Go recovery regressions passed across DTO, service, worktree, SQLite,
  WebSocket, agent handler, and agentctl process packages. Service and worktree
  recovery regressions also passed under `go test -race`.
- The focused web recovery suites passed 196/196 tests. Full ESLint, TypeScript
  typecheck, and the production Vite build passed. Localization checks, catalog
  validation, spec lint, public-doc tests (62/62), all 47 published-page
  validations, and `git diff --check` passed.
- The PostgreSQL gate remains unavailable because
  `KANDEV_TEST_POSTGRES_DSN` is unset; no PostgreSQL pass is claimed.

## Risks

- Weak artifact authentication can hide user content. Uncertain entries remain visible.
- Moving locks or losing legacy journal discovery can strand a retained operation.
- Persisted running state can outlive a runner. Restart reconciliation must retain claims.
- Per-file progress writes can slow copying. Report phases and bounded heartbeats only.
- A late hydration or action response can re-enable repair against a newer operation.
- Large snapshots remain slow and disk-heavy. This package does not reduce their cost.
