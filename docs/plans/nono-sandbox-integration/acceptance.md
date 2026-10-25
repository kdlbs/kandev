
## Pinned runtime conformance

```json
{
  "scope": "runtime_conformance",
  "status": "passed",
  "version": "0.80.0",
  "source_commit": "8bb041356d48f771a221e58c98d9b1ff442e76fe",
  "archive_sha256": "cae6755f1188b561ea02f62995feb8ad87d457f6d5255956ec54f41bbefca31e",
  "binary_sha256": "a66396777ed756dff196ef61cf704abd4feebf58599d41e2da3363bbe90e22af",
  "platform": "linux/x86_64",
  "checks": {
    "pipe": "passed",
    "filesystem": "passed",
    "network": "passed",
    "approval_denied": "passed",
    "approval_granted": "passed",
    "approval_invalid": "passed",
    "approval_timeout": "passed",
    "pty_input_resize": "passed",
    "cancellation_cleanup": "passed",
    "fresh_restart": "passed"
  },
  "active_status_evidence": "unsupported",
  "limitations": [
    "No current policy digest in the session registry",
    "Fresh runtime restart is not Kandev same-session recovery",
    "No ACP agent, provider login, MCP, callback isolation, or plugin acceptance"
  ],
  "recorded_at": "2026-10-23T18:45:15.113366+00:00"
}
```

## Dedicated plugin compiler checks

The local `kandev-plugin-nono` checkout compiled and executed its generated policy with the pinned Linux amd64 runtime. These tests do not use the Kandev launch path.

- `TestPinnedRuntimeEnforcesCompiledPolicy` passed: permitted work-directory reads/writes, denied outside reads/writes, removal of implicit `/etc/hosts` access, denied direct networking, and child environment filtering.
- `TestPinnedRuntimeEnforcesProxyDestinations` passed: permitted exact host/port through the proxy, denied second live destination, and denied direct networking. The denied destination received no requests.
- The compiler rejects HTTP/HTTPS protocol-specific grants. Its TCP destination grants do not promise application-protocol enforcement.

Commands run from the plugin checkout with `NONO_ACCEPTANCE_BINARY` set to the verified executable:

```bash
go test ./server -run TestPinnedRuntimeEnforcesCompiledPolicy -count=1 -v
go test ./server -run TestPinnedRuntimeEnforcesProxyDestinations -count=1 -v
```

Kandev ACP, discovery, terminal, recovery, approval identity, MCP connectivity, packaging, and full product acceptance remain pending.

## Packaged provider and retained runtime

The dedicated plugin passed `make validate verify-package`. Its two real-runtime tests also parsed the generated audit logs through the provider and returned partial denial reports.

The host ran this check against that package:

```bash
KANDEV_TEST_SANDBOX_PACKAGE=/absolute/path/to/kandev-plugin-nono-0.1.0.tar.gz go test -race ./internal/plugins -run TestSandboxProviderExternalPackageRetainsOwnedRuntime -count=1 -v
```

It passed installation, SDK compilation, verified runtime copying, private artifact creation, and a live confined child. After plugin uninstall, new runtime reads failed and the existing child completed permitted work from the retained attempt files. This does not prove agentctl integration, backend recovery, or complete sandbox acceptance.

## Agentctl launch-envelope checks

Focused race tests pass for generic sandbox envelopes on utility probes, utility prompts, secondary model discovery, persistent commands, one-shot initial/continuation commands, and deferred native PTYs. They cover final environment filtering, wrapper order, changed artifacts, lost recovery configuration, authenticated configuration, and refusal to remove a selected sandbox by omission. Utility failure tests check that runtime diagnostics do not expose private details. These are generic process-boundary checks. Real Nono ACP, terminal, backend restart, approval ownership, and MCP acceptance remain pending.

## Immutable attempt persistence

`TestSandboxAttemptsSQLite` and `TestSandboxAttemptsPostgres` pass under the race detector against real isolated databases. They verify immutable policy and launch snapshots across service reload, stale-selection refusal, versioned lifecycle changes, process ownership retention, and refusal of late running events after stop. These are persistence checks; product restart and adoption remain pending.

