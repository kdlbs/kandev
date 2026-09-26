"use client";

import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useSystemInfoQueryIdentity } from "@/hooks/domains/system/system-info-query";

const SystemInfoBootIdContext = createContext<string | undefined>(undefined);
const SystemInfoRequestSignalContext = createContext<AbortSignal | null>(null);

export function SystemInfoQueryProvider({
  bootId,
  children,
}: {
  bootId: string | undefined;
  children: ReactNode;
}) {
  const identity = useSystemInfoQueryIdentity(bootId);
  const identityKey = JSON.stringify([
    identity.apiBaseUrl,
    identity.bootId ?? null,
    identity.authMode,
    identity.authenticated,
    identity.userId,
  ]);

  return (
    <SystemInfoBootIdContext.Provider value={bootId}>
      <ScopedQueryClient key={identityKey}>{children}</ScopedQueryClient>
    </SystemInfoBootIdContext.Provider>
  );
}

export function useSystemInfoBootId(): string | undefined {
  return useContext(SystemInfoBootIdContext);
}

export function useSystemInfoRequestSignal(): AbortSignal {
  const signal = useContext(SystemInfoRequestSignalContext);
  if (!signal) throw new Error("SystemInfoQueryProvider is required");
  return signal;
}

function ScopedQueryClient({ children }: { children: ReactNode }) {
  const [scope] = useState(() => ({
    queryClient: new QueryClient(),
    requestController: new AbortController(),
  }));
  const mountedRef = useRef(false);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      queueMicrotask(() => {
        if (mountedRef.current) return;
        scope.requestController.abort();
        scope.queryClient.clear();
      });
    };
  }, [scope]);

  return (
    <SystemInfoRequestSignalContext.Provider value={scope.requestController.signal}>
      <QueryClientProvider client={scope.queryClient}>{children}</QueryClientProvider>
    </SystemInfoRequestSignalContext.Provider>
  );
}
