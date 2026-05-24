import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";

/**
 * WS → TanStack Query bridge for the linear domain.
 *
 * Linear issue watches are managed exclusively via REST — the backend does
 * not emit WebSocket events for watch creation, updates, or polling results.
 * Availability / auth health is polled by the integrations bridge
 * (`registerIntegrationsBridge`) via `refetchInterval: 90_000`.
 *
 * Therefore this bridge is intentionally a no-op registrar. It exists to:
 * 1. Satisfy the bridge/index.ts module contract so the coordinator can wire
 *    it into registerQueryBridge without special-casing the linear domain.
 * 2. Serve as an extension point if the backend ever adds
 *    `linear.watch.triggered` WS pushes — add a handler here and call
 *    qc.invalidateQueries({ queryKey: qk.linear.watches(wsId) }).
 */
export function registerLinearBridge(
  _ws: WebSocketClient,
  _qc: QueryClient,
): () => void {
  // No WS events for linear watches or availability.
  // Mutations invalidate the watches key directly after each operation.
  return () => {
    // No-op cleanup — nothing to unsubscribe.
  };
}