## Private registry identity

The rebuilt dedicated plugin passed `make validate verify-package`. `TestSandboxProviderExternalPackageRetainsOwnedRuntime` passed under the race detector with a real Nono child held alive. Core read the bounded declared registry source, normalized its identity, matched the owned supervisor, captured the child birth identity, and verified live Linux ancestry. The response contained no confinement evidence. Private file-source tests reject path escapes, aliases, oversized data, excessive files, and cancelled reads. This does not yet prove operator approval delivery.

## Real operator callback and exact egress

`TestSandboxRuntimeRealApprovalAndPrivateBoundary` now passes against the rebuilt dedicated package and pinned Nono 0.80.0. It performs an actual denied file operation, persists the request, sends a human operator's allow-once decision, and verifies that a second read requires a new decision. Denying that second request preserves the immutable policy. The child cannot read the private profile, supervisor environment or descriptor path, or an unapproved environment variable.

The test first failed when a permitted host at one port could tunnel to the callback at another port. Nono's plain `allow_domain` parser removes port suffixes. Earlier permitted/denied destination checks did not prove exact-port enforcement. The provider now requires core's generic `egress_proxy` capability for network grants. The corrected real-runtime test permits the selected destination and denies both direct and authenticated proxy connections to the callback. Core relay tests cover exact host/port refusal and cancellation of active tunnels. The provider-only pinned-runtime suite passes with a bounded host-relay fixture.

These are actual process, policy, and callback checks. They do not substitute for Kandev ACP, discovery, terminal, restart, MCP, or UI acceptance. Backend admission and peer capability advertisement remain disabled while that wiring is incomplete.


### MCP component round trip (2026-10-24)

`TestSandboxRuntimeRealApprovalAndPrivateBoundary` passed with the packaged Nono 0.80.0 runtime. It initializes MCP and calls the actual Kandev `get_task_plan_kandev` handler against a task-scoped fixture dispatcher. The settings path on the relay is refused. The same run retains the permitted/denied filesystem, environment, callback, and exact-port checks. Policy content remains unchanged.

The core relay race tests pass for path isolation, header filtering, request bounds, redirect refusal, and cancellation. Agentctl control-route tests pass for authentication and live attempt/generation matching. These are component results; discovery, ACP chat, native terminal, restart/recovery, and product UI acceptance remain pending.

### Explicit child state component check, 2026-10-24

The packaged Nono 0.80.0 test passed with `environment.set_vars` bound to a core-owned child home. The child wrote its home/config/state/temporary files. A new attempt retained session content. Supervisor profile/proc access, direct/proxied callback access, and unapproved environment access remained denied. Actual allow-once/deny and MCP tool-call fixtures still passed. The test used a purpose-built child, not a production ACP agent.

Evidence: `/tmp/kandev-nono-state-runtime-green.log`; plugin package checks: `/tmp/kandev-nono-child-state-package.log`; PostgreSQL policy checks: `/tmp/kandev-nono-state-policy-postgres.log`. Full product discovery, ACP, native terminal, restart, cancellation, cleanup, UI, and seed delivery remain open.


### Recovery and startup component checks, 2026-10-24

The pinned package test now records supervisor identity before provider status normalization completes. An uncertain cleanup persists `recovery_required`. A new coordinator rejects live adoption and removes only its captured private runtime files after the process session ends. See `/tmp/kandev-nono-reconcile-green.log`.

SQLite and PostgreSQL pass frozen-policy restart, active-session uniqueness, session history lookup, and idempotent backfill of the session column. See `/tmp/kandev-nono-session-restart-postgres.log`.

Backend factory tests pass local preparation, captured restart, remote-runtime refusal, and competing-start refusal. Recovery admission rejects an attempt with lost backend-owned listeners before command reconstruction or prompt replay. Ordinary recovery remains available. See `/tmp/kandev-nono-backend-recovery-green.log`.

