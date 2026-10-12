---
status: in_progress
created: 2026-10-11
requirements:
  - REQ-TASKS-QUEUE-RECOVERY-001
system_design:
  - ../../specs/tasks/system-design/bounded-queue-recovery.md
---

# Bounded session and direct-child queue recovery
Task: 07b423a1-d52e-40a4-8815-23f6841dbfa9
Session: 904c6405-23d6-4d5c-b7b6-ede866ddd11f
Parent: a68df3ae-aaf5-4591-a46d-9d73db62e46d
Handoff: owner-local-platform-fixes-20261011-v1
Status: implementation in progress. Source checks recorded below; independent gates pending.
Design checklist: complete and approved. Implementation, independent verification and delivery use the work packets below.

## 1. Problem & Constraints
A replacement session cannot recover queued instructions left on its own terminal predecessor, and a parent cannot inspect exact direct-child queues through upstream MCP. Agents need scoped recovery evidence without consuming it or bypassing workspace guards.

Affected layer: portable Go control-plane messagequeue domain and authenticated in-session MCP transport. The server is Docker-hosted, and local/worktree agents inherit that server container environment. This is independent of the task Docker executor. No Dockerfile, image fix, executor change, raw Docker access, live database inspection, restart, or deployment is required.

Evidence inspected read-only on 2026-10-11:
- GitHub commits/main returns e8c1761cb5b7006b905c0aa995925372ce8664e1. Canonical git grep for all four tool names returns no matches at that main and exact v0.97.0 e43881c7555372897b57ec51c705f1e05da43c40.
- Local commit 360ea9139e3e54c452aba16aea631564dd92fd2a supplies census and terminal reader; 75efb8ec3dbdb686619995271773022dbee045c4 supplies child inventory/body reader. Both canonical objects inspected without checkout/fetch/stash.
- Overlay baseline 37b8f8a744ea57fe212e8a72d7ea879b37809615 contains those readers. Available deployment README records v0.96 reconstruction including both commits. Workspace-adoption patch has separate admission/registration ownership.
- This assigned checkout HEAD is cfd6bdf65a, not upstream/main; status is clean at inspection. Preserve any later user edits. Never implement against this divergent overlay by blindly copying commits.
- Requested .local-hotfix/queue-recovery-46e8.md and .local-hotfix/child-queue-recovery-197e.md were not present under /data/home/Code/kandev nor in the overlay tree. Their contents are not claimed verified. Canonical implementation/test objects plus the available README are the planning evidence. Coordinator may supply exact notes before delivery evidence is finalized.
- README, architecture, backend guide, Go module, queue repository/service/types, principal, target access and MCP profiles inspected. SQL and memory Repository.Snapshot mutate status_generation; memory identity binding may also mutate. Do not reuse this path for read-only inspection.
- Existing queue capacities can be unlimited; a body byte limit alone does not bound metadata, empty-message fragments, source memory, sessions, or SQL work.
- Source-level audit only: no claim of unmodified release runtime acceptance.

