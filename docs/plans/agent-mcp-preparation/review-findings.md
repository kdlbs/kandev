# Agent MCP preparation implementation review

Scope: local working-tree implementation over HEAD
`2ea738e16e6872abd468adde4c3e7458e10c40ca`, including untracked files.
This is not a review of the remote PR head. Preexisting Cursor discovery/auth
repair changes are retained and form the implementation baseline.

## Review status

Code review is complete for the working-tree snapshot. The 20 findings below
were remediated through the workers and reviewed again by the coordinator.
No known feature-code blocker remains. This is not an unconditional merge-ready
verdict: desktop/mobile browser acceptance is unverified because Chromium cannot
launch in this sandbox, and broader backend suites have the failures listed below.

## Design issues addressed before implementation

- Preparation attempt UUIDs require an ordering marker: delayed unseen old
  progress/completion must not replace successor state. The design and worker
  contracts require ordered markers and stale-event tests.
- Native approval must preserve the exact emitted server identity, recheck
  source/task disables and final ownership, and remain within the workspace
  generation boundary. Lowercased ownership keys are not CLI identifiers.
- Credential provenance must preserve whole native-refreshed objects without
  allowing a removed source or unverified old master to become authoritative.

## Implementation findings

1. Native readiness fixtures initially assumed JSON output
   (`apps/backend/internal/agent/mcpconfig/cursor_native_mcp_test.go:44`).
   The installed Cursor CLI actually emits `Tools for <id> (<count>):` followed
   by text rows. A parser tested only against invented JSON could reject every
   real successful connection or accept arbitrary exit-zero output. The runtime
   worker was given the captured synthetic fixture output and asked to test the
   observed header/identity/count and actual unapproved diagnostic. Resolved: installed-version text fixtures and unknown-success-output rejection
   are implemented; native adapter targeted tests pass.

2. Successful native output was classified by broad error substrings before
   its success header (`cursor_native_mcp.go`, `verify`/`classifyNativeMCPText`).
   A legitimate tool named `is_disabled` or `handle_401` could make a working
   server appear unapproved or unauthenticated. Parse verified successful output
   first and classify only failed command diagnostics with observed markers.
   Resolved: success-header parsing precedes failure classification; marker
   false-positive regressions pass.
3. The native runner used `exec.CommandContext` without bounded pipe draining
   or descendant cleanup (`cursor_native_mcp.go`, `ExecNativeMCPCommandRunner.Run`).
   A spawned stdio MCP child can retain stdout after its parent is killed,
   causing `Run` to outlive its context. Use existing process cleanup patterns
   and bounded waiting; add a child-holds-pipe cancellation regression.
   Resolved: bounded pipe draining and owned process-tree cleanup are implemented;
   descendant-held-stdout cancellation regression passes on Unix. Windows uses
   the existing kill-on-close job helper; native Windows runtime remains untested here.

4. The new progress reducer test named for agent-readiness gating only emitted
   a progress event (`apps/web/lib/ws/handlers/executor-prepare.test.ts:148`).
   It did not exercise the component's agentctl-ready fallback, which could
   still mark an unfinished attempt complete. Requested a production-shaped
   component regression with agentctl ready and no currently running row while
   final preparation completion remains pending. Resolved: `deriveStatus` is
   tested with agentctl ready, a stamped preparing attempt and no running rows;
   it stays preparing until completion.
5. Profile duplication omitted the existing import toggle in `duplicateClone`
   (`apps/backend/internal/agent/settings/controller/profile_crud.go`).
   Copying selected IDs without the import gate changes duplicated behavior.
   Resolved: clone now copies both Cursor toggles and selected IDs; duplication,
   migration rebuild, contract and runtime resolver tests pass. SQLguard and
   persistence store-conformance race checks passed.

6. Discovery initially checked only ACP runtime strategy, hiding the controls for
   Cursor terminal agents. It also introduced a workspace-ownership restriction
   that runtime imports did not enforce. Resolved: discovery uses both registered
   ACP and passthrough capabilities; tests cover both Cursor types.
7. Preparation persistence could accept malformed or tied successor markers,
   and repeated progress performed redundant database writes. Resolved in the
   current implementation: valid timestamps fence legacy/malformed completions,
   equal timestamps require the same attempt ID, unchanged markers skip writes,
   and metadata operations have bounded contexts. Focused checks pass; broad-suite limits are recorded below.
