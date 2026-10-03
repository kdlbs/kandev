---
created: 2026-09-25
status: complete
requirements:
  - REQ-EXECUTORS-CURSOR-CLOUD-001
  - REQ-EXECUTORS-CURSOR-CLOUD-002
  - REQ-EXECUTORS-CURSOR-CLOUD-003
  - REQ-EXECUTORS-CURSOR-CLOUD-004
  - REQ-EXECUTORS-CURSOR-CLOUD-005
  - REQ-EXECUTORS-CURSOR-CLOUD-006
system_design:
  - ../../specs/executors/system-design/cursor-cloud.md
legacy_specs: []
---

# Implementation plan: Cursor Cloud execution

## Overview

Add Cursor Cloud as a managed remote-agent runtime for normal Kandev tasks.
Build the provider client, admission, persistence, and scoped tools before connecting paid execution.
This backend-first order protects remote authority and paid submission identity before exposing launch controls.
Task 05 is the first composed start/follow-up slice through the real backend with a fake provider; task 06 adds streamed completion.
Task 07 adds the first browser slice. Earlier tasks remain independently testable behind the off flag; they do not replace a working executor.
Then integrate activity and recovery, expose desktop/phone flows, and prove failure behavior with isolated E2E fixtures.
All eight work orders are sequential. The package itself does not authorize implementation or delegation; the user explicitly requested sequential implementation after the design handoff.

## Scope

### In scope

- One GitHub repository, a published starting ref, text prompts, and Cursor-hosted compute.
- Dedicated cloud agent/executor profiles, secret references, model discovery, and connection diagnostics.
- Start, queued follow-up, streamed activity, confirmed cancellation, backend recovery, and branch/PR results.
- Authenticated task-scoped MCP tools, desktop/phone parity, and disabled-by-default rollout.

### Out of scope

- Office, autopilot, automation, utility agents, passthrough, and dynamic provider routing.
- Local workspace synchronization, terminal, editor, LSP, preview ports, and direct Git mutation.
- Multi-repository tasks, images/files, arbitrary environment injection, artifacts UI, and billing dashboards.
- Private worker pools, existing PR takeover, local/cloud conversation migration, and automatic provider deletion.
- New concurrency/spending controls per installation or profile; existing task admission limits still apply.

## Planning assumptions

The user requested a plan after the feasibility assessment. These release defaults are proposed for review, not shipped contracts.
A session owns one cloud conversation. Cursor owns its output branch. Automatic PR creation defaults off.
Operators provide a reachable HTTPS callback; this package does not create tunnels or change public exposure.
API credentials use the existing global-secret reference contract for shared executor profiles.
Authorized profile users share the key owner's Cursor billing and attribution; dispatch rechecks existing profile-use and task/workspace authority.
Account entitlement and actual callback reachability remain live rollout checks. No credentials or paid execution were used during planning.

## Technical approach

### Runtime and task integration

Use the existing `internal/agent/runtime` facade as a boundary, not as a completed provider abstraction.
Add a proposed RuntimeRouter and cursorcloud implementation without changing the agentctl ExecutorBackend contract.
Adapt the `AgentManagerClient` seam in `internal/orchestrator/executor` and wire all launch, prompt, stop, lookup, and recovery consumers.
The current facade returns ErrUnsupported for SubscribeEvents; cloud observation needs explicit event-pipeline integration.
Keep existing lifecycle behavior behind a compatibility adapter and prove existing executor tests still pass.

### Configuration and security

Add cursor_cloud execution identity, a separate managed agent family, secret references, and callback configuration.
User-confirmed discovery rule: show the Cursor Cloud agent type on the Agents page only after a qualifying executor is saved.
Executor configuration is independently accessible. Last-executor removal hides the type; transient connection failures do not.
Use the existing profile transaction, secret picker, and `ValidateGlobalReference` pattern.
Reject unsupported profile combinations and task modes before a network mutation.
Add features.cursorCloud through the typed flag registry; retain false in every shipped profile.
Expose only scoped MCP grants, bounded to the task session and operation generation.

### Persistence and recovery

