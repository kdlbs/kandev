"use client";

import { createContext, useContext, useState, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useSystemInfoQueryIdentity } from "@/hooks/domains/system/system-info-query";

const SystemInfoBootIdContext = createContext<string | undefined>(undefined);

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

function ScopedQueryClient({ children }: { children: ReactNode }) {
  const [queryClient] = useState(() => new QueryClient());

  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}