Related work and supersession:
- Task ca015838-e5cf-4294-b3bb-9c50576a5fe6 / open PR #3377 at 37ca515b0c8fd94cc98b38f898a9335765378ac2 already registers get_message_queue_census_kandev and an own-session authority helper. Reuse the exact census contract/implementation where it meets this plan; coordinate ownership before touching overlapping paths. Do not duplicate registration or import its disposal, routine wakes, or unrelated runtime fixes. This overlap supersedes any proposal to independently recreate the census.
- Open PR #2974 at b542ba3a580b90654ca71c9c099cdc360dae2973 provides task inbox/transcript aggregation; it excludes reserved rows and has broader conversational semantics. Reuse safe projections if useful, but it is not immutable terminal/direct-child recovery.
- Open PR #4418 at 52156c50f38453e6479c7288a655ffd62d22e087 concerns historical durable-delivery payload retention, not these four readers. Do not expand this queue reader to delivery journals.
- Task 86c8b47e-e7a5-4693-8e11-dce08899a0bf / fork PR #15 preserves unread queues on session deletion. Coordinate evidence availability; do not reproduce deletion behavior.
- Retirement task 8ddb2808-39e8-4c84-b1f2-25727d10ae11 owns preservation and retirement. Canonical #3937 merged dfce4dac05809c0fcec156166f5479f14b0cdb76; #3988 merged 64f8830d0d7b1ea98da96d3373d68bc0386cf49e. Main's task-03-preservation-handoff.md requires per-session FIFO identities and separate replacement acknowledgement, archive-byte and dispatch-claim evidence.
- Retention task 3cf0e07b-eac6-4703-bd5d-5d31920bf3e0 / #3930 remains open at 6c38a372382aae728a249188c508cc795030f0c2. Silent Done routing remains owned by 2473291a-614b-418f-9cb1-8b73c0ea7b57. No duplicate terminal tasks.
Recheck provider state and exact upstream head at implementation and before publishing; these observations are not permanent merge receipts.

## 2. Acceptance Criteria
AC1: get_message_queue_census_kandev accepts no caller-supplied target. Trusted caller task/session/workspace identify the current session. Return body-free ordered pending IDs, positions, hashes, sizes, safe provenance, capacity and auto_run; reserved entries are excluded from visible pending count. No content, attachment data, metadata payload or unrelated identifiers leak.
AC2: get_terminal_session_message_queue_kandev requires an exact session ID on the caller's own task, different from the calling session, in CANCELLED, FAILED or COMPLETED. No primary/latest fallback. Live predecessor states return conflict; foreign/missing/archived authority is uniformly denied.
AC3: get_child_queue_inventory_kandev requires exact same-workspace target.ParentID == callerTask.ID and returns bounded body-free session IDs/states and FIFO manifests. Self, sibling, parent, grandchild, unrelated and cross-workspace targets fail even if ordinary task visibility permits them.
AC4: get_child_session_message_queue_kandev requires exact child task/session membership on each call, with the same direct-parent check. Child sessions may be live or terminal; a live queue may invalidate pagination. Missing/foreign session does not fall back. Inventory snapshot can be pinned on the first body call.
AC5: Every recovery manifest/page binds workspace, exact task, session incarnation, ordered entry IDs/positions, original body SHA-256 and byte counts, attachment/settings digest, reservation observation and canonical schema version. Fragments carry ID, full body hash/length, byte offset/length, completion flag and bounded UTF-8 body. Concatenation exactly reconstructs source bodies in FIFO order, including empty and identical-body entries. Attachment/settings digests are opaque evidence, not exposed payloads.
AC6: Continuation reauthorizes live authority and compares the complete snapshot digest before returning any body. Added/deleted/edited/reordered entries, changed attachment/settings/reservation observation, changed incarnation or revoked parent/workspace authority reject continuation with no partial data. Pin mismatch on the inventory-to-first-page transition rejects likewise.
AC7: No read calls queue reserve/dequeue/delete/ack/restore/transfer/dispose/send/recover, modifies auto_run/status generations/pending moves/edit leases/delivery claims, emits queue-status events, starts inference or changes task/session state. Empty FIFO is not proof of absent ordinary/SendNow claims or preserved replacement intake.
AC8: Enforce hard source and response limits; oversize inventory is rejected with a typed limit error and never presented as a complete receipt. Defaults: body budget 65,536 bytes; allowed 1,024..262,144. Max 256 sessions, 1,024 entries per session, 16 MiB aggregate source payload per session, 32 MiB across an inventory, 256 fragments/page, 2 MiB serialized response, 4 KiB cursor. Bound source work before loading oversized bodies/attachments and stop scans at cap+1. Metadata cannot evade limits through zero-length bodies.
AC9: Only authenticated task MCP surfaces (Kanban and Office task profiles) expose tools; managed allowlists retain their existing restrictions. External/configuration/automation/coordinator conversation surfaces gain no ambient access. Backend rejects raw WS mcp actions and forged principal/task/session payloads. No role label or plugin name grants child authority.
AC10: Tests prove SQLite, memory and supported PostgreSQL parity, privacy/non-disclosure, bounds, FIFO reconstruction and zero mutations, plus real server forwarding through authenticated backend dispatch. Public documentation describes scope/errors/retry and source-level versus runtime evidence.
AC11: One narrow source PR with exact-head test receipts. Record canonical merge SHA, containing release tag/source SHA and published backend/agentctl artifact identities, then isolated unmodified-release acceptance of all four use cases plus negative privacy/no-write tests. Overlay parity record names 360ea9139/75efb8ec3 and superseded behavior. No overlay removal or deployment occurs in this task's triage/design.

