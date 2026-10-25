"use client";

import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { previewSavedProfileCommand } from "@/lib/api/domains/settings-api";
import { getBackendConfig } from "@/lib/config";

export function useExecutionCommandPreview(profileId: string) {
  const auth = useAppStore((state) => state.auth);
  const version = useAppStore((state) => state.agentProfiles.version);
  const admin = useIsAdmin();
  const enabled = Boolean(profileId) && admin && (auth.mode === "disabled" || auth.authenticated);
  const identity = [
    getBackendConfig().apiBaseUrl,
    auth.mode,
    auth.authenticated,
    auth.user?.id,
    auth.user?.role,
  ];
  const query = useQuery({
    queryKey: ["execution-command-preview", identity, profileId, version],
    queryFn: ({ signal }) => previewSavedProfileCommand(profileId, { init: { signal } }),
    enabled,
    retry: false,
    gcTime: 0,
  });
  return {
    data: enabled ? query.data?.composition : undefined,
    isPending: enabled && query.isPending,
    isError: enabled && query.isError,
  };
}
