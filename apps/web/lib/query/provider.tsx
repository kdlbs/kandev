"use client";

import { useEffect } from "react";
import {
  QueryClientProvider,
  HydrationBoundary,
  useQueryClient,
  type DehydratedState,
} from "@tanstack/react-query";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";
import { getBrowserQueryClient } from "@/lib/query/client";
import { registerQueryBridge } from "@/lib/query/bridge/index";
import { subscribeWebSocketClient } from "@/lib/ws/connection";
import { useAppStoreApi } from "@/components/state-provider";

interface QueryProviderProps {
  children: React.ReactNode;
  /** Dehydrated state from SSR prefetch, passed by page-level server components. */
  state?: DehydratedState;
}

/**
 * Root QueryClientProvider for the app.
 *
 * Mounts the QueryClient + HydrationBoundary + devtools. Bridge
 * registration lives in <QueryBridge /> (mounted inside StateProvider)
 * because the office bridge needs to read the active workspace ID
 * from the Zustand store.
 *
 * Usage in app/layout.tsx (outside <StateProvider>):
 *   <QueryProvider state={dehydratedState}>{children}</QueryProvider>
 *
 * Usage in page-level server components (for SSR prefetch):
 *   <QueryProvider state={dehydrate(serverQueryClient)}>{children}</QueryProvider>
 */
export function QueryProvider({ children, state }: QueryProviderProps) {
  const client = getBrowserQueryClient();

  return (
    <QueryClientProvider client={client}>
      <HydrationBoundary state={state}>{children}</HydrationBoundary>
      {process.env.NODE_ENV !== "production" && <ReactQueryDevtools initialIsOpen={false} />}
    </QueryClientProvider>
  );
}

/**
 * Mounts the WS → TanStack Query bridge once after the StateProvider
 * is available. Lives inside StateProvider so it can read the active
 * workspace ID from the Zustand store — the office bridge scopes its
 * cache invalidations to the active workspace.
 */
export function QueryBridge() {
  const queryClient = useQueryClient();
  const storeApi = useAppStoreApi();

  useEffect(() => {
    // <QueryBridge /> mounts higher in the tree than <WebSocketConnector />,
    // so on first render the WS client is still null. Subscribe so we get
    // the client the moment it's assigned and (re-)assigned across
    // reconnects.
    let unregister: (() => void) | null = null;
    const unsubscribe = subscribeWebSocketClient((ws) => {
      unregister?.();
      unregister = null;
      if (!ws) return;
      unregister = registerQueryBridge(ws, queryClient, {
        getActiveWorkspaceId: () => storeApi.getState().workspaces.activeId ?? undefined,
        getEnvKey: (sessionId: string) =>
          storeApi.getState().environmentIdBySessionId[sessionId] ?? sessionId,
      });
    });
    return () => {
      unsubscribe();
      unregister?.();
    };
  }, [queryClient, storeApi]);

  return null;
}