Add the managed-agent binding, operation, stream checkpoint, event-receipt, and tool-grant tables defined in the design.
Use unique operation identity, revisions, leases, and atomic message/checkpoint writes.
Preallocate the initial remote agent ID. Treat follow-up submission timeouts as unknown without blind retry.
Restore observers from local bindings, reconcile terminal results once, and integrate remote liveness with task stall detection.
Existing runs retain observation/cancel draining when the feature is disabled. No new dispatch is allowed in that state.

### Repository and UI

Validate a published GitHub ref and use a Cursor-generated branch.
Associate validated result links with the correct repository; do not mutate local worktrees.
Drive server and browser capability checks from the selected execution.
Reuse task layout, profile editor, secret selector, and MobilePickerSheet patterns.
Follow the previews below and keep business logic shared between desktop and phone.

### Source and test patterns

- `apps/backend/internal/agent/runtime/runtime.go`, `facade.go`, and `lifecycle/executor_backend.go`.
- `apps/backend/internal/orchestrator/executor/executor_execute.go`, `executor_resume.go`, and `executor_interaction.go`.
- `apps/backend/internal/backendapp/task_agent_executor_compatibility.go` and its tests.
- `apps/backend/internal/secrets/store.go` and repository SQLite/PostgreSQL migration patterns.
- `apps/backend/internal/mcp/scope`, `profile`, and `server/external_integration_test.go`.
- `apps/web/components/settings/profile-edit/sprites-api-key-card.tsx` and `serialize-executor-config.ts`.
- `apps/web/components/task/task-layout.tsx` and `mobile/mobile-picker-sheet.tsx`.
- `apps/web/e2e/tests/settings/executor-profile-routing.spec.ts` and its mobile counterpart.

## ASCII UI preview

The labels and grouping below are structural requirements; spacing and copy are illustrative.
All final user-facing copy uses localization. Shared state and permission logic serve both layouts.

### UI-00: Executor-first discovery

Entry: Agents page, on desktop and phone. Configure the executor through executor settings first.

```text
No configured executor:   Agents [existing agent types]
After executor is saved:  Agents [existing types] [Cursor Cloud]
Last executor removed:    Agents [existing agent types]
```

No placeholder Cursor Cloud agent card appears before configuration.
Temporary connection errors keep the configured type visible with its error state.
Map: AC-EXECUTORS-CURSOR-CLOUD-001.6 and -001.7.

### UI-01: Profile configuration, ready and error states

Entry: Settings > agent profile > executor configuration. Desktop uses the existing profile page.
Phone uses direct navigation with Back, one scrolling form, and a reachable Save action.

```text
Desktop                              Phone
+--------------------------------+   +--------------------------+
| Cursor Cloud                   |   | < Profiles  Cursor Cloud |
| API key [saved secret v]       |   | API key [saved secret v] |
| Callback [https://host/...   ] |   | Callback [https://...  ] |
| Model [provider model v]      |   | Model [provider model v] |
| [Test connection] Ready       |   | [Test connection]        |
|                        [Save] |   | Ready                    |
+--------------------------------+   |                   [Save] |
                                     +--------------------------+
Checking: Test shows progress; repeat clicks are disabled.
Error:    The failing field shows a readable error and a retry action.
No key:   Select or create a secret; starting work remains unavailable.
Billing:  A visible notice explains that profile users share the key owner's Cursor billing.
```

Map: AC-EXECUTORS-CURSOR-CLOUD-001.1 through -001.5 and -006.1 through -006.4.

### UI-02: Start a cloud task

Entry: existing task start surface. Phone choices use an inset picker drawer; the form remains a focused surface.

```text
[Executor: Cursor Cloud v] [Agent: Cursor Cloud v]
Repository: owner/repo     Published ref: [main v]
Uses published repository content. Local edits stay here.
[ ] Create a pull request automatically
[Cancel]                                    [Start]
```

Starting disables duplicate submission. Invalid profile, repository, model, or callback errors keep the form open.
Map: AC-EXECUTORS-CURSOR-CLOUD-002.1, -004.1, -004.4, and -006.1.

### UI-03: Active conversation and results