These checks do not prove a complete user recovery journey. Full production startup wiring, failed-start receipt cleanup, settings and approval surfaces, and real discovery/ACP/native-terminal acceptance remain open. Secondary discovery without a separate owned attempt is refused; earlier envelope-only tests do not establish its support.


## Detached descendant cleanup correction (2026-10-24)

A credential-free pinned Nono reproduction created a child with `setsid`, then ended the supervisor. The child remained alive in another process session. The earlier cancellation test covered its fixture only. It did not establish complete descendant cleanup.

`TestSandboxRuntimePreparesAndBindsPackagedProvider` reproduced the false stopped result with the packaged provider. Its new detached-child assertion failed before the fix and passed afterward. Core now owns a cgroup v2 leaf and uses `clone3` to start inside it. Recovery verifies the captured kernel identity, terminates remaining descendants, and removes the empty leaf before retiring the attempt. `TestProcessContainmentOwnsDetachedDescendant` also verifies refusal to signal a changed kernel identity and refusal to remove a populated group.

These Linux checks require a writable delegated cgroup v2 subtree and `clone3`. Set `KANDEV_TEST_SANDBOX_CGROUP=1` to require this prerequisite during verification. The implementation does not modify the main service's controllers or move its processes. Missing support refuses sandbox launches. Generic command wrapping remains independent.

The startup receipt and transport integration checks remain in progress. Full product discovery, ACP, native terminal, restart, UI, and acceptance checks are still pending.


### Utility startup completion receipts

Focused tests now cover authenticated, bounded receipt retrieval, refusal of stale or oversized receipts, and no redirect following. A confirmed failed spawn releases the immutable attempt without a fabricated process owner. Utility cleanup consumes the receipt after dispatch failure and retains the failure result. Completed utility requests terminate their captured detached descendants before issuing a stopped receipt. Selected sandbox errors omit private receipt bodies, including malformed responses.

Persistent ACP and native terminal failed-spawn receipts now have focused component coverage. Both paths reject reuse of a completed attempt. The authenticated persistent receipt route and local native receipt are consumed by lifecycle cleanup. Fast supervisor exit before ownership binding now has passing integration coverage for persistent ACP and native terminals. Captured completion receipts retire stopped attempts after the original request deadline, while preserving the startup failure. These results do not enable launch admission or satisfy full product acceptance.


### Discovery selection wiring

The backend utility factory now creates a separate execution-scoped attempt for each selected profile probe. Its packaged-provider test verifies captured working directories, distinct identities, cleanup, and rejection of stale or missing selection fingerprints. Admission clears prior caller data and resolves current server policy. The discovery fingerprint includes policy revisions, binding versions, and the live provider installation. Provider loss blocks cache access.

The host utility suite passes under the race detector after adding regressions for policy changes during capability and model-option probes. The host rechecks policy before publishing either result. Main runtime activation and full product acceptance remain pending.


### Native MCP artifacts

Generated native config now uses the explicitly granted child home. Core does not fall back to private host configuration when that grant is absent. Linux writes use directory-rooted operations, bounded regular-file reads, and atomic replacement. Focused regressions verify permitted writes, outside-path refusal, parent and leaf symlink refusal, malformed-merge refusal, and cleanup after directory replacement. Existing passthrough MCP tests and sandbox lifecycle tests pass under the race detector. These are transport checks, not actual Pi chat or terminal acceptance.


### Generic settings and policy selection (2026-10-24)

Four desktop and four phone browser flows pass for agent wrappers, executor policy creation/configuration, workflow policy selection, and task policy editing. Each uses generic provider fixtures. Context choices preserve assignment. Task Update saves policy before launch, while Cancel discards its draft. Invalid, failed, and obsolete saves cannot continue to launch. Task previews include workflow inheritance. The phone task-policy screenshot was inspected.

