---
created: 2026-11-14
status: done
requirements:
  - REQ-AGENTS-MCP-APPLY-001
  - REQ-AGENTS-MCP-APPLY-002
  - REQ-AGENTS-MCP-APPLY-003
system_design:
  - ../../specs/agents/system-design/task-mcp-application.md
legacy_specs: []
---
# Implementation plan: Apply saved task MCP packages

## Scope and authority

Human task-package saves can reach supported retained sessions through a provider-only restart.
Keep task, session, execution, helper, native conversation, workspace, journal owner, and queued input.
Independent terminals, previews, servers, and editor connections stay available during success and failure.
Provider work, unresolved input, and uncertain delivery defer application. No cancellation or fresh conversation is allowed.
Workspace policy saves, automation edits, third-party MCP changes, and unsupported providers retain their existing next-start behavior.

The user authorized implementation on 2026-11-15 and autonomous completion on 2026-11-16.
The user authorized commit, push, PR creation, and CI/review remediation on 2026-11-17.
No delegation, deployment, live task restart, or homelab work is authorized.
The earlier helper-replacement and empty-helper proposals are superseded by the
[lifetime ADR](../../decisions/2026-11-15-provider-refresh-workspace-lifetime.md).

## Implementation

Atomic task preference saves also persist per-session application intent. Equivalent effective saves cannot create a restart or reset a failure.
A coalesced wake and service-lifetime sweep recover unfinished SQL requests. Session and prompt admission serialize replacement.
After a durable closed-effect barrier, queue writes can proceed while dispatch remains blocked.

The optional `runtime.ProviderMCPReplacement` seam controls prepare, release, retire, activate, and accept inside the retained helper.
The provider gets a new process generation, immutable binding, and private scope credential.
Retired routes and backend scope checks reject stale calls before effects. Existing workspace services keep their original ownership.
The conversation journal stream, incarnation, cursor, SQL harness generation, and native thread do not change for MCP refresh.
Scope journal generation records provenance at acceptance. A later authorized context reset owns its separate delivery transition.

Durable helper receipts and SQL acceptance reconcile lost responses without duplicate starts or turns.
A newer save cannot rewrite an in-flight target. The latest valid selection wins before acceptance, or follows an already accepted target.
Reverting to an unchanged accepted selection avoids restart. Failures require explicit versioned Retry.
Retry resolves saved choices under current workspace policy. It cannot supply package overrides.
Uncertain acceptance parks after its deadline, preserving the operation and queued input for explicit recovery.

Codex app-server 0.166.0 on Linux passed permanent native conformance.
Other providers, versions, transports, and incompatible helpers remain pending with a reason.
An existing resume declaration alone does not enable application.

## UI-01: Saved request and accepted tools

Use the existing task MCP explorer. Desktop keeps its dialog and split Connections view.
Phone and coarse-pointer layouts keep the full-height drawer and one-level navigation.
The shared status region wraps below the tabs and above the content scroller.

The editor retains its unsaved draft, Save/Reset, and save-error feedback.
The session notice separately shows saved/pending, applying, applied, failed, or unsupported state.
Retry is separate from Save, uses current request/version evidence, and never submits a chat turn.
Each selected session has its own durable result, including after reload.
No status update moves focus or opens another overlay.

Connections uses the verified current provider catalog after application.
Retired package tools disappear. Closed provider effects cannot display historical tools as active.
A selected package alone does not prove application or a healthy connection.
Current accepted evidence remains separate from saved choices and historical attachment records.

Phone controls keep at least 44px touch targets. Text wraps without document horizontal overflow.
Status uses a polite live region. Motion respects reduced-motion preferences.
English, six translated locales, and the pseudo catalog contain the state and recovery copy.

## Work orders