## 3. Approaches Considered
A. Recommended: core queue domain read-only primitive with thin task MCP adapters. Backend owns storage, immutable identity and authorization. Portable across executors and SQLite/Postgres; small public contract, direct regression coverage.
B. Plugin-owned reader via Host API: a plugin could own recovery policy, but it still needs a new core bounded queue API and exact caller authority. Direct DB/filesystem access violates ADR 0043 and scope. Adds a repository/release dependency without solving the missing primitive.
C. Deployment-only binary/image overlay: preserves local tools short term, but cannot supply upstream portable contract/release coverage. Keep only as rollback until source and release parity proven.

API owner: core internal/orchestrator/messagequeue owns canonical snapshot/hash/bounds; internal/mcp/handlers owns trusted caller/direct-child/terminal authority; internal/mcp/server owns schema/forwarding and profile metadata. The source Coordinator owns one upstream PR and duplicate-work reconciliation. Plugin policy may later consume sanctioned core evidence; plugin Host/proto/SDK expansion is not included. Deployment owner exclusively owns release rollout and overlay removal.
Record this ownership decision through /record before production coding in implementation; planned ADR below. Existing ADRs on authenticated MCP actions, tool profiles, server-owned auto_run and plugin Host APIs remain authoritative.

## 4. Assumptions Made
1. May we use the explicit limits in AC8, rejecting oversized complete inventories rather than returning misleading partial receipts? Chosen to bound unbounded queues while retaining existing body budgets. If vetoed, revise limits and boundary fixtures before coding; do not weaken boundedness.
2. May we add optional expected_snapshot_sha256 to body readers, particularly the first child body page, while preserving existing session_id/task_id/cursor/max_body_bytes calls? Chosen to close the inventory-to-first-body race without breaking existing callers. If vetoed, return an equivalent pinned start cursor from inventory and revise compatibility tests.
3. May we retain ordinary Kanban/Office task scope and exclude external, automation, configuration and special coordinator conversation surfaces? Chosen because own-task/direct-child authority is the requested portable boundary and new surfaces require distinct authorization design. If vetoed, design the explicit surface contract before exposing it.

## 5. Author Decisions
No additional answers requested. Author requirements already fixed: four audited readers; own terminal/direct-child authority; FIFO/hash/bounded fragments; changed-snapshot rejection; no queue mutation/unrelated workspace access; source audit against stated baselines; reuse equivalent work; server Docker hosting distinguished from task executor; no live deployment. Phase allows only tool state changes, so no local PROMPT.md/spec/ADR/task files were written.

