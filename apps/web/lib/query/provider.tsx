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
import { getWebSocketClient } from "@/lib/ws/connection";

interface QueryProviderProps {
  children: React.ReactNode;
  /** Dehydrated state from SSR prefetch, passed by page-level server components. */
  state?: DehydratedState;
}

/**
 * Registers the WS → TQ bridge once after mount.
 * Kept in a separate component so the bridge setup can reference the
 * QueryClient from context without prop-drilling.
 */
function BridgeRegistrar() {
  const queryClient = useQueryClient();

  useEffect(() => {
    const ws = getWebSocketClient();
    if (!ws) return;
    return registerQueryBridge(ws, queryClient);
  }, [queryClient]);

  return null;
}

/**
 * Root QueryClientProvider for the app.
 *
 * Usage in app/layout.tsx (outside <StateProvider>):
 *   <QueryProvider>{children}</QueryProvider>
 *
 * Usage in page-level server components (for SSR prefetch):
 *   <QueryProvider state={dehydrate(serverQueryClient)}>{children}</QueryProvider>
 */
export function QueryProvider({ children, state }: QueryProviderProps) {
  const client = getBrowserQueryClient();

  return (
    <QueryClientProvider client={client}>
      <HydrationBoundary state={state}>
        <BridgeRegistrar />
        {children}
      </HydrationBoundary>
      {process.env.NODE_ENV !== "production" && <ReactQueryDevtools initialIsOpen={false} />}
    </QueryClientProvider>
  );
}
