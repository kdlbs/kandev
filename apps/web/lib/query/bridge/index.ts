import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";
import { registerFeaturesBridge } from "./features";
import { registerCommentsBridge } from "./comments";
import { registerWorkspaceBridge } from "./workspace";
import { registerSettingsBridge } from "./settings";
import { registerAutomationsBridge } from "./automations";
import { registerIntegrationsBridge } from "./integrations";
import { registerKanbanBridge } from "./kanban";

/**
 * Registers the WS → TanStack Query bridge.
 *
 * Single entry point called from QueryProvider on mount. Returns a
 * cleanup function that unregisters every per-domain handler.
 *
 * Each per-domain module mirrors lib/ws/handlers/<domain>.ts 1:1 but
 * writes into the TQ cache (queryClient.setQueryData) instead of the
 * Zustand store. Migration waves add their registrar to the list below.
 */
export function registerQueryBridge(
  ws: WebSocketClient,
  queryClient: QueryClient,
): () => void {
  const cleanups: Array<() => void> = [
    registerFeaturesBridge(ws, queryClient),
    registerCommentsBridge(ws, queryClient),
    registerWorkspaceBridge(ws, queryClient),
    registerSettingsBridge(ws, queryClient),
    registerAutomationsBridge(ws, queryClient),
    registerIntegrationsBridge(ws, queryClient),
    registerKanbanBridge(ws, queryClient),
  ];
  return () => {
    for (const fn of cleanups) fn();
  };
}