Evidence: `/tmp/kandev-nono-task-edit-desktop3.log`, `/tmp/kandev-nono-task-settings-phone.log`, and `/tmp/kandev-nono-task-context-green.log` (64 focused tests). Typecheck and localization pass. Provider/runtime availability is a separate explicit probe. No fixture result establishes real discovery, ACP, native terminal, restart, cancellation, cleanup, or an active-confinement indicator. Session details, denial review, and full product acceptance remain open.


### Session access for approvals

Approval reads and operator decisions now use the existing session authorizer. A missing authorizer, deleted/inaccessible session, or denied caller returns no records and cannot change the pending decision. The regression failed before wiring this guard. HTTP, SQLite/PostgreSQL approval, and packaged Nono approval integration tests now pass under the race detector. Backend lint passes. Evidence: `/tmp/kandev-nono-session-access-green.log`, `/tmp/kandev-nono-session-access-postgres.log`, and `/tmp/kandev-nono-session-access-nono.log`.


### Immutable public launch history

SQLite and PostgreSQL race checks pass for immutable capture, replay conflicts, concurrent sequence allocation, stale execution refusal, safe command summaries, and schema replay. Read/list projections preserve frozen policy metadata and isolate sibling sessions. Malformed or foreign stored records yield unknown history. Session deletion removes the records. The PostgreSQL deletion fixture includes the queue schema required by the real deletion path. Evidence: `/tmp/kandev-nono-snapshot-projection-green2.log`.

Frontend reconciliation passes stale/foreign update, omitted-field, unknown refresh, replacement execution, immutable sequence, and recreated-session cases. The owning session-store suite passes 428 tests. The final focused run passes 11 tests after a lint refactor. Typecheck and scoped frontend lint pass. Evidence: `/tmp/kandev-nono-snapshot-session-tests.log`, `/tmp/kandev-nono-snapshot-merge-final.log`, `/tmp/kandev-nono-snapshot-typecheck.log`, and `/tmp/kandev-nono-snapshot-web-lint2.log`. Production launch wiring and active-confinement projection remain unfinished.


### Launch capture and session transport

Chat and native-terminal capture precede process start. Failed persistence blocks dispatch. The core recorder works without plugins. The packaged provider check verifies matching attempt identity, policy revision, runtime digest, and generation; mismatched public/private attempt identities are refused. Native ownership checks include capture before dispatch. Evidence: `/tmp/kandev-nono-snapshot-selected-green.log`.

Kubernetes refresh, workspace rebind, and rollback also capture before start. A sandboxed workspace rebind is refused before stopping the existing process, because the frozen scope is tied to its original directory. Existing rebind and selected refresh regressions pass under the race detector: `/tmp/kandev-nono-snapshot-restarts-green.log`.

State-change and reconnect payloads carry the saved snapshot. Focused backend transport checks pass, and 72 frontend handler tests pass with the real session store. Evidence: `/tmp/kandev-nono-snapshot-ws-green.log` and `/tmp/kandev-nono-snapshot-ws-web-green.log`. Immediate publication when capture commits, live confinement evidence, and rendered session details remain pending.


## Execution publication and evidence observation

- `/tmp/kandev-nono-snapshot-publication-green.log`: backend capture notification and gateway workspace scoping pass with race detection.
- `/tmp/kandev-nono-snapshot-publication-web-green.log`: 73 handler tests pass. A separate execution event preserves task state and rejects old session owners.
- `/tmp/kandev-nono-snapshot-publication-typecheck.log` and `/tmp/kandev-nono-snapshot-publication-web-lint.log`: pass.
- `/tmp/kandev-nono-snapshot-go-lint-storage.log`: storage/model/DTO/catalog lint passes.
- `/tmp/kandev-nono-execution-evidence-green.log`: bound-evidence and private-source tests pass. The real packaged Nono process reports provider available with confinement unknown, then provider unavailable after removal, and stopped after verified cleanup.

These checks do not prove active Nono confinement evidence or actual agent product acceptance. Observation persistence, shared session details, learning, and full actual-agent acceptance remain open.


### Observation persistence and browser freshness

