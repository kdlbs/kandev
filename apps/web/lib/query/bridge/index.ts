import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";

/**
 * Registers the WS → TanStack Query bridge.
 *
 * This is the single entry point called from QueryProvider on mount.
 * It returns a cleanup function that unregisters all handlers.
 *
 * Current state: no-op stub — waves 1–5 will extend this by importing
 * and calling per-domain registrars, e.g.:
 *
 *   import { registerKanbanBridge } from "./kanban";
 *   import { registerSessionBridge } from "./session";
 *   // etc.
 *
 * Each per-domain module mirrors the shape of the corresponding
 * lib/ws/handlers/<domain>.ts file 1:1, but uses:
 *   queryClient.setQueryData(qk.X(...), updater)
 * instead of:
 *   store.getState().X(...)
 */
export function registerQueryBridge(
  _ws: WebSocketClient,
  _queryClient: QueryClient,
): () => void {
  // No-op until domain bridges are added in waves 1–5.
  return () => {
    // cleanup — unsubscribe per-domain handlers when added
  };
}
