"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createSystemInfoQueryKey,
  DATABASE_STATS_QUERY_KEY_PREFIX,
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
  const identityKey = JSON.stringify(createSystemInfoQueryKey(identity).slice(2));

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
      predicate: (query: { queryKey: readonly unknown[] }) => {
        const { queryKey } = query;
        const isSystemInfo =
          queryKey.length === 7 &&
          queryKey[0] === SYSTEM_INFO_QUERY_KEY_PREFIX[0] &&
          queryKey[1] === SYSTEM_INFO_QUERY_KEY_PREFIX[1];
        const isDatabaseStats =
          queryKey.length === 7 &&
          queryKey[0] === DATABASE_STATS_QUERY_KEY_PREFIX[0] &&
          queryKey[1] === DATABASE_STATS_QUERY_KEY_PREFIX[1];

        return (
          (isSystemInfo || isDatabaseStats) && JSON.stringify(queryKey.slice(2)) !== identityKey
        );
      },
    };
    void queryClient.cancelQueries(obsoleteQueries);
    queryClient.removeQueries(obsoleteQueries);
  }, [identityKey, queryClient]);

  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}
