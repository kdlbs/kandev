"use client";

import { useEffect, type ReactNode } from "react";
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";
import { subscribeWebSocketClient } from "@/lib/ws/connection";
import { registerQueryBridge } from "./bridge";
import { getBrowserQueryClient } from "./client";

type QueryProviderProps = {
  children: ReactNode;
  client?: QueryClient;
};

type E2EWindow = Window & {
  __KANDEV_E2E_EXPOSE_STORE__?: boolean;
  __KANDEV_E2E_QUERY_CLIENT__?: QueryClient;
};

export function QueryProvider({ children, client }: QueryProviderProps) {
  const queryClient = client ?? getBrowserQueryClient();
  const showDevtools =
    typeof process !== "undefined" ? process.env.NODE_ENV !== "production" : false;

  useEffect(() => {
    const win = window as E2EWindow;
    if (win.__KANDEV_E2E_EXPOSE_STORE__) {
      win.__KANDEV_E2E_QUERY_CLIENT__ = queryClient;
    }
  }, [queryClient]);

  useEffect(() => {
    let cleanupBridge: (() => void) | null = null;
    const unsubscribeClient = subscribeWebSocketClient((client) => {
      cleanupBridge?.();
      cleanupBridge = client ? registerQueryBridge(client, queryClient) : null;
    });

    return () => {
      unsubscribeClient();
      cleanupBridge?.();
    };
  }, [queryClient]);

  return (
    <QueryClientProvider client={queryClient}>
      {children}
      {showDevtools ? <ReactQueryDevtools initialIsOpen={false} /> : null}
    </QueryClientProvider>
  );
}