8. `mergePreparationAttemptResult` treated failed optional MCP verification as a
   fatal workspace failure, contradicting degraded preparation. It also failed to
   copy fatal errors into the execution's stored result. Resolved: optional MCP failures preserve environment success, while fatal
   materialization failures reach both stored results and completion events.
   Focused regression tests pass; focused checks pass; broad-suite limits are recorded below.
9. Credential preparation wrappers reported completion even when the underlying
   bridge logged and swallowed a failure or skipped an ineligible context.
   Resolved: explicit completed/skipped/degraded states carry sanitized reasons.
   Tests cover disabled reuse, runtime HOME isolation and bridge failure.
10. Cursor config symlink skips and ignored ownership publication errors could
    report successful materialization without a valid owned configuration.
    Resolved: unsafe parent/leaf paths and failed ownership publication are
    launch-blocking errors with focused regressions.
11. Native executable-start failures were converted to truncated command output,
    losing the unavailable classification. Executable lookup also assumed Unix
    executable bits and omitted Windows executable extensions. Requested bounded,
    platform-aware lookup and preservation of the sanitized unavailable sentinel.
    Resolved: runtime PATH (including an explicit empty override), Windows
    PATHEXT and platform executable rules are covered; start failures preserve
    the sentinel. Package tests, race tests and Windows cross-compilation pass.

12. The ordinary terminal service persisted an initial command but its PTY
    registration discarded it. An Authenticate endpoint that only called Create
    would open a blank shell rather than execute native login. The ordinary
    terminal list also serialized that command. Requested backend-only command
    retrieval/registration across create and reattach, command redaction at the
    client projection, and real service/runner regressions. Resolved: the gateway
    retrieves the trusted persisted command; ordinary terminal API projections
    omit it. A real HTTP/PTY regression executes the command exactly once.

13. The first recovery draft validated eligibility before acquiring its runtime
    lock, then enabled the server without sharing the workspace preparation
    boundary. Selection, disables or ownership could change before approval.
    Requested current session admission, current policy/ownership validation under
    the preparation boundary, retry progress persistence, and TUI capability
    coverage. The implementation now revalidates admission, current policy,
    disables and ownership at native boundaries, retains other servers in retry
    progress, and fails closed for unsupported TUI reload. Focused recovery tests, generation-barrier regressions and race checks pass.

14. Reusing any open shell after a native login exits strands the user at a
    prompt. Conversely, terminating the shell with `exec` interacts with the
    browser's unconditional socket reconnect and can automatically execute the
    saved login command again. Requested explicit one-shot attempt state:
    reconnect never starts a second OAuth login; only a new Authenticate action
    does. Pending/running attempts remain deduplicated independently of socket
    lifetime. Resolved with an atomic persisted one-shot claim, suppressed runner
    fallback, and a typed normal socket close for completed attempts. A real
    Authenticate/PTY/reconnect/Authenticate regression verifies two explicit
    attempts execute exactly twice and reconnect executes no extra command.

15. A real Cursor 2026.09.26 fixture run emitted `MCP '<id>' requires
    authentication.` followed by `Please run: agent mcp login <id>`. The parser
    accepted only failure-prefixed diagnostics, so a true login requirement was
    classified as connection failure and hid Authenticate. Resolved: the adapter
    recognizes the exact observed server-specific message; mismatched IDs fail
    closed. Versioned fixture tests and the first stage of the native smoke pass.
    Earlier fixture HTTP 405/404 errors were correctly classified as connection
    failures and were fixed in test infrastructure, not production classification.
16. Discovery refresh errors discarded previously loaded server metadata. The
    desktop browser test exposed the disappearing row. Resolved: the hook retains its last successful response within the same
    profile, with focused regression coverage. Browser execution is blocked before assertions by the host Chromium launch restriction.

17. Native Cursor acknowledged same-process `session/load` after credential
    repair but kept the failed MCP client cached. The subsequent fixture prompt
    had no fixture tool; this disproved mocked LoadSession-success readiness.
    Recovery must restart the ACP child, initialize it, and load the exact saved
    ACP conversation ID without creating a new session or replaying a prompt.
    The isolated replacement-child native proof passes, preserving the saved
    conversation and restoring fixture tools. Production recovery now performs
    stop/configure/start/initialize/exact-load; focused replacement-process and failed-load regressions pass.