```text
Desktop
+-----------------------------------------------------------+
| Task / Cursor Cloud / Running    [Open in Cursor] [Stop]    |
+----------------------------------+------------------------+
| Conversation and tool activity    | Remote results         |
|                                  | Branch: cursor/...     |
| (one scroll region)              | PR: Open when present  |
+----------------------------------+------------------------+
| [Follow-up message                                    ]   |
|                                                [Send]     |
+-----------------------------------------------------------+

Phone
+----------------------------+
| < Tasks  Cursor Cloud  [v] |
| Running             [Stop] |
+----------------------------+
| Conversation               |
| Tool activity              |
| (one scroll region)        |
+----------------------------+
| [Message              ]    |
| [Results]           [Send] |
+----------------------------+
       safe-area clearance

Phone Results drawer
+----------------------------+
| Remote results         [x] |
| Branch: cursor/...         |
| [Open pull request]        |
| [Open in Cursor]           |
+----------------------------+
```

Headers and composer remain fixed within a dynamic-viewport layout. The conversation owns vertical scrolling.
Phone targets are at least 44px. Desktop ordinary controls retain 28px sizing.
Workspace panels and their requests are absent for cloud sessions; desktop layout preferences remain stored.
Map: AC-EXECUTORS-CURSOR-CLOUD-003.1, -004.2, -004.3, and -006.1 through -006.4.

### UI-04: Recovery and uncertain submission

```text
Reconnecting:      Activity connection lost. Retrying...
Cancelling:        Stopping remote work... [Send disabled]
History gap:       Some activity is unavailable. Saved history remains.
Submission unknown:
  Cursor may have received your message.
  [Open in Cursor] [Resolve submission] [Send disabled]

Resolve submission (desktop dialog / phone full-height surface)
  Remote runs: [candidate ID, start time, status]
  [Bind selected run]
  OR [ ] I understand retrying may start duplicate work
     [Retry this message]
```

Resolution uses one scroll owner and preserves focus on dismissal. Candidate binding requires server-side identity checks.
Retry is an explicit acknowledgment, never an automatic response to a timeout.
Map: AC-EXECUTORS-CURSOR-CLOUD-002.3, -002.5, -003.2 through -003.5, and -006.2.

## Tests

All named new tests are planned outputs. Work orders carry runnable commands for their implementation stage.
Use Red-Green-Refactor for changed logic; do not add tests that only repeat field assignments.

