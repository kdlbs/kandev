/**
 * Session-runtime streams → ring-buffer bridge (Wave 5a).
 *
 * High-frequency output streams (shell, process, terminal) MUST NOT flow
 * through the TanStack Query cache — TQ's per-chunk structural sharing is
 * a performance cliff at thousands of chunks/sec.
 *
 * Instead, each stream writes directly into the module-level ring-buffer
 * registry via `appendToRing` / `clearRing`. Components subscribe via
 * `useShellRingBuffer(key)` from `lib/query/streams/ring.ts`.
 *
 * Key conventions:
 *   shell output:    `shell:${sessionId}` or `shell:passthrough:${sessionId}`
 *   process output:  `process:${processId}`
 *   terminal output: `terminal:${terminalId}`
 *
 * ProcessStatus (not high-frequency) continues to live in Zustand.
 * This bridge only handles the raw output stream events.
 *
 * Sub-registrars:
 *   registerShellStreamHandlers   — session.shell.output
 *   registerProcessStreamHandlers — session.process.output
 *   registerTerminalStreamHandlers — terminal.output
 */

import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";
import { appendToRing, clearRing, destroyRing } from "@/lib/query/streams/ring";

// ---------------------------------------------------------------------------
// Key helpers (exported for tests)
// ---------------------------------------------------------------------------

/** Ring-buffer key for the shell output stream of a session or env. */
export function shellRingKey(sessionId: string): string {
  return `shell:${sessionId}`;
}

/** Ring-buffer key for the passthrough agent output of a session. */
export function passthroughRingKey(sessionId: string): string {
  return `shell:passthrough:${sessionId}`;
}

/** Ring-buffer key for the output of a specific process. */
export function processRingKey(processId: string): string {
  return `process:${processId}`;
}

/** Ring-buffer key for the output of a specific terminal. */
export function terminalRingKey(terminalId: string): string {
  return `terminal:${terminalId}`;
}

// ---------------------------------------------------------------------------
// Shell stream handlers
// ---------------------------------------------------------------------------

function registerShellStreamHandlers(ws: WebSocketClient): () => void {
  const unsubShell = ws.on("session.shell.output", (message) => {
    const { session_id, type, data } = message.payload;
    if (!session_id) return;
    if (type === "output" && data) {
      appendToRing(shellRingKey(session_id as string), data as string);
    } else if (type === "exit") {
      // Shell exited — clear the ring so the next session starts fresh.
      clearRing(shellRingKey(session_id as string));
    }
  });
  return unsubShell;
}

// ---------------------------------------------------------------------------
// Process stream handlers
// ---------------------------------------------------------------------------

function registerProcessStreamHandlers(ws: WebSocketClient): () => void {
  const unsubProcess = ws.on("session.process.output", (message) => {
    const { process_id, session_id, kind, data } = message.payload;
    if (!process_id || !data) return;
    appendToRing(processRingKey(process_id as string), data as string);
    // Passthrough mode: also route output under the session ring so the
    // PassthroughTerminal component can subscribe without knowing the processId.
    if (kind === "agent_passthrough" && session_id) {
      appendToRing(passthroughRingKey(session_id as string), data as string);
    }
  });
  return unsubProcess;
}

// ---------------------------------------------------------------------------
// Terminal stream handlers
// ---------------------------------------------------------------------------

function registerTerminalStreamHandlers(ws: WebSocketClient): () => void {
  const unsubTerminal = ws.on("terminal.output", (message) => {
    const { terminalId, data } = message.payload;
    if (!terminalId || !data) return;
    appendToRing(terminalRingKey(terminalId as string), data as string);
  });
  return unsubTerminal;
}

// ---------------------------------------------------------------------------
// Permanent teardown helpers
// ---------------------------------------------------------------------------

// TODO(destroy-ring): When the backend emits a permanent session-removed,
// process-removed, or terminal-closed WS event, call destroyRing here:
//
//   destroyRing(shellRingKey(sessionId));
//   destroyRing(passthroughRingKey(sessionId));
//   destroyRing(processRingKey(processId));
//   destroyRing(terminalRingKey(terminalId));
//
// No such event exists in the current WS protocol — add the call site once
// the backend publishes one. The `destroyRing` export is ready in
// lib/query/streams/ring.ts.
export { destroyRing };

// ---------------------------------------------------------------------------
// Top-level registrar
// ---------------------------------------------------------------------------

/**
 * Registers ring-buffer bridges for all high-frequency stream events.
 *
 * The `qc` parameter is accepted but unused — it's here so the registrar
 * signature is consistent with other bridge modules (future use possible).
 *
 * Returns a cleanup function that removes all WS handlers.
 */
export function registerSessionRuntimeStreamsBridge(
  ws: WebSocketClient,
  _qc: QueryClient,
): () => void {
  const unsubs = [
    registerShellStreamHandlers(ws),
    registerProcessStreamHandlers(ws),
    registerTerminalStreamHandlers(ws),
  ];
  return () => {
    for (const fn of unsubs) fn();
  };
}
