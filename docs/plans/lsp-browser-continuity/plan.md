---
created: 2026-09-23
status: draft
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

- Retain one task-host process per LSP lease until explicit Stop, actual process exit, task-runtime shutdown, or backend shutdown.
- Reattach to a detached lease for the same execution and language; preserve independent simultaneously open windows.
- Resume providers, progress, and current-document diagnostics without treating transport loss as a process crash.
- Count detached leases against the existing LSP limit and preserve supported-executor and access checks.
- Update user-facing status, six locale catalogs, and public LSP/configuration documentation.

### Out of scope

- Persisting a language-server process through task-host or backend restart.
- Sharing one live protocol session between concurrently open browser windows.
- Phone LSP, unsupported executors, new language features, or a global LSP dashboard.

## Technical approach

### Runtime lease and protocol continuity

Refactor `apps/backend/internal/gateway/websocket/lsp_handler.go` around a lease manager keyed by execution identity, language, and opaque lease ID. Keep the agentctl upstream WebSocket open after browser detachment, continuously drain it, and track the initialized capabilities, workspace metadata, open documents, active work progress, and bounded latest diagnostics. A returning authorized browser claims its detached lease or an eligible detached lease for the same execution/language. Each concurrently attached browser window keeps its own lease. `lsp_capacity.go` admits a new process lease only when a slot is free; it does not charge reattachment twice. The existing `limits.lspMaxConnections` / `KANDEV_LSP_MAX_CONNECTIONS` identity remains compatible but counts server leases.

The broker maps JSON-RPC request IDs to the active attachment generation, answers server requests while detached, and drops replies to a prior generation. It handles an acknowledged explicit-Stop control separately from socket close. It distinguishes actual task-host process exit (`4006`) from transport/broker failure (`4009` or abnormal browser close), and never forwards reserved WebSocket close codes `1005` or `1006` as close frames. Agentctl remains the process owner; its existing teardown still reaps descendants. A lost execution or backend shutdown closes the upstream and releases the lease exactly once.

The orchestrator's idle-session reclaim path (`apps/backend/internal/orchestrator/reconcile_liveness.go`) must treat an active lease as live work. Wire a narrow lease-liveness interface through backend initialization, without importing the WebSocket gateway into the orchestrator. The lease admission path and reclaim cleanup must share a session lifecycle fence or use equivalent generation checks, so a new lease cannot race with cleanup of the execution it needs.

### Editor reconnection

Update `apps/web/lib/lsp/lsp-client-manager.ts` and its protocol/state helpers to remember an opaque lease ID for tab restoration, reattach automatically after transient transport loss, and accept a resumed ready handshake with retained server capabilities. A resumed browser does not send a second LSP `initialize`; it rebuilds Monaco providers, sends current document text, waits for synchronization, and accepts only diagnostics matching that text. It restores current reported work progress from the lease snapshot. Explicit Stop sends the control and waits for acknowledgment. Backend or task-host restart clears stale lease state and follows existing auto-start/manual-enable policy.

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

- AC .1, .4, .5: Go lease-manager tests prove detach preserves upstream/process, explicit Stop and task stop release it, independent attached windows, exact capacity accounting, and cancellation of a detached install on teardown.
- AC .2, .3, .6: Go protocol tests prove initialized-capability reuse, request-generation isolation, bounded diagnostics/progress replay, content mismatch invalidation, transport versus process-exit close codes, and reserved-code normalization.
- AC .2, .3, .6, .7: web manager tests prove resumed provider setup without second initialize, document synchronization, reconnect/backoff, localized states, stale diagnostic suppression, and phone boundary.

## E2E tests

- AC .1, .2, .3: Extend `apps/web/e2e/tests/lsp/lsp-file-intelligence.spec.ts` and the fake LSP server. Close the desktop page, reopen the task in a fresh page, confirm the fake server's process/initialize count did not increase, and verify current-file diagnostics and status return.
- AC .4, .5, .6: Cover explicit Stop, a second live browser window, capacity with a detached lease, and a task-host restart followed by fresh initialization in focused integration/E2E cases.
- AC .7: Extend `apps/web/e2e/tests/lsp/mobile-lsp-file-intelligence.spec.ts` for tablet reattachment through its drawer; preserve its existing phone no-socket assertion.

## Work orders

- [ ] [Task 01: Runtime LSP leases](task-01-runtime-lsp-leases.md)
- [ ] [Task 02: Editor reconnection and status](task-02-editor-reconnection.md) — depends on Task 01.
- [ ] [Task 03: Browser proof and public docs](task-03-browser-proof-and-docs.md) — depends on Tasks 01 and 02.

Execution is sequential because the broker handshake, browser state, and E2E fixture share one protocol contract.

## Verification results

Pending implementation.

## Risks

- A retained server consumes a capacity slot and task-host memory while the browser is closed. The task stop and explicit Stop paths must remain reachable and leak-free.
- The existing idle-session reaper can reclaim an agent runtime after a turn completes. It must not reclaim an execution with a live LSP lease, and lease admission must not race with that decision.
- Some servers issue client requests or progress while detached. The broker must continue draining and responding without a browser.
- Multiple browser windows may have divergent unsaved text. They must remain on independent leases.
- Reattached diagnostics must be bound to the current document text; a stale cached diagnostic can mislead a user after closing an unsaved editor.

## Open questions

None blocking the design. Retention ends with the active task runtime; backend or task-host restart uses a fresh server and visible initialization.