| Acceptance criteria (AC-EXECUTORS-CURSOR-CLOUD prefix) | Planned evidence | Work order |
| --- | --- | --- |
| 001.2, 002.5 | `internal/cursorcloud/client_test.go: TestClientAdmissionErrors, TestFollowupTimeoutNoRetry` | 01-cursor-api-client |
| 003.1, 003.3 | `internal/cursorcloud/stream_test.go: TestStreamEvents, TestStreamRetentionExpired` | 01-cursor-api-client |
| 001.1, 001.2, 001.4 | `internal/backendapp/cursor_cloud_admission_test.go: TestCursorCloudAdmission` | 02-profiles-and-capabilities |
| 001.3 | `internal/backendapp/cursor_cloud_gate_test.go: TestCursorCloudDisabledEntryPoints` | 02-profiles-and-capabilities |
| 004.3 | `internal/backendapp/cursor_cloud_capabilities_test.go: TestCursorCloudWorkspaceDenied` | 02-profiles-and-capabilities |
| 002.1, 002.2, 002.5 | `internal/task/repository/sqlite/managed_agent_operations_test.go: TestManagedOperationSingleWriter, TestManagedOperationUnknown` | 03-durable-submissions |
| 002.2, 003.2 | `internal/task/repository/sqlite/managed_agent_postgres_test.go: TestPostgresManagedSingleWriter, TestPostgresManagedRecovery` | 03-durable-submissions |
| 003.2, 003.5 | `internal/task/repository/sqlite/managed_agent_recovery_test.go: TestManagedBindingReopen, TestManagedCheckpointTransaction` | 03-durable-submissions |
| 005.1, 005.2, 001.3 | `internal/mcp/managed/transport_test.go: TestGrantScope, TestGrantRevocation, TestDisabledGrant` | 04-scoped-mcp-callback |
| 005.3, 005.4 | `internal/backendapp/cursor_cloud_mcp_test.go: TestCloudQuestionBarrier, TestCloudCompletionGuard, TestCloudCallbackAdmission` | 04-scoped-mcp-callback |
| 002.1, 002.2, 002.5 | `internal/agent/runtime/cursorcloud/dispatch_test.go: TestCreateRecovery, TestSerializedFollowups, TestUnknownSubmission` | 05-runtime-dispatch |
| 004.1, 004.4, 001.4 | `internal/orchestrator/executor/executor_cursor_cloud_test.go: TestCursorCloudLaunch, TestCursorCloudRepositoryAdmission, TestCursorCloudCompatibility` | 05-runtime-dispatch |
| 003.1, 003.2, 003.3 | `internal/agent/runtime/cursorcloud/recovery_test.go: TestReplayCheckpoint, TestRestartObservation, TestRetentionGap` | 06-observation-and-results |
| 002.3, 003.4, 003.5 | `internal/orchestrator/executor/executor_cursor_cloud_recovery_test.go: TestCloudCancelRace, TestCloudAuthFailure, TestCloudLiveness` | 06-observation-and-results |
| 002.4, 002.5, 004.2 | `internal/backendapp/cursor_cloud_results_test.go: TestCloudCompletionGuards, TestResolveUnknownSubmission, TestCloudResultIdentity` | 06-observation-and-results |
| 001.1-001.4, 004.3 | `components/settings/profile-edit/cursor-cloud-config.test.ts: cloud validation; lib/cursor-cloud-capabilities.test.ts: unavailable operations` | 07-desktop-and-phone |
| 002.3, 004.2, 004.4, 006.1-006.4 | `e2e/tests/session/cursor-cloud.spec.ts and mobile-cursor-cloud.spec.ts: ordinary task flow, results, geometry` | 07-desktop-and-phone |
| 002.5, 003.1-003.5, 006.2 | `components/task/cursor-cloud-recovery.test.tsx: recovery rendering only` | 07-desktop-and-phone |
| 001.2, 001.3, 002.4, 002.5, 003.2-003.5 | `e2e/tests/session/cursor-cloud-recovery.spec.ts: restart, unknown submission, drain, provider errors` | 08-failure-e2e-and-docs |
| 005.1-005.4, 006.2 | `e2e/tests/session/mobile-cursor-cloud-recovery.spec.ts: callback rejection, question barrier, recovery controls` | 08-failure-e2e-and-docs |

### Review-added evidence and ownership

| Criteria | Planned evidence | Owner |
| --- | --- | --- |
| 001.6-001.7 | TestCursorCloudAgentDiscovery; desktop/phone executor-first discovery scenarios | 02, 07 |
| 001.3 | TestCursorCloudProdIgnoresMockOrigin, TestCursorCloudDevIgnoresMockOrigin, TestCursorCloudE2ELoopbackCallback | 02 |
| 001.5, 004.5 | TestCursorCloudAdmission, TestCloudFrozenSelection; shared billing and disabled selector UI | 02, 05, 07 |
| 002.6 | TestCloudWorkflowAutoStartSerialized, TestCloudQueueDrainOnce | 05; failure E2E in 08 |
| 002.7 | TestCloudArchiveRunning, TestCloudArchiveUnknown, TestCloudUnarchiveNoAutoLaunch | 06; E2E in 08 |
| 003.5 | TestCloudNeverStartedGuard, TestCloudStuckSignalGuard, TestCloudStartupRecovery | 06 |
| 003.1-003.5, 002.5 | cursor-cloud-recovery.test.tsx renders UI-04; no injected failure-flow E2E here | 07 |
| 002.5, 003.1-003.5 | cursor-cloud-recovery.spec.ts and mobile-cursor-cloud-recovery.spec.ts inject failures and resolve unknown submissions | 08 |

## E2E tests

