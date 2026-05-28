import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";

/**
 * WS → TanStack Query bridge for the automations domain.
 *
 * Automations do not have backend-push WS events — the current backend
 * only exposes request/response actions (automation.list,
 * automation.create, etc.).  There are no "automation.updated" or
 * "automation.run.completed" push notifications in lib/ws/router.ts.
 *
 * When the backend adds push events in future, add handlers here that
 * call qc.setQueryData(qk.automations.list(wsId), updater) or
 * qc.invalidateQueries({ queryKey: qk.automations.prefix(wsId) }).
 *
 * Returns a cleanup function (no-op until push events exist).
 */
export function registerAutomationsBridge(_ws: WebSocketClient, _qc: QueryClient): () => void {
  // No push handlers needed — mutations in the hooks invalidate the
  // affected query keys directly via qc.invalidateQueries().
  return () => {
    // no-op cleanup
  };
}
