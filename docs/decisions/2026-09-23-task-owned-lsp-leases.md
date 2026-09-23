# ADR-2026-09-23-task-owned-lsp-leases: Retain LSP processes across browser disconnects

**Status:** accepted
**Date:** 2026-09-23
**Area:** protocol

## Context

The browser WebSocket currently owns a task-host language-server process. Closing the tab or losing the network closes the stream and interrupts the process, even when the task workspace remains active. A later editor must start a new server and repeat project analysis. Concurrent browser windows currently have independent protocol sessions and may hold different unsaved document buffers.

## Decision

The task runtime will own bounded LSP leases. One lease owns one language-server process and one logical LSP protocol session. A browser connection attaches to a lease; closing that connection detaches it while Kandev continues to drain and service the language server. A later editor for the same task and language may reclaim a detached lease. Concurrently attached browser windows keep independent leases. An active lease prevents idle-session reclaim of its execution. Explicit Stop, server exit, task-host shutdown, or backend shutdown ends a lease. The existing LSP limit counts retained leases as well as attached ones.

The runtime broker retains only bounded protocol state needed to reattach: server capabilities, workspace identity, active progress, open-document revisions, and diagnostics. Reattachment must reconcile the browser's current documents before old diagnostics become visible. It must not treat a transport disconnect as a server crash.

## Consequences

Project analysis can continue while no browser is open, and a returning editor can regain its state. Retained processes consume memory and capacity until explicitly stopped or until the task runtime ends. The broker must handle server requests and drain output without a browser, distinguish explicit Stop from transport loss, and reject stale responses from previous browser attachments. Backend restart loses runtime-only leases and requires a fresh server start.

## Alternatives Considered

- Keep browser-owned processes and restart automatically on reopen: easy to implement, but discards in-flight analysis and does not satisfy continuous work while the browser is closed.
- Retain a process for a short grace period: bounds resource use, but a user returning after the grace period still loses project analysis.
- Share one protocol session across all browser windows: reduces process count, but conflicting unsaved buffers and request IDs would change the current independent-window behavior.