- [x] [01: Persist human application intent](task-01-save-intent.md)
- [x] [05: Separate provider refresh from workspace lifetime](task-05-replacement-admission.md)
- [x] [06: Bind MCP authority to provider generations](task-06-generation-binding.md)
- [x] [02: Apply changes at safe boundaries](task-02-safe-replacement.md)
- [x] [03: Prove provider resume and catalog acceptance](task-03-provider-acceptance.md)
- [x] [04: Expose application status and recovery](task-04-status-ui.md)

All six work orders are implemented and validated. Baseline validation limits are recorded below.

## Acceptance and test mapping

| Contract | Permanent tests | Work orders |
| --- | --- | --- |
| Atomic saved intent and equivalent edits | settings/store `builtin_application_test.go`; SQLite claim/progress/retry tests | 01, 02 |
| Safe boundary, latest save, no duplicate turns/starts | orchestrator `builtin_mcp_application_test.go`, `builtin_mcp_application_concurrency_test.go`, queue/admission tests | 02 |
| Independent workspace lifetime and failure survival | process `provider_replacement_test.go`, environment/configuration/piped/activation/event tests | 05 |
| Stale authority, durable receipts, closed recovery | MCP `builtin_generation_test.go`; journal/provider API/lifecycle/SQL binding tests | 06 |
| Native conversation and live catalog acceptance | API `provider_native_peer_test.go`, `provider_conformance_test.go`; Codex adapter tests | 03 |
| Status proof, polling, Retry, current catalog | web `builtin-application-status`, `builtin-provider-catalog`, `builtin-application-feedback`, session-hook tests | 04 |
| Desktop/phone interaction and geometry | `task-mcp-application.spec.ts`, `mobile-task-mcp-application.spec.ts` | 04 |

Backend test paths are under `apps/backend/internal/`. Web unit tests are beside the explorer and session hook.
Browser tests use a real isolated backend for the saved request, then inject receipt views for UI transitions.
They validate presentation and API request shape. They do not prove a provider restart.
The actual managed-binary conformance test supplies native restart and workspace continuity evidence separately.
Remote executor launch/outage behavior is not claimed as browser coverage.

## Validation results

Final behavior checks on 2026-11-16:

- Fourteen focused backend package groups pass with race detection. The command below covers saved intent, canonical selection, source admission, binding/receipt integrity, coordinator, queue, context compatibility, dispatch, retry, and lifecycle regressions.
- The actual managed Codex conformance fixture passes three consecutive runs with race detection (80.685s). The final API provider family, with native conformance enabled, also passes (82.226s). It waits for observed provider background settlement before preparing replacement. A completed model response alone is not the safe-boundary proof.
- Native conformance retains one total model call, the same conversation/journal owner, terminal command output, preview URL/content, and editor subprocess connection. No live user or external model endpoint is used.
- Changed-code Go lint passes with zero issues, including all untracked Go files through a generated patch. Go formatting and whitespace checks pass.
- Desktop (11.0s) and phone (13.8s) Playwright runs pass on the final UI. Phone coverage checks 320px, 393px, 767px, 768px, and 900px coarse-pointer layouts, 44px Retry height, no horizontal overflow, durable status after layout remount, one Retry request, and catalog convergence.
- All 86 explorer/session unit tests pass across 11 files. An additional 24 tests for existing walkthrough and settings consumers pass. New cases cover complete/current catalog proof, unavailable counts, saved versus applied state, retry success/failure/busy/late-owner behavior, polling completion/close, and recovery after a transient read failure.
- Web typecheck, full web lint, translation completeness, pseudo key parity, and the new-code copy ratchet pass. All new copy has English and six translations. Unrelated generated translation changes were restored.
- Public-doc validation passes for 55 pages. Catalog validation passes for 451 decisions and 1997 specifications. Specification lint and its 36 tests pass. The documentation coverage validator accepts all six work orders.
- The full frontend suite completed in 1269.96s: 3381 files passed and one unchanged Git fixture file failed; 30,858 tests passed, three failed, and four skipped. All browser and locale-loading test files pass. The three Node failures require a Docker bridge, but this host has no Docker executable. The test and fixture match HEAD byte for byte; the isolated file reproduces the same baseline failures. No feature test failed.

