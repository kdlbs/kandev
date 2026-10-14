import { useCallback, useEffect, useLayoutEffect, useMemo, useRef } from "react";
import { type Query, type QueryFunctionContext } from "@tanstack/react-query";
import { useSystemInfoBootId } from "@/components/system-info-query-provider";
import { fetchDiskUsage } from "@/lib/api/domains/system-api";
import type { DiskUsageResponse } from "@/lib/types/system";
import {
  createDiskUsageQueryKey,
  DISK_USAGE_QUERY_KEY_PREFIX,
  useSystemInfoQueryIdentity,
  type SystemInfoQueryIdentity,
} from "./system-info-query";

export { createDiskUsageQueryKey, DISK_USAGE_QUERY_KEY_PREFIX };
export const DISK_USAGE_POLL_INTERVAL_MS = 1500;

export type DiskUsageQueryIdentity = SystemInfoQueryIdentity;
export type DiskUsageQueryKey = ReturnType<typeof createDiskUsageQueryKey>;

export type DiskUsageScope = {
  identity: DiskUsageQueryIdentity;
  queryKey: DiskUsageQueryKey;
  identityKey: string;
  generation: number;
};

type ScopeState = {
  identityKey: string;
  generation: number;
  mounted: boolean;
};

export function createDiskUsageQueryOptions(identity: DiskUsageQueryIdentity) {
  const queryKey = createDiskUsageQueryKey(identity);
  return {
    queryKey,
    queryFn: ({ signal }: QueryFunctionContext<DiskUsageQueryKey>) =>
      fetchDiskUsage({
        baseUrl: identity.apiBaseUrl,
        cache: "no-store",
        init: { signal },
      }),
    staleTime: Infinity,
    gcTime: Infinity,
    refetchOnMount: true,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    retry: false,
    networkMode: "always" as const,
    refetchInterval: (
      query: Query<DiskUsageResponse, Error, DiskUsageResponse, DiskUsageQueryKey>,
    ) => (query.state.data?.computing ? DISK_USAGE_POLL_INTERVAL_MS : false),
    refetchIntervalInBackground: true,
  };
}

export function useDiskUsageScope() {
  const bootId = useSystemInfoBootId();
  const rawIdentity = useSystemInfoQueryIdentity(bootId);
  const identity = useMemo(
    () => rawIdentity,
    [
      rawIdentity.apiBaseUrl,
      rawIdentity.bootId,
      rawIdentity.authMode,
      rawIdentity.authenticated,
      rawIdentity.userId,
    ],
  );
  const queryKey = useMemo(() => createDiskUsageQueryKey(identity), [identity]);
  const identityKey = JSON.stringify(queryKey.slice(2));
  const scopeRef = useRef<ScopeState>({ identityKey, generation: 0, mounted: false });
  const generation =
    scopeRef.current.identityKey === identityKey
      ? scopeRef.current.generation
      : scopeRef.current.generation + 1;

  useLayoutEffect(() => {
    scopeRef.current = { identityKey, generation, mounted: true };
  }, [generation, identityKey]);

  useEffect(() => {
    scopeRef.current.mounted = true;
    return () => {
      scopeRef.current.mounted = false;
    };
  }, []);

  const captureScope = useCallback(
    () => ({ identity, queryKey, identityKey, generation }),
    [generation, identity, identityKey, queryKey],
  );
  const isCurrentScope = useCallback(
    (captured: DiskUsageScope) =>
      scopeRef.current.mounted &&
      scopeRef.current.identityKey === captured.identityKey &&
      scopeRef.current.generation === captured.generation,
    [],
  );

  return { identity, queryKey, identityKey, generation, captureScope, isCurrentScope };
}