`/tmp/kandev-nono-observation-projection-final.log` passes SQLite/PostgreSQL snapshot, observation, DTO, and required-table checks with race detection. Backend owner replacement invalidates live status and blocks earlier writers. Terminal observations retain historical evidence. Subsequent observer-queue schema changes are under verification.

`/tmp/kandev-nono-observation-web-final.log` passes 543 tests across the session store, transport handlers, and status derivation. Red tests exposed dropped observation fields, retained active status on disconnect, and a late update during reconnect. The fixes preserve attempt ordering and require a newer projection before active status returns. These are browser data-flow checks; rendered details and actual-agent acceptance remain pending.

`/tmp/kandev-nono-snapshot-go-lint-orchestrator.log` reports no findings but exits with a timeout. This is not a passing lint result.


### Core observer and session status model

- `/tmp/kandev-nono-observation-candidates-green2.log`: SQLite/PostgreSQL candidate pagination, tenant identity, terminal exclusion, latest-attempt selection, and cascade deletion pass.
- `/tmp/kandev-nono-observer-final.log`: observer startup ownership, provider failure/nil/timeout, replacement attempt, wake/timer, and joined shutdown checks pass with race detection. The timer test's counter uses atomic synchronization; the earlier race was in that test counter.
- `/tmp/kandev-nono-observation-final-backend.log`: final snapshot/observation repository, DTO, catalog, packaged runtime, backend wiring, orchestrator, and gateway checks pass. The common-package filter matched no tests; its actual source tests pass separately in `/tmp/kandev-nono-observation-runtime-source-final.log`.
- `/tmp/kandev-nono-observation-runtime-final.log`: packaged Nono and evidence checks pass after the final constant extraction.
- `/tmp/kandev-nono-observer-policy-final.log`: the complete policy package passes with SQLite/PostgreSQL enabled after explicit SQL rebinding. `/tmp/kandev-nono-observation-sqlguard-green.log` passes.
- `/tmp/kandev-nono-observation-go-lint-storage2.log` and `/tmp/kandev-nono-observation-go-lint-runtime2.log`: pass with zero issues. Event-package lint and schema conformance remain pending.
- `/tmp/kandev-nono-session-execution-hook-green.log`: 33 hook, shared-clock, and status tests pass. The latest hook typecheck is pending. `/tmp/kandev-nono-status-typecheck3.log` passed before the hooks were added. Scoped status/transport and hook lint pass.

These checks establish the observation data path. Rendered session details, approvals/learning surfaces, and full actual-agent acceptance remain open.

## Execution detail controls (2026-10-24)

- `/tmp/kandev-nono-snapshot-binding-red.log`: the new mismatched-attempt regression failed before the storage guard.
- `/tmp/kandev-nono-snapshot-binding-green.log`: SQLite/PostgreSQL snapshot and observation race checks pass after the guard. `/tmp/kandev-nono-snapshot-binding-lint.log`: scoped Go lint passes.
- `/tmp/kandev-nono-observation-upgrade-green.log`: the two previously failing upgrade checks pass on both engines. Only the fixture manifest identifies the new store/tables; historical SQL fixtures remain unchanged.
- `/tmp/kandev-nono-execution-ui-unit.log`: 40 frontend tests pass. The controls preserve focus and session identity, remove expired active claims, and distinguish captured permissions from current policy reads.
- `/tmp/kandev-nono-execution-ui-typecheck2.log`, `/tmp/kandev-nono-execution-ui-lint2.log`, and `/tmp/kandev-nono-execution-ui-i18n2.log`: pass.
- Desktop and phone rendered checks are pending. The current view has no approval decision or denial-learning controls. Full managed-agent acceptance remains open.


## Saved defaults and responsive details (2026-10-24)

