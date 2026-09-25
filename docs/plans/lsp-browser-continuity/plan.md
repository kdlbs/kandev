---
created: 2026-09-23
status: implemented
requirements:
  - REQ-PLATFORM-LSP-FILE-INTELLIGENCE-002
system_design:
  - ../../specs/platform/system-design/lsp-file-intelligence-01.md
  - ../../specs/platform/system-design/lsp-file-intelligence-02.md
legacy_specs: []
---

# Implementation Plan: LSP browser continuity

## Overview

Keep a task-host language server working after its browser tab closes, then reconnect a later editor to the retained process and restore valid LSP information. Replace the browser-to-agentctl pass-through with a bounded runtime lease that remains the protocol peer while no browser is attached. Add browser reattachment and status recovery, then prove the user flow with desktop and tablet E2E tests. The design follows [ADR-2026-09-23](../../decisions/2026-09-23-task-owned-lsp-leases.md).

## Scope

### In scope

- Retain one task-host process per LSP lease until intentional editor-idle release, one-hour detached expiry, explicit Stop, capacity eviction, actual process exit, task-runtime shutdown, or backend shutdown.
- Reattach to a detached lease for the same execution and language; preserve independent simultaneously open windows.
- Resume providers and progress, then accept fresh current-document diagnostics without treating transport loss as a process crash.
- Count detached leases against the existing LSP limit and preserve supported-executor and access checks.
- Update user-facing status, six locale catalogs, and public LSP/configuration documentation.

### Out of scope

- Persisting a language-server process through task-host or backend restart.
- Sharing one live protocol session between concurrently open browser windows.
- Phone LSP, unsupported executors, new language features, or a global LSP dashboard.

## Technical approach

### Runtime lease and protocol continuity

Refactor `apps/backend/internal/gateway/websocket/lsp_handler.go` around a lease manager keyed by execution identity, language, and opaque lease ID. Keep the agentctl upstream WebSocket open after browser detachment, continuously drain it, and track initialized and dynamic capabilities, workspace metadata, document version counters, latest configuration, and active work progress. Clear diagnostics on detach and accept only fresh, synchronized publications. A returning authorized browser claims its detached lease or an eligible detached lease for the same execution/language. Each concurrently attached browser window keeps its own lease. `lsp_capacity.go` charges one slot per lease and evicts the oldest detached lease before admitting a new process at capacity; if all are attached it returns `4005`. Reattachment does not charge twice. Intentional two-minute browser editor-idle release frees a lease; tab close and network loss retain it for at most one hour. The existing `limits.lspMaxConnections` / `KANDEV_LSP_MAX_CONNECTIONS` identity remains compatible but counts server leases. Update its config-catalog description without changing the key or default; coordinate with the startup-configuration-parity work order that also mentions the key.

The broker maps JSON-RPC request IDs and `$/cancelRequest` to the active attachment generation, cancels in-flight requests on detach, answers dynamic capability and configuration requests while detached, and drops replies to a prior generation. A settings-save event updates detached leases. It handles acknowledged explicit Stop and editor-idle release separately from socket close; Stop rejects pending browser requests, sends LSP `shutdown` and `exit`, then closes upstream after bounded cleanup. Agentctl sends explicit `4006` only for confirmed process exit. Upstream read failure without that signal maps to `4009` transport/broker failure, never to server-exited; reserved close codes `1005` and `1006` are normalized. Agentctl remains the process owner and reaps descendants. A lost execution or backend shutdown closes the upstream and releases the lease exactly once.

The orchestrator's idle-session reclaim path (`apps/backend/internal/orchestrator/reconcile_liveness.go`) must treat an active lease as live work at all three callers: the periodic idle reaper, the agent event handler, and the streaming event handler. Wire a narrow lease-liveness interface through backend initialization, without importing the WebSocket gateway into the orchestrator. The lease admission path and reclaim cleanup must share a session lifecycle fence or use equivalent generation checks, so a new lease cannot race with cleanup of the execution it needs. Lease release/eviction removes the pin so normal reclaim can resume.