Use the real isolated backend, database, and frontend with a local Cursor HTTP/SSE fixture.
Do not mock only the browser API: tests must exercise runtime routing and persistence.
Task 07 owns baseline fixtures, project registration, UI components, and ordinary flow E2E.
Task 08 alone owns failure controls and E2E unknown-submission recovery. Task 07 proves recovery rendering with component tests.
Use dedicated `cursor-cloud` and `cursor-cloud-mobile` projects; exclude their files from ordinary chromium/mobile-chrome.
A worker-local HTTP/SSE fixture starts before backend.restart with explicit feature=true, mock selector, and allocated loopback origin.
Restore fixture baseline between specs and the original baseline on teardown. Feature defaults remain off, including the e2e profile.
Mock origin and loopback HTTP callback exceptions require resolved e2e profile plus mock selector; production/dev ignore them.
Task 02 owns those backend gates and negative tests. Task 07 owns CI project/manifests inclusion and duplicate-discovery tests.
Mock activation uses explicit E2E fixture configuration and is unavailable in production.
The managed E2E runner builds current binaries and enforces worker limits. Do not overlap suites or override workers.

| Flow | File and project | Acceptance criteria |
| --- | --- | --- |
| Configure, start, stream, follow up, stop, results, capability limits | `tests/session/cursor-cloud.spec.ts`, cursor-cloud | 001.1-001.5, 002.1-002.3, 004.1-004.5, 006.1 |
| Same user value, phone navigation, touch, focus, safe areas, no overflow | `tests/session/mobile-cursor-cloud.spec.ts`, cursor-cloud-mobile | 006.1-006.4 |
| Restart, reconnect, history gap, provider errors, unknown submission, disabled draining | `tests/session/cursor-cloud-recovery.spec.ts`, cursor-cloud | 001.2-001.3, 002.4-002.7, 003.1-003.5 |
| Scoped callbacks, pending question, completion rejection, phone recovery | `tests/session/mobile-cursor-cloud-recovery.spec.ts`, cursor-cloud-mobile | 005.1-005.4, 006.2 |

Inspect screenshots from the phone flow against UI-01 through UI-04 during the existing E2E pass.
Check 767px/768px layout boundaries when changing responsive rules. Use the dedicated cursor-cloud-mobile Pixel 5 project for this feature.

## Work orders

- [x] [Task 01: Add the Cursor API client and contract fixtures](task-01-cursor-api-client.md)
- [x] [Task 02: Add gated cloud profiles and execution capabilities](task-02-profiles-and-capabilities.md)
- [x] [Task 03: Persist cloud bindings and submission operations](task-03-durable-submissions.md)
- [x] [Task 04: Add session-scoped cloud MCP callbacks](task-04-scoped-mcp-callback.md)
- [x] [Task 05: Route task launch and follow-ups to Cursor Cloud](task-05-runtime-dispatch.md)
- [x] [Task 06: Integrate cloud activity, recovery, stop, and results](task-06-observation-and-results.md)
- [x] [Task 07: Expose cloud configuration and task controls on desktop and phone](task-07-desktop-and-phone.md)
- [x] [Task 08: Add failure-flow E2E coverage and operator documentation](task-08-failure-e2e-and-docs.md)

## Dependency order

01 -> 02 -> 03 -> 04 -> 05 -> 06 -> 07 -> 08.

Shared identities, migrations, routing, and fixture state make these tasks sequential.
Tasks 01-08 are complete in dependency order. Requirements and system design remain draft artifacts until their review lifecycle permits promotion.

## Verification results

Implementation checks passed on 2026-09-26:

- Web `typecheck`, `lint` (zero warnings), `i18n:zh-hant`, `i18n:check`, `i18n:ratchet`, and `build:e2e` passed. Focused Vitest coverage passed 51/51; E2E runner tests passed 15/15.
- Cursor Cloud browser coverage passed: desktop normal flow 5/5, phone normal flow 4/4, desktop recovery 4/4, and phone recovery 2/2.
- Backend focused packages passed, including Cursor client/runtime, orchestrator executor, task handlers, profile handlers, MCP handlers, and lifecycle. `make build` and `make e2e-plugin-package` passed.
- Public-doc validation passed: 62 validator tests, all 47 public pages, catalog validation (306 decisions and 1146 specifications), full spec lint, and `git diff --check`.
- The serial aggregate `go test ./...` run was not completed. Its process-probe package fails in isolation in this environment (`got "live", want "settled"`); the aggregate run was stopped while in `internal/orchestrator`. Config and launcher checks pass with inherited internal config-path variables unset, and lifecycle passes with a short temp root.

