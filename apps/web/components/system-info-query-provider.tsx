"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createSystemInfoQueryKey,
  SYSTEM_INFO_QUERY_KEY_PREFIX,
  useSystemInfoQueryIdentity,
} from "@/hooks/domains/system/system-info-query";

const SystemInfoBootIdContext = createContext<string | undefined>(undefined);

export function SystemInfoQueryProvider({
  bootId,
  children,
}: {
  bootId: string | undefined;
  children: ReactNode;
}) {
  const identity = useSystemInfoQueryIdentity(bootId);
  const identityKey = JSON.stringify(createSystemInfoQueryKey(identity));

  return (
    <SystemInfoBootIdContext.Provider value={bootId}>
      <ScopedQueryClient identityKey={identityKey}>{children}</ScopedQueryClient>
    </SystemInfoBootIdContext.Provider>
  );
}

export function useSystemInfoBootId(): string | undefined {
  return useContext(SystemInfoBootIdContext);
}

function ScopedQueryClient({
  children,
  identityKey,
}: {
  children: ReactNode;
  identityKey: string;
}) {
  const [queryClient] = useState(() => new QueryClient());

  useEffect(() => {
    const obsoleteQueries = {
      queryKey: SYSTEM_INFO_QUERY_KEY_PREFIX,
      predicate: (query: { queryKey: readonly unknown[] }) =>
        JSON.stringify(query.queryKey) !== identityKey,
    };
    void queryClient.cancelQueries(obsoleteQueries);
    queryClient.removeQueries(obsoleteQueries);
  }, [identityKey, queryClient]);

  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}