## 6. File Map
All paths repository-native, no submodules.
Create during implementation:
- docs/specs/bounded-queue-recovery/spec.md and index entry: behavior/use cases/ACs.
- docs/decisions/2026-10-11-core-bounded-queue-recovery.md and decisions index: core/plugin/deployment ownership.
- docs/plans/bounded-queue-recovery/plan.md and task-01-contract.md through task-06-delivery.md: materialize this saved package, statuses/results.
- Local-only PROMPT.md: task/session identity, orchestration rules, Coordinator and sibling ownership; never stage/commit/push.
- apps/backend/internal/orchestrator/messagequeue/recovery_read.go, recovery_snapshot.go, recovery_census.go and corresponding tests: bounded immutable read, typed projections, digest, cursor/UTF-8 paging.
- apps/backend/internal/orchestrator/messagequeue/repository_recovery_read.go and memory implementation, plus SQLite/Postgres conformance tests: pure observation adapter. Use existing read-only connection and SQL dialect helpers; no schema migration.
- apps/backend/internal/mcp/handlers/message_queue_recovery.go and tests: terminal and child gates, census reuse adapter, typed errors, body-free logs.
- apps/backend/internal/mcp/server/message_queue_recovery.go and tests: registrations/forwarding/compatibility tests.
Modify only where required:
- messagequeue/repository.go, repository_memory.go, repository_sqlite.go, service.go: narrow read interface, no mutation semantics.
- mcp/handlers/handlers.go and existing queueManager dependency: wire route handlers/read interface.
- mcp/server/handlers.go, server.go, server_test.go, tool_metadata_test.go: exact registration counts/profile/risk annotations.
- pkg/websocket/actions.go and gateway websocket dispatch tests: four internal action constants/raw-client denial.
- mcp/profile/profile_test.go (and profile policy files only if actually required): prove existing managed restrictions and no ambient surface expansion.
- docs/public/automation-and-mcp.md: reference for authority, bounds, pagination and retries. Public reference impact must run /docs-maintainer.
- docs/plans/guarded-exact-task-retirement/task-03-preservation-handoff.md: coordinate source-reference linkage only with owner; do not change retirement eligibility.
No frontend UI, generated protobuf, package lock, Docker image or executor paths anticipated.

## Work packets

- [task-01-contract](task-01-contract.md)
- [task-02-read](task-02-read.md)
- [task-03-projection](task-03-projection.md)
- [task-04-authority](task-04-authority.md)
- [task-05-mcp](task-05-mcp.md)
- [task-06-delivery](task-06-delivery.md)

## 7. Implementation Steps
Six ordered work packets; execute in this primary session. Each repository task file records status, wave, dependencies, plan and contract links, acceptance criteria, owned files, verification commands and risks. No subagents or additional platform tasks authorized.

1. Contract/reuse package (task-01-contract; depends_on none).
Refresh upstream/main and linked PR/task states. Coordinate #3377 census overlap with ca015838 and Coordinator: reuse its read-only subset/contract if suitable; do not wait for or import unrelated disposal/runtime work. Record which implementation supersedes local census behavior. Materialize spec, ADR, plan and six task files before code. Establish exact upstream source baseline without changing the user's checkout. Create local PROMPT.md when writes are allowed. AC1-11 mapped; verification: python3 scripts/list-docs.py validate; python3 scripts/lint-spec-files.py --all; git diff --check. Risk: overlapping pending work; ownership resolved before source edits.

2. Pure bounded repository observation (task-02-read; depends_on 1).
Implement memory mutex-protected deep copies with no bind/generation writes, and a bounded SQL read-only consistent transaction using ro connection with SQLite/Postgres semantics. Observe task/session/workspace/incarnation and relevant parent relationships in the same transaction or equivalent authority fence; no lock-row upsert/UPDATE. Inventory session discovery must be bounded and consistent, not 256 individually unbounded reads. Use size preflight and cap+1 scans before loading payloads. Rows with mismatched task/session identity fail closed. Reuse read-only identity code only if verified nonmutating. AC3/6/7/8/10; verification: cd apps/backend && go test -count=1 ./internal/orchestrator/messagequeue -run 'TestRecoveryRead'; then go test -race -count=1 ./internal/orchestrator/messagequeue -run 'TestRecoveryRead'. Same suite runs PostgreSQL cases when KANDEV_TEST_POSTGRES_DSN is supplied through the supported task-owned fixture. A skipped Postgres branch is not acceptance.

