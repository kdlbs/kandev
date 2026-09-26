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

export function normalizeSystemInfoApiBaseUrl(apiBaseUrl: string): string {
  const url = new URL(apiBaseUrl);
  const path = url.pathname.replace(/\/+$/, "");
  return `${url.origin}${path}${url.search}${url.hash}`;
}

export function createSystemInfoQueryKey(identity: SystemInfoQueryIdentity) {
  return [
    "system",
    "info",
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