### Reproduction commands

Run backend commands from `apps/backend`. The native binary variable points to an isolated installed managed 0.166.0 binary, not a live session.

```bash
GOMEMLIMIT=12GiB GOMAXPROCS=2 go test -trimpath -race -p 1 \
  ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/task/repository/sqlite \
  ./internal/orchestrator ./internal/backendapp ./internal/orchestrator/messagequeue \
  ./internal/agent/settings/handlers ./internal/agentctl/server/process ./internal/agentctl/server/api \
  ./internal/agent/runtime/lifecycle ./internal/mcp/server ./internal/agentctl/journal \
  ./internal/agent/runtime/agentctl ./internal/agentctl/server/adapter/transport/codexappserver \
  -run 'TestProvider|TestMCPProvider|TestBuiltin|TestYieldedSession|TestSessionAdmission' -count=1
KANDEV_CODEX_MCP_CONFORMANCE_BIN=/path/to/managed-0.166.0-codex go test -trimpath -race \
  ./internal/agentctl/server/api -run TestProviderReplacementManagedCodexConformance -timeout=120s -count=3
```

The patch-based Go lint command includes the listed package trees and uses `--new-from-patch` with tracked and untracked Go diffs, `--concurrency=2`, and a 15-minute limit.
Use the repository-pinned golangci-lint version. The stock installed older linter cannot analyze this Go toolchain.

Run web commands from `apps/web`:

```bash
pnpm exec vitest run components/task/chat/mcp-explorer hooks/domains/session/use-builtin-mcp-session.test.ts
pnpm run typecheck
pnpm run lint
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm exec vitest run --reporter=verbose
pnpm e2e:run --host --shards 1 --project chromium tests/settings/task-mcp-application.spec.ts
pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/settings/mobile-task-mcp-application.spec.ts
```

The recorded browser runs clear inherited `KANDEV_*` and `AGENTCTL_*` host wiring and provider credentials for the disposable test process. They use a loopback bind, `KANDEV_MOCK_AGENT=only`, and the installed Nix Playwright library path. Existing host config selected a different bind address, and the downloaded browser needed those system libraries. Build backend, web, delivery/send-now fixtures, and the E2E plugin package before a no-build run. No freshness guard was bypassed.

The browser tests use a real isolated backend for saves and injected receipt views for later UI states. They do not assert a real provider restart. Native helper conformance supplies that independent proof. Session layout changes can remount the explorer. Reopening retrieves the same saved request and Retry state.

PR screenshots use isolated desktop and phone captures under ignored `apps/web/.pr-assets/`. They are UI review evidence, not public documentation assets.

### Baseline limits

The earlier full API/process audit found unrelated Git capture/numstat and LSP bridge failures.
They reproduce under an overlay restoring original process manager source.
The full instance suite passes. This package does not claim those full API/process suites are green.
Exact failing backend test families and baseline evidence are recorded in work order 05 below.

The full frontend suite has three unrelated failures in `lib/http-git-server.test.ts`: trusted GitLab URL rewrites, listener cleanup after config failure, and setup/close error aggregation. The unchanged helper always discovers a Docker bridge unless its caller supplies an override; these tests supply none. The Docker executable is absent on this host, so setup fails before those assertions. Both files match HEAD byte for byte. The standalone file reproduces the same three failures. This package does not change the fixture or claim a fully green frontend suite.

## Delivery state

The automatic save caller is wired in code. No deployed runtime or live user session was changed.
Public docs updated: `builtin-mcp-packages.md` (how-to), and the timing link in `agents-and-profiles.md` (reference).
All task-defined feature checks are complete. The independent Docker/Git/LSP baseline limits above remain documented. PR delivery and current-head checks are tracked separately.