3. Canonical manifest/census/body pages (task-03-projection; depends_on 2).
Port behavioral fixtures from local recovery tests, not blind cherry-picks. Use explicit versioned digest fields; JSON of QueuedMessage omits private claim/reservation fields and alone is insufficient. Bind identity, order, body and metadata/attachment/settings digest and durable observable reservation evidence; never serialize raw metadata/attachments in output. Current census filters reserved entries; recovery manifest includes retained reserved rows as observed, without claiming coverage of removed dispatch sources. Validate cursor size/version/identity/digest/index/UTF-8 boundary, reject malformed/trailing/unknown data, enforce fragment and serialized output bounds. Returned cursor is untrusted input and confers no authority; reauthorize every call. No retained snapshot cache or mutation-based generation scheme. AC1/5/6/7/8; verification: cd apps/backend && go test -count=1 ./internal/orchestrator/messagequeue -run 'Test(RecoverySnapshot|RecoveryCensus)'; then same command with -race.

4. Trusted handler authority (task-04-authority; depends_on 3).
Derive caller solely from mcpscope principal; validate live caller task/session/workspace and ordinary access policy. Own-terminal exact state gates; child exact ParentID/workspace gates on every read; deny archived/missing targets uniformly. Bind repository observation to these identities and invalidate if authority/incarnation changes between validation/read. Return safe stable typed denial/conflict/limit/internal errors, never raw SQL/source content. AC2/3/4/6/7/9; verification: cd apps/backend && go test -count=1 ./internal/mcp/handlers -run 'Test(QueueRecovery|TerminalQueueRecovery|ChildQueueRecovery|QueueCensus)'; repeat with -race for interleavings.

5. MCP transport/profile integration (task-05-mcp; depends_on 4).
Register exact four tools once, reuse #3377 census contract, body params remain backwards compatible plus pinned snapshot option. Wrappers inject own task/current session, never allow payload principal override; forwarding returns actual typed page/error, no fallback. ReadOnly=true, Destructive=false, Idempotent=true, OpenWorld=false. Existing task profiles/managed tool restrictions preserved. Integration tests traverse real server wrapper and authenticated dispatch into repository fixture and exercise raw WS denial. AC1-10; verification: cd apps/backend && go test -count=1 ./internal/mcp/server ./internal/mcp/handlers ./internal/gateway/websocket -run 'Test(QueueRecovery|CoreToolRiskAnnotations|RegisterTools_LoggedCountMatchesRegisteredTools|MCPAction)'. Add new tests under these prefixes so exact commands execute them.

6. Documentation/delivery/retirement receipt (task-06-delivery; depends_on 5).
Run /docs-maintainer for public reference; reconcile task/spec/ADR results. Recheck main/open PRs and equivalent merges immediately before /commit, /push, /pr. One narrow PR owned by source Coordinator; no queue disposal, retention or retirement implementation. PR review/CI remediation follows repository skills and task commands. Collect merge/release identities and isolated acceptance when available; report source completion separately from deployment acceptance. Update saved task evidence and notify retirement/retention/routing owners about available reads and remaining limits. Verification: python3 scripts/list-docs.py validate; python3 scripts/lint-spec-files.py --all; git diff --check; affected Go checks from steps 2-5 plus task-required changed-code lint. No broad verification added by default. No actual deployment/removal performed.