Native evidence passes; browser execution limits are recorded in the final validation section.

18. Terminal launch failure cleanup was registered conditionally but did not
    recheck the success flag inside the deferred closure
    (`apps/backend/internal/gateway/websocket/terminal_handler.go`,
    `startUserShellProcess`). It removed attempt state even after a successful
    start. Requested a flag check inside deferred cleanup and a successful-start
    regression. Resolved: deferred cleanup rechecks the flag before removing
    attempt state; terminal-focused checks pass.

19. A lint-driven recovery refactor indexed resolved candidates by the native
    server ID, but the existing candidate map uses the lowercase configuration
    name (`cursor_plugin_mcp.go`, `resolveAndAddImportCandidate`). This would
    reject valid plugin recovery. The same extraction began rejecting inventory
    errors that previously permitted partial/local discovery. Requested exact
    native-name iteration and preservation of partial inventory behavior, with
    regression coverage before the final freeze. Resolved: exact-name iteration
    and partial inventory behavior are restored; mixed-case native-ID recovery
    tests and post-refactor race checks pass.

20. Fresh-launch and worktree-resume tests did not prove completion ordering on
    the separately changed workspace-promotion and terminal start/resume paths
    (`manager_launch.go` and `manager_passthrough.go`). Requested narrow
    path-level regressions proving MCP preparation finishes before the aggregate
    completion event. Resolved: the promotion and start/resume tests block
    discovery, assert no completion during the block and verify final MCP
    progress afterward. The focused tests and final lifecycle lint pass.

## Current implementation anchors

These anchors identify the reviewed and repaired boundaries in the working tree.

| Finding | File and line |
| --- | --- |
| 1 | `apps/backend/internal/agent/mcpconfig/cursor_native_mcp.go:295` |
| 2 | `apps/backend/internal/agent/mcpconfig/cursor_native_mcp.go:198` |
| 3 | `apps/backend/internal/agent/mcpconfig/cursor_native_mcp.go:104` |
| 4 | `apps/web/components/session/prepare-progress.tsx:230` |
| 5 | `apps/backend/internal/agent/settings/controller/profile_crud.go:777` |
| 6 | `apps/backend/internal/agent/settings/controller/mcp_discovery.go:75` |
| 7 | `apps/backend/internal/orchestrator/event_handlers_prepare.go:148` |
| 8 | `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go:1029` |
| 9 | `apps/backend/internal/agent/runtime/lifecycle/cursor_mcp_auth.go:67` |
| 10 | `apps/backend/internal/agent/runtime/lifecycle/cursor_plugin_mcp.go:912` |
| 11 | `apps/backend/internal/agent/mcpconfig/cursor_native_mcp.go:63` |
| 12 | `apps/backend/internal/gateway/websocket/terminal_handler.go:524` |
| 13 | `apps/backend/internal/agent/runtime/lifecycle/cursor_mcp_recovery.go:203` |
| 14 | `apps/backend/internal/terminal/service/service.go:156` |
| 15 | `apps/backend/internal/agent/mcpconfig/cursor_native_mcp.go:275` |
| 16 | `apps/web/hooks/domains/settings/use-agent-mcp-discovery.ts:13` |
| 17 | `apps/backend/internal/agent/runtime/lifecycle/cursor_mcp_recovery.go:347` |
| 18 | `apps/backend/internal/gateway/websocket/terminal_handler.go:413` |
| 19 | `apps/backend/internal/agent/runtime/lifecycle/cursor_mcp_recovery.go:257` |
| 20 | `apps/backend/internal/agent/runtime/lifecycle/manager_launch_prepare_events_test.go:80` |

## Final validation and limitations

Passing evidence:

- 95 focused frontend tests across 12 files, TypeScript checking, six-language
  localization validation/ratchet and scoped frontend lint.
- Settings and MCP config package tests/race tests; terminal and gateway package
  tests; focused lifecycle/recovery/process and orchestrator race tests.
- Changed backend package lint, SQLguard, store-conformance race tests and MCP
  config Windows cross-compilation.
- Two installed Cursor native fixture tests, including both fresh ACP/terminal
  tool use and replacement-child recovery of the saved ACP conversation.
