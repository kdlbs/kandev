"use client";

import { useAppStore } from "@/components/state-provider";
import { getBackendConfig } from "@/lib/config";

export function useSandboxAccess() {
  const enabled = useAppStore((state) => state.features.sandboxPolicies);
  const auth = useAppStore((state) => state.auth);
  const identity = JSON.stringify([
    getBackendConfig().apiBaseUrl,
    auth.mode,
    auth.authenticated,
    auth.user?.id,
    auth.user?.role,
  ]);
  const canRead = auth.mode === "enabled" && auth.authenticated && Boolean(auth.user?.id);
  return { enabled, identity, canRead, canEdit: canRead && auth.user?.role === "admin" };
}