## 8. Testing Strategy
TDD after explicit implementation entry; no tests changed or run during this read-only design phase.
- Unit AC1/5/6/8: exact hashes, FIFO empty/duplicate bodies, multi-page multibyte reconstruction, valid exact budget endpoints, malformed cursor/offset/version/target, entries with large metadata/attachments, >256 empty fragments, source and wire caps. Full page digest stable across reads; edits/order/reservation/attachment/settings/incarnation changes reject.
- Memory/SQLite/Postgres AC6/7/10: deep-copy immutability and transaction consistency; snapshot all relevant durable tables/generations/pending moves/claims before/after success and every error. Assert zero write calls using recording repository and read-only DB handle. Race queue mutations with readers; observe one consistent snapshot or reject, never mixed content. Empty FIFO with claim remains explicitly not a retirement receipt.
- Handler privacy AC2/3/4/9: all three terminal states accepted only on own exact predecessor; STARTING/RUNNING/IDLE/WAITING/PAUSED not terminal. Exhaustive relationship matrix including sibling/grandchild/cross-workspace and mismatched session. Test missing/foreign identical responses and no queue calls before denial; principal absent/automation/spoofed payload/archived caller/workspace missing/reparent after first page fails. Privileged workspace visibility never widens direct-parent authority. Capture logs to prove bodies/metadata/attachment paths and tokens absent.
- Transport AC9/10: actual registered schema, annotation count, profile allowlist, trusted forwarding and raw WebSocket mcp.* deny tests. Avoid tests that only mirror helper code.
- Release AC11: task-owned isolated fixture on unmodified containing release verifies all four tools, long-body FIFO reassembly, hash pin/rejection and negative privacy/no-write checks. Run host-native fixture and Docker-hosted SERVER with local/worktree agent path where available. Task Docker executor is unnecessary for this defect. No live deployment or real queue consumption. Archive exact commands/output and artifact SHA/digest, not an invented PASS.

## 9. Submodule Dependencies
None. git submodule status returned no entries; canonical tree has no .gitmodules or gitlink entries. All proposed paths are in the main repository. One repository and one PR; no persistent subtasks needed.

## 10. Risks & Open Questions
No unanswered blocking author questions.
- Missing two incident note files limits deployment-history detail. Obtain minimum exact notes through Coordinator if needed before release parity; do not scan other workspaces or recover via host Docker.
- #3377 current census reuse is mandatory coordination; its unmerged head is not release evidence. Later merges may reduce remaining scope.
- Existing Snapshot mutates, ListBySession is unbounded, JSON excludes private delivery fields. Regression tests must catch these tempting unsafe shortcuts.
- Body contents are intentionally readable only within exact task/child authority. Hashes also remain scoped. Digests of attachment/settings evidence do not grant access to attachment bytes or secrets.
- Snapshot means immutable observed contents with optimistic consistency across calls, not a frozen queue or durable transfer. Live child queue churn may require restarting at first page. No history-of-every-intermediate-mutation guarantee is asserted.
- SQL read consistency/auth authority must be proven on both databases. If ro transaction support is unavailable, fail closed and fix the observation adapter rather than reuse mutating snapshot.
- Overlay retirement gate: containing canonical release plus exact artifact identity and isolated runtime PASS are needed. Removing queue patches from a shared backend/agentctl overlay must preserve every other still-needed capability (retention, routing, workspace adoption, PR URL linking, etc.). Only deployment owner executes safe rollout after all relevant overlays have parity. Release acceptance is pending, not a spec blocker.

## 11. Out of Scope
Queue consumption, acknowledgement, replay, transfer, disposal, reordering or claim settlement; replacement-session-bound intake acknowledgement; archive-byte verification; pending-move cancellation; exact retirement/physical cleanup; terminal retention/silent Done routing; task inbox redesign; provider-specific behavior; plugin RPC/protobuf/SDK implementation; UI; Docker executor/image repair; live deployment/restart or overlay removal; broad unrelated fixes.

Implementation handoff: core bounded read service and task MCP adapters, 6 work packets, 3 vetoable assumptions, no submodules. Next action is explicit implementation entry. No human answer needed for this saved design; release/overlay retirement evidence remains future delivery work.