- Documentation catalog/spec/public-page validation and whitespace checks.

Browser acceptance is blocked: the desktop build succeeded, but all four tests
stopped before assertions when macOS denied Chromium's Mach port bootstrap
(`Permission denied (1100)`, SIGTRAP). Mobile uses the same browser binary and
was stopped before Playwright launch; neither suite is claimed as passed.
[Task 04](task-04-preparation-ui.md) records commands and error artifact paths.

Broader backend package failures were reproduced independently:

- Task handlers: `TestRepositoryBranchPolicyHTTPGitflowMapsConflicts` and
  `TestRepositoryBranchPolicyWSHandlersCoverCRUDAndGitflow` reject the macOS
  `/var` versus `/private/var` temporary-path alias.
- Backend startup: `TestBackendStartupConflictStopsBeforeSharedStateInitialization`
  and `TestBackendStartupExternalDatabaseConflictDiagnostic` compare those same
  differing temporary-path spellings.
- Process Git fixtures: `TestGitOperatorPushRetainsMarkerWhenUnrelatedRefAppearsDuringBasePublication`,
  `TestGitOperatorPushWithoutOptionsIsUnchanged`,
  `TestGitOperatorPushPublishesHeadToExpectedBranch`,
  `TestGitOperatorPushCreatesMissingDestinationBranch`,
  `TestGitOperatorPushNamedFanoutRemotePublishesToEveryPushURL`,
  `TestGitOperatorPushExpectedBranchAloneOnlyGates`,
  `TestGitOperatorPushRepeatedIdenticalRequestSucceeds`,
  `TestGitOperatorPushReportsMismatchAfterBaselinePublication`, and
  `TestGitOperatorPushUsesExpectedBranchNotARaceableRereadForNoTargetRefspec`
  fail on absent local/backup remote refs.
- Runtime cache fixtures: `TestRepairManagedRuntimeCacheClearsPreviousStderr`
  and `TestRepairManagedRuntimeCacheUsesIsolatedNpmPrefix` fail removing a managed
  npm execution tree.

The relevant failing tests and implementation paths above are unchanged, but no
clean-baseline run proves they predate this branch. The earlier SQLite-lock log
was from a transient-error case in `TestBetaOfficeCreateContract`, which passes
standalone; it is not an outstanding test failure.

Native Windows runtime and real SaaS consent are not proven by the synthetic
fixture. POSIX login terminals and ACP conversation recovery are supported;
automatic live TUI reload remains unavailable without an owned native chat ID.
See [compatibility evidence](compatibility-evidence.md).

The full lifecycle rerun used a disposable `GIT_CONFIG_GLOBAL` after the first
run encountered the protected home Git configuration. It still failed after
138.704s in these non-MCP cases:

- `TestDefaultPrepareScriptKubernetesReusesRetainedPVCWorkspace`
- `TestDefaultPrepareScriptKubernetesRejectsRetainedWorkspaceFromDifferentRepository`
- `TestWorktreePreparer_MultiRepo_RollbackOnPartialFailure`
- `TestWorktreePreparer_MultiRepo_RequiredRefreshIdentifiesFailingRepository`
- `TestWorktreePreparer_MultiRepo_RollbackRemovesWorktreeCreatedForStaleReuseID`
- `TestWorktreePreparer_FreshStartRejectsStaleWorktreePathOwnedByLiveTask`
- `TestWorktreePreparer_FreshStartRejectsStaleWorktreePathOwnedByLiveTask_WorktreeIDOnly`
- `TestWorktreePreparer_FreshStartRejectsStaleWorktreePath_NestedProjectMarker`
- `TestWorktreePreparer_MultiRepoUnresolvedQualifiedBaseFailsDespiteValidSibling`
- `TestBuildAuthMethodsIdentityAgentOverridesEnvironment`
- `TestFullWorkerPreparationClonesBeforeCachesAndRetainsWorkspace`
- `TestKubernetesPreparationAcceptsEquivalentGitHubOrigins` (two subtests)

Reported errors involve retained mount-root/path comparisons, macOS temporary
path aliases, worktree cleanup/rollback expectations, and Unix socket path
length. No Cursor/MCP-focused acceptance test failed. These failures remain
reported limits rather than claimed baseline failures.
