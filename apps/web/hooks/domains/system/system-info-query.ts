import { getBackendConfig } from "@/lib/config";
import { useAppStore } from "@/components/state-provider";
import type { AuthMode } from "@/lib/state/slices/auth/types";

export type SystemInfoQueryIdentity = {
  apiBaseUrl: string;
  bootId: string | undefined;
  authMode: AuthMode;
  authenticated: boolean;
  userId: string | null;
};

export const SYSTEM_INFO_QUERY_KEY_PREFIX = ["system", "info"] as const;
export const DISK_USAGE_QUERY_KEY_PREFIX = ["system", "disk-usage"] as const;
export const DATABASE_STATS_QUERY_KEY_PREFIX = ["system", "database-stats"] as const;
export const BACKUP_LIST_QUERY_KEY_PREFIX = ["system", "backups"] as const;

export function normalizeSystemInfoApiBaseUrl(apiBaseUrl: string): string {
  const url = new URL(apiBaseUrl);
  const path = url.pathname.replace(/\/+$/, "");
  return `${url.origin}${path}${url.search}${url.hash}`;
}

export function createSystemInfoQueryKey(identity: SystemInfoQueryIdentity) {
  return [
    ...SYSTEM_INFO_QUERY_KEY_PREFIX,
    normalizeSystemInfoApiBaseUrl(identity.apiBaseUrl),
    identity.bootId ?? null,
    identity.authMode,
    identity.authenticated,
    identity.userId,
  ] as const;
}

export function createDiskUsageQueryKey(identity: SystemInfoQueryIdentity) {
  return [
    ...DISK_USAGE_QUERY_KEY_PREFIX,
    normalizeSystemInfoApiBaseUrl(identity.apiBaseUrl),
    identity.bootId ?? null,
    identity.authMode,
    identity.authenticated,
    identity.userId,
  ] as const;
}

export function createDatabaseStatsQueryKey(identity: SystemInfoQueryIdentity) {
  return [
    ...DATABASE_STATS_QUERY_KEY_PREFIX,
    normalizeSystemInfoApiBaseUrl(identity.apiBaseUrl),
    identity.bootId ?? null,
    identity.authMode,
    identity.authenticated,
    identity.userId,
  ] as const;
}

export function useSystemInfoQueryIdentity(bootId: string | undefined) {
  const authMode = useAppStore((state) => state.auth.mode);
  const authenticated = useAppStore((state) => state.auth.authenticated);
  const userId = useAppStore((state) => state.auth.user?.id ?? null);

  return {
    apiBaseUrl: normalizeSystemInfoApiBaseUrl(getBackendConfig().apiBaseUrl),
    bootId,
    authMode,
    authenticated,
    userId,
  } satisfies SystemInfoQueryIdentity;
}