Phase completion receipt (2026-10-11): STEP 1-3 complete. STEP 4 plan save and exact content readback succeeded (initial version 57497ca9-43e4-40cb-b436-afd0a2644634, 24,827 bytes). Coordinator, retirement owner and #3377 census owner were notified through supported task messages. Todo resolved dynamically from this task's Daily workflow. Move accepted with disposition=deferred and move_id=a6fc5f40-cd50-4864-b2ff-dedeed44e8ce; subsequent workflow readback still reports Spec 2352cf1e-3005-4602-9f4d-41215a5b399a. Todo a1ffe5cc-d614-45ae-a5aa-afcd89d65557 is the accepted deferred destination, not an applied move. Application must be verified after turn end; do not retry the accepted deferred move. No source/filesystem/deployment changes. Next: explicit implementation entry using the saved six-step package. No author input pending. Release and overlay removal remain future acceptance gates.

## Implementation record

Approved user implementation turn. Current main advanced to 1bba2c18b2c0142474b5f0f8b7f51ec1af109691 (#4418 merged). Current specification layout requires docs/specs/tasks/requirements/bounded-queue-recovery.md and system-design/bounded-queue-recovery.md, superseding saved legacy spec.md path. Census owner response and PostgreSQL delivery pending. Main is sole writer. All implementation acceptance results pending.



## Current source validation receipt

Branch: feature/bounded-queue-recovery-main. Immutable base: 1bba2c18b2c0142474b5f0f8b7f51ec1af109691.

Latest user instruction narrows this candidate to the three remaining terminal/child readers and the reusable pure observation core. Census task ca015838 retains registration/handler/removal ownership. Its stable source branch is queue-census-routine-envelope-20260929 at 354a7c724228e0049a977b1bf0ff49a2684c29f4; historical PR #3377 remains 37ca515. Owner granted serialized recovery-only registration hooks, no census/removal edits. Pure RecoveryRead is the agreed future census dependency; mutating Service.Snapshot is rejected. AC1's census delivery remains with that owner, while this candidate implements AC2-10's remaining-reader behavior. AC11 merge/release/overlay retirement evidence is a later delivery gate, not claimed by source tests.

Current upstream spec convention supersedes legacy spec.md paths: requirements and design live under docs/specs/tasks/. The static catalog discovers them without a generated index entry.

TDD receipts: initial observer tests failed on absent nonmutating RecoveryRead, then passed memory/SQLite. Initial page tests failed on absent RecoverySnapshotPage, then passed reconstruction/hash/pinned continuation. Catalog and dispatch tests failed on missing registered readers/UNKNOWN_ACTION, then passed. Legacy policy test found a binding write and passed after pure lookup replaced the helper. Log-privacy test found argument logging and passed with sensitive wrappers. Oversize metadata test found encoding before preflight and passed after bounded value traversal. Bounded session discovery failed on missing capability; source-limit error mapping was later strengthened to the shared typed limit.

Focused commands passed on this worktree before candidate freeze:
- go test -race -count=1 ./internal/orchestrator/messagequeue ./internal/mcp/handlers ./internal/mcp/server ./internal/task/repository/sqlite -run 'Test(RecoveryRead|RecoverySnapshot|QueueRecovery|TerminalQueueRecovery|ChildQueueRecovery|CoreToolRiskAnnotations|RegisterTools_LoggedCountMatchesRegisteredTools)' (all four packages; PostgreSQL cases skipped without fixture).
- Authenticated transport path test: TestQueueRecoveryAuthenticatedTransportPreservesSource passed, registered tool -> authenticated backend dispatcher -> handler -> repository fixture, with unchanged queue observation.
- Raw-WS tests: TestReadPumpRefusesMCPActionsOverTheRawSocket, TestClient_HandleMessageRejectsMCPNamespaceBeforeDispatch and TestDispatcher_DirectMCPDispatchRemainsAvailable passed.
- Changed-code Go lint returned zero issues before the final metadata preflight/cursor coverage delta; final delta must be rerun before freeze.
- python3 scripts/list-docs.py validate; python3 scripts/lint-spec-files.py --all; git diff --check passed.
- node --test scripts/validate-public-docs.test.mjs: 63 passed; node scripts/validate-public-docs.mjs: 47 pages validated.

TEST_RUNTIME: TRANSIENT. Tests use repository-owned isolated SQLite/memory fixtures and unique temporary paths; no server, production queue, Docker executor, LAN endpoint or deployment was used. PostgreSQL requires Coordinator-delivered task-owned endpoint/receipt; requests are pending. KANDEV_TEST_POSTGRES_DSN is absent, postgres/initdb are not on PATH, and PostgreSQL remains UNVERIFIED. Do not borrow ca015838's DSN or credentials. No submodules. UI_VISUAL_CHANGE: NO (MCP read API only, no rendered state changes); screenshots are inapplicable.

REVIEW_RESULT: PENDING. QA_RESULT: PENDING. Two distinct Kandev sessions will inspect the frozen candidate; neither gate is waived. No PR/routing/completion signal before source checks and independent gates pass. Release artifact acceptance and physical overlay removal remain outside this work phase.


## Fixed during review

Round 1 on 513fd1a943511720fc0767a720fef1b16cd5a82f returned two confirmed in-scope findings. REVIEW session 58c8aff8-48f8-4418-a0b7-f327348402d8 reported QR-REVIEW-001: SQL float64 metadata decoding hid changes between adjacent integers above 2^53. QA session 1e04753d-dce9-417b-a297-787fb3cff81f reported QAR-001: final inventory wrappers could exceed 2 MiB although domain manifests passed their cap. Both sessions reached WAITING_FOR_INPUT before correction.

MAIN reproduced both with permanent tests before production edits. The large-number setting regression passed memory, failed SQLite (no changed-snapshot rejection), and skipped unavailable PostgreSQL. The final inventory regression returned a successful 2,097,287-byte payload above its 2,097,152-byte cap. A separate MCP text-wrapper regression also failed because nested encoding exceeded the cap.

Corrections stay in recovery-only paths: scanRecoveryRow decodes metadata losslessly with UseNumber without changing the shared delivery scanner. The shared scalar contract internal/task/contract/queue_recovery.go defines the 2 MiB limit across producer/transport boundaries; serialized WebSocket response and MCP tool-result wrappers are included, transport framing excluded. Final envelope bounds return safe errors rather than body data. Permanent regressions now reject changed large settings, verify equivalent changed-metadata hashes across repositories, and cover final inventory and MCP wrapper overflow. Existing dispatcher/profile registration hooks and census/removal/routine files remain unchanged by these corrections.

Targeted correction and focused race checks passed memory/SQLite in all affected packages. Changed-code lint: zero issues. PostgreSQL remains operationally UNVERIFIED, not a source failure claim. Final new candidate and both re-verification verdicts must be recorded in task metadata; old reports do not cover the correction commit until both sessions inspect/retest its delta.

## Follow-up tasks created

None. Both findings fixed in scope; no duplicate census/retirement/terminal tasks.

## Action required by author

No new product/security decision. Coordinator-delivered task-owned PostgreSQL fixture/receipt remains an essential execution prerequisite. No borrowed credentials, host Docker access or live data are permitted.

## QA fixes worth reviewer attention

The response bound now includes wrapper bytes and lossless number decoding preserves immutable settings evidence. Public docs and system design explain both boundaries.

## Assumptions made during QA

MCP tool-result and WebSocket response serialization include their wrappers in the 2 MiB bound; lower transport framing is excluded. This safe interpretation tightens the approved output bound.

## Pre-existing issues found

None established. Missing PostgreSQL endpoint/server binaries is an environment limitation.

## UI screenshots for PR/MR

Not applicable: UI_VISUAL_CHANGE NO. No rendered feature change or persistent runtime.
