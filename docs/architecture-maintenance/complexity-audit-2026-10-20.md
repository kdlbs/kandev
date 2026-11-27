# Code complexity and cleanup audit

Assessment date: 2026-10-20. Source snapshot: `013facd0198f6a2b0192e32b5e694f4fa55c96c6`.

[Architecture maintenance roadmap](README.md) · [Implementation outcomes](complexity-audit-2026-10-20/implementation.md)

Kandev contains removable code and avoidable complexity of the broader kind raised in [the linked tweet](https://x.com/dwlz/status/2112515295873048993/photo/1). The most useful cleanup is larger than removing one-line wrappers: retire inactive implementations, remove unused state and dependencies, consolidate duplicated policy, and finish the existing typing migrations. Identity functions and forwarding aliases are smaller, inexpensive follow-ups.

This audit records **22 findings and five investigation areas**. Findings establish a concrete maintenance problem; their proposed remedies still have the compatibility and verification conditions described below. Investigation areas have evidence worth pursuing, but do not yet establish a safe change. Nothing here attributes authorship to AI.

This is an assessment and remediation backlog, not an approved implementation package. Production code and tests were not changed. It does not assert that every defect or unnecessary abstraction in the repository has been found.

## Scope and evidence

The audit combined repository-wide searches, static reachability analysis, duplicate-block discovery, and focused reads of implementations, callers, tests, and existing architectural guidance.

Areas covered:

- Go backend and agentctl: dead functions, redundant wrappers, test-only production helpers, duplicated identity/credential/process policy, scheduler capacity, and compatibility boundaries.
- Web application: active SPA entry points versus legacy pages, unused hooks/components/state, identity functions, store typing, duplicated request and draft mechanics, and layout compatibility.
- Shared packages and tooling: dependency declarations, license generation, analyzer configuration, tests, and maintenance records.
- CLI and desktop: the native shim, managed child-process launching, external URL validation, and a scan of desktop source. No removal finding was established for those inspected boundaries.

The source-pattern scan covered tracked `.go`, `.ts`, `.tsx`, `.rs`, `.js`, `.mjs`, `.py`, and `.sh` files under `apps/`, `scripts/`, and `.github/`, excluding generated/protobuf paths and locale catalogs. Search coverage is broader than the manually inspected findings. It is not a line-by-line review of the repository. The duplicate scan was a candidate generator based on repeated 12-line blocks; matching text alone was not treated as a finding.

### Recorded analyzer results

| Check                                                                                             | Result               | Interpretation                                                                                                          |
| ------------------------------------------------------------------------------------------------- | -------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `pnpm install --frozen-lockfile`, from `apps/`                                                    | Passed               | Installed worktree dependencies without changing the lockfile                                                           |
| `pnpm exec knip --include files,exports,dependencies --no-progress --reporter json`, from `apps/` | Exit 1 with findings | 108 file candidates, 775 export candidates, 27 dependency and six dev-dependency candidates                             |
| `go run golang.org/x/tools/cmd/deadcode@v0.48.0 -tags fts5 ./...`, from `apps/backend/`           | Exit 0 with findings | 1,047 functions unreachable from the analyzed Linux production entry points; this tool reports findings without failing |
| `python3 scripts/lint-architecture.py --all`, from the repository root                            | Passed               | Existing architecture budgets and registered rules passed; this does not mean the budgets are zero                      |

The Go tool automatically selected Go 1.26.10. Its run did not include `-test`, Windows, or macOS. Test-only helpers, exported SDK functions, and platform-specific entry points therefore require separate interpretation. No runtime behavior tests or browser sessions were run for this documentation-only audit.

The complete analyzer observations are preserved, including untriaged entries:

- [Static analysis inventory](complexity-audit-2026-10-20/static-analysis.csv): **1,963 rows**, comprising all Knip and Go observations. Rows are candidates unless marked and explained otherwise.
- [Backend single-mention shortlist](complexity-audit-2026-10-20/backend-single-mention-candidates.csv): **48 private functions/methods** that the Go analyzer reported and whose name occurs only once in tracked Go source, with no test mention. Nine were inspected for F06; the remaining 39 need body/caller-history review before removal.

The shortlist counts lexical mentions across all tracked Go files, including other platforms. This is a conservative screening aid, not a replacement for compilation, generated-code checks, or a semantic caller analysis. The shortlist is a subset of the full inventory; do not add their counts together.

## Priorities

Priority describes remediation value, not incident severity. No production incident or exploitable vulnerability is claimed by this audit.

| Priority                    | Findings          | Intended result                                                                    |
| --------------------------- | ----------------- | ---------------------------------------------------------------------------------- |
| First                       | F01-F06, F18, F20 | Remove inactive implementations and improve the precision of the removal inventory |
| Next                        | F12-F16           | Restore type checking and give duplicated policy a single owner                    |
| Small accompanying cleanup  | F07-F11, F17, F19 | Remove misleading names, aliases, redundant test helpers, and comment repetition   |
| Existing maintenance work   | F21-F22           | Keep current architecture and compatibility tracking accurate                      |
| Investigate before changing | I01-I05           | Establish behavior and compatibility before proposing a fix or extraction          |

## Confirmed findings

### F01 Retained Next.js entry pages duplicate the active SPA

**Priority:** First. **Confidence:** High for the named pages.

The active entry point is [src/main.tsx](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/src/main.tsx#L1), which mounts `AppShell` and `SpaRoutes`. [spa-routes.tsx](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/src/spa-routes.tsx#L9) imports the GitHub, GitLab, Jira, Linear, Stats, and Tasks client components directly. The old async page functions are not those route implementations.

Knip reports the following files as unused, and inspection confirms they contain the obsolete root shell or server-style data-loading composition:

- [app/layout.tsx:1](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/app/layout.tsx#L1)
- [app/tasks/page.tsx:1](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/app/tasks/page.tsx#L1)
- [app/github/page.tsx:1](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/app/github/page.tsx#L1)
- [app/gitlab/page.tsx:1](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/app/gitlab/page.tsx#L1)
- [app/jira/page.tsx:1](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/app/jira/page.tsx#L1)
- [app/linear/page.tsx:1](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/app/linear/page.tsx#L1)
- [app/stats/page.tsx:1](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/app/stats/page.tsx#L1)

These seven files total 629 lines. They retain alternate workspace selection, hydration, error handling, provider composition, and metadata. `app/layout.tsx` even documents that its metadata is consumed by nothing. A reader or code generator can easily update the wrong implementation.

**Address:** Remove these inactive entry files and then inspect their newly orphaned imports. Preserve the active `*-page-client` components and shared helpers still used by SPA routes. Other `page.tsx` entries in the inventory need the same route check; do not delete the entire `app/` directory.

**Verify:** Web typecheck and Vite build; existing route/bootstrap tests and focused desktop/phone navigation coverage for the affected routes. Follow [the SPA migration guide](../nextjs-spa-migration.md).

### F02 An unused cookie compatibility chain retains an impossible fallback

**Priority:** First. **Confidence:** High.

[read-layout-defaults.ts:4](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/layout/read-layout-defaults.ts#L4) calls [readCookies](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/server/cookies.ts#L35), whose local `CookieStore` contract requires `getAll()` and whose implementation always supplies it. The caller nevertheless casts the object twice, checks whether `getAll` exists, and provides a second cookie parser in an immediately invoked fallback function.

Both modules are reported unused. The redundant fallback would be unnecessary even if the chain were live. [server-navigation.ts:1](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/routing/server-navigation.ts#L1) is another unused compatibility module; its remaining imports are in inactive page files.

**Address:** Retire the unused chain with its page consumers. If a real caller is intentionally restored, use `cookieStore.getAll()` directly and retain validation of the cookie values themselves. Do not maintain a fallback for a different API than the one actually imported.

**Verify:** Web typecheck/build and the route/layout preference tests selected by F01. Search both module paths and exported symbols after deletion.

### F03 Four orphaned hooks preserve obsolete alternatives

**Priority:** First. **Confidence:** High for local application use.

Knip and repository searches identify no consumers of:

- [use-mobile.ts:5](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/hooks/use-mobile.ts#L5): a second mobile hook with a 640px breakpoint, whereas application layout uses `useResponsiveBreakpoint` and its 768px phone boundary.
- [use-task-repositories.ts:5](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/hooks/use-task-repositories.ts#L5): the old hook resolves one `repositoryId` by flattening workspace repositories. Live consumers import [the domain hook](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/hooks/domains/kanban/use-task-repositories.ts#L21).
- [use-task-chat-session.ts:13](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/hooks/use-task-chat-session.ts#L13): an unused primary-session selector with its own working-state interpretation.
- [use-worktree.ts:3](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/hooks/use-worktree.ts#L3): an unused store selector wrapper.

**Address:** Delete these unused modules rather than keeping alternative APIs available for accidental reuse. The old mobile hook is not evidence of a current responsive bug because it has no caller.

**Verify:** Typecheck/build and a symbol/module-path search. Preserve the active domain hooks and responsive hook.

### F04 The unused GitLab statistics hook leaves a state chain behind

**Priority:** First. **Confidence:** High for the orphaned local chain; compatibility must be checked before removal.

[use-gitlab-stats.ts:12](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/hooks/domains/gitlab/use-gitlab-stats.ts#L12) has no application caller. Its `gitlabStats` snapshot still has defaults, types, setters, initial-state merging, and store overrides in:

- [gitlab-slice.ts:11](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/slices/gitlab/gitlab-slice.ts#L11), including `statsActions` around line 178.
- [GitLab slice types](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/slices/gitlab/types.ts#L68).
- [default-state.ts:147](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/default-state.ts#L147).
- [app-state-types.ts:196](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/app-state-types.ts#L196).
- [store-overrides.ts:81](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/store-overrides.ts#L81).

The slice test exercises setting this snapshot, and the API-client test exercises `fetchGitLabStats`; neither establishes a live UI consumer. This is more maintenance than an unused hook alone.

**Address:** Confirm the intended product and supported plugin/boot contracts, then remove the unused frontend state chain together. Do not infer that the backend statistics endpoint can also be removed; it has a separate API contract. If the missing UI is intentional future work, record that separately rather than treating these setters as evidence the feature exists.

**Verify:** Root-store initialization/hydration and GitLab slice tests, typecheck/build, and GitLab page coverage. Check public plugin compatibility before deleting exposed state shape.

### F05 Unused presentation components and an obsolete re-export remain available

**Priority:** First. **Confidence:** High.

The inspected unused modules are [empty-state-inline.tsx:9](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/components/empty-state-inline.tsx#L9), [task/sidebar-button.tsx:16](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/components/task/sidebar-button.tsx#L16), [task/quick-chat-button.tsx:19](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/components/task/quick-chat-button.tsx#L19), and [task-create-dialog-branch-options.tsx:1](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/components/task-create-dialog-branch-options.tsx#L1).

The last file only re-exports `branch-picker-options`; the branch-picker boundary test already asserts consumers do not use the obsolete module. The Quick Chat button also survives as a stale mock in `task-top-bar.test.tsx`, which is not a production consumer.

**Address:** Delete these modules and stale test mocks where they serve no current test dependency. Keep active Quick Chat launchers and shared branch-picker code. Additional unused component files in the CSV remain untriaged.

**Verify:** Typecheck/build, the branch-picker boundary test, and the affected top-bar test. Deleting an unmounted component does not authorize redesigning its active replacement.

### F06 Private backend functions have no source or test callers

**Priority:** First. **Confidence:** High for the nine inspected symbols below.

The Go analyzer reports these functions unreachable. Across tracked Go files, each name occurs only at its definition, including files for other platforms; no test mentions it. Their bodies and neighboring active implementations were inspected.

| Symbol                                 | Location                                                                                                                                                                           | What remains unnecessarily                                               |
| -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| `Manager.publishStreamDisconnectError` | [manager_events.go:1345](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agent/runtime/lifecycle/manager_events.go#L1345)      | Forwards to the attempt-aware method with an empty attempt ID            |
| `matchLegacyRuntimeEnvironmentRules`   | [runtime_rules.go:403](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agent/runtime/routingerr/runtime_rules.go#L403)         | Provider-less legacy matching wrapper beside the provider-aware variants |
| `validateRuntimeCatalogueTarget`       | [agent_update.go:802](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agent/settings/controller/agent_update.go#L802)          | Calls package-aware validation with an empty package                     |
| `stringSliceToAny`                     | [sidebar_task_query.go:305](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/task/repository/sqlite/sidebar_task_query.go#L305) | Unused slice conversion loop                                             |
| `Controller.httpConfigureHost`         | [controller.go:96](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/gitlab/controller.go#L96)                                   | Unregistered legacy host-configuration handler                           |
| `Controller.httpSyncTaskMR`            | [controller.go:228](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/gitlab/controller.go#L228)                                 | Unregistered task-MR synchronization handler                             |
| `PlanService.readLatestRevision`       | [plan_service.go:260](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/task/service/plan_service.go#L260)                       | Discards the error from the detailed reader and returns the other values |
| `Cloner.fetchWithHTTPHeader`           | [clone.go:878](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/repoclone/clone.go#L878)                                        | Unused separate authenticated fetch implementation                       |
| `Cloner.cloneWithHTTPHeader`           | [clone.go:892](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/repoclone/clone.go#L892)                                        | Unused separate authenticated clone implementation                       |

**Address:** Remove these private leftovers in small package-scoped changes. Inspect resulting unused imports and dependent helpers; do not delete the current attempt-aware, package-aware, or detailed-reader implementations. Unregistered handlers should be removed, not wired back into the router to make them reachable.

The [48-symbol shortlist](complexity-audit-2026-10-20/backend-single-mention-candidates.csv) preserves the remaining candidates, including old credential, cancellation, rescan, and cleanup helpers. Their names alone are insufficient grounds for a behavior change.

**Verify:** Focused tests and compilation for each affected package, changed-Go lint, and the production reachability scan. Platform-sensitive package changes also need their platform compilation path.

### F07 Identity functions and a constant predicate imply nonexistent work

**Priority:** Small cleanup. **Confidence:** High.

| Helper                        | Evidence                                                                                                                                                                                                                                | Suggested simplification                                                                     |
| ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `latestRuntimeVersions`       | [agent-runtime-update.ts:9](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/agent-runtime-update.ts#L9) returns the array unchanged; the picker calls it at `runtime-version-picker.tsx:195` | Use the available versions/fallback array directly                                           |
| `stripVersionSuffix`          | [generate-licenses.ts:420](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/scripts/generate-licenses.ts#L420) returns `pkg`, while its name promises stripping                                   | Use `row.pkg`; keep the useful note that Go major-version suffixes belong to module identity |
| `shouldUsePermissionTaskIcon` | [state-icons.tsx:362](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/ui/state-icons.tsx#L362) returns the boolean argument with a default of `false`                                        | Use the condition directly, preserving the default at callers that can pass `undefined`      |

The dedicated `latestRuntimeVersions` test checks only that the identity function returns the array. It does not prove that the visible version picker renders the full backend projection.

There is a related backend example: [Executor.CanExecute at executor_state.go:73](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/orchestrator/executor/executor_state.go#L73) always returns `true`, and [scheduler.go:279](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/orchestrator/scheduler/scheduler.go#L279) uses it as the loop condition with a comment about capacity. The scheduler holds the concrete executor, so this is not a polymorphic capacity implementation. Replace the constant condition with the queue-draining loop it actually represents, preserving its cancellation and empty-queue exits. This is not evidence that all Kandev admission limits are absent; those have other owners and must remain intact.

**Address:** Remove the identity functions and imports. Put any valuable regression assertion at the consuming picker/icon behavior. Do not add replacement one-line helpers or mirror tests for the inlined expressions.

**Verify:** Runtime picker tests, relevant task-state icon tests, and existing license-generator tests. Preserve order, fallback arrays, optional booleans, and Go `/v2` or `/v3` suffixes.

### F08 Forwarding aliases add navigation without owning a boundary

**Priority:** Small cleanup. **Confidence:** High that the forwarding is redundant; naming value is a local judgment.

| Wrapper                                    | Location                                                                                                                                                          | Existing operation                                                               |
| ------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| `fixedAutomaticTaskColor(token)`           | [task-color-presentation.ts:91](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/task-color-presentation.ts#L91)        | `taskColorPresentation(token)` with identical input/output types                 |
| `mrKey(host, projectPath, iid)`            | [use-mr-key-to-tasks.ts:9](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/hooks/domains/gitlab/use-mr-key-to-tasks.ts#L9) | `gitLabMRKey` with unchanged arguments; only production call is in the same file |
| `integrationWritable(...)`                 | [defaults.go:72](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/settingscatalog/defaults.go#L72)             | `domainWritable` with all seven arguments forwarded unchanged                    |
| `foldMilestoneIntoQuery(query, milestone)` | [client_helpers.go:555](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/gitlab/client_helpers.go#L555)        | `appendQueryParam(query, "milestone", milestone)` at one call site               |
| `countDiffLines(diff)`                     | [client_helpers.go:629](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/gitlab/client_helpers.go#L629)        | `unidiff.CountLines(diff)`                                                       |

**Address:** Prefer the existing operation where it is already equally clear. Consolidate the two task-color names; call the shared GitLab identity and diff helpers directly; remove the seven-argument pass-through. The milestone wrapper can be inlined at its sole caller without making that expression obscure.

These are internal aliases, not an argument against repository interfaces, transport adapters, capability checks, or public SDK facades.

**Verify:** Existing color, GitLab identity/query/diff-statistics, and settings-catalog tests. Retain integration-level assertions when moving wrapper tests to the actual shared algorithm.

### F09 Two state detectors implement the same no-op behavior

**Priority:** Small cleanup. **Confidence:** High.

[IdleDetector](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agentctl/server/process/idle_detector.go#L15) returns `StateUnknown` from `DetectState` and always accepts state changes. [NoOpDetector](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agentctl/server/process/status_tracker.go#L248) implements the same two methods. The interactive runner constructs `IdleDetector`; `NoOpDetector` is referenced by a dedicated unit test and is unreachable in the production analyzer result.

**Address:** Retire the unused production detector and its self-confirming test, or move a genuinely needed test fake to `_test.go`. Keep one production implementation and document that the actual idle timer belongs to `InteractiveRunner`.

**Verify:** Status-tracker and interactive-runner behavior tests. Do not remove idle timing or the detector interface merely because one implementation is intentionally a no-op.

### F10 Tests keep convenience functions in production files

**Priority:** Small cleanup. **Confidence:** High for the named examples.

- [executor_remote_docker.go:756](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker.go#L756) defines `executorNameRemoteDocker()` only to return `executor.NameRemoteDocker` to `executor_remote_docker_engine_test.go`.
- [docker/handlers.go:288](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agent/docker/handlers.go#L288) keeps `newContainerResponses` as a default-argument wrapper used by `handlers_test.go`; production calls `newContainerResponsesWithTaskTitles`.
- [status_tracker.go:50](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agentctl/server/process/status_tracker.go#L50) exposes `DefaultStatusTrackerConfig`, whose found consumers are tests, including a test of its literal defaults.

**Address:** Use the constant directly, call the current formatter with explicit test inputs, and move a configuration fixture into test support if it has no production contract. Avoid preserving superseded APIs only to keep older tests unchanged.

**Verify:** The tests that currently call these functions must continue to exercise the actual runtime operation. This finding does not apply to cross-package conformance helpers or exported SDK test support without a separate contract review.

### F11 An obsolete sidebar action deliberately hides stale callers

**Priority:** Small cleanup. **Confidence:** High.

[dockview-store.ts:701](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/dockview-store.ts#L701) exposes `toggleSidebar` as an empty function. Its comment says it exists so stragglers compile. Searches found the interface declaration and implementation but no production call. The current AppSidebar action lives elsewhere.

**Address:** Remove the dead action and interface member. A stale consumer should fail type checking and migrate to the correct action rather than silently do nothing.

**Verify:** Store typecheck and sidebar shortcut/navigation tests. Do not remove `sidebarVisible` or `sidebarGroupId` as part of this tiny cleanup: those have real remaining reads and writes, discussed in I01.

### F12 Root-store composition still bypasses type checking 45 times

**Priority:** Next. **Confidence:** High; already tracked architectural debt.

[store.ts:55](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/store.ts#L55) contains **45 `as any` assertions**, matching the [root-state-cast baseline](../../config/architecture-lint/frontend_root_state_cast.json). Most surround slice construction; another casts the hydration draft. This preserves a repeated mismatch between the root store and slice creator signatures and requires local linter suppressions.

The [Features slice](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/slices/features/features-slice.ts#L8) and already migrated slices demonstrate a smaller accepted input contract. For example, [the System slice](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/slices/system/system-slice.ts#L24) still declares `_get` and `_api` even though its implementation uses `set`.

**Address:** Continue the existing [DEP-01 work](dependency-cleanup.md#dep-01-type-one-slice), one slice at a time. Give each creator the setter/getter capabilities it actually uses and remove the associated root assertions. Do not move the same casts into a new composition helper.

**Verify:** Root composition and hydration tests, the affected slice tests, web typecheck, and architecture lint. Preserve unrelated state references and caller-supplied initial state.

### F13 Managed credential cleanup policy is duplicated across process boundaries

**Priority:** Next. **Confidence:** High.

`removeObsoleteManagedCredentialEnvironment` appears in [lifecycle/profile_env.go:97](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agent/runtime/lifecycle/profile_env.go#L97) and [agentctl/process/manager.go:2417](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agentctl/server/process/manager.go#L2417). Both delete the same 14 managed credential environment keys.

The cleanup is necessary, but two manually maintained policy lists can diverge when another credential field is added. Both callers already depend on the shared `githubauth` contract.

**Address:** Give the key set or deletion operation one cross-tier owner, using the existing shared credential/environment boundary. Keep the lifecycle and agentctl call sites because they protect different execution stages. Do not merge their surrounding environment composition or remove a cleanup stage.

**Verify:** Credential replacement/removal tests for both paths. Include preservation of unrelated user variables and every managed key. Review the helper's package placement against the cross-tier ownership rule.

### F14 Executor actor identity policy is copied into seven integrations

**Priority:** Next. **Confidence:** High for duplication; extraction needs ownership review.

`executorCreatorUserID` is copied into `executor_ownership.go` in [automation](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/automation/executor_ownership.go#L10), [azuredevops](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/azuredevops/executor_ownership.go#L10), [github](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/github/executor_ownership.go#L10), [gitlab](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/gitlab/executor_ownership.go#L10), [jira](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/jira/executor_ownership.go#L10), [linear](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/linear/executor_ownership.go#L10), and [sentry](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/sentry/executor_ownership.go#L10).

Each handles synthetic identity, bound executor actor, authenticated user, and missing identity in the same order. The surrounding context forwarding is also repeated. This is a shared authority policy rather than provider-specific issue logic.

**Address:** Extract the pure creator-identity policy to its existing neutral owner after agreeing the boundary. Leave provider migrations and route-specific authorization local. Preserve the exact precedence and synthetic-user semantics; this is not authorization to change access behavior.

**Verify:** Table-driven identity precedence coverage and integration wiring tests. Do not replace the seven implementations with a configurable authorization framework.

### F15 Linux process-group liveness parsing is duplicated

**Priority:** Next. **Confidence:** High.

[process/procattr_linux.go:81](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agentctl/server/process/procattr_linux.go#L81) and [shell/process_group_linux.go:66](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agentctl/server/shell/process_group_linux.go#L66) contain identical `processStatMatchesLiveGroup` functions. Both find the last `") "`, parse `/proc` fields, reject zombie/dead states, and compare the process group. Both have corresponding parser tests. The `processGroupResponds` fallback is duplicated too.

**Address:** Share the pure `/proc` parsing predicate through an appropriate neutral process utility. Consider the syscall probe separately; keep the process and shell ownership/lifecycle code local. Do not simplify this to `kill(pid, 0)` alone, which would lose the zombie distinction.

**Verify:** Parser cases including spaces/parentheses in command names, malformed records, `Z`/`X` states, and mismatched groups; keep process and shell cleanup behavior tests.

### F16 Workspace request decoration is repeated in API clients

**Priority:** Next. **Confidence:** High.

[jira-api.ts:18](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/api/domains/jira-api.ts#L18), [linear-api.ts:22](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/api/domains/linear-api.ts#L22), and [workflow-sync-api.ts:10](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/api/domains/workflow-sync-api.ts#L10) independently define the same `WorkspaceApiOptions`, `withWorkspace`, and `requestOptions` mechanics. The helpers append an encoded `workspace_id` and strip the local option before transport.

**Address:** Share this small request-decoration operation within the API layer. Keep endpoint functions and provider response types explicit. GitLab and Azure have similar but different signatures; compare their missing-workspace contracts before including them.

**Verify:** Existing API tests for an existing query string, escaped workspace IDs, absent optional workspace, and preservation of abort signals/headers/init options. A generic configurable API-client factory would add more complexity than this duplication warrants.

### F17 Known repository and project accumulators duplicate one mechanism

**Priority:** Small cleanup. **Confidence:** High for duplicated mechanism.

[GitHub use-known-repos.ts:9](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/components/github/my-github/use-known-repos.ts#L9) and [GitLab use-known-projects.ts:9](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/components/gitlab/my-gitlab/use-known-projects.ts#L9) each own a reset key, a set, a stable sorted snapshot, listeners, record/reset operations, and `useSyncExternalStore` subscription.

**Address:** If these paths are being maintained, share the small accumulator implementation while retaining a separate instance for each provider. Preserve reset timing, stable snapshots, and filter-option accumulation. Do not merge provider state into one singleton or introduce a general store framework.

**Verify:** The existing accumulator tests for both providers, including reset, no-change identity, deduplication, ordering, and provider isolation. This is lower value than deleting inactive code.

### F18 Dependency declarations outlive their direct usage

**Priority:** First. **Confidence:** High for the inspected declaration groups; the full list remains candidates.

[packages/ui/package.json:11](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/packages/ui/package.json#L11) declares 13 individual `@radix-ui/react-*` packages. The UI implementations inspected/searchable in this snapshot import primitives from the aggregate `radix-ui` package; for example, [dialog.tsx:4](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/packages/ui/src/dialog.tsx#L4). Knip flags all 13 direct declarations, and the source search found no direct imports requiring them.

[packages/theme/package.json:18](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/packages/theme/package.json#L18) also declares `hono` as a dev dependency without a theme source consumer.

**Address:** Remove unused direct declarations after verifying package-level build and export usage, and regenerate the lockfile. Keep `radix-ui` and any packages still present transitively; removing a redundant declaration does not imply those libraries disappear from the installed graph or bundle.

The other web dependencies in the CSV need the same examination. In particular, **retain** `@tailwindcss/postcss`: [the exported PostCSS configuration](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/packages/theme/src/postcss.config.mjs#L3) refers to it by plugin key. Retain repository-invoked `@commitlint/cli` and `@playwright/cli` as well. Knip's missing import edge is not enough to remove them.

**Verify:** Frozen-lockfile install after the lockfile update, typecheck, web/desktop builds as relevant, and focused shared-UI rendering tests. No bundle-size savings were measured in this audit.

### F19 Historical narration obscures the current invariant

**Priority:** Small accompanying cleanup. **Confidence:** High for the examples.

- [client_helpers.go:620](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/gitlab/client_helpers.go#L620) has nine lines of diff-algorithm and old-bug explanation above a function that now delegates to `unidiff.CountLines`.
- [client_helpers.go:548](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/gitlab/client_helpers.go#L548) explains the milestone helper's caller and previous precedent above a one-call wrapper.
- [agent-runtime-update.ts:16](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/agent-runtime-update.ts#L16) places a detailed history of the old English `"Unknown"` comparison on the `hasCurrentVersion` field.
- [dockview-layout-builders.ts:224](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/dockview-layout-builders.ts#L224) explains that a helper was extracted to meet the complexity limit.
- [store.ts:91](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/store.ts#L91) narrates the CodeQL reason for not passing unused arguments.

**Address:** State the present invariant briefly at its owner. Keep useful regression explanations in tests or linked design/PR history. Delete obsolete wrapper documentation when deleting its wrapper. Do not strip rationale that explains a race, identity boundary, or platform quirk merely to reduce line count.

**Verify:** Review comment claims against the surviving implementation. No new tests are needed for a comment-only change.

### F20 Dead-code tooling needs explicit non-import entry points

**Priority:** First, alongside removal triage. **Confidence:** High.

[apps/knip.json](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/knip.json#L1) currently adds Playwright entry configuration but does not describe all repository execution mechanisms. This run reports known live material as unused:

- [inspector.js](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agentctl/server/api/scripts/inspector.js) is embedded by `html_injector.go`.
- Canvas scaffold `appearance.js` and `script.js` are bundled assets referenced by the scaffold and Go embedding.
- `check-new-code-i18n.mjs` and `check-guard-allowlist.mjs` are launched by pre-commit and the i18n ratchet.
- `@tailwindcss/postcss`, commitlint, and Playwright CLI have configuration/tooling entry points.

**Address:** Add precise entry points or documented exclusions for these mechanisms, then use the reduced report for incremental cleanup. Do not suppress all scripts, all backend JavaScript, all exports, or all `page.tsx` files. That would hide F01 and other genuine leftovers.

For Go, classify test utilities, public SDK symbols, and platform-specific code before interpreting the 1,047 results. Keep production and test reachability distinguishable so tests do not automatically justify obsolete production APIs.

**Verify:** Rerun Knip and confirm known false positives disappear while the confirmed orphan examples remain detectable until removed. Validate embedded assets through their actual owners.

### F21 Architecture maintenance records mix historical and current ownership

**Priority:** Existing maintenance work. **Confidence:** High for source/document disagreement.

The [server-state tracker](server-state-migrations.md) describes disk usage as Zustand-owned and discusses a future Query migration. [use-disk-usage.ts:4](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/hooks/domains/system/use-disk-usage.ts#L4) already imports `useQuery` and uses the scoped disk Query. The [main roadmap](README.md) still proposes the second-resource milestone and next review on October 11, while individual tracker rows and source have advanced.

The [dependency tracker](dependency-cleanup.md) labels its counts as a dated September inventory; those historical counts are not themselves errors. However, they should not be reused as a current todo count: the live compatibility ledger now has 29 entries, including 27 Office run aliases, rather than the older totals described there.

**Address:** Refresh current-owner and next-action prose from source and delivery records. Keep historical measurements labelled with their date. Confirm actual PR/merge evidence before marking delivery complete; implementation presence in this checkout is not proof of merge status.

**Verify:** Cross-check each updated tracker against its source owner, plan/work-order status, and live delivery evidence. Do not change requirements to fit stale tracker prose.

### F22 Existing architecture exceptions remain a real bounded backlog

**Priority:** Existing maintenance work. **Confidence:** High as recorded exceptions, not newly proven design defects.

The full architecture check passed with these current baseline/ledger entries:

| Record                                   | Entries | Existing work to use                                                      |
| ---------------------------------------- | ------: | ------------------------------------------------------------------------- |
| Root-state casts                         |      45 | F12 and DEP-01                                                            |
| Runtime implementation imports           |      61 | DEP-04 caller-by-caller facade capability investigation                   |
| Runs importing Office                    |       6 | DEP-03 ownership design                                                   |
| Task importing Office                    |       7 | DEP-05 domain contract design                                             |
| Deprecation declarations in the baseline |      15 | DEP-06 disposition                                                        |
| Compatibility ledger                     |      29 | 27 Office run aliases plus registry aliases and passthrough compatibility |

Source: [architecture-lint configuration](../../config/architecture-lint/) and [dependency cleanup](dependency-cleanup.md). Counts describe entries, not independent bugs, and overlap with findings above.

**Address:** Continue the existing bounded migrations. Remove aliases together with their final consumers and ledger entries. Define missing runtime capabilities before rerouting imports. Keep Office policy Office-owned; moving it into generic Runs merely to reduce a count would make the architecture worse.

**Verify:** Each existing entry's removal condition, affected package tests, compilation, and architecture lint. These counts are not grounds for a repository-wide abstraction rewrite.

## Investigation areas

### I01 Sidebar state contradicts its inert-state comment

**Priority:** Investigate early. **Confidence:** High that the contradiction exists; user-visible impact is unproven.

[dockview-store.ts:1874](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/dockview-store.ts#L1874) says the legacy sidebar fields are inert and `sidebarVisible` is always false. [Preset application at line 827](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/dockview-store.ts#L827) writes `true`; custom-layout and restore paths write derived values. [dockview-pinned-enforce.ts:77](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/dockview-pinned-enforce.ts#L77) reads the value when calculating available width and uses it to resize the first column.

**Next action:** Reproduce preset/custom-layout restoration with and without a legacy sidebar panel, including maximized layouts and viewport changes. Decide whether legacy layout migration is still supported, and then remove obsolete state or correct the projection. Do not assume the fields can be deleted with F11.

**Acceptance evidence:** Desktop resize/preset tests, stored-layout restoration coverage, and phone/tablet transitions preserving saved desktop geometry. This investigation may become a behavior-changing fix and would need its own requirements/design checkpoint.

### I02 Some durable-delivery tests exercise a retired adapter path

**Priority:** Investigate early because the invariant is important.

[durable_delivery_stream_test.go:162](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream_test.go#L162) tests projection-before-acknowledgement through `projectAndAcknowledgeDurableAgentEvent`. The analyzer reports that function and its adapter chain unreachable from production. Searches find calls from tests, while production projection is invoked in [stream_event_processor.go:228](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agent/runtime/lifecycle/stream_event_processor.go#L228) and acknowledgement in [streams.go:608](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/backend/internal/agent/runtime/lifecycle/streams.go#L608).

This does **not** establish missing production coverage: other tests may cover the active flow. It does establish that passing these particular tests is not, by itself, proof of the current receive/project/ack orchestration.

**Next action:** Trace the current stream admission, projection, settlement, and acknowledgement paths; map existing ordering/failure tests to those paths. Move the assertions to the actual entry point where needed, then delete or move test-only adapters. Preserve projection failure, duplicate delivery, replay, and terminal settlement cases.

### I03 Repeated settings draft mechanics already have divergent save semantics

**Priority:** Investigate before extracting a common hook.

The following settings components repeat `saved`/`draft` state, a draft ref, and the same preference-sync effect:

- [agent-generated-task-title-settings.tsx:22](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/components/settings/agent-generated-task-title-settings.tsx#L22)
- [agent-tab-close-behavior-settings.tsx:28](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/components/settings/agent-tab-close-behavior-settings.tsx#L28)
- [message-time-display-settings.tsx:27](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/components/settings/message-time-display-settings.tsx#L27)
- [mcp-task-agent-profile-default-settings.tsx:148](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/components/settings/mcp-task-agent-profile-default-settings.tsx#L148)

The duplication is concrete, but their save paths are not identical: some apply response-revision guards, while others apply the submitted local field after the request. A generic extraction that ignores this distinction could erase concurrency behavior.

**Next action:** Establish one intended dirty-draft/external-update/save/discard contract. Share only the repeated mechanism that has the same semantics, using the existing save-contributor owner. Test edits made during save, stale responses, external preference updates, and discard. Keep provider-specific presentation local.

### I04 Private Dockview access spreads unchecked structural assumptions

**Priority:** Design investigation.

[applier.ts:24](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/layout-manager/applier.ts#L24) returns a private splitview as `any`; [serializer.ts:364](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/web/lib/state/layout-manager/serializer.ts#L364) reads `component.gridview.root` independently; pinned enforcement accepts `any` again. Runtime guards and catches exist because private internals can be absent or reject sizes.

**Next action:** Check the installed library's supported APIs and types, then determine whether one small typed compatibility adapter can own the remaining private access. Preserve the runtime checks at that boundary. Do not replace them with unchecked casts or an invented complete copy of the library's internals.

**Acceptance evidence:** Layout serialization/restoration, missing-grid behavior, unsupported sizes, resizing, and library-upgrade coverage. No claim is made here that a public replacement API exists.

### I05 Remaining static candidates need disposition by owner

**Priority:** Ongoing triage, not a bulk deletion task.

The CSV retains the entire analyzer output so unexamined candidates are not lost. Examples include unused API exports, old presentation files, compatibility barrels, test-only Go adapters, exported plugin SDK methods, and conformance utilities.

**Next action:** For each candidate group, check the current entry point, dynamic/string-based usage, tests, generated source, platform builds, and supported external consumers. Record one of: remove; make private; move to test support; retain with a reason; or investigate a missing product connection. Re-run the analyzer after each bounded change.

For an unused export that is still used inside its own module, the appropriate change may be removing `export`, not deleting the function. For a test utility, reachability from production is not its purpose. For a public plugin SDK method, local reachability says nothing about external consumers.

## Patterns that should be preserved

The tweet's screenshot is not sufficient evidence that its added cancellation is useless. A returned stop function can cancel a goroutine and wait for it to exit, which is a meaningful behavior change. The same caution applies to this repository.

The audit explicitly does not recommend removing:

- **Cancellation, wait groups, and completion channels** that establish shutdown ownership. Kandev's backend guidance requires drain semantics for long-running goroutines.
- **HTTP/WS generation, revision, and scope guards** that reject late results or protect auth/workspace identity. They may be verbose and still necessary.
- **Boundary validation** for wire messages, saved layouts, plugin data, cookies, URLs, and untrusted filesystem paths. A TypeScript type assertion does not validate those values.
- **Provider adapters and one-implementation interfaces** when they express a real capability/transport/test boundary. The runtime facade is an intentional architecture seam.
- **Platform-specific no-ops** required by a shared lifecycle interface. An empty Unix setup method may have a substantive Windows counterpart.
- **Managed desktop child launching:** [child_process.rs:3](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/desktop/src-tauri/src/child_process.rs#L3) creates the wait worker before spawning and acknowledges launch separately from exit. That machinery owns reaping and failure behavior.
- **Desktop external-link validation:** [external_links.rs:31](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/desktop/src-tauri/src/external_links.rs#L31) enforces scheme, credentials, and local-destination boundaries before invoking the OS opener.
- **The CLI shim:** [native-shim.js](https://github.com/kdlbs/kandev/blob/013facd0198f6a2b0192e32b5e694f4fa55c96c6/apps/cli/bin/native-shim.js) resolves the platform package, validates the bundle, forwards the environment, and propagates launch failure. Its small helpers carry real behavior.
- **Architecture tests that inspect source** when their purpose is preventing forbidden imports or retired feature-flag coupling. The branch-picker and parked-projection source tests inspected here are not ordinary behavior tests and should not be discarded merely for reading source text.
- **Health-poll and provider-specific issue schemas** already intentionally shared or separated by the integration guide. Jira JQL and Linear structured filters should not be forced into one universal model.

## Suggested remediation sequence

These are suggested batches for later planning, not work orders or authorization to implement.

1. **Make the inventory trustworthy:** F20. Add precise analyzer entries and retain the confirmed orphan checks as reference cases.
2. **Remove inactive frontend paths:** F01-F05, with route and plugin-compatibility checks. Remove newly orphaned helpers only after following their consumers.
3. **Remove private backend leftovers:** F06, one package group at a time. Triage the remaining single-mention shortlist separately.
4. **Clean small redundant APIs and tests:** F07-F11 and associated F19 comments. Keep behavior-level assertions and simplify callers in the same change.
5. **Remove redundant dependency declarations:** F18, with package builds and lockfile verification.
6. **Continue root-store typing:** F12, selecting a small slice and proving composition/hydration behavior.
7. **Consolidate duplicated policy/mechanics:** F13-F17. Start with exact pure operations; review authority ownership before extracting actor or credential policy.
8. **Resolve the higher-risk investigations:** I01-I04. Reproduce and define behavior before modifying layout, delivery ordering, or settings concurrency.
9. **Refresh maintenance records and retire compatibility incrementally:** F21-F22. Use existing owners and removal conditions.

For every batch, the review should answer: what concrete code or state disappears; which behavior/contract remains; how callers now reach the owner; and which tests demonstrate that behavior. Avoid replacing deleted code with a larger generic framework or adding mirror tests for trivial expressions.

## Reproducing and updating the inventory

Run from the named directories without applying analyzer auto-fixes:

```sh
# apps/
pnpm install --frozen-lockfile
pnpm exec knip --include files,exports,dependencies --no-progress --reporter json

# apps/backend/
go run golang.org/x/tools/cmd/deadcode@v0.48.0 -tags fts5 ./...

# repository root
python3 scripts/lint-architecture.py --all
git status --short
```

The CSV uses repository-relative paths. `status=inspected` refers to the finding in the `finding` column and its conditions; it does not mean a fix has been implemented. `retain` marks an identified false positive. `single-mention-candidate` records the additional lexical screen. `untriaged` carries no deletion verdict. Source and test mention counts include comments and are supplied only for Go observations.

Refresh this snapshot after substantial cleanup or rebasing. Do not use its line numbers, reachability, or counts as evidence for a different revision without rechecking them.