- `/tmp/kandev-nono-saved-command-preview-red2.log`: ACP/native saved-preview requests failed with 404 before implementation.
- `/tmp/kandev-nono-saved-command-preview-final2.log` and `/tmp/kandev-nono-saved-command-preview-lint2.log`: handler/controller race checks and scoped lint pass. Responses omit raw argv, private executable paths, and store error details.
- `/tmp/kandev-nono-command-preview-hook-red.log`: expected missing preview/refresh behavior failed before implementation. `/tmp/kandev-nono-command-preview-ui-unit.log`: 40 tests pass, including settings refresh, sign-out, late profile responses, captured/current separation, and mobile layout feedback.
- `/tmp/kandev-nono-command-preview-ui-typecheck.log`, `/tmp/kandev-nono-command-preview-ui-lint.log`, `/tmp/kandev-nono-command-preview-ui-i18n.log`: pass.
- `/tmp/kandev-nono-execution-details-desktop4.log` and `/tmp/kandev-nono-execution-details-phone5.log`: desktop and phone rendered flows pass. Phone targets, full-width details at 393–767px, focus return, and sibling isolation pass.
- `/tmp/kandev-nono-execution-details-phone-visual.log`: phone capture repeated because the next browser run removed the earlier test-result directory. Both captures were retained outside that directory and inspected.

These browser fixtures verify interface behavior. They do not establish actual managed-agent Nono acceptance.


## Approval review and live updates (2026-10-25)

- `/tmp/kandev-nono-approval-changes-green.log`: committed approval state-change tests pass on SQLite and PostgreSQL.
- `/tmp/kandev-nono-approval-events-green.log`: attempt/session binding and workspace-scoped publication race tests pass. `/tmp/kandev-nono-approval-events-lint3.log`: zero lint issues.
- `/tmp/kandev-nono-approval-desktop-red2.log`: closing the decided request failed to restore keyboard focus before the fallback was added.
- `/tmp/kandev-nono-approval-initial-event-red.log`: a live request was lost behind the pending initial empty read. `/tmp/kandev-nono-approval-initial-event-green.log`: all nine hook tests pass after cancelling stale reads.
- `/tmp/kandev-nono-approval-ui-final-unit2.log`: 24 focused frontend tests pass. `/tmp/kandev-nono-approval-ui-final-typecheck.log`, `/tmp/kandev-nono-approval-ui-final-lint2.log`, `/tmp/kandev-nono-approval-i18n.log`, and `/tmp/kandev-nono-approval-public-docs.log`: pass.
- `/tmp/kandev-nono-approval-final-desktop.log` and `/tmp/kandev-nono-approval-final-phone.log`: final desktop and phone browser scenarios pass. They cover versioned one-operation decisions, read-only access, expiry, connection loss, focus return, long content, and phone geometry. Both retained screenshots were inspected.

The browser uses controlled API and event fixtures. This does not prove full managed-agent Nono acceptance. Main port 9998 and retained manual instances were not changed.


## Denial reports (2026-10-25)

- Actual packaged Nono: final capture and malformed-audit preservation pass (`/tmp/kandev-nono-report-runtime-refactor.log`). Reports remain partial and do not claim complete filesystem or network coverage.
- SQLite/PostgreSQL storage and conformance checks pass (report-store and report-conformance logs). Session-scoped HTTP authority, post-refresh access checks, and failure sanitization pass (`/tmp/kandev-nono-report-http-green.log`).
- Seven frontend hook cases pass (`/tmp/kandev-nono-report-ui-final.log`). Desktop and phone browser scenarios pass (`/tmp/kandev-nono-report-browser-desktop.log`, `/tmp/kandev-nono-report-browser-phone.log`). These browser fixtures are UI evidence, not full managed-agent acceptance.
- Desktop and phone screenshots were inspected: `/tmp/kandev-sandbox-report-desktop.png`, `/tmp/kandev-sandbox-report-phone.png`. Phone widths 393, 700, and 767 preserve touch targets and prevent document overflow.
- Final typecheck, scoped frontend/backend lint, localization, SQL guard, and public-doc validation pass. Main `100.101.37.39:9998` remains PID 46448. Retained manual instances were not changed.
- Learning publication, explicit restart review, child-state deletion, and actual managed-agent acceptance remain open.