All eight implementation work orders are complete. The build and task-scoped checks pass; the aggregate Go suite remains incomplete for the environment-sensitive process-probe issue above.

The review revision adds explicit fixture isolation, lifecycle mapping, watchdog coverage, and contract gates.
Design-package checks passed on 2026-09-25:

- `python3 scripts/list-docs.py validate`: 306 decisions and 1146 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Local-link and work-order traceability audit: 32 of 32 criteria covered; no unknown IDs or broken file links.
- `git diff --check -- docs/specs docs/decisions docs/plans/cursor-cloud`: passed for tracked changes.
- A separate whitespace scan covers new, untracked package files.
- Catalog queries discover both specifications and the proposed ADR.

Implementation files remain unstaged and uncommitted. Live Cursor entitlement and callback reachability remain unvalidated rollout gates; no credentials or paid execution were used.

## Live rollout procedure

Automated tests never require a Cursor key. Before enabling an installation, use a disposable repository and a dedicated credential.
The operator must authorize this paid smoke test during implementation or rollout.

1. Enable the flag on that installation only and restart.
2. Test the key, published ref, model, and remote callback from an actual cloud run.
3. Exercise a Kandev question and answer, streamed tool activity, a follow-up, and Stop.
4. Restart Kandev during a run; verify the same agent/run reconnects without another submission.
5. Verify the output branch, optional PR, and unchanged local workspace.
6. Disable the flag; verify read/cancel draining and rejection of new work.
7. Record redacted outcomes and remove disposable provider resources through Cursor.

No account access means this gate is not run. Keep shipped defaults off and report the unvalidated rollout explicitly.

## Risks

- Cursor v1 is public beta; contract drift needs fixture updates before rollout.
- Follow-up calls lack a verified idempotency contract. Unknown submission state may require explicit user resolution.
- Runtime facade and orchestrator integration are incomplete; adapter-only tests are insufficient.
- Reachable HTTPS MCP hosting and account entitlement can block actual use despite passing local tests.
- Cloud runs continue while Kandev is down. Secret expiry, provider retention, and external Cursor edits can prevent seamless continuation.
- Cloud capability guards must reach direct HTTP/WS handlers and hidden background subscriptions.

## Related artifacts

- [Requirements and acceptance criteria](../../specs/executors/requirements/cursor-cloud.md).
- [System design](../../specs/executors/system-design/cursor-cloud.md).
- [Proposed architecture decision](../../decisions/2026-09-25-managed-remote-agent-runtime.md).

## Review disposition (2026-09-25)

All fifteen findings were checked against source and incorporated or bounded explicitly:

1. Real-binary E2E has a profile-gated mock origin and loopback callback exception; production/dev negative tests are required.
2. Two dedicated Playwright projects isolate restart environment changes and are included in CI discovery.
3. Current docs specify follow-up MCP replacement and stable create IDs. Task 01 tests those contracts; unsupported behavior blocks rollout without unsafe fallback.
4. The design maps every AgentManagerClient method, with turn cancellation distinct from local execution termination.
5. Task 06 covers lifecycle stalls, orchestrator never-started/stuck-signal watchdogs, and restart reconciliation.
6. Normal workflow prompts and queue drains use journaled operations and preserve step/question guards.
7. Model/profile, plan/permission mode, native permission UI, and context-reset capabilities fail closed after binding.
8. Task archive persists termination intent, retains unresolved cleanup, and does not delete the remote conversation.
9. Task 07 includes all UI-03/UI-04 criteria and owns rendering; task 08 owns injected failure E2E and unknown-submission resolution.
10. Four metrics have explicit labels and increment boundaries; task 08 documents them in root AGENTS.md.
11. Task 02 links the mandatory flag skill and names contract/default files and tests.
12. The backend-first sequence has independently verifiable slices and a documented reason before paid work is exposed.
13. Shared account billing is disclosed, and existing authority is rechecked at dispatch.
14. New profile/installation cost and concurrency limits are explicitly outside this package; existing admission limits still apply.
15. The baseline uses the full runtime/lifecycle path.
