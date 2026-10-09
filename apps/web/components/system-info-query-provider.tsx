"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  BACKUP_LIST_QUERY_KEY_PREFIX,
  DISK_USAGE_QUERY_KEY_PREFIX,
  createSystemInfoQueryKey,
  DATABASE_STATS_QUERY_KEY_PREFIX,
  SYSTEM_INFO_QUERY_KEY_PREFIX,
  useSystemInfoQueryIdentity,
} from "@/hooks/domains/system/system-info-query";
import { SystemDiskUsageQueryBridge } from "@/components/system-disk-usage-query-bridge";

const SystemInfoBootIdContext = createContext<string | undefined>(undefined);

function hasQueryKeyPrefix(queryKey: readonly unknown[], prefix: readonly string[]): boolean {
  return prefix.every((segment, index) => queryKey[index] === segment);
}

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
      <ScopedQueryClient bootId={bootId} identityKey={identityKey}>
        {children}
      </ScopedQueryClient>
    </SystemInfoBootIdContext.Provider>
  );
}

export function useSystemInfoBootId(): string | undefined {
  return useContext(SystemInfoBootIdContext);
}

function ScopedQueryClient({
  children,
  bootId,
  identityKey,
}: {
  children: ReactNode;
  bootId: string | undefined;
  identityKey: string;
}) {
  const [queryClient] = useState(() => new QueryClient());

  useEffect(() => {
    const obsoleteQueries = {
      predicate: (query: { queryKey: readonly unknown[] }) => {
        const { queryKey } = query;
        const isSystemInfo = hasQueryKeyPrefix(queryKey, SYSTEM_INFO_QUERY_KEY_PREFIX);
        const isDiskUsage = hasQueryKeyPrefix(queryKey, DISK_USAGE_QUERY_KEY_PREFIX);
        const isDatabaseStats = hasQueryKeyPrefix(queryKey, DATABASE_STATS_QUERY_KEY_PREFIX);
        const isBackupList = hasQueryKeyPrefix(queryKey, BACKUP_LIST_QUERY_KEY_PREFIX);

        return (
          (isSystemInfo || isDiskUsage || isDatabaseStats || isBackupList) &&
          JSON.stringify(queryKey.slice(2)) !== identityKey
        );
      },
    };
    void queryClient.cancelQueries(obsoleteQueries);
    queryClient.removeQueries(obsoleteQueries);
  }, [identityKey, queryClient]);

  return (
    <QueryClientProvider client={queryClient}>
      <SystemDiskUsageQueryBridge bootId={bootId} />
      {children}
    </QueryClientProvider>
  );
}