Add restart-required `features.lspBrowserContinuity` / `KANDEV_FEATURES_LSP_BROWSER_CONTINUITY` as a runtime release toggle, off in prod, dev, and e2e profiles. Gate backend lease construction and WebSocket admission as well as frontend resume behavior. The disabled path keeps the current browser-owned process and reclaim behavior. Explicitly enable the flag in focused tests and E2E fixtures; exercise both enabled and disabled paths.

### Editor reconnection

Update `apps/web/lib/lsp/lsp-client-manager.ts` and its protocol/state helpers to remember an opaque lease ID for tab restoration, reattach automatically after transient transport loss, and accept a resumed ready handshake with retained and dynamic capabilities. A resumed browser does not send a second LSP `initialize`; it rebuilds Monaco providers, sends current document text, waits for synchronization, and accepts only new qualifying diagnostics. It restores current reported work progress from the lease snapshot. Explicit Stop and the existing two-minute last-editor idle timer send acknowledged release controls. A hinted tab may resume its existing lease after auto-start is disabled, unless the user explicitly stopped it; an unhinted new tab follows current auto-start/manual-enable policy. Backend or task-host restart clears stale lease state and follows the same policy. Duplicated-tab hints never steal an attached lease.

The toolbar and fine-pointer status bar show a localized reconnecting state; the coarse-pointer tablet uses the existing drawer. A true server exit still shows Retry. Phone viewing neither attaches nor starts LSP.

### Public documentation

Update `docs/public/developer-tools.md` for continuity, Stop, and restart behavior; `docs/public/configuration.md` for the retained-lease meaning of the existing limit; and `docs/public/websocket-api.md` for the close-code distinction. Check `docs/public/feature-status.md` and update its summary if the new behavior needs mention. These pages change with implementation, after the behavior is verified.

## ASCII UI preview

`UI-01: Desktop active Monaco file after transient LSP disconnection` (AC .2, .3). The status control stays in its selected toolbar or status-bar location. The reconnecting state replaces the misleading server-exited message.

```text
Before:  Go  [!] Error: language server exited       [Retry]
After:   Go  [~] Reconnecting to language server...  [Stop]
          Go  [o] Ready | Project work: <server report> [Stop]
```

`UI-02: Coarse-pointer tablet LSP drawer` (AC .2, .7). The existing toolbar trigger opens the same drawer; no new navigation surface is introduced.

```text
File editor                 [Go LSP: Reconnecting]
  +-------------------------------------------+
  | Language server                           |
  | Reconnecting to language server...        |
  | Project progress resumes when connected.  |
  |                                    [Stop]  |
  +-------------------------------------------+
```

`UI-03: Phone file viewer` (AC .7). The phone viewer retains its existing file header and content; it has no LSP trigger or background attachment.

```text
< Back                  main.go
--------------------------------
File content (one scroll region)
```

The structure and lifecycle states are required; the shown text and spacing are illustrative and must use localized copy. Desktop and tablet reuse their existing status placement and surfaces. Tablet keeps the current inset drawer, safe-area handling, touch-sized action, and internal scroll owner. The phone boundary is a no-attachment assertion, not a new screen design.

## Tests

- AC .1, .4, .5, .8, .10: Go lease-manager tests prove detach survives a completed turn and reaper interval, Stop/idle release/task stop release capacity, independent attached windows, one-hour detached expiry and reattachment cancellation, LRU detached eviction, exact accounting, and cancellation of a detached install on teardown.
- AC .2, .3, .6, .8: Go protocol tests prove capability registration replay, detached configuration updates, monotonic document versions, diagnostic invalidation, version matching and resumed unversioned suppression, request cancellation and generation isolation, process-exit versus transport codes, and reserved-code normalization.
- AC .2, .3, .6, .7, .8, .9: web manager tests prove resumed provider setup without second initialize, document synchronization, reconnect/backoff, idle release, close-code translation, policy after auto-start changes, duplicate-hint isolation, localized states, stale-marker suppression, and phone boundary. Flag contract tests cover both modes.

## E2E tests

- AC .1, .2, .3: With the flag enabled, extend `apps/web/e2e/tests/lsp/lsp-file-intelligence.spec.ts` and the fake LSP server. Close the desktop page, complete an agent turn, pass a reaper interval, reopen the task, confirm the same fake server process/initialize count, and verify fresh current-file diagnostics and status return.
- AC .4, .5, .6, .8, .9: Cover explicit Stop, browser editor-idle release, a second live window and duplicated tab, detached-lease eviction at capacity, all-attached `4005`, and task-host restart followed by fresh initialization.
- AC .7: Extend `apps/web/e2e/tests/lsp/mobile-lsp-file-intelligence.spec.ts` for tablet reattachment through its drawer; preserve its existing phone no-socket assertion.

