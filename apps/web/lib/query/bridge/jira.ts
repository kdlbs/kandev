import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";

/**
 * WS → TanStack Query bridge for the Jira domain.
 *
 * Jira is a REST-only integration. The backend does not emit WebSocket events
 * for issue-watch state changes, configuration, or watcher trigger results.
 * All mutations invalidate the relevant query keys via their own onSettled
 * callbacks in useJiraIssueWatches.
 *
 * This bridge is intentionally a no-op registrar. It exists so:
 *   1. The bridge/index.ts pattern is followed consistently across all domains.
 *   2. Future backend work adding `jira.*` WS events can be wired here without
 *      touching the bridge index.
 *
 * If the backend ever adds `jira.watch.updated` / `jira.watch.triggered` WS
 * events, handlers should call:
 *   qc.invalidateQueries({ queryKey: qk.jira.issueWatches() })
 * from inside this function and wire it into registerQueryBridge.
 */
export function registerJiraBridge(_ws: WebSocketClient, _qc: QueryClient): () => void {
  // No WS events for Jira — REST-only integration.
  // Mutations invalidate keys directly via onSettled.
  return () => {
    // No-op cleanup — nothing to unsubscribe.
  };
}
