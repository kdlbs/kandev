import { useCallback, useEffect, useLayoutEffect, useMemo, useRef } from "react";
import { type QueryClient, type QueryFunctionContext } from "@tanstack/react-query";
import { useSystemInfoBootId } from "@/components/system-info-query-provider";
import { fetchBackups } from "@/lib/api/domains/system-api";
import type { SnapshotInfo } from "@/lib/types/system";
import {
  BACKUP_LIST_QUERY_KEY_PREFIX as SYSTEM_BACKUP_LIST_QUERY_KEY_PREFIX,
  normalizeSystemInfoApiBaseUrl,
  useSystemInfoQueryIdentity,
  type SystemInfoQueryIdentity,
} from "./system-info-query";

const BACKUP_LIST_STALE_TIME_MS = 30_000;
const BACKUP_LIST_GC_TIME_MS = 5 * 60_000;
export const BACKUP_LIST_QUERY_KEY_PREFIX = SYSTEM_BACKUP_LIST_QUERY_KEY_PREFIX;

export type BackupListQueryIdentity = SystemInfoQueryIdentity;
export type BackupListQueryKey = ReturnType<typeof createBackupListQueryKey>;

export type BackupListScope = {
  identity: BackupListQueryIdentity;
  queryKey: BackupListQueryKey;
  identityKey: string;
  generation: number;
};

type ScopeState = {
  identityKey: string;
  generation: number;
  mounted: boolean;
};

export function createBackupListQueryKey(identity: BackupListQueryIdentity) {
  return [
    ...BACKUP_LIST_QUERY_KEY_PREFIX,
    normalizeSystemInfoApiBaseUrl(identity.apiBaseUrl),
    identity.bootId ?? null,
    identity.authMode,
    identity.authenticated,
    identity.userId,
  ] as const;
}

export function createBackupListQueryOptions(identity: BackupListQueryIdentity) {
  const queryKey = createBackupListQueryKey(identity);
  return {
    queryKey,
    queryFn: ({ signal }: QueryFunctionContext<BackupListQueryKey>) =>
      fetchBackups({
        baseUrl: identity.apiBaseUrl,
        cache: "no-store",
        init: { signal },
      }),
    staleTime: BACKUP_LIST_STALE_TIME_MS,
    gcTime: BACKUP_LIST_GC_TIME_MS,
    refetchOnMount: "always" as const,
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
    retry: false,
    networkMode: "always" as const,
  };
}

export function useBackupListScope() {
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
  const queryKey = useMemo(() => createBackupListQueryKey(identity), [identity]);
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
    (captured: BackupListScope) =>
      scopeRef.current.mounted &&
      scopeRef.current.identityKey === captured.identityKey &&
      scopeRef.current.generation === captured.generation,
    [],
  );

  return {
    identity,
    queryKey,
    identityKey,
    generation,
    captureScope,
    isCurrentScope,
  };
}

function cancelledScopeError(): DOMException {
  return new DOMException("Backup list request belongs to an obsolete identity", "AbortError");
}

export async function reloadBackupList(
  queryClient: QueryClient,
  scope: BackupListScope,
  isCurrentScope: (scope: BackupListScope) => boolean,
): Promise<SnapshotInfo[]> {
  if (!isCurrentScope(scope)) throw cancelledScopeError();
  return queryClient.fetchQuery({ ...createBackupListQueryOptions(scope.identity), staleTime: 0 });
}

export async function reloadBackupListAfterWrite(
  queryClient: QueryClient,
  scope: BackupListScope,
  isCurrentScope: (scope: BackupListScope) => boolean,
): Promise<SnapshotInfo[]> {
  if (!isCurrentScope(scope)) throw cancelledScopeError();
  await queryClient.cancelQueries({ queryKey: scope.queryKey, exact: true });
  if (!isCurrentScope(scope)) throw cancelledScopeError();
  return reloadBackupList(queryClient, scope, isCurrentScope);
}

export async function invalidateBackupList(
  queryClient: QueryClient,
  identity: BackupListQueryIdentity,
  isCurrent: () => boolean = () => true,
): Promise<void> {
  const queryKey = createBackupListQueryKey(identity);
  await queryClient.cancelQueries({ queryKey, exact: true });
  if (!isCurrent()) return;
  await queryClient.invalidateQueries({ queryKey, exact: true, refetchType: "active" });
}
