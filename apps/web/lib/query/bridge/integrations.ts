import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";

/**
 * WS → TanStack Query bridge for the integrations domain.
 *
 * Integration availability (auth health) is driven by a backend HTTP poller
 * that probes credentials roughly every 90 seconds. There are no WebSocket
 * events for integration availability or the on/off enabled toggle:
 *
 * - Auth health is polled by `useIntegrationAuthed` (old) / `useQuery` with
 *   `refetchInterval: 90_000` (new). No WS push — the backend does not emit
 *   `integration.*` events over the gateway.
 *
 * - The enabled toggle is stored in localStorage and synced across tabs via
 *   the browser's `storage` event. It has no server-side representation.
 *
 * Therefore this bridge is intentionally a no-op registrar. It exists to
 * satisfy the module contract so wave 2 workers (jira, linear, slack) can
 * extend it if the backend ever adds WS pushes for integration status, and
 * so the bridge/index.ts pattern is followed consistently across all domains.
 *
 * If future backend work adds `integration.health.updated` WS events, the
 * handler should call:
 *   qc.invalidateQueries({ queryKey: qk.integrations.health(kind) })
 * from inside this function and wire it into registerQueryBridge.
 */
export function registerIntegrationsBridge(_ws: WebSocketClient, _qc: QueryClient): () => void {
  // No WS events for integration availability or enabled state.
  // Availability is managed by HTTP polling (refetchInterval: 90_000).
  // Enabled is managed by localStorage + browser storage events.
  return () => {
    // No-op cleanup — nothing to unsubscribe.
  };
}
