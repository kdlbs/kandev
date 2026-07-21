"use client";

import { useCallback } from "react";
import { getSlackConfig } from "@/lib/api/domains/slack-api";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
  type IntegrationConfigStatus,
} from "../integrations/use-integration-availability";
import { useSlackEnabled } from "./use-slack-enabled";

// Slack stores two secrets (token + cookie) instead of one — the shared
// availability hook only checks `hasSecret`, so adapt by reporting authed
// when *both* halves are present.
function useSlackStatusLoader(workspaceId?: string | null) {
  return useCallback(async (): Promise<IntegrationConfigStatus | null> => {
    const cfg = await getSlackConfig(workspaceId ? { workspaceId } : undefined);
    if (!cfg) return null;
    return {
      hasSecret: cfg.hasToken && cfg.hasCookie,
      lastOk: cfg.lastOk,
    };
  }, [workspaceId]);
}

export function useSlackAuthed(workspaceId?: string | null): boolean {
  return useIntegrationAuthed(useSlackStatusLoader(workspaceId));
}

export function useSlackAvailable(workspaceId?: string | null): boolean {
  const fetchConfig = useSlackStatusLoader(workspaceId);
  return useIntegrationAvailable({
    useEnabled: useSlackEnabled,
    fetchConfig,
  });
}
