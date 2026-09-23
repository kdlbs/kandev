# ADR-2026-09-23-task-owned-lsp-leases: Retain LSP processes across browser disconnects

**Status:** accepted
**Date:** 2026-09-23
**Area:** protocol

## Context

The browser WebSocket currently owns a task-host language-server process. Closing the tab or losing the network closes the stream and interrupts the process, even when the task workspace remains active. A later editor must start a new server and repeat project analysis. Concurrent browser windows currently have independent protocol sessions and may hold different unsaved document buffers.

## Decision

The task runtime will own bounded LSP leases. One lease owns one language-server process and one logical LSP protocol session. A browser connection attaches to a lease; closing that connection detaches it while Kandev continues to drain and service the language server. A later editor for the same task and language may reclaim a detached lease. Concurrently attached browser windows keep independent leases. An active lease prevents idle-session reclaim of its execution. An intentional two-minute browser editor-idle timeout releases its lease, while tab close or network loss retains it. At the LSP limit, admission of a new server evicts the least recently detached lease; attached leases are never evicted. Explicit Stop, server exit, task-host shutdown, or backend shutdown also ends a lease. The existing LSP limit counts retained leases as well as attached ones.

The runtime broker retains only bounded protocol state needed to reattach: server capabilities and dynamic registrations, workspace identity, current configuration, active progress, and open-document revision counters. It discards diagnostics on detach and accepts fresh publications only after the new document is synchronized. It must not treat a transport disconnect as a server crash. A release toggle merges disabled in all shipped profiles; the disabled path preserves the current browser-owned lifecycle.

## Consequences

Project analysis can continue while no browser is open, and a returning editor can regain its state. Keeping the lease active also retains the whole Local PC or Docker task host, including agentctl, container and worktree runtime resources, after an agent turn completes. Intentional browser editor-idle release and detached-lease eviction bound this cost; a user returning after eviction gets a fresh server and visible initialization. Servers that publish diagnostics without a document version cannot safely restore those markers on a resumed lease, so they stay hidden until a fresh server generation. The broker must handle server requests and drain output without a browser, distinguish explicit Stop from transport loss, and reject stale responses from previous browser attachments. Backend restart loses runtime-only leases and requires a fresh server start.

## Alternatives Considered

- Keep browser-owned processes and restart automatically on reopen: easy to implement, but discards in-flight analysis and does not satisfy continuous work while the browser is closed.
- Retain a process for a short grace period: bounds resource use, but a user returning after the grace period still loses project analysis.
- Expire detached leases after a long TTL: bounds idle resources but discards analysis at a fixed time even when capacity is available. LRU eviction preserves a detached lease until another server needs its slot.
- Share one protocol session across all browser windows: reduces process count, but conflicting unsaved buffers and request IDs would change the current independent-window behavior.
