import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";
import { registerFeaturesBridge } from "./features";
import { registerCommentsBridge } from "./comments";
import { registerWorkspaceBridge } from "./workspace";
import { registerSettingsBridge } from "./settings";
import { registerAutomationsBridge } from "./automations";
import { registerIntegrationsBridge } from "./integrations";
import { registerGithubBridge } from "./github";
import { registerGitlabBridge } from "./gitlab";
import { registerJiraBridge } from "./jira";
import { registerLinearBridge } from "./linear";
import { registerKanbanBridge } from "./kanban";
import { registerOfficeBridge } from "./office";
import { registerSessionBridge } from "./session";
import { registerSessionRuntimeBridge } from "./session-runtime";
import { registerSessionRuntimeStreamsBridge } from "./session-runtime-streams";

export interface QueryBridgeOptions {
  /** Returns the currently-active workspace ID, or undefined if none. */
  getActiveWorkspaceId: () => string | undefined;
  /** Resolves sessionId → environmentId for session-runtime cache key routing. */
  getEnvKey: (sessionId: string) => string;
}

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
  options: QueryBridgeOptions,
): () => void {
  const cleanups: Array<() => void> = [
    registerFeaturesBridge(ws, queryClient),
    registerCommentsBridge(ws, queryClient),
    registerWorkspaceBridge(ws, queryClient),
    registerSettingsBridge(ws, queryClient),
    registerAutomationsBridge(ws, queryClient),
    registerIntegrationsBridge(ws, queryClient),
    registerGithubBridge(ws, queryClient),
    registerGitlabBridge(ws, queryClient),
    registerJiraBridge(ws, queryClient),
    registerLinearBridge(ws, queryClient),
    registerKanbanBridge(ws, queryClient),
    registerOfficeBridge(ws, queryClient, options.getActiveWorkspaceId),
    registerSessionBridge(ws, queryClient),
    registerSessionRuntimeBridge(ws, queryClient, options.getEnvKey),
    registerSessionRuntimeStreamsBridge(ws, queryClient),
  ];
  return () => {
    for (const fn of cleanups) fn();
  };
}