## Work orders

- [x] [Task 01: Runtime LSP leases](task-01-runtime-lsp-leases.md) — done.
- [x] [Task 02: Editor reconnection and status](task-02-editor-reconnection.md) — done.
- [x] [Task 03: Browser proof and public docs](task-03-browser-proof-and-docs.md) — done.

Execution is sequential because the broker handshake, browser state, and E2E fixture share one protocol contract.

## Verification results

Implemented all three work orders. Focused backend tests and `go vet` passed, including the LSP gateway, agentctl API, orchestrator, backend wiring, runtime flags, profiles, and configuration catalog. The focused web LSP/component/feature suite passed (58 tests), web typecheck and changed-file ESLint passed, and `pnpm run i18n:pseudo && pnpm run i18n:check` passed. Managed desktop LSP E2E scenarios passed for close/reopen retention, independent windows and duplicate tabs, detached capacity eviction, all-attached capacity, intentional idle release, disabled-flag behavior, and fresh initialization after task-host restart. Managed mobile E2E passed for tablet drawer reattachment and the phone no-socket boundary. Public-doc validators and spec catalog/lint checks passed. `git diff --check` passed. The repository-wide E2E sleep lint reports unrelated existing violations outside the changed LSP files; targeted lint for the changed E2E files passed.

### Code-review remediation results (2026-09-23)

- Normal `agent.completed` cleanup now defers teardown while the exact execution owns a live LSP lease. Regression coverage drives the actual completion event and a stale-row idle-reaper scan; the runtime row remains running.
- Detach keeps the upstream write fence through old-generation cancellation and `didClose` writes. Controlled concurrency coverage proves cleanup precedes successor `didOpen`, and a stale handler generation cannot detach its successor.
- The broker owns the single `initialize` request and response across browser detaches. Tests cover reconnect before the response and reconnect after the response but before `initialized`; the resumed client completes the missing notification without a second initialize.
- Detached reuse now requires the same user ID. Broker configuration lookup restores full-language and language-prefixed section behavior. Tests cover both-user lease selection and nested/missing configuration sections.
- The browser advertises only supported dynamic provider methods, maps semantic tokens, and rebuilds registrations across resume. Focused backend/frontend registration tests pass.
- The full non-race orchestrator suite passed (265.6s), the complete gateway race suite passed, and focused race tests for the LSP gateway and orchestrator reaper regressions passed. The combined full race run ended with a nil-executor panic in an asynchronous queued-message test goroutine (`Executor.GetExecutionBySession` from `executeQueuedMessageWithReservation`); no LSP regression test failed. All 134 LSP Vitest tests passed; web typecheck, changed-file ESLint, `git diff --check`, and the managed desktop E2E that completes a real follow-up turn and waits 95 seconds through the reaper age threshold and scan passed.
- The one-hour detached expiry follow-up adds timer and admission cleanup, protects reattached leases from stale deadlines, and documents fresh analysis after expiry. The gateway race suite, `go vet`, spec catalog/lint, and public-doc validators pass.

## Risks

- A retained server keeps the entire Local PC or Docker task host alive, including agentctl, container, and worktree runtime resources, after a turn completes. Intentional editor-idle release, one-hour detached expiry, and oldest-detached eviction bound the cost; users returning after expiry or eviction see fresh initialization.
- All three idle-reclaim call sites can end an execution after a turn completes. None may reclaim one with a live LSP lease, and lease admission must not race with those decisions.
- Some servers issue client requests or progress while detached. The broker must continue draining and responding without a browser.
- Multiple browser windows may have divergent unsaved text. They must remain on independent leases.
- Reattached diagnostics need a matching server-facing document version. Unversioned diagnostics stay hidden on resumed leases until a fresh server generation, which reduces diagnostic coverage for servers that omit versions.

## Open questions

None blocking the design. Retention ends with the active task runtime; backend or task-host restart uses a fresh server and visible initialization.
